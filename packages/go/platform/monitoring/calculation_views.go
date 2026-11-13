package monitoring

import (
	"context"
	"math/big"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

func (s *Service) calculationCalendar(ctx context.Context, a access.Actor, scope Scope, channel string, in QueryInput, plan queryPlan, background bool, total ChannelStatistic, resume string, limit int) ([]CalendarStatistic, string, error) {
	out := []CalendarStatistic{}
	loc, _ := time.LoadLocation(in.Timezone)
	query := queryWindow(in)
	first, last := time.Unix(0, -2208988800000000000), time.Unix(0, 1<<63-1)
	if query != nil {
		if query.From != nil {
			first = time.Unix(0, *query.From)
		}
		if query.To != nil {
			last = time.Unix(0, *query.To-1)
		}
	} else {
		if total.Earliest == nil {
			return out, "", nil
		}
		first, _ = time.Parse(time.RFC3339Nano, *total.Earliest)
		last, _ = time.Parse(time.RFC3339Nano, *total.Latest)
	}
	start := calendarStart(first.In(loc), in.Period)
	if resume != "" {
		v, e := time.Parse(time.RFC3339Nano, resume)
		if e != nil {
			return nil, "", invalid("cursor", "无效日历游标。")
		}
		start = v.In(loc)
	}
	for !start.After(last) {
		if len(out) >= limit {
			return out, start.Format(time.RFC3339Nano), nil
		}
		end := calendarNext(start, in.Period)
		full := boundedCalendar(start, end)
		window := full
		if query != nil {
			var ok bool
			window, ok = window.Intersect(*query)
			if !ok {
				start = end
				continue
			}
		}
		part := in
		dto := window.rangeDTO()
		part.ViewRange = &dto
		part.AllHistory = false
		stat, _, e := s.calculationStatistic(ctx, a, scope, channel, part, plan, background)
		if e != nil {
			return nil, "", e
		}
		included := []Range{}
		clipped := []Window{}
		covered := false
		for _, allowed := range plan.windows[channel] {
			if allowed.Covers(window) {
				covered = true
			}
			if v, ok := allowed.Intersect(window); ok {
				clipped = append(clipped, v)
				included = append(included, v.rangeDTO())
			}
		}
		actual := window
		if len(clipped) > 0 {
			actual.From = clipped[0].From
			actual.To = clipped[len(clipped)-1].To
		}
		out = append(out, CalendarStatistic{Included: included, ChannelStatistic: stat, Period: in.Period, Timezone: in.Timezone, Start: start.Format(time.RFC3339Nano), End: end.Format(time.RFC3339Nano), Actual: actual.rangeDTO(), Partial: digest(full) != digest(window) || !covered || start.Before(time.Unix(0, -2208988800000000000)) || end.After(time.Unix(0, 1<<63-1))})
		start = end
	}
	return out, "", nil
}

func (s *Service) calculationTrend(ctx context.Context, a access.Actor, scope Scope, channel string, in QueryInput, plan queryPlan, background bool, total ChannelStatistic, curves int) (TrendSeries, error) {
	out := TrendSeries{Channel: channel, Samples: total.N, Buckets: []TrendBucket{}, Coverage: "complete", Connection: "calculated_points", Resolution: "raw", GapExplanation: "显示正式计算结果；无结果、配置边界和未计算范围均不连接。"}
	w := queryWindow(in)
	if w == nil && total.Earliest != nil {
		first, _ := time.Parse(time.RFC3339Nano, *total.Earliest)
		last, _ := time.Parse(time.RFC3339Nano, *total.Latest)
		lo := first.UnixNano()
		var hi *int64
		if last.UnixNano() < 1<<63-1 {
			v := last.UnixNano() + 1
			hi = &v
		}
		w = &Window{&lo, hi}
	}
	if w != nil {
		out.Range = w.rangeDTO()
	}
	if total.Latest == nil {
		return out, nil
	}
	width := in.PlotWidth
	if width == 0 {
		width = 960
	}
	budget := min(2000, 20000/curves, width*2)
	if in.PlotBudget != 0 {
		budget = min(budget, in.PlotBudget)
	}
	logical, _ := strconv.ParseUint(total.Derived.LogicalOutputs, 10, 64)
	summary := logical > uint64(budget)
	out.Summary = summary
	buckets := []*calculationAccumulator{}
	ranges := []Window{}
	points := []Observation{}
	excluded := false
	var resolution int64 = 1
	firstMinute, lastMinute := int64(0), int64(0)
	if summary {
		out.Connection = "extrema_envelope"
		out.GapExplanation = "摘要保留真实首点、极值和末点；各桶均作为不连续包络，缩放不会改变计算规则。"
		first, _ := time.Parse(time.RFC3339Nano, *total.Earliest)
		last, _ := time.Parse(time.RFC3339Nano, *total.Latest)
		firstMinute, lastMinute = minuteKey(first.UnixNano()), minuteKey(last.UnixNano())
		maxBuckets := max(1, budget/4)
		if budget < 4 {
			return out, invalid("plot_point_budget", "派生摘要至少需要四个点的预算。")
		}
		for lastMinute/resolution-firstMinute/resolution+1 > int64(maxBuckets) {
			resolution *= 2
		}
		firstMinute = floorMultiple(firstMinute, resolution)
		for at := firstMinute; at <= lastMinute; at += resolution {
			lo := (at + minimumMinute) * minuteNS
			var hi *int64
			if at+resolution <= minuteKey(1<<63-1) {
				v := (at + resolution + minimumMinute) * minuteNS
				hi = &v
			}
			part := Window{&lo, hi}
			if w != nil {
				if v, ok := part.Intersect(*w); ok {
					part = v
				}
			}
			ranges = append(ranges, part)
			buckets = append(buckets, newCalculationAccumulator())
		}
		out.Resolution = strconv.FormatInt(resolution, 10) + "m"
	}
	scanBudget := uint64(100_000)
	if background {
		scanBudget = 2_000_000
	}
	coverage, e := s.calculationReader.ScanCalculated(ctx, a, scope, CalculationQuery{Heads: map[string]CalculationHead{channel: plan.vector.Calculated[channel]}, Windows: map[string][]Window{channel: plan.windows[channel]}, Range: w, Qualities: allQualities, History: in.Status == "history"}, scanBudget, func(row CalculatedObservation) error {
		if !slices.Contains(in.Qualities, row.Quality) {
			excluded = true
			return nil
		}
		if !summary {
			points = append(points, *calculatedDTO(&row))
			return nil
		}
		index := (minuteKey(row.AtNS) - firstMinute) / resolution
		if index < 0 || index >= int64(len(buckets)) {
			return revisionChanged()
		}
		return buckets[index].add(row, in.Qualities)
	})
	if e != nil {
		return out, e
	}
	for _, part := range coverage {
		if part.State != "current" {
			out.Coverage = "partial"
		}
	}
	if !summary {
		slices.SortFunc(points, func(a, b Observation) int {
			if a.Calculated.AtNS < b.Calculated.AtNS {
				return -1
			}
			if a.Calculated.AtNS > b.Calculated.AtNS {
				return 1
			}
			return 0
		})
		discontinuous := excluded || out.Coverage != "complete" || len(plan.windows[channel]) > 1
		config := ""
		for _, p := range points {
			if p.Calculated.Value == nil || config != "" && config != p.Calculated.Config {
				discontinuous = true
			}
			config = p.Calculated.Config
		}
		out.Buckets = append(out.Buckets, TrendBucket{Range: out.Range, Count: total.N, Points: points, States: []EnumStatistic{}, Discontinuous: discontinuous})
		return out, nil
	}
	for i, acc := range buckets {
		selected := []*CalculatedObservation{}
		seen := map[string]bool{}
		for _, p := range []*CalculatedObservation{acc.earliest, acc.minimum, acc.maximum, acc.latest} {
			if p != nil && !seen[p.Result] {
				seen[p.Result] = true
				selected = append(selected, p)
			}
		}
		slices.SortFunc(selected, func(a, b *CalculatedObservation) int { return big.NewInt(a.AtNS).Cmp(big.NewInt(b.AtNS)) })
		chosen := []Observation{}
		for _, p := range selected {
			chosen = append(chosen, *calculatedDTO(p))
		}
		out.Buckets = append(out.Buckets, TrendBucket{Range: ranges[i].rangeDTO(), Count: decimalCount(acc.moments.N), Points: chosen, States: []EnumStatistic{}, Discontinuous: true})
	}
	return out, nil
}
