package tasks

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// RestoreEpochTx fences stale task pages and source-index leases without replaying business execution.
func RestoreEpochTx(ctx context.Context, tx pgx.Tx) error {
	_, e := tx.Exec(ctx, `UPDATE task_protocol SET restore_epoch=gen_random_uuid(),cursor_secret=gen_random_uuid()::text||gen_random_uuid()::text,mode='backfilling',updated_at=clock_timestamp() WHERE id=1`)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE task_source_states SET restore_epoch=(SELECT restore_epoch FROM task_protocol WHERE id=1),backfill_complete=false,backfill_cursor='{}',lease_owner=NULL,lease_until=NULL,fence=fence+1`)
	return e
}
