package monitoring

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
)

// resolveLocationsTx selects exactly one verified location at the snapshot's
// route version. A retained old snapshot never follows a newer relocation.
func resolveLocationsTx(ctx context.Context, tx pgx.Tx, tenant string, entries []selectedChunk, route int64) error {
	if len(entries) == 0 {
		return nil
	}
	if route == 0 {
		if e := tx.QueryRow(ctx, `SELECT route_revision FROM monitoring_tenant_states WHERE tenant_id=$1`, tenant).Scan(&route); e != nil {
			return e
		}
	}
	ids := make([]string, len(entries))
	for i, v := range entries {
		ids[i] = v.Chunk
	}
	rows, e := tx.Query(ctx, `SELECT c.id::text,a.id::text,a.physical_shard,r.physical_shard,a.state FROM monitoring_chunks c JOIN monitoring_publication_chunks pc ON pc.tenant_id=c.tenant_id AND pc.chunk_id=c.id LEFT JOIN LATERAL(SELECT attempt_id FROM monitoring_chunk_locations WHERE tenant_id=c.tenant_id AND chunk_id=c.id AND route_revision<=$3 ORDER BY route_revision DESC LIMIT 1) l ON true JOIN monitoring_write_attempts a ON a.tenant_id=c.tenant_id AND a.id=COALESCE(l.attempt_id,pc.attempt_id) JOIN monitoring_routes r ON r.tenant_id=c.tenant_id AND r.logical_shard=c.logical_shard AND r.revision=$3 WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[])`, tenant, ids, route)
	if e != nil {
		return e
	}
	defer rows.Close()
	type location struct{ attempt, physical string }
	locations := map[string]location{}
	for rows.Next() {
		var id, attempt, physical, expected, state string
		if e = rows.Scan(&id, &attempt, &physical, &expected, &state); e != nil {
			return e
		}
		if state != "sealed" || physical != expected {
			return fmt.Errorf("route location is not sealed or mismatches route")
		}
		locations[id] = location{attempt, physical}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	for i := range entries {
		v, ok := locations[entries[i].Chunk]
		if !ok {
			return fmt.Errorf("missing route location")
		}
		entries[i].Attempt = v.attempt
		entries[i].Physical = v.physical
	}
	return nil
}

func mustRevision(v string) int64 { n, _ := revisionNumber(v); return n }
