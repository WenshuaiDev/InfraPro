package planning

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
)

// Impact propagates source changes forwards through the DAG. Rank/predecessor
// operators conservatively cover the bounded remaining assignment when an old
// and new ordinal search is not available. This is deliberately independent of
// the calculation read halo: max_gap rejects a predecessor but does not erase
// its identity or its influence on a later gap_exceeded result.
func Impact(p *compute.Plan, inputs map[string][]Range, bound Range) ([]Range, bool, error) {
	changed := map[string][]Range{}
	for key, ranges := range inputs {
		changed["input:"+key] = ranges
	}
	conservative := false
	number := func(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
	for _, node := range p.Rule.Nodes {
		refs := slices.Clone(node.Inputs)
		if node.Op == "formula" {
			for _, ref := range node.Params.Variables {
				refs = append(refs, ref)
			}
		}
		if p.Rule.Clock.Kind == "primary_input" {
			refs = append(refs, "input:"+p.Rule.Clock.Input)
		}
		ranges := []Range{}
		for _, ref := range refs {
			ranges = append(ranges, changed[ref]...)
		}
		merged, e := Union(ranges)
		if e != nil {
			return nil, false, e
		}
		for _, r := range merged {
			switch {
			case node.Params.Baseline != nil && node.Params.Baseline.Kind != "constant":
				r = bound
				conservative = true
			case node.Op == "difference" || node.Op == "rate":
				if node.Params.DifferenceMode == "adjacent" {
					r.To = bound.To
					conservative = true
				} else {
					lag, tolerance := number(node.Params.LagNS), number(node.Params.MatchToleranceNS)
					width := lag
					if width > math.MaxInt64-tolerance {
						width = math.MaxInt64
					} else {
						width += tolerance
					}
					r.To = shiftedEnd(r.To, width)
				}
			case node.Params.Window != nil:
				w := node.Params.Window
				switch w.Kind {
				case "trailing_duration":
					r, e = DurationImpact(r, number(w.DurationNS))
				case "fixed_duration":
					r.To = shiftedEnd(r.To, number(w.DurationNS))
				case "trailing_count", "whole_range":
					r.To = bound.To
					conservative = true
				case "calendar":
					// A Gregorian year, including timezone offset transitions, is
					// shorter than 368 days. The grid selects actual calendar ends.
					r.To = shiftedEnd(r.To, int64(368*24*time.Hour))
					conservative = true
				}
			case node.Params.Alignment == "previous":
				r.To = shiftedEnd(r.To, number(node.Params.ToleranceNS))
			case node.Params.Alignment == "nearest":
				r.From = leftShift(r.From, number(node.Params.ToleranceNS))
				r.To = shiftedEnd(r.To, number(node.Params.ToleranceNS))
			}
			if e != nil {
				return nil, false, e
			}
			// Intermediate nodes before an assignment boundary can influence an
			// output inside it; clip only at the final outputs, never at a node.
			if r.Valid() {
				changed["node:"+node.ID] = append(changed["node:"+node.ID], r)
			}
		}
	}
	out := []Range{}
	for _, output := range p.Rule.Outputs {
		key := output.Node
		if !strings.HasPrefix(key, "node:") {
			key = "node:" + key
		}
		for _, r := range changed[key] {
			if part, ok := Intersection(r, bound); ok {
				out = append(out, part)
			}
		}
	}
	out, e := Union(out)
	return out, conservative, e
}
