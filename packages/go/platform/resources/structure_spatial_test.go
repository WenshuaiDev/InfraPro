//go:build spatialintegration || monitoringintegration

package resources_test

import (
	"bytes"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/location"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	sp "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
)

func TestStructureSpatialPublicationGeometryAndSDKIsolation(t *testing.T) {
	f := spatialSetup(t)
	facility := f.typedFacility(t)
	root := r.StructureRef{Kind: r.Facility, ID: facility.ID}
	section := f.structureCreate(t, facility, r.MonitoringSection, "S", root, nil)
	hole := f.structureCreate(t, facility, r.Borehole, "H", root, nil)
	point := f.structureCreate(t, facility, r.Point, "P", root, nil)
	hidden := f.structureCreate(t, facility, r.MonitoringSection, "SECRET", root, nil)
	component := f.structureCreate(t, facility, r.Component, "C", root, map[string]any{"category_key": "span"})
	project := f.project(t, "StructureSpatial")
	grant := f.accessInput(t, project, facility.ID, "create_link")
	grant.Add = []r.GrantInput{{Kind: r.Facility, Capabilities: []string{"read"}}, {Kind: r.MonitoringSection, MonitoringSectionID: &section.ID, Capabilities: []string{"read"}}, {Kind: r.Borehole, BoreholeID: &hole.ID, Capabilities: []string{"read"}}, {Kind: r.Point, PointID: &point.ID, Capabilities: []string{"read"}}, {Kind: r.Component, ComponentID: &component.ID, Capabilities: []string{"read"}}}
	f.grant(t, project, facility.ID, grant)
	versions := map[string]sp.Version{}
	for _, kind := range []string{"layout", "model"} {
		asset := f.newAsset(t, facility.ID, kind, "geometry_"+kind)
		version := f.newVersion(t, asset.ID, "", true)
		file := "bridge-layouts-v1.pdf"
		if kind == "model" {
			file = "bridge-v1.glb"
		}
		converted := f.process(t, version, file)
		page, anchor := "page-1", []float64{.25, .3}
		sectionGeometry := &location.AssociationGeometry{Kind: "region", Vertices: [][]float64{{.2, .2}, {.7, .2}, {.5, .6}}}
		path := [][]float64{{.2, .3}, {.4, .5}}
		element := ""
		if kind == "model" {
			page = ""
			anchor = []float64{0, 0, 0}
			sectionGeometry = &location.AssociationGeometry{Kind: "plane", Normal: []float64{0, 1, 0}, Size: []float64{10, 5}}
			path = [][]float64{{0, 0, 0}, {0, -1, 0}, {.2, -2, 0}}
			elements, err := f.s.Elements(ctx, f.actor, f.scope, version.ID, sp.Filter{Limit: 1})
			must(t, err)
			element = elements.Items[0].Key
		}
		base := sp.Placement{PageID: page, Anchor: anchor, ElementKey: element, LabelOffset: []float64{0, 0}}
		sectionPlacement := base
		sectionPlacement.TargetRef = &sp.ObjectRef{Kind: "monitoring_section", ID: section.ID}
		sectionPlacement.Geometry = sectionGeometry
		holePlacement := base
		holePlacement.TargetRef = &sp.ObjectRef{Kind: "borehole", ID: hole.ID}
		holePlacement.Geometry = &location.AssociationGeometry{Kind: "path", Vertices: path}
		pointPlacement := base
		pointPlacement.PointID = point.ID
		hiddenPlacement := base
		hiddenPlacement.TargetRef = &sp.ObjectRef{Kind: "monitoring_section", ID: hidden.ID}
		hiddenPlacement.Geometry = sectionGeometry
		link := sp.ComponentLink{ComponentID: component.ID, PageID: page, ElementKey: element}
		if kind == "layout" {
			link.Region = [][]float64{{.1, .1}, {.2, .1}, {.2, .2}}
		}
		_, err := f.s.EditDraft(ctx, f.actor, f.scope, version.ID, sp.DraftInput{Meta: spatialMeta(), Revision: converted.Revision, Placements: []sp.Placement{sectionPlacement, holePlacement, pointPlacement, hiddenPlacement}, ComponentLinks: []sp.ComponentLink{link}})
		must(t, err)
		versions[kind] = f.publish(t, version.ID)
		f.grantSpatial(t, project, version.ID, false)
		pageResult, err := f.s.Placements(ctx, project.actor, sp.Project(f.tenant, project.id), version.ID, sp.Filter{})
		must(t, err)
		if len(pageResult.Items) != 3 || bytes.Contains(dr.Bytes(pageResult), []byte(hidden.ID)) {
			t.Fatal("spatial grant expanded engineering visibility", pageResult)
		}
		for _, p := range pageResult.Items {
			if p.TargetRef != nil && p.TargetRef.Kind != "point" && (p.PointID != "" || p.Geometry == nil) {
				t.Fatal("typed geometry or legacy point contract lost", p)
			}
		}
		_, err = f.env.Pool.Exec(ctx, `UPDATE spatial_point_placements SET geometry=NULL WHERE version_id=$1 AND target_kind='monitoring_section'`, version.ID)
		if err == nil {
			t.Fatal("rewrote immutable published association")
		}
		copy := f.newVersion(t, asset.ID, version.ID, false)
		draft, err := f.s.Draft(ctx, f.actor, f.scope, copy.ID)
		must(t, err)
		if len(draft.Placements) != 4 || len(draft.ComponentLinks) != 1 {
			t.Fatal(draft)
		}
		geometryCount := 0
		for _, p := range draft.Placements {
			if p.Geometry != nil {
				geometryCount++
			}
		}
		if geometryCount != 3 {
			t.Fatal("copy lost known geometry")
		}
	}
	b := runtime.Compose(displayReadPlanAccess(t, f.resourceFixture))
	store, err := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", "contract-spatial", "spatial")
	must(t, err)
	b.Spatial.WithStorage(store)
	scopes := []dr.Definition{{ID: "resources", Domain: "resources", StructureModel: 2, Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "resource.position"}}, {ID: "spatial", Domain: "spatial", StructureModel: 2, Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"spatial.metadata", "spatial.geometry"}}}
	views := map[string]dr.Definition{}
	for _, s := range scopes {
		v := s
		v.ID = ""
		v.ScopeID = s.ID
		views[s.ID] = v
	}
	h := createReadDisplay(t, f.resourceFixture, project, b, scopes, views, []string{"views.read", "queries.read", "spatial.catalog", "spatial.links", "spatial.read", "resources.directory"})
	st, rt := dr.Target{View: "spatial"}, dr.Target{View: "resources"}
	for kind, v := range versions {
		result := readData[dr.Page[dr.SpatialPlacement]](t, readCall(t, h, "spatial.placements", dr.SpatialVersionRequest{Target: st, ID: v.ID, ResourceTarget: &rt}))
		if result.Total != "3" || bytes.Contains(dr.Bytes(result), []byte(hidden.ID)) {
			t.Fatal(result)
		}
		for _, p := range result.Items {
			if p.Target.Kind == "monitoring_section" && (p.Point != nil || p.Geometry == nil || p.Geometry.Kind != map[string]string{"layout": "region", "model": "plane"}[kind]) {
				t.Fatal(p)
			}
		}
		old := readData[dr.Page[dr.SpatialComponentLink]](t, readCall(t, h, "spatial.componentLinks", dr.SpatialVersionRequest{Target: st, ID: v.ID, ResourceTarget: &rt}))
		if old.Total != "1" {
			t.Fatal("old component link lost", old)
		}
	}
	locations := readData[dr.Page[dr.SpatialLocation]](t, readCall(t, h, "spatial.locations", dr.SpatialLocationsRequest{Target: rt, SpatialTarget: st, ID: hole.ID, Kind: "borehole"}))
	if locations.Total != "2" {
		t.Fatal(locations)
	}
	for _, v := range locations.Items {
		if v.Geometry == nil || v.Geometry.Kind != "path" {
			t.Fatal(v)
		}
	}
	// Revocation invalidates an already materialized SDK result, including its
	// geometry, rather than leaving a cached marker behind.
	old := readCall(t, h, "spatial.placements", dr.SpatialVersionRequest{Target: st, ID: versions["layout"].ID, ResourceTarget: &rt})
	end := f.accessInput(t, project, facility.ID, "change_grants")
	var grantID, grantRevision string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text,revision::text FROM project_resource_grants WHERE project_id=$1 AND monitoring_section_id=$2 AND ended_at IS NULL`, project.id, section.ID).Scan(&grantID, &grantRevision))
	end.End = []r.GrantEnd{{ID: grantID, Revision: grantRevision}}
	f.grant(t, project, facility.ID, end)
	_, err = h.s.Query(ctx, h.session, display.Call{Method: "queries.get", Params: dr.Bytes(dr.QueryRequest{QueryID: old.Query.ID})})
	if err == nil {
		t.Fatal("revoked section geometry survived cached delivery")
	}
}
