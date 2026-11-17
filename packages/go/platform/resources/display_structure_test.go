package resources_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	cl "github.com/WenshuaiDev/InfraPro/packages/go/platform/classification"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	src "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/google/uuid"
)

// This fixture is section 9.5 of the approved engineering-structure proposal.
// Every entity, grant, classification and binding goes through the owning service.
type structureDisplayFixture struct {
	resourceFixture
	b          runtime.Business
	p          projectFixture
	h          *displayHarness
	nodes      map[string]r.Result
	kinds      map[string]r.Kind
	facilityOf map[string]string
	channels   map[string]string
	devices    []d.Result
	categories map[string]string
	release    string
}

func structureDisplaySetup(t *testing.T, f resourceFixture, b runtime.Business, projects ...projectFixture) structureDisplayFixture {
	t.Helper()
	var project projectFixture
	if len(projects) > 0 {
		project = projects[0]
	} else {
		project = f.project(t, "SBMCanonical")
	}
	x := structureDisplayFixture{resourceFixture: f, b: b, p: project, nodes: map[string]r.Result{}, kinds: map[string]r.Kind{}, facilityOf: map[string]string{}, channels: map[string]string{}, categories: map[string]string{}}
	for i, name := range []string{"路基", "隧道"} {
		content := simple(name)
		content.MetamodelVersion = "2"
		content.Structure = &eng.Structure{MonitoringSection: &eng.StructureCapability{Supported: ptr(true), Fields: []eng.Field{}}, Borehole: &eng.StructureCapability{Supported: ptr(true), Fields: []eng.Field{}}}
		content.Components = []eng.Component{{Key: "span", Name: "跨"}}
		content.Relations = []eng.Relation{{Parent: "root", Child: "span"}}
		typ := f.fixture.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, fmt.Sprintf("sbm_type_%d", i), content))
		code := []string{"A", "B"}[i]
		facility, e := f.write(t, r.Target{Kind: r.Facility}, "create", map[string]any{"code": code, "name": name, "type_version_id": typ.VersionID, "values": map[string]any{}})
		structureDisplayMust(t, e)
		x.nodes[code] = facility
		x.kinds[code] = r.Facility
		x.facilityOf[code] = code
	}
	for _, spec := range []struct {
		code, parent string
		kind         r.Kind
	}{
		{"S1", "A", r.MonitoringSection}, {"H1", "S1", r.Borehole}, {"P1", "H1", r.Point}, {"P2", "H1", r.Point},
		{"S1a", "S1", r.MonitoringSection}, {"P3", "S1a", r.Point}, {"C1", "A", r.Component}, {"S2", "C1", r.MonitoringSection}, {"H3", "S2", r.Borehole}, {"P6", "H3", r.Point},
		{"H2", "A", r.Borehole}, {"S3", "H2", r.MonitoringSection}, {"P4", "S3", r.Point}, {"P5", "A", r.Point}, {"S4", "B", r.MonitoringSection}, {"H4", "S4", r.Borehole}, {"P7", "H4", r.Point},
	} {
		root := x.facilityOf[spec.parent]
		extra := map[string]any{}
		if spec.kind == r.Component {
			extra["category_key"] = "span"
		}
		x.nodes[spec.code] = f.structureCreate(t, x.nodes[root], spec.kind, spec.code, r.StructureRef{Kind: x.kinds[spec.parent], ID: x.nodes[spec.parent].ID}, extra)
		x.kinds[spec.code] = spec.kind
		x.facilityOf[spec.code] = root
		if spec.kind == r.Point {
			ct := target(x.nodes[root], r.Channel, "")
			ct.Point = x.nodes[spec.code].ID
			channel, e := f.write(t, ct, "create", map[string]any{"code": "CH-" + spec.code, "name": "通道 " + spec.code, "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
			structureDisplayMust(t, e)
			x.channels[spec.code] = channel.ID
		}
	}
	for _, root := range []string{"A", "B"} {
		in := f.accessInput(t, x.p, x.nodes[root].ID, "create_link")
		for code, node := range x.nodes {
			if x.facilityOf[code] != root {
				continue
			}
			g := r.GrantInput{Kind: x.kinds[code], Capabilities: []string{"read"}}
			switch g.Kind {
			case r.Component:
				g.ComponentID = ptr(node.ID)
			case r.MonitoringSection:
				g.MonitoringSectionID = ptr(node.ID)
			case r.Borehole:
				g.BoreholeID = ptr(node.ID)
			case r.Point:
				g.PointID = ptr(node.ID)
			}
			in.Add = append(in.Add, g)
			if channel := x.channels[code]; channel != "" {
				in.Add = append(in.Add, r.GrantInput{Kind: r.Channel, ChannelID: ptr(channel), Capabilities: []string{"read"}})
			}
		}
		f.grant(t, x.p, x.nodes[root].ID, in)
	}
	for _, channel := range x.channels {
		var grant, revision string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, x.p.id, channel).Scan(&grant))
		must(t, f.env.Pool.QueryRow(ctx, `SELECT COALESCE((SELECT revision::text FROM monitoring_access_states WHERE tenant_id=$1 AND project_id=$2),'0')`, f.tenant, x.p.id).Scan(&revision))
		_, e := b.Monitoring.GrantData(ctx, f.actor, f.tenant, x.p.id, md.GrantInput{WriteMeta: md.WriteMeta{RequestID: uuid.NewString(), ExpectedRevision: revision, Reason: "SBM deterministic fixture"}, ChannelID: channel, ResourceGrantID: grant, Range: &md.Range{}, Capabilities: []string{"read", "history"}})
		structureDisplayMust(t, e)
	}
	return x
}

