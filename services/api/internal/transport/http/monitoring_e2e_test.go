package http_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
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
	temporary := t.TempDir()
	storage, err := monitoring.OpenStorage(ctx, monitoring.StorageConfig{Replicas: []string{"127.0.0.1:59000", "127.0.0.1:59001"}, Database: database, User: "default", Policy: "monitoring_tiered", ObjectEndpoint: "http://127.0.0.1:58333", ObjectAccessKey: "contract-admin", ObjectSecretKey: "contract-admin-test-only", Bucket: "contract-source", TempDirectory: temporary, ExportRows: 500})
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
	if err = derivedmonitoring.InitializeStorage(ctx, storage); err != nil {
		storage.Close()
		t.Fatal(err)
	}
	// Optional test-only operator connection, used to exercise the actual CLI
	// against this real listener/schema. Never expose it through an HTTP route.
	if path := os.Getenv("INFRAPRO_E2E_OPS_CONFIG"); path != "" {
		if !filepath.IsAbs(path) {
			t.Fatal("operator fixture configuration must use an absolute path")
		}
		body, e := json.MarshalIndent(map[string]string{"database_url": env.Config.DatabaseURL, "monitoring_clickhouse_replicas": "127.0.0.1:59000,127.0.0.1:59001", "monitoring_clickhouse_database": database, "monitoring_clickhouse_user": "default", "monitoring_clickhouse_policy": "monitoring_tiered", "monitoring_object_endpoint": "http://127.0.0.1:58333", "monitoring_object_access_key": "contract-admin", "monitoring_object_secret_key": "contract-admin-test-only", "monitoring_source_bucket": "contract-source", "monitoring_temp_directory": temporary, "monitoring_export_rows_per_file": "500"}, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			t.Fatal(e)
		}
		_, e = file.Write(body)
		closeError := file.Close()
		if e != nil || closeError != nil {
			t.Fatal("cannot write isolated operator configuration", e, closeError)
		}
		t.Cleanup(func() { _ = os.Remove(path) })
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
