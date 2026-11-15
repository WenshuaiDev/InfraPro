package monitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
)

// TaskIndex exposes only the safe owner metadata view.
type TaskIndex struct{ Module, Kind string }

func (s TaskIndex) BackfillTx(ctx context.Context, tx pgx.Tx, after string, limit int) ([]taskrecord.Change, string, bool, error) {
	rows, e := tx.Query(ctx, `SELECT id::text,snapshot FROM monitoring_task_metadata WHERE ($1::uuid IS NULL OR id>$1) AND snapshot IS NOT NULL AND snapshot->>'source_module'=$2 AND snapshot->>'source_kind'=$3 ORDER BY id LIMIT $4`, taskrecord.Null(after), s.Module, s.Kind, limit)
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
func (s TaskIndex) MissingTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	var missing bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_task_metadata v LEFT JOIN task_records r ON r.source_module=v.snapshot->>'source_module' AND r.source_kind=v.snapshot->>'source_kind' AND r.source_root_id=(v.snapshot->>'source_root_id')::uuid WHERE v.snapshot IS NOT NULL AND v.snapshot->>'source_module'=$1 AND v.snapshot->>'source_kind'=$2 AND (r.id IS NULL OR r.source_revision<v.task_revision))`, s.Module, s.Kind).Scan(&missing)
	return missing, e
}
