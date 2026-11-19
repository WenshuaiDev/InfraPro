//go:build monitoringintegration

package monitoring

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/requestlimit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

func TestRoleStorageHardBudgets(t *testing.T) {
	if os.Getenv("INFRAPRO_MONITORING_CONTRACT") != "1" {
		t.Fatal("requires the disposable monitoring laboratory")
	}
	ctx := t.Context()
	quota := requestlimit.New(1)
	cfg := StorageConfig{
		Replicas: []string{"127.0.0.1:59000"}, Shards: []PhysicalShardConfig{{Name: "extra", Database: "quota_extra", Replicas: []string{"127.0.0.1:59001"}}},
		ConnectionsPerReplica: 1, ObjectRequests: quota, Database: "quota_base", Policy: "monitoring_tiered", User: "default",
		ObjectEndpoint: "http://127.0.0.1:58333", ObjectAccessKey: "contract-admin", ObjectSecretKey: "contract-admin-test-only",
		Bucket: "quota-" + strings.ReplaceAll(uuid.NewString(), "-", ""),
	}
	storage, err := OpenStorage(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer storage.Close()
	if storage.ConnectionLimit() != 2 || !quota.Attached() {
		t.Fatal("incorrect physical client limits")
	}
	// Occupy the first endpoint while a separate physical shard stays readable.
	first := storage.replicas[0]
	done := make(chan error, 1)
	go func() {
		var result uint8
		done <- first.QueryRow(ctx, "SELECT sleep(0.6)").Scan(&result)
	}()
	deadline := time.Now().Add(time.Second)
	for first.Stats().Open == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	check, cancel := context.WithTimeout(ctx, 70*time.Millisecond)
	err = first.Ping(check)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second ClickHouse operation escaped one-slot budget: %v", err)
	}
	if err = storage.shards["extra"].replicas[0].Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = first.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err = storage.objects.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := storage.objects.RemoveBucket(ctx, cfg.Bucket); err != nil {
			t.Error(err)
		}
	}()
	other, err := blobstore.Open(cfg.ObjectEndpoint, cfg.ObjectAccessKey, cfg.ObjectSecretKey, cfg.Bucket, "fixture", quota)
	if err != nil {
		t.Fatal(err)
	}
	key := "fixture/transfer"
	data := bytes.Repeat([]byte("x"), 2<<20)
	if err = other.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := other.Delete(ctx, key); err != nil {
			t.Error(err)
		}
	}()
	body, err := other.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if _, err = io.ReadFull(body, data[:1]); err != nil {
		t.Fatal(err)
	}
	check, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = storage.objects.BucketExists(check, cfg.Bucket)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second object namespace escaped shared one-slot budget: %v", err)
	}
	if err = body.Close(); err != nil {
		t.Fatal(err)
	}
	if found, err := storage.objects.BucketExists(ctx, cfg.Bucket); err != nil || !found {
		t.Fatalf("object slot not released: %v", err)
	}
}
