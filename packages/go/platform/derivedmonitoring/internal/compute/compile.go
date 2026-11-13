package compute

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

func positive(s string) (int64, error) {
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n <= 0 || strconv.FormatInt(n, 10) != s {
		return 0, fmt.Errorf("positive integer required")
	}
	return n, nil
}
func nonnegative(s string) (int64, error) {
	if s == "0" {
		return 0, nil
	}
	return positive(s)
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func calendarValid(zone, period string) bool {
	_, e := time.LoadLocation(zone)
	return zone != "" && e == nil && (period == "day" || period == "month" || period == "year")
}
func knownQuality(s string) bool {
	return s == "good" || s == "unmarked" || s == "suspect" || s == "bad"
}

func Compile(raw []byte, mode string, values map[string]string, limits Limits) (*Plan, error) {
	var rule Rule
	if err := Decode(raw, &rule); err != nil {
		return nil, err
	}
	if limits.Inputs == 0 {
		limits = DefaultLimits()
	}
	if mode != "channel" && mode != "analysis" {
		return nil, issue("invalid_rule", "", "unknown execution mode")
	}
	if rule.SchemaVersion != SchemaVersion {
		return nil, issue("engine_version_unavailable", "/schema_version", "unsupported rule version")
	}
	if len(rule.Inputs) == 0 || len(rule.Inputs) > limits.Inputs || len(rule.Nodes) == 0 || len(rule.Nodes) > limits.Nodes || len(rule.Parameters) > limits.Parameters || len(rule.Outputs) == 0 || len(rule.Outputs) > limits.Outputs {
		return nil, issue("plan_limit_exceeded", "", "rule exceeds input/node/parameter/output limits")
	}
	if mode == "channel" && len(rule.Outputs) != 1 {
		return nil, issue("invalid_rule", "/outputs", "a channel must have exactly one output")
	}
	p := &Plan{Rule: rule, Mode: mode, Values: map[string]string{}, Quantities: map[string]Quantity{}, Times: map[string]TimeSemantics{}, limits: limits, formulas: map[string]*expression{}}
	for i, in := range rule.Inputs {
		path := fmt.Sprintf("/inputs/%d", i)
		ref := "input:" + in.Key
		if _, exists := p.Quantities[ref]; exists || !keyPattern.MatchString(in.Key) {
			return nil, issue("invalid_rule", path+"/key", "unique slot key required")
		}
		if in.Shape != "numeric" {
			return nil, issue("unsupported_input_semantics", path+"/shape", "only numeric input is supported")
		}
		if err := validateQuantity(in.Quantity); err != nil {
			return nil, issue("unit_mismatch", path+"/quantity_spec", err.Error())
		}
		if len(in.SourceKinds) == 0 {
			return nil, issue("invalid_rule", path+"/source_kinds", "explicit permitted source kinds required")
		}
		for _, kind := range in.SourceKinds {
			if kind != "direct" && kind != "derived" {
				return nil, issue("invalid_rule", path+"/source_kinds", "unsupported source kind")
			}
		}
		if in.Direction != "must_match" && in.Direction != "not_applicable" || in.Reference != "must_match" && in.Reference != "not_applicable" {
			return nil, issue("unsupported_input_semantics", path, "explicit direction and reference compatibility required")
		}
		if in.Time.Kind != "instant" && in.Time.Kind != "interval_end" && in.Time.Kind != "derived_window" {
			return nil, issue("unsupported_input_semantics", path+"/time_semantics", "explicit time semantics required")
		}
		if in.Time.Kind != "instant" && !slices.Contains([]string{"count", "sum", "min", "max", "range", "mean", "variance", "stddev", "median", "quantile"}, in.Time.Aggregation) {
			return nil, issue("unsupported_input_semantics", path+"/time_semantics/aggregation", "machine readable aggregation required")
		}
		if in.Time.Kind == "instant" && in.Time.Aggregation != "" {
			return nil, issue("unsupported_input_semantics", path+"/time_semantics/aggregation", "instant input has no aggregation")
		}
		p.Quantities[ref] = in.Quantity
		p.Times[ref] = in.Time
	}
	for i, param := range rule.Parameters {
		path := fmt.Sprintf("/parameters/%d", i)
		ref := "param:" + param.Key
		if _, ok := p.Values[param.Key]; ok || !keyPattern.MatchString(param.Key) {
			return nil, issue("invalid_rule", path+"/key", "unique parameter key required")
		}
		text := param.Default
		if value, ok := values[param.Key]; ok {
			text = value
		}
		value, err := monitoringvalue.ParseDecimal(text)
		if err != nil {
			return nil, issue("invalid_rule", path+"/default", err.Error())
		}
		switch param.Type {
		case "duration", "count":
			if _, err := positive(text); err != nil {
				return nil, issue("invalid_rule", path, "positive integer required")
			}
		case "percentile":
			if value.Sign() < 0 || value.Cmp(big.NewRat(100, 1)) > 0 {
				return nil, issue("invalid_rule", path, "percentile must be 0–100")
			}
		case "decimal":
		default:
			return nil, issue("invalid_rule", path+"/type", "unknown parameter type")
		}
		for _, bound := range []struct {
			text string
			min  bool
		}{{param.Minimum, true}, {param.Maximum, false}} {
			if bound.text != "" {
				r, e := monitoringvalue.ParseDecimal(bound.text)
				if e != nil || bound.min && value.Cmp(r) < 0 || !bound.min && value.Cmp(r) > 0 {
					return nil, issue("invalid_rule", path, "parameter outside declared bounds")
				}
			}
		}
		p.Values[param.Key], _ = monitoringvalue.CanonicalDecimal(text)
		q := scalar()
		if param.Quantity != nil {
			q = *param.Quantity
		}
		if e := validateQuantity(q); e != nil {
			return nil, issue("unit_mismatch", path+"/quantity_spec", e.Error())
		}
		p.Quantities[ref] = q
		p.Times[ref] = TimeSemantics{Kind: "constant"}
	}
	for key := range values {
		if _, ok := p.Values[key]; !ok {
			return nil, issue("invalid_rule", "/parameters/"+pointer(key), "undeclared parameter")
		}
	}
	if err := p.clock(); err != nil {
		return nil, err
	}
	if p.Rule.Policies.ConfigBoundary == "" {
		p.Rule.Policies.ConfigBoundary = "break"
	}
	if p.Rule.Policies.ConfigBoundary != "break" {
		return nil, issue("invalid_rule", "/policies/config_boundary", "only break is supported")
	}
	if len(p.Rule.Policies.Quality) == 0 {
		p.Rule.Policies.Quality = []string{"good", "unmarked"}
	}
	seen := map[string]bool{}
	for _, q := range p.Rule.Policies.Quality {
		if !knownQuality(q) || seen[q] {
			return nil, issue("invalid_rule", "/policies/quality", "unique known quality values required")
		}
		seen[q] = true
	}
	slices.Sort(p.Rule.Policies.Quality)
	if p.Rule.Policies.AllowedLatenessNS == "" {
		p.Rule.Policies.AllowedLatenessNS = "0"
	}
	if _, e := nonnegative(p.Rule.Policies.AllowedLatenessNS); e != nil {
		return nil, issue("invalid_rule", "/policies/allowed_lateness_ns", e.Error())
	}
	for i := range p.Rule.Nodes {
		if err := p.node(i); err != nil {
			return nil, err
		}
	}
	outputKeys := map[string]bool{}
	for i := range p.Rule.Outputs {
		out := &p.Rule.Outputs[i]
		path := fmt.Sprintf("/outputs/%d", i)
		if !keyPattern.MatchString(out.Key) || outputKeys[out.Key] || out.SemanticKey == "" {
			return nil, issue("invalid_rule", path, "unique output key and semantic key required")
		}
		outputKeys[out.Key] = true
		q, ok := p.Quantities["node:"+out.Node]
		if !ok {
			return nil, issue("invalid_rule", path+"/node", "unknown output node")
		}
		if out.Quantity != nil && *out.Quantity != q {
			return nil, issue("unit_mismatch", path+"/quantity_spec", "output declaration differs from inferred unit")
		}
		out.Quantity = &q
		if out.Numeric.Rounding != "half_even" || out.Numeric.Scale < 0 || out.Numeric.Scale > 60 || q.Mode == "count" && out.Numeric.Scale != 0 {
			return nil, issue("invalid_rule", path+"/numeric_contract", "half_even with scale 0–60 required; count scale must be 0")
		}
	}
	// json.Marshal on typed structures and sorted maps is an unambiguous,
	// length-delimited canonical encoding. All numeric literals are normalized.
	canonical := struct {
		Schema, Compiler, Engine, Mode string
		Rule                           Rule
		Parameters                     map[string]string
	}{SchemaVersion, CompilerVersion, EngineVersion, mode, p.Rule, p.Values}
	p.Canonical, _ = json.Marshal(canonical)
	p.PlanHash = digest(canonical)
	p.OutputContractHash = p.semanticHash()
	return p, nil
}
func (p *Plan) clock() error {
	c := &p.Rule.Clock
	if c.StepNS != "" {
		if v, ok := p.Values[strings.TrimPrefix(c.StepNS, "param:")]; strings.HasPrefix(c.StepNS, "param:") && ok {
			c.StepNS = v
		}
	}
	switch c.Kind {
	case "primary_input":
		if _, ok := p.Quantities["input:"+c.Input]; !ok || c.StepNS != "" || c.AnchorNS != "" || c.Timezone != "" || c.Period != "" {
			return issue("invalid_rule", "/clock", "primary input clock requires only a declared input")
		}
	case "fixed_grid":
		step, e := positive(c.StepNS)
		if e != nil || step < 1_000_000_000 {
			return issue("invalid_window", "/clock/step_ns", "grid step must be at least one second")
		}
		if _, e := validNS(c.AnchorNS); e != nil {
			return issue("invalid_window", "/clock/anchor_ns", "anchor nanoseconds required")
		}
		if c.Input != "" || c.Timezone != "" || c.Period != "" {
			return issue("invalid_window", "/clock", "unexpected clock fields")
		}
	case "calendar":
		if !calendarValid(c.Timezone, c.Period) || c.Input != "" || c.StepNS != "" || c.AnchorNS != "" {
			return issue("invalid_window", "/clock", "valid IANA zone and calendar period required")
		}
	case "whole_range":
		if p.Mode != "analysis" {
			return issue("noncausal_rule", "/clock", "whole_range is analysis only")
		}
		if c.Input != "" || c.StepNS != "" || c.AnchorNS != "" || c.Timezone != "" || c.Period != "" {
			return issue("invalid_window", "/clock", "unexpected whole_range fields")
		}
	default:
		return issue("invalid_window", "/clock/kind", "unknown clock")
	}
	return nil
}
func expand(v reflect.Value, values map[string]string) error {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return expand(v.Elem(), values)
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := expand(v.Field(i), values); err != nil {
				return err
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if err := expand(v.Index(i), values); err != nil {
				return err
			}
		}
	case reflect.String:
		if strings.HasPrefix(v.String(), "param:") {
			s, ok := values[strings.TrimPrefix(v.String(), "param:")]
			if !ok {
				return fmt.Errorf("undeclared parameter")
			}
			v.SetString(s)
		}
	}
	return nil
}
func allowedParams(n Node) bool {
	allowed := []string{}
	switch n.Op {
	case "add", "subtract", "multiply", "divide", "min", "max", "weighted_mean", "formula":
		allowed = []string{"alignment", "tolerance_ns"}
	}
	switch n.Op {
	case "weighted_mean":
		allowed = append(allowed, "weights")
	case "formula":
		allowed = append(allowed, "expression", "variables")
	case "baseline_delta", "baseline_ratio":
		allowed = append(allowed, "baseline")
	case "difference", "rate":
		allowed = append(allowed, "difference_mode", "lag_ns", "match_tolerance_ns", "max_gap_ns", "invalid_policy")
		if n.Op == "rate" {
			allowed = append(allowed, "rate_time_unit")
		}
	}
	if strings.HasPrefix(n.Op, "window_") {
		allowed = append(allowed, "window", "min_samples", "max_gap_ns", "max_age_ns")
		if n.Op == "window_quantile" {
			allowed = append(allowed, "percentile")
		}
	}
	b, _ := json.Marshal(n.Params)
	var fields map[string]any
	_ = json.Unmarshal(b, &fields)
	for f := range fields {
		if !slices.Contains(allowed, f) {
			return false
		}
	}
	return true
}
func (p *Plan) node(i int) error {
	n := &p.Rule.Nodes[i]
	path := fmt.Sprintf("/nodes/%d", i)
	ref := "node:" + n.ID
	if _, ok := p.Quantities[ref]; ok || !keyPattern.MatchString(n.ID) {
		return issue("invalid_rule", path+"/id", "unique node id required")
	}
	if !allowedParams(*n) {
		return issue("invalid_rule", path+"/params", "parameter not applicable to this operator")
	}
	// Variable references stay namespaced; only scalar parameter fields expand.
	vars := n.Params.Variables
	n.Params.Variables = nil
	if e := expand(reflect.ValueOf(&n.Params), p.Values); e != nil {
		return issue("invalid_rule", path+"/params", e.Error())
	}
	n.Params.Variables = vars
	inputs := n.Inputs
	if n.Op == "formula" {
		inputs = nil
		keys := make([]string, 0, len(vars))
		for key := range vars {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if !keyPattern.MatchString(key) {
				return issue("invalid_rule", path+"/params/variables", "invalid variable name")
			}
			inputs = append(inputs, vars[key])
		}
		if len(n.Inputs) != 0 {
			return issue("invalid_rule", path+"/inputs", "formula uses explicit variable references")
		}
	}
	if len(inputs) == 0 || len(inputs) > 16 {
		return issue("invalid_rule", path+"/inputs", "operator requires 1–16 inputs")
	}
	quantities := make([]Quantity, len(inputs))
	times := make([]TimeSemantics, len(inputs))
	for j, r := range inputs {
		q, ok := p.Quantities[r]
		if !ok {
			return issue("invalid_rule", path+"/inputs", "input must refer to a declared slot, parameter or preceding node")
		}
		quantities[j] = q
		times[j] = p.Times[r]
	}
	unary := slices.Contains([]string{"abs", "baseline_delta", "baseline_ratio", "difference", "rate"}, n.Op) || strings.HasPrefix(n.Op, "window_")
	if unary && len(inputs) != 1 {
		return issue("invalid_rule", path+"/inputs", "operator requires exactly one input")
	}
	if !unary && n.Op != "formula" && len(inputs) < 2 {
		return issue("invalid_rule", path+"/inputs", "operator requires at least two inputs")
	}
	if slices.Contains([]string{"subtract", "divide"}, n.Op) && len(inputs) != 2 {
		return issue("invalid_rule", path+"/inputs", "operator requires exactly two inputs")
	}
	q := quantities[0]
	tm := times[0]
	var err error
	switch n.Op {
	case "add", "subtract", "multiply", "divide", "min", "max":
		for _, other := range quantities[1:] {
			q, err = quantityBinary(n.Op, q, other)
			if err != nil {
				break
			}
		}
	case "abs", "baseline_delta", "baseline_ratio", "difference", "rate":
		if n.Op == "rate" && n.Params.RateTimeUnit == "" {
			n.Params.RateTimeUnit = "second"
		}
		q, err = quantityUnary(n.Op, q, n.Params)
	case "weighted_mean":
		if len(n.Params.Weights) != len(inputs) {
			return issue("invalid_rule", path+"/params/weights", "one weight per required input")
		}
		total := new(big.Rat)
		for j, w := range n.Params.Weights {
			v, e := monitoringvalue.ParseDecimal(w)
			if e != nil || v.Sign() < 0 {
				return issue("invalid_rule", path+"/params/weights", "nonnegative weights required")
			}
			total.Add(total, v)
			n.Params.Weights[j], _ = monitoringvalue.CanonicalDecimal(w)
			if quantities[j] != q {
				err = fmt.Errorf("weighted inputs must have the same quantity")
			}
		}
		if total.Sign() == 0 {
			return issue("invalid_rule", path+"/params/weights", "positive total weight required")
		}
	case "formula":
		named := map[string]Quantity{}
		for k, r := range vars {
			named[k] = p.Quantities[r]
		}
		f, e := parseExpression(n.Params.Expression, named, p.limits)
		if e != nil {
			return issue("invalid_rule", path+"/params/expression", e.Error())
		}
		p.formulas[n.ID] = f
		n.Params.Expression = f.render()
		q = f.quantity
	case "window_count", "window_sum", "window_min", "window_max", "window_range", "window_mean", "window_variance", "window_stddev", "window_median", "window_quantile":
		if tm.Kind == "interval_end" || tm.Kind == "constant" {
			return issue("unsupported_input_semantics", path, "raw interval results and parameters cannot be windowed")
		}
		if e := p.window(n, path); e != nil {
			return e
		}
		q, err = quantityUnary(n.Op, q, n.Params)
		tm = TimeSemantics{Kind: "derived_window", Aggregation: strings.TrimPrefix(n.Op, "window_")}
	default:
		return issue("invalid_rule", path+"/op", "unsupported operator")
	}
	if err != nil {
		return issue("unit_mismatch", path, err.Error())
	}
	if n.Op == "difference" || n.Op == "rate" {
		if tm.Kind == "interval_end" || tm.Kind == "constant" {
			return issue("unsupported_input_semantics", path, "temporal operator requires a compatible sample sequence")
		}
		if _, e := positive(n.Params.MaxGapNS); e != nil {
			return issue("invalid_window", path+"/params/max_gap_ns", "finite max_gap required")
		}
		if n.Params.DifferenceMode == "" {
			n.Params.DifferenceMode = "adjacent"
		}
		if n.Params.InvalidPolicy == "" {
			n.Params.InvalidPolicy = "break"
		}
		if n.Params.InvalidPolicy != "break" && n.Params.InvalidPolicy != "skip_excluded" {
			return issue("invalid_rule", path+"/params/invalid_policy", "unknown exclusion policy")
		}
		switch n.Params.DifferenceMode {
		case "adjacent":
			if n.Params.LagNS != "" || n.Params.MatchToleranceNS != "" {
				return issue("invalid_rule", path+"/params", "lag options require lag mode")
			}
		case "lag":
			lag, e := positive(n.Params.LagNS)
			tol, te := nonnegative(n.Params.MatchToleranceNS)
			gap, _ := positive(n.Params.MaxGapNS)
			if e != nil || te != nil || lag > gap || tol > gap-lag {
				return issue("invalid_window", path+"/params", "lag plus tolerance must fit max_gap")
			}
		default:
			return issue("invalid_rule", path+"/params/difference_mode", "unknown difference mode")
		}
	}
	if n.Op == "baseline_delta" || n.Op == "baseline_ratio" {
		if e := p.baseline(n, path); e != nil {
			return e
		}
	}
	if !unary {
		if n.Params.Alignment == "" {
			n.Params.Alignment = "exact"
		}
		switch n.Params.Alignment {
		case "exact", "same_interval":
			if n.Params.ToleranceNS != "" {
				return issue("invalid_rule", path+"/params/tolerance_ns", "tolerance only applies to previous/nearest")
			}
		case "previous", "nearest":
			if _, e := nonnegative(n.Params.ToleranceNS); e != nil {
				return issue("invalid_rule", path+"/params/tolerance_ns", "finite tolerance required")
			}
			if n.Params.Alignment == "nearest" && p.Mode != "analysis" {
				return issue("noncausal_rule", path+"/params/alignment", "nearest is historical analysis only")
			}
		default:
			return issue("invalid_rule", path+"/params/alignment", "unknown alignment")
		}
		tm = TimeSemantics{Kind: "instant"}
		for _, t := range times {
			if t.Kind != "constant" {
				tm = t
				break
			}
		}
		for _, t := range times {
			if t.Kind == "constant" {
				continue
			}
			if t.Kind == "interval_end" || tm.Kind == "interval_end" {
				if n.Params.Alignment != "same_interval" || t != tm {
					return issue("unsupported_input_semantics", path, "raw interval combination requires same_interval and identical aggregation")
				}
			}
		}
	}
	p.Quantities[ref] = q
	p.Times[ref] = tm
	return nil
}
