// Package devices owns tenant hardware, independent output generations,
// installations and explicit project device/output grants.
package devices

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/location"
	"github.com/jackc/pgx/v5"
	"time"
)

type Scope = k.Scope
type Meta = k.Meta
type RevisionInput = k.RevisionInput
type Result = k.Result
type Preview = k.Preview
type Error = k.Error
type Issue = k.Issue
type Filter = k.Filter

func TenantScope(id string) Scope               { return k.Tenant(id) }
func ProjectScope(tenant, project string) Scope { return k.Project(tenant, project) }

type Actions struct {
	Edit      bool   `json:"edit"`
	State     bool   `json:"state"`
	Delete    bool   `json:"delete"`
	Install   bool   `json:"install"`
	Configure bool   `json:"configure"`
	History   bool   `json:"history"`
	UseSource bool   `json:"use_source"`
	Reason    string `json:"reason"`
}
type ProductSummary struct {
	ID               string `json:"id"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	Model            string `json:"model"`
	VersionID        string `json:"version_id"`
	Number           int    `json:"number"`
	ManufacturerID   string `json:"manufacturer_id"`
	ManufacturerName string `json:"manufacturer_name"`
	Source           string `json:"source"`
}
type Device struct {
	ID                   string         `json:"id"`
	TenantID             string         `json:"tenant_id"`
	Code                 string         `json:"code"`
	Name                 string         `json:"name"`
	SerialNumber         string         `json:"serial_number"`
	Description          string         `json:"description"`
	Status               string         `json:"status"`
	ProductID            string         `json:"product_id"`
	ProductVersionID     string         `json:"product_version_id"`
	Product              ProductSummary `json:"product"`
	Revision             string         `json:"revision"`
	OutputRevision       string         `json:"output_revision"`
	InstallationRevision string         `json:"installation_revision"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            *time.Time     `json:"deleted_at"`
	Visibility           string         `json:"visibility"`
	Actions              Actions        `json:"actions"`
	CurrentInstallation  *Installation  `json:"current_installation"`
}

