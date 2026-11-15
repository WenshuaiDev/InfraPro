package display

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
)

type TaskHandoffs struct{}

func (TaskHandoffs) PendingTx(ctx context.Context, tx pgx.Tx, limit, retries int) ([]tasks.Handoff, error) {
	rows, e := tx.Query(ctx, `SELECT h.id::text,h.snapshot,(h.fingerprint=md5(h.snapshot::text) AND h.snapshot->>'source_root_id'=h.job_id::text AND h.snapshot->>'source_revision'=h.task_revision::text AND h.snapshot->>'generation'=h.execution_generation::text AND h.snapshot->'scope'=m.snapshot->'scope' AND h.snapshot->>'originator_account_id'=m.snapshot->>'originator_account_id' AND h.snapshot->>'input_hash'=m.snapshot->>'input_hash' AND h.task_revision<=m.task_revision AND h.execution_generation<=(m.snapshot->>'generation')::bigint) FROM display_task_handoffs h JOIN display_task_metadata m ON m.id=h.job_id WHERE h.translated_at IS NULL AND h.attempts<$2 AND h.available_at<=clock_timestamp() ORDER BY h.occurred_at,h.id LIMIT $1 FOR UPDATE OF h SKIP LOCKED`, limit, retries)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []tasks.Handoff{}
	for rows.Next() {
		var h tasks.Handoff
		var valid bool
		if e = rows.Scan(&h.ID, &h.Change, &valid); e != nil {
			return nil, e
		}
		if !valid {
			h.Change.Type = "invalid_handoff"
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
func (TaskHandoffs) CompleteTx(ctx context.Context, tx pgx.Tx, id string) error {
	_, e := tx.Exec(ctx, `UPDATE display_task_handoffs SET translated_at=clock_timestamp(),last_error_code='' WHERE id=$1 AND translated_at IS NULL`, id)
	return e
}
func (TaskHandoffs) FailTx(ctx context.Context, tx pgx.Tx, id string) error {
	_, e := tx.Exec(ctx, `UPDATE display_task_handoffs SET attempts=attempts+1,available_at=clock_timestamp()+least(300,power(2,attempts+1))::integer*interval '1 second',last_error_code='task_translation_failed' WHERE id=$1 AND translated_at IS NULL`, id)
	return e
}
func (TaskHandoffs) ReplayTx(ctx context.Context, tx pgx.Tx, id string) error {
	tag, e := tx.Exec(ctx, `UPDATE display_task_handoffs SET attempts=0,available_at=clock_timestamp(),last_error_code='' WHERE id=$1 AND translated_at IS NULL AND last_error_code<>''`, id)
	if e == nil && tag.RowsAffected() != 1 {
		return tasks.Problem("handoff_not_replayable", "交接不存在、已完成或尚未失败。", 409)
	}
	return e
}
func (TaskHandoffs) DiagnosticsTx(ctx context.Context, tx pgx.Tx) (map[string]any, error) {
	var pending, failed string
	var oldest *string
	e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE attempts<8)::text,count(*) FILTER(WHERE attempts>=8)::text,min(occurred_at)::text FROM display_task_handoffs WHERE translated_at IS NULL`).Scan(&pending, &failed, &oldest)
	return map[string]any{"pending": pending, "failed": failed, "oldest_at": oldest}, e
}

// TaskReferenceTx is used by source notification writers without public access.
func TaskReferenceTx(ctx context.Context, tx pgx.Tx, job string) (taskrecord.Ref, error) {
	var generation string
	e := tx.QueryRow(ctx, `SELECT execution_generation::text FROM display_package_jobs WHERE id=$1`, job).Scan(&generation)
	return taskrecord.Ref{SourceRef: taskrecord.SourceRef{Module: "display", Kind: "job", RootID: job}, Generation: generation}, e
}
