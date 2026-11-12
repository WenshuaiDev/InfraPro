package resources_test

import (
	"bytes"
	"fmt"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Restore a populated isolated schema into a new database, never over the source.
func TestResourceBackupRestore(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "BACKUP")
	component, e := f.write(t, target(facility, res.Component, ""), "create", map[string]any{"code": "C", "name": "备份构件", "category_key": "span", "expected_structure_revision": 1})
	must(t, e)
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "备份测点", "component_id": component.ID})
	must(t, e)
	ct := target(facility, res.Channel, "")
	ct.Point = point.ID
	ch, e := f.write(t, ct, "create", map[string]any{"code": "CH", "name": "备份通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	ct.ID = ch.ID
	_, e = f.write(t, ct, "delete", map[string]any{"expected_revision": 1, "reason": "验证删除墓碑"})
	must(t, e)
	u, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	schema := u.Query().Get("search_path")
	var db string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT 'resource_restore_'||replace(gen_random_uuid()::text,'-','')`).Scan(&db))
	compose, e := filepath.Abs("../../../../deploy/compose/test.yaml")
	must(t, e)
	run := func(input []byte, args ...string) []byte {
		t.Helper()
		cmd := exec.Command("docker", append([]string{"compose", "-f", compose, "exec", "-T", "postgres"}, args...)...)
		cmd.Stdin = bytes.NewReader(input)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, e := cmd.Output()
		if e != nil {
			t.Fatalf("backup tool failed: %v: %s", e, stderr.String())
		}
		return out
	}
	dump := run(nil, "pg_dump", "-U", "infrapro_test", "-d", "infrapro_test", "-n", schema, "-Fc")
	run(nil, "createdb", "-U", "infrapro_test", db)
	defer run(nil, "dropdb", "-U", "infrapro_test", db)
	// A schema-only dump excludes extensions installed in public.
	run(nil, "psql", "-v", "ON_ERROR_STOP=1", "-U", "infrapro_test", "-d", db, "-c", "CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public")
	run(dump, "pg_restore", "--exit-on-error", "--no-owner", "-U", "infrapro_test", "-d", db)
	restoreURL, e := url.Parse(os.Getenv("INFRAPRO_TEST_DATABASE_URL"))
	must(t, e)
	restoreURL.Path = "/" + db
	restoreURL.RawQuery = u.RawQuery
	pool, e := pgxpool.New(ctx, restoreURL.String())
	must(t, e)
	defer pool.Close()
	for _, table := range []string{"accounts", "account_passwords", "engineering_definitions", "engineering_versions", "engineering_version_states", "engineering_dependencies", "engineering_facilities", "engineering_components", "engineering_points", "engineering_channels", "engineering_resource_changes", "engineering_resource_receipts"} {
		query := fmt.Sprintf(`SELECT md5(COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text,'[]')) FROM %s r`, table)
		var before, after string
		must(t, f.env.Pool.QueryRow(ctx, query).Scan(&before))
		must(t, pool.QueryRow(ctx, query).Scan(&after))
		if before != after {
			t.Fatalf("restore mismatch: %s", table)
		}
	}
}
