//go:build monitoringintegration

package resources_test

import (
	"testing"

	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func verifyRecursiveProjectHistory(t *testing.T, f monitoringFixture, service *dm.Service, channels []string, generation string, part dm.NanosecondRange) {
	t.Helper()
	project := f.project(t, "RecursiveHistory")
	var facility string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	link := f.accessInput(t, project, facility, "create_link")
	link.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	for _, id := range append(append([]string{}, channels...), f.channel) {
		id := id
		link.Add = append(link.Add, res.GrantInput{Kind: res.Channel, ChannelID: &id, Capabilities: []string{"read"}})
	}
	f.grant(t, project, facility, link)
	from, to := "2024-01-01T00:00:00Z", "2024-01-01T00:01:00Z"
	revision := "0"
	grants := map[string]string{}
	add := func(channel string, capabilities []string) {
		t.Helper()
		var resource string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, channel).Scan(&resource))
		result, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(revision), ChannelID: channel, ResourceGrantID: resource, Range: &md.Range{From: &from, To: &to}, Capabilities: capabilities})
		must(t, e)
		revision = result.AccessRevision
		grants[channel] = result.ID
	}
	for _, channel := range channels {
		add(channel, []string{"read", "history"})
	}
	scope := dm.ProjectScope(f.tenant, project.id)
	query := dm.GenerationQuery{Range: &part}
	if _, e := service.GenerationDetails(ctx, project.actor, scope, generation, query); e == nil {
		t.Fatal("all intermediate grants bypassed missing recursive raw grant")
	}
	add(f.channel, []string{"read"})
	if _, e := service.GenerationDetails(ctx, project.actor, scope, generation, query); e == nil {
		t.Fatal("all intermediate history plus leaf read bypassed leaf history")
	}
	add(f.channel, []string{"read", "history"})
	page, e := service.GenerationDetails(ctx, project.actor, scope, generation, query)
	must(t, e)
	if len(page.Items) != 1 || page.Generation.Rows != nil || page.Items[0].Value == nil || *page.Items[0].Value != "512.000000000000000000" {
		t.Fatal("partial recursive history changed or leaked full count", page)
	}
	if _, e = service.GenerationDetails(ctx, project.actor, scope, generation, dm.GenerationQuery{}); e == nil {
		t.Fatal("partial recursive grant exposed full generation")
	}
	middle := channels[len(channels)/2]
	preview, e := f.service.PreviewRevokeData(ctx, f.actor, f.tenant, project.id, grants[middle])
	must(t, e)
	_, e = f.service.RevokeData(ctx, f.actor, f.tenant, project.id, grants[middle], md.RevokeInput{WriteMeta: f.meta(preview.Grant.Revision), PreviewHash: preview.Hash})
	must(t, e)
	if _, e = service.GenerationDetails(ctx, project.actor, scope, generation, query); e == nil {
		t.Fatal("recursive history bypassed revoked middle output")
	}
	if _, e = service.GenerationLineage(ctx, project.actor, scope, generation, query); e == nil {
		t.Fatal("recursive lineage bypassed revoked middle output")
	}
}
