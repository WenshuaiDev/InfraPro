package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/process"
)

func (r *Runtime) native(ctx context.Context, p settings.Probe, b settings.Binding, o model.Observation, self bool) model.Observation {
	o.Evidence.Scope = "host"
	o.Facets.Presence = "unknown"
	policy := securefile.Policy{UID: *b.ExpectedUID, SharedGroup: true}
	var report diagnosticreport.Report
	var pid int32
	var start string
	path, limit := b.ReportFile, int64(diagnosticreport.MaxReportBytes)
	if path == "" {
		path = b.PIDFile
		limit = 4096
	}
	if !settings.Within(path, b.AllowedRoot) {
		return model.Failure(o, "invalid_report", "report_invalid")
	}
	f, err := securefile.Open(path, policy)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.Failure(o, "not_configured", "process_locator_missing")
		}
		return failure(o, err)
	}
	raw, err := readLimit(f, limit)
	info, statErr := f.Stat()
	f.Close()
	if err != nil || statErr != nil {
		return model.Failure(o, "invalid_report", "report_invalid")
	}
	if b.ReportFile != "" {
		if settings.Strict(raw, &report) != nil || r.validateReport(report, p, b, self) != nil {
			return model.Failure(o, "invalid_report", "report_invalid")
		}
		pid = report.Process.PID
		start = report.Process.OSStartTimeMS
	} else if b.PIDFileFormat == "diagnostics" {
		var v struct {
			PID   int32  `json:"pid"`
			Start string `json:"os_start_time_ms"`
		}
		if settings.Strict(raw, &v) != nil || v.PID < 1 || !settings.Decimal(v.Start, false) {
			return model.Failure(o, "invalid_report", "identity_mismatch")
		}
		pid, start = v.PID, v.Start
	} else {
		line := strings.TrimSpace(string(raw))
		if b.PIDFileFormat == "postgres" {
			line = strings.Split(line, "\n")[0]
		}
		n, e := strconv.ParseInt(line, 10, 32)
		if e != nil || n < 1 {
			return model.Failure(o, "invalid_report", "identity_mismatch")
		}
		pid = int32(n)
	}
	if self && b.PIDNamespace == "container" {
		if !r.containerReportIdentity(ctx, p, b, report) {
			return model.Failure(o, "unsupported", "vantage_mismatch")
		}
		o.Evidence.Scope = "container"
	} else {
		proc, e := process.NewProcessWithContext(ctx, pid)
		if e != nil {
			if errors.Is(e, process.ErrorProcessNotRunning) {
				o.Facets.Presence = "missing"
				return model.Failure(o, "failed", "process_not_found")
			}
			return failure(o, e)
		}
		exists, e := proc.IsRunningWithContext(ctx)
		if e != nil {
			return failure(o, e)
		}
		if !exists {
			o.Facets.Presence = "missing"
			return model.Failure(o, "failed", "process_not_found")
		}
		created, e := proc.CreateTimeWithContext(ctx)
		if e != nil {
			return failure(o, e)
		}
		executable, e := proc.ExeWithContext(ctx)
		if e != nil {
			return failure(o, e)
		}
		uids, e := proc.UidsWithContext(ctx)
		if e != nil {
			return failure(o, e)
		}
		if start != "" && strconv.FormatInt(created, 10) != start || start == "" && info.ModTime().UnixMilli() <= created || filepath.Clean(executable) != b.ExpectedExecutable || len(uids) == 0 || int(uids[0]) != *b.ExpectedUID {
			return model.Failure(o, "invalid_report", "identity_mismatch")
		}
		start = strconv.FormatInt(created, 10)
		if !self {
			id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(p.TargetID+":"+strconv.Itoa(int(pid))+":"+start)).String()
			rid := p.TargetID
			if rr, ok := r.Manifest.ReplicaFor(p); ok {
				rid = rr.ID
			}
			o.Process = &model.ProcessIdentity{ReplicaID: rid, BootID: id, PID: &pid, OSStartTimeMS: start, PIDNamespace: "host", UID: b.ExpectedUID}
			if times, e := proc.TimesWithContext(ctx); e == nil {
				next := sample{at: time.Now(), identity: id, cpu: times.Total()}
				if old, ok := r.metricDelta(p.ID, next); ok && next.cpu >= old.cpu {
					o.Metrics = append(o.Metrics, metric("process.cpu_cores", "", (next.cpu-old.cpu)/next.at.Sub(old.at).Seconds()))
				} else {
					o.Metrics = append(o.Metrics, absentMetric("process.cpu_cores", "", "not_configured", "check_pending"))
				}
			}
			if memory, e := proc.MemoryInfoWithContext(ctx); e == nil {
				o.Metrics = append(o.Metrics, metric("process.rss_bytes", "", strconv.FormatUint(memory.RSS, 10)))
			}
		}
	}
	o.Facets.Presence = "running"
	if !self {
		return o
	}
	// A deterministic attempt reflects the source report, never the read time.
	sampleAt := report.EmittedAt
	identity := report.Process.BootID + ":" + p.ID + ":" + report.ReportSeq
	o.Process = &model.ProcessIdentity{ReplicaID: report.ReplicaID, BootID: report.Process.BootID, PID: &pid, OSStartTimeMS: start, PIDNamespace: report.Process.PIDNamespace, BuildID: report.Process.BuildID, UID: b.ExpectedUID}
	o.Facets.Readiness = map[string]string{"starting": "starting", "ready": "ready", "unconfigured": "unconfigured", "stopping": "not_ready", "failed": "not_ready"}[report.Phase]
	if report.Phase == "unconfigured" {
		o = model.Failure(o, "not_configured", "storage_unconfigured")
	}
	if report.Phase == "failed" || report.Phase == "stopping" {
		o = model.Failure(o, "failed", "loop_failed")
	}
	if b.DependencyKey != "" {
		found := false
		for _, d := range report.Dependencies {
			if d.ID == b.DependencyKey {
				found = true
				sampleAt = d.CheckedAt
				identity = report.Process.BootID + ":" + p.ID + ":" + d.ID + ":" + d.CheckedAt.Format(time.RFC3339Nano)
				o.Facets = model.NeutralFacets()
				o.Facets.Readiness = d.State
				o.Outcome = "ok"
				o.ReasonCode = "ok"
				if d.State != "ready" {
					reason := "dependency_unavailable"
					if d.ReasonCode != nil {
						reason = *d.ReasonCode
					}
					o = model.Failure(o, "failed", reason)
				}
				break
			}
		}
		if !found {
			return model.Failure(o, "not_configured", "not_configured")
		}
	} else {
		if b.LaneID == "" {
			for _, m := range report.Metrics {
				var value any = float64(m.Value)
				unit := diagnosticreport.MetricUnit(m.Key)
				if unit == "milliseconds" || unit == "position" {
					value = strconv.FormatInt(m.Value, 10)
				}
				o.Metrics = append(o.Metrics, metric(m.Key, m.Dimension, value))
			}
		}
		lanes := []diagnosticreport.Lane{}
		for _, l := range report.Lanes {
			if b.LaneID == "" || b.LaneID == l.ID {
				lanes = append(lanes, l)
			}
		}
		if b.LaneID != "" && len(lanes) == 0 {
			return model.Failure(o, "not_configured", "not_configured")
		}
		o.Evidence.Lanes = lanes
		o.Facets.Loop = "not_applicable"
		for _, l := range lanes {
			state := l.State
			if l.State == "working" && l.DeadlineAt != nil && report.EmittedAt.After(*l.DeadlineAt) || l.NextAttemptAt != nil && report.EmittedAt.After(l.NextAttemptAt.Add(time.Duration(l.CheckGraceMS)*time.Millisecond)) {
				state = "stalled"
			}
			if state == "stalled" {
				o.Facets.Loop = "stalled"
				o = model.Failure(o, "failed", "deadline_exceeded")
			} else if state == "blocked" && o.Facets.Loop != "stalled" {
				o.Facets.Loop = "blocked"
				reason := "loop_failed"
				if l.ReasonCode != nil {
					reason = *l.ReasonCode
				}
				o = model.Failure(o, "failed", reason)
			} else if state == "working" && o.Facets.Loop != "blocked" && o.Facets.Loop != "stalled" {
				o.Facets.Loop = "working"
			} else if o.Facets.Loop == "not_applicable" {
				o.Facets.Loop = "idle"
			}
		}
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(identity)).String()
	o.AttemptID = &id
	o.ObservedAt = &sampleAt
	age := max(int64(0), time.Since(sampleAt).Milliseconds())
	o.SampleAgeMS = &age
	duration := int64(0)
	o.DurationMS = &duration
	// Do not mutate cached attempt content because wall clock moved. Freshness is
	// computed centrally from the original emitted_at; old reports stay old.
	return o
}
func (r *Runtime) validateReport(v diagnosticreport.Report, p settings.Probe, b settings.Binding, self bool) error {
	replica, ok := r.Manifest.ReplicaFor(p)
	if !ok || v.SchemaVersion != 1 || v.InstanceID != r.Manifest.InstanceID || v.ReplicaID != replica.ID || v.NodeID != replica.NodeID || self && v.ReplicaID != b.ExpectedReplicaID || !model.UUID(v.Process.BootID) || v.Process.PID < 1 || !settings.Decimal(v.Process.OSStartTimeMS, false) || !settings.OneOf(v.Process.PIDNamespace, "host", "container") || !model.SafeBuild(v.Process.BuildID) || !settings.Decimal(v.ReportSeq, false) || !settings.Decimal(v.UptimeMS, true) || v.EmittedAt.IsZero() || v.EmittedAt.Nanosecond()%1e6 != 0 || !settings.OneOf(v.Phase, "starting", "ready", "unconfigured", "stopping", "failed") || v.Lanes == nil || len(v.Lanes) > 16 || v.Dependencies == nil || len(v.Dependencies) > 16 || len(v.Metrics) > diagnosticreport.MaxProcessMetrics {
		return settings.Invalid
	}
	if self && v.Process.PIDNamespace != b.PIDNamespace {
		return settings.Invalid
	}
	seen := map[string]bool{}
	for _, l := range v.Lanes {
		if seen[l.ID] || model.ValidateLane(l) != nil {
			return settings.Invalid
		}
		seen[l.ID] = true
		for _, t := range []*time.Time{l.LastAttemptAt, l.LastSuccessAt, l.ActiveSince} {
			if t != nil && (t.After(v.EmittedAt) || t.IsZero()) {
				return settings.Invalid
			}
		}
	}
	seen = map[string]bool{}
	for _, d := range v.Dependencies {
		if !settings.ValidID(d.ID) || seen[d.ID] || !settings.OneOf(d.State, "ready", "not_ready") || d.CheckedAt.IsZero() || d.CheckedAt.After(v.EmittedAt) || d.ReasonCode != nil && !diagnosticreport.KnownReason(*d.ReasonCode) {
			return settings.Invalid
		}
		seen[d.ID] = true
	}
	seen = map[string]bool{}
	tenantCount := 0
	for _, metric := range v.Metrics {
		key := metric.Key + ":" + metric.Dimension
		if seen[key] || !metric.Valid() {
			return settings.Invalid
		}
		seen[key] = true
		if metric.Key == diagnosticreport.TenantQueueKey {
			tenantCount++
		}
	}
	if tenantCount > diagnosticreport.MaxQueueDimensions {
		return settings.Invalid
	}
	return nil
}

// This method is completed by the Docker adapter using the explicitly assigned
// container binding and its declared read-only report mount.
func (r *Runtime) containerReportIdentity(ctx context.Context, p settings.Probe, b settings.Binding, report diagnosticreport.Report) bool {
	return r.verifyContainerReport(ctx, p, b, report)
}
