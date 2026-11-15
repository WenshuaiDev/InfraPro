package model

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
)

type Facets struct {
	Presence  string `json:"presence"`
	Readiness string `json:"readiness"`
	Loop      string `json:"loop"`
}

func UnknownFacets() Facets { return Facets{"unknown", "unknown", "unknown"} }
func NeutralFacets() Facets { return Facets{"not_applicable", "not_applicable", "not_applicable"} }

type ProcessIdentity struct {
	ReplicaID          string     `json:"replica_id"`
	BootID             string     `json:"boot_id"`
	PID                *int32     `json:"pid,omitempty"`
	OSStartTimeMS      string     `json:"os_start_time_ms,omitempty"`
	PIDNamespace       string     `json:"pid_namespace,omitempty"`
	ContainerID        string     `json:"container_id,omitempty"`
	ContainerStartedAt *time.Time `json:"container_started_at,omitempty"`
	BuildID            string     `json:"build_id,omitempty"`
	UID                *int       `json:"uid,omitempty"`
}
type Evidence struct {
	Source        string                  `json:"source"`
	VantageNodeID string                  `json:"vantage_node_id"`
	Scope         string                  `json:"scope"`
	Codes         []string                `json:"codes"`
	ExitCode      *int                    `json:"exit_code,omitempty"`
	OOMKilled     *bool                   `json:"oom_killed,omitempty"`
	Health        string                  `json:"health,omitempty"`
	RestartCount  *int                    `json:"restart_count,omitempty"`
	HTTPStatus    *int                    `json:"http_status,omitempty"`
	SystemdState  string                  `json:"systemd_state,omitempty"`
	Lanes         []diagnosticreport.Lane `json:"lanes,omitempty"`
}
type Metric struct {
	Key          string `json:"key"`
	DimensionID  string `json:"dimension_id,omitempty"`
	Unit         string `json:"unit"`
	Value        any    `json:"value"`
	Availability string `json:"availability"`
	ReasonCode   string `json:"reason_code,omitempty"`
}
type Observation struct {
	ProbeID        string           `json:"probe_id"`
	AttemptID      *string          `json:"attempt_id"`
	ObserverBootID string           `json:"observer_boot_id"`
	ObservedAt     *time.Time       `json:"observed_at"`
	SampleAgeMS    *int64           `json:"sample_age_ms"`
	DurationMS     *int64           `json:"duration_ms"`
	Outcome        string           `json:"outcome"`
	Facets         Facets           `json:"facets"`
	ReasonCode     string           `json:"reason_code"`
	Process        *ProcessIdentity `json:"process,omitempty"`
	Metrics        []Metric         `json:"metrics"`
	Evidence       Evidence         `json:"evidence"`
}
type StoredObservation struct {
	Observation
	InstanceID            string    `json:"instance_id"`
	NodeID                string    `json:"node_id"`
	ManifestRevision      string    `json:"manifest_revision"`
	AssignmentRevision    string    `json:"assignment_revision"`
	CredentialGeneration  string    `json:"credential_generation"`
	Epoch                 string    `json:"epoch"`
	Sequence              string    `json:"sequence"`
	ReceivedAt            time.Time `json:"received_at"`
	AcceptedServerBootID  string    `json:"accepted_server_boot_id"`
	FirstAcceptedAt       time.Time `json:"first_accepted_at"`
	FirstServerBootID     string    `json:"first_server_boot_id"`
	EffectiveAgeAtReceipt int64     `json:"effective_age_at_receipt"`
	ClockUncertain        bool      `json:"clock_uncertain"`
	// monotonic receipt is deliberately not persisted.
	ReceiptMonotonic time.Time  `json:"-"`
	Persistence      string     `json:"persistence"`
	LastSuccessAt    *time.Time `json:"last_success_at,omitempty"`
}
type ObservationView struct {
	Observation
	ReceivedAt     *time.Time `json:"received_at"`
	EffectiveAgeMS *int64     `json:"effective_age_ms"`
	Freshness      string     `json:"freshness"`
	Persistence    string     `json:"persistence"`
}

var MetricUnits = map[string]string{
	"cpu.utilization_ratio": "ratio", "process.cpu_cores": "cores", "memory.total_bytes": "bytes", "memory.available_bytes": "bytes", "memory.used_bytes": "bytes", "process.rss_bytes": "bytes", "container.memory_used_bytes": "bytes", "container.memory_limit_bytes": "bytes", "container.cpu_cores": "cores", "container.cpu_limit_cores": "cores", "disk.total_bytes": "bytes", "disk.available_bytes": "bytes", "disk.used_ratio": "ratio", "network.rx_bytes_per_second": "bytes_per_second", "network.tx_bytes_per_second": "bytes_per_second", "process.restart_count": "count", "uptime_ms": "milliseconds",
}

