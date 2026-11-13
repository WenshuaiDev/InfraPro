//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type derivedRestoreProof struct {
	service                   *dm.Service
	channels                  []string
	original                  []string
	generations               []string
	rows                      map[string][]md.CalculatedObservation
	artifact, artifactVersion string
	artifactRows              []dm.AnalysisArtifactRow
}

func restoreMeta(rev string) dm.WriteMeta {
	return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "独立派生备份恢复验证"}
}

// Build the chain before revoking A: the baseline belongs to A, while the
// output timestamps belong to B. Recovery must replay the original baseline
// and also retain later baseline_invalid results, without reviving A.
func prepareDerivedRestore(t *testing.T, f monitoringFixture, base time.Time) *derivedRestoreProof {
	t.Helper()
	ns := func(n int) string { return strconv.FormatInt(base.Add(time.Duration(n)).UnixNano(), 10) }
	s, channel, cfg := derivedChannelSetupBindings(t, f, map[string]dm.BaselineBinding{"origin": {Input: "x"}}, func(rule *dm.Rule) {
		rule.Nodes = []dm.Node{{ID: "result", Op: "baseline_delta", Inputs: []string{"input:x"}, Params: dm.NodeParams{Baseline: &dm.Baseline{Kind: "reference_window", Reference: "origin", Method: "mean", FromNS: ns(20), ToNS: ns(25)}}}}
		rule.Outputs[0].SemanticKey = "recovery_baseline"
	})
	s.WithStorage(f.storage)
	f.srv.WithExternalReferences(s)
	must(t, dm.InitializeStorage(ctx, f.storage))
	p := &derivedRestoreProof{service: s, rows: map[string][]md.CalculatedObservation{}}
	scope := dm.TenantScope(f.tenant)
	from, to := ns(100), ns(120)
	for level := 0; level < 2; level++ {
		if level == 1 {
			var rule dm.Rule
			must(t, json.Unmarshal(cfg.Rule, &rule))
			must(t, json.Unmarshal(channel.Resource.QuantitySpec, &rule.Inputs[0].Quantity))
			rule.Inputs[0].SourceKinds = []string{"derived"}
			rule.Inputs[0].MetricVersionID = ""
			rule.Nodes = []dm.Node{{ID: "result", Op: "formula", Params: dm.NodeParams{Expression: "x * 2", Variables: map[string]string{"x": "input:x"}}}}
			rule.Outputs[0].Quantity = nil
			rule.Outputs[0].SemanticKey = "recovery_chain"
			raw, e := json.Marshal(rule)
			must(t, e)
			d, e := s.CreateMetric(ctx, f.actor, scope, dm.CreateMetricInput{WriteMeta: restoreMeta(""), Code: "RESTORE_CHAIN", Name: "恢复多级来源", Rule: raw}, access.Meta{})
			must(t, e)
			v, e := s.ValidateMetric(ctx, f.actor, scope, d.Definition.ID, dm.ValidateInput{DraftRevision: d.Definition.Draft.Revision})
			must(t, e)
			published, e := s.PublishMetric(ctx, f.actor, scope, d.Definition.ID, dm.PublishMetricInput{WriteMeta: restoreMeta(d.Definition.Revision), DraftRevision: d.Definition.Draft.Revision, ValidationHash: v.ValidationHash}, access.Meta{})
			must(t, e)
			created, e := s.CreateChannel(ctx, f.actor, scope, dm.CreateChannelInput{WriteMeta: restoreMeta(""), Code: "RESTORE_CHAIN", Name: "第二层恢复样本", Owner: dm.Owner{Kind: res.Point, ID: channel.Resource.PointID, FacilityID: channel.Resource.FacilityID}, DefinitionVersionID: published.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: channel.Resource.ID}}, Parameters: map[string]string{}}, access.Meta{})
			must(t, e)
			channel = created.Channel
			configs, e := s.ChannelConfigs(ctx, f.actor, scope, channel.Resource.ID, dm.MetricFilter{})
			must(t, e)
			cfg = configs.Items[0]
		}
		p.channels = append(p.channels, channel.Resource.ID)
		trial, e := s.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, access.Meta{})
		must(t, e)
		if trial.Trial == nil {
			_, e = f.service.RunTask(ctx, "restore-trial", f.tenant, trial.Job.ID)
			must(t, e)
			trial, e = s.Run(ctx, f.actor, scope, trial.ID)
			must(t, e)
		}
		if trial.Trial == nil || trial.Trial.OutputCount != "20" {
			t.Fatal("restore fixture trial", trial)
		}
		preview, e := s.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
		must(t, e)
		_, e = s.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
		must(t, e)
		_, e = s.ScheduleTenant(ctx, f.tenant)
		must(t, e)
		var job, generation string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND channel_id=$2 AND mode='automatic' ORDER BY created_at DESC LIMIT 1`, f.tenant, channel.Resource.ID).Scan(&job))
		_, e = f.service.RunTask(ctx, "restore-formal", f.tenant, job)
		must(t, e)
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM derived_generations WHERE tenant_id=$1 AND channel_id=$2 ORDER BY created_at LIMIT 1`, f.tenant, channel.Resource.ID).Scan(&generation))
		p.original = append(p.original, generation)
	}
	var rule dm.Rule
	must(t, json.Unmarshal(cfg.Rule, &rule))
	copyOutput := rule.Outputs[0]
	copyOutput.Key = "audit_copy"
	copyOutput.SemanticKey = "recovery_audit_copy"
	rule.Outputs = append(rule.Outputs, copyOutput)
	raw, e := json.Marshal(rule)
	must(t, e)
	analysis, e := s.CreateAnalysis(ctx, f.actor, scope, dm.CreateAnalysisInput{WriteMeta: restoreMeta(""), Code: "RESTORE_ANALYSIS", Name: "恢复多输出分析", Body: dm.AnalysisBody{Rule: raw, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters}}, access.Meta{})
	must(t, e)
	validation, e := s.ValidateAnalysis(ctx, f.actor, scope, analysis.Config.ID, dm.ValidateInput{DraftRevision: analysis.Config.Draft.Revision})
	must(t, e)
	published, e := s.PublishAnalysis(ctx, f.actor, scope, analysis.Config.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: restoreMeta(analysis.Config.Revision), DraftRevision: analysis.Config.Draft.Revision, ValidationHash: validation.ValidationHash}}, access.Meta{})
	must(t, e)
	run, e := s.StartAnalysis(ctx, f.actor, scope, dm.AnalysisRunInput{WriteMeta: restoreMeta(""), ConfigVersionID: published.Version.ID, ConfigHash: published.Version.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, false, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "restore-analysis", f.tenant, run.Job.ID)
	must(t, e)
	p.artifact, p.artifactVersion = run.Analysis.ArtifactID, run.Analysis.ArtifactVersionID
	return p
}
func (p *derivedRestoreProof) freeze(t *testing.T, f monitoringFixture) {
	t.Helper()
	s := p.service
	for round := 0; round < 8; round++ {
		_, e := s.ConsumeChanges(ctx, f.tenant, time.Now())
		must(t, e)
		_, e = s.ScheduleTenant(ctx, f.tenant)
		must(t, e)
		var jobs []string
		rows, e := f.env.Pool.Query(ctx, `SELECT j.id::text FROM monitoring_jobs j JOIN derived_runs r ON r.job_id=j.id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state='queued' ORDER BY r.created_at`, f.tenant)
		must(t, e)
		for rows.Next() {
			var id string
			must(t, rows.Scan(&id))
			jobs = append(jobs, id)
		}
		must(t, rows.Err())
		rows.Close()
		for _, job := range jobs {
			_, e = f.service.RunTask(ctx, "restore-invalidation", f.tenant, job)
			must(t, e)
		}
		if len(jobs) == 0 {
			break
		}
		if round == 7 {
			t.Fatal("restore fixture automatic backlog did not converge")
		}
	}
	scope := dm.TenantScope(f.tenant)
	for i, id := range p.channels {
		channel, e := s.Channel(ctx, f.actor, scope, id)
		must(t, e)
		if i == 0 {
			preview, e := s.PreviewControl(ctx, f.actor, scope, id, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "pause"}})
			must(t, e)
			_, e = s.ControlChannel(ctx, f.actor, scope, id, dm.ControlInput{WriteMeta: restoreMeta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
			must(t, e)
		} else {
			target := res.Target{Kind: res.Channel, ID: id, Facility: channel.Resource.FacilityID, Point: channel.Resource.PointID}
			_, e = f.write(t, target, "retire", map[string]any{"expected_revision": channel.Resource.Revision, "reason": "备份前保留退役派生历史"})
			must(t, e)
		}
	}
	rows, e := f.env.Pool.Query(ctx, `SELECT id::text FROM derived_generations WHERE tenant_id=$1 ORDER BY id`, f.tenant)
	must(t, e)
	for rows.Next() {
		var id string
		must(t, rows.Scan(&id))
		p.generations = append(p.generations, id)
	}
	must(t, rows.Err())
	rows.Close()
	if len(p.generations) < 4 {
		t.Fatal("missing revoked baseline generations", p.generations)
	}
	for _, id := range p.generations {
		details, e := s.GenerationDetails(ctx, f.actor, scope, id, dm.GenerationQuery{Limit: 100})
		must(t, e)
		if len(details.Items) != 20 {
			t.Fatal("incomplete recovery fixture", id, len(details.Items))
		}
		p.rows[id] = details.Items
	}
	artifact, e := s.AnalysisArtifactRows(ctx, f.actor, scope, p.artifact, dm.AnalysisArtifactQuery{VersionID: p.artifactVersion, Limit: 100})
	must(t, e)
	if len(artifact.Items) != 40 || artifact.Version.InputState != "revoked" {
		t.Fatal("fixed analysis recovery fixture", artifact)
	}
	p.artifactRows = artifact.Items
	report, e := dm.NewRecovery(f.storage).Reconcile(ctx, f.env.Pool, f.tenant)
	must(t, e)
	if report.State != "passed" || report.CurrentBytes == 0 || report.HistoryBytes == 0 || report.ArtifactBytes == 0 || report.CurrentBytes+report.HistoryBytes+report.ArtifactBytes != report.Bytes {
		t.Fatal("result payload inventory or reconciliation failed", report)
	}
}
func (p *derivedRestoreProof) verify(t *testing.T, f monitoringFixture, pool *pgxpool.Pool, storage *md.Storage, actor, projectActor access.Actor, project string, host *md.Service) {
	t.Helper()
	s := dm.New(access.New(pool, slog.Default())).WithMonitoring(host).WithStorage(storage)
	must(t, s.RegisterTasks())
	scope := dm.TenantScope(f.tenant)
	for i, id := range p.channels {
		channel, e := s.Channel(ctx, actor, scope, id)
		must(t, e)
		expected := []string{"paused", "retired"}[i]
		if channel.ControlState != expected {
			t.Fatal("restored control changed", channel.ControlState, expected)
		}
	}
	for _, id := range p.generations {
		details, e := s.GenerationDetails(ctx, actor, scope, id, dm.GenerationQuery{Limit: 100})
		must(t, e)
		a, _ := json.Marshal(details.Items)
		b, _ := json.Marshal(p.rows[id])
		if string(a) != string(b) {
			t.Fatal("restored generation differs", id)
		}
		if _, e = s.GenerationDetails(ctx, projectActor, dm.ProjectScope(f.tenant, project), id, dm.GenerationQuery{Limit: 100}); e == nil {
			t.Fatal("restored project gained derived input authority")
		}
		run, e := s.ReplayGeneration(ctx, actor, scope, id, dm.ReplayInput{WriteMeta: restoreMeta("")}, access.Meta{})
		must(t, e)
		_, e = host.RunTask(ctx, "restored-historical-replay", f.tenant, run.Job.ID)
		must(t, e)
		run, e = s.Run(ctx, actor, scope, run.ID)
		must(t, e)
		if run.Replay == nil || !run.Replay.Matched || run.Replay.Expected != 20 {
			t.Fatal("restored replay mismatch", id, run)
		}
	}
	artifact, e := s.AnalysisArtifactRows(ctx, actor, scope, p.artifact, dm.AnalysisArtifactQuery{VersionID: p.artifactVersion, Limit: 100})
	must(t, e)
	actual, _ := json.Marshal(artifact.Items)
	expected, _ := json.Marshal(p.artifactRows)
	if string(actual) != string(expected) || artifact.Version.InputState != "revoked" {
		t.Fatal("restored fixed analysis changed")
	}
	if _, e = s.AnalysisArtifactRows(ctx, projectActor, dm.ProjectScope(f.tenant, project), p.artifact, dm.AnalysisArtifactQuery{VersionID: p.artifactVersion, Limit: 100}); e == nil {
		t.Fatal("tenant artifact leaked to project after recovery")
	}
	report, e := dm.NewRecovery(storage).Reconcile(ctx, pool, f.tenant)
	must(t, e)
	if report.State != "passed" || report.CurrentBytes == 0 || report.HistoryBytes == 0 || report.ArtifactBytes == 0 || report.CurrentBytes+report.HistoryBytes+report.ArtifactBytes != report.Bytes {
		t.Fatal("result payload inventory or reconciliation failed", report)
	}
	t.Log(fmt.Sprintf("restored %d immutable generations, fixed revoked baseline and two-level chain; all original hashes replayed", len(p.generations)))
}
