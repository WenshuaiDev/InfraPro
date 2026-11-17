// Package displayread coordinates authorized, snapshot-bound display reads.
// Business facts and their visibility rules remain in the owning domains.
package displayread

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

const (
	MaxMembers       = 50000
	MaxIDs           = 500
	PageSize         = 50
	MaxPageSize      = 200
	MaxMapPageSize   = 500
	MaxDetails       = 1000
	MaxTrendChannels = 50
	MaxResultBytes   = 8 << 20
	MaxFileBytes     = 4 << 20
	QueryLifetime    = 15 * time.Minute
	SyncDeadline     = 15 * time.Second
	JobDeadline      = 120 * time.Second
)

type Error struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Status  int     `json:"-"`
	Issues  []Issue `json:"issues,omitempty"`
}
type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string   { return e.Message }
func (e *Error) StatusCode() int { return e.Status }
func Problem(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status}
}
func Invalid(path, message string) *Error {
	return &Error{Code: "query_invalid", Message: "请求未通过校验。", Status: 422, Issues: []Issue{{path, "invalid", message}}}
}
func Hidden() *Error     { return Problem("not_found", "对象不存在或不可访问。", 404) }
func Bytes(v any) []byte { b, _ := json.Marshal(v); return b }
func Hash(v any) string  { sum := sha256.Sum256(Bytes(v)); return hex.EncodeToString(sum[:]) }

var NamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,79}$`)

type Ref struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type MetricRef struct {
	Kind      string `json:"kind"`
	VersionID string `json:"version_id"`
}
type Window struct {
	From *string `json:"from,omitempty"`
	To   *string `json:"to,omitempty"`
}
type ViewSchema struct {
	StructureModel int    `json:"structure_model,omitempty"`
	Domain         string `json:"domain"`
	Required       bool   `json:"required"`
	Purpose        string `json:"purpose"`
	AllowDynamic   bool   `json:"allow_dynamic"`
}
type Selector struct {
	StructureModel  int         `json:"-"`
	Kind            string      `json:"kind"`
	IDs             []string    `json:"ids,omitempty"`
	FacilityIDs     []string    `json:"facility_ids,omitempty"`
	TypeIDs         []string    `json:"type_definition_ids,omitempty"`
	Kinds           []string    `json:"kinds,omitempty"`
	Statuses        []string    `json:"statuses,omitempty"`
	SourceKinds     []string    `json:"source_kinds,omitempty"`
	Metrics         []MetricRef `json:"metric_refs,omitempty"`
	Owners          []Ref       `json:"owner_refs,omitempty"`
	ManufacturerIDs []string    `json:"manufacturer_ids,omitempty"`
	ProductIDs      []string    `json:"product_ids,omitempty"`
	OutputIDs       []string    `json:"output_ids,omitempty"`
	AssetKinds      []string    `json:"asset_kinds,omitempty"`
}
type Definition struct {
	StructureModel int      `json:"structure_model,omitempty"`
	ID             string   `json:"id,omitempty"`
	ScopeID        string   `json:"scope_id,omitempty"`
	Domain         string   `json:"domain"`
	Selector       Selector `json:"selector"`
	IncludeFuture  bool     `json:"include_future"`
	FieldGroups    []string `json:"field_groups"`
	TimeWindow     *Window  `json:"time_window,omitempty"`
	// SetID and Digest are server-generated, immutable membership evidence.
	SetID  string `json:"set_id,omitempty"`
	Digest string `json:"digest,omitempty"`
}
type Target struct {
	View      string   `json:"view,omitempty"`
	IDs       []string `json:"ids,omitempty"`
	Selection string   `json:"selection,omitempty"`
}
type Member struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}
type Scope struct {
	StructureModel int                            `json:"structure_model,omitempty"`
	Domain         string                         `json:"domain"`
	Members        []Member                       `json:"members"`
	FieldGroups    []string                       `json:"field_groups"`
	Windows        map[string][]access.ReadWindow `json:"windows,omitempty"`
	Hash           string                         `json:"hash"`
}

func (s Scope) IDs(kind string) []string {
	ids := []string{}
	for _, m := range s.Members {
		if kind == "" || m.Kind == kind {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

type Completeness struct {
	Scope   string   `json:"scope"`
	Data    string   `json:"data"`
	Reasons []string `json:"reasons"`
}
type QueryInfo struct {
	ID           string       `json:"id"`
	Method       string       `json:"method"`
	ScopeHash    string       `json:"scope_hash"`
	FilterHash   string       `json:"filter_hash"`
	SnapshotID   string       `json:"read_snapshot_id"`
	MetadataAsOf time.Time    `json:"metadata_as_of"`
	CreatedAt    time.Time    `json:"created_at"`
	ExpiresAt    time.Time    `json:"expires_at"`
	Completeness Completeness `json:"completeness"`
}
type Result struct {
	State          string            `json:"state"`
	Data           any               `json:"data"`
	ReadAt         string            `json:"read_at"`
	Authorization  string            `json:"authorization_revision"`
	Generation     string            `json:"generation"`
	SourceVector   map[string]string `json:"source_vector"`
	Query          *QueryInfo        `json:"query"`
	IncludedFields []string          `json:"included_field_groups"`
	RetryAfter     int               `json:"retry_after_ms,omitempty"`
}
type Page[T any] struct {
	Section    string `json:"section,omitempty"`
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
	QueryID    string `json:"query_id"`
	HasMore    bool   `json:"has_more"`
	Total      string `json:"total"`
}

// Resolver is installed by runtime.Compose. Each implementation uses its owning
// domain's current project visibility and operation authorization in this tx.
type Resolver interface {
	Members(context.Context, pgx.Tx, access.Actor, string, string, Selector) ([]Member, error)
}
type ResolverFunc func(context.Context, pgx.Tx, access.Actor, string, string, Selector) ([]Member, error)

func (f ResolverFunc) Members(c context.Context, t pgx.Tx, a access.Actor, tenant, project string, s Selector) ([]Member, error) {
	return f(c, t, a, tenant, project, s)
}

// ScopeQualifier keeps recursive and field eligibility in the owning domain.
type ScopeQualifier func(context.Context, pgx.Tx, access.Actor, string, string, Definition, []Member) error
