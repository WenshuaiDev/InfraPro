package resources_test

import (
	"bytes"
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesFrozenBackupIndependentDatabaseAndObjectsRestore(t *testing.T) {
	if os.Getenv("INFRAPRO_FILES_RESTORE") != "1" {
		t.Skip("run pnpm test:files:restore")
	}
	f := newFilesFixture(t)
	first := f.run(t, f.upload(t, []byte("backup fixed source one"), nil, nil))
	if first.State != "succeeded" {
		t.Fatal(first)
	}
	second := f.run(t, f.upload(t, []byte("backup second source"), nil, nil))
	if second.State != "succeeded" {
		t.Fatal(second)
	}
	pending := f.upload(t, []byte("queued across frozen point"), nil, nil)
	data := []byte("receiving interrupted")
	input := files.StartUploadInput{WriteMeta: f.meta(), Host: f.scope.Host(), Intent: "add", Filename: "interrupted.txt", DisplayName: "断点", SizeBytes: "21", SHA256: fileDigest(data), DeclaredMediaType: "text/plain"}
	input.SizeBytes = "21"
	receiving, e := f.s.StartUpload(ctx, f.actor, f.scope, input)
	must(t, e)
	page, e := f.s.Attachments(ctx, f.actor, f.scope, files.AttachmentFilter{Limit: 1})
	must(t, e)
	if page.NextCursor == "" {
		t.Fatal("missing cursor")
	}
	read, e := f.s.OpenRead(ctx, f.actor, f.scope, *first.ResultAttachmentID, *first.ResultRevisionID, "source")
	must(t, e)
	defer read.Close()
	directory := t.TempDir()
	manifest, e := f.s.BackupPrepare(ctx, "isolated-file-snapshot", "文件备份恢复功能验证")
	must(t, e)
	blocked := input
	blocked.WriteMeta = f.meta()
	_, e = f.s.StartUpload(ctx, f.actor, f.scope, blocked)
	if e == nil {
		t.Fatal("backup barrier bypassed")
	}
	var schema string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
	dump, e := exec.Command("docker", "exec", "infrapro-account-tests-postgres-1", "pg_dump", "-U", "infrapro_test", "-d", "infrapro_test", "-Fc", "--no-owner", "--no-privileges", "--schema", schema).Output()
	must(t, e)
	must(t, os.WriteFile(filepath.Join(directory, "database.dump"), dump, 0600))
	_, e = f.s.BackupCopy(ctx, manifest.ID, directory)
	must(t, e)
	sealed, e := f.s.BackupSeal(ctx, manifest.ID, directory, "校验冻结点数据库与每个对象")
	must(t, e)
	if sealed.State != "sealed" || sealed.DatabaseHash == "" {
		t.Fatal(sealed)
	}
	admin, e := pgxpool.New(ctx, os.Getenv("INFRAPRO_TEST_DATABASE_URL"))
	must(t, e)
	defer admin.Close()
	db := "files_restore_" + strings.ReplaceAll(uuid.NewString(), "-", "") + "_test"
	_, e = admin.Exec(ctx, "CREATE DATABASE "+db)
	must(t, e)
	defer admin.Exec(ctx, "DROP DATABASE "+db+" WITH (FORCE)")
	if out, e := exec.Command("docker", "exec", "infrapro-account-tests-postgres-1", "psql", "-U", "infrapro_test", "-d", db, "-v", "ON_ERROR_STOP=1", "-c", "CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public").CombinedOutput(); e != nil {
		t.Fatalf("extension %v %s", e, out)
	}
	restore := exec.Command("docker", "exec", "-i", "infrapro-account-tests-postgres-1", "pg_restore", "-U", "infrapro_test", "-d", db, "--no-owner", "--no-privileges", "--exit-on-error")
	restore.Stdin = bytes.NewReader(dump)
	if out, e := restore.CombinedOutput(); e != nil {
		t.Fatalf("restore %v %s", e, out)
	}
	destination, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	destination.Path = "/" + db
	pool, e := pgxpool.New(ctx, destination.String())
	must(t, e)
	defer pool.Close()
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	bucket := "files-restore-" + uuid.NewString()
	must(t, client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}))
	defer func() {
		for o := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if o.Err == nil {
				_ = client.RemoveObject(ctx, bucket, o.Key, minio.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(ctx, bucket)
	}()
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", bucket, "files/v1")
	must(t, e)
	restored := runtime.Compose(access.New(pool, slog.Default())).Files.WithOptions(f.options).WithStorage(store).WithChecker(fileLocalChecker{f.options.TempDirectory})
	actor := f.actor
	actor.Check = func(ctx context.Context, tx pgx.Tx, _ bool) error {
		var valid bool
		e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, actor.ID).Scan(&valid)
		if e != nil {
			return e
		}
		if !valid {
			return access.ErrDenied
		}
		return nil
	}
	var before int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM notification_events`).Scan(&before))
	verification, e := restored.RestoreReconcile(ctx, "缺对象必须阻止恢复")
	if e == nil && verification.Complete {
		t.Fatal("missing bodies reported restored")
	}
	status, e := restored.Status(ctx)
	must(t, e)
	if status.Mode != "reconciling" || status.RestoreEpoch == f.epoch {
		t.Fatal(status)
	}
	_, e = restored.CleanupPlan(ctx, "test", "恢复期间禁止删除")
	if e == nil {
		t.Fatal("cleanup during reconciliation")
	}
	_, e = restored.RestoreObjects(ctx, directory, "导入真实备份的固定对象")
	must(t, e)
	verification, e = restored.RestoreReconcile(ctx, "重新核验全部关系和摘要")
	must(t, e)
	if !verification.Complete {
		t.Fatal(verification)
	}
	caps, e := restored.Capabilities(ctx, actor, f.scope)
	must(t, e)
	if caps.RestoreEpoch == f.epoch || caps.Mode != "online" {
		t.Fatal(caps)
	}
	_, e = restored.Attachments(ctx, actor, f.scope, files.AttachmentFilter{Limit: 1, Cursor: page.NextCursor})
	fileError(t, e, "cursor_stale")
	_, e = restored.StartUpload(ctx, actor, f.scope, input)
	fileError(t, e, "file_epoch_changed")
	u, e := restored.Upload(ctx, actor, f.scope, *receiving.UploadID)
	must(t, e)
	if u.State != "expired" {
		t.Fatal(u.State)
	}
	j, e := restored.Job(ctx, actor, f.scope, *pending.JobID, "")
	must(t, e)
	if j.State != "failed" || j.ErrorCode != "restore_interrupted" {
		t.Fatal(j)
	}
	var live int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM file_read_leases WHERE closed_at IS NULL`).Scan(&live))
	if live != 0 {
		t.Fatal("old read leases active")
	}
	r, e := restored.OpenRead(ctx, actor, f.scope, *first.ResultAttachmentID, *first.ResultRevisionID, "source")
	must(t, e)
	var body bytes.Buffer
	must(t, r.Write(ctx, 0, r.Size, body.Write))
	r.Close()
	if body.String() != "backup fixed source one" {
		t.Fatal(body.String())
	}
	var after int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM notification_events`).Scan(&after))
	if before != after {
		t.Fatal("restore replayed notifications")
	}
	t.Logf("restored actual PGDMP into independent database and %d verified objects into independent S3 bucket; missing objects blocked reconciliation; old epoch/cursor/leases rejected; historical notifications unchanged", sealed.Count)
}
