package monitoring

import (
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

// CalculationSummary is an immutable per-block exact summary. It is usable only
// after the caller has qualified the block's current root and recursive source
// proof, and only when the requested interval includes the whole block.
type CalculationSummary map[string]CalculationSummaryPart

type CalculationSummaryPart struct {
	Counts   CalculationCounts      `json:"counts"`
	N        uint64                 `json:"n,string"`
	Sum      string                 `json:"sum"`
	Squares  string                 `json:"squares"`
	First    *CalculatedObservation `json:"first"`
	Last     *CalculatedObservation `json:"last"`
	Minimum  *CalculatedObservation `json:"minimum"`
	Maximum  *CalculatedObservation `json:"maximum"`
	Latest   *CalculatedObservation `json:"latest"`
	Earliest *CalculatedObservation `json:"earliest"`
	MinTies  uint64                 `json:"minimum_ties,string"`
	MaxTies  uint64                 `json:"maximum_ties,string"`
}

type CalculationSummaryBuilder struct {
	groups map[string]*calculationAccumulator
}

func (b *CalculationSummaryBuilder) Add(row CalculatedObservation) error {
	if b.groups == nil {
		b.groups = map[string]*calculationAccumulator{}
	}
	c := b.groups[row.Quality]
	if c == nil {
		c = newCalculationAccumulator()
		b.groups[row.Quality] = c
	}
	return c.add(row, allQualities)
}
func (b *CalculationSummaryBuilder) Summary() CalculationSummary {
	out := CalculationSummary{}
	for q, c := range b.groups {
		out[q] = CalculationSummaryPart{c.counts, c.moments.N, c.moments.Sum.RatString(), c.moments.Squares.RatString(), c.first, c.last, c.minimum, c.maximum, c.latest, c.earliest, c.minTies, c.maxTies}
	}
	return out
}
func calculationSummaryPoint(p *CalculatedObservation) (*CalculatedObservation, error) {
	if p == nil {
		return nil, nil
	}
	out := *p
	at, e := time.Parse(time.RFC3339Nano, out.At)
	if e != nil {
		return nil, e
	}
	out.AtNS = at.UnixNano()
	return &out, nil
}
func (c *calculationAccumulator) mergeSummary(summary CalculationSummary, qualities []string) error {
	for q, v := range summary {
		if !slices.Contains(allQualities, q) {
			return fmt.Errorf("invalid_calculation_summary_quality")
		}
		points := []*CalculatedObservation{v.First, v.Last, v.Minimum, v.Maximum, v.Latest, v.Earliest}
		for i, p := range points {
			var e error
			points[i], e = calculationSummaryPoint(p)
			if e != nil {
				return e
			}
		}
		first, last, minimum, maximum, latest, earliest := points[0], points[1], points[2], points[3], points[4], points[5]
		c.counts.merge(v.Counts)
		c.qualities[q] = addCount(c.qualities[q], v.Counts.LogicalOutputs)
		if latest != nil && (c.latest == nil || calculatedBefore(c.latest, latest)) {
			c.latest = latest
		}
		if earliest != nil && (c.earliest == nil || calculatedBefore(earliest, c.earliest)) {
			c.earliest = earliest
		}
		if !slices.Contains(qualities, q) {
			continue
		}
		moments := monitoringvalue.Moments{N: v.N}
		if _, ok := moments.Sum.SetString(v.Sum); !ok {
			return fmt.Errorf("invalid_calculation_summary_sum")
		}
		if _, ok := moments.Squares.SetString(v.Squares); !ok || moments.Squares.Sign() < 0 {
			return fmt.Errorf("invalid_calculation_summary_squares")
		}
		if v.Counts.ValidResults != strconv.FormatUint(v.N, 10) {
			return fmt.Errorf("invalid_calculation_summary_count")
		}
		if e := c.moments.Merge(&moments); e != nil {
			return e
		}
		if first != nil && (c.first == nil || calculatedBefore(first, c.first)) {
			c.first = first
		}
		if last != nil && (c.last == nil || calculatedBefore(c.last, last)) {
			c.last = last
		}
		if minimum != nil {
			if minimum.Value == nil {
				return fmt.Errorf("invalid_calculation_summary_minimum")
			}
			n, e := monitoringvalue.ParseDecimal(*minimum.Value)
			if e != nil {
				return e
			}
			if c.minValue == nil || n.Cmp(c.minValue) < 0 {
				c.minValue = new(big.Rat).Set(n)
				c.minimum = minimum
				c.minTies = v.MinTies
			} else if n.Cmp(c.minValue) == 0 {
				c.minTies += v.MinTies
				if calculatedBefore(minimum, c.minimum) {
					c.minimum = minimum
				}
			}
		}
		if maximum != nil {
			if maximum.Value == nil {
				return fmt.Errorf("invalid_calculation_summary_maximum")
			}
			n, e := monitoringvalue.ParseDecimal(*maximum.Value)
			if e != nil {
				return e
			}
			if c.maxValue == nil || n.Cmp(c.maxValue) > 0 {
				c.maxValue = new(big.Rat).Set(n)
				c.maximum = maximum
				c.maxTies = v.MaxTies
			} else if n.Cmp(c.maxValue) == 0 {
				c.maxTies += v.MaxTies
				if calculatedBefore(maximum, c.maximum) {
					c.maximum = maximum
				}
			}
		}
	}
	return nil
}
