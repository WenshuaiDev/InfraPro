package identity

import (
	"context"
	"encoding/base32"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/testkeyring"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

func TestMFACryptoVectors(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, v := range []struct {
		at   int64
		code string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}} {
		step, ok := matchTOTPStep(secret, v.code, time.Unix(v.at, 0), nil)
		if !ok || step != v.at/30 {
			t.Fatal("RFC 6238 vector failed", v.at)
		}
		if _, ok = matchTOTPStep(secret, v.code, time.Unix(v.at, 0), &step); ok {
			t.Fatal("consumed time step replayed")
		}
		if _, ok = matchTOTPStep(secret, v.code, time.Unix(v.at+90, 0), nil); ok {
			t.Fatal("out-of-window accepted")
		}
	}
	if _, ok := matchTOTPStep(secret, " 287082", time.Unix(59, 0), nil); ok {
		t.Fatal("non canonical OTP")
	}
	file, cleanup, e := testkeyring.Create()
	if e != nil {
		t.Fatal(e)
	}
	defer cleanup()
	k, e := loadMFAKeyring(file)
	if e != nil {
		t.Fatal(e)
	}
	v, e := k.seal(secret, "account-one", "factor", 2)
	if e != nil {
		t.Fatal(e)
	}
	if got, e := k.open(v, "account-one", "factor", 2); e != nil || got != secret {
		t.Fatal("encrypted round trip failed")
	}
	for _, args := range [][2]string{{"account-two", "factor"}, {"account-one", "challenge:one"}} {
		if _, e = k.open(v, args[0], args[1], 2); e == nil {
			t.Fatal("AAD substitution accepted")
		}
	}
	if _, e = k.open(v, "account-one", "factor", 3); e == nil {
		t.Fatal("version substitution accepted")
	}
	v.Ciphertext[0] ^= 1
	if _, e = k.open(v, "account-one", "factor", 2); e == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if e = os.Chmod(file, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = loadMFAKeyring(file); e == nil {
		t.Fatal("public keyring accepted")
	}
}
func mfaCode(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, e := totp.GenerateCode(secret, at)
	if e != nil {
		t.Fatal("test OTP generation failed")
	}
	return code
}
func enrollMFA(t *testing.T, s *Service, p Principal) (Principal, AuthDelivery, EnrollmentSetup) {
	t.Helper()
	ctx := context.Background()
	status, e := s.MFA(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	mode := "enable"
	if status.State == "enabled" {
		mode = "replace"
	}
	setup, e := s.StartSelfEnrollment(ctx, p, mode, status.FactorVersion, Meta{})
	if e != nil {
		t.Fatal(e)
	}
	d, e := s.ConfirmEnrollment(ctx, &p, "", setup.ID, mfaCode(t, setup.Secret, s.now().Add(-30*time.Second)), Meta{})
	if e != nil {
		t.Fatal(e)
	}
	next, e := s.Authenticate(ctx, d.SessionToken)
	if e != nil {
		t.Fatal(e)
	}
	return next, d, setup
}
func mfaMember(t *testing.T, s *Service, admin Principal, name string) Principal {
	t.Helper()
	ctx := context.Background()
	link, e := s.CreateInvitation(ctx, admin, "", "MFA 合成账号", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.AcceptInvitation(ctx, linkToken(link), name, name, testPassword, Meta{}); e != nil {
		t.Fatal(e)
	}
	p, _ := login(t, s, name, testPassword)
	return p
}
func TestMFAPersonalLifecycle(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, old := login(t, s, "Alice", testPassword)
	p, d, config := enrollMFA(t, s, p)
	if len(d.Result.RecoveryCodes) != 10 {
		t.Fatal("recovery set size")
	}
	if _, e := s.Authenticate(ctx, old); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("old session survived")
	}
	start, e := s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil || start.Result.Status != "mfa_required" || start.SessionToken != "" {
		t.Fatal("MFA password-only login")
	}
	if _, e = s.Authenticate(ctx, start.ChallengeToken); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("challenge became session")
	}
	verified, e := s.CompleteLogin(ctx, start.ChallengeToken, start.Result.ID, "totp", mfaCode(t, config.Secret, s.now()), Meta{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteLogin(ctx, start.ChallengeToken, start.Result.ID, "totp", mfaCode(t, config.Secret, s.now()), Meta{}); !errors.Is(e, ErrChallengeCompleted) {
		t.Fatal("challenge replay")
	}
	status, e := s.MFA(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	command := uuid.NewString()
	codes, e := s.RegenerateRecoveryCodes(ctx, p, status.FactorVersion, status.RecoveryGeneration, command, Meta{})
	if e != nil || len(codes.RecoveryCodes) != 10 {
		t.Fatal("regeneration failed", e)
	}
	replay, e := s.RegenerateRecoveryCodes(ctx, p, status.FactorVersion, status.RecoveryGeneration, command, Meta{})
	if e != nil || !replay.SecretUnavailable || len(replay.RecoveryCodes) != 0 {
		t.Fatal("secret command replay", e)
	}
	if _, e = s.Authenticate(ctx, verified.SessionToken); e != nil {
		t.Fatal("regeneration revoked valid session", e)
	}
	next, _, _ := enrollMFA(t, s, p)
	if _, e = s.Authenticate(ctx, verified.SessionToken); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("replacement preserved old session")
	}
	status, e = s.MFA(ctx, next)
	if e != nil {
		t.Fatal(e)
	}
	disabled, delivery, e := s.DisableMFA(ctx, next, status.FactorVersion, uuid.NewString(), Meta{})
	if e != nil || disabled.State != "disabled" {
		t.Fatal("disable failed", e)
	}
	if p, e = s.Authenticate(ctx, delivery.SessionToken); e != nil || p.Session.AuthenticationMethod != "password" {
		t.Fatal("disable rotation failed", e)
	}
	login(t, s, "Alice", testPassword)
}
func TestMFAPolicyGenerationAndNoStepUp(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, old := login(t, s, "Alice", testPassword)
	// Populate the shared Redis session cache on both instances before changing policy.
	other, e := NewService(x.pool, x.redis, x.cfg, slog.Default())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = other.Authenticate(ctx, old); e != nil {
		t.Fatal(e)
	}
	policy, e := s.Policy(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := s.UpdatePolicy(ctx, p, true, policy.Revision, uuid.NewString(), Meta{Reason: "隔离验证"})
	if e != nil || saved.SessionAction != "sign_in_again" {
		t.Fatal("unbound administrator policy save", e)
	}
	if saved.Policy.UpdatedBy != p.Account.ID || saved.Policy.UpdatedByLabel == "" {
		t.Fatal("policy omitted its accountable updater")
	}
	if _, e = other.Authenticate(ctx, old); !errors.Is(e, ErrMFALogin) {
		t.Fatal("cached pre-policy session accepted", e)
	}
	d, e := s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil || d.Result.Status != "mfa_enrollment_required" {
		t.Fatal("required enrollment", e)
	}
	setup, e := s.StartLoginEnrollment(ctx, d.ChallengeToken, d.Result.ID, Meta{})
	if e != nil {
		t.Fatal(e)
	}
	d, e = s.ConfirmEnrollment(ctx, nil, d.ChallengeToken, d.Result.ID, mfaCode(t, setup.Secret, s.now().Add(-30*time.Second)), Meta{})
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.Authenticate(ctx, d.SessionToken)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.DisableMFA(ctx, p, "2", uuid.NewString(), Meta{}); !errors.Is(e, ErrMFAPolicy) {
		t.Fatal("forced disable accepted", e)
	}
	if _, e = x.pool.Exec(ctx, `UPDATE account_sessions SET mfa_verified_at=now()-interval '1 hour',reauthenticated_at=now()-interval '6 minutes' WHERE id=$1`, p.Session.ID); e != nil {
		t.Fatal(e)
	}
	if p, e = s.Authenticate(ctx, d.SessionToken); e != nil {
		t.Fatal("MFA proof incorrectly expires", e)
	}
	if _, e = s.UpdatePolicy(ctx, p, false, saved.Policy.Revision, uuid.NewString(), Meta{Reason: "结束验证"}); !errors.Is(e, ErrReauth) {
		t.Fatal("password recency missing", e)
	}
	if e = s.Reauthenticate(ctx, p, testPassword, Meta{}); e != nil {
		t.Fatal(e)
	}
	saved, e = s.UpdatePolicy(ctx, p, false, saved.Policy.Revision, uuid.NewString(), Meta{Reason: "结束验证"})
	if e != nil || saved.SessionAction != "keep" {
		t.Fatal("MFA session should remain", e)
	}
	if _, e = other.Authenticate(ctx, old); !errors.Is(e, ErrMFALogin) && !errors.Is(e, ErrUnauthorized) {
		t.Fatal("policy disable revived old session", e)
	}
	if _, e = other.Authenticate(ctx, d.SessionToken); e != nil {
		t.Fatal(e)
	}
}
func TestMFARecoveryAndReset(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	admin, _ := login(t, s, "Alice", testPassword)
	user := mfaMember(t, s, admin, "MfaUser")
	user, bound, _ := enrollMFA(t, s, user)
	d, e := s.BeginLogin(ctx, "MfaUser", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	recovery, e := s.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, "recovery_code", bound.Result.RecoveryCodes[0], Meta{})
	if e != nil {
		t.Fatal(e)
	}
	proof, e := s.Authenticate(ctx, recovery.SessionToken)
	if e != nil || proof.Session.AuthenticationMethod != "password_recovery_code" {
		t.Fatal("recovery login proof", e)
	}
	status, e := s.AccountMFA(ctx, admin, user.Account.ID)
	if e != nil {
		t.Fatal(e)
	}
	cmd := uuid.NewString()
	reset, e := s.ResetMFA(ctx, admin, user.Account.ID, status.FactorVersion, cmd, Meta{Reason: "本人身份已核验"})
	if e != nil || reset.RecoveryURL == "" {
		t.Fatal("admin reset", e)
	}
	replay, e := s.ResetMFA(ctx, admin, user.Account.ID, status.FactorVersion, cmd, Meta{Reason: "本人身份已核验"})
	if e != nil || !replay.SecretUnavailable || replay.RecoveryURL != "" {
		t.Fatal("reset replay", e)
	}
	if _, e = s.Authenticate(ctx, recovery.SessionToken); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("reset session survived")
	}
	d, e = s.BeginLogin(ctx, "MfaUser", testPassword, "", Meta{})
	if e != nil || d.Result.Status != "mfa_recovery_required" {
		t.Fatal("optional reset bypass", e)
	}
	grant := linkToken(Link{URL: reset.RecoveryURL})
	claimed, e := s.ClaimMFARecovery(ctx, d.ChallengeToken, d.Result.ID, grant, Meta{})
	if e != nil || claimed.Result.Status != "mfa_enrollment_required" {
		t.Fatal("claim failed", e)
	}
	challenge, e := s.Challenge(ctx, d.ChallengeToken)
	if e != nil || challenge.Status != "mfa_enrollment_required" {
		t.Fatal("claim refresh", e)
	}
	setup, e := s.StartLoginEnrollment(ctx, d.ChallengeToken, d.Result.ID, Meta{})
	if e != nil {
		t.Fatal(e)
	}
	completed, e := s.ConfirmEnrollment(ctx, nil, d.ChallengeToken, d.Result.ID, mfaCode(t, setup.Secret, s.now().Add(-30*time.Second)), Meta{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, completed.SessionToken); e != nil {
		t.Fatal(e)
	}
	if _, e = s.ResetMFA(ctx, admin, admin.Account.ID, "1", uuid.NewString(), Meta{Reason: "self"}); PublicError(e).Code != "self_mfa_reset_forbidden" {
		t.Fatal("administrative self-reset accepted")
	}
}
func TestMFAConcurrentConsumptionAndDurableFailures(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, _ := login(t, s, "Alice", testPassword)
	_, bound, config := enrollMFA(t, s, p)
	other, e := NewService(x.pool, x.redis, x.cfg, slog.Default())
	if e != nil {
		t.Fatal(e)
	}
	for _, method := range []string{"totp", "recovery_code"} {
		first, e := s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
		if e != nil {
			t.Fatal(e)
		}
		second, e := other.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
		if e != nil {
			t.Fatal(e)
		}
		code := mfaCode(t, config.Secret, s.now())
		if method == "recovery_code" {
			code = bound.Result.RecoveryCodes[0]
		}
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i, d := range []AuthDelivery{first, second} {
			wg.Add(1)
			go func(i int, d AuthDelivery) {
				defer wg.Done()
				node := s
				if i == 1 {
					node = other
				}
				_, e := node.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, method, code, Meta{})
				results <- e
			}(i, d)
		}
		wg.Wait()
		close(results)
		success := 0
		for e := range results {
			if e == nil {
				success++
			} else if !errors.Is(e, ErrMFACode) {
				t.Fatal(e)
			}
		}
		if success != 1 {
			t.Fatal("one-time credential did not have exactly one consumer", success)
		}
	}
	d, e := s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	// Successful MFA resets only this account's MFA failure range.
	if _, e = s.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, "recovery_code", bound.Result.RecoveryCodes[1], Meta{}); e != nil {
		t.Fatal(e)
	}
	d, e = s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	for range 5 {
		_, e = s.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, "totp", "invalid", Meta{})
		requireCode(t, e, "mfa_code_invalid")
	}
	d, e = other.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	_, e = other.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, "recovery_code", bound.Result.RecoveryCodes[2], Meta{})
	requireCode(t, e, "rate_limited")
	if PublicError(e).RetryAfter < 1 || PublicError(e).RetryAfter > 31 {
		t.Fatal("inaccurate cooldown")
	}
	var failures int
	if e = x.pool.QueryRow(ctx, `SELECT failures FROM account_rate_limits WHERE key_hash=$1`, digest("mfa:"+p.Account.ID)).Scan(&failures); e != nil || failures < 5 {
		t.Fatal("failure transaction rolled back", e)
	}
}
