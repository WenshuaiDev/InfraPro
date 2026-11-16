package http_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"os"
	"testing"
)

func browserParsing(t *testing.T, ctx context.Context, env *testsupport.Environment) func() {
	if os.Getenv("INFRAPRO_PARSING_E2E") != "1" {
		return func() {}
	}
	opts := config.Parsing{Enabled: true, URL: os.Getenv("INFRAPRO_PARSING_URL"), TokenFile: os.Getenv("INFRAPRO_PARSING_TOKEN_FILE"), Queue: 20, Concurrency: 2, StorageBudget: 1 << 30, Timeout: 10000000000}
	env.Config.Parsing = opts
	m := &env.Config.Parsing
	m.ObjectEndpoint = "http://127.0.0.1:58333"
	m.ObjectAccessKey = "contract-admin"
	m.ObjectSecretKey = "contract-admin-test-only"
	m.Bucket = "parsing-browser-" + uuid.NewString()
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4(m.ObjectAccessKey, m.ObjectSecretKey, ""), BucketLookup: minio.BucketLookupPath})
	if e != nil {
		t.Fatal(e)
	}
	if e = client.MakeBucket(ctx, m.Bucket, minio.MakeBucketOptions{}); e != nil {
		t.Fatal(e)
	}
	if path := os.Getenv("INFRAPRO_E2E_OPS_CONFIG"); path != "" {
		raw, _ := json.Marshal(map[string]any{"database_url": env.Config.DatabaseURL, "redis_url": env.Config.RedisURL, "support_contact": env.Config.SupportContact, "csrf_auth_key": hex.EncodeToString(env.Config.CSRFKey), "parsing": map[string]string{"enabled": "true", "url": opts.URL, "token_file": opts.TokenFile, "object_endpoint": m.ObjectEndpoint, "object_access_key": m.ObjectAccessKey, "object_secret_key": m.ObjectSecretKey, "bucket": m.Bucket}})
		if e = os.WriteFile(path, raw, 0600); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = os.Remove(path) })
	}
	return func() {
		clean := context.Background()
		for item := range client.ListObjects(clean, m.Bucket, minio.ListObjectsOptions{Recursive: true}) {
			if item.Err == nil {
				_ = client.RemoveObject(clean, m.Bucket, item.Key, minio.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(clean, m.Bucket)
	}
}