func (x *structureDisplayFixture) addDevices(t *testing.T) {
	t.Helper()
	f, b := x.resourceFixture, x.b
	meta := func() c.Meta { return c.Meta{RequestID: uuid.NewString(), Reason: "SBM device classification"} }
	admin := f.fixture.actor(f.p)
	content := cl.Content{DictionaryCode: cl.DictionaryCode, Nodes: []cl.Node{}}
	for _, code := range []string{"sensor", "displacement", "temperature", "environment", "collector"} {
		id := uuid.NewString()
		x.categories[code] = id
		n := cl.Node{ID: id, Code: code, Name: code, Selectable: true, Status: "active"}
		if code != "sensor" && code != "collector" {
			n.ParentID = x.categories["sensor"]
		}
		content.Nodes = append(content.Nodes, n)
	}
	content.SensorRootID = x.categories["sensor"]
	dictionary, e := b.Classification.Dictionary(ctx, admin)
	structureDisplayMust(t, e)
	saved, e := b.Classification.SaveDraft(ctx, admin, cl.SaveInput{Meta: meta(), Revision: dictionary.DraftRevision, Content: content}, access.Meta{})
	structureDisplayMust(t, e)
	publish := cl.PublishInput{Meta: meta(), Revision: saved.Revision}
	preview, e := b.Classification.PreviewPublish(ctx, admin, publish)
	structureDisplayMust(t, e)
	publish.PreviewHash = preview.Hash
	released, e := b.Classification.Publish(ctx, admin, publish, access.Meta{})
	structureDisplayMust(t, e)
	x.release = released.ReleaseID
	maker, e := b.DeviceCatalog.CreateManufacturer(ctx, admin, c.PlatformScope(), c.ManufacturerCreate{Meta: meta(), Code: "sbm-maker", Name: "SBM Manufacturer"}, access.Meta{})
	structureDisplayMust(t, e)
	product, e := b.DeviceCatalog.CreateProduct(ctx, admin, c.PlatformScope(), c.ProductCreate{Meta: meta(), ManufacturerID: maker.ID, Code: "sbm-product", Name: "SBM two-output device", Model: "SBM", OutputSlots: []c.Slot{{Key: "a", Name: "A", Mode: "fixed", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}, {Key: "b", Name: "B", Mode: "fixed", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}}}, access.Meta{})
	structureDisplayMust(t, e)
	product, e = b.DeviceCatalog.PublishClassified(ctx, admin, c.PlatformScope(), product.ID, c.DraftRevision{Meta: meta(), Revision: "1"}, cl.Selection{Mode: "classify", ReleaseID: x.release, CategoryID: x.categories["displacement"]}, access.Meta{})
	structureDisplayMust(t, e)
	uncategorized, e := b.DeviceCatalog.CreateProduct(ctx, admin, c.PlatformScope(), c.ProductCreate{Meta: meta(), ManufacturerID: maker.ID, Code: "sbm-uncategorized", Name: "Unclassified device", Model: "SBM-U", OutputSlots: []c.Slot{{Key: "a", Name: "A", Mode: "fixed", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}, {Key: "b", Name: "B", Mode: "fixed", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}}}, access.Meta{})
	structureDisplayMust(t, e)
	uncategorized, e = b.DeviceCatalog.PublishClassified(ctx, admin, c.PlatformScope(), uncategorized.ID, c.DraftRevision{Meta: meta(), Revision: "1"}, cl.Selection{Mode: "unclassified"}, access.Meta{})
	structureDisplayMust(t, e)
	outputs := [][]d.Output{}
	for i, category := range []string{"", "", "temperature", "environment", "collector", "unclassified"} {
		version := product.VersionID
		if category == "unclassified" {
			version = uncategorized.VersionID
		}
		device, e := b.Devices.Create(ctx, f.actor, d.TenantScope(f.tenant), d.DeviceCreate{Meta: meta(), Code: fmt.Sprintf("V%d", i+1), Name: fmt.Sprintf("设备 V%d", i+1), ProductVersionID: version}, access.Meta{})
		structureDisplayMust(t, e)
		x.devices = append(x.devices, device)
		slots, e := b.Devices.Slots(ctx, f.actor, d.TenantScope(f.tenant), device.ID, d.Filter{})
		structureDisplayMust(t, e)
		outputs = append(outputs, []d.Output{*slots.Items[0].Current, *slots.Items[1].Current})
		if category != "" && category != "unclassified" {
			target, e := b.Classification.Target(ctx, f.actor, cl.TenantScope(f.tenant), "device", device.ID)
			structureDisplayMust(t, e)
			choice := cl.Selection{Mode: "classify", ReleaseID: x.release, CategoryID: x.categories[category]}
			if category == "unclassified" {
				choice = cl.Selection{Mode: "unclassified"}
			}
			in := cl.BatchInput{Meta: meta(), Targets: []cl.Target{target.Target}, Selection: choice}
			preview, e := b.Classification.PreviewBatch(ctx, f.actor, cl.TenantScope(f.tenant), in)
			structureDisplayMust(t, e)
			in.PreviewID = preview.ID
			in.PreviewHash = preview.Hash
			_, e = b.Classification.ApplyBatch(ctx, f.actor, cl.TenantScope(f.tenant), in, access.Meta{})
			structureDisplayMust(t, e)
		}
	}
	for _, binding := range []struct {
		point        string
		device, slot int
	}{{"P1", 0, 0}, {"P2", 0, 1}, {"P3", 1, 0}, {"P7", 1, 1}, {"P6", 2, 0}, {"P4", 4, 0}} {
		out := outputs[binding.device][binding.slot]
		// Binding a sibling output changes the device revision; read the current basis.
		detail, e := b.Devices.Detail(ctx, f.actor, d.TenantScope(f.tenant), x.devices[binding.device].ID)
		structureDisplayMust(t, e)
		cmd := src.Bind(src.BindInput{Revisions: src.Revisions{Meta: meta(), SourceRevision: "0", ChannelRevision: "1"}, NewEnd: src.NewEnd{OutputID: out.ID, NewOutputRevision: out.Revision, NewDeviceRevision: detail.Revision}})
		target := src.Target{FacilityID: x.nodes[x.facilityOf[binding.point]].ID, PointID: x.nodes[binding.point].ID, ChannelID: x.channels[binding.point]}
		preview, e := b.Sources.Preview(ctx, f.actor, d.TenantScope(f.tenant), target, cmd)
		structureDisplayMust(t, e)
		cmd.PreviewHash = preview.Hash
		_, e = b.Sources.Mutate(ctx, f.actor, d.TenantScope(f.tenant), target, cmd, access.Meta{})
		structureDisplayMust(t, e)
	}
	grants, e := b.Devices.Grants(ctx, f.actor, f.tenant, x.p.id, d.Filter{})
	structureDisplayMust(t, e)
	in := d.GrantChangeSet{Meta: meta(), AccessRevision: grants.AccessRevision, ProjectRevision: grants.ProjectRevision}
	for i, device := range x.devices {
		in.Add = append(in.Add, d.GrantAdd{Kind: "device", ID: device.ID})
		for _, out := range outputs[i] {
			in.Add = append(in.Add, d.GrantAdd{Kind: "output", ID: out.ID})
		}
	}
	grantPreview, e := b.Devices.PreviewGrants(ctx, f.actor, f.tenant, x.p.id, in)
	structureDisplayMust(t, e)
	in.PreviewHash = grantPreview.Hash
	_, e = b.Devices.ChangeGrants(ctx, f.actor, f.tenant, x.p.id, in, access.Meta{})
	structureDisplayMust(t, e)
}

