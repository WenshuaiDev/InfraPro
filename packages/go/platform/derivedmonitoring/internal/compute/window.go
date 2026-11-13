package compute

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

type gap struct {
	right    int
	distance *big.Int
}

func (p *Plan) windowSeries(ctx context.Context, n Node, input []Sample, grid []int64, options RunOptions) ([]Sample, error) {
	times := grid

	var moments monitoringvalue.Moments
	tree := orderTree{numeric: strings.Contains("|window_min|window_max|window_range|window_median|window_quantile|", "|"+n.Op+"|")}
	needMoments := n.Op == "window_sum" || n.Op == "window_mean" || n.Op == "window_variance" || n.Op == "window_stddev"
	if needMoments {
		for _, s := range input {
			if p.accepted(s) {
				if _, ok := s.Value.Exact(); !ok {
					tree.aggregate = true
					break
				}
			}
		}
	}
	needTree := tree.numeric || tree.aggregate
	quality := map[string]int{}
	configs := map[string]int{}
	accepted := []int{}
	head := 0
	gaps := []gap{}
	gapHead := 0
	// Prefix counts allow excluded input reporting without rescanning each window.
	excluded := make([]uint64, len(input)+1)
	for i, s := range input {
		excluded[i+1] = excluded[i]
		if !p.accepted(s) {
			excluded[i+1]++
		}
	}
	left, right := 0, 0
	add := func(i int) error {
		s := input[i]
		if !p.accepted(s) {
			return nil
		}
		if len(accepted) > head {
			prev := accepted[len(accepted)-1]
			distance := new(big.Int).Sub(big.NewInt(s.At), big.NewInt(input[prev].At))
			for len(gaps) > gapHead && gaps[len(gaps)-1].distance.Cmp(distance) <= 0 {
				gaps = gaps[:len(gaps)-1]
			}
			gaps = append(gaps, gap{i, distance})
		}
		accepted = append(accepted, i)
		quality[s.Quality]++
		configs[s.ConfigID]++
		if len(accepted)-head > p.limits.WindowSamples {
			return fmt.Errorf("window_budget_exceeded")
		}
		if needTree {
			if e := tree.add(ctx, s, i); e != nil {
				return e
			}
		}
		if needMoments && !tree.aggregate {
			r, _ := s.Value.Exact()
			return moments.Add(r)
		}
		return nil
	}
	remove := func() error {
		i := accepted[head]
		s := input[i]
		head++
		quality[s.Quality]--
		configs[s.ConfigID]--
		if configs[s.ConfigID] == 0 {
			delete(configs, s.ConfigID)
		}
		if head < len(accepted) {
			for gapHead < len(gaps) && gaps[gapHead].right <= accepted[head] {
				gapHead++
			}
		} else {
			gapHead = len(gaps)
		}
		if needTree {
			if e := tree.delete(ctx, s, i); e != nil {
				return e
			}
		}
		if needMoments && !tree.aggregate {
			r, _ := s.Value.Exact()
			return moments.Remove(r)
		}
		return nil
	}
	out := make([]Sample, 0, len(times))
	for index, at := range times {
		if index%64 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		w := n.Params.Window
		inclusive := true
		bounds := TimeRange{}
		if w.Kind == "trailing_count" {
			for right < len(input) && input[right].At <= at { // Remove before adding to keep exactly N active samples.
				if p.accepted(input[right]) && len(accepted)-head == int(ns(w.Count)) {
					if e := remove(); e != nil {
						return nil, e
					}
				}
				if e := add(right); e != nil {
					return nil, e
				}
				right++
			}
			if head < len(accepted) {
				left = accepted[head]
				bounds = TimeRange{input[left].At, at}
			} else {
				left = right
				bounds = TimeRange{at, at}
			}
		} else {
			var err error
			bounds, inclusive, err = windowRange(*w, at, options.SourceRange)
			if err != nil {
				return nil, err
			}
			// Evict first: adjacent fixed windows must not temporarily hold both full
			// windows and incorrectly exceed the active-window budget.
			for head < len(accepted) && (input[accepted[head]].At < bounds.From || inclusive && input[accepted[head]].At == bounds.From) {
				if e := remove(); e != nil {
					return nil, e
				}
			}
			for left < len(input) && (input[left].At < bounds.From || inclusive && input[left].At == bounds.From) {
				left++
			}
			if right < left {
				right = left
			}
			for right < len(input) && (input[right].At < bounds.To || inclusive && input[right].At == bounds.To) {
				if e := add(right); e != nil {
					return nil, e
				}
				right++
			}
		}
		count := len(accepted) - head
		s := Sample{At: at, Quality: "unmarked", Status: "valid", Count: uint64(count), Excluded: excluded[right] - excluded[left]}
		if w.Kind == "fixed_duration" || w.Kind == "calendar" || w.Kind == "whole_range" {
			copy := bounds
			s.Interval = &copy
		}
		if count == 0 {
			s.Reasons = append(s.Reasons, "empty_window")
		} else {
			first, last := input[accepted[head]], input[accepted[len(accepted)-1]]
			from, to := first.At, last.At
			s.InputFrom = &from
			s.InputTo = &to
			s.ConfigID = first.ConfigID
			s.Quality = "good"
			for q, c := range quality {
				if c > 0 {
					s.Quality = worst(s.Quality, q)
				}
			}
			if len(configs) > 1 {
				addReason(&s, "configuration_boundary")
			}
			if n.Params.MaxGapNS != "" && gapHead < len(gaps) && gaps[gapHead].distance.Cmp(big.NewInt(ns(n.Params.MaxGapNS))) > 0 {
				addReason(&s, "gap_exceeded")
			}
			if n.Params.MaxAgeNS != "" && new(big.Int).Sub(big.NewInt(at), big.NewInt(last.At)).Cmp(big.NewInt(ns(n.Params.MaxAgeNS))) > 0 {
				addReason(&s, "age_exceeded")
			}
			if w.Kind == "trailing_count" && new(big.Int).Sub(big.NewInt(last.At), big.NewInt(first.At)).Cmp(big.NewInt(ns(w.MaxSpanNS))) > 0 {
				addReason(&s, "gap_exceeded")
			}
		}
		if count < int(ns(n.Params.MinSamples)) {
			addReason(&s, "insufficient_samples")
		}
		if s.Status == "valid" {
			if n.Op == "window_count" {
				s.Value = monitoringvalue.Integer(int64(count))
			} else if count == 0 {
				addReason(&s, "insufficient_samples")
			} else {
				value, e := windowValue(n, &moments, &tree, count)
				if e == nil && (n.Op == "window_min" || n.Op == "window_max" || n.Op == "window_range") {
					minAt, maxAt, minTies, maxTies, err := tree.extrema(ctx)
					if err != nil {
						return nil, err
					}
					s.MinAt = &minAt
					s.MaxAt = &maxAt
					s.MinTies = minTies
					s.MaxTies = maxTies
				}
				s.Value = value
				if err := numericFailure(&s, e); err != nil {
					return nil, err
				}
			}
		}
		out = append(out, s)
	}
	return out, nil
}
func windowValue(n Node, m *monitoringvalue.Moments, tree *orderTree, count int) (*monitoringvalue.Number, error) {
	switch n.Op {
	case "window_min":
		return tree.rank(0).Value, nil
	case "window_max":
		return tree.rank(count - 1).Value, nil
	case "window_range":
		return monitoringvalue.Binary("subtract", tree.rank(count-1).Value, tree.rank(0).Value)
	case "window_median":
		return tree.quantile(big.NewRat(50, 1))
	case "window_quantile":
		p, _ := monitoringvalue.ParseDecimal(n.Params.Percentile)
		return tree.quantile(p)
	}
	var sum, squares *monitoringvalue.Number
	var err error
	if tree.aggregate {
		sum = tree.root.sum
		squares = tree.root.squares
	} else {
		sum, err = monitoringvalue.Rational(&m.Sum)
		if err != nil {
			return nil, err
		}
		squares, err = monitoringvalue.Rational(&m.Squares)
		if err != nil {
			return nil, err
		}
	}
	if n.Op == "window_sum" {
		return sum, nil
	}
	den := monitoringvalue.Integer(int64(count))
	if n.Op == "window_mean" {
		return monitoringvalue.Binary("divide", sum, den)
	}
	var variance *monitoringvalue.Number
	if !tree.aggregate {
		r, e := m.Variance()
		if e != nil {
			return nil, e
		}
		variance, err = monitoringvalue.Rational(r)
	} else {
		// Exact algebraic expression with interval refinement at the final boundary.
		a, e := monitoringvalue.Binary("multiply", den, squares)
		if e != nil {
			return nil, e
		}
		b, e := monitoringvalue.Binary("multiply", sum, sum)
		if e != nil {
			return nil, e
		}
		v, e := monitoringvalue.Binary("subtract", a, b)
		if e != nil {
			return nil, e
		}
		d := monitoringvalue.Integer(int64(count) * int64(count))
		variance, err = monitoringvalue.Binary("divide", v, d)
	}
	if err != nil {
		return nil, err
	}
	if n.Op == "window_stddev" {
		return monitoringvalue.Unary("sqrt", variance)
	}
	return variance, nil
}
