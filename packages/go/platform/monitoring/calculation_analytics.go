package monitoring

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

var ErrCalculationBudget = errQueryBudget

func earlierTime(a, b *string) *string {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	x, _ := time.Parse(time.RFC3339Nano, *a)
	y, _ := time.Parse(time.RFC3339Nano, *b)
	if y.Before(x) {
		return b
	}
	return a
}
func laterTime(a, b *string) *string {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	x, _ := time.Parse(time.RFC3339Nano, *a)
	y, _ := time.Parse(time.RFC3339Nano, *b)
	if y.After(x) {
		return b
	}
	return a
}

type CalculationCounts struct {
	LogicalOutputs string            `json:"logical_output_count"`
	ValidResults   string            `json:"valid_result_count"`
	NoResults      string            `json:"no_result_count"`
	Statuses       map[string]string `json:"status_counts"`
}

func newCalculationCounts() CalculationCounts {
	return CalculationCounts{LogicalOutputs: "0", ValidResults: "0", NoResults: "0", Statuses: map[string]string{}}
}
func addCount(a, b string) string {
	x, _ := new(big.Int).SetString(a, 10)
	if x == nil {
		x = new(big.Int)
	}
	y, _ := new(big.Int).SetString(b, 10)
	if y != nil {
		x.Add(x, y)
	}
	return x.String()
}
func (c *CalculationCounts) merge(other CalculationCounts) {
	c.LogicalOutputs = addCount(c.LogicalOutputs, other.LogicalOutputs)
	c.ValidResults = addCount(c.ValidResults, other.ValidResults)
	c.NoResults = addCount(c.NoResults, other.NoResults)
	for key, count := range other.Statuses {
		c.Statuses[key] = addCount(c.Statuses[key], count)
	}
}

type calculationAccumulator struct {
	counts                                          CalculationCounts
	moments                                         monitoringvalue.Moments
	first, last, minimum, maximum, latest, earliest *CalculatedObservation
	minValue, maxValue                              *big.Rat
	minTies, maxTies                                uint64
	qualities                                       map[string]string
}

