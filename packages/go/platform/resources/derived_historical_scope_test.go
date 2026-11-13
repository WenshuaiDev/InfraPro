//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
)

func TestDerivedExplicitPrestartBackfillAndRetiredCorrection(t *testing.T) {
	f := monitoringSetup(t)
	service, channel, cfg := derivedChannelSetup(t, f)
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	// Register the same lifecycle seam used by the runtime composition.
	f.srv.WithExternalReferences(service)
	scope := dm.TenantScope(f.tenant)
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "明确历史范围与退役修正验证"}
	}
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := func(m int) string { return strconv.FormatInt(base.Add(time.Duration(m)*time.Minute).UnixNano(), 10) }
	end, autoStart := ns(3), ns(2)
	trial := func(config dm.ChannelConfig, from, to string) dm.Run {
		t.Helper()
		run, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: meta(channel.ConfigRevision), ConfigID: config.ID, ConfigHash: config.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, access.Meta{})
		must(t, e)
		if run.Trial == nil {
			t.Fatal(run)
		}
		return run
	}
	initial := trial(cfg, autoStart, end)
	preview, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &autoStart, StopNS: &end, TrialReceiptID: initial.Trial.ReceiptID}})
	must(t, e)
	enabled, e := service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	channel = enabled.Channel
	_, e = service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	var job string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND mode='automatic'`, f.tenant).Scan(&job))
	_, e = f.service.RunTask(ctx, "prestart-auto", f.tenant, job)
	must(t, e)
	beforeRange := dm.NanosecondRange{From: ns(1), To: &autoStart}
	if _, e = service.PreviewBackfill(ctx, f.actor, scope, channel.Resource.ID, dm.BackfillPreviewInput{Revision: channel.ConfigRevision, Range: beforeRange}); e == nil {
		t.Fatal("unassigned history silently used current configuration")
	}
	backfill, e := service.PreviewBackfill(ctx, f.actor, scope, channel.Resource.ID, dm.BackfillPreviewInput{Revision: channel.ConfigRevision, Range: beforeRange, UnassignedConfigID: cfg.ID})
	must(t, e)
	if len(backfill.Targets) != 1 || !backfill.Targets[0].Assign {
		t.Fatal("explicit assignment absent from preview", backfill)
	}
	result, e := service.BackfillChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ConfigRevision), PreviewID: backfill.ID, PreviewHash: backfill.Hash}, access.Meta{})
	must(t, e)
	if len(result.Runs) != 1 {
		t.Fatal(result)
	}
	_, e = f.service.RunTask(ctx, "prestart-backfill", f.tenant, result.Runs[0].Job.ID)
	must(t, e)
	channel, e = service.Channel(ctx, f.actor, scope, channel.Resource.ID)
	must(t, e)
	var automaticRange string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT m.output_ranges::text FROM derived_channels c JOIN derived_execution_mandates m ON m.id=c.active_mandate_id WHERE c.tenant_id=$1 AND c.channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&automaticRange))
	if automaticRange != "{["+autoStart+","+end+")}" {
		t.Fatal("manual history expanded automatic mandate", automaticRange)
	}
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	fromText, toText := base.Format(time.RFC3339Nano), base.Add(3*time.Minute).Format(time.RFC3339Nano)
	query := md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Limit: 10}
	page, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 2 {
		t.Fatal("explicit historical range not published", page)
	}
	var notComputed bool
	for _, part := range page.Coverage {
		notComputed = notComputed || part.State == "not_computed"
	}
	if !notComputed {
		t.Fatal("unselected earlier history became computed", page)
	}
	pause, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "pause"}})
	must(t, e)
	_, e = service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: pause.ID, PreviewHash: pause.Hash}, access.Meta{})
	must(t, e)
	target := res.Target{Kind: res.Point, ID: channel.Resource.PointID, Facility: channel.Resource.FacilityID}
	body := map[string]any{"request_id": uuid.NewString(), "expected_revision": 1, "reason": "停止祖先及所属派生通道"}
	raw, e := json.Marshal(body)
	must(t, e)
	retire, e := f.srv.PreviewMutation(ctx, f.actor, res.TenantScope(f.tenant), target, "retire", raw)
	must(t, e)
	if !retire.CanSubmit {
		t.Fatal(retire)
	}
	body["preview_hash"] = retire.Hash
	raw, e = json.Marshal(body)
	must(t, e)
	_, e = f.srv.Mutate(ctx, f.actor, res.TenantScope(f.tenant), target, "retire", raw, access.Meta{})
	must(t, e)
	channel, e = service.Channel(ctx, f.actor, scope, channel.Resource.ID)
	must(t, e)
	if channel.ControlState != "retired" || channel.StopNS == nil {
		t.Fatal(channel)
	}
	stop, _ := strconv.ParseInt(*channel.StopNS, 10, 64)
	outsideEnd := strconv.FormatInt(stop+1, 10)
	if _, e = service.PreviewBackfill(ctx, f.actor, scope, channel.Resource.ID, dm.BackfillPreviewInput{Revision: channel.ConfigRevision, Range: dm.NanosecondRange{From: *channel.StopNS, To: &outsideEnd}, UnassignedConfigID: cfg.ID}); e == nil {
		t.Fatal("retired channel accepted new output after stop")
	}
	candidate, e := service.SaveChannelConfig(ctx, f.actor, scope, channel.Resource.ID, dm.SaveConfigInput{WriteMeta: meta(channel.ConfigRevision), BaseConfigID: cfg.ID, DefinitionVersionID: cfg.DefinitionVersionID, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters, ChangeKind: "correction", SemanticIntentConfirmed: true, NumericContract: &dm.NumericContract{Scale: 24, Rounding: "half_even"}}, access.Meta{})
	must(t, e)
	correctedTrial := trial(candidate.Config, ns(1), autoStart)
	change, e := service.PreviewConfig(ctx, f.actor, scope, channel.Resource.ID, dm.ConfigPreviewInput{Revision: channel.ConfigRevision, ConfigProposal: dm.ConfigProposal{ConfigID: candidate.Config.ID, Mode: "correction", Range: beforeRange, TrialReceiptID: correctedTrial.Trial.ReceiptID}})
	must(t, e)
	applied, e := service.ApplyConfig(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ConfigRevision), PreviewID: change.ID, PreviewHash: change.Hash}, access.Meta{})
	must(t, e)
	channel = applied.Channel
	if channel.ControlState != "retired" {
		t.Fatal("correction restored retired calculation", channel)
	}
	backfill, e = service.PreviewBackfill(ctx, f.actor, scope, channel.Resource.ID, dm.BackfillPreviewInput{Revision: channel.ConfigRevision, Range: beforeRange})
	must(t, e)
	result, e = service.BackfillChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ConfigRevision), PreviewID: backfill.ID, PreviewHash: backfill.Hash}, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "retired-correction", f.tenant, result.Runs[0].Job.ID)
	must(t, e)
	queued, e := service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	if queued != 0 {
		t.Fatal("retired correction restarted automatic work", queued)
	}
	selection, e = f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	query.Selection = selection.ID
	page, e = f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	found := false
	for _, item := range page.Items {
		if item.Calculated.Config == candidate.Config.ID {
			found = true
			if item.Calculated.Scale != 24 || item.Calculated.Value == nil || *item.Calculated.Value != "3.000000000000000000000000" {
				t.Fatal(item)
			}
		}
	}
	if !found {
		t.Fatal("retired history correction missing", page)
	}
}
