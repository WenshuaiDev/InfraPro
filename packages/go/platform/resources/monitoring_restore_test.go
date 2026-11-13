//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMonitoringIndependentRestore(t *testing.T) {
	if os.Getenv("INFRAPRO_MONITORING_RESTORE") != "1" {
		t.Skip("run pnpm test:monitoring:restore for the independent environment")
	}
	f := monitoringSetup(t)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	file := func(start, count int) string {
		var s strings.Builder
		s.WriteString("observed_at,value,quality\n")
		for i := start; i < start+count; i++ {
			fmt.Fprintf(&s, "%s,%d,good\n", base.Add(time.Duration(i)*time.Nanosecond).Format(time.RFC3339Nano), i)
		}
		return s.String()
	}
	a := f.publish(t, f.validate(t, f.upload(t, file(0, 100))))
	b := f.validate(t, f.upload(t, file(20, 100)))
	var e error
	b, e = f.service.ResolveImport(ctx, f.actor, f.scope, b.ID, md.ResolveInput{WriteMeta: f.meta(b.Revision), PreviewHash: b.ValidationHash, ConfirmSameObservation: true})
	must(t, e)
	b = f.publish(t, b)
	derivedProof := prepareDerivedRestore(t, f, base)
	preview, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, a.ID, a.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, a.ID, md.RevokeInput{WriteMeta: f.meta(a.Revision), PreviewHash: preview.Hash})
	must(t, e)
	c := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n1900-01-01T00:00:00Z,10000000000000000000000000.000000000001,good\n2262-04-11T23:47:16.854775807Z,10000000000000000000000000.000000000003,good\n")))
	_ = f.analytic(t, "overview", f.query(t))
	project := f.project(t, "RestoreProof")
	var facility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	ri := f.accessInput(t, project, facility, "create_link")
	ri.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, project, facility, ri)
	var resource string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, f.channel).Scan(&resource))
	from, to := base.Add(100*time.Nanosecond).Format(time.RFC3339Nano), base.Add(110*time.Nanosecond).Format(time.RFC3339Nano)
	_, e = f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta("0"), ChannelID: f.channel, ResourceGrantID: resource, Range: &md.Range{From: &from, To: &to}, Capabilities: []string{"read", "export", "source"}})
	must(t, e)
	for _, channel := range derivedProof.channels {
		change := f.accessInput(t, project, facility, "change_grants")
		change.Add = []res.GrantInput{{Kind: res.Channel, ChannelID: &channel, Capabilities: []string{"read"}}}
		f.grant(t, project, facility, change)
		var grant string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, channel).Scan(&grant))
		var revision string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT revision::text FROM monitoring_access_states WHERE tenant_id=$1 AND project_id=$2`, f.tenant, project.id).Scan(&revision))
		_, e = f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(revision), ChannelID: channel, ResourceGrantID: grant, Range: &md.Range{}, Capabilities: []string{"read", "history", "export", "source"}})
		must(t, e)
	}
	q := f.query(t)
	q.Limit = 1000
	before, e := f.service.Details(ctx, f.actor, f.scope, q)
	must(t, e)
	q.Status = "history"
	history, e := f.service.Details(ctx, f.actor, f.scope, q)
	must(t, e)
	if len(before.Items) != 22 || len(history.Items) != 122 {
		t.Fatal(len(before.Items), len(history.Items))
	}
	proofTables := []string{"monitoring_source_files", "monitoring_import_mappings", "monitoring_batch_resolutions", "monitoring_chunks", "monitoring_write_attempts", "monitoring_publications", "monitoring_publication_chunks", "monitoring_data_grants", "monitoring_routes", "project_resource_grants"}
	derivedProof.freeze(t, f)
	proofTables = append(proofTables, "derived_metric_versions", "derived_channel_configs", "derived_config_assignments", "derived_execution_mandates", "derived_channel_heads", "derived_generations", "derived_fixed_baselines", "derived_input_sources", "derived_publication_receipts", "monitoring_raw_snapshots", "monitoring_change_consumers")
	proofTables = append(proofTables, "analysis_configs", "analysis_config_versions", "analysis_artifacts", "analysis_artifact_versions", "analysis_artifact_input_ranges", "analysis_runs")
	hashes := map[string]string{}
	for _, table := range proofTables {
		var hash string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text,'[]')) FROM `+table+` r`).Scan(&hash))
		hashes[table] = hash
	}
	admin, e := minio.New("127.0.0.1:58334", &minio.Options{Creds: credentials.NewStaticV4("backup-admin", "backup-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	exists, e := admin.BucketExists(ctx, "monitoring-backups")
	must(t, e)
	if !exists {
		must(t, admin.MakeBucket(ctx, "monitoring-backups", minio.MakeBucketOptions{}))
	}
	dir := filepath.Join(os.Getenv("INFRAPRO_RESTORE_ARTIFACTS"), "archive")
	opt := md.BackupOptions{Directory: dir, DatabaseURL: f.env.Config.DatabaseURL, PGContainer: "infrapro-account-tests-postgres-1", Reason: "独立本机恢复合成样本", Store: md.BackupConfig{Endpoint: "http://127.0.0.1:58334", AccessKey: "backup-local", SecretKey: "backup-local-test-only", Bucket: "monitoring-backups", NamedCollection: "monitoring_backups"}}
	opt.Calculations = dm.NewRecovery(f.storage)
	backup, e := md.Backup(ctx, f.env.Pool, f.storage, opt)
	must(t, e)
	if backup.Build == nil || backup.Build.GoVersion == "" || len(backup.Build.BinarySHA256) != 64 || backup.Build.BinaryBytes <= 0 || backup.Calculations == nil || len(backup.Calculations.FixtureSHA256) != 64 || len(backup.Calculations.Fixtures) == 0 {
		t.Fatal("backup omitted executable identity or exact numeric recovery fixtures")
	}
	t.Cleanup(func() {
		for v := range admin.ListObjects(context.Background(), "monitoring-backups", minio.ListObjectsOptions{Prefix: backup.ID + "/", Recursive: true}) {
			if v.Err != nil {
				t.Error(v.Err)
				break
			}
			if e := admin.RemoveObject(context.Background(), "monitoring-backups", v.Key, minio.RemoveObjectOptions{}); e != nil {
				t.Error(e)
			}
		}
	})
	seal, e := os.ReadFile(filepath.Join(dir, "manifest.sha256"))
	must(t, e)
	targetAdmin, e := minio.New("127.0.0.1:59333", &minio.Options{Creds: credentials.NewStaticV4("restore-admin", "restore-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	for _, bucket := range []string{"restore-source", "restore-ch", "restore-backups"} {
		must(t, targetAdmin.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}))
	}
	u, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	targetURL := "postgres://restore_local:restore-local-test-only@127.0.0.1:57432/restore_local?" + u.RawQuery
	pool, e := pgxpool.New(ctx, targetURL)
	must(t, e)
	defer pool.Close()
	storage, e := md.OpenStorage(ctx, md.StorageConfig{Replicas: []string{"127.0.0.1:60000", "127.0.0.1:60001"}, Database: "restored_monitoring", Policy: "monitoring_tiered", User: "default", ObjectEndpoint: "http://127.0.0.1:59333", ObjectAccessKey: "restore-source", ObjectSecretKey: "restore-source-test-only", Bucket: "restore-source", TempDirectory: t.TempDir()})
	must(t, e)
	defer storage.Close()
	// Source CH, its S3 and the backup staging server are stopped for the restore.
	// The archive must carry actual native parts and source/evidence bytes.
	source := []string{"infrapro-monitoring-contract-ch1-1", "infrapro-monitoring-contract-ch2-1", "infrapro-monitoring-contract-s3-1", "infrapro-monitoring-contract-backup-s3-1"}
	command := func(action string) {
		t.Helper()
		out, e := exec.Command("docker", append([]string{action}, source...)...).CombinedOutput()
		if e != nil {
			t.Fatalf("%s failed: %v %s", action, e, out)
		}
	}
	command("pause")
	paused := true
	t.Cleanup(func() {
		if paused {
			command("unpause")
		}
	})
	start := time.Now()
	opt.DatabaseURL = targetURL
	opt.PGContainer = os.Getenv("INFRAPRO_RESTORE_PG_CONTAINER")
	opt.Store = md.BackupConfig{Endpoint: "http://127.0.0.1:59333", AccessKey: "restore-backup", SecretKey: "restore-backup-test-only", Bucket: "restore-backups", NamedCollection: "monitoring_backups"}
	opt.Calculations = dm.NewRecovery(storage)
	_, e = md.Restore(ctx, pool, storage, opt, strings.TrimSpace(string(seal)))
	must(t, e)
	service := md.New(access.New(pool, slog.Default())).WithStorage(storage)
	actor := func(id string) access.Actor {
		return access.Actor{ID: id, Check: func(ctx context.Context, tx pgx.Tx, _ bool) error {
			var active bool
			e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, id).Scan(&active)
			if e != nil {
				return e
			}
			if !active {
				return access.ErrDenied
			}
			return nil
		}}
	}
	tenantActor := actor(f.actor.ID)
	projectActor := actor(project.actor.ID)
	fresh := func(a access.Actor, scope md.Scope, status string) md.QueryInput {
		selection, e := service.Select(ctx, a, scope, md.SelectionInput{Channels: []string{f.channel}})
		must(t, e)
		return md.QueryInput{Selection: selection.ID, AllHistory: true, Limit: 1000, Status: status, Qualities: []string{"good", "unmarked", "suspect", "bad"}}
	}
	current, e := service.Details(ctx, tenantActor, f.scope, fresh(tenantActor, f.scope, "active"))
	must(t, e)
	all, e := service.Details(ctx, tenantActor, f.scope, fresh(tenantActor, f.scope, "history"))
	must(t, e)
	if !reflect.DeepEqual(current.Items, before.Items) || !reflect.DeepEqual(all.Items, history.Items) {
		t.Fatal("independent restore changed stable IDs, values, time or revocations")
	}
	for _, table := range proofTables {
		var hash string
		must(t, pool.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text,'[]')) FROM `+table+` r`).Scan(&hash))
		if hash != hashes[table] {
			t.Fatal("restored control evidence differs", table)
		}
	}
	scope := md.ProjectScope(f.tenant, project.id)
	partial, e := service.Details(ctx, projectActor, scope, fresh(projectActor, scope, "active"))
	must(t, e)
	if len(partial.Items) != 10 {
		t.Fatal("restored authorization boundary", partial)
	}
	_, e = service.Source(ctx, projectActor, scope, b.SourceFile)
	monitoringCode(t, e, "monitoring_not_found")
	sourceFile, e := service.Source(ctx, tenantActor, f.scope, c.SourceFile)
	must(t, e)
	var original bytes.Buffer
	must(t, sourceFile.Write(ctx, &original))
	actual := sha256.Sum256(original.Bytes())
	var expected string
	must(t, pool.QueryRow(ctx, `SELECT sha256 FROM monitoring_source_files WHERE id=$1`, c.SourceFile).Scan(&expected))
	if hex.EncodeToString(actual[:]) != expected {
		t.Fatal("restored source differs")
	}
	var sessions int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM account_sessions WHERE revoked_at IS NULL`).Scan(&sessions))
	if sessions != 0 {
		t.Fatal("old sessions revived")
	}
	_, e = service.CreateImport(ctx, tenantActor, f.scope, md.ImportInput{WriteMeta: f.meta("0"), Name: "blocked.csv", Size: "20"})
	monitoringCode(t, e, "monitoring_restoring")
	must(t, md.OpenRestoredWrites(ctx, pool, storage, backup.ID, "恢复对账已通过，开放本机写入", dm.NewRecovery(storage)))
	derivedProof.verify(t, f, pool, storage, tenantActor, projectActor, project.id, service)
	afterFixture := f
	afterFixture.service = service
	afterFixture.actor = tenantActor
	afterFixture.storage = storage
	// Publishing after release proves the restored replica group is writable.
	afterFixture.publish(t, afterFixture.validate(t, afterFixture.upload(t, "observed_at,value,quality\n2026-01-01T00:00:00Z,7,good\n")))
	final, e := service.Details(ctx, tenantActor, f.scope, fresh(tenantActor, f.scope, "active"))
	must(t, e)
	if len(final.Items) != 23 {
		t.Fatal(final)
	}
	command("unpause")
	paused = false
	report := map[string]any{"status": "passed", "backup_id": backup.ID, "backup_files": backup.Files, "backup_bytes": backup.Bytes, "backup_seconds": backup.Completed.Sub(backup.Started).Seconds(), "restore_and_verify_seconds": time.Since(start).Seconds(), "active_before": "22", "history_before": "122", "active_after_new_publication": "23", "project_visible": "10", "source_components_paused": true, "scope": "synthetic P-0, independent PG/CH/Keeper/S3/Redis containers on one local host; no production failure-domain or PITR claim"}
	report["build"] = backup.Build
	report["calculations"] = backup.Calculations
	report["derived_generations_replayed"] = "4"
	report["analysis_rows_preserved"] = "40"
	raw, e := json.MarshalIndent(report, "", "  ")
	must(t, e)
	must(t, os.WriteFile(filepath.Join(os.Getenv("INFRAPRO_RESTORE_ARTIFACTS"), "restore-proof.json"), raw, 0600))
	t.Log(string(raw))
}
