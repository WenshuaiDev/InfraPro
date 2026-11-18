package tenancy_test

import (
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/pquerna/otp/totp"
)

func recoverAfterRestore(t *testing.T, env *testsupport.Environment, username string) identity.Principal {
	t.Helper()
	s := env.Service
	grant, e := s.MaintenanceResetMFA(ctx, username, "隔离恢复演练", identity.Meta{Reason: "备份恢复后验证原租户关系"})
	must(t, e)
	login, e := s.BeginLogin(ctx, username, password, "", identity.Meta{})
	must(t, e)
	if login.Result.Status != "mfa_recovery_required" {
		t.Fatal("restore allowed a password-only login")
	}
	login, e = s.ClaimMFARecovery(ctx, login.ChallengeToken, login.Result.ID, strings.Split(grant.RecoveryURL, "#token=")[1], identity.Meta{})
	must(t, e)
	setup, e := s.StartLoginEnrollment(ctx, login.ChallengeToken, login.Result.ID, identity.Meta{})
	must(t, e)
	code, e := totp.GenerateCode(setup.Secret, time.Now().Add(-30*time.Second))
	must(t, e)
	delivery, e := s.ConfirmEnrollment(ctx, nil, login.ChallengeToken, login.Result.ID, code, identity.Meta{})
	must(t, e)
	principal, e := s.Authenticate(ctx, delivery.SessionToken)
	must(t, e)
	return principal
}
