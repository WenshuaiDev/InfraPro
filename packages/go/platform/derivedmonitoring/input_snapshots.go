package derivedmonitoring

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

type inputSnapshotDetails struct {
	monitoring.RawSnapshotDetails
	Direct  []monitoring.RawNeed
	Sources []inputSourceProof
}

func loadInputSourcesTx(ctx context.Context, tx pgx.Tx, tenant, snapshot string) (inputSources, bool, error) {
	var proof inputSources
	var hash string
	e := tx.QueryRow(ctx, `SELECT proof,proof_hash FROM derived_input_sources WHERE tenant_id=$1 AND snapshot_id=$2`, tenant, snapshot).Scan(&proof, &hash)
	if e == pgx.ErrNoRows {
		return proof, false, nil
	}
	if e != nil {
		return proof, false, e
	}
	if proof.Schema != "derived-input-sources/1" || proof.Snapshot.ID != snapshot || fingerprint(proof) != hash {
		return proof, false, problem("result_integrity_unavailable", "固定输入来源清单不完整。", 503)
	}
	return proof, true, nil
}
func sourceCandidates(direct []monitoring.RawNeed, proofs []inputSourceProof) ([]monitoring.RawNeed, error) {
	needs := append([]monitoring.RawNeed{}, direct...)
	for _, proof := range proofs {
		r, e := proof.Range.rangeValue()
		if e != nil {
			return nil, e
		}
		needs = append(needs, monitoring.RawNeed{Channel: proof.Channel, From: &r.From, To: r.To})
	}
	return normalizedNeeds(needs)
}

// Native raw snapshots pin canonical leaf observations. The companion ledger
// pins upstream published outputs without copying or relabeling them as raw.
func (s *Service) freezeInputs(ctx context.Context, auth inputAccess, request monitoring.RawFreezeRequest) (monitoring.RawSnapshot, error) {
	var snapshot monitoring.RawSnapshot
	var direct, derived []monitoring.RawNeed
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var e error
		direct, derived, e = splitInputKindsTx(ctx, tx, auth.Scope.Tenant, request.Needs)
		return e
	})
	if e != nil {
		return snapshot, e
	}
	if len(derived) == 0 {
		if auth.Permit != nil {
			return s.raw.FreezeSystem(ctx, *auth.Permit, request)
		}
		return s.raw.FreezeUser(ctx, auth.Actor, auth.Scope, request)
	}
	sources, e := s.resolveCurrentSources(ctx, auth, derived, selectInputPrefixes(request.Prefixes, derived))
	if e != nil {
		return snapshot, e
	}
	leaves, e := sourceProofLeaves(sources)
	if e != nil {
		return snapshot, e
	}
	leaves, e = normalizedNeeds(append(leaves, direct...))
	if e != nil {
		return snapshot, e
	}
	rawRequest := monitoring.RawFreezeRequest{Needs: leaves, Prefixes: selectInputPrefixes(request.Prefixes, direct)}
	if auth.Permit != nil {
		snapshot, e = s.raw.FreezeSystem(ctx, *auth.Permit, rawRequest)
	} else {
		snapshot, e = s.raw.FreezeUser(ctx, auth.Actor, auth.Scope, rawRequest)
	}
	if e != nil {
		return snapshot, e
	}
	if len(direct) > 0 {
		request := monitoring.RawFreezeRequest{Needs: direct, Prefixes: rawRequest.Prefixes}
		var resolved monitoring.RawSnapshotDetails
		if auth.Permit != nil {
			resolved, e = s.raw.ResolvePartSystem(ctx, *auth.Permit, snapshot.ID, request)
		} else {
			resolved, e = s.raw.ResolvePartUser(ctx, auth.Actor, auth.Scope, snapshot.ID, request)
		}
		if e != nil {
			return snapshot, e
		}
		direct = resolved.Candidates
	}
	candidates, e := sourceCandidates(direct, sources)
	if e != nil {
		return snapshot, e
	}
	proof := inputSources{Schema: "derived-input-sources/1", Snapshot: snapshot, Direct: direct, Candidates: candidates, Sources: sources}
	if len(jsonBytes(proof)) > 7<<20 {
		return snapshot, problem("plan_limit_exceeded", "固定来源清单超过规划预算。", 422)
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		state, e := s.checkInputSourcesTx(ctx, tx, auth, sources)
		if e != nil {
			return e
		}
		if !state.Current {
			return problem("upstream_pending", "冻结期间上游来源已变化，请等待更新。", 409)
		}
		_, e = tx.Exec(ctx, `INSERT INTO derived_input_sources(tenant_id,snapshot_id,proof,proof_hash) VALUES($1,$2,$3,$4)`, auth.Scope.Tenant, snapshot.ID, jsonBytes(proof), fingerprint(proof))
		return e
	})
	return snapshot, e
}

