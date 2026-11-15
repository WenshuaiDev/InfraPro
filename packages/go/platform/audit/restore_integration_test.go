package audit_test

import (
	"bytes"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAuditFrozenBackupIndependentRestore(t *testing.T) {
	f := setup(t)
	source := exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	tenant := f.tenant(t, "restore-business")
	f.project(t, tenant, "restore-project")
	completed, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	pending, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	oldCursor, e := s.Events(ctx, f.actor, scope, audit.Filter{Limit: 1})
	must(t, e)
	if oldCursor.NextCursor == "" {
		t.Fatal("cursor fixture missing")
	}
	read, e := s.OpenRead(ctx, f.actor, scope, completed.ID, "1", "GET", "before-backup")
	must(t, e)
	defer read.Close()
	_, e = f.env.Pool.Exec(ctx, `UPDATE audit_export_generations SET state='running',fence=17,attempt=1,lease_owner='stopped-worker',lease_until=clock_timestamp()+interval '1 minute' WHERE export_id=$1`, pending.ID)
	must(t, e)
	_, e = s.Hold(ctx, "backup", "audit-isolated-point", "freeze a consistent small business chain", time.Now().Add(time.Hour))
	must(t, e)
	must(t, s.SetMaintenanceMode(ctx, "read_only", uuid.NewString(), "frozen small restore exercise"))
	manifest, e := s.Manifest(ctx, "audit-isolated-point", "")
	must(t, e)
	if len(manifest.Objects) != 1 {
		t.Fatal(manifest)
	}
	var beforeNotifications, sourceEvents int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events`).Scan(&beforeNotifications))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events`).Scan(&sourceEvents))
	var schema string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
	dump, e := exec.Command("docker", "exec", "infrapro-account-tests-postgres-1", "pg_dump", "-U", "infrapro_test", "-d", "infrapro_test", "-Fc", "--no-owner", "--no-privileges", "--schema", schema).Output()
	must(t, e)
	directory := t.TempDir()
	must(t, os.WriteFile(filepath.Join(directory, "database.dump"), dump, 0600))
	admin, e := pgxpool.New(ctx, os.Getenv("INFRAPRO_TEST_DATABASE_URL"))
	must(t, e)
	defer admin.Close()
	database := "audit_restore_" + strings.ReplaceAll(uuid.NewString(), "-", "") + "_test"
	_, e = admin.Exec(ctx, "CREATE DATABASE "+database)
	must(t, e)
	defer admin.Exec(ctx, "DROP DATABASE "+database+" WITH (FORCE)")
	if out, e := exec.Command("docker", "exec", "infrapro-account-tests-postgres-1", "psql", "-U", "infrapro_test", "-d", database, "-v", "ON_ERROR_STOP=1", "-c", "CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public").CombinedOutput(); e != nil {
		t.Fatalf("extension %v %s", e, out)
	}
	restore := exec.Command("docker", "exec", "-i", "infrapro-account-tests-postgres-1", "pg_restore", "-U", "infrapro_test", "-d", database, "--no-owner", "--no-privileges", "--exit-on-error")
	restore.Stdin = bytes.NewReader(dump)
	if out, e := restore.CombinedOutput(); e != nil {
		t.Fatalf("pg_restore %v %s", e, out)
	}
	destination, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	destination.Path = "/" + database
	pool, e := pgxpool.New(ctx, destination.String())
	must(t, e)
	defer pool.Close()
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	bucket := "audit-restore-" + uuid.NewString()
	must(t, client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}))
	defer func() {
		for o := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if o.Err == nil {
				_ = client.RemoveObject(ctx, bucket, o.Key, minio.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(ctx, bucket)
	}()
	objectStore, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", bucket, "audit-exports")
	must(t, e)
	recovered := runtime.Compose(access.New(pool, slog.Default())).Audit.WithStorage(objectStore)
	verification, e := recovered.Verify(ctx, "content")
	must(t, e)
	if verification.Invalid != "1" {
		t.Fatal("absent body not detected", verification)
	}
	for _, object := range manifest.Objects {
		r, e := source.Get(ctx, object.Key)
		must(t, e)
		body, e := io.ReadAll(r)
		must(t, e)
		must(t, r.Close())
		size, e := strconv.ParseInt(object.Size, 10, 64)
		must(t, e)
		if !audit.VerifyCSV(body, size, object.SHA256) {
			t.Fatal("source manifest mismatch")
		}
		must(t, objectStore.Put(ctx, object.Key, bytes.NewReader(body), size, "text/csv"))
	}
	verification, e = recovered.Verify(ctx, "content")
	must(t, e)
	if verification.Invalid != "0" || verification.Checked != "1" {
		t.Fatal(verification)
	}
	must(t, recovered.SetMaintenanceMode(ctx, "reconciling", uuid.NewString(), "restored consistent database and copied private objects"))
	request := uuid.NewString()
	summary, e := recovered.RestoreReconcile(ctx, request, "rotate restored protocols and fence interrupted work")
	must(t, e)
	again, e := recovered.RestoreReconcile(ctx, request, "idempotent recovery replay")
	must(t, e)
	if summary["epoch"] != again["epoch"] {
		t.Fatal("replay rotated epoch again")
	}
	if summary["epoch"] == manifest.Epoch || summary["interrupted_exports"] != 1 || summary["interrupted_transfers"] != 1 {
		t.Fatal(summary)
	}
	_, e = recovered.Events(ctx, f.actor, scope, audit.Filter{Limit: 1, Cursor: oldCursor.NextCursor})
	errorCode(t, e, "audit_cursor_stale")
	var afterNotifications, openReads, oldFence, activeBusiness int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM notification_events`).Scan(&afterNotifications))
	if afterNotifications != beforeNotifications {
		t.Fatal("restore reissued notifications")
	}
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_export_read_leases WHERE closed_at IS NULL`).Scan(&openReads))
	if openReads != 0 {
		t.Fatal("restored old read lease still open")
	}
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_export_generations WHERE export_id=$1 AND fence=17 AND state='running'`, pending.ID).Scan(&oldFence))
	if oldFence != 0 {
		t.Fatal("old worker fence survived")
	}
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM projects WHERE tenant_id=$1`, tenant).Scan(&activeBusiness))
	if activeBusiness != 1 {
		t.Fatal("business chain absent after restore")
	}
	interrupted, e := recovered.Export(ctx, f.actor, scope, pending.ID, "1")
	must(t, e)
	if interrupted.State != "failed" || interrupted.ErrorCode != "restore_interrupted" {
		t.Fatal(interrupted)
	}
	terminal, e := recovered.Export(ctx, f.actor, scope, completed.ID, "1")
	must(t, e)
	if terminal.State != "succeeded" || terminal.ResultState != "invalidated" {
		t.Fatal("old authorization regained body", terminal)
	}
	must(t, recovered.SetMaintenanceMode(ctx, "ready", uuid.NewString(), "verification completed in isolated test"))
	_, e = recovered.OpenRead(ctx, f.actor, scope, completed.ID, "1", "HEAD", "restored-old-page")
	errorCode(t, e, "audit_export_invalidated")
	var currentSourceEvents int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events`).Scan(&currentSourceEvents))
	if currentSourceEvents != sourceEvents {
		t.Fatal("restore changed source database")
	}
	t.Logf("restored independent PostgreSQL database and S3 bucket; %d source events, %s verified CSV; old epochs/cursor/fence/lease rejected; no notification replay", sourceEvents, verification.Checked)
}
