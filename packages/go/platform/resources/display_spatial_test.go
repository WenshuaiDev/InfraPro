//go:build spatialintegration || monitoringintegration

package resources_test

import (
	"encoding/base64"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"testing"
)

func TestDisplaySpatialFixedFilesAndPermission(t *testing.T) {
	f := spatialSetup(t)
	facility := f.facility(t, "DISPLAY-SPATIAL")
	asset := f.newAsset(t, facility.ID, "model", "DISPLAY-SPATIAL")
	version := f.newVersion(t, asset.ID, "", true)
	f.process(t, version, "bridge-v1.glb")
	v := f.publish(t, version.ID)
	p := f.project(t, "DisplaySpatial")
	g := f.accessInput(t, p, facility.ID, "create_link")
	g.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, g)
	f.grantSpatial(t, p, v.ID, false)
	b := runtime.Compose(f.env.Service.Access())
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", "contract-spatial", "spatial")
	must(t, e)
	b.Spatial.WithStorage(store)
	h := createDisplayHarness(t, f.resourceFixture, p, b.Displays, map[string]display.Binding{"model": {Kind: "spatial", IDs: []string{v.ID}}}, display.Grant{Capabilities: []string{"spatial.read"}, Fields: []string{"metadata", "properties"}, Scope: display.ReadScope{Spatial: []string{v.ID}}})
	for _, method := range []string{"spatial.list", "spatial.manifest", "spatial.objects", "spatial.placements"} {
		h.query(t, method, display.Query{Target: display.Target{Binding: "model"}})
	}
	var file string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT file_id FROM spatial_version_files WHERE tenant_id=$1 AND version_id=$2 LIMIT 1`, f.tenant, v.ID).Scan(&file))
	r := h.query(t, "spatial.readFile", display.Query{Target: display.Target{Binding: "model"}, FileID: file, Length: 8})
	var chunk struct {
		Base64 string `json:"base64"`
		Length string `json:"length"`
	}
	raw, _ := json.Marshal(r.Data)
	must(t, json.Unmarshal(raw, &chunk))
	data, e := base64.StdEncoding.DecodeString(chunk.Base64)
	must(t, e)
	if len(data) != 8 || chunk.Length != "8" {
		t.Fatal("wrong chunk", chunk)
	}
	q, _ := json.Marshal(display.Query{Target: display.Target{Binding: "model"}, FileID: "source"})
	if _, e = h.s.Query(ctx, h.session, display.Call{Method: "spatial.readFile", Params: q}); e == nil {
		t.Fatal("source download exposed")
	}
}
