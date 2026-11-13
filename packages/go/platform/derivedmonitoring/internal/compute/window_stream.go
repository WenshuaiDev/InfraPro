package compute

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/exactsort"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

func spillWindow(n Node) bool {
	return strings.HasPrefix(n.Op, "window_") && len(n.Inputs) == 1 && strings.HasPrefix(n.Inputs[0], "input:") && n.Params.Window != nil && (n.Params.Window.Kind == "whole_range" || n.Params.Window.Kind == "fixed_duration" || n.Params.Window.Kind == "calendar" || n.Params.Window.Kind == "trailing_duration")
}

type WindowTask struct {
	Node   Node
	At     int64
	Source TimeRange
}

// Streaming applies to independent direct-input fixed windows, including a
// rule that combines their outputs. Other shapes retain ordinary partitioning
// and the active-window budget; they are never approximated.
func (p *Plan) StreamingWindows(options RunOptions) ([]WindowTask, bool, error) {
	if p.Rule.Clock.Kind == "primary_input" {
		return nil, false, nil
	}
	has := false
	for _, n := range p.Rule.Nodes {
		refs := append([]string{}, n.Inputs...)
		for _, ref := range n.Params.Variables {
			refs = append(refs, ref)
		}
		for _, ref := range refs {
			if strings.HasPrefix(ref, "input:") {
				if !spillWindow(n) {
					return nil, false, nil
				}
				has = true
			}
		}
	}
	if !has {
		return nil, false, nil
	}
	times, e := p.executionGrid(options, 64)
	if e != nil {
		return nil, false, nil
	}
	out := []WindowTask{}
	for _, n := range p.Rule.Nodes {
		if !spillWindow(n) {
			continue
		}
		for _, at := range times {
			out = append(out, WindowTask{n, at, options.SourceRange})
		}
	}
	if len(out) > 128 {
		return nil, false, nil
	}
	return out, true, nil
}

// WindowSummary is the exact, portable checkpoint of an aggregate. Rational
// text is not rounded to the output scale, including intermediate variance.
type WindowSummary struct {
	Count                       uint64 `json:"count,string"`
	Excluded                    uint64 `json:"excluded,string"`
	First, Last                 *int64
	Quality, Config             string
	MixedConfig                 bool
	MaximumGap                  string
	Sum, Squares, Min, Max      string
	MinAt, MaxAt                *int64
	MinTies, MaxTies            uint64
	RankLow, RankHigh, Fraction string
}
type WindowAccumulator struct {
	p                *Plan
	task             WindowTask
	bounds           TimeRange
	inclusive        bool
	state            WindowSummary
	moments          monitoringvalue.Moments
	sorter           *exactsort.Sorter
	minimum, maximum *big.Rat
	previous         *int64
	maximumGap       big.Int
}

