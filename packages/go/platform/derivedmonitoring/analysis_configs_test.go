package derivedmonitoring_test

import (
	"encoding/json"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/google/uuid"
)

func TestAnalysisConfigurationVersionsScopesAndArchive(t *testing.T) {
	f := channelsSetup(t)
	actor := f.actor(f.admin)
	body := dm.AnalysisBody{Rule: f.rule, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: f.direct}}, Parameters: map[string]string{}}
	in := dm.CreateAnalysisInput{WriteMeta: wm(""), Code: "HISTORY", Name: "多输出历史分析", Body: body}
	created, e := f.s.CreateAnalysis(ctx, actor, f.scope, in, access.Meta{})
	must(t, e)
	again, e := f.s.CreateAnalysis(ctx, actor, f.scope, in, access.Meta{})
	must(t, e)
	if !again.Replayed || created.Config.ID != again.Config.ID {
		t.Fatal("draft create receipt not stable", again)
	}
	validation, e := f.s.ValidateAnalysis(ctx, actor, f.scope, created.Config.ID, dm.ValidateInput{DraftRevision: created.Config.Draft.Revision})
	must(t, e)
	publish := func(c dm.AnalysisConfig, hash string) dm.AnalysisConfigResult {
		t.Helper()
		v, e := f.s.PublishAnalysis(ctx, actor, f.scope, c.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: wm(c.Revision), DraftRevision: c.Draft.Revision, ValidationHash: hash}}, access.Meta{})
		must(t, e)
		return v
	}
	first := publish(created.Config, validation.ValidationHash)
	if first.Version == nil || first.Version.Number != "1" || first.Config.LatestVersion != "1" {
		t.Fatal(first)
	}
	var channelCount int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_channels WHERE tenant_id=$1`, f.scope.Tenant).Scan(&channelCount))
	if channelCount != 0 {
		t.Fatal("analysis created engineering output channel")
	}
	var rule dm.Rule
	must(t, json.Unmarshal(body.Rule, &rule))
	rule.Outputs = append(rule.Outputs, rule.Outputs[0])
	rule.Outputs[1].Key = "alternate"
	rule.Outputs[1].SemanticKey = "alternate_difference"
	body.Rule, e = json.Marshal(rule)
	must(t, e)
	saved, e := f.s.SaveAnalysis(ctx, actor, f.scope, first.Config.ID, dm.SaveAnalysisInput{WriteMeta: wm(first.Config.Revision), Name: first.Config.Name, Body: body}, access.Meta{})
	must(t, e)
	_, e = f.s.PublishAnalysis(ctx, actor, f.scope, first.Config.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: wm(saved.Config.Revision), DraftRevision: saved.Config.Draft.Revision, ValidationHash: validation.ValidationHash}}, access.Meta{})
	if e == nil {
		t.Fatal("stale validation allowed publication")
	}
	validation, e = f.s.ValidateAnalysis(ctx, actor, f.scope, saved.Config.ID, dm.ValidateInput{DraftRevision: saved.Config.Draft.Revision})
	must(t, e)
	if len(validation.Outputs) != 2 {
		t.Fatal(validation)
	}
	second := publish(saved.Config, validation.ValidationHash)
	versions, e := f.s.AnalysisVersions(ctx, actor, f.scope, first.Config.ID, "")
	must(t, e)
	if len(versions.Items) != 2 || versions.Items[1].ConfigHash != first.Version.ConfigHash || string(versions.Items[1].Body.Rule) != string(first.Version.Body.Rule) {
		t.Fatal("saved version reinterpreted old body", versions)
	}
	_, e = f.e.Pool.Exec(ctx, `UPDATE analysis_config_versions SET name='changed' WHERE id=$1`, first.Version.ID)
	if e == nil {
		t.Fatal("immutable analysis version updated")
	}
	_, e = f.e.Pool.Exec(ctx, `INSERT INTO derived_calculation_config_keys(tenant_id,id,kind) VALUES($1,$2,'analysis')`, f.scope.Tenant, uuid.NewString())
	if e == nil {
		t.Fatal("orphan shared config identity accepted")
	}
	tenant, e := tenancy.New(f.e.Service.Access()).Context(ctx, actor, f.scope.Tenant)
	must(t, e)
	project, e := projects.New(f.e.Service.Access()).Create(ctx, actor, f.scope.Tenant, projects.CreateInput{Fields: projects.Fields{Name: "隔离项目", Code: "analysis", Purpose: "分析成果空间隔离"}, WriteMeta: projects.WriteMeta{RequestID: uuid.NewString(), Reason: "验证空间隔离"}, Owner: tenant.MembershipID, Administrator: tenant.MembershipID}, access.Meta{})
	must(t, e)
	projectScope := dm.ProjectScope(f.scope.Tenant, project.ProjectID)
	_, e = f.s.AnalysisConfig(ctx, actor, projectScope, first.Config.ID)
	if e == nil {
		t.Fatal("tenant analysis leaked to project path")
	}
	_, e = f.s.CopyAnalysis(ctx, actor, projectScope, first.Config.ID, dm.CopyAnalysisInput{WriteMeta: wm(""), Code: "LEAK", Name: "隐式共享", SourceVersionID: first.Version.ID}, access.Meta{})
	if e == nil {
		t.Fatal("project copied tenant analysis through membership fallback")
	}
	copy, e := f.s.CopyAnalysis(ctx, actor, f.scope, first.Config.ID, dm.CopyAnalysisInput{WriteMeta: wm(""), Code: "COPY", Name: "独立副本", SourceVersionID: first.Version.ID}, access.Meta{})
	must(t, e)
	if copy.Config.ID == first.Config.ID || copy.Config.LatestVersion != "0" {
		t.Fatal(copy)
	}
	archived, e := f.s.ArchiveAnalysis(ctx, actor, f.scope, first.Config.ID, wm(second.Config.Revision), access.Meta{})
	must(t, e)
	_, e = f.s.SaveAnalysis(ctx, actor, f.scope, first.Config.ID, dm.SaveAnalysisInput{WriteMeta: wm(archived.Config.Revision), Name: "禁止修改", Body: body}, access.Meta{})
	if e == nil {
		t.Fatal("archived configuration edited")
	}
	versions, e = f.s.AnalysisVersions(ctx, actor, f.scope, first.Config.ID, "")
	must(t, e)
	if len(versions.Items) != 2 {
		t.Fatal("archive removed execution versions")
	}
}
