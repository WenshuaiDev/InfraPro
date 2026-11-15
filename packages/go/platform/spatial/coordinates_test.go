package spatial

import (
	"math"
	"testing"
)

func TestCoordinatesNeverInventOrClampPositions(t *testing.T) {
	lon, lat, e := Geographic("EPSG:3857", 12957588.728337044, 4851421.175)
	if e != nil || math.Abs(lon-116.4) > 1e-6 || math.Abs(lat-39.9) > 1e-6 {
		t.Fatal(lon, lat, e)
	}
	for _, test := range []struct {
		crs  string
		x, y float64
	}{{"unknown", 1, 1}, {"EPSG:4326", 180.01, 0}, {"EPSG:4326", 0, 86}, {"EPSG:3857", math.Inf(1), 0}, {"EPSG:3857", 0, 1e9}} {
		if _, _, e := Geographic(test.crs, test.x, test.y); e == nil {
			t.Fatal("unsupported/clipped position accepted", test)
		}
	}
	if !InBBox(179, 0, []float64{170, -10, -170, 10}) || !InBBox(-179, 0, []float64{170, -10, -170, 10}) || InBBox(0, 0, []float64{170, -10, -170, 10}) {
		t.Fatal("antimeridian viewport")
	}
}
func TestDrawingAnchorsAndIndependentLabels(t *testing.T) {
	asset := Asset{Kind: "layout"}
	v := Version{Manifest: Manifest{Pages: []Page{{ID: "page-1"}}}}
	p := Placement{PointID: "11111111-1111-4111-8111-111111111111", PageID: "page-1", Anchor: []float64{.5, .5}, LabelOffset: []float64{80, -20}}
	if e := validatePlacement(asset, v, p); e != nil {
		t.Fatal(e)
	}
	p.LabelOffset = []float64{200, 100}
	if e := validatePlacement(asset, v, p); e != nil || p.Anchor[0] != .5 {
		t.Fatal("label changed anchor", e)
	}
	p.PageID = "foreign-page"
	if validatePlacement(asset, v, p) == nil {
		t.Fatal("foreign page accepted")
	}
	p.PageID = "page-1"
	p.Anchor[0] = math.NaN()
	if validatePlacement(asset, v, p) == nil {
		t.Fatal("NaN accepted")
	}
}
func TestVersionMigrationDoesNotEquateNamesOrNodeIndices(t *testing.T) {
	c := []Candidate{{Placement: &Placement{ElementKey: "stable"}}, {Placement: &Placement{ElementKey: "gone"}}, {Placement: &Placement{ElementKey: "node:1"}}}
	old := Manifest{Elements: []Element{{Key: "stable", Name: "Beam", GeometryHash: "old"}, {Key: "gone", Name: "Pier"}, {Key: "node:1"}}}
	next := Manifest{Elements: []Element{{Key: "stable", Name: "Beam", GeometryHash: "new", Properties: map[string]any{"source_identity": true}}, {Key: "new-id", Name: "Pier"}, {Key: "node:1", Name: "Other"}}}
	result := classifyCandidates(c, old, next)
	if result[0].State != "needs_review" || result[1].State != "unmatched" || result[2].State != "uncertain" {
		t.Fatal(result)
	}
}
