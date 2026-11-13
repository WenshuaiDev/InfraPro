//go:build monitoringintegration

package resources_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
)

func TestDerivedActualPredecessorAcrossGapAndSubrange(t *testing.T) {
	f := monitoringSetup(t)
	service, channel, cfg := derivedChannelSetup(t, f)
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	old := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n")))
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T02:00:00Z,5,good\n2024-01-01T02:01:00Z,9,good\n")))
	scope := dm.TenantScope(f.tenant)
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "真实前驱与分块边界验证"}
	}
	start := time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC)
	from, to := strconv.FormatInt(start.UnixNano(), 10), strconv.FormatInt(start.Add(2*time.Minute).UnixNano(), 10)
	trialInput := dm.TrialInput{WriteMeta: meta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}
	trial, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, trialInput, access.Meta{})
	must(t, e)
	if trial.Trial == nil || len(trial.Trial.Examples) != 2 || trial.Trial.Examples[0].Status != "gap_exceeded" || trial.Trial.Examples[0].Count != "2" || len(trial.Trial.Examples[0].Matches) != 2 || trial.Trial.Examples[0].Matches[0].Offset != "-7200000000000" {
		t.Fatal("finite max_gap truncated the actual predecessor", trial.Trial)
	}
	if trial.Trial.InputCounts.Rows != 3 {
		t.Fatal("actual prefix not counted against input budget", trial.Trial.InputCounts)
	}
	narrowInput := trialInput
	narrowInput.WriteMeta = meta(channel.ConfigRevision)
	narrowInput.OutputRange.From = strconv.FormatInt(start.Add(time.Minute).UnixNano(), 10)
	narrow, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, narrowInput, access.Meta{})
	must(t, e)
	if narrow.Trial == nil || len(narrow.Trial.Examples) != 1 || narrow.Trial.Examples[0].Value == nil || *narrow.Trial.Examples[0].Value != "4.000000000000000000" || narrow.Trial.InputCounts.Rows != 2 {
		t.Fatal("subrange lost predecessor or read unrelated distant evidence", narrow.Trial)
	}
	preview, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	_, e = service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	queued, e := service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	if queued != 1 {
		t.Fatal(queued)
	}
	runQueued := func() {
		t.Helper()
		var job string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT j.id::text FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state='queued'`, f.tenant).Scan(&job))
		_, e = f.service.RunTask(ctx, "prefix-test", f.tenant, job)
		must(t, e)
	}
	runQueued()
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	fromText, toText := start.Add(time.Minute).Format(time.RFC3339Nano), start.Add(2*time.Minute).Format(time.RFC3339Nano)
	query := md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Qualities: []string{"good", "unmarked"}, Limit: 10}
	page, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].Calculated == nil || page.Items[0].Calculated.Value == nil || *page.Items[0].Calculated.Value != "4.000000000000000000" {
		t.Fatal(page)
	}
	stableID := page.Items[0].Calculated.Result
	revoke, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, old.ID, old.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, old.ID, md.RevokeInput{WriteMeta: f.meta(old.Revision), PreviewHash: revoke.Hash})
	must(t, e)
	page, e = f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].Calculated.Result != stableID {
		t.Fatal("unrelated predecessor revocation invalidated later output", page)
	}
	fromText = start.Format(time.RFC3339Nano)
	page, e = f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 0 || page.Coverage[0].State != "pending" {
		t.Fatal("revoked distant predecessor was omitted from freshness proof", page)
	}
	_, e = service.ConsumeChanges(ctx, f.tenant, time.Now())
	must(t, e)
	queued, e = service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	if queued != 1 {
		t.Fatal(queued)
	}
	runQueued()
	page, e = f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 2 || page.Items[1].Calculated.Status != "missing_input" || page.Items[0].Calculated.Result != stableID {
		t.Fatal("new generation failed to preserve result identity and actual absence", page)
	}
}

func TestDerivedSampleWindowUsesNthActualPredecessor(t *testing.T) {
	f := monitoringSetup(t)
	service, channel, cfg := derivedChannelSetup(t, f, func(rule *dm.Rule) {
		rule.Nodes = []dm.Node{{ID: "result", Op: "window_mean", Inputs: []string{"input:x"}, Params: dm.NodeParams{Window: &dm.Window{Kind: "trailing_count", Count: "2", MaxSpanNS: "600000000000"}, MinSamples: "2", MaxGapNS: "600000000000", MaxAgeNS: "600000000000"}}}
		rule.Outputs[0].SemanticKey = "last_two_mean"
	})
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T01:59:00Z,99,bad\n2024-01-01T02:00:00Z,5,good\n2024-01-01T02:01:00Z,9,good\n")))
	start := time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC)
	to := strconv.FormatInt(start.Add(2*time.Minute).UnixNano(), 10)
	input := dm.TrialInput{WriteMeta: dm.WriteMeta{RequestID: uuid.NewString(), Revision: channel.ConfigRevision, Reason: "样本窗实际前驱验证"}, ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: strconv.FormatInt(start.UnixNano(), 10), To: &to}}
	trial, e := service.ChannelTrial(ctx, f.actor, dm.TenantScope(f.tenant), channel.Resource.ID, input, access.Meta{})
	must(t, e)
	if trial.Trial == nil || len(trial.Trial.Examples) != 2 {
		t.Fatal(trial)
	}
	first, last := trial.Trial.Examples[0], trial.Trial.Examples[1]
	if first.Status != "gap_exceeded" || first.Count != "2" || first.Excluded != "1" || last.Value == nil || *last.Value != "7.000000000000000000" || last.Count != "2" || trial.Trial.InputCounts.Rows != 4 {
		t.Fatal("max_span clipped N or excluded quality evidence", trial.Trial)
	}
	input.RequestID = uuid.NewString()
	input.OutputRange.From = strconv.FormatInt(start.Add(time.Minute).UnixNano(), 10)
	narrow, e := service.ChannelTrial(ctx, f.actor, dm.TenantScope(f.tenant), channel.Resource.ID, input, access.Meta{})
	must(t, e)
	if narrow.Trial == nil || len(narrow.Trial.Examples) != 1 || narrow.Trial.Examples[0].Value == nil || *narrow.Trial.Examples[0].Value != *last.Value {
		t.Fatal("sample window changed at output partition", narrow.Trial)
	}
}
