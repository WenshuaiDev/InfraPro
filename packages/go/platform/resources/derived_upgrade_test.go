//go:build monitoringintegration

package resources_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	src "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDerivedUpgradeFromLegacyBackup(t *testing.T) {
	if os.Getenv("INFRAPRO_MONITORING_RESTORE") != "1" {
		t.Skip("run node scripts/test-monitoring-restore.mjs --legacy")
	}
	directory := os.Getenv("INFRAPRO_RESTORE_ARTIFACTS")
	raw, e := os.ReadFile(filepath.Join(directory, "legacy-proof.json"))
	must(t, e)
	var proof struct {
		Identity map[string]string `json:"identity"`
		Tables   map[string]struct {
			Columns []string `json:"columns"`
			Hash    string   `json:"hash"`
			Rows    int64    `json:"rows"`
		} `json:"tables"`
		Version int `json:"migration_version"`
	}
	must(t, json.Unmarshal(raw, &proof))
	if proof.Version != 12 || !regexp.MustCompile(`^identity_test_[a-z0-9]+$`).MatchString(proof.Identity["schema"]) {
		t.Fatal("not a legacy disposable restore fixture")
	}
	databaseURL := "postgres://restore_local:restore-local-test-only@127.0.0.1:57432/restore_local?search_path=" + proof.Identity["schema"]
	pool, e := pgxpool.New(ctx, databaseURL)
	must(t, e)
	defer pool.Close()
	if e = migrations.Check(ctx, pool); e == nil {
		t.Fatal("new instance accepted v12 before migration")
	}
	start := time.Now()
	must(t, migrations.Up(ctx, databaseURL))
	must(t, migrations.Check(ctx, pool))
	for table, want := range proof.Tables {
		cols := make([]string, len(want.Columns))
		for i, col := range want.Columns {
			cols[i] = pgx.Identifier{col}.Sanitize()
		}
		var hash string
		var rows int64
		must(t, pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text,'[]')),count(*) FROM (SELECT `+strings.Join(cols, ",")+` FROM `+pgx.Identifier{table}.Sanitize()+`)r`).Scan(&hash, &rows))
		if hash != want.Hash || rows != want.Rows {
			t.Fatalf("old columns changed in %s: rows %d/%d", table, rows, want.Rows)
		}
	}
	var bad int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM engineering_channels WHERE source_kind<>'direct' OR owner_kind<>'point' OR metric_version_id IS NULL OR point_id IS NULL OR derived_metric_version_id IS NOT NULL OR output_contract_hash IS NOT NULL`).Scan(&bad))
	if bad != 0 {
		t.Fatal("old direct channels changed semantic identity")
	}
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM project_resource_grants WHERE can_create_derived_channel OR can_manage_calculation`).Scan(&bad))
	if bad != 0 {
		t.Fatal("old grants gained calculation rights")
	}
	for _, table := range []string{"derived_metric_definitions", "derived_channels", "derived_generations", "analysis_configs"} {
		must(t, pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&bad))
		if bad != 0 {
			t.Fatal("migration seeded derived business data", table)
		}
	}
	a := access.New(pool, slog.Default())
	must(t, a.Sync(ctx))
	var codes []string
	must(t, pool.QueryRow(ctx, `SELECT array_agg(permission_code::text ORDER BY permission_code) FROM role_permissions WHERE role_id=$1`, proof.Identity["custom_role"]).Scan(&codes))
	if len(codes) != 1 || codes[0] != string(access.ProfileRead) {
		t.Fatal("custom role expanded", codes)
	}
	actor := func(id string) access.Actor {
		return access.Actor{ID: id, Check: func(ctx context.Context, tx pgx.Tx, _ bool) error {
			var active bool
			if e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, id).Scan(&active); e != nil {
				return e
			}
			if !active {
				return access.ErrDenied
			}
			return nil
		}}
	}
	storage, e := md.OpenStorage(ctx, md.StorageConfig{Replicas: []string{"127.0.0.1:60000", "127.0.0.1:60001"}, Database: "restored_monitoring", Policy: "monitoring_tiered", User: "default", ObjectEndpoint: "http://127.0.0.1:59333", ObjectAccessKey: "restore-source", ObjectSecretKey: "restore-source-test-only", Bucket: "restore-source", TempDirectory: t.TempDir()})
	must(t, e)
	defer storage.Close()
	b := runtime.Compose(a, storage).Configure(config.Config{Derived: config.DefaultDerived()})
	if e = b.DerivedMonitoring.Ready(ctx); e == nil {
		t.Fatal("new calculations ready before physical layout initialization")
	}
	must(t, dm.InitializeStorage(ctx, storage))
	must(t, b.DerivedMonitoring.Ready(ctx))
	owner := actor(proof.Identity["actor"])
	current, e := b.Sources.Current(ctx, owner, d.TenantScope(proof.Identity["tenant"]), src.Target{FacilityID: proof.Identity["facility"], PointID: proof.Identity["point"], ChannelID: proof.Identity["channel"]})
	must(t, e)
	if current.Source.ID != proof.Identity["binding"] || current.Source.Output == nil || current.Source.Output.DeviceID != proof.Identity["device"] {
		t.Fatal("upgraded device binding changed", current)
	}
	for _, spec := range []struct {
		scope  md.Scope
		actor  access.Actor
		status string
		count  int
	}{{md.TenantScope(proof.Identity["tenant"]), owner, "active", 23}, {md.TenantScope(proof.Identity["tenant"]), owner, "history", 123}, {md.ProjectScope(proof.Identity["tenant"], proof.Identity["project"]), actor(proof.Identity["project_actor"]), "active", 10}} {
		selection, e := b.Monitoring.Select(ctx, spec.actor, spec.scope, md.SelectionInput{Channels: []string{proof.Identity["channel"]}})
		must(t, e)
		page, e := b.Monitoring.Details(ctx, spec.actor, spec.scope, md.QueryInput{Selection: selection.ID, AllHistory: true, Limit: 1000, Status: spec.status, Qualities: []string{"good", "unmarked", "suspect", "bad"}})
		must(t, e)
		if len(page.Items) != spec.count {
			t.Fatal("upgraded observation history or authority changed", spec.status, len(page.Items), spec.count)
		}
	}
	for _, key := range []string{"retired", "deleted"} {
		var state bool
		query := `SELECT status='retired' FROM engineering_facilities WHERE id=$1`
		if key == "deleted" {
			query = `SELECT deleted_at IS NOT NULL FROM engineering_facilities WHERE id=$1`
		}
		must(t, pool.QueryRow(ctx, query, proof.Identity[key]).Scan(&state))
		if !state {
			t.Fatal("lifecycle lost", key)
		}
	}
	report := map[string]any{"status": "passed", "from_version": 12, "to_version": migrations.Version, "old_tables_compared": len(proof.Tables), "elapsed_seconds": time.Since(start).Seconds(), "source": "actual pinned v12 binary backup independently restored before migration", "device_binding_preserved": true, "custom_role_unchanged": true, "new_capabilities_default_false": true, "current_rows": 23, "historical_rows": 123, "project_visible_rows": 10}
	raw, e = json.MarshalIndent(report, "", "  ")
	must(t, e)
	must(t, os.WriteFile(filepath.Join(directory, "upgrade-proof.json"), raw, 0600))
	t.Log(string(raw))
}
