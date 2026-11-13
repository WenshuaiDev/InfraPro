package derivedmonitoring

type AnalysisOutputMapping struct {
	Left  string `json:"left"`
	Right string `json:"right"`
}
type AnalysisCompareInput struct {
	WriteMeta
	LeftVersionID  string                  `json:"left_version_id"`
	RightVersionID string                  `json:"right_version_id"`
	Outputs        []AnalysisOutputMapping `json:"output_mapping,omitempty"`
	Range          *NanosecondRange        `json:"output_range,omitempty"`
}
type AnalysisComparisonOutput struct {
	AnalysisOutputMapping
	NumericComparable bool   `json:"numeric_comparable"`
	SemanticChanged   bool   `json:"semantic_changed"`
	LeftRows          uint64 `json:"left_rows,string"`
	RightRows         uint64 `json:"right_rows,string"`
	Added             uint64 `json:"added,string"`
	Removed           uint64 `json:"removed,string"`
	ValueChanged      uint64 `json:"value_changed,string"`
	StatusChanged     uint64 `json:"status_changed,string"`
	QualityChanged    uint64 `json:"quality_changed,string"`
	SamplesChanged    uint64 `json:"samples_changed,string"`
	IntervalChanged   uint64 `json:"interval_changed,string"`
	Unchanged         uint64 `json:"unchanged,string"`
}
type AnalysisComparisonDifference struct {
	LeftOutput  string               `json:"left_output"`
	RightOutput string               `json:"right_output"`
	At          string               `json:"at_ns"`
	Kind        string               `json:"kind"`
	Changes     []string             `json:"changes"`
	Left        *AnalysisArtifactRow `json:"left,omitempty"`
	Right       *AnalysisArtifactRow `json:"right,omitempty"`
	Difference  *string              `json:"numeric_difference,omitempty"`
}
type AnalysisComparisonResult struct {
	LeftQuality          []string                       `json:"left_quality"`
	RightQuality         []string                       `json:"right_quality"`
	Left                 AnalysisArtifactVersion        `json:"left"`
	Right                AnalysisArtifactVersion        `json:"right"`
	LeftConfigHash       string                         `json:"left_config_hash"`
	RightConfigHash      string                         `json:"right_config_hash"`
	LeftInputHash        string                         `json:"left_input_hash"`
	RightInputHash       string                         `json:"right_input_hash"`
	ConfigurationChanged bool                           `json:"configuration_changed"`
	InputsChanged        bool                           `json:"inputs_changed"`
	RangeChanged         bool                           `json:"range_changed"`
	Range                *NanosecondRange               `json:"output_range,omitempty"`
	Outputs              []AnalysisComparisonOutput     `json:"outputs"`
	Differences          uint64                         `json:"difference_count,string"`
	Examples             []AnalysisComparisonDifference `json:"examples"`
	ExamplesTruncated    bool                           `json:"examples_truncated"`
}
type analysisComparisonTarget struct {
	LeftSelection, RightSelection                                *analysisSelection
	LeftVersion, RightVersion, LeftArtifact, RightArtifact, Hash string
	Outputs                                                      []AnalysisOutputMapping
	Range                                                        *NanosecondRange
}