func (x *structureDisplayFixture) display(t *testing.T, devices bool, additionalChannels ...string) {
	t.Helper()
	scopes := []dr.Definition{{ID: "resources", StructureModel: 2, Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "resource.position", "statistics.counts"}}, {ID: "channels", StructureModel: 2, Domain: "channels", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "source.binding", "monitoring.values", "statistics.counts"}}}
	if len(additionalChannels) > 0 {
		ids := append([]string{}, additionalChannels...)
		for _, id := range x.channels {
			ids = append(ids, id)
		}
		scopes[1].Selector = dr.Selector{Kind: "ids", IDs: ids}
	}
	if devices {
		scopes = append(scopes, dr.Definition{ID: "devices", StructureModel: 2, Domain: "devices", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"device.metadata", "device.outputs", "device.installations", "device.classification", "statistics.counts", "statistics.classification"}})
	}
	views := map[string]dr.Definition{}
	for _, scope := range scopes {
		v := scope
		v.ScopeID = v.ID
		v.ID = ""
		views[v.ScopeID] = v
	}
	x.h = createReadDisplay(t, x.resourceFixture, x.p, x.b, scopes, views, []string{"views.read", "queries.read", "resources.directory", "resources.detail", "statistics.resources", "statistics.structure", "statistics.devices", "classification.categories", "statistics.deviceCategories", "devices.read", "resources.hierarchy", "statistics.monitoring", "monitoring.details", "channels.read"})
}

