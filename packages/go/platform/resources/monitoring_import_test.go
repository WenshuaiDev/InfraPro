//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/ext"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type monitoringFixture struct {
	resourceFixture
	service  *md.Service
	scope    md.Scope
	channel  string
	storage  *md.Storage
	database string
}

func monitoringSetup(t *testing.T, options ...func(*md.StorageConfig)) monitoringFixture {
	t.Helper()
	if os.Getenv("INFRAPRO_MONITORING_CONTRACT") != "1" {
		t.Fatal("requires the disposable monitoring laboratory")
	}
	f := resourcesSetup(t)
	database := "monitoring_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	bucket := "contract-source"
	cfg := md.StorageConfig{Replicas: []string{"127.0.0.1:59000", "127.0.0.1:59001"}, Database: database, User: "default", Policy: "monitoring_tiered", ObjectEndpoint: "http://127.0.0.1:58333", ObjectAccessKey: "contract-admin", ObjectSecretKey: "contract-admin-test-only", Bucket: bucket, TempDirectory: t.TempDir()}
	for _, option := range options {
		option(&cfg)
	}
	storage, e := md.OpenStorage(ctx, cfg)
	must(t, e)
	must(t, storage.Initialize(ctx))
	must(t, storage.Check(ctx))
	t.Cleanup(func() {
		storage.Close()
		for _, port := range []string{"59000", "59001"} {
			ch, e := clickhouse.Open(&clickhouse.Options{Addr: []string{"127.0.0.1:" + port}})
			if e != nil {
				t.Error(e)
				continue
			}
			if e = ch.Exec(context.Background(), "DROP DATABASE "+database+" SYNC"); e != nil {
				t.Error(e)
			}
			ch.Close()
		}
		s3, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
		if e != nil {
			t.Error(e)
			return
		}
		for object := range s3.ListObjects(context.Background(), bucket, minio.ListObjectsOptions{Recursive: true, Prefix: f.tenant + "/"}) {
			if object.Err != nil {
				t.Error(object.Err)
				break
			}
			if e = s3.RemoveObject(context.Background(), bucket, object.Key, minio.RemoveObjectOptions{}); e != nil {
				t.Error(e)
			}
		}
	})
	facility := f.facility(t, "MONITORING-IMPORT")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "POINT", "name": "合成测点"})
	must(t, e)
	ct := target(facility, res.Channel, "")
	ct.Point = point.ID
	channel, e := f.write(t, ct, "create", map[string]any{"code": "CHANNEL", "name": "合成监测通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	return monitoringFixture{f, md.New(f.env.Service.Access()).WithStorage(storage), md.TenantScope(f.tenant), channel.ID, storage, database}
}
func (f monitoringFixture) meta(revision string) md.WriteMeta {
	return md.WriteMeta{RequestID: uuid.NewString(), ExpectedRevision: revision, Reason: "本机合成文件验收"}
}
func (f monitoringFixture) upload(t *testing.T, content string) md.ImportBatch {
	t.Helper()
	return f.uploadBytes(t, "synthetic.csv", []byte(content), md.Mapping{Format: "csv", Mode: "single", Encoding: "utf-8", Delimiter: ",", HeaderRow: 1, TimeColumn: "observed_at", ValueColumn: "value", QualityColumn: "quality", Timezone: "UTC", EmptyWide: "error", Channels: []md.ChannelMapping{{Alias: "C", Channel: f.channel, Unit: "mm"}}})
}
func (f monitoringFixture) uploadBytes(t *testing.T, name string, content []byte, mapping md.Mapping) md.ImportBatch {
	t.Helper()
	in := md.ImportInput{WriteMeta: f.meta("0"), Name: name, Size: fmt.Sprint(len(content))}
	batch, e := f.service.CreateImport(ctx, f.actor, f.scope, in)
	must(t, e)
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	_, e = f.service.UploadPart(ctx, f.actor, f.scope, batch.ID, batch.Revision, 1, hash, bytes.NewReader(content))
	must(t, e)
	// Resending the same part keeps the file identity and has no revision race.
	_, e = f.service.UploadPart(ctx, f.actor, f.scope, batch.ID, batch.Revision, 1, hash, bytes.NewReader(content))
	must(t, e)
	batch, e = f.service.CompleteUpload(ctx, f.actor, f.scope, batch.ID, md.UploadComplete{WriteMeta: f.meta(batch.Revision), SHA256: hash})
	must(t, e)
	batch, e = f.service.SetMapping(ctx, f.actor, f.scope, batch.ID, md.MappingInput{WriteMeta: f.meta(batch.Revision), Mapping: mapping})
	must(t, e)
	return batch
}
func (f monitoringFixture) validate(t *testing.T, b md.ImportBatch) md.ImportBatch {
	t.Helper()
	queued, e := f.service.QueueImport(ctx, f.actor, f.scope, b.ID, "validate", f.meta(b.Revision))
	must(t, e)
	worked, e := f.service.RunOne(ctx, "integration-worker")
	must(t, e)
	if !worked {
		t.Fatal("worker did not claim validation")
	}
	job, e := f.service.Job(ctx, f.actor, f.scope, queued.JobID)
	must(t, e)
	if job.State != "succeeded" {
		t.Fatal(job)
	}
	b, e = f.service.Import(ctx, f.actor, f.scope, b.ID)
	must(t, e)
	return b
}
func (f monitoringFixture) publish(t *testing.T, b md.ImportBatch) md.ImportBatch {
	t.Helper()
	_, e := f.service.QueueImport(ctx, f.actor, f.scope, b.ID, "publish", f.meta(b.Revision))
	must(t, e)
	worked, e := f.service.RunOne(ctx, "integration-worker")
	must(t, e)
	if !worked {
		t.Fatal("worker did not claim publication")
	}
	b, e = f.service.Import(ctx, f.actor, f.scope, b.ID)
	must(t, e)
	if b.State != "published" {
		t.Fatal(b)
	}
	return b
}
func (f monitoringFixture) query(t *testing.T) md.QueryInput {
	t.Helper()
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{f.channel}})
	must(t, e)
	return md.QueryInput{Selection: selection.ID, AllHistory: true, Timezone: "UTC", Qualities: []string{"good", "unmarked", "suspect", "bad"}, Limit: 2}
}

