// Package ingest implements fenced, atomic, full-frame observation ingestion.
package ingest

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/store"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
)

type SessionRequest struct {
	ProtocolMajor      int    `json:"protocol_major"`
	InstanceID         string `json:"instance_id"`
	NodeID             string `json:"node_id"`
	ManifestRevision   string `json:"manifest_revision"`
	AssignmentRevision string `json:"assignment_revision"`
	Epoch              string `json:"epoch"`
	BootID             string `json:"boot_id"`
}
type SessionResponse struct {
	Token                string    `json:"token"`
	ExpiresAt            time.Time `json:"expires_at"`
	ServerBootID         string    `json:"server_boot_id"`
	LastAcceptedSequence string    `json:"last_accepted_sequence"`
	ProbeIDs             []string  `json:"probe_ids"`
	MinReportIntervalMS  int       `json:"min_report_interval_ms"`
}
type Frame struct {
	SessionRequest
	Sequence     string              `json:"sequence"`
	GeneratedAt  time.Time           `json:"generated_at"`
	Observations []model.Observation `json:"observations"`
}
type Ack struct {
	AcceptedSequence string    `json:"accepted_sequence"`
	Duplicate        bool      `json:"duplicate"`
	ServerBootID     string    `json:"server_boot_id"`
	ReceivedAt       time.Time `json:"received_at"`
	ConfigRevision   string    `json:"config_revision"`
}
type session struct {
	Request           SessionRequest
	KeyID, Generation string
	Created, Expires  time.Time
}
type Snapshot struct {
	Manifest                              settings.Manifest
	Latest                                map[string]model.StoredObservation
	Fences                                map[string]store.Fence
	Online                                map[string]bool
	BootID                                string
	Now                                   time.Time
	ConfigState, KeyringState, StoreState string
	Meta                                  store.Meta
	Transitions                           []model.Transition
	Metrics                               []model.MetricPoint
	CPUSamples                            map[string][]model.MetricPoint
	StoreInfo                             map[string]any
}
type Engine struct {
	mu                                    sync.RWMutex
	db                                    *store.Store
	manifest                              settings.Manifest
	keys                                  settings.Keyring
	meta                                  store.Meta
	latest                                map[string]model.StoredObservation
	fences                                map[string]store.Fence
	sessions                              map[string]session
	online                                map[string]time.Time
	generated                             map[string]time.Time
	transitions                           []model.Transition
	metrics                               []model.MetricPoint
	cpuSamples                            map[string][]model.MetricPoint
	lastStates                            map[string]string
	boot                                  string
	local                                 string
	now                                   func() time.Time
	configState, keyringState, storeState string
}

