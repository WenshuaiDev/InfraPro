// Package ingestion owns production receipts and machine-authorized fixed
// processing. It does not publish standard monitoring observations.
package ingestion

import (
	"encoding/json"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"time"
)

type Scope = k.Scope
type Meta = k.Meta
type Result struct {
	ID        string          `json:"id"`
	Revision  string          `json:"revision"`
	State     string          `json:"state"`
	ErrorCode string          `json:"error_code,omitempty"`
	Replayed  bool            `json:"replayed"`
	TaskRef   *taskrecord.Ref `json:"task_ref,omitempty"`
}
type Filter struct {
	k.Filter
	SourceID      string     `json:"source_id,omitempty"`
	ProjectID     string     `json:"project_id,omitempty"`
	TerminalID    string     `json:"terminal_id,omitempty"`
	DeviceID      string     `json:"device_id,omitempty"`
	ReceiptID     string     `json:"receipt_id,omitempty"`
	MethodVersion string     `json:"method_version,omitempty"`
	ErrorCode     string     `json:"error_code,omitempty"`
	From          *time.Time `json:"from,omitempty"`
	Until         *time.Time `json:"until,omitempty"`
}
type Source struct {
	ID                string     `json:"id"`
	NamespaceID       string     `json:"namespace_id"`
	TenantID          string     `json:"tenant_id"`
	ProjectID         string     `json:"project_id"`
	ManufacturerID    string     `json:"manufacturer_id"`
	ManufacturerScope string     `json:"manufacturer_scope"`
	RelayID           string     `json:"relay_id"`
	Name              string     `json:"name"`
	Revision          string     `json:"revision"`
	State             string     `json:"state"`
	AppliedGeneration string     `json:"applied_generation"`
	AppliedRevision   *string    `json:"applied_revision"`
	Application       *r.Applied `json:"application"`
	PlanningPaused    bool       `json:"planning_paused"`
	Blocked           bool       `json:"blocked"`
	ErrorCode         string     `json:"error_code"`
	Synthetic         bool       `json:"synthetic"`
	CreatedAt         time.Time  `json:"created_at"`
}
type SourceRevision struct {
	SourceID           string    `json:"source_id"`
	Revision           string    `json:"revision"`
	Snapshot           r.Source  `json:"snapshot"`
	Digest             string    `json:"digest"`
	CommandID          string    `json:"command_id"`
	ExpectedGeneration string    `json:"expected_generation"`
	CreatedAt          time.Time `json:"created_at"`
	Confirmed          bool      `json:"confirmed"`
}
type SourceDetail struct {
	Source     Source           `json:"source"`
	Revisions  []SourceRevision `json:"revisions"`
	Status     json.RawMessage  `json:"relay_status"`
	ObservedAt *time.Time       `json:"observed_at"`
	Fresh      bool             `json:"fresh"`
}
type RelayInput struct {
	Meta
	ID               string `json:"id"`
	Name             string `json:"name"`
	ConfigurationRef string `json:"configuration_ref"`
}
type SourceInput struct {
	Meta
	Revision          string   `json:"expected_revision"`
	Name              string   `json:"name"`
	TenantID          string   `json:"tenant_id"`
	ProjectID         string   `json:"source_project_id"`
	ManufacturerID    string   `json:"manufacturer_id"`
	ManufacturerScope string   `json:"manufacturer_scope"`
	RelayID           string   `json:"relay_id"`
	ProtocolID        string   `json:"ingress_protocol_id"`
	ProtocolVersion   string   `json:"ingress_protocol_version"`
	Address           string   `json:"listener_address"`
	Port              int      `json:"listener_port"`
	SourceCode        string   `json:"source_code"`
	ProducerIDs       []string `json:"producer_principal_ids"`
	Segregated        bool     `json:"upstream_segregated"`
	Synthetic         bool     `json:"synthetic"`
	Exclusive         bool     `json:"exclusive"`
	FixedTerminal     string   `json:"fixed_terminal,omitempty"`
	IdleSeconds       int      `json:"idle_seconds"`
	Budget            string   `json:"budget_bytes"`
}
type Command struct {
	Meta
	Revision string `json:"expected_revision"`
	Action   string `json:"action"`
	Digest   string `json:"digest,omitempty"`
}
type Terminal struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	SourceID      string    `json:"source_id"`
	Key           string    `json:"terminal_key"`
	Name          string    `json:"name"`
	Mode          string    `json:"mode"`
	Revision      string    `json:"revision"`
	Enabled       bool      `json:"enabled"`
	Demultiplexer *i.Method `json:"demultiplexer"`
	CreatedAt     time.Time `json:"created_at"`
}
type TerminalInput struct {
	Meta
	Revision      string    `json:"expected_revision,omitempty"`
	Enabled       *bool     `json:"enabled,omitempty"`
	SourceID      string    `json:"source_id"`
	Key           string    `json:"terminal_key"`
	Name          string    `json:"name"`
	Mode          string    `json:"mode"`
	Demultiplexer *i.Method `json:"demultiplexer,omitempty"`
}
type Association struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	SourceID         string    `json:"source_id"`
	TerminalID       string    `json:"terminal_id"`
	TerminalRevision string    `json:"terminal_revision"`
	ChildKey         string    `json:"child_key"`
	DeviceID         string    `json:"device_id"`
	Epoch            string    `json:"epoch"`
	StartSequence    string    `json:"start_sequence"`
	EndSequence      *string   `json:"end_sequence"`
	Enabled          bool      `json:"enabled"`
	InputView        string    `json:"input_view"`
	Demultiplexer    *i.Method `json:"demultiplexer"`
	CreatedAt        time.Time `json:"created_at"`
}
type AssociationInput struct {
	Meta
	Revision  string `json:"expected_revision"`
	ChildKey  string `json:"child_key"`
	DeviceID  string `json:"device_id"`
	InputView string `json:"input_view"`
}
type GrantInput struct {
	Meta
	Revision   string `json:"expected_revision"`
	SourceID   string `json:"source_id"`
	TerminalID string `json:"terminal_id,omitempty"`
	Identify   bool   `json:"identify"`
	Measure    bool   `json:"measure"`
	Enabled    bool   `json:"enabled"`
	Budget     int    `json:"budget"`
}
type Grant struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	SourceID   string    `json:"source_id"`
	TerminalID *string   `json:"terminal_id"`
	Identify   bool      `json:"identify"`
	Measure    bool      `json:"measure"`
	Enabled    bool      `json:"enabled"`
	Generation string    `json:"generation"`
	Budget     int       `json:"budget"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type Receipt struct {
	ID             string     `json:"id"`
	RelayID        string     `json:"relay_id"`
	Epoch          string     `json:"epoch"`
	ReceiptID      string     `json:"receipt_id"`
	SourceID       string     `json:"source_id"`
	SourceRevision string     `json:"source_revision"`
	SourceSequence string     `json:"source_sequence"`
	TenantID       *string    `json:"tenant_id"`
	ProjectID      *string    `json:"project_id"`
	Envelope       r.Envelope `json:"envelope"`
	EnvelopeDigest string     `json:"envelope_digest"`
	ObjectID       string     `json:"object_id"`
	State          string     `json:"state"`
	ErrorCode      string     `json:"error_code"`
	ReceivedAt     time.Time  `json:"received_at"`
	StoredAt       time.Time  `json:"stored_at"`
	Generation     string     `json:"current_generation"`
}
type Decision struct {
	ID                 string           `json:"id"`
	ReceiptID          string           `json:"receipt_id"`
	Generation         string           `json:"generation"`
	Method             i.Identification `json:"method"`
	State              string           `json:"state"`
	ErrorCode          string           `json:"error_code"`
	Fence              int64            `json:"fence,string"`
	Attempts           int              `json:"attempts"`
	GrantID            *string          `json:"grant_id"`
	GrantGeneration    *string          `json:"grant_generation"`
	RecoveryID         *string          `json:"recovery_id"`
	RecoveryGeneration *string          `json:"recovery_generation"`
	Response           *i.Response      `json:"response"`
}
type Unit struct {
	ID            string          `json:"id"`
	ReceiptID     string          `json:"receipt_id"`
	DecisionID    string          `json:"decision_id"`
	Key           string          `json:"unit_key"`
	TerminalKey   string          `json:"terminal_key"`
	ChildKey      string          `json:"child_key"`
	View          i.View          `json:"view"`
	MessageKind   string          `json:"message_kind"`
	TenantID      *string         `json:"tenant_id"`
	DeviceID      *string         `json:"device_id"`
	TerminalID    *string         `json:"terminal_id"`
	AssociationID *string         `json:"association_id"`
	State         string          `json:"state"`
	ErrorCode     string          `json:"error_code"`
	Assignment    json.RawMessage `json:"assignment"`
	CreatedAt     time.Time       `json:"created_at"`
}
type PlanSnapshot struct {
	ReceiptID          string                     `json:"receipt_id"`
	Source             r.Source                   `json:"source"`
	Terminal           Terminal                   `json:"terminal"`
	Association        Association                `json:"association"`
	Grant              Grant                      `json:"execution_grant"`
	SourceGrant        Grant                      `json:"source_grant"`
	Compatibility      i.Compatibility            `json:"compatibility"`
	Parsing            parsing.ProductionSnapshot `json:"parsing"`
	Input              i.View                     `json:"input"`
	ReceivedAt         time.Time                  `json:"received_at"`
	TraceID            string                     `json:"trace_id"`
	ProjectGrantID     string                     `json:"project_device_grant_id"`
	RecoveryGeneration string                     `json:"recovery_generation,omitempty"`
	RecoveryID         string                     `json:"recovery_id,omitempty"`
}
type Plan struct {
	ID         string       `json:"id"`
	TenantID   string       `json:"tenant_id"`
	UnitID     string       `json:"unit_id"`
	DecisionID string       `json:"decision_id"`
	DeviceID   string       `json:"device_id"`
	AdoptionID string       `json:"adoption_id"`
	Snapshot   PlanSnapshot `json:"snapshot"`
	State      string       `json:"state"`
	ErrorCode  string       `json:"error_code"`
	Fence      int64        `json:"fence,string"`
	Attempts   int          `json:"attempts"`
	CreatedAt  time.Time    `json:"created_at"`
	FinishedAt *time.Time   `json:"finished_at"`
}
type RecoveryInput struct {
	Identification *i.Method `json:"identification,omitempty"`
	Meta
	Kind             string   `json:"kind"`
	ReceiptIDs       []string `json:"receipt_ids"`
	UnitIDs          []string `json:"unit_ids"`
	AssociationID    string   `json:"association_id,omitempty"`
	ConfigRevisionID string   `json:"config_revision_id,omitempty"`
	Epoch            string   `json:"epoch,omitempty"`
	RelayID          string   `json:"relay_id,omitempty"`
}
