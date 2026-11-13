package monitoring

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
)

// ChangeRange is an exact nanosecond dependency interval. A nil upper endpoint
// includes the maximum stored timestamp without a max+1 overflow sentinel.
type ChangeRange struct {
	Channel string `json:"channel_id"`
	From    *int64 `json:"from_ns,string"`
	To      *int64 `json:"to_ns,string"`
}

// AppendChangeTx must run in the same transaction that advances commit_sequence
// and publishes the source change. The returned event is permanent evidence.
func AppendChangeTx(ctx context.Context, tx pgx.Tx, tenant string, position int64, kind, sourceKind, sourceID string, ranges []ChangeRange) (string, error) {
	ranges = slices.Clone(ranges)
	slices.SortFunc(ranges, func(a, b ChangeRange) int {
		if a.Channel < b.Channel {
			return -1
		}
		if a.Channel > b.Channel {
			return 1
		}
		if a.From == nil {
			if b.From == nil {
				return 0
			}
			return -1
		}
		if b.From == nil {
			return 1
		}
		if *a.From < *b.From {
			return -1
		}
		if *a.From > *b.From {
			return 1
		}
		return 0
	})
	var current int64
	if e := tx.QueryRow(ctx, `SELECT commit_sequence FROM monitoring_tenant_states WHERE tenant_id=$1 FOR UPDATE`, tenant).Scan(&current); e != nil {
		return "", e
	}
	if current != position || position <= 0 {
		return "", revisionChanged()
	}
	var id string
	if e := tx.QueryRow(ctx, `INSERT INTO monitoring_changes(tenant_id,source_position,change_type,source_kind,source_id,impact_hash) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, tenant, position, kind, sourceKind, sourceID, digest(ranges)).Scan(&id); e != nil {
		return "", e
	}
	for i, r := range ranges {
		if _, e := tx.Exec(ctx, `INSERT INTO monitoring_change_ranges(tenant_id,event_id,ordinal,channel_id,affected) VALUES($1,$2,$3,$4,$5::int8range)`, tenant, id, i, r.Channel, (Window{r.From, r.To}).pg()); e != nil {
			return "", e
		}
	}
	return id, nil
}
