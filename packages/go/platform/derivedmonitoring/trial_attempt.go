package derivedmonitoring

import (
	"context"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TrialAttempt always targets a job committed by the caller. Losing the
// response leaves that very same job available for the normal background path.
type TrialAttempt func(ctx context.Context, tenant, id string, budget time.Duration) (bool, error)

func (s *Service) WithTrialAttempt(attempt TrialAttempt) { s.trialAttempt = attempt }

// An ordinary incremental worker can win the lease between enqueue and the
// private RPC. If it finishes within the original interaction budget, return
// its result to the same request. Never claim again or extend the deadline.
func (s *Service) waitClaimedTrial(ctx context.Context, tenant, job string, deadline time.Time) {
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	delay := 20 * time.Millisecond
	for ctx.Err() == nil {
		var running bool
		err := s.access.Transact(ctx, func(tx pgx.Tx) error {
			// The claimant uses FOR UPDATE SKIP LOCKED. A plain read here could
			// still see 'queued' after our RPC skipped an uncommitted claim.
			return tx.QueryRow(ctx, `SELECT state='running' FROM monitoring_jobs WHERE tenant_id=$1 AND id=$2 FOR SHARE`, tenant, job).Scan(&running)
		})
		if err != nil || !running {
			return
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(2*delay, 100*time.Millisecond)
	}
}

// AttemptTrial runs on the incremental owner. Check the immutable run and
// range independently from the caller; the host then checks the actual claim
// and its actor's current authorization inside the existing transactions.
func (s *Service) AttemptTrial(ctx context.Context, tenant, id string, budget time.Duration) (bool, error) {
	if !access.ValidID(tenant) || !access.ValidID(id) {
		return false, notFound()
	}
	if budget <= 0 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, min(budget, s.options.TrialDeadline))
	defer cancel()
	var eligible bool
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_runs r JOIN monitoring_jobs j ON j.tenant_id=r.tenant_id AND j.id=r.job_id
		   WHERE r.tenant_id=$1 AND r.job_id=$2 AND r.mode='trial' AND j.kind='derive' AND j.input->>'mode'='trial'
		   AND j.target_id=r.id AND j.actor_id=r.actor_id AND NOT upper_inf(r.output_range)
		   AND upper(r.output_range)::numeric-lower(r.output_range)::numeric <= $3)`, tenant, id, s.options.TrialRange.Nanoseconds()).Scan(&eligible)
	})
	if e != nil || !eligible {
		return false, e
	}
	ctx = context.WithValue(ctx, inlineTrialKey{}, true)
	return s.monitoring.RunInteractiveTask(monitoring.WorkerContext(ctx, monitoring.IncrementalWorker), "inline-trial-"+uuid.NewString(), tenant, id, budget)
}
