package display

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
	"strconv"
)

func (s *Service) TaskResolveTx(ctx context.Context, tx pgx.Tx, a access.Actor, refs []taskrecord.SourceRef, mode string) ([]tasks.View, error) {
	ids := []string{}
	for _, r := range refs {
		if r.Module == "display" && r.Kind == "job" {
			ids = append(ids, r.RootID)
		}
	}
	rows, e := tx.Query(ctx, `SELECT snapshot FROM display_task_metadata WHERE id::text=ANY($1::text[])`, ids)
	if e != nil {
		return nil, e
	}
	facts := []taskrecord.Change{}
	for rows.Next() {
		var c taskrecord.Change
		if e = rows.Scan(&c); e != nil {
			rows.Close()
			return nil, e
		}
		facts = append(facts, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := []tasks.View{}
	for _, c := range facts {
		codes, label, status, e := tasks.ScopeTx(ctx, tx, a, c.Scope)
		if tasks.Denied(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if !tasks.Has(codes, string(access.ProjectDisplayUpload)) || mode == "mine" && c.Originator != a.ID {
			continue
		}
		v := tasks.Present(c, label)
		v.ResultReadable = tasks.Has(codes, string(access.ProjectDisplayView))
		if c.Originator == a.ID {
			v.ActorLabel = "我"
		} else {
			v.ActorLabel = "其他成员"
		}
		var instance, upload string
		var attempt int
		e = tx.QueryRow(ctx, `SELECT i.state,u.state,j.attempt FROM display_package_jobs j JOIN display_uploads u ON u.id=j.upload_id JOIN display_instances i ON i.id=u.instance_id WHERE j.id=$1`, refID(c)).Scan(&instance, &upload, &attempt)
		if e != nil {
			return nil, e
		}
		tasks.Capability(&v, "cancel", taskrecord.Active(c.State), c.Originator != a.ID, "task_not_active")
		tasks.Capability(&v, "retry", c.State == "failed" && attempt < 5 && upload == "uploaded" && instance != "archived" && status != "archived", true, "attempts_or_input_unavailable")
		if c.State == "cancelled" {
			v.CleanupState = "unknown"
		}
		v.NextStep = "configure_publish"
		if !v.ResultReadable {
			v.ResultState = "restricted"
		}
		if c.State == "succeeded" && v.ResultReadable {
			var ps string
			e = tx.QueryRow(ctx, `SELECT state FROM display_packages WHERE job_id=$1`, c.RootID).Scan(&ps)
			if e == pgx.ErrNoRows {
				v.ResultState = "missing"
			} else if e != nil {
				return nil, e
			} else if ps != "ready" {
				v.ResultState = "missing"
			}
		}
		out = append(out, v)
	}
	return out, nil
}
func refID(c taskrecord.Change) string { return c.RootID }
func (s *Service) TaskControl(ctx context.Context, a access.Actor, ref taskrecord.SourceRef, c tasks.Command) (tasks.CommandResult, error) {
	out := tasks.CommandResult{}
	if e := c.Validate(); e != nil {
		return out, e
	}
	var fact taskrecord.Change
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if a.Check == nil || a.ReadSubject() != nil {
			return access.ErrDenied
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
		return tx.QueryRow(ctx, `SELECT snapshot FROM display_task_metadata WHERE id=$1`, ref.RootID).Scan(&fact)
	})
	if e != nil {
		return out, e
	}
	action := "upload.cancel"
	if c.Action == "retry" {
		action = "package.retry"
	}
	reason := c.Reason
	if reason == "" {
		reason = "取消本人任务"
	}
	result, e := s.Command(ctx, a, Scope{fact.Scope.Tenant, fact.Scope.Project}, fact.ResultRef["instance_id"], action, fact.ResultRef["upload_id"], Command{Meta: Meta{RequestID: c.RequestID, Reason: reason}, taskCommand: &c})
	if e != nil {
		return out, e
	}
	gen := c.ExpectedGeneration
	state := "cancelled"
	if c.Action == "retry" {
		n, _ := taskrecord.Number(gen, 1)
		gen = strconv.FormatInt(n+1, 10)
		state = "queued"
	}
	replayed, _ := result["replayed"].(bool)
	return tasks.CommandResult{TaskID: ref.ID(), Generation: gen, CommandState: "applied", State: state, Replayed: replayed}, nil
}
func (s *Service) TaskTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, ref taskrecord.SourceRef, generation, kind string) (tasks.Target, error) {
	views, e := s.TaskResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
	if e != nil {
		return tasks.Target{}, e
	}
	if len(views) != 1 {
		return tasks.Target{}, tasks.Hidden()
	}
	v := views[0]
	out := tasks.Target{Scope: v.Scope, Generation: generation, Reason: "result_restricted"}
	if !v.ResultReadable {
		return out, nil
	}
	if generation != v.Generation {
		out.Reason = "historical_result_unavailable"
		return out, nil
	}
	var c taskrecord.Change
	if e = tx.QueryRow(ctx, `SELECT snapshot FROM display_task_metadata WHERE id=$1`, ref.RootID).Scan(&c); e != nil {
		return out, e
	}
	out.Available = true
	out.Kind = "display"
	out.ID = c.ResultRef["instance_id"]
	out.Reason = ""
	if kind == "result" && v.ResultState != "available" {
		out.Available = false
		out.Reason = "result_" + v.ResultState
	}
	return out, nil
}
func (s *Service) TaskOperationsTx(ctx context.Context, tx pgx.Tx, a access.Actor, f tasks.Filter) ([]tasks.Operation, error) {
	return tasks.ScopedOperationsTx(ctx, tx, a, f, "display", func(ctx context.Context, tx pgx.Tx, scope taskrecord.Scope) (string, string, string, error) {
		var q, r, failed string
		e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE j.state='queued')::text,count(*) FILTER(WHERE j.state='validating')::text,count(*) FILTER(WHERE j.state='failed')::text FROM display_package_jobs j JOIN display_uploads u ON u.id=j.upload_id WHERE u.tenant_id=$1 AND u.project_id=$2`, scope.Tenant, scope.Project).Scan(&q, &r, &failed)
		return q, r, failed, e
	})
}
