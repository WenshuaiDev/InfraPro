package displayread

import "encoding/json"

type ChannelSummary struct {
	ID                     string          `json:"id"`
	Code                   string          `json:"code"`
	Name                   string          `json:"name"`
	Status                 string          `json:"status"`
	Revision               string          `json:"revision"`
	SourceKind             string          `json:"source_kind"`
	Facility               Ref             `json:"facility_ref"`
	Owner                  Ref             `json:"owner_ref"`
	Point                  *Ref            `json:"point_ref,omitempty"`
	Metric                 MetricRef       `json:"metric_ref"`
	MetricName             string          `json:"metric_name"`
	UnitVersionID          *string         `json:"unit_version_id"`
	Unit                   string          `json:"unit"`
	ValueKind              string          `json:"value_kind"`
	TimestampMeaning       string          `json:"timestamp_meaning"`
	AggregationDescription string          `json:"aggregation_description"`
	ObservationDirection   string          `json:"observation_direction"`
	ReferenceBasis         string          `json:"reference_basis"`
	Sampling               json.RawMessage `json:"sampling"`
	QuantitySpec           json.RawMessage `json:"quantity_spec"`
	OutputSemantics        json.RawMessage `json:"output_semantics"`
	ReadRanges             []Window        `json:"read_ranges"`
}
type RawCounts struct {
	State     string            `json:"state"`
	Ever      *string           `json:"ever_published_count,omitempty"`
	Active    *string           `json:"active_count,omitempty"`
	Samples   *string           `json:"sample_count,omitempty"`
	Qualities map[string]string `json:"quality_counts,omitempty"`
	First     *string           `json:"first_time"`
	Last      *string           `json:"last_time"`
}
type DerivedCounts struct {
	State    string            `json:"state"`
	Logical  *string           `json:"logical_output_count,omitempty"`
	Valid    *string           `json:"valid_result_count,omitempty"`
	Samples  *string           `json:"sample_count,omitempty"`
	NoResult *string           `json:"no_result_count,omitempty"`
	Statuses map[string]string `json:"status_counts,omitempty"`
	First    *string           `json:"first_time"`
	Last     *string           `json:"last_time"`
}
type MonitoringCounts struct {
	ScopeChannels   string        `json:"scope_channel_count"`
	RawChannels     string        `json:"raw_channel_count"`
	DerivedChannels string        `json:"derived_channel_count"`
	Raw             RawCounts     `json:"raw"`
	Derived         DerivedCounts `json:"derived"`
}
type WindowSummary struct {
	SourceKind   string `json:"source_kind"`
	Range        Window `json:"range"`
	ChannelCount string `json:"channel_count"`
}
type CoverageSummary struct {
	SourceKind   string `json:"source_kind"`
	Range        Window `json:"range"`
	State        string `json:"state"`
	ChannelCount string `json:"channel_count"`
	UpdatedAt    string `json:"updated_at"`
}
