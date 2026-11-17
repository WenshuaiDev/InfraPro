package migrations

import (
	"context"
	"database/sql"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

func TestMonitoringUpgradePreservesExistingRowsAndCustomRoles(t *testing.T) {
	raw := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("run with the isolated *_test database")
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
	schemaName := "monitoring_upgrade_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e = admin.Exec(ctx, "CREATE SCHEMA "+schemaName)
	must(e)
	defer admin.Exec(ctx, "DROP SCHEMA "+schemaName+" CASCADE")
	q := u.Query()
	q.Set("search_path", schemaName)
	u.RawQuery = q.Encode()
	db, e := sql.Open("pgx", u.String())
	must(e)
	defer db.Close()
	provider, e := goose.NewProvider(goose.DialectPostgres, db, files)
	must(e)
	_, e = provider.UpTo(ctx, 11)
	must(e)
	pool, e := pgxpool.New(ctx, u.String())
	must(e)
	defer pool.Close()
	svc := access.New(pool, slog.Default())
	seedLegacyCatalog(t, ctx, pool)
	var account, role string
	must(pool.QueryRow(ctx, `INSERT INTO accounts(username,username_normalized,display_name) VALUES('MonitoringLegacy','monitoringlegacy','迁移保留样本') RETURNING id::text`).Scan(&account))
	must(pool.QueryRow(ctx, `INSERT INTO roles(name,name_normalized,description,binding_scope) VALUES('Legacy profile','legacy profile','migration fixture','platform') RETURNING id::text`).Scan(&role))
	_, e = pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code) VALUES($1,$2)`, role, access.ProfileRead)
	must(e)
	_, e = pool.Exec(ctx, `INSERT INTO account_roles(account_id,role_id,source) VALUES($1,$2,'migration-fixture')`, account, role)
	must(e)
	// Model the old catalog without monitoring permissions. Upgrade may extend
	// only system-maintained roles; a user's custom role must stay byte-identical.
	_, e = pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_code LIKE 'tenant.monitoring.%' AND permission_code<>$1 OR permission_code LIKE 'project.monitoring.%' AND permission_code<>$2`, access.TenantMonitoringSourceManage, access.ProjectMonitoringSourceManage)
	must(e)
	_, e = pool.Exec(ctx, `DELETE FROM permission_catalog WHERE code LIKE 'tenant.monitoring.%' AND code<>$1 OR code LIKE 'project.monitoring.%' AND code<>$2`, access.TenantMonitoringSourceManage, access.ProjectMonitoringSourceManage)
	must(e)
	_, e = pool.Exec(ctx, `UPDATE authorization_state SET catalog_checksum='pre-monitoring-v11' WHERE id=1`)
	must(e)
	rows, e := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname=$1 AND tablename<>'goose_db_version' ORDER BY tablename`, schemaName)
	must(e)
	tables := []string{}
	for rows.Next() {
		var name string
		must(rows.Scan(&name))
		tables = append(tables, name)
	}
	must(rows.Err())
	rows.Close()
	columns := map[string][]string{}
	for _, table := range tables {
		var names []string
		must(pool.QueryRow(ctx, `SELECT array_agg(column_name::text ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2`, schemaName, table).Scan(&names))
		columns[table] = names
	}
	snapshot := func(table string) string {
		var v string
		must(pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(p.value ORDER BY p.value::text),'[]'::jsonb)::text FROM `+pgx.Identifier{table}.Sanitize()+` r CROSS JOIN LATERAL (SELECT jsonb_object_agg(key,value) AS value FROM jsonb_each(to_jsonb(r)) WHERE key=ANY($1::text[])) p`, columns[table]).Scan(&v))
		return v
	}
	before := map[string]string{}
	for _, table := range tables {
		before[table] = snapshot(table)
	}
	must(Up(ctx, u.String()))
	must(Check(ctx, pool))
	for table, want := range before {
		if got := snapshot(table); got != want {
			t.Fatal("migration changed existing rows", table)
		}
	}
	var customBefore, customAfter string
	must(pool.QueryRow(ctx, `SELECT jsonb_agg(permission_code ORDER BY permission_code)::text FROM role_permissions WHERE role_id=$1`, role).Scan(&customBefore))
	must(svc.Sync(ctx))
	must(pool.QueryRow(ctx, `SELECT jsonb_agg(permission_code ORDER BY permission_code)::text FROM role_permissions WHERE role_id=$1`, role).Scan(&customAfter))
	if customBefore != customAfter {
		t.Fatal("custom role expanded")
	}
	var count int
	must(pool.QueryRow(ctx, `SELECT count(*) FROM role_permissions p JOIN roles r ON r.id=p.role_id WHERE (r.builtin_key=$1 AND p.permission_code=$2) OR (r.builtin_key=$3 AND p.permission_code=$4)`, access.TenantAdminKey, access.TenantMonitoringDataRead, access.ProjectAdminKey, access.ProjectMonitoringDataRead).Scan(&count))
	if count != 2 {
		t.Fatal("administrator catalog was not upgraded")
	}
	for _, table := range []string{"monitoring_import_batches", "monitoring_data_grants", "monitoring_publications"} {
		must(pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count))
		if count != 0 {
			t.Fatal("migration seeded monitoring data", table)
		}
	}
	t.Logf("v11→v%d preserved every row of %d existing tables; custom role unchanged; no monitoring samples/grants seeded", Version, len(tables))
}
