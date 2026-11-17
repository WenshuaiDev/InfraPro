package resources_test

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	cl "github.com/WenshuaiDev/InfraPro/packages/go/platform/classification"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
)

func TestDisplayStructureTypeIdentityVersionsAndAdoptionEvidence(t *testing.T) {
	f := resourcesSetup(t)
	b := runtime.Compose(displayReadPlanAccess(t, f))
	f.srv = b.Resources
	content := simple("同名设施类型")
	content.MetamodelVersion = "2"
	content.Structure = &eng.Structure{MonitoringSection: &eng.StructureCapability{Supported: ptr(true), Fields: []eng.Field{}}, Borehole: &eng.StructureCapability{Supported: ptr(true), Fields: []eng.Field{}}}
	first := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "identity_a", content))
	secondVersion := f.structureVersion(t, first.VersionID, true)
	other := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "identity_b", content))
	project := f.project(t, "TypeIdentity")
	facilities := []r.Result{}
	for i, version := range []string{first.VersionID, secondVersion, other.VersionID} {
		facility, e := f.write(t, r.Target{Kind: r.Facility}, "create", map[string]any{"code": fmt.Sprintf("IDENTITY_%d", i), "name": "同名设施", "type_version_id": version, "values": map[string]any{}})
		must(t, e)
		facilities = append(facilities, facility)
		section := f.structureCreate(t, facility, r.MonitoringSection, "S", r.StructureRef{Kind: r.Facility, ID: facility.ID}, nil)
		grant := f.accessInput(t, project, facility.ID, "create_link")
		grant.Add = []r.GrantInput{{Kind: r.Facility, Capabilities: []string{"read"}}, {Kind: r.MonitoringSection, MonitoringSectionID: &section.ID, Capabilities: []string{"read"}}}
		f.grant(t, project, facility.ID, grant)
	}
	x := structureDisplayFixture{resourceFixture: f, b: b, p: project}
	scope := dr.Definition{ID: "resources", StructureModel: 2, Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts"}}
	view := scope
	view.ScopeID, view.ID = scope.ID, ""
	x.h = createReadDisplay(t, f, project, b, []dr.Definition{scope}, map[string]dr.Definition{"resources": view}, []string{"views.read", "queries.read", "resources.directory", "statistics.structure"})
	targetView := dr.Target{View: "resources"}
	query := dr.StructureStatisticsRequest{Target: targetView, ReadFields: dr.ReadFields{FieldGroups: []string{"statistics.counts", "resource.metadata"}}, GroupBy: "facility_type", Membership: "nearest"}
	before := readCall(t, x.h, "statistics.structure", query)
	stats := readData[dr.StructureStatistics](t, before)
	if stats.Totals.SectionTypeCount != "2" || stats.Totals.MonitoringSectionCount != "3" || len(stats.Groups) != 2 {
		t.Fatal("type identity was collapsed by name or split by version", stats)
	}
	for _, group := range stats.Groups {
		want := map[string]string{first.DefinitionID: "2", other.DefinitionID: "1"}[group.Key]
		if want == "" || group.Label != "同名设施类型" || group.Totals.MonitoringSectionCount != want {
			t.Fatal("incorrect definition group", group)
		}
	}
	selection := readData[dr.DrilldownSelection](t, readCall(t, x.h, "queries.drilldown", dr.DrilldownRequest{QueryID: before.Query.ID, GroupKey: first.DefinitionID, Basis: "resource_count", Purpose: "resources"}))
	facility := facilities[0]
	detail, e := f.srv.Detail(ctx, f.actor, r.TenantScope(f.tenant), target(facility, r.Facility, facility.ID))
	must(t, e)
	if detail.TypeVersionID != first.VersionID {
		t.Fatal("publishing a version changed the adopted version", detail.TypeVersionID)
	}
	in := r.TypeAdoptionInput{WriteMeta: r.WriteMeta{RequestID: f.wm(t).RequestID, Reason: "核对固定类型依据", Revision: detail.Revision, StructureRevision: detail.StructureRevision}, TypeVersionID: other.VersionID}
	_, e = f.srv.PreviewTypeAdoption(ctx, f.actor, f.tenant, facility.ID, in)
	code(t, e, "type_conversion_required")
	in.TypeVersionID = secondVersion
	preview, e := f.srv.PreviewTypeAdoption(ctx, f.actor, f.tenant, facility.ID, in)
	must(t, e)
	in.PreviewHash = preview.Hash
	adopted, e := f.srv.AdoptType(ctx, f.actor, f.tenant, facility.ID, in, access.Meta{})
	must(t, e)
	if adopted.ID != facility.ID {
		t.Fatal("adoption changed facility identity")
	}
	var historyCount int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM facility_type_adoptions WHERE tenant_id=$1 AND facility_id=$2 AND from_version_id=$3 AND to_version_id=$4 AND actor_id=$5 AND preview_hash=$6 AND evidence->'from'->>'id'=$3::text AND evidence->'to'->>'id'=$4::text`, f.tenant, facility.ID, first.VersionID, secondVersion, f.actor.ID, preview.Hash).Scan(&historyCount))
	if historyCount != 1 {
		t.Fatal("adoption evidence does not retain both fixed versions", historyCount)
	}
	for _, fixed := range []bool{true, false} {
		target := targetView
		if fixed {
			target = dr.Target{Selection: selection.Selection}
		}
		page := readData[dr.Page[dr.ResourceSummary]](t, readCall(t, x.h, "resources.query", dr.ResourcesRequest{Target: target, Filters: dr.ResourceFilters{Kinds: []string{"facility"}}}))
		wantCount := "3"
		if fixed {
			wantCount = "2"
		}
		if page.Total != wantCount {
			t.Fatal("wrong fixed group population", page)
		}
		found := false
		for _, item := range page.Items {
			if item.ID != facility.ID {
				continue
			}
			found = true
			want := secondVersion
			if fixed {
				want = first.VersionID
			}
			if item.Type == nil || item.Type.DefinitionID != first.DefinitionID || item.Type.VersionID != want {
				t.Fatal("fixed query mixed adopted versions", fixed, item.Type)
			}
		}
		if !found {
			t.Fatal("adopted facility missing from query")
		}
	}
}

func TestDisplayCategoryFrontierIdentityAndDictionaryMoveSnapshot(t *testing.T) {
	f := resourcesSetup(t)
	x := structureDisplaySetup(t, f, runtime.Compose(displayReadPlanAccess(t, f)))
	x.addDevices(t)
	x.display(t, true)
	admin := f.fixture.actor(f.p)
	publish := func(moveEnvironment bool) string {
		t.Helper()
		dict, e := x.b.Classification.Dictionary(ctx, admin)
		must(t, e)
		for i := range dict.Draft.Nodes {
			node := &dict.Draft.Nodes[i]
			if node.ID == x.categories["temperature"] || node.ID == x.categories["displacement"] {
				node.Name = "同名分类"
			}
			if node.ID == x.categories["temperature"] {
				node.ParentID = x.categories["displacement"]
			}
			if moveEnvironment && node.ID == x.categories["environment"] {
				node.ParentID = x.categories["collector"]
			}
		}
		saved, e := x.b.Classification.SaveDraft(ctx, admin, cl.SaveInput{Meta: cl.Meta{RequestID: f.wm(t).RequestID, Reason: "核对分类身份与前沿"}, Revision: dict.DraftRevision, Content: dict.Draft}, access.Meta{})
		must(t, e)
		in := cl.PublishInput{Meta: cl.Meta{RequestID: f.wm(t).RequestID, Reason: "发布核对分类"}, Revision: saved.Revision}
		preview, e := x.b.Classification.PreviewPublish(ctx, admin, in)
		must(t, e)
		in.PreviewHash = preview.Hash
		released, e := x.b.Classification.Publish(ctx, admin, in, access.Meta{})
		must(t, e)
		return released.ReleaseID
	}
	release := publish(false)
	query := dr.DeviceCategoryStatisticsRequest{Target: dr.Target{View: "devices"}, Population: "authorized", Mode: "direct"}
	direct := readData[dr.DeviceCategoryStatistics](t, readCall(t, x.h, "statistics.deviceCategories", query))
	matched := 0
	for _, group := range direct.Groups {
		if group.Label == "同名分类" {
			matched++
			if group.Count != map[string]string{x.categories["displacement"]: "2", x.categories["temperature"]: "1"}[group.Key] {
				t.Fatal("same-name categories merged or direct membership migrated", group)
			}
		}
	}
	if matched != 2 {
		t.Fatal("same-name category identities were lost", direct)
	}
	query.Mode = "inclusive"
	oldResult := readCall(t, x.h, "statistics.deviceCategories", query)
	inclusive := readData[dr.DeviceCategoryStatistics](t, oldResult)
	if inclusive.Additivity != "overlapping" {
		t.Fatal("ancestor counts claimed additive", inclusive)
	}
	for _, group := range inclusive.Groups {
		if group.Key == x.categories["displacement"] && (group.DirectCount != "2" || group.IncludedCount != "3" || group.Count != "3") {
			t.Fatal("parent direct and descendant populations were conflated", group)
		}
	}
	selection := readData[dr.DrilldownSelection](t, readCall(t, x.h, "queries.drilldown", dr.DrilldownRequest{QueryID: oldResult.Query.ID, GroupKey: x.categories["sensor"], Basis: "device_count", Purpose: "devices"}))
	query.Mode, query.FrontierIDs = "frontier", []string{x.categories["displacement"], x.categories["collector"]}
	frontier := readData[dr.DeviceCategoryStatistics](t, readCall(t, x.h, "statistics.deviceCategories", query))
	want := map[string]string{x.categories["displacement"]: "3", x.categories["collector"]: "1", "outside_frontier": "1", "unclassified": "1"}
	total := 0
	for _, group := range frontier.Groups {
		if group.Count != want[group.Key] {
			t.Fatal("frontier failed to retain parent-direct, outside or unclassified members", group)
		}
		count, e := strconv.Atoi(group.Count)
		must(t, e)
		total += count
	}
	if len(frontier.Groups) != 4 || total != 6 || frontier.Totals.UniqueDeviceCount != "6" || frontier.Additivity != "disjoint" {
		t.Fatal("frontier does not reconcile with unique devices", frontier)
	}
	newRelease := publish(true)
	query.Mode, query.FrontierIDs = "direct", nil
	current := readData[dr.DeviceCategoryStatistics](t, readCall(t, x.h, "statistics.deviceCategories", query))
	if newRelease == release || current.DictionaryReleaseID != newRelease || current.Totals.SensorDeviceCount != "3" || current.Totals.NonSensorDeviceCount != "2" || current.Totals.UsedSensorCategoryCount != "2" {
		t.Fatal("new query did not use the moved dictionary", current)
	}
	fixed := readData[dr.Page[dr.DeviceDetail]](t, readCall(t, x.h, "devices.list", dr.DevicesRequest{Target: dr.Target{Selection: selection.Selection}, ReadFields: dr.ReadFields{FieldGroups: []string{"device.metadata", "device.classification"}}}))
	if fixed.Total != "4" {
		t.Fatal("fixed sensor population changed after dictionary move", fixed)
	}
	for _, device := range fixed.Items {
		if device.Classification == nil || device.Classification.DictionaryReleaseID != release || !slices.Contains(device.Classification.Path, x.categories["sensor"]) || slices.Contains(device.Classification.Path, x.categories["collector"]) {
			t.Fatal("fixed drilldown mixed dictionary versions", device.Classification)
		}
	}
}
