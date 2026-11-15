package model

import "time"

type Transition struct {
	ID             string    `json:"id"`
	TargetKind     string    `json:"target_kind"`
	TargetID       string    `json:"target_id"`
	ProbeID        string    `json:"probe_id,omitempty"`
	At             time.Time `json:"at"`
	FromState      string    `json:"from_state"`
	ToState        string    `json:"to_state"`
	ReasonCode     string    `json:"reason_code"`
	ObservationRef *string   `json:"observation_ref"`
	Retired        bool      `json:"retired"`
}
type MetricPoint struct {
	TargetKind string    `json:"target_kind"`
	TargetID   string    `json:"target_id"`
	NodeID     string    `json:"node_id"`
	Source     string    `json:"source"`
	Scope      string    `json:"scope"`
	At         time.Time `json:"at"`
	Metric
}
type AccessRecord struct {
	ID        string    `json:"id"`
	At        time.Time `json:"at"`
	SubjectID string    `json:"subject_id"`
	Code      string    `json:"code"`
	RequestID string    `json:"request_id"`
}
