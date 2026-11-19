package tasks

import (
	"context"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"time"
)

// IndexSource exposes only metadata. It cannot claim or mutate business jobs.
type IndexSource interface {
	BackfillTx(context.Context, pgx.Tx, string, int) ([]taskrecord.Change, string, bool, error)
}
type Handoff struct {
	ID     string
	Change taskrecord.Change
}
type HandoffSource interface {
	PendingTx(context.Context, pgx.Tx, int, int) ([]Handoff, error)
	CompleteTx(context.Context, pgx.Tx, string) error
	FailTx(context.Context, pgx.Tx, string) error
	ReplayTx(context.Context, pgx.Tx, string) error
	DiagnosticsTx(context.Context, pgx.Tx) (map[string]any, error)
}
type Indexer struct {
	db      *access.Service
	sources map[string]IndexSource
	handoff HandoffSource
	options Options
}

func NewIndexer(a *access.Service, sources map[string]IndexSource, h HandoffSource) *Indexer {
	return &Indexer{a, sources, h, DefaultOptions()}
}
func (i *Indexer) WithOptions(o Options) *Indexer { i.options = o; return i }
func (i *Indexer) Tick(ctx context.Context, worker string, reporters ...diagnosticreport.Sink) (int, error) {
	return i.tick(ctx, worker, true, i.options.Poll, reporters...)
}

func (i *Indexer) tick(ctx context.Context, worker string, scan bool, scanInterval time.Duration, reporters ...diagnosticreport.Sink) (int, error) {
	reporter := diagnosticreport.Optional(reporters)
	reporter.Begin("reconcile", diagnosticreport.Deadline(ctx))
	count := 0
	e := i.db.Transact(ctx, func(tx pgx.Tx) error {
		var epoch, mode string
		if e := tx.QueryRow(ctx, `SELECT restore_epoch::text,mode FROM task_protocol WHERE id=1 AND schema_version=1 FOR SHARE`).Scan(&epoch, &mode); e != nil {
			return e
		}
		if mode == "paused" {
			return ErrProjectionPaused
		}
		if _, e := tx.Exec(ctx, `INSERT INTO task_worker_heartbeats(worker_id,heartbeat_at,protocol_version,restore_epoch) VALUES($1,clock_timestamp(),1,$2) ON CONFLICT(worker_id) DO UPDATE SET heartbeat_at=EXCLUDED.heartbeat_at,restore_epoch=EXCLUDED.restore_epoch,safe_error_code=''`, worker, epoch); e != nil {
			return e
		}
		recovered, e := taskrecord.DrainTx(ctx, tx, i.options.HandoffBatch)
		if e != nil {
			return e
		}
		count += recovered
		if i.handoff != nil {
			items, e := i.handoff.PendingTx(ctx, tx, i.options.HandoffBatch, i.options.RetryLimit)
			if e != nil {
				return e
			}
			for _, h := range items {
				save, e := tx.Begin(ctx)
				if e != nil {
					return e
				}
				_, e = taskrecord.ApplyTx(ctx, save, h.Change)
				if e == nil {
					e = i.handoff.CompleteTx(ctx, save, h.ID)
				}
				if e != nil {
					if e = save.Rollback(ctx); e != nil {
						return e
					}
					if e = i.handoff.FailTx(ctx, tx, h.ID); e != nil {
						return e
					}
					continue
				}
				if e = save.Commit(ctx); e != nil {
					return e
				}
				count++
			}
		}
		return nil
	})
	reporter.End("reconcile", diagnosticreport.Result(e), time.Now().Add(i.options.Poll))
	reporter.Dependency("postgres", e == nil, diagnosticreport.Result(e))
	if e != nil || !scan {
		return count, e
	}
	reporter.Begin("backfill", diagnosticreport.Deadline(ctx))
	// One bounded page per source. Independent transactions isolate source faults.
	var failures []error
	for _, key := range []string{"monitoring:job", "derived:run", "derived:export", "spatial:job", "display:job", "derived:group", "files:job"} {
		if i.sources[key] == nil {
			continue
		}
		n, e := i.backfill(ctx, key, worker, false)
		if e != nil {
			failures = append(failures, e)
			_ = i.db.Transact(ctx, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `UPDATE task_source_states SET last_error_code='source_reconciliation_failed' WHERE source_module||':'||source_kind=$1`, key)
				return err
			})
			continue
		}
		count += n
	}
	if len(failures) > 0 {
		reporter.End("backfill", "loop_failed", time.Now().Add(scanInterval))
		return count, errors.Join(failures...)
	}
	e = i.markReady(ctx)
	reporter.End("backfill", diagnosticreport.Result(e), time.Now().Add(scanInterval))
	return count, e
}

var ErrProjectionPaused = errors.New("task projection paused")

