package identity

import (
	"bytes"
	"context"
	"errors"
	"github.com/google/uuid"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"
)

// A real pg_dump/psql rollback of only the random fixture schema. The dump is
// memory-only, and subprocess output is deliberately never attached to failures.
func TestMFAPostgresSnapshotRestore(t *testing.T) {
	if os.Getenv("INFRAPRO_MFA_RESTORE") != "1" {
		t.Skip("run pnpm test:mfa")
	}
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, _ := login(t, s, "Alice", testPassword)
	p, bound, oldFactor := enrollMFA(t, s, p)
	u, _ := url.Parse(x.cfg.DatabaseURL)
	schema := u.Query().Get("search_path")
	if u.Host != "127.0.0.1:55432" || u.Path != "/infrapro_test" || !regexp.MustCompile(`^identity_test_[a-z0-9]+$`).MatchString(schema) {
		t.Fatal("restore fixture boundary")
	}
	dump := exec.CommandContext(ctx, "docker", "exec", "infrapro-account-tests-postgres-1", "pg_dump", "-U", "infrapro_test", "-d", "infrapro_test", "--schema", schema, "--clean", "--if-exists", "--no-owner", "--no-acl")
	raw, e := dump.Output()
	if e != nil {
		t.Fatal("isolated pg_dump failed")
	}
	defer clear(raw)
	d, e := s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	used, e := s.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, "recovery_code", bound.Result.RecoveryCodes[0], Meta{})
	if e != nil {
		t.Fatal(e)
	}
	current, e := s.Authenticate(ctx, used.SessionToken)
	if e != nil {
		t.Fatal(e)
	}
	current, latest, nextFactor := enrollMFA(t, s, current)
	if nextFactor.Secret == oldFactor.Secret {
		t.Fatal("replacement fixture")
	}
	if _, e = s.UpdatePolicy(ctx, current, true, "1", uuid.NewString(), Meta{Reason: "备份后开启强制"}); e != nil {
		t.Fatal(e)
	}
	restore := exec.CommandContext(ctx, "docker", "exec", "-i", "infrapro-account-tests-postgres-1", "psql", "-X", "-q", "-v", "ON_ERROR_STOP=1", "-U", "infrapro_test", "-d", "infrapro_test")
	restore.Stdin = bytes.NewReader(raw)
	if _, e = restore.CombinedOutput(); e != nil {
		t.Fatal("isolated snapshot restore failed")
	}
	x.pool.Reset()
	var required bool
	var consumed int
	if e = x.pool.QueryRow(ctx, `SELECT require_mfa,(SELECT count(*) FROM account_mfa_recovery_codes WHERE used_at IS NOT NULL) FROM platform_security_policy`).Scan(&required, &consumed); e != nil || required || consumed != 0 {
		t.Fatal("snapshot did not reproduce all rollback cases", e)
	}
	if e = s.RestoreRevoke(ctx, Meta{Reason: "真实快照恢复后撤销不可信人员访问"}); e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{bound.SessionToken, used.SessionToken, latest.SessionToken} {
		if _, e = s.Authenticate(ctx, token); !errors.Is(e, ErrUnauthorized) {
			t.Fatal("restored cookie accepted", e)
		}
	}
	d, e = s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil || d.Result.Status != "mfa_recovery_required" {
		t.Fatal("old snapshot weakened authentication", e)
	}
	for _, method := range []string{"totp", "recovery_code"} {
		code := bound.Result.RecoveryCodes[0]
		if method == "totp" {
			code = mfaCode(t, oldFactor.Secret, time.Now())
		}
		if _, e = s.CompleteLogin(ctx, d.ChallengeToken, d.Result.ID, method, code, Meta{}); !errors.Is(e, ErrChallenge) {
			t.Fatal("restored old factor accepted", e)
		}
	}
	var secrets, codes, grants int
	if e = x.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM account_mfa WHERE key_id IS NOT NULL),(SELECT count(*) FROM account_mfa_recovery_codes WHERE revoked_at IS NULL),(SELECT count(*) FROM account_auth_challenges WHERE purpose='admin_reset_grant' AND state='pending')`).Scan(&secrets, &codes, &grants); e != nil || secrets+codes+grants != 0 {
		t.Fatal("restore left reusable credentials", e)
	}
}
