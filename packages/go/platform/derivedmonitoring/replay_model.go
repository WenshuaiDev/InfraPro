package derivedmonitoring

type ReplayInput struct {
	WriteMeta
	Range *NanosecondRange `json:"output_range,omitempty"`
}
type ReplayDifference struct {
	ResultID     string `json:"result_id"`
	At           string `json:"at_ns"`
	Kind         string `json:"kind"`
	ExpectedHash string `json:"expected_hash"`
	ActualHash   string `json:"actual_hash"`
}
type ReplayResult struct {
	GenerationID       string             `json:"generation_id"`
	Range              NanosecondRange    `json:"output_range"`
	PlanHash           string             `json:"compiled_plan_hash"`
	ManifestHash       string             `json:"manifest_hash"`
	Matched            bool               `json:"matched"`
	Expected           uint64             `json:"expected_count,string"`
	Actual             uint64             `json:"actual_count,string"`
	Differences        uint64             `json:"difference_count,string"`
	DifferenceExamples []ReplayDifference `json:"difference_examples"`
	Examples           []TrialValue       `json:"examples"`
	InputState         string             `json:"input_state"`
}
