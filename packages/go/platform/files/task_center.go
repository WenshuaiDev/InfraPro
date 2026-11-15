package files

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
)

// metadataStateTx is used only inside an already authenticated task/notification
// transaction. It does not manufacture a session-bearing actor for writes.
func (s *Service) metadataStateTx(ctx context.Context, tx pgx.Tx, a access.Actor, id string) (txState, taskrecord.Change, error) {
	var st txState
	var c taskrecord.Change
	if a.ReadSubject() != nil || !access.ValidID(a.ID) || !access.ValidID(id) {
		return st, c, access.ErrDenied
	}
	e := tx.QueryRow(ctx, `SELECT space_id::text,host_id::text,task_snapshot FROM file_jobs WHERE id=$1`, id).Scan(&st.SpaceID, &st.HostID, &c)
	if e != nil {
		return st, c, notFound(e)
	}
	scope := Scope{Kind: c.Scope.Kind, TenantID: c.Scope.Tenant, ProjectID: c.Scope.Project}
	st.Auth, e = s.hosts.ResolveTx(ctx, tx, a, scope, scope.Host(), Metadata)
	if e != nil {
		return st, c, e
	}
	if c.Originator != a.ID && !st.Auth.ManagePermission {
		return st, c, tasks.Hidden()
	}
	e = tx.QueryRow(ctx, `SELECT restore_epoch::text,mode FROM file_protocol WHERE id=1`).Scan(&st.Epoch, &st.Mode)
	return st, c, e
}
func (s *Service) TaskResolveTx(ctx context.Context, tx pgx.Tx, a access.Actor, refs []taskrecord.SourceRef, mode string) ([]tasks.View, error) {
	out := []tasks.View{}
	for _, ref := range refs {
		if ref.Module != "files" || ref.Kind != "job" {
			continue
		}
		st, c, e := s.metadataStateTx(ctx, tx, a, ref.RootID)
		if isDenied(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if mode == "mine" && c.Originator != a.ID || mode == "managed" && !st.Auth.ManagePermission {
			continue
		}
		_, label, _, e := tasks.ScopeTx(ctx, tx, a, c.Scope)
		if e != nil {
			return nil, e
		}
		j, e := s.jobTx(ctx, tx, st, a, ref.RootID, c.Generation)
		if e != nil {
			return nil, e
		}
		v := tasks.Present(c, label)
		v.ResultState = j.ResultState
		v.ResultReadable = j.ResultReadable
		v.NextStep = "open_file"
		if j.State == "needs_action" {
			v.NextStep = "reconfirm"
		}
		v.ActorLabel = "其他成员"
		if c.Originator == a.ID {
			v.ActorLabel = "我"
		}
		for _, cap := range j.AllowedActions {
			if cap.Kind == "cancel" || cap.Kind == "retry" {
				tasks.Capability(&v, cap.Kind, cap.Enabled, cap.RequiresReason, "file_action_unavailable")
			}
		}
		if j.State == "cancelled" {
			v.CleanupState = "pending"
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) ResultForExecutionTx(ctx context.Context, tx pgx.Tx, a access.Actor, ref taskrecord.SourceRef, generation string) (tasks.ExecutionResult, error) {
	st, _, e := s.metadataStateTx(ctx, tx, a, ref.RootID)
	if e != nil {
		return tasks.ExecutionResult{}, e
	}
	j, e := s.jobTx(ctx, tx, st, a, ref.RootID, generation)
	if e != nil {
		return tasks.ExecutionResult{}, e
	}
	next := "open_file"
	if j.State == "needs_action" {
		next = "reconfirm"
	}
	return tasks.ExecutionResult{ResultState: j.ResultState, ResultReadable: j.ResultReadable, NextStep: next}, nil
}
func (s *Service) TaskTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, ref taskrecord.SourceRef, generation, kind string) (tasks.Target, error) {
	st, c, e := s.metadataStateTx(ctx, tx, a, ref.RootID)
	if e != nil {
		return tasks.Target{}, e
	}
	j, e := s.jobTx(ctx, tx, st, a, ref.RootID, generation)
	if e != nil {
		return tasks.Target{}, e
	}
	out := tasks.Target{Scope: c.Scope, Generation: j.Generation, Parameters: map[string]string{"job": j.ID, "upload": j.UploadID}}
	switch kind {
	case "business":
		out.Available = true
		out.Kind = "file_upload"
		out.ID = j.UploadID
	case "recreate":
		out.Available = st.Auth.CanManage && st.Mode == "online"
		out.Kind = "file_host"
		out.ID = st.Auth.Host.ID
	case "result":
		if j.ResultAttachmentID != nil && j.ResultReadable {
			out.Available = true
			out.Kind = "file_attachment"
			out.ID = *j.ResultAttachmentID
			out.Parameters["revision"] = value(j.ResultRevisionID)
		} else {
			out.Reason = "result_" + j.ResultState
		}
	default:
		return tasks.Target{}, access.ErrDenied
	}
	return out, nil
}
func (s *Service) TaskOperationsTx(ctx context.Context, tx pgx.Tx, a access.Actor, f tasks.Filter) ([]tasks.Operation, error) {
	return tasks.ScopedOperationsTx(ctx, tx, a, f, "files", func(ctx context.Context, tx pgx.Tx, scope taskrecord.Scope) (string, string, string, error) {
		var q, r, failed string
		e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE j.state IN ('queued','retry_wait'))::text,count(*) FILTER(WHERE j.state IN ('running','cancelling'))::text,count(*) FILTER(WHERE j.state='failed')::text FROM file_jobs j JOIN file_spaces sp ON sp.id=j.space_id WHERE sp.tenant_id=$1 AND sp.project_id IS NOT DISTINCT FROM $2::uuid`, scope.Tenant, taskrecord.Null(scope.Project)).Scan(&q, &r, &failed)
		return q, r, failed, e
	})
}
func (s *Service) NotificationTargetsTx(ctx context.Context, tx pgx.Tx, account string, scope Scope, ids []string) (map[string]notificationevent.TargetContext, error) {
	out := map[string]notificationevent.TargetContext{}
	for _, id := range ids {
		st, c, e := s.metadataStateTx(ctx, tx, access.Actor{ID: account}, id)
		if isDenied(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if c.Scope.Kind != scope.Kind || c.Scope.Tenant != scope.TenantID || c.Scope.Project != scope.ProjectID {
			continue
		}
		var name string
		e = tx.QueryRow(ctx, `SELECT u.display_name FROM file_jobs j JOIN file_uploads u ON u.id=j.upload_id WHERE j.id=$1 AND j.space_id=$2`, id, st.SpaceID).Scan(&name)
		if e != nil {
			return nil, e
		}
		out[id] = notificationevent.TargetContext{Allowed: true, Available: true, Name: name, State: c.State}
	}
	return out, nil
}