// ProjectEvent repairs relation edges of the task referenced by a durable
// snapshot. The projection itself was committed atomically by the producer.
func (i *Indexer) ProjectEvent(ctx context.Context, eventID string, revision int64, scope string) error {
	if !access.ValidID(eventID) || revision < 0 {
		return pgx.ErrNoRows
	}
	return i.db.Transact(ctx, func(tx pgx.Tx) error {
		var active bool
		if e := tx.QueryRow(ctx, `SELECT mode<>'paused' FROM task_protocol WHERE id=1 AND schema_version=1 FOR SHARE`).Scan(&active); e != nil {
			return e
		}
		if !active {
			return ErrProjectionPaused
		}
		var ref taskrecord.SourceRef
		e := tx.QueryRow(ctx, `SELECT t.source_module,t.source_kind,t.source_root_id::text
			FROM task_events e JOIN task_records t ON t.id=e.task_id
			WHERE e.id=$1 AND e.source_revision=$2 AND e.event_kind='snapshot' AND COALESCE(t.tenant_id::text,'platform')=$3`, eventID, revision, scope).Scan(&ref.Module, &ref.Kind, &ref.RootID)
		if e != nil {
			return e
		}
		return taskrecord.ReconcileRelationsTx(ctx, tx, ref)
	})
}
func (i *Indexer) backfill(ctx context.Context, key, worker string, force bool) (int, error) {
	source := i.sources[key]
	if source == nil {
		return 0, fmt.Errorf("unsupported task source")
	}
	n := 0
	e := i.db.Transact(ctx, func(tx pgx.Tx) error {
		var module, kind, after string
		var complete bool
		var last *time.Time
		var epoch string
		e := tx.QueryRow(ctx, `SELECT source_module,source_kind,COALESCE(backfill_cursor->>'after',''),backfill_complete,last_reconciled_at,restore_epoch::text FROM task_source_states WHERE source_module||':'||source_kind=$1 FOR UPDATE SKIP LOCKED`, key).Scan(&module, &kind, &after, &complete, &last, &epoch)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		var current string
		if e = tx.QueryRow(ctx, `SELECT restore_epoch::text FROM task_protocol WHERE id=1 FOR SHARE`).Scan(&current); e != nil {
			return e
		}
		if epoch != current {
			after = ""
			complete = false
		}
		if complete && !force && last != nil && time.Since(*last) < i.options.ReconcileInterval {
			return nil
		}
		if complete {
			after = ""
		}
		changes, next, done, e := source.BackfillTx(ctx, tx, after, i.options.ReconcileBatch)
		if e != nil {
			return e
		}
		for _, c := range changes {
			if _, e = taskrecord.ApplyTx(ctx, tx, c); e != nil {
				return e
			}
			if e = taskrecord.ReconcileRelationsTx(ctx, tx, c.SourceRef); e != nil {
				return e
			}
			n++
		}
		// Completion of an ordered pass is followed by another bounded full pass.
		// Registration handles concurrent newcomers; Verify performs a missing-key
		// anti-join before the protocol can claim complete coverage.
		_, e = tx.Exec(ctx, `UPDATE task_source_states SET backfill_cursor=$2,backfill_complete=$3,last_reconciled_at=CASE WHEN $3 THEN clock_timestamp() ELSE last_reconciled_at END,lease_owner=$4,lease_until=NULL,fence=fence+1,restore_epoch=$5,last_error_code='' WHERE source_module||':'||source_kind=$1`, key, taskrecord.JSON(map[string]string{"after": next}), done, worker, current)
		return e
	})
	return n, e
}
func (i *Indexer) Backfill(ctx context.Context, source string) error {
	if source == "all" {
		for key := range i.sources {
			if e := i.Backfill(ctx, key); e != nil {
				return e
			}
		}
		return i.markReady(ctx)
	}
	if i.sources[source] == nil {
		return fmt.Errorf("source must be a registered module:kind")
	}
	if e := i.db.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE task_source_states SET backfill_complete=false,backfill_cursor='{}' WHERE source_module||':'||source_kind=$1`, source)
		return e
	}); e != nil {
		return e
	}
	for {
		if _, e := i.backfill(ctx, source, "cli", true); e != nil {
			return e
		}
		var done bool
		if e := i.db.Transact(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT backfill_complete FROM task_source_states WHERE source_module||':'||source_kind=$1`, source).Scan(&done)
		}); e != nil {
			return e
		}
		if done {
			return i.markReady(ctx)
		}
	}
}
func (i *Indexer) markReady(ctx context.Context) error {
	return i.db.Transact(ctx, func(tx pgx.Tx) error {
		var missing bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_source_states WHERE NOT backfill_complete OR last_error_code<>'' OR restore_epoch<>(SELECT restore_epoch FROM task_protocol WHERE id=1)) OR EXISTS(SELECT 1 FROM display_task_handoffs WHERE translated_at IS NULL) OR EXISTS(SELECT 1 FROM task_transaction_facts)`).Scan(&missing); e != nil {
			return e
		}
		if missing {
			return nil
		}
		// Each Source checks retained source keys through its whitelisted view. A late
		// lower UUID cannot be missed merely because the last ordered cursor passed it.
		for _, source := range i.sources {
			if checker, ok := source.(interface {
				MissingTx(context.Context, pgx.Tx) (bool, error)
			}); ok {
				missing, e := checker.MissingTx(ctx, tx)
				if e != nil {
					return e
				}
				if missing {
					return nil
				}
			}
		}
		_, e := tx.Exec(ctx, `UPDATE task_protocol SET mode='ready',updated_at=clock_timestamp() WHERE id=1 AND mode IN ('registering','backfilling')`)
		return e
	})
}
func (i *Indexer) Diagnostics(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	e := i.db.Transact(ctx, func(tx pgx.Tx) error {
		var mode, epoch string
		var total string
		if e := tx.QueryRow(ctx, `SELECT mode,restore_epoch::text,(SELECT count(*)::text FROM task_records) FROM task_protocol WHERE id=1`).Scan(&mode, &epoch, &total); e != nil {
			return e
		}
		out["mode"], out["restore_epoch"], out["records"] = mode, epoch, total
		rows, e := tx.Query(ctx, `SELECT source_module,source_kind,backfill_complete,last_reconciled_at,last_error_code FROM task_source_states ORDER BY 1,2`)
		if e != nil {
			return e
		}
		states := []map[string]any{}
		for rows.Next() {
			var m, k, code string
			var done bool
			var at *time.Time
			if e = rows.Scan(&m, &k, &done, &at, &code); e != nil {
				rows.Close()
				return e
			}
			states = append(states, map[string]any{"source": m + ":" + k, "complete": done, "observed_at": at, "error_code": code})
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		out["sources"] = states
		var heartbeat *time.Time
		if e = tx.QueryRow(ctx, `SELECT max(heartbeat_at) FROM task_worker_heartbeats WHERE restore_epoch=(SELECT restore_epoch FROM task_protocol WHERE id=1)`).Scan(&heartbeat); e != nil {
			return e
		}
		out["heartbeat_at"] = heartbeat
		if i.handoff != nil {
			out["handoffs"], e = i.handoff.DiagnosticsTx(ctx, tx)
		}
		return e
	})
	return out, e
}
func (i *Indexer) Replay(ctx context.Context, id, reason string) error {
	if !access.ValidID(id) {
		return access.Validation("id", "交接编号无效。")
	}
	if e := access.ValidReason(reason); e != nil {
		return e
	}
	return i.db.Transact(ctx, func(tx pgx.Tx) error {
		if e := i.handoff.ReplayTx(ctx, tx, id); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO task_index_repairs(handoff_id,reason) VALUES($1,$2)`, id, reason)
		return e
	})
}
func (i *Indexer) Verify(ctx context.Context) error {
	return i.db.Transact(ctx, func(tx pgx.Tx) error {
		for _, src := range i.sources {
			checker, ok := src.(interface {
				MissingTx(context.Context, pgx.Tx) (bool, error)
			})
			if !ok {
				return fmt.Errorf("missing source verifier")
			}
			missing, e := checker.MissingTx(ctx, tx)
			if e != nil {
				return e
			}
			if missing {
				return fmt.Errorf("task source reconciliation required")
			}
		}
		var ok bool
		e := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM task_records r LEFT JOIN task_executions e ON e.task_id=r.id AND e.generation=r.current_generation WHERE e.task_id IS NULL OR e.state<>r.state) AND NOT EXISTS(SELECT 1 FROM display_task_handoffs WHERE translated_at IS NULL) AND NOT EXISTS(SELECT 1 FROM task_transaction_facts)`).Scan(&ok)
		if e != nil {
			return e
		}
		if !ok {
			return fmt.Errorf("task history or handoff mismatch")
		}
		return nil
	})
}
func (i *Indexer) RestoreEpoch(ctx context.Context, reason string) error {
	if e := access.ValidReason(reason); e != nil {
		return e
	}
	return i.db.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE task_protocol SET restore_epoch=gen_random_uuid(),mode='backfilling',updated_at=clock_timestamp() WHERE id=1`)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE task_source_states SET lease_owner=NULL,lease_until=NULL,fence=fence+1,backfill_complete=false,backfill_cursor='{}'`)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO task_index_repairs(repair_kind,reason) VALUES('restore_epoch',$1)`, reason)
		return e
	})
}
func (i *Indexer) Run(ctx context.Context, worker string, onError func(error), reporters ...diagnosticreport.Sink) {
	i.RunWithFallback(ctx, 0, worker, onError, reporters...)
}
func (i *Indexer) RunWithFallback(ctx context.Context, interval time.Duration, worker string, onError func(error), reporters ...diagnosticreport.Sink) {
	var nextScan time.Time
	for ctx.Err() == nil {
		tickCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		finished := make(chan struct{})
		go func() {
			select {
			case <-finished:
				return
			case <-ctx.Done():
			}
			timer := time.NewTimer(i.options.ShutdownGrace)
			defer timer.Stop()
			select {
			case <-finished:
			case <-timer.C:
				cancel()
			}
		}()
		scan := !time.Now().Before(nextScan)
		_, e := i.tick(tickCtx, worker, scan, max(interval, i.options.Poll), reporters...)
		if scan {
			nextScan = time.Now().Add(interval)
		}
		close(finished)
		cancel()
		if e != nil && ctx.Err() == nil {
			onError(e)
		}
		timer := time.NewTimer(i.options.Poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
