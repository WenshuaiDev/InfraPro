// Package diagnosticreport is the business-side, bounded, optional diagnostic
// sink. No call to Sink performs filesystem or network IO.
package diagnosticreport

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/securefile"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/process"
)

const MaxReportBytes = 32 << 10

type Identity struct {
	BootID        string `json:"boot_id"`
	PID           int32  `json:"pid"`
	OSStartTimeMS string `json:"os_start_time_ms"`
	PIDNamespace  string `json:"pid_namespace"`
	BuildID       string `json:"build_id"`
}
type Lane struct {
	ID            string     `json:"id"`
	State         string     `json:"state"`
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	LastSuccessAt *time.Time `json:"last_success_at"`
	ActiveSince   *time.Time `json:"active_since"`
	DeadlineAt    *time.Time `json:"deadline_at"`
	NextAttemptAt *time.Time `json:"next_attempt_at"`
	CheckGraceMS  int64      `json:"check_grace_ms"`
	ReasonCode    *string    `json:"reason_code"`
}
type DependencyState struct {
	ID         string    `json:"id"`
	State      string    `json:"state"`
	CheckedAt  time.Time `json:"checked_at"`
	ReasonCode *string   `json:"reason_code"`
}
type Report struct {
	SchemaVersion int               `json:"schema_version"`
	InstanceID    string            `json:"instance_id"`
	ReplicaID     string            `json:"replica_id"`
	NodeID        string            `json:"node_id"`
	Process       Identity          `json:"process"`
	ReportSeq     string            `json:"report_seq"`
	EmittedAt     time.Time         `json:"emitted_at"`
	UptimeMS      string            `json:"uptime_ms"`
	Phase         string            `json:"phase"`
	Lanes         []Lane            `json:"lanes"`
	Metrics       []ProcessMetric   `json:"metrics,omitempty"`
	Dependencies  []DependencyState `json:"dependencies"`
}

// Sink methods take only static lane/dependency IDs and fixed reasons. A nil
// *Reporter implements the no-op sink, so callers do not need diagnostic guards.
type Sink interface {
	Starting()
	Ready()
	Unconfigured()
	Failed(string)
	Begin(string, time.Time)
	End(string, string, time.Time)
	Idle(string, time.Time)
	Dependency(string, bool, string)
	Stopping()
}

type Reporter struct {
	mu       sync.Mutex
	report   Report
	started  time.Time
	sequence uint64
	trigger  chan struct{}
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	write    func([]byte) error
}

var validID = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
var reasons = map[string]bool{"dependency_unavailable": true, "configuration_invalid": true, "storage_unconfigured": true, "checker_unavailable": true, "loop_failed": true, "deadline_exceeded": true,
	"event_publish_failed": true, "event_position_gap": true, "event_quarantined": true,
	"outbox_backpressure": true, "dispatch_fence_rejected": true, "broker_unavailable": true,
}

// KnownReason is shared by producers and observers so a private error string
// cannot be reclassified as safe by passing it through the process report.
func KnownReason(s string) bool { return reasons[s] }

var BuildID = "dev"

func safeReason(s string) *string {
	if s == "" {
		return nil
	}
	if !reasons[s] {
		s = "loop_failed"
	}
	return &s
}

// FromEnv must be called in the owning parent before business configuration and
// database initialization. It never propagates an error into business startup.
func FromEnv() *Reporter {
	path := os.Getenv("INFRAPRO_DIAGNOSTICS_REPORT_FILE")
	instance := os.Getenv("INFRAPRO_DIAGNOSTICS_INSTANCE_ID")
	replica := os.Getenv("INFRAPRO_DIAGNOSTICS_REPLICA_ID")
	node := os.Getenv("INFRAPRO_DIAGNOSTICS_NODE_ID")
	if path == "" && instance == "" && replica == "" && node == "" {
		return nil
	}
	if path == "" || !validID.MatchString(instance) || !validID.MatchString(replica) || !validID.MatchString(node) {
		slog.Warn("diagnostic reporter disabled", "code", "configuration_invalid")
		return nil
	}
	// Identity lookup is bounded and happens before entering business execution.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p, e := process.NewProcessWithContext(ctx, int32(os.Getpid()))
	if e != nil {
		return nil
	}
	created, e := p.CreateTimeWithContext(ctx)
	if e != nil {
		return nil
	}
	build := BuildID
	if len(build) > 64 || !regexp.MustCompile(`^[a-zA-Z0-9_.+-]+$`).MatchString(build) {
		build = "unknown"
	}
	namespace := "host"
	if os.Getenv("INFRAPRO_DIAGNOSTICS_PID_NAMESPACE") == "container" {
		namespace = "container"
	}
	policy := securefile.Private()
	if os.Getenv("INFRAPRO_DIAGNOSTICS_REPORT_SHARED_GROUP") == "true" {
		policy.SharedGroup = true
	}
	r := newReporter(Report{SchemaVersion: 1, InstanceID: instance, ReplicaID: replica, NodeID: node,
		Process: Identity{BootID: uuid.NewString(), PID: int32(os.Getpid()), OSStartTimeMS: strconv.FormatInt(created, 10), PIDNamespace: namespace, BuildID: build}, Phase: "starting", Lanes: []Lane{}, Dependencies: []DependencyState{}},
		func(data []byte) error { return securefile.Atomic(path, data, policy, false) })
	return r
}

