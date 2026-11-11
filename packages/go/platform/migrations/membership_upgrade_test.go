package migrations

import (
	"context"
	"crypto/rand"
	"database/sql"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

func TestMembershipV3UpgradePreservesRelationsAndCredentials(t *testing.T) {
	testMembershipUpgrade(t, 3)
}
func TestRoleV4UpgradePreservesRelationsInvitationsAndCredentials(t *testing.T) {
	testMembershipUpgrade(t, 4)
}
func testMembershipUpgrade(t *testing.T, baseline int64) {
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
	schema := "membership_upgrade_" + strings.ToLower(rand.Text())
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
	_, e = provider.UpTo(ctx, 3)
	must(e)
	pool, e := pgxpool.New(ctx, u.String())
	must(e)
	defer pool.Close()
	tx, e := pool.Begin(ctx)
	must(e)
	defer tx.Rollback(ctx)
	var account, previous, role, tenant, current, ended string
	must(tx.QueryRow(ctx, `INSERT INTO accounts(username,username_normalized,display_name) VALUES('Legacy','legacy','Legacy') RETURNING id::text`).Scan(&account))
	must(tx.QueryRow(ctx, `INSERT INTO accounts(username,username_normalized,display_name) VALUES('Previous','previous','Previous') RETURNING id::text`).Scan(&previous))
	must(tx.QueryRow(ctx, `INSERT INTO roles(name,name_normalized,builtin_key,binding_scope) VALUES('Tenant admin','tenant admin','tenant_admin','tenant') RETURNING id::text`).Scan(&role))
	must(tx.QueryRow(ctx, `SELECT gen_random_uuid()::text,gen_random_uuid()::text,gen_random_uuid()::text`).Scan(&tenant, &current, &ended))
	exec := func(query string, args ...any) { t.Helper(); _, e := tx.Exec(ctx, query, args...); must(e) }
	exec(`INSERT INTO tenants(id,name,code,administrator_membership_id,created_by,updated_by,creation_request_id,creation_payload_hash) VALUES($1,'Legacy tenant','legacy-tenant',$2,$3,$3,gen_random_uuid(),'legacy')`, tenant, current, account)
	exec(`INSERT INTO tenant_codes(code,tenant_id) VALUES('legacy-tenant',$1),('old-code',$1)`, tenant)
	exec(`INSERT INTO tenant_memberships(id,tenant_id,account_id,joined_by) VALUES($1,$2,$3,$3)`, current, tenant, account)
	exec(`INSERT INTO tenant_memberships(id,tenant_id,account_id,joined_by,status,ended_at,ended_by,end_reason) VALUES($1,$2,$3,$4,'ended',now(),$4,'handover')`, ended, tenant, previous, account)
	exec(`INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) VALUES($1,$2,$3,$4)`, tenant, current, role, account)
	exec(`INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by,revoked_at,revoked_by) VALUES($1,$2,$3,$4,now(),$4)`, tenant, ended, role, account)
	exec(`INSERT INTO account_passwords(account_id,password_hash) VALUES($1,'preserved-test-hash')`, account)
	exec(`INSERT INTO account_sessions(account_id,token_hash,auth_version,expires_at,user_agent,ip) VALUES($1,'legacy-session',1,now()+interval '1 hour','legacy','127.0.0.1')`, account)
	exec(`INSERT INTO account_invitations(inviter_id,token_hash,note,expires_at) VALUES($1,'legacy-invite','legacy',now()+interval '1 day')`, account)
	exec(`INSERT INTO account_security_events(event_type,account_id,outcome,tenant_id) VALUES('tenant.created',$1,'success',$2)`, account, tenant)
	must(tx.Commit(ctx))
	if baseline >= 4 {
		_, e = provider.UpTo(ctx, 4)
		must(e)
		_, e = pool.Exec(ctx, `INSERT INTO tenant_invitations(tenant_id,target_account_id,inviter_account_id,inviter_membership_id,token_hash,expires_at,creation_request_id,creation_payload_hash,creation_operation) VALUES($1,$2,$3,$4,repeat('b',64),clock_timestamp()+interval '1 day',gen_random_uuid(),'legacy','create')`, tenant, previous, account, current)
		must(e)
	}
	if baseline >= 5 {
		_, e = provider.UpTo(ctx, 5)
		must(e)
		_, e = pool.Exec(ctx, `INSERT INTO tenant_memberships(tenant_id,account_id,status,joined_by,status_changed_by) VALUES($1,$2,'suspended',$3,$3)`, tenant, previous, account)
		must(e)
		var local string
		must(pool.QueryRow(ctx, `INSERT INTO roles(name,name_normalized,binding_scope,owner_tenant_id) VALUES('Legacy local','legacy local','tenant',$1) RETURNING id::text`, tenant).Scan(&local))
		_, e = pool.Exec(ctx, `INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) SELECT $1,id,$2,$3 FROM tenant_memberships WHERE tenant_id=$1 AND status='suspended'`, tenant, local, account)
		must(e)
		_, e = pool.Exec(ctx, `UPDATE roles SET status='disabled' WHERE id=$1`, local)
		must(e)
		_, e = pool.Exec(ctx, `UPDATE accounts SET status='disabled' WHERE id=$1`, account)
		must(e)
	}
	if baseline >= 6 {
		_, e = provider.UpTo(ctx, 6)
		must(e)
	}

	if baseline >= 7 {
		_, e = provider.UpTo(ctx, 7)
		must(e)
		must(access.New(pool, slog.Default()).Sync(ctx))
		seed, e := pool.Begin(ctx)
		must(e)
		defer seed.Rollback(ctx)
		var project, member, preset string
		must(seed.QueryRow(ctx, `SELECT gen_random_uuid()::text,gen_random_uuid()::text`).Scan(&project, &member))
		must(seed.QueryRow(ctx, `SELECT id::text FROM roles WHERE builtin_key='project_admin'`).Scan(&preset))
		_, e = seed.Exec(ctx, `INSERT INTO projects(id,tenant_id,name,code,purpose,owner_tenant_membership_id,administrator_membership_id,created_by,updated_by) VALUES($1,$2,'Legacy project','legacy-project','Preserved',$3,$4,$5,$5)`, project, tenant, current, member, account)
		must(e)
		_, e = seed.Exec(ctx, `INSERT INTO project_codes(tenant_id,code,project_id) VALUES($1,'legacy-project',$2)`, tenant, project)
		must(e)
		_, e = seed.Exec(ctx, `INSERT INTO project_memberships(id,tenant_id,project_id,tenant_membership_id,account_id,joined_by,status_changed_by) VALUES($1,$2,$3,$4,$5,$5,$5)`, member, tenant, project, current, account)
		must(e)
		_, e = seed.Exec(ctx, `INSERT INTO project_member_roles(tenant_id,project_id,membership_id,role_id,assigned_by) VALUES($1,$2,$3,$4,$5)`, tenant, project, member, preset, account)
		must(e)
		must(seed.Commit(ctx))
	}
	if baseline >= 8 {
		_, e = provider.UpTo(ctx, 8)
		must(e)
		seed, e := pool.Begin(ctx)
		must(e)
		var definition, version string
		must(seed.QueryRow(ctx, `INSERT INTO engineering_definitions(kind,code,created_by) VALUES('type-definitions','legacy_type',$1) RETURNING id::text`, account).Scan(&definition))
		must(seed.QueryRow(ctx, `INSERT INTO engineering_versions(definition_id,number,content,digest,schema_version,schemas,published_by) VALUES($1,1,'{"metamodel_version":"1","name":"Legacy type"}','preserved','1','{}',$2) RETURNING id::text`, definition, account).Scan(&version))
		_, e = seed.Exec(ctx, `INSERT INTO engineering_version_states(version_id,updated_by) VALUES($1,$2)`, version, account)
		must(e)
		must(seed.Commit(ctx))
	}

	snapshots := map[string]string{}
	tables := []string{"accounts", "account_passwords", "account_sessions", "account_invitations", "tenants", "tenant_codes", "tenant_member_roles", "account_security_events"}
	if baseline >= 4 {
		tables = append(tables, "tenant_invitations")
	}
	if baseline >= 5 {
		tables = append(tables, "roles")
	}
	if baseline >= 7 {
		tables = append(tables, "projects", "project_codes", "project_memberships", "project_member_roles")
	}
	if baseline >= 8 {
		tables = append(tables, "engineering_definitions", "engineering_versions", "engineering_version_states")
	}

	for _, table := range tables {
		var s string
		projection := "to_jsonb(t)"
		if table == "roles" {
			projection += "-'owner_project_id'"
		}
		if table == "account_security_events" {
			projection += "-'project_id'"
		}
		if table == "tenants" {
			projection = "to_jsonb(t)-'status'-'status_changed_at'-'status_changed_by'-'status_change_reason'-'status_notice'"
		}
		must(pool.QueryRow(ctx, `SELECT jsonb_agg(`+projection+` ORDER BY `+projection+`::text)::text FROM `+table+` t`).Scan(&s))
		snapshots[table] = s
	}
	var memberships string
	must(pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(t) ORDER BY id)::text FROM tenant_memberships t`).Scan(&memberships))
	must(Up(ctx, u.String()))
	must(Check(ctx, pool))
	var initial bool
	must(pool.QueryRow(ctx, `SELECT bool_and(status='active' AND status_changed_at IS NULL AND status_changed_by IS NULL AND status_change_reason IS NULL AND status_notice='') FROM tenants`).Scan(&initial))
	if !initial {
		t.Fatal("migration fabricated lifecycle metadata")
	}

	for _, table := range tables {
		var s string
		projection := "to_jsonb(t)"
		if table == "roles" {
			projection += "-'owner_project_id'"
		}
		if table == "account_security_events" {
			projection += "-'project_id'"
		}
		if table == "tenants" {
			projection = "to_jsonb(t)-'status'-'status_changed_at'-'status_changed_by'-'status_change_reason'-'status_notice'"
		}
		must(pool.QueryRow(ctx, `SELECT jsonb_agg(`+projection+` ORDER BY `+projection+`::text)::text FROM `+table+` t`).Scan(&s))
		if s != snapshots[table] {
			t.Fatal("changed legacy table", table)
		}
	}
	var legacy string
	if baseline >= 4 {
		must(pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(t) ORDER BY id)::text FROM tenant_memberships t`).Scan(&legacy))
	} else {
		must(pool.QueryRow(ctx, `SELECT jsonb_agg(to_jsonb(t)-'version'-'status_changed_at'-'status_changed_by'-'status_reason'-'source_invitation_id' ORDER BY id)::text FROM tenant_memberships t`).Scan(&legacy))
	}

	if legacy != memberships {
		t.Fatal("changed legacy membership")
	}
	service := access.New(pool, slog.Default())
	must(service.Sync(ctx))
	must(service.Check(ctx))
	var first string
	must(pool.QueryRow(ctx, `SELECT revision::text||':'||(SELECT count(*) FROM account_security_events)::text FROM authorization_state`).Scan(&first))
	must(Up(ctx, u.String()))
	must(service.Sync(ctx))
	must(service.Check(ctx))
	var second string
	must(pool.QueryRow(ctx, `SELECT revision::text||':'||(SELECT count(*) FROM account_security_events)::text FROM authorization_state`).Scan(&second))
	if first != second {
		t.Fatal("repeated sync changed history")
	}
	var n int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM tenant_invitations`).Scan(&n))
	expectedInvitations := 0
	if baseline >= 4 {
		expectedInvitations = 1
	}
	if n != expectedInvitations {
		t.Fatal("manufactured invitation")
	}
	must(pool.QueryRow(ctx, `SELECT count(*) FROM tenant_memberships`).Scan(&n))
	wantMembers := 2
	if baseline >= 5 {
		wantMembers = 3
	}
	if count := func() int { var v int; must(pool.QueryRow(ctx, `SELECT count(*) FROM projects`).Scan(&v)); return v }(); count != func() int {
		if baseline >= 7 {
			return 1
		}
		return 0
	}() {
		t.Fatal("migration changed project history")
	}
	if n != wantMembers {
		t.Fatal("manufactured membership")
	}
}

func TestLifecycleV5UpgradePreservesRelationsAndCredentials(t *testing.T) {
	testMembershipUpgrade(t, 5)
}

func TestProjectV6UpgradePreservesExistingData(t *testing.T) { testMembershipUpgrade(t, 6) }

func TestEngineeringV7UpgradePreservesProjectsAndCredentials(t *testing.T) {
	testMembershipUpgrade(t, 7)
}

func TestResourcesV8UpgradePreservesDefinitionsAndCredentials(t *testing.T) {
	testMembershipUpgrade(t, 8)
}
