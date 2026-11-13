//go:build monitoringintegration

package resources_test

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
)

func TestDerivedExplicitDependencyBackfillWaitsForSelectedUpstream(t *testing.T) {
	f := monitoringSetup(t)
	service, root, cfg := derivedChannelSetup(t, f, func(rule *dm.Rule) {
		rule.Inputs[0].SourceKinds = []string{"direct", "derived"}
		rule.Inputs[0].MetricVersionID = ""
		rule.Nodes = []dm.Node{{ID: "result", Op: "formula", Params: dm.NodeParams{Expression: "x * 2", Variables: map[string]string{"x": "input:x"}}}}
		rule.Outputs[0].SemanticKey = "double_value"
	})
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:01:00Z,2,good\n2024-01-01T00:02:00Z,5,good\n2024-01-01T00:03:00Z,9,good\n")))
	scope := dm.TenantScope(f.tenant)
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "明确选择上游历史回算"}
	}
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := func(m int) string { return strconv.FormatInt(start.Add(time.Duration(m)*time.Minute).UnixNano(), 10) }
	end := ns(4)
	enable := func(channel dm.Channel, config dm.ChannelConfig, from string) dm.Channel {
		t.Helper()
		trial, e := service.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: meta(channel.ConfigRevision), ConfigID: config.ID, ConfigHash: config.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &end}}, access.Meta{})
		must(t, e)
		if trial.Trial == nil {
			t.Fatal(trial)
		}
		preview, e := service.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &end, TrialReceiptID: trial.Trial.ReceiptID}})
		must(t, e)
		result, e := service.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: meta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
		must(t, e)
		_, e = service.ScheduleTenant(ctx, f.tenant)
		must(t, e)
		var job string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND channel_id=$2 AND mode='automatic'`, f.tenant, channel.Resource.ID).Scan(&job))
		_, e = f.service.RunTask(ctx, "dependency-auto", f.tenant, job)
		must(t, e)
		return result.Channel
	}
	root = enable(root, cfg, ns(2))
	created, e := service.CreateChannel(ctx, f.actor, scope, dm.CreateChannelInput{WriteMeta: meta(""), Code: "CHILD", Name: "依赖回算子通道", Owner: dm.Owner{Kind: res.Point, ID: root.Resource.PointID, FacilityID: root.Resource.FacilityID}, DefinitionVersionID: cfg.DefinitionVersionID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: root.Resource.ID}}}, access.Meta{})
	must(t, e)
	configs, e := service.ChannelConfigs(ctx, f.actor, scope, created.Channel.Resource.ID, dm.MetricFilter{})
	must(t, e)
	childConfig := configs.Items[0]
	child := enable(created.Channel, childConfig, ns(3))
	to := ns(2)
	history := dm.NanosecondRange{From: ns(1), To: &to}
	preview, e := service.PreviewBackfill(ctx, f.actor, scope, child.Resource.ID, dm.BackfillPreviewInput{Revision: child.ConfigRevision, Range: history, UnassignedConfigID: childConfig.ID, Dependencies: []dm.BackfillDependencySelection{{ChannelID: root.Resource.ID, Revision: root.ConfigRevision, Range: history, UnassignedConfigID: cfg.ID}}})
	must(t, e)
	if len(preview.Dependencies) != 1 || !preview.Dependencies[0].Targets[0].Assign || !preview.Targets[0].Assign {
		t.Fatal(preview)
	}
	input := dm.ControlInput{WriteMeta: meta(child.ConfigRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}
	result, e := service.BackfillChannel(ctx, f.actor, scope, child.Resource.ID, input, access.Meta{})
	must(t, e)
	if len(result.Runs) != 2 || result.Runs[0].ChannelID != root.Resource.ID || result.Runs[1].ChannelID != child.Resource.ID {
		t.Fatal(result)
	}
	again, e := service.BackfillChannel(ctx, f.actor, scope, child.Resource.ID, input, access.Meta{})
	must(t, e)
	if len(again.Runs) != 2 || again.Runs[0].ID != result.Runs[0].ID {
		t.Fatal("idempotent group duplicated", again)
	}
	parent := result.Runs[1]
	_, e = f.service.RunTask(ctx, "dependency-parent-early", f.tenant, parent.Job.ID)
	var waiting *md.TaskError
	if !errors.As(e, &waiting) || waiting.Code != "dependency_pending" || waiting.Disposition != "waiting" {
		t.Fatal("missing explicit dependency wait", e)
	}
	var state, code string
	var attempts, snapshots int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT state,COALESCE(error_code,''),attempts FROM monitoring_jobs WHERE id=$1`, parent.Job.ID).Scan(&state, &code, &attempts))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2`, f.tenant, parent.ID).Scan(&snapshots))
	if state != "retry_wait" || code != "dependency_pending" || attempts != 0 || snapshots != 0 {
		t.Fatal("parent captured incomplete input or spent retry budget", state, code, attempts, snapshots)
	}
	// Waiting is an execution state; current authorization still permits inspection.
	_, e = service.Run(ctx, f.actor, scope, parent.ID)
	must(t, e)
	_, e = f.service.RunTask(ctx, "dependency-root", f.tenant, result.Runs[0].Job.ID)
	must(t, e)
	_, e = f.service.RunTask(ctx, "dependency-parent-ready", f.tenant, parent.Job.ID)
	must(t, e)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT state,COALESCE(error_code,'') FROM monitoring_jobs WHERE id=$1`, parent.Job.ID).Scan(&state, &code))
	if state != "succeeded" {
		t.Fatal("parent failed after upstream publication", state, code)
	}
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{child.Resource.ID}})
	must(t, e)
	fromText, toText := start.Add(time.Minute).Format(time.RFC3339Nano), start.Add(2*time.Minute).Format(time.RFC3339Nano)
	page, e := f.service.Details(ctx, f.actor, f.scope, md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &fromText, To: &toText}, Limit: 10})
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].Calculated.Value == nil || *page.Items[0].Calculated.Value != "8.000000000000000000" {
		t.Fatal(page)
	}
	for _, expected := range []struct{ id, from string }{{root.Resource.ID, ns(2)}, {child.Resource.ID, ns(3)}} {
		channel, e := service.Channel(ctx, f.actor, scope, expected.id)
		must(t, e)
		if channel.AutoStartNS == nil || *channel.AutoStartNS != expected.from || channel.ControlState != "enabled" {
			t.Fatal("manual dependency backfill changed automatic control", channel)
		}
	}
}
