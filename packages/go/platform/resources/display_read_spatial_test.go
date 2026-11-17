//go:build spatialintegration || monitoringintegration

package resources_test

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	sp "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
)

func TestDisplayReadSpatial101LocationsFixedVersionAndFiles(t *testing.T) {
	f := spatialSetup(t)
	facility := f.facility(t, "READ-SPATIAL")
	asset := f.newAsset(t, facility.ID, "model", "MODEL")
	version := f.newVersion(t, asset.ID, "", true)
	f.process(t, version, "bridge-v1.glb")
	v := f.publish(t, version.ID)
	versions := []string{v.ID}
	for i := 1; i < 101; i++ {
		next := f.newVersion(t, asset.ID, v.ID, false)
		versions = append(versions, f.publish(t, next.ID).ID)
	}
	p := f.project(t, "ReadSpatial")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	state, e := f.s.Access(ctx, f.actor, f.scope, p.id)
	must(t, e)
	grants := sp.AccessInput{Meta: spatialMeta(), Revision: state.Revision, Add: []sp.GrantChange{}, End: []string{}}
	for i, id := range versions {
		grants.Add = append(grants.Add, sp.GrantChange{VersionID: id, Default: i == 100})
	}
	preview, e := f.s.PreviewAccess(ctx, f.actor, f.scope, p.id, grants)
	must(t, e)
	grants.PreviewHash = preview.Hash
	_, e = f.s.SaveAccess(ctx, f.actor, f.scope, p.id, grants)
	must(t, e)
	b := runtime.Compose(displayReadPlanAccess(t, f.resourceFixture))
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", "contract-spatial", "spatial")
	must(t, e)
	b.Spatial.WithStorage(store)
	scopes := []dr.Definition{{ID: "spatial", Domain: "spatial", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"spatial.metadata", "spatial.geometry", "spatial.properties"}}, {ID: "resources", Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "resource.position"}}}
	views := map[string]dr.Definition{}
	for _, s := range scopes {
		v := s
		v.ScopeID = s.ID
		v.ID = ""
		views[s.ID] = v
	}
	h := createReadDisplay(t, f.resourceFixture, p, b, scopes, views, []string{"views.read", "queries.read", "spatial.catalog", "spatial.links", "spatial.read", "map.read"})
	preparation, e := h.s.Prepare(ctx, p.actor, h.scope, h.id, display.Command{Meta: display.Meta{Revision: h.revision}, ConfigID: h.config})
	must(t, e)
	for _, view := range preparation.ReadViews {
		if view.Name == "spatial" {
			if view.DefaultVersions == nil || view.DefaultVersions.Total != "1" || len(view.DefaultVersions.Items) != 1 || view.DefaultVersions.Items[0].VersionID != versions[100] {
				t.Fatal("prepare default version missing", view)
			}
		}
	}
	sTarget, rTarget := dr.Target{View: "spatial"}, dr.Target{View: "resources"}
	assets := readData[dr.Page[dr.SpatialAsset]](t, readCall(t, h, "spatial.assets", dr.SpatialAssetsRequest{Target: sTarget, ResourceTarget: &rTarget}))
	if len(assets.Items) != 1 || assets.Items[0].VersionID != versions[100] || assets.Items[0].VersionCount != "101" {
		t.Fatalf("asset default: %+v", assets)
	}
	query := dr.SpatialLocationsRequest{Target: rTarget, SpatialTarget: sTarget, ID: facility.ID, Paging: dr.Paging{Limit: 50}}
	seen := map[string]bool{}
	for {
		result := readCall(t, h, "spatial.locations", query)
		page := readData[dr.Page[dr.SpatialLocation]](t, result)
		if page.Total != "101" {
			t.Fatal("locations truncated", page.Total)
		}
		for _, item := range page.Items {
			if seen[item.VersionID] {
				t.Fatal("duplicate location")
			}
			seen[item.VersionID] = true
		}
		if !page.HasMore {
			break
		}
		query.QueryID, query.Cursor = page.QueryID, page.NextCursor
	}
	if len(seen) != 101 {
		t.Fatal("missing 101st location", len(seen))
	}
	metadata := readCall(t, h, "spatial.manifest", dr.SpatialVersionRequest{Target: sTarget, ID: v.ID})
	for _, field := range []string{`"coordinates"`, `"files"`, `"pages"`} {
		if bytes.Contains(dr.Bytes(metadata.Data), []byte(field)) {
			t.Fatal("unrequested geometry", field)
		}
	}
	manifest := readData[dr.SpatialManifest](t, readCall(t, h, "spatial.manifest", dr.SpatialVersionRequest{Target: sTarget, ID: v.ID, ReadFields: dr.ReadFields{FieldGroups: []string{"spatial.metadata", "spatial.geometry"}}}))
	if manifest.Coordinates == nil || len(manifest.Files) == 0 || manifest.ID != v.ID {
		t.Fatal("fixed geometry missing")
	}
	chunk := readData[dr.FileChunk](t, readCall(t, h, "spatial.readFile", dr.SpatialFileRequest{Target: sTarget, ID: v.ID, FileID: manifest.Files[0].ID, Length: 8}))
	content, e := base64.StdEncoding.DecodeString(chunk.Base64)
	must(t, e)
	if len(content) != 8 {
		t.Fatal("file chunk", fmt.Sprint(chunk))
	}
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "spatial.readFile", Params: dr.Bytes(dr.SpatialFileRequest{Target: sTarget, ID: v.ID, FileID: "source"})})
	if e == nil {
		t.Fatal("source exposed")
	}
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "spatial.objects", Params: dr.Bytes(dr.SpatialVersionRequest{Target: sTarget, ID: v.ID, ReadFields: dr.ReadFields{FieldGroups: []string{"spatial.metadata", "spatial.geometry", "spatial.properties"}}})})
	if !v.Settings.SourceProperties && e == nil {
		t.Fatal("business property restriction bypassed")
	}
	basemaps := readData[struct {
		Items []dr.Basemap `json:"items"`
	}](t, readCall(t, h, "map.basemaps", dr.EmptyRequest{}))
	if len(basemaps.Items) < 2 || basemaps.Items[1].ID != "local" || basemaps.Items[1].Attribution == "" {
		t.Fatal("local basemap lost synthetic attribution")
	}
	tile := readData[dr.FileChunk](t, readCall(t, h, "map.readTile", dr.TileRequest{BasemapID: "local", Z: 0, X: 0, Y: 0}))
	if tile.MediaType != "image/png" || tile.Base64 == "" {
		t.Fatal("local tile unavailable")
	}
	for _, q := range []dr.TileRequest{{BasemapID: "https://invalid.example/private"}, {BasemapID: "local", Z: 0, X: 1}} {
		if _, e = h.s.Query(ctx, h.session, display.Call{Method: "map.readTile", Params: dr.Bytes(q)}); e == nil {
			t.Fatal("invalid tile accepted")
		}
	}
	// Real blob client receives an HTTP storage failure; no data/empty response
	// may escape, even though a preceding request read the same artifact.
	failure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer failure.Close()
	unavailable, e := blobstore.Open(failure.URL, "test", "test-only", "contract-spatial", "spatial")
	must(t, e)
	b.Spatial.WithStorage(unavailable)
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "spatial.readFile", Params: dr.Bytes(dr.SpatialFileRequest{Target: sTarget, ID: v.ID, FileID: manifest.Files[0].ID, Length: 8})})
	var problem *dr.Error
	if !errors.As(e, &problem) || problem.Code != "dependency_unavailable" {
		t.Fatal("storage failure misreported", e)
	}
	b.Spatial.WithStorage(store)
	h.command(t, "grant", "", display.Command{Grant: &display.Grant{Revoked: true}})
	if _, e = h.s.Query(ctx, h.session, display.Call{Method: "spatial.readFile", Params: dr.Bytes(dr.SpatialFileRequest{Target: sTarget, ID: v.ID, FileID: manifest.Files[0].ID, Length: 8})}); e == nil {
		t.Fatal("file read survived grant revocation")
	}
}
