package audit

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"sync"
	"time"
)

func (s *Service) WithHandoffBridge(fn func(context.Context, pgx.Tx, int) (int, error)) *Service {
	s.handoff = fn
	return s
}
func (s *Service) WithHandoffReplayer(fn func(context.Context, pgx.Tx, string, int) (bool, error)) *Service {
	s.handoffReplay = fn
	return s
}
func (s *Service) Reconcile(ctx context.Context) (int, error) {
	count := 0
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var mode string
		var epoch string
		if e := tx.QueryRow(ctx, `SELECT mode,restore_epoch::text FROM audit_protocol WHERE id=1 FOR SHARE`).Scan(&mode, &epoch); e != nil {
			return e
		}
		if mode == "reconciling" || mode == "migrating" {
			return nil
		}
		if s.handoff != nil {
			var e error
			count, e = s.handoff(ctx, tx, 100)
			if e != nil {
				return e
			}
		}
		if _, e := tx.Exec(ctx, `UPDATE audit_export_objects SET state='expired' WHERE state='available' AND expires_at<=clock_timestamp()`); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO audit_worker_states(id,heartbeat_at,epoch) VALUES($1,clock_timestamp(),$2) ON CONFLICT(id) DO UPDATE SET heartbeat_at=EXCLUDED.heartbeat_at,epoch=EXCLUDED.epoch,error_code=''`, s.options.NodeID, epoch)
		return e
	})
	if e != nil {
		return count, e
	}
	closed, e := s.ReconcileTransfers(ctx)
	return count + closed, e
}
func (s *Service) RunWorker(ctx context.Context, reporters ...diagnosticreport.Sink) error {
	return s.RunWithFallback(ctx, 0, reporters...)
}
func (s *Service) RunWithFallback(ctx context.Context, interval time.Duration, reporters ...diagnosticreport.Sink) error {
	reporter := diagnosticreport.Optional(reporters)
	reconcile := diagnosticreport.NewGroup(reporter, "reconcile", s.options.Workers)
	exports := diagnosticreport.NewGroup(reporter, "export", s.options.Workers)
	if s.options.Workers < 1 || s.options.Workers > 8 {
		return Problem("audit_configuration_invalid", 500)
	}
	var wg sync.WaitGroup
	for i := 0; i < s.options.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var nextScan time.Time
			for {
				if ctx.Err() != nil {
					return
				}
				reconcile.Begin(i, diagnosticreport.Deadline(ctx))
				_, e := s.Reconcile(ctx)
				next := time.Now().Add(time.Second)
				if e != nil {
					next = time.Now().Add(5 * time.Second)
				}
				reconcile.End(i, diagnosticreport.Result(e), next)
				reporter.Dependency("postgres", e == nil, diagnosticreport.Result(e))
				worked := false
				if e == nil && s.objects != nil && !time.Now().Before(nextScan) {
					nextScan = time.Now().Add(interval)
					exports.Begin(i, diagnosticreport.Deadline(ctx))
					worked, e = s.RunExportOnce(exports.Context(ctx, i))
					next = time.Now().Add(time.Second)
					if worked && e == nil {
						next = time.Now()
					}
					if e != nil {
						next = time.Now().Add(5 * time.Second)
					}
					if nextScan.After(next) {
						next = nextScan
					}
					exports.End(i, diagnosticreport.Result(e), next)
				}
				delay := time.Second
				if e != nil {
					if ctx.Err() != nil {
						return
					}
					slog.Warn("audit worker dependency unavailable", "code", "audit_worker_dependency_unavailable")
					delay = 5 * time.Second
				} else if worked {
					continue
				}
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	}
	wg.Wait()
	return nil
}
