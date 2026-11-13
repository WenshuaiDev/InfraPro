package compute

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"math"
	"strings"
	"testing"
	"time"
)

func TestCalendarDSTLeapAndDomain(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	for _, tc := range []struct {
		year       int
		month      time.Month
		day, hours int
	}{{2024, 3, 10, 23}, {2024, 11, 3, 25}, {2024, 2, 29, 24}} {
		start := time.Date(tc.year, tc.month, tc.day, 0, 0, 0, 0, loc)
		end := start.AddDate(0, 0, 1)
		window, _, e := windowRange(Window{Kind: "calendar", Timezone: "America/New_York", Period: "day"}, end.UnixNano(), TimeRange{})
		if e != nil || window.From != start.UnixNano() || (window.To-window.From)/int64(time.Hour) != int64(tc.hours) {
			t.Fatal(window, e)
		}
		times, e := GridTimes(Clock{Kind: "calendar", Timezone: "America/New_York", Period: "day"}, TimeRange{start.UnixNano(), end.Add(time.Nanosecond).UnixNano()}, 10)
		if e != nil || len(times) != 2 || times[1] != end.UnixNano() {
			t.Fatal(times, e)
		}
	}
	times, e := GridTimes(Clock{Kind: "fixed_grid", AnchorNS: "-1000000000", StepNS: "1000000000"}, TimeRange{-500000000, 1500000001}, 10)
	if e != nil || len(times) != 2 || times[0] != 0 || times[1] != 1_000_000_000 {
		t.Fatal(times, e)
	}
	times, e = GridTimes(Clock{Kind: "fixed_grid", AnchorNS: "0", StepNS: "1000000000"}, TimeRange{math.MaxInt64 - 2_000_000_000, math.MaxInt64}, 10)
	if e != nil || len(times) != 2 {
		t.Fatal(times, e)
	}
}
func TestFixedEmptyAndSampleWindows(t *testing.T) {
	d := fixture(t)
	n := rolling("count", "count", "input:A")
	n.Params.Window = &Window{Kind: "fixed_duration", AnchorNS: "0", DurationNS: "600000000000"}
	n.Params.MinSamples = "0"
	r := baseRule(n)
	r.Clock = Clock{Kind: "fixed_grid", AnchorNS: "0", StepNS: "600000000000"}
	r.Outputs[0].Numeric.Scale = 0
	got := run(t, r, d, "channel", RunOptions{OutputRange: TimeRange{10 * minute, 31 * minute}, SourceRange: TimeRange{10 * minute, 31 * minute}, NowNS: 31 * minute})
	expect(t, got, "1", "1", "0")
	if got[2].Quality != "unmarked" || !strings.Contains(strings.Join(got[2].Reasons, ","), "empty_window") {
		t.Fatal(got[2])
	}
	// Window [20,30) excludes the sample exactly at 30, and an empty window does
	// not compare a fictitious last sample with max_age.
	r.Nodes[0].Params.MaxAgeNS = "1"
	got = run(t, r, d, "channel", RunOptions{OutputRange: TimeRange{30 * minute, 31 * minute}, SourceRange: TimeRange{30 * minute, 31 * minute}, NowNS: 31 * minute})
	expect(t, got, "0")
	n = rolling("mean", "mean", "input:A")
	n.Params.Window = &Window{Kind: "trailing_count", Count: "2", MaxSpanNS: "1800000000000"}
	r = baseRule(n)
	expect(t, run(t, r, d, "channel"), "insufficient_samples", "12.000000000000000000", "17.000000000000000000")
	n.Params.Window.MaxSpanNS = "600000000000"
	r = baseRule(n)
	expect(t, run(t, r, d, "channel"), "insufficient_samples", "12.000000000000000000", "gap_exceeded")
}
func TestWholeRangeAndFutureSchedulingBoundary(t *testing.T) {
	d := fixture(t)
	n := rolling("mean", "mean", "input:A")
	n.Params.Window = &Window{Kind: "whole_range"}
	r := baseRule(n)
	r.Clock = Clock{Kind: "whole_range"}
	got := run(t, r, d, "analysis", RunOptions{OutputRange: TimeRange{0, 31 * minute}, SourceRange: TimeRange{0, 31 * minute}})
	expect(t, got, "14.666666666666666667")
	if got[0].At != 31*minute || got[0].Interval == nil || got[0].Interval.To != 31*minute {
		t.Fatal(got[0])
	}
	r = baseRule(Node{ID: "delta", Op: "subtract", Inputs: []string{"input:A", "input:B"}})
	r.Policies.AllowedLatenessNS = "1"
	got = run(t, r, d, "channel", RunOptions{OutputRange: TimeRange{0, 31 * minute}, NowNS: 30 * minute})
	if len(got) != 2 {
		t.Fatal("future output emitted", got)
	}
}
func TestRepeatedQuantileRanksAndSingleSampleVariance(t *testing.T) {
	d := fixture(t)
	for i := range d.Inputs["A"] {
		d.Inputs["A"][i].Value = monitoringvalue.Integer(7)
	}
	for _, percentile := range []string{"0", "50", "95", "100"} {
		n := rolling("quantile", "q", "input:A")
		n.Params.Percentile = percentile
		expect(t, run(t, baseRule(n), d, "channel")[2:], "7.000000000000000000")
	}
	n := rolling("variance", "v", "input:A")
	n.Params.MinSamples = "1"
	expect(t, run(t, baseRule(n), d, "channel"), "0.000000000000000000", "0.000000000000000000", "0.000000000000000000")
}

func TestExtremaTiesChooseEarliestTime(t *testing.T) {
	d := fixture(t)
	for i := range d.Inputs["A"] {
		d.Inputs["A"][i].Value = monitoringvalue.Integer(7)
	}
	got := run(t, baseRule(rolling("max", "max", "input:A")), d, "channel")
	last := got[2]
	if last.MinAt == nil || last.MaxAt == nil || *last.MinAt != 10*minute || *last.MaxAt != 10*minute || last.MinTies != 2 || last.MaxTies != 2 {
		t.Fatal(last)
	}
}

func TestMaximalNanosecondIsPublishableWithoutOverflow(t *testing.T) {
	d := Data{Complete: true, Inputs: map[string][]Sample{"A": {{At: math.MaxInt64, Value: monitoringvalue.Integer(10), Quality: "good"}}, "B": {{At: math.MaxInt64, Value: monitoringvalue.Integer(6), Quality: "good"}}}}
	r := baseRule(Node{ID: "delta", Op: "subtract", Inputs: []string{"input:A", "input:B"}})
	got := run(t, r, d, "channel", RunOptions{OutputRange: TimeRange{math.MaxInt64, math.MaxInt64}, IncludeMax: true, NowNS: math.MaxInt64})
	expect(t, got, "4.000000000000000000")
	if got[0].At != math.MaxInt64 {
		t.Fatal(got)
	}
}
