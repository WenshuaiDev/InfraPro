package files

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
)

type TaskIndex struct{}

func (TaskIndex) BackfillTx(ctx context.Context, tx pgx.Tx, after string, limit int) ([]taskrecord.Change, string, bool, error) {
	rows, e := tx.Query(ctx, `SELECT id::text,snapshot FROM file_task_metadata WHERE ($1::uuid IS NULL OR id>$1) ORDER BY id LIMIT $2`, taskrecord.Null(after), limit)
	if e != nil {
		return nil, "", false, e
	}
	defer rows.Close()
	out := []taskrecord.Change{}
	next := after
	for rows.Next() {
		var c taskrecord.Change
		if e = rows.Scan(&next, &c); e != nil {
			return nil, "", false, e
		}
		out = append(out, c)
	}
	return out, next, len(out) < limit, rows.Err()
}
func (TaskIndex) MissingTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	var missing bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_task_metadata v LEFT JOIN task_records r ON r.source_module='files' AND r.source_kind='job' AND r.source_root_id=v.id WHERE r.id IS NULL OR r.source_revision<v.task_revision)`).Scan(&missing)
	return missing, e
}
