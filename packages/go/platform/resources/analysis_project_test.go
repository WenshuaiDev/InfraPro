//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
)

func TestAnalysisWholeRangeProjectOwnershipHistoryAndArchive(t *testing.T) {
	f := monitoringSetup(t)
	t.Cleanup(func() { verifyTerminalNotifications(t, f.env) })
	service, _, cfg := derivedChannelSetup(t, f)
	service.WithStorage(f.storage)
	options := config.DefaultDerived()
	options.WriteRows = 2 // Three source rows must take the complete streaming path.
	service.WithOptions(options)
	must(t, dm.InitializeStorage(ctx, f.storage))
	batch := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	project := f.project(t, "AnalysisWhole")
	scope := dm.ProjectScope(f.tenant, project.id)
	var facility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	grant := f.accessInput(t, project, facility, "create_link")
	grant.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, project, facility, grant)
	var resourceGrant string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, f.channel).Scan(&resourceGrant))
	fromText, toText := "2024-01-01T00:00:00Z", "2024-01-01T00:03:00Z"
	read, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta("0"), ChannelID: f.channel, ResourceGrantID: resourceGrant, Range: &md.Range{From: &fromText, To: &toText}, Capabilities: []string{"read"}})
	must(t, e)
	meta := func(rev string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: rev, Reason: "项目全范围成果与当前权限验证"}
	}
	var rule dm.Rule
	must(t, json.Unmarshal(cfg.Rule, &rule))
	rule.Clock = dm.Clock{Kind: "whole_range"}
	rule.Nodes = []dm.Node{{ID: "mean", Op: "window_mean", Inputs: []string{"input:x"}, Params: dm.NodeParams{Window: &dm.Window{Kind: "whole_range"}, MinSamples: "1", MaxGapNS: "600000000000", MaxAgeNS: "600000000000"}}, {ID: "median", Op: "window_median", Inputs: []string{"input:x"}, Params: dm.NodeParams{Window: &dm.Window{Kind: "whole_range"}, MinSamples: "1", MaxGapNS: "600000000000", MaxAgeNS: "600000000000"}}}
	rule.Outputs[0].Node = "mean"
	rule.Outputs[0].Key = "mean"
	rule.Outputs[0].SemanticKey = "whole_mean"
	rule.Outputs[0].Quantity = nil
	rule.Outputs = append(rule.Outputs, rule.Outputs[0])
	rule.Outputs[1].Node = "median"
	rule.Outputs[1].Key = "median"
	rule.Outputs[1].SemanticKey = "whole_median"
	raw, e := json.Marshal(rule)
	must(t, e)
	created, e := service.CreateAnalysis(ctx, project.actor, scope, dm.CreateAnalysisInput{WriteMeta: meta(""), Code: "WHOLE", Name: "项目全范围分析", Body: dm.AnalysisBody{Rule: raw, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters}}, access.Meta{})
	must(t, e)
	validation, e := service.ValidateAnalysis(ctx, project.actor, scope, created.Config.ID, dm.ValidateInput{DraftRevision: created.Config.Draft.Revision})
	must(t, e)
	published, e := service.PublishAnalysis(ctx, project.actor, scope, created.Config.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: meta(created.Config.Revision), DraftRevision: created.Config.Draft.Revision, ValidationHash: validation.ValidationHash}}, access.Meta{})
	must(t, e)
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	from, to := strconv.FormatInt(start.UnixNano(), 10), strconv.FormatInt(start.Add(3*time.Minute).UnixNano(), 10)
	interval := dm.NanosecondRange{From: from, To: &to}
	input := dm.AnalysisRunInput{WriteMeta: meta(""), ConfigVersionID: published.Version.ID, ConfigHash: published.Version.ConfigHash, OutputRange: interval, SourceRange: &interval}
	run, e := service.StartAnalysis(ctx, project.actor, scope, input, false, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "analysis-project-whole", f.tenant, run.Job.ID)
	must(t, e)
	artifact, version := run.Analysis.ArtifactID, run.Analysis.ArtifactVersionID
	page, e := service.AnalysisArtifactRows(ctx, project.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: version, Limit: 10})
	must(t, e)
	if len(page.Items) != 2 || page.Items[0].At != to || page.Items[0].Interval == nil || page.Items[0].Interval.From != from || page.Items[0].Value == nil || *page.Items[0].Value != "5.333333333333333333" || page.Items[1].Value == nil || *page.Items[1].Value != "5.000000000000000000" {
		t.Fatal("whole-range end label or exact statistics changed", page)
	}
	if _, e = service.AnalysisArtifact(ctx, f.actor, dm.TenantScope(f.tenant), artifact, version); e == nil {
		t.Fatal("project artifact leaked through tenant admin path")
	}
	other := f.project(t, "AnalysisOther")
	if _, e = service.AnalysisArtifact(ctx, other.actor, dm.ProjectScope(f.tenant, other.id), artifact, version); e == nil {
		t.Fatal("shared input implied cross-project artifact ownership")
	}
	forbidAnalysisExport(t, service, project.actor, scope, artifact, version, false)
	revoke, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: revoke.Hash})
	must(t, e)
	if _, e = service.AnalysisArtifactRows(ctx, project.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: version}); e == nil {
		t.Fatal("input read alone revealed revoked analysis source")
	}
	history, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(read.AccessRevision), ChannelID: f.channel, ResourceGrantID: resourceGrant, Range: &md.Range{From: &fromText, To: &toText}, Capabilities: []string{"read", "history"}})
	must(t, e)
	page, e = service.AnalysisArtifactRows(ctx, project.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: version})
	must(t, e)
	if len(page.Items) != 2 || page.Version.InputState != "revoked" {
		t.Fatal(page)
	}
	forbidAnalysisExport(t, service, project.actor, scope, artifact, version, true)
	previewGrant, e := f.service.PreviewRevokeData(ctx, f.actor, f.tenant, project.id, history.ID)
	must(t, e)
	revoked, e := f.service.RevokeData(ctx, f.actor, f.tenant, project.id, history.ID, md.RevokeInput{WriteMeta: f.meta(previewGrant.Grant.Revision), PreviewHash: previewGrant.Hash})
	must(t, e)
	if _, e = service.AnalysisArtifactRows(ctx, project.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: version}); e == nil {
		t.Fatal("cached artifact bypassed revoked input history")
	}
	_, e = f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(revoked.AccessRevision), ChannelID: f.channel, ResourceGrantID: resourceGrant, Range: &md.Range{From: &fromText, To: &toText}, Capabilities: []string{"read", "history", "export"}})
	must(t, e)
	ps := projects.New(f.env.Service.Access())
	archive := projects.Change{ChangeMeta: projects.ChangeMeta{WriteMeta: projects.WriteMeta{RequestID: uuid.NewString(), Reason: "归档保留历史成果"}}}
	preview, e := ps.Preview(ctx, project.actor, f.tenant, project.id, "", access.OpProjectArchive, archive)
	must(t, e)
	archive.ProjectVersion, archive.Version, archive.Hash = preview.ProjectVersion, preview.Version, preview.Hash
	_, e = ps.Change(ctx, project.actor, f.tenant, project.id, "", access.OpProjectArchive, archive, access.Meta{})
	must(t, e)
	_, e = service.AnalysisArtifactRows(ctx, project.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: version})
	must(t, e)
	verifyAnalysisExports(t, f, service, project.actor, scope, artifact, version, "2", "5.333333333333333333", true)
	input.WriteMeta = meta("")
	input.ArtifactID = artifact
	if _, e = service.StartAnalysis(ctx, project.actor, scope, input, false, access.Meta{}); e == nil {
		t.Fatal("archived project started analysis")
	}

}
