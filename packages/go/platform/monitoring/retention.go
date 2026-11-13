package monitoring

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
)

func (l TaskLease) BeginRetention(ctx context.Context, kind, target string, details map[string]string) (bool, error) {
	if l.job.Kind != "verify" || l.job.actor != "" || !slices.Contains([]string{"export", "derived_temp", "derived_attempt", "derived_checkpoint", "derived_trial", "analysis_trial"}, kind) || len(target) == 0 || len(target) > 200 || len(encodeJSON(details)) > 65536 {
		return false, access.ErrDenied
	}
	pending := false
	e := l.Transaction(ctx, func(tx pgx.Tx) error {
		var state string
		e := tx.QueryRow(ctx, `INSERT INTO monitoring_retention_receipts(tenant_id,kind,target,state,details,job_id) VALUES($1,$2,$3,'pending',$4,$5) ON CONFLICT(tenant_id,kind,target) DO UPDATE SET job_id=EXCLUDED.job_id WHERE monitoring_retention_receipts.state='pending' RETURNING state`, l.job.tenant, kind, target, encodeJSON(details), l.job.ID).Scan(&state)
		if e == pgx.ErrNoRows {
			return nil
		}
		pending = state == "pending"
		return e
	})
	return pending, e
}

func (l TaskLease) FinishRetention(ctx context.Context, kind, target string) error {
	if l.job.Kind != "verify" || l.job.actor != "" {
		return access.ErrDenied
	}
	return l.Transaction(ctx, func(tx pgx.Tx) error {
		updated, e := tx.Exec(ctx, `UPDATE monitoring_retention_receipts SET state='completed',completed_at=clock_timestamp() WHERE tenant_id=$1 AND kind=$2 AND target=$3 AND state='pending'`, l.job.tenant, kind, target)
		if e != nil || updated.RowsAffected() == 0 {
			return e
		}
		return access.Audit(ctx, tx, "monitoring.retention.completed", "monitoring_job", l.job.ID, "", nil, map[string]string{"kind": kind, "target": target}, access.Meta{ActorType: "system", ActorScope: "tenant", TenantID: l.job.tenant, Reason: "共享核验任务清理已过期且不再用于执行的临时产物"})
	})
}

// Only a caller-qualified temporary namespace reaches this private primitive.
// Small pages and a lease check before each removal bound memory and cancellation.
func (s *Storage) purgeObjects(ctx context.Context, prefix string, check func(context.Context) error) error {
	for {
		pageContext, cancel := context.WithCancel(ctx)
		objects := s.objects.ListObjects(pageContext, s.config.Bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true, MaxKeys: 100})
		count := 0
		for object := range objects {
			if object.Err != nil {
				cancel()
				return object.Err
			}
			if !strings.HasPrefix(object.Key, prefix) {
				cancel()
				return fmt.Errorf("temporary_object_prefix_mismatch")
			}
			if e := check(ctx); e != nil {
				cancel()
				return e
			}
			if e := s.objects.RemoveObject(ctx, s.config.Bucket, object.Key, minio.RemoveObjectOptions{}); e != nil {
				cancel()
				return e
			}
			count++
			if count == 100 {
				break
			}
		}
		cancel()
		if count < 100 {
			return nil
		}
	}
}

func (s *Service) pruneExpiredExports(ctx context.Context, lease TaskLease) error {
	if lease.job.Kind != "verify" || lease.job.actor != "" {
		return access.ErrDenied
	}
	var after string
	for {
		var ids []string
		e := lease.Transaction(ctx, func(tx pgx.Tx) error {
			rows, e := tx.Query(ctx, `SELECT e.id::text FROM monitoring_exports e JOIN monitoring_jobs j ON j.tenant_id=e.tenant_id AND j.id=e.job_id WHERE e.tenant_id=$1 AND e.expires_at<=clock_timestamp() AND j.state IN ('succeeded','failed','cancelled','needs_action') AND ($2='' OR e.id::text>$2) AND NOT EXISTS(SELECT 1 FROM monitoring_retention_receipts r WHERE r.tenant_id=e.tenant_id AND r.kind='export' AND r.target=e.id::text AND r.state='completed') ORDER BY e.id::text LIMIT 100`, lease.job.tenant, after)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				if e = rows.Scan(&id); e != nil {
					return e
				}
				ids = append(ids, id)
			}
			return rows.Err()
		})
		if e != nil {
			return e
		}
		for _, id := range ids {
			pending, e := lease.BeginRetention(ctx, "export", id, map[string]string{"source": "expired_managed_export"})
			if e != nil {
				return e
			}
			if pending {
				if e = s.storage.purgeObjects(ctx, lease.job.tenant+"/exports/"+id+"/", lease.Check); e != nil {
					return e
				}
				if e = lease.FinishRetention(ctx, "export", id); e != nil {
					return e
				}
			}
			after = id
		}
		if len(ids) < 100 {
			return nil
		}
	}
}