func (p *Plan) NewWindowAccumulator(task WindowTask, sortOptions exactsort.Options) (*WindowAccumulator, error) {
	if !spillWindow(task.Node) {
		return nil, fmt.Errorf("invalid_streaming_window")
	}
	bounds, inclusive, e := windowRange(*task.Node.Params.Window, task.At, task.Source)
	if e != nil {
		return nil, e
	}
	a := &WindowAccumulator{p: p, task: task, bounds: bounds, inclusive: inclusive, state: WindowSummary{Quality: "unmarked"}}
	if task.Node.Op == "window_median" || task.Node.Op == "window_quantile" {
		a.sorter, e = exactsort.New(sortOptions)
		if e != nil {
			return nil, e
		}
	}
	return a, nil
}
func (a *WindowAccumulator) Close() error {
	if a.sorter != nil {
		return a.sorter.Close()
	}
	return nil
}
func (a *WindowAccumulator) Add(ctx context.Context, s Sample) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := monitoringvalue.CheckTime(time.Unix(0, s.At)); e != nil {
		return e
	}
	if !knownQuality(s.Quality) || s.Status != "" && s.Status != "valid" && !slices.Contains(statusOrder, s.Status) {
		return fmt.Errorf("invalid_input_status")
	}
	if (s.Status == "" || s.Status == "valid") && s.Value == nil {
		return fmt.Errorf("invalid_input_value")
	}
	if s.At < a.bounds.From || a.inclusive && s.At == a.bounds.From || s.At > a.bounds.To || !a.inclusive && s.At == a.bounds.To {
		return nil
	}
	if a.previous != nil && s.At <= *a.previous {
		return fmt.Errorf("input_not_uniquely_ordered")
	}
	at := s.At
	a.previous = &at
	if !a.p.accepted(s) {
		a.state.Excluded++
		return nil
	}
	if a.task.Node.Params.Window.Kind == "trailing_duration" && a.state.Count >= uint64(a.p.limits.WindowSamples) {
		return fmt.Errorf("window_budget_exceeded")
	}
	r, exact := s.Value.Exact()
	if !exact {
		return fmt.Errorf("external_window_requires_published_decimal")
	}
	if a.state.Last != nil {
		gap := new(big.Int).Sub(big.NewInt(at), big.NewInt(*a.state.Last))
		if gap.Cmp(&a.maximumGap) > 0 {
			a.maximumGap.Set(gap)
		}
	}
	if a.state.Count == 0 {
		a.state.First = &at
		a.state.Quality = "good"
		a.state.Config = s.ConfigID
	}
	a.state.Last = &at
	a.state.Count++
	a.state.Quality = worst(a.state.Quality, s.Quality)
	a.state.MixedConfig = a.state.MixedConfig || a.state.Config != s.ConfigID
	if a.minimum == nil || r.Cmp(a.minimum) < 0 {
		a.minimum = new(big.Rat).Set(r)
		a.state.MinAt = &at
		a.state.MinTies = 1
	} else if r.Cmp(a.minimum) == 0 {
		a.state.MinTies++
	}
	if a.maximum == nil || r.Cmp(a.maximum) > 0 {
		a.maximum = new(big.Rat).Set(r)
		a.state.MaxAt = &at
		a.state.MaxTies = 1
	} else if r.Cmp(a.maximum) == 0 {
		a.state.MaxTies++
	}
	if a.task.Node.Op == "window_sum" || a.task.Node.Op == "window_mean" || a.task.Node.Op == "window_variance" || a.task.Node.Op == "window_stddev" {
		if e := a.moments.Add(r); e != nil {
			return e
		}
	}
	if a.sorter != nil {
		return a.sorter.Add(ctx, r)
	}
	return nil
}
func (a *WindowAccumulator) Summary() WindowSummary {
	v := a.state
	v.Sum = a.moments.Sum.RatString()
	v.Squares = a.moments.Squares.RatString()
	v.MaximumGap = a.maximumGap.String()
	if a.minimum != nil {
		v.Min = a.minimum.RatString()
		v.Max = a.maximum.RatString()
	}
	return v
}
func (a *WindowAccumulator) Finish(ctx context.Context) (WindowSummary, error) {
	v := a.Summary()
	if a.sorter != nil && v.Count > 0 {
		percent := big.NewRat(50, 1)
		if a.task.Node.Op == "window_quantile" {
			percent, _ = monitoringvalue.ParseDecimal(a.task.Node.Params.Percentile)
		}
		rank := new(big.Rat).Mul(new(big.Rat).SetInt(new(big.Int).SetUint64(v.Count-1)), percent)
		rank.Quo(rank, big.NewRat(100, 1))
		low := new(big.Int).Quo(rank.Num(), rank.Denom()).Uint64()
		high := min(low+1, v.Count-1)
		values, e := a.sorter.Ranks(ctx, []uint64{low, high})
		if e != nil {
			return v, e
		}
		v.RankLow = values[low].RatString()
		v.RankHigh = values[high].RatString()
		v.Fraction = new(big.Rat).Sub(rank, new(big.Rat).SetInt(new(big.Int).SetUint64(low))).RatString()
	}
	return v, nil
}
func WindowSample(task WindowTask, v WindowSummary) (Sample, error) {
	bounds, _, e := windowRange(*task.Node.Params.Window, task.At, task.Source)
	if e != nil {
		return Sample{}, e
	}
	n := task.Node
	s := Sample{At: task.At, Status: "valid", Quality: v.Quality, ConfigID: v.Config, Count: v.Count, Excluded: v.Excluded, Interval: &bounds, InputFrom: v.First, InputTo: v.Last}
	if n.Params.Window.Kind == "trailing_duration" {
		// A rolling result is an instant at t; its input halo is not the
		// interval-valued output of a fixed or calendar aggregate.
		s.Interval = nil
	}
	if v.Count == 0 {
		s.Reasons = append(s.Reasons, "empty_window")
	}
	if v.MixedConfig {
		addReason(&s, "configuration_boundary")
	}
	if v.Count > 0 {
		gap, ok := new(big.Int).SetString(v.MaximumGap, 10)
		if !ok {
			return s, fmt.Errorf("invalid_window_checkpoint")
		}
		if n.Params.MaxGapNS != "" && gap.Cmp(big.NewInt(ns(n.Params.MaxGapNS))) > 0 {
			addReason(&s, "gap_exceeded")
		}
		if n.Params.MaxAgeNS != "" && new(big.Int).Sub(big.NewInt(task.At), big.NewInt(*v.Last)).Cmp(big.NewInt(ns(n.Params.MaxAgeNS))) > 0 {
			addReason(&s, "age_exceeded")
		}
	}
	if v.Count < uint64(ns(n.Params.MinSamples)) {
		addReason(&s, "insufficient_samples")
	}
	if s.Status != "valid" {
		return s, nil
	}
	if n.Op == "window_count" {
		s.Value, e = monitoringvalue.Rational(new(big.Rat).SetInt(new(big.Int).SetUint64(v.Count)))
		return s, e
	}
	if v.Count == 0 {
		addReason(&s, "insufficient_samples")
		return s, nil
	}
	parse := func(text string) (*big.Rat, error) {
		r, ok := new(big.Rat).SetString(text)
		if !ok {
			return nil, fmt.Errorf("invalid_window_checkpoint")
		}
		return r, monitoringvalue.Check(r)
	}
	var value *big.Rat
	switch n.Op {
	case "window_min", "window_max", "window_range":
		lo, e := parse(v.Min)
		if e != nil {
			return s, e
		}
		hi, e := parse(v.Max)
		if e != nil {
			return s, e
		}
		value = lo
		if n.Op == "window_max" {
			value = hi
		}
		if n.Op == "window_range" {
			value = new(big.Rat).Sub(hi, lo)
		}
		s.MinAt, s.MaxAt, s.MinTies, s.MaxTies = v.MinAt, v.MaxAt, v.MinTies, v.MaxTies
	case "window_median", "window_quantile":
		lo, e := parse(v.RankLow)
		if e != nil {
			return s, e
		}
		hi, e := parse(v.RankHigh)
		if e != nil {
			return s, e
		}
		fraction, e := parse(v.Fraction)
		if e != nil {
			return s, e
		}
		value = new(big.Rat).Add(lo, new(big.Rat).Mul(new(big.Rat).Sub(hi, lo), fraction))
	default:
		sum, e := parse(v.Sum)
		if e != nil {
			return s, e
		}
		squares, e := parse(v.Squares)
		if e != nil {
			return s, e
		}
		m := monitoringvalue.Moments{N: v.Count, Sum: *sum, Squares: *squares}
		value = sum
		if n.Op == "window_mean" {
			value = m.Mean()
		}
		if n.Op == "window_variance" || n.Op == "window_stddev" {
			value, e = m.Variance()
			if e != nil {
				return s, e
			}
		}
	}
	s.Value, e = monitoringvalue.Rational(value)
	if e == nil && n.Op == "window_stddev" {
		s.Value, e = monitoringvalue.Unary("sqrt", s.Value)
	}
	if err := numericFailure(&s, e); err != nil {
		return s, err
	}
	return s, nil
}
