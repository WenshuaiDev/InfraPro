package planning

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

// Inputs walks the compiled DAG backwards. Each edge contributes its candidate
// (including non-hit) interval, not just the rows a successful match happened to
// pick. The resulting union is the authorization and fixed snapshot boundary.
// Fixed baselines have separate immutable snapshots and are joined by the host.
type Predecessor struct {
	Input        string
	Before       int64
	Count        int
	Qualities    []string
	WholeHistory bool
}

func Inputs(p *compute.Plan, output Range, source *Range, prefixes ...*[]Predecessor) (map[string][]Range, error) {
	if !output.Valid() {
		return nil, fmt.Errorf("invalid_output_range")
	}
	if p.Rule.Clock.Kind == "whole_range" && (source == nil || !source.Valid() || source.To == nil) {
		return nil, fmt.Errorf("invalid_source_range")
	}
	required := map[string][]Range{}
	for _, o := range p.Rule.Outputs {
		required["node:"+o.Node] = append(required["node:"+o.Node], output)
	}
	if p.Rule.Clock.Kind == "primary_input" {
		required["input:"+p.Rule.Clock.Input] = append(required["input:"+p.Rule.Clock.Input], output)
	}
	number := func(s string) int64 { v, _ := strconv.ParseInt(s, 10, 64); return v }
	for i := len(p.Rule.Nodes) - 1; i >= 0; i-- {
		n := p.Rule.Nodes[i]
		ranges, e := Union(required["node:"+n.ID])
		if e != nil {
			return nil, e
		}
		refs := slices.Clone(n.Inputs)
		if n.Op == "formula" {
			refs = nil
			for _, ref := range n.Params.Variables {
				refs = append(refs, ref)
			}
			slices.Sort(refs)
			refs = slices.Compact(refs)
		}
		for _, r := range ranges {
			need := r
			if len(prefixes) > 0 {
				count := 0
				qualities := p.Rule.Policies.Quality
				before := r.From
				if (n.Op == "difference" || n.Op == "rate") && n.Params.DifferenceMode == "adjacent" {
					count = 1
					if n.Params.InvalidPolicy == "break" {
						qualities = nil
					}
				}
				if n.Params.Window != nil && n.Params.Window.Kind == "trailing_count" {
					count = int(number(n.Params.Window.Count))
					if before < math.MaxInt64 {
						before++
					}
				}
				if count > 0 {
					input := strings.TrimPrefix(n.Inputs[0], "input:")
					*prefixes[0] = append(*prefixes[0], Predecessor{Input: input, Before: before, Count: count, Qualities: qualities, WholeHistory: !strings.HasPrefix(n.Inputs[0], "input:")})
				}
			}
			switch {
			case n.Op == "difference" || n.Op == "rate":
				width := number(n.Params.MaxGapNS)
				if n.Params.DifferenceMode == "lag" {
					lag, tolerance := number(n.Params.LagNS), number(n.Params.MatchToleranceNS)
					if lag > math.MaxInt64-tolerance {
						width = math.MaxInt64
					} else {
						width = max(width, lag+tolerance)
					}
				}
				if width <= 0 {
					return nil, fmt.Errorf("unbounded_predecessor")
				}
				// With an explicit predecessor resolver, max_gap is a result
				// validity rule rather than a candidate-search interval. Retaining
				// the old halo here would require unrelated permissions and make
				// a fully computed upstream prefix appear to have missing coverage.
				if len(prefixes) == 0 || n.Params.DifferenceMode == "lag" {
					need.From = leftShift(r.From, width)
				}
			case n.Params.Window != nil:
				w := n.Params.Window
				switch w.Kind {
				case "trailing_duration":
					need, e = DurationHalo(r, number(w.DurationNS))
				case "trailing_count":
					if len(prefixes) == 0 {
						need.From = leftShift(r.From, number(w.MaxSpanNS))
					}
				case "fixed_duration":
					need.From = leftShift(r.From, number(w.DurationNS))
				case "calendar":
					loc, err := time.LoadLocation(w.Timezone)
					if err != nil {
						return nil, err
					}
					at := time.Unix(0, r.From).In(loc)
					year, month, day := at.Date()
					start := time.Date(year, month, day, 0, 0, 0, 0, loc)
					switch w.Period {
					case "day":
						if start.Equal(at) {
							start = start.AddDate(0, 0, -1)
						}
					case "month":
						start = time.Date(year, month, 1, 0, 0, 0, 0, loc)
						if start.Equal(at) {
							start = start.AddDate(0, -1, 0)
						}
					case "year":
						start = time.Date(year, time.January, 1, 0, 0, 0, 0, loc)
						if start.Equal(at) {
							start = start.AddDate(-1, 0, 0)
						}
					default:
						return nil, fmt.Errorf("invalid_calendar_period")
					}
					if start.Before(monitoringvalue.MinTime) {
						need.From = monitoringvalue.MinTime.UnixNano()
					} else {
						need.From = start.UnixNano()
					}
				case "whole_range":
					need = *source
				}
				if e != nil {
					return nil, e
				}
			default:
				if n.Params.Alignment == "previous" || n.Params.Alignment == "nearest" {
					need, e = MatchHalo(r, number(n.Params.ToleranceNS), n.Params.Alignment == "nearest")
					if e != nil {
						return nil, e
					}
				}
			}
			// Multi-input nodes use the declared primary input's timeline even when it
			// does not appear as a value operand of this particular node.
			if p.Rule.Clock.Kind == "primary_input" {
				required["input:"+p.Rule.Clock.Input] = append(required["input:"+p.Rule.Clock.Input], need)
			}
			for _, ref := range refs {
				if !strings.HasPrefix(ref, "param:") {
					required[ref] = append(required[ref], need)
				}
			}
		}
	}
	out := map[string][]Range{}
	for _, input := range p.Rule.Inputs {
		ranges, e := Union(required["input:"+input.Key])
		if e != nil {
			return nil, e
		}
		out[input.Key] = ranges
	}
	return out, nil
}
