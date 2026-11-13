//go:build monitoringintegration

package resources_test

import (
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func TestCalculationRawReaderRealIntervalSnapshotAndReplay(t *testing.T) {
	f := monitoringSetup(t)
	var facility, point string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text,point_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility, &point))
	interval, e := f.write(t, res.Target{Kind: res.Channel, Facility: facility, Point: point}, "create", map[string]any{"code": "CALC-INTERVAL", "name": "区间计算输入", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "interval_end", "aggregation_description": "前一小时平均"})
	must(t, e)
	f.channel = interval.ID
	mapping := md.Mapping{Format: "csv", Mode: "single", Encoding: "utf-8", Delimiter: ",", HeaderRow: 1, TimeColumn: "observed_at", IntervalColumn: "start", ValueColumn: "value", Timezone: "UTC", EmptyWide: "error", Channels: []md.ChannelMapping{{Alias: "C", Channel: f.channel, Unit: "mm"}}}
	batch := f.publish(t, f.validate(t, f.uploadBytes(t, "interval.csv", []byte("observed_at,start,value\n2024-01-01T01:00:00Z,2024-01-01T00:00:00Z,3\n"), mapping)))
	at := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC).UnixNano()
	lo, hi := at-15*int64(time.Minute), at+1
	input := md.RawFreezeRequest{Needs: []md.RawNeed{{Channel: f.channel, From: &lo, To: &hi}}}
	reader := f.service.RawReader()
	snapshot, e := reader.FreezeUser(ctx, f.actor, f.scope, input)
	must(t, e)
	var rows []md.RawSample
	budget := md.RawReadBudget{Rows: 10, Bytes: 1 << 20}
	count, e := reader.ReadUser(ctx, f.actor, f.scope, snapshot.ID, budget, func(r md.RawSample) error { rows = append(rows, r); return nil })
	must(t, e)
	if count.Rows != 1 || len(rows) != 1 || rows[0].At != at || rows[0].Start == nil || *rows[0].Start != at-int64(time.Hour) || rows[0].Value != "3.000000000000" {
		t.Fatal("interval endpoint was silently clipped", count, rows)
	}
	original := rows[0]
	var sealed []md.RawNeed
	must(t, f.env.Pool.QueryRow(ctx, `SELECT needs FROM monitoring_raw_snapshots WHERE id=$1`, snapshot.ID).Scan(&sealed))
	if len(sealed) != 1 || sealed[0].From == nil || *sealed[0].From != *original.Start {
		t.Fatal("full interval not preserved as authorization evidence", sealed)
	}
	other, e := f.write(t, res.Target{Kind: res.Channel, Facility: facility, Point: point}, "create", map[string]any{"code": "UNRELATED-INPUT", "name": "同快照其他输入", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	multiNeeds := append(append([]md.RawNeed{}, input.Needs...), md.RawNeed{Channel: other.ID, From: &lo, To: &hi})
	multi, e := reader.FreezeUser(ctx, f.actor, f.scope, md.RawFreezeRequest{Needs: multiNeeds})
	must(t, e)
	resolved, e := reader.ResolvePartUser(ctx, f.actor, f.scope, multi.ID, input)
	must(t, e)
	if len(resolved.Needs) != 1 || *resolved.Needs[0].From != *original.Start {
		t.Fatal("multi-input part lost complete interval proof", resolved)
	}
	tx, e := f.env.Pool.Begin(ctx)
	must(t, e)
	_, e = f.service.CheckRawSnapshotPartTx(ctx, tx, f.actor, f.scope, multi.ID, input.Needs, false)
	_ = tx.Rollback(ctx)
	must(t, e)
	checkParts := func(actor access.Actor, scope md.Scope, capability string) error {
		t.Helper()
		tx, e := f.env.Pool.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		return f.service.CheckRawSnapshotPartsTx(ctx, tx, actor, scope, []md.RawSnapshotPart{{Snapshot: snapshot.ID, Needs: input.Needs}, {Snapshot: multi.ID, Needs: input.Needs}, {Snapshot: snapshot.ID, Needs: input.Needs}}, capability)
	}
	must(t, checkParts(f.actor, f.scope, "read"))
	project := f.project(t, "IntervalCalculation")
	grant := f.accessInput(t, project, facility, "create_link")
	grant.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, project, facility, grant)
	var resourceGrant string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, f.channel).Scan(&resourceGrant))
	from, to := "2024-01-01T00:30:00Z", "2024-01-01T01:00:00.000000001Z"
	g, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta("0"), ChannelID: f.channel, ResourceGrantID: resourceGrant, Range: &md.Range{From: &from, To: &to}, Capabilities: []string{"read"}})
	must(t, e)
	projectScope := md.ProjectScope(f.tenant, project.id)
	_, e = reader.FreezeUser(ctx, project.actor, projectScope, input)
	monitoringCode(t, e, "monitoring_not_found")
	monitoringCode(t, checkParts(project.actor, projectScope, "read"), "monitoring_not_found")
	from, to = "2024-01-01T00:00:00Z", "2024-01-01T00:30:00Z"
	_, e = f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(g.AccessRevision), ChannelID: f.channel, ResourceGrantID: resourceGrant, Range: &md.Range{From: &from, To: &to}, Capabilities: []string{"read"}})
	must(t, e)
	_, e = reader.FreezeUser(ctx, project.actor, projectScope, input)
	must(t, e)
	must(t, checkParts(project.actor, projectScope, "read"))
	if checkParts(project.actor, projectScope, "export") == nil {
		t.Fatal("batched qualification widened read-only project grants to export")
	}
	// A new input outside the candidate interval cannot invalidate the receipt.
	f.publish(t, f.validate(t, f.uploadBytes(t, "later.csv", []byte("observed_at,start,value\n2024-01-01T03:00:00Z,2024-01-01T02:00:00Z,9\n"), mapping)))
	state, e := reader.CheckUser(ctx, f.actor, f.scope, snapshot.ID, false)
	must(t, e)
	if state.Changed {
		t.Fatal("unrelated append affected fixed snapshot")
	}
	must(t, checkParts(project.actor, projectScope, "read"))
	preview, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: preview.Hash})
	must(t, e)
	_, e = reader.ReadUser(ctx, f.actor, f.scope, snapshot.ID, budget, func(md.RawSample) error { t.Fatal("revoked value escaped current reader"); return nil })
	monitoringCode(t, e, "snapshot_stale")
	monitoringCode(t, checkParts(f.actor, f.scope, "read"), "snapshot_stale")
	rows = nil
	_, e = reader.ReplayUser(ctx, f.actor, f.scope, snapshot.ID, budget, func(r md.RawSample) error { rows = append(rows, r); return nil })
	must(t, e)
	if len(rows) != 1 || rows[0].Observation != original.Observation || rows[0].Adoption != original.Adoption || rows[0].Value != original.Value {
		t.Fatal("historical evidence changed", rows)
	}
	rows = nil
	_, e = reader.ReplayPartUser(ctx, f.actor, f.scope, multi.ID, input.Needs, budget, func(r md.RawSample) error { rows = append(rows, r); return nil })
	must(t, e)
	if len(rows) != 1 || rows[0].Observation != original.Observation {
		t.Fatal("partial multi-input replay failed after revocation", rows)
	}
	_, e = reader.ReplayUser(ctx, project.actor, projectScope, snapshot.ID, budget, func(md.RawSample) error { t.Fatal("read grant allowed history"); return nil })
	if e == nil {
		t.Fatal("history qualification missing")
	}
	batch, e = f.service.Import(ctx, f.actor, f.scope, batch.ID)
	must(t, e)
	again, e := f.service.Readopt(ctx, f.actor, f.scope, batch.ID, md.ReadoptInput{WriteMeta: f.meta(batch.Revision), ReadoptSelection: md.ReadoptSelection{All: true}})
	must(t, e)
	f.publish(t, f.validate(t, again))
	var types []string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT array_agg(change_type ORDER BY source_position) FROM monitoring_changes WHERE tenant_id=$1`, f.tenant).Scan(&types))
	if len(types) != 4 || types[0] != "raw_publish" || types[1] != "raw_publish" || types[2] != "raw_revoke" || types[3] != "raw_readopt" {
		t.Fatal("source changes were lost or duplicated", types)
	}
	fresh, e := reader.FreezeUser(ctx, f.actor, f.scope, input)
	must(t, e)
	rows = nil
	_, e = reader.ReadUser(ctx, f.actor, f.scope, fresh.ID, budget, func(r md.RawSample) error { rows = append(rows, r); return nil })
	must(t, e)
	if len(rows) != 1 || rows[0].Observation != original.Observation || rows[0].Adoption == original.Adoption {
		t.Fatal("readoption lost observation/adoption distinction", rows)
	}
}
