// Package parsing owns fixed adoption, samples and execution evidence. It never
// interprets protocol bytes or writes standard monitoring observations.
package parsing

import (
	"context"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"io"
	"time"
)

type Scope = k.Scope
type Meta = k.Meta
type Filter = k.Filter
type Result struct {
	OtherID          string          `json:"other_id,omitempty"`
	ID               string          `json:"id"`
	Revision         string          `json:"revision"`
	ConfigRevisionID string          `json:"config_revision_id,omitempty"`
	Generation       string          `json:"generation,omitempty"`
	TaskRef          *taskrecord.Ref `json:"task_ref,omitempty"`
	Replayed         bool            `json:"replayed"`
	State            string          `json:"state,omitempty"`
	ErrorCode        string          `json:"error_code,omitempty"`
}
type CatalogStatus struct {
	ID        string     `json:"id"`
	State     string     `json:"state"`
	ErrorCode string     `json:"error_code"`
	CreatedAt *time.Time `json:"created_at"`
	Added     int        `json:"added"`
}
type Method struct {
	ID               string    `json:"id"`
	Entry            c.Entry   `json:"entry"`
	State            string    `json:"state"`
	Revision         string    `json:"revision"`
	Available        bool      `json:"available"`
	ExecutionEnabled bool      `json:"execution_enabled"`
	CreatedAt        time.Time `json:"created_at"`
}
type StateInput struct {
	Meta
	Revision string `json:"expected_revision"`
	State    string `json:"state"`
}
type Binding struct {
	OutputKey string `json:"output_key"`
	SlotKey   string `json:"slot_key"`
	Confirmed bool   `json:"confirmed"`
}
type Ignored struct {
	OutputKey string `json:"output_key"`
	Reason    string `json:"reason"`
}
type Mapping struct {
	OutputKey          string       `json:"output_key"`
	SlotKey            string       `json:"slot_key,omitempty"`
	Status             string       `json:"status"`
	Reason             string       `json:"reason,omitempty"`
	MethodSemantics    c.Semantics  `json:"method_semantics"`
	SlotSemantics      *d.Semantics `json:"slot_semantics,omitempty"`
	OutputID           string       `json:"output_id,omitempty"`
	OutputGeneration   int          `json:"output_generation,omitempty"`
	OutputSemanticHash string       `json:"output_semantic_hash,omitempty"`
}
type Snapshot struct {
	OwnerKind            string    `json:"owner_kind"`
	OwnerID              string    `json:"owner_id"`
	TenantID             string    `json:"tenant_id,omitempty"`
	ProductVersionID     string    `json:"product_version_id"`
	Method               c.Method  `json:"method"`
	OutputContractDigest string    `json:"output_contract_digest"`
	Bindings             []Binding `json:"output_bindings"`
	Ignored              []Ignored `json:"ignored_outputs"`
	Mapping              []Mapping `json:"mapping"`
	Example              bool      `json:"example"`
	SourceRevisionID     string    `json:"source_revision_id,omitempty"`
}
type ConfigInput struct {
	Meta
	Revision         string    `json:"expected_revision"`
	Name             string    `json:"name"`
	MethodID         string    `json:"method_id"`
	MethodVersion    string    `json:"method_version"`
	Bindings         []Binding `json:"output_bindings"`
	Ignored          []Ignored `json:"ignored_outputs"`
	SourceRevisionID string    `json:"source_revision_id,omitempty"`
}
type ConfigRevision struct {
	ID               string    `json:"id"`
	ConfigID         string    `json:"config_id"`
	ProductVersionID string    `json:"product_version_id"`
	Number           string    `json:"number"`
	Name             string    `json:"name"`
	Enabled          bool      `json:"enabled"`
	Revision         string    `json:"revision"`
	Snapshot         Snapshot  `json:"snapshot"`
	ContentDigest    string    `json:"content_digest"`
	CreatedBy        string    `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	Reason           string    `json:"reason"`
}
type Default struct {
	ProductVersionID string          `json:"product_version_id"`
	Revision         string          `json:"revision"`
	Config           *ConfigRevision `json:"config"`
}
type DefaultInput struct {
	Meta
	Revision         string `json:"expected_revision"`
	ConfigRevisionID string `json:"config_revision_id"`
}
type Adoption struct {
	ID                    string     `json:"id"`
	DeviceID              string     `json:"device_id"`
	Mode                  string     `json:"mode"`
	Revision              string     `json:"revision"`
	ConfigRevisionID      *string    `json:"config_revision_id"`
	SourceDefaultRevision *string    `json:"source_default_revision"`
	Snapshot              *Snapshot  `json:"snapshot"`
	StartedAt             time.Time  `json:"started_at"`
	EndedAt               *time.Time `json:"ended_at"`
	ActorID               string     `json:"actor_id"`
	Reason                string     `json:"reason"`
}
type DeviceState struct {
	Revision       string    `json:"revision"`
	Mode           string    `json:"mode"`
	Current        *Adoption `json:"current"`
	Default        Default   `json:"product_default"`
	DefaultChanged bool      `json:"default_changed"`
	Mapping        []Mapping `json:"mapping"`
	CanManage      bool      `json:"can_manage"`
	CanTest        bool      `json:"can_test"`
}
type Command struct {
	Meta
	Action           string `json:"action"`
	Revision         string `json:"expected_revision"`
	DefaultRevision  string `json:"expected_default_revision,omitempty"`
	ConfigRevisionID string `json:"config_revision_id,omitempty"`
	AdoptionID       string `json:"adoption_id,omitempty"`
}
type BatchDevice struct {
	DeviceID string `json:"device_id"`
	Revision string `json:"expected_revision"`
}
type BatchCommand struct {
	Command
	Devices []BatchDevice `json:"devices"`
}
type Preview struct {
	CanSubmit bool            `json:"can_submit"`
	Current   DeviceState     `json:"current"`
	Target    *ConfigRevision `json:"target"`
	Mapping   []Mapping       `json:"mapping"`
	Issues    []k.Issue       `json:"issues"`
}
type SampleInput struct {
	Meta
	Representation string    `json:"representation"`
	Source         string    `json:"source"`
	ReceivedAt     time.Time `json:"received_at"`
}
type Sample struct {
	ID                     string    `json:"id"`
	Representation         string    `json:"representation"`
	Source                 string    `json:"source"`
	State                  string    `json:"state"`
	SHA256                 string    `json:"sha256"`
	ByteLength             int       `json:"byte_length"`
	ReceivedAt             time.Time `json:"received_at"`
	CreatedAt              time.Time `json:"created_at"`
	CreatedBy              string    `json:"created_by"`
	ObjectID               string    `json:"-"`
	RepresentationObjectID string    `json:"-"`
}
type RunInput struct {
	Meta
	Kind             string `json:"kind"`
	SampleID         string `json:"sample_id"`
	MethodID         string `json:"method_id,omitempty"`
	MethodVersion    string `json:"method_version,omitempty"`
	ConfigRevisionID string `json:"config_revision_id,omitempty"`
	DeviceID         string `json:"device_id,omitempty"`
	Transport        string `json:"transport"`
	ComparedToRunID  string `json:"compared_to_run_id,omitempty"`
	ReplayRunID      string `json:"replay_run_id,omitempty"`
}
type FrozenInput struct {
	Sample       Sample          `json:"sample"`
	Context      c.Context       `json:"context"`
	Entry        c.Entry         `json:"entry"`
	Config       *ConfigRevision `json:"config"`
	AdoptionID   string          `json:"adoption_id,omitempty"`
	Mapping      []Mapping       `json:"mapping"`
	DeviceStatus string          `json:"device_status,omitempty"`
	TraceID      string          `json:"trace_id"`
}
type Run struct {
	MethodID         string         `json:"method_id"`
	MethodVersion    string         `json:"method_version"`
	MethodName       string         `json:"method_name"`
	ID               string         `json:"id"`
	ScopeKey         string         `json:"-"`
	TenantID         *string        `json:"tenant_id"`
	Kind             string         `json:"kind"`
	SampleID         string         `json:"sample_id"`
	DeviceID         *string        `json:"device_id"`
	ProductVersionID *string        `json:"product_version_id"`
	Input            FrozenInput    `json:"input_snapshot"`
	ComparedToRunID  *string        `json:"compared_to_run_id"`
	Generation       string         `json:"current_generation"`
	Revision         string         `json:"revision"`
	ControlRevision  string         `json:"control_revision"`
	CreatedBy        string         `json:"created_by"`
	CreatedAt        time.Time      `json:"created_at"`
	State            string         `json:"state"`
	ErrorCode        string         `json:"error_code"`
	TaskRef          taskrecord.Ref `json:"task_ref"`
}
type Execution struct {
	RunID          string     `json:"run_id"`
	Generation     string     `json:"generation"`
	ActorID        string     `json:"actor_id"`
	State          string     `json:"state"`
	Fence          string     `json:"fence"`
	ResponseStatus *string    `json:"response_status"`
	ErrorCode      string     `json:"error_code"`
	Mapping        []Mapping  `json:"mapping"`
	ResultObjectID *string    `json:"result_object_id"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	Attempts       []Attempt  `json:"attempts"`
}
type Attempt struct {
	ExecutionID string     `json:"execution_id"`
	Attempt     int        `json:"attempt"`
	Fence       string     `json:"fence"`
	State       string     `json:"state"`
	ErrorCode   string     `json:"error_code"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}
type RunDetail struct {
	Run            Run         `json:"run"`
	Executions     []Execution `json:"executions"`
	CurrentChanged bool        `json:"current_changed"`
	CanReadSample  bool        `json:"can_read_sample"`
}
type ResultView struct {
	ResultDigest string      `json:"result_digest,omitempty"`
	State        string      `json:"state"`
	Response     *c.Response `json:"response"`
	Mapping      []Mapping   `json:"mapping"`
	Quality      string      `json:"business_quality"`
	Adopted      bool        `json:"formally_adopted"`
	NextCursor   string      `json:"next_cursor"`
}
type CompareInput struct {
	Meta
	Left  RunInput `json:"left"`
	Right RunInput `json:"right"`
}
type CompareResult struct {
	Left  Result `json:"left"`
	Right Result `json:"right"`
}
type ObjectStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Delete(context.Context, string) error
}
type Options struct {
	Enabled            bool
	URL, TokenFile     string
	Timeout            time.Duration
	Queue, Concurrency int
	StorageBudget      int64
}

func DefaultOptions() Options {
	return Options{Timeout: 10 * time.Second, Queue: 20, Concurrency: 2, StorageBudget: 10 << 30}
}
