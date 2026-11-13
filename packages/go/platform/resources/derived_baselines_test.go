//go:build monitoringintegration

package resources_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
)

func TestDerivedFixedMedianKeepsSnapshotAcrossBackgroundHandoff(t *testing.T) {
	f := monitoringSetup(t)
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,10,good\n2024-01-01T00:00:01Z,14,good\n2024-01-01T00:00:02Z,20,good\n2024-01-01T00:00:03Z,26,good\n2024-01-01T00:00:04Z,30,good\n2024-01-01T00:00:05Z,34,good\n")))
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := func(second int) string {
		return strconv.FormatInt(base.Add(time.Duration(second)*time.Second).UnixNano(), 10)
	}
	s, ch, cfg := derivedChannelSetupBindings(t, f, map[string]dm.BaselineBinding{"origin": {Input: "x"}}, func(rule *dm.Rule) {
		rule.Nodes = []dm.Node{{ID: "result", Op: "baseline_delta", Inputs: []string{"input:x"}, Params: dm.NodeParams{Baseline: &dm.Baseline{Kind: "reference_window", Reference: "origin", Method: "median", FromNS: ns(0), ToNS: ns(4)}}}}
	})
	s.WithStorage(f.storage)
	options := config.DefaultDerived()
	options.TrialRows = 2
	options.WriteRows = 8
	s.WithOptions(options)
	end := ns(6)
	scope := dm.TenantScope(f.tenant)
	run, e := s.ChannelTrial(ctx, f.actor, scope, ch.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(ch.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: ns(4), To: &end}}, access.Meta{})
	must(t, e)
	if run.Job.State != "queued" {
		t.Fatal("baseline did not cross the interactive budget", run)
	}
	var snapshot string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT result->>'id' FROM monitoring_job_steps WHERE job_id=$1 AND step_key LIKE 'baseline-snapshot:%'`, run.Job.ID).Scan(&snapshot))
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:01.5Z,100,good\n")))
	_, e = f.service.RunTask(ctx, "median-after-handoff", f.tenant, run.Job.ID)
	must(t, e)
	run, e = s.Run(ctx, f.actor, scope, run.ID)
	must(t, e)
	if run.Trial == nil || len(run.Trial.Examples) != 2 || run.Trial.Examples[0].Value == nil || *run.Trial.Examples[0].Value != "13.000000000000000000" || *run.Trial.Examples[1].Value != "17.000000000000000000" || run.Trial.Baselines[0].Snapshot != snapshot {
		t.Fatal("background retry changed fixed baseline", run)
	}
	var count string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT evidence->>'count' FROM derived_fixed_baselines WHERE config_id=$1`, cfg.ID).Scan(&count))
	if count != "4" {
		t.Fatal("later append entered the original baseline", count)
	}
}

func TestDerivedObservationBaselineIdentityAndCausality(t *testing.T) {
	f := monitoringSetup(t)
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:02:00Z,9,good\n")))
	page, e := f.service.Details(ctx, f.actor, f.scope, f.query(t))
	must(t, e)
	if len(page.Items) != 2 {
		t.Fatal(page)
	}
	selected := page.Items[0]
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	binding := dm.BaselineBinding{Input: "x", ObservationID: selected.ID, AdoptionID: selected.Adoption, AtNS: strconv.FormatInt(start.Add(2*time.Minute).UnixNano(), 10)}
	service, channel, cfg := derivedChannelSetupBindings(t, f, map[string]dm.BaselineBinding{"origin": binding}, func(rule *dm.Rule) {
		rule.Nodes = []dm.Node{{ID: "result", Op: "baseline_delta", Inputs: []string{"input:x"}, Params: dm.NodeParams{Baseline: &dm.Baseline{Kind: "observation", Reference: "origin"}}}}
		rule.Outputs[0].SemanticKey = "fixed_observation_difference"
	})
	end := strconv.FormatInt(start.Add(3*time.Minute).UnixNano(), 10)
	run, e := service.ChannelTrial(ctx, f.actor, dm.TenantScope(f.tenant), channel.Resource.ID, dm.TrialInput{WriteMeta: dm.WriteMeta{RequestID: uuid.NewString(), Revision: channel.ConfigRevision, Reason: "固定观测身份及非因果基准验证"}, ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: strconv.FormatInt(start.UnixNano(), 10), To: &end}}, access.Meta{})
	must(t, e)
	if run.Trial == nil || len(run.Trial.Examples) != 2 {
		t.Fatal(run)
	}
	first, last := run.Trial.Examples[0], run.Trial.Examples[1]
	if first.Status != "noncausal_baseline" || first.Value != nil || last.Value == nil || *last.Value != "0.000000000000000000" {
		t.Fatal(run.Trial)
	}
	found := false
	for _, match := range last.Matches {
		if match.Reference == "baseline:origin" && match.ID == selected.Adoption {
			found = true
		}
	}
	if !found {
		t.Fatal("fixed adoption identity missing", last)
	}
}

