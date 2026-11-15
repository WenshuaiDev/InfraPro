package taskrecord

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

// ApplyTx joins the owner's transaction. It never commits or runs a business job.
// A source revision has exactly one immutable fingerprint, including historical
// revisions arriving after a newer projection.
func ApplyTx(ctx context.Context, tx pgx.Tx, c Change) (Ref, error) {
	ref := Ref{SourceRef: c.SourceRef, Generation: c.Generation, TaskID: c.SourceRef.ID()}
	if e := c.Validate(); e != nil {
		return ref, e
	}
	var mode string
	if e := tx.QueryRow(ctx, `SELECT mode FROM task_protocol WHERE id=1 AND schema_version=1`).Scan(&mode); e != nil {
		return ref, e
	}
	if mode == "paused" {
		return ref, fmt.Errorf("task_protocol_paused")
	}
	fingerprint := Hash(c)
	_, e := tx.Exec(ctx, `INSERT INTO task_records(id,source_module,source_kind,source_root_id,task_type,scope_kind,tenant_id,project_id,originator_account_id,origin_kind,created_at,current_generation,source_revision,state,phase,progress,result_ref,source_observed_at,history_incomplete,snapshot,fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21) ON CONFLICT(source_module,source_kind,source_root_id) DO NOTHING`, ref.TaskID, c.Module, c.Kind, c.RootID, c.Type, c.Scope.Kind, Null(c.Scope.Tenant), Null(c.Scope.Project), Null(c.Originator), c.OriginKind, c.CreatedAt, c.Generation, c.Revision, c.State, c.Phase, JSON(c.Progress), JSON(c.ResultRef), c.ObservedAt, c.HistoryIncomplete, JSON(c), fingerprint)
	if e != nil {
		return ref, e
	}
	var old Change
	if e = tx.QueryRow(ctx, `SELECT snapshot FROM task_records WHERE id=$1 FOR UPDATE`, ref.TaskID).Scan(&old); e != nil {
		return ref, e
	}
	if old.SourceRef != c.SourceRef || old.Type != c.Type || old.Scope != c.Scope || old.Originator != c.Originator || old.OriginKind != c.OriginKind || !old.CreatedAt.Equal(c.CreatedAt) {
		return ref, ErrConflict
	}
	var existing string
	e = tx.QueryRow(ctx, `SELECT fingerprint FROM task_events WHERE task_id=$1 AND source_revision=$2 AND event_kind='snapshot'`, ref.TaskID, c.Revision).Scan(&existing)
	if e == nil {
		if existing != fingerprint {
			return ref, ErrConflict
		}
		return ref, nil
	}
	if e != pgx.ErrNoRows {
		return ref, e
	}
	rev, _ := Number(c.Revision, 0)
	oldRev, _ := Number(old.Revision, 0)
	gen, _ := Number(c.Generation, 1)
	oldGen, _ := Number(old.Generation, 1)
	if rev > oldRev && (gen < oldGen || gen > oldGen+1 && !c.HistoryIncomplete) {
		return ref, ErrConflict
	}
	if gen > oldGen && !Terminal(old.State) && !c.HistoryIncomplete {
		return ref, ErrConflict
	}
	// Terminal snapshots are immutable. A later observation can change result
	// availability without changing the terminal execution fact.
	var priorState, priorHash string
	var closed bool
	var executionRevision int64
	e = tx.QueryRow(ctx, `SELECT state,input_hash,finished_at IS NOT NULL,source_revision FROM task_executions WHERE task_id=$1 AND generation=$2`, ref.TaskID, c.Generation).Scan(&priorState, &priorHash, &closed, &executionRevision)
	if e != nil && e != pgx.ErrNoRows {
		return ref, e
	}
	if e == nil && priorHash != c.InputHash {
		return ref, ErrConflict
	}
	if closed && rev > executionRevision && priorState != c.State {
		return ref, ErrConflict
	}
	_, e = tx.Exec(ctx, `INSERT INTO task_executions(task_id,generation,requested_by,executor_kind,executor_ref,input_ref,input_hash,started_at,finished_at,state,reason_code,result_ref,history_incomplete,source_revision) VALUES($1,$2,$3,$4,$5,$6,$7,CASE WHEN $8 IN ('running','cancelling') THEN $9::timestamptz END,CASE WHEN $10 THEN $9::timestamptz END,$8,$11,$12,$13,$14) ON CONFLICT(task_id,generation) DO UPDATE SET started_at=COALESCE(task_executions.started_at,EXCLUDED.started_at),finished_at=EXCLUDED.finished_at,state=EXCLUDED.state,reason_code=EXCLUDED.reason_code,result_ref=EXCLUDED.result_ref,source_revision=EXCLUDED.source_revision WHERE task_executions.finished_at IS NULL AND task_executions.source_revision<EXCLUDED.source_revision`, ref.TaskID, c.Generation, Null(c.RequestedBy), c.ExecutorKind, c.ExecutorRef, JSON(c.InputRef), c.InputHash, c.State, c.ObservedAt, Terminal(c.State), c.ErrorCode, JSON(c.ResultRef), c.HistoryIncomplete, c.Revision)
	if e != nil {
		return ref, e
	}
	fence, _ := Number(c.Fence, 0)
	// Out-of-order history must not close a newer attempt. A late observed claim
	// can be inserted with the next already-recorded boundary, without reopening it.
	if rev >= executionRevision {
		_, e = tx.Exec(ctx, `UPDATE task_attempts SET finished_at=$3,outcome=CASE WHEN source_fence<>$4 THEN 'superseded' ELSE $5 END,reason_code=$6 WHERE task_id=$1 AND generation=$2 AND finished_at IS NULL AND started_at<=$3 AND (source_fence<>$4 OR $5 NOT IN ('running','cancelling','waiting'))`, ref.TaskID, c.Generation, c.ObservedAt, fence, c.State, c.ErrorCode)
		if e != nil {
			return ref, e
		}
	}
	if fence > 0 && c.Attempt > 0 && (c.State == "running" || c.State == "cancelling" || c.State == "waiting") && !c.HistoryIncomplete {
		var finished *time.Time
		outcome, reason := "running", ""
		if rev < executionRevision {
			e = tx.QueryRow(ctx, `SELECT occurred_at,CASE WHEN safe_details->>'fence'<>$4 THEN 'superseded' ELSE safe_details->>'state' END,COALESCE(safe_details->>'reason_code','') FROM task_events WHERE task_id=$1 AND generation=$2 AND source_revision>$3 AND (safe_details->>'fence'<>$4 OR safe_details->>'state' NOT IN ('running','cancelling','waiting')) ORDER BY source_revision LIMIT 1`, ref.TaskID, c.Generation, rev, c.Fence).Scan(&finished, &outcome, &reason)
			if e != nil && e != pgx.ErrNoRows {
				return ref, e
			}
		}
		_, e = tx.Exec(ctx, `INSERT INTO task_attempts(task_id,generation,unit_key,source_fence,attempt_no,started_at,finished_at,outcome,reason_code,diagnostic_ref) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`, ref.TaskID, c.Generation, c.InputRef["job_id"], fence, c.Attempt, c.ObservedAt, finished, outcome, reason, JSON(map[string]string{"job_id": c.InputRef["job_id"], "generation": c.Generation}))
		if e != nil {
			return ref, e
		}
	}
	eventActor := c.EventActor
	if eventActor == "" {
		eventActor = c.RequestedBy
	}
	details := map[string]string{"state": c.State, "phase": c.Phase, "reason_code": c.ErrorCode, "action": c.Event, "reason": c.Reason, "fence": c.Fence, "attempt": strconv.Itoa(c.Attempt)}
	_, e = tx.Exec(ctx, `INSERT INTO task_events(task_id,generation,source_revision,event_kind,actor_id,occurred_at,safe_details,fingerprint) VALUES($1,$2,$3,'snapshot',$4,$5,$6,$7)`, ref.TaskID, c.Generation, c.Revision, Null(eventActor), c.ObservedAt, JSON(details), fingerprint)
	if e != nil {
		return ref, e
	}
	if rev > oldRev {
		_, e = tx.Exec(ctx, `UPDATE task_records SET current_generation=$2,source_revision=$3,revision=revision+1,state=$4,phase=$5,progress=$6,result_ref=$7,source_observed_at=$8,projected_at=clock_timestamp(),history_incomplete=$9,snapshot=$10,fingerprint=$11 WHERE id=$1`, ref.TaskID, c.Generation, c.Revision, c.State, c.Phase, JSON(c.Progress), JSON(c.ResultRef), c.ObservedAt, c.HistoryIncomplete, JSON(c), fingerprint)
	}
	return ref, e
}

