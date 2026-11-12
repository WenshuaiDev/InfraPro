// Package devicecatalog owns public and tenant-private device product definitions.
package devicecatalog

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
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

func PlatformScope() Scope        { return k.Platform() }
func TenantScope(id string) Scope { return k.Tenant(id) }

// Semantics is the exact compatibility vector; only declared text normalization
// is applied, never unit conversions, inferred directions or metric aliases.
type Semantics struct {
	MetricVersionID        string  `json:"metric_version_id"`
	UnitVersionID          *string `json:"unit_version_id"`
	ValueShape             string  `json:"value_shape"`
	TimestampMeaning       string  `json:"timestamp_meaning"`
	AggregationDescription string  `json:"aggregation_description"`
	ObservationDirection   string  `json:"observation_direction"`
	ReferenceBasis         string  `json:"reference_basis"`
}

func (s *Semantics) Normalize() {
	s.AggregationDescription = k.Text(s.AggregationDescription)
	s.ObservationDirection = k.Text(s.ObservationDirection)
	s.ReferenceBasis = k.Text(s.ReferenceBasis)
}
func (s Semantics) Hash() string { s.Normalize(); return k.Hash(s) }
func Compatible(output, channel Semantics) []Issue {
	output.Normalize()
	channel.Normalize()
	issues := []Issue{}
	a, b := map[string]any{}, map[string]any{}
	_ = json.Unmarshal(k.JSON(output), &a)
	_ = json.Unmarshal(k.JSON(channel), &b)
	for _, field := range []string{"metric_version_id", "unit_version_id", "value_shape", "timestamp_meaning", "aggregation_description", "observation_direction", "reference_basis"} {
		if k.Hash(a[field]) != k.Hash(b[field]) {
			issues = append(issues, Issue{Path: "/" + field, Code: "incompatible_" + map[string]string{"metric_version_id": "metric", "unit_version_id": "unit", "value_shape": "shape", "timestamp_meaning": "time", "aggregation_description": "aggregation", "observation_direction": "direction", "reference_basis": "reference"}[field], Message: "输出与业务通道的" + map[string]string{"metric_version_id": "指标版本", "unit_version_id": "单位版本", "value_shape": "值形态", "timestamp_meaning": "时间含义", "aggregation_description": "统计含义", "observation_direction": "观测方向", "reference_basis": "参考基准"}[field] + "不一致。"})
		}
	}
	return issues
}

type Slot struct {
	Key                    string   `json:"key"`
	Name                   string   `json:"name"`
	Mode                   string   `json:"mode"`
	Description            string   `json:"description"`
	Order                  int      `json:"order"`
	MetricVersionID        string   `json:"metric_version_id,omitempty"`
	UnitVersionID          *string  `json:"unit_version_id,omitempty"`
	TimestampMeaning       string   `json:"timestamp_meaning,omitempty"`
	AggregationDescription string   `json:"aggregation_description,omitempty"`
	AllowedShapes          []string `json:"allowed_shapes,omitempty"`
}

func (s *Slot) UnmarshalJSON(raw []byte) error {
	type plain Slot
	var p plain
	if e := k.Strict(raw, &p); e != nil {
		return e
	}
	fields := map[string]json.RawMessage{}
	if e := json.Unmarshal(raw, &fields); e != nil {
		return e
	}
	forbidden := []string{}
	if p.Mode == "fixed" {
		forbidden = []string{"allowed_shapes"}
	} else if p.Mode == "configurable" {
		forbidden = []string{"metric_version_id", "unit_version_id", "timestamp_meaning", "aggregation_description"}
	}
	for _, key := range forbidden {
		if _, ok := fields[key]; ok {
			return k.Problem("invalid_request", "输出槽位包含不适用于当前模式的字段。", 400)
		}
	}
	*s = Slot(p)
	return nil
}
func (s Slot) Core() string {
	if s.Mode == "configurable" {
		return k.Hash(s.Mode)
	}
	return k.Hash([]any{s.Mode, s.MetricVersionID, s.UnitVersionID, s.TimestampMeaning, k.Text(s.AggregationDescription)})
}

