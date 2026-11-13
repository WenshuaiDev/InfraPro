package derivedmonitoring_test

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
)

func TestDerivedConfigurationCandidatesAssignmentsAndIntent(t *testing.T) {
	f := channelsSetup(t)
	actor := f.actor(f.admin)
	host := monitoring.New(f.e.Service.Access()).WithStorage(new(monitoring.Storage))
	f.s.WithMonitoring(host)
	must(t, f.s.RegisterTasks())
	definition := f.publish(t, f.create(t, "configurations").Definition)
	created, e := f.s.CreateChannel(ctx, actor, f.scope, dm.CreateChannelInput{WriteMeta: wm(""), Code: "CONFIG", Name: "配置分配", Owner: dm.Owner{Kind: resources.Point, FacilityID: f.facility, ID: f.point}, DefinitionVersionID: definition.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: f.direct}}, Parameters: map[string]string{}}, access.Meta{})
	must(t, e)
	channel := created.Channel
	page, e := f.s.ChannelConfigs(ctx, actor, f.scope, channel.Resource.ID, dm.MetricFilter{})
	must(t, e)
	original := page.Items[0]
	save := dm.SaveConfigInput{WriteMeta: wm(channel.ConfigRevision), BaseConfigID: original.ID, DefinitionVersionID: original.DefinitionVersionID, InputBindings: original.InputBindings, Parameters: map[string]string{}, Quality: []string{"good"}, NumericContract: &dm.NumericContract{Scale: 36, Rounding: "half_even"}, ChangeKind: "ordinary"}
	candidate, e := f.s.SaveChannelConfig(ctx, actor, f.scope, channel.Resource.ID, save, access.Meta{})
	must(t, e)
	if candidate.Channel.DraftConfigID != candidate.Config.ID || candidate.Channel.ConfigRevision != "2" || candidate.Config.OutputContractHash != original.OutputContractHash {
		t.Fatal(candidate)
	}
	replay, e := f.s.SaveChannelConfig(ctx, actor, f.scope, channel.Resource.ID, save, access.Meta{})
	must(t, e)
	if !replay.Replayed || replay.Config.ID != candidate.Config.ID {
		t.Fatal("candidate request replay", replay)
	}
	channel = candidate.Channel
	trial := func(cfg dm.ChannelConfig, from, to string) dm.Run {
		t.Helper()
		run, e := f.s.ChannelTrial(ctx, actor, f.scope, channel.Resource.ID, dm.TrialInput{WriteMeta: wm(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, access.Meta{})
		must(t, e)
		if run.Trial == nil {
			t.Fatal(run)
		}
		return run
	}
	start := "100"
	initialTrial := trial(candidate.Config, start, "400")
	enable, e := f.s.PreviewControl(ctx, actor, f.scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &start, TrialReceiptID: initialTrial.Trial.ReceiptID, AllowWaiting: true}})
	must(t, e)
	enabled, e := f.s.ControlChannel(ctx, actor, f.scope, channel.Resource.ID, dm.ControlInput{WriteMeta: wm(channel.ControlRevision), PreviewID: enable.ID, PreviewHash: enable.Hash}, access.Meta{})
	must(t, e)
	channel = enabled.Channel
	save.WriteMeta = wm(channel.ConfigRevision)
	save.BaseConfigID = candidate.Config.ID
	save.NumericContract = &dm.NumericContract{Scale: 24, Rounding: "half_even"}
	next, e := f.s.SaveChannelConfig(ctx, actor, f.scope, channel.Resource.ID, save, access.Meta{})
	must(t, e)
	if next.Channel.DraftConfigID != candidate.Config.ID || next.Channel.ConfigRevision != channel.ConfigRevision {
		t.Fatal("saving candidate changed live configuration", next)
	}
	trialNext := trial(next.Config, "200", "400")
	end := "300"
	proposal := dm.ConfigPreviewInput{Revision: channel.ConfigRevision, ConfigProposal: dm.ConfigProposal{ConfigID: next.Config.ID, Mode: "ordinary", Range: dm.NanosecondRange{From: "200"}, TrialReceiptID: trialNext.Trial.ReceiptID, AllowWaiting: true}}
	_, e = f.s.PreviewConfig(ctx, actor, f.scope, channel.Resource.ID, proposal)
	code(t, e, "invalid_rule")
	proposal.Mode = "correction"
	proposal.Range.To = &end
	preview, e := f.s.PreviewConfig(ctx, actor, f.scope, channel.Resource.ID, proposal)
	must(t, e)
	if len(preview.Assignments) != 1 || preview.Assignments[0].ConfigID != candidate.Config.ID {
		t.Fatal(preview)
	}
	apply := dm.ControlInput{WriteMeta: wm(channel.ConfigRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}
	applied, e := f.s.ApplyConfig(ctx, actor, f.scope, channel.Resource.ID, apply, access.Meta{})
	must(t, e)
	previousRevision := channel.ConfigRevision
	channel = applied.Channel
	repeated, e := f.s.ApplyConfig(ctx, actor, f.scope, channel.Resource.ID, apply, access.Meta{})
	must(t, e)
	if !repeated.Replayed {
		t.Fatal(repeated)
	}
	assignments, e := f.s.ChannelAssignments(ctx, actor, f.scope, channel.Resource.ID, "", "")
	must(t, e)
	if len(assignments.Items) != 3 {
		t.Fatal("correction did not preserve both outside intervals", assignments)
	}
	old, e := f.s.ChannelAssignments(ctx, actor, f.scope, channel.Resource.ID, previousRevision, "")
	must(t, e)
	if len(old.Items) != 1 || old.Items[0].Range.From != "100" || old.Items[0].Range.To != nil || old.Items[0].ConfigID != candidate.Config.ID {
		t.Fatal("old assignment graph not reconstructible", old)
	}
	if _, e = f.e.Pool.Exec(ctx, `UPDATE derived_config_assignments SET effective_range=int8range(100,999,'[)') WHERE id=$1`, old.Items[0].ID); e == nil {
		t.Fatal("historical assignment mutated")
	}
	_, e = f.s.ApplyConfig(ctx, actor, f.scope, channel.Resource.ID, dm.ControlInput{WriteMeta: wm(previousRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	code(t, e, "revision_conflict")
	// A future boundary uses the new configuration exactly at that nanosecond.
	future := strconv.FormatInt(time.Now().Add(time.Hour).UnixNano(), 10)
	toFuture := strconv.FormatInt(time.Now().Add(2*time.Hour).UnixNano(), 10)
	futureTrial := trial(next.Config, future, toFuture)
	ordinary, e := f.s.PreviewConfig(ctx, actor, f.scope, channel.Resource.ID, dm.ConfigPreviewInput{Revision: channel.ConfigRevision, ConfigProposal: dm.ConfigProposal{Mode: "ordinary", ConfigID: next.Config.ID, Range: dm.NanosecondRange{From: future}, TrialReceiptID: futureTrial.Trial.ReceiptID, AllowWaiting: true}})
	must(t, e)
	applied, e = f.s.ApplyConfig(ctx, actor, f.scope, channel.Resource.ID, dm.ControlInput{WriteMeta: wm(channel.ConfigRevision), PreviewID: ordinary.ID, PreviewHash: ordinary.Hash}, access.Meta{})
	must(t, e)
	channel = applied.Channel
	if channel.DraftConfigID != next.Config.ID {
		t.Fatal(channel)
	}
	var atBoundary, beforeBoundary string
	at, _ := strconv.ParseInt(future, 10, 64)
	must(t, f.e.Pool.QueryRow(ctx, `SELECT config_id::text FROM derived_config_assignments WHERE tenant_id=$1 AND channel_id=$2 AND superseded_revision IS NULL AND effective_range @> $3::bigint`, f.scope.Tenant, channel.Resource.ID, at).Scan(&atBoundary))
	must(t, f.e.Pool.QueryRow(ctx, `SELECT config_id::text FROM derived_config_assignments WHERE tenant_id=$1 AND channel_id=$2 AND superseded_revision IS NULL AND effective_range @> $3::bigint`, f.scope.Tenant, channel.Resource.ID, at-1).Scan(&beforeBoundary))
	if atBoundary != next.Config.ID || beforeBoundary != candidate.Config.ID {
		t.Fatal("half-open boundary", beforeBoundary, atBoundary)
	}
	// New business meaning remains a new channel even if the name is unchanged.
	var rule dm.Rule
	must(t, json.Unmarshal(f.rule, &rule))
	rule.Outputs[0].SemanticKey = "a_different_business_quantity"
	raw, e := json.Marshal(rule)
	must(t, e)
	draft, e := f.s.SaveMetricDraft(ctx, actor, f.scope, definition.Definition.ID, dm.SaveMetricInput{WriteMeta: wm(definition.Definition.Revision), Name: definition.Definition.Name, Rule: raw}, access.Meta{})
	must(t, e)
	version := f.publish(t, draft.Definition)
	save.WriteMeta = wm(channel.ConfigRevision)
	save.DefinitionVersionID = version.Version.ID
	_, e = f.s.SaveChannelConfig(ctx, actor, f.scope, channel.Resource.ID, save, access.Meta{})
	code(t, e, "new_channel_required")
	save.RequestID = uuid.NewString()
	save.ChangeKind = "correction"
	save.SemanticIntentConfirmed = true
	_, e = f.s.SaveChannelConfig(ctx, actor, f.scope, channel.Resource.ID, save, access.Meta{})
	code(t, e, "new_channel_required")
}
