// Package relaycontract defines the finite cloud relay machine API. It has no
// dependency on platform business data or permission models.
package relaycontract

import (
	"encoding/base64"
	"encoding/json"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/google/uuid"
	"net"
	"regexp"
	"slices"
	"time"
)

const Version = 1
const MaxBody = 4 << 20
const MaxPullBytes = 8 << 20
const MaxResponse = 12 << 20
const Lease = 120 * time.Second

type Source struct {
	IngressID         string    `json:"ingress_id"`
	NamespaceID       string    `json:"source_namespace_id"`
	Revision          int64     `json:"source_revision,string"`
	Digest            string    `json:"source_revision_digest"`
	RelayID           string    `json:"relay_id"`
	TenantID          string    `json:"tenant_id"`
	ProjectID         string    `json:"source_project_id"`
	ManufacturerID    string    `json:"manufacturer_id"`
	ManufacturerScope string    `json:"manufacturer_scope"`
	ProtocolID        string    `json:"ingress_protocol_id"`
	ProtocolVersion   string    `json:"ingress_protocol_version"`
	ProtocolDigest    string    `json:"ingress_protocol_digest"`
	Transport         string    `json:"listener_transport"`
	Address           string    `json:"listener_address"`
	Port              int       `json:"listener_port"`
	SourceCode        string    `json:"source_code"`
	ProducerIDs       []string  `json:"producer_principal_ids"`
	Adapter           i.Adapter `json:"transport_adapter"`
	Active            bool      `json:"active"`
	Synthetic         bool      `json:"synthetic"`
	Exclusive         bool      `json:"exclusive"`
	FixedTerminal     string    `json:"fixed_terminal,omitempty"`
	IdleSeconds       int       `json:"idle_seconds"`
	Budget            int64     `json:"budget_bytes,string"`
}
type Apply struct {
	CommandID          string `json:"command_id"`
	ExpectedGeneration int64  `json:"expected_generation,string"`
	Source             Source `json:"source"`
}
type Applied struct {
	CommandID  string    `json:"command_id"`
	Generation int64     `json:"generation,string"`
	Digest     string    `json:"source_revision_digest"`
	State      string    `json:"state"`
	ErrorCode  string    `json:"error_code,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
}
type Envelope struct {
	Version            int       `json:"relay_contract_version"`
	RelayID            string    `json:"relay_id"`
	Epoch              string    `json:"spool_epoch"`
	ReceiptID          string    `json:"receipt_id"`
	IngressID          string    `json:"ingress_id"`
	NamespaceID        string    `json:"source_namespace_id"`
	Revision           int64     `json:"source_revision,string"`
	RevisionDigest     string    `json:"source_revision_digest"`
	TenantID           string    `json:"tenant_id"`
	ProjectID          string    `json:"source_project_id"`
	ManufacturerID     string    `json:"manufacturer_id"`
	ManufacturerScope  string    `json:"manufacturer_scope"`
	ProtocolID         string    `json:"ingress_protocol_id"`
	ProtocolVersion    string    `json:"ingress_protocol_version"`
	ProtocolDigest     string    `json:"ingress_protocol_digest"`
	ListenerTransport  string    `json:"listener_transport"`
	ListenerAddress    string    `json:"listener_address"`
	ListenerPort       int       `json:"listener_port"`
	Sequence           int64     `json:"source_sequence,string"`
	PrincipalID        string    `json:"producer_principal_id"`
	Transport          string    `json:"transport"`
	Authentication     string    `json:"authentication_level"`
	ConnectionID       string    `json:"connection_id,omitempty"`
	ConnectionSequence int64     `json:"connection_sequence,string"`
	ReceivedAt         time.Time `json:"received_at"`
	CommittedAt        time.Time `json:"committed_at"`
	ContentKind        string    `json:"content_kind"`
	Payload            string    `json:"payload_base64"`
	ByteLength         int64     `json:"byte_length,string"`
	SHA256             string    `json:"sha256"`
	ProducerDeliveryID string    `json:"producer_delivery_id,omitempty"`
	Adapter            i.Adapter `json:"transport_adapter"`
	EvidenceKind       string    `json:"evidence_kind"`
	Truncated          bool      `json:"truncated"`
	Synthetic          bool      `json:"synthetic"`
	EnvelopeDigest     string    `json:"envelope_digest"`
	DeliveryID         string    `json:"delivery_id,omitempty"`
	DeliveryAttempt    int       `json:"delivery_attempt"`
	LeaseToken         string    `json:"lease_token,omitempty"`
	LeaseUntil         time.Time `json:"lease_until"`
}
type Pull struct {
	RequestID   string `json:"request_id"`
	Limit       int    `json:"limit"`
	MaxBytes    int    `json:"max_bytes"`
	WaitSeconds int    `json:"wait_seconds"`
}
type Delivery struct {
	Items []Envelope `json:"items"`
}
type Ack struct {
	RelayID        string `json:"relay_id"`
	Epoch          string `json:"spool_epoch"`
	ReceiptID      string `json:"receipt_id"`
	LeaseToken     string `json:"lease_token"`
	SHA256         string `json:"sha256"`
	LocalReceiptID string `json:"local_receipt_id"`
}
type Acks struct {
	Items []Ack `json:"items"`
}
type AckResult struct {
	ReceiptID string `json:"receipt_id"`
	State     string `json:"state"`
	ErrorCode string `json:"error_code,omitempty"`
}
type AckResults struct {
	Items []AckResult `json:"items"`
}
type Release struct {
	Ack
	Reason       string `json:"reason"`
	DelaySeconds int    `json:"delay_seconds"`
}
type Releases struct {
	Items []Release `json:"items"`
}
type WatermarkRequest struct {
	CommandID string `json:"command_id"`
	IngressID string `json:"ingress_id"`
	Epoch     string `json:"spool_epoch"`
}
type Watermark struct {
	WatermarkRequest
	Sequence int64 `json:"source_sequence,string"`
}

func Hash(v any) string            { b, _ := json.Marshal(v); return c.Digest(b) }
func SourceDigest(s Source) string { s.Digest = ""; return Hash(s) }
func EnvelopeDigest(e Envelope) string {
	e.EnvelopeDigest = ""
	e.Payload = ""
	e.DeliveryID = ""
	e.DeliveryAttempt = 0
	e.LeaseToken = ""
	e.LeaseUntil = time.Time{}
	return Hash(e)
}
func validID(s string) bool { v, e := uuid.Parse(s); return e == nil && v.String() == s }

var safe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

func ValidateSource(s Source) error {
	for _, id := range []string{s.IngressID, s.NamespaceID, s.RelayID, s.TenantID, s.ProjectID, s.ManufacturerID} {
		if !validID(id) {
			return c.Fail("invalid_source")
		}
	}
	if s.Revision < 1 || s.Digest != SourceDigest(s) || !c.IsDigest(s.ProtocolDigest) || !c.IsDigest(s.Adapter.ArtifactDigest) || !slices.Contains([]string{"public", "tenant"}, s.ManufacturerScope) || !slices.Contains([]string{"ddp_tcp", "https_push"}, s.Transport) || net.ParseIP(s.Address) == nil || s.Port < 1024 || s.Port > 65535 || !safe.MatchString(s.SourceCode) || len(s.ProducerIDs) < 1 || len(s.ProducerIDs) > 100 || s.IdleSeconds < 1 || s.IdleSeconds > 3600 || s.Budget < MaxBody {
		return c.Fail("invalid_source")
	}
	for _, id := range s.ProducerIDs {
		if !safe.MatchString(id) {
			return c.Fail("invalid_source")
		}
	}
	if s.FixedTerminal != "" && (!s.Exclusive || len(s.FixedTerminal) != 11) {
		return c.Fail("fixed_terminal_requires_exclusive_source")
	}
	return nil
}
func Decode(e Envelope) ([]byte, error) {
	for _, id := range []string{e.RelayID, e.Epoch, e.ReceiptID, e.IngressID, e.NamespaceID, e.TenantID, e.ProjectID, e.ManufacturerID} {
		if !validID(id) {
			return nil, c.Fail("invalid_envelope")
		}
	}
	if e.Version != Version || e.Sequence < 1 || e.Revision < 1 || e.ByteLength < 1 || e.ByteLength > MaxBody || len(e.Payload) > base64.StdEncoding.EncodedLen(MaxBody) || e.ReceivedAt.IsZero() || e.CommittedAt.IsZero() || e.Authentication != "mtls" || !slices.Contains([]string{"complete_message", "invalid_fragment"}, e.EvidenceKind) || e.Truncated && e.EvidenceKind != "invalid_fragment" || e.EnvelopeDigest != EnvelopeDigest(e) {
		return nil, c.Fail("invalid_envelope")
	}
	b, err := base64.StdEncoding.Strict().DecodeString(e.Payload)
	if err != nil || int64(len(b)) != e.ByteLength || c.Digest(b) != e.SHA256 {
		return nil, c.Fail("input_hash_mismatch")
	}
	return b, nil
}
func Match(e Envelope, s Source) error {
	if e.RelayID != s.RelayID || e.IngressID != s.IngressID || e.NamespaceID != s.NamespaceID || e.Revision != s.Revision || e.RevisionDigest != s.Digest || e.TenantID != s.TenantID || e.ProjectID != s.ProjectID || e.ManufacturerID != s.ManufacturerID || e.ManufacturerScope != s.ManufacturerScope || e.ProtocolID != s.ProtocolID || e.ProtocolVersion != s.ProtocolVersion || e.ProtocolDigest != s.ProtocolDigest || e.ListenerTransport != s.Transport || e.ListenerAddress != s.Address || e.ListenerPort != s.Port || e.Adapter != s.Adapter || !slices.Contains(s.ProducerIDs, e.PrincipalID) || e.Synthetic != s.Synthetic {
		return c.Fail("envelope_config_mismatch")
	}
	return nil
}
func Wrap(s Source, epoch, receipt, principal string, sequence int64, raw []byte, received, committed time.Time) Envelope {
	e := Envelope{Version: Version, RelayID: s.RelayID, Epoch: epoch, ReceiptID: receipt, IngressID: s.IngressID, NamespaceID: s.NamespaceID, Revision: s.Revision, RevisionDigest: s.Digest, TenantID: s.TenantID, ProjectID: s.ProjectID, ManufacturerID: s.ManufacturerID, ManufacturerScope: s.ManufacturerScope, ProtocolID: s.ProtocolID, ProtocolVersion: s.ProtocolVersion, ProtocolDigest: s.ProtocolDigest, ListenerTransport: s.Transport, ListenerAddress: s.Address, ListenerPort: s.Port, Sequence: sequence, PrincipalID: principal, Transport: s.Transport, Authentication: "mtls", ReceivedAt: received, CommittedAt: committed, Payload: base64.StdEncoding.EncodeToString(raw), ByteLength: int64(len(raw)), SHA256: c.Digest(raw), Adapter: s.Adapter, EvidenceKind: "complete_message", Synthetic: s.Synthetic, ContentKind: "application/octet-stream"}
	if s.Transport == "https_push" {
		e.ContentKind = "application/json"
	}
	return e
}
func EndpointConflicts(a, b Source) bool {
	return a.RelayID == b.RelayID && a.Transport == b.Transport && a.Port == b.Port && (a.Address == b.Address || net.ParseIP(a.Address).IsUnspecified() || net.ParseIP(b.Address).IsUnspecified())
}
