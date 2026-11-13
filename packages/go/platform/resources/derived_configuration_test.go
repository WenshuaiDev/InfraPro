//go:build monitoringintegration

package resources_test

import (
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
)

func TestDerivedConfigurationCorrectionKeepsUnaffectedCurrentAndHistory(t *testing.T) {
	f := monitoringSetup(t)
	service, channel, cfg := derivedChannelSetup(t, f)
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	scope := dm.TenantScope(f.tenant)
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "历史配置分段与稳定结果身份验证"}
	}
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := func(minutes int) string {
		return strconv.FormatInt(base.Add(time.Duration(minutes)*time.Minute).UnixNano(), 10)
	}
	start, end := ns(0), ns(3)
	trial := func(config dm.ChannelConfig, from, to string) dm.Run {
		t.Helper()
		run, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: meta(channel.ConfigRevision), ConfigID: config.ID, ConfigHash: config.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, access.Meta{})
		must(t, e)
		if run.Trial == nil {
			t.Fatal(run)
		}
		return run
	}
	first := trial(cfg, start, end)
	preview, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &start, StopNS: &end, TrialReceiptID: first.Trial.ReceiptID}})
	must(t, e)
	enabled, e := service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	channel = enabled.Channel
	drain := func() {
		t.Helper()
		for n := 0; n < 12; n++ {
			queued, e := service.ScheduleTenant(ctx, f.tenant)
			must(t, e)
			if queued == 0 {
				return
			}
			var job string
			must(t, f.env.Pool.QueryRow(ctx, `SELECT j.id::text FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state='queued' ORDER BY j.created_at LIMIT 1`, f.tenant).Scan(&job))
			worked, e := f.service.RunTask(ctx, "configuration-test", f.tenant, job)
			must(t, e)
			if !worked {
				t.Fatal("queued task not claimed")
			}
			var state, code string
			must(t, f.env.Pool.QueryRow(ctx, `SELECT state,COALESCE(error_code,'') FROM monitoring_jobs WHERE id=$1`, job).Scan(&state, &code))
			if state != "succeeded" {
				t.Fatal(state, code)
			}
		}
		t.Fatal("configuration backlog did not drain")
	}
	_, e = service.ConsumeChanges(ctx, f.tenant, time.Now())
	must(t, e)
	drain()
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	fromText, toText := base.Format(time.RFC3339Nano), base.Add(3*time.Minute).Format(time.RFC3339Nano)
	query := md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Limit: 50}
	read := func() (md.DetailPage, error) {
		page, e := f.service.Details(ctx, f.actor, f.scope, query)
		sort.Slice(page.Items, func(i, j int) bool { return page.Items[i].Calculated.AtNS < page.Items[j].Calculated.AtNS })
		return page, e
	}
	before, e := read()
	must(t, e)
	if len(before.Items) != 3 {
		t.Fatal(before)
	}
	oldMiddle := before.Items[1].Calculated
	oldGeneration := oldMiddle.Generation
	next, e := service.SaveChannelConfig(ctx, f.actor, scope, channel.Resource.ID, dm.SaveConfigInput{WriteMeta: meta(channel.ConfigRevision), BaseConfigID: cfg.ID, DefinitionVersionID: cfg.DefinitionVersionID, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters, ChangeKind: "ordinary", NumericContract: &dm.NumericContract{Scale: 24, Rounding: "half_even"}}, access.Meta{})
	must(t, e)
	if next.Channel.DraftConfigID != cfg.ID {
		t.Fatal("candidate changed current default", next)
	}
	trialNext := trial(next.Config, ns(1), ns(2))
	correctionEnd := ns(2)
	change, e := service.PreviewConfig(ctx, f.actor, scope, channel.Resource.ID, dm.ConfigPreviewInput{Revision: channel.ConfigRevision, ConfigProposal: dm.ConfigProposal{ConfigID: next.Config.ID, Mode: "correction", Range: dm.NanosecondRange{From: ns(1), To: &correctionEnd}, TrialReceiptID: trialNext.Trial.ReceiptID}})
	must(t, e)
	changed, e := service.ApplyConfig(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ConfigRevision), PreviewID: change.ID, PreviewHash: change.Hash}, access.Meta{})
	must(t, e)
	channel = changed.Channel
	pending, e := read()
	must(t, e)
	if len(pending.Items) != 2 || len(pending.Coverage) != 3 || pending.Coverage[1].State != "pending" || pending.Items[1].Calculated.Generation != oldGeneration || *pending.Items[1].Calculated.Value != "4.000000000000000000" {
		t.Fatal("correction hid unaffected prefix/suffix or exposed old middle", pending)
	}
	drain()
	after, e := read()
	must(t, e)
	if len(after.Items) != 3 {
		t.Fatal(after)
	}
	middle := after.Items[1].Calculated
	if middle.Result != oldMiddle.Result || middle.Config != next.Config.ID || middle.Generation == oldGeneration || middle.Scale != 24 || *middle.Value != "3.000000000000000000000000" {
		t.Fatal("corrected identity/precision", middle)
	}
	if after.Items[0].Calculated.Generation != oldGeneration || after.Items[2].Calculated.Generation != oldGeneration {
		t.Fatal("unaffected results rewritten", after)
	}
	// Late input reaches both old and corrected assignments. A single current
	// default must never prevent recalculation of an older active config segment.
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:30Z,3,good\n")))
	_, e = service.ConsumeChanges(ctx, f.tenant, time.Now())
	must(t, e)
	drain()
	latest, e := read()
	must(t, e)
	if len(latest.Items) != 4 || *latest.Items[1].Calculated.Value != "1.000000000000000000" || *latest.Items[2].Calculated.Value != "2.000000000000000000000000" || latest.Items[2].Calculated.Config != next.Config.ID || latest.Items[3].Calculated.Config != cfg.ID {
		t.Fatal("multiple assignment recomputation", latest)
	}
	// Pausing automatic updates still allows an explicitly authorized manual
	// backfill. Its lease uses the user's current input rights, not the mandate.
	pause, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "pause"}})
	must(t, e)
	paused, e := service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: pause.ID, PreviewHash: pause.Hash}, access.Meta{})
	must(t, e)
	channel = paused.Channel
	backfill, e := service.PreviewBackfill(ctx, f.actor, scope, channel.Resource.ID, dm.BackfillPreviewInput{Revision: channel.ConfigRevision, Range: dm.NanosecondRange{From: start, To: &end}})
	must(t, e)
	if len(backfill.Targets) != 3 {
		t.Fatal("backfill lost active configuration segments", backfill)
	}
	backfillInput := dm.ControlInput{WriteMeta: meta(channel.ConfigRevision), PreviewID: backfill.ID, PreviewHash: backfill.Hash}
	manual, e := service.BackfillChannel(ctx, f.actor, scope, channel.Resource.ID, backfillInput, access.Meta{})
	must(t, e)
	replay, e := service.BackfillChannel(ctx, f.actor, scope, channel.Resource.ID, backfillInput, access.Meta{})
	must(t, e)
	if !replay.Replayed || len(manual.Runs) != 3 || replay.Runs[0].ID != manual.Runs[0].ID {
		t.Fatal("backfill request duplicated", manual, replay)
	}
	cancelled, e := service.ControlRun(ctx, f.actor, scope, manual.Runs[0].ID, "cancel", meta(manual.Runs[0].Job.Revision), access.Meta{})
	must(t, e)
	retried, e := service.ControlRun(ctx, f.actor, scope, cancelled.ID, "retry", meta(cancelled.Job.Revision), access.Meta{})
	must(t, e)
	if retried.Job.State != "queued" {
		t.Fatal(retried)
	}
	for _, run := range manual.Runs {
		worked, e := f.service.RunTask(ctx, "manual-backfill-test", f.tenant, run.Job.ID)
		must(t, e)
		if !worked {
			t.Fatal("manual backfill not claimed")
		}
		view, e := service.Run(ctx, f.actor, scope, run.ID)
		must(t, e)
		if view.Job.State != "succeeded" || view.Mode != "backfill" {
			t.Fatal(view)
		}
	}
	refilled, e := read()
	must(t, e)
	if len(refilled.Items) != 4 || *refilled.Items[2].Calculated.Value != "2.000000000000000000000000" || refilled.Items[2].Calculated.Result != latest.Items[2].Calculated.Result || refilled.Items[2].Calculated.Generation == latest.Items[2].Calculated.Generation {
		t.Fatal("manual backfill changed semantic value or failed to publish a new generation", refilled)
	}
	queued, e := service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	if queued != 0 {
		t.Fatal("manual backfill resumed automatic control", queued)
	}
	var preserved int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_generations WHERE tenant_id=$1 AND id=$2`, f.tenant, oldGeneration).Scan(&preserved))
	if preserved != 1 {
		t.Fatal("old generation erased")
	}
}
