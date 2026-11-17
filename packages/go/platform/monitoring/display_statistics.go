package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
)

func (s *Service) DisplayExecute(ctx context.Context, p dr.Plan, raw json.RawMessage) (dr.Materialized, error) {
	var work displayWork
	if e := json.Unmarshal(raw, &work); e != nil {
		return dr.Materialized{}, e
	}
	out := dr.Materialized{Rows: []json.RawMessage{}, Sections: map[string][]json.RawMessage{}, Completeness: dr.Completeness{Scope: "complete", Data: "complete", Reasons: []string{}}}
	if work.ScopeLimited {
		out.Completeness.Scope = "limited"
	}
	if !work.Empty {
		actor := p.ReadSubject().ReadActor()
		scope := displayScope(p)
		deadline := time.Now().Add(3 * time.Second)
		for {
			if work.Action == "details" {
				page, e := s.Details(ctx, actor, scope, work.Input)
				if e != nil {
					return out, e
				}
				for _, v := range page.Items {
					if work.Basis == "derived_samples" && (v.Calculated == nil || v.Calculated.Status != "valid" || v.Calculated.Value == nil) {
						continue
					}
					work.Rows = append(work.Rows, displayValue(v, p.Scopes["target"].FieldGroups))
				}
				work.Coverage = append(work.Coverage, page.Coverage...)
				for _, v := range page.Coverage {
					if len(v.Proof) > 0 {
						work.Proofs = append(work.Proofs, v.Proof)
					}
				}
				if !page.HasMore {
					break
				}
				work.Input.Cursor = page.NextCursor
			} else {
				result, e := s.Analytics(ctx, actor, scope, work.Action, work.Input)
				if e != nil {
					return out, e
				}
				if result.State == "pending" {
					return dr.Materialized{Work: dr.Bytes(work)}, nil
				}
				if result.State != "ready" {
					return out, dr.Problem("query_failed", "监测子查询未能完成。", 409)
				}
				work.Coverage = append(work.Coverage, result.Coverage...)
				for _, v := range result.Coverage {
					if len(v.Proof) > 0 {
						work.Proofs = append(work.Proofs, v.Proof)
					}
				}
				if p.Method == "statistics.monitoring" {
					work.Statistics = append(work.Statistics, result.Channels...)
				} else {
					switch work.Action {
					case "overview":
						out.Single = result.Overview
					case "trends":
						for _, v := range result.Trends {
							work.Rows = append(work.Rows, displayValue(v, p.Scopes["target"].FieldGroups))
						}
					case "latest", "statistics":
						for _, v := range result.Channels {
							work.Rows = append(work.Rows, displayValue(v, p.Scopes["target"].FieldGroups))
						}
						for _, v := range result.Groups {
							work.Rows = append(work.Rows, displayValue(v, p.Scopes["target"].FieldGroups))
						}
					}
				}
				if !result.HasMore {
					break
				}
				work.Input.Cursor = result.NextCursor
			}
			if len(work.Rows) > 2_000_000 || len(dr.Bytes(work.Rows)) > 128<<20 {
				return out, dr.Problem("budget_exceeded", "明细快照超过临时结果预算，请收窄通道或时段。", 429)
			}
			if time.Now().After(deadline) {
				return dr.Materialized{Work: dr.Bytes(work)}, nil
			}
		}
	}
	windows, coverage, partial := displayCoverage(p, work)
	out.Sections["effective_windows"], out.Sections["coverage"] = windows, coverage
	if partial {
		out.State = "partial"
		out.Completeness.Data = "partial"
		out.Completeness.Reasons = []string{"visible_calculations_have_coverage_gaps"}
	}
	if p.Method == "statistics.monitoring" {
		result := displayMonitoringStatistics(p, work)
		out.Rows = result.Rows
		out.Metadata = result.Metadata
		if work.Empty && !partial {
			out.State = "empty"
		}
	} else {
		out.Rows = work.Rows
		out.Metadata = map[string]any{"filters": map[string]any{"range": work.Input.Range, "all_history": work.Input.AllHistory, "qualities": work.Input.Qualities, "timezone": work.Input.Timezone}, "monitoring_snapshot": map[string]string{"raw_watermark": workWatermark(work), "derived_head_digest": dr.Hash(work.Heads)}}
		if work.Empty {
			out.State = "empty"
		}
	}
	// Keep only the fixed selection and its recursive proofs for delivery checks.
	work.Rows = nil
	work.Statistics = nil
	work.Coverage = nil
	work.Input.Cursor = ""
	out.Proof = dr.Bytes(work)
	return out, nil
}

