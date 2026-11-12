//go:build monitoringintegration

package resources_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
)

func TestMonitoringRouteCopyPauseTailSwitchAndReverseCopy(t *testing.T) {
	var second string
	f := monitoringSetup(t, func(c *md.StorageConfig) {
		second = c.Database + "_second"
		c.Shards = []md.PhysicalShardConfig{{Name: "second", Database: second, Replicas: c.Replicas}}
	})
	t.Cleanup(func() {
		for _, port := range []string{"59000", "59001"} {
			c, e := clickhouse.Open(&clickhouse.Options{Addr: []string{"127.0.0.1:" + port}})
			if e == nil {
				e = c.Exec(context.Background(), "DROP DATABASE "+second+" SYNC")
				c.Close()
			}
			if e != nil {
				t.Error(e)
			}
		}
	})
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n1900-01-01T00:00:00Z,10000000000000000000000000.000000000001,good\n2024-01-01T00:00:00.000000001Z,3,bad\n")))
	// A second channel on a different logical shard remains on the local route.
	var facility, point string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text,point_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility, &point))
	ct := target(res.Result{ID: facility}, res.Channel, "")
	ct.Point = point
	// Use fixture target's facility ID directly (the constructor only needs ID).
	other := f
	for n := 0; ; n++ {
		v, e := f.write(t, ct, "create", map[string]any{"code": fmt.Sprint("OTHER", n), "name": "并行物理分片", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
		must(t, e)
		other.channel = v.ID
		if testShard(f.tenant, other.channel) != testShard(f.tenant, f.channel) {
			break
		}
	}
	other.publish(t, other.validate(t, other.upload(t, "observed_at,value,quality\n2023-01-01T00:00:00.000000001Z,2,good\n2262-04-11T23:47:16.854775807Z,4,good\n")))
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{f.channel, other.channel}})
	must(t, e)
	q := f.query(t)
	q.Selection = selection.ID
	q.Limit = 2
	before, e := f.service.Details(ctx, f.actor, f.scope, q)
	must(t, e)
	q.Snapshot = before.Snapshot.ID
	plan, e := f.service.PlanRoute(ctx, f.tenant, testShard(f.tenant, f.channel), "second")
	must(t, e)
	if plan.Chunks != "2" || plan.Rows != "2" || plan.Source != "local" {
		t.Fatal(plan)
	}
	r, e := f.service.CreateRelocation(ctx, plan, "合成分片复制验收")
	must(t, e)
	run := func(mode string) {
		t.Helper()
		var e error
		r, e = f.service.Relocation(ctx, f.tenant, r.ID)
		must(t, e)
		id, e := f.service.QueueRoute(ctx, f.tenant, r.ID, r.Revision, mode, "合成迁移继续")
		must(t, e)
		for n := 0; n < 30; n++ {
			worked, e := f.service.RunOne(ctx, "route-test")
			must(t, e)
			if !worked {
				t.Fatal("route job unavailable")
			}
			var state string
			must(t, f.env.Pool.QueryRow(ctx, `SELECT state FROM monitoring_jobs WHERE id=$1`, id).Scan(&state))
			if state == "succeeded" {
				r, e = f.service.Relocation(ctx, f.tenant, r.ID)
				must(t, e)
				return
			}
		}
		t.Fatal("relocation did not complete")
	}
	// A lost seal commit leaves a real private physical write. Retrying must
	// allocate a new attempt and keep the original route visible throughout.
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION test_route_seal_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='sealed' AND NEW.physical_shard='second' THEN RAISE EXCEPTION 'synthetic lost route seal commit'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_route_seal_failure BEFORE UPDATE ON monitoring_write_attempts FOR EACH ROW EXECUTE FUNCTION test_route_seal_failure()`)
	must(t, e)
	failedID, e := f.service.QueueRoute(ctx, f.tenant, r.ID, r.Revision, "copy", "迁移封存提交故障")
	must(t, e)
	var failed md.Job
	for n := 0; n < 20; n++ {
		_, runErr := f.service.RunOne(ctx, "route-fault")
		failed, e = f.service.InspectOperatorJob(ctx, f.tenant, failedID)
		must(t, e)
		if runErr != nil && failed.State == "retry_wait" {
			break
		}
		if n == 19 {
			t.Fatal("route fault was not reached", failed, runErr)
		}
	}
	still, e := f.service.Details(ctx, f.actor, f.scope, q)
	must(t, e)
	if fmt.Sprint(still.Items) != fmt.Sprint(before.Items) {
		t.Fatal("private failed copy leaked")
	}
	var uncertain int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_write_attempts WHERE physical_shard='second' AND state='uncertain'`).Scan(&uncertain))
	if uncertain != 1 {
		t.Fatal("uncertain attempt not retained", uncertain)
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER test_route_seal_failure ON monitoring_write_attempts; DROP FUNCTION test_route_seal_failure()`)
	must(t, e)
	must(t, f.service.RetryOperatorJob(ctx, f.tenant, failedID, failed.Revision, "故障修复后使用新 attempt"))
	for n := 0; n < 20; n++ {
		_, e = f.service.RunOne(ctx, "route-resume")
		must(t, e)
		failed, e = f.service.InspectOperatorJob(ctx, f.tenant, failedID)
		must(t, e)
		if failed.State == "succeeded" {
			break
		}
		if n == 19 {
			t.Fatal(failed)
		}
	}
	run("copy")
	if r.State != "verifying" || r.Copied != "2" {
		t.Fatal(r)
	}
	must(t, f.service.PauseRoute(ctx, f.tenant, r.ID, r.Revision, "暂停验收"))
	r, e = f.service.Relocation(ctx, f.tenant, r.ID)
	must(t, e)
	if r.State != "paused" {
		t.Fatal(r)
	}
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2025-01-01T00:00:00.000000001Z,5,good\n")))
	run("switch")
	if r.State != "switched" || r.Copied != "3" {
		t.Fatal(r)
	}
	old, e := f.service.Details(ctx, f.actor, f.scope, q)
	must(t, e)
	if old.Snapshot.RouteRevision != "1" || fmt.Sprint(old.Items) != fmt.Sprint(before.Items) {
		t.Fatal("old snapshot moved", old)
	}
	// Cross-shard keyset merge: first and subsequent pages are exact, without
	// counting private copies as new observations or losing nanosecond endpoints.
	fresh := q
	fresh.Snapshot = ""
	fresh.Cursor = ""
	seen := map[string]bool{}
	var ordered []string
	for n := 0; n < 10; n++ {
		page, e := f.service.Details(ctx, f.actor, f.scope, fresh)
		must(t, e)
		if page.Snapshot.RouteRevision != "2" {
			t.Fatal(page.Snapshot)
		}
		fresh.Snapshot = page.Snapshot.ID
		for _, v := range page.Items {
			if seen[v.ID] {
				t.Fatal("duplicate across physical shards")
			}
			seen[v.ID] = true
			ordered = append(ordered, v.Value)
		}
		if !page.HasMore {
			break
		}
		fresh.Cursor = page.NextCursor
	}
	if fmt.Sprint(ordered) != "[4.000000000000 5.000000000000 3.000000000000 2.000000000000 10000000000000000000000000.000000000001]" {
		t.Fatal(ordered)
	}
	// Publish on the new physical shard, then reverse-copy it before rollback.
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2026-01-01T00:00:00.000000001Z,6,good\n")))
	reverse, e := f.service.RollbackRoute(ctx, f.tenant, r.ID, r.Revision, "含切换后新增数据的安全回退")
	must(t, e)
	r = reverse
	run("switch")
	if r.State != "switched" || r.Plan.Source != "second" || r.Plan.Target != "local" || r.Copied != "4" {
		t.Fatal(r)
	}
	latest := q
	latest.Snapshot = ""
	latest.Limit = 100
	page, e := f.service.Details(ctx, f.actor, f.scope, latest)
	must(t, e)
	if len(page.Items) != 6 || page.Snapshot.RouteRevision != "3" {
		t.Fatal(page)
	}
	retained, e := f.service.Details(ctx, f.actor, f.scope, q)
	must(t, e)
	if fmt.Sprint(retained.Items) != fmt.Sprint(before.Items) {
		t.Fatal("rollback changed retained snapshot")
	}
	var locations int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_chunk_locations WHERE tenant_id=$1`, f.tenant).Scan(&locations))
	if locations != 7 {
		t.Fatal(locations)
	}
}
func testShard(tenant, channel string) int {
	a, b := uuid.MustParse(tenant), uuid.MustParse(channel)
	h := sha256.New()
	h.Write(a[:])
	h.Write(b[:])
	return int(h.Sum(nil)[0])
}
