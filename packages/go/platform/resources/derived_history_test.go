//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
)

func verifyDerivedProjectHistory(t *testing.T, f monitoringFixture, service *dm.Service, channel, generation string, part dm.NanosecondRange) {
	t.Helper()
	project := f.project(t, "DerivedHistory")
	var facility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, channel).Scan(&facility))
	grant := f.accessInput(t, project, facility, "create_link")
	grant.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &channel, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, project, facility, grant)
	resourceGrant := func(id string) string {
		var g string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, id).Scan(&g))
		return g
	}
	from, to := "2024-01-01T00:01:00Z", "2024-01-01T00:02:00Z"
	outputGrant, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta("0"), ChannelID: channel, ResourceGrantID: resourceGrant(channel), Range: &md.Range{From: &from, To: &to}, Capabilities: []string{"read", "history"}})
	must(t, e)
	from = "2024-01-01T00:00:00Z"
	inputGrant, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(outputGrant.AccessRevision), ChannelID: f.channel, ResourceGrantID: resourceGrant(f.channel), Range: &md.Range{From: &from, To: &to}, Capabilities: []string{"read"}})
	must(t, e)
	scope := dm.ProjectScope(f.tenant, project.id)
	if _, e = service.GenerationDetails(ctx, project.actor, scope, generation, dm.GenerationQuery{Range: &part}); e == nil {
		t.Fatal("output history plus input read bypassed input history qualification")
	}
	historyGrant, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(inputGrant.AccessRevision), ChannelID: f.channel, ResourceGrantID: resourceGrant(f.channel), Range: &md.Range{From: &from, To: &to}, Capabilities: []string{"read", "history"}})
	must(t, e)
	page, e := service.GenerationDetails(ctx, project.actor, scope, generation, dm.GenerationQuery{Range: &part})
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].Value == nil || *page.Items[0].Value != "3.000000000000000000" || page.Generation.Rows != nil {
		t.Fatal("authorized partial historical result", page)
	}
	if _, e = service.GenerationDetails(ctx, project.actor, scope, generation, dm.GenerationQuery{}); e == nil {
		t.Fatal("partial output grant exposed full historical generation")
	}

	exportInput := dm.GenerationExportInput{WriteMeta: restoreMeta(""), Range: &part, Format: "csv"}
	if _, e = service.CreateGenerationExport(ctx, project.actor, scope, generation, exportInput, access.Meta{}); e == nil {
		t.Fatal("history read implicitly permitted source export")
	}
	outputFrom := "2024-01-01T00:01:00Z"
	outputExport, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(historyGrant.AccessRevision), ChannelID: channel, ResourceGrantID: resourceGrant(channel), Range: &md.Range{From: &outputFrom, To: &to}, Capabilities: []string{"read", "history", "export"}})
	must(t, e)
	inputExport, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(outputExport.AccessRevision), ChannelID: f.channel, ResourceGrantID: resourceGrant(f.channel), Range: &md.Range{From: &from, To: &to}, Capabilities: []string{"read", "history", "export"}})
	must(t, e)
	var prepared md.Download
	var exported string
	for _, format := range []string{"csv", "parquet"} {
		exportInput = dm.GenerationExportInput{WriteMeta: restoreMeta(""), Range: &part, Format: format}
		record, e := service.CreateGenerationExport(ctx, project.actor, scope, generation, exportInput, access.Meta{})
		must(t, e)
		again, e := service.CreateGenerationExport(ctx, project.actor, scope, generation, exportInput, access.Meta{})
		must(t, e)
		if again.ID != record.ID {
			t.Fatal("input export receipt duplicated")
		}
		_, e = f.service.RunTask(ctx, "project-history-input-export", f.tenant, record.Job.ID)
		must(t, e)
		manifest, e := service.GenerationExportFiles(ctx, project.actor, scope, record.ID)
		must(t, e)
		verifyInputEvidenceFile(t, manifest, func(p string) (md.Download, error) {
			return service.GenerationExportDownload(ctx, project.actor, scope, record.ID, p)
		}, []string{"2", "5"})
		prepared, e = service.GenerationExportDownload(ctx, project.actor, scope, record.ID, "1")
		must(t, e)
		exported = record.ID
	}
	inputExportPreview, e := f.service.PreviewRevokeData(ctx, f.actor, f.tenant, project.id, inputExport.ID)
	must(t, e)
	_, e = f.service.RevokeData(ctx, f.actor, f.tenant, project.id, inputExport.ID, md.RevokeInput{WriteMeta: f.meta(inputExportPreview.Grant.Revision), PreviewHash: inputExportPreview.Hash})
	must(t, e)
	var downloaded bytes.Buffer
	if e = prepared.Write(ctx, &downloaded); e == nil || downloaded.Len() != 0 {
		t.Fatal("prepared evidence download bypassed revoked export permission", e, downloaded.Len())
	}
	if _, e = service.GenerationExportFiles(ctx, project.actor, scope, exported); e == nil {
		t.Fatal("revoked input export permission exposed file manifest")
	}
	if _, e = service.CreateGenerationExport(ctx, project.actor, scope, generation, exportInput, access.Meta{}); e == nil {
		t.Fatal("evidence idempotency bypassed revoked permission")
	}
	request := dm.ReplayInput{WriteMeta: dm.WriteMeta{RequestID: uuid.NewString(), Reason: "项目原依据部分回放"}, Range: &part}
	run, e := service.ReplayGeneration(ctx, project.actor, scope, generation, request, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "project-history", f.tenant, run.Job.ID)
	must(t, e)
	run, e = service.Run(ctx, project.actor, scope, run.ID)
	must(t, e)
	if run.Replay == nil || !run.Replay.Matched || run.Replay.Actual != 1 {
		t.Fatal("project partial replay", run)
	}
	preview, e := f.service.PreviewRevokeData(ctx, f.actor, f.tenant, project.id, historyGrant.ID)
	must(t, e)
	_, e = f.service.RevokeData(ctx, f.actor, f.tenant, project.id, historyGrant.ID, md.RevokeInput{WriteMeta: f.meta(preview.Grant.Revision), PreviewHash: preview.Hash})
	must(t, e)
	if _, e = service.GenerationDetails(ctx, project.actor, scope, generation, dm.GenerationQuery{Range: &part}); e == nil {
		t.Fatal("revoked input history still returned results")
	}
	if _, e = service.GenerationLineage(ctx, project.actor, scope, generation, dm.GenerationQuery{Range: &part}); e == nil {
		t.Fatal("revoked input history still returned lineage")
	}
	if _, e = service.Run(ctx, project.actor, scope, run.ID); e == nil {
		t.Fatal("completed replay report bypassed current input history revocation")
	}
	if _, e = service.ReplayGeneration(ctx, project.actor, scope, generation, request, access.Meta{}); e == nil {
		t.Fatal("idempotent replay receipt bypassed current history revocation")
	}
}