func (s *Service) inspectInputs(ctx context.Context, auth inputAccess, id string) (inputSnapshotDetails, error) {
	out := inputSnapshotDetails{Direct: []monitoring.RawNeed{}, Sources: []inputSourceProof{}}
	var proof inputSources
	var found bool
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var e error
		proof, found, e = loadInputSourcesTx(ctx, tx, auth.Scope.Tenant, id)
		return e
	})
	if e != nil {
		return out, e
	}
	if auth.Permit != nil {
		out.RawSnapshotDetails, e = s.raw.InspectSystem(ctx, *auth.Permit, id, auth.History)
	} else {
		out.RawSnapshotDetails, e = s.raw.InspectUser(ctx, auth.Actor, auth.Scope, id, auth.History)
	}
	if e != nil {
		return out, e
	}
	out.Direct = out.Candidates
	if found {
		if proof.Snapshot.Hash != out.Snapshot.Hash {
			return out, problem("result_integrity_unavailable", "原始叶子与上游来源快照不匹配。", 503)
		}
		out.Direct = proof.Direct
		out.Sources = proof.Sources
		out.Needs = proof.Candidates
		out.Candidates = proof.Candidates
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			state, e := s.checkInputSourcesTx(ctx, tx, auth, proof.Sources)
			out.State.Changed = out.State.Changed || state.Changed || !state.Current
			out.State.Revoked = out.State.Revoked || state.Revoked
			return e
		})
	}
	return out, e
}

// Projection always starts with fixed upstream references. It can reduce an
// interval but cannot silently substitute a newly published generation.
func (s *Service) resolveInputs(ctx context.Context, auth inputAccess, id string, request monitoring.RawFreezeRequest) (inputSnapshotDetails, error) {
	out := inputSnapshotDetails{Direct: []monitoring.RawNeed{}, Sources: []inputSourceProof{}}
	var direct, derived []monitoring.RawNeed
	var ledger inputSources
	var found bool
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var e error
		direct, derived, e = splitInputKindsTx(ctx, tx, auth.Scope.Tenant, request.Needs)
		if e != nil {
			return e
		}
		ledger, found, e = loadInputSourcesTx(ctx, tx, auth.Scope.Tenant, id)
		return e
	})
	if e != nil {
		return out, e
	}
	if len(derived) > 0 && !found {
		return out, problem("result_integrity_unavailable", "缺少固定上游输入证据。", 503)
	}
	if len(direct) > 0 {
		rawRequest := monitoring.RawFreezeRequest{Needs: direct, Prefixes: selectInputPrefixes(request.Prefixes, direct), History: auth.History}
		if auth.Permit != nil {
			out.RawSnapshotDetails, e = s.raw.ResolvePartSystem(ctx, *auth.Permit, id, rawRequest)
		} else {
			out.RawSnapshotDetails, e = s.raw.ResolvePartUser(ctx, auth.Actor, auth.Scope, id, rawRequest)
		}
		if e != nil {
			return out, e
		}
		out.Direct = out.Candidates
	} else if found {
		out.Snapshot = ledger.Snapshot
	}
	if len(derived) > 0 {
		out.Sources, e = s.resolveFrozenSources(ctx, auth, ledger.Sources, derived, selectInputPrefixes(request.Prefixes, derived), 1)
		if e != nil {
			return out, e
		}
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			state, e := s.checkInputSourcesTx(ctx, tx, auth, out.Sources)
			out.State.Changed = out.State.Changed || state.Changed || !state.Current
			out.State.Revoked = out.State.Revoked || state.Revoked
			return e
		})
		if e != nil {
			return out, e
		}
	}
	out.Candidates, e = sourceCandidates(out.Direct, out.Sources)
	out.Needs = out.Candidates
	return out, e
}

func (s *Service) checkInputLedgerTx(ctx context.Context, tx pgx.Tx, auth inputAccess, id string) (inputSourceState, error) {
	proof, found, e := loadInputSourcesTx(ctx, tx, auth.Scope.Tenant, id)
	if e != nil || !found {
		return inputSourceState{Current: true}, e
	}
	return s.checkInputSourcesTx(ctx, tx, auth, proof.Sources)
}

func (s *Service) checkInputPartTx(ctx context.Context, tx pgx.Tx, auth inputAccess, id string, direct []monitoring.RawNeed, sources []inputSourceProof) (monitoring.RawSnapshotState, error) {
	var out monitoring.RawSnapshotState
	var e error
	if len(direct) > 0 {
		if auth.Permit != nil {
			out, e = s.monitoring.CheckRawDependencyPartSystemTx(ctx, tx, *auth.Permit, id, direct)
		} else {
			out, e = s.monitoring.CheckRawSnapshotPartTx(ctx, tx, auth.Actor, auth.Scope, id, direct, auth.History, auth.Capability)
		}
		if e != nil {
			return out, e
		}
	}
	state, e := s.checkInputSourcesTx(ctx, tx, auth, sources)
	out.Changed = out.Changed || state.Changed || !state.Current
	out.Revoked = out.Revoked || state.Revoked
	return out, e
}
