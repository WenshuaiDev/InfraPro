package derivedmonitoring

import (
	"context"
	"errors"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

// Source proofs describe published generations, never copied display values.
// Each projection carries exactly the dependency intervals required by its
// consumer. The tree permits transaction-only requalification at publication,
// export and download boundaries after external manifests have been verified.
type inputSourceProof struct {
	Channel    string               `json:"channel_id"`
	Range      NanosecondRange      `json:"output_range"`
	Generation string               `json:"generation_id"`
	Config     string               `json:"config_id"`
	Manifest   string               `json:"manifest_hash"`
	Snapshot   string               `json:"raw_snapshot_id"`
	RawNeeds   []monitoring.RawNeed `json:"raw_input_ranges"`
	Baselines  []BaselineProof      `json:"baselines"`
	Inputs     []inputSourceProof   `json:"upstream_inputs"`
}
type inputSources struct {
	Schema     string                 `json:"schema"`
	Snapshot   monitoring.RawSnapshot `json:"raw_snapshot"`
	Candidates []monitoring.RawNeed   `json:"input_ranges"`
	Direct     []monitoring.RawNeed   `json:"direct_input_ranges"`
	Sources    []inputSourceProof     `json:"upstream_inputs"`
}
type inputAccess struct {
	Actor        access.Actor
	Scope        monitoring.Scope
	Permit       *monitoring.RawSystemPermit
	History      bool
	SelectedPart bool
	Fixed        bool
	AllowChanged bool
	Capability   string
}
type inputSourceState struct {
	Current bool
	Changed bool
	Revoked bool
}

func (s *Service) checkInputSourcesTx(ctx context.Context, tx pgx.Tx, auth inputAccess, proofs []inputSourceProof) (inputSourceState, error) {
	out := inputSourceState{Current: true}
	visited := 0
	path := map[string]bool{}
	var visit func(inputSourceProof, int) error
	visit = func(proof inputSourceProof, depth int) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		visited++
		if visited > 10000 || depth > 8 {
			return problem("plan_limit_exceeded", "递归来源证明超过预算。", 422)
		}
		if path[proof.Channel] {
			return problem("result_integrity_unavailable", "固定来源证明存在循环。", 503)
		}
		path[proof.Channel] = true
		defer delete(path, proof.Channel)
		r, e := proof.Range.rangeValue()
		if e != nil {
			return e
		}
		if e = s.authorizeSourceOutputTx(ctx, tx, auth, proof.Channel, r); e != nil {
			return e
		}
		if e = requireResultAvailableTx(ctx, tx, auth.Scope.Tenant, "derived_generations", proof.Generation); e != nil {
			return e
		}
		var published, assigned bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_generations WHERE tenant_id=$1 AND id=$2 AND channel_id=$3 AND config_id=$4 AND raw_snapshot_id=$5 AND manifest_root=$6 AND output_range @> $7::int8range),EXISTS(SELECT 1 FROM derived_config_assignments WHERE tenant_id=$1 AND channel_id=$3 AND config_id=$4 AND superseded_revision IS NULL AND effective_range @> $7::int8range)`, auth.Scope.Tenant, proof.Generation, proof.Channel, proof.Config, proof.Snapshot, proof.Manifest, rangeSQL(r)).Scan(&published, &assigned)
		if e != nil {
			return e
		}
		if !published {
			return problem("result_integrity_unavailable", "上游来源未对应完整正式代次。", 503)
		}
		ownChanged := !assigned
		if !auth.History && !auth.Fixed && !assigned {
			out.Current = false
			out.Changed = true
		}
		if len(proof.RawNeeds) > 0 {
			var state monitoring.RawSnapshotState
			if auth.Fixed && auth.Permit == nil {
				state, e = s.monitoring.CheckFixedRawPartTx(ctx, tx, auth.Actor, auth.Scope, proof.Snapshot, proof.RawNeeds, auth.Capability)
			} else if auth.Fixed {
				state, e = s.monitoring.CheckFixedRawDependencySystemTx(ctx, tx, *auth.Permit, proof.Snapshot, proof.RawNeeds)
			} else if auth.Permit == nil {
				state, e = s.monitoring.CheckRawSnapshotPartTx(ctx, tx, auth.Actor, auth.Scope, proof.Snapshot, proof.RawNeeds, auth.History, auth.Capability)
			} else {
				state, e = s.monitoring.CheckRawDependencyPartSystemTx(ctx, tx, *auth.Permit, proof.Snapshot, proof.RawNeeds)
			}
			var stale *monitoring.Error
			if errors.As(e, &stale) && stale.Code == "snapshot_stale" {
				out.Current = false
			} else if e != nil {
				return e
			}
			out.Changed = out.Changed || state.Changed
			ownChanged = ownChanged || state.Changed
			out.Revoked = out.Revoked || state.Revoked
			if !auth.History && state.Changed {
				out.Current = false
			}
		}
		for _, basis := range proof.Baselines {
			fixed, found, e := loadBaselineMetadataTx(ctx, tx, auth.Scope.Tenant, proof.Config, basis.Reference, &basis.Hash)
			if e != nil {
				return e
			}
			if !found || fixed.Snapshot.ID != basis.Snapshot {
				return problem("result_integrity_unavailable", "上游固定基准证据不完整。", 503)
			}
			var state monitoring.RawSnapshotState
			if auth.History {
				state, e = s.checkFixedBaselineHistoryTx(ctx, tx, auth, fixed)
			} else if auth.Permit != nil {
				state, e = s.checkFixedInputsTx(ctx, tx, auth, fixed.Snapshot.ID)
			} else {
				if e = s.authorizeFixedBaselineTx(ctx, tx, auth, fixed); e != nil {
					return e
				}
				state, e = s.checkFixedInputsTx(ctx, tx, auth, fixed.Snapshot.ID)
			}
			if e != nil {
				return e
			}
			// A generation that already published baseline_invalid is valid
			// evidence of that null result. Its known revoked baseline must not
			// make every downstream calculation look newly revoked forever.
			// A baseline that was valid when frozen still invalidates immediately.
			basisChanged := basis.Valid == state.Revoked
			out.Changed = out.Changed || basisChanged
			out.Revoked = out.Revoked || state.Revoked && (basis.Valid || auth.History)
			if !auth.History && basisChanged {
				out.Current = false
				ownChanged = true
			}
		}
		for _, child := range proof.Inputs {
			if e = visit(child, depth+1); e != nil {
				return e
			}
		}
		if auth.AllowChanged && !auth.History && ownChanged {
			historical := auth
			historical.History = true
			if e = s.authorizeSourceOutputTx(ctx, tx, historical, proof.Channel, r); e != nil {
				return e
			}
		}
		return nil
	}
	for _, proof := range proofs {
		if e := visit(proof, 1); e != nil {
			return out, e
		}
	}
	return out, nil
}

func sourceProofLeaves(proofs []inputSourceProof) ([]monitoring.RawNeed, error) {
	needs := []monitoring.RawNeed{}
	var visit func([]inputSourceProof, int) error
	visit = func(items []inputSourceProof, depth int) error {
		if depth > 8 {
			return problem("plan_limit_exceeded", "来源深度超过预算。", 422)
		}
		for _, p := range items {
			needs = append(needs, p.RawNeeds...)
			if len(needs) > 50000 {
				return problem("plan_limit_exceeded", "原始叶子范围超过预算。", 422)
			}
			if len(p.Inputs) > 0 {
				if e := visit(p.Inputs, depth+1); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e := visit(proofs, 1); e != nil {
		return nil, e
	}
	return normalizedNeeds(needs)
}

func sourceProofRanges(proofs []inputSourceProof, channel string) ([]planning.Range, error) {
	ranges := []planning.Range{}
	for _, proof := range proofs {
		if proof.Channel == channel {
			r, e := proof.Range.rangeValue()
			if e != nil {
				return nil, e
			}
			ranges = append(ranges, r)
		}
	}
	return planning.Union(ranges)
}
