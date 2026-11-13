package derivedmonitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/exactsort"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

func retentionIDs(ctx context.Context, lease monitoring.TaskLease, sql, after string, parameters ...any) ([]string, error) {
	var ids []string
	args := append([]any{lease.Task().Scope.Tenant, after}, parameters...)
	e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, sql, args...)
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
	return ids, e
}

func (s *Service) retainCompletedCalculations(ctx context.Context, lease monitoring.TaskLease) error {
	if s.results == nil {
		return nil
	}
	if _, e := exactsort.PruneOrphans(ctx, s.results.TempDirectory(), time.Now().Add(-s.options.TemporaryTTL)); e != nil {
		return e
	}
	tenant := lease.Task().Scope.Tenant
	var after string
	for {
		ids, e := retentionIDs(ctx, lease, `SELECT r.id::text FROM derived_runs r JOIN monitoring_jobs j ON j.tenant_id=r.tenant_id AND j.id=r.job_id WHERE r.tenant_id=$1 AND j.state='succeeded' AND j.updated_at<clock_timestamp()-($3::bigint*interval '1 millisecond') AND ($2='' OR r.id::text>$2) ORDER BY r.id::text LIMIT 100`, after, s.options.TemporaryTTL.Milliseconds())
		if e != nil {
			return e
		}
		for _, id := range ids {
			pending, e := lease.BeginRetention(ctx, "derived_temp", id, map[string]string{"basis": "completed_run"})
			if e != nil {
				return e
			}
			if pending {
				if e = s.results.PurgeTemporaryRun(ctx, tenant, id, lease.Check); e != nil {
					return e
				}
				if e = lease.FinishRetention(ctx, "derived_temp", id); e != nil {
					return e
				}
			}
			if e = s.retainUnselectedAttempts(ctx, lease, id); e != nil {
				return e
			}
			after = id
		}
		if len(ids) < 100 {
			break
		}
	}
	if e := s.retainWindowCheckpoints(ctx, lease); e != nil {
		return e
	}
	return s.retainTrialReceipts(ctx, lease)
}

