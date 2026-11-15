package resources_test

import (
	"bytes"
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestTaskIndependentBackupRestoreAndLegacyNotification(t *testing.T) {
	f := resourcesSetup(t)
	h := newTaskDisplay(t, f, "TaskRestore")
	source := h.upload(t, []byte("synthetic invalid ZIP"))
	_, e := h.s.ProcessOne(ctx)
	if e == nil {
		t.Fatal("expected invalid package")
	}
	index := runtime.ComposeTaskIndexer(f.env.Service.Access())
	_, e = index.Tick(ctx, "restore-source")
	must(t, e)
	s := runtime.ComposeTasks(f.env.Service.Access(), h.b)
	failed, e := s.Get(ctx, h.p.actor, source.ID(), "")
	must(t, e)
	cmd := tasks.Command{RequestID: uuid.NewString(), Action: "retry", ExpectedGeneration: failed.Generation, ExpectedRevision: failed.ControlRevision, Reason: "保留到备份的人工重试理由"}
	committed, e := s.Command(ctx, h.p.actor, source.ID(), cmd)
	must(t, e)
	_, e = index.Tick(ctx, "restore-source")
	must(t, e)
	// A second source remains queued; restoring must not execute it to repair metadata.
	must(t, f.env.Pool.QueryRow(ctx, `SELECT revision::text FROM display_instances WHERE id=$1`, h.id).Scan(&h.revision))
	h.upload(t, displayZIP(t))
	_, e = index.Tick(ctx, "restore-source")
	must(t, e)
	page, e := s.List(ctx, h.p.actor, tasks.Filter{Limit: 1})
	must(t, e)
	if page.NextCursor == "" {
		t.Fatal("cursor fixture missing")
	}
	legacy := ne.Event{Module: "monitoring", Key: "legacy-fixture", Type: "task.result", Scope: ne.BusinessScope(f.tenant, ""), Target: ne.Target{Kind: "monitoring_job", ID: uuid.NewString()}, Outcome: "failed", Reason: "job_attempts_exhausted", Recipients: ne.Personal(f.actor.ID)}
	var eventID string
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { var err error; eventID, err = ne.AppendTx(ctx, tx, legacy); return err }))
	var fingerprint string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT fingerprint FROM notification_events WHERE id=$1`, eventID).Scan(&fingerprint))
	legacy.TaskReference = &taskrecord.Ref{SourceRef: taskrecord.SourceRef{Module: "monitoring", Kind: "job", RootID: legacy.Target.ID}, Generation: "1"}
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { _, e := ne.AppendTx(ctx, tx, legacy); return e }))
	var same bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT fingerprint=$2 AND schema_version=1 AND task_reference IS NULL FROM notification_events WHERE id=$1`, eventID, fingerprint).Scan(&same))
	if !same {
		t.Fatal("v1 rewritten")
	}
	notifications := runtime.ComposeNotifications(f.env.Service.Access())
	_, e = notifications.Tick(ctx)
	must(t, e)
	tables := []string{"task_records", "task_executions", "task_attempts", "task_events", "task_relations", "task_protocol", "task_source_states", "display_task_handoffs", "display_task_execution_evidence", "display_package_jobs", "display_uploads", "display_packages", "display_receipts", "display_notification_handoffs", "notification_events", "notification_recipients", "notification_deliveries", "account_notifications", "notification_inbox_states", "notification_preferences"}
	hashes := func(pool *pgxpool.Pool) map[string]string {
		t.Helper()
		out := map[string]string{}
		for _, name := range tables {
			var hash string
			must(t, pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text,'[]')) FROM `+name+` r`).Scan(&hash))
			out[name] = hash
		}
		return out
	}
	before := hashes(f.env.Pool)
	var schema string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
	raw, e := exec.Command("docker", "exec", "infrapro-account-tests-postgres-1", "pg_dump", "-U", "infrapro_test", "-d", "infrapro_test", "--no-owner", "--no-privileges", "--schema", schema).Output()
	must(t, e)
	admin, e := pgxpool.New(ctx, os.Getenv("INFRAPRO_TEST_DATABASE_URL"))
	must(t, e)
	defer admin.Close()
	db := "task_restore_" + strings.ReplaceAll(uuid.NewString(), "-", "") + "_test"
	_, e = admin.Exec(ctx, "CREATE DATABASE "+db)
	must(t, e)
	defer admin.Exec(ctx, "DROP DATABASE "+db+" WITH (FORCE)")
	if out, e := exec.Command("docker", "exec", "infrapro-account-tests-postgres-1", "psql", "-U", "infrapro_test", "-d", db, "-v", "ON_ERROR_STOP=1", "-c", "CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public;").CombinedOutput(); e != nil {
		t.Fatalf("extension restore %v %s", e, out)
	}
	restore := exec.Command("docker", "exec", "-i", "infrapro-account-tests-postgres-1", "psql", "-U", "infrapro_test", "-d", db, "-v", "ON_ERROR_STOP=1")
	restore.Stdin = bytes.NewReader(raw)
	if out, e := restore.CombinedOutput(); e != nil {
		t.Fatalf("restore %v %s", e, out)
	}
	destination, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	destination.Path = "/" + db
	pool, e := pgxpool.New(ctx, destination.String())
	must(t, e)
	defer pool.Close()
	if !reflect.DeepEqual(before, hashes(pool)) {
		t.Fatal("full backup lost durable task or notification state")
	}
	a := h.p.actor
	a.Check = func(ctx context.Context, tx pgx.Tx, _ bool) error {
		var active bool
		e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, a.ID).Scan(&active)
		if e != nil {
			return e
		}
		if !active {
			return access.ErrDenied
		}
		return nil
	}
	restoredAccess := access.New(pool, slog.Default())
	restored := runtime.ComposeTasks(restoredAccess)
	restoredIndex := runtime.ComposeTaskIndexer(restoredAccess)
	must(t, restoredIndex.RestoreEpoch(ctx, "隔离环境真实完整备份恢复"))
	_, e = restored.List(ctx, a, tasks.Filter{Limit: 1, Cursor: page.NextCursor})
	if e == nil {
		t.Fatal("pre-restore cursor survived")
	}
	for n := 0; n < 3; n++ {
		_, e = restoredIndex.Tick(ctx, "restored")
		must(t, e)
	}
	must(t, restoredIndex.Verify(ctx))
	replay, e := restored.Command(ctx, a, source.ID(), cmd)
	must(t, e)
	if replay.Generation != committed.Generation {
		t.Fatal("restored command replay changed generation")
	}
	after := hashes(pool)
	for _, table := range []string{"display_package_jobs", "display_packages", "display_receipts", "task_executions", "task_attempts", "task_events", "notification_events", "account_notifications"} {
		if before[table] != after[table] {
			t.Fatalf("metadata restore re-executed or rewrote %s", table)
		}
	}
	old, e := restored.Get(ctx, a, source.ID(), "1")
	must(t, e)
	if old.State != "failed" || old.SelectedExecution.ReasonCode == "" {
		t.Fatal("restored old failure unavailable")
	}
	t.Log("independent pg_dump/psql restore preserved 20 durable tables, old cursor rejected, receipt replay and history retained without business rerun")
}
