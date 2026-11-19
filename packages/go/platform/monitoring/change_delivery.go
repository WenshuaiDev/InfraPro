package monitoring

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrMissingChange = errors.New("monitoring change reference unavailable")

func verifyChangeTx(ctx context.Context, tx pgx.Tx, tenant, id string, position int64) error {
	var valid bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_changes c JOIN monitoring_tenant_states s USING(tenant_id)
	 WHERE c.tenant_id=$1 AND c.id=$2 AND c.source_position=$3 AND s.commit_sequence>=c.source_position)`, tenant, id, position).Scan(&valid)
	if e == nil && !valid {
		return ErrMissingChange
	}
	return e
}
func (s *Service) VerifyChange(ctx context.Context, tenant, id string, position int64) error {
	return s.db(ctx, func(tx pgx.Tx) error { return verifyChangeTx(ctx, tx, tenant, id, position) })
}

// Cache floors rule out only candidates that the owning snapshot/vector checks
// already invalidate. Existing pages and actual result records stay readable
// under their original snapshot and current authorization.
func (s *Service) ApplyCacheChange(ctx context.Context, consumer, tenant, id string, position int64) error {
	return s.db(ctx, func(tx pgx.Tx) error {
		if e := verifyChangeTx(ctx, tx, tenant, id, position); e != nil {
			return e
		}
		switch consumer {
		case "query-cache":
			_, e := tx.Exec(ctx, `INSERT INTO monitoring_cache_floors(consumer,tenant_id,domain_key,revision)
				SELECT 'query-cache',$1,r.channel_id,v.revoke_revision FROM monitoring_changes c JOIN monitoring_change_ranges r ON r.tenant_id=c.tenant_id AND r.event_id=c.id
				JOIN monitoring_channel_versions v ON v.tenant_id=r.tenant_id AND v.channel_id=r.channel_id
				WHERE c.tenant_id=$1 AND c.id=$2 AND c.change_type='raw_revoke' GROUP BY r.channel_id,v.revoke_revision
				ON CONFLICT(consumer,tenant_id,domain_key) DO UPDATE SET revision=greatest(monitoring_cache_floors.revision,EXCLUDED.revision)`, tenant, id)
			return e
		case "display-cache":
			_, e := tx.Exec(ctx, `INSERT INTO monitoring_cache_floors(consumer,tenant_id,domain_key,revision)
				VALUES('display-cache',$1,$1,$2)
				ON CONFLICT(consumer,tenant_id,domain_key) DO UPDATE SET revision=greatest(monitoring_cache_floors.revision,EXCLUDED.revision)`, tenant, position)
			return e
		default:
			return errors.New("unknown monitoring cache consumer")
		}
	})
}

// Publication already enqueued the complete set of maintenance jobs atomically.
// Attempt a bounded set by exact reference. The ordinary indexed queue retains
// the remaining channels, cancelled jobs and leases already held by a peer.
func (s *Service) WakeSummaries(ctx context.Context, tenant, id string, position int64) error {
	var ids []string
	e := s.db(ctx, func(tx pgx.Tx) error {
		if e := verifyChangeTx(ctx, tx, tenant, id, position); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT j.id::text FROM monitoring_jobs j WHERE j.tenant_id=$1 AND j.kind='summary'
			AND j.input->>'mode'='maintenance' AND j.input->>'watermark'=$3::bigint::text
			AND j.target_id IN (SELECT channel_id FROM monitoring_change_ranges WHERE tenant_id=$1 AND event_id=$2)
			AND j.state IN ('queued','retry_wait') AND j.available_at<=clock_timestamp() ORDER BY j.created_at,j.id LIMIT 8`, tenant, id, position)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var job string
			if e = rows.Scan(&job); e != nil {
				return e
			}
			ids = append(ids, job)
		}
		return rows.Err()
	})
	if e != nil {
		return e
	}
	for _, job := range ids {
		if _, e = s.RunTask(WorkerContext(ctx, IncrementalWorker), "summary-event-"+uuid.NewString(), tenant, job); e != nil {
			return e
		}
	}
	return nil
}
