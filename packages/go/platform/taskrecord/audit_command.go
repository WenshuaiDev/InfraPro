package taskrecord

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

// ObserveAuditCommandTx reads only this transaction's registered source journal.
// It never infers a task from an object name, time or client request identifier.
func ObserveAuditCommandTx(ctx context.Context, tx pgx.Tx) error {
	command, e := ar.CommandTx(ctx, tx)
	if e != nil || command.OperationID == "" {
		return e
	}
	var c Change
	e = tx.QueryRow(ctx, `SELECT change FROM task_transaction_facts WHERE transaction_id=pg_current_xact_id() AND change->>'source_module'=$1 AND change->>'source_kind'=$2 AND change->>'source_root_id'=$3 ORDER BY id DESC LIMIT 1`, command.Module, command.Kind, command.RootID).Scan(&c)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	return SetAuditCommandOutcomeTx(ctx, tx, c)
}
func SetAuditCommandOutcomeTx(ctx context.Context, tx pgx.Tx, c Change) error {
	command, e := ar.CommandTx(ctx, tx)
	if e != nil || command.OperationID == "" {
		return e
	}
	if command.Module != c.Module || command.Kind != c.Kind || command.RootID != c.RootID {
		return nil
	}
	command.TaskID = c.SourceRef.ID()
	command.Generation = c.Generation
	command.AfterState = c.State
	command.RequestedBy = c.RequestedBy
	return ar.SetCommandTx(ctx, tx, command)
}
