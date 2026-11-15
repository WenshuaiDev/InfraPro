package spatial

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
		if r.Module == "spatial" && r.Kind == "job" {
			ids = append(ids, r.RootID)
		}
	}
	rows, e := tx.Query(ctx, `SELECT snapshot FROM spatial_task_metadata WHERE id::text=ANY($1::text[])`, ids)
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
		codes, label, _, e := tasks.ScopeTx(ctx, tx, a, c.Scope)
		if tasks.Denied(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		manage := tasks.Has(codes, string(access.TenantSpatialManage))
		read := tasks.Has(codes, string(access.TenantSpatialRead))
		if !manage && !read || mode == "managed" && !manage || mode == "mine" && c.Originator != a.ID {
			continue
		}
		v := tasks.Present(c, label)
		v.ResultReadable = read
		if c.Originator == a.ID {
			v.ActorLabel = "我"
		} else if c.Originator == "" {
			v.ActorLabel = "历史发起人未知"
		} else {
			v.ActorLabel = "其他成员"
		}
		if !read {
			v.ResultState = "restricted"
		}
		var state, assetState, source, manifest string
		e = tx.QueryRow(ctx, `SELECT v.state,a.status,COALESCE(v.source_blob_id::text,''),v.manifest_hash FROM spatial_asset_versions v JOIN spatial_assets a ON a.id=v.asset_id WHERE v.id=$1 AND v.tenant_id=$2`, c.ResultRef["version_id"], c.Scope.Tenant).Scan(&state, &assetState, &source, &manifest)
		if e != nil {
			return nil, e
		}
		tasks.Capability(&v, "cancel", manage && taskrecord.Active(c.State), c.Originator != a.ID, "task_not_active")
		tasks.Capability(&v, "retry", manage && read && (c.State == "failed" || c.State == "cancelled") && state == "draft" && assetState == "normal" && manifest == "" && source == c.InputRef["source_blob_id"], true, "input_changed")
		if c.State == "cancelled" {
			v.CleanupState = "unknown"
		}
		if read {
			v.NextStep = "configure_publish"
			if state == "withdrawn" || assetState != "normal" {
				v.ResultState = "invalidated"
			}
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) TaskControl(ctx context.Context, a access.Actor, ref taskrecord.SourceRef, c tasks.Command) (tasks.CommandResult, error) {
	out := tasks.CommandResult{}
	if e := c.Validate(); e != nil {
		return out, e
	}
	var v tasks.View
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
		v = views[0]
		return nil
	})
	if e != nil {
		return out, e
	}
	reason := c.Reason
	if reason == "" {
		reason = "取消本人任务"
	}
	r, e := s.ControlJob(ctx, a, Tenant(v.Scope.Tenant), ref.RootID, "jobs."+c.Action, RevisionInput{Meta: Meta{RequestID: c.RequestID, Reason: reason}, Revision: c.ExpectedRevision}, c)
	if e != nil {
		return out, e
	}
	gen := c.ExpectedGeneration
	state := "cancelled"
	if c.Action == "retry" {
		g, _ := taskrecord.Number(gen, 1)
		gen = strconv.FormatInt(g+1, 10)
		state = "queued"
	}
	return tasks.CommandResult{TaskID: ref.ID(), Generation: gen, CommandState: "applied", State: state, Replayed: r.Replayed}, nil
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
	var version string
	if e = tx.QueryRow(ctx, `SELECT version_id::text FROM spatial_processing_jobs WHERE id=$1`, ref.RootID).Scan(&version); e != nil {
		return out, e
	}
	out.Available = true
	out.Kind = "spatial_version"
	out.ID = version
	out.Reason = ""
	if kind == "result" && v.ResultState != "available" {
		out.Available = false
		out.Reason = "result_" + v.ResultState
	}
	return out, nil
}
func (s *Service) TaskOperationsTx(ctx context.Context, tx pgx.Tx, a access.Actor, f tasks.Filter) ([]tasks.Operation, error) {
	return tasks.ScopedOperationsTx(ctx, tx, a, f, "spatial", func(ctx context.Context, tx pgx.Tx, scope taskrecord.Scope) (string, string, string, error) {
		var q, r, failed string
		e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='queued')::text,count(*) FILTER(WHERE state='running')::text,count(*) FILTER(WHERE state='failed')::text FROM spatial_processing_jobs WHERE tenant_id=$1`, scope.Tenant).Scan(&q, &r, &failed)
		return q, r, failed, e
	})
}
