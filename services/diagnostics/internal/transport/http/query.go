package httptransport

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/access"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/state"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/web"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

type pageSnapshot struct {
	id, session, auth, manifest, boot, filter string
	expires                                   time.Time
	source                                    ingest.Snapshot
	size                                      int
}
type snapshotCache struct {
	mu      sync.Mutex
	entries map[string]pageSnapshot
	secret  string
	bytes   int
}
type cursor struct {
	ID     string `json:"id"`
	Offset int    `json:"offset"`
	Filter string `json:"filter"`
}

func newSnapshotCache() *snapshotCache {
	return &snapshotCache{entries: map[string]pageSnapshot{}, secret: settings.NewSecret()}
}
func (c *snapshotCache) sign(body []byte) []byte {
	h := hmac.New(sha256.New, []byte(c.secret))
	h.Write(body)
	return h.Sum(nil)
}
func (c *snapshotCache) encode(id string, offset int, filter string) string {
	raw, _ := json.Marshal(cursor{id, offset, filter})
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(c.sign(raw))
}
func (c *snapshotCache) decode(value string) (cursor, error) {
	var cur cursor
	if len(value) > 2048 {
		return cur, settings.Error("snapshot_expired")
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return cur, settings.Error("snapshot_expired")
	}
	body, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return cur, settings.Error("snapshot_expired")
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil || !hmac.Equal(sig, c.sign(body)) || settings.Strict(body, &cur) != nil || cur.Offset < 0 {
		return cur, settings.Error("snapshot_expired")
	}
	return cur, nil
}
func (c *snapshotCache) get(id string, g access.Grant, current ingest.Snapshot, filter string) (pageSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.entries[id]
	if !ok || time.Now().After(v.expires) || v.session != g.Hash || v.auth != g.Revision || v.manifest != current.Manifest.Revision || v.boot != current.BootID || filter != "" && filter != v.filter {
		return v, settings.Error("snapshot_expired")
	}
	return v, nil
}
func (c *snapshotCache) add(g access.Grant, source ingest.Snapshot, filter string) (pageSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for id, v := range c.entries {
		if now.After(v.expires) {
			c.bytes -= v.size
			delete(c.entries, id)
		}
	}
	raw, e := json.Marshal(source)
	if e != nil || len(raw) > 32<<20 {
		return pageSnapshot{}, settings.Error("response_limit_exceeded")
	}
	for len(c.entries) >= 8 || c.bytes+len(raw) > 32<<20 {
		oldID := ""
		var oldest time.Time
		for id, v := range c.entries {
			if oldest.IsZero() || v.expires.Before(oldest) {
				oldID, oldest = id, v.expires
			}
		}
		if oldID == "" {
			break
		}
		c.bytes -= c.entries[oldID].size
		delete(c.entries, oldID)
	}
	v := pageSnapshot{id: settings.NewID("snapshot"), session: g.Hash, auth: g.Revision, manifest: source.Manifest.Revision, boot: source.BootID, filter: filter, expires: now.Add(time.Minute), source: source, size: len(raw)}
	c.entries[v.id] = v
	c.bytes += v.size
	return v, nil
}
func filters(path string, q url.Values) string {
	clone := url.Values{}
	for key, values := range q {
		if key != "cursor" && key != "snapshot_id" {
			clone[key] = values
		}
	}
	return settings.Digest([]byte(path + "?" + clone.Encode()))
}
func validateQuery(q url.Values, allowed ...string) error {
	for key, values := range q {
		if !settings.OneOf(key, allowed...) || len(values) != 1 || len(values[0]) > 128 && key != "cursor" {
			return settings.Error("invalid_request")
		}
	}
	return nil
}
func (s *Server) query(w http.ResponseWriter, r *http.Request, id string, g access.Grant) {
	path := strings.TrimPrefix(r.URL.Path, base)
	q := r.URL.Query()
	current := s.Engine.Read()
	source := current
	snapshotID := ""
	expires := time.Now().Add(time.Minute)
	offset := 0
	filter := filters(path, q)
	list := settings.OneOf(path, "/nodes", "/components", "/transitions")
	if list {
		if e := validateListQuery(path, q, current.Now); e != nil {
			s.fail(w, id, e)
			return
		}
	}
	if !list && path != "/metrics" {
		if e := validateQuery(q, "snapshot_id"); e != nil {
			s.fail(w, id, e)
			return
		}
	}
	if value := q.Get("cursor"); value != "" {
		cur, e := s.snapshots.decode(value)
		if e != nil || cur.Filter != filter {
			s.fail(w, id, settings.Error("snapshot_expired"))
			return
		}
		entry, e := s.snapshots.get(cur.ID, g, current, filter)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		source, snapshotID, expires, offset = entry.source, entry.id, entry.expires, cur.Offset
	} else if value := q.Get("snapshot_id"); value != "" {
		entry, e := s.snapshots.get(value, g, current, "")
		if e != nil {
			s.fail(w, id, e)
			return
		}
		source, snapshotID, expires = entry.source, entry.id, entry.expires
	} else if list {
		entry, e := s.snapshots.add(g, current, filter)
		if e != nil {
			s.fail(w, id, e)
			return
		}
		snapshotID, expires = entry.id, entry.expires
	}
	// Preserve ordering/counts for paging while computing age from server time.
	ordered := state.Build(source)
	source.Now = time.Now()
	if snapshotID != "" {
		online := map[string]bool{}
		for _, v := range source.Latest {
			if source.Now.Sub(v.ReceivedAt) < 20*time.Second && source.Online[v.NodeID] {
				online[v.NodeID] = true
			}
		}
		source.Online = online
	}
	views := state.Build(source)
	meta := s.meta(id)
	meta["manifest_revision"] = source.Manifest.Revision
	meta["auth_revision"] = g.Revision
	meta["snapshot_id"] = snapshotID
	meta["diagnostics_state"] = map[string]string{"config": current.ConfigState, "store": current.StoreState, "keyring": current.KeyringState}
	var data any
	switch {
	case path == "/overview":
		summary := map[string]int{"components": len(views.Components), "expected_replicas": 0, "ready_replicas": 0, "failed_replicas": 0, "unknown_replicas": 0, "disabled_replicas": 0}
		for _, c := range views.Components {
			summary["expected_replicas"] += c.ExpectedReplicas
			summary["ready_replicas"] += c.ReadyReplicas
			summary["failed_replicas"] += c.FailedReplicas
			summary["unknown_replicas"] += c.UnknownReplicas
			summary["disabled_replicas"] += c.DisabledReplicas
		}
		online, expectedObservers := 0, 0
		for _, n := range views.Nodes {
			if !n.Enabled || n.Kind == "external" {
				continue
			}
			expectedObservers++
			if n.Observer.Online {
				online++
			}
		}
		recent := transitions(source, q)
		if len(recent) > 20 {
			recent = recent[:20]
		}
		abnormal := []state.ComponentView{}
		for _, c := range views.Components {
			if settings.OneOf(c.DisplayState, "unavailable", "degraded", "unknown") {
				abnormal = append(abnormal, c)
			}
		}
		if len(abnormal) > 20 {
			abnormal = abnormal[:20]
		}
		data = map[string]any{"instance": map[string]string{"id": source.Manifest.InstanceID, "display_name": source.Manifest.DisplayName}, "summary": summary, "coverage": views.Coverage, "observer_summary": map[string]int{"expected": expectedObservers, "online": online}, "resource_warnings": resourceWarnings(source, s.Config.ResourceHints), "recent_transitions": recent, "abnormal_components": abnormal, "min_refresh_interval_ms": 5000}
	case path == "/components":
		items := []any{}
		byID := map[string]state.ComponentView{}
		for _, c := range views.Components {
			byID[c.ID] = c
		}
		for _, c := range ordered.Components {
			if q.Get("state") != "" && c.DisplayState != q.Get("state") || q.Get("kind") != "" && c.Kind != q.Get("kind") {
				continue
			}
			if node := q.Get("node_id"); node != "" {
				found := false
				for _, r := range source.Manifest.Replicas {
					if r.ComponentID == c.ID && r.NodeID == node {
						found = true
					}
				}
				if !found {
					continue
				}
			}
			items = append(items, byID[c.ID])
		}
		data = s.page(items, offset, q, snapshotID, expires, filter)
	case path == "/nodes":
		items := []any{}
		byID := map[string]state.NodeView{}
		for _, n := range views.Nodes {
			byID[n.ID] = n
		}
		for _, n := range ordered.Nodes {
			if q.Get("state") != "" && n.DisplayState != q.Get("state") || q.Get("kind") != "" && n.Kind != q.Get("kind") {
				continue
			}
			items = append(items, byID[n.ID])
		}
		data = s.page(items, offset, q, snapshotID, expires, filter)
	case path == "/transitions":
		items := []any{}
		for _, v := range transitions(source, q) {
			items = append(items, v)
		}
		data = s.page(items, offset, q, snapshotID, expires, filter)
	case strings.HasPrefix(path, "/components/"):
		target := strings.TrimPrefix(path, "/components/")
		for _, c := range views.Components {
			if c.ID == target {
				deps := []state.DependencyView{}
				for _, r := range views.Replicas {
					if r.Replica.ComponentID == target {
						deps = append(deps, r.Dependencies...)
					}
				}
				data = map[string]any{"component": c, "replica_summaries": views.ReplicaSummaries(target), "dependency_summaries": deps, "coverage": c.Coverage}
			}
		}
	case strings.HasPrefix(path, "/replicas/"):
		if v, ok := views.Replicas[strings.TrimPrefix(path, "/replicas/")]; ok {
			data = v
		}
	case strings.HasPrefix(path, "/checks/"):
		if v, ok := views.Checks[strings.TrimPrefix(path, "/checks/")]; ok {
			data = v
		}
	case strings.HasPrefix(path, "/nodes/"):
		target := strings.TrimPrefix(path, "/nodes/")
		for _, n := range views.Nodes {
			if n.ID == target {
				counts := map[string]int{}
				summaries := []state.ReplicaSummary{}
				for _, r := range views.Replicas {
					if r.Replica.NodeID == target {
						counts[r.DisplayState]++
						summaries = append(summaries, state.ReplicaSummary{ID: r.Replica.ID, NodeID: r.Replica.NodeID, Expectation: r.Replica.Expectation, DisplayState: r.DisplayState, Freshness: r.Freshness, Coverage: r.Coverage, ObservedAt: r.ObservedAt})
					}
				}
				sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
				data = map[string]any{"node": n, "observer": n.Observer, "resource_series_summary": n.Metrics, "replica_counts": counts, "replica_summaries": summaries, "coverage": n.Coverage}
			}
		}
	case path == "/metrics":
		var e error
		data, e = metrics(source, q)
		if e != nil {
			s.fail(w, id, e)
			return
		}
	case path == "/self":
		initialized, allowed, rev := s.Access.State()
		gaps := []state.Check{}
		for _, check := range views.Checks {
			if settings.OneOf(check.CurrentObservation.Outcome, "unsupported", "permission_denied") {
				gaps = append(gaps, check)
			}
		}
		sort.Slice(gaps, func(i, j int) bool { return gaps[i].ProbeSummary.ID < gaps[j].ProbeSummary.ID })
		collectors := map[string]any{"enabled": false}
		if s.Collector != nil {
			collectors = s.Collector.Stats()
		}
		data = map[string]any{"build_id": web.BuildID, "protocol_major": 1, "uptime_ms": strconv.FormatInt(time.Since(s.Started).Milliseconds(), 10), "config_state": current.ConfigState, "auth_state": map[string]any{"initialized": initialized, "available": allowed, "revision": rev}, "store_state": map[string]any{"status": current.StoreState, "budget": current.StoreInfo}, "collectors": collectors, "limits": s.Config.Limits, "retention": retention(source), "warnings": []string{current.ConfigState, current.StoreState, current.KeyringState}, "manifest_revision": source.Manifest.Revision, "server_boot_id": source.BootID, "observers": views.Nodes, "diagnostic_gaps": gaps}
	}
	if r.Context().Err() != nil {
		s.fail(w, id, settings.Error("response_limit_exceeded"))
		return
	}
	if e, ok := data.(error); ok {
		s.fail(w, id, e)
		return
	}
	if data == nil {
		s.fail(w, id, settings.Error("target_removed"))
		return
	}
	s.write(w, id, data, &g, meta)
}
func validateListQuery(path string, q url.Values, now time.Time) error {
	fields := []string{"state", "kind", "cursor", "limit"}
	if path == "/components" {
		fields = append(fields, "node_id")
	}
	if path == "/transitions" {
		fields = []string{"target_kind", "target_id", "from", "to", "cursor", "limit"}
	}
	if err := validateQuery(q, fields...); err != nil {
		return err
	}
	if s := q.Get("state"); s != "" && !settings.OneOf(s, "healthy", "degraded", "unavailable", "unknown", "idle", "disabled") {
		return settings.Error("invalid_request")
	}
	kinds := []string{"physical", "vm", "container", "logical_test", "external"}
	if path == "/components" {
		kinds = []string{"application", "worker", "database", "cache", "object_store", "gateway", "tool", "diagnostics"}
	}
	if k := q.Get("kind"); k != "" && !settings.OneOf(k, kinds...) {
		return settings.Error("invalid_request")
	}
	if k := q.Get("target_kind"); k != "" && !settings.OneOf(k, "node", "replica", "endpoint", "dependency") {
		return settings.Error("invalid_request")
	}
	for _, key := range []string{"node_id", "target_id"} {
		if v := q.Get(key); v != "" && !settings.ValidID(v) {
			return settings.Error("invalid_request")
		}
	}
	var from, to time.Time
	for key, dst := range map[string]*time.Time{"from": &from, "to": &to} {
		if v := q.Get(key); v != "" {
			parsed, err := time.Parse(time.RFC3339Nano, v)
			if err != nil || parsed.After(now.Add(time.Minute)) {
				return settings.Error("invalid_request")
			}
			*dst = parsed
		}
	}
	if !from.IsZero() && !to.IsZero() && (to.Before(from) || to.Sub(from) > 24*time.Hour) {
		return settings.Error("invalid_request")
	}
	return nil
}
func (s *Server) page(items []any, offset int, q url.Values, id string, expires time.Time, filter string) any {
	limit := 50
	if raw := q.Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 1 || n > 100 {
			return settings.Error("invalid_request")
		}
		limit = n
	}
	if offset > len(items) {
		return settings.Error("snapshot_expired")
	}
	end := min(offset+limit, len(items))
	next := ""
	if end < len(items) {
		next = s.snapshots.encode(id, end, filter)
	}
	return map[string]any{"items": items[offset:end], "total": len(items), "next_cursor": next, "snapshot_id": id, "expires_at": expires}
}
func transitions(s ingest.Snapshot, q url.Values) []model.Transition {
	out := []model.Transition{}
	from, _ := time.Parse(time.RFC3339Nano, q.Get("from"))
	to, _ := time.Parse(time.RFC3339Nano, q.Get("to"))
	for _, t := range s.Transitions {
		if t.At.Before(s.Now.Add(-24*time.Hour)) || q.Get("target_kind") != "" && t.TargetKind != q.Get("target_kind") || q.Get("target_id") != "" && t.TargetID != q.Get("target_id") || !from.IsZero() && t.At.Before(from) || !to.IsZero() && t.At.After(to) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].ID > out[j].ID
		}
		return out[i].At.After(out[j].At)
	})
	return out
}
func retention(s ingest.Snapshot) map[string]any {
	var actual *time.Time
	for _, t := range s.Transitions {
		if actual == nil || t.At.Before(*actual) {
			at := t.At
			actual = &at
		}
	}
	return map[string]any{"transition_hours": 24, "transition_limit": 10000, "metric_hours": 6, "points_per_series": 360, "metric_limit": 50000, "access_days": 7, "access_limit": 2000, "actual_from": actual, "evicted_count": strconv.FormatUint(s.Meta.EvictedCount, 10)}
}
func metrics(s ingest.Snapshot, q url.Values) (any, error) {
	if e := validateQuery(q, "target_kind", "target_id", "metric_key", "dimension_id", "from", "to"); e != nil {
		return nil, e
	}
	key := q.Get("metric_key")
	if model.MetricUnits[key] == "" || !settings.ValidID(q.Get("target_id")) || !settings.OneOf(q.Get("target_kind"), "node", "replica", "endpoint", "dependency") {
		return nil, settings.Error("invalid_request")
	}
	if strings.HasPrefix(key, "disk.") || strings.HasPrefix(key, "network.") {
		if !settings.ValidID(q.Get("dimension_id")) {
			return nil, settings.Error("invalid_request")
		}
	} else if key == diagnosticreport.TenantQueueKey {
		if !(diagnosticreport.ProcessMetric{Key: key, Dimension: q.Get("dimension_id")}).Valid() {
			return nil, settings.Error("invalid_request")
		}
	} else if q.Get("dimension_id") != "" {
		return nil, settings.Error("invalid_request")
	}
	found := false
	target := q.Get("target_id")
	switch q.Get("target_kind") {
	case "node":
		for _, item := range s.Manifest.Nodes {
			found = found || item.ID == target
		}
	case "replica":
		for _, item := range s.Manifest.Replicas {
			found = found || item.ID == target
		}
	case "endpoint":
		for _, item := range s.Manifest.Endpoints {
			found = found || item.ID == target
		}
	case "dependency":
		for _, item := range s.Manifest.Dependencies {
			found = found || item.ID == target
		}
	}
	if !found {
		return nil, settings.Error("target_removed")
	}
	from, e := time.Parse(time.RFC3339Nano, q.Get("from"))
	to, e2 := time.Parse(time.RFC3339Nano, q.Get("to"))
	if e != nil || e2 != nil || to.Before(from) || to.Sub(from) > 6*time.Hour || to.After(s.Now.Add(time.Minute)) {
		return nil, settings.Error("invalid_request")
	}
	points := map[string]model.MetricPoint{}
	scope, source := "none", "os"
	for _, p := range s.Metrics {
		if p.TargetKind != q.Get("target_kind") || p.TargetID != q.Get("target_id") || p.Key != key || p.DimensionID != q.Get("dimension_id") || p.At.Before(from) || p.At.After(to) {
			continue
		}
		slot := p.At.UTC().Truncate(time.Minute).Format(time.RFC3339)
		if old, ok := points[slot]; !ok || old.At.Before(p.At) {
			points[slot] = p
		}
		scope, source = p.Scope, p.Source
	}
	out := []model.MetricPoint{}
	missing := []time.Time{}
	for t := from.UTC().Truncate(time.Minute); t.Before(to) && len(out)+len(missing) < 360; t = t.Add(time.Minute) {
		if p, ok := points[t.Format(time.RFC3339)]; ok {
			out = append(out, p)
		} else {
			missing = append(missing, t)
		}
	}
	return map[string]any{"unit": model.MetricUnits[key], "scope": scope, "source": source, "points": out, "missing_intervals": missing, "retention": retention(s)}, nil
}
