package derivedmonitoring

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
)

// This seam deliberately returns no run inputs, channel names or artifacts.
// Those retain AuthorizeTaskTx/readRunTx and their recursive data qualification.
func (s *Service) AuthorizeTaskSummaryTx(ctx context.Context, tx pgx.Tx, t monitoring.Task, a access.Actor, managed bool) error {
	scope := taskrecord.Scope{Kind: t.Scope.Entry, Tenant: t.Scope.Tenant, Project: t.Scope.Project}
	codes, _, _, e := tasks.ScopeTx(ctx, tx, a, scope)
	if e != nil {
		return e
	}
	if tasks.Has(codes, scope.Kind+".derived.runs.manage") || tasks.Has(codes, scope.Kind+".derived.runs.read") {
		return nil
	}
	return access.ErrDenied
}

// TaskResultStateTx evaluates only retained metadata and fixed-input validity;
// it does not load receipt bodies, raw values, objects or result manifests.
func (s *Service) TaskResultStateTx(ctx context.Context, tx pgx.Tx, t monitoring.Task, a access.Actor) (string, error) {
	if t.Job.State != "succeeded" || t.Job.Kind == "analysis_export" || t.Job.Kind == "derived_evidence_export" {
		return "", nil
	}
	var snapshot string
	e := tx.QueryRow(ctx, `SELECT snapshot_id::text FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2`, t.Scope.Tenant, t.Job.Target).Scan(&snapshot)
	if e != nil && e != pgx.ErrNoRows {
		return "", e
	}
	if e == nil {
		if t.Job.Kind == "analysis" {
			_, e = s.checkAnalysisInputsTx(ctx, tx, a, t.Scope, snapshot, false)
		} else if t.Job.Kind == "derive" {
			_, e = s.checkUserInputsTx(ctx, tx, a, t.Scope, snapshot, false)
		}
		var raw *monitoring.Error
		var domain *Error
		if errors.As(e, &raw) && raw.Code == "snapshot_stale" || errors.As(e, &domain) && domain.Code == "snapshot_stale" {
			return "invalidated", nil
		}
		if e != nil {
			return "", e
		}
	}
	if t.Job.Kind == "analysis" {
		var trial bool
		e = tx.QueryRow(ctx, `SELECT COALESCE((input->>'trial')::boolean,false) FROM monitoring_jobs WHERE id=$1`, t.Job.ID).Scan(&trial)
		if e != nil {
			return "", e
		}
		if trial {
			var live bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM analysis_trial_receipt_metadata WHERE tenant_id=$1 AND run_id=$2 AND expires_at>clock_timestamp())`, t.Scope.Tenant, t.Job.Target).Scan(&live); e != nil {
				return "", e
			}
			if !live {
				return "expired", nil
			}
		}
	}
	return "", nil
}
