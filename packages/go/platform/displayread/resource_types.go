package displayread

import (
	"encoding/json"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/location"
)

type TypeRef struct {
	DefinitionID string `json:"definition_id"`
	VersionID    string `json:"version_id"`
	Name         string `json:"name"`
	Version      string `json:"version"`
}
type ResourceSummary struct {
	StructureApplicability map[string]string `json:"structure_applicability,omitempty"`
	BasisHash              string            `json:"-"`
	ID                     string            `json:"id"`
	Kind                   string            `json:"kind"`
	Code                   string            `json:"code"`
	Name                   string            `json:"name"`
	Visibility             string            `json:"visibility"`
	FacilityID             string            `json:"facility_id,omitempty"`
	Parent                 *Ref              `json:"parent_ref,omitempty"`
	Status                 string            `json:"status,omitempty"`
	Revision               string            `json:"revision,omitempty"`
	Type                   *TypeRef          `json:"type_ref,omitempty"`
	HasVisibleChildren     bool              `json:"has_visible_children"`
	SourceKind             string            `json:"source_kind,omitempty"`
	Metric                 *MetricRef        `json:"metric_ref,omitempty"`
	Owner                  *Ref              `json:"owner_ref,omitempty"`
	MetricVersionID        string            `json:"metric_version_id,omitempty"`
	CreatedAt              *time.Time        `json:"created_at,omitempty"`
}
type ResourceType struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Versions []TypeRef `json:"versions"`
}
type DefinitionVersion struct {
	ID           string          `json:"id"`
	DefinitionID string          `json:"definition_id"`
	Kind         string          `json:"kind"`
	Number       string          `json:"number"`
	Content      json.RawMessage `json:"content"`
}
type ResourceDetail struct {
	StructureModel        int                    `json:"structure_model"`
	StructureProjection   string                 `json:"structure_projection"`
	Purpose               *string                `json:"purpose,omitempty"`
	Borehole              *BoreholeMeasurements  `json:"borehole,omitempty"`
	BoreholePosition      *BoreholePointPosition `json:"borehole_position,omitempty"`
	BoreholePositionState string                 `json:"borehole_position_state,omitempty"`
	Summary               ResourceSummary        `json:"summary"`
	Ancestors             []ResourceSummary      `json:"ancestors"`
	Definitions           []DefinitionVersion    `json:"definitions"`
	Values                map[string]any         `json:"values,omitempty"`
	Description           *string                `json:"description,omitempty"`
	Position              *location.Position     `json:"position,omitempty"`
	CategoryKey           string                 `json:"category_key,omitempty"`
	Unit                  *DefinitionVersion     `json:"unit_ref,omitempty"`
	ObservationDirection  string                 `json:"observation_direction,omitempty"`
	ReferenceBasis        string                 `json:"reference_basis,omitempty"`
	TimestampMeaning      string                 `json:"timestamp_meaning,omitempty"`
	Sampling              json.RawMessage        `json:"sampling,omitempty"`
	QuantitySpec          json.RawMessage        `json:"quantity_spec,omitempty"`
}

type BoreholeMeasurements struct {
	CollarElevation    *float64 `json:"collar_elevation,omitempty"`
	ElevationDatum     string   `json:"elevation_datum,omitempty"`
	DesignDepthM       *float64 `json:"design_depth_m,omitempty"`
	MeasuredDepthM     *float64 `json:"measured_depth_m,omitempty"`
	DiameterMM         *float64 `json:"diameter_mm,omitempty"`
	Azimuth            *float64 `json:"azimuth,omitempty"`
	Inclination        *float64 `json:"inclination,omitempty"`
	DirectionReference string   `json:"direction_reference,omitempty"`
	Source             string   `json:"source,omitempty"`
	MeasuredAt         string   `json:"measured_at,omitempty"`
}
type BoreholePointPosition struct {
	ID                string   `json:"id"`
	BoreholeID        string   `json:"borehole_id"`
	Revision          int      `json:"revision"`
	DistanceM         *float64 `json:"distance_m"`
	ReferenceStart    string   `json:"reference_start"`
	Direction         string   `json:"direction"`
	Unit              string   `json:"unit"`
	VerticalDepthM    *float64 `json:"vertical_depth_m"`
	VerticalDatum     string   `json:"vertical_datum"`
	Source            string   `json:"source"`
	MeasuredAt        *string  `json:"measured_at"`
	BoreholeRevision  int      `json:"borehole_revision"`
	ConsistencyReason string   `json:"consistency_reason"`
}
type ResourceCounts struct {
	FacilityCount  string                       `json:"facility_count"`
	ComponentCount string                       `json:"component_count"`
	PointCount     string                       `json:"point_count"`
	ChannelCount   *string                      `json:"channel_count,omitempty"`
	DirectCount    *string                      `json:"direct_channel_count,omitempty"`
	DerivedCount   *string                      `json:"derived_channel_count,omitempty"`
	ChannelsState  string                       `json:"channels_state"`
	StatusCounts   map[string]map[string]string `json:"status_counts"`
}
type StatisticsGroup[T any] struct {
	Key        string `json:"group_key"`
	Label      string `json:"label"`
	Visibility string `json:"visibility"`
	Ref        *Ref   `json:"ref,omitempty"`
	Totals     T      `json:"totals"`
}
type FacilityMapItem struct {
	Facility      ResourceSummary    `json:"facility_ref"`
	Type          *TypeRef           `json:"type_ref"`
	PositionState string             `json:"position_status"`
	Position      *location.Position `json:"position,omitempty"`
	Longitude     *float64           `json:"longitude,omitempty"`
	Latitude      *float64           `json:"latitude,omitempty"`
	CoordinateCRS string             `json:"coordinate_crs,omitempty"`
	Notice        string             `json:"position_notice"`
}