// DeviceContext deliberately cannot carry serial numbers, location or sibling counts.
type DeviceContext struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	ProductName  string `json:"product_name"`
	ProductModel string `json:"product_model"`
}
type Output struct {
	Definitions []engineering.Version `json:"definitions"`
	c.Semantics
	ID               string        `json:"id"`
	TenantID         string        `json:"tenant_id"`
	DeviceID         string        `json:"device_id"`
	ProductVersionID string        `json:"product_version_id"`
	SlotKey          string        `json:"slot_key"`
	Generation       int           `json:"generation"`
	Name             string        `json:"name"`
	Description      string        `json:"description"`
	SemanticHash     string        `json:"semantic_hash"`
	Revision         string        `json:"revision"`
	StartedAt        time.Time     `json:"started_at"`
	EndedAt          *time.Time    `json:"ended_at"`
	Device           DeviceContext `json:"device"`
	DeviceStatus     string        `json:"device_status"`
	DeviceRevision   string        `json:"device_revision"`
	Template         c.Slot        `json:"template"`
	Occupied         bool          `json:"occupied"`
	Actions          Actions       `json:"actions"`
	Visibility       string        `json:"visibility"`
}
type OutputSlot struct {
	Definitions []engineering.Version `json:"definitions"`
	Template    c.Slot                `json:"template"`
	Current     *Output               `json:"current"`
}
type DeviceCreate struct {
	Meta
	Status           string `json:"status,omitempty"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	ProductVersionID string `json:"product_version_id"`
	SerialNumber     string `json:"serial_number,omitempty"`
	Description      string `json:"description,omitempty"`
}
type DeviceUpdate struct {
	RevisionInput
	Name         *string `json:"name,omitempty"`
	SerialNumber *string `json:"serial_number,omitempty"`
	Description  *string `json:"description,omitempty"`
}
type DeviceState struct {
	RevisionInput
	Status string `json:"status"`
}
type DeviceMutation struct {
	Action string `json:"action"`
	DeviceUpdate
	Status string `json:"status,omitempty"`
}
type OutputFields struct {
	FixedCorePresent       bool    `json:"-"`
	Name                   string  `json:"name"`
	Description            string  `json:"description,omitempty"`
	MetricVersionID        string  `json:"metric_version_id,omitempty"`
	UnitVersionID          *string `json:"unit_version_id,omitempty"`
	TimestampMeaning       string  `json:"timestamp_meaning,omitempty"`
	AggregationDescription string  `json:"aggregation_description,omitempty"`
	ObservationDirection   string  `json:"observation_direction,omitempty"`
	ReferenceBasis         string  `json:"reference_basis,omitempty"`
}
type OutputConfigure struct {
	Meta
	OutputFields
	SlotKey        string `json:"slot_key"`
	OutputRevision string `json:"expected_output_revision"`
}
type OutputReplace struct {
	RevisionInput
	OutputFields
	OutputRevision string `json:"expected_output_revision"`
}
type OutputEnd struct {
	RevisionInput
	OutputRevision string `json:"expected_output_revision"`
}
type OutputUpdate struct {
	RevisionInput
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}
type Target struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}
type Installation struct {
	ID                  string             `json:"id"`
	DeviceID            string             `json:"device_id"`
	TargetKind          string             `json:"target_kind"`
	FacilityID          *string            `json:"facility_id,omitempty"`
	ComponentID         *string            `json:"component_id,omitempty"`
	PointID             *string            `json:"point_id,omitempty"`
	TargetName          string             `json:"target_name,omitempty"`
	TargetStatus        string             `json:"target_status,omitempty"`
	Visibility          string             `json:"visibility"`
	LocationDescription string             `json:"location_description,omitempty"`
	Position            *location.Position `json:"position,omitempty"`
	InstalledAt         time.Time          `json:"installed_at"`
	UninstalledAt       *time.Time         `json:"uninstalled_at"`
	Revision            string             `json:"revision"`
	RecordedAt          time.Time          `json:"recorded_at"`
	UpdatedAt           time.Time          `json:"updated_at"`
}
type InstallationFields struct {
	Target              Target             `json:"target"`
	LocationDescription string             `json:"location_description,omitempty"`
	Position            *location.Position `json:"position"`
}
type InstallationCreate struct {
	Meta
	InstallationFields
	InstallationRevision string `json:"expected_installation_revision"`
	InstalledAt          string `json:"installed_at,omitempty"`
	UninstalledAt        string `json:"uninstalled_at,omitempty"`
}
type InstallationEnd struct {
	RevisionInput
	InstallationRevision string `json:"expected_installation_revision"`
	UninstalledAt        string `json:"uninstalled_at,omitempty"`
}
type InstallationRelocate struct {
	RevisionInput
	InstallationFields
	InstallationRevision string `json:"expected_installation_revision"`
	MovedAt              string `json:"moved_at,omitempty"`
}
type InstallationCorrect struct {
	RevisionInput
	InstallationFields
	InstallationRevision string  `json:"expected_installation_revision"`
	InstalledAt          string  `json:"installed_at"`
	UninstalledAt        *string `json:"uninstalled_at"`
}

// InstallationResources is a narrow transaction seam. Implementations must check
// current role and full object visibility; context-only ancestors never qualify.
// Resolve derives all redundant IDs and rejects deleted targets, permitting
// retired ones only for a closed historical period. The caller owns coordination.
type InstallationResources interface {
	ResolveInstallationTx(context.Context, pgx.Tx, access.Actor, Scope, Target, bool) (Installation, error)
	ProjectInstallationsTx(context.Context, pgx.Tx, access.Actor, Scope, []Installation) ([]Installation, error)
	InstallationTargetsTx(context.Context, pgx.Tx, access.Actor, Scope, Filter, bool) (k.Page[InstallationTarget], error)
}
type InstallationTarget struct {
	Kind       string    `json:"kind"`
	ID         string    `json:"id"`
	FacilityID string    `json:"facility_id"`
	Name       string    `json:"name"`
	Code       string    `json:"code"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}
type Change struct {
	ID             string          `json:"id"`
	DeviceID       string          `json:"device_id"`
	OutputID       *string         `json:"output_id"`
	InstallationID *string         `json:"installation_id"`
	BindingID      *string         `json:"binding_id"`
	Operation      string          `json:"operation"`
	ActorName      string          `json:"actor_name,omitempty"`
	ActorScope     string          `json:"actor_scope"`
	Reason         string          `json:"reason,omitempty"`
	Before         json.RawMessage `json:"before,omitempty"`
	After          json.RawMessage `json:"after,omitempty"`
	ChangedFields  []string        `json:"changed_fields"`
	ChangedAt      time.Time       `json:"changed_at"`
	Visibility     string          `json:"visibility"`
}
