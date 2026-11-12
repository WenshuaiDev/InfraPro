// Package sources owns immediate, atomic, tenant-unified direct source relations.
package sources

import (
	"encoding/json"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"time"
)

type Scope = k.Scope
type Meta = k.Meta
type Result = k.Result
type Preview = k.Preview
type Error = k.Error
type Issue = k.Issue
type Filter = k.Filter
type Target struct {
	FacilityID string `json:"facility_id"`
	PointID    string `json:"point_id"`
	ChannelID  string `json:"channel_id"`
}
type Revisions struct {
	Meta
	SourceRevision         string `json:"expected_source_revision"`
	ChannelRevision        string `json:"expected_channel_revision"`
	ResourceAccessRevision string `json:"expected_resource_access_revision,omitempty"`
	LinkRevision           string `json:"expected_link_revision,omitempty"`
	DeviceAccessRevision   string `json:"expected_device_access_revision,omitempty"`
}
type NewEnd struct {
	OutputID          string `json:"output_id"`
	NewOutputRevision string `json:"expected_new_output_revision"`
	NewDeviceRevision string `json:"expected_new_device_revision"`
}
type OldEnd struct {
	BindingID         string `json:"expected_binding_id"`
	BindingRevision   string `json:"expected_binding_revision"`
	OldOutputRevision string `json:"expected_old_output_revision"`
	OldDeviceRevision string `json:"expected_old_device_revision"`
}
type BindInput struct {
	Revisions
	NewEnd
}
type SwitchInput struct {
	Revisions
	NewEnd
	OldEnd
}
type UnbindInput struct {
	Revisions
	OldEnd
}

// Command is internal to decoded discriminated commands; handlers never decode
// this union directly and therefore reject fields that belong to another action.
type Command struct {
	Action string `json:"action"`
	Revisions
	NewEnd
	OldEnd
}

func Bind(in BindInput) Command {
	return Command{Action: "bind", Revisions: in.Revisions, NewEnd: in.NewEnd}
}
func Switch(in SwitchInput) Command {
	return Command{Action: "switch", Revisions: in.Revisions, NewEnd: in.NewEnd, OldEnd: in.OldEnd}
}
func Unbind(in UnbindInput) Command {
	return Command{Action: "unbind", Revisions: in.Revisions, OldEnd: in.OldEnd}
}

type Binding struct {
	ID                string          `json:"id"`
	TenantID          string          `json:"tenant_id"`
	DeviceID          string          `json:"device_id"`
	OutputID          string          `json:"output_id"`
	FacilityID        string          `json:"facility_id"`
	PointID           string          `json:"point_id"`
	ChannelID         string          `json:"channel_id"`
	StartedAt         time.Time       `json:"started_at"`
	EndedAt           *time.Time      `json:"ended_at"`
	Revision          string          `json:"revision"`
	SemanticHash      string          `json:"semantic_hash"`
	OutputSnapshot    json.RawMessage `json:"output_snapshot"`
	ChannelSnapshot   json.RawMessage `json:"channel_snapshot"`
	CreatedActorScope string          `json:"created_actor_scope"`
	CreatedProjectID  *string         `json:"created_project_id"`
	CreatedBy         string          `json:"created_by"`
	EndedBy           *string         `json:"ended_by"`
	CreatedReason     string          `json:"created_reason"`
	EndedReason       *string         `json:"ended_reason"`
}

// Projection has no hidden IDs in its restricted form, including snapshots.
type Projection struct {
	Configured      bool            `json:"configured"`
	Visibility      string          `json:"visibility"`
	ID              string          `json:"id,omitempty"`
	Revision        string          `json:"revision,omitempty"`
	Output          *devices.Output `json:"output,omitempty"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	EndedAt         *time.Time      `json:"ended_at,omitempty"`
	OutputSnapshot  json.RawMessage `json:"output_snapshot,omitempty"`
	ChannelSnapshot json.RawMessage `json:"channel_snapshot,omitempty"`
	Reason          string          `json:"reason,omitempty"`
}
type Actions struct {
	Bind   bool `json:"bind"`
	Switch bool `json:"switch"`
	Unbind bool `json:"unbind"`
}
type Current struct {
	SourceRevision         string      `json:"source_revision"`
	ChannelRevision        string      `json:"channel_revision"`
	ResourceAccessRevision string      `json:"resource_access_revision,omitempty"`
	DeviceAccessRevision   string      `json:"device_access_revision,omitempty"`
	LinkRevision           string      `json:"link_revision,omitempty"`
	TargetRetired          bool        `json:"target_retired"`
	Semantics              c.Semantics `json:"semantics"`
	Source                 Projection  `json:"source"`
	Actions                Actions     `json:"actions"`
}
type Candidate struct {
	Output     devices.Output `json:"output"`
	Compatible bool           `json:"compatible"`
	Available  bool           `json:"available"`
	Issues     []Issue        `json:"issues"`
}
type ChannelCandidate struct {
	Configured bool `json:"configured"`
	Target
	Code       string      `json:"code"`
	Name       string      `json:"name"`
	Revision   string      `json:"revision"`
	Semantics  c.Semantics `json:"semantics"`
	Compatible bool        `json:"compatible"`
	Available  bool        `json:"available"`
	Issues     []Issue     `json:"issues"`
	CreatedAt  time.Time   `json:"created_at"`
}
