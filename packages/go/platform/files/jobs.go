package files

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) jobTx(ctx context.Context, tx pgx.Tx, st txState, a access.Actor, id, generation string) (JobDetail, error) {
	var out JobDetail
	if !access.ValidID(id) || st.SpaceID == "" {
		return out, fault("file_not_found", 404)
	}
	if generation != "" {
		if _, e := integer(generation, 1); e != nil {
			return out, e
		}
	}
	var currentGeneration string
	e := tx.QueryRow(ctx, `SELECT id::text,upload_id::text,originator_id::text,generation::text,control_revision::text,state,phase,created_at FROM file_jobs WHERE space_id=$1 AND host_id=$2 AND id=$3`, st.SpaceID, st.HostID, id).Scan(&out.ID, &out.UploadID, &out.OriginatorID, &currentGeneration, &out.ControlRevision, &out.State, &out.Phase, &out.CreatedAt)
	if e != nil {
		return out, notFound(e)
	}
	if out.OriginatorID != a.ID && !st.Auth.ManagePermission {
		return JobDetail{}, fault("file_not_found", 404)
	}
	if generation == "" {
		generation = currentGeneration
	}
	out.Generation = generation
	out.TaskRef = jobRef(id, generation)
	var retain time.Time
	var epoch string
	e = tx.QueryRow(ctx, `SELECT requested_by::text,attempt_no,state,terminal_code,result_file_id::text,result_attachment_id::text,result_revision_id::text,finished_at,safe_report,input_retain_until,restore_epoch::text FROM file_job_runs WHERE job_id=$1 AND generation=$2`, id, generation).Scan(&out.RequestedBy, &out.Attempt, &out.State, &out.ErrorCode, &out.ResultFileID, &out.ResultAttachmentID, &out.ResultRevisionID, &out.FinishedAt, &out.Report, &retain, &epoch)
	if e != nil {
		return out, notFound(e)
	}
	if generation != currentGeneration {
		out.Phase = map[string]string{"succeeded": "completed", "failed": "failed", "cancelled": "cancelled", "needs_action": "waiting"}[out.State]
	}
	e = tx.QueryRow(ctx, `SELECT unbound_until FROM file_records WHERE upload_id=$1`, out.UploadID).Scan(&out.PreparedUntil)
	if e != nil && e != pgx.ErrNoRows {
		return out, e
	}
	out.ResultState = "none"
	out.ResultReadable = false
	if taskrecord.Active(out.State) {
		out.ResultState = "preparing"
	}
	if out.State == "needs_action" {
		out.ResultState = "preparing"
		if out.PreparedUntil == nil || !s.now().Before(*out.PreparedUntil) {
			out.ResultState = "expired"
		}
	}
	if out.ResultAttachmentID != nil {
		var status, lifecycle, integrity, object string
		e = tx.QueryRow(ctx, `SELECT a.status,f.lifecycle,f.integrity,o.state FROM file_attachments a JOIN file_attachment_revisions r ON r.attachment_id=a.id JOIN file_records f ON f.id=r.file_id JOIN file_objects o ON o.id=f.source_object_id WHERE a.space_id=$1 AND a.id=$2 AND r.id=$3`, st.SpaceID, *out.ResultAttachmentID, out.ResultRevisionID).Scan(&status, &lifecycle, &integrity, &object)
		if e != nil && e != pgx.ErrNoRows {
			return out, e
		}
		out.ResultState = "available"
		out.ResultReadable = e == nil
		if e == pgx.ErrNoRows || status != "active" {
			out.ResultState = "invalidated"
		} else if lifecycle != "ready" {
			out.ResultState = "expired"
		} else if integrity != "ok" || object != "available" {
			out.ResultState = "missing"
		}
	}
	out.AllowedActions = []Action{}
	if generation == currentGeneration {
		mutable := st.Auth.CanManage && st.Mode == "online"
		retry := out.State == "failed" && retryCode(out.ErrorCode) && s.now().Before(retain)
		if retry {
			valid, e := s.retryInputsTx(ctx, tx, out.UploadID)
			if e != nil {
				return out, e
			}
			retry = valid
		}
		out.AllowedActions = []Action{{Kind: "cancel", Enabled: mutable && epoch == st.Epoch && taskrecord.Active(out.State), RequiresReason: true}, {Kind: "retry", Enabled: mutable && retry, RequiresReason: true}, {Kind: "reconfirm", Enabled: mutable && out.State == "needs_action" && out.PreparedUntil != nil && s.now().Before(*out.PreparedUntil), RequiresReason: true}}
	}
	return out, nil
}
func retryCode(code string) bool {
	return code == "file_storage_unavailable" || code == "file_worker_unavailable" || code == "temporary_budget_exceeded" || code == "execution_expired" || code == "restore_interrupted" || code == "content_check_timeout" || code == "file_processing_failed"
}
func (s *Service) retryInputsTx(ctx context.Context, tx pgx.Tx, uploadID string) (bool, error) {
	var valid bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_records f JOIN file_objects o ON o.id=f.source_object_id WHERE f.upload_id=$1 AND f.lifecycle='ready' AND f.integrity='ok' AND o.state='available' AND f.unbound_until>$2) OR (NOT EXISTS(SELECT 1 FROM file_records WHERE upload_id=$1) AND EXISTS(SELECT 1 FROM file_uploads u WHERE u.id=$1 AND u.part_count=(SELECT count(*) FROM file_upload_parts p JOIN file_objects o ON o.id=p.object_id WHERE p.upload_id=u.id AND p.state='received' AND o.state='available')))`, uploadID, s.now()).Scan(&valid)
	return valid, e
}
func (s *Service) Job(ctx context.Context, a access.Actor, scope Scope, id, generation string) (JobDetail, error) {
	var out JobDetail
	e := s.transact(ctx, a, scope, false, false, "", func(tx pgx.Tx, st txState) error {
		var e error
		out, e = s.jobTx(ctx, tx, st, a, id, generation)
		return e
	})
	return out, e
}
func (s *Service) Reconfirm(ctx context.Context, a access.Actor, scope Scope, uploadID string, in ReconfirmInput) (CommandResult, error) {
	var out CommandResult
	if e := validateMeta(in.WriteMeta); e != nil {
		return out, e
	}
	payload := struct {
		ID    string
		Input ReconfirmInput
	}{uploadID, in}
	e := s.transact(ctx, a, scope, true, false, in.Epoch, func(tx pgx.Tx, st txState) error {
		var replay bool
		var e error
		out, replay, e = receiptTx(ctx, tx, st, a, in.WriteMeta, "upload.reconfirm", payload)
		if replay || e != nil {
			return e
		}
		if e = st.mutable(); e != nil {
			return e
		}
		u, e := uploadTx(ctx, tx, st.SpaceID, uploadID, true)
		if e != nil {
			return e
		}
		var id string
		e = tx.QueryRow(ctx, `SELECT id::text FROM file_jobs WHERE upload_id=$1 FOR UPDATE`, uploadID).Scan(&id)
		if e != nil {
			return notFound(e)
		}
		j, e := s.jobTx(ctx, tx, st, a, id, "")
		if e != nil {
			return e
		}
		if j.Generation != in.ExpectedGeneration || j.ControlRevision != in.ExpectedControlRevision || j.State != "needs_action" {
			return fault("task_revision_changed", 409)
		}
		if j.PreparedUntil == nil || !s.now().Before(*j.PreparedUntil) || j.ResultFileID == nil {
			return fault("prepared_file_expired", 410)
		}
		if u.Intent == "replace" {
			target, e := attachmentTx(ctx, tx, st, value(u.AttachmentID), true)
			if e != nil {
				return e
			}
			if target.Status != "active" || target.Revision != value(in.ExpectedAttachmentRevision) {
				return fault("attachment_revision_changed", 409)
			}
		} else if in.ExpectedAttachmentRevision != nil {
			return fault("invalid_request", 400)
		}
		if e = s.lockReadyTx(ctx, tx, st.SpaceID, st.HostID, *j.ResultFileID); e != nil {
			return e
		}
		out, e = s.newGenerationTx(ctx, tx, st, a, j, in.ExpectedAttachmentRevision, value(j.ResultFileID), in.Reason)
		if e != nil {
			return e
		}
		return s.commitReceipt(ctx, tx, st, a, in.WriteMeta, "upload.reconfirm", uploadID, payload, out)
	})
	return out, e
}
func (s *Service) newGenerationTx(ctx context.Context, tx pgx.Tx, st txState, a access.Actor, j JobDetail, expected *string, prepared, reason string) (CommandResult, error) {
	var out CommandResult
	if e := s.queueBudgetTx(ctx, tx, st.Auth.Scope.TenantID); e != nil {
		return out, e
	}
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT input_snapshot FROM file_job_runs WHERE job_id=$1 AND generation=$2`, j.ID, j.Generation).Scan(&raw)
	if e != nil {
		return out, e
	}
	input, e := decodeInput(raw)
	if e != nil {
		return out, e
	}
	input.ExpectedRevision = expected
	input.PreparedFileID = prepared
	input.Reason = reason
	generation, _ := integer(j.Generation, 1)
	generation++
	now := s.now()
	deadline := now.Add(s.options.QueueTimeout)
	if prepared != "" && j.PreparedUntil != nil {
		deadline = minTime(deadline, *j.PreparedUntil)
	}
	_, e = tx.Exec(ctx, `INSERT INTO file_job_runs(space_id,job_id,generation,requested_by,executor_account_id,tenant_membership_id,project_membership_id,input_snapshot,input_hash,expected_attachment_revision,state,available_at,deadline_at,input_retain_until,restore_epoch) VALUES($1,$2,$3,$4,$4,$5,$6,$7,$8,$9,'queued',$10,$11,$12,$13)`, st.SpaceID, j.ID, generation, a.ID, st.Auth.TenantMembershipID, taskrecord.Null(st.Auth.ProjectMembershipID), taskrecord.JSON(input), taskrecord.Hash(input), expected, now, deadline, deadline.Add(s.options.RetryInputTTL), st.Epoch)
	if e != nil {
		return out, e
	}
	var control string
	e = tx.QueryRow(ctx, `UPDATE file_jobs SET generation=$2,source_revision=source_revision+1,control_revision=control_revision+1,state='queued',phase='queued',error_code='',progress_completed=0,observed_at=$3 WHERE id=$1 RETURNING control_revision::text`, j.ID, generation, now).Scan(&control)
	if e != nil {
		return out, e
	}
	ref, e := s.projectJobTx(ctx, tx, j.ID, a.ID, "new_generation", reason)
	if e != nil {
		return out, e
	}
	gen := number(generation)
	out = CommandResult{UploadID: &j.UploadID, Revision: control, State: "queued", JobID: &j.ID, JobGeneration: &gen, JobControlRevision: &control, TaskRef: &ref, Changed: true}
	return out, nil
}
func (s *Service) taskScope(ctx context.Context, a access.Actor, ref taskrecord.SourceRef) (Scope, string, error) {
	var scope Scope
	var epoch string
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if a.Check == nil || a.ReadSubject() != nil || ref.Module != "files" || ref.Kind != "job" || !access.ValidID(ref.RootID) {
			return access.ErrDenied
		}
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if e := a.Check(ctx, tx, false); e != nil {
			return e
		}
		views, e := s.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
		if e != nil {
			return e
		}
		if len(views) != 1 {
			return tasks.Hidden()
		}
		scope = Scope{Kind: views[0].Scope.Kind, TenantID: views[0].Scope.Tenant, ProjectID: views[0].Scope.Project}
		return tx.QueryRow(ctx, `SELECT restore_epoch::text FROM file_protocol WHERE id=1`).Scan(&epoch)
	})
	return scope, epoch, e
}
func (s *Service) TaskControl(ctx context.Context, a access.Actor, ref taskrecord.SourceRef, in tasks.Command) (tasks.CommandResult, error) {
	var out tasks.CommandResult
	if e := in.Validate(); e != nil {
		return out, e
	}
	scope, epoch, e := s.taskScope(ctx, a, ref)
	if e != nil {
		return out, e
	}
	reason := in.Reason
	if reason == "" {
		reason = "取消本人文件处理"
	}
	meta := WriteMeta{RequestID: in.RequestID, Reason: reason, Epoch: epoch}
	payload := struct {
		ID    string
		Input tasks.Command
	}{ref.RootID, in}
	e = s.transact(ctx, a, scope, true, false, epoch, func(tx pgx.Tx, st txState) error {
		receipt, replay, e := receiptTx(ctx, tx, st, a, meta, "task."+in.Action, payload)
		if e != nil {
			return e
		}
		if replay {
			out = tasks.CommandResult{TaskID: ref.ID(), Generation: value(receipt.JobGeneration), State: receipt.State, CommandState: "applied", Replayed: true}
			return nil
		}
		if e = st.mutable(); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT id FROM file_jobs WHERE space_id=$1 AND id=$2 FOR UPDATE`, st.SpaceID, ref.RootID); e != nil {
			return e
		}
		views, e := s.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
		if e != nil {
			return e
		}
		if len(views) != 1 {
			return tasks.Hidden()
		}
		if e = tasks.ValidateCommand(in, views[0], a); e != nil {
			return e
		}
		j, e := s.jobTx(ctx, tx, st, a, ref.RootID, "")
		if e != nil {
			return e
		}
		var result CommandResult
		if in.Action == "retry" {
			var expected *string
			e = tx.QueryRow(ctx, `SELECT expected_attachment_revision::text FROM file_job_runs WHERE job_id=$1 AND generation=$2`, j.ID, j.Generation).Scan(&expected)
			if e != nil {
				return e
			}
			result, e = s.newGenerationTx(ctx, tx, st, a, j, expected, value(j.ResultFileID), reason)
			if e != nil {
				return e
			}
		} else {
			_, e = tx.Exec(ctx, `UPDATE file_job_runs SET state='cancelling',cancel_requested_at=$3,fence=fence+1 WHERE job_id=$1 AND generation=$2 AND finished_at IS NULL`, j.ID, j.Generation, s.now())
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE file_jobs SET state='cancelling',control_revision=control_revision+1,source_revision=source_revision+1,observed_at=$2 WHERE id=$1`, j.ID, s.now())
			if e != nil {
				return e
			}
			if _, e = s.projectJobTx(ctx, tx, j.ID, a.ID, "cancel_requested", reason); e != nil {
				return e
			}
			if e = s.finishTx(ctx, tx, j.ID, j.Generation, "cancelled", "user_cancelled", a.ID, value(j.ResultFileID), "", "", true); e != nil {
				return e
			}
			result = CommandResult{JobID: &j.ID, JobGeneration: &j.Generation, State: "cancelled", Changed: true}
		}
		out = tasks.CommandResult{TaskID: ref.ID(), Generation: value(result.JobGeneration), State: result.State, CommandState: "applied"}
		return s.commitReceipt(ctx, tx, st, a, meta, "task."+in.Action, j.ID, payload, result)
	})
	return out, e
}
func isDenied(e error) bool {
	var fe *Error
	return tasks.Denied(e) || errors.As(e, &fe) && (fe.Status == 403 || fe.Status == 404)
}