func Pending(p settings.Probe, boot string) Observation {
	return Observation{ProbeID: p.ID, ObserverBootID: boot, Outcome: "pending", Facets: UnknownFacets(), ReasonCode: "check_pending", Metrics: []Metric{}, Evidence: Evidence{Source: Source(p.Adapter), VantageNodeID: p.NodeID, Scope: "none", Codes: []string{"check_pending"}}}
}
func Source(adapter string) string {
	switch adapter {
	case "native_process", "host_resources":
		return "os"
	case "self_report":
		return "self_report"
	case "docker_container":
		return "docker"
	case "systemd_unit":
		return "systemd"
	default:
		return "protocol"
	}
}
func New(p settings.Probe, boot string, now time.Time) Observation {
	o := Pending(p, boot)
	id := uuid.NewString()
	age, duration := int64(0), int64(0)
	t := now.UTC().Truncate(time.Millisecond)
	o.AttemptID = &id
	o.ObservedAt = &t
	o.SampleAgeMS = &age
	o.DurationMS = &duration
	o.Outcome = "ok"
	o.ReasonCode = "ok"
	o.Facets = NeutralFacets()
	o.Evidence.Codes = []string{}
	return o
}
func Failure(o Observation, outcome, reason string) Observation {
	o.Outcome = outcome
	o.ReasonCode = reason
	o.Evidence.Codes = []string{reason}
	return o
}
func UUID(s string) bool { v, e := uuid.Parse(s); return e == nil && v.String() == s && v != uuid.Nil }
func SafeBuild(s string) bool {
	return len(s) <= 64 && regexp.MustCompile(`^[A-Za-z0-9_.+-]*$`).MatchString(s)
}
func Validate(o Observation, p settings.Probe, boot string) error {
	raw, e := json.Marshal(o)
	if e != nil || len(raw) > 16<<10 || o.ProbeID != p.ID || o.ObserverBootID != boot || !UUID(boot) || o.Evidence.VantageNodeID != p.NodeID || o.Evidence.Source != Source(p.Adapter) || !settings.OneOf(o.Evidence.Scope, "host", "vm", "container", "none") || !settings.KnownReason(o.ReasonCode) {
		return settings.Error("invalid_report")
	}
	if !settings.OneOf(o.Outcome, "pending", "ok", "failed", "timeout", "permission_denied", "unsupported", "not_configured", "invalid_report") || !settings.OneOf(o.Facets.Presence, "running", "stopped", "missing", "unknown", "not_applicable") || !settings.OneOf(o.Facets.Readiness, "ready", "not_ready", "starting", "unconfigured", "unknown", "not_applicable") || !settings.OneOf(o.Facets.Loop, "idle", "working", "blocked", "stalled", "unknown", "not_applicable") {
		return settings.Error("invalid_report")
	}
	if o.Outcome == "pending" {
		if o.AttemptID != nil || o.ObservedAt != nil || o.SampleAgeMS != nil || o.DurationMS != nil || o.Process != nil || len(o.Metrics) > 0 || o.Facets != UnknownFacets() {
			return settings.Error("invalid_report")
		}
	} else if o.AttemptID == nil || !UUID(*o.AttemptID) || o.ObservedAt == nil || o.ObservedAt.IsZero() || o.ObservedAt.Nanosecond()%1e6 != 0 || o.SampleAgeMS == nil || *o.SampleAgeMS < 0 || *o.SampleAgeMS > 31536000000 || o.DurationMS == nil || *o.DurationMS < 0 || *o.DurationMS > 300000 {
		return settings.Error("invalid_report")
	}
	if len(o.Metrics) > 64 || len(o.Evidence.Codes) > 16 || len(o.Evidence.Lanes) > 16 {
		return settings.Error("invalid_report")
	}
	for _, code := range o.Evidence.Codes {
		if !settings.KnownReason(code) {
			return settings.Error("invalid_report")
		}
	}
	if o.Evidence.Health != "" && !settings.OneOf(o.Evidence.Health, "none", "starting", "healthy", "unhealthy") {
		return settings.Error("invalid_report")
	}
	if o.Evidence.SystemdState != "" && !settings.OneOf(o.Evidence.SystemdState, "active", "activating", "deactivating", "inactive", "failed", "reloading", "not-found") {
		return settings.Error("invalid_report")
	}
	if o.Evidence.ExitCode != nil && (*o.Evidence.ExitCode < 0 || *o.Evidence.ExitCode > 65535) || o.Evidence.RestartCount != nil && (*o.Evidence.RestartCount < 0 || *o.Evidence.RestartCount > 2147483647) || o.Evidence.HTTPStatus != nil && (*o.Evidence.HTTPStatus < 100 || *o.Evidence.HTTPStatus > 599) {
		return settings.Error("invalid_report")
	}
	seen := map[string]bool{}
	for _, m := range o.Metrics {
		key := m.Key + ":" + m.DimensionID
		if seen[key] || MetricUnits[m.Key] != m.Unit || m.Unit == "" || m.DimensionID != "" && !settings.ValidID(m.DimensionID) || !settings.OneOf(m.Availability, "available", "unlimited", "unsupported", "permission_denied", "not_configured", "error") || m.ReasonCode != "" && !settings.KnownReason(m.ReasonCode) {
			return settings.Error("invalid_report")
		}
		seen[key] = true
		if strings.HasPrefix(m.Key, "disk.") || strings.HasPrefix(m.Key, "network.") {
			if m.DimensionID == "" {
				return settings.Error("invalid_report")
			}
		} else if m.DimensionID != "" {
			return settings.Error("invalid_report")
		}
		if m.Availability != "available" {
			if m.Value != nil {
				return settings.Error("invalid_report")
			}
			continue
		}
		if m.Unit == "bytes" || m.Unit == "milliseconds" {
			s, ok := m.Value.(string)
			if !ok || !settings.Decimal(s, true) {
				return settings.Error("invalid_report")
			}
		} else {
			f, ok := m.Value.(float64)
			if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || m.Unit == "ratio" && f > 1 {
				return settings.Error("invalid_report")
			}
		}
	}
	if o.Process != nil {
		v := o.Process
		if !settings.ValidID(v.ReplicaID) || !UUID(v.BootID) || !SafeBuild(v.BuildID) || v.PID != nil && (*v.PID < 1 || !settings.Decimal(v.OSStartTimeMS, false) || !settings.OneOf(v.PIDNamespace, "host", "container")) || v.ContainerID != "" && !regexp.MustCompile(`^[a-f0-9]{12,64}$`).MatchString(v.ContainerID) || v.UID != nil && *v.UID < 0 {
			return settings.Error("invalid_report")
		}
	}
	for _, l := range o.Evidence.Lanes {
		if ValidateLane(l) != nil {
			return settings.Error("invalid_report")
		}
	}
	return nil
}
func ValidateLane(l diagnosticreport.Lane) error {
	if !settings.ValidID(l.ID) || !settings.OneOf(l.State, "idle", "working", "blocked") || l.CheckGraceMS < 15000 || l.CheckGraceMS > 60000 || l.ReasonCode != nil && !settings.OneOf(*l.ReasonCode, "dependency_unavailable", "configuration_invalid", "storage_unconfigured", "checker_unavailable", "loop_failed", "deadline_exceeded") {
		return settings.Error("invalid_report")
	}
	return nil
}
func SampleDigest(o Observation) string {
	o.SampleAgeMS = nil
	raw, _ := json.Marshal(o)
	return settings.Digest(raw)
}
func View(s *StoredObservation, p settings.Probe, boot string, now time.Time, online bool) ObservationView {
	if s == nil {
		return ObservationView{Observation: Pending(p, boot), Freshness: "never", Persistence: "durable"}
	}
	v := ObservationView{Observation: s.Observation, ReceivedAt: &s.ReceivedAt, Persistence: s.Persistence, Freshness: "fresh"}
	if s.Outcome == "pending" {
		v.Freshness = "never"
		return v
	}
	elapsed := now.Sub(s.ReceivedAt)
	if !s.ReceiptMonotonic.IsZero() {
		elapsed = now.Sub(s.ReceiptMonotonic)
	}
	if elapsed < 0 {
		elapsed = 0
	}
	age := s.EffectiveAgeAtReceipt + elapsed.Milliseconds()
	v.EffectiveAgeMS = &age
	switch {
	case s.FirstServerBootID != boot:
		v.Freshness = "historical"
	case s.ClockUncertain:
		v.Freshness = "clock_uncertain"
	case !online || age >= int64(p.StaleAfterMS):
		v.Freshness = "stale"
	}
	return v
}
