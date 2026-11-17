package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

func TestMFARotationCleanupAndRestore(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, _ := login(t, s, "Alice", testPassword)
	p, bound, oldFactor := enrollMFA(t, s, p)
	pending, e := s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	self, e := s.StartSelfEnrollment(ctx, p, "replace", "2", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(x.cfg.MFAKeyringFile)
	if e != nil {
		t.Fatal(e)
	}
	var file struct {
		Format int                 `json:"format_version"`
		Active string              `json:"active_key_id"`
		Keys   []map[string]string `json:"keys"`
	}
	if e = json.Unmarshal(raw, &file); e != nil {
		t.Fatal(e)
	}
	clear(raw)
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 17)
	}
	file.Keys = append(file.Keys, map[string]string{"id": "next", "key_base64": base64.StdEncoding.EncodeToString(key)})
	file.Active = "next"
	raw, _ = json.Marshal(file)
	if e = os.WriteFile(x.cfg.MFAKeyringFile, raw, 0600); e != nil {
		t.Fatal(e)
	}
	clear(raw)
	next, e := NewService(x.pool, x.redis, x.cfg, slog.Default())
	if e != nil {
		t.Fatal(e)
	}
	// Mixed keyrings fail readiness until all nodes have the new decryptor.
	if n, e := next.RotateMFAKeys(ctx); e != nil || n != 2 {
		t.Fatal("rotation batch", n, e)
	}
	if e = s.Ready(ctx); e == nil {
		t.Fatal("missing referenced key accepted")
	}
	if n, e := next.RotateMFAKeys(ctx); e != nil || n != 0 {
		t.Fatal("rotation not resumable", n, e)
	}
	if _, e = next.Authenticate(ctx, bound.SessionToken); e != nil {
		t.Fatal("rotation invalidated session", e)
	}
	resumed, e := next.StartSelfEnrollment(ctx, p, "replace", "2", Meta{})
	if e != nil || resumed.ID != self.ID || resumed.Secret != self.Secret {
		t.Fatal("rotation altered pending configuration", e)
	}
	if e = next.CancelSelfEnrollment(ctx, p, self.ID); e != nil {
		t.Fatal(e)
	}
	login, e := next.CompleteLogin(ctx, pending.ChallengeToken, pending.Result.ID, "recovery_code", bound.Result.RecoveryCodes[0], Meta{})
	if e != nil {
		t.Fatal(e)
	}
	// Simulate rollback of an already consumed code, old factor and weak policy.
	// Restore revocation must destroy all three restored access capabilities.
	if _, e = x.pool.Exec(ctx, `UPDATE account_mfa_recovery_codes SET used_at=NULL WHERE account_id=$1`, p.Account.ID); e != nil {
		t.Fatal(e)
	}
	if e = next.RestoreRevoke(ctx, Meta{Reason: "隔离的时间点恢复演练"}); e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{bound.SessionToken, login.SessionToken} {
		if _, e = next.Authenticate(ctx, token); !errors.Is(e, ErrUnauthorized) {
			t.Fatal("restored session revived", e)
		}
	}
	var state string
	var required bool
	var remaining, keys int
	if e = x.pool.QueryRow(ctx, `SELECT state,(SELECT require_mfa FROM platform_security_policy WHERE id=1),(SELECT count(*) FROM account_mfa_recovery_codes WHERE revoked_at IS NULL),(SELECT count(*) FROM account_mfa WHERE key_id IS NOT NULL) FROM account_mfa WHERE account_id=$1`, p.Account.ID).Scan(&state, &required, &remaining, &keys); e != nil {
		t.Fatal(e)
	}
	if state != "reset_required" || !required || remaining != 0 || keys != 0 {
		t.Fatal("snapshot weakened restored protection")
	}
	grant, e := next.MaintenanceResetMFA(ctx, "Alice", "合成维护员", Meta{Reason: "恢复唯一管理员"})
	if e != nil {
		t.Fatal(e)
	}
	d, e := next.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil || d.Result.Status != "mfa_recovery_required" {
		t.Fatal("maintenance bypass", e)
	}
	d, e = next.ClaimMFARecovery(ctx, d.ChallengeToken, d.Result.ID, linkToken(Link{URL: grant.RecoveryURL}), Meta{})
	if e != nil {
		t.Fatal(e)
	}
	setup, e := next.StartLoginEnrollment(ctx, d.ChallengeToken, d.Result.ID, Meta{})
	if e != nil || setup.Secret == oldFactor.Secret {
		t.Fatal("old factor restored", e)
	}
	if _, e = next.ConfirmEnrollment(ctx, nil, d.ChallengeToken, d.Result.ID, mfaCode(t, setup.Secret, time.Now().Add(-30*time.Second)), Meta{}); e != nil {
		t.Fatal(e)
	}
	// Expired ciphertext is cleared in a bounded maintenance batch, not retained.
	if _, e = x.pool.Exec(ctx, `UPDATE account_auth_challenges SET expires_at=created_at+interval '1 second',created_at=created_at-interval '1 hour' WHERE state='pending'`); e != nil {
		t.Fatal(e)
	}
	if e = next.repo.CleanupMFA(ctx); e != nil {
		t.Fatal(e)
	}
}

type failMFASetHook struct{}

