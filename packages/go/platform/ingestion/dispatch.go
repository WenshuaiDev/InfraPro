package ingestion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/jackc/pgx/v5"
)

// A lease attempt has its own audit row and slot token. The parser receives
// the first attempt's ID for this immutable decision, unit or plan, including
// when a process lost a successful reply before recording its selection.
const machineExecutionID = `CASE WHEN attempt=1 THEN execution_id ELSE COALESCE(
	(SELECT prior.execution_id FROM ingestion_attempts prior WHERE prior.stage=ingestion_attempts.stage
	 AND prior.target_id=ingestion_attempts.target_id AND prior.attempt=1),execution_id) END::text`

func recordAttempt(ctx context.Context, tx pgx.Tx, stage, target, execution string, fence int64, attempt int) (string, error) {
	var machineID string
	err := tx.QueryRow(ctx, `INSERT INTO ingestion_attempts(execution_id,stage,target_id,fence,attempt)
		VALUES($1,$2,$3,$4,$5) RETURNING `+machineExecutionID,
		execution, stage, target, fence, attempt).Scan(&machineID)
	return machineID, err
}

type workKey struct {
	id, scope  string
	generation int64
	settled    bool
}

// ConfigureDispatch is set before any work starts. With messages enabled an
// idle lane scans less often; once active it still drains the durable backlog.
func (s *Service) ConfigureDispatch(fallback time.Duration, sink diagnosticreport.Sink) {
	s.fallback = fallback
	s.reporter = sink
}

// Once a fact has a delivery in the active epoch, its durable event-work row
// owns both keyed and broker-independent fallback attempts. Historical rows
// without a matching event still use the original scanner. A quarantined
// delivery therefore cannot be silently executed by the legacy discovery lane.
func (s *Service) unrecorded(producer, topic, consumer, id, generation string) string {
	if s.fallback <= 0 {
		return ""
	}
	// Every argument is a constant supplied below or at a static SQL callsite.
	return fmt.Sprintf(` AND NOT EXISTS(SELECT 1 FROM event_protocol ep JOIN event_outbox o
		ON o.restore_epoch=ep.restore_epoch AND o.producer='%s' AND o.type='%s'
		AND o.business_effect_key=(%s)::text || '/' || (%s)::text
		JOIN event_work w ON (w.consumer,w.restore_epoch,w.event_id)=('%s',o.restore_epoch,o.event_id)
		WHERE ep.id=1 AND ep.enabled)`, producer, topic, id, generation, consumer)
}

func (s *Service) CandidateScans() uint64 { return s.candidateScans.Load() }
func (s *Service) recordCandidateScan() {
	s.candidateScans.Add(1)
	diagnosticreport.Count(s.reporter, "event.pg_reconciliations", 1)
}
func (s *Service) parserDuration(start time.Time) {
	diagnosticreport.SetMetric(s.reporter, "parser.duration_ms", time.Since(start).Milliseconds())
}

// IdentifyReceipt, PlanUnit and ParsePlan use the very same claim and commit
// functions as scans. The event narrows the candidate set, never the checks.
func (s *Service) IdentifyReceipt(ctx context.Context, scope, id string, generation int64) (bool, error) {
	return s.identify(ctx, &workKey{id: id, scope: scope, generation: generation})
}
func (s *Service) PlanUnit(ctx context.Context, scope, id string, generation int64) (bool, error) {
	return s.plan(ctx, &workKey{id: id, scope: scope, generation: generation})
}
func (s *Service) ParsePlan(ctx context.Context, scope, id string, generation int64) (bool, error) {
	return s.parse(ctx, &workKey{id: id, scope: scope, generation: generation})
}

// AttemptDispatch can confirm a terminal domain write directly from the
// transaction that made it. Ambiguous commits, stale keys and no-op claims
// still reread the authority instead of guessing from a worker return value.
func (s *Service) AttemptDispatch(ctx context.Context, stage, scope, id string, generation int64) (DispatchStatus, error) {
	key := workKey{id: id, scope: scope, generation: generation}
	if scope != "platform" {
		var err error
		switch stage {
		case "identify":
			_, err = s.identify(ctx, &key)
		case "plan":
			_, err = s.plan(ctx, &key)
		case "parse":
			_, err = s.parse(ctx, &key)
		default:
			return DispatchStatus{}, errors.New("unknown ingestion stage")
		}
		if err != nil {
			// A connection can fail after commit; an invalid scoped hint may also
			// name a coordination row that does not exist. Resolve both from the
			// original fact before scheduling another delivery.
			state, readErr := s.DispatchStatus(ctx, stage, id)
			if readErr == nil && (!state.Exists || state.Scope != scope || state.Generation != generation || !state.Pending) {
				return state, nil
			}
			return DispatchStatus{}, err
		}
		if key.settled {
			return DispatchStatus{Exists: true, Scope: scope, Generation: generation}, nil
		}
	}
	return s.DispatchStatus(ctx, stage, id)
}
func (k *workKey) terminalWrite(e error, terminal bool) error {
	if k != nil && e == nil {
		k.settled = terminal
	}
	return e
}

// DispatchStatus rereads the domain after an attempt, including ambiguous
// commits. Missing, superseded and pending work have different MQ semantics.
type DispatchStatus struct {
	Exists, Pending bool
	Scope           string
	Generation      int64
}

func (s *Service) DispatchStatus(ctx context.Context, stage, id string) (DispatchStatus, error) {
	var q string
	switch stage {
	case "identify":
		q = `SELECT COALESCE(r.tenant_id::text,'platform'),r.current_generation,r.tenant_id IS NOT NULL AND
		(r.state='stored' AND d.id IS NULL OR r.state IN ('stored','retry_wait','identifying') AND d.state IN ('queued','identifying','retry_wait'))
		FROM ingestion_receipts r LEFT JOIN ingestion_identity_decisions d ON d.receipt_id=r.id AND d.generation=r.current_generation WHERE r.id=$1`
	case "plan":
		q = `SELECT r.tenant_id::text,d.generation,u.state IN ('identified','demultiplexing','demultiplex_retry') FROM ingestion_units u
		JOIN ingestion_receipts r ON r.id=u.receipt_id JOIN ingestion_identity_decisions d ON d.id=u.decision_id WHERE u.id=$1`
	case "parse":
		q = `SELECT p.tenant_id::text,d.generation,p.state IN ('queued','parsing','retry_wait') FROM ingestion_parse_plans p JOIN ingestion_identity_decisions d ON d.id=p.decision_id WHERE p.id=$1`
	default:
		return DispatchStatus{}, errors.New("unknown ingestion stage")
	}
	var out DispatchStatus
	err := s.access.Transact(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, q, id).Scan(&out.Scope, &out.Generation, &out.Pending)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		out.Exists = e == nil
		return e
	})
	return out, err
}
