package notifications_test

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notifications"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

func TestUpgradeFrom35AndIndependentNotificationRestore(t *testing.T) {
	if os.Getenv("INFRAPRO_NOTIFICATIONS_RESTORE") != "1" {
		t.Skip("run pnpm test:notifications:restore")
	}
	f := setup(t)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	oldSchema := "notification_upgrade_" + suffix
	admin, e := pgxpool.New(ctx, os.Getenv("INFRAPRO_TEST_DATABASE_URL"))
	must(t, e)
	defer admin.Close()
	_, e = admin.Exec(ctx, "CREATE SCHEMA "+oldSchema)
	must(t, e)
	defer admin.Exec(ctx, "DROP SCHEMA "+oldSchema+" CASCADE")
	parsed, e := url.Parse(os.Getenv("INFRAPRO_TEST_DATABASE_URL"))
	must(t, e)
	q := parsed.Query()
	q.Set("search_path", oldSchema)
	parsed.RawQuery = q.Encode()
	database, e := sql.Open("pgx", parsed.String())
	must(t, e)
	defer database.Close()
	provider, e := goose.NewProvider(goose.DialectPostgres, database, os.DirFS("../migrations"))
	must(t, e)
	_, e = provider.UpTo(ctx, 35)
	must(t, e)
	old, e := pgxpool.New(ctx, parsed.String())
	must(t, e)
	defer old.Close()
	_, e = old.Exec(ctx, `INSERT INTO accounts(username,username_normalized,display_name) VALUES('Existing35','existing35','旧版合成账号')`)
	must(t, e)
	var before, after string
	must(t, old.QueryRow(ctx, `SELECT to_jsonb(a)::text FROM accounts a`).Scan(&before))
	if migrations.Check(ctx, old) == nil {
		t.Fatal("new binary accepted v35 before migrate")
	}
	must(t, migrations.Up(ctx, parsed.String()))
	must(t, migrations.Check(ctx, old))
	must(t, old.QueryRow(ctx, `SELECT to_jsonb(a)::text FROM accounts a`).Scan(&after))
	if before != after {
		t.Fatal("existing account changed on upgrade")
	}
	var history int
	must(t, old.QueryRow(ctx, `SELECT count(*) FROM notification_events`).Scan(&history))
	if history != 0 {
		t.Fatal("migration backfilled old audit data")
	}
	// Back up a real, independently seeded schema after user state is persisted.
	first := event(f.a.ID)
	f.emit(t, first)
	f.drain(t)
	page, e := f.s.List(ctx, f.a, notifications.Filter{Category: "security"})
	must(t, e)
	yes := true
	saved, e := f.s.Change(ctx, f.a, page.Items[0].ID, notifications.Change{ExpectedRevision: page.Items[0].Revision, Archived: &yes})
	must(t, e)
	_, e = f.s.SavePreferences(ctx, f.a, notifications.Preferences{Tasks: false, Updates: true, Revision: "0"})
	must(t, e)
	pending := event(f.a.ID)
	f.emit(t, pending)
	var schema string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
	tables := []string{"notification_events", "notification_recipients", "notification_deliveries", "notification_inbox_states", "account_notifications", "notification_preferences", "notification_conditions", "display_notification_handoffs"}
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
	sourceHashes := hashes(f.env.Pool)
	dump := exec.Command("docker", "exec", "infrapro-account-tests-postgres-1", "pg_dump", "-U", "infrapro_test", "-d", "infrapro_test", "--no-owner", "--no-privileges", "--schema", schema)
	raw, e := dump.Output()
	must(t, e)
	restoredDB := "notification_restore_" + suffix + "_test"
	_, e = admin.Exec(ctx, "CREATE DATABASE "+restoredDB)
	must(t, e)
	defer admin.Exec(ctx, "DROP DATABASE "+restoredDB+" WITH (FORCE)")
	extensions := exec.Command("docker", "exec", "infrapro-account-tests-postgres-1", "psql", "-U", "infrapro_test", "-d", restoredDB, "-v", "ON_ERROR_STOP=1", "-c", "CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public;")
	if output, e := extensions.CombinedOutput(); e != nil {
		t.Fatalf("restore extensions: %v %s", e, output)
	}
	restore := exec.Command("docker", "exec", "-i", "infrapro-account-tests-postgres-1", "psql", "-U", "infrapro_test", "-d", restoredDB, "-v", "ON_ERROR_STOP=1")
	restore.Stdin = bytes.NewReader(raw)
	if output, e := restore.CombinedOutput(); e != nil {
		t.Fatalf("independent restore: %v %s", e, output)
	}
	destination, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	destination.Path = "/" + restoredDB
	restored, e := pgxpool.New(ctx, destination.String())
	must(t, e)
	defer restored.Close()
	must(t, migrations.Check(ctx, restored))
	if !reflect.DeepEqual(sourceHashes, hashes(restored)) {
		t.Fatal("restore changed notification facts/receipts/preferences/state")
	}
	service := runtime.ComposeNotifications(access.New(restored, slog.Default()))
	for i := 0; i < 10; i++ {
		n, e := service.Tick(ctx)
		must(t, e)
		if n == 0 {
			break
		}
	}
	actor := access.Actor{ID: f.a.ID, Check: func(_ctx context.Context, tx pgx.Tx, _ bool) error {
		var active bool
		if e := tx.QueryRow(_ctx, `SELECT status='active' FROM accounts WHERE id=$1`, f.a.ID).Scan(&active); e != nil {
			return e
		}
		if !active {
			return access.ErrDenied
		}
		return nil
	}}
	got, e := service.Get(ctx, actor, saved.ID)
	must(t, e)
	if got.ReadAt == nil || got.ArchivedAt == nil || got.Revision != saved.Revision || got.Sequence != saved.Sequence {
		t.Fatal("restore lost per-account state", got)
	}
	preferences, e := service.Preferences(ctx, actor)
	must(t, e)
	if preferences.Tasks {
		t.Fatal("restore lost preferences")
	}
	var count int
	must(t, restored.QueryRow(ctx, `SELECT count(*) FROM account_notifications n JOIN notification_events e ON e.id=n.event_id WHERE n.account_id=$1 AND e.source_key IN ($2,$3)`, f.a.ID, first.Key, pending.Key).Scan(&count))
	if count != 2 {
		t.Fatal("restored pending delivery duplicated or lost", count)
	}
	for i := 0; i < 2; i++ {
		_, e = service.Tick(ctx)
		must(t, e)
	}
	t.Log("v35 -> v36: existing account unchanged, no historical backfill; independent pg_dump/psql restore: all 8 durable notification tables identical; pending resumes once and read/archive/preferences survive")
}
