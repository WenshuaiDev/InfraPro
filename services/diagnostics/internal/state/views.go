// Package state derives read-only public views. Bindings and credentials never
// enter this package; target failures are data, not query transport failures.
package state

import (
	"sort"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

type Coverage struct {
	Expected         int                 `json:"expected"`
	Configured       int                 `json:"configured"`
	Attempted        int                 `json:"attempted"`
	Fresh            int                 `json:"fresh"`
	Unavailable      int                 `json:"unavailable"`
	MissingBinding   int                 `json:"missing_binding"`
	Unsupported      int                 `json:"unsupported"`
	PermissionDenied int                 `json:"permission_denied"`
	VantageMismatch  int                 `json:"vantage_mismatch"`
	Complete         bool                `json:"complete"`
	State            string              `json:"state"`
	ProbeIDs         map[string][]string `json:"probe_ids"`
}
type ProbeSummary struct {
	ID           string `json:"id"`
	NodeID       string `json:"node_id"`
	TargetKind   string `json:"target_kind"`
	TargetID     string `json:"target_id"`
	Adapter      string `json:"adapter"`
	Facet        string `json:"facet"`
	Required     bool   `json:"required"`
	IntervalMS   int    `json:"interval_ms"`
	TimeoutMS    int    `json:"timeout_ms"`
	StaleAfterMS int    `json:"stale_after_ms"`
}

func Summary(p settings.Probe) ProbeSummary {
	return ProbeSummary{p.ID, p.NodeID, p.TargetKind, p.TargetID, p.Adapter, p.Facet, p.Required, p.IntervalMS, p.TimeoutMS, p.StaleAfterMS}
}

type Check struct {
	ProbeSummary       ProbeSummary          `json:"probe_summary"`
	CurrentObservation model.ObservationView `json:"current_observation"`
	LastSuccessAt      *time.Time            `json:"last_success_at"`
	Reason             settings.Reason       `json:"reason"`
	RelatedIDs         map[string]string     `json:"related_ids"`
}
type ReplicaView struct {
	Replica      settings.Replica       `json:"replica"`
	DisplayState string                 `json:"display_state"`
	Facets       model.Facets           `json:"facets"`
	Freshness    string                 `json:"freshness"`
	Process      *model.ProcessIdentity `json:"process"`
	Endpoints    []settings.Endpoint    `json:"endpoints"`
	Checks       []Check                `json:"checks"`
	Dependencies []DependencyView       `json:"dependencies"`
	Coverage     Coverage               `json:"coverage"`
	ReasonCodes  []string               `json:"reason_codes"`
	ObservedAt   *time.Time             `json:"observed_at"`
}
type DependencyView struct {
	settings.Dependency
	Check  Check  `json:"check"`
	Impact string `json:"impact"`
}
type ReplicaSummary struct {
	ID           string     `json:"id"`
	NodeID       string     `json:"node_id"`
	Expectation  string     `json:"expectation"`
	DisplayState string     `json:"display_state"`
	Freshness    string     `json:"freshness"`
	Coverage     Coverage   `json:"coverage"`
	ObservedAt   *time.Time `json:"observed_at"`
}
type ComponentView struct {
	settings.Component
	ExpectedReplicas int        `json:"expected_replicas"`
	ReadyReplicas    int        `json:"ready_replicas"`
	FailedReplicas   int        `json:"failed_replicas"`
	UnknownReplicas  int        `json:"unknown_replicas"`
	DisabledReplicas int        `json:"disabled_replicas"`
	DisplayState     string     `json:"display_state"`
	MinReadyMet      bool       `json:"min_ready_met"`
	Coverage         Coverage   `json:"coverage"`
	ReasonCodes      []string   `json:"reason_codes"`
	ObservedAt       *time.Time `json:"observed_at"`
}
type ObserverView struct {
	Online         bool       `json:"online"`
	LastReceivedAt *time.Time `json:"last_received_at"`
	BootID         string     `json:"boot_id"`
	ReasonCode     string     `json:"reason_code"`
}
type NodeView struct {
	settings.Node
	Observer     ObserverView        `json:"observer"`
	DisplayState string              `json:"display_state"`
	Coverage     Coverage            `json:"coverage"`
	Metrics      []model.MetricPoint `json:"metrics"`
	ObservedAt   *time.Time          `json:"observed_at"`
}
type Views struct {
	Snapshot   ingest.Snapshot
	Checks     map[string]Check
	Replicas   map[string]ReplicaView
	Components []ComponentView
	Nodes      []NodeView
	Coverage   Coverage
}

func emptyCoverage() Coverage {
	return Coverage{Complete: true, State: "complete", ProbeIDs: map[string][]string{"expected": {}, "configured": {}, "attempted": {}, "fresh": {}, "unavailable": {}, "missing_binding": {}, "unsupported": {}, "permission_denied": {}, "vantage_mismatch": {}}}
}
func covered(checks []Check) Coverage {
	c := emptyCoverage()
	for _, ch := range checks {
		v := ch.CurrentObservation
		id := ch.ProbeSummary.ID
		c.Expected++
		c.ProbeIDs["expected"] = append(c.ProbeIDs["expected"], id)
		if v.Outcome == "not_configured" {
			c.MissingBinding++
			c.ProbeIDs["missing_binding"] = append(c.ProbeIDs["missing_binding"], id)
		} else {
			c.Configured++
			c.ProbeIDs["configured"] = append(c.ProbeIDs["configured"], id)
		}
		if v.Outcome != "pending" {
			c.Attempted++
			c.ProbeIDs["attempted"] = append(c.ProbeIDs["attempted"], id)
		}
		if v.Freshness == "fresh" {
			c.Fresh++
			c.ProbeIDs["fresh"] = append(c.ProbeIDs["fresh"], id)
		}
		switch v.Outcome {
		case "permission_denied":
			c.PermissionDenied++
			c.ProbeIDs["permission_denied"] = append(c.ProbeIDs["permission_denied"], id)
		case "unsupported":
			c.Unsupported++
			c.ProbeIDs["unsupported"] = append(c.ProbeIDs["unsupported"], id)
		}
		if v.ReasonCode == "vantage_mismatch" || contains(v.Evidence.Codes, "vantage_mismatch") {
			c.VantageMismatch++
			c.ProbeIDs["vantage_mismatch"] = append(c.ProbeIDs["vantage_mismatch"], id)
		}
		if v.Freshness != "fresh" || v.Outcome == "invalid_report" || v.Outcome == "not_configured" || v.Outcome == "permission_denied" || v.Outcome == "unsupported" || v.ReasonCode == "vantage_mismatch" {
			c.Unavailable++
			c.ProbeIDs["unavailable"] = append(c.ProbeIDs["unavailable"], id)
		}
	}
	c.Complete = c.Unavailable == 0
	c.State = "complete"
	if !c.Complete {
		c.State = "partial"
		if c.Expected == c.Unavailable {
			c.State = "none"
		}
	}
	return c
}
func contains(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}
func KnownFailure(v model.ObservationView) bool {
	return v.Freshness == "fresh" && (v.Facets.Presence == "stopped" || v.Facets.Presence == "missing" || v.Facets.Readiness == "not_ready" || v.Facets.Loop == "blocked" || v.Facets.Loop == "stalled")
}
func Ready(v model.ObservationView) bool {
	return v.Freshness == "fresh" && v.Outcome == "ok" && settings.OneOf(v.Facets.Presence, "running", "not_applicable") && settings.OneOf(v.Facets.Readiness, "ready", "not_applicable") && settings.OneOf(v.Facets.Loop, "idle", "working", "not_applicable")
}
func Build(s ingest.Snapshot) Views {
	v := Views{Snapshot: s, Checks: map[string]Check{}, Replicas: map[string]ReplicaView{}, Components: []ComponentView{}, Nodes: []NodeView{}}
	all := []Check{}
	for _, p := range s.Manifest.Probes {
		var stored *model.StoredObservation
		if old, ok := s.Latest[p.ID]; ok {
			stored = &old
		}
		obs := model.View(stored, p, s.BootID, s.Now, s.Online[p.NodeID])
		code := obs.ReasonCode
		switch obs.Freshness {
		case "stale":
			code = "report_stale"
		case "historical":
			code = "historical"
		case "clock_uncertain":
			code = "clock_uncertain"
		}
		related := map[string]string{"node_id": p.NodeID}
		if r, ok := s.Manifest.ReplicaFor(p); ok {
			related["replica_id"] = r.ID
			related["component_id"] = r.ComponentID
		}
		var success *time.Time
		if stored != nil {
			success = stored.LastSuccessAt
		}
		ch := Check{Summary(p), obs, success, settings.Reasons[code], related}
		v.Checks[p.ID] = ch
		all = append(all, ch)
	}
	v.Coverage = covered(all)
	for _, r := range s.Manifest.Replicas {
		rv := ReplicaView{Replica: r, DisplayState: "healthy", Facets: model.NeutralFacets(), Freshness: "fresh", Endpoints: []settings.Endpoint{}, Checks: []Check{}, Dependencies: []DependencyView{}, ReasonCodes: []string{}}
		node, _ := s.Manifest.Node(r.NodeID)
		endIDs := map[string]bool{}
		for _, ep := range s.Manifest.Endpoints {
			if ep.ReplicaID == r.ID {
				rv.Endpoints = append(rv.Endpoints, ep)
				endIDs[ep.ID] = true
			}
		}
		required := map[string]bool{}
		for _, id := range r.RequiredProbeIDs {
			required[id] = true
		}
		failed, unknown, warn, active := false, false, false, false
		for _, p := range s.Manifest.Probes {
			if !(p.TargetKind == "replica" && p.TargetID == r.ID || p.TargetKind == "endpoint" && endIDs[p.TargetID]) {
				continue
			}
			ch := v.Checks[p.ID]
			o := ch.CurrentObservation
			rv.Checks = append(rv.Checks, ch)
			if o.Process != nil {
				rv.Process = o.Process
			}
			if o.ObservedAt != nil && (rv.ObservedAt == nil || o.ObservedAt.After(*rv.ObservedAt)) {
				rv.ObservedAt = o.ObservedAt
			}
			if o.Freshness != "fresh" {
				rv.Freshness = o.Freshness
			}
			if o.Facets.Presence == "running" && o.Freshness == "fresh" {
				active = true
			}
			if o.Facets.Presence != "not_applicable" {
				rv.Facets.Presence = worst(rv.Facets.Presence, o.Facets.Presence, []string{"stopped", "missing", "unknown", "running"})
			}
			if o.Facets.Readiness != "not_applicable" {
				rv.Facets.Readiness = worst(rv.Facets.Readiness, o.Facets.Readiness, []string{"not_ready", "unconfigured", "starting", "unknown", "ready"})
			}
			if o.Facets.Loop != "not_applicable" {
				rv.Facets.Loop = worst(rv.Facets.Loop, o.Facets.Loop, []string{"stalled", "blocked", "unknown", "working", "idle"})
			}
			if required[p.ID] {
				if KnownFailure(o) {
					failed = true
				} else if !Ready(o) {
					unknown = true
				}
			} else if !Ready(o) {
				warn = true
			}
			if !Ready(o) {
				rv.ReasonCodes = appendUnique(rv.ReasonCodes, ch.Reason.Code)
			}
		}
		for _, d := range s.Manifest.Dependencies {
			if d.FromReplicaID != r.ID {
				continue
			}
			ch := v.Checks[d.ProbeID]
			impact := "limited_evidence"
			if !Ready(ch.CurrentObservation) {
				impact = "may_affect_source"
			}
			rv.Dependencies = append(rv.Dependencies, DependencyView{d, ch, impact})
			if d.Required && d.EvidenceMode == "self_report" && KnownFailure(ch.CurrentObservation) {
				failed = true
				rv.Facets.Readiness = "not_ready"
			}
		}
		rv.Coverage = covered(rv.Checks)
		switch {
		case r.Expectation == "disabled" || !node.Enabled:
			rv.DisplayState = "disabled"
		case r.Expectation == "on_demand" && !active:
			rv.DisplayState = "idle"
		case failed:
			rv.DisplayState = "unavailable"
		case unknown || len(required) == 0:
			rv.DisplayState = "unknown"
		case warn || !rv.Coverage.Complete:
			rv.DisplayState = "degraded"
		}
		v.Replicas[r.ID] = rv
	}
	for _, c := range s.Manifest.Components {
		cv := ComponentView{Component: c, DisplayState: "healthy", ReasonCodes: []string{}}
		checks := []Check{}
		for _, r := range s.Manifest.Replicas {
			if r.ComponentID != c.ID {
				continue
			}
			rv := v.Replicas[r.ID]
			checks = append(checks, rv.Checks...)
			for _, code := range rv.ReasonCodes {
				cv.ReasonCodes = appendUnique(cv.ReasonCodes, code)
			}
			if rv.ObservedAt != nil && (cv.ObservedAt == nil || rv.ObservedAt.After(*cv.ObservedAt)) {
				cv.ObservedAt = rv.ObservedAt
			}
			if r.Expectation == "disabled" || rv.DisplayState == "disabled" {
				cv.DisabledReplicas++
				continue
			}
			if r.Expectation != "continuous" {
				continue
			}
			cv.ExpectedReplicas++
			switch rv.DisplayState {
			case "healthy", "degraded":
				cv.ReadyReplicas++
				if rv.DisplayState == "degraded" {
					cv.DisplayState = "degraded"
				}
			case "unavailable":
				cv.FailedReplicas++
			default:
				cv.UnknownReplicas++
			}
		}
		cv.MinReadyMet = cv.ReadyReplicas >= c.MinReadyReplicas
		cv.Coverage = covered(checks)
		switch {
		case c.Expectation == "disabled":
			cv.DisplayState = "disabled"
		case c.Expectation == "on_demand" && cv.ExpectedReplicas == 0:
			cv.DisplayState = "idle"
		case cv.FailedReplicas > 0:
			if cv.ReadyReplicas == 0 {
				cv.DisplayState = "unavailable"
			} else {
				cv.DisplayState = "degraded"
			}
		case cv.UnknownReplicas > 0 || cv.ExpectedReplicas == 0:
			if cv.ReadyReplicas == 0 {
				cv.DisplayState = "unknown"
			} else {
				cv.DisplayState = "degraded"
			}
		case !cv.Coverage.Complete:
			cv.DisplayState = "degraded"
		}
		if settings.OneOf(c.Expectation, "external", "on_demand") && cv.ExpectedReplicas == 0 {
			states := []string{}
			for _, r := range s.Manifest.Replicas {
				if r.ComponentID == c.ID && r.Expectation != "disabled" {
					states = append(states, v.Replicas[r.ID].DisplayState)
				}
			}
			cv.DisplayState = "unknown"
			if len(states) > 0 {
				cv.DisplayState = states[0]
				for _, value := range states[1:] {
					cv.DisplayState = worst(cv.DisplayState, value, []string{"unavailable", "unknown", "degraded", "healthy", "idle", "disabled"})
				}
			}
			if c.Expectation == "on_demand" && len(states) == 0 {
				cv.DisplayState = "idle"
			}
		}
		v.Components = append(v.Components, cv)
	}
	for _, n := range s.Manifest.Nodes {
		nv := NodeView{Node: n, Observer: ObserverView{Online: s.Online[n.ID], ReasonCode: "observer_offline"}, DisplayState: "unknown", Metrics: []model.MetricPoint{}}
		checks := []Check{}
		if nv.Observer.Online {
			nv.Observer.ReasonCode = "ok"
			nv.DisplayState = "healthy"
		}
		if !n.Enabled {
			nv.DisplayState = "disabled"
		}
		for _, p := range s.Manifest.Probes {
			if p.NodeID != n.ID {
				continue
			}
			ch := v.Checks[p.ID]
			checks = append(checks, ch)
			if o, ok := s.Latest[p.ID]; ok {
				if nv.Observer.LastReceivedAt == nil || o.ReceivedAt.After(*nv.Observer.LastReceivedAt) {
					t := o.ReceivedAt
					nv.Observer.LastReceivedAt = &t
					nv.Observer.BootID = o.ObserverBootID
				}
				if p.TargetKind == "node" && p.TargetID == n.ID {
					for _, metric := range ch.CurrentObservation.Metrics {
						at := time.Time{}
						if ch.CurrentObservation.ObservedAt != nil {
							at = *ch.CurrentObservation.ObservedAt
						}
						nv.Metrics = append(nv.Metrics, model.MetricPoint{TargetKind: "node", TargetID: n.ID, NodeID: n.ID, Source: o.Evidence.Source, Scope: o.Evidence.Scope, At: at, Metric: metric})
					}
				}
			}
		}
		nv.ObservedAt = nv.Observer.LastReceivedAt
		nv.Coverage = covered(checks)
		if nv.DisplayState == "healthy" && !nv.Coverage.Complete {
			nv.DisplayState = "degraded"
		}
		v.Nodes = append(v.Nodes, nv)
	}
	sort.Slice(v.Components, func(i, j int) bool {
		return Order(v.Components[i].DisplayState, v.Components[i].ID) < Order(v.Components[j].DisplayState, v.Components[j].ID)
	})
	sort.Slice(v.Nodes, func(i, j int) bool {
		return Order(v.Nodes[i].DisplayState, v.Nodes[i].ID) < Order(v.Nodes[j].DisplayState, v.Nodes[j].ID)
	})
	return v
}
func Order(state, id string) string {
	n := map[string]string{"unavailable": "0", "degraded": "1", "unknown": "2", "healthy": "3", "idle": "4", "disabled": "5"}[state]
	return n + id
}
func appendUnique(v []string, s string) []string {
	if s != "" && !contains(v, s) {
		return append(v, s)
	}
	return v
}
func worst(a, b string, rank []string) string {
	for _, x := range rank {
		if x == a || x == b {
			return x
		}
	}
	return b
}
func (v Views) ReplicaSummaries(component string) []ReplicaSummary {
	out := []ReplicaSummary{}
	for _, r := range v.Replicas {
		if r.Replica.ComponentID == component {
			out = append(out, ReplicaSummary{r.Replica.ID, r.Replica.NodeID, r.Replica.Expectation, r.DisplayState, r.Freshness, r.Coverage, r.ObservedAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(out[i].ID, out[j].ID) < 0 })
	return out
}
