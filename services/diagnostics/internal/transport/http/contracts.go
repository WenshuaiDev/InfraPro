package httptransport

import (
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/state"
	"time"
)

// These response DTOs are also the source for the standalone OpenAPI/TS contract.
// Runtime response shape is verified against the emitted schemas in HTTP tests.
type Meta struct {
	RequestID        string            `json:"request_id"`
	ServerTime       time.Time         `json:"server_time"`
	ServerBootID     string            `json:"server_boot_id"`
	ManifestRevision string            `json:"manifest_revision,omitempty"`
	AuthRevision     string            `json:"auth_revision,omitempty"`
	SnapshotID       string            `json:"snapshot_id,omitempty"`
	DiagnosticsState map[string]string `json:"diagnostics_state,omitempty"`
}
type Bootstrap struct {
	Initialized  bool   `json:"initialized"`
	LoginAllowed bool   `json:"login_allowed"`
	CSRFToken    string `json:"csrf_token"`
}
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type Activity struct {
	IdleExpiresAt time.Time `json:"idle_expires_at"`
}
type ComponentDetail struct {
	Component           state.ComponentView    `json:"component"`
	ReplicaSummaries    []state.ReplicaSummary `json:"replica_summaries"`
	DependencySummaries []state.DependencyView `json:"dependency_summaries"`
	Coverage            state.Coverage         `json:"coverage"`
}
type NodeDetail struct {
	Node                  state.NodeView         `json:"node"`
	Observer              state.ObserverView     `json:"observer"`
	ResourceSeriesSummary []model.MetricPoint    `json:"resource_series_summary"`
	ReplicaCounts         map[string]int         `json:"replica_counts"`
	ReplicaSummaries      []state.ReplicaSummary `json:"replica_summaries"`
	Coverage              state.Coverage         `json:"coverage"`
}
type Overview struct {
	Instance struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"instance"`
	Summary              map[string]int        `json:"summary"`
	Coverage             state.Coverage        `json:"coverage"`
	ObserverSummary      map[string]int        `json:"observer_summary"`
	ResourceWarnings     []resourceWarning     `json:"resource_warnings"`
	RecentTransitions    []model.Transition    `json:"recent_transitions"`
	AbnormalComponents   []state.ComponentView `json:"abnormal_components"`
	MinRefreshIntervalMS int                   `json:"min_refresh_interval_ms"`
}
type Retention struct {
	TransitionHours int        `json:"transition_hours"`
	TransitionLimit int        `json:"transition_limit"`
	MetricHours     int        `json:"metric_hours"`
	PointsPerSeries int        `json:"points_per_series"`
	MetricLimit     int        `json:"metric_limit"`
	AccessDays      int        `json:"access_days"`
	AccessLimit     int        `json:"access_limit"`
	ActualFrom      *time.Time `json:"actual_from"`
	EvictedCount    string     `json:"evicted_count"`
}
type MetricSeries struct {
	Unit             string              `json:"unit"`
	Scope            string              `json:"scope"`
	Source           string              `json:"source"`
	Points           []model.MetricPoint `json:"points"`
	MissingIntervals []time.Time         `json:"missing_intervals"`
	Retention        Retention           `json:"retention"`
}
