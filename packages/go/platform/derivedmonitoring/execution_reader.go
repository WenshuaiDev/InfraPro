package derivedmonitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

func systemPermit(run runRecord) *monitoring.RawSystemPermit {
	if run.Mode != "automatic" {
		return nil
	}
	return &monitoring.RawSystemPermit{Tenant: run.Scope.Tenant, RunID: run.ID, MandateRevision: run.MandateRevision}
}
func runInputAccess(a access.Actor, run runRecord) inputAccess {
	return inputAccess{Actor: a, Scope: monitoringScope(run.Scope), Permit: systemPermit(run), AllowChanged: run.Mode == "analysis"}
}
func (s *Service) freezeRunInputs(ctx context.Context, a access.Actor, run runRecord, request monitoring.RawFreezeRequest) (monitoring.RawSnapshot, error) {
	return s.freezeInputs(ctx, runInputAccess(a, run), request)
}
func (s *Service) inspectRunInputs(ctx context.Context, a access.Actor, run runRecord, id string) (inputSnapshotDetails, error) {
	return s.inspectInputs(ctx, runInputAccess(a, run), id)
}
func (s *Service) resolveRunInputs(ctx context.Context, a access.Actor, run runRecord, id string, request monitoring.RawFreezeRequest) (inputSnapshotDetails, error) {
	return s.resolveInputs(ctx, runInputAccess(a, run), id, request)
}
func (s *Service) readRunInputs(ctx context.Context, a access.Actor, run runRecord, id string, part inputSnapshotDetails, budget monitoring.RawReadBudget, consume func(inputSample) error) (monitoring.RawReadCount, error) {
	return s.readInputs(ctx, runInputAccess(a, run), id, part, budget, consume)
}
func (s *Service) checkUserInputsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, id string, history bool) (monitoring.RawSnapshotState, error) {
	state, e := s.monitoring.CheckRawSnapshotTx(ctx, tx, a, scope, id, history)
	if e != nil {
		return state, e
	}
	source, e := s.checkInputLedgerTx(ctx, tx, inputAccess{Actor: a, Scope: scope, History: history}, id)
	state.Changed = state.Changed || source.Changed || !source.Current
	state.Revoked = state.Revoked || source.Revoked
	return state, e
}
func (s *Service) checkSystemInputsTx(ctx context.Context, tx pgx.Tx, permit monitoring.RawSystemPermit, id string) (monitoring.RawSnapshotState, error) {
	state, e := s.monitoring.CheckRawSystemSnapshotTx(ctx, tx, permit, id)
	if e != nil {
		return state, e
	}
	source, e := s.checkInputLedgerTx(ctx, tx, inputAccess{Scope: monitoring.Scope{Entry: "tenant", Tenant: permit.Tenant}, Permit: &permit}, id)
	state.Changed = state.Changed || source.Changed || !source.Current
	state.Revoked = state.Revoked || source.Revoked
	return state, e
}
func (s *Service) checkRunInputsTx(ctx context.Context, tx pgx.Tx, a access.Actor, run runRecord, id string) (monitoring.RawSnapshotState, error) {
	if permit := systemPermit(run); permit != nil {
		return s.checkSystemInputsTx(ctx, tx, *permit, id)
	}
	return s.checkUserInputsTx(ctx, tx, a, monitoringScope(run.Scope), id, false)
}
func (s *Service) runBaselinesTx(ctx context.Context, tx pgx.Tx, a access.Actor, run runRecord) (map[string]compute.FixedBaseline, []BaselineProof, error) {
	return s.baselinesTx(ctx, tx, a, monitoringScope(run.Scope), systemPermit(run), run.ConfigID)
}

func executionActor(id string) access.Actor {
	return access.Actor{ID: id, Check: func(ctx context.Context, tx pgx.Tx, _ bool) error {
		var active bool
		if e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, id).Scan(&active); e != nil {
			return e
		}
		if !active {
			return access.ErrDenied
		}
		return nil
	}}
}
