package projects_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/url"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProjectBackupRestoreWithAllHistory(t *testing.T) {
	f := setup(t)
	p := f.create(t, "restore-drill", f.bid)
	m := f.add(t, f.b, p.ProjectID, f.cid)
	r := f.role(t, p.ProjectID, "恢复演练角色", []access.Code{access.ProjectProfileRead})
	f.change(t, f.b, p.ProjectID, m.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{r.ObjectID}})
	f.change(t, f.b, p.ProjectID, m.ObjectID, access.OpProjectMemberSuspend, projects.Change{})
	f.change(t, f.b, p.ProjectID, r.ObjectID, access.OpProjectRoleStatus, projects.Change{Status: "disabled"})
	f.change(t, f.b, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
	u, e := url.Parse(f.e.Config.DatabaseURL)
	must(t, e)
	schema := u.Query().Get("search_path")
	compose, e := filepath.Abs("../../../../deploy/compose/test.yaml")
	must(t, e)
	run := func(input []byte, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, "docker", append([]string{"compose", "-f", compose, "exec", "-T", "postgres"}, args...)...)
		cmd.Stdin = bytes.NewReader(input)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, e := cmd.Output()
		if e != nil {
			t.Fatalf("restore command %s failed: %v (%s)", args[0], e, stderr.String())
		}
		return out
	}
	db := "project_restore_" + schema + "_test"
	if len(db) > 63 {
		db = db[:58] + "_test"
	}
	run(nil, "createdb", "-U", "infrapro_test", db)
	defer run(nil, "dropdb", "-U", "infrapro_test", db)
	dump := run(nil, "pg_dump", "-U", "infrapro_test", "-d", "infrapro_test", "-n", schema, "-Fc")
	run(dump, "pg_restore", "-U", "infrapro_test", "-d", db, "--no-owner")
	u.Path = "/" + db
	restored, e := pgxpool.New(ctx, u.String())
	must(t, e)
	defer restored.Close()
	snapshot := func(pool *pgxpool.Pool) map[string]string {
		t.Helper()
		rows, e := pool.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname=$1 ORDER BY tablename`, schema)
		must(t, e)
		names := []string{}
		for rows.Next() {
			var n string
			must(t, rows.Scan(&n))
			names = append(names, n)
		}
		must(t, rows.Err())
		rows.Close()
		result := map[string]string{}
		for _, table := range names {
			var digest string
			q := `SELECT count(*)::text||':'||md5(COALESCE(string_agg(to_jsonb(t)::text,E'\n' ORDER BY to_jsonb(t)::text),'')) FROM ` + pgx.Identifier{schema, table}.Sanitize() + ` t`
			must(t, pool.QueryRow(ctx, q).Scan(&digest))
			result[table] = digest
		}
		return result
	}
	before, after := snapshot(f.e.Pool), snapshot(restored)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("backup restoration changed entity/history digests")
	}
	restoredAccess := access.New(restored, slog.Default())
	must(t, restoredAccess.Check(ctx))
	var valid bool
	must(t, restored.QueryRow(ctx, `SELECT project_roles_valid() AND tenant_roles_valid()`).Scan(&valid))
	if !valid {
		t.Fatal("restored constraints invalid")
	}
	// Capture counts and cryptographic digests only; never include passwords, credentials or tokens in evidence.
	safe, _ := json.Marshal(map[string]any{"restored": true, "tables": after})
	t.Log(string(safe))
}
