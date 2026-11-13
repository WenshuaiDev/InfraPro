//go:build monitoringintegration

package resources_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
)

func TestDerivedFutureInputImportedBeforeEnable(t *testing.T) {
	f := monitoringSetup(t)
	start := time.Now().UTC().Truncate(time.Minute).Add(24 * time.Hour)
	end := start.Add(2 * time.Minute)
	from, to := strconv.FormatInt(start.UnixNano(), 10), strconv.FormatInt(end.UnixNano(), 10)
	service, channel, cfg := derivedChannelSetup(t, f, func(rule *dm.Rule) {
		q := rule.Inputs[0].Quantity
		rule.Nodes = []dm.Node{{ID: "result", Op: "baseline_delta", Inputs: []string{"input:x"}, Params: dm.NodeParams{Baseline: &dm.Baseline{Kind: "constant", Value: "9", Quantity: &q}}}}
		rule.Outputs[0].SemanticKey = "future_zero"
		rule.Policies.AllowedLatenessNS = strconv.FormatInt(int64(10*time.Second), 10)
	})
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	f.publish(t, f.validate(t, f.upload(t, fmt.Sprintf("observed_at,value,quality\n%s,9,good\n", start.Format(time.RFC3339Nano)))))
	_, e := service.ConsumeChanges(ctx, f.tenant, time.Now())
	must(t, e)
	scope := dm.TenantScope(f.tenant)
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "启用前已导入未来观测验证"}
	}
	trial, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: meta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, access.Meta{})
	must(t, e)
	if trial.Trial == nil || trial.Trial.OutputCount != "0" {
		t.Fatal("future trial produced current output", trial)
	}
	preview, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	_, e = service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	var dirty int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&dirty))
	if dirty != 1 {
		t.Fatal("future imported input lost at enable", dirty)
	}
	queued, e := service.ScheduleTenantAt(ctx, f.tenant, start.Add(10*time.Second-time.Nanosecond))
	must(t, e)
	if queued != 0 {
		t.Fatal("future sample scheduled before lateness", queued)
	}
	restarted := dm.New(f.env.Service.Access()).WithResources(f.srv).WithMonitoring(f.service).WithStorage(f.storage)
	must(t, restarted.RegisterTasks())
	queued, e = restarted.ScheduleTenantAt(ctx, f.tenant, start.Add(10*time.Second))
	must(t, e)
	if queued != 1 {
		t.Fatal("future target depended on another import", queued)
	}
	var job string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT j.id::text FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state='queued'`, f.tenant).Scan(&job))
	_, e = f.service.RunTask(ctx, "future-test", f.tenant, job)
	must(t, e)
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	fromText, toText := start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano)
	page, e := f.service.Details(ctx, f.actor, f.scope, md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Limit: 10})
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].Calculated.Value == nil || *page.Items[0].Calculated.Value != "0.000000000000000000" {
		t.Fatal(page)
	}
}

func TestDerivedClockDueRestartAndEmptyCount(t *testing.T) {
	f := monitoringSetup(t)
	start := time.Now().UTC().Truncate(time.Minute).Add(24 * time.Hour)
	minute := strconv.FormatInt(int64(time.Minute), 10)
	service, channel, cfg := derivedChannelSetup(t, f, func(rule *dm.Rule) {
		rule.Clock = dm.Clock{Kind: "fixed_grid", AnchorNS: "0", StepNS: minute}
		rule.Nodes = []dm.Node{{ID: "result", Op: "window_count", Inputs: []string{"input:x"}, Params: dm.NodeParams{Window: &dm.Window{Kind: "fixed_duration", AnchorNS: "0", DurationNS: minute}, MinSamples: "0", MaxAgeNS: minute, MaxGapNS: minute}}}
		rule.Outputs[0].SemanticKey = "sample_count"
		rule.Outputs[0].Numeric.Scale = 0
		rule.Policies.AllowedLatenessNS = strconv.FormatInt(int64(10*time.Second), 10)
	})
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	scope := dm.TenantScope(f.tenant)
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "持久定时窗口验证"}
	}
	from, to := strconv.FormatInt(start.UnixNano(), 10), strconv.FormatInt(start.Add(3*time.Minute+time.Nanosecond).UnixNano(), 10)
	trial, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: meta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, access.Meta{})
	must(t, e)
	if trial.Trial == nil {
		t.Fatal(trial)
	}
	preview, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID, AllowWaiting: true}})
	must(t, e)
	_, e = service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	var dirty int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_dirty_ranges WHERE tenant_id=$1`, f.tenant).Scan(&dirty))
	if dirty != 0 {
		t.Fatal("future activation fabricated already due coverage", dirty)
	}
	queued, e := service.ScheduleTenantAt(ctx, f.tenant, start.Add(10*time.Second-time.Nanosecond))
	must(t, e)
	if queued != 0 {
		t.Fatal("window ran before allowed lateness", queued)
	}
	queued, e = service.ScheduleTenantAt(ctx, f.tenant, start.Add(10*time.Second))
	must(t, e)
	if queued != 1 {
		t.Fatal("due empty window was lost", queued)
	}
	runQueued := func() {
		t.Helper()
		var job string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT j.id::text FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state='queued'`, f.tenant).Scan(&job))
		_, e = f.service.RunTask(ctx, "clock-test", f.tenant, job)
		must(t, e)
	}
	runQueued()
	var next int64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT next_due_ns FROM derived_schedules WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&next))
	if next != start.Add(time.Minute).UnixNano() {
		t.Fatal("schedule did not advance with durable task receipt", next)
	}
	// A newly composed scheduler has no in-memory clock or consumer state.
	restarted := dm.New(f.env.Service.Access()).WithResources(f.srv).WithMonitoring(f.service).WithStorage(f.storage)
	must(t, restarted.RegisterTasks())
	queued, e = restarted.ScheduleTenantAt(ctx, f.tenant, start.Add(3*time.Minute+10*time.Second))
	must(t, e)
	if queued != 1 {
		t.Fatal("restart did not recover missed windows", queued)
	}
	runQueued()
	queued, e = restarted.ScheduleTenantAt(ctx, f.tenant, start.Add(10*time.Second))
	must(t, e)
	if queued != 0 {
		t.Fatal("clock rollback duplicated windows", queued)
	}
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	fromText, toText := start.Format(time.RFC3339Nano), start.Add(3*time.Minute+time.Nanosecond).Format(time.RFC3339Nano)
	page, e := f.service.Details(ctx, f.actor, f.scope, md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Qualities: []string{"unmarked"}, Limit: 10})
	must(t, e)
	if len(page.Items) != 4 {
		t.Fatal("missed or duplicated empty window output", page)
	}
	for i, item := range page.Items {
		p := item.Calculated
		if p == nil || p.Value == nil || *p.Value != "0" || p.Status != "valid" || p.Count != "0" || p.Start == nil || p.End == nil || p.At != start.Add(time.Duration(3-i)*time.Minute).Format(time.RFC3339Nano) {
			t.Fatal("empty count lost window semantics", p)
		}
	}
}
