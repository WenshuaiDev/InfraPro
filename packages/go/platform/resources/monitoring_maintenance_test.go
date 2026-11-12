//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMonitoringReadOnlyVerificationAndImmutableSummaryRepair(t *testing.T) {
	f := monitoringSetup(t)
	a := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n1900-01-01T00:00:00Z,10000000000000000000000000.000000000001,good\n2024-01-01T00:00:00.000000001Z,10000000000000000000000000.000000000003,good\n")))
	b := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2025-01-01T00:00:00Z,99,bad\n")))
	prev, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, b.ID, b.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, b.ID, md.RevokeInput{WriteMeta: f.meta(b.Revision), PreviewHash: prev.Hash})
	must(t, e)
	query := f.query(t)
	query.Limit = 100
	before, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	query.Snapshot = before.Snapshot.ID
	analytic := f.analytic(t, "overview", query)
	good, e := f.service.Verify(ctx, f.tenant)
	must(t, e)
	if good.State != "passed" || good.Rows != "3" || good.Files != "2" {
		t.Fatal(good)
	}
	var key string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT summary_object FROM monitoring_chunks WHERE batch_id=$1 ORDER BY id LIMIT 1`, a.ID).Scan(&key))
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	// Deliberately corrupt one derived object in this disposable tenant prefix.
	_, e = client.PutObject(ctx, "contract-source", key, bytes.NewBufferString("synthetic corruption"), 20, minio.PutObjectOptions{})
	must(t, e)
	bad, e := f.service.Verify(ctx, f.tenant)
	must(t, e)
	if bad.State != "failed" || len(bad.Findings) == 0 {
		t.Fatal(bad)
	}
	var quarantine bool
	var replacements int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT quarantined FROM monitoring_channel_versions WHERE channel_id=$1`, f.channel).Scan(&quarantine))
	if quarantine {
		t.Fatal("read-only verification mutated state")
	}
	plan, e := f.service.PlanSummaryRepair(ctx, f.tenant, f.channel)
	must(t, e)
	job, e := f.service.QueueSummaryRepair(ctx, plan, "合成损坏摘要重建")
	must(t, e)
	_, e = f.service.Details(ctx, f.actor, f.scope, query)
	monitoringCode(t, e, "monitoring_integrity_unavailable")
	_, e = f.service.QueueSummaryRepair(ctx, plan, "重复旧预览")
	if e == nil {
		t.Fatal("stale repair plan accepted")
	}
	for n := 0; n < 30; n++ {
		state, e := f.service.InspectOperatorJob(ctx, f.tenant, job)
		must(t, e)
		if state.State == "succeeded" {
			break
		}
		worked, e := f.service.RunOne(ctx, "repair-test")
		must(t, e)
		if !worked || n == 29 {
			t.Fatal("repair did not finish", state)
		}
	}
	_, e = f.service.Details(ctx, f.actor, f.scope, query)
	monitoringCode(t, e, "snapshot_stale")
	query.Snapshot = ""
	after, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if fmt.Sprint(before.Items) != fmt.Sprint(after.Items) {
		t.Fatal("summary repair changed raw identity")
	}
	updated := f.analytic(t, "overview", query)
	if !reflect.DeepEqual(updated.Overview, analytic.Overview) {
		t.Fatal("summary repair changed exact results", analytic, updated)
	}
	good, e = f.service.Verify(ctx, f.tenant)
	must(t, e)
	if good.State != "passed" {
		t.Fatal(good)
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_summary_replacements WHERE tenant_id=$1`, f.tenant).Scan(&replacements))
	if replacements != 1 {
		t.Fatal(replacements)
	}
	// Corrupted old evidence is retained; the repair has a new immutable path.
	old, e := client.GetObject(ctx, "contract-source", key, minio.GetObjectOptions{})
	must(t, e)
	defer old.Close()
	var text bytes.Buffer
	_, e = text.ReadFrom(old)
	must(t, e)
	if text.String() != "synthetic corruption" {
		t.Fatal("repair overwrote old evidence")
	}
	// Future revoke must use the replacement contribution and preserve history.
	prev, e = f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, a.ID, a.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, a.ID, md.RevokeInput{WriteMeta: f.meta(a.Revision), PreviewHash: prev.Hash})
	must(t, e)
	query = f.query(t)
	_ = f.analytic(t, "overview", query)
	good, e = f.service.Verify(ctx, f.tenant)
	must(t, e)
	if good.State != "passed" {
		t.Fatal(good)
	}
}

func TestMonitoringDailyVerificationAndUnrepairableSourceQuarantine(t *testing.T) {
	f := monitoringSetup(t)
	// Change only the isolated fixture's default clock; immutable published rows
	// are never rewritten to make the scheduler test eligible.
	_, e := f.env.Pool.Exec(ctx, `ALTER TABLE monitoring_publications ALTER COLUMN committed_at SET DEFAULT (clock_timestamp()-INTERVAL '2 days')`)
	must(t, e)
	b := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2020-01-01T00:00:00Z,2,good\n")))
	_ = f.analytic(t, "overview", f.query(t))
	worker, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 2)
	for n := 0; n < 2; n++ {
		go func() { done <- f.service.RunWorker(worker, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	}
	finished := false
	for n := 0; n < 100; n++ {
		var count, passed int
		must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE state='succeeded' AND progress->>'phase'='passed') FROM monitoring_jobs WHERE tenant_id=$1 AND kind='verify'`, f.tenant).Scan(&count, &passed))
		if count > 1 {
			t.Fatal("duplicate scheduled checks", count)
		}
		if passed == 1 {
			finished = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	stop()
	for n := 0; n < 2; n++ {
		must(t, <-done)
	}
	if !finished {
		t.Fatal("daily check did not complete")
	}
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	var key, watermark string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT object_key FROM monitoring_source_files WHERE id=$1`, b.SourceFile).Scan(&key))
	_, e = client.PutObject(ctx, "contract-source", key, bytes.NewBufferString("corrupt"), 7, minio.PutObjectOptions{})
	must(t, e)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT commit_sequence::text FROM monitoring_tenant_states WHERE tenant_id=$1`, f.tenant).Scan(&watermark))
	id, e := f.service.QueueVerification(ctx, f.tenant, watermark, "合成源文件损坏隔离演练")
	must(t, e)
	for n := 0; n < 20; n++ {
		_, e = f.service.RunOne(ctx, "verify-source")
		must(t, e)
		job, e := f.service.InspectOperatorJob(ctx, f.tenant, id)
		must(t, e)
		if job.State == "succeeded" {
			break
		}
		if n == 19 {
			t.Fatal(job)
		}
	}
	report, e := f.service.OperatorJobReport(ctx, f.tenant, id)
	must(t, e)
	if report.State != "failed" {
		t.Fatal(report)
	}
	_, e = f.service.Details(ctx, f.actor, f.scope, f.query(t))
	monitoringCode(t, e, "monitoring_integrity_unavailable")
	_, e = f.service.RunOne(ctx, "unrepairable-source")
	if e == nil {
		t.Fatal("summary repair concealed corrupted source")
	}
	var quarantined bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT quarantined FROM monitoring_channel_versions WHERE channel_id=$1`, f.channel).Scan(&quarantined))
	if !quarantined {
		t.Fatal("unverified source was released")
	}
}
