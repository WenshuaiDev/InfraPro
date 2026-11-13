//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
)

func TestAnalysisNearestAndFutureFixedBasisRetainEvidence(t *testing.T) {
	f := monitoringSetup(t)
	s, _, cfg := derivedChannelSetup(t, f)
	s.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:30Z,2,good\n2024-01-01T00:01:30Z,5,good\n")))
	basis := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:02:30Z,9,good\n")))
	scope := dm.TenantScope(f.tenant)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := func(minutes int) string {
		return strconv.FormatInt(start.Add(time.Duration(minutes)*time.Minute).UnixNano(), 10)
	}
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "历史非因果依据与基准失效验证"}
	}
	var rule dm.Rule
	must(t, json.Unmarshal(cfg.Rule, &rule))
	publish := func(code string, r dm.Rule, bindings map[string]dm.BaselineBinding) dm.AnalysisVersion {
		t.Helper()
		raw, e := json.Marshal(r)
		must(t, e)
		created, e := s.CreateAnalysis(ctx, f.actor, scope, dm.CreateAnalysisInput{WriteMeta: meta(""), Code: code, Name: code, Body: dm.AnalysisBody{Rule: raw, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters, BaselineBindings: bindings}}, access.Meta{})
		must(t, e)
		validation, e := s.ValidateAnalysis(ctx, f.actor, scope, created.Config.ID, dm.ValidateInput{DraftRevision: created.Config.Draft.Revision})
		must(t, e)
		published, e := s.PublishAnalysis(ctx, f.actor, scope, created.Config.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: meta(created.Config.Revision), DraftRevision: created.Config.Draft.Revision, ValidationHash: validation.ValidationHash}}, access.Meta{})
		must(t, e)
		return *published.Version
	}
	run := func(version dm.AnalysisVersion, trial bool) dm.Run {
		t.Helper()
		to := ns(2)
		r, e := s.StartAnalysis(ctx, f.actor, scope, dm.AnalysisRunInput{WriteMeta: meta(""), ConfigVersionID: version.ID, ConfigHash: version.ConfigHash, OutputRange: dm.NanosecondRange{From: ns(0), To: &to}}, trial, access.Meta{})
		must(t, e)
		_, e = f.service.RunTask(ctx, "analysis-causality", f.tenant, r.Job.ID)
		must(t, e)
		r, e = s.Run(ctx, f.actor, scope, r.ID)
		must(t, e)
		return r
	}
	rule.Clock = dm.Clock{Kind: "fixed_grid", AnchorNS: ns(0), StepNS: "60000000000"}
	rule.Nodes = []dm.Node{{ID: "nearest", Op: "formula", Params: dm.NodeParams{Expression: "x * 2", Variables: map[string]string{"x": "input:x"}, Alignment: "nearest", ToleranceNS: "30000000000"}}}
	rule.Outputs[0].Node = "nearest"
	rule.Outputs[0].SemanticKey = "nearest_value"
	rule.Outputs[0].Quantity = nil
	nearest := publish("NEAREST", rule, nil)
	trial := run(nearest, true)
	if trial.Trial == nil || len(trial.Trial.Examples) != 2 {
		t.Fatal(trial)
	}
	first, second := trial.Trial.Examples[0], trial.Trial.Examples[1]
	if first.Value == nil || *first.Value != "4.000000000000000000" || !slices.Contains(first.Reasons, "noncausal_alignment") || len(first.Matches) != 1 || first.Matches[0].Offset != "30000000000" || second.Value == nil || *second.Value != "4.000000000000000000" || second.Matches[0].Offset != "-30000000000" {
		t.Fatal("nearest direction or equal-distance tie lost", first, second)
	}
	rule.Clock = dm.Clock{Kind: "primary_input", Input: "x"}
	rule.Nodes = []dm.Node{{ID: "baseline", Op: "baseline_delta", Inputs: []string{"input:x"}, Params: dm.NodeParams{Baseline: &dm.Baseline{Kind: "reference_window", Reference: "origin", Method: "mean", FromNS: ns(2), ToNS: ns(3)}}}}
	rule.Outputs[0].Node = "baseline"
	rule.Outputs[0].SemanticKey = "future_basis_delta"
	version := publish("FUTURE_BASIS", rule, map[string]dm.BaselineBinding{"origin": {Input: "x"}})
	baselineTrial := run(version, true)
	if baselineTrial.Trial == nil || baselineTrial.Trial.Examples[0].Value == nil || *baselineTrial.Trial.Examples[0].Value != "-7.000000000000000000" || !slices.Contains(baselineTrial.Trial.Examples[0].Reasons, "noncausal_basis") {
		t.Fatal("future fixed analysis baseline refused or not explained", baselineTrial)
	}
	formal := run(version, false)
	artifact, artifactVersion := formal.Analysis.ArtifactID, formal.Analysis.ArtifactVersionID
	rows, e := s.AnalysisArtifactRows(ctx, f.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: artifactVersion})
	must(t, e)
	if len(rows.Items) != 2 || *rows.Items[0].Value != "-7.000000000000000000" {
		t.Fatal(rows)
	}
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:02:45Z,99,good\n")))
	still := run(version, true)
	if *still.Trial.Examples[0].Value != "-7.000000000000000000" || still.Trial.Baselines[0].Snapshot != baselineTrial.Trial.Baselines[0].Snapshot {
		t.Fatal("later reference-window append replaced fixed basis", still)
	}
	preview, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, basis.ID, basis.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, basis.ID, md.RevokeInput{WriteMeta: f.meta(basis.Revision), PreviewHash: preview.Hash})
	must(t, e)
	oldTrial, e := s.Run(ctx, f.actor, scope, baselineTrial.ID)
	must(t, e)
	if oldTrial.Trial != nil || oldTrial.Job.ResultState != "input_invalidated" {
		t.Fatal("revoked fixed baseline leaked old trial", oldTrial)
	}
	detail, e := s.AnalysisArtifact(ctx, f.actor, scope, artifact, artifactVersion)
	must(t, e)
	found := false
	for _, change := range detail.Changes {
		if change.Kind == "raw_revoke" && change.Range.From == strconv.FormatInt(start.Add(150*time.Second).UnixNano(), 10) {
			found = true
		}
	}
	if detail.Version.InputState != "revoked" || !found {
		t.Fatal("out-of-range fixed baseline revoke omitted from artifact reasons", detail)
	}
	forbidAnalysisExport(t, s, f.actor, scope, artifact, artifactVersion, false)
	verifyAnalysisExports(t, f, s, f.actor, scope, artifact, artifactVersion, "2", "-7.000000000000000000", true)
}
