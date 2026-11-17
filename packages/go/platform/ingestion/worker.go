package ingestion

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"math/rand/v2"
	"sync"
	"time"
)

func (s *Service) PollStatus(ctx context.Context, cfg RelayConfig) error {
	var status map[string]any
	if e := relayCall(ctx, cfg, false, "/v1/status", nil, &status); e != nil {
		return e
	}
	if status["relay_id"] != cfg.ID {
		return k.Problem("relay_identity_mismatch", "云节点身份不一致。", 409)
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE ingestion_relays SET status=$2,observed_at=clock_timestamp() WHERE id=$1`, cfg.ID, k.JSON(status))
		return e
	})
}
func (s *Service) Ready(ctx context.Context) error {
	if !s.options.Enabled || s.objects == nil {
		return k.Problem("ingestion_disabled", "接入尚未配置。", 503)
	}
	if probe, ok := s.objects.(interface{ Probe(context.Context) error }); ok {
		e := probe.Probe(ctx)
		observeDependency(ctx, "s3", e)
		if e != nil {
			return e
		}
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var ready bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM goose_db_version WHERE version_id=45 AND is_applied)`).Scan(&ready); e != nil {
			return e
		}
		if !ready {
			return k.Problem("schema_incompatible", "接入 schema 未就绪。", 503)
		}
		return nil
	})
	observeDependency(ctx, "postgres", e)
	return e
}

// Each lane iteration records dependency failures independently from the durable
// business outcome: persisting a retry is successful bookkeeping, not a healthy
// parser invocation. No raw error text is written to process reports.
type observationKey struct{}
type workerObservation struct {
	sink   diagnosticreport.Sink
	failed bool
}

func observeDependency(ctx context.Context, id string, err error) {
	observation, _ := ctx.Value(observationKey{}).(*workerObservation)
	if observation == nil || observation.sink == nil {
		return
	}
	if err != nil {
		observation.failed = true
	}
	reason := ""
	if err != nil {
		reason = "dependency_unavailable"
	}
	observation.sink.Dependency(id, err == nil, reason)
}

type workerSlot struct{}
type shard struct{ slot, count int }

func (s *Service) RunWorker(ctx context.Context, reporter diagnosticreport.Sink) error {
	var wg sync.WaitGroup
	lane := func(name string, n int, delay time.Duration, work func(context.Context) (bool, error)) {
		group := diagnosticreport.NewGroup(reporter, name, n)
		for slot := 0; slot < n; slot++ {
			wg.Go(func() {
				failures := 0
				for ctx.Err() == nil {
					group.Begin(slot, time.Now().Add(40*time.Second))
					workCtx := ctx
					workCtx = context.WithValue(ctx, workerSlot{}, shard{slot, n})
					var observation *workerObservation
					if reporter != nil {
						observation = &workerObservation{sink: reporter}
						workCtx = context.WithValue(workCtx, observationKey{}, observation)
					}
					worked, e := work(workCtx)
					reason := ""
					wait := time.Duration(0)
					if e != nil {
						reason = "dependency_unavailable"
						failures = min(failures+1, 6)
						wait = time.Duration(1<<failures)*time.Second + time.Duration(rand.IntN(500))*time.Millisecond
					} else {
						failures = 0
						if !worked {
							wait = delay
						}
					}
					if observation != nil && observation.failed {
						reason = "dependency_unavailable"
					}
					group.End(slot, reason, time.Now().Add(wait))
					if wait > 0 {
						timer := time.NewTimer(wait)
						select {
						case <-ctx.Done():
							timer.Stop()
							return
						case <-timer.C:
						}
					}
				}
			})
		}
	}
	if len(s.options.Relays) > 0 {
		lane("pull", 4*len(s.options.Relays), 100*time.Millisecond, func(ctx context.Context) (bool, error) {
			v := ctx.Value(workerSlot{}).(shard)
			return s.Pull(ctx, s.options.Relays[v.slot%len(s.options.Relays)])
		})
		lane("relay-status", len(s.options.Relays), 10*time.Second, func(ctx context.Context) (bool, error) {
			v := ctx.Value(workerSlot{}).(shard)
			return false, s.PollStatus(ctx, s.options.Relays[v.slot])
		})
	}
	lane("object-maintenance", 1, time.Minute, func(ctx context.Context) (bool, error) {
		if e := s.Ready(ctx); e != nil {
			return false, e
		}
		return s.CleanupOne(ctx)
	})
	lane("confirmation", 2, 100*time.Millisecond, s.AckOne)
	lane("identify", 4, 100*time.Millisecond, s.IdentifyOne)
	lane("planning", 4, 100*time.Millisecond, s.PlanOne)
	lane("parse", 4, 100*time.Millisecond, s.ParseOne)
	lane("source-publication", 1, time.Second, s.PublishOne)
	lane("association-watermark", 1, time.Second, s.ChangeOne)
	lane("recovery", 1, 2*time.Second, func(ctx context.Context) (bool, error) { _, e := s.RecoveryOne(ctx); return false, e })
	<-ctx.Done()
	wg.Wait()
	return nil
}
