package spatial

import "math"

func validateAssociationGeometry(a Asset, p Placement) error {
	g := p.Geometry
	if g == nil {
		return nil
	}
	kind := placementRef(p).Kind
	bad := func() error {
		return invalid("/placements/geometry", "断面区域须为 3–64 个非共线顶点，孔位路径为 2–512 个顶点；模型平面须提供非零法向和两个正尺寸。仅采用明确提供的资料坐标。")
	}
	if kind == "point" {
		return bad()
	}
	if g.Kind == "plane" {
		if kind != "monitoring_section" || a.Kind != "model" || len(g.Vertices) > 0 || !vector(g.Normal, 3, false) || !vector(g.Size, 2, false) {
			return bad()
		}
		norm := math.Hypot(math.Hypot(g.Normal[0], g.Normal[1]), g.Normal[2])
		if norm < 1e-12 || math.IsInf(norm, 0) || g.Size[0] <= 0 || g.Size[1] <= 0 || g.Size[0] > 1e9 || g.Size[1] > 1e9 {
			return bad()
		}
		return nil
	}
	if len(g.Normal) > 0 || len(g.Size) > 0 {
		return bad()
	}
	min, max := 3, 64
	if g.Kind == "path" && kind == "borehole" {
		min, max = 2, 512
	} else if g.Kind != "region" || kind != "monitoring_section" {
		return bad()
	}
	if len(g.Vertices) < min || len(g.Vertices) > max {
		return bad()
	}
	dim := 3
	if a.Kind == "layout" {
		dim = 2
	}
	for _, v := range g.Vertices {
		if !vector(v, dim, a.Kind == "layout") {
			return bad()
		}
		for _, n := range v {
			if math.Abs(n) > 1e9 {
				return bad()
			}
		}
	}
	distance := 0.
	for i := 1; i < len(g.Vertices); i++ {
		for j := 0; j < dim; j++ {
			distance += math.Abs(g.Vertices[i][j] - g.Vertices[i-1][j])
		}
	}
	if distance < 1e-12 {
		return bad()
	}
	if g.Kind == "region" {
		// Polygon area vector: rejects a degenerate region in both drawing and
		// model coordinates without inventing a surface or transforming axes.
		x, y, z := 0., 0., 0.
		for i, v := range g.Vertices {
			w := g.Vertices[(i+1)%len(g.Vertices)]
			z += v[0]*w[1] - w[0]*v[1]
			if dim == 3 {
				x += v[1]*w[2] - w[1]*v[2]
				y += v[2]*w[0] - w[2]*v[0]
			}
		}
		if math.Hypot(math.Hypot(x, y), z) < 1e-12 {
			return bad()
		}
	}
	return nil
}