func TestDisplayStructureCanonicalCountsAndClassification(t *testing.T) {
	f := resourcesSetup(t)
	x := structureDisplaySetup(t, f, runtime.Compose(displayReadPlanAccess(t, f)))
	x.addDevices(t)
	x.display(t, true)
	rt, ct, dt := dr.Target{View: "resources"}, dr.Target{View: "channels"}, dr.Target{View: "devices"}
	query := dr.StructureStatisticsRequest{ReadFields: dr.ReadFields{FieldGroups: []string{"statistics.counts", "resource.metadata"}}, Target: rt, ChannelTarget: &ct, GroupBy: "monitoring_section", Membership: "nearest"}
	structureResult := readCall(t, x.h, "statistics.structure", query)
	statistics := readData[dr.StructureStatistics](t, structureResult)
	if statistics.Totals.FacilityCount != "2" || statistics.Totals.ComponentCount != "1" || statistics.Totals.MonitoringSectionCount != "5" || statistics.Totals.BoreholeCount != "4" || statistics.Totals.PointCount != "7" || statistics.Totals.SectionTypeCount != "2" || statistics.Totals.ChannelCount == nil || *statistics.Totals.ChannelCount != "7" {
		t.Fatalf("9.5 totals: %+v", statistics)
	}
	expected := map[string]string{x.nodes["S1"].ID: "2", x.nodes["S1a"].ID: "1", x.nodes["S2"].ID: "1", x.nodes["S3"].ID: "1", x.nodes["S4"].ID: "1", "unassigned": "1"}
	for _, group := range statistics.Groups {
		if group.Totals.PointCount != expected[group.Key] {
			t.Fatalf("nearest section: %+v", group)
		}
	}
	query.Membership = "subtree"
	subtree := readData[dr.StructureStatistics](t, readCall(t, x.h, "statistics.structure", query))
	if subtree.Additivity != "overlapping" {
		t.Fatal(subtree)
	}
	for _, group := range subtree.Groups {
		if group.Key == x.nodes["S1"].ID && (group.Totals.PointCount != "3" || group.Descendants.PointCount != "3" || group.Self.MonitoringSectionCount != "1") {
			t.Fatal(group)
		}
	}
	query.GroupBy = "borehole"
	query.Membership = "nearest"
	holes := readData[dr.StructureStatistics](t, readCall(t, x.h, "statistics.structure", query))
	for _, group := range holes.Groups {
		want := "1"
		if group.Key == x.nodes["H1"].ID || group.Key == "unassigned" {
			want = "2"
		}
		if group.Totals.PointCount != want {
			t.Fatal(group)
		}
	}
	query.GroupBy = "facility_type"
	types := readData[dr.StructureStatistics](t, readCall(t, x.h, "statistics.structure", query))
	for _, group := range types.Groups {
		want := "4"
		if group.Label == "隧道" {
			want = "1"
		}
		if group.Totals.MonitoringSectionCount != want {
			t.Fatal(group)
		}
	}
	if types.Applicability["borehole"]["applicable"] != "2" {
		t.Fatal(types.Applicability)
	}
	dictionary := readData[dr.CategoryPage](t, readCall(t, x.h, "classification.categories", dr.CategoriesRequest{}))
	if dictionary.Total != "5" || dictionary.DictionaryReleaseID != x.release {
		t.Fatal(dictionary)
	}
	for _, population := range []string{"authorized", "monitoring_bound"} {
		stats := readData[dr.DeviceCategoryStatistics](t, readCall(t, x.h, "statistics.deviceCategories", dr.DeviceCategoryStatisticsRequest{Target: dt, ChannelTarget: &ct, Population: population, Mode: "direct"}))
		want := dr.CategoryCounts{UniqueDeviceCount: "6", SensorDeviceCount: "4", UsedSensorCategoryCount: "3", NonSensorDeviceCount: "1", UnclassifiedDeviceCount: "1", UnavailableDeviceCount: "0"}
		if population == "monitoring_bound" {
			want.UniqueDeviceCount = "4"
			want.SensorDeviceCount = "3"
			want.UsedSensorCategoryCount = "2"
			want.UnclassifiedDeviceCount = "0"
		}
		if stats.Totals != want {
			t.Fatalf("%s: %+v", population, stats)
		}
		if bytes.Contains(dr.Bytes(stats), []byte(x.devices[0].ID)) {
			t.Fatal("summary leaked device membership")
		}
		bySection := readData[dr.DeviceStatistics](t, readCall(t, x.h, "statistics.devices", dr.DeviceStatisticsRequest{Target: dt, ChannelTarget: &ct, ResourceTarget: &rt, Population: population, Relation: "source", GroupBy: "monitoring_section", Membership: "nearest"}))
		if bySection.Totals.UniqueDeviceCount != want.UniqueDeviceCount || bySection.Additivity != "overlapping" {
			t.Fatal(bySection)
		}
		for _, g := range bySection.Groups {
			want := "1"
			if g.Key == "unassigned" {
				want = "2"
				if population == "monitoring_bound" {
					want = "0"
				}
			}
			if g.Totals.UniqueDeviceCount != want {
				t.Fatal("source devices not deduplicated by section", g)
			}
		}
	}
	_, e := x.h.s.Query(ctx, x.h.session, display.Call{Method: "statistics.deviceCategories", Params: dr.Bytes(dr.DeviceCategoryStatisticsRequest{Target: dt, Population: "authorized", Mode: "frontier", FrontierIDs: []string{x.categories["sensor"], x.categories["displacement"]}}), Generation: "read-1"})
	if e == nil {
		t.Fatal("overlapping category frontier accepted")
	}
	// Pinned category and structure selections survive ordinary metadata changes.
	oldCategory := readCall(t, x.h, "statistics.deviceCategories", dr.DeviceCategoryStatisticsRequest{Target: dt, Population: "authorized", Mode: "direct"})
	selectedCategory := readData[dr.DrilldownSelection](t, readCall(t, x.h, "queries.drilldown", dr.DrilldownRequest{QueryID: oldCategory.Query.ID, GroupKey: x.categories["displacement"], Basis: "device_count", Purpose: "devices"}))
	selectedStructure := readData[dr.DrilldownSelection](t, readCall(t, x.h, "queries.drilldown", dr.DrilldownRequest{QueryID: structureResult.Query.ID, GroupKey: x.nodes["S1a"].ID, Basis: "resource_count", Purpose: "resources"}))
	targetClass, e := x.b.Classification.Target(ctx, f.actor, cl.TenantScope(f.tenant), "device", x.devices[1].ID)
	structureDisplayMust(t, e)
	change := cl.BatchInput{Meta: c.Meta{RequestID: uuid.NewString(), Reason: "Verify fixed classification snapshot"}, Targets: []cl.Target{targetClass.Target}, Selection: cl.Selection{Mode: "classify", ReleaseID: x.release, CategoryID: x.categories["temperature"]}}
	changePreview, e := x.b.Classification.PreviewBatch(ctx, f.actor, cl.TenantScope(f.tenant), change)
	structureDisplayMust(t, e)
	change.PreviewID = changePreview.ID
	change.PreviewHash = changePreview.Hash
	_, e = x.b.Classification.ApplyBatch(ctx, f.actor, cl.TenantScope(f.tenant), change, access.Meta{})
	structureDisplayMust(t, e)
	move := f.moveInput(t, x.nodes["A"], r.Point, x.nodes["P3"].ID, r.StructureRef{Kind: r.MonitoringSection, ID: x.nodes["S1"].ID})
	mp, e := f.srv.PreviewMove(ctx, f.actor, r.TenantScope(f.tenant), target(x.nodes["A"], r.Point, x.nodes["P3"].ID), move)
	structureDisplayMust(t, e)
	move.PreviewHash = mp.Hash
	_, e = f.srv.Move(ctx, f.actor, r.TenantScope(f.tenant), target(x.nodes["A"], r.Point, x.nodes["P3"].ID), move, access.Meta{})
	structureDisplayMust(t, e)
	oldDevices := readData[dr.Page[dr.DeviceDetail]](t, readCall(t, x.h, "devices.list", dr.DevicesRequest{Target: dr.Target{Selection: selectedCategory.Selection}, ReadFields: dr.ReadFields{FieldGroups: []string{"device.metadata", "device.classification"}}}))
	if oldDevices.Total != "2" {
		t.Fatal(oldDevices)
	}
	for _, device := range oldDevices.Items {
		if device.Classification == nil || device.Classification.CategoryID != x.categories["displacement"] {
			t.Fatal("classification changed inside fixed drilldown", device)
		}
	}
	oldPoints := readData[dr.Page[dr.ResourceSummary]](t, readCall(t, x.h, "resources.query", dr.ResourcesRequest{Target: dr.Target{Selection: selectedStructure.Selection}, Filters: dr.ResourceFilters{Kinds: []string{"point"}}}))
	if oldPoints.Total != "1" || oldPoints.Items[0].ID != x.nodes["P3"].ID || oldPoints.Items[0].Parent.ID != x.nodes["S1a"].ID {
		t.Fatal("structure path changed inside fixed drilldown", oldPoints)
	}
	query.GroupBy = "monitoring_section"
	newStructure := readData[dr.StructureStatistics](t, readCall(t, x.h, "statistics.structure", query))
	for _, g := range newStructure.Groups {
		if g.Key == x.nodes["S1"].ID && g.Totals.PointCount != "3" {
			t.Fatal("new query retained old path", g)
		}
	}
	newCategories := readData[dr.DeviceCategoryStatistics](t, readCall(t, x.h, "statistics.deviceCategories", dr.DeviceCategoryStatisticsRequest{Target: dt, Population: "authorized", Mode: "direct"}))
	for _, g := range newCategories.Groups {
		if g.Key == x.categories["displacement"] && g.Count != "1" {
			t.Fatal("new query retained old classification", g)
		}
	}
	// Dictionary and aggregate pages are real stored-query pages, not resampling.
	first := readCall(t, x.h, "classification.categories", dr.CategoriesRequest{Paging: dr.Paging{Limit: 2}})
	firstPage := readData[dr.CategoryPage](t, first)
	second := readData[dr.CategoryPage](t, readCall(t, x.h, "classification.categories", dr.CategoriesRequest{Paging: dr.Paging{Limit: 2, QueryID: first.Query.ID, Cursor: firstPage.NextCursor}}))
	if firstPage.Total != "5" || !firstPage.HasMore || len(second.Items) != 2 || second.Items[0].ID == firstPage.Items[0].ID {
		t.Fatal("dictionary paging", firstPage, second)
	}
	filtered := readData[dr.Page[dr.DeviceDetail]](t, readCall(t, x.h, "devices.list", dr.DevicesRequest{Target: dt, Filters: dr.DeviceFilters{CategoryGroup: "sensor"}}))
	if filtered.Total != "4" {
		t.Fatal("sensor filter", filtered)
	}
	// An old view keeps the legacy projection and cannot call full hierarchy.
	legacy := dr.Definition{ID: "legacy", Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts"}}
	legacyView := legacy
	legacyView.ID = ""
	legacyView.ScopeID = "legacy"
	lh := createReadDisplay(t, f, x.p, x.b, []dr.Definition{legacy}, map[string]dr.Definition{"legacy": legacyView}, []string{"views.read", "queries.read", "resources.directory", "resources.detail", "resources.hierarchy"})
	legacyData := readCall(t, lh, "resources.query", dr.ResourcesRequest{Target: dr.Target{View: "legacy"}})
	var legacyPage struct {
		dr.Page[dr.ResourceSummary]
		Projection string `json:"structure_projection"`
	}
	legacyPage = readData[struct {
		dr.Page[dr.ResourceSummary]
		Projection string `json:"structure_projection"`
	}](t, legacyData)
	if legacyPage.Total != "17" || legacyPage.Projection != "legacy" {
		t.Fatal(legacyPage)
	}
	_, e = lh.s.Query(ctx, lh.session, display.Call{Method: "resources.hierarchy", Params: dr.Bytes(dr.ResourceTreeRequest{Target: dr.Target{View: "legacy"}, Mode: "root"}), Generation: "read-1"})
	if e == nil || !bytes.Contains(dr.Bytes(e), []byte("structure_model_upgrade_required")) {
		t.Fatal("legacy full hierarchy allowed", e)
	}
	// A statistics-only device view cannot reveal a member list or a drilldown.
	counts := dr.Definition{ID: "counts", Domain: "devices", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"statistics.counts", "statistics.classification"}}
	countsView := counts
	countsView.ID = ""
	countsView.ScopeID = "counts"
	ch := createReadDisplay(t, f, x.p, x.b, []dr.Definition{counts}, map[string]dr.Definition{"counts": countsView}, []string{"views.read", "queries.read", "statistics.deviceCategories", "devices.read"})
	summary := readCall(t, ch, "statistics.deviceCategories", dr.DeviceCategoryStatisticsRequest{Target: dr.Target{View: "counts"}, Population: "authorized", Mode: "direct"})
	_, e = ch.s.Query(ctx, ch.session, display.Call{Method: "devices.list", Params: dr.Bytes(dr.DevicesRequest{Target: dr.Target{View: "counts"}}), Generation: "read-1"})
	if e == nil {
		t.Fatal("summary-only view listed devices")
	}
	_, e = ch.s.Query(ctx, ch.session, display.Call{Method: "queries.drilldown", Params: dr.Bytes(dr.DrilldownRequest{QueryID: summary.Query.ID, Basis: "device_count", Purpose: "devices"}), Generation: "read-1"})
	if e == nil {
		t.Fatal("summary-only view drilled into devices")
	}
}

func structureDisplayMust(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatalf("%v: %s", e, dr.Bytes(e))
	}
}