func New(db *store.Store, instance, local string) (*Engine, error) {
	e := &Engine{db: db, boot: uuid.NewString(), local: local, now: time.Now, latest: map[string]model.StoredObservation{}, fences: map[string]store.Fence{}, sessions: map[string]session{}, online: map[string]time.Time{}, generated: map[string]time.Time{}, lastStates: map[string]string{}, configState: "configuration_invalid", keyringState: "keyring_invalid", storeState: "state_unavailable", meta: store.Meta{SchemaVersion: 1, InstanceID: instance, ManifestRevision: "0", KeyringRevision: "0"}, transitions: []model.Transition{}, metrics: []model.MetricPoint{}}
	e.cpuSamples = map[string][]model.MetricPoint{}
	if db != nil {
		d, err := db.Load()
		if err != nil {
			return nil, err
		}
		e.meta = d.Meta
		e.latest = d.Latest
		e.fences = d.Fences
		e.transitions = d.Transitions
		e.metrics = d.Metrics
		e.storeState = "ok"
	}
	return e, nil
}
func (e *Engine) BootID() string { return e.boot }
func (e *Engine) ConfigFailed()  { e.mu.Lock(); e.configState = "reload_error"; e.mu.Unlock() }
func (e *Engine) KeysFailed() {
	e.mu.Lock()
	e.keyringState = "keyring_invalid"
	e.sessions = map[string]session{}
	e.online = map[string]time.Time{}
	e.mu.Unlock()
}
func (e *Engine) SetConfig(ctx context.Context, m settings.Manifest, keys settings.Keyring, limits settings.Limits) error {
	if m.Validate(limits) != nil || keys.Validate(m, e.local) != nil {
		return settings.Invalid
	}
	if err := e.SetManifest(ctx, m, limits); err != nil {
		return err
	}
	return e.SetKeys(ctx, keys)
}
func (e *Engine) SetManifest(ctx context.Context, m settings.Manifest, limits settings.Limits) error {
	if m.Validate(limits) != nil {
		return settings.Invalid
	}
	raw, _ := json.Marshal(m)
	digest := settings.Digest(raw)
	e.mu.Lock()
	defer e.mu.Unlock()
	if m.InstanceID != e.meta.InstanceID {
		return settings.Invalid
	}
	if settings.Number(m.Revision) < settings.Number(e.meta.ManifestRevision) || m.Revision == e.meta.ManifestRevision && e.meta.ManifestDigest != "" && digest != e.meta.ManifestDigest {
		return settings.Error("config_rollback")
	}
	changed := e.manifest.Revision != m.Revision
	meta := e.meta
	meta.ManifestRevision = m.Revision
	meta.ManifestDigest = digest
	removed := []string{}
	current := map[string]bool{}
	for _, p := range m.Probes {
		current[p.ID] = true
	}
	for id := range e.latest {
		if !current[id] {
			removed = append(removed, id)
		}
	}
	if e.db != nil {
		if err := e.db.Commit(ctx, store.Change{Meta: &meta, DeleteLatest: removed, Now: e.now()}); err != nil {
			e.storeState = "state_unavailable"
			return err
		}
		e.storeState = "ok"
	}
	e.meta = meta
	e.manifest = m
	e.configState = "ok"
	if changed {
		e.sessions = map[string]session{}
		e.online = map[string]time.Time{}
		e.keyringState = "keyring_invalid"
	}
	for _, id := range removed {
		delete(e.latest, id)
		delete(e.lastStates, id)
		delete(e.cpuSamples, id)
	}
	return nil
}
func (e *Engine) SetKeys(ctx context.Context, keys settings.Keyring) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if keys.Validate(e.manifest, e.local) != nil {
		return settings.Invalid
	}
	raw, _ := json.Marshal(keys)
	digest := settings.Digest(raw)
	if settings.Number(keys.Revision) < settings.Number(e.meta.KeyringRevision) || keys.Revision == e.meta.KeyringRevision && e.meta.KeyringDigest != "" && digest != e.meta.KeyringDigest {
		return settings.Error("config_rollback")
	}
	previous := map[string]settings.NodeKey{}
	for _, k := range e.keys.Keys {
		previous[k.KeyID] = k
	}
	meta := e.meta
	meta.KeyringRevision = keys.Revision
	meta.KeyringDigest = digest
	if e.db != nil {
		if err := e.db.Commit(ctx, store.Change{Meta: &meta, Now: e.now()}); err != nil {
			e.storeState = "state_unavailable"
			return err
		}
		e.storeState = "ok"
	}
	e.meta = meta
	e.keys = keys
	e.keyringState = "ok"
	// Rotating one node must not interrupt unrelated observers. The keyring
	// revision fences edits; sessions are tied to their exact key and generation.
	for token, session := range e.sessions {
		current, ok := e.key(session.KeyID)
		if !ok || current != previous[session.KeyID] {
			delete(e.sessions, token)
			delete(e.online, session.Request.NodeID)
		}
	}
	return nil
}
func (e *Engine) key(id string) (settings.NodeKey, bool) {
	if e.keyringState != "ok" {
		return settings.NodeKey{}, false
	}
	for _, k := range e.keys.Keys {
		if k.KeyID == id && k.Enabled {
			return k, true
		}
	}
	return settings.NodeKey{}, false
}