func (failMFASetHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (failMFASetHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (failMFASetHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "set" {
			return errors.New("synthetic cache write failure")
		}
		return next(ctx, cmd)
	}
}
func TestMFACacheFailureAndMissingState(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, _ := login(t, s, "Alice", testPassword)
	setup, e := s.StartSelfEnrollment(ctx, p, "enable", "1", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	client := redis.NewClient(x.redis.Options())
	defer client.Close()
	client.AddHook(failMFASetHook{})
	broken, e := NewService(x.pool, client, x.cfg, slog.Default())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = broken.ConfirmEnrollment(ctx, &p, "", setup.ID, mfaCode(t, setup.Secret, time.Now().Add(-30*time.Second)), Meta{}); !errors.Is(e, ErrUnavailable) {
		t.Fatal("undelivered session not failed closed", e)
	}
	var state string
	var sessions int
	e = x.pool.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM account_sessions WHERE account_id=$1 AND revoked_at IS NULL) FROM account_mfa WHERE account_id=$1`, p.Account.ID).Scan(&state, &sessions)
	if e != nil || state != "enabled" || sessions != 0 {
		t.Fatal("cache failure did not revoke undispatched session", e)
	}
	d, e := s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil || d.Result.Status != "mfa_required" {
		t.Fatal("cache failure downgraded MFA", e)
	}
	if _, e = x.pool.Exec(ctx, `DELETE FROM platform_security_policy`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.BeginLogin(ctx, "Alice", testPassword, "", Meta{}); !errors.Is(e, ErrUnavailable) {
		t.Fatal("missing policy defaulted to optional", e)
	}
}

func TestMFAPasswordResetRevocationAndAccountIsolation(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	admin, _ := login(t, s, "Alice", testPassword)
	p := mfaMember(t, s, admin, "MfaMember")
	p, _, _ = enrollMFA(t, s, p)
	before, e := s.BeginLogin(ctx, "MfaMember", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	link, e := s.ResetLink(ctx, "MfaMember", Meta{Reason: "密码恢复"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CompleteReset(ctx, linkToken(link), "Changed MFA password 592!", Meta{}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteLogin(ctx, before.ChallengeToken, before.Result.ID, "totp", "123456", Meta{}); !errors.Is(e, ErrChallenge) {
		t.Fatal("password change challenge survived", e)
	}
	after, e := s.BeginLogin(ctx, "MfaMember", "Changed MFA password 592!", "", Meta{})
	if e != nil || after.Result.Status != "mfa_required" {
		t.Fatal("password recovery removed MFA", e)
	}
	if e = s.SetStatus(ctx, "MfaMember", false, false, Meta{Reason: "停用"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteLogin(ctx, after.ChallengeToken, after.Result.ID, "totp", "123456", Meta{}); !errors.Is(e, ErrChallenge) {
		t.Fatal("disabled challenge survived", e)
	}
	status, e := s.AccountMFA(ctx, admin, p.Account.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ResetMFA(ctx, admin, p.Account.ID, status.FactorVersion, uuid.NewString(), Meta{Reason: "停用账号恢复准备"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.BeginLogin(ctx, "MfaMember", "Changed MFA password 592!", "", Meta{}); !errors.Is(e, ErrCredentials) {
		t.Fatal("reset re-enabled disabled account", e)
	}
	var details string
	e = x.pool.QueryRow(ctx, `SELECT string_agg(details::text||safe_context::text,'') FROM account_security_events WHERE event_type LIKE 'mfa_%'`).Scan(&details)
	if e != nil {
		t.Fatal(e)
	}
	for _, forbidden := range []string{"secret_ciphertext", "qr_png", "recovery_url", "code_hash", "new_totp_code", "token_hash"} {
		if strings.Contains(details, forbidden) {
			t.Fatal("secret field in audit", forbidden)
		}
	}
}

func TestMFADisplayRuleAndPolicyToggleRace(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	admin, old := login(t, s, "Alice", testPassword)
	user := mfaMember(t, s, admin, "MfaRace")
	user, _, _ = enrollMFA(t, s, user)
	check := func() error {
		return s.repo.transact(ctx, func(tx pgx.Tx) error {
			return CheckDisplaySessionTx(ctx, tx, admin.Account.ID, admin.Session.ID, s.cfg.SessionIdle)
		})
	}
	if e := check(); e != nil {
		t.Fatal(e)
	}
	errorsOut := make(chan error, 2)
	go func() {
		_, e := s.UpdatePolicy(ctx, admin, true, "1", uuid.NewString(), Meta{Reason: "并发策略"})
		errorsOut <- e
	}()
	go func() { _, _, e := s.DisableMFA(ctx, user, "2", uuid.NewString(), Meta{}); errorsOut <- e }()
	for range 2 {
		if e := <-errorsOut; e != nil && !errors.Is(e, ErrMFAPolicy) {
			t.Fatal("policy/disable race failed", e)
		}
	}
	if e := check(); !errors.Is(e, ErrMFALogin) {
		t.Fatal("person display accepted old session", e)
	}
	if _, e := s.Authenticate(ctx, old); !errors.Is(e, ErrMFALogin) {
		t.Fatal(e)
	}
	// Existing business transactions also use the same proof predicate.
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		return s.AccessActor(admin).Check(ctx, tx, false)
	})
	if !errors.Is(e, ErrMFALogin) {
		t.Fatal("business actor bypass", e)
	}
}
