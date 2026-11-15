// Package derivedmonitoring owns tenant derived definitions, calculation
// channels and scoped analysis. Computation and persistence remain internal.
package derivedmonitoring

import (
	"encoding/json"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
)

type Scope struct{ Tenant, Project, Entry string }

func TenantScope(tenant string) Scope { return Scope{Tenant: tenant, Entry: "tenant"} }
func ProjectScope(tenant, project string) Scope {
	return Scope{Tenant: tenant, Project: project, Entry: "project"}
}
func (s Scope) valid() bool {
	return access.ValidID(s.Tenant) && (s.Entry == "tenant" && s.Project == "" || s.Entry == "project" && access.ValidID(s.Project))
}
func (s Scope) key() string {
	if s.Project != "" {
		return s.Project
	}
	return "tenant"
}

type Issue = compute.Issue
type InputSpec = compute.Input
type Rule = compute.Rule
type Quantity = compute.Quantity
type Node = compute.Node
type NodeParams = compute.Params
type Clock = compute.Clock
type Window = compute.Window
type Baseline = compute.Baseline
type Output = compute.Output
type NumericContract = compute.NumericContract
type TimeSemantics = compute.TimeSemantics

type Error struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Issues  []Issue `json:"issues"`
	Status  int     `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func problem(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status, Issues: []Issue{}}
}
func invalid(path, message string) *Error {
	e := problem("invalid_rule", "请修正标记的问题后重试。", 422)
	e.Issues = []Issue{{Code: "invalid_rule", Path: path, Message: message}}
	return e
}
func notFound() *Error { return problem("not_found", "对象不存在或当前不可访问。", 404) }
func conflict() *Error {
	return problem("revision_conflict", "对象已变化，请重新读取后提交。", 409)
}

type WriteMeta struct {
	RequestID string `json:"request_id"`
	Revision  string `json:"expected_revision,omitempty"`
	Reason    string `json:"reason,omitempty"`
}
type CreateMetricInput struct {
	WriteMeta
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Rule        json.RawMessage `json:"rule"`
}
type SaveMetricInput struct {
	WriteMeta
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	BaseVersionID string          `json:"base_version_id,omitempty"`
	Rule          json.RawMessage `json:"rule"`
}
type CopyMetricInput struct {
	WriteMeta
	Code            string `json:"code"`
	Name            string `json:"name"`
	SourceVersionID string `json:"source_version_id"`
}
type ValidateInput struct {
	DraftRevision string `json:"draft_revision"`
}
type PublishMetricInput struct {
	WriteMeta
	DraftRevision  string `json:"draft_revision"`
	ValidationHash string `json:"validation_hash"`
}
type VersionStateInput struct {
	WriteMeta
	Enabled bool `json:"enabled"`
}

type MetricDraft struct {
	Revision      string          `json:"revision"`
	BaseVersionID string          `json:"base_version_id,omitempty"`
	Rule          json.RawMessage `json:"rule"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
type MetricDefinition struct {
	ID            string       `json:"id"`
	TenantID      string       `json:"tenant_id"`
	Code          string       `json:"code"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Status        string       `json:"status"`
	Revision      string       `json:"revision"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	LatestVersion string       `json:"latest_version"`
	Draft         *MetricDraft `json:"draft,omitempty"`
}
type MetricVersion struct {
	ID                 string          `json:"id"`
	TenantID           string          `json:"tenant_id"`
	DefinitionID       string          `json:"definition_id"`
	Number             string          `json:"version_no"`
	Name               string          `json:"name"`
	Rule               json.RawMessage `json:"rule"`
	SchemaVersion      string          `json:"schema_version"`
	CompilerVersion    string          `json:"compiler_version"`
	EngineVersion      string          `json:"engine_version"`
	ContentHash        string          `json:"content_hash"`
	OutputContractHash string          `json:"output_contract_hash"`
	Enabled            bool            `json:"enabled"`
	Usable             bool            `json:"usable"`
	StateRevision      string          `json:"state_revision"`
	PublishedAt        time.Time       `json:"published_at"`
}
type MetricResult struct {
	Definition MetricDefinition `json:"definition"`
	Version    *MetricVersion   `json:"version,omitempty"`
	Replayed   bool             `json:"replayed"`
}
type MetricPage struct {
	Items      []MetricDefinition `json:"items"`
	NextCursor string             `json:"next_cursor"`
}
type VersionPage struct {
	Items      []MetricVersion `json:"items"`
	NextCursor string          `json:"next_cursor"`
}
type MetricFilter struct {
	Query, Cursor, State string
	Limit                int
}
type RuleValidation struct {
	Valid              bool    `json:"valid"`
	Issues             []Issue `json:"issues"`
	ValidationHash     string  `json:"validation_hash"`
	CompiledPlanHash   string  `json:"compiled_plan_hash"`
	OutputContractHash string  `json:"output_contract_hash"`
	NormalizedRule     Rule    `json:"normalized_rule"`
	InputCount         string  `json:"input_count"`
	NodeCount          string  `json:"node_count"`
}
type Template struct {
	Key         string `json:"key"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description"`
	InputCount  string `json:"input_count"`
}
type TemplatePage struct {
	Items      []Template `json:"items"`
	NextCursor string     `json:"next_cursor"`
}
type ExpandTemplateInput struct {
	Inputs []InputSpec `json:"inputs"`
}

// StatusCode allows cross-module metadata adapters to distinguish hidden data from outages.
func (e *Error) StatusCode() int { return e.Status }
