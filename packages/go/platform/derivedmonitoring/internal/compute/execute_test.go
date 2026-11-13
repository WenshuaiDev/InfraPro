package compute

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

const minute = int64(time.Minute)

func displacement() Quantity {
	return Quantity{BaseUnitVersionID: "millimetre-version", BasePower: 1, Mode: "level", LevelScale: "ratio"}
}
func input(key string) Input {
	return Input{Key: key, Shape: "numeric", SourceKinds: []string{"direct", "derived"}, Quantity: displacement(), Direction: "must_match", Reference: "must_match", Time: TimeSemantics{Kind: "instant"}}
}
func baseRule(nodes ...Node) Rule {
	return Rule{SchemaVersion: SchemaVersion, Inputs: []Input{input("A"), input("B")}, Clock: Clock{Kind: "primary_input", Input: "A"}, Nodes: nodes, Outputs: []Output{{Key: "value", Node: nodes[len(nodes)-1].ID, SemanticKey: "fixture", Numeric: NumericContract{Scale: 18, Rounding: "half_even"}}}}
}
func fixture(t *testing.T) Data {
	t.Helper()
	d := Data{Inputs: map[string][]Sample{}, Complete: true}
	for key, values := range map[string][]string{"A": {"10", "14", "20"}, "B": {"6", "9", "11"}} {
		for i, s := range values {
			n, e := monitoringvalue.Decimal(s)
			if e != nil {
				t.Fatal(e)
			}
			d.Inputs[key] = append(d.Inputs[key], Sample{At: []int64{0, 10 * minute, 30 * minute}[i], Value: n, Quality: "good", ID: key + s})
		}
	}
	return d
}
func compiled(t *testing.T, r Rule, mode string) *Plan {
	t.Helper()
	b, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	p, e := Compile(b, mode, nil, DefaultLimits())
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func run(t *testing.T, r Rule, d Data, mode string, opts ...RunOptions) []Result {
	t.Helper()
	o := RunOptions{OutputRange: TimeRange{0, 31 * minute}, NowNS: 31 * minute}
	if len(opts) > 0 {
		o = opts[0]
	}
	out := []Result{}
	e := Run(t.Context(), compiled(t, r, mode), d, o, func(v Result) error { out = append(out, v); return nil })
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func values(rows []Result) []string {
	out := []string{}
	for _, r := range rows {
		if r.Value == nil {
			out = append(out, r.Status)
		} else {
			out = append(out, *r.Value)
		}
	}
	return out
}
func expect(t *testing.T, got []Result, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(values(got), want) {
		t.Fatalf("values=%v want=%v", values(got), want)
	}
}
func rolling(op, id, ref string) Node {
	return Node{ID: id, Op: "window_" + op, Inputs: []string{ref}, Params: Params{Window: &Window{Kind: "trailing_duration", DurationNS: "1800000000000"}, MinSamples: "2", MaxGapNS: "1800000000000", MaxAgeNS: "1800000000000"}}
}
func difference(op, id, ref string) Node {
	return Node{ID: id, Op: op, Inputs: []string{ref}, Params: Params{MaxGapNS: "1800000000000", RateTimeUnit: "minute"}}
}
func TestAcceptanceArithmeticAndOrder(t *testing.T) {
	d := fixture(t)
	q := displacement()
	base := Node{ID: "baseline", Op: "baseline_delta", Inputs: []string{"input:A"}, Params: Params{Baseline: &Baseline{Kind: "constant", Value: "10", Quantity: &q}}}
	expect(t, run(t, baseRule(base), d, "channel"), "0.000000000000000000", "4.000000000000000000", "10.000000000000000000")
	adjacent := difference("difference", "delta", "input:A")
	adjacent.Params.RateTimeUnit = ""
	expect(t, run(t, baseRule(adjacent), d, "channel"), "missing_input", "4.000000000000000000", "6.000000000000000000")
	rate := difference("rate", "rate", "input:A")
	expect(t, run(t, baseRule(rate), d, "channel"), "missing_input", "0.400000000000000000", "0.300000000000000000")
	subtract := Node{ID: "delta", Op: "subtract", Inputs: []string{"input:A", "input:B"}}
	expect(t, run(t, baseRule(subtract), d, "channel"), "4.000000000000000000", "5.000000000000000000", "9.000000000000000000")
	tests := []struct{ op, want string }{{"count", "2"}, {"sum", "34.000000000000000000"}, {"min", "14.000000000000000000"}, {"max", "20.000000000000000000"}, {"range", "6.000000000000000000"}, {"mean", "17.000000000000000000"}, {"variance", "9.000000000000000000"}, {"stddev", "3.000000000000000000"}, {"median", "17.000000000000000000"}, {"quantile", "19.700000000000000000"}}
	for _, tc := range tests {
		t.Run(tc.op, func(t *testing.T) {
			n := rolling(tc.op, "stat", "input:A")
			r := baseRule(n)
			if tc.op == "count" {
				r.Nodes[0].Params.MinSamples = "0"
				r.Outputs[0].Numeric.Scale = 0
			}
			if tc.op == "quantile" {
				r.Nodes[0].Params.Percentile = "95"
			}
			got := run(t, r, d, "channel")
			expect(t, got[2:], tc.want)
			if got[2].Count != 2 {
				t.Fatal(got[2])
			}
		})
	}
	mean := rolling("mean", "mean", "input:A")
	expect(t, run(t, baseRule(mean, difference("rate", "rate", "node:mean")), d, "channel")[2:], "0.250000000000000000")
	expect(t, run(t, baseRule(rate, rolling("mean", "mean", "node:rate")), d, "channel")[2:], "0.350000000000000000")
	expect(t, run(t, baseRule(subtract, rolling("mean", "mean", "node:delta")), d, "channel")[2:], "7.000000000000000000")
	weighted := Node{ID: "weighted", Op: "weighted_mean", Inputs: []string{"input:A", "input:B"}, Params: Params{Weights: []string{"1", "3"}}}
	expect(t, run(t, baseRule(weighted), d, "channel"), "7.000000000000000000", "10.250000000000000000", "13.250000000000000000")
	formula := Node{ID: "formula", Op: "formula", Params: Params{Variables: map[string]string{"a": "input:A", "b": "input:B"}, Expression: "abs(a - b) / 2"}}
	expect(t, run(t, baseRule(formula), d, "channel"), "2.000000000000000000", "2.500000000000000000", "4.500000000000000000")
}
func TestPrecisionAndStatus(t *testing.T) {
	d := fixture(t)
	d.Inputs["A"] = d.Inputs["A"][:2]
	d.Inputs["A"][1].Value, _ = monitoringvalue.Decimal("10.000000000001")
	r := baseRule(rolling("variance", "variance", "input:A"))
	r.Outputs[0].Numeric.Scale = 36
	expect(t, run(t, r, d, "channel"), "insufficient_samples", "0.000000000000000000000000250000000000")
	r.Outputs[0].Numeric.Scale = 18
	expect(t, run(t, r, d, "channel"), "insufficient_samples", "numeric_underflow")
	r = baseRule(rolling("stddev", "stddev", "input:A"))
	expect(t, run(t, r, d, "channel"), "insufficient_samples", "0.000000000000500000")
	r = baseRule(Node{ID: "ratio", Op: "divide", Inputs: []string{"input:A", "input:B"}})
	d.Inputs["B"][0].Value = monitoringvalue.Integer(0)
	got := run(t, r, d, "channel")
	if got[0].Status != "division_by_zero" || got[0].Value != nil {
		t.Fatal(got[0])
	}
}
func TestMatchingUsesActualTimesAndPrimaryClock(t *testing.T) {
	d := fixture(t)
	d.Inputs["A"] = d.Inputs["A"][1:2]
	d.Inputs["B"] = []Sample{{At: 9 * minute, Value: monitoringvalue.Integer(1), Quality: "good", ID: "early"}, {At: 11 * minute, Value: monitoringvalue.Integer(9), Quality: "good", ID: "late"}}
	n := Node{ID: "delta", Op: "subtract", Inputs: []string{"input:B", "input:A"}, Params: Params{Alignment: "nearest", ToleranceNS: "60000000000"}}
	got := run(t, baseRule(n), d, "analysis")
	expect(t, got, "-13.000000000000000000")
	if got[0].Matches[0].ID != "early" || got[0].Matches[0].OffsetNS != "-60000000000" {
		t.Fatal(got[0].Matches)
	}
	n.Params.Alignment = "previous"
	expect(t, run(t, baseRule(n), d, "channel"), "-13.000000000000000000")
	n.Params.ToleranceNS = "59999999999"
	expect(t, run(t, baseRule(n), d, "channel"), "missing_input")
	n.Params.Alignment = "exact"
	n.Params.ToleranceNS = ""
	expect(t, run(t, baseRule(n), d, "channel"), "missing_input")
}
func TestExclusionsGapsAgeAndConfigurationBoundaries(t *testing.T) {
	d := fixture(t)
	d.Inputs["A"][1].Quality = "bad"
	r := baseRule(difference("rate", "rate", "input:A"))
	expect(t, run(t, r, d, "channel"), "missing_input", "invalid_input", "missing_input")
	r.Nodes[0].Params.InvalidPolicy = "skip_excluded"
	expect(t, run(t, r, d, "channel"), "missing_input", "invalid_input", "0.333333333333333333")
	r.Nodes[0].Params.MaxGapNS = "1799999999999"
	expect(t, run(t, r, d, "channel"), "missing_input", "invalid_input", "gap_exceeded")
	d = fixture(t)
	d.Inputs["A"][0].ConfigID = "old"
	d.Inputs["A"][1].ConfigID = "new"
	d.Inputs["A"][2].ConfigID = "new"
	r = baseRule(difference("rate", "rate", "input:A"))
	expect(t, run(t, r, d, "channel"), "missing_input", "configuration_boundary", "0.300000000000000000")
	d = fixture(t)
	d.Inputs["A"][1].Quality = "suspect"
	r = baseRule(rolling("mean", "mean", "input:A"))
	r.Policies.Quality = []string{"good", "suspect"}
	got := run(t, r, d, "channel")
	if got[2].Quality != "suspect" {
		t.Fatal(got[2])
	}
}
func TestFixedBaselinesEvidenceAndCausality(t *testing.T) {
	d := fixture(t)
	d.Baselines = map[string]FixedBaseline{"reference": {Value: monitoringvalue.Integer(10), At: 10 * minute, Valid: true, Proven: true, Quality: "good", ID: "fixed"}}
	r := baseRule(Node{ID: "baseline", Op: "baseline_delta", Inputs: []string{"input:A"}, Params: Params{Baseline: &Baseline{Kind: "observation", Reference: "reference"}}})
	expect(t, run(t, r, d, "channel"), "noncausal_baseline", "4.000000000000000000", "10.000000000000000000")
	expect(t, run(t, r, d, "analysis"), "0.000000000000000000", "4.000000000000000000", "10.000000000000000000")
	b := d.Baselines["reference"]
	b.Valid = false
	d.Baselines["reference"] = b
	expect(t, run(t, r, d, "channel"), "baseline_invalid", "baseline_invalid", "baseline_invalid")
	b.Proven = false
	d.Baselines["reference"] = b
	e := Run(t.Context(), compiled(t, r, "channel"), d, RunOptions{OutputRange: TimeRange{0, 31 * minute}, NowNS: 31 * minute}, func(Result) error { return nil })
	if e == nil || !strings.Contains(e.Error(), "baseline_evidence_unavailable") {
		t.Fatal(e)
	}
}
func TestPartitionAndCancellation(t *testing.T) {
	d := fixture(t)
	r := baseRule(rolling("mean", "mean", "input:A"), difference("rate", "rate", "node:mean"))
	want := run(t, r, d, "channel")
	var parts []Result
	for _, bounds := range []TimeRange{{0, 15 * minute}, {15 * minute, 31 * minute}} {
		parts = append(parts, run(t, r, d, "channel", RunOptions{OutputRange: bounds, NowNS: 31 * minute})...)
	}
	if !reflect.DeepEqual(want, parts) {
		t.Fatal("halo changed results", want, parts)
	}
	expected := errors.New("cancel requested by sink")
	e := Run(t.Context(), compiled(t, r, "channel"), d, RunOptions{OutputRange: TimeRange{0, 31 * minute}, NowNS: 31 * minute}, func(Result) error { return expected })
	if !errors.Is(e, expected) {
		t.Fatal(e)
	}
	d.Complete = false
	e = Run(t.Context(), compiled(t, r, "channel"), d, RunOptions{OutputRange: TimeRange{0, 31 * minute}}, func(Result) error { t.Fatal("incomplete input emitted output"); return nil })
	if e == nil {
		t.Fatal("accepted incomplete input")
	}
}

func TestEmptyInputIsAValidWaitingState(t *testing.T) {
	d := Data{Complete: true, Inputs: map[string][]Sample{"A": nil, "B": nil}}
	r := baseRule(Node{ID: "delta", Op: "subtract", Inputs: []string{"input:A", "input:B"}})
	if got := run(t, r, d, "channel"); len(got) != 0 {
		t.Fatal(got)
	}
	n := rolling("count", "count", "input:A")
	n.Params.Window = &Window{Kind: "fixed_duration", AnchorNS: "0", DurationNS: "600000000000"}
	n.Params.MinSamples = "0"
	r = baseRule(n)
	r.Clock = Clock{Kind: "fixed_grid", AnchorNS: "0", StepNS: "600000000000"}
	r.Outputs[0].Numeric.Scale = 0
	expect(t, run(t, r, d, "channel"), "0", "0", "0", "0")
}

func TestNegativeBaselineLagAndMissingWeightedInputs(t *testing.T) {
	d := fixture(t)
	q := displacement()
	r := baseRule(Node{ID: "baseline", Op: "baseline_ratio", Inputs: []string{"input:A"}, Params: Params{Baseline: &Baseline{Kind: "constant", Value: "-10", Quantity: &q}}})
	expect(t, run(t, r, d, "channel"), "-2.000000000000000000", "-2.400000000000000000", "-3.000000000000000000")
	n := difference("rate", "rate", "input:A")
	n.Params.DifferenceMode = "lag"
	n.Params.LagNS = "1200000000000"
	n.Params.MatchToleranceNS = "600000000000"
	expect(t, run(t, baseRule(n), d, "channel"), "missing_input", "missing_input", "0.300000000000000000")
	d.Inputs["B"] = d.Inputs["B"][:2]
	weighted := Node{ID: "mean", Op: "weighted_mean", Inputs: []string{"input:A", "input:B"}, Params: Params{Weights: []string{"1", "0"}}}
	expect(t, run(t, baseRule(weighted), d, "channel"), "10.000000000000000000", "14.000000000000000000", "missing_input")
}
func TestRawIntervalMatchingDoesNotGuessAggregation(t *testing.T) {
	d := fixture(t)
	r := baseRule(Node{ID: "delta", Op: "subtract", Inputs: []string{"input:A", "input:B"}, Params: Params{Alignment: "same_interval"}})
	for i := range r.Inputs {
		r.Inputs[i].Time = TimeSemantics{Kind: "interval_end", Aggregation: "mean"}
	}
	for key, rows := range d.Inputs {
		for i := range rows {
			rows[i].Interval = &TimeRange{rows[i].At - minute, rows[i].At}
		}
		d.Inputs[key] = rows
	}
	d.Inputs["B"][1].Interval = &TimeRange{8 * minute, 10 * minute}
	expect(t, run(t, r, d, "channel"), "4.000000000000000000", "invalid_input", "9.000000000000000000")
}

func TestGridFormulaUsesDeclaredTimelineAndNearestHalo(t *testing.T) {
	r := baseRule(Node{ID: "formula", Op: "formula", Params: Params{Expression: "x * 2", Variables: map[string]string{"x": "input:A"}, Alignment: "nearest", ToleranceNS: "30000000000"}})
	r.Inputs = r.Inputs[:1]
	r.Clock = Clock{Kind: "fixed_grid", AnchorNS: "0", StepNS: "60000000000"}
	data := Data{Complete: true, Inputs: map[string][]Sample{"A": {{At: minute / 2, Value: monitoringvalue.Integer(2), Quality: "good", ID: "future", ConfigID: "old"}, {At: minute + minute/2, Value: monitoringvalue.Integer(5), Quality: "good", ID: "later", ConfigID: "new"}}}}
	got := run(t, r, data, "analysis", RunOptions{OutputRange: TimeRange{0, 2 * minute}, SourceRange: TimeRange{-minute / 2, 3 * minute}, NowNS: 3 * minute})
	expect(t, got, "4.000000000000000000", "4.000000000000000000")
	if got[0].Matches[0].OffsetNS != "30000000000" || got[1].Matches[0].ID != "future" {
		t.Fatal(got)
	}
	r.Nodes[0].Params.Alignment = "previous"
	got = run(t, r, data, "channel", RunOptions{OutputRange: TimeRange{0, 2 * minute}, SourceRange: TimeRange{-minute / 2, 2 * minute}, NowNS: 3 * minute})
	expect(t, got, "missing_input", "4.000000000000000000")
}