func newCalculationAccumulator() *calculationAccumulator {
	return &calculationAccumulator{counts: newCalculationCounts(), qualities: map[string]string{"good": "0", "unmarked": "0", "suspect": "0", "bad": "0"}}
}
func calculatedBefore(a, b *CalculatedObservation) bool {
	if a.AtNS != b.AtNS {
		return a.AtNS < b.AtNS
	}
	return a.Result < b.Result
}
func (c *calculationAccumulator) add(row CalculatedObservation, qualities []string) error {
	c.counts.LogicalOutputs = addCount(c.counts.LogicalOutputs, "1")
	c.counts.Statuses[row.Status] = addCount(c.counts.Statuses[row.Status], "1")
	c.qualities[row.Quality] = addCount(c.qualities[row.Quality], "1")
	if c.latest == nil || calculatedBefore(c.latest, &row) {
		v := row
		c.latest = &v
	}
	if c.earliest == nil || calculatedBefore(&row, c.earliest) {
		v := row
		c.earliest = &v
	}
	if row.Status != "valid" || row.Value == nil {
		c.counts.NoResults = addCount(c.counts.NoResults, "1")
		return nil
	}
	c.counts.ValidResults = addCount(c.counts.ValidResults, "1")
	if !slices.Contains(qualities, row.Quality) {
		return nil
	}
	value, e := monitoringvalue.ParseDecimal(*row.Value)
	if e != nil {
		return e
	}
	if e = c.moments.Add(value); e != nil {
		return e
	}
	if c.first == nil || calculatedBefore(&row, c.first) {
		v := row
		c.first = &v
	}
	if c.last == nil || calculatedBefore(c.last, &row) {
		v := row
		c.last = &v
	}
	if c.minValue == nil || value.Cmp(c.minValue) < 0 {
		v := row
		c.minimum = &v
		c.minValue = value
		c.minTies = 1
	} else if value.Cmp(c.minValue) == 0 {
		c.minTies++
		if calculatedBefore(&row, c.minimum) {
			v := row
			c.minimum = &v
		}
	}
	if c.maxValue == nil || value.Cmp(c.maxValue) > 0 {
		v := row
		c.maximum = &v
		c.maxValue = value
		c.maxTies = 1
	} else if value.Cmp(c.maxValue) == 0 {
		c.maxTies++
		if calculatedBefore(&row, c.maximum) {
			v := row
			c.maximum = &v
		}
	}
	return nil
}
func calculatedDTO(v *CalculatedObservation) *Observation {
	if v == nil {
		return nil
	}
	out := &Observation{SourceKind: "derived", Calculated: v, ID: v.Result, Channel: v.Channel, At: v.At, Start: v.Start, Kind: "decimal", Quality: v.Quality}
	if v.Value != nil {
		out.Value = *v.Value
	}
	return out
}
func (c *calculationAccumulator) statistic(ctx context.Context, channel string, in QueryInput) (ChannelStatistic, error) {
	out := ChannelStatistic{SourceKind: "derived", Channel: channel, N: strconv.FormatUint(c.moments.N, 10), Scale: queryDigits(in), Rounding: "half_even", Overview: Overview{Ever: "0", Active: "0", Samples: "0", Qualities: c.qualities, Derived: &c.counts}, LatestOutput: c.latest, First: calculatedDTO(c.first), Last: calculatedDTO(c.last), Min: calculatedDTO(c.minimum), Max: calculatedDTO(c.maximum), MinCount: decimalCount(c.minTies), MaxCount: decimalCount(c.maxTies), States: []EnumStatistic{}}
	if c.latest != nil {
		out.Latest = &c.latest.At
		out.Earliest = &c.earliest.At
	}
	if c.moments.N == 0 {
		return out, nil
	}
	mean, e := monitoringvalue.Rational(c.moments.Mean())
	if e != nil {
		return out, e
	}
	variance, e := c.moments.Variance()
	if e != nil {
		return out, e
	}
	v, e := monitoringvalue.Rational(variance)
	if e != nil {
		return out, e
	}
	std, e := monitoringvalue.Unary("sqrt", v)
	if e != nil {
		return out, e
	}
	encode := func(key string, number *monitoringvalue.Number) (*string, error) {
		value, e := number.Encode(ctx, out.Scale)
		if e != nil {
			if errors.Is(e, monitoringvalue.ErrUnderflow) || errors.Is(e, monitoringvalue.ErrRange) || errors.Is(e, monitoringvalue.ErrPrecision) {
				if out.NumericIssues == nil {
					out.NumericIssues = map[string]string{}
				}
				out.NumericIssues[key] = e.Error()
				return nil, nil
			}
			return nil, e
		}
		return &value.Value, nil
	}
	out.Mean, e = encode("mean", mean)
	if e != nil {
		return out, e
	}
	out.Stddev, e = encode("population_stddev", std)
	return out, e
}
func (s *Service) calculationStatistic(ctx context.Context, a access.Actor, scope Scope, channel string, in QueryInput, plan queryPlan, background bool) (ChannelStatistic, []CalculationCoverage, error) {
	if s.calculationReader == nil {
		return ChannelStatistic{}, nil, problem("calculation_storage_unavailable", "派生读取服务未就绪。", 503)
	}
	acc := newCalculationAccumulator()
	budget := uint64(100_000)
	if background {
		budget = 2_000_000
	}
	coverage, e := s.calculationReader.ScanCalculated(ctx, a, scope, CalculationQuery{Summary: func(summary CalculationSummary) error { return acc.mergeSummary(summary, in.Qualities) }, Heads: map[string]CalculationHead{channel: plan.vector.Calculated[channel]}, Windows: map[string][]Window{channel: plan.windows[channel]}, Range: queryWindow(in), Qualities: allQualities, History: in.Status == "history"}, budget, func(row CalculatedObservation) error { return acc.add(row, in.Qualities) })
	if e != nil {
		return ChannelStatistic{}, nil, e
	}
	stat, e := acc.statistic(ctx, channel, in)
	return stat, coverage, e
}
