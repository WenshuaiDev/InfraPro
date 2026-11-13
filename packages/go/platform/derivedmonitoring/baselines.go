package derivedmonitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/exactsort"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

type fixedBaseline struct {
	Reference   string                 `json:"reference"`
	Snapshot    monitoring.RawSnapshot `json:"snapshot"`
	Needs       []monitoring.RawNeed   `json:"input_ranges"`
	Numerator   string                 `json:"numerator"`
	Denominator string                 `json:"denominator"`
	At          int64                  `json:"at_ns,string"`
	Quality     string                 `json:"quality"`
	Count       uint64                 `json:"count,string"`
	Excluded    uint64                 `json:"excluded_count,string"`
	Observation string                 `json:"observation_id,omitempty"`
	Adoption    string                 `json:"adoption_id,omitempty"`
	Result      string                 `json:"result_id,omitempty"`
	Generation  string                 `json:"generation_id,omitempty"`
}
type BaselineProof struct {
	Reference string `json:"reference"`
	Snapshot  string `json:"snapshot_id"`
	Hash      string `json:"evidence_hash"`
	Valid     bool   `json:"valid"`
}

func validateBaselineBindings(p *compute.Plan, bindings map[string]BaselineBinding) error {
	expected := map[string]compute.Baseline{}
	for _, n := range p.Rule.Nodes {
		b := n.Params.Baseline
		if b == nil || b.Kind == "constant" {
			continue
		}
		path := "/baseline_bindings/" + b.Reference
		if old, ok := expected[b.Reference]; ok && fingerprint(old) != fingerprint(b) {
			return invalid(path, "同一基准引用必须使用同一选择规则。")
		}
		expected[b.Reference] = *b
		binding, ok := bindings[b.Reference]
		if !ok {
			return invalid(path, "需要显式选择基准来源。")
		}
		q, exists := p.Quantities["input:"+binding.Input]
		if !exists || q != p.Quantities[n.Inputs[0]] {
			return invalid(path+"/input", "基准输入必须具有相同单位及数值语义。")
		}
		if b.Kind == "observation" {
			at, e := strconv.ParseInt(binding.AtNS, 10, 64)
			raw := access.ValidID(binding.ObservationID) && access.ValidID(binding.AdoptionID) && binding.ResultID == "" && binding.GenerationID == ""
			derived := access.ValidID(binding.ResultID) && access.ValidID(binding.GenerationID) && binding.ObservationID == "" && binding.AdoptionID == ""
			if e != nil || at < monitoringvalue.MinTime.UnixNano() || strconv.FormatInt(at, 10) != binding.AtNS || (!raw && !derived) {
				return invalid(path, "固定观测需要准确时间以及原始观测／采用编号，或派生结果／代次编号。")
			}
		} else if binding.ObservationID != "" || binding.AdoptionID != "" || binding.ResultID != "" || binding.GenerationID != "" || binding.AtNS != "" {
			return invalid(path, "参考窗口不能同时指定单点观测。")
		}
	}
	if len(bindings) != len(expected) {
		return invalid("/baseline_bindings", "基准绑定必须与规则引用完全一致。")
	}
	return nil
}
func baselineSelections(p *compute.Plan, cfg ChannelConfig) map[string]monitoring.RawNeed {
	out := map[string]monitoring.RawNeed{}
	for _, n := range p.Rule.Nodes {
		b := n.Params.Baseline
		if b == nil || b.Kind == "constant" {
			continue
		}
		binding := cfg.BaselineBindings[b.Reference]
		from, _ := strconv.ParseInt(b.FromNS, 10, 64)
		to, _ := strconv.ParseInt(b.ToNS, 10, 64)
		end := &to
		if b.Kind == "observation" {
			from, _ = strconv.ParseInt(binding.AtNS, 10, 64)
			if from == math.MaxInt64 {
				end = nil
			} else {
				to = from + 1
			}
		}
		out[b.Reference] = monitoring.RawNeed{Channel: cfg.InputBindings[binding.Input].ChannelID, From: &from, To: end}
	}
	return out
}
func (s *Service) baselineNeedsTx(ctx context.Context, tx pgx.Tx, tenant string, cfg ChannelConfig, p *compute.Plan) ([]monitoring.RawNeed, error) {
	out := []monitoring.RawNeed{}
	for key, need := range baselineSelections(p, cfg) {
		fixed, ok, e := loadBaselineTx(ctx, tx, tenant, cfg.ID, key)
		if e != nil {
			return nil, e
		}
		if ok {
			out = append(out, fixed.Needs...)
		} else {
			out = append(out, need)
		}
	}
	return out, nil
}
func (s *Service) authorizeBaselineNeedsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, config string, capabilities ...string) error {
	cfg, mode, e := calculationConfigKeyTx(ctx, tx, scope.Tenant, config)
	if e != nil {
		return e
	}
	p, e := s.compile(cfg.Rule, mode, cfg.Parameters)
	if e != nil {
		return e
	}
	for key, need := range baselineSelections(p, cfg) {
		fixed, found, e := loadBaselineTx(ctx, tx, scope.Tenant, cfg.ID, key)
		if e != nil {
			return e
		}
		if !found {
			if e = s.authorizeInputsTx(ctx, tx, a, scope, []monitoring.RawNeed{need}, false, capabilities...); e != nil {
				return e
			}
			continue
		}
		all := append([]string{""}, capabilities...)
		for _, capability := range all {
			if e = s.authorizeFixedBaselineTx(ctx, tx, inputAccess{Actor: a, Scope: scope, Capability: capability}, fixed); e != nil {
				return e
			}
		}
	}
	return nil
}
func loadBaselineTx(ctx context.Context, tx pgx.Tx, tenant, config, key string) (fixedBaseline, bool, error) {
	var out fixedBaseline
	var hash, snapshot string
	e := tx.QueryRow(ctx, `SELECT evidence,evidence_hash,snapshot_id::text FROM derived_fixed_baselines WHERE tenant_id=$1 AND config_id=$2 AND reference_key=$3`, tenant, config, key).Scan(&out, &hash, &snapshot)
	if e == pgx.ErrNoRows {
		return out, false, nil
	}
	if e != nil {
		return out, false, e
	}
	if fingerprint(out) != hash || out.Reference != key || out.Snapshot.ID != snapshot {
		return out, false, fmt.Errorf("baseline_evidence_integrity")
	}
	return out, true, nil
}