// AuthenticateKey bounds per-node handshakes before parsing or changing a fence.
func (e *Engine) AuthenticateKey(bearer string) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.keyringState != "ok" {
		return "", settings.Error("keyring_invalid")
	}
	parts := strings.Split(bearer, ".")
	if len(parts) != 2 {
		return "", settings.Error("observer_scope_denied")
	}
	k, ok := e.key(parts[0])
	digest := settings.Digest([]byte(parts[1]))
	if !ok || subtle.ConstantTimeCompare([]byte(digest), []byte(k.SecretSHA256)) != 1 || k.NodeID == e.local {
		return "", settings.Error("observer_scope_denied")
	}
	return k.NodeID, nil
}
func (e *Engine) validateIdentity(r SessionRequest) error {
	if r.ProtocolMajor != 1 {
		return settings.Error("protocol_unsupported")
	}
	if r.InstanceID != e.manifest.InstanceID {
		return settings.Error("observer_scope_denied")
	}
	if r.ManifestRevision != e.manifest.Revision {
		return settings.Error("manifest_mismatch")
	}
	n, ok := e.manifest.Node(r.NodeID)
	if !ok || !n.Enabled {
		return settings.Error("observer_scope_denied")
	}
	if n.AssignmentRevision != r.AssignmentRevision {
		return settings.Error("assignment_mismatch")
	}
	if !settings.Decimal(r.Epoch, false) || !model.UUID(r.BootID) {
		return settings.Error("invalid_request")
	}
	return nil
}
func (e *Engine) Handshake(ctx context.Context, bearer string, r SessionRequest) (SessionResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out SessionResponse
	if e.keyringState != "ok" {
		return out, settings.Error("keyring_invalid")
	}
	if e.storeState != "ok" || e.db == nil {
		return out, settings.Error("state_unavailable")
	}
	parts := strings.Split(bearer, ".")
	if len(parts) != 2 {
		return out, settings.Error("observer_scope_denied")
	}
	k, ok := e.key(parts[0])
	digest := settings.Digest([]byte(parts[1]))
	if !ok || subtle.ConstantTimeCompare([]byte(digest), []byte(k.SecretSHA256)) != 1 || k.NodeID != r.NodeID || k.AssignmentRevision != r.AssignmentRevision || r.NodeID == e.local {
		return out, settings.Error("observer_scope_denied")
	}
	if err := e.validateIdentity(r); err != nil {
		return out, err
	}
	fk := store.Key(k.NodeID, k.Generation)
	f, exists := e.fences[fk]
	if exists && (settings.Number(r.Epoch) < settings.Number(f.Epoch) || r.Epoch == f.Epoch && r.BootID != f.BootID) {
		return out, settings.Error("epoch_conflict")
	}
	if !exists || settings.Number(r.Epoch) > settings.Number(f.Epoch) {
		f = store.Fence{NodeID: k.NodeID, Generation: k.Generation, Epoch: r.Epoch, BootID: r.BootID, Sequence: "0"}
		if err := e.db.Commit(ctx, store.Change{Fences: []store.Fence{f}, Now: e.now()}); err != nil {
			e.storeState = "state_unavailable"
			return out, err
		}
		e.fences[fk] = f
		delete(e.online, k.NodeID)
		delete(e.generated, k.NodeID)
		for token, s := range e.sessions {
			if s.Request.NodeID == k.NodeID {
				delete(e.sessions, token)
			}
		}
	}
	now := e.now()
	token := settings.NewSecret()
	hash := settings.Digest([]byte(token))
	for id, s := range e.sessions {
		if !s.Expires.After(now) {
			delete(e.sessions, id)
		}
	}
	same := []string{}
	for id, s := range e.sessions {
		if s.Request.NodeID == r.NodeID {
			same = append(same, id)
		}
	}
	sort.Slice(same, func(i, j int) bool { return e.sessions[same[i]].Created.Before(e.sessions[same[j]].Created) })
	for len(same) >= 2 {
		delete(e.sessions, same[0])
		same = same[1:]
	}
	e.sessions[hash] = session{r, k.KeyID, k.Generation, now, now.Add(10 * time.Minute)}
	ids := []string{}
	for _, p := range e.manifest.Assigned(r.NodeID) {
		ids = append(ids, p.ID)
	}
	return SessionResponse{token, now.Add(10 * time.Minute), e.boot, f.Sequence, ids, 5000}, nil
}
func (e *Engine) authorize(token string) (session, error) {
	s, ok := e.sessions[settings.Digest([]byte(token))]
	if !ok || !s.Expires.After(e.now()) {
		return s, settings.Error("observer_session_expired")
	}
	k, ok := e.key(s.KeyID)
	if !ok || k.Generation != s.Generation || e.validateIdentity(s.Request) != nil {
		return s, settings.Error("observer_scope_denied")
	}
	f := e.fences[store.Key(k.NodeID, k.Generation)]
	if f.Epoch != s.Request.Epoch || f.BootID != s.Request.BootID {
		return s, settings.Error("epoch_conflict")
	}
	return s, nil
}
func (e *Engine) Authorize(token string) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, err := e.authorize(token)
	return s.Request.NodeID, err
}
func (e *Engine) Accept(ctx context.Context, token string, raw []byte) (Ack, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var ack Ack
	s, err := e.authorize(token)
	if err != nil {
		return ack, err
	}
	if len(raw) > 1<<20 {
		return ack, settings.Error("payload_too_large")
	}
	var frame Frame
	if settings.Strict(raw, &frame) != nil {
		return ack, settings.Error("invalid_report")
	}
	if frame.SessionRequest != s.Request {
		return ack, settings.Error("observer_scope_denied")
	}
	if e.storeState != "ok" || e.db == nil {
		return ack, settings.Error("state_unavailable")
	}
	fk := store.Key(s.Request.NodeID, s.Generation)
	f := e.fences[fk]
	digest := settings.Digest(raw)
	if !settings.Decimal(frame.Sequence, false) {
		return ack, settings.Error("invalid_report")
	}
	if frame.Sequence == f.Sequence {
		if digest != f.Digest {
			return ack, settings.Error("sequence_conflict")
		}
		if f.ReceivedAt == nil {
			return ack, settings.Error("invalid_report")
		}
		return Ack{f.Sequence, true, e.boot, *f.ReceivedAt, e.manifest.Revision}, nil
	}
	if settings.Number(frame.Sequence) < settings.Number(f.Sequence) {
		return ack, settings.Error("stale_sequence")
	}
	now := e.now()
	if last := e.online[s.Request.NodeID]; !last.IsZero() && now.Sub(last) < time.Second {
		return ack, settings.Error("rate_limited")
	}
	next, err := e.prepare(frame, s.Generation, now)
	if err != nil {
		return ack, err
	}
	f.Sequence = frame.Sequence
	f.Digest = digest
	t := now.UTC().Truncate(time.Millisecond)
	f.ReceivedAt = &t
	transitions, metrics := e.history(next, now)
	if err = e.db.Commit(ctx, store.Change{Fences: []store.Fence{f}, Latest: next, Transitions: transitions, Metrics: metrics, Now: now}); err != nil {
		e.storeState = "state_unavailable"
		return ack, err
	}
	e.apply(next, transitions, metrics)
	e.fences[fk] = f
	e.online[s.Request.NodeID] = now
	e.generated[s.Request.NodeID] = frame.GeneratedAt
	return Ack{f.Sequence, false, e.boot, t, e.manifest.Revision}, nil
}
func (e *Engine) prepare(frame Frame, generation string, now time.Time) ([]model.StoredObservation, error) {
	if err := e.validateIdentity(frame.SessionRequest); err != nil {
		return nil, err
	}
	if frame.GeneratedAt.IsZero() || frame.GeneratedAt.Nanosecond()%1e6 != 0 || frame.GeneratedAt.Before(e.generated[frame.NodeID]) {
		return nil, settings.Error("invalid_report")
	}
	assigned := e.manifest.Assigned(frame.NodeID)
	if len(frame.Observations) != len(assigned) || len(assigned) > 256 {
		return nil, settings.Error("observer_scope_denied")
	}
	specs := map[string]settings.Probe{}
	for _, p := range assigned {
		specs[p.ID] = p
	}
	next := make([]model.StoredObservation, 0, len(assigned))
	total := 0
	uncertain := now.Sub(frame.GeneratedAt) > 5*time.Second || frame.GeneratedAt.Sub(now) > 5*time.Second
	for _, o := range frame.Observations {
		p, ok := specs[o.ProbeID]
		if !ok {
			return nil, settings.Error("observer_scope_denied")
		}
		delete(specs, o.ProbeID)
		if model.Validate(o, p, frame.BootID) != nil {
			return nil, settings.Error("invalid_report")
		}
		if o.ObservedAt != nil && o.ObservedAt.After(frame.GeneratedAt) {
			return nil, settings.Error("invalid_report")
		}
		if o.Process != nil {
			r, ok := e.manifest.ReplicaFor(p)
			if !ok || o.Process.ReplicaID != r.ID {
				return nil, settings.Error("invalid_report")
			}
		}
		s := model.StoredObservation{Observation: o, InstanceID: frame.InstanceID, NodeID: frame.NodeID, ManifestRevision: frame.ManifestRevision, AssignmentRevision: frame.AssignmentRevision, CredentialGeneration: generation, Epoch: frame.Epoch, Sequence: frame.Sequence, ReceivedAt: now.UTC().Truncate(time.Millisecond), AcceptedServerBootID: e.boot, FirstAcceptedAt: now.UTC().Truncate(time.Millisecond), FirstServerBootID: e.boot, ClockUncertain: uncertain, ReceiptMonotonic: now, Persistence: "durable"}
		if o.ObservedAt != nil {
			s.EffectiveAgeAtReceipt = max(*o.SampleAgeMS, max(int64(0), now.Sub(*o.ObservedAt).Milliseconds()))
		}
		if old, ok := e.latest[o.ProbeID]; ok && old.ManifestRevision == frame.ManifestRevision {
			s.LastSuccessAt = old.LastSuccessAt
		}
		if o.Outcome == "ok" && !uncertain && s.EffectiveAgeAtReceipt < int64(p.StaleAfterMS) {
			s.LastSuccessAt = o.ObservedAt
		}
		if old, ok := e.latest[o.ProbeID]; ok && old.AttemptID != nil && o.AttemptID != nil && *old.AttemptID == *o.AttemptID {
			// Observer boot is transport metadata, not a new sample of a cached report.
			before := old.Observation
			before.ObserverBootID = o.ObserverBootID
			if model.SampleDigest(before) != model.SampleDigest(o) || o.SampleAgeMS == nil || *o.SampleAgeMS < *old.SampleAgeMS {
				return nil, settings.Error("invalid_report")
			}
			s.FirstAcceptedAt = old.FirstAcceptedAt
			s.FirstServerBootID = old.FirstServerBootID
			s.ClockUncertain = s.ClockUncertain || old.ClockUncertain
			s.EffectiveAgeAtReceipt = max(s.EffectiveAgeAtReceipt, old.EffectiveAgeAtReceipt+max(int64(0), now.Sub(old.ReceivedAt).Milliseconds()))
		}
		raw, _ := json.Marshal(s)
		total += len(raw)
		next = append(next, s)
	}
	if total > 4<<20 {
		return nil, settings.Error("payload_too_large")
	}
	return next, nil
}

