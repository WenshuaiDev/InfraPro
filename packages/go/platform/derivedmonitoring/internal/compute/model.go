// Package compute compiles the restricted rule language and evaluates it with
// fixed input samples. It has no database, HTTP, authorization or wall clock.
package compute

import "fmt"

const (
	SchemaVersion   = "derived-rule/1"
	CompilerVersion = "derived-compiler/1"
	EngineVersion   = "derived-engine/1"
)

type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e *Issue) Error() string               { return fmt.Sprintf("%s at %s: %s", e.Code, e.Path, e.Message) }
func issue(code, path, message string) error { return &Issue{code, path, message} }

type Limits struct{ Inputs, Nodes, Parameters, Outputs, ExpressionLength, ExpressionDepth, ExpressionNodes, WindowSamples, OutputRows int }

func DefaultLimits() Limits { return Limits{16, 32, 32, 16, 4096, 16, 1024, 1_000_000, 1_000_000} }

type Quantity struct {
	BaseUnitVersionID string `json:"base_unit_version_id,omitempty"`
	BasePower         int    `json:"base_power"`
	TimeUnit          string `json:"time_unit,omitempty"`
	TimePower         int    `json:"time_power"`
	Mode              string `json:"quantity_mode"`
	LevelScale        string `json:"level_scale"`
	DimensionlessKind string `json:"dimensionless_kind,omitempty"`
}
type TimeSemantics struct {
	Kind        string `json:"kind"`
	Aggregation string `json:"aggregation,omitempty"`
}
type Input struct {
	Key             string        `json:"key"`
	Name            string        `json:"name,omitempty"`
	Shape           string        `json:"shape"`
	SourceKinds     []string      `json:"source_kinds"`
	MetricVersionID string        `json:"metric_version_id,omitempty"`
	Quantity        Quantity      `json:"quantity_spec"`
	Direction       string        `json:"direction"`
	Reference       string        `json:"reference"`
	Time            TimeSemantics `json:"time_semantics"`
}
type Parameter struct {
	Key               string    `json:"key"`
	Type              string    `json:"type"`
	Default           string    `json:"default"`
	Minimum           string    `json:"minimum,omitempty"`
	Maximum           string    `json:"maximum,omitempty"`
	Quantity          *Quantity `json:"quantity_spec,omitempty"`
	IdentityAffecting bool      `json:"identity_affecting"`
}
type Clock struct {
	Kind     string `json:"kind"`
	Input    string `json:"input,omitempty"`
	AnchorNS string `json:"anchor_ns,omitempty"`
	StepNS   string `json:"step_ns,omitempty"`
	Timezone string `json:"timezone,omitempty"`
	Period   string `json:"period,omitempty"`
}
type Window struct {
	Kind       string `json:"kind"`
	DurationNS string `json:"duration_ns,omitempty"`
	Count      string `json:"count,omitempty"`
	AnchorNS   string `json:"anchor_ns,omitempty"`
	Timezone   string `json:"timezone,omitempty"`
	Period     string `json:"period,omitempty"`
	MaxSpanNS  string `json:"max_span_ns,omitempty"`
}
type Baseline struct {
	Kind      string    `json:"kind"`
	Value     string    `json:"value,omitempty"`
	Quantity  *Quantity `json:"quantity_spec,omitempty"`
	Reference string    `json:"reference,omitempty"`
	Method    string    `json:"method,omitempty"`
	FromNS    string    `json:"from_ns,omitempty"`
	ToNS      string    `json:"to_ns,omitempty"`
}
type Params struct {
	Alignment        string            `json:"alignment,omitempty"`
	ToleranceNS      string            `json:"tolerance_ns,omitempty"`
	Expression       string            `json:"expression,omitempty"`
	Variables        map[string]string `json:"variables,omitempty"`
	Window           *Window           `json:"window,omitempty"`
	MinSamples       string            `json:"min_samples,omitempty"`
	MaxGapNS         string            `json:"max_gap_ns,omitempty"`
	MaxAgeNS         string            `json:"max_age_ns,omitempty"`
	Percentile       string            `json:"percentile,omitempty"`
	DifferenceMode   string            `json:"difference_mode,omitempty"`
	LagNS            string            `json:"lag_ns,omitempty"`
	MatchToleranceNS string            `json:"match_tolerance_ns,omitempty"`
	InvalidPolicy    string            `json:"invalid_policy,omitempty"`
	RateTimeUnit     string            `json:"rate_time_unit,omitempty"`
	Weights          []string          `json:"weights,omitempty"`
	Baseline         *Baseline         `json:"baseline,omitempty"`
}
type Node struct {
	ID     string   `json:"id"`
	Op     string   `json:"op"`
	Inputs []string `json:"inputs,omitempty"`
	Params Params   `json:"params"`
}
type NumericContract struct {
	Scale    int    `json:"scale"`
	Rounding string `json:"rounding"`
}
type Output struct {
	Key         string          `json:"key"`
	Node        string          `json:"node"`
	SemanticKey string          `json:"semantic_key"`
	Name        string          `json:"name,omitempty"`
	Quantity    *Quantity       `json:"quantity_spec,omitempty"`
	Numeric     NumericContract `json:"numeric_contract"`
}
type Policies struct {
	Quality           []string `json:"quality,omitempty"`
	ConfigBoundary    string   `json:"config_boundary"`
	AllowedLatenessNS string   `json:"allowed_lateness_ns"`
}
type Rule struct {
	SchemaVersion string      `json:"schema_version"`
	Inputs        []Input     `json:"inputs"`
	Parameters    []Parameter `json:"parameters,omitempty"`
	Clock         Clock       `json:"clock"`
	Nodes         []Node      `json:"nodes"`
	Outputs       []Output    `json:"outputs"`
	Policies      Policies    `json:"policies"`
}

// Plan is built only through Compile. Canonical includes expanded defaults and
// version tags; consumers persist it with its digest, never reconstruct it from
// whichever defaults happen to be installed later.
type Plan struct {
	Rule                         Rule
	Mode                         string
	Values                       map[string]string
	Quantities                   map[string]Quantity
	Times                        map[string]TimeSemantics
	Canonical                    []byte
	PlanHash, OutputContractHash string
	limits                       Limits
	formulas                     map[string]*expression
}