// The first successful explicit trial seals each selection exactly once for
// this immutable config. Concurrent trials converge on the stored evidence.
func (s *Service) freezeBaselines(ctx context.Context, lease monitoring.TaskLease, cfg ChannelConfig, p *compute.Plan) error {
	selections := baselineSelections(p, cfg)
	keys := []string{}
	for k := range selections {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, key := range keys {
		found := false
		if e := lease.Transaction(ctx, func(tx pgx.Tx) error {
			_, ok, e := loadBaselineTx(ctx, tx, lease.Task().Scope.Tenant, cfg.ID, key)
			found = ok
			return e
		}); e != nil {
			return e
		}
		if found {
			continue
		}
		selected := selections[key]
		auth := inputAccess{Actor: lease.Actor(), Scope: lease.Task().Scope, AllowChanged: true}
		checkpointKey := "baseline-snapshot:" + fingerprint([]string{cfg.ID, key})
		saved, savedSnapshot, e := lease.Checkpoint(ctx, checkpointKey)
		if e != nil {
			return e
		}
		var snapshot monitoring.RawSnapshot
		if savedSnapshot {
			if e = json.Unmarshal(saved.Result, &snapshot); e != nil {
				return e
			}
		} else {
			snapshot, e = s.freezeInputs(ctx, auth, monitoring.RawFreezeRequest{Needs: []monitoring.RawNeed{selected}})
			if e != nil {
				return e
			}
			if e = lease.SaveCheckpoint(ctx, checkpointKey, "completed", snapshot); e != nil {
				return e
			}
		}
		details, e := s.inspectInputs(ctx, auth, snapshot.ID)
		if e != nil {
			return e
		}
		fixed := fixedBaseline{Reference: key, Snapshot: snapshot, Needs: details.Needs, Quality: "good"}
		var rule compute.Baseline
		for _, n := range p.Rule.Nodes {
			if n.Params.Baseline != nil && n.Params.Baseline.Reference == key {
				rule = *n.Params.Baseline
				break
			}
		}
		binding := cfg.BaselineBindings[key]
		total := new(big.Rat)
		var extreme *big.Rat
		values := []*big.Rat{}
		var sorter *exactsort.Sorter
		if rule.Kind == "reference_window" && rule.Method == "median" && s.results != nil {
			sorter, e = exactsort.New(exactsort.Options{Directory: s.results.TempDirectory(), MemoryBytes: min(8<<20, s.options.MemoryBytes/16), Budget: &exactsort.Budget{Limit: s.options.TemporaryBytes}, Seal: func(c context.Context, path string, segment exactsort.Segment) error {
				object, e := s.results.SealSortSegment(c, lease.Task().Scope.Tenant, lease.Task().Job.Target, path, segment)
				if e != nil {
					return e
				}
				return lease.SaveCheckpoint(c, "baseline-sort:"+fingerprint([]string{cfg.ID, key})[:24]+":"+segment.SHA256, "completed", map[string]any{"object": object, "segment": segment, "snapshot_id": snapshot.ID, "reference": key})
			}})
			if e != nil {
				return e
			}
			defer sorter.Close()
		}
		budget := monitoring.RawReadBudget{Rows: math.MaxUint64, Bytes: s.options.TemporaryBytes}
		if ctx.Value(inlineTrialKey{}) == true {
			budget.Rows = uint64(s.options.TrialRows)
			budget.Bytes = s.options.ReadBytes
		}
		if rule.Method == "median" && sorter == nil {
			budget.Rows = min(budget.Rows, s.options.MemoryBytes/512)
			budget.Bytes = min(budget.Bytes, s.options.ReadBytes)
		}
		_, e = s.readInputs(ctx, auth, snapshot.ID, details, budget, func(input inputSample) error {
			row := input.Sample
			if rule.Kind == "observation" && (input.Raw != nil && (input.Raw.Observation != binding.ObservationID || input.Raw.Adoption != binding.AdoptionID) || input.Derived != nil && (input.Derived.Result != binding.ResultID || input.Derived.Generation != binding.GenerationID)) {
				return problem("baseline_selection_changed", "选中的观测采用身份已变化，请明确重新选择。", 409)
			}
			if row.Status != "valid" || row.Value == nil || !slices.Contains(p.Rule.Policies.Quality, row.Quality) {
				fixed.Excluded++
				return nil
			}
			v, exact := row.Value.Exact()
			if !exact {
				return problem("result_integrity_unavailable", "固定输入必须为规范十进制值。", 503)
			}
			fixed.Count++
			fixed.At = max(fixed.At, row.At)
			if fixed.Count == 1 {
				fixed.At = row.At
			}
			ranks := map[string]int{"good": 0, "unmarked": 1, "suspect": 2, "bad": 3}
			if ranks[row.Quality] > ranks[fixed.Quality] {
				fixed.Quality = row.Quality
			}
			if rule.Kind == "observation" {
				if input.Raw != nil {
					fixed.Observation = input.Raw.Observation
					fixed.Adoption = input.Raw.Adoption
				}
				if input.Derived != nil {
					fixed.Result = input.Derived.Result
					fixed.Generation = input.Derived.Generation
				}
				extreme = v
				return nil
			}
			switch rule.Method {
			case "mean":
				total.Add(total, v)
				if e = monitoringvalue.Check(total); e != nil {
					return e
				}
			case "median":
				if sorter != nil {
					return sorter.Add(ctx, v)
				}
				values = append(values, v)
			case "min":
				if extreme == nil || v.Cmp(extreme) < 0 {
					extreme = v
				}
			case "max":
				if extreme == nil || v.Cmp(extreme) > 0 {
					extreme = v
				}
			}
			return nil
		})
		if e != nil {
			return e
		}
		if fixed.Count == 0 {
			return problem("baseline_selection_empty", "固定基准没有符合质量条件的观测。", 422)
		}
		if rule.Kind == "reference_window" {
			if rule.Method == "mean" {
				extreme = total.Quo(total, new(big.Rat).SetInt(new(big.Int).SetUint64(fixed.Count)))
			}
			if rule.Method == "median" {
				if sorter != nil {
					lo, hi := (fixed.Count-1)/2, fixed.Count/2
					ranks, e := sorter.Ranks(ctx, []uint64{lo, hi})
					if e != nil {
						return e
					}
					extreme = new(big.Rat).Quo(new(big.Rat).Add(ranks[lo], ranks[hi]), big.NewRat(2, 1))
					if e = sorter.Close(); e != nil {
						return e
					}
				} else {
					slices.SortFunc(values, func(a, b *big.Rat) int { return a.Cmp(b) })
					extreme = values[len(values)/2]
					if len(values)%2 == 0 {
						extreme = new(big.Rat).Quo(new(big.Rat).Add(values[len(values)/2-1], extreme), big.NewRat(2, 1))
					}
				}
			}
		}
		if e = monitoringvalue.Check(extreme); e != nil {
			return e
		}
		fixed.Numerator = extreme.Num().String()
		fixed.Denominator = extreme.Denom().String()
		if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
			state, e := s.checkFixedInputsTx(ctx, tx, auth, snapshot.ID)
			if e != nil {
				return e
			}
			if state.Revoked {
				return problem("baseline_selection_changed", "选择的基准依据已撤销。", 409)
			}
			_, e = tx.Exec(ctx, `INSERT INTO derived_fixed_baselines(tenant_id,config_id,reference_key,snapshot_id,evidence,evidence_hash,created_by) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, lease.Task().Scope.Tenant, cfg.ID, key, snapshot.ID, jsonBytes(fixed), fingerprint(fixed), lease.Actor().ID)
			return e
		}); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) baselinesTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, permit *monitoring.RawSystemPermit, config string) (map[string]compute.FixedBaseline, []BaselineProof, error) {
	cfg, _, e := calculationConfigKeyTx(ctx, tx, scope.Tenant, config)
	if e != nil {
		return nil, nil, e
	}
	out := map[string]compute.FixedBaseline{}
	proofs := []BaselineProof{}
	keys := []string{}
	for key := range cfg.BaselineBindings {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		fixed, found, e := loadBaselineTx(ctx, tx, scope.Tenant, config, key)
		if e != nil {
			return nil, nil, e
		}
		if !found {
			return nil, nil, problem("baseline_binding_required", "请先完成固定基准的试算。", 409)
		}
		state, e := s.checkFixedInputsTx(ctx, tx, inputAccess{Actor: a, Scope: scope, Permit: permit}, fixed.Snapshot.ID)
		if e != nil {
			return nil, nil, e
		}
		value, ok := new(big.Rat).SetString(fixed.Numerator + "/" + fixed.Denominator)
		if !ok {
			return nil, nil, fmt.Errorf("baseline_evidence_integrity")
		}
		number, e := monitoringvalue.Rational(value)
		if e != nil {
			return nil, nil, e
		}
		out[key] = compute.FixedBaseline{Value: number, At: fixed.At, Valid: !state.Revoked, Proven: true, Quality: fixed.Quality, ID: fixed.Adoption}
		if fixed.Result != "" {
			b := out[key]
			b.ID = fixed.Result + ":" + fixed.Generation
			out[key] = b
		} else if fixed.Adoption == "" {
			b := out[key]
			b.ID = fixed.Snapshot.ID
			out[key] = b
		}
		proofs = append(proofs, BaselineProof{key, fixed.Snapshot.ID, fingerprint(fixed), !state.Revoked})
	}
	return out, proofs, nil
}