func TestMonitoringCSVPublicationAndFixedDuplicateSet(t *testing.T) {
	f := monitoringSetup(t)
	a := f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00.000000001Z,10000000000000000000000000.000000000001,good\n2024-01-01T00:00:00.000000003Z,-99999999999999999999999999.999999999999,bad\n2262-04-11T23:47:16.854775807Z,3,unmarked\n"))
	if a.State != "ready" || a.Counts.Proposed != "3" {
		t.Fatal(a)
	}
	before, e := f.service.Details(ctx, f.actor, f.scope, f.query(t))
	must(t, e)
	if len(before.Items) != 0 {
		t.Fatal("unpublished candidates visible")
	}
	a = f.publish(t, a)
	query := f.query(t)
	page, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 2 || !page.HasMore || page.Items[0].At != "2262-04-11T23:47:16.854775807Z" || page.Items[1].Value != "-99999999999999999999999999.999999999999" {
		f.diagnoseRead(t)
		t.Fatal(page)
	}
	query.Snapshot = page.Snapshot.ID
	query.Cursor = page.NextCursor
	next, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(next.Items) != 1 || next.Items[0].Value != "10000000000000000000000000.000000000001" || next.HasMore {
		t.Fatal(next)
	}
	b := f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00.000000001Z,10000000000000000000000000.000000000001,bad\n2024-01-01T00:00:00.000000003Z,-99999999999999999999999999.999999999999,good\n2025-01-01T00:00:00Z,4,good\n"))
	if b.State != "needs_resolution" || b.Counts.Duplicates != "2" || b.Counts.Errors != "0" || b.Counts.Proposed != "1" {
		t.Fatal(b)
	}
	issues, e := f.service.Issues(ctx, f.actor, f.scope, b.ID, "")
	must(t, e)
	if len(issues.Items) != 2 || issues.Items[0].Target == "" {
		t.Fatal(issues)
	}
	b, e = f.service.ResolveImport(ctx, f.actor, f.scope, b.ID, md.ResolveInput{WriteMeta: f.meta(b.Revision), PreviewHash: issues.PreviewHash, ConfirmSameObservation: true})
	must(t, e)
	if b.Counts.Ignored != "2" {
		t.Fatal(b)
	}
	b = f.publish(t, b)
	// The fixed old snapshot still sees its earlier append watermark.
	same, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(same.Items) != 1 || same.Items[0].ID != next.Items[0].ID {
		t.Fatal("append changed old snapshot", same)
	}
	fresh := f.query(t)
	fresh.Limit = 100
	all, e := f.service.Details(ctx, f.actor, f.scope, fresh)
	must(t, e)
	if len(all.Items) != 4 {
		t.Fatal(all)
	}
	if all.Items[2].Quality != "bad" {
		t.Fatal("duplicate changed original quality", all.Items)
	}
	conflict := f.validate(t, f.upload(t, "observed_at,value,quality\n2025-01-01T00:00:00Z,999,good\n"))
	if conflict.State != "needs_resolution" || conflict.Counts.Errors != "1" {
		t.Fatal(conflict)
	}
	_, e = f.service.ResolveImport(ctx, f.actor, f.scope, conflict.ID, md.ResolveInput{WriteMeta: f.meta(conflict.Revision), PreviewHash: conflict.ValidationHash, ConfirmSameObservation: true})
	if e == nil {
		t.Fatal("different value ignored")
	}
	var pgRows int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_publications`).Scan(&pgRows))
	if pgRows != 2 {
		t.Fatal(pgRows)
	}
}

func TestMonitoringPublicationCommitFailureAndRecovery(t *testing.T) {
	f := monitoringSetup(t)
	batch := f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00.000000001Z,1,good\n2025-01-01T00:00:00.000000001Z,2,good\n"))
	queued, e := f.service.QueueImport(ctx, f.actor, f.scope, batch.ID, "publish", f.meta(batch.Revision))
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION test_monitoring_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic commit failure'; END $$; CREATE TRIGGER test_monitoring_commit_failure BEFORE INSERT ON monitoring_publications FOR EACH ROW EXECUTE FUNCTION test_monitoring_commit_failure()`)
	must(t, e)
	worked, e := f.service.RunOne(ctx, "before-crash")
	if !worked || e == nil {
		t.Fatal("expected failed publication", worked, e)
	}
	var attempts, published int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_write_attempts WHERE state='sealed'`).Scan(&attempts))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_publications`).Scan(&published))
	if attempts != 2 || published != 0 {
		t.Fatal("unexpected prepared visibility", attempts, published)
	}
	hidden, e := f.service.Details(ctx, f.actor, f.scope, f.query(t))
	must(t, e)
	if len(hidden.Items) != 0 {
		t.Fatal("partially published batch", hidden)
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER test_monitoring_commit_failure ON monitoring_publications; DROP FUNCTION test_monitoring_commit_failure()`)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE monitoring_jobs SET available_at=clock_timestamp()-interval '1 second' WHERE id=$1`, queued.JobID)
	must(t, e)
	worked, e = f.service.RunOne(ctx, "replacement-worker")
	must(t, e)
	if !worked {
		t.Fatal("retry was lost")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_write_attempts WHERE state='sealed'`).Scan(&attempts))
	if attempts != 2 {
		t.Fatal("sealed attempt replayed", attempts)
	}
	visible, e := f.service.Details(ctx, f.actor, f.scope, f.query(t))
	must(t, e)
	if len(visible.Items) != 2 {
		t.Fatal(visible)
	}
	job, e := f.service.Job(ctx, f.actor, f.scope, queued.JobID)
	must(t, e)
	if job.State != "succeeded" || job.Attempts != 2 {
		t.Fatal(job)
	}
}

func TestMonitoringExpiredWorkerAndWholeBatchInvalidInput(t *testing.T) {
	f := monitoringSetup(t)
	batch := f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00.000000001Z,1,good\n2024-01-01T00:00:00.000000002Z,,good\n2024-01-01T00:00:00.000000003Z,2,good\n")
	queued, e := f.service.QueueImport(ctx, f.actor, f.scope, batch.ID, "validate", f.meta(batch.Revision))
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE monitoring_jobs SET state='running',fence=1,lease_owner='dead-worker',lease_until=clock_timestamp()-interval '1 second',attempts=1 WHERE id=$1`, queued.JobID)
	must(t, e)
	worked, e := f.service.RunOne(ctx, "recovery-worker")
	must(t, e)
	if !worked {
		t.Fatal("expired lease not recovered")
	}
	var fence int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT fence FROM monitoring_jobs WHERE id=$1`, queued.JobID).Scan(&fence))
	if fence != 2 {
		t.Fatal("fence not advanced", fence)
	}
	batch, e = f.service.Import(ctx, f.actor, f.scope, batch.ID)
	must(t, e)
	if batch.State != "needs_resolution" || batch.Counts.Errors != "1" || batch.Counts.Proposed != "2" {
		t.Fatal(batch)
	}
	_, e = f.service.QueueImport(ctx, f.actor, f.scope, batch.ID, "publish", f.meta(batch.Revision))
	if e == nil {
		t.Fatal("invalid file partially published")
	}
	page, e := f.service.Details(ctx, f.actor, f.scope, f.query(t))
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal(page)
	}
}

func TestMonitoringUnsealedPhysicalWriteUsesNewAttempt(t *testing.T) {
	f := monitoringSetup(t)
	batch := f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00.000000001Z,1,good\n"))
	queued, e := f.service.QueueImport(ctx, f.actor, f.scope, batch.ID, "publish", f.meta(batch.Revision))
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION test_monitoring_seal_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='sealed' THEN RAISE EXCEPTION 'synthetic crash after physical write'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_monitoring_seal_failure BEFORE UPDATE ON monitoring_write_attempts FOR EACH ROW EXECUTE FUNCTION test_monitoring_seal_failure()`)
	must(t, e)
	worked, e := f.service.RunOne(ctx, "unsealed-worker")
	if !worked || e == nil {
		t.Fatal(worked, e)
	}
	page, e := f.service.Details(ctx, f.actor, f.scope, f.query(t))
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal("unsealed write was visible")
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER test_monitoring_seal_failure ON monitoring_write_attempts; DROP FUNCTION test_monitoring_seal_failure()`)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `UPDATE monitoring_jobs SET available_at=clock_timestamp()-interval '1 second' WHERE id=$1`, queued.JobID)
	must(t, e)
	worked, e = f.service.RunOne(ctx, "new-attempt-worker")
	must(t, e)
	if !worked {
		t.Fatal("no retry")
	}
	var attempts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_write_attempts`).Scan(&attempts))
	if attempts != 2 {
		t.Fatal("uncertain identity reused", attempts)
	}
	ch, e := clickhouse.Open(&clickhouse.Options{Addr: []string{"127.0.0.1:59000"}})
	must(t, e)
	defer ch.Close()
	var physical uint64
	must(t, ch.QueryRow(ctx, "SELECT count() FROM "+f.database+".observations").Scan(&physical))
	if physical != 2 {
		t.Fatal("fault did not leave a physical write", physical)
	}
	page, e = f.service.Details(ctx, f.actor, f.scope, f.query(t))
	must(t, e)
	if len(page.Items) != 1 {
		t.Fatal("manifest did not isolate the selected attempt", page)
	}
}

