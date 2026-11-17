package identity_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
)

func TestMFAMigrationAndRestrictedRoles(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:mfa")
	}
	ctx := context.Background()
	env, e := testsupport.NewAtMigration(ctx, slog.Default(), 55)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	var id string
	e = env.Pool.QueryRow(ctx, `INSERT INTO accounts(username,username_normalized,display_name) VALUES('LegacyMfa','legacymfa','旧版合成账号') RETURNING id::text`).Scan(&id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = env.Pool.Exec(ctx, `INSERT INTO account_sessions(token_hash,account_id,auth_version,expires_at,user_agent,ip) VALUES('synthetic-legacy-session',$1,1,now()+interval '1 day','fixture','loopback')`, id); e != nil {
		t.Fatal(e)
	}
	if e = migrations.Up(ctx, env.Config.DatabaseURL); e != nil {
		t.Fatal(e)
	}
	var state, method string
	var required, empty bool
	var generation int
	e = env.Pool.QueryRow(ctx, `SELECT f.state,p.require_mfa,s.authentication_method,s.policy_enforcement_generation,f.key_id IS NULL AND f.secret_ciphertext IS NULL AND s.mfa_verified_at IS NULL FROM account_mfa f CROSS JOIN platform_security_policy p JOIN account_sessions s ON true WHERE f.account_id=$1 AND s.account_id=$1`, id).Scan(&state, &required, &method, &generation, &empty)
	if e != nil || state != "disabled" || required || method != "password" || generation != 0 || !empty {
		t.Fatal("unsafe upgrade backfill", e)
	}
	for _, sql := range []string{
		`UPDATE account_mfa SET state='enabled',secret_ciphertext=decode(repeat('00',32),'hex'),key_id='invalid',enabled_at=now()`,
		`UPDATE account_sessions SET authentication_method='password_totp',mfa_verified_at=now()`,
		`INSERT INTO platform_security_policy(id) VALUES(2)`,
		`UPDATE platform_security_policy SET enforcement_generation=-1`,
	} {
		if _, e = env.Pool.Exec(ctx, sql); e == nil {
			t.Fatal("invalid authentication state accepted")
		}
	}
	for _, role := range []string{"infrapro_audit_worker", "infrapro_files_worker"} {
		var exists bool
		if e = env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role).Scan(&exists); e != nil {
			t.Fatal(e)
		}
		if !exists {
			t.Fatal("fixture restricted role missing", role)
		}
		for _, table := range []string{"account_mfa", "account_mfa_recovery_codes", "account_auth_challenges"} {
			tx, e := env.Pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = tx.Exec(ctx, "SET LOCAL ROLE "+role); e != nil {
				t.Fatal(e)
			}
			_, e = tx.Exec(ctx, "SELECT * FROM "+table+" LIMIT 1")
			_ = tx.Rollback(ctx)
			if e == nil {
				t.Fatal("worker can read MFA credentials", role, table)
			}
		}
	}
}
