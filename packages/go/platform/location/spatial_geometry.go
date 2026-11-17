package location

// AssociationGeometry records explicitly supplied coordinates in an existing
// spatial publication. It is never inferred from borehole depth or map position.
// Plane origin is the placement anchor; size is width/height in view metres.
type AssociationGeometry struct {
	Kind     string      `json:"kind"`
	Vertices [][]float64 `json:"vertices,omitempty"`
	Normal   []float64   `json:"normal,omitempty"`
	Size     []float64   `json:"size,omitempty"`
}
