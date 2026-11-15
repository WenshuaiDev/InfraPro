package tasks

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

type Execution struct {
	Generation        string     `json:"generation"`
	State             string     `json:"state"`
	RequestedBy       string     `json:"requested_by,omitempty"`
	ExecutorKind      string     `json:"executor_kind"`
	ExecutorRef       string     `json:"executor_ref,omitempty"`
	StartedAt         *time.Time `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at"`
	ReasonCode        string     `json:"reason_code,omitempty"`
	HistoryIncomplete bool       `json:"history_incomplete"`
}
type Attempt struct {
	Unit       string     `json:"unit_key"`
	Fence      string     `json:"source_fence"`
	Number     int        `json:"attempt_no"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Outcome    string     `json:"outcome"`
	Reason     string     `json:"reason_code,omitempty"`
}
type Event struct {
	ID         string          `json:"id"`
	Generation string          `json:"generation"`
	OccurredAt time.Time       `json:"occurred_at"`
	Details    json.RawMessage `json:"details"`
}
type Detail struct {
	View
	SelectedExecution   Execution `json:"selected_execution"`
	Attempts            []Attempt `json:"attempts"`
	Relations           []View    `json:"relations"`
	RelationsRestricted bool      `json:"relations_restricted"`
	CurrentGeneration   string    `json:"current_generation"`
	HistoryState        string    `json:"history_state"`
	AttemptsTruncated   bool      `json:"attempts_truncated"`
}
type HistoryPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func (s *Service) getTx(ctx context.Context, tx pgx.Tx, a access.Actor, id string) (View, error) {
	if !access.ValidID(id) {
		return View{}, Hidden()
	}
	var c taskrecord.Change
	if e := tx.QueryRow(ctx, `SELECT snapshot FROM task_records WHERE id=$1`, id).Scan(&c); e != nil {
		if e == pgx.ErrNoRows {
			return View{}, Hidden()
		}
		return View{}, e
	}
	src := s.sources[c.Module]
	if src == nil {
		return View{}, unavailable()
	}
	v, e := src.ResolveTx(ctx, tx, a, []taskrecord.SourceRef{c.SourceRef}, "detail")
	if e != nil {
		return View{}, e
	}
	if len(v) == 0 {
		return s.retainedTx(ctx, tx, a, c, "detail")
	}
	return v[0], nil
}

const executionSelect = `SELECT generation::text,state,COALESCE(requested_by::text,''),executor_kind,executor_ref,started_at,finished_at,reason_code,history_incomplete FROM task_executions `

func readExecution(row pgx.Row) (Execution, error) {
	var v Execution
	e := row.Scan(&v.Generation, &v.State, &v.RequestedBy, &v.ExecutorKind, &v.ExecutorRef, &v.StartedAt, &v.FinishedAt, &v.ReasonCode, &v.HistoryIncomplete)
	return v, e
}
func (s *Service) Get(ctx context.Context, a access.Actor, id, generation string) (Detail, error) {
	out := Detail{Attempts: []Attempt{}, Relations: []View{}}
	if generation != "" {
		if _, e := taskrecord.Number(generation, 1); e != nil {
			return out, access.Validation("generation", "执行代次无效。")
		}
	}
	e := s.transaction(ctx, a, func(tx pgx.Tx, _ string) error {
		v, e := s.getTx(ctx, tx, a, id)
		if e != nil {
			return e
		}
		out.View = v
		out.CurrentGeneration = v.Generation
		if generation == "" {
			generation = v.Generation
		}
		out.SelectedExecution, e = readExecution(tx.QueryRow(ctx, executionSelect+` WHERE task_id=$1 AND generation=$2`, id, generation))
		if e == pgx.ErrNoRows && generation == v.Generation {
			out.HistoryState = "syncing"
			out.SelectedExecution = Execution{Generation: v.Generation, State: v.State, HistoryIncomplete: true}
		} else if e != nil {
			return pageError(e)
		} else {
			out.HistoryState = "complete"
		}
		if generation == v.Generation && out.SelectedExecution.State != v.State {
			out.HistoryState = "syncing"
		}
		if d, ok := taskrecord.Lookup(v.Type); ok && d.DisplayOnly {
			out.SelectedExecution.State = v.State
		}
		if generation != v.Generation {
			out.Generation = generation
			out.State = out.SelectedExecution.State
			out.ErrorCode = out.SelectedExecution.ReasonCode
			out.Actions = []Action{}
			out.Progress = taskrecord.Progress{}
			out.ResultState = "none"
			out.NextStep = ""
			out.HistoryIncomplete = out.SelectedExecution.HistoryIncomplete
			if out.State == "succeeded" {
				out.ResultState = "unknown"
			}
		}
		if source, ok := s.sources[v.Source.Module].(ExecutionResultSource); ok && !v.SourceMissing {
			result, err := source.ResultForExecutionTx(ctx, tx, a, v.Source, generation)
			if err != nil {
				return err
			}
			out.ResultState, out.ResultReadable, out.NextStep = result.ResultState, result.ResultReadable, result.NextStep
		}
		rows, e := tx.Query(ctx, `SELECT unit_key,source_fence::text,attempt_no,started_at,finished_at,outcome,reason_code FROM task_attempts WHERE task_id=$1 AND generation=$2 ORDER BY started_at DESC,source_fence DESC LIMIT 101`, id, generation)
		if e != nil {
			return e
		}
		for rows.Next() {
			var t Attempt
			if e = rows.Scan(&t.Unit, &t.Fence, &t.Number, &t.StartedAt, &t.FinishedAt, &t.Outcome, &t.Reason); e != nil {
				rows.Close()
				return e
			}
			out.Attempts = append(out.Attempts, t)
		}
		e = rows.Err()
		rows.Close()
		if len(out.Attempts) > 100 {
			out.AttemptsTruncated = true
			out.Attempts = out.Attempts[:100]
		}
		if e != nil {
			return e
		}
		rows, e = tx.Query(ctx, `SELECT child_task_id::text FROM task_relations WHERE parent_task_id=$1 AND (generation IS NULL OR generation=$2) ORDER BY child_task_id LIMIT 101`, id, generation)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var child string
			if e = rows.Scan(&child); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, child)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(ids) > 100 {
			out.RelationsRestricted = true
			ids = ids[:100]
		}
		for _, child := range ids {
			v, e := s.getTx(ctx, tx, a, child)
			if Denied(e) {
				out.RelationsRestricted = true
				continue
			}
			if e != nil {
				return e
			}
			out.Relations = append(out.Relations, v)
		}
		return nil
	})
	return out, e
}

// History cursors bind the current authorization/restore context as list cursors do.
func historyCursorTx(ctx context.Context, tx pgx.Tx, a access.Actor, rev, id, generation, token string) (cursor, string, error) {
	var epoch, secret string
	if e := tx.QueryRow(ctx, `SELECT restore_epoch::text,cursor_secret FROM task_protocol WHERE id=1`).Scan(&epoch, &secret); e != nil {
		return cursor{}, "", e
	}
	auth, e := authorizationVector(ctx, tx, a.ID, rev)
	if e != nil {
		return cursor{}, "", e
	}
	c := cursor{Account: a.ID, Filter: id + ":" + generation, Authorization: auth, Epoch: epoch, Upper: time.Now().UTC()}
	if token != "" {
		old, e := decode(token, secret)
		if e != nil {
			return c, "", e
		}
		if old.Account != c.Account || old.Filter != c.Filter || old.Authorization != auth || old.Epoch != epoch {
			return c, "", stale()
		}
		c = old
	}
	return c, secret, nil
}
func (s *Service) Executions(ctx context.Context, a access.Actor, id, token string) (HistoryPage[Execution], error) {
	out := HistoryPage[Execution]{Items: []Execution{}}
	e := s.transaction(ctx, a, func(tx pgx.Tx, rev string) error {
		if _, e := s.getTx(ctx, tx, a, id); e != nil {
			return e
		}
		c, secret, e := historyCursorTx(ctx, tx, a, rev, id, "executions", token)
		if e != nil {
			return e
		}
		var after any
		if c.ID != "" {
			n, e := taskrecord.Number(c.ID, 1)
			if e != nil {
				return stale()
			}
			after = n
		}
		rows, e := tx.Query(ctx, executionSelect+` WHERE task_id=$1 AND ($2::bigint IS NULL OR generation<$2) ORDER BY generation DESC LIMIT 51`, id, after)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := readExecution(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if len(out.Items) > 50 {
			c.ID = out.Items[49].Generation
			out.NextCursor = sign(c, secret)
			out.Items = out.Items[:50]
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) Events(ctx context.Context, a access.Actor, id, generation, token string) (HistoryPage[Event], error) {
	out := HistoryPage[Event]{Items: []Event{}}
	if _, e := taskrecord.Number(generation, 1); e != nil {
		return out, access.Validation("generation", "请选择执行代次。")
	}
	e := s.transaction(ctx, a, func(tx pgx.Tx, rev string) error {
		if _, e := s.getTx(ctx, tx, a, id); e != nil {
			return e
		}
		c, secret, e := historyCursorTx(ctx, tx, a, rev, id, generation, token)
		if e != nil {
			return e
		}
		var at any
		if !c.At.IsZero() {
			at = c.At
		}
		rows, e := tx.Query(ctx, `SELECT id::text,generation::text,occurred_at,safe_details FROM task_events WHERE task_id=$1 AND generation=$2 AND occurred_at<=$3 AND ($4::timestamptz IS NULL OR (occurred_at,id)<($4::timestamptz,$5::uuid)) ORDER BY occurred_at DESC,id DESC LIMIT 51`, id, generation, c.Upper, at, taskrecord.Null(c.ID))
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v Event
			if e = rows.Scan(&v.ID, &v.Generation, &v.OccurredAt, &v.Details); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if len(out.Items) > 50 {
			c.At, c.ID = out.Items[49].OccurredAt, out.Items[49].ID
			out.NextCursor = sign(c, secret)
			out.Items = out.Items[:50]
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) Resolve(ctx context.Context, a access.Actor, ref taskrecord.SourceRef) (taskrecord.Ref, error) {
	out := taskrecord.Ref{}
	if !access.ValidID(ref.RootID) {
		return out, Hidden()
	}
	source := s.sources[ref.Module]
	if source == nil {
		return out, Hidden()
	}
	e := s.transaction(ctx, a, func(tx pgx.Tx, _ string) error {
		views, e := source.ResolveTx(ctx, tx, a, []taskrecord.SourceRef{ref}, "detail")
		if e != nil {
			return e
		}
		if len(views) != 1 {
			return Hidden()
		}
		v := views[0]
		var exists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_records WHERE id=$1)`, v.ID).Scan(&exists); e != nil {
			return e
		}
		if !exists {
			return Problem("index_pending", "任务已建立，索引正在同步，请稍后再试。", 409)
		}
		out = taskrecord.Ref{SourceRef: v.Source, Generation: v.Generation, TaskID: v.ID}
		return nil
	})
	return out, e
}

