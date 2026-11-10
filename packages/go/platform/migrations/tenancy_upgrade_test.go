package migrations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

func TestTenantUpgradePreservesLegacyAndRollsBackIncompatibility(t *testing.T) {
	raw := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("run pnpm test:identity")
	}
	u, e := url.Parse(raw)
	if e != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("test database required")
	}
	ctx := context.Background()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	admin, e := pgxpool.New(ctx, raw)
	must(e)
	defer admin.Close()
	schema := "tenant_upgrade_" + strings.ToLower(rand.Text())
	_, e = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	must(e)
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, e := sql.Open("pgx", u.String())
	must(e)
	defer db.Close()
	provider, e := goose.NewProvider(goose.DialectPostgres, db, files)
	must(e)
	_, e = provider.UpTo(ctx, 2)
	must(e)
	pool, e := pgxpool.New(ctx, u.String())
	must(e)
	defer pool.Close()
	exec := func(query string, args ...any) { t.Helper(); _, e := pool.Exec(ctx, query, args...); must(e) }
	var account, custom string
	must(pool.QueryRow(ctx, `INSERT INTO accounts(username,username_normalized,display_name) VALUES('Legacy','legacy','旧库账号') RETURNING id::text`).Scan(&account))
	exec(`INSERT INTO account_passwords(account_id,password_hash) VALUES($1,'legacy-test-hash')`, account)
	exec(`UPDATE system_initialization SET first_account_id=$1,completed_at=now()`, account)
	exec(`INSERT INTO account_sessions(account_id,token_hash,auth_version,expires_at,user_agent,ip) VALUES($1,'test-legacy-session',1,now()+interval '1 hour','legacy','127.0.0.1')`, account)
	exec(`INSERT INTO account_invitations(inviter_id,token_hash,note,expires_at) VALUES($1,'test-legacy-invitation','旧邀请',now()+interval '1 day')`, account)
	exec(`INSERT INTO account_security_events(event_type,account_id,outcome,reason) VALUES('legacy.event',$1,'success','旧审计')`, account)
	codes := []access.Code{}
	for _, p := range access.Catalog() {
		if p.Scope == "tenant" || strings.HasPrefix(string(p.Code), "platform.tenant.") {
			continue
		}
		codes = append(codes, p.Code)
		exec(`INSERT INTO permission_catalog(code,name,group_name,scope,description,risk,lifecycle,catalog_version) VALUES($1,$2,$3,$4,$5,$6,$7,'legacy')`, p.Code, p.Name, p.Group, p.Scope, p.Description, p.Risk, p.Lifecycle)
	}
	for _, b := range access.Builtins() {
		if b.BindingScope != "platform" {
			continue
		}
		var rid string
		must(pool.QueryRow(ctx, `INSERT INTO roles(name,name_normalized,description,builtin_key) VALUES($1,$1,$2,$3) RETURNING id::text`, b.Name, b.Description, b.Key).Scan(&rid))
		for _, c := range b.Permissions {
			if strings.HasPrefix(string(c), "platform.tenant.") {
				continue
			}
			exec(`INSERT INTO role_permissions(role_id,permission_code) VALUES($1,$2)`, rid, c)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO roles(name,name_normalized,description) VALUES('Legacy custom','legacy custom','旧自定义角色') RETURNING id::text`).Scan(&custom))
	for _, c := range codes {
		exec(`INSERT INTO role_permissions(role_id,permission_code) VALUES($1,$2)`, custom, c)
	}
	exec(`INSERT INTO account_roles(account_id,role_id,source) VALUES($1,$2,'legacy')`, account, custom)
	before := map[string]string{}
	for _, table := range []string{"accounts", "account_passwords", "account_sessions", "account_invitations", "system_initialization"} {
		var value string
		must(pool.QueryRow(ctx, `SELECT json_agg(t)::text FROM `+table+` t`).Scan(&value))
		before[table] = value
	}
	// A bad pre-existing cross-scope permission must stop and roll back all migration DDL.
	exec(`INSERT INTO permission_catalog(code,name,group_name,scope,description,risk,lifecycle,catalog_version) VALUES($1,'bad','bad','tenant','','normal','active','legacy')`, access.TenantProfileRead)
	exec(`INSERT INTO role_permissions(role_id,permission_code) VALUES($1,$2)`, custom, access.TenantProfileRead)
	if e = Up(ctx, u.String()); e == nil {
		t.Fatal("incompatible legacy row silently migrated")
	}
	var columns int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name='roles' AND column_name='binding_scope'`, schema).Scan(&columns))
	if columns != 0 {
		t.Fatal("migration partially committed")
	}
	exec(`DELETE FROM role_permissions WHERE permission_code=$1`, access.TenantProfileRead)
	exec(`DELETE FROM permission_catalog WHERE code=$1`, access.TenantProfileRead)
	must(Up(ctx, u.String()))
	must(Check(ctx, pool))
	for table, want := range before {
		var value string
		must(pool.QueryRow(ctx, `SELECT json_agg(t)::text FROM `+table+` t`).Scan(&value))
		if value != want {
			t.Fatal("legacy data changed", table)
		}
	}
	var count int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM tenants`).Scan(&count))
	if count != 0 {
		t.Fatal("default tenant created")
	}
	var scope string
	must(pool.QueryRow(ctx, `SELECT binding_scope FROM roles WHERE id=$1`, custom).Scan(&scope))
	if scope != "platform" {
		t.Fatal(scope)
	}
	a := access.New(pool, slog.Default())
	if e = a.Sync(ctx); !errors.Is(e, access.ErrLastManager) {
		t.Fatal("missing new management permissions not caught", e)
	}
	must(pool.QueryRow(ctx, `SELECT count(*) FROM permission_catalog`).Scan(&count))
	if count != 22 {
		t.Fatal("failed sync partially updated catalog", count)
	}
	must(a.RecoverAdmin(ctx, "Legacy", "显式发布前保障"))
	must(a.Sync(ctx))
	must(a.Check(ctx))
	must(pool.QueryRow(ctx, `SELECT count(*) FROM role_permissions WHERE role_id=$1`, custom).Scan(&count))
	if count != 22 {
		t.Fatal("custom role silently expanded", count)
	}
	var rev1, rev2 string
	must(pool.QueryRow(ctx, `SELECT revision::text FROM authorization_state`).Scan(&rev1))
	must(a.Sync(ctx))
	must(pool.QueryRow(ctx, `SELECT revision::text FROM authorization_state`).Scan(&rev2))
	if rev1 != rev2 {
		t.Fatal("sync not idempotent")
	}
	must(pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events WHERE event_type='legacy.event' AND reason='旧审计' AND tenant_id IS NULL`).Scan(&count))
	if count != 1 {
		t.Fatal("legacy audit changed")
	}
	// Down refuses to destroy tenant history, keeping version and data intact.
	if _, e = provider.Down(ctx); e == nil {
		t.Fatal("destructive downgrade allowed")
	}
	must(Check(ctx, pool))
}