// FlushTx is called by the shared transaction boundary. Source-owned triggers
// collect only changes made in this transaction, including direct publications;
// a failure here rolls back the original state change as well as its metadata.
// Display-only transactions do not set this flag and need no public-table grants.
func FlushTx(ctx context.Context, tx pgx.Tx) error {
	var pending string
	if e := tx.QueryRow(ctx, `SELECT COALESCE(current_setting('infrapro.task_changes',true),'')`).Scan(&pending); e != nil {
		return e
	}
	if pending != "1" {
		return nil
	}
	rows, e := tx.Query(ctx, `SELECT id,change FROM task_transaction_facts WHERE transaction_id=pg_current_xact_id() ORDER BY id`)
	if e != nil {
		return e
	}
	type fact struct {
		id int64
		c  Change
	}
	facts := []fact{}
	for rows.Next() {
		var v fact
		if e = rows.Scan(&v.id, &v.c); e != nil {
			rows.Close()
			return e
		}
		facts = append(facts, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range facts {
		if e = auditTransitionTx(ctx, tx, v.c); e != nil {
			return e
		}
		if _, e = ApplyTx(ctx, tx, v.c); e != nil {
			return fmt.Errorf("apply task fact %d: %w", v.id, e)
		}
	}
	_, e = tx.Exec(ctx, `DELETE FROM task_transaction_facts WHERE transaction_id=pg_current_xact_id()`)
	return e
}
func LinkTx(ctx context.Context, tx pgx.Tx, parent, child SourceRef, kind string, generation string) error {
	var gen any
	if generation != "" {
		n, e := strconv.ParseInt(generation, 10, 64)
		if e != nil || n < 1 {
			return ErrConflict
		}
		gen = n
	}
	_, e := tx.Exec(ctx, `INSERT INTO task_relations(parent_task_id,child_task_id,relation_kind,generation,source_ref) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, parent.ID(), child.ID(), kind, gen, JSON(child))
	return e
}
func Decode(raw []byte) (Change, error) {
	var c Change
	e := json.Unmarshal(raw, &c)
	if e == nil {
		e = c.Validate()
	}
	return c, e
}

// DrainTx recovers committed owner journals written by migrations or recovery
// tools outside the usual transaction boundary. Uncommitted rows stay invisible;
// no maximum-ID watermark can skip a late commit.
func DrainTx(ctx context.Context, tx pgx.Tx, limit int) (int, error) {
	rows, e := tx.Query(ctx, `SELECT id,change FROM task_transaction_facts ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if e != nil {
		return 0, e
	}
	type item struct {
		id     int64
		change Change
	}
	items := []item{}
	for rows.Next() {
		var v item
		if e = rows.Scan(&v.id, &v.change); e != nil {
			rows.Close()
			return 0, e
		}
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	for _, v := range items {
		if _, e = ApplyTx(ctx, tx, v.change); e != nil {
			return 0, e
		}
		if _, e = tx.Exec(ctx, `DELETE FROM task_transaction_facts WHERE id=$1`, v.id); e != nil {
			return 0, e
		}
	}
	return len(items), nil
}

// ReconcileRelationsTx consumes owner-proven identity edges. Only neutral
// metadata is read; missing or unauthorized relationships never grant access.
func ReconcileRelationsTx(ctx context.Context, tx pgx.Tx, ref SourceRef) error {
	rows, e := tx.Query(ctx, `SELECT rel.relation_kind,m.snapshot FROM task_source_relations rel JOIN monitoring_task_metadata m ON m.snapshot->>'source_module'=rel.child_module AND m.snapshot->>'source_kind'=rel.child_kind AND m.snapshot->>'source_root_id'=rel.child_root_id::text WHERE rel.source_module=$1 AND rel.source_kind=$2 AND rel.source_root_id=$3`, ref.Module, ref.Kind, ref.RootID)
	if e != nil {
		return e
	}
	type edge struct {
		kind   string
		change Change
	}
	edges := []edge{}
	for rows.Next() {
		var v edge
		if e = rows.Scan(&v.kind, &v.change); e != nil {
			rows.Close()
			return e
		}
		edges = append(edges, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range edges {
		if _, e = ApplyTx(ctx, tx, v.change); e != nil {
			return e
		}
		if e = LinkTx(ctx, tx, ref, v.change.SourceRef, v.kind, ""); e != nil {
			return e
		}
	}
	return nil
}
