package files

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"time"
)

// Each stored source revision is projected once with its original observation
// time and payload. Reconciliation replays this snapshot without reconstructing it.
func (s *Service) projectJobTx(ctx context.Context, tx pgx.Tx, id, eventActor, event, reason string) (taskrecord.Ref, error) {
	var c taskrecord.Change
	var input frozenInput
	var file, attachment, revision *string
	c.SourceRef = taskrecord.SourceRef{Module: "files", Kind: "job", RootID: id}
	c.Type = "files.prepare"
	c.OriginKind = "person"
	c.ExecutorKind = "person_delegation"
	c.EventActor = eventActor
	c.Event = event
	c.Reason = reason
	var raw []byte
	var completed int64
	var total *int64
	e := tx.QueryRow(ctx, `SELECT sp.scope_kind,sp.tenant_id::text,COALESCE(sp.project_id::text,''),j.originator_id::text,r.requested_by::text,r.executor_account_id::text,j.generation::text,j.source_revision::text,j.control_revision::text,j.state,j.phase,j.progress_completed,j.progress_total,j.progress_unit,j.error_code,r.input_snapshot,r.input_hash,r.result_file_id::text,r.result_attachment_id::text,r.result_revision_id::text,j.created_at,j.observed_at,r.fence::text,r.attempt_no FROM file_jobs j JOIN file_spaces sp ON sp.id=j.space_id JOIN file_job_runs r ON r.job_id=j.id AND r.generation=j.generation WHERE j.id=$1`, id).Scan(&c.Scope.Kind, &c.Scope.Tenant, &c.Scope.Project, &c.Originator, &c.RequestedBy, &c.ExecutorRef, &c.Generation, &c.Revision, &c.ControlRevision, &c.State, &c.Phase, &completed, &total, &c.Progress.Unit, &c.ErrorCode, &raw, &c.InputHash, &file, &attachment, &revision, &c.CreatedAt, &c.ObservedAt, &c.Fence, &c.Attempt)
	if e != nil {
		return taskrecord.Ref{}, e
	}
	if e = json.Unmarshal(raw, &input); e != nil {
		return taskrecord.Ref{}, e
	}
	c.SourceState = c.State
	c.Progress.Completed = number(completed)
	if total != nil {
		v := number(*total)
		c.Progress.Total = &v
	}
	c.InputRef = map[string]string{"job_id": id, "upload_id": input.UploadID}
	if input.PreparedFileID != "" {
		c.InputRef["prepared_file_id"] = input.PreparedFileID
	}
	c.ResultRef = map[string]string{}
	if file != nil {
		c.ResultRef["file_id"] = *file
	}
	if attachment != nil {
		c.ResultRef["attachment_id"] = *attachment
	}
	if revision != nil {
		c.ResultRef["attachment_revision_id"] = *revision
	}
	if c.State == "needs_action" {
		c.ResultRef["upload_id"] = input.UploadID
	}
	ref, e := taskrecord.PublishTx(ctx, tx, c)
	if e != nil {
		return ref, e
	}
	_, e = tx.Exec(ctx, `UPDATE file_jobs SET task_snapshot=$2 WHERE id=$1`, id, taskrecord.JSON(c))
	return ref, e
}
func (s *Service) terminalEventTx(ctx context.Context, tx pgx.Tx, id, actor string) error {
	var e notificationevent.Event
	var recipient notificationevent.Recipient
	var generation string
	e.Module = "files"
	e.Type = "task.result"
	e.Actor = actor
	e.Target = notificationevent.Target{Kind: "file_job", ID: id}
	recipient.Policy = "scope"
	recipient.Relationship = "initiator"
	err := tx.QueryRow(ctx, `SELECT sp.scope_kind,sp.tenant_id::text,COALESCE(sp.project_id::text,''),j.generation::text,j.originator_id::text,j.origin_tenant_membership_id::text,COALESCE(j.origin_project_membership_id::text,''),r.state,r.terminal_code,r.finished_at FROM file_jobs j JOIN file_spaces sp ON sp.id=j.space_id JOIN file_job_runs r ON r.job_id=j.id AND r.generation=j.generation WHERE j.id=$1`, id).Scan(&e.Scope.Kind, &e.Scope.Tenant, &e.Scope.Project, &generation, &recipient.Account, &recipient.TenantMembership, &recipient.ProjectMembership, &e.Outcome, &e.Reason, &e.OccurredAt)
	if err != nil {
		return err
	}
	e.Key = "file-job:" + id + ":generation:" + generation + ":terminal"
	ref := jobRef(id, generation)
	e.TaskReference = &ref
	e.Recipients = []notificationevent.Recipient{recipient}
	_, err = notificationevent.AppendTx(ctx, tx, e)
	return err
}
func (s *Service) finishTx(ctx context.Context, tx pgx.Tx, id, generation, state, code, actor, file, attachment, revision string, notify bool) error {
	phase := map[string]string{"succeeded": "completed", "failed": "failed", "cancelled": "cancelled", "needs_action": "waiting"}[state]
	now := s.now().Truncate(time.Microsecond)
	tag, e := tx.Exec(ctx, `UPDATE file_job_runs SET state=$3,terminal_code=$4,finished_at=$5,lease_until=NULL,result_file_id=$6,result_attachment_id=$7,result_revision_id=$8,input_retain_until=LEAST(input_retain_until,$9),safe_report=jsonb_build_object('schema_version',1,'code',$4::text,'state',$3::text) WHERE job_id=$1 AND generation=$2 AND finished_at IS NULL`, id, generation, state, code, now, taskrecord.Null(file), taskrecord.Null(attachment), taskrecord.Null(revision), now.Add(s.options.RetryInputTTL))
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fault("task_state_changed", 409)
	}
	_, e = tx.Exec(ctx, `UPDATE file_jobs SET state=$3,phase=$4,error_code=$5,progress_completed=CASE WHEN $3='succeeded' THEN COALESCE(progress_total,progress_completed) ELSE progress_completed END,source_revision=source_revision+1,observed_at=$6 WHERE id=$1 AND generation=$2`, id, generation, state, phase, code, now)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE file_job_attempts SET finished_at=$3,outcome=$4,error_code=$5 WHERE job_id=$1 AND generation=$2 AND finished_at IS NULL`, id, generation, now, state, code)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE file_spaces SET content_revision=content_revision+1 WHERE id=(SELECT space_id FROM file_jobs WHERE id=$1)`, id); e != nil {
		return e
	}
	if _, e = s.projectJobTx(ctx, tx, id, actor, "terminal", code); e != nil {
		return e
	}
	if notify {
		return s.terminalEventTx(ctx, tx, id, actor)
	}
	return nil
}
