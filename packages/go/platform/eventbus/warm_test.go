package eventbus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestPostgresWarmDrain(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	st, err := Operate(ctx, p, Operation{Action: "enable", Reason: "measure PG retry and downstream waiting"})
	must(t, err)
	add := func() string {
		id := uuid.NewString()
		must(t, pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error { return notification(ctx, tx, id) }))
		return id
	}
	finished := func(id string) bool {
		var completed bool
		must(t, p.QueryRow(ctx, `SELECT EXISTS(SELECT FROM event_work w JOIN event_outbox o USING(restore_epoch,event_id)
    WHERE w.consumer='notification' AND o.aggregate_id=$1 AND w.state='done')`, id).Scan(&completed))
		return completed
	}
	first := add()
	var attempts atomic.Int32
	config := Defaults()
	config.Mode, config.Workers, config.Batch, config.FallbackScanMS, config.BackoffMS = "pg", 1, 4, 5000, 100
	run, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	defer func() { stop(); <-done }()
	start := time.Now()
	go func() {
		done <- consumeEpoch(run, p, config, "notification", st.Epoch, func(_ context.Context, e Event) error {
			if e.AggregateID == first && attempts.Add(1) == 1 {
				return ErrDeferred
			}
			return nil
		}, diagnosticreport.Optional(nil))
	}()
	wait(t, 3*time.Second, func() bool { return finished(first) }, "deferred work waited for the idle scan")
	retryMS := time.Since(start).Milliseconds()
	require(t, attempts.Load() == 2, "unexpected callback count")
	start = time.Now()
	second := add()
	wait(t, 3*time.Second, func() bool { return finished(second) }, "next stage waited for the idle scan")
	evidence(t, "pg-warm-drain", map[string]any{"idle_scan_ms": config.FallbackScanMS, "configured_retry_ms": config.BackoffMS, "actual_retry_completion_ms": retryMS, "next_stage_completion_ms": time.Since(start).Milliseconds(), "first_key_attempts": attempts.Load()})
}
