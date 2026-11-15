//go:build monitoringintegration

package resources_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

type firstBlockWriter struct {
	written    int
	afterFirst func()
}

func (w *firstBlockWriter) Write(p []byte) (int, error) {
	w.written += len(p)
	if w.afterFirst != nil {
		fn := w.afterFirst
		w.afterFirst = nil
		fn()
	}
	return len(p), nil
}

func TestMonitoringPartialProjectMetadataAndDownloadRevocation(t *testing.T) {
	f := monitoringSetup(t)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var data strings.Builder
	data.WriteString("observed_at,value,quality\n")
	for n := 0; n < 4000; n++ {
		fmt.Fprintf(&data, "%s,%d,good\n", base.Add(time.Duration(n)*time.Nanosecond).Format(time.RFC3339Nano), n)
	}
	batch := f.publish(t, f.validate(t, f.upload(t, data.String())))
	project := f.project(t, "MonitoringScope")
	var facility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	resourceInput := f.accessInput(t, project, facility, "create_link")
	resourceInput.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, project, facility, resourceInput)
	var resourceGrant string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, f.channel).Scan(&resourceGrant))
	from, to := base.Format(time.RFC3339Nano), base.Add(2000*time.Nanosecond).Format(time.RFC3339Nano)
	grant, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta("0"), ChannelID: f.channel, ResourceGrantID: resourceGrant, Range: &md.Range{From: &from, To: &to}, Capabilities: []string{"read", "revoke", "source", "export"}})
	must(t, e)
	scope := md.ProjectScope(f.tenant, project.id)
	points, e := f.service.Points(ctx, project.actor, scope, md.NavigationQuery{})
	must(t, e)
	if len(points.Items) != 1 {
		t.Fatal(points)
	}
	channels, e := f.service.Channels(ctx, project.actor, scope, points.Items[0].ID, "")
	must(t, e)
	if len(channels.Items) != 1 || channels.Items[0].ID != f.channel || len(channels.Items[0].Ranges) != 1 {
		t.Fatal(channels)
	}
	selection, e := f.service.Select(ctx, project.actor, scope, md.SelectionInput{Channels: []string{f.channel}})
	must(t, e)
	query := md.QueryInput{Selection: selection.ID, AllHistory: true, Limit: 1000}
	page, e := f.service.Details(ctx, project.actor, scope, query)
	must(t, e)
	if len(page.Items) != 1000 || !page.HasMore || page.Items[0].At != base.Add(1999*time.Nanosecond).Format(time.RFC3339Nano) {
		t.Fatal(page)
	}
	stats := f.analyticAs(t, project.actor, scope, "statistics", query)
	if len(stats.Channels) != 1 || stats.Channels[0].Ever != "2000" || stats.Channels[0].Active != "2000" || *stats.Channels[0].Mean != "999.500000000000" {
		t.Fatal("summary crossed authorization boundary", stats)
	}
	view, e := f.service.Import(ctx, project.actor, scope, batch.ID)
	must(t, e)
	if !view.ScopeClipped || view.VisibleAdopted != "2000" || view.Counts != nil || view.Size != "" || view.SourceFile != "" || view.Mapping != nil {
		t.Fatal("full batch metadata leaked", view)
	}
	list, e := f.service.Imports(ctx, project.actor, scope, "")
	must(t, e)
	if len(list.Items) != 1 || !list.Items[0].ScopeClipped || list.Items[0].Counts != nil {
		t.Fatal(list)
	}
	for _, q := range []md.ImportQuery{{Search: batch.Name, Limit: 10}, {Formats: "csv", Limit: 10}} {
		filtered, err := f.service.QueryImports(ctx, project.actor, scope, q)
		must(t, err)
		if len(filtered.Items) != 0 {
			t.Fatal("hidden source metadata leaked through filters", filtered)
		}
	}
	for _, search := range []string{batch.ID, "共享批次"} {
		filtered, err := f.service.QueryImports(ctx, project.actor, scope, md.ImportQuery{Search: search, Limit: 10})
		must(t, err)
		if len(filtered.Items) != 1 || !filtered.Items[0].ScopeClipped {
			t.Fatal("visible partial batch was not searchable", filtered)
		}
	}
	_, e = f.service.Source(ctx, project.actor, scope, batch.SourceFile)
	monitoringCode(t, e, "monitoring_not_found")
	_, e = f.service.PreviewRevokeBatch(ctx, project.actor, scope, batch.ID, batch.Revision)
	monitoringCode(t, e, "monitoring_not_found")
	full, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(grant.AccessRevision), ChannelID: f.channel, ResourceGrantID: resourceGrant, Range: &md.Range{}, Capabilities: []string{"read", "revoke", "source", "export"}})
	must(t, e)
	preview, e := f.service.PreviewRevokeData(ctx, f.actor, f.tenant, project.id, full.ID)
	must(t, e)
	download, e := f.service.Source(ctx, project.actor, scope, batch.SourceFile)
	must(t, e)
	writer := &firstBlockWriter{afterFirst: func() {
		_, e := f.service.RevokeData(ctx, f.actor, f.tenant, project.id, full.ID, md.RevokeInput{WriteMeta: f.meta(full.Revision), PreviewHash: preview.Hash})
		must(t, e)
	}}
	e = download.Write(ctx, writer)
	monitoringCode(t, e, "authorization_changed")
	if writer.written != 128*1024 {
		t.Fatal("stream continued after revocation", writer.written)
	}
	projectExporter := f
	projectExporter.actor = project.actor
	projectExporter.scope = scope
	export := projectExporter.completeExport(t, md.ExportInput{WriteMeta: f.meta("0"), Query: projectExporter.query(t), Format: "csv"})
	if export.Rows != "2000" {
		t.Fatal("export crossed current grant", export.Rows)
	}
	result, e := projectExporter.service.ExportDownload(ctx, project.actor, scope, export.ID, "1")
	must(t, e)
	revoke, e := f.service.PreviewRevokeData(ctx, f.actor, f.tenant, project.id, grant.ID)
	must(t, e)
	stream := &firstBlockWriter{afterFirst: func() {
		_, e := f.service.RevokeData(ctx, f.actor, f.tenant, project.id, grant.ID, md.RevokeInput{WriteMeta: f.meta(grant.Revision), PreviewHash: revoke.Hash})
		must(t, e)
	}}
	e = result.Write(ctx, stream)
	monitoringCode(t, e, "authorization_changed")
	if stream.written != 128*1024 {
		t.Fatal("export continued after revoke", stream.written)
	}
	points, e = f.service.Points(ctx, project.actor, scope, md.NavigationQuery{})
	must(t, e)
	if len(points.Items) != 0 {
		t.Fatal("revoked navigation remained visible", points)
	}

}

func TestMonitoringWorkspaceWithoutDataGrantsIsAnEmptySelection(t *testing.T) {
	f := monitoringSetup(t)
	project := f.project(t, "MonitoringEmpty")
	scope := md.ProjectScope(f.tenant, project.id)
	selected, e := f.service.Select(ctx, project.actor, scope, md.SelectionInput{All: true})
	must(t, e)
	if selected.Count != "0" {
		t.Fatal(selected)
	}
	q := md.QueryInput{Selection: selected.ID, AllHistory: true}
	overview := f.analyticAs(t, project.actor, scope, "overview", q)
	if overview.Overview.Active != "0" || overview.Overview.Ever != "0" || overview.Overview.Latest != nil {
		t.Fatal(overview)
	}
	q.Snapshot = overview.Snapshot.ID
	details, e := f.service.Details(ctx, project.actor, scope, q)
	must(t, e)
	if len(details.Items) != 0 {
		t.Fatal(details)
	}
}
