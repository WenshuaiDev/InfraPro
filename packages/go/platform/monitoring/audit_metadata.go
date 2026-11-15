package monitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
)

// Map a source-owned fixed file/export reference to its metadata route. The
// caller must still pass NotificationTargetsTx before displaying its name.
func (s *Service) AuditReferenceTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, kind, id string) (string, string, error) {
	if a.ReadSubject() != nil {
		return "", "", access.ErrDenied
	}
	var target string
	if kind == "source" {
		e := tx.QueryRow(ctx, `SELECT id::text FROM monitoring_import_batches WHERE tenant_id=$1 AND source_file_id::text=$2 AND project_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid ORDER BY created_at,id LIMIT 1`, scope.Tenant, id, scope.Project).Scan(&target)
		return "monitoring_batch", target, e
	}
	e := tx.QueryRow(ctx, `SELECT x.job_id::text FROM monitoring_exports x JOIN monitoring_jobs j ON j.id=x.job_id WHERE x.tenant_id=$1 AND x.id::text=$2 AND j.project_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid`, scope.Tenant, id, scope.Project).Scan(&target)
	return "monitoring_job", target, e
}

// AuditExportTargetTx resolves metadata only, through the exact original
// source adapter (including managed analysis/evidence exports).
func (s *Service) AuditExportTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id, generation string) (tasks.Target, error) {
	var c taskrecord.Change
	e := tx.QueryRow(ctx, `SELECT m.snapshot FROM monitoring_exports x JOIN monitoring_task_metadata m ON m.id=x.job_id WHERE x.tenant_id=$1 AND x.id::text=$2`, scope.Tenant, id).Scan(&c)
	if e != nil {
		return tasks.Target{}, e
	}
	if c.Scope.Tenant != scope.Tenant || c.Scope.Project != scope.Project {
		return tasks.Target{}, access.ErrDenied
	}
	if generation == "" {
		return tasks.Target{Scope: c.Scope, Reason: "historical_result_unavailable"}, nil
	}
	return s.TaskTargetTx(ctx, tx, a, c.SourceRef, generation, "result")
}
