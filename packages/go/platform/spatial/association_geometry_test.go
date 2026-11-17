package spatial

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/location"
	"math"
	"testing"
)

func TestSectionAndBoreholeGeometryKeepsExplicitCoordinates(t *testing.T) {
	for _, tc := range []struct {
		name, asset, target string
		geometry            location.AssociationGeometry
		valid               bool
	}{
		{"drawing_region", "layout", "monitoring_section", location.AssociationGeometry{Kind: "region", Vertices: [][]float64{{0, 0}, {1, 0}, {1, 1}}}, true},
		{"model_region", "model", "monitoring_section", location.AssociationGeometry{Kind: "region", Vertices: [][]float64{{0, 0, 0}, {10, 0, 0}, {10, 10, 0}}}, true},
		{"model_plane", "model", "monitoring_section", location.AssociationGeometry{Kind: "plane", Normal: []float64{0, 0, 1}, Size: []float64{10, 5}}, true},
		{"known_path", "model", "borehole", location.AssociationGeometry{Kind: "path", Vertices: [][]float64{{0, 0, 0}, {0, 0, -10}}}, true},
		{"no_invented_surface", "layout", "monitoring_section", location.AssociationGeometry{Kind: "region", Vertices: [][]float64{{0, 0}, {.5, .5}, {1, 1}}}, false},
		{"finite_path", "model", "borehole", location.AssociationGeometry{Kind: "path", Vertices: [][]float64{{0, 0, 0}, {0, 0, math.Inf(1)}}}, false},
		{"plane_normal", "model", "monitoring_section", location.AssociationGeometry{Kind: "plane", Normal: []float64{0, 0, 0}, Size: []float64{10, 5}}, false},
		{"no_point_region", "layout", "point", location.AssociationGeometry{Kind: "region", Vertices: [][]float64{{0, 0}, {1, 0}, {1, 1}}}, false},
		{"no_clipping", "layout", "borehole", location.AssociationGeometry{Kind: "path", Vertices: [][]float64{{0, 0}, {2, 1}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Placement{TargetRef: &ObjectRef{Kind: tc.target}, Geometry: &tc.geometry}
			if err := validateAssociationGeometry(Asset{Kind: tc.asset}, p); (err == nil) != tc.valid {
				t.Fatal(err)
			}
		})
	}
	vertices := make([][]float64, 513)
	for i := range vertices {
		vertices[i] = []float64{0, 0, float64(i)}
	}
	if validateAssociationGeometry(Asset{Kind: "model"}, Placement{TargetRef: &ObjectRef{Kind: "borehole"}, Geometry: &location.AssociationGeometry{Kind: "path", Vertices: vertices}}) == nil {
		t.Fatal("path budget silently truncated")
	}
}
