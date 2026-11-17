//go:build monitoringintegration

package monitoring

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func topologyStorage(t *testing.T, replicas int, extra bool) *Storage {
	t.Helper()
	if os.Getenv("INFRAPRO_MONITORING_CONTRACT") != "1" {
		t.Fatal("requires the disposable monitoring laboratory")
	}
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	cfg := StorageConfig{
		Replicas: []string{"127.0.0.1:59000", "127.0.0.1:59001"}[:replicas],
		Database: "topology_it_" + id, User: "default", Policy: "monitoring_tiered",
		ObjectEndpoint: "http://127.0.0.1:58333", ObjectAccessKey: "contract-admin", ObjectSecretKey: "contract-admin-test-only",
		Bucket: "topology-" + id, TempDirectory: t.TempDir(),
	}
	if extra {
		cfg.Shards = []PhysicalShardConfig{{Name: "extra", Database: cfg.Database + "_extra", Replicas: cfg.Replicas[:1]}}
	}
	s, err := OpenStorage(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		defer s.Close()
		for _, name := range s.physicalNames() {
			p, err := s.physical(name)
			if err != nil {
				t.Error(err)
				continue
			}
			for _, c := range p.replicas {
				if err := c.Exec(ctx, "DROP DATABASE IF EXISTS "+p.config.Database+" SYNC"); err != nil {
					t.Error(err)
				}
			}
		}
		if err := s.objects.RemoveBucket(ctx, cfg.Bucket); err != nil {
			t.Error(err)
		}
	})
	for i := 0; i < 2; i++ {
		if err := s.Initialize(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStorageOneOrMoreReplicas(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprintf("replicas_%d", count), func(t *testing.T) {
			s := topologyStorage(t, count, true)
			ctx := t.Context()
			var first selectedChunk
			for _, name := range s.physicalNames() {
				p, err := s.physical(name)
				if err != nil {
					t.Fatal(err)
				}
				var total, active uint32
				if err := p.replicas[0].QueryRow(ctx, "SELECT total_replicas,active_replicas FROM system.replicas WHERE database=? AND table='observations'", p.config.Database).Scan(&total, &active); err != nil {
					t.Fatal(err)
				}
				if int(total) != len(p.replicas) || total != active {
					t.Fatalf("wrong real replica topology for %s: total=%d active=%d", name, total, active)
				}
				row := rawObservation{
					Tenant: uuid.NewString(), Channel: uuid.NewString(), Observation: uuid.NewString(), Adoption: uuid.NewString(), Batch: uuid.NewString(), File: uuid.NewString(),
					At: time.Now().UTC().UnixNano(), Received: time.Now().UTC().UnixNano(), Precision: 9,
					Kind: "decimal", Value: "99999999999999999999999999.000000000003", Quality: "good", Original: "exact decimal", Mapping: 1, Validation: 1, First: true,
				}
				rows := []rawObservation{row}
				entry := selectedChunk{Physical: name, Chunk: uuid.NewString(), Attempt: uuid.NewString(), Hash: payloadHash(rows), Count: 1}
				if err := s.writeAt(ctx, name, entry.Chunk, entry.Attempt, rows); err != nil {
					t.Fatal(err)
				}
				if err := s.sealCheck(ctx, entry); err != nil {
					t.Fatal(err)
				}
				if _, err := p.readyReplica(ctx, []selectedChunk{entry}); err != nil {
					t.Fatal(err)
				}
				bad := entry
				bad.Hash = strings.Repeat("0", 64)
				if _, err := p.readyReplica(ctx, []selectedChunk{bad}); err == nil {
					t.Fatal("corrupt publication manifest accepted")
				}
				if name == "local" {
					first = entry
				}
			}
			if count == 1 {
				topologyBackupRestore(t, s, first)
			}
			t.Logf("%d real replica(s) plus a single-node additional shard: init/check/write/seal/read passed", count)
		})
	}
}

func topologyBackupRestore(t *testing.T, source *Storage, entry selectedChunk) {
	t.Helper()
	ctx := t.Context()
	admin, err := minio.New("127.0.0.1:58334", &minio.Options{Creds: credentials.NewStaticV4("backup-admin", "backup-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	if err != nil {
		t.Fatal(err)
	}
	exists, err := admin.BucketExists(ctx, "monitoring-backups")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		if err := admin.MakeBucket(ctx, "monitoring-backups", minio.MakeBucketOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	prefix := "single-node/" + uuid.NewString() + "/"
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for object := range admin.ListObjects(ctx, "monitoring-backups", minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			if object.Err != nil {
				t.Error(object.Err)
				break
			}
			if err := admin.RemoveObject(ctx, "monitoring-backups", object.Key, minio.RemoveObjectOptions{}); err != nil {
				t.Error(err)
			}
		}
	})
	if _, err := source.nativeOperation(ctx, "BACKUP DATABASE "+source.config.Database+" TO", "monitoring_backups", prefix); err != nil {
		t.Fatal(err)
	}
	target := topologyStorage(t, 1, false)
	if _, err := target.nativeOperation(ctx, "RESTORE TABLE "+source.config.Database+".observations AS "+target.config.Database+".observations FROM", "monitoring_backups", prefix); err != nil {
		t.Fatal(err)
	}
	if err := target.replicas[0].Exec(ctx, "SYSTEM SYNC REPLICA "+target.config.Database+".observations"); err != nil {
		t.Fatal(err)
	}
	if err := target.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if err := target.sealCheck(ctx, entry); err != nil {
		t.Fatal(err)
	}
	t.Log("single-node native backup restored into an independent empty database with matching payload integrity")
}
