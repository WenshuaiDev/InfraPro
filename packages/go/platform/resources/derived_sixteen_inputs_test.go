//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func createRuleChannel(t *testing.T, f monitoringFixture, s *dm.Service, code string, rule dm.Rule, bindings map[string]dm.InputBinding) (dm.Channel, dm.ChannelConfig) {
	t.Helper()
	scope := dm.TenantScope(f.tenant)
	raw, e := json.Marshal(rule)
	must(t, e)
	d, e := s.CreateMetric(ctx, f.actor, scope, dm.CreateMetricInput{WriteMeta: restoreMeta(""), Code: code, Name: code, Rule: raw}, access.Meta{})
	must(t, e)
	v, e := s.ValidateMetric(ctx, f.actor, scope, d.Definition.ID, dm.ValidateInput{DraftRevision: d.Definition.Draft.Revision})
	must(t, e)
	p, e := s.PublishMetric(ctx, f.actor, scope, d.Definition.ID, dm.PublishMetricInput{WriteMeta: restoreMeta(d.Definition.Revision), DraftRevision: d.Definition.Draft.Revision, ValidationHash: v.ValidationHash}, access.Meta{})
	must(t, e)
	var facility, point string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text,point_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility, &point))
	c, e := s.CreateChannel(ctx, f.actor, scope, dm.CreateChannelInput{WriteMeta: restoreMeta(""), Code: code, Name: code, Owner: dm.Owner{Kind: res.Point, ID: point, FacilityID: facility}, DefinitionVersionID: p.Version.ID, InputBindings: bindings, Parameters: map[string]string{}}, access.Meta{})
	must(t, e)
	configs, e := s.ChannelConfigs(ctx, f.actor, scope, c.Channel.Resource.ID, dm.MetricFilter{})
	must(t, e)
	return c.Channel, configs.Items[0]
}

func createInputChannels(t *testing.T, f monitoringFixture, count int) []string {
	t.Helper()
	ids := []string{f.channel}
	var facility, point string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text,point_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility, &point))
	for i := 1; i < count; i++ {
		c, e := f.write(t, res.Target{Kind: res.Channel, Facility: facility, Point: point}, "create", map[string]any{"code": fmt.Sprintf("SOURCE_%02d", i), "name": fmt.Sprintf("独立输入 %d", i), "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
		must(t, e)
		ids = append(ids, c.ID)
	}
	return ids
}

func TestDerivedSixteenDistinctInputsFormalAndReplay(t *testing.T) {
	f := monitoringSetup(t)
	t.Cleanup(func() { verifyTerminalNotifications(t, f.env) })
	s, _, cfg := derivedChannelSetup(t, f)
	s.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	inputs := createInputChannels(t, f, 16)
	var rule dm.Rule
	must(t, json.Unmarshal(cfg.Rule, &rule))
	input := rule.Inputs[0]
	rule.Inputs = nil
	bindings := map[string]dm.InputBinding{}
	refs := []string{}
	mapping := md.Mapping{Format: "csv", Mode: "wide", Encoding: "utf-8", Delimiter: ",", HeaderRow: 1, TimeColumn: "observed_at", QualityColumn: "quality", Timezone: "UTC", EmptyWide: "error"}
	var csv strings.Builder
	csv.WriteString("observed_at,quality")
	for i, id := range inputs {
		key := fmt.Sprintf("x%02d", i)
		in := input
		in.Key = key
		rule.Inputs = append(rule.Inputs, in)
		refs = append(refs, "input:"+key)
		bindings[key] = dm.InputBinding{ChannelID: id}
		mapping.Channels = append(mapping.Channels, md.ChannelMapping{Alias: key, Channel: id, ValueColumn: key, Unit: "mm"})
		fmt.Fprintf(&csv, ",%s", key)
	}
	csv.WriteByte('\n')
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&csv, "%s,good", base.Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano))
		for j := 0; j < 16; j++ {
			fmt.Fprintf(&csv, ",%d", i+j)
		}
		csv.WriteByte('\n')
	}
	f.publish(t, f.validate(t, f.uploadBytes(t, "sixteen.csv", []byte(csv.String()), mapping)))
	rule.Clock.Input = "x00"
	rule.Nodes = []dm.Node{{ID: "result", Op: "add", Inputs: refs, Params: dm.NodeParams{Alignment: "exact"}}}
	rule.Outputs[0].SemanticKey, rule.Outputs[0].Quantity = "sixteen_sum", nil
	channel, config := createRuleChannel(t, f, s, "SIXTEEN", rule, bindings)
	from, to := "1704067200000000000", "1704067203000000000"
	scope := dm.TenantScope(f.tenant)
	interval := dm.NanosecondRange{From: from, To: &to}
	trial, e := s.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(channel.ConfigRevision), ConfigID: config.ID, ConfigHash: config.ConfigHash, OutputRange: interval}, access.Meta{})
	must(t, e)
	if trial.Job.State != "succeeded" {
		_, e = f.service.RunTask(ctx, "sixteen-trial", f.tenant, trial.Job.ID)
		must(t, e)
		trial, e = s.Run(ctx, f.actor, scope, trial.ID)
		must(t, e)
	}
	if trial.Trial == nil || trial.Trial.InputCounts.Rows != 48 || trial.Trial.OutputCount != "3" {
		t.Fatal("sixteen inputs were not all read", trial)
	}
	preview, e := s.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	_, e = s.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	_, e = s.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	var job, generation string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND channel_id=$2 AND mode='automatic'`, f.tenant, channel.Resource.ID).Scan(&job))
	_, e = f.service.RunTask(ctx, "sixteen-formal", f.tenant, job)
	must(t, e)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2`, f.tenant, channel.Resource.ID).Scan(&generation))
	page, e := s.GenerationDetails(ctx, f.actor, scope, generation, dm.GenerationQuery{})
	must(t, e)
	if len(page.Items) != 3 {
		t.Fatal(page)
	}
	for i, row := range page.Items {
		if row.Value == nil || *row.Value != fmt.Sprintf("%d.000000000000000000", 120+16*i) {
			t.Fatal("sixteen-input exact sum differs", row)
		}
	}
	replay, e := s.ReplayGeneration(ctx, f.actor, scope, generation, dm.ReplayInput{WriteMeta: restoreMeta("")}, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "sixteen-replay", f.tenant, replay.Job.ID)
	must(t, e)
	replay, e = s.Run(ctx, f.actor, scope, replay.ID)
	must(t, e)
	if replay.Replay == nil || !replay.Replay.Matched || replay.Replay.Actual != 3 {
		t.Fatal(replay)
	}
}
