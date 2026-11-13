package monitoring

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5"
)

// CurrentRawCoverageTx exposes only authoritative adopted endpoint coverage.
// Calculation control uses it inside its authorized configuration transaction
// to retain future targets even when the source event was consumed earlier.
// Batch bounds can be conservative; they never manufacture observations.
func (s *Service) CurrentRawCoverageTx(ctx context.Context, tx pgx.Tx, tenant, channel string) ([]RawNeed, error) {
	rows, e := tx.Query(ctx, `SELECT (b.semantic_snapshot->'actual_coverage'->$2->>'first')::bigint,(b.semantic_snapshot->'actual_coverage'->$2->>'last')::bigint FROM monitoring_publications p JOIN monitoring_import_batches b ON b.tenant_id=p.tenant_id AND b.id=p.batch_id WHERE p.tenant_id=$1 AND p.revoke_sequence IS NULL AND b.semantic_snapshot->'actual_coverage' ? $2 ORDER BY p.sequence`, tenant, channel)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	windows := []Window{}
	for rows.Next() {
		var first, last int64
		if e = rows.Scan(&first, &last); e != nil {
			return nil, e
		}
		var to *int64
		if last < math.MaxInt64 {
			end := last + 1
			to = &end
		}
		windows = append(windows, Window{&first, to})
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	out := []RawNeed{}
	for _, w := range UnionWindows(windows) {
		out = append(out, RawNeed{channel, w.From, w.To})
	}
	return out, nil
}
