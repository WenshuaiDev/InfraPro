package compute

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

type Match struct {
	Reference, ID string
	At            int64
	OffsetNS      string
}
type Sample struct {
	MinAt, MaxAt                  *int64
	MinTies, MaxTies              uint64
	At                            int64
	Value                         *monitoringvalue.Number
	Quality, Status, ConfigID, ID string
	Interval                      *TimeRange
	Reasons                       []string
	Count, Excluded               uint64
	InputFrom, InputTo            *int64
	Matches                       []Match
}
type FixedBaseline struct {
	Value         *monitoringvalue.Number
	At            int64
	Valid, Proven bool
	Quality, ID   string
}
type Data struct {
	Inputs    map[string][]Sample
	Baselines map[string]FixedBaseline
	// Windows contains host-computed exact streaming aggregates. It is internal
	// execution data, never a DSL field. Only direct fixed/whole window nodes may
	// use it; all later nodes still execute through the ordinary rule evaluator.
	Windows map[string][]Sample
	// Complete certifies that the Reader supplied every shard and required halo.
	// Missing transport/storage data is a task error, never a missing_input row.
	Complete bool
}
type RunOptions struct {
	OutputRange, SourceRange TimeRange
	NowNS                    int64
	// IncludeMax represents the clipped domain upper bound. It is legal only
	// when OutputRange.To is MaxInt64 and avoids overflowing max+1.
	IncludeMax bool
	// MaxSeriesRows is a host partition budget, including grid-only output and
	// halo series. Zero keeps the compiled limit for standalone calculations.
	MaxSeriesRows int
	// Observe is a bounded host-owned explanation collector. It never changes
	// numeric execution or supplies values back to the rule.
	Observe func(string, []Sample) error
}
type Result struct {
	MinAt, MaxAt       *int64
	MinTies, MaxTies   uint64
	OutputKey          string
	At                 int64
	Interval           *TimeRange
	Value              *string
	Scale              int
	Rounded            bool
	Status, Quality    string
	Reasons            []string
	Count, Excluded    uint64
	InputFrom, InputTo *int64
	Matches            []Match
}

var statusOrder = []string{"baseline_invalid", "noncausal_baseline", "configuration_boundary", "invalid_input", "missing_input", "insufficient_samples", "gap_exceeded", "age_exceeded", "division_by_zero", "numeric_out_of_range", "numeric_underflow", "precision_unresolved"}

func addReason(s *Sample, reasons ...string) {
	for _, r := range reasons {
		if r != "valid" && r != "" && !slices.Contains(s.Reasons, r) {
			s.Reasons = append(s.Reasons, r)
		}
	}
	s.Status = "valid"
	for _, r := range statusOrder {
		if slices.Contains(s.Reasons, r) {
			s.Status = r
			break
		}
	}
	if s.Status != "valid" {
		s.Value = nil
	}
}
func qualityRank(q string) int {
	switch q {
	case "bad":
		return 3
	case "suspect":
		return 2
	case "unmarked":
		return 1
	default:
		return 0
	}
}
func worst(a, b string) string {
	if qualityRank(a) >= qualityRank(b) {
		return a
	}
	return b
}
func (p *Plan) accepted(s Sample) bool {
	return s.Value != nil && (s.Status == "valid" || s.Status == "") && slices.Contains(p.Rule.Policies.Quality, s.Quality)
}
func numericFailure(s *Sample, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, monitoringvalue.ErrBudget) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	for _, status := range []string{"division_by_zero", "numeric_out_of_range", "numeric_underflow", "precision_unresolved"} {
		if err.Error() == status {
			addReason(s, status)
			return nil
		}
	}
	return err
}

