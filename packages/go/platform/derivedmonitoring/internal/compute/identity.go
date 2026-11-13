package compute

import "encoding/json"

// semanticHash removes only the documented ordinary configuration whitelist.
// Window extent, formulas, input slot semantics, fixed baselines and identity
// parameters remain part of the immutable channel output contract. Concrete
// input owner identities are added by the binding service, never guessed here.
func (p *Plan) semanticHash() string {
	b, _ := json.Marshal(p.Rule)
	var rule Rule
	_ = json.Unmarshal(b, &rule)
	rule.Policies = Policies{}
	rule.Clock.StepNS = ""
	for i := range rule.Inputs {
		rule.Inputs[i].Name = ""
	}
	identityValues := map[string]string{}
	for i := range rule.Parameters {
		v := &rule.Parameters[i]
		if v.IdentityAffecting {
			identityValues[v.Key] = p.Values[v.Key]
		}
		v.Default = ""
		v.Minimum = ""
		v.Maximum = ""
	}
	for i := range rule.Nodes {
		params := &rule.Nodes[i].Params
		params.ToleranceNS = ""
		params.MatchToleranceNS = ""
		params.MaxGapNS = ""
		params.MaxAgeNS = ""
		params.MinSamples = ""
		params.InvalidPolicy = ""
	}
	for i := range rule.Outputs {
		rule.Outputs[i].Name = ""
		rule.Outputs[i].Numeric = NumericContract{}
	}
	return digest(struct {
		Version string
		Rule    Rule
		Values  map[string]string
	}{"derived-output-contract/1", rule, identityValues})
}

// CorrectionIdentity is used only for an explicit, audited assertion of the
// same business intent. It permits an algorithm correction while preserving
// physical meaning, input slots, window extents, clocks and fixed baselines.
// Ordinary edits MUST continue to compare OutputContractHash instead.
func (p *Plan) CorrectionIdentity() string {
	b, _ := json.Marshal(p.Rule)
	var rule Rule
	_ = json.Unmarshal(b, &rule)
	constraints := []any{}
	for _, node := range rule.Nodes {
		if node.Params.Window != nil || node.Params.Baseline != nil || node.Params.RateTimeUnit != "" {
			constraints = append(constraints, []any{node.Params.Window, node.Params.Baseline, node.Params.RateTimeUnit})
		}
	}
	rule.Nodes = nil
	rule.Policies = Policies{}
	rule.Clock.StepNS = ""
	for i := range rule.Inputs {
		rule.Inputs[i].Name = ""
	}
	for i := range rule.Outputs {
		rule.Outputs[i].Node = ""
		rule.Outputs[i].Name = ""
		rule.Outputs[i].Numeric = NumericContract{}
	}
	values := map[string]string{}
	for i := range rule.Parameters {
		v := &rule.Parameters[i]
		if v.IdentityAffecting {
			values[v.Key] = p.Values[v.Key]
		}
		v.Default = ""
		v.Minimum = ""
		v.Maximum = ""
	}
	return digest([]any{"derived-correction-intent/1", rule, constraints, values})
}