// AcceptLocal uses the same validator and bounded state path, but never grants
// remote credentials. If storage fails it keeps explicitly volatile observations.
func (e *Engine) AcceptLocal(ctx context.Context, frame Frame) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if frame.NodeID != e.local {
		return settings.Error("observer_scope_denied")
	}
	now := e.now()
	next, err := e.prepare(frame, "0", now)
	if err != nil {
		return err
	}
	transitions, metrics := e.history(next, now)
	if e.db == nil || e.db.Commit(ctx, store.Change{Latest: next, Transitions: transitions, Metrics: metrics, Now: now}) != nil {
		e.storeState = "state_unavailable"
		for i := range next {
			next[i].Persistence = "volatile"
		}
		transitions = nil
		metrics = nil
	} else {
		e.storeState = "ok"
	}
	e.apply(next, transitions, metrics)
	e.online[frame.NodeID] = now
	e.generated[frame.NodeID] = frame.GeneratedAt
	return nil
}
func (e *Engine) apply(next []model.StoredObservation, t []model.Transition, m []model.MetricPoint) {
	for _, s := range next {
		old, exists := e.latest[s.ProbeID]
		if s.AttemptID != nil && (!exists || old.AttemptID == nil || *old.AttemptID != *s.AttemptID) && !s.ClockUncertain && s.ObservedAt != nil {
			for _, metric := range s.Metrics {
				if metric.Key == "cpu.utilization_ratio" {
					points := e.cpuSamples[s.ProbeID]
					points = append(points, model.MetricPoint{TargetKind: "node", TargetID: s.NodeID, NodeID: s.NodeID, Source: s.Evidence.Source, Scope: s.Evidence.Scope, At: *s.ObservedAt, Metric: metric})
					if len(points) > 60 {
						points = points[len(points)-60:]
					}
					e.cpuSamples[s.ProbeID] = points
				}
			}
		}
		e.latest[s.ProbeID] = s
	}
	e.transitions = append(e.transitions, t...)
	for _, transition := range t {
		e.lastStates[transition.ProbeID] = transition.ToState
	}
	if len(e.transitions) > 10000 {
		e.transitions = e.transitions[len(e.transitions)-10000:]
	}
	indices := make(map[string]int, len(e.metrics))
	metricKey := func(p model.MetricPoint) string {
		return store.Key(p.TargetKind, p.TargetID, p.NodeID, p.Key, p.DimensionID, p.At.UTC().Truncate(time.Minute).Format(time.RFC3339))
	}
	for i, p := range e.metrics {
		indices[metricKey(p)] = i
	}
	for _, p := range m {
		key := metricKey(p)
		if i, ok := indices[key]; ok {
			if !p.At.Before(e.metrics[i].At) {
				e.metrics[i] = p
			}
		} else {
			indices[key] = len(e.metrics)
			e.metrics = append(e.metrics, p)
		}
	}
	if len(e.metrics) > 50000 {
		e.metrics = e.metrics[len(e.metrics)-50000:]
	}
}
func signature(v model.ObservationView) string {
	boot := ""
	if v.Process != nil {
		boot = v.Process.BootID
	}
	return strings.Join([]string{v.Outcome, v.Facets.Presence, v.Facets.Readiness, v.Facets.Loop, v.Freshness, v.ReasonCode, boot}, "/")
}
func (e *Engine) history(next []model.StoredObservation, now time.Time) ([]model.Transition, []model.MetricPoint) {
	transitions := []model.Transition{}
	metrics := []model.MetricPoint{}
	specs := map[string]settings.Probe{}
	for _, p := range e.manifest.Probes {
		specs[p.ID] = p
	}
	for _, s := range next {
		p := specs[s.ProbeID]
		v := model.View(&s, p, e.boot, now, true)
		sig := signature(v)
		previous := e.lastStates[s.ProbeID]
		if sig != previous {
			transitions = append(transitions, model.Transition{ID: uuid.NewString(), TargetKind: p.TargetKind, TargetID: p.TargetID, ProbeID: p.ID, At: now.UTC().Truncate(time.Millisecond), FromState: previous, ToState: sig, ReasonCode: s.ReasonCode, ObservationRef: s.AttemptID})
		}
		if v.Freshness == "fresh" && s.ObservedAt != nil {
			old, ok := e.latest[s.ProbeID]
			isNew := !ok || old.AttemptID == nil || s.AttemptID != nil && *old.AttemptID != *s.AttemptID
			if isNew {
				for _, metric := range s.Metrics {
					metrics = append(metrics, model.MetricPoint{TargetKind: p.TargetKind, TargetID: p.TargetID, NodeID: p.NodeID, Source: s.Evidence.Source, Scope: s.Evidence.Scope, At: *s.ObservedAt, Metric: metric})
				}
			}
		}
	}
	return transitions, metrics
}
func (e *Engine) Read() Snapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	now := e.now()
	s := Snapshot{Manifest: e.manifest, Latest: map[string]model.StoredObservation{}, Fences: map[string]store.Fence{}, Online: map[string]bool{}, BootID: e.boot, Now: now, ConfigState: e.configState, KeyringState: e.keyringState, StoreState: e.storeState, Meta: e.meta, Transitions: append([]model.Transition{}, e.transitions...), Metrics: append([]model.MetricPoint{}, e.metrics...)}
	s.CPUSamples = map[string][]model.MetricPoint{}
	for id, points := range e.cpuSamples {
		s.CPUSamples[id] = append([]model.MetricPoint{}, points...)
	}
	for n, t := range e.online {
		s.Online[n] = now.Sub(t) < 20*time.Second
	}
	for k, f := range e.fences {
		s.Fences[k] = f
	}
	for id, v := range e.latest {
		if v.ManifestRevision != e.manifest.Revision {
			v.FirstServerBootID = ""
		}
		if v.NodeID != e.local {
			k, ok := e.activeNode(v.NodeID)
			f := e.fences[store.Key(v.NodeID, k.Generation)]
			if !ok || v.CredentialGeneration != k.Generation || v.Epoch != f.Epoch || v.ObserverBootID != f.BootID {
				v.FirstServerBootID = ""
			}
		}
		s.Latest[id] = v
	}
	if e.db != nil {
		s.StoreInfo = e.db.Info()
		if n, ok := s.StoreInfo["evicted_count"].(uint64); ok {
			s.Meta.EvictedCount = n
		}
	}
	return s
}
func (e *Engine) activeNode(node string) (settings.NodeKey, bool) {
	for _, k := range e.keys.Keys {
		if k.NodeID == node && k.Enabled && e.keyringState == "ok" {
			return k, true
		}
	}
	return settings.NodeKey{}, false
}
func (e *Engine) Tick(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	changes := []model.Transition{}
	for _, p := range e.manifest.Probes {
		s, ok := e.latest[p.ID]
		if !ok {
			continue
		}
		v := model.View(&s, p, e.boot, now, now.Sub(e.online[p.NodeID]) < 20*time.Second)
		sig := signature(v)
		if before := e.lastStates[p.ID]; sig != before {
			reason := v.ReasonCode
			if v.Freshness == "stale" {
				reason = "report_stale"
			}
			changes = append(changes, model.Transition{ID: uuid.NewString(), TargetKind: p.TargetKind, TargetID: p.TargetID, ProbeID: p.ID, At: now.UTC().Truncate(time.Millisecond), FromState: before, ToState: sig, ReasonCode: reason, ObservationRef: s.AttemptID})
		}
	}
	if len(changes) > 0 && e.db != nil {
		if e.db.Commit(ctx, store.Change{Transitions: changes, Now: now}) != nil {
			e.storeState = "state_unavailable"
			return
		}
		for _, t := range changes {
			e.lastStates[t.ProbeID] = t.ToState
		}
		e.apply(nil, changes, nil)
	}
	// Expire history even during quiet periods; minute slots remain holes, not zero.
	cut := now.Add(-24 * time.Hour)
	keep := e.transitions[:0]
	for _, t := range e.transitions {
		if !t.At.Before(cut) {
			keep = append(keep, t)
		}
	}
	e.transitions = keep
	points := e.metrics[:0]
	for _, m := range e.metrics {
		if !m.At.Before(now.Add(-6 * time.Hour)) {
			points = append(points, m)
		}
	}
	e.metrics = points
}
