//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
)

func TestDerivedEightLayerFixedSourcesAndRecursiveRevocation(t *testing.T) {
	f := monitoringSetup(t)
	service, channel, cfg := derivedChannelSetup(t, f, func(rule *dm.Rule) {
		rule.Inputs[0].SourceKinds = []string{"direct", "derived"}
		rule.Inputs[0].MetricVersionID = ""
		rule.Nodes = []dm.Node{{ID: "result", Op: "formula", Params: dm.NodeParams{Expression: "x * 2", Variables: map[string]string{"x": "input:x"}}}}
		rule.Outputs[0].SemanticKey = "double_value"
	})
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	batch := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,bad\n")))
	scope := dm.TenantScope(f.tenant)
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "八层正式来源与递归失效验证"}
	}
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	from, to := strconv.FormatInt(start.UnixNano(), 10), strconv.FormatInt(start.Add(3*time.Minute).UnixNano(), 10)
	output := dm.NanosecondRange{From: from, To: &to}
	var generation string
	channels := []string{}
	for level := 1; level <= 8; level++ {
		if level > 1 {
			created, e := service.CreateChannel(ctx, f.actor, scope, dm.CreateChannelInput{WriteMeta: meta(""), Code: fmt.Sprintf("LAYER_%d", level), Name: fmt.Sprintf("派生第 %d 层", level), Owner: dm.Owner{Kind: res.Point, ID: channel.Resource.PointID, FacilityID: channel.Resource.FacilityID}, DefinitionVersionID: cfg.DefinitionVersionID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: channel.Resource.ID}}, Parameters: map[string]string{}}, access.Meta{})
			must(t, e)
			channel = created.Channel
			configs, e := service.ChannelConfigs(ctx, f.actor, scope, channel.Resource.ID, dm.MetricFilter{})
			must(t, e)
			cfg = configs.Items[0]
		}
		channels = append(channels, channel.Resource.ID)
		trial, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: meta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: output}, access.Meta{})
		must(t, e)
		if trial.Job.State != "succeeded" {
			_, e = f.service.RunTask(ctx, "chain-trial", f.tenant, trial.Job.ID)
			must(t, e)
			trial, e = service.Run(ctx, f.actor, scope, trial.ID)
			must(t, e)
		}
		if trial.Trial == nil || trial.Trial.InputCounts.Rows != 3 || len(trial.Trial.Examples) != 3 {
			t.Fatalf("layer %d trial: %+v", level, trial)
		}
		if level > 1 && (len(trial.Trial.InputExamples) != 0 || len(trial.Trial.DerivedInputExamples) != 3 || len(trial.Trial.Sources) != 1) {
			t.Fatal("derived inputs were presented as raw or lost provenance", level, trial.Trial)
		}
		want := fmt.Sprintf("%d.000000000000000000", 2*(1<<level))
		if trial.Trial.Examples[0].Value == nil || *trial.Trial.Examples[0].Value != want || trial.Trial.Examples[2].Value != nil {
			t.Fatal("layer value/null propagation", level, trial.Trial.Examples)
		}
		preview, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
		must(t, e)
		if len(preview.InputRanges) != level {
			t.Fatal("mandate omitted recursive inputs", level, preview.InputRanges)
		}
		_, e = service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
		must(t, e)
		queued, e := service.ScheduleTenant(ctx, f.tenant)
		must(t, e)
		if queued != 1 {
			t.Fatal("chain scheduling", level, queued)
		}
		var job string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND channel_id=$2 AND mode='automatic' ORDER BY created_at DESC LIMIT 1`, f.tenant, channel.Resource.ID).Scan(&job))
		_, e = f.service.RunTask(ctx, "chain-formal", f.tenant, job)
		must(t, e)
		var state, code string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT state,COALESCE(error_code,'') FROM monitoring_jobs WHERE id=$1`, job).Scan(&state, &code))
		if state != "succeeded" {
			t.Fatal("chain publication", level, state, code)
		}
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&generation))
	}
	// Creation enforces the same depth bound as execution and recursive proofs.
	_, e := service.CreateChannel(ctx, f.actor, scope, dm.CreateChannelInput{WriteMeta: meta(""), Code: "LAYER_9", Name: "超过八层", Owner: dm.Owner{Kind: res.Point, ID: channel.Resource.PointID, FacilityID: channel.Resource.FacilityID}, DefinitionVersionID: cfg.DefinitionVersionID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: channel.Resource.ID}}}, access.Meta{})
	if e == nil {
		t.Fatal("ninth derived layer accepted")
	}
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	fromText, toText := start.Format(time.RFC3339Nano), start.Add(3*time.Minute).Format(time.RFC3339Nano)
	query := md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Limit: 10}
	page, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 3 || len(page.Coverage) != 1 || page.Coverage[0].State != "current" {
		t.Fatal("eight-layer current read", page)
	}
	manifest := f.completeExport(t, md.ExportInput{WriteMeta: f.meta("0"), Query: query, Format: "csv", Kind: "details"})
	if manifest.Rows != "3" {
		t.Fatal(manifest)
	}
	files := []string{manifest.ID}
	download, e := f.service.ExportDownload(ctx, f.actor, f.scope, files[0], "1")
	must(t, e)
	// Raw revocation must be observed before the asynchronous change consumer.
	revoke, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: revoke.Hash})
	must(t, e)
	page, e = f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 0 || len(page.Coverage) != 1 || page.Coverage[0].State != "pending" {
		t.Fatal("recursive leaf revocation remained current", page)
	}
	var outputBytes bytes.Buffer
	if e = download.Write(ctx, &outputBytes); e == nil || outputBytes.Len() != 0 {
		t.Fatal("frozen recursive export bypassed leaf revocation", e, outputBytes.Len())
	}
	history, e := service.GenerationDetails(ctx, f.actor, scope, generation, dm.GenerationQuery{})
	must(t, e)
	if len(history.Items) != 3 || history.Generation.InputState != "revoked" {
		t.Fatal("recursive historical evidence", history)
	}
	lineage, e := service.GenerationLineage(ctx, f.actor, scope, generation, dm.GenerationQuery{})
	must(t, e)
	if len(lineage.Sources) != 1 {
		t.Fatal("upstream lineage missing", lineage)
	}
	replay, e := service.ReplayGeneration(ctx, f.actor, scope, generation, dm.ReplayInput{WriteMeta: meta("")}, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "chain-replay", f.tenant, replay.Job.ID)
	must(t, e)
	replay, e = service.Run(ctx, f.actor, scope, replay.ID)
	must(t, e)
	if replay.Replay == nil || !replay.Replay.Matched || replay.Replay.Actual != 3 || replay.Replay.InputState != "revoked" {
		t.Fatal("fixed upstream replay changed original results", replay)
	}
	values := []string{"2", "5", "9"}
	for level := 1; level <= 7; level++ {
		values = append(values, fmt.Sprint(2*(1<<level)), fmt.Sprint(5*(1<<level)), "")
	}
	verifyGenerationInputEvidence(t, f, service, generation, values)
	var generations int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_generations WHERE tenant_id=$1`, f.tenant).Scan(&generations))
	if generations != 8 {
		t.Fatal("replay modified formal heads", generations)
	}
	partEnd := strconv.FormatInt(start.Add(time.Minute).UnixNano(), 10)
	verifyRecursiveProjectHistory(t, f, service, channels, generation, dm.NanosecondRange{From: from, To: &partEnd})
	// Source changes fan out immediately, but only the earliest eligible layer
	// is dispatched. Later layers wait for complete formal coverage to advance.
	for level := 1; level <= 8; level++ {
		_, e = service.ConsumeChanges(ctx, f.tenant, time.Now())
		must(t, e)
		queued, e := service.ScheduleTenant(ctx, f.tenant)
		must(t, e)
		if queued != 1 {
			t.Fatal("pending ancestors were not scheduled in dependency order", level, queued)
		}
		var channelID, job string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT r.channel_id::text,r.job_id::text FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state='queued'`, f.tenant).Scan(&channelID, &job))
		if channelID != channels[level-1] {
			t.Fatal("later layer dispatched before pending ancestor", level, channelID)
		}
		_, e = f.service.RunTask(ctx, "chain-revocation", f.tenant, job)
		must(t, e)
	}
	selection, e = f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	query.Selection = selection.ID
	page, e = f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal("revoked values remained in recomputed current", page)
	}
	for _, part := range page.Coverage {
		if part.State != "current" {
			t.Fatal("complete empty upstream coverage treated as missing", part)
		}
	}
}
