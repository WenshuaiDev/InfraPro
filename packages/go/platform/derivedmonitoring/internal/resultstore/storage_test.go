//go:build monitoringintegration

package resultstore

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func require(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func TestDerivedResultPhysicalProtocol(t *testing.T) {
	if os.Getenv("INFRAPRO_MONITORING_CONTRACT") != "1" {
		t.Fatal("requires the explicit disposable monitoring contract laboratory")
	}
	ctx := t.Context()
	database := "derived_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	cfg := monitoring.StorageConfig{Replicas: []string{"127.0.0.1:59000", "127.0.0.1:59001"}, Database: database, User: "default", Policy: "contract_tiered", ObjectEndpoint: "http://127.0.0.1:58333", ObjectAccessKey: "contract-admin", ObjectSecretKey: "contract-admin-test-only", Bucket: "contract-source", TempDirectory: t.TempDir()}
	storage, e := monitoring.OpenStorage(ctx, cfg)
	require(t, e)
	t.Cleanup(storage.Close)
	transport := storage.CalculationTransport()
	group, e := transport.Group("local")
	require(t, e)
	for _, replica := range group.Replicas {
		require(t, replica.Exec(ctx, "CREATE DATABASE "+database))
		t.Cleanup(func() { require(t, replica.Exec(context.Background(), "DROP DATABASE "+database+" SYNC")) })
	}
	tenant, series := uuid.NewString(), uuid.NewString()
	objects, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4(cfg.ObjectAccessKey, cfg.ObjectSecretKey, "")})
	require(t, e)
	t.Cleanup(func() {
		for item := range objects.ListObjects(context.Background(), cfg.Bucket, minio.ListObjectsOptions{Prefix: tenant + "/derived/", Recursive: true}) {
			if item.Err != nil {
				t.Error(item.Err)
				continue
			}
			if e := objects.RemoveObject(context.Background(), cfg.Bucket, item.Key, minio.RemoveObjectOptions{}); e != nil {
				t.Error(e)
			}
		}
	})
	store := New(transport)
	if e = store.Initialize(ctx); e != nil {
		var actual string
		_ = group.Replicas[0].QueryRow(ctx, "SHOW CREATE TABLE "+database+".derived_results_local").Scan(&actual)
		t.Fatalf("%v: %s", e, actual)
	}
	require(t, store.Initialize(ctx))
	generation, run, config := uuid.NewString(), uuid.NewString(), uuid.NewString()
	times := []int64{monitoringvalue.MinTime.UnixNano(), -1, time.Date(2024, 2, 29, 12, 0, 0, 123456789, time.UTC).UnixNano(), math.MaxInt64}
	coefficients := []string{strings.Repeat("9", 76), "-" + strings.Repeat("9", 76), "250000000000", "1"}
	scales := []uint8{0, 60, 36, 60}
	fences := 0
	fence := func(ctx context.Context) error { fences++; return ctx.Err() }
	var saved Block
	for i, at := range times {
		result, e := ResultID(series, "output", at)
		require(t, e)
		row := Row{Tenant: tenant, Series: series, Channel: &series, Result: result, Generation: generation, Run: run, Config: config, Output: "output", At: at, Coefficient: &coefficients[i], Scale: scales[i], Quality: "good", Status: "valid", Count: 1, InputFirst: &at, InputLast: &at, Lineage: strings.Repeat("a", 64)}
		block, e := store.Write(ctx, "local", uuid.NewString(), uuid.NewString(), []Row{row}, fence)
		require(t, e)
		require(t, store.Verify(ctx, block))
		got := []Row{}
		require(t, store.ReadBlock(ctx, block, func(r Row) error { got = append(got, r); return nil }))
		if len(got) != 1 || got[0].Hash() != row.Hash() || *got[0].Decimal() != *row.Decimal() {
			t.Fatalf("exact payload lost: %#v", got)
		}
		if i == 2 {
			saved = block
		}
	}
	at := times[2] + 1
	result, e := ResultID(series, "output", at)
	require(t, e)
	missing := Row{Tenant: tenant, Series: series, Channel: &series, Result: result, Generation: generation, Run: run, Config: config, Output: "output", At: at, Scale: 18, Quality: "unmarked", Status: "missing_input", Reasons: []string{"no_predecessor"}, Lineage: strings.Repeat("b", 64)}
	missingBlock, e := store.Write(ctx, "local", uuid.NewString(), uuid.NewString(), []Row{missing}, fence)
	require(t, e)
	require(t, store.ReadBlock(ctx, missingBlock, func(r Row) error {
		if r.Coefficient != nil || r.Status != "missing_input" || r.Hash() != missing.Hash() {
			t.Fatal("missing result became a number")
		}
		return nil
	}))
	if fences < 20 {
		t.Fatal("external writes omitted lease fences", fences)
	}
	stopped := errors.New("lease_lost")
	_, e = store.Write(ctx, "local", uuid.NewString(), uuid.NewString(), []Row{missing}, func(context.Context) error { return stopped })
	if !errors.Is(e, stopped) {
		t.Fatal("stale lease performed physical write", e)
	}
	if saved.Endpoints == nil || saved.Summary == nil {
		t.Fatal("sealed block omitted query indexes")
	}
	tampered := saved
	endpoints := *saved.Endpoints
	endpoints.Last.Status = "missing_input"
	tampered.Endpoints = &endpoints
	if store.Verify(ctx, tampered) == nil {
		t.Fatal("tampered endpoint index accepted")
	}
	tampered = saved
	summary := monitoring.CalculationSummary{}
	for q, v := range saved.Summary {
		v.Sum = "0"
		summary[q] = v
	}
	tampered.Summary = summary
	if store.Verify(ctx, tampered) == nil {
		t.Fatal("tampered exact summary accepted")
	}
	legacy := saved
	legacy.Summary = nil
	legacy.Endpoints = nil
	legacy.First = nil
	legacy.Last = nil
	legacy.Output = ""
	require(t, store.ReadBlock(ctx, legacy, func(Row) error { return nil }))
	before := saved
	saved.Hash = strings.Repeat("0", 64)
	if store.ReadBlock(ctx, saved, func(Row) error { t.Fatal("unverified payload escaped"); return nil }) == nil {
		t.Fatal("hash mismatch accepted")
	}
	saved = before
	require(t, objects.RemoveObject(ctx, cfg.Bucket, saved.ObjectKey, minio.RemoveObjectOptions{}))
	if store.Verify(ctx, saved) == nil {
		t.Fatal("missing object accepted as sealed")
	}
	manifest, e := store.SealDocument(ctx, tenant, "manifests", map[string]any{"schema": "range-test/1", "rows": "0"})
	require(t, e)
	firstGeneration, secondGeneration, thirdGeneration := uuid.NewString(), uuid.NewString(), uuid.NewString()
	root, e := store.ReplaceRange(ctx, tenant, "", RangeEntry{From: 0, To: planning.End(100), Generation: firstGeneration, Manifest: manifest, SourcePosition: "1"})
	require(t, e)
	second, e := store.ReplaceRange(ctx, tenant, root, RangeEntry{From: 40, To: planning.End(60), Generation: secondGeneration, Manifest: manifest, SourcePosition: "2"})
	require(t, e)
	third, e := store.ReplaceRange(ctx, tenant, second, RangeEntry{From: 50, To: planning.End(150), Generation: thirdGeneration, Manifest: manifest, SourcePosition: "3"})
	require(t, e)
	ranges, e := store.ReadRanges(ctx, tenant, third, planning.Range{From: 0, To: planning.End(200)}, 10)
	require(t, e)
	if len(ranges) != 3 || ranges[0].From != 0 || *ranges[0].To != 40 || ranges[1].From != 40 || *ranges[1].To != 50 || ranges[1].Generation != secondGeneration || ranges[2].Generation != thirdGeneration {
		t.Fatal("range replacement resurrected overwritten coverage", ranges)
	}
	original, e := store.ReadRanges(ctx, tenant, root, planning.Range{From: 0, To: planning.End(200)}, 10)
	require(t, e)
	if len(original) != 1 || original[0].Generation != firstGeneration || *original[0].To != 100 {
		t.Fatal("immutable historical root changed", original)
	}
	clipped, e := store.ReadRanges(ctx, tenant, third, planning.Range{From: 45, To: planning.End(55)}, 10)
	require(t, e)
	if len(clipped) != 2 {
		t.Fatal("range query lost a partial interval", clipped)
	}
	if _, e = store.ReadRanges(ctx, tenant, third, planning.Range{From: 0}, 1); e == nil {
		t.Fatal("bounded range query returned a successful prefix")
	}
	verifier := store.RangeVerifier(tenant, func(entry RangeEntry) error { return nil })
	for hash, count := range map[string]uint64{root: 1, second: 3, third: 3} {
		n, e := verifier.Verify(ctx, hash)
		require(t, e)
		if n != count {
			t.Fatal("historical range cardinality differs", n, count)
		}
	}
	// This obsolete root is absent from the latest tree. Checking only the
	// latest root would incorrectly approve a backup missing old evidence.
	key, e := documentKey(tenant, "ranges", root)
	require(t, e)
	require(t, objects.RemoveObject(ctx, cfg.Bucket, key, minio.RemoveObjectOptions{}))
	verifier = store.RangeVerifier(tenant, func(entry RangeEntry) error { return nil })
	_, e = verifier.Verify(ctx, third)
	require(t, e)
	if _, e = verifier.Verify(ctx, root); e == nil {
		t.Fatal("missing obsolete historical root was ignored")
	}
	// A constraint change must fail startup even if names and layout receipt stay intact.
	require(t, group.Replicas[0].Exec(ctx, "ALTER TABLE "+database+".derived_results_local DROP CONSTRAINT valid_value"))
	require(t, group.Replicas[0].Exec(ctx, "ALTER TABLE "+database+".derived_results_local ADD CONSTRAINT valid_value CHECK 1"))
	if store.Check(ctx) == nil {
		t.Fatal("weakened nullable coefficient constraint accepted")
	}
	t.Log("verified exact Int256, all supported ns boundaries, both replicas, immutable S3 payloads, non-value statuses, lease fence and tamper detection")
}
