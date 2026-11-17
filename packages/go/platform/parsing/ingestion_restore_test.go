package parsing_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	wire "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Restores actual PostgreSQL dumps and separately copied private objects. No test
// replaces persistence, TLS, the packaged parser, or cloud metadata with a mock.
func TestIngestionJointPostgresS3PackageAndEpochRestore(t *testing.T) {
	f := setupIngestion(t)
	pkg := os.Getenv("INFRAPRO_PARSER_TEST_PACKAGE")
	if pkg == "" {
		t.Fatal("packaged parser fixture required")
	}
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	f.work(t, 12)
	var unit string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM ingestion_units WHERE device_id=$1 LIMIT 1`, f.device).Scan(&unit))
	before, e := f.s.Content(ctx, f.owner, c.TenantScope(f.tenant), unit)
	must(t, e)
	// Persist a second receipt and fixed plans, then stop before their execution.
	if f.send(t, batch, uuid.NewString()) != 202 {
		t.Fatal("send pending")
	}
	_, e = f.s.Pull(ctx, f.opts.Relays[0])
	must(t, e)
	for n := 0; n < 4; n++ {
		_, e = f.s.IdentifyOne(ctx)
		must(t, e)
		_, e = f.s.PlanOne(ctx)
		must(t, e)
	}
	_, e = f.s.AckOne(ctx)
	must(t, e)
	manifest, e := f.s.OperatorManifest(ctx)
	must(t, e)
	if len(manifest.Receipts) != 2 {
		t.Fatal("receipt snapshot")
	}
	directory := t.TempDir()
	body, e := manifest.Bytes()
	must(t, e)
	must(t, os.WriteFile(filepath.Join(directory, "inner-manifest.json"), body, 0600))
	var schema string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
	restoreDB := func(container, role, originalDSN, schema, prefix string, extensions bool) *pgxpool.Pool {
		t.Helper()
		u, e := url.Parse(originalDSN)
		must(t, e)
		original := strings.TrimPrefix(u.Path, "/")
		dump, e := exec.Command("docker", "exec", container, "pg_dump", "-U", role, "-d", original, "-Fc", "--no-owner", "--no-privileges", "--schema", schema).Output()
		must(t, e)
		must(t, os.WriteFile(filepath.Join(directory, prefix+".dump"), dump, 0600))
		admin, e := pgxpool.New(ctx, originalDSN)
		must(t, e)
		t.Cleanup(admin.Close)
		db := prefix + strings.ReplaceAll(uuid.NewString(), "-", "") + "_test"
		_, e = admin.Exec(ctx, "CREATE DATABASE "+db)
		must(t, e)
		t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE "+db+" WITH (FORCE)") })
		if extensions {
			if b, e := exec.Command("docker", "exec", container, "psql", "-U", role, "-d", db, "-v", "ON_ERROR_STOP=1", "-c", "CREATE EXTENSION btree_gist WITH SCHEMA public").CombinedOutput(); e != nil {
				t.Fatalf("extension: %v %s", e, b)
			}
		}
		command := exec.Command("docker", "exec", "-i", container, "pg_restore", "-U", role, "-d", db, "--no-owner", "--no-privileges", "--exit-on-error")
		command.Stdin = bytes.NewReader(dump)
		if b, e := command.CombinedOutput(); e != nil {
			t.Fatalf("restore: %v %s", e, b)
		}
		u.Path = "/" + db
		pool, e := pgxpool.New(ctx, u.String())
		must(t, e)
		t.Cleanup(pool.Close)
		return pool
	}
	businessContainer := os.Getenv("INFRAPRO_INGESTION_TEST_BUSINESS_CONTAINER")
	if businessContainer == "" {
		businessContainer = "infrapro-account-tests-postgres-1"
	}
	businessURL, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	pool := restoreDB(businessContainer, businessURL.User.Username(), f.env.Config.DatabaseURL, schema, "ingest_restore_", true)
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	bucket := "ingestion-restore-" + uuid.NewString()
	must(t, client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}))
	t.Cleanup(func() {
		for o := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if o.Err == nil {
				_ = client.RemoveObject(ctx, bucket, o.Key, minio.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(ctx, bucket)
	})
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", bucket, "ingestion")
	must(t, e)
	source, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", f.bucket, "ingestion")
	must(t, e)
	runtime := runtime.Compose(access.New(pool, slog.Default()))
	runtime.Parsing.WithOptions(f.options(os.Getenv("INFRAPRO_PARSING_URL"))).WithStorage(store)
	recovered := runtime.Ingestion.WithOptions(f.opts).WithStorage(store)
	copied := filepath.Join(directory, "release")
	must(t, exec.Command("cp", "-R", pkg, copied).Run())
	coverage, e := recovered.VerifyRelease(ctx, copied)
	must(t, e)
	if int64(coverage["schema"]) != migrations.Version || coverage["identification_versions"] < 4 {
		t.Fatal(coverage)
	}
	cli := newIngestionOperatorCLI(t, pool.Config().ConnString(), bucket)
	verified, e := recovered.VerifyObjects(ctx)
	must(t, e)
	if verified["missing_or_corrupt"] != len(manifest.Objects) {
		t.Fatal("missing objects not reported", verified)
	}
	missingReport := filepath.Join(directory, "missing-objects.json")
	cli.run(t, "objects", missingReport, false)
	var missingObjects map[string]int
	operatorReport(t, missingReport, &missingObjects)
	if missingObjects["missing_or_corrupt"] != len(manifest.Objects) {
		t.Fatal("CLI did not report missing restored objects", missingObjects)
	}
	for _, o := range manifest.Objects {
		reader, e := source.Get(ctx, o.Key)
		must(t, e)
		raw, e := io.ReadAll(reader)
		must(t, e)
		must(t, reader.Close())
		if wire.Digest(raw) != o.SHA256 || int64(len(raw)) != o.Size {
			t.Fatal("source integrity")
		}
		must(t, store.Put(ctx, o.Key, bytes.NewReader(raw), o.Size, "application/octet-stream"))
	}
	verified, e = recovered.VerifyObjects(ctx)
	must(t, e)
	if verified["missing_or_corrupt"] != 0 {
		t.Fatal(verified)
	}
	cli.verify(t, copied, len(manifest.Objects), len(manifest.Receipts))
	// Preserve the length while corrupting real S3 bytes, so the CLI must
	// validate the digest rather than just object existence or size.
	corrupt := manifest.Objects[0]
	reader, e := store.Get(ctx, corrupt.Key)
	must(t, e)
	original, e := io.ReadAll(reader)
	must(t, e)
	must(t, reader.Close())
	changed := append([]byte(nil), original...)
	changed[0] ^= 1
	_, e = client.PutObject(ctx, bucket, corrupt.Key, bytes.NewReader(changed), corrupt.Size, minio.PutObjectOptions{})
	must(t, e)
	corruptReport := filepath.Join(directory, "corrupt-objects.json")
	cli.run(t, "objects", corruptReport, false)
	var corruptResult map[string]int
	operatorReport(t, corruptReport, &corruptResult)
	if corruptResult["missing_or_corrupt"] != 1 {
		t.Fatal("CLI did not reject corrupt restored object", corruptResult)
	}
	_, e = client.PutObject(ctx, bucket, corrupt.Key, bytes.NewReader(original), corrupt.Size, minio.PutObjectOptions{})
	must(t, e)
	cli.run(t, "objects", filepath.Join(directory, "repaired-objects.json"), true)
	after, e := recovered.Content(ctx, f.owner, c.TenantScope(f.tenant), unit)
	must(t, e)
	if !bytes.Equal(before, after) {
		t.Fatal("restored body changed")
	}
	for n := 0; n < 4; n++ {
		_, e = recovered.ParseOne(ctx)
		must(t, e)
	}
	var results int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM ingestion_results`).Scan(&results))
	if results != 4 {
		t.Fatal("frozen plans not resumed", results)
	}
	// Cloud database is independently restored and must rotate only NEW receipts.
	cfgPath := os.Getenv("INFRAPRO_INGESTION_TEST_RELAY_CONFIG")
	raw, e := os.ReadFile(cfgPath)
	must(t, e)
	var cfg map[string]any
	must(t, json.Unmarshal(raw, &cfg))
	u, e := url.Parse(cfg["database_url"].(string))
	must(t, e)
	cloud := restoreDB("infrapro-ingestion-lab-postgres-1", "relay_test", u.String(), u.Query().Get("search_path"), "relay_restore_", false)
	binary := filepath.Join(filepath.Dir(cfgPath), "ingest-relay")
	cloudCfg := filepath.Join(directory, "restored-relay.json")
	cfg["database_url"] = cloud.Config().ConnString()
	raw, e = json.Marshal(cfg)
	must(t, e)
	must(t, os.WriteFile(cloudCfg, raw, 0600))
	var oldEpoch, newEpoch string
	must(t, cloud.QueryRow(ctx, `SELECT spool_epoch::text FROM relay_state WHERE id=1`).Scan(&oldEpoch))
	must(t, exec.Command(binary, "new-epoch", cloudCfg).Run())
	must(t, cloud.QueryRow(ctx, `SELECT spool_epoch::text FROM relay_state WHERE id=1`).Scan(&newEpoch))
	if oldEpoch == newEpoch {
		t.Fatal("restore reused epoch")
	}
	var retained int
	must(t, cloud.QueryRow(ctx, `SELECT count(*) FROM relay_receipts WHERE envelope->>'spool_epoch'=$1 AND envelope->>'ingress_id'=$2`, oldEpoch, f.httpSource).Scan(&retained))
	if retained != 2 {
		t.Fatal("old identities changed", retained)
	}
	exported, e := exec.Command(binary, "manifest", cloudCfg).Output()
	must(t, e)
	must(t, os.WriteFile(filepath.Join(directory, "cloud-manifest.json"), exported, 0600))
	var all []json.RawMessage
	must(t, json.Unmarshal(exported, &all))
	own := []json.RawMessage{}
	for _, v := range all {
		var m map[string]any
		must(t, json.Unmarshal(v, &m))
		if m["ingress_id"] == f.httpSource {
			own = append(own, v)
		}
	}
	report, e := recovered.ReconcileManifest(ctx, own)
	must(t, e)
	if report["matched"] != 2 || report["gaps"] != 0 {
		t.Fatal(report)
	}
	var sample map[string]any
	must(t, json.Unmarshal(own[0], &sample))
	sample["receipt_id"] = uuid.NewString()
	sample["payload_present"] = false
	sample["acknowledged"] = true
	missing, _ := json.Marshal(sample)
	report, e = recovered.ReconcileManifest(ctx, append(own, missing))
	must(t, e)
	if report["gaps"] != 1 {
		t.Fatal("missing receipt not reported", report)
	}
	sample["payload_present"] = true
	sample["acknowledged"] = false
	pending, _ := json.Marshal(sample)
	report, e = recovered.ReconcileManifest(ctx, []json.RawMessage{pending})
	must(t, e)
	if report["awaiting_transfer"] != 1 {
		t.Fatal(report)
	}
	must(t, json.Unmarshal(own[0], &sample))
	sample["envelope_digest"] = strings.Repeat("0", 64)
	conflict, _ := json.Marshal(sample)
	report, e = recovered.ReconcileManifest(ctx, []json.RawMessage{conflict})
	must(t, e)
	if report["conflicts"] != 1 {
		t.Fatal(report)
	}
	// Rollback package that lacks a referenced identification executable is refused.
	artifact := filepath.Join(copied, "artifacts", manifest.Capabilities.IdentificationMethods[0].Method.ArtifactDigest)
	must(t, os.Remove(artifact))
	if _, e = recovered.VerifyRelease(ctx, copied); e == nil {
		t.Fatal("incomplete rollback package accepted")
	}
	cli.run(t, "release", copied, false)
	evidence := map[string]any{"passed": true, "objects": len(manifest.Objects), "receipts": len(manifest.Receipts), "results_after_resume": results, "coverage": coverage, "new_epoch": oldEpoch != newEpoch, "missing_receipt_reported": true, "independent_databases": true, "operator_cli_verified": true, "corrupt_object_rejected": true, "incomplete_package_rejected": true}
	if out := os.Getenv("INFRAPRO_INGESTION_EVIDENCE"); out != "" {
		b, _ := json.MarshalIndent(evidence, "", "  ")
		must(t, os.WriteFile(filepath.Join(out, "joint-restore.json"), b, 0600))
	}
	t.Log(fmt.Sprintf("joint restore: %d objects, 2 receipts, 4 results; frozen plans resumed, epoch rotated, missing receipt and incomplete release refused", len(manifest.Objects)))
}
