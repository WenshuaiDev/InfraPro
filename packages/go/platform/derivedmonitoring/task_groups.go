package derivedmonitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) registerTaskGroupTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, in WriteMeta, runs []Run) (*taskrecord.Ref, error) {
	if e := taskrecord.FlushTx(ctx, tx); e != nil {
		return nil, e
	}
	// Relations only come from durable dependency edges produced by the planner.
	for _, run := range runs {
		rows, e := tx.Query(ctx, `SELECT dependency_run_id::text FROM derived_run_dependencies WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, run.ID)
		if e != nil {
			return nil, e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return nil, e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		for _, id := range ids {
			if e = taskrecord.LinkTx(ctx, tx, taskrecord.SourceRef{Module: "derived", Kind: "run", RootID: run.ID}, taskrecord.SourceRef{Module: "derived", Kind: "run", RootID: id}, "dependency", ""); e != nil {
				return nil, e
			}
		}
	}
	if len(runs) < 2 {
		return nil, nil
	}
	root := uuid.NewString()
	at := time.Now().UTC().Truncate(time.Microsecond)
	c := taskrecord.Change{SourceRef: taskrecord.SourceRef{Module: "derived", Kind: "group", RootID: root}, Type: "derived.backfill_group", Scope: taskrecord.Scope{Kind: scope.Entry, Tenant: scope.Tenant, Project: scope.Project}, Originator: a.ID, OriginKind: "person", RequestedBy: a.ID, ExecutorKind: "none", Generation: "1", Revision: "1", ControlRevision: "1", State: "queued", SourceState: "display_only", Phase: "queued", InputRef: map[string]string{"request_id": in.RequestID}, InputHash: taskrecord.Hash(runs), ResultRef: map[string]string{}, CreatedAt: at, ObservedAt: at, Fence: "0", Event: "created"}
	if _, e := tx.Exec(ctx, `INSERT INTO derived_task_groups(id,tenant_id,project_id,actor_id,request_id,snapshot) VALUES($1,$2,$3,$4,$5,$6)`, root, scope.Tenant, nullable(scope.Project), a.ID, in.RequestID, taskrecord.JSON(c)); e != nil {
		return nil, e
	}
	ref, e := taskrecord.ApplyTx(ctx, tx, c)
	if e != nil {
		return nil, e
	}
	for _, run := range runs {
		if _, e = tx.Exec(ctx, `INSERT INTO derived_task_group_members(group_id,run_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, root, run.ID); e != nil {
			return nil, e
		}
		if e = taskrecord.LinkTx(ctx, tx, c.SourceRef, taskrecord.SourceRef{Module: "derived", Kind: "run", RootID: run.ID}, "member", ""); e != nil {
			return nil, e
		}
	}
	return &ref, nil
}
func (s *Service) TaskResolveTx(ctx context.Context, tx pgx.Tx, a access.Actor, refs []taskrecord.SourceRef, mode string) ([]tasks.View, error) {
	ordinary := []taskrecord.SourceRef{}
	groups := []string{}
	for _, r := range refs {
		if r.Kind == "group" {
			groups = append(groups, r.RootID)
		} else {
			ordinary = append(ordinary, r)
		}
	}
	out, e := s.monitoring.TaskResolveTx(ctx, tx, a, ordinary, mode)
	if e != nil {
		return nil, e
	}
	for _, id := range groups {
		var c taskrecord.Change
		e = tx.QueryRow(ctx, `SELECT snapshot FROM derived_task_groups WHERE id=$1`, id).Scan(&c)
		if e == pgx.ErrNoRows {
			continue
		}
		if e != nil {
			return nil, e
		}
		codes, label, _, e := tasks.ScopeTx(ctx, tx, a, c.Scope)
		if tasks.Denied(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		d, _ := taskrecord.Lookup(c.Type)
		if !tasks.CanUse(codes, d, c.Scope.Kind, mode) || mode == "mine" && c.Originator != a.ID {
			continue
		}
		rows, e := tx.Query(ctx, `SELECT r.source_module,r.source_kind,r.source_root_id::text FROM task_relations rel JOIN task_records r ON r.id=rel.child_task_id WHERE rel.parent_task_id=$1 AND rel.relation_kind='member' ORDER BY r.id LIMIT 501`, c.SourceRef.ID())
		if e != nil {
			return nil, e
		}
		children := []taskrecord.SourceRef{}
		for rows.Next() {
			var r taskrecord.SourceRef
			if e = rows.Scan(&r.Module, &r.Kind, &r.RootID); e != nil {
				rows.Close()
				return nil, e
			}
			children = append(children, r)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		resolved, e := s.monitoring.TaskResolveTx(ctx, tx, a, children, "detail")
		if e != nil {
			return nil, e
		}
		v := tasks.Present(c, label)
		v.Actions = []tasks.Action{}
		v.ResultState = "none"
		v.NextStep = "view_members"
		v.State = "succeeded"
		v.Phase = "completed"
		if len(resolved) == 0 {
			continue
		}
		for _, child := range resolved {
			if child.ObservedAt.After(v.ObservedAt) {
				v.ObservedAt = child.ObservedAt
			}
			if child.State == "failed" || child.State == "needs_action" {
				v.State = "needs_action"
				v.Phase = "failed"
			} else if taskrecord.Active(child.State) && v.State != "needs_action" {
				v.State = "running"
				v.Phase = "computing"
			} else if child.State == "cancelled" && v.State == "succeeded" {
				v.State = "cancelled"
				v.Phase = "cancelled"
			}
		}
		if len(children) > 500 || len(resolved) != len(children) {
			v.State = "unknown"
			v.Phase = "unknown"
			v.ErrorCode = "partial_execution_restricted"
		}
		out = append(out, v)
	}
	return out, nil
}

type TaskGroupIndex struct{}

func (TaskGroupIndex) BackfillTx(ctx context.Context, tx pgx.Tx, after string, limit int) ([]taskrecord.Change, string, bool, error) {
	rows, e := tx.Query(ctx, `SELECT id::text,snapshot FROM derived_task_groups WHERE ($1::uuid IS NULL OR id>$1) ORDER BY id LIMIT $2`, taskrecord.Null(after), limit)
	if e != nil {
		return nil, "", false, e
	}
	defer rows.Close()
	out := []taskrecord.Change{}
	for rows.Next() {
		var c taskrecord.Change
		if e = rows.Scan(&after, &c); e != nil {
			return nil, "", false, e
		}
		out = append(out, c)
	}
	return out, after, len(out) < limit, rows.Err()
}
func (TaskGroupIndex) MissingTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	var missing bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_task_groups g LEFT JOIN task_records r ON r.source_module='derived' AND r.source_kind='group' AND r.source_root_id=g.id WHERE r.id IS NULL)`).Scan(&missing)
	return missing, e
}