func (s *Service) retainUnselectedAttempts(ctx context.Context, lease monitoring.TaskLease, run string) error {
	tenant := lease.Task().Scope.Tenant
	var roots []string
	e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT manifest_root FROM derived_generations WHERE tenant_id=$1 AND run_id=$2 UNION ALL SELECT manifest_root FROM analysis_artifact_versions WHERE tenant_id=$1 AND run_id=$2`, tenant, run)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var root string
			if e = rows.Scan(&root); e != nil {
				return e
			}
			roots = append(roots, root)
		}
		return rows.Err()
	})
	if e != nil {
		return e
	}
	if len(roots) > 1 {
		return fmt.Errorf("retention_publication_identity_mismatch")
	}
	selected := map[string]bool{}
	for _, root := range roots {
		var manifest resultManifest
		if e = s.results.ReadDocument(ctx, tenant, "manifests", root, &manifest); e != nil {
			// A missing publication cannot prove which attempts are unreferenced.
			if provenResultCorruption(e) {
				return nil
			}
			return e
		}
		if manifest.Run != run {
			return fmt.Errorf("retention_publication_identity_mismatch")
		}
		for _, block := range manifest.Blocks {
			selected[block.Attempt] = true
		}
	}
	var after string
	for {
		ids, e := retentionIDs(ctx, lease, `SELECT a.id::text FROM derived_write_attempts a WHERE a.tenant_id=$1 AND a.run_id=$3 AND ($2='' OR a.id::text>$2) AND NOT EXISTS(SELECT 1 FROM monitoring_retention_receipts r WHERE r.tenant_id=a.tenant_id AND r.kind='derived_attempt' AND r.target=a.id::text AND r.state='completed') ORDER BY a.id::text LIMIT 100`, after, run)
		if e != nil {
			return e
		}
		for _, id := range ids {
			after = id
			if selected[id] {
				continue
			}
			var shard, chunk string
			if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT physical_shard,chunk_id::text FROM derived_write_attempts WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&shard, &chunk)
			}); e != nil {
				return e
			}
			pending, e := lease.BeginRetention(ctx, "derived_attempt", id, map[string]string{"run_id": run, "chunk_id": chunk, "physical_shard": shard})
			if e != nil {
				return e
			}
			if pending {
				if e = s.results.PurgeUnselectedAttempt(ctx, tenant, shard, chunk, id, lease.Check); e != nil {
					return e
				}
				if e = lease.FinishRetention(ctx, "derived_attempt", id); e != nil {
					return e
				}
			}
		}
		if len(ids) < 100 {
			return nil
		}
	}
}

func (s *Service) retainWindowCheckpoints(ctx context.Context, lease monitoring.TaskLease) error {
	tenant := lease.Task().Scope.Tenant
	var after string
	for {
		page, e := s.results.CheckpointObjects(ctx, tenant, after)
		if e != nil {
			return e
		}
		for _, object := range page {
			after = object.Hash
			if time.Since(object.Modified) < s.options.TemporaryTTL {
				continue
			}
			var document map[string]json.RawMessage
			if e = s.results.ReadDocument(ctx, tenant, "checkpoints", object.Hash, &document); e != nil {
				return e
			}
			var schema, run string
			_ = json.Unmarshal(document["schema"], &schema)
			_ = json.Unmarshal(document["run_id"], &run)
			if !slices.Contains([]string{"derived-window-stream/1", "derived-window-source/1", "derived-trial-checkpoint/1"}, schema) || !access.ValidID(run) {
				continue
			}
			var eligible bool
			if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_runs r JOIN monitoring_jobs j ON j.tenant_id=r.tenant_id AND j.id=r.job_id WHERE r.tenant_id=$1 AND r.id=$2 AND j.state='succeeded' AND j.updated_at<clock_timestamp()-($3::bigint*interval '1 millisecond'))`, tenant, run, s.options.TemporaryTTL.Milliseconds()).Scan(&eligible)
			}); e != nil {
				return e
			}
			if !eligible {
				continue
			}
			pending, e := lease.BeginRetention(ctx, "derived_checkpoint", object.Hash, map[string]string{"run_id": run, "schema": schema})
			if e != nil {
				return e
			}
			if pending {
				if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
					_, e := tx.Exec(ctx, `DELETE FROM monitoring_job_steps WHERE tenant_id=$1 AND job_id=(SELECT job_id FROM derived_runs WHERE tenant_id=$1 AND id=$2) AND result->>'document'=$3 AND (step_key LIKE 'window-stream:%' OR step_key LIKE 'window-source:%' OR step_key='trial-progress')`, tenant, run, object.Hash)
					return e
				}); e != nil {
					return e
				}
				if e = s.results.RemoveCheckpoint(ctx, tenant, object.Hash); e != nil {
					return e
				}
				if e = lease.FinishRetention(ctx, "derived_checkpoint", object.Hash); e != nil {
					return e
				}
			}
		}
		if len(page) < 100 {
			return nil
		}
	}
}

func (s *Service) retainTrialReceipts(ctx context.Context, lease monitoring.TaskLease) error {
	for _, kind := range []string{"derived_trial", "analysis_trial"} {
		table, column := "derived_trial_receipts", "id"
		query := `SELECT t.id::text FROM derived_trial_receipts t WHERE t.tenant_id=$1 AND t.expires_at<=clock_timestamp() AND ($2='' OR t.id::text>$2) AND NOT EXISTS(SELECT 1 FROM derived_execution_mandates m WHERE m.tenant_id=t.tenant_id AND m.basis->>'trial_receipt_id'=t.id::text) ORDER BY t.id::text LIMIT 100`
		if kind == "analysis_trial" {
			table, column = "analysis_trial_receipts", "run_id"
			query = `SELECT t.run_id::text FROM analysis_trial_receipts t WHERE t.tenant_id=$1 AND (t.result->>'expires_at')::timestamptz<=clock_timestamp() AND ($2='' OR t.run_id::text>$2) ORDER BY t.run_id::text LIMIT 100`
		}
		var after string
		for {
			ids, e := retentionIDs(ctx, lease, query, after)
			if e != nil {
				return e
			}
			for _, id := range ids {
				pending, e := lease.BeginRetention(ctx, kind, id, map[string]string{"basis": "expired_unreferenced_trial"})
				if e != nil {
					return e
				}
				if pending {
					if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
						_, e := tx.Exec(ctx, `DELETE FROM `+table+` WHERE tenant_id=$1 AND `+column+`=$2`, lease.Task().Scope.Tenant, id)
						return e
					}); e != nil {
						return e
					}
					if e = lease.FinishRetention(ctx, kind, id); e != nil {
						return e
					}
				}
				after = id
			}
			if len(ids) < 100 {
				break
			}
		}
	}
	return nil
}
