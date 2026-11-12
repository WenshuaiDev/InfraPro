//go:build monitoringintegration

package resources_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMonitoringNativeBackupCheckpoint(t *testing.T) {
	f := monitoringSetup(t)
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n1900-01-01T00:00:00Z,-99999999999999999999999999.000000000001,good\n2262-04-11T23:47:16.854775807Z,0.000000000001,good\n")))
	_ = f.analytic(t, "overview", f.query(t))
	admin, e := minio.New("127.0.0.1:58334", &minio.Options{Creds: credentials.NewStaticV4("backup-admin", "backup-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	exists, e := admin.BucketExists(ctx, "monitoring-backups")
	must(t, e)
	if !exists {
		must(t, admin.MakeBucket(ctx, "monitoring-backups", minio.MakeBucketOptions{}))
	}
	dir := filepath.Join(t.TempDir(), "checkpoint")
	opt := md.BackupOptions{Directory: dir, DatabaseURL: f.env.Config.DatabaseURL, PGContainer: "infrapro-account-tests-postgres-1", Reason: "本机真实原生备份契约", Store: md.BackupConfig{Endpoint: "http://127.0.0.1:58334", AccessKey: "backup-local", SecretKey: "backup-local-test-only", Bucket: "monitoring-backups", NamedCollection: "monitoring_backups"}}
	manifest, e := md.Backup(ctx, f.env.Pool, f.storage, opt)
	must(t, e)
	t.Cleanup(func() {
		for v := range admin.ListObjects(ctx, "monitoring-backups", minio.ListObjectsOptions{Prefix: manifest.ID + "/", Recursive: true}) {
			if v.Err != nil {
				t.Error(v.Err)
				break
			}
			if e := admin.RemoveObject(ctx, "monitoring-backups", v.Key, minio.RemoveObjectOptions{}); e != nil {
				t.Error(e)
			}
		}
	})
	if manifest.State != "complete" || len(manifest.Native) != 1 {
		t.Fatal(manifest)
	}
	seal, e := os.ReadFile(filepath.Join(dir, "manifest.sha256"))
	must(t, e)
	checked, e := md.ReadBackupManifest(dir, strings.TrimSpace(string(seal)))
	must(t, e)
	if checked.ID != manifest.ID {
		t.Fatal(checked)
	}
	must(t, os.WriteFile(filepath.Join(dir, "attempts.ndjson"), []byte("corrupt"), 0600))
	if _, e = md.ReadBackupManifest(dir, strings.TrimSpace(string(seal))); e == nil {
		t.Fatal("corrupt archive accepted")
	}
	t.Logf("native backup %s: %s files, %s bytes, %s elapsed", manifest.ID, manifest.Files, manifest.Bytes, manifest.Completed.Sub(manifest.Started))
}
