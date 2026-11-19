package monitoring

import (
	"context"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type WorkerRole string

const (
	PublishWorker     WorkerRole = "monitoring-publish-worker"
	IncrementalWorker WorkerRole = "monitoring-incremental-worker"
	HistoryWorker     WorkerRole = "monitoring-history-worker"
	ReadWorker        WorkerRole = "monitoring-read-worker"
)

// The same expression classifies a scan, an explicit ID and expired leases.
// Missing analysis.trial was historically decoded as false. Unknown summary
// inputs still go through the summary executor's bounded failure handling.
const taskOwnerSQL = `(CASE
 WHEN kind IN ('validate','publish','relocate','verify') THEN 'monitoring-publish-worker'
 WHEN kind='summary' AND input->>'mode'='maintenance'
   OR kind='derive' AND input->>'mode' IN ('trial','automatic','correction')
   OR kind='analysis' AND input->>'trial'='true' THEN 'monitoring-incremental-worker'
 WHEN kind='summary' AND input->>'mode'='query' THEN 'monitoring-read-worker'
 ELSE 'monitoring-history-worker' END)`

type roleKey struct{}

func WorkerContext(ctx context.Context, role WorkerRole) context.Context {
	return context.WithValue(ctx, roleKey{}, role)
}
func workerRole(ctx context.Context) WorkerRole {
	role, _ := ctx.Value(roleKey{}).(WorkerRole)
	return role
}

// JobOwner resolves the actual stored input, including pre-split jobs. Supplying
// a CLI flag or a different pool cannot relabel the operation.
func (s *Service) JobOwner(ctx context.Context, tenant, id string) (WorkerRole, error) {
	if !access.ValidID(tenant) || !access.ValidID(id) {
		return "", notFound()
	}
	var role WorkerRole
	err := s.db(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT `+taskOwnerSQL+` FROM monitoring_jobs WHERE tenant_id=$1 AND id=$2 AND kind=ANY($3::text[])`, tenant, id, s.claimKinds()).Scan(&role)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		return e
	})
	return role, err
}

// Serialize only the brief admission decision. Once claimed, the existing
// heartbeat/expiry is the durable occupancy record shared by all replicas.
func (s *Service) hasWorkerSlot(ctx context.Context, tx pgx.Tx, role WorkerRole, queue string, interactive bool) (bool, error) {
	if role == "" {
		return true, nil
	}
	bucket := queue
	budget := s.workers.Historical
	if queue == "foreground" {
		budget = s.workers.Foreground
	}
	if interactive {
		bucket = "interactive"
		budget = 1
	}
	var locked bool
	if e := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "monitoring/claim/"+string(role)+"/"+bucket).Scan(&locked); e != nil || !locked {
		return false, e
	}
	var occupied int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM monitoring_jobs WHERE state='running' AND lease_until>clock_timestamp()
	 AND `+taskOwnerSQL+`=$1 AND `+taskQueueSQL+`=$2 AND (lease_owner LIKE 'inline-trial-%')=$3`, role, queue, interactive).Scan(&occupied)
	return occupied < budget, err
}

// Existing callers without a role keep their original local clock.
func (s *Service) scheduleDue(ctx context.Context, lane string, interval time.Duration, schedule func(context.Context) error) error {
	if workerRole(ctx) == "" {
		return schedule(ctx)
	}
	return s.access.RunScheduled(ctx, lane, interval, schedule)
}

func (r WorkerRole) valid() bool {
	switch r {
	case PublishWorker, IncrementalWorker, HistoryWorker, ReadWorker:
		return true
	}
	return false
}
