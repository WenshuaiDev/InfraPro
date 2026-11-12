package monitoring

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

type Overview struct {
	Ever      string            `json:"ever_published_count"`
	Active    string            `json:"active_count"`
	Samples   string            `json:"sample_count"`
	Qualities map[string]string `json:"quality_counts"`
	Earliest  *string           `json:"earliest"`
	Latest    *string           `json:"latest"`
}
type EnumStatistic struct {
	Value      string `json:"value"`
	Count      string `json:"count"`
	Proportion string `json:"sample_proportion"`
	First      string `json:"first_at"`
	Last       string `json:"last_at"`
}
type ChannelStatistic struct {
	N        string `json:"n"`
	Scale    int    `json:"scale"`
	Rounding string `json:"rounding"`
	Channel  string `json:"channel_id"`
	Overview
	First    *Observation    `json:"first"`
	Last     *Observation    `json:"last"`
	Min      *Observation    `json:"minimum"`
	Max      *Observation    `json:"maximum"`
	MinCount string          `json:"minimum_ties"`
	MaxCount string          `json:"maximum_ties"`
	Mean     *string         `json:"mean"`
	Stddev   *string         `json:"population_stddev"`
	States   []EnumStatistic `json:"states"`
}
type CalendarStatistic struct {
	Included []Range `json:"included_ranges"`
	ChannelStatistic
	Period   string `json:"period"`
	Timezone string `json:"timezone"`
	Start    string `json:"calendar_start"`
	End      string `json:"calendar_end"`
	Actual   Range  `json:"actual_range"`
	Partial  bool   `json:"partial"`
}
type AnalyticResult struct {
	State       string              `json:"state"`
	Snapshot    Snapshot            `json:"snapshot"`
	JobID       string              `json:"job_id,omitempty"`
	Overview    *Overview           `json:"overview,omitempty"`
	Channels    []ChannelStatistic  `json:"channels,omitempty"`
	Groups      []CalendarStatistic `json:"groups,omitempty"`
	Trends      []TrendSeries       `json:"trends,omitempty"`
	HasMore     bool                `json:"has_more"`
	NextCursor  string              `json:"next_cursor,omitempty"`
	Computation string              `json:"computation"`
}

