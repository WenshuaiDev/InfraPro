package compute

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

type TimeRange struct{ From, To int64 }

func validNS(text string) (int64, error) {
	n, e := strconv.ParseInt(text, 10, 64)
	if e != nil || strconv.FormatInt(n, 10) != text {
		return 0, fmt.Errorf("canonical nanoseconds required")
	}
	return n, monitoringvalue.CheckTime(time.Unix(0, n))
}
func addNS(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}
func subtractSaturated(a, b int64) int64 {
	v, ok := addNS(a, -b)
	if !ok {
		return math.MinInt64
	}
	return v
}
func ns(s string) int64 { v, _ := strconv.ParseInt(s, 10, 64); return v }
func (p *Plan) window(n *Node, path string) error {
	w := n.Params.Window
	if w == nil {
		return issue("invalid_window", path+"/params/window", "explicit window required")
	}
	bad := func(message string) error { return issue("invalid_window", path+"/params/window", message) }
	switch w.Kind {
	case "trailing_duration":
		if _, e := positive(w.DurationNS); e != nil {
			return bad("positive duration required")
		}
		if w.Count != "" || w.AnchorNS != "" || w.Timezone != "" || w.Period != "" || w.MaxSpanNS != "" {
			return bad("unexpected trailing_duration fields")
		}
	case "trailing_count":
		count, e := positive(w.Count)
		if e != nil || count > int64(p.limits.WindowSamples) {
			return issue("plan_limit_exceeded", path+"/params/window/count", "sample count outside window budget")
		}
		if _, e := positive(w.MaxSpanNS); e != nil {
			return bad("finite max_span required")
		}
		if w.DurationNS != "" || w.AnchorNS != "" || w.Timezone != "" || w.Period != "" {
			return bad("unexpected trailing_count fields")
		}
	case "fixed_duration":
		if _, e := positive(w.DurationNS); e != nil {
			return bad("positive duration required")
		}
		if _, e := validNS(w.AnchorNS); e != nil {
			return bad(e.Error())
		}
		if w.Count != "" || w.Timezone != "" || w.Period != "" || w.MaxSpanNS != "" {
			return bad("unexpected fixed_duration fields")
		}
		if p.Rule.Clock.Kind != "fixed_grid" || ns(p.Rule.Clock.StepNS) != ns(w.DurationNS) || ns(p.Rule.Clock.AnchorNS) != ns(w.AnchorNS) {
			return bad("fixed window requires an identical anchor/step clock")
		}
	case "calendar":
		if !calendarValid(w.Timezone, w.Period) || w.DurationNS != "" || w.Count != "" || w.AnchorNS != "" || w.MaxSpanNS != "" {
			return bad("calendar requires only IANA zone and period")
		}
		if p.Rule.Clock.Kind != "calendar" || p.Rule.Clock.Timezone != w.Timezone || p.Rule.Clock.Period != w.Period {
			return bad("calendar window must match its clock")
		}
	case "whole_range":
		if p.Mode != "analysis" || p.Rule.Clock.Kind != "whole_range" {
			return issue("noncausal_rule", path+"/params/window", "whole_range requires analysis whole_range clock")
		}
		if w.DurationNS != "" || w.Count != "" || w.AnchorNS != "" || w.Timezone != "" || w.Period != "" || w.MaxSpanNS != "" {
			return bad("whole_range uses the explicit run source range")
		}
	default:
		return bad("unsupported window kind")
	}
	if n.Params.MinSamples == "" {
		n.Params.MinSamples = "1"
		if n.Op == "window_count" {
			n.Params.MinSamples = "0"
		}
		if n.Op == "window_variance" || n.Op == "window_stddev" {
			n.Params.MinSamples = "2"
		}
		if w.Kind == "trailing_count" {
			n.Params.MinSamples = w.Count
		}
	}
	minimum, e := nonnegative(n.Params.MinSamples)
	if e != nil || n.Op == "window_count" && minimum != 0 && w.Kind != "trailing_count" || n.Op != "window_count" && minimum == 0 {
		return bad("invalid min_samples")
	}
	if w.Kind == "trailing_count" && minimum != ns(w.Count) {
		return bad("sample window requires exactly N accepted samples")
	}
	for _, value := range []string{n.Params.MaxGapNS, n.Params.MaxAgeNS} {
		if value != "" || p.Mode == "channel" {
			if _, e := positive(value); e != nil {
				return bad("finite max_gap and max_age required for sustained windows")
			}
		}
	}
	if n.Op == "window_quantile" {
		r, e := monitoringvalue.ParseDecimal(n.Params.Percentile)
		if e != nil || r.Sign() < 0 || r.Cmp(big.NewRat(100, 1)) > 0 {
			return bad("percentile must be 0–100")
		}
		n.Params.Percentile, _ = monitoringvalue.CanonicalDecimal(n.Params.Percentile)
	}
	return nil
}
func (p *Plan) baseline(n *Node, path string) error {
	b := n.Params.Baseline
	bad := func(s string) error { return issue("invalid_rule", path+"/params/baseline", s) }
	if b == nil {
		return bad("explicit fixed baseline required")
	}
	switch b.Kind {
	case "constant":
		if _, e := monitoringvalue.ParseDecimal(b.Value); e != nil {
			return bad(e.Error())
		}
		if b.Quantity == nil || *b.Quantity != p.Quantities[n.Inputs[0]] {
			return issue("unit_mismatch", path+"/params/baseline/quantity_spec", "baseline quantity must match input")
		}
		if b.Reference != "" || b.Method != "" || b.FromNS != "" || b.ToNS != "" {
			return bad("constant baseline has no observation/range")
		}
		b.Value, _ = monitoringvalue.CanonicalDecimal(b.Value)
	case "observation":
		if !keyPattern.MatchString(b.Reference) || b.Value != "" || b.Quantity != nil || b.Method != "" || b.FromNS != "" || b.ToNS != "" {
			return bad("observation baseline requires a binding reference")
		}
	case "reference_window":
		if !keyPattern.MatchString(b.Reference) || b.Value != "" || b.Quantity != nil {
			return bad("reference window requires a binding reference")
		}
		from, e := validNS(b.FromNS)
		to, e2 := validNS(b.ToNS)
		if e != nil || e2 != nil || from >= to {
			return bad("valid fixed baseline range required")
		}
		switch b.Method {
		case "mean", "median", "min", "max":
		default:
			return bad("unsupported reference statistic")
		}
	default:
		return bad("unknown baseline kind")
	}
	return nil
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
func calendarNext(t time.Time, period string, delta int) time.Time {
	switch period {
	case "day":
		return t.AddDate(0, 0, delta)
	case "month":
		return t.AddDate(0, delta, 0)
	default:
		return t.AddDate(delta, 0, 0)
	}
}

// GridTimes uses exact integer arithmetic for anchors before the epoch and
// validates each complete calendar endpoint. It never clips a window end.
func NextClockTime(clock Clock, from int64) (*int64, error) {
	switch clock.Kind {
	case "fixed_grid":
		step, e := positive(clock.StepNS)
		if e != nil {
			return nil, e
		}
		anchor, e := validNS(clock.AnchorNS)
		if e != nil {
			return nil, e
		}
		delta := new(big.Int).Sub(big.NewInt(from), big.NewInt(anchor))
		k, rem := new(big.Int), new(big.Int)
		k.DivMod(delta, big.NewInt(step), rem)
		if rem.Sign() != 0 {
			k.Add(k, big.NewInt(1))
		}
		value := new(big.Int).Add(big.NewInt(anchor), new(big.Int).Mul(k, big.NewInt(step)))
		if !value.IsInt64() {
			return nil, nil
		}
		at := value.Int64()
		if e = monitoringvalue.CheckTime(time.Unix(0, at)); e != nil {
			return nil, e
		}
		return &at, nil
	case "calendar":
		if !calendarValid(clock.Timezone, clock.Period) {
			return nil, fmt.Errorf("invalid_calendar")
		}
		loc, _ := time.LoadLocation(clock.Timezone)
		start := time.Unix(0, from).In(loc)
		at := calendarStart(start, clock.Period)
		if at.Before(start) {
			at = calendarNext(at, clock.Period, 1)
		}
		if e := monitoringvalue.CheckTime(at); e != nil {
			return nil, nil
		}
		value := at.UnixNano()
		return &value, nil
	default:
		return nil, fmt.Errorf("not_a_grid_clock")
	}
}

func GridTimes(clock Clock, r TimeRange, limit int) ([]int64, error) {
	if r.From >= r.To {
		return nil, fmt.Errorf("invalid_output_range")
	}
	out := []int64{}
	emit := func(at int64) error {
		if at >= r.From && at < r.To {
			if len(out) >= limit {
				return fmt.Errorf("output_budget_exceeded")
			}
			if err := monitoringvalue.CheckTime(time.Unix(0, at)); err != nil {
				return err
			}
			out = append(out, at)
		}
		return nil
	}
	switch clock.Kind {
	case "fixed_grid":
		step, e := positive(clock.StepNS)
		if e != nil {
			return nil, e
		}
		anchor, e := validNS(clock.AnchorNS)
		if e != nil {
			return nil, e
		}
		delta := new(big.Int).Sub(big.NewInt(r.From), big.NewInt(anchor))
		k, rem := new(big.Int), new(big.Int)
		k.DivMod(delta, big.NewInt(step), rem)
		if rem.Sign() != 0 {
			k.Add(k, big.NewInt(1))
		}
		v := new(big.Int).Add(big.NewInt(anchor), new(big.Int).Mul(k, big.NewInt(step)))
		for v.IsInt64() && v.Int64() < r.To {
			if err := emit(v.Int64()); err != nil {
				return nil, err
			}
			v.Add(v, big.NewInt(step))
		}
	case "calendar":
		if !calendarValid(clock.Timezone, clock.Period) {
			return nil, fmt.Errorf("invalid_calendar")
		}
		loc, _ := time.LoadLocation(clock.Timezone)
		at := calendarStart(time.Unix(0, r.From).In(loc), clock.Period)
		if at.Before(time.Unix(0, r.From)) {
			at = calendarNext(at, clock.Period, 1)
		}
		for at.Before(time.Unix(0, r.To)) {
			if err := monitoringvalue.CheckTime(at); err != nil {
				return nil, err
			}
			if err := emit(at.UnixNano()); err != nil {
				return nil, err
			}
			at = calendarNext(at, clock.Period, 1)
		}
	default:
		return nil, fmt.Errorf("not_a_grid_clock")
	}
	return out, nil
}
func windowRange(w Window, t int64, source TimeRange) (TimeRange, bool, error) {
	switch w.Kind {
	case "trailing_duration":
		return TimeRange{subtractSaturated(t, ns(w.DurationNS)), t}, true, nil
	case "fixed_duration":
		from, ok := addNS(t, -ns(w.DurationNS))
		if !ok {
			return TimeRange{}, false, fmt.Errorf("time_out_of_range")
		}
		return TimeRange{from, t}, false, nil
	case "calendar":
		loc, _ := time.LoadLocation(w.Timezone)
		end := time.Unix(0, t).In(loc)
		if !end.Equal(calendarStart(end, w.Period)) {
			return TimeRange{}, false, fmt.Errorf("invalid_calendar_boundary")
		}
		start := calendarNext(end, w.Period, -1)
		if e := monitoringvalue.CheckTime(start); e != nil {
			return TimeRange{}, false, e
		}
		return TimeRange{start.UnixNano(), t}, false, nil
	case "whole_range":
		return source, false, nil
	}
	return TimeRange{}, false, fmt.Errorf("invalid_window")
}
