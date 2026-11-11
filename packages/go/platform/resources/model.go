// Package resources owns actual tenant facilities, components, points and channels.
package resources

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"time"
)

type Kind string

const (
	Facility  Kind = "facility"
	Component Kind = "component"
	Point     Kind = "point"
	Channel   Kind = "channel"
)

var tables = map[Kind]string{Facility: "engineering_facilities", Component: "engineering_components", Point: "engineering_points", Channel: "engineering_channels"}

type Target struct {
	Facility, Point, ID string
	Kind                Kind
}
type WriteMeta struct {
	AccessRevision    string `json:"expected_access_revision,omitempty"`
	LinkRevision      string `json:"expected_link_revision,omitempty"`
	PreviewHash       string `json:"preview_hash,omitempty"`
	RequestID         string `json:"request_id"`
	Revision          int    `json:"expected_revision"`
	StructureRevision int    `json:"expected_structure_revision"`
	Reason            string `json:"reason"`
}
type Result struct {
	ProjectID         string `json:"project_id,omitempty"`
	LinkID            string `json:"link_id,omitempty"`
	LinkRevision      string `json:"link_revision,omitempty"`
	AccessRevision    string `json:"access_revision,omitempty"`
	ID                string `json:"id"`
	FacilityID        string `json:"facility_id"`
	Revision          int    `json:"revision"`
	StructureRevision int    `json:"structure_revision"`
	Changed           bool   `json:"changed"`
	Replayed          bool   `json:"replayed"`
}
type Resource struct {
	CreatedProjectID        *string        `json:"created_project_id,omitempty"`
	Visibility              string         `json:"visibility,omitempty"`
	HistoricalContextHidden bool           `json:"historical_context_hidden,omitempty"`
	ID                      string         `json:"id"`
	TenantID                string         `json:"tenant_id"`
	FacilityID              string         `json:"facility_id,omitempty"`
	Kind                    Kind           `json:"kind"`
	Code                    string         `json:"code"`
	Name                    string         `json:"name"`
	Status                  string         `json:"status"`
	Revision                int            `json:"revision"`
	StructureRevision       int            `json:"structure_revision,omitempty"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
	CreatedBy               string         `json:"created_by,omitempty"`
	UpdatedBy               string         `json:"updated_by,omitempty"`
	DeletedAt               *time.Time     `json:"deleted_at"`
	TypeVersionID           string         `json:"type_version_id,omitempty"`
	CategoryKey             string         `json:"category_key,omitempty"`
	ParentComponentID       *string        `json:"parent_component_id"`
	ComponentID             *string        `json:"component_id"`
	PointID                 string         `json:"point_id,omitempty"`
	Values                  map[string]any `json:"values,omitempty"`
	Position                *Position      `json:"position,omitempty"`
	Description             string         `json:"description"`
	SourceReference         string         `json:"source_reference"`
	Purpose                 string         `json:"purpose"`
	DirectionDescription    string         `json:"direction_description"`
	ReferenceDescription    string         `json:"reference_description"`
	MetricVersionID         string         `json:"metric_version_id,omitempty"`
	UnitVersionID           *string        `json:"unit_version_id"`
	ObservationDirection    string         `json:"observation_direction"`
	ReferenceBasis          string         `json:"reference_basis"`
	TimestampMeaning        string         `json:"timestamp_meaning"`
	AggregationDescription  string         `json:"aggregation_description"`
	Sampling                *Sampling      `json:"sampling"`
}
type Sampling struct {
	Mode            string   `json:"mode"`
	IntervalSeconds *float64 `json:"interval_seconds,omitempty"`
	Description     string   `json:"description"`
}
type Actions struct {
	Read            bool   `json:"read"`
	CreateComponent bool   `json:"create_component"`
	CreatePoint     bool   `json:"create_point"`
	CreateChannel   bool   `json:"create_channel"`
	Edit            bool   `json:"edit"`
	Add             bool   `json:"add"`
	Retire          bool   `json:"retire"`
	Restore         bool   `json:"restore"`
	Delete          bool   `json:"delete"`
	Reason          string `json:"reason"`
}
type Item struct {
	Resource
	HasChildren      bool   `json:"has_children"`
	TypeName         string `json:"type_name,omitempty"`
	TypeNumber       int    `json:"type_number,omitempty"`
	TypeDefinitionID string `json:"type_definition_id,omitempty"`
}
type Detail struct {
	AccessRevision string `json:"access_revision,omitempty"`
	LinkID         string `json:"link_id,omitempty"`
	LinkRevision   string `json:"link_revision,omitempty"`
	Shared         bool   `json:"shared"`
	Resource
	Ancestors   []Resource            `json:"ancestors"`
	Definitions []engineering.Version `json:"definitions"`
	Actions     Actions               `json:"actions"`
}
type Page struct {
	AccessRevision    string `json:"access_revision,omitempty"`
	CanCreateFacility bool   `json:"can_create_facility"`
	Items             []Item `json:"items"`
	NextCursor        string `json:"next_cursor"`
}
type Filter struct {
	Query, Status, TypeDefinition, Deleted, Parent, Owner, Cursor, Kind, ID string
	Limit                                                                   int
}
type Change struct {
	ActorScope               string    `json:"actor_scope"`
	ActorProjectID           string    `json:"actor_project_id,omitempty"`
	ActorTenantMembershipID  string    `json:"actor_tenant_membership_id,omitempty"`
	ActorProjectMembershipID string    `json:"actor_project_membership_id,omitempty"`
	ActorLinkID              string    `json:"actor_link_id,omitempty"`
	ID                       string    `json:"id"`
	TenantID                 string    `json:"tenant_id"`
	FacilityID               string    `json:"facility_id"`
	ResourceKind             Kind      `json:"resource_kind"`
	ResourceID               string    `json:"resource_id"`
	ResourceRevision         int       `json:"resource_revision"`
	Operation                string    `json:"operation"`
	ActorID                  string    `json:"actor_id,omitempty"`
	ActorName                string    `json:"actor_name"`
	RequestID                string    `json:"request_id"`
	ChangedAt                time.Time `json:"changed_at"`
	Reason                   string    `json:"reason"`
	Before                   *Resource `json:"before"`
	After                    Resource  `json:"after"`
	ChangedFields            []string  `json:"changed_fields"`
	DefinitionVersions       []string  `json:"definition_versions"`
}
type Changes struct {
	Items       []Change              `json:"items"`
	NextCursor  string                `json:"next_cursor"`
	Definitions []engineering.Version `json:"definitions"`
}
type Error struct {
	Code         string              `json:"code"`
	Message      string              `json:"message"`
	Status       int                 `json:"-"`
	Issues       []engineering.Issue `json:"issues"`
	Dependencies map[string]int      `json:"dependencies,omitempty"`
}

func (e *Error) Error() string { return e.Message }
func problem(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status, Issues: []engineering.Issue{}}
}
func invalid(path, code, message string) *Error {
	e := problem("resource_invalid", "请修正标记的问题后重试。", 422)
	e.Issues = append(e.Issues, engineering.Issue{Code: code, Path: path, Message: message, Severity: "error"})
	return e
}
func jsonBytes(v any) []byte { b, _ := json.Marshal(v); return b }
