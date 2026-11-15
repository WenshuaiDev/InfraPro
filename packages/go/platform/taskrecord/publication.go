package taskrecord

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// PublishTx is for a live source transaction only. Replay/backfill uses ApplyTx
// and cannot create a new business execution fact or notification.
func PublishTx(ctx context.Context, tx pgx.Tx, c Change) (Ref, error) {
	if e := auditTransitionTx(ctx, tx, c); e != nil {
		return Ref{}, e
	}
	if e := SetAuditCommandOutcomeTx(ctx, tx, c); e != nil {
		return Ref{}, e
	}
	return ApplyTx(ctx, tx, c)
}
