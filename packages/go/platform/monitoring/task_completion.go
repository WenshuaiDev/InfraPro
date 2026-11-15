package monitoring

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"slices"

	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

// TaskOutcomeWriter lets the owning domain record its automatic failure cycle.
// Actor-less display/read subjobs and internal monitoring jobs have no inbox.
type TaskOutcomeWriter interface {
	TaskOutcomeTx(context.Context, pgx.Tx, Task, string, string) error
}

func (l TaskLease) CompleteTx(ctx context.Context, tx pgx.Tx, progress any) error {
	if l.service == nil {
		return problem("job_lease_lost", "任务租约已失效。", 409)
	}
	return l.service.FinalizeTaskTx(ctx, tx, l.job.ID, l.job.fence, "succeeded", "", progress)
}

// FinalizeTaskTx is the only direct-success publisher. The caller's result,
// task terminal state and notification fact commit or roll back together.
func (s *Service) FinalizeTaskTx(ctx context.Context, tx pgx.Tx, id string, fence int64, state, code string, progress any) error {
	if !slices.Contains([]string{"succeeded", "failed", "needs_action", "cancelled"}, state) {
		return invalid("state", "无效任务终态。")
	}
	tag, e := tx.Exec(ctx, `UPDATE monitoring_jobs SET state=$3,error_code=NULLIF($4,''),progress=COALESCE($5::jsonb,progress),lease_owner=NULL,lease_until=NULL,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 AND fence=$2 AND state='running' AND lease_until>clock_timestamp() AND (NOT cancel_requested OR $3='cancelled')`, id, fence, state, code, func() any {
		if progress == nil {
			return nil
		}
		return encodeJSON(progress)
	}())
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return problem("job_lease_lost", "任务租约已失效。", 409)
	}
	return s.RecordTaskOutcomeTx(ctx, tx, id)
}

// RecordTaskOutcomeTx also handles already-validated cancellation and lease
// recovery transitions. Nonterminal retries are intentionally ignored.
func (s *Service) RecordTaskOutcomeTx(ctx context.Context, tx pgx.Tx, id string) error {
	j, e := scanJob(tx.QueryRow(ctx, jobSelect+` WHERE id=$1`, id))
	if e != nil {
		return e
	}
	if !slices.Contains([]string{"succeeded", "failed", "needs_action", "cancelled"}, j.State) {
		return nil
	}
	var generation int64
	var code string
	if e = tx.QueryRow(ctx, `SELECT notification_generation,COALESCE(error_code,'') FROM monitoring_jobs WHERE id=$1`, id).Scan(&generation, &code); e != nil {
		return e
	}
	if j.State == "cancelled" && code == "" {
		code = "job_cancelled"
	}
	if writer, ok := s.executors[j.Kind].(TaskOutcomeWriter); ok {
		if e = writer.TaskOutcomeTx(ctx, tx, taskOf(j), j.State, code); e != nil {
			return e
		}
	}
	if j.actor == "" || !slices.Contains(append([]string{"validate", "publish", "export"}, calculationKinds...), j.Kind) {
		return nil
	}
	kind, target, outcome := "monitoring_job", j.ID, j.State
	if j.Kind == "validate" || j.Kind == "publish" {
		kind, target = "monitoring_import", j.Target
		if outcome == "succeeded" {
			if j.Kind == "validate" {
				outcome = "validated"
				var batchState string
				if e = tx.QueryRow(ctx, `SELECT state FROM monitoring_import_batches WHERE id=$1`, j.Target).Scan(&batchState); e != nil {
					return e
				}
				if batchState != "ready" {
					outcome = "needs_action"
					code = "validation_requires_resolution"
				}
			} else {
				outcome = "adopted"
			}
		}
	} else if slices.Contains(calculationKinds, j.Kind) {
		kind = "derived_job"
	}
	var fact *taskrecord.Change
	if e = tx.QueryRow(ctx, `SELECT snapshot FROM monitoring_task_metadata WHERE id=$1`, id).Scan(&fact); e != nil {
		return e
	}
	var ref *taskrecord.Ref
	requestedBy := ""
	if fact != nil {
		ref = &taskrecord.Ref{SourceRef: fact.SourceRef, Generation: fact.Generation}
		requestedBy = fact.RequestedBy
	}
	return ne.EmitTx(ctx, tx, ne.Event{TaskReference: ref, Module: "monitoring", Key: fmt.Sprintf("%s:%d:%s", id, generation, outcome), Type: "task.result", Scope: ne.BusinessScope(j.tenant, j.project), Target: ne.Target{Kind: kind, ID: target}, Outcome: outcome, Reason: code, Recipients: ne.Personal(j.actor, requestedBy)})
}
func (s *Service) recordImportOutcomesTx(ctx context.Context, tx pgx.Tx, tenant, batch string) error {
	rows, e := tx.Query(ctx, `SELECT id::text FROM monitoring_jobs WHERE tenant_id=$1 AND target_id=$2 AND kind IN ('validate','publish') AND state='cancelled'`, tenant, batch)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if e = s.RecordTaskOutcomeTx(ctx, tx, id); e != nil {
			return e
		}
	}
	return nil
}
