package derivedmonitoring

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

func (s *Service) checkFixedInputsTx(ctx context.Context, tx pgx.Tx, auth inputAccess, id string) (monitoring.RawSnapshotState, error) {
	var state monitoring.RawSnapshotState
	var e error
	if auth.Permit == nil {
		state, e = s.monitoring.CheckFixedRawUserTx(ctx, tx, auth.Actor, auth.Scope, id)
	} else {
		state, e = s.monitoring.CheckFixedRawSystemTx(ctx, tx, *auth.Permit, id)
	}
	if e != nil {
		return state, e
	}
	auth.Fixed = true
	source, e := s.checkInputLedgerTx(ctx, tx, auth, id)
	state.Revoked = state.Revoked || source.Revoked
	state.Changed = state.Revoked
	return state, e
}
func (s *Service) authorizeFixedBaselineTx(ctx context.Context, tx pgx.Tx, auth inputAccess, fixed fixedBaseline) error {
	ledger, found, e := loadInputSourcesTx(ctx, tx, auth.Scope.Tenant, fixed.Snapshot.ID)
	if e != nil {
		return e
	}
	direct := fixed.Needs
	if found {
		direct = ledger.Direct
	}
	if len(direct) > 0 {
		if auth.Permit != nil {
			e = s.AuthorizeRawSystemTx(ctx, tx, *auth.Permit, direct, false)
		} else {
			e = s.monitoring.AuthorizeRawInputTx(ctx, tx, auth.Actor, auth.Scope, direct, auth.History, auth.Capability)
		}
		if e != nil {
			return e
		}
	}
	if found {
		auth.Fixed = true
		_, e = s.checkInputSourcesTx(ctx, tx, auth, ledger.Sources)
	}
	return e
}
func (s *Service) checkFixedBaselineHistoryTx(ctx context.Context, tx pgx.Tx, auth inputAccess, fixed fixedBaseline) (monitoring.RawSnapshotState, error) {
	auth.History = true
	if e := s.authorizeFixedBaselineTx(ctx, tx, auth, fixed); e != nil {
		return monitoring.RawSnapshotState{}, e
	}
	return s.checkUserInputsTx(ctx, tx, auth.Actor, auth.Scope, fixed.Snapshot.ID, true)
}
