package eventbus

import (
	"context"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each scope seek and count uses the partial index on (restore_epoch, scope).
// Jump to the next scope without scanning all the preceding tenant's rows.
const tenantPressureSQL = `WITH RECURSIVE
 active AS (SELECT restore_epoch FROM event_protocol WHERE id=1 AND enabled AND NOT frozen AND restore_epoch=$1),
 scopes(scope,n) AS (
   (SELECT o.scope,1 FROM event_outbox o WHERE o.restore_epoch=(TABLE active)
     AND o.state IN ('pending','leased') AND o.scope<>'platform' AND o.scope>$2 ORDER BY o.scope LIMIT 1)
   UNION ALL
   SELECT candidate.scope,s.n+1 FROM scopes s CROSS JOIN LATERAL (
     SELECT o.scope FROM event_outbox o WHERE o.restore_epoch=(TABLE active)
      AND o.state IN ('pending','leased') AND o.scope<>'platform' AND o.scope>s.scope ORDER BY o.scope LIMIT 1
   ) candidate WHERE s.n<$3
 )
 SELECT s.scope,(SELECT count(*) FROM event_outbox o WHERE o.restore_epoch=(TABLE active)
   AND o.state IN ('pending','leased') AND o.scope<>'platform' AND o.scope=s.scope)
 FROM scopes s ORDER BY s.scope`

type tenantQueueSample struct {
	epoch, after string
	next         time.Time
}

func tenantDepths(ctx context.Context, pool *pgxpool.Pool, epoch, after string) (map[string]int64, string, error) {
	rows, err := pool.Query(ctx, tenantPressureSQL, epoch, after, diagnosticreport.MaxQueueDimensions)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	depths := make(map[string]int64, diagnosticreport.MaxQueueDimensions)
	var last string
	for rows.Next() {
		var count int64
		if err := rows.Scan(&last, &count); err != nil {
			return nil, "", err
		}
		depths[last] = count
	}
	if len(depths) < diagnosticreport.MaxQueueDimensions {
		last = ""
	}
	return depths, last, rows.Err()
}

// The relay samples between publish batches, before claiming any new work. It
// needs no dedicated connection even when LISTEN is disabled on a one-slot pool.
func (s *tenantQueueSample) collect(ctx context.Context, pool *pgxpool.Pool, epoch string, report *diagnosticreport.Reporter) {
	if epoch != s.epoch {
		s.epoch, s.after = epoch, ""
	}
	check, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	depths, after, err := tenantDepths(check, pool, epoch, s.after)
	if err == nil {
		s.after = after
	} else {
		depths = nil
	}
	diagnosticreport.SetTenantQueue(report, depths)
	s.next = time.Now().Add(30 * time.Second)
}
