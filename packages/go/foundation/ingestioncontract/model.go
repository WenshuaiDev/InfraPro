// Package ingestioncontract owns the strictly versioned, storage-independent
// identity and ingress capability contract. Legacy parsing manifests are separate.
package ingestioncontract

import (
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"time"
)

const Version = 1
const MaxUnits = 256

type Method = c.Method
type Identification struct {
	Method            Method `json:"method"`
	Name              string `json:"name"`
	Purpose           string `json:"purpose"`
	InputUnit         string `json:"input_unit"`
	Transport         string `json:"transport"`
	MaxLength         int    `json:"max_length"`
	MaxUnits          int    `json:"max_units"`
	Seconds           int    `json:"execution_seconds"`
	SampleSuiteDigest string `json:"sample_suite_digest"`
	Example           bool   `json:"example"`
	ReleaseNotes      string `json:"release_notes"`
}
type Adapter struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	ArtifactDigest string `json:"artifact_digest"`
}
type Protocol struct {
	ID                 string   `json:"id"`
	Version            string   `json:"version"`
	Digest             string   `json:"digest"`
	Name               string   `json:"name"`
	Transport          string   `json:"transport"`
	Adapter            Adapter  `json:"transport_adapter"`
	Identification     Method   `json:"identification"`
	IdentityConvention string   `json:"identity_convention"`
	InputViews         []string `json:"input_views"`
	Example            bool     `json:"example"`
}
type Compatibility struct {
	ID              string  `json:"id"`
	Revision        int64   `json:"revision,string"`
	Digest          string  `json:"digest"`
	ProtocolID      string  `json:"ingress_protocol_id"`
	ProtocolVersion string  `json:"ingress_protocol_version"`
	ProtocolDigest  string  `json:"ingress_protocol_digest"`
	Demultiplexer   *Method `json:"demultiplexer,omitempty"`
	InputView       string  `json:"input_view"`
	ParseTransport  string  `json:"parse_transport"`
	Measurement     Method  `json:"measurement"`
	FixtureDigest   string  `json:"fixture_digest"`
}
type Catalog struct {
	Version               int              `json:"ingestion_contract_version"`
	IdentificationMethods []Identification `json:"identification_methods"`
	IngressProtocols      []Protocol       `json:"ingress_protocols"`
	Compatibilities       []Compatibility  `json:"compatibilities"`
	ReleaseCoverage       []string         `json:"release_coverage"`
}
type Input struct {
	BytesBase64 string `json:"bytes_base64"`
	ByteLength  int64  `json:"byte_length,string"`
	SHA256      string `json:"sha256"`
}
type Context struct {
	FixedTerminal          string    `json:"fixed_terminal,omitempty"`
	ExclusiveAuthenticated bool      `json:"exclusive_authenticated,omitempty"`
	ReceivedAt             time.Time `json:"received_at"`
	Transport              string    `json:"transport"`
	InputUnit              string    `json:"input_unit"`
}
type Request struct {
	Version     int     `json:"ingestion_contract_version"`
	ExecutionID string  `json:"execution_id"`
	Method      Method  `json:"method"`
	Input       Input   `json:"input"`
	Context     Context `json:"context"`
	TraceID     string  `json:"trace_id"`
}
type Range struct {
	Start int64 `json:"start,string"`
	End   int64 `json:"end,string"`
}
type Transform struct {
	Rule         string `json:"rule"`
	Range        Range  `json:"range"`
	InputSHA256  string `json:"input_sha256"`
	OutputSHA256 string `json:"output_sha256"`
	OutputLength int64  `json:"output_length,string"`
}
type View struct {
	Kind           string      `json:"kind"`
	InputUnit      string      `json:"input_unit"`
	ParseTransport string      `json:"parse_transport"`
	ByteLength     int64       `json:"byte_length,string"`
	SHA256         string      `json:"sha256"`
	Transforms     []Transform `json:"transforms"`
}
type Identity struct {
	Kind   string `json:"kind"`
	Raw    string `json:"raw"`
	Value  string `json:"value"`
	Parent string `json:"parent"`
	// Parent is input or unit; decoded offsets never masquerade as root offsets.
	Location Range `json:"location"`
}
type Unit struct {
	Key         string     `json:"unit_key"`
	MessageKind string     `json:"message_kind"`
	TerminalKey string     `json:"terminal_key"`
	ChildKey    string     `json:"child_key,omitempty"`
	ExternalID  string     `json:"external_id,omitempty"`
	View        View       `json:"input_view"`
	Identities  []Identity `json:"external_identities"`
}
type Response struct {
	Version            int      `json:"ingestion_contract_version"`
	ExecutionID        string   `json:"execution_id"`
	Method             Method   `json:"method"`
	InputSHA256        string   `json:"input_sha256"`
	ServiceRelease     string   `json:"service_release"`
	ServiceBuildDigest string   `json:"service_build_digest"`
	Status             string   `json:"status"`
	Units              []Unit   `json:"units"`
	Diagnostics        []string `json:"diagnostics"`
}