// Run evaluates a bounded, complete partition plus its halo. Emitted outputs
// belong only to OutputRange; temporary halo nodes are never published. The
// caller owns durable chunking and must not publish until Run succeeds.
func Run(ctx context.Context, p *Plan, data Data, options RunOptions, emit func(Result) error) error {
	if !data.Complete {
		return fmt.Errorf("input_manifest_incomplete")
	}
	if options.OutputRange.From > options.OutputRange.To || options.OutputRange.From == options.OutputRange.To && !options.IncludeMax || options.IncludeMax && options.OutputRange.To != math.MaxInt64 {
		return fmt.Errorf("invalid_output_range")
	}
	if err := monitoringvalue.CheckTime(time.Unix(0, options.OutputRange.From)); err != nil {
		return err
	}
	if err := monitoringvalue.CheckTime(time.Unix(0, options.OutputRange.To)); err != nil {
		return err
	}
	if p.Rule.Clock.Kind == "whole_range" && (options.SourceRange.From >= options.SourceRange.To || options.SourceRange.To != options.OutputRange.To) {
		return fmt.Errorf("invalid_source_range")
	}
	series := map[string][]Sample{}
	partitionLimit := p.limits.WindowSamples + p.limits.OutputRows
	if options.MaxSeriesRows > 0 {
		partitionLimit = min(partitionLimit, options.MaxSeriesRows)
	}
	for _, input := range p.Rule.Inputs {
		rows, ok := data.Inputs[input.Key]
		if !ok {
			return fmt.Errorf("input_manifest_incomplete: %s", input.Key)
		}
		if len(rows) > partitionLimit {
			return fmt.Errorf("input_partition_budget_exceeded")
		}
		for i, s := range rows {
			if i%256 == 0 {
				if e := ctx.Err(); e != nil {
					return e
				}
			}
			if i > 0 && s.At <= rows[i-1].At {
				return fmt.Errorf("input_not_uniquely_ordered")
			}
			if e := monitoringvalue.CheckTime(time.Unix(0, s.At)); e != nil {
				return e
			}
			if !knownQuality(s.Quality) {
				return fmt.Errorf("invalid_input_quality")
			}
			if input.Time.Kind != "instant" && (s.Interval == nil || s.Interval.To != s.At || s.Interval.From >= s.Interval.To) {
				return fmt.Errorf("invalid_input_interval")
			}
			if s.Status != "" && s.Status != "valid" && !slices.Contains(statusOrder, s.Status) {
				return fmt.Errorf("invalid_input_status")
			}
			if (s.Status == "valid" || s.Status == "") && s.Value == nil {
				return fmt.Errorf("invalid_input_value")
			}
		}
		series["input:"+input.Key] = rows
	}
	grid, gridError := p.executionGrid(options, partitionLimit)
	if gridError != nil {
		return gridError
	}
	for _, n := range p.Rule.Nodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		var rows []Sample
		var err error
		if prepared, ok := data.Windows[n.ID]; ok {
			if !spillWindow(n) {
				return fmt.Errorf("invalid_prepared_window")
			}
			rows = prepared
		} else if strings.HasPrefix(n.Op, "window_") {
			timeline := grid
			if p.Rule.Clock.Kind == "primary_input" {
				timeline = nil
				for _, sample := range series["input:"+p.Rule.Clock.Input] {
					timeline = append(timeline, sample.At)
				}
			}
			rows, err = p.windowSeries(ctx, n, series[n.Inputs[0]], timeline, options)
		} else {
			rows, err = p.nodeSeries(ctx, n, series, data.Baselines, grid)
		}
		if err != nil {
			return fmt.Errorf("node %s: %w", n.ID, err)
		}
		series["node:"+n.ID] = rows
		if options.Observe != nil {
			if e := options.Observe(n.ID, rows); e != nil {
				return e
			}
		}
	}
	times := grid
	if p.Rule.Clock.Kind == "primary_input" {
		for _, s := range series["input:"+p.Rule.Clock.Input] {
			if s.At >= options.OutputRange.From && (s.At < options.OutputRange.To || options.IncludeMax && s.At == math.MaxInt64) {
				times = append(times, s.At)
			}
		}
	}
	if len(times) > p.limits.OutputRows {
		return fmt.Errorf("output_budget_exceeded")
	}
	for _, at := range times {
		if err := ctx.Err(); err != nil {
			return err
		}
		if p.Rule.Clock.Kind != "whole_range" && (at < options.OutputRange.From || at >= options.OutputRange.To && !(options.IncludeMax && at == math.MaxInt64)) {
			continue
		}
		if p.Mode == "channel" {
			due, ok := addNS(at, ns(p.Rule.Policies.AllowedLatenessNS))
			if !ok || due > options.NowNS {
				continue
			}
		}
		for _, out := range p.Rule.Outputs {
			s, ok := atSample(series["node:"+out.Node], at)
			if !ok {
				s = Sample{At: at, Quality: "unmarked"}
				addReason(&s, "missing_input")
			}
			// Primary timestamps are formed before quality filtering. Rejected primary
			// samples remain visible with a reason even when a rolling statistic exists.
			if p.Rule.Clock.Kind == "primary_input" {
				primary, _ := atSample(series["input:"+p.Rule.Clock.Input], at)
				if !p.accepted(primary) {
					addReason(&s, "invalid_input")
				}
			}
			if s.Status == "" {
				s.Status = "valid"
			}
			var encoded monitoringvalue.Encoded
			if s.Value != nil {
				var e error
				encoded, e = s.Value.Encode(ctx, out.Numeric.Scale)
				if err := numericFailure(&s, e); err != nil {
					return err
				}
			}
			result := Result{MinAt: s.MinAt, MaxAt: s.MaxAt, MinTies: s.MinTies, MaxTies: s.MaxTies, OutputKey: out.Key, At: at, Interval: s.Interval, Scale: out.Numeric.Scale, Rounded: encoded.Rounded, Status: s.Status, Quality: s.Quality, Reasons: s.Reasons, Count: s.Count, Excluded: s.Excluded, InputFrom: s.InputFrom, InputTo: s.InputTo, Matches: s.Matches}
			if s.Status == "valid" && s.Value != nil {
				result.Value = &encoded.Value
			}
			if err := emit(result); err != nil {
				return err
			}
		}
	}
	return nil
}
func atSample(rows []Sample, at int64) (Sample, bool) {
	i := sort.Search(len(rows), func(i int) bool { return rows[i].At >= at })
	if i < len(rows) && rows[i].At == at {
		return rows[i], true
	}
	return Sample{}, false
}
func (p *Plan) match(rows []Sample, at int64, alignment string, tolerance int64) (Sample, bool) {
	i := sort.Search(len(rows), func(i int) bool { return rows[i].At >= at })
	if alignment == "exact" || alignment == "same_interval" {
		if i < len(rows) && rows[i].At == at {
			return rows[i], true
		}
		return Sample{}, false
	}
	best := -1
	distance := new(big.Int)
	consider := func(j int) {
		s := rows[j]
		d := new(big.Int).Abs(new(big.Int).Sub(big.NewInt(s.At), big.NewInt(at)))
		if d.Cmp(big.NewInt(tolerance)) <= 0 && p.accepted(s) && (best < 0 || d.Cmp(distance) < 0 || d.Cmp(distance) == 0 && s.At < rows[best].At) {
			best = j
			distance.Set(d)
		}
	}
	for j := i - 1; j >= 0 && rows[j].At >= subtractSaturated(at, tolerance); j-- {
		consider(j)
		if best >= 0 {
			break
		}
	}
	if i < len(rows) && rows[i].At == at {
		consider(i)
	}
	if alignment == "nearest" {
		end, ok := addNS(at, tolerance)
		if !ok {
			end = int64(^uint64(0) >> 1)
		}
		for j := i; j < len(rows) && rows[j].At <= end; j++ {
			consider(j)
			if p.accepted(rows[j]) {
				break
			}
		}
	}
	if best < 0 {
		return Sample{}, false
	}
	return rows[best], true
}
func (p *Plan) nodeSeries(ctx context.Context, n Node, series map[string][]Sample, baselines map[string]FixedBaseline, grid []int64) ([]Sample, error) {
	refs := n.Inputs
	if n.Op == "formula" {
		refs = nil
		keys := make([]string, 0, len(n.Params.Variables))
		for k := range n.Params.Variables {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			refs = append(refs, n.Params.Variables[k])
		}
	}
	var primary []Sample
	hasTimeline := false
	for _, ref := range refs {
		if !strings.HasPrefix(ref, "param:") {
			primary, hasTimeline = series[ref]
			break
		}
	}
	if p.Rule.Clock.Kind == "primary_input" && (len(refs) > 1 || n.Op == "formula") {
		primary, hasTimeline = series["input:"+p.Rule.Clock.Input]
	}
	if p.Rule.Clock.Kind != "primary_input" && n.Params.Alignment != "" {
		// Composite nodes evaluate on the declared grid, including its halo.
		// Temporal unary nodes retain the actual input sequence so adjacent/lag
		// semantics are never changed into interpolation or grid differencing.
		primary = make([]Sample, len(grid))
		for i, at := range grid {
			primary[i] = Sample{At: at}
			for _, ref := range refs {
				if strings.HasPrefix(ref, "param:") {
					continue
				}
				if exact, ok := atSample(series[ref], at); ok {
					primary[i].Interval = exact.Interval
					primary[i].ConfigID = exact.ConfigID
				}
				break
			}
		}
		hasTimeline = true
	}
	if !hasTimeline {
		return nil, fmt.Errorf("node_requires_sample_timeline")
	}
	out := make([]Sample, 0, len(primary))
	previous := -1
	for i, current := range primary {
		if i%128 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		s := Sample{At: current.At, Quality: "good", Status: "valid", Interval: current.Interval, ConfigID: current.ConfigID}
		if n.Op == "difference" || n.Op == "rate" {
			predecessor := previous
			if n.Params.DifferenceMode == "lag" {
				target := subtractSaturated(current.At, ns(n.Params.LagNS))
				old, ok := p.match(primary, target, "previous", ns(n.Params.MatchToleranceNS))
				predecessor = -1
				if ok {
					predecessor = sort.Search(len(primary), func(j int) bool { return primary[j].At >= old.At })
				}
				if n.Params.InvalidPolicy == "break" && predecessor >= 0 {
					for j := predecessor + 1; j < i; j++ {
						if !p.accepted(primary[j]) {
							predecessor = -1
							break
						}
					}
				}
			}
			if !p.accepted(current) {
				addReason(&s, current.Reasons...)
				addReason(&s, "invalid_input")
				if n.Params.InvalidPolicy == "break" {
					previous = -1
				}
			}
			if s.Status == "valid" {
				if predecessor < 0 {
					addReason(&s, "missing_input")
				} else {
					old := primary[predecessor]
					dt := new(big.Int).Sub(big.NewInt(current.At), big.NewInt(old.At))
					s.Count = 2
					from, to := old.At, current.At
					s.InputFrom = &from
					s.InputTo = &to
					s.Quality = worst(current.Quality, old.Quality)
					s.Matches = []Match{{refs[0], old.ID, old.At, new(big.Int).Neg(dt).String()}, {refs[0], current.ID, current.At, "0"}}
					if old.ConfigID != current.ConfigID {
						addReason(&s, "configuration_boundary")
					}
					if dt.Sign() <= 0 {
						addReason(&s, "invalid_input")
					} else if dt.Cmp(big.NewInt(ns(n.Params.MaxGapNS))) > 0 {
						addReason(&s, "gap_exceeded")
					}
					if s.Status == "valid" {
						value, e := monitoringvalue.Binary("subtract", current.Value, old.Value)
						if e != nil {
							return nil, e
						}
						if n.Op == "rate" {
							unit := int64(time.Second)
							switch n.Params.RateTimeUnit {
							case "minute":
								unit = int64(time.Minute)
							case "hour":
								unit = int64(time.Hour)
							case "day":
								unit = int64(24 * time.Hour)
							}
							factor, e := monitoringvalue.Rational(new(big.Rat).SetFrac(big.NewInt(unit), dt))
							if e != nil {
								return nil, e
							}
							value, e = monitoringvalue.Binary("multiply", value, factor)
							if e != nil {
								return nil, e
							}
						}
						s.Value = value
					}
				}
				previous = i
			}
			if s.Count == 0 {
				s.Quality = "unmarked"
			}
			out = append(out, s)
			continue
		}
		values := []*monitoringvalue.Number{}
		named := map[string]*monitoringvalue.Number{}
		configs := []string{}
		for _, ref := range refs {
			if strings.HasPrefix(ref, "param:") {
				v, e := monitoringvalue.Decimal(p.Values[strings.TrimPrefix(ref, "param:")])
				if e != nil {
					return nil, e
				}
				values = append(values, v)
				named[ref] = v
				continue
			}
			matched, ok := p.match(series[ref], current.At, n.Params.Alignment, ns(n.Params.ToleranceNS))
			if n.Params.Alignment == "" {
				matched, ok = atSample(series[ref], current.At)
			}
			if !ok {
				addReason(&s, "missing_input")
				continue
			}
			if len(configs) == 0 {
				s.ConfigID = matched.ConfigID
			}
			configs = append(configs, ref+":"+matched.ConfigID)
			s.Matches = append(s.Matches, Match{ref, matched.ID, matched.At, new(big.Int).Sub(big.NewInt(matched.At), big.NewInt(current.At)).String()})
			if !p.accepted(matched) {
				addReason(&s, matched.Reasons...)
				addReason(&s, matched.Status)
				addReason(&s, "invalid_input")
				continue
			}
			if n.Params.Alignment == "same_interval" && (current.Interval == nil || matched.Interval == nil || *current.Interval != *matched.Interval) {
				addReason(&s, "invalid_input")
				continue
			}
			s.Quality = worst(s.Quality, matched.Quality)
			s.Count++
			if s.InputFrom == nil || matched.At < *s.InputFrom {
				v := matched.At
				s.InputFrom = &v
			}
			if s.InputTo == nil || matched.At > *s.InputTo {
				v := matched.At
				s.InputTo = &v
			}
			values = append(values, matched.Value)
			named[ref] = matched.Value
		}
		if len(configs) > 1 {
			s.ConfigID = digest(configs)
		}
		if n.Op == "baseline_delta" || n.Op == "baseline_ratio" {
			b := n.Params.Baseline
			var v FixedBaseline
			if b.Kind == "constant" {
				value, e := monitoringvalue.Decimal(b.Value)
				if e != nil {
					return nil, e
				}
				v = FixedBaseline{Value: value, Valid: true, Proven: true, Quality: "good"}
			} else {
				var ok bool
				v, ok = baselines[b.Reference]
				if !ok || !v.Proven {
					return nil, fmt.Errorf("baseline_evidence_unavailable")
				}
			}
			if !v.Valid {
				addReason(&s, "baseline_invalid")
			} else if b.Kind != "constant" && p.Mode == "channel" && v.At > current.At {
				addReason(&s, "noncausal_baseline")
			}
			if b.Kind == "reference_window" && p.Mode == "channel" && ns(b.ToNS) > current.At {
				addReason(&s, "noncausal_baseline")
			}
			if s.Status == "valid" {
				if v.Value == nil {
					return nil, fmt.Errorf("baseline_evidence_unavailable")
				}
				value, e := monitoringvalue.Binary("subtract", values[0], v.Value)
				if e == nil && n.Op == "baseline_ratio" {
					value, e = monitoringvalue.Binary("divide", value, v.Value)
				}
				s.Value = value
				if err := numericFailure(&s, e); err != nil {
					return nil, err
				}
				s.Quality = worst(s.Quality, v.Quality)
				if b.Kind != "constant" {
					s.Matches = append(s.Matches, Match{"baseline:" + b.Reference, v.ID, v.At, new(big.Int).Sub(big.NewInt(v.At), big.NewInt(current.At)).String()})
					if v.At > current.At {
						s.Reasons = append(s.Reasons, "noncausal_basis")
					}
				}
			}
		} else if s.Status == "valid" {
			var value *monitoringvalue.Number
			var e error
			switch n.Op {
			case "formula":
				vars := map[string]*monitoringvalue.Number{}
				for k, ref := range n.Params.Variables {
					vars[k] = named[ref]
				}
				value, e = p.formulas[n.ID].evaluate(vars)
			case "abs":
				value, e = monitoringvalue.Unary("abs", values[0])
			case "weighted_mean":
				value = monitoringvalue.Integer(0)
				total := new(big.Rat)
				for j, v := range values {
					weight, _ := monitoringvalue.ParseDecimal(n.Params.Weights[j])
					total.Add(total, weight)
					w, err := monitoringvalue.Rational(weight)
					if err != nil {
						return nil, err
					}
					term, err := monitoringvalue.Binary("multiply", v, w)
					if err != nil {
						return nil, err
					}
					value, err = monitoringvalue.Binary("add", value, term)
					if err != nil {
						return nil, err
					}
				}
				d, err := monitoringvalue.Rational(total)
				if err != nil {
					return nil, err
				}
				value, e = monitoringvalue.Binary("divide", value, d)
			default:
				value = values[0]
				for _, v := range values[1:] {
					value, e = monitoringvalue.Binary(n.Op, value, v)
					if e != nil {
						break
					}
				}
			}
			s.Value = value
			if err := numericFailure(&s, e); err != nil {
				return nil, err
			}
		}
		if n.Params.Alignment == "nearest" {
			s.Reasons = append(s.Reasons, "noncausal_alignment")
		}
		if s.Count == 0 {
			s.Quality = "unmarked"
		}
		out = append(out, s)
	}
	return out, nil
}

