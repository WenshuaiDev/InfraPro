package http_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// The browser suite opts into the already-qualified disposable contract lab.
// Every run owns a new CH database and only the tenant prefixes in its PG schema.
func browserMonitoring(t *testing.T, ctx context.Context, env *testsupport.Environment) (*monitoring.Storage, func()) {
	t.Helper()
	if os.Getenv("INFRAPRO_MONITORING_E2E") != "1" {
		return nil, func() {}
	}
	database := "monitoring_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	storage, err := monitoring.OpenStorage(ctx, monitoring.StorageConfig{Replicas: []string{"127.0.0.1:59000", "127.0.0.1:59001"}, Database: database, User: "default", Policy: "monitoring_tiered", ObjectEndpoint: "http://127.0.0.1:58333", ObjectAccessKey: "contract-admin", ObjectSecretKey: "contract-admin-test-only", Bucket: "contract-source", TempDirectory: t.TempDir(), ExportRows: 500})
	if err != nil {
		t.Fatal(err)
	}
	if err = storage.Initialize(ctx); err != nil {
		storage.Close()
		t.Fatal(err)
	}
	if err = storage.Check(ctx); err != nil {
		storage.Close()
		t.Fatal(err)
	}
	worker, stop := context.WithCancel(ctx)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = runtime.Compose(env.Service.Access(), storage).Monitoring.RunWorker(worker, logger)
	}()
	return storage, func() {
		stop()
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			t.Error("monitoring browser worker did not stop")
		}
		storage.Close()
		clean, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		for _, port := range []string{"59000", "59001"} {
			ch, e := clickhouse.Open(&clickhouse.Options{Addr: []string{"127.0.0.1:" + port}})
			if e != nil {
				t.Error(e)
				continue
			}
			if e = ch.Exec(clean, "DROP DATABASE "+database+" SYNC"); e != nil {
				t.Error(e)
			}
			_ = ch.Close()
		}
		objects, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
		if e != nil {
			t.Error(e)
			return
		}
		rows, e := env.Pool.Query(clean, "SELECT id::text FROM tenants")
		if e != nil {
			t.Error(e)
			return
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				t.Error(e)
				break
			}
			ids = append(ids, id)
		}
		rows.Close()
		for _, id := range ids {
			for object := range objects.ListObjects(clean, "contract-source", minio.ListObjectsOptions{Prefix: id + "/", Recursive: true}) {
				if object.Err != nil {
					t.Error(object.Err)
					break
				}
				if e = objects.RemoveObject(clean, "contract-source", object.Key, minio.RemoveObjectOptions{}); e != nil {
					t.Error(e)
				}
			}
		}
	}
}
