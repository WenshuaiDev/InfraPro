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
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/google/uuid"
)

func TestAnalysisAtomicMultiOutputVersionsAndChangedInputs(t *testing.T) {
	f := monitoringSetup(t)
	t.Cleanup(func() { verifyTerminalNotifications(t, f.env) })
	service, _, cfg := derivedChannelSetup(t, f, func(rule *dm.Rule) {
		rule.Nodes = []dm.Node{{ID: "result", Op: "formula", Params: dm.NodeParams{Expression: "x * 2", Variables: map[string]string{"x": "input:x"}}}}
		rule.Outputs[0].SemanticKey = "double_value"
	})
	service.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	batch := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "完整多输出成果与固定来源验证"}
	}
	scope := dm.TenantScope(f.tenant)
	var rule dm.Rule
	must(t, json.Unmarshal(cfg.Rule, &rule))
	rule.Nodes = append(rule.Nodes, dm.Node{ID: "triple", Op: "formula", Params: dm.NodeParams{Expression: "x * 3", Variables: map[string]string{"x": "input:x"}}})
	rule.Outputs = append(rule.Outputs, rule.Outputs[0])
	rule.Outputs[1].Key = "triple"
	rule.Outputs[1].Node = "triple"
	rule.Outputs[1].SemanticKey = "triple_value"
	raw, e := json.Marshal(rule)
	must(t, e)
	created, e := service.CreateAnalysis(ctx, f.actor, scope, dm.CreateAnalysisInput{WriteMeta: meta(""), Code: "MULTI", Name: "多输出历史分析", Body: dm.AnalysisBody{Rule: raw, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters}}, access.Meta{})
	must(t, e)
	validation, e := service.ValidateAnalysis(ctx, f.actor, scope, created.Config.ID, dm.ValidateInput{DraftRevision: created.Config.Draft.Revision})
	must(t, e)
	published, e := service.PublishAnalysis(ctx, f.actor, scope, created.Config.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: meta(created.Config.Revision), DraftRevision: created.Config.Draft.Revision, ValidationHash: validation.ValidationHash}}, access.Meta{})
	must(t, e)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	from, to := strconv.FormatInt(start.UnixNano(), 10), strconv.FormatInt(start.Add(3*time.Minute).UnixNano(), 10)
	input := func(artifact string) dm.AnalysisRunInput {
		return dm.AnalysisRunInput{WriteMeta: meta(""), ConfigVersionID: published.Version.ID, ConfigHash: published.Version.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}, ArtifactID: artifact}
	}
	trial, e := service.StartAnalysis(ctx, f.actor, scope, input(""), true, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "analysis-trial", f.tenant, trial.Job.ID)
	must(t, e)
	trial, e = service.Run(ctx, f.actor, scope, trial.ID)
	must(t, e)
	if trial.Trial == nil || trial.Trial.OutputCount != "6" || trial.Analysis == nil || !trial.Analysis.Trial {
		t.Fatal(trial)
	}
	verifyTaskAvailableResult(t, f.resourceFixture, taskrecord.SourceRef{Module: "derived", Kind: "run", RootID: trial.ID})
	fail := func() {
		t.Helper()
		_, e := f.env.Pool.Exec(ctx, `CREATE FUNCTION fail_analysis_publication_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected analysis commit failure' USING ERRCODE='40001'; END $$; CREATE TRIGGER fail_analysis_publication_test BEFORE INSERT ON analysis_artifact_versions FOR EACH ROW EXECUTE FUNCTION fail_analysis_publication_test()`)
		must(t, e)
	}
	recover := func(job string) {
		t.Helper()
		_, e := f.env.Pool.Exec(ctx, `DROP TRIGGER fail_analysis_publication_test ON analysis_artifact_versions; DROP FUNCTION fail_analysis_publication_test()`)
		must(t, e)
		_, e = f.env.Pool.Exec(ctx, `UPDATE monitoring_jobs SET available_at=clock_timestamp() WHERE id=$1`, job)
		must(t, e)
	}
	first, e := service.StartAnalysis(ctx, f.actor, scope, input(""), false, access.Meta{})
	must(t, e)
	fail()
	_, e = f.service.RunTask(ctx, "analysis-before-commit", f.tenant, first.Job.ID)
	if e == nil {
		t.Fatal("publication fault ignored")
	}
	var visible, sealed int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM analysis_artifact_versions WHERE tenant_id=$1`, f.tenant).Scan(&visible))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_write_attempts WHERE tenant_id=$1 AND run_id=$2 AND state='sealed'`, f.tenant, first.ID).Scan(&sealed))
	if visible != 0 || sealed != 2 {
		t.Fatal("partial multi-output artifact became visible or lost immutable blocks", visible, sealed)
	}
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:30Z,3,good\n")))
	recover(first.Job.ID)
	_, e = f.service.RunTask(ctx, "analysis-recovered", f.tenant, first.Job.ID)
	must(t, e)
	artifact, version := first.Analysis.ArtifactID, first.Analysis.ArtifactVersionID
	detail, e := service.AnalysisArtifact(ctx, f.actor, scope, artifact, version)
	must(t, e)
	if !detail.Version.Stale || detail.Version.InputState != "affected" || detail.Version.OutputRows != "6" || len(detail.Outputs) != 2 || len(detail.Changes) == 0 {
		t.Fatal("original fixed snapshot not published as affected", detail)
	}
	rows, e := service.AnalysisArtifactRows(ctx, f.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: version, Limit: 100})
	must(t, e)
	if len(rows.Items) != 6 || rows.Items[0].Value == nil || *rows.Items[0].Value != "4.000000000000000000" || rows.Items[3].Value == nil || *rows.Items[3].Value != "6.000000000000000000" {
		t.Fatal(rows)
	}
	second, e := service.StartAnalysis(ctx, f.actor, scope, input(artifact), false, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "analysis-new-version", f.tenant, second.Job.ID)
	must(t, e)
	latest, e := service.AnalysisArtifact(ctx, f.actor, scope, artifact, "")
	must(t, e)
	if latest.Version.Number != "2" || latest.Version.OutputRows != "8" || latest.Version.Stale {
		t.Fatal("rerun did not make an independent complete new version", latest)
	}
	compare := func(left, right string, mapping []dm.AnalysisOutputMapping) dm.Run {
		t.Helper()
		run, e := service.CompareAnalysis(ctx, f.actor, scope, dm.AnalysisCompareInput{WriteMeta: meta(""), LeftVersionID: left, RightVersionID: right, Outputs: mapping}, access.Meta{})
		must(t, e)
		_, e = f.service.RunTask(ctx, "analysis-comparison", f.tenant, run.Job.ID)
		must(t, e)
		run, e = service.Run(ctx, f.actor, scope, run.ID)
		must(t, e)
		if run.Comparison == nil {
			t.Fatal(run)
		}
		return run
	}
	comparison := compare(version, latest.Version.ID, nil)
	if comparison.Comparison.Differences != 2 || len(comparison.Comparison.Outputs) != 2 || !comparison.Comparison.InputsChanged || comparison.Comparison.ConfigurationChanged {
		t.Fatal("comparison conflated source and configuration changes", comparison)
	}
	for _, output := range comparison.Comparison.Outputs {
		if output.Added != 1 || output.ValueChanged != 0 || output.Unchanged != 3 {
			t.Fatal("added-row comparison incorrect", output)
		}
	}
	mapped := compare(version, latest.Version.ID, []dm.AnalysisOutputMapping{{Left: rule.Outputs[0].Key, Right: "triple"}})
	if mapped.Comparison.Differences != 4 || mapped.Comparison.Outputs[0].ValueChanged != 3 || mapped.Comparison.Examples[0].Difference == nil || *mapped.Comparison.Examples[0].Difference != "2.000000000000000000" {
		t.Fatal("comparison lost exact numerical difference", mapped)
	}
	differences, e := service.AnalysisComparisonRows(ctx, f.actor, scope, mapped.ID, dm.AnalysisComparisonQuery{Limit: 2})
	must(t, e)
	if len(differences.Items) != 2 || differences.NextCursor == "" {
		t.Fatal("complete comparison differences not pageable", differences)
	}
	rest, e := service.AnalysisComparisonRows(ctx, f.actor, scope, mapped.ID, dm.AnalysisComparisonQuery{Limit: 2, Cursor: differences.NextCursor})
	must(t, e)
	if len(rest.Items) != 2 || rest.NextCursor != "" || rest.Items[0].At == differences.Items[0].At {
		t.Fatal("comparison pages repeated or truncated", rest)
	}
	identical := compare(version, version, nil)
	if identical.Comparison.Differences != 0 {
		t.Fatal("same immutable version compared differently", identical)
	}
	old, e := service.AnalysisArtifactRows(ctx, f.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: version, Limit: 2})
	must(t, e)
	if len(old.Items) != 2 || old.NextCursor == "" {
		t.Fatal(old)
	}
	page, e := service.AnalysisArtifactRows(ctx, f.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: version, Limit: 2, Cursor: old.NextCursor})
	must(t, e)
	if len(page.Items) != 2 || page.Items[0].ResultID == old.Items[0].ResultID {
		t.Fatal("fixed artifact pagination repeated", page)
	}
	exports := verifyAnalysisExports(t, f, service, f.actor, scope, artifact, version, "6", "4.000000000000000000", false)
	exports = append(exports, verifyAnalysisInputEvidence(t, f, service, artifact, version, false)...)
	third, e := service.StartAnalysis(ctx, f.actor, scope, input(artifact), false, access.Meta{})
	must(t, e)
	fail()
	_, e = f.service.RunTask(ctx, "analysis-revoked-before-commit", f.tenant, third.Job.ID)
	if e == nil {
		t.Fatal("publication fault ignored")
	}
	revoke, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: revoke.Hash})
	must(t, e)
	recover(third.Job.ID)
	_, e = f.service.RunTask(ctx, "analysis-revoked-retry", f.tenant, third.Job.ID)
	if e == nil {
		t.Fatal("revoked unfinished artifact was published")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM analysis_artifact_versions WHERE tenant_id=$1`, f.tenant).Scan(&visible))
	if visible != 2 {
		t.Fatal("revoked unfinished artifact visible", visible)
	}
	for _, id := range exports {
		if _, e := service.AnalysisExportDownload(ctx, f.actor, scope, id, "1"); e == nil {
			t.Fatal("revoked source export remained downloadable")
		}
	}
	forbidAnalysisExport(t, service, f.actor, scope, artifact, version, false)
	verifyAnalysisExports(t, f, service, f.actor, scope, artifact, version, "6", "4.000000000000000000", true)
	verifyAnalysisInputEvidence(t, f, service, artifact, version, true)
	oldVersion, e := service.AnalysisArtifact(ctx, f.actor, scope, artifact, version)
	must(t, e)
	if oldVersion.Version.InputState != "revoked" {
		t.Fatal(oldVersion)
	}
	reconciled, e := dm.NewRecovery(f.storage).Reconcile(ctx, f.env.Pool, f.tenant)
	must(t, e)
	if reconciled.State != "passed" || reconciled.Artifacts != 2 || reconciled.Rows != 14 {
		t.Fatal("artifact evidence reconciliation failed", reconciled)
	}
}
