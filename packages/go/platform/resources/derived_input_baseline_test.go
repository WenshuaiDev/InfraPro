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

func TestDerivedPublishedBaselineKeepsOriginalReferenceAndRevocation(t *testing.T) {
	f := monitoringSetup(t)
	service, upstream, cfg := derivedChannelSetup(t, f, func(rule *dm.Rule) {
		rule.Inputs[0].MetricVersionID = ""
		rule.Inputs[0].SourceKinds = []string{"direct", "derived"}
		rule.Nodes = []dm.Node{{ID: "result", Op: "formula", Params: dm.NodeParams{Expression: "x * 2", Variables: map[string]string{"x": "input:x"}}}}
		rule.Outputs[0].SemanticKey = "double_value"
	})
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	basis := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n")))
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:02:00Z,9,good\n")))
	scope := dm.TenantScope(f.tenant)
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "派生固定基准证据验证"}
	}
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	from, to := strconv.FormatInt(start.UnixNano(), 10), strconv.FormatInt(start.Add(3*time.Minute).UnixNano(), 10)
	trial, e := service.ChannelTrial(ctx, f.actor, scope, upstream.Resource.ID, dm.TrialInput{WriteMeta: meta(upstream.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, access.Meta{})
	must(t, e)
	if trial.Trial == nil {
		t.Fatal(trial)
	}
	preview, e := service.PreviewControl(ctx, f.actor, scope, upstream.Resource.ID, dm.ControlPreviewInput{Revision: upstream.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	_, e = service.ControlChannel(ctx, f.actor, scope, upstream.Resource.ID, dm.ControlInput{WriteMeta: meta(upstream.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	_, e = service.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	var job, generation string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND channel_id=$2 AND mode='automatic'`, f.tenant, upstream.Resource.ID).Scan(&job))
	_, e = f.service.RunTask(ctx, "baseline-upstream", f.tenant, job)
	must(t, e)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, upstream.Resource.ID).Scan(&generation))
	original, e := service.GenerationDetails(ctx, f.actor, scope, generation, dm.GenerationQuery{})
	must(t, e)
	if len(original.Items) != 3 {
		t.Fatal(original)
	}
	endBasis := strconv.FormatInt(start.Add(2*time.Minute).UnixNano(), 10)
	type target struct {
		channel dm.Channel
		config  dm.ChannelConfig
		want    string
	}
	targets := []target{}
	for _, kind := range []string{"observation", "reference_window"} {
		var rule dm.Rule
		must(t, json.Unmarshal(cfg.Rule, &rule))
		baseline := &dm.Baseline{Kind: kind, Reference: "b"}
		binding := dm.BaselineBinding{Input: "x"}
		want := "14.000000000000000000"
		if kind == "observation" {
			binding.ResultID = original.Items[0].Result
			binding.GenerationID = generation
			binding.AtNS = from
		} else {
			baseline.FromNS = from
			baseline.ToNS = endBasis
			baseline.Method = "mean"
			want = "11.000000000000000000"
		}
		rule.Nodes = []dm.Node{{ID: "result", Op: "baseline_delta", Inputs: []string{"input:x"}, Params: dm.NodeParams{Baseline: baseline}}}
		rule.Outputs[0].SemanticKey = "delta_" + kind
		rule.Outputs[0].Quantity = nil
		raw, e := json.Marshal(rule)
		must(t, e)
		definition, e := service.CreateMetric(ctx, f.actor, scope, dm.CreateMetricInput{WriteMeta: meta(""), Code: "BASE_" + kind, Name: "固定派生基准 " + kind, Rule: raw}, access.Meta{})
		must(t, e)
		validation, e := service.ValidateMetric(ctx, f.actor, scope, definition.Definition.ID, dm.ValidateInput{DraftRevision: definition.Definition.Draft.Revision})
		must(t, e)
		version, e := service.PublishMetric(ctx, f.actor, scope, definition.Definition.ID, dm.PublishMetricInput{WriteMeta: meta(definition.Definition.Revision), DraftRevision: definition.Definition.Draft.Revision, ValidationHash: validation.ValidationHash}, access.Meta{})
		must(t, e)
		result, e := service.CreateChannel(ctx, f.actor, scope, dm.CreateChannelInput{WriteMeta: meta(""), Code: "BASE_" + kind, Name: "固定基准通道 " + kind, Owner: dm.Owner{Kind: res.Point, ID: upstream.Resource.PointID, FacilityID: upstream.Resource.FacilityID}, DefinitionVersionID: version.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: upstream.Resource.ID}}, BaselineBindings: map[string]dm.BaselineBinding{"b": binding}}, access.Meta{})
		must(t, e)
		configs, e := service.ChannelConfigs(ctx, f.actor, scope, result.Channel.Resource.ID, dm.MetricFilter{})
		must(t, e)
		targets = append(targets, target{result.Channel, configs.Items[0], want})
	}
	check := func(invalid bool) {
		t.Helper()
		for _, target := range targets {
			run, e := service.ChannelTrial(ctx, f.actor, scope, target.channel.Resource.ID, dm.TrialInput{WriteMeta: meta(target.channel.ConfigRevision), ConfigID: target.config.ID, ConfigHash: target.config.ConfigHash, OutputRange: dm.NanosecondRange{From: endBasis, To: &to}}, access.Meta{})
			must(t, e)
			if run.Job.State != "succeeded" {
				_, e = f.service.RunTask(ctx, "derived-baseline-trial", f.tenant, run.Job.ID)
				must(t, e)
				run, e = service.Run(ctx, f.actor, scope, run.ID)
				must(t, e)
			}
			if run.Trial == nil || len(run.Trial.Examples) != 1 {
				t.Fatal(run)
			}
			value := run.Trial.Examples[0]
			if invalid {
				if value.Status != "baseline_invalid" || value.Value != nil {
					t.Fatal("revoked original basis revived", run.Trial)
				}
			} else if value.Value == nil || *value.Value != target.want {
				t.Fatal("fixed derived basis changed", run.Trial)
			}
		}
	}
	check(false)
	// A new point inside the reference interval must not enter either original
	// fixed baseline, even though it makes that upstream interval pending.
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:30Z,3,good\n")))
	check(false)
	revoke, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, basis.ID, basis.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, basis.ID, md.RevokeInput{WriteMeta: f.meta(basis.Revision), PreviewHash: revoke.Hash})
	must(t, e)
	check(true)
}
