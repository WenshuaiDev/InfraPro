package displayread

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/location"
	"time"
)

type SpatialAsset struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Category     string `json:"category"`
	Status       string `json:"status"`
	Revision     string `json:"revision"`
	Facility     *Ref   `json:"facility_ref,omitempty"`
	DefaultState string `json:"default_state"`
	VersionID    string `json:"version_id,omitempty"`
	VersionCount string `json:"version_count"`
}
type SpatialVersion struct {
	ID           string     `json:"id"`
	AssetID      string     `json:"asset_id"`
	Number       string     `json:"number"`
	Format       string     `json:"format"`
	State        string     `json:"state"`
	Revision     string     `json:"revision"`
	ManifestHash string     `json:"manifest_hash"`
	PublishedAt  *time.Time `json:"published_at"`
}
type SpatialFile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	Size      string `json:"size"`
	MediaType string `json:"media_type"`
}
type SpatialCoordinates struct {
	Unit         string    `json:"unit"`
	UpAxis       string    `json:"up_axis"`
	SourceUnit   string    `json:"source_unit"`
	SourceToView []float64 `json:"source_to_view"`
	ViewToSource []float64 `json:"view_to_source"`
	CRS          string    `json:"crs,omitempty"`
}
type SpatialPageImage struct {
	FileID string `json:"file_id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type SpatialPage struct {
	ID           string             `json:"id"`
	Number       int                `json:"number"`
	Width        int                `json:"width"`
	Height       int                `json:"height"`
	SourceWidth  float64            `json:"source_width"`
	SourceHeight float64            `json:"source_height"`
	Rotation     int                `json:"rotation"`
	CropBox      []float64          `json:"crop_box"`
	FileID       string             `json:"file_id"`
	Resolutions  []SpatialPageImage `json:"resolutions"`
}
type SpatialManifest struct {
	SpatialVersion
	Contract     string              `json:"contract"`
	Coordinates  *SpatialCoordinates `json:"coordinates,omitempty"`
	Files        []SpatialFile       `json:"files,omitempty"`
	Pages        []SpatialPage       `json:"pages,omitempty"`
	ElementCount string              `json:"element_count"`
	FeatureCount string              `json:"feature_count"`
	Warnings     []string            `json:"warnings"`
}
type SpatialFragment struct {
	Node     int  `json:"node"`
	Instance *int `json:"instance,omitempty"`
}
type SpatialElement struct {
	Key            string            `json:"key"`
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	Parent         string            `json:"parent,omitempty"`
	TypeKey        string            `json:"type_key,omitempty"`
	Fragments      []SpatialFragment `json:"fragments"`
	GeometryHash   string            `json:"geometry_hash"`
	Transform      []float64         `json:"transform"`
	Properties     map[string]any    `json:"properties,omitempty"`
	TypeProperties map[string]any    `json:"type_properties,omitempty"`
}
type SpatialPlacement struct {
	Geometry     *location.AssociationGeometry `json:"geometry,omitempty"`
	ID           string                        `json:"id"`
	Point        *Ref                          `json:"point_ref,omitempty"`
	Target       *Ref                          `json:"target_ref,omitempty"`
	Name         string                        `json:"name"`
	Code         string                        `json:"code"`
	Status       string                        `json:"status"`
	PageID       string                        `json:"page_id"`
	Anchor       []float64                     `json:"anchor"`
	ElementKey   string                        `json:"element_key"`
	LabelOffset  []float64                     `json:"label_offset"`
	BasisChanged bool                          `json:"basis_changed"`
}
type SpatialComponentLink struct {
	ID           string      `json:"id"`
	Component    Ref         `json:"component_ref"`
	Name         string      `json:"name"`
	Code         string      `json:"code"`
	Status       string      `json:"status"`
	PageID       string      `json:"page_id"`
	ElementKey   string      `json:"element_key"`
	Region       [][]float64 `json:"region"`
	BasisChanged bool        `json:"basis_changed"`
}
type SpatialLocation struct {
	Geometry   *location.AssociationGeometry `json:"geometry,omitempty"`
	Asset      SpatialAsset                  `json:"asset"`
	VersionID  string                        `json:"version_id"`
	Number     string                        `json:"number"`
	Owner      Ref                           `json:"owner_ref"`
	PageID     string                        `json:"page_id"`
	Anchor     []float64                     `json:"anchor,omitempty"`
	ElementKey string                        `json:"element_key"`
}
type SpatialCamera struct {
	Position []float64 `json:"position"`
	Target   []float64 `json:"target"`
}
type SpatialMainChannel struct {
	Owner   Ref    `json:"owner_ref"`
	State   string `json:"state"`
	Channel *Ref   `json:"channel_ref,omitempty"`
}
type SpatialSettings struct {
	VersionID        string               `json:"version_id"`
	Revision         string               `json:"revision"`
	Camera           *SpatialCamera       `json:"camera,omitempty"`
	PageID           string               `json:"page_id"`
	HiddenElements   []string             `json:"hidden_elements"`
	MainChannelState string               `json:"main_channel_state"`
	MainChannels     []SpatialMainChannel `json:"main_channels"`
}
type MapFeature struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	Geometry      map[string]any    `json:"geometry"`
	BBox          []float64         `json:"bbox"`
	Properties    map[string]string `json:"properties,omitempty"`
	Facility      *Ref              `json:"facility_ref,omitempty"`
	FacilityState string            `json:"facility_state"`
}
type Basemap struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	Reason      string `json:"reason,omitempty"`
	Attribution string `json:"attribution"`
	MinZoom     int    `json:"min_zoom"`
	MaxZoom     int    `json:"max_zoom"`
}
type FileChunk struct {
	ID        string `json:"id"`
	Base64    string `json:"base64"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	Size      string `json:"size"`
	Offset    string `json:"offset"`
	Length    string `json:"length"`
}
