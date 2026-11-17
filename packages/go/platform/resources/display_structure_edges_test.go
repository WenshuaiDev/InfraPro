package resources_test

import (
	"bytes"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
)

func TestDisplayStructureTypedIdentitiesAndDerivedOwners(t *testing.T) {
	f := resourcesSetup(t)
	b := runtime.Compose(displayReadPlanAccess(t, f))
	f.srv = b.Resources
	x := structureDisplaySetup(t, f, b)
	// IDs are stable within a kind; a legal cross-table collision must not merge entities.
	id := x.nodes["P5"].ID
	_, e := f.env.Pool.Exec(ctx, `INSERT INTO engineering_monitoring_sections(id,tenant_id,facility_id,code,name,created_by,updated_by) VALUES($1,$2,$3,'COLLISION','同 UUID 的独立断面',$4,$4)`, id, f.tenant, x.nodes["A"].ID, f.actor.ID)
	must(t, e)
	g := f.accessInput(t, x.p, x.nodes["A"].ID, "change_grants")
	g.Add = []r.GrantInput{{Kind: r.MonitoringSection, MonitoringSectionID: &id, Capabilities: []string{"read"}}}
	f.grant(t, x.p, x.nodes["A"].ID, g)
	x.display(t, false)
	target := dr.Target{View: "resources"}
	for _, kind := range []string{"point", "monitoring_section"} {
		detail := readData[dr.ResourceDetail](t, readCall(t, x.h, "resources.detail", dr.ResourceDetailRequest{Target: target, ID: id, Kind: kind}))
		if detail.Summary.ID != id || detail.Summary.Kind != kind {
			t.Fatal(detail)
		}
	}
	if _, e = x.h.s.Query(ctx, x.h.session, display.Call{Method: "resources.detail", Params: dr.Bytes(dr.ResourceDetailRequest{Target: target, ID: id})}); e == nil {
		t.Fatal("ambiguous identity selected an arbitrary kind")
	}
	counts := readData[dr.StructureStatistics](t, readCall(t, x.h, "statistics.structure", dr.StructureStatisticsRequest{Target: target, GroupBy: "none", Membership: "nearest"}))
	if counts.Totals.MonitoringSectionCount != "6" || counts.Totals.PointCount != "7" {
		t.Fatal(counts.Totals)
	}
	// A published calculation is owned directly by a section or a hole, with no fabricated point.
	s := b.DerivedMonitoring
	scope := dm.TenantScope(f.tenant)
	rule, e := s.ExpandTemplate(ctx, f.actor, scope, "difference", dm.ExpandTemplateInput{Inputs: []dm.InputSpec{{Key: "x", Shape: "numeric", SourceKinds: []string{"direct"}, MetricVersionID: f.metric, Quantity: dm.Quantity{BaseUnitVersionID: f.unit, BasePower: 1, Mode: "level", LevelScale: "ratio"}, Time: dm.TimeSemantics{Kind: "instant"}, Direction: "not_applicable", Reference: "not_applicable"}}})
	must(t, e)
	definition, e := s.CreateMetric(ctx, f.actor, scope, dm.CreateMetricInput{WriteMeta: structureDerivedMeta(t, f, ""), Code: "STRUCTURE_DIFF", Name: "结构所有者差分", Rule: dr.Bytes(rule)}, access.Meta{})
	must(t, e)
	validation, e := s.ValidateMetric(ctx, f.actor, scope, definition.Definition.ID, dm.ValidateInput{DraftRevision: definition.Definition.Draft.Revision})
	must(t, e)
	published, e := s.PublishMetric(ctx, f.actor, scope, definition.Definition.ID, dm.PublishMetricInput{WriteMeta: structureDerivedMeta(t, f, definition.Definition.Revision), DraftRevision: definition.Definition.Draft.Revision, ValidationHash: validation.ValidationHash}, access.Meta{})
	must(t, e)
	channels := []string{}
	for _, name := range []string{"S1", "H1"} {
		owner := dm.Owner{Kind: x.kinds[name], ID: x.nodes[name].ID, FacilityID: x.nodes["A"].ID}
		created, e := s.CreateChannel(ctx, f.actor, scope, dm.CreateChannelInput{WriteMeta: structureDerivedMeta(t, f, ""), Code: "DIFF_" + name, Name: "结构差分 " + name, Owner: owner, DefinitionVersionID: published.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: x.channels["P1"]}}}, access.Meta{})
		must(t, e)
		if created.Channel.Resource.OwnerKind != string(owner.Kind) || created.Channel.Resource.PointID != "" {
			t.Fatal(created)
		}
		channels = append(channels, created.Channel.Resource.ID)
	}
	g = f.accessInput(t, x.p, x.nodes["A"].ID, "change_grants")
	for _, id := range channels {
		id := id
		g.Add = append(g.Add, r.GrantInput{Kind: r.Channel, ChannelID: &id, Capabilities: []string{"read"}})
	}
	f.grant(t, x.p, x.nodes["A"].ID, g)
	var dataRevision string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT revision::text FROM monitoring_access_states WHERE project_id=$1`, x.p.id).Scan(&dataRevision))
	for _, id := range channels {
		var grant string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, x.p.id, id).Scan(&grant))
		v, e := b.Monitoring.GrantData(ctx, f.actor, f.tenant, x.p.id, md.GrantInput{WriteMeta: md.WriteMeta{RequestID: f.wm(t).RequestID, ExpectedRevision: dataRevision, Reason: "结构派生读取"}, ChannelID: id, ResourceGrantID: grant, Range: &md.Range{}, Capabilities: []string{"read", "history"}})
		must(t, e)
		dataRevision = v.AccessRevision
	}
	x.display(t, false, channels...)
	channelTarget := dr.Target{View: "channels"}
	counts = readData[dr.StructureStatistics](t, readCall(t, x.h, "statistics.structure", dr.StructureStatisticsRequest{Target: target, ChannelTarget: &channelTarget, GroupBy: "monitoring_section", Membership: "nearest"}))
	if counts.Totals.DerivedCount == nil || *counts.Totals.DerivedCount != "2" || *counts.Totals.ChannelCount != "9" {
		t.Fatal(counts)
	}
	page := readData[dr.Page[dr.ChannelSummary]](t, readCall(t, x.h, "channels.list", dr.ChannelsRequest{Target: channelTarget}))
	seen := 0
	for _, c := range page.Items {
		if c.SourceKind == "derived" {
			seen++
			if c.Owner.Kind != "borehole" && c.Owner.Kind != "monitoring_section" {
				t.Fatal(c)
			}
		}
	}
	if seen != 2 {
		t.Fatal(page)
	}
	// Both pre-view SDK and format-2 legacy projections refuse unrepresentable owners.
	_, e = b.Resources.ReadCatalog(ctx, x.p.actor, r.ProjectScope(f.tenant, x.p.id), channels, "", "", "", 100)
	structureUpgradeError(t, e)
	structureUpgradeError(t, b.Monitoring.CheckLegacyStructure(ctx, x.p.actor, md.ProjectScope(f.tenant, x.p.id), channels))
	legacy := dr.Definition{ID: "legacy", Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts"}}
	v := legacy
	v.ID = ""
	v.ScopeID = "legacy"
	h := createReadDisplay(t, f, x.p, b, []dr.Definition{legacy}, map[string]dr.Definition{"legacy": v}, []string{"views.read", "queries.read", "resources.directory", "statistics.resources"})
	for method, query := range map[string]any{"resources.query": dr.ResourcesRequest{Target: dr.Target{View: "legacy"}}, "statistics.resources": dr.ResourceStatisticsRequest{Target: dr.Target{View: "legacy"}, GroupBy: "none"}} {
		_, e = h.s.Query(ctx, h.session, display.Call{Method: method, Params: dr.Bytes(query)})
		structureUpgradeError(t, e)
	}
}

func structureDerivedMeta(t *testing.T, f resourceFixture, revision string) dm.WriteMeta {
	t.Helper()
	return dm.WriteMeta{RequestID: f.wm(t).RequestID, Revision: revision, Reason: "结构所有者真实服务验证"}
}
func structureUpgradeError(t *testing.T, e error) {
	t.Helper()
	if e == nil || !bytes.Contains(dr.Bytes(e), []byte("structure_model_upgrade_required")) {
		t.Fatal("expected explicit structure upgrade requirement", e)
	}
}