func (s *Service) Target(ctx context.Context, a access.Actor, id, generation, kind string) (Target, error) {
	out := Target{}
	if !slices.Contains([]string{"business", "result", "recreate"}, kind) {
		return out, access.Validation("target_kind", "不支持此任务目标。")
	}
	if generation != "" {
		if _, e := taskrecord.Number(generation, 1); e != nil {
			return out, access.Validation("generation", "执行代次无效。")
		}
	}
	e := s.transaction(ctx, a, func(tx pgx.Tx, _ string) error {
		v, e := s.getTx(ctx, tx, a, id)
		if e != nil {
			return e
		}
		if generation == "" {
			generation = v.Generation
		}
		if v.SourceMissing || v.Type == "derived.backfill_group" {
			out = Target{Scope: v.Scope, Generation: generation, Reason: "source_missing_or_group"}
			return nil
		}
		out, e = s.sources[v.Source.Module].TargetTx(ctx, tx, a, v.Source, generation, kind)
		return e
	})
	return out, e
}

// ResolveReferenceTx is the notification adapter seam. It grants no access from
// the receipt itself and preserves the referenced execution after a retry.
func (s *Service) ResolveReferenceTx(ctx context.Context, tx pgx.Tx, a access.Actor, ref taskrecord.Ref) (taskrecord.Ref, error) {
	v, e := s.getTx(ctx, tx, a, ref.SourceRef.ID())
	if e != nil {
		return taskrecord.Ref{}, e
	}
	var found bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_executions WHERE task_id=$1 AND generation=$2)`, v.ID, ref.Generation).Scan(&found)
	if e != nil {
		return taskrecord.Ref{}, e
	}
	if !found {
		return taskrecord.Ref{}, Hidden()
	}
	ref.TaskID = v.ID
	return ref, nil
}

// A removed owner record cannot prove detailed data access. Retained history is
// disclosed only to current minimal-summary managers of that exact scope/type.
func (s *Service) retainedTx(ctx context.Context, tx pgx.Tx, a access.Actor, c taskrecord.Change, mode string) (View, error) {
	exists, e := s.sources[c.Module].ExistsTx(ctx, tx, c.SourceRef)
	if e != nil {
		return View{}, e
	}
	if exists {
		return View{}, Hidden()
	}
	codes, label, _, e := ScopeTx(ctx, tx, a, c.Scope)
	if e != nil {
		return View{}, e
	}
	d, ok := taskrecord.Lookup(c.Type)
	if !ok || !CanManage(codes, d, c.Scope.Kind) || mode == "mine" && c.Originator != a.ID {
		return View{}, Hidden()
	}
	v := Present(c, label)
	v.SourceMissing = true
	v.ResultState = "missing"
	v.ErrorCode = "source_missing"
	v.Actions = []Action{}
	return v, nil
}
