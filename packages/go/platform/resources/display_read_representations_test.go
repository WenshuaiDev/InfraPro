//go:build spatialintegration || monitoringintegration

package resources_test

import (
	"bytes"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	sp "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
)

func TestDisplayReadSpatialRepresentationsAndRelationships(t *testing.T) {
	f := spatialSetup(t)
	facility := f.facility(t, "READ-REPRESENTATIONS")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P1", "name": "可读测点"})
	must(t, e)
	hidden, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "SECRET_POINT", "name": "隐藏测点"})
	must(t, e)
	detail, e := f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), target(facility, res.Facility, facility.ID))
	must(t, e)
	component, e := f.write(t, target(facility, res.Component, ""), "create", map[string]any{"code": "C1", "name": "可读构件", "category_key": "span", "expected_structure_revision": detail.StructureRevision})
	must(t, e)
	versions := map[string]sp.Version{}
	for _, sample := range []struct{ kind, file string }{{"layout", "bridge-layouts-v1.pdf"}, {"model", "bridge-v1.glb"}, {"geolayer", "reference.geojson"}} {
		asset := f.newAsset(t, facility.ID, sample.kind, sample.kind)
		version := f.newVersion(t, asset.ID, "", true)
		converted := f.process(t, version, sample.file)
		edit := sp.DraftInput{Meta: spatialMeta(), Revision: converted.Revision, Settings: &sp.Settings{SourceProperties: true}}
		switch sample.kind {
		case "layout":
			edit.Settings.PageID = "page-2"
			edit.Placements = []sp.Placement{{PointID: point.ID, LabelOffset: []float64{0, 0}, PageID: "page-1", Anchor: []float64{.3, .4}}, {PointID: hidden.ID, LabelOffset: []float64{0, 0}, PageID: "page-1", Anchor: []float64{.7, .8}}}
			edit.ComponentLinks = []sp.ComponentLink{{ComponentID: component.ID, PageID: "page-1", Region: [][]float64{{.1, .1}, {.4, .1}, {.4, .4}}}}
		case "model":
			elements, e := f.s.Elements(ctx, f.actor, f.scope, version.ID, sp.Filter{Limit: 1})
			must(t, e)
			if len(elements.Items) == 0 {
				t.Fatal("converted GLB missing elements")
			}
			key := elements.Items[0].Key
			edit.Settings.Camera = &sp.Camera{Position: []float64{10, 10, 10}, Target: []float64{0, 0, 0}}
			edit.Settings.HiddenElements = []string{key}
			edit.Placements = []sp.Placement{{PointID: point.ID, LabelOffset: []float64{0, 0}, ElementKey: key, Anchor: []float64{0, 0, 0}}}
			edit.ComponentLinks = []sp.ComponentLink{{ComponentID: component.ID, ElementKey: key}}
		case "geolayer":
			features, e := f.s.Features(ctx, f.actor, f.scope, version.ID, sp.Filter{Limit: 1})
			must(t, e)
			if len(features.Items) == 0 {
				t.Fatal("converted GeoJSON missing features")
			}
			edit.FeatureLinks = []sp.FeatureLink{{FeatureKey: features.Items[0].ID, FacilityID: facility.ID}}
		}
		_, e = f.s.EditDraft(ctx, f.actor, f.scope, version.ID, edit)
		must(t, e)
		versions[sample.kind] = f.publish(t, version.ID)
	}
	p := f.project(t, "ReadRepresentations")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Point, PointID: &point.ID, Capabilities: []string{"read"}}, {Kind: res.Component, ComponentID: &component.ID, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	for _, v := range versions {
		f.grantSpatial(t, p, v.ID, false)
	}
	b := runtime.Compose(displayReadPlanAccess(t, f.resourceFixture))
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", "contract-spatial", "spatial")
	must(t, e)
	b.Spatial.WithStorage(store)
	scopes := []dr.Definition{{ID: "spatial", Domain: "spatial", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"spatial.metadata", "spatial.geometry", "spatial.properties"}}, {ID: "resources", Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "resource.position"}}}
	views := map[string]dr.Definition{}
	for _, s := range scopes {
		v := s
		v.ID, v.ScopeID = "", s.ID
		views[s.ID] = v
	}
	h := createReadDisplay(t, f.resourceFixture, p, b, scopes, views, []string{"views.read", "queries.read", "spatial.catalog", "spatial.links", "spatial.read", "map.read"})
	st, rt := dr.Target{View: "spatial"}, dr.Target{View: "resources"}
	geometry := dr.ReadFields{FieldGroups: []string{"spatial.metadata", "spatial.geometry"}}
	for _, kind := range []string{"layout", "model"} {
		v := versions[kind]
		request := dr.SpatialVersionRequest{Target: st, ID: v.ID, ResourceTarget: &rt, ReadFields: geometry, Paging: dr.Paging{Limit: 1}}
		placements := readData[dr.Page[dr.SpatialPlacement]](t, readCall(t, h, "spatial.placements", request))
		if placements.Total != "1" || placements.Items[0].Point.ID != point.ID || bytes.Contains(dr.Bytes(placements), []byte(hidden.ID)) {
			t.Fatal("placement projection expanded business visibility", placements)
		}
		links := readData[dr.Page[dr.SpatialComponentLink]](t, readCall(t, h, "spatial.componentLinks", request))
		if links.Total != "1" || links.Items[0].Component.ID != component.ID {
			t.Fatal("component association lost", links)
		}
		settings := readData[dr.SpatialSettings](t, readCall(t, h, "spatial.settings", dr.SpatialSettingsRequest{Target: st, ID: v.ID, ResourceTarget: &rt, ReadFields: geometry}))
		if settings.HiddenElements == nil || settings.MainChannels == nil {
			t.Fatal("empty spatial setting lists must serialize as arrays", settings)
		}
		if kind == "layout" {
			pages := readData[dr.SpatialManifest](t, readCall(t, h, "spatial.manifest", request))
			if len(pages.Pages) != 3 || settings.PageID != "page-2" || len(links.Items[0].Region) != 3 {
				t.Fatal("drawing pages, regions or default page lost", pages, settings)
			}
		} else {
			objects := readData[dr.Page[dr.SpatialElement]](t, readCall(t, h, "spatial.objects", request))
			if objects.Total == "0" || settings.Camera == nil || len(settings.HiddenElements) != 1 || objects.Items[0].Properties != nil || objects.Items[0].TypeProperties != nil {
				t.Fatal("model geometry/settings/property separation lost", objects, settings)
			}
			request.FieldGroups = []string{"spatial.metadata", "spatial.geometry", "spatial.properties"}
			readCall(t, h, "spatial.objects", request)
		}
	}
	layer := versions["geolayer"]
	request := dr.MapFeaturesRequest{Target: st, ID: layer.ID, ReadFields: geometry, Paging: dr.Paging{Limit: 1}}
	features := readData[dr.Page[dr.MapFeature]](t, readCall(t, h, "map.features", request))
	if features.Total != "6" || !features.HasMore || bytes.Contains(dr.Bytes(features), []byte(facility.ID)) {
		t.Fatal("GeoJSON page or unrequested facility projection", features)
	}
	next := readData[dr.Page[dr.MapFeature]](t, readCall(t, h, "queries.get", dr.QueryRequest{QueryID: features.QueryID, Cursor: features.NextCursor}))
	if len(next.Items) != 1 || next.Items[0].ID == features.Items[0].ID {
		t.Fatal("GeoJSON pagination repeated features", next)
	}
}