type Content struct {
	SchemaVersion int    `json:"schema_version"`
	Name          string `json:"name"`
	Model         string `json:"model"`
	Description   string `json:"description"`
	Purpose       string `json:"purpose"`
	OutputSlots   []Slot `json:"output_slots"`
	ReleaseNotes  string `json:"release_notes"`
}
type Manufacturer struct {
	ID            string     `json:"id"`
	OwnerTenantID *string    `json:"owner_tenant_id"`
	Code          string     `json:"code"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Status        string     `json:"status"`
	Revision      string     `json:"revision"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeletedAt     *time.Time `json:"deleted_at"`
	Actions       Actions    `json:"actions"`
}
type Actions struct {
	Edit    bool `json:"edit"`
	Publish bool `json:"publish"`
	State   bool `json:"state"`
	Delete  bool `json:"delete"`
	Copy    bool `json:"copy"`
	Draft   bool `json:"draft"`
}
type Product struct {
	HasDraft             bool       `json:"has_draft"`
	LatestVersionNumber  *int       `json:"latest_version_number"`
	LatestVersionEnabled *bool      `json:"latest_version_enabled"`
	ID                   string     `json:"id"`
	OwnerTenantID        *string    `json:"owner_tenant_id"`
	ManufacturerID       string     `json:"manufacturer_id"`
	ManufacturerName     string     `json:"manufacturer_name"`
	Code                 string     `json:"code"`
	Revision             string     `json:"revision"`
	Name                 string     `json:"name"`
	Model                string     `json:"model"`
	CreatedAt            time.Time  `json:"created_at"`
	DeletedAt            *time.Time `json:"deleted_at"`
	Actions              Actions    `json:"actions"`
	Draft                *Draft     `json:"draft,omitempty"`
	Versions             []Version  `json:"versions"`
	NextCursor           string     `json:"next_cursor"`
}
type Draft struct {
	Content             Content   `json:"content"`
	Revision            string    `json:"revision"`
	BaseVersionID       *string   `json:"base_version_id"`
	CopiedFromVersionID *string   `json:"copied_from_version_id"`
	UpdatedAt           time.Time `json:"updated_at"`
}
type Version struct {
	ManufacturerID   string                `json:"manufacturer_id"`
	ManufacturerName string                `json:"manufacturer_name"`
	ID               string                `json:"id"`
	ProductID        string                `json:"product_id"`
	Number           int                   `json:"number"`
	Content          Content               `json:"content"`
	ContentHash      string                `json:"content_hash"`
	PublishedAt      time.Time             `json:"published_at"`
	Enabled          bool                  `json:"enabled"`
	Revision         string                `json:"revision"`
	Usable           bool                  `json:"usable"`
	Issues           []Issue               `json:"issues"`
	Definitions      []engineering.Version `json:"definitions"`
}
type ManufacturerCreate struct {
	Meta
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}
type ManufacturerUpdate struct {
	RevisionInput
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}
type ManufacturerState struct {
	RevisionInput
	Status string `json:"target_status"`
}
type ManufacturerMutation struct {
	Action string `json:"action"`
	ManufacturerUpdate
	Status string `json:"target_status,omitempty"`
}
type ProductCreate struct {
	Meta
	ManufacturerID string `json:"manufacturer_id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Model          string `json:"model"`
	Description    string `json:"description,omitempty"`
	Purpose        string `json:"purpose,omitempty"`
	OutputSlots    []Slot `json:"output_slots,omitempty"`
	ReleaseNotes   string `json:"release_notes,omitempty"`
}
type DraftSave struct {
	Meta
	Revision string  `json:"expected_draft_revision"`
	Content  Content `json:"content"`
}
type DraftCreate struct {
	RevisionInput
	VersionID string `json:"version_id"`
}
type DraftRevision struct {
	Meta
	Revision string `json:"expected_draft_revision"`
}
type ProductCopy struct {
	Meta
	SourceVersionID string  `json:"source_version_id"`
	ManufacturerID  string  `json:"manufacturer_id"`
	Code            string  `json:"code"`
	Name            *string `json:"name,omitempty"`
	Model           *string `json:"model,omitempty"`
}
type VersionState struct {
	RevisionInput
	Enabled *bool `json:"enabled"`
}
type ProductMutation struct {
	Action string `json:"action"`
	VersionState
	VersionID string `json:"version_id,omitempty"`
}
type Validation struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues"`
}
type Change struct {
	ID             string          `json:"id"`
	ManufacturerID *string         `json:"manufacturer_id"`
	ProductID      *string         `json:"product_id"`
	VersionID      *string         `json:"version_id"`
	Operation      string          `json:"operation"`
	ActorName      string          `json:"actor_name"`
	Reason         string          `json:"reason"`
	Before         json.RawMessage `json:"before"`
	After          json.RawMessage `json:"after"`
	ChangedAt      time.Time       `json:"changed_at"`
}

// DecodeRequest applies the shared device contract without exposing transaction helpers.
func DecodeRequest(raw []byte, value any) error { return k.Strict(raw, value) }
