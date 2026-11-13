package derivedmonitoring

import (
	"context"
	"encoding/json"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

// Server-generated proof tickets close publication/download races inside the
// host transaction. They contain qualified subranges, never user credentials.
type calculationReadProof struct {
	Sources    []inputSourceProof   `json:"upstream_inputs,omitempty"`
	Schema     string               `json:"schema"`
	Channel    string               `json:"channel_id"`
	Range      NanosecondRange      `json:"output_range"`
	State      string               `json:"state"`
	Generation string               `json:"generation_id,omitempty"`
	Config     string               `json:"config_id,omitempty"`
	Snapshot   string               `json:"snapshot_id,omitempty"`
	Needs      []monitoring.RawNeed `json:"input_ranges,omitempty"`
	Baselines  []BaselineProof      `json:"baselines,omitempty"`
}

// CheckCalculationProofsTx batches transaction-only qualification. It repeats
// current authority and source-change checks on every call; no authorization or
// validity result survives the transaction. Immutable proof trees are flattened
// without replacing each snapshot's own ranges or source position.
func (s *Service) CheckCalculationProofsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, proofs []json.RawMessage, capability string) error {
	outputs := map[string][]monitoring.Window{}
	rawParts := []monitoring.RawSnapshotPart{}
	generations, channels, configs, snapshots, ranges, manifests := []string{}, []string{}, []string{}, []string{}, []string{}, []string{}
	baselines := map[string][]BaselineProof{}
	appendProof := func(channel, generation, config, snapshot, manifest string, interval NanosecondRange, needs []monitoring.RawNeed, fixed []BaselineProof) error {
		r, e := interval.rangeValue()
		if e != nil {
			return e
		}
		outputs[channel] = append(outputs[channel], rangeWindow(r))
		generations, channels, configs = append(generations, generation), append(channels, channel), append(configs, config)
		snapshots, ranges, manifests = append(snapshots, snapshot), append(ranges, rangeSQL(r)), append(manifests, manifest)
		if len(needs) > 0 {
			rawParts = append(rawParts, monitoring.RawSnapshotPart{Snapshot: snapshot, Needs: needs})
		}
		if previous, exists := baselines[config]; exists && (len(previous) != len(fixed) || len(fixed) > 0 && fingerprint(previous) != fingerprint(fixed)) {
			return problem("export_invalidated", "固定基准状态已变化。", 409)
		}
		baselines[config] = fixed
		return nil
	}
	for _, raw := range proofs {
		var proof calculationReadProof
		if json.Unmarshal(raw, &proof) != nil || proof.Schema != "calculation-read-proof/1" {
			return problem("result_integrity_unavailable", "计算结果读取证明不完整。", 503)
		}
		r, e := proof.Range.rangeValue()
		if e != nil {
			return e
		}
		if proof.State == "not_computed" {
			if e = s.qualifyUncomputedTx(ctx, tx, a, scope, proof.Channel, r, capability); e != nil {
				return e
			}
			continue
		}
		if proof.State != "current" {
			return problem("export_invalidated", "当前结果已失效。", 409)
		}
		if e = appendProof(proof.Channel, proof.Generation, proof.Config, proof.Snapshot, "", proof.Range, proof.Needs, proof.Baselines); e != nil {
			return e
		}
		visited := 0
		path := map[string]bool{proof.Channel: true}
		var visit func(inputSourceProof, int) error
		visit = func(source inputSourceProof, depth int) error {
			if e := ctx.Err(); e != nil {
				return e
			}
			visited++
			if depth > 8 || visited > 10000 {
				return problem("plan_limit_exceeded", "递归来源证明超过预算。", 422)
			}
			if path[source.Channel] {
				return problem("result_integrity_unavailable", "固定来源证明存在循环。", 503)
			}
			path[source.Channel] = true
			defer delete(path, source.Channel)
			if e := appendProof(source.Channel, source.Generation, source.Config, source.Snapshot, source.Manifest, source.Range, source.RawNeeds, source.Baselines); e != nil {
				return e
			}
			for _, child := range source.Inputs {
				if e := visit(child, depth+1); e != nil {
					return e
				}
			}
			return nil
		}
		for _, source := range proof.Sources {
			if e = visit(source, 1); e != nil {
				return e
			}
		}
	}
	if len(generations) == 0 {
		return nil
	}
	if e := s.monitoring.AuthorizeCalculationOutputsTx(ctx, tx, a, scope, outputs, false, capability); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `SELECT revision FROM derived_graph_states WHERE tenant_id=$1 FOR SHARE`, scope.Tenant); e != nil {
		return e
	}
	var unavailable, valid bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_result_integrity WHERE tenant_id=$1 AND result_kind='derived_generations' AND result_id=ANY($2::uuid[]) AND state='unavailable'),
 NOT EXISTS(SELECT 1 FROM unnest($2::uuid[],$3::uuid[],$4::uuid[],$5::uuid[],$6::int8range[],$7::text[]) p(generation,channel,config,snapshot,affected,manifest)
 WHERE NOT EXISTS(SELECT 1 FROM derived_generations g WHERE g.tenant_id=$1 AND g.id=p.generation AND g.channel_id=p.channel AND g.config_id=p.config AND g.raw_snapshot_id=p.snapshot AND g.output_range @> p.affected AND (p.manifest='' OR g.manifest_root=p.manifest))
 OR NOT EXISTS(SELECT 1 FROM derived_config_assignments a WHERE a.tenant_id=$1 AND a.channel_id=p.channel AND a.config_id=p.config AND a.superseded_revision IS NULL AND a.effective_range @> p.affected))`, scope.Tenant, generations, channels, configs, snapshots, ranges, manifests).Scan(&unavailable, &valid)
	if e != nil {
		return e
	}
	if unavailable {
		return resultUnavailableError()
	}
	if !valid {
		return problem("export_invalidated", "结果代次或配置分配已变化。", 409)
	}
	if e = s.monitoring.CheckRawSnapshotPartsTx(ctx, tx, a, scope, rawParts, capability); e != nil {
		return e
	}
	for config, expected := range baselines {
		if e = s.authorizeBaselineNeedsTx(ctx, tx, a, scope, config, capability); e != nil {
			return e
		}
		_, actual, e := s.baselinesTx(ctx, tx, a, scope, nil, config)
		if e != nil {
			return e
		}
		if len(actual) != len(expected) || len(actual) > 0 && fingerprint(actual) != fingerprint(expected) {
			return problem("export_invalidated", "固定基准状态已变化。", 409)
		}
	}
	return nil
}
