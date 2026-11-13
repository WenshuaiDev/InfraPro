package compute

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func ruleJSON(t *testing.T, r Rule) []byte {
	t.Helper()
	b, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestStrictRuleAndReferenceRejection(t *testing.T) {
	r := baseRule(Node{ID: "delta", Op: "subtract", Inputs: []string{"input:A", "input:B"}})
	raw := string(ruleJSON(t, r))
	for _, tc := range []struct{ name, raw, code string }{
		{"unknown", strings.Replace(raw, `"schema_version"`, `"unknown"`, 1), "unknown_field"},
		{"case alias", strings.Replace(raw, `"schema_version"`, `"Schema_version"`, 1), "unknown_field"},
		{"duplicate", strings.Replace(raw, `"schema_version":`, `"schema_version":"derived-rule/1","schema_version":`, 1), "duplicate_field"},
		{"trailing", raw + `{}`, "malformed_request"},
		{"future", strings.Replace(raw, `derived-rule/1`, `derived-rule/2`, 1), "engine_version_unavailable"},
		{"cycle", strings.Replace(raw, `input:A`, `node:delta`, 1), "invalid_rule"},
		{"null", strings.Replace(raw, `"name"`, `"name"`, 1), ""},
	} {
		if tc.code == "" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			_, e := Compile([]byte(tc.raw), "channel", nil, DefaultLimits())
			var got *Issue
			if !errors.As(e, &got) || got.Code != tc.code {
				t.Fatal(e)
			}
			if got.Path == "" && tc.code != "malformed_request" {
				t.Fatal("missing field pointer", e)
			}
		})
	}
	var decoded Rule
	if e := Decode([]byte(`{"inputs":null}`), &decoded); e == nil {
		t.Fatal("accepted null")
	}
}
func TestUnitAndCausalContracts(t *testing.T) {
	for _, tc := range []struct {
		op, scale string
		valid     bool
	}{{"add", "affine", false}, {"subtract", "affine", true}, {"divide", "affine", false}, {"abs", "unknown", false}, {"window_sum", "unknown", false}, {"window_mean", "affine", true}, {"window_stddev", "unknown", true}} {
		t.Run(tc.op+tc.scale, func(t *testing.T) {
			n := Node{ID: "result", Op: tc.op, Inputs: []string{"input:A", "input:B"}}
			if tc.op == "abs" {
				n.Inputs = n.Inputs[:1]
			}
			if strings.HasPrefix(tc.op, "window_") {
				n = rolling(strings.TrimPrefix(tc.op, "window_"), "result", "input:A")
			}
			r := baseRule(n)
			for i := range r.Inputs {
				r.Inputs[i].Quantity.LevelScale = tc.scale
			}
			_, e := Compile(ruleJSON(t, r), "channel", nil, DefaultLimits())
			if (e == nil) != tc.valid {
				t.Fatal(e)
			}
		})
	}
	r := baseRule(Node{ID: "delta", Op: "subtract", Inputs: []string{"input:A", "input:B"}, Params: Params{Alignment: "nearest", ToleranceNS: "10"}})
	if _, e := Compile(ruleJSON(t, r), "channel", nil, DefaultLimits()); e == nil {
		t.Fatal("accepted noncausal sustained rule")
	}
	_ = compiled(t, r, "analysis")
	r.Nodes[0].Params = Params{}
	r.Inputs[1].Quantity.BaseUnitVersionID = "different-unit-same-name"
	if _, e := Compile(ruleJSON(t, r), "channel", nil, DefaultLimits()); e == nil {
		t.Fatal("implicit unit conversion")
	}
	r = baseRule(rolling("mean", "mean", "input:A"))
	r.Inputs[0].Time = TimeSemantics{Kind: "interval_end", Aggregation: "mean"}
	if _, e := Compile(ruleJSON(t, r), "channel", nil, DefaultLimits()); e == nil {
		t.Fatal("aggregated raw time window accepted")
	}
}
func TestFormulaWhitelistAndBudget(t *testing.T) {
	for _, expr := range []string{"a.value", "a[0]", "a=2", "a>2", "a?2:3", "sum(a)", "abs(a,b)", "min(a)", "1e10*a", "unknown+a", "a;system()", strings.Repeat("(", 17) + "a" + strings.Repeat(")", 17), strings.Repeat("a+", 20) + "a"} {
		r := baseRule(Node{ID: "f", Op: "formula", Params: Params{Expression: expr, Variables: map[string]string{"a": "input:A", "b": "input:B"}}})
		if _, e := Compile(ruleJSON(t, r), "channel", nil, DefaultLimits()); e == nil {
			t.Fatal("accepted", expr)
		}
	}
	r := baseRule(Node{ID: "f", Op: "formula", Params: Params{Expression: "max(abs(a-b),abs(b-a))", Variables: map[string]string{"a": "input:A", "b": "input:B"}}})
	_ = compiled(t, r, "channel")
	r.Nodes[0].Params.MaxAgeNS = "12"
	if _, e := Compile(ruleJSON(t, r), "channel", nil, DefaultLimits()); e == nil {
		t.Fatal("ignored irrelevant parameter")
	}
}
func TestParametersExpandedAndHashesDeterministic(t *testing.T) {
	r := baseRule(rolling("mean", "mean", "input:A"))
	r.Parameters = []Parameter{{Key: "window", Type: "duration", Default: "1800000000000", IdentityAffecting: true}}
	r.Nodes[0].Params.Window.DurationNS = "param:window"
	p := compiled(t, r, "channel")
	if p.Rule.Nodes[0].Params.Window.DurationNS != "1800000000000" {
		t.Fatal("parameter not expanded")
	}
	raw := ruleJSON(t, r)
	var untyped map[string]any
	_ = json.Unmarshal(raw, &untyped)
	reordered, _ := json.Marshal(untyped)
	other, e := Compile(reordered, "channel", nil, DefaultLimits())
	if e != nil || p.PlanHash != other.PlanHash {
		t.Fatal("JSON key order changed canonical hash", e)
	}
	if _, e := Compile(raw, "channel", map[string]string{"unknown": "1"}, DefaultLimits()); e == nil {
		t.Fatal("unknown binding accepted")
	}
	if _, e := Compile(raw, "channel", map[string]string{"window": "0"}, DefaultLimits()); e == nil {
		t.Fatal("zero duration accepted")
	}
}

