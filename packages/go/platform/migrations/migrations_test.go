package migrations

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConcurrentMigrationAndVersionCheck(t *testing.T) {
	raw := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("run pnpm test:identity")
	}
	u, e := url.Parse(raw)
	if e != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("refusing non-test database")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	schema := "migration_test_" + strings.ToLower(rand.Text())
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Go(func() { errs <- Up(ctx, u.String()) })
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	pool, e := pgxpool.New(ctx, u.String())
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if e = Check(ctx, pool); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = pool.QueryRow(ctx, "SELECT count(*) FROM system_initialization").Scan(&count); e != nil || count != 1 {
		t.Fatal("migration concurrency duplicated initialization", e)
	}
	var epoch string
	if e = pool.QueryRow(ctx, "SELECT restore_epoch::text FROM event_protocol WHERE id=1").Scan(&epoch); e != nil {
		t.Fatal(e)
	}
	// Recreate the earliest schema-59 installation, which already has a
	// dispatch epoch but predates the restricted baseline projection.
	const invitation = "architecture_notification_invitation_targets(uuid,uuid,uuid[])"
	if _, e = pool.Exec(ctx, "DROP FUNCTION architecture_fixed_baseline_metadata(uuid,uuid,text,text),"+invitation); e != nil {
		t.Fatal(e)
	}
	if e = Up(ctx, u.String()); e != nil {
		t.Fatal(e)
	}
	var restored bool
	e = pool.QueryRow(ctx, `SELECT restore_epoch::text=$1 AND
		to_regprocedure(format('%I.architecture_fixed_baseline_metadata(uuid,uuid,text,text)', current_schema())) IS NOT NULL
		AND to_regprocedure(format('%I.architecture_notification_invitation_targets(uuid,uuid,uuid[])',current_schema())) IS NOT NULL
		FROM event_protocol WHERE id=1`, epoch).Scan(&restored)
	if e != nil || !restored {
		t.Fatal("schema 59 upgrade lost the selected epoch or its restricted metadata function", e)
	}
	// A restore without ACLs leaves the function already present with PUBLIC
	// EXECUTE. Restore its narrow ACL without discarding a named caller grant.
	const signature = "architecture_fixed_baseline_metadata(uuid,uuid,text,text)"
	if _, e = pool.Exec(ctx, "GRANT EXECUTE ON FUNCTION "+signature+" TO PUBLIC,infrapro_audit_observer"); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, "REVOKE EXECUTE ON FUNCTION "+signature+" FROM infrapro_audit_worker"); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, "GRANT EXECUTE ON FUNCTION "+invitation+" TO PUBLIC; ALTER FUNCTION "+invitation+" SET search_path FROM CURRENT"); e != nil {
		t.Fatal(e)
	}
	if e = Up(ctx, u.String()); e != nil {
		t.Fatal(e)
	}
	var publicExec, auditExec, retainedGrant bool
	e = pool.QueryRow(ctx, `SELECT
		EXISTS(SELECT FROM pg_catalog.aclexplode(coalesce(p.proacl,pg_catalog.acldefault('f',p.proowner))) a
			WHERE a.grantee=0 AND a.privilege_type='EXECUTE'),
		has_function_privilege('infrapro_audit_worker',p.oid,'EXECUTE'),
		has_function_privilege('infrapro_audit_observer',p.oid,'EXECUTE')
		FROM pg_catalog.pg_proc p WHERE p.oid=to_regprocedure($1)`, signature).Scan(&publicExec, &auditExec, &retainedGrant)
	if e != nil || publicExec || !auditExec || !retainedGrant {
		t.Fatalf("restored projection permissions public=%v audit=%v original caller=%v err=%v", publicExec, auditExec, retainedGrant, e)
	}
	var restricted bool
	e = pool.QueryRow(ctx, `SELECT
		p.proconfig=ARRAY[format('search_path=pg_catalog, %I, pg_temp',current_schema())]
		AND NOT EXISTS(SELECT FROM aclexplode(coalesce(p.proacl,acldefault('f',p.proowner))) a
			WHERE a.grantee=0 AND a.privilege_type='EXECUTE')
		AND (to_regrole('infrapro_control_api') IS NULL OR has_function_privilege(to_regrole('infrapro_control_api'),p.oid,'EXECUTE'))
		FROM pg_proc p WHERE p.oid=to_regprocedure($1)`, invitation).Scan(&restricted)
	if e != nil || !restricted {
		t.Fatal("restored invitation reader permissions or search path are invalid", e)
	}
	if _, e = pool.Exec(ctx, "INSERT INTO goose_db_version(version_id,is_applied) VALUES(999,true)"); e != nil {
		t.Fatal(e)
	}
	if e = Check(ctx, pool); e == nil {
		t.Fatal("unknown migration accepted")
	}
}
