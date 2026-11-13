package derivedmonitoring

import (
	"encoding/json"
	"time"
)

// Analysis owns a workspace configuration, never an engineering channel.
type AnalysisBody struct {
	Rule             json.RawMessage            `json:"rule"`
	InputBindings    map[string]InputBinding    `json:"input_bindings"`
	BaselineBindings map[string]BaselineBinding `json:"baseline_bindings,omitempty"`
	Parameters       map[string]string          `json:"parameters"`
}
type AnalysisDraft struct {
	Revision string       `json:"revision"`
	Body     AnalysisBody `json:"body"`
}
type AnalysisConfig struct {
	ID               string         `json:"id"`
	TenantID         string         `json:"tenant_id"`
	ProjectID        *string        `json:"project_id"`
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	Status           string         `json:"status"`
	Revision         string         `json:"revision"`
	LatestVersion    string         `json:"latest_version"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	InputsRestricted bool           `json:"inputs_restricted"`
	Draft            *AnalysisDraft `json:"draft,omitempty"`
}
type AnalysisVersion struct {
	ID              string       `json:"id"`
	ConfigID        string       `json:"config_id"`
	Number          string       `json:"version_no"`
	Name            string       `json:"name"`
	Body            AnalysisBody `json:"body"`
	ConfigHash      string       `json:"config_hash"`
	PlanHash        string       `json:"compiled_plan_hash"`
	EngineVersion   string       `json:"engine_version"`
	CompilerVersion string       `json:"compiler_version"`
	BaseVersionID   string       `json:"base_version_id,omitempty"`
	ResetBaselines  bool         `json:"reset_baselines"`
	CreatedAt       time.Time    `json:"created_at"`
}
type AnalysisConfigPage struct {
	Items      []AnalysisConfig `json:"items"`
	NextCursor string           `json:"next_cursor"`
}
type AnalysisVersionPage struct {
	Items      []AnalysisVersion `json:"items"`
	NextCursor string            `json:"next_cursor"`
}
type CreateAnalysisInput struct {
	WriteMeta
	Code        string       `json:"code"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Body        AnalysisBody `json:"body"`
}
type SaveAnalysisInput struct {
	WriteMeta
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Body        AnalysisBody `json:"body"`
}
type CopyAnalysisInput struct {
	WriteMeta
	Code            string `json:"code"`
	Name            string `json:"name"`
	SourceVersionID string `json:"source_version_id"`
}
type AnalysisValidation struct {
	DraftRevision      string   `json:"draft_revision"`
	ValidationHash     string   `json:"validation_hash"`
	ConfigHash         string   `json:"config_hash"`
	PlanHash           string   `json:"compiled_plan_hash"`
	Outputs            []Output `json:"outputs"`
	IdentityParameters []string `json:"identity_parameters"`
	Issues             []Issue  `json:"issues"`
}
type PublishAnalysisInput struct {
	PublishMetricInput
	BaseVersionID  string `json:"base_version_id,omitempty"`
	ResetBaselines bool   `json:"reset_baselines"`
}
type AnalysisConfigResult struct {
	Config   AnalysisConfig   `json:"config"`
	Version  *AnalysisVersion `json:"version,omitempty"`
	Replayed bool             `json:"replayed"`
}
