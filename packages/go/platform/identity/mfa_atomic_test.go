package identity

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

func TestMFARollbackReceiptsAndWireUnion(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, _ := login(t, s, "Alice", testPassword)
	p, bound, setup := enrollMFA(t, s, p)
	d, e := s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	// Failure at the durable audit boundary must undo code consumption, challenge
	// completion and session creation, even after the credential was correct.
	_, e = x.pool.Exec(ctx, `CREATE FUNCTION reject_mfa_login() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic transaction failure'; END $$; CREATE TRIGGER reject_mfa_login BEFORE INSERT ON account_security_events FOR EACH ROW WHEN (NEW.event_type='login' AND NEW.outcome='success') EXECUTE FUNCTION reject_mfa_login()`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, "recovery_code", bound.Result.RecoveryCodes[0], Meta{}); e == nil {
		t.Fatal("audit failure ignored")
	}
	var used int
	var state string
	if e = x.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM account_mfa_recovery_codes WHERE account_id=$1 AND used_at IS NOT NULL),state FROM account_auth_challenges WHERE id=$2`, p.Account.ID, d.Result.ID).Scan(&used, &state); e != nil || used != 0 || state != "pending" {
		t.Fatal("credential consumption escaped rollback", e)
	}
	if _, e = x.pool.Exec(ctx, `DROP TRIGGER reject_mfa_login ON account_security_events; DROP FUNCTION reject_mfa_login()`); e != nil {
		t.Fatal(e)
	}
	complete, e := s.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, "recovery_code", bound.Result.RecoveryCodes[0], Meta{})
	if e != nil {
		t.Fatal(e)
	}
	encoded, e := json.Marshal(complete.Result)
	if e != nil {
		t.Fatal(e)
	}
	var wire map[string]any
	_ = json.Unmarshal(encoded, &wire)
	if len(wire) != 3 || wire["status"] != "authenticated" || wire["account"] == nil {
		t.Fatal("authenticated wire union leaked challenge fields")
	}
	command := uuid.NewString()
	fresh, e := s.RegenerateRecoveryCodes(ctx, p, "2", "1", command, Meta{})
	if e != nil {
		t.Fatal(e)
	}
	replay, e := s.RegenerateRecoveryCodes(ctx, p, "2", "1", command, Meta{})
	if e != nil || !replay.SecretUnavailable || len(replay.RecoveryCodes) != 0 || replay.RecoveryGeneration != fresh.RecoveryGeneration {
		t.Fatal("secret replay", e)
	}
	var receipts string
	if e = x.pool.QueryRow(ctx, `SELECT COALESCE((SELECT string_agg(row_to_json(r)::text,'') FROM audit_command_receipts r),'')||COALESCE((SELECT string_agg(row_to_json(r)::text,'') FROM account_security_events r),'')||COALESCE((SELECT string_agg(row_to_json(r)::text,'') FROM notification_events r),'')`).Scan(&receipts); e != nil {
		t.Fatal(e)
	}
	for _, secret := range append(append(bound.Result.RecoveryCodes, fresh.RecoveryCodes...), setup.Secret) {
		if strings.Contains(receipts, secret) {
			t.Fatal("secret retained in receipt")
		}
	}
	var notifications int
	if e = x.pool.QueryRow(ctx, `SELECT count(*) FROM notification_events WHERE source_module='identity' AND event_type='security.changed' AND outcome IN ('mfa_enabled','mfa_recovery_codes_regenerated')`).Scan(&notifications); e != nil || notifications != 2 {
		t.Fatal("MFA configuration changes did not create one notification fact each", e)
	}
}

func TestMFAGrantExpiryAccountAndGeneration(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	admin, _ := login(t, s, "Alice", testPassword)
	a := mfaMember(t, s, admin, "MfaGrantA")
	b := mfaMember(t, s, admin, "MfaGrantB")
	a, _, _ = enrollMFA(t, s, a)
	b, _, _ = enrollMFA(t, s, b)
	reset := func(p Principal, version string) MFAResult {
		t.Helper()
		v, e := s.ResetMFA(ctx, admin, p.Account.ID, version, uuid.NewString(), Meta{Reason: "合成身份核验"})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	first := reset(a, "2")
	_ = reset(b, "2")
	start := func(user string) AuthDelivery {
		t.Helper()
		d, e := s.BeginLogin(ctx, user, testPassword, "", Meta{})
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	other := start("MfaGrantB")
	if _, e := s.ClaimMFARecovery(ctx, other.ChallengeToken, other.Result.ID, linkToken(Link{URL: first.RecoveryURL}), Meta{}); !errors.Is(e, ErrMFACode) {
		t.Fatal("cross-account grant", e)
	}
	second := reset(a, first.FactorVersion)
	d := start("MfaGrantA")
	if _, e := s.ClaimMFARecovery(ctx, d.ChallengeToken, d.Result.ID, linkToken(Link{URL: first.RecoveryURL}), Meta{}); !errors.Is(e, ErrMFACode) {
		t.Fatal("superseded grant", e)
	}
	if _, e := x.pool.Exec(ctx, `UPDATE account_auth_challenges SET expires_at=now()+interval '1 second' WHERE account_id=$1 AND purpose='admin_reset_grant' AND state='pending'`, a.Account.ID); e != nil {
		t.Fatal(e)
	}
	s.now = func() time.Time { return time.Now().Add(2 * time.Second) }
	if _, e := s.ClaimMFARecovery(ctx, d.ChallengeToken, d.Result.ID, linkToken(Link{URL: second.RecoveryURL}), Meta{}); !errors.Is(e, ErrMFACode) {
		t.Fatal("expired grant", e)
	}
	s.now = time.Now
	third := reset(a, second.FactorVersion)
	d = start("MfaGrantA")
	token := linkToken(Link{URL: third.RecoveryURL})
	if _, e := s.ClaimMFARecovery(ctx, d.ChallengeToken, d.Result.ID, token, Meta{}); e != nil {
		t.Fatal(e)
	}
	another := start("MfaGrantA")
	if _, e := s.ClaimMFARecovery(ctx, another.ChallengeToken, another.Result.ID, token, Meta{}); !errors.Is(e, ErrMFACode) {
		t.Fatal("grant reused", e)
	}
}

func TestMFARegenerationConsumptionRace(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, _ := login(t, s, "Alice", testPassword)
	p, bound, _ := enrollMFA(t, s, p)
	d, e := s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	errs := make(chan error, 2)
	go func() {
		_, e := s.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, "recovery_code", bound.Result.RecoveryCodes[0], Meta{})
		errs <- e
	}()
	go func() { _, e := s.RegenerateRecoveryCodes(ctx, p, "2", "1", uuid.NewString(), Meta{}); errs <- e }()
	for range 2 {
		if e := <-errs; e != nil && !errors.Is(e, ErrMFACode) {
			t.Fatal(e)
		}
	}
	var invalid, current int
	if e = x.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE generation=1 AND revoked_at IS NULL),count(*) FILTER(WHERE generation=2 AND revoked_at IS NULL AND used_at IS NULL) FROM account_mfa_recovery_codes WHERE account_id=$1`, p.Account.ID).Scan(&invalid, &current); e != nil || invalid != 0 || current != 10 {
		t.Fatal("revoked generation revived", e)
	}
	d, e = s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, "recovery_code", bound.Result.RecoveryCodes[0], Meta{}); !errors.Is(e, ErrMFACode) {
		t.Fatal("old generation accepted", e)
	}
}
