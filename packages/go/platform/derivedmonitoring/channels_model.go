package derivedmonitoring

import (
	"encoding/json"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

type Owner struct {
	Kind       resources.Kind `json:"kind"`
	FacilityID string         `json:"facility_id"`
	ID         string         `json:"id"`
}
type InputBinding struct {
	ChannelID string `json:"channel_id"`
}
type BaselineBinding struct {
	Input         string `json:"input"`
	ObservationID string `json:"observation_id,omitempty"`
	AdoptionID    string `json:"adoption_id,omitempty"`
	ResultID      string `json:"result_id,omitempty"`
	GenerationID  string `json:"generation_id,omitempty"`
	AtNS          string `json:"at_ns,omitempty"`
}
type CreateChannelInput struct {
	WriteMeta
	Code                string                     `json:"code"`
	Name                string                     `json:"name"`
	Description         string                     `json:"description"`
	Owner               Owner                      `json:"owner"`
	DefinitionVersionID string                     `json:"definition_version_id"`
	InputBindings       map[string]InputBinding    `json:"input_bindings"`
	BaselineBindings    map[string]BaselineBinding `json:"baseline_bindings,omitempty"`
	Parameters          map[string]string          `json:"parameters"`
	Quality             []string                   `json:"quality,omitempty"`
	NumericContract     *NumericContract           `json:"numeric_contract,omitempty"`
}
type Channel struct {
	MetricName      string             `json:"metric_name"`
	CurrentConfigID *string            `json:"current_config_id"`
	OwnerName       string             `json:"owner_name"`
	OwnerCode       string             `json:"owner_code"`
	FacilityName    string             `json:"facility_name"`
	Resource        resources.Resource `json:"resource"`
	ControlState    string             `json:"control_state"`
	ControlRevision string             `json:"control_revision"`
	ConfigRevision  string             `json:"config_revision"`
	MandateRevision string             `json:"mandate_revision"`
	DraftConfigID   string             `json:"draft_config_id"`
	AutoStartNS     *string            `json:"auto_start_ns"`
	StopNS          *string            `json:"stop_ns"`
	Health          string             `json:"health"`
	BlockedReason   string             `json:"blocked_reason"`
}
type ChannelResult struct {
	Channel  Channel `json:"channel"`
	Replayed bool    `json:"replayed"`
}
type ChannelPage struct {
	Items      []Channel `json:"items"`
	NextCursor string    `json:"next_cursor"`
}
type ChannelFilter struct {
	Query, Cursor, ControlState, FacilityID, OwnerKind, OwnerID string
	Limit                                                       int
}
type ChannelConfig struct {
	BaseConfigID            string                     `json:"base_config_id,omitempty"`
	ChangeKind              string                     `json:"change_kind"`
	SemanticIntentConfirmed bool                       `json:"semantic_intent_confirmed"`
	ResetBaselines          bool                       `json:"reset_baselines"`
	ID                      string                     `json:"id"`
	ChannelID               string                     `json:"channel_id"`
	DefinitionVersionID     string                     `json:"definition_version_id"`
	InputBindings           map[string]InputBinding    `json:"input_bindings,omitempty"`
	BaselineBindings        map[string]BaselineBinding `json:"baseline_bindings,omitempty"`
	Parameters              map[string]string          `json:"parameters,omitempty"`
	Rule                    json.RawMessage            `json:"rule,omitempty"`
	SemanticHash            string                     `json:"semantic_hash"`
	ConfigHash              string                     `json:"config_hash"`
	OutputContractHash      string                     `json:"output_contract_hash"`
	GraphRevision           string                     `json:"graph_revision"`
	CreatedAt               time.Time                  `json:"created_at"`
	InputsRestricted        bool                       `json:"inputs_restricted"`
}
type ConfigPage struct {
	Items      []ChannelConfig `json:"items"`
	NextCursor string          `json:"next_cursor"`
}
type ChannelDependency struct {
	ConfigID        string          `json:"config_id"`
	Slot            string          `json:"slot"`
	ChannelID       string          `json:"channel_id"`
	SourceKind      string          `json:"source_kind"`
	SourceSemantics json.RawMessage `json:"source_semantics"`
}
type DependencyPage struct {
	Items      []ChannelDependency `json:"items"`
	Restricted bool                `json:"restricted"`
	NextCursor string              `json:"next_cursor"`
}
