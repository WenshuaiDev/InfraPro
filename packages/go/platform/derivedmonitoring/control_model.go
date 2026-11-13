package derivedmonitoring

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"time"
)

type ControlProposal struct {
	Action         string  `json:"action"`
	AutoStartNS    *string `json:"auto_start_ns,omitempty"`
	StopNS         *string `json:"stop_ns,omitempty"`
	ResumeMode     string  `json:"resume_mode,omitempty"`
	TrialReceiptID string  `json:"trial_receipt_id,omitempty"`
	AllowWaiting   bool    `json:"allow_waiting_for_input"`
}
type ControlPreviewInput struct {
	ControlProposal
	Revision string `json:"expected_revision"`
}
type ControlInput struct {
	WriteMeta
	PreviewID   string `json:"preview_id"`
	PreviewHash string `json:"preview_hash"`
}
type ControlPreview struct {
	ID              string               `json:"id"`
	Hash            string               `json:"hash"`
	Proposal        ControlProposal      `json:"proposal"`
	ChannelID       string               `json:"channel_id"`
	ConfigID        string               `json:"config_id"`
	ConfigHash      string               `json:"config_hash"`
	ControlRevision string               `json:"control_revision"`
	ConfigRevision  string               `json:"config_revision"`
	GraphRevision   string               `json:"graph_revision"`
	OutputRanges    []NanosecondRange    `json:"output_ranges"`
	InputRanges     []monitoring.RawNeed `json:"input_ranges"`
	WaitingForInput bool                 `json:"waiting_for_input"`
	EffectiveAtNS   string               `json:"effective_at_ns"`
	ExpiresAt       time.Time            `json:"expires_at"`
}