func overviewOf(v summaryTotals, in QueryInput) Overview {
	active := v.samples(allQualities, false)
	samples := v.samples(in.Qualities, false)
	out := Overview{Ever: decimalCount(v.Ever), Active: decimalCount(active.N), Samples: decimalCount(samples.N), Qualities: map[string]string{}}
	for _, q := range allQualities {
		out.Qualities[q] = decimalCount(v.Active[q].N)
	}
	if active.First != nil {
		out.Earliest = nsText(&active.First.At)
		out.Latest = nsText(&active.Last.At)
	}
	return out
}
func statisticOf(channel string, v summaryTotals, in QueryInput) (ChannelStatistic, error) {
	s := v.samples(in.Qualities, in.Status == "history")
	out := ChannelStatistic{N: decimalCount(s.N), Scale: queryDigits(in), Rounding: "half_even", Channel: channel, Overview: overviewOf(v, in), First: s.First.dto(channel), Last: s.Last.dto(channel), Min: s.Min.dto(channel), Max: s.Max.dto(channel), MinCount: decimalCount(s.MinCount), MaxCount: decimalCount(s.MaxCount), States: []EnumStatistic{}}
	if len(s.States) == 0 {
		var e error
		out.Mean, out.Stddev, e = s.moments(queryDigits(in))
		return out, e
	}
	keys := make([]string, 0, len(s.States))
	for k := range s.States {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, key := range keys {
		state := s.States[key]
		ratio := new(big.Rat).SetFrac(new(big.Int).SetUint64(state.N), new(big.Int).SetUint64(s.N))
		out.States = append(out.States, EnumStatistic{key, decimalCount(state.N), ratio.FloatString(12), *nsText(&state.First.At), *nsText(&state.Last.At)})
	}
	return out, nil
}
func queryDigits(in QueryInput) int {
	if in.Digits == nil {
		return 12
	}
	return *in.Digits
}
func analyticInput(in QueryInput, action string) (QueryInput, error) {
	n, e := normalizedQuery(in)
	if e != nil {
		return n, e
	}
	if !slices.Contains([]string{"overview", "latest", "statistics", "trends"}, action) {
		return n, notFound()
	}
	if queryDigits(n) < 0 || queryDigits(n) > 18 {
		return n, invalid("digits", "统计精度应为 0–18 位。")
	}
	if n.Period != "" && (!slices.Contains([]string{"day", "month", "year"}, n.Period) || action != "statistics") {
		return n, invalid("period", "分组只支持日、月、年统计。")
	}
	if n.PlotWidth != 0 && (n.PlotWidth < 160 || n.PlotWidth > 4096) {
		return n, invalid("plot_width_px", "绘图区宽度应为 160–4096 像素。")
	}
	return n, nil
}
func analyticKey(action string, in QueryInput) string {
	in.Snapshot = ""
	return digest([]any{action, in})
}
func (s *Service) Analytics(ctx context.Context, a access.Actor, scope Scope, action string, input QueryInput) (AnalyticResult, error) {
	in, e := analyticInput(input, action)
	if e != nil {
		return AnalyticResult{}, e
	}
	plan, e := s.queryPlan(ctx, a, scope, in, action, "read")
	if e != nil {
		return AnalyticResult{}, e
	}
	in.Snapshot = plan.snapshot.ID
	if cached, ok, e := s.cachedAnalytic(ctx, a, scope, action, in, plan); e != nil {
		return AnalyticResult{}, e
	} else if ok {
		return cached, nil
	}
	out, e := s.computeAnalytic(ctx, scope.Tenant, action, in, plan, false)
	if errors.Is(e, errSummaryPending) || errors.Is(e, errQueryBudget) {
		return s.queueAnalytic(ctx, a, scope, action, in, plan)
	}
	if e != nil {
		return AnalyticResult{}, storageError(e)
	}
	if _, e = s.queryPlan(ctx, a, scope, in, action, "read"); e != nil {
		return AnalyticResult{}, e
	}
	return out, nil
}

type analyticCursor struct{ Key, Channel, Start string }

func encodeAnalyticCursor(plan queryPlan, action string, in QueryInput, c analyticCursor) string {
	in.Cursor = ""
	c.Key = digest([]any{plan.snapshot.ID, analyticKey(action, in)})
	raw := encodeJSON(c)
	h := hmac.New(sha256.New, []byte(plan.vector.Secret))
	h.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func decodeAnalyticCursor(plan queryPlan, action string, in QueryInput) (analyticCursor, error) {
	c := analyticCursor{}
	if in.Cursor == "" {
		return c, nil
	}
	parts := strings.Split(in.Cursor, ".")
	if len(parts) != 2 || len(in.Cursor) > 2048 {
		return c, invalid("cursor", "无效统计游标。")
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return c, invalid("cursor", "无效统计游标。")
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return c, invalid("cursor", "无效统计游标。")
	}
	h := hmac.New(sha256.New, []byte(plan.vector.Secret))
	h.Write(raw)
	in.Cursor = ""
	if !hmac.Equal(sig, h.Sum(nil)) || json.Unmarshal(raw, &c) != nil || c.Key != digest([]any{plan.snapshot.ID, analyticKey(action, in)}) {
		return c, invalid("cursor", "统计游标与快照或条件不匹配。")
	}
	return c, nil
}
func (s *Service) computeAnalytic(ctx context.Context, tenant, action string, in QueryInput, plan queryPlan, background bool) (AnalyticResult, error) {
	out := AnalyticResult{State: "ready", Snapshot: plan.snapshot, Computation: "immutable-minute-tree-v1"}
	r := summaryReader{service: s, plan: plan, tenant: tenant, budget: 2048, rawBudget: 100000, nodes: map[string]*summaryNode{}}
	if background {
		r.budget = 0
		r.rawBudget = 0
	}
	cursor, e := decodeAnalyticCursor(plan, action, in)
	if e != nil {
		return out, e
	}
	total := summaryTotals{}
	processed := 0
	for _, channel := range plan.channels {
		if len(plan.windows[channel]) == 0 {
			continue
		}
		if cursor.Channel != "" && channel < cursor.Channel {
			continue
		}
		if action != "overview" && action != "trends" && in.Period == "" && processed >= in.Limit {
			out.HasMore = true
			out.NextCursor = encodeAnalyticCursor(plan, action, in, analyticCursor{Channel: channel})
			break
		}
		if !background && processed >= 2048 {
			return out, errQueryBudget
		}
		processed++
		root, e := s.readSummaryRoot(ctx, tenant, channel, plan.vector.Contents[channel])
		if e != nil {
			return out, e
		}
		v, e := r.collect(ctx, channel, root.Root, queryWindow(in))
		if e != nil {
			return out, e
		}
		switch action {
		case "overview":
			total.merge(v)
		case "latest", "statistics":
			if in.Period != "" {
				groups, next, e := r.calendar(ctx, channel, root.Root, v, in, cursor.Start, in.Limit-len(out.Groups))
				if e != nil {
					return out, e
				}
				out.Groups = append(out.Groups, groups...)
				if next != "" {
					out.HasMore = true
					out.NextCursor = encodeAnalyticCursor(plan, action, in, analyticCursor{Channel: channel, Start: next})
					return out, nil
				}
				cursor.Start = ""
			} else {
				stat, e := statisticOf(channel, v, in)
				if e != nil {
					return out, e
				}
				out.Channels = append(out.Channels, stat)
			}
		case "trends":
			if len(plan.channels) > 50 {
				return out, invalid("selection_id", "单图最多比较 50 个通道，请分组显示。")
			}
			series, e := r.trend(ctx, channel, root.Root, v, in, max(1, len(plan.channels)))
			if e != nil {
				return out, e
			}
			out.Trends = append(out.Trends, series)
		}
	}
	if action == "overview" {
		v := overviewOf(total, in)
		out.Overview = &v
	}
	return out, nil
}
func calendarStart(t time.Time, period string) time.Time {
	y, m, d := t.Date()
	if period == "year" {
		m = time.January
		d = 1
	} else if period == "month" {
		d = 1
	}
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
func calendarNext(t time.Time, period string) time.Time {
	switch period {
	case "year":
		return t.AddDate(1, 0, 0)
	case "month":
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 0, 1)
	}
}
func boundedCalendar(start, end time.Time) Window {
	var lo, hi *int64
	domainMin := time.Unix(0, -2208988800000000000)
	domainMax := time.Unix(0, 1<<63-1)
	lower := int64(-2208988800000000000)
	if start.After(domainMin) {
		lower = start.UnixNano()
	}
	lo = &lower
	if !end.After(domainMax) {
		v := end.UnixNano()
		hi = &v
	}
	return Window{lo, hi}
}
func (r *summaryReader) calendar(ctx context.Context, channel string, ref summaryRef, total summaryTotals, in QueryInput, resume string, limit int) ([]CalendarStatistic, string, error) {
	out := []CalendarStatistic{}
	loc, _ := time.LoadLocation(in.Timezone)
	query := queryWindow(in)
	var first, last time.Time
	if query != nil {
		first = time.Unix(0, -2208988800000000000)
		last = time.Unix(0, 1<<63-1)
		if query.From != nil {
			first = time.Unix(0, *query.From)
		}
		if query.To != nil {
			last = time.Unix(0, *query.To).Add(-time.Nanosecond)
		}
	} else {
		cover := total.samples(allQualities, in.Status == "history")
		if cover.First == nil {
			return out, "", nil
		}
		first = time.Unix(0, cover.First.At)
		last = time.Unix(0, cover.Last.At)
	}
	start := calendarStart(first.In(loc), in.Period)
	if resume != "" {
		v, e := time.Parse(time.RFC3339Nano, resume)
		if e != nil {
			return out, "", invalid("cursor", "无效日历游标。")
		}
		start = v.In(loc)
	}
	for !start.After(last) {
		if len(out) >= limit {
			return out, start.Format(time.RFC3339Nano), nil
		}
		end := calendarNext(start, in.Period)
		window := boundedCalendar(start, end)
		if query != nil {
			var ok bool
			window, ok = window.Intersect(*query)
			if !ok {
				start = end
				continue
			}
		}
		v, e := r.collect(ctx, channel, ref, &window)
		if e != nil {
			return out, "", e
		}
		stat, e := statisticOf(channel, v, in)
		if e != nil {
			return out, "", e
		}
		partial := !boundedCalendar(start, end).Covers(window) || digest(boundedCalendar(start, end)) != digest(window)
		// A calendar interval can also be clipped by data authorization. Preserve
		// the exact disjoint allowed intervals in its sample calculation.
		full := false
		for _, w := range r.plan.windows[channel] {
			if w.Covers(window) {
				full = true
				break
			}
		}
		partial = partial || !full
		included := []Range{}
		clipped := []Window{}
		for _, allowed := range r.plan.windows[channel] {
			if x, ok := allowed.Intersect(window); ok {
				clipped = append(clipped, x)
				included = append(included, x.rangeDTO())
			}
		}
		actual := window
		if len(clipped) > 0 {
			actual.From = clipped[0].From
			actual.To = clipped[len(clipped)-1].To
		}
		out = append(out, CalendarStatistic{Included: included, ChannelStatistic: stat, Period: in.Period, Timezone: in.Timezone, Start: start.Format(time.RFC3339Nano), End: end.Format(time.RFC3339Nano), Actual: actual.rangeDTO(), Partial: partial || start.Before(time.Unix(0, -2208988800000000000)) || end.After(time.Unix(0, 1<<63-1))})
		start = end
	}
	return out, "", nil
}

// String keeps storage and programming errors out of the public API; precise
// diagnosis is retained in worker/test evidence.
func analyticFailure(action string, e error) error { return fmt.Errorf("%s summary: %w", action, e) }
