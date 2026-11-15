//go:build monitoringintegration

package resources_test

import (
	"database/sql"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
)

// Copies the intersection of real owner fixture columns into an independently
// migrated v36 schema. New projection/evidence tables are absent there. This is
// a migration test only: no production table/guard is dropped or changed.
func verifyTaskLegacyUpgrade(t *testing.T, f resourceFixture) {
	t.Helper()
	schema := "identity_test_task_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, e := f.env.Pool.Exec(ctx, "CREATE SCHEMA "+schema)
	must(t, e)
	defer f.env.Pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	u, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, e := sql.Open("pgx", u.String())
	must(t, e)
	provider, e := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("../migrations"))
	must(t, e)
	_, e = provider.UpTo(ctx, 36)
	must(t, e)
	must(t, db.Close())
	pool, e := pgxpool.New(ctx, u.String())
	must(t, e)
	defer pool.Close()
	tables, e := pool.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema=current_schema() AND table_type='BASE TABLE' AND table_name<>'goose_db_version' ORDER BY table_name`)
	must(t, e)
	names := []string{}
	for tables.Next() {
		var name string
		must(t, tables.Scan(&name))
		names = append(names, name)
	}
	must(t, tables.Err())
	tables.Close()
	var originalSchema string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&originalSchema))
	hashes := map[string]string{}
	columns := map[string][]string{}
	must(t, pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SET LOCAL session_replication_role=replica`); e != nil {
			return e
		}
		quotedTables := make([]string, len(names))
		for n, table := range names {
			quotedTables[n] = pgx.Identifier{table}.Sanitize()
		}
		if _, e := tx.Exec(ctx, "TRUNCATE "+strings.Join(quotedTables, ",")); e != nil {
			return e
		}
		for _, table := range names {
			rows, e := tx.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1 AND is_generated='NEVER' ORDER BY ordinal_position`, table)
			if e != nil {
				return e
			}
			cols := []string{}
			for rows.Next() {
				var col string
				if e = rows.Scan(&col); e != nil {
					rows.Close()
					return e
				}
				cols = append(cols, col)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			columns[table] = cols
			quoted := make([]string, len(cols))
			for n, col := range cols {
				quoted[n] = pgx.Identifier{col}.Sanitize()
			}
			where := ""
			switch table {
			case "notification_events":
				where = " WHERE schema_version=1"
			case "notification_recipients", "notification_deliveries", "account_notifications":
				where = " WHERE event_id IN(SELECT id FROM notification_events WHERE schema_version=1)"
			}
			if strings.Contains(where, "SELECT id FROM notification_events") {
				where = strings.ReplaceAll(where, "SELECT id FROM notification_events", "SELECT id FROM "+pgx.Identifier{originalSchema, "notification_events"}.Sanitize())
			}
			_, e = tx.Exec(ctx, "INSERT INTO "+pgx.Identifier{table}.Sanitize()+" ("+strings.Join(quoted, ",")+") OVERRIDING SYSTEM VALUE SELECT "+strings.Join(quoted, ",")+" FROM "+pgx.Identifier{originalSchema, table}.Sanitize()+where)
			if e != nil {
				return e
			}
		}
		return nil
	}))
	hash := func(table string) string {
		t.Helper()
		cols := []string{}
		for _, c := range columns[table] {
			cols = append(cols, pgx.Identifier{c}.Sanitize())
		}
		var h string
		must(t, pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text,'[]')) FROM (SELECT `+strings.Join(cols, ",")+` FROM `+pgx.Identifier{table}.Sanitize()+`) r`).Scan(&h))
		return h
	}
	for _, table := range names {
		hashes[table] = hash(table)
	}
	if migrations.Check(ctx, pool) == nil {
		t.Fatal("new process accepted v36")
	}
	must(t, migrations.Up(ctx, u.String()))
	must(t, migrations.Check(ctx, pool))
	for _, table := range names {
		if table != "notification_protocol" && hashes[table] != hash(table) {
			t.Fatalf("migration changed legacy business columns: %s", table)
		}
	}
	a := access.New(pool, slog.Default())
	index := runtime.ComposeTaskIndexer(a)
	o := tasks.DefaultOptions()
	o.ReconcileBatch = 1
	index.WithOptions(o)
	_, e = index.Tick(ctx, "first-page")
	must(t, e)
	// Discard the worker instance, resume its persisted cursors with another.
	index = runtime.ComposeTaskIndexer(a).WithOptions(o)
	for n := 0; n < 80; n++ {
		_, e = index.Tick(ctx, "resumed")
		must(t, e)
		var mode string
		must(t, pool.QueryRow(ctx, `SELECT mode FROM task_protocol WHERE id=1`).Scan(&mode))
		if mode == "ready" {
			break
		}
	}
	must(t, index.Verify(ctx))
	var incomplete int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM task_records WHERE history_incomplete`).Scan(&incomplete))
	if incomplete == 0 {
		t.Fatal("legacy history was invented")
	}
	var attempts int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM task_attempts`).Scan(&attempts))
	if attempts != 0 {
		t.Fatal("legacy attempt counters became fabricated attempts")
	}
	for _, table := range names {
		if table != "notification_protocol" && hashes[table] != hash(table) {
			t.Fatalf("backfill replayed business/notifications: %s", table)
		}
	}
	var unknownWithProof int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM spatial_processing_jobs j WHERE originator_account_id IS NULL AND EXISTS(SELECT 1 FROM spatial_changes c WHERE c.target_id=j.id AND c.operation='versions.process' AND c.actor_id IS NOT NULL)`).Scan(&unknownWithProof))
	if unknownWithProof != 0 {
		t.Fatal("creation evidence was ignored")
	}
	var groupCount, links, expectedLinks int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM derived_task_groups`).Scan(&groupCount))
	if groupCount > 0 {
		must(t, pool.QueryRow(ctx, `SELECT count(*) FROM derived_task_group_members`).Scan(&expectedLinks))
		must(t, pool.QueryRow(ctx, `SELECT count(*) FROM task_relations WHERE relation_kind='member'`).Scan(&links))
		if expectedLinks < 2 || links != expectedLinks {
			t.Fatal("historical command group lost members", expectedLinks, links)
		}
	}
	t.Logf("v36→v39: %d original tables unchanged, one-row backfill resumed, historical attempts marked incomplete, no business or notification rerun", len(names))
}
func TestTaskMigrationWithRealRetainedAndQueuedSources(t *testing.T) {
	f := monitoringSetup(t)
	f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,12,good\n"))
	queued := f.upload(t, "observed_at,value,quality\n2024-01-02T00:00:00Z,13,good\n")
	_, e := f.service.QueueImport(ctx, f.actor, f.scope, queued.ID, "validate", f.meta(queued.Revision))
	must(t, e)
	h := newTaskDisplay(t, f.resourceFixture, "LegacyTask")
	h.upload(t, []byte("legacy failed ZIP"))
	_, e = h.s.ProcessOne(ctx)
	if e == nil {
		t.Fatal("expected source validation failure")
	}
	verifyTaskLegacyUpgrade(t, f.resourceFixture)
}
