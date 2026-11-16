// Package parsingcontract owns the storage-independent parser wire contract.
package parsingcontract

import "time"

const (
	Version     = 1
	MaxInput    = 4 << 20
	MaxRequest  = 6 << 20
	MaxResponse = 16 << 20
	MaxRecords  = 10000
	MaxSteps    = 50000
	MaxDepth    = 64
)

type Method struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	ArtifactDigest string `json:"artifact_digest"`
	ManifestDigest string `json:"manifest_digest"`
}
type Semantics struct {
	ValueType        string `json:"value_type"`
	Meaning          string `json:"meaning"`
	Unit             string `json:"unit"`
	TimestampMeaning string `json:"timestamp_meaning"`
	Aggregation      string `json:"aggregation"`
	Direction        string `json:"direction"`
	Reference        string `json:"reference"`
}
type Output struct {
	Key string `json:"output_key"`
	Semantics
}
type Manifest struct {
	ContractVersion      int      `json:"contract_version"`
	MethodID             string   `json:"method_id"`
	MethodVersion        string   `json:"method_version"`
	Name                 string   `json:"name"`
	Category             string   `json:"category"`
	Example              bool     `json:"example"`
	ProtocolReference    string   `json:"protocol_reference"`
	InputUnit            string   `json:"input_unit"`
	Transports           []string `json:"transports"`
	MaxLength            int      `json:"max_length"`
	Outputs              []Output `json:"outputs"`
	ArtifactDigest       string   `json:"artifact_digest"`
	OutputContractDigest string   `json:"output_contract_digest"`
	SampleSuiteDigest    string   `json:"sample_suite_digest"`
	ReleaseNotes         string   `json:"release_notes"`
}
type Entry struct {
	Manifest
	ManifestDigest       string `json:"manifest_digest"`
	ManifestBase64       string `json:"manifest_base64"`
	OutputContractBase64 string `json:"output_contract_base64"`
}

func (e Entry) Method() Method {
	return Method{e.MethodID, e.MethodVersion, e.ArtifactDigest, e.ManifestDigest}
}

type Catalog struct {
	ContractVersion    int     `json:"contract_version"`
	ServiceRelease     string  `json:"service_release"`
	ServiceBuildDigest string  `json:"service_build_digest"`
	Entries            []Entry `json:"entries"`
	Limits             Limits  `json:"limits"`
}
type Limits struct {
	Input       int `json:"input_bytes"`
	Response    int `json:"response_bytes"`
	Records     int `json:"records"`
	Steps       int `json:"steps"`
	Depth       int `json:"depth"`
	Seconds     int `json:"execution_seconds"`
	Concurrency int `json:"concurrency"`
}
type Input struct {
	BytesBase64 string `json:"bytes_base64"`
	SHA256      string `json:"sha256"`
	ByteLength  int    `json:"byte_length"`
}
type Context struct {
	ReceivedAt time.Time `json:"received_at"`
	Transport  string    `json:"transport"`
}
type Request struct {
	ContractVersion int     `json:"contract_version"`
	ExecutionID     string  `json:"execution_id"`
	Method          Method  `json:"method"`
	Input           Input   `json:"input"`
	Context         Context `json:"context"`
	TraceID         string  `json:"trace_id"`
}
type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}
type Location struct {
	Range   Range  `json:"range"`
	Pointer string `json:"json_pointer,omitempty"`
}
type Identity struct {
	Role          string   `json:"role"`
	Text          string   `json:"text,omitempty"`
	Hex           string   `json:"hex"`
	Location      Location `json:"location"`
	Authenticated bool     `json:"authenticated"`
}
type Field struct {
	Key      string   `json:"key"`
	Value    string   `json:"value"`
	RawText  string   `json:"raw_text,omitempty"`
	Location Location `json:"location"`
	StepIDs  []string `json:"step_ids"`
}
type Measurement struct {
	OutputKey string   `json:"output_key"`
	ValueType string   `json:"value_type"`
	Value     *string  `json:"value"`
	Unit      string   `json:"unit"`
	RawText   string   `json:"raw_text,omitempty"`
	Location  Location `json:"location"`
	StepIDs   []string `json:"step_ids"`
}
type Record struct {
	Index        int           `json:"index"`
	ObservedAt   *time.Time    `json:"observed_at"`
	TimeSource   string        `json:"time_source"`
	Measurements []Measurement `json:"measurements"`
	SourceStatus string        `json:"source_status,omitempty"`
}
type Step struct {
	ID           string   `json:"id"`
	RuleID       string   `json:"rule_id"`
	Range        Range    `json:"range"`
	Status       string   `json:"status"`
	ParentIDs    []string `json:"parent_ids"`
	FieldKeys    []string `json:"field_keys"`
	InputDigest  string   `json:"input_digest"`
	OutputDigest string   `json:"output_digest"`
}
type Diagnostic struct {
	Code     string    `json:"code"`
	Severity string    `json:"severity"`
	Message  string    `json:"message"`
	Location *Location `json:"location,omitempty"`
}
type Check struct {
	RuleID string `json:"rule_id"`
	Status string `json:"status"`
	Range  *Range `json:"range,omitempty"`
}
type Response struct {
	ContractVersion    int          `json:"contract_version"`
	ExecutionID        string       `json:"execution_id"`
	Method             Method       `json:"method"`
	InputSHA256        string       `json:"input_sha256"`
	ServiceRelease     string       `json:"service_release"`
	ServiceBuildDigest string       `json:"service_build_digest"`
	Status             string       `json:"status"`
	MessageKind        string       `json:"message_kind"`
	Identities         []Identity   `json:"identities"`
	Records            []Record     `json:"records"`
	Fields             []Field      `json:"fields"`
	Steps              []Step       `json:"steps"`
	Diagnostics        []Diagnostic `json:"diagnostics"`
	Checks             []Check      `json:"checks"`
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code }
func Fail(code string) error   { return &Error{code, code} }
