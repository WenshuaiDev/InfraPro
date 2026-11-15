package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
)

// metadataExportTx is only used under the task/notification reader's already
// authenticated transaction; it still performs the current domain scope gate.
func (s *Service) metadataExportTx(ctx context.Context, tx pgx.Tx, a access.Actor, id, generation string) (exportRecord, authorization, error) {
	v := exportRecord{}
	auth := authorization{}
	if a.ReadSubject() != nil || !ar.ID(a.ID) || !ar.ID(id) {
		return v, auth, access.ErrDenied
	}
	v, e := loadExportTx(ctx, tx, id, generation, false)
	if e != nil {
		return v, auth, e
	}
	rev, e := access.Lock(ctx, tx, false)
	if e != nil {
		return v, auth, e
	}
	auth.AuditAuthorization, e = access.AuditAuthorizationTx(ctx, tx, a, v.Scope, rev)
	if e != nil {
		return v, auth, e
	}
	if e = tx.QueryRow(ctx, `SELECT restore_epoch::text,cursor_secret,projection_version::text,mode FROM audit_protocol WHERE id=1 AND version=1`).Scan(&auth.Epoch, &auth.Secret, &auth.Projection, &auth.Mode); e != nil {
		return v, auth, e
	}
	auth.Vector, e = s.vectorTx(ctx, tx, a, auth)
	if e != nil {
		return v, auth, e
	}
	if v.Owner != a.ID && !auth.Has("exports.manage") {
		return v, auth, Problem("audit_not_found", 404)
	}
	return v, auth, nil
}
func (s *Service) TaskResolveTx(ctx context.Context, tx pgx.Tx, a access.Actor, refs []taskrecord.SourceRef, mode string) ([]tasks.View, error) {
	out := []tasks.View{}
	if len(refs) > 100 {
		return nil, Problem("audit_filter_invalid", 400)
	}
	for _, ref := range refs {
		if ref.Module != "audit" || ref.Kind != "export" {
			continue
		}
		v, auth, e := s.metadataExportTx(ctx, tx, a, ref.RootID, "")
		if tasks.Denied(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if mode == "mine" && v.Owner != a.ID || mode == "managed" && !auth.Has("exports.manage") {
			continue
		}
		p, e := s.presentExport(v, a, auth)
		if e != nil {
			return nil, e
		}
		var c taskrecord.Change
		if e = tx.QueryRow(ctx, `SELECT task_snapshot FROM audit_exports WHERE id=$1`, v.ID).Scan(&c); e != nil {
			return nil, e
		}
		_, label, _, e := tasks.ScopeTx(ctx, tx, a, c.Scope)
		if e != nil {
			return nil, e
		}
		view := tasks.Present(c, label)
		view.ActorLabel = "我"
		if v.Owner != a.ID {
			view.ActorLabel = "其他成员"
		}
		view.ResultState = p.ResultState
		view.ResultReadable = p.Owned
		view.NextStep = p.NextAction
		for _, action := range p.AllowedActions {
			if action.Kind == "cancel" || action.Kind == "retry" {
				view.Actions = append(view.Actions, action)
			}
		}
		out = append(out, view)
	}
	return out, nil
}
func (s *Service) ResultForExecutionTx(ctx context.Context, tx pgx.Tx, a access.Actor, ref taskrecord.SourceRef, generation string) (tasks.ExecutionResult, error) {
	v, auth, e := s.metadataExportTx(ctx, tx, a, ref.RootID, generation)
	if e != nil {
		return tasks.ExecutionResult{}, e
	}
	p, e := s.presentExport(v, a, auth)
	return tasks.ExecutionResult{ResultState: p.ResultState, ResultReadable: p.Owned, NextStep: p.NextAction}, e
}
func (s *Service) TaskTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, ref taskrecord.SourceRef, generation, kind string) (tasks.Target, error) {
	v, auth, e := s.metadataExportTx(ctx, tx, a, ref.RootID, generation)
	if e != nil {
		return tasks.Target{}, e
	}
	p, e := s.presentExport(v, a, auth)
	if e != nil {
		return tasks.Target{}, e
	}
	out := tasks.Target{Kind: "audit_export", Scope: taskrecord.Scope{Kind: v.Scope.Kind, Tenant: v.Scope.Tenant, Project: v.Scope.Project}, ID: v.ID, Generation: v.Generation}
	switch kind {
	case "business":
		out.Available = true
	case "result":
		out.Available = p.Owned
	case "recreate":
		out.Kind = "audit_export_recreate"
		out.Available = p.Owned && auth.Has("export")
	default:
		return tasks.Target{}, access.ErrDenied
	}
	if !out.Available {
		out.Reason = "result_restricted"
	}
	return out, nil
}
func (s *Service) TaskOperationsTx(ctx context.Context, tx pgx.Tx, a access.Actor, f tasks.Filter) ([]tasks.Operation, error) {
	return tasks.ScopedOperationsTx(ctx, tx, a, f, "audit", func(ctx context.Context, tx pgx.Tx, scope taskrecord.Scope) (string, string, string, error) {
		var q, r, failed string
		e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE g.state IN ('queued','retry_wait'))::text,count(*) FILTER(WHERE g.state IN ('running','cancelling'))::text,count(*) FILTER(WHERE g.state='failed')::text FROM audit_exports x JOIN audit_export_generations g ON g.export_id=x.id AND g.generation=x.current_generation WHERE x.scope_kind=$1 AND x.tenant_id IS NOT DISTINCT FROM $2::uuid AND x.project_id IS NOT DISTINCT FROM $3::uuid`, scope.Kind, taskrecord.Null(scope.Tenant), taskrecord.Null(scope.Project)).Scan(&q, &r, &failed)
		return q, r, failed, e
	})
}
func (s *Service) ExistsTx(ctx context.Context, tx pgx.Tx, ref taskrecord.SourceRef) (bool, error) {
	var exists bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_exports WHERE id=$1)`, ref.RootID).Scan(&exists)
	return exists, e
}
func (s *Service) TaskControl(ctx context.Context, a access.Actor, ref taskrecord.SourceRef, c tasks.Command) (tasks.CommandResult, error) {
	out := tasks.CommandResult{}
	if ref.Module != "audit" || ref.Kind != "export" || !ar.ID(ref.RootID) {
		return out, tasks.Hidden()
	}
	if e := c.Validate(); e != nil {
		return out, e
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		v, e := loadExportTx(ctx, tx, ref.RootID, "", true)
		if e != nil {
			return e
		}
		auth, e := s.authorizeTx(ctx, tx, a, v.Scope)
		if e != nil {
			return e
		}
		if auth.Mode != "ready" {
			return Problem("audit_protocol_unavailable", 503)
		}
		p, e := s.presentExport(v, a, auth)
		if e != nil {
			return e
		}
		event := "audit.export.cancel.accepted"
		if c.Action == "retry" {
			event = "audit.export.retry"
		}
		receipt, e := ar.BeginCommandTx(ctx, tx, ar.Command{Module: "audit", ActorID: a.ID, MembershipKey: auth.MembershipKey, Scope: v.Scope, Action: event, ID: c.RequestID, Payload: c, Parent: v.Operation})
		if e != nil {
			return commandError(e)
		}
		if receipt.Replayed {
			var saved struct{ Generation, State string }
			if e = json.Unmarshal(receipt.Response, &saved); e != nil {
				return e
			}
			out = tasks.CommandResult{TaskID: ref.ID(), Generation: saved.Generation, CommandState: "applied", State: saved.State, Replayed: true}
			return nil
		}
		view := tasks.View{ID: ref.ID(), Generation: v.Generation, ControlRevision: v.ControlRevision, Originator: v.Owner, Actions: p.AllowedActions}
		if e = tasks.ValidateCommand(c, view, a); e != nil {
			return e
		}
		if c.Action == "retry" {
			if s.objects == nil {
				return Problem("audit_storage_unavailable", 503)
			}
			if e = s.exportLimitTx(ctx, tx, a.ID, v.Scope); e != nil {
				return e
			}
			if v.FieldProfile == "technical" && !auth.Has("technical.read") {
				return access.ErrDenied
			}
			generation, _ := strconv.ParseInt(v.Generation, 10, 64)
			generation++
			_, e = tx.Exec(ctx, `UPDATE audit_exports SET current_generation=$2,source_revision=source_revision+1,control_revision=control_revision+1,membership_key=$3 WHERE id=$1`, v.ID, generation, auth.MembershipKey)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `INSERT INTO audit_export_generations(export_id,generation,requested_by,operation_id,authorization_vector,projection_version,restore_epoch,state) VALUES($1,$2,$3,$4,$5,$6,$7,'queued')`, v.ID, generation, a.ID, receipt.OperationID, auth.Vector, auth.Projection, auth.Epoch)
			if e != nil {
				return e
			}
			v, e = loadExportTx(ctx, tx, v.ID, "", false)
			if e != nil {
				return e
			}
			if e = s.exportFactTx(ctx, tx, v, event, a.ID, "account", c.Reason); e != nil {
				return e
			}
			if e = s.indexExportTx(ctx, tx, v, a.ID, "retry"); e != nil {
				return e
			}
		} else {
			if _, e = tx.Exec(ctx, `UPDATE audit_exports SET control_revision=control_revision+1,source_revision=source_revision+1 WHERE id=$1`, v.ID); e != nil {
				return e
			}
			if e = s.exportFactTx(ctx, tx, v, event, a.ID, "account", c.Reason); e != nil {
				return e
			}
			if v.State == "queued" || v.State == "retry_wait" {
				if e = s.terminalExportTx(ctx, tx, v, "cancelled", "", ""); e != nil {
					return e
				}
			} else {
				_, e = tx.Exec(ctx, `UPDATE audit_export_generations SET state='cancelling',phase='waiting' WHERE export_id=$1 AND generation=$2`, v.ID, v.Generation)
				if e != nil {
					return e
				}
				v, e = loadExportTx(ctx, tx, v.ID, "", false)
				if e != nil {
					return e
				}
				if e = s.indexExportTx(ctx, tx, v, a.ID, "cancel"); e != nil {
					return e
				}
			}
			v, e = loadExportTx(ctx, tx, v.ID, "", false)
			if e != nil {
				return e
			}
		}
		out = tasks.CommandResult{TaskID: ref.ID(), Generation: v.Generation, State: v.State, CommandState: "applied"}
		if v.State == "cancelling" {
			out.CommandState = "requested"
		}
		return ar.CompleteCommandTx(ctx, tx, receipt, map[string]string{"generation": out.Generation, "state": out.State}, out.CommandState, v.ID)
	})
	return out, e
}
func (s *Service) NotificationTargetsTx(ctx context.Context, tx pgx.Tx, account string, scope Scope, ids []string) (map[string]ne.TargetContext, error) {
	out := map[string]ne.TargetContext{}
	if len(ids) > 100 {
		return nil, Problem("audit_filter_invalid", 400)
	}
	for _, id := range ids {
		v, auth, e := s.metadataExportTx(ctx, tx, access.Actor{ID: account}, id, "")
		if tasks.Denied(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if v.Owner != account || v.Scope != scope {
			continue
		}
		p, e := s.presentExport(v, access.Actor{ID: account}, auth)
		if e != nil {
			return nil, e
		}
		out[id] = ne.TargetContext{Allowed: true, Available: true, Name: "审计导出结果", State: p.State}
	}
	return out, nil
}
func (e *Error) StatusCode() int { return e.Status }

// Keep transport and source errors finite even when the durable owner was
// removed after a worker claim.
func isAuthorizationError(e error) bool {
	var p interface{ StatusCode() int }
	return errors.As(e, &p) && (p.StatusCode() == 401 || p.StatusCode() == 403 || p.StatusCode() == 404 || p.StatusCode() == 409 || p.StatusCode() == 410)
}