// executionGrid is shared by in-memory and externally aggregated execution.
func (p *Plan) executionGrid(options RunOptions, partitionLimit int) ([]int64, error) {
	var grid []int64
	if p.Rule.Clock.Kind == "fixed_grid" || p.Rule.Clock.Kind == "calendar" {
		r := options.OutputRange
		// Callers include a halo in the source range for multi-step grid nodes.
		if options.SourceRange.From < r.From && options.SourceRange.From >= monitoringvalue.MinTime.UnixNano() {
			r.From = options.SourceRange.From
		}
		if p.Mode == "analysis" && options.SourceRange.To > r.To {
			r.To = options.SourceRange.To
		}
		var e error
		if r.From == r.To && options.IncludeMax {
			grid = nil
		} else {
			grid, e = GridTimes(p.Rule.Clock, r, min(p.limits.OutputRows, partitionLimit))
		}
		if options.IncludeMax && p.Rule.Clock.Kind == "fixed_grid" {
			delta := new(big.Int).Sub(big.NewInt(math.MaxInt64), big.NewInt(ns(p.Rule.Clock.AnchorNS)))
			if new(big.Int).Mod(delta, big.NewInt(ns(p.Rule.Clock.StepNS))).Sign() == 0 {
				grid = append(grid, math.MaxInt64)
			}
		}
		if e != nil {
			return nil, e
		}
	} else if p.Rule.Clock.Kind == "whole_range" {
		grid = []int64{options.SourceRange.To}
	}
	return grid, nil
}
