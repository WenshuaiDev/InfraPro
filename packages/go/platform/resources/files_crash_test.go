package resources_test

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type fileCrashFixture struct{ Database, Bucket, Directory, Entered string }
type fileCrashChecker struct{ entered string }

func (c fileCrashChecker) Check(ctx context.Context, _ files.CheckRequest) (files.CheckResult, error) {
	if e := os.WriteFile(c.entered, []byte("checker entered"), 0600); e != nil {
		return files.CheckResult{}, e
	}
	<-ctx.Done()
	return files.CheckResult{}, ctx.Err()
}

// This subprocess uses the actual worker orchestration, PG state and S3. A
// blocking checker provides an observable kill boundary before final binding.
func TestFilesWorkerCrashProcessHelper(t *testing.T) {
	path := os.Getenv("INFRAPRO_FILES_CRASH_FIXTURE")
	if path == "" {
		return
	}
	raw, e := os.ReadFile(path)
	must(t, e)
	var fixture fileCrashFixture
	must(t, json.Unmarshal(raw, &fixture))
	pool, e := pgxpool.New(ctx, fixture.Database)
	must(t, e)
	defer pool.Close()
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", fixture.Bucket, "files/v1")
	must(t, e)
	options := config.DefaultFiles()
	options.TempDirectory = fixture.Directory
	service := runtime.Compose(access.New(pool, slog.Default())).Files.WithOptions(options).WithStorage(store).WithChecker(fileCrashChecker{fixture.Entered})
	_, e = service.RunOnce(ctx)
	must(t, e)
	t.Fatal("kill boundary was not reached")
}
func TestFilesSIGKILLWorkerRecoversSameGeneration(t *testing.T) {
	if os.Getenv("INFRAPRO_FILES_CONTRACT") != "1" {
		t.Skip("run pnpm test:files")
	}
	f := newFilesFixture(t)
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	bucket := "files-crash-" + uuid.NewString()
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
	f.s.WithStorage(store)
	submitted := f.upload(t, []byte("persisted input survives SIGKILL"), nil, nil)
	directory := t.TempDir()
	entered := filepath.Join(directory, "entered")
	path := filepath.Join(directory, "fixture.json")
	raw, e := json.Marshal(fileCrashFixture{f.env.Config.DatabaseURL, bucket, f.options.TempDirectory, entered})
	must(t, e)
	must(t, os.WriteFile(path, raw, 0600))
	executable, e := os.Executable()
	must(t, e)
	child := exec.Command(executable, "-test.run=^TestFilesWorkerCrashProcessHelper$", "-test.timeout=2m")
	child.Env = append(os.Environ(), "INFRAPRO_FILES_CRASH_FIXTURE="+path)
	must(t, child.Start())
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	reached := false
	for n := 0; n < 200; n++ {
		if _, e := os.Stat(entered); e == nil {
			reached = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !reached {
		t.Fatal("worker never reached real persistent checker phase")
	}
	must(t, child.Process.Kill())
	if e = child.Wait(); e == nil {
		t.Fatal("worker was not killed")
	}
	clock := time.Now().UTC().Add(2 * time.Minute)
	f.s.WithClock(func() time.Time { return clock })
	recovered := f.run(t, submitted)
	if recovered.State != "succeeded" || recovered.Generation != "1" || recovered.Attempt != 2 {
		t.Fatal("crash changed generation or lost input", recovered)
	}
	var attachments, events, attempts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_attachments`).Scan(&attachments))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_events WHERE source_module='files'`).Scan(&events))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_job_attempts WHERE job_id=$1`, submitted.JobID).Scan(&attempts))
	if attachments != 1 || events != 1 || attempts != 2 {
		t.Fatal("crash duplicated business outcome", attachments, events, attempts)
	}
	t.Log("SIGKILL at checker boundary recovered generation 1 on attempt 2 with one attachment and one event")
}
