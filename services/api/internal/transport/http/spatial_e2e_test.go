package http_test

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"os"
	"testing"
	"time"
)

// The spatial browser lab uses the same isolated PG schema and real private S3.
func browserSpatial(t *testing.T, ctx context.Context, env *testsupport.Environment, storage *monitoring.Storage) func() {
	t.Helper()
	if os.Getenv("INFRAPRO_SPATIAL_E2E") != "1" {
		return func() {}
	}
	env.Config.SpatialPython = os.Getenv("INFRAPRO_SPATIAL_PYTHON")
	env.Config.SpatialConverterScript = os.Getenv("INFRAPRO_SPATIAL_CONVERTER_SCRIPT")
	env.Config.SpatialTempDirectory = t.TempDir()
	env.Config.Monitoring.ObjectEndpoint = "http://127.0.0.1:58333"
	env.Config.Monitoring.ObjectAccessKey = "contract-admin"
	env.Config.Monitoring.ObjectSecretKey = "contract-admin-test-only"
	env.Config.Monitoring.Bucket = "contract-source"
	service := runtime.Compose(env.Service.Access(), storage).Configure(env.Config).Spatial
	if e := (spatial.PythonConverter{Python: env.Config.SpatialPython, Script: env.Config.SpatialConverterScript}).Ready(ctx); e != nil {
		t.Fatal(e)
	}
	worker, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.RunWorker(worker, "spatial-browser", func(e error) { t.Log("spatial worker", e) })
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("spatial worker did not stop")
		}
		clean, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
		if e != nil {
			t.Error(e)
			return
		}
		rows, e := env.Pool.Query(clean, `SELECT id::text FROM tenants`)
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
			for o := range client.ListObjects(clean, "contract-source", minio.ListObjectsOptions{Prefix: "spatial/" + id + "/", Recursive: true}) {
				if o.Err != nil {
					t.Error(o.Err)
					break
				}
				if e = client.RemoveObject(clean, "contract-source", o.Key, minio.RemoveObjectOptions{}); e != nil {
					t.Error(e)
				}
			}
		}
	}
}
