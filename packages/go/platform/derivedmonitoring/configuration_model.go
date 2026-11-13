package derivedmonitoring

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"time"
)

type SaveConfigInput struct {
	WriteMeta
	BaseConfigID            string                     `json:"base_config_id"`
	DefinitionVersionID     string                     `json:"definition_version_id"`
	InputBindings           map[string]InputBinding    `json:"input_bindings"`
	BaselineBindings        map[string]BaselineBinding `json:"baseline_bindings,omitempty"`
	Parameters              map[string]string          `json:"parameters"`
	Quality                 []string                   `json:"quality,omitempty"`
	NumericContract         *NumericContract           `json:"numeric_contract,omitempty"`
	ChangeKind              string                     `json:"change_kind"`
	SemanticIntentConfirmed bool                       `json:"semantic_intent_confirmed"`
	ResetBaselines          bool                       `json:"reset_baselines"`
}
type ConfigResult struct {
	Config   ChannelConfig `json:"config"`
	Channel  Channel       `json:"channel"`
	Replayed bool          `json:"replayed"`
}
type ConfigAssignment struct {
	ID                 string          `json:"id"`
	ConfigID           string          `json:"config_id"`
	Range              NanosecondRange `json:"effective_range"`
	CreatedRevision    string          `json:"created_revision"`
	SupersededRevision *string         `json:"superseded_revision"`
}
type AssignmentPage struct {
	Items      []ConfigAssignment `json:"items"`
	Revision   string             `json:"revision"`
	NextCursor string             `json:"next_cursor"`
}
type ConfigProposal struct {
	ConfigID       string          `json:"config_id"`
	Mode           string          `json:"mode"`
	Range          NanosecondRange `json:"output_range"`
	TrialReceiptID string          `json:"trial_receipt_id"`
	AllowWaiting   bool            `json:"allow_waiting_for_input"`
}
type ConfigPreviewInput struct {
	ConfigProposal
	Revision string `json:"expected_revision"`
}
type ConfigPreview struct {
	ID              string               `json:"id"`
	Hash            string               `json:"hash"`
	Proposal        ConfigProposal       `json:"proposal"`
	ChannelID       string               `json:"channel_id"`
	ConfigHash      string               `json:"config_hash"`
	ConfigRevision  string               `json:"config_revision"`
	ControlRevision string               `json:"control_revision"`
	GraphRevision   string               `json:"graph_revision"`
	Assignments     []ConfigAssignment   `json:"replaced_assignments"`
	InputRanges     []monitoring.RawNeed `json:"input_ranges"`
	OutputRanges    []NanosecondRange    `json:"output_ranges"`
	DownstreamCount string               `json:"downstream_count"`
	WaitingForInput bool                 `json:"waiting_for_input"`
	EffectiveAtNS   string               `json:"effective_at_ns"`
	ExpiresAt       time.Time            `json:"expires_at"`
}