func TestMonitoringActualAdoptionsRevokeAndReadopt(t *testing.T) {
	f := monitoringSetup(t)
	file := func(first, count int) string {
		var out strings.Builder
		out.WriteString("observed_at,value,quality\n")
		base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		for n := first; n < first+count; n++ {
			fmt.Fprintf(&out, "%s,%d,good\n", base.Add(time.Duration(n)*time.Nanosecond).Format(time.RFC3339Nano), n)
		}
		return out.String()
	}
	a := f.publish(t, f.validate(t, f.upload(t, file(0, 100))))
	b := f.validate(t, f.upload(t, file(20, 100)))
	if b.Counts.Duplicates != "80" || b.Counts.Proposed != "20" {
		t.Fatal(b)
	}
	var e error
	b, e = f.service.ResolveImport(ctx, f.actor, f.scope, b.ID, md.ResolveInput{WriteMeta: f.meta(b.Revision), PreviewHash: b.ValidationHash, ConfirmSameObservation: true})
	must(t, e)
	b = f.publish(t, b)
	query := f.query(t)
	query.Limit = 1000
	before, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(before.Items) != 120 {
		t.Fatal(len(before.Items))
	}
	query.Snapshot = before.Snapshot.ID
	revoke := func(batch md.ImportBatch) md.ImportBatch {
		t.Helper()
		preview, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
		must(t, e)
		input := md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: preview.Hash}
		result, e := f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, input)
		must(t, e)
		again, e := f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, input)
		must(t, e)
		if !again.Replayed || again.Sequence != result.Sequence {
			t.Fatal(again, result)
		}
		batch, e = f.service.Import(ctx, f.actor, f.scope, batch.ID)
		must(t, e)
		return batch
	}
	a = revoke(a)
	_, e = f.service.Details(ctx, f.actor, f.scope, query)
	monitoringCode(t, e, "snapshot_stale")
	duplicateHistory := f.validate(t, f.upload(t, file(0, 100)))
	if duplicateHistory.State != "needs_resolution" || duplicateHistory.Counts.Duplicates != "100" || duplicateHistory.Counts.Proposed != "0" {
		t.Fatal("retained history was treated as a new identity", duplicateHistory)
	}
	fresh := f.query(t)
	fresh.Limit = 1000
	active, e := f.service.Details(ctx, f.actor, f.scope, fresh)
	must(t, e)
	if len(active.Items) != 20 {
		t.Fatal("ignored B sources revived", len(active.Items))
	}
	again, e := f.service.Readopt(ctx, f.actor, f.scope, a.ID, md.ReadoptInput{WriteMeta: f.meta(a.Revision), ReadoptSelection: md.ReadoptSelection{All: true}})
	must(t, e)
	again = f.publish(t, f.validate(t, again))
	if again.Counts.Proposed != "100" {
		t.Fatal(again)
	}
	fresh = f.query(t)
	fresh.Limit = 1000
	fresh.Status = "history"
	history, e := f.service.Details(ctx, f.actor, f.scope, fresh)
	must(t, e)
	identities := map[string]bool{}
	var current int
	for _, row := range history.Items {
		identities[row.ID] = true
		if !row.Revoked {
			current++
		}
	}
	if len(history.Items) != 220 || len(identities) != 120 || current != 120 {
		t.Fatal("observation identity changed on readoption", len(history.Items), len(identities), current)
	}
	var ever int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT sum(first_adopted_count) FROM monitoring_publication_chunks`).Scan(&ever))
	if ever != 120 {
		t.Fatal("ever count inflated", ever)
	}
	b = revoke(b)
	fresh = f.query(t)
	fresh.Limit = 1000
	active, e = f.service.Details(ctx, f.actor, f.scope, fresh)
	must(t, e)
	if len(active.Items) != 100 {
		t.Fatal("B revoke affected the wrong observations", len(active.Items))
	}
	readoptB, e := f.service.Readopt(ctx, f.actor, f.scope, b.ID, md.ReadoptInput{WriteMeta: f.meta(b.Revision), ReadoptSelection: md.ReadoptSelection{All: true}})
	must(t, e)
	readoptB = f.validate(t, readoptB)
	if readoptB.Counts.Proposed != "20" || readoptB.Counts.Duplicates != "0" {
		t.Fatal("readoption included formerly ignored source rows", readoptB)
	}
}

func (f monitoringFixture) diagnoseRead(t *testing.T) {
	ch, e := clickhouse.Open(&clickhouse.Options{Addr: []string{"127.0.0.1:59000"}})
	must(t, e)
	defer ch.Close()
	ranges, e := ext.NewTable("authorized_windows", ext.Column("channel_id", "UUID"), ext.Column("lo", "Nullable(Int64)"), ext.Column("hi", "Nullable(Int64)"))
	must(t, e)
	var end *int64
	must(t, ranges.Append(uuid.MustParse(f.channel), end, end))
	table, e := ext.NewTable("selected_manifest", ext.Column("chunk_id", "UUID"), ext.Column("attempt_id", "UUID"))
	must(t, e)
	rows, e := f.env.Pool.Query(ctx, `SELECT chunk_id::text,attempt_id::text FROM monitoring_publication_chunks`)
	must(t, e)
	for rows.Next() {
		var chunk, attempt string
		must(t, rows.Scan(&chunk, &attempt))
		must(t, table.Append(uuid.MustParse(chunk), uuid.MustParse(attempt)))
	}
	rows.Close()
	for _, query := range []string{"SELECT count() FROM " + f.database + ".observations", "SELECT count() FROM " + f.database + ".observations WHERE (chunk_id,attempt_id) IN (SELECT chunk_id,attempt_id FROM selected_manifest)", "SELECT count() FROM " + f.database + ".observations WHERE channel_id IN (SELECT channel_id FROM authorized_windows)", "SELECT count() FROM authorized_windows WHERE isNull(lo) AND isNull(hi)", "SELECT count() FROM " + f.database + ".observations WHERE EXISTS(SELECT 1 FROM authorized_windows w WHERE w.channel_id=observations.channel_id AND (isNull(w.lo) OR toUnixTimestamp64Nano(coalesce(interval_start,observed_at))>=w.lo) AND (isNull(w.hi) OR toUnixTimestamp64Nano(observed_at)<w.hi))"} {
		var n uint64
		e = ch.QueryRow(clickhouse.Context(ctx, clickhouse.WithExternalTable(table, ranges)), query).Scan(&n)
		t.Log(query, n, e)
	}
}