func TestSemanticIdentityWhitelist(t *testing.T) {
	r := baseRule(rolling("mean", "mean", "input:A"))
	a := compiled(t, r, "channel")
	r.Outputs[0].Numeric.Scale = 36
	r.Nodes[0].Params.MaxGapNS = "60000000000"
	r.Policies.Quality = []string{"good"}
	b := compiled(t, r, "channel")
	if a.PlanHash == b.PlanHash || a.OutputContractHash != b.OutputContractHash {
		t.Fatal("ordinary configuration changed identity or escaped plan hash")
	}
	r.Nodes[0].Params.Window.DurationNS = "86400000000000"
	c := compiled(t, r, "channel")
	if b.OutputContractHash == c.OutputContractHash {
		t.Fatal("24-hour window reused 30-minute identity")
	}
	r = baseRule(Node{ID: "f", Op: "formula", Params: Params{Expression: "a - b", Variables: map[string]string{"a": "input:A", "b": "input:B"}}})
	a = compiled(t, r, "channel")
	r.Nodes[0].Params.Expression = "(a-b)"
	b = compiled(t, r, "channel")
	if a.PlanHash != b.PlanHash {
		t.Fatal("formula whitespace changed canonical plan")
	}
	r.Nodes[0].Params.Expression = "a + b"
	c = compiled(t, r, "channel")
	if b.OutputContractHash == c.OutputContractHash {
		t.Fatal("formula change reused semantic identity")
	}
}
