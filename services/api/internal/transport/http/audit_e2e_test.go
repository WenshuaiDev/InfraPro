package http_test

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"os"
	"testing"
	"time"
)

// The browser fixture starts with no ClickHouse, parser or file checker. Its
// external audit worker and notification worker share only this isolated schema.
func browserAudit(t *testing.T, ctx context.Context, env *testsupport.Environment) func() {
	t.Helper()
	if os.Getenv("INFRAPRO_AUDIT_E2E") != "1" {
		return func() {}
	}
	o := config.DefaultAudit()
	o.TempDirectory = t.TempDir()
	o.NodeID = "audit-browser"
	o.ObjectEndpoint = "http://127.0.0.1:58333"
	o.ObjectAccessKey = "contract-admin"
	o.ObjectSecretKey = "contract-admin-test-only"
	o.Bucket = "audit-browser-" + uuid.NewString()
	env.Config.Audit = o
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4(o.ObjectAccessKey, o.ObjectSecretKey, ""), BucketLookup: minio.BucketLookupPath})
	if e != nil {
		t.Fatal(e)
	}
	if e = client.MakeBucket(ctx, o.Bucket, minio.MakeBucketOptions{}); e != nil {
		t.Fatal(e)
	}
	path := os.Getenv("INFRAPRO_E2E_OPS_CONFIG")
	values := map[string]any{}
	if raw, e := os.ReadFile(path); e == nil {
		if e = json.Unmarshal(raw, &values); e != nil {
			t.Fatal(e)
		}
	}
	values["database_url"] = env.Config.DatabaseURL
	values["audit"] = map[string]string{"object_endpoint": o.ObjectEndpoint, "access_key": o.ObjectAccessKey, "secret_key": o.ObjectSecretKey, "bucket": o.Bucket, "temp_directory": o.TempDirectory, "node_id": o.NodeID}
	raw, e := json.Marshal(values)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return func() {
		clean, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for obj := range client.ListObjects(clean, o.Bucket, minio.ListObjectsOptions{Recursive: true}) {
			if obj.Err == nil {
				_ = client.RemoveObject(clean, o.Bucket, obj.Key, minio.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(clean, o.Bucket)
		_ = os.Remove(path)
	}
}
