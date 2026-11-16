package parsing_test

import (
	"bytes"
	"context"
	"errors"
	wire "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	parse "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type faultStore struct {
	parse.ObjectStore
	writes int
	after  func(int) error
}

func (s *faultStore) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if e := s.ObjectStore.Put(ctx, key, body, size, contentType); e != nil {
		return e
	}
	s.writes++
	return s.after(s.writes)
}
func TestObjectCommitInterruptionMissingAndRetention(t *testing.T) {
	f := setup(t)
	f.sync(t)
	scope := c.TenantScope(f.tenant)
	raw, e := os.ReadFile("../../../../samples/parsing/measurement.bin")
	must(t, e)
	sample, e := f.ps.CreateSample(ctx, f.owner, scope, parse.SampleInput{Meta: f.meta(t), Representation: "binary", Source: "synthetic", ReceivedAt: time.Now().UTC()})
	must(t, e)
	upload := f.meta(t)
	f.ps.WithStorage(&faultStore{ObjectStore: f.store, after: func(n int) error {
		if n == 2 {
			_, e := f.env.Pool.Exec(ctx, `UPDATE accounts SET status='disabled' WHERE id=$1`, f.owner.ID)
			return e
		}
		return errors.New("injected lost acknowledgement")
	}})
	_, e = f.ps.UploadSample(ctx, f.owner, scope, sample.ID, upload, bytes.NewReader(raw))
	if e == nil {
		t.Fatal("revoked post-upload commit")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE accounts SET status='active' WHERE id=$1`, f.owner.ID)
	must(t, e)
	f.ps.WithStorage(f.store)
	_, e = f.ps.UploadSample(ctx, f.owner, scope, sample.ID, upload, bytes.NewReader(raw))
	must(t, e)
	_, e = f.ps.ConfirmSample(ctx, f.owner, scope, sample.ID, f.meta(t))
	must(t, e)
	r := f.methodRun(t, scope, f.owner, sample.ID, "1.0.0")
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	var key string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT o.object_key FROM parsing_run_executions e JOIN parsing_objects o ON o.id=e.result_object_id WHERE run_id=$1`, r.ID).Scan(&key))
	must(t, f.store.Delete(ctx, key))
	result, _, e := f.ps.Result(ctx, f.owner, scope, r.ID, "1", false, 0, 100)
	must(t, e)
	if result.State != "missing" {
		t.Fatal(result)
	}
	f.checkRun(t, scope, f.owner, r.ID, "succeeded", "")
	// Frozen sample/body identity is enforced independently by PostgreSQL.
	for _, sql := range []string{`UPDATE parsing_runs SET input_snapshot='{}'`, `UPDATE parsing_samples SET sha256=repeat('0',64) WHERE state='ready'`, `UPDATE parsing_run_executions SET error_code='rewrite' WHERE state='succeeded'`} {
		_, e = f.env.Pool.Exec(ctx, sql)
		if e == nil {
			t.Fatal("mutable evidence", sql)
		}
	}
	// An old, unreferenced intent is bounded cleanup input. Referenced evidence is retained.
	var orphan string
	must(t, f.env.Pool.QueryRow(ctx, `INSERT INTO parsing_objects(scope_key,object_key,state,size_bytes,created_at) VALUES('platform','parsing/v1/platform/'||gen_random_uuid(),'intent',16,clock_timestamp()-interval '25 hours') RETURNING id::text`).Scan(&orphan))
	must(t, f.ps.Cleanup(ctx))
	var state string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT state FROM parsing_objects WHERE id=$1`, orphan).Scan(&state))
	if state != "deleted" {
		t.Fatal("orphan not cleaned")
	}
	s, e := f.ps.Sample(ctx, f.owner, scope, sample.ID)
	must(t, e)
	if s.State != "ready" {
		t.Fatal("referenced sample expired")
	}
}
func TestIndependentPostgresS3AndPackageRestore(t *testing.T) {
	f := setup(t)
	f.sync(t)
	pkg := os.Getenv("INFRAPRO_PARSER_TEST_PACKAGE")
	if pkg == "" {
		t.Skip("packaged artifact fixture required")
	}
	scope := c.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "backup", Name: "模拟厂家"}, access.Meta{})
	must(t, e)
	product := f.product(t, c.PlatformScope(), f.actor(), m.ID, "backup", []c.Slot{f.fixed()})
	cfg := f.config(t, product, "1.0.0")
	def, e := f.ps.SetDefault(ctx, f.actor(), c.PlatformScope(), product.ID, product.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: "0", ConfigRevisionID: cfg.ConfigRevisionID})
	must(t, e)
	device, e := f.b.Devices.Create(ctx, f.owner, scope, devices.DeviceCreate{Meta: f.meta(t), ProductVersionID: product.VersionID, Code: "RESTORE", Name: "恢复模拟设备", ParsingAdoption: &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: def.Revision}}, access.Meta{})
	must(t, e)
	sample := f.sample(t, scope, f.owner, "measurement.bin", "binary")
	run, e := f.ps.CreateRun(ctx, f.owner, scope, parse.RunInput{Meta: f.meta(t), Kind: "device_test", DeviceID: device.ID, SampleID: sample, Transport: "tcp"})
	must(t, e)
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	pending := f.methodRun(t, scope, f.owner, sample, "1.1.0")
	_, e = f.env.Pool.Exec(ctx, `UPDATE parsing_run_executions SET state='running',fence=17,started_at=clock_timestamp(),lease_until=clock_timestamp()+interval '1 minute' WHERE run_id=$1`, pending.ID)
	must(t, e)
	manifest, e := f.ps.RecoveryManifest(ctx)
	must(t, e)
	before, e := f.ps.Device(ctx, f.owner, scope, device.ID)
	must(t, e)
	var schema string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
	dump, e := exec.Command("docker", "exec", "infrapro-account-tests-postgres-1", "pg_dump", "-U", "infrapro_test", "-d", "infrapro_test", "-Fc", "--no-owner", "--no-privileges", "--schema", schema).Output()
	must(t, e)
	directory := t.TempDir()
	must(t, os.WriteFile(filepath.Join(directory, "database.dump"), dump, 0600))
	body, e := manifest.Bytes()
	must(t, e)
	must(t, os.WriteFile(filepath.Join(directory, "manifest.json"), body, 0600))
	admin, e := pgxpool.New(ctx, os.Getenv("INFRAPRO_TEST_DATABASE_URL"))
	must(t, e)
	defer admin.Close()
	database := "parsing_restore_" + strings.ReplaceAll(uuidString(), "-", "") + "_test"
	_, e = admin.Exec(ctx, "CREATE DATABASE "+database)
	must(t, e)
	defer admin.Exec(ctx, "DROP DATABASE "+database+" WITH (FORCE)")
	if b, e := exec.Command("docker", "exec", "infrapro-account-tests-postgres-1", "psql", "-U", "infrapro_test", "-d", database, "-v", "ON_ERROR_STOP=1", "-c", "CREATE EXTENSION btree_gist WITH SCHEMA public").CombinedOutput(); e != nil {
		t.Fatalf("extension: %v %s", e, b)
	}
	command := exec.Command("docker", "exec", "-i", "infrapro-account-tests-postgres-1", "pg_restore", "-U", "infrapro_test", "-d", database, "--no-owner", "--no-privileges", "--exit-on-error")
	command.Stdin = bytes.NewReader(dump)
	if b, e := command.CombinedOutput(); e != nil {
		t.Fatalf("restore: %v %s", e, b)
	}
	destination, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	destination.Path = "/" + database
	pool, e := pgxpool.New(ctx, destination.String())
	must(t, e)
	defer pool.Close()
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	bucket := "parsing-restore-" + uuidString()
	must(t, client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}))
	defer func() {
		for o := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if o.Err == nil {
				_ = client.RemoveObject(ctx, bucket, o.Key, minio.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(ctx, bucket)
	}()
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", bucket, "parsing")
	must(t, e)
	recovered := runtime.Compose(access.New(pool, slog.Default())).Parsing.WithOptions(f.options(os.Getenv("INFRAPRO_PARSING_URL"))).WithStorage(store)
	copied := filepath.Join(directory, "release")
	must(t, exec.Command("cp", "-R", pkg, copied).Run())
	if _, e = wire.VerifyPackage(copied, manifest.Methods); e != nil {
		t.Fatal(e)
	}
	checked, e := recovered.VerifyRecovery(ctx, copied)
	must(t, e)
	if checked["missing_or_corrupt"] != len(manifest.Objects) {
		t.Fatal(checked)
	}
	for _, o := range manifest.Objects {
		reader, e := f.store.Get(ctx, o.Key)
		must(t, e)
		b, e := io.ReadAll(reader)
		must(t, e)
		must(t, reader.Close())
		if wire.Digest(b) != o.SHA256 || int64(len(b)) != o.Size {
			t.Fatal("source corruption")
		}
		must(t, store.Put(ctx, o.Key, bytes.NewReader(b), o.Size, "application/octet-stream"))
	}
	checked, e = recovered.VerifyRecovery(ctx, copied)
	must(t, e)
	if checked["missing_or_corrupt"] != 0 {
		t.Fatal(checked)
	}
	n, e := recovered.ReconcileRecovery(ctx)
	must(t, e)
	if n != 1 {
		t.Fatal(n)
	}
	again, e := recovered.ReconcileRecovery(ctx)
	must(t, e)
	if again != 0 {
		t.Fatal("nonidempotent recovery")
	}
	after, e := recovered.Device(ctx, f.owner, scope, device.ID)
	must(t, e)
	if after.Current.ID != before.Current.ID || after.Current.Snapshot.Method != before.Current.Snapshot.Method {
		t.Fatal("adoption altered by restore")
	}
	replay, e := recovered.CreateRun(ctx, f.owner, scope, parse.RunInput{Meta: f.meta(t), ReplayRunID: run.ID})
	must(t, e)
	_, e = recovered.RunOne(ctx)
	must(t, e)
	result, _, e := recovered.Result(ctx, f.owner, scope, replay.ID, "1", false, 0, 100)
	must(t, e)
	if result.Response == nil || *result.Response.Records[0].Measurements[0].Value != "12.34" {
		t.Fatal("old method replay failed")
	}
	must(t, os.Remove(filepath.Join(copied, "artifacts", manifest.Methods[0].ArtifactDigest)))
	if _, e = recovered.VerifyRecovery(ctx, copied); e == nil {
		t.Fatal("missing old package accepted")
	}
	t.Logf("restored separate PostgreSQL database, %d private S3 objects, %d immutable versions and original adoption; replay verified", len(manifest.Objects), len(manifest.Methods))
}