// Values remain canonical strings. Field omission is recursive because trends
// and per-channel statistics embed individual observations at several levels.
func displayValue(value any, fields []string) json.RawMessage {
	raw := dr.Bytes(value)
	if slices.Contains(fields, "monitoring.quality") {
		return raw
	}
	var v any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	_ = decoder.Decode(&v)
	var project func(any)
	project = func(v any) {
		switch item := v.(type) {
		case map[string]any:
			for _, key := range []string{"quality", "quality_reason", "data_quality", "reason_flags"} {
				delete(item, key)
			}
			for _, child := range item {
				project(child)
			}
		case []any:
			for _, child := range item {
				project(child)
			}
		}
	}
	project(v)
	return dr.Bytes(v)
}

func displayCoverage(p dr.Plan, w displayWork) ([]json.RawMessage, []json.RawMessage, bool) {
	type bucket struct {
		kind, state string
		window      dr.Window
		channels    map[string]bool
	}
	windows, coverage := map[string]*bucket{}, map[string]*bucket{}
	kinds := map[string]string{}
	for _, c := range w.Channels {
		kinds[c.ID] = c.SourceKind
	}
	add := func(values map[string]*bucket, channel, kind, state string, r dr.Window) {
		key := kind + state + string(dr.Bytes(r))
		b := values[key]
		if b == nil {
			b = &bucket{kind: kind, state: state, window: r, channels: map[string]bool{}}
			values[key] = b
		}
		b.channels[channel] = true
	}
	for id, parts := range w.Windows {
		for _, part := range parts {
			r := part.rangeDTO()
			rangeValue := dr.Window{From: r.From, To: r.To}
			add(windows, id, kinds[id], "", rangeValue)
			if kinds[id] == "direct" {
				add(coverage, id, "direct", "complete", rangeValue)
			}
		}
	}
	partial := false
	for _, c := range w.Coverage {
		state := c.State
		if state != "current" {
			partial = true
		}
		add(coverage, c.Channel, "derived", state, dr.Window{From: c.Range.From, To: c.Range.To})
	}
	// A selected derived channel without a coverage result is not silently ready.
	for id, kind := range kinds {
		if kind != "derived" {
			continue
		}
		found := false
		for _, c := range w.Coverage {
			if c.Channel == id {
				found = true
				break
			}
		}
		if !found {
			for _, part := range w.Windows[id] {
				r := part.rangeDTO()
				add(coverage, id, kind, "pending", dr.Window{From: r.From, To: r.To})
				partial = true
			}
		}
	}
	serialize := func(values map[string]*bucket, cover bool) []json.RawMessage {
		keys := []string{}
		for key := range values {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		out := []json.RawMessage{}
		for _, key := range keys {
			b := values[key]
			if cover {
				out = append(out, dr.Bytes(dr.CoverageSummary{SourceKind: b.kind, State: b.state, Range: b.window, ChannelCount: strconv.Itoa(len(b.channels)), UpdatedAt: p.Query.MetadataAsOf.Format(time.RFC3339Nano)}))
			} else {
				out = append(out, dr.Bytes(dr.WindowSummary{SourceKind: b.kind, Range: b.window, ChannelCount: strconv.Itoa(len(b.channels))}))
			}
		}
		return out
	}
	return serialize(windows, false), serialize(coverage, true), partial
}
func workWatermark(w displayWork) string { return w.Watermark }
func displayMonitoringStatistics(p dr.Plan, w displayWork) dr.Materialized {
	q := p.Request.(*dr.MonitoringStatisticsRequest)
	out := dr.Materialized{Rows: []json.RawMessage{}}
	stats := map[string]ChannelStatistic{}
	for _, v := range w.Statistics {
		stats[v.Channel] = v
	}
	resources := map[string]dr.ResourceSummary{}
	for _, r := range w.Resources {
		resources[r.ID] = r
	}
	type bucket struct {
		label, visibility string
		ref               *dr.Ref
		channels          []dr.ChannelSummary
	}
	groups := map[string]*bucket{}
	add := func(key, label, visibility string, ref *dr.Ref, c *dr.ChannelSummary) {
		b := groups[key]
		if b == nil {
			b = &bucket{label: label, visibility: visibility, ref: ref}
			groups[key] = b
		}
		if c != nil {
			b.channels = append(b.channels, *c)
		}
	}
	if dr.NewStructureKind(q.GroupBy) {
		for _, r := range resources {
			if r.Visibility == "full" && r.Kind == q.GroupBy {
				ref := dr.Ref{Kind: r.Kind, ID: r.ID}
				add(r.ID, r.Name, "full", &ref, nil)
			}
		}
	} else if q.GroupBy != "none" {
		for _, r := range resources {
			if r.Kind != "facility" || r.Visibility != "full" {
				continue
			}
			if q.GroupBy == "facility" {
				add(r.ID, r.Name, "full", &dr.Ref{Kind: "facility", ID: r.ID}, nil)
			} else if r.Type != nil {
				add(r.Type.DefinitionID, r.Type.Name, "full", &dr.Ref{Kind: "facility_type", ID: r.Type.DefinitionID}, nil)
			}
		}
	}
	for _, c := range w.Channels {
		if q.GroupBy == "none" {
			continue
		}
		if dr.NewStructureKind(q.GroupBy) {
			for _, group := range w.StructureGroups[dr.RefKey(dr.Ref{Kind: "channel", ID: c.ID})] {
				add(group.Key, group.Label, group.Visibility, group.Ref, &c)
			}
			continue
		}
		r, ok := resources[c.Facility.ID]
		if !ok {
			add("restricted_context", "受限工程归属", "restricted", nil, &c)
		} else if q.GroupBy == "facility" {
			add(r.ID, r.Name, "full", &dr.Ref{Kind: "facility", ID: r.ID}, &c)
		} else if r.Type != nil {
			add(r.Type.DefinitionID, r.Type.Name, "full", &dr.Ref{Kind: "facility_type", ID: r.Type.DefinitionID}, &c)
		}
	}
	keys := []string{}
	for key := range groups {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		b := groups[key]
		out.Rows = append(out.Rows, dr.Bytes(dr.StatisticsGroup[dr.MonitoringCounts]{Key: key, Label: b.label, Visibility: b.visibility, Ref: b.ref, Totals: displayCounts(b.channels, stats, q.SourceKinds)}))
	}
	out.Metadata = map[string]any{"definitions_version": "1", "population": "authorized_monitoring", "group_by": q.GroupBy, "additivity": "disjoint", "totals": displayCounts(w.Channels, stats, q.SourceKinds), "requested_range": w.Input.Range, "all_history": w.Input.AllHistory, "qualities": w.Input.Qualities, "timezone": w.Input.Timezone, "source_kinds": q.SourceKinds, "count_basis": map[string]string{"raw": "unique_observation_id", "derived": "fixed_head_logical_output", "metadata": "current_read_snapshot"}, "monitoring_snapshot": map[string]string{"raw_watermark": workWatermark(w), "derived_head_digest": dr.Hash(w.Heads), "route_revision_digest": dr.Hash(w.Route)}}
	if dr.NewStructureKind(q.GroupBy) {
		out.Metadata["membership"] = q.Membership
		out.Metadata["structure_model"] = 2
		out.Metadata["definitions_version"] = "2"
		if q.Membership == "subtree" {
			out.Metadata["additivity"] = "overlapping"
		}
	}
	return out
}
func displayCounts(channels []dr.ChannelSummary, stats map[string]ChannelStatistic, requested []string) dr.MonitoringCounts {
	zero := func() *string { v := "0"; return &v }
	out := dr.MonitoringCounts{ScopeChannels: strconv.Itoa(len(channels)), RawChannels: "0", DerivedChannels: "0", Raw: dr.RawCounts{State: "not_requested"}, Derived: dr.DerivedCounts{State: "not_requested"}}
	if slices.Contains(requested, "direct") {
		out.Raw = dr.RawCounts{State: "ready", Ever: zero(), Active: zero(), Samples: zero(), Qualities: map[string]string{"good": "0", "unmarked": "0", "suspect": "0", "bad": "0"}}
	}
	if slices.Contains(requested, "derived") {
		out.Derived = dr.DerivedCounts{State: "ready", Logical: zero(), Valid: zero(), NoResult: zero(), Samples: zero(), Statuses: map[string]string{}}
	}
	raw, derived := 0, 0
	for _, channel := range channels {
		s := stats[channel.ID]
		if channel.SourceKind == "derived" {
			derived++
			if out.Derived.Logical != nil && s.Derived != nil {
				*out.Derived.Logical = addCount(*out.Derived.Logical, s.Derived.LogicalOutputs)
				*out.Derived.Valid = addCount(*out.Derived.Valid, s.Derived.ValidResults)
				*out.Derived.NoResult = addCount(*out.Derived.NoResult, s.Derived.NoResults)
				// Calculated analytics keeps legacy raw Overview counts at zero.
				// N is its quality-filtered valid numeric result population.
				*out.Derived.Samples = addCount(*out.Derived.Samples, s.N)
				for k, n := range s.Derived.Statuses {
					out.Derived.Statuses[k] = addCount(out.Derived.Statuses[k], n)
				}
				out.Derived.First = earlierTime(out.Derived.First, s.Earliest)
				out.Derived.Last = laterTime(out.Derived.Last, s.Latest)
			}
		} else {
			raw++
			if out.Raw.Ever != nil {
				*out.Raw.Ever = addCount(*out.Raw.Ever, s.Ever)
				*out.Raw.Active = addCount(*out.Raw.Active, s.Active)
				*out.Raw.Samples = addCount(*out.Raw.Samples, s.Samples)
				for k, n := range s.Qualities {
					out.Raw.Qualities[k] = addCount(out.Raw.Qualities[k], n)
				}
				out.Raw.First = earlierTime(out.Raw.First, s.Earliest)
				out.Raw.Last = laterTime(out.Raw.Last, s.Latest)
			}
		}
	}
	out.RawChannels = strconv.Itoa(raw)
	out.DerivedChannels = strconv.Itoa(derived)
	return out
}