func TestDerivedFixedBaselineAppendRevokeAndReadopt(t *testing.T) {
	f := monitoringSetup(t)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := func(minutes int) string {
		return strconv.FormatInt(start.Add(time.Duration(minutes)*time.Minute).UnixNano(), 10)
	}
	original := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:00:40Z,4,good\n")))
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:02:00Z,9,good\n")))
	service, channel, cfg := derivedChannelSetupBindings(t, f, map[string]dm.BaselineBinding{"origin": {Input: "x"}}, func(rule *dm.Rule) {
		rule.Nodes = []dm.Node{{ID: "result", Op: "baseline_delta", Inputs: []string{"input:x"}, Params: dm.NodeParams{Baseline: &dm.Baseline{Kind: "reference_window", Reference: "origin", Method: "mean", FromNS: ns(0), ToNS: ns(1)}}}}
		rule.Outputs[0].SemanticKey = "fixed_mean_difference"
	})
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	scope := dm.TenantScope(f.tenant)
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "固定基准失效及采用身份验证"}
	}
	from, to := ns(2), ns(3)
	trialInput := dm.TrialInput{WriteMeta: meta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}
	trial, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, trialInput, access.Meta{})
	must(t, e)
	if trial.Trial == nil || len(trial.Trial.Examples) != 1 || trial.Trial.Examples[0].Value == nil || *trial.Trial.Examples[0].Value != "6.000000000000000000" || len(trial.Trial.Baselines) != 1 || !trial.Trial.Baselines[0].Valid {
		t.Fatal(trial)
	}
	baselineSnapshot := trial.Trial.Baselines[0].Snapshot
	added := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:20Z,99,good\n")))
	trialInput.WriteMeta = meta(channel.ConfigRevision)
	again, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, trialInput, access.Meta{})
	must(t, e)
	if again.Trial == nil || *again.Trial.Examples[0].Value != "6.000000000000000000" || again.Trial.Baselines[0].Snapshot != baselineSnapshot {
		t.Fatal("append replaced fixed reference", again)
	}
	preview, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: again.Trial.ReceiptID}})
	must(t, e)
	if len(preview.InputRanges) != 2 {
		t.Fatal("baseline authority omitted", preview.InputRanges)
	}
	_, e = service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	runQueued := func() {
		t.Helper()
		queued, e := service.ScheduleTenant(ctx, f.tenant)
		must(t, e)
		if queued != 1 {
			t.Fatal("queued", queued)
		}
		var id string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT j.id::text FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state='queued'`, f.tenant).Scan(&id))
		claimed, e := f.service.RunTask(ctx, "fixed-baseline-test", f.tenant, id)
		must(t, e)
		if !claimed {
			t.Fatal("job not claimed")
		}
		var state string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT state FROM monitoring_jobs WHERE id=$1`, id).Scan(&state))
		if state != "succeeded" {
			t.Fatal(state)
		}
	}
	runQueued()
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	fromText, toText := start.Add(2*time.Minute).Format(time.RFC3339Nano), start.Add(3*time.Minute).Format(time.RFC3339Nano)
	query := md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Limit: 10}
	read := func() md.DetailPage {
		t.Helper()
		page, e := f.service.Details(ctx, f.actor, f.scope, query)
		must(t, e)
		return page
	}
	page := read()
	if len(page.Items) != 1 || *page.Items[0].Calculated.Value != "6.000000000000000000" {
		t.Fatal(page)
	}
	originalGeneration := page.Items[0].Calculated.Generation
	verifyGenerationInputEvidence(t, f, service, originalGeneration, []string{"9", "2", "4"})
	revoke := func(id, revision string) {
		t.Helper()
		p, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, id, revision)
		must(t, e)
		_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, id, md.RevokeInput{WriteMeta: f.meta(revision), PreviewHash: p.Hash})
		must(t, e)
	}
	revoke(added.ID, added.Revision)
	page = read()
	if len(page.Items) != 1 || page.Items[0].Calculated.Status != "valid" {
		t.Fatal("revoking later non-reference adoption invalidated original basis", page)
	}
	revoke(original.ID, original.Revision)
	page = read()
	if len(page.Items) != 0 || page.Coverage[0].State != "pending" {
		t.Fatal("revoked fixed baseline remained current", page)
	}
	replayOriginal, e := service.ReplayGeneration(ctx, f.actor, scope, originalGeneration, dm.ReplayInput{WriteMeta: meta("")}, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "replay-frozen-baseline", f.tenant, replayOriginal.Job.ID)
	must(t, e)
	replayOriginal, e = service.Run(ctx, f.actor, scope, replayOriginal.ID)
	must(t, e)
	if replayOriginal.Replay == nil || !replayOriginal.Replay.Matched || replayOriginal.Replay.InputState != "revoked" || replayOriginal.Replay.Actual != 1 || replayOriginal.Replay.Examples[0].Value == nil || *replayOriginal.Replay.Examples[0].Value != "6.000000000000000000" {
		t.Fatal("historical replay reinterpreted the original valid baseline", replayOriginal)
	}
	old, e := service.Run(ctx, f.actor, scope, trial.ID)
	must(t, e)
	if old.Trial != nil || old.Job.ResultState != "input_invalidated" {
		t.Fatal("stale trial value exposed", old)
	}
	_, e = service.ConsumeChanges(ctx, f.tenant, time.Now())
	must(t, e)
	runQueued()
	page = read()
	if len(page.Items) != 1 || page.Items[0].Calculated.Status != "baseline_invalid" || page.Items[0].Calculated.Value != nil {
		t.Fatal(page)
	}
	replayInvalid, e := service.ReplayGeneration(ctx, f.actor, scope, page.Items[0].Calculated.Generation, dm.ReplayInput{WriteMeta: meta("")}, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "replay-invalid-baseline", f.tenant, replayInvalid.Job.ID)
	must(t, e)
	replayInvalid, e = service.Run(ctx, f.actor, scope, replayInvalid.ID)
	must(t, e)
	if replayInvalid.Replay == nil || !replayInvalid.Replay.Matched || replayInvalid.Replay.Actual != 1 || replayInvalid.Replay.Examples[0].Value != nil || replayInvalid.Replay.Examples[0].Status != "baseline_invalid" {
		t.Fatal("historical replay revived a baseline invalid at publication", replayInvalid)
	}
	latest, e := f.service.Analytics(ctx, f.actor, f.scope, "latest", query)
	must(t, e)
	if len(latest.Channels) != 1 || latest.Channels[0].LatestOutput == nil || latest.Channels[0].LatestOutput.Status != "baseline_invalid" || latest.Channels[0].Last != nil || latest.Channels[0].Derived.NoResults != "1" {
		t.Fatal("latest no-result was replaced with a former valid value", latest)
	}
	state, e := service.Channel(ctx, f.actor, scope, channel.Resource.ID)
	must(t, e)
	if state.Health != "blocked" || state.BlockedReason != "baseline_invalid" {
		t.Fatal(state)
	}
	original, e = f.service.Import(ctx, f.actor, f.scope, original.ID)
	must(t, e)
	readopt, e := f.service.Readopt(ctx, f.actor, f.scope, original.ID, md.ReadoptInput{WriteMeta: f.meta(original.Revision), ReadoptSelection: md.ReadoptSelection{All: true}})
	must(t, e)
	f.publish(t, f.validate(t, readopt))
	page = read()
	if len(page.Items) != 1 || page.Items[0].Calculated.Status != "baseline_invalid" {
		t.Fatal("readoption revived old fixed adoption", page)
	}
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_fixed_baselines WHERE tenant_id=$1 AND config_id=$2`, f.tenant, cfg.ID).Scan(&count))
	if count != 1 {
		t.Fatal(count)
	}
}
