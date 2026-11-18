package derivedmonitoring

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
	"strconv"
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

// Count only automatic records visible through this workspace's run reader.
func (s *Service) TaskAutomaticCountsTx(ctx context.Context, tx pgx.Tx, a access.Actor, fixed taskrecord.Scope) (string, string, string, error) {
	scope := Scope{Entry: fixed.Kind, Tenant: fixed.Tenant, Project: fixed.Project}
	rows, e := tx.Query(ctx, `SELECT r.id::text FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND r.mode='automatic' AND j.state IN ('queued','retry_wait','running','failed','needs_action')`, scope.Tenant)
	if e != nil {
		return "", "", "", e
	}
	ids, e := pgx.CollectRows(rows, pgx.RowTo[string])
	if e != nil {
		return "", "", "", e
	}
	var queued, running, failed int
	for _, id := range ids {
		run, err := s.readRunTx(ctx, tx, a, scope, id)
		if tasks.Denied(err) {
			continue
		}
		if err != nil {
			return "", "", "", err
		}
		switch run.Job.State {
		case "queued", "retry_wait":
			queued++
		case "running":
			running++
		case "failed", "needs_action":
			failed++
		}
	}
	return strconv.Itoa(queued), strconv.Itoa(running), strconv.Itoa(failed), nil
}

// TaskRunResultTx resolves a successful run to the immutable object it published.
func (s *Service) TaskRunResultTx(ctx context.Context, tx pgx.Tx, a access.Actor, fixed taskrecord.Scope, typ, run string) (string, string, map[string]string, error) {
	scope := Scope{Entry: fixed.Kind, Tenant: fixed.Tenant, Project: fixed.Project}
	if typ == "analysis.run" {
		if e := requireAnyDerivedTx(ctx, tx, a, scope, "analysis.results.read"); e != nil {
			return "", "", nil, e
		}
		target, e := analysisRunTargetTx(ctx, tx, scope.Tenant, run)
		if e != nil {
			return "", "", nil, e
		}
		if target.ArtifactID == "" || target.ArtifactVersionID == "" {
			return "", "", nil, pgx.ErrNoRows
		}
		proof, e := s.analysisArtifactProofTx(ctx, tx, a, scope, target.ArtifactID, target.ArtifactVersionID)
		if e != nil {
			return "", "", nil, e
		}
		return "analysis_artifact_version", proof.Version.ArtifactID, map[string]string{"version": proof.Version.ID}, nil
	}
	generation, e := s.publishedRunGenerationTx(ctx, tx, a, scope, run)
	if e != nil {
		return "", "", nil, e
	}
	return "derived_generation", generation, map[string]string{"run": run}, nil
}

// The caller has already authorized the run's channel, input and output ranges.
func (s *Service) publishedRunGenerationTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, run string) (string, error) {
	if e := requireAnyDerivedTx(ctx, tx, a, scope, "results.history"); e != nil {
		return "", e
	}
	var generation string
	e := tx.QueryRow(ctx, `SELECT id::text FROM derived_generations WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, run).Scan(&generation)
	return generation, e
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