func newReporter(report Report, writer func([]byte) error) *Reporter {
	r := &Reporter{report: report, started: time.Now(), trigger: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}), write: writer}
	go r.run()
	r.signal()
	return r
}
func (r *Reporter) signal() {
	if r != nil {
		select {
		case r.trigger <- struct{}{}:
		default:
		}
	}
}
func (r *Reporter) phase(v string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.report.Phase = v
	r.mu.Unlock()
	r.signal()
}
func (r *Reporter) Starting()     { r.phase("starting") }
func (r *Reporter) Ready()        { r.phase("ready") }
func (r *Reporter) Unconfigured() { r.phase("unconfigured") }
func (r *Reporter) Failed(reason string) {
	if r == nil {
		return
	}
	r.phase("failed")
	r.End("startup", reason, time.Time{})
}
func (r *Reporter) Stopping() { r.phase("stopping") }

func timeptr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	t = t.UTC().Truncate(time.Millisecond)
	return &t
}
func (r *Reporter) lane(id string, f func(*Lane)) {
	if r == nil || !validID.MatchString(id) {
		return
	}
	r.mu.Lock()
	index := -1
	for i := range r.report.Lanes {
		if r.report.Lanes[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 && len(r.report.Lanes) < 16 {
		r.report.Lanes = append(r.report.Lanes, Lane{ID: id, State: "idle", CheckGraceMS: 15000})
		index = len(r.report.Lanes) - 1
	}
	if index >= 0 {
		f(&r.report.Lanes[index])
	}
	r.mu.Unlock()
	r.signal()
}
func (r *Reporter) Begin(id string, deadline time.Time) {
	r.lane(id, func(l *Lane) {
		if l.State != "working" {
			l.LastAttemptAt = timeptr(time.Now())
			l.ActiveSince = l.LastAttemptAt
		}
		l.State = "working"
		l.DeadlineAt = timeptr(deadline)
		l.NextAttemptAt = nil
		l.ReasonCode = nil
	})
}
func (r *Reporter) End(id, reason string, next time.Time) {
	r.lane(id, func(l *Lane) {
		l.State = "idle"
		l.ReasonCode = safeReason(reason)
		if reason != "" {
			l.State = "blocked"
		} else {
			l.LastSuccessAt = timeptr(time.Now())
		}
		if l.LastAttemptAt == nil {
			l.LastAttemptAt = timeptr(time.Now())
		}
		l.ActiveSince = nil
		l.DeadlineAt = nil
		l.NextAttemptAt = timeptr(next)
	})
}
func (r *Reporter) Idle(id string, next time.Time) {
	r.lane(id, func(l *Lane) {
		l.State = "idle"
		l.LastAttemptAt = timeptr(time.Now())
		l.LastSuccessAt = l.LastAttemptAt
		l.ActiveSince = nil
		l.DeadlineAt = nil
		l.NextAttemptAt = timeptr(next)
		l.ReasonCode = nil
	})
}
func (r *Reporter) Dependency(id string, ready bool, reason string) {
	if r == nil || !validID.MatchString(id) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	d := DependencyState{ID: id, State: "ready", CheckedAt: time.Now().UTC().Truncate(time.Millisecond), ReasonCode: safeReason(reason)}
	if !ready {
		d.State = "not_ready"
		if d.ReasonCode == nil {
			d.ReasonCode = safeReason("dependency_unavailable")
		}
	}
	for i := range r.report.Dependencies {
		if r.report.Dependencies[i].ID == id {
			r.report.Dependencies[i] = d
			r.signal()
			return
		}
	}
	if len(r.report.Dependencies) < 16 {
		r.report.Dependencies = append(r.report.Dependencies, d)
	}
	r.signal()
}
func (r *Reporter) snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sequence++
	r.report.ReportSeq = strconv.FormatUint(r.sequence, 10)
	r.report.EmittedAt = time.Now().UTC().Truncate(time.Millisecond)
	r.report.UptimeMS = strconv.FormatInt(time.Since(r.started).Milliseconds(), 10)
	sort.Slice(r.report.Lanes, func(i, j int) bool { return r.report.Lanes[i].ID < r.report.Lanes[j].ID })
	data, _ := json.Marshal(r.report)
	return data
}
func (r *Reporter) run() {
	defer close(r.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var last, warned time.Time
	write := func() {
		data := r.snapshot()
		if len(data) > MaxReportBytes {
			return
		}
		if r.write(data) != nil && time.Since(warned) > time.Minute {
			slog.Warn("diagnostic report unavailable", "code", "report_write_failed")
			warned = time.Now()
		}
		last = time.Now()
	}
	for {
		select {
		case <-r.stop:
			write()
			return
		case <-ticker.C:
			write()
		case <-r.trigger:
			if delay := time.Until(last.Add(time.Second)); delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-r.stop:
					timer.Stop()
					write()
					return
				}
			}
			write()
		}
	}
}

// Close bounds waiting even if a filesystem stalls forever. There is only one
// writer goroutine; failed writes never spawn retries in separate goroutines.
func (r *Reporter) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.report.Phase != "failed" {
		r.report.Phase = "stopping"
	}
	r.mu.Unlock()
	r.once.Do(func() { close(r.stop) })
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-r.done:
	case <-timer.C:
	}
}

func OrNoop(s Sink) Sink {
	if s == nil {
		return (*Reporter)(nil)
	}
	return s
}

func Deadline(ctx context.Context) time.Time { t, _ := ctx.Deadline(); return t }
func Result(err error) string {
	if err != nil {
		return "loop_failed"
	}
	return ""
}
