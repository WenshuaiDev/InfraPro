package notifications

import (
	"context"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

type Options struct {
	Batch                   int
	Lease, Poll, MaxBackoff time.Duration
	MaxAttempts             int
}

func DefaultOptions() Options { return Options{32, 30 * time.Second, time.Second, 5 * time.Minute, 8} }
func (s *Service) WithOptions(o Options) (*Service, error) {
	if o.Batch < 1 || o.Batch > 500 || o.Lease < time.Second || o.Poll < 100*time.Millisecond || o.MaxAttempts < 1 || o.MaxAttempts > 50 || o.MaxBackoff < time.Second {
		return s, errors.New("invalid notification worker options")
	}
	s.options = o
	return s, nil
}

type claim struct {
	event, account string
	fence          int64
	attempt        int
}

func (s *Service) claim(ctx context.Context) ([]claim, error) {
	return s.claimDeliveries(ctx, "")
}

// An empty ID runs bounded reconciliation. A message targets the same
// recipient rows and leases by their immutable, indexed source event ID.
func (s *Service) claimDeliveries(ctx context.Context, eventID string) ([]claim, error) {
	out := []claim{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		// Expand a bounded piece of the original immutable recipient set. A late
		// commit remains pending even when larger IDs have already been consumed.
		if _, e := tx.Exec(ctx, `WITH batch AS (SELECT event_id,account_id FROM notification_recipients WHERE expanded_at IS NULL AND ($2::uuid IS NULL OR event_id=$2) ORDER BY event_id,account_id LIMIT $1 FOR UPDATE SKIP LOCKED), staged AS (INSERT INTO notification_deliveries(event_id,account_id) SELECT event_id,account_id FROM batch ON CONFLICT DO NOTHING) UPDATE notification_recipients r SET expanded_at=clock_timestamp() FROM batch b WHERE r.event_id=b.event_id AND r.account_id=b.account_id`, s.options.Batch, eventKey(eventID)); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `WITH candidates AS (SELECT event_id,account_id FROM notification_deliveries WHERE ($3::uuid IS NULL OR event_id=$3) AND ((state IN ('pending','retry') AND available_at<=clock_timestamp()) OR (state='processing' AND lease_until<clock_timestamp())) ORDER BY available_at,event_id,account_id LIMIT $1 FOR UPDATE SKIP LOCKED) UPDATE notification_deliveries d SET state='processing',lease_until=clock_timestamp()+$2::bigint*interval '1 millisecond',fence=fence+1,attempts=attempts+1,updated_at=clock_timestamp() FROM candidates c WHERE d.event_id=c.event_id AND d.account_id=c.account_id RETURNING d.event_id::text,d.account_id::text,d.fence,d.attempts`, s.options.Batch, s.options.Lease.Milliseconds(), eventKey(eventID))
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var c claim
			if e = rows.Scan(&c.event, &c.account, &c.fence, &c.attempt); e != nil {
				return e
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, e
}

var errFence = errors.New("notification lease lost")
var errTemplate = errors.New("unknown notification event template")

func (s *Service) deliver(ctx context.Context, c claim) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		// Qualification reads precede inbox/delivery locks. Writers follow the same
		// graph -> account -> business -> inbox order; queue claims hold no graph lock.
		var ref Reference
		var policy string
		e := tx.QueryRow(ctx, `SELECT e.id::text,e.event_type,e.scope,COALESCE(e.tenant_id::text,''),COALESCE(e.project_id::text,''),e.target_kind,e.target_id,e.outcome,r.policy,COALESCE(r.tenant_membership_id::text,''),COALESCE(r.project_membership_id::text,'') FROM notification_events e JOIN notification_recipients r ON r.event_id=e.id WHERE e.id=$1 AND r.account_id=$2`, c.event, c.account).Scan(&ref.EventID, &ref.Type, &ref.Scope.Kind, &ref.Scope.Tenant, &ref.Scope.Project, &ref.Target.Kind, &ref.Target.ID, &ref.Outcome, &policy, &ref.TenantMembership, &ref.ProjectMembership)
		if e != nil {
			return e
		}
		definition, ok := ne.Catalog[ref.Type]
		if !ok {
			return errTemplate
		}
		state, reason := "delivered", ""
		if policy == "scope" {
			if s.resolver == nil {
				return errors.New("notification resolver unavailable")
			}
			resolved, e := s.resolver.ResolveTx(ctx, tx, c.account, []Reference{ref})
			if e != nil {
				return e
			}
			if !resolved[ref.EventID].Allowed {
				state, reason = "suppressed", "qualification_lost"
			}
		}
		if e = lockInbox(ctx, tx, c.account); e != nil {
			return e
		}
		var fence int64
		var live bool
		e = tx.QueryRow(ctx, `SELECT fence,state='processing' AND lease_until>clock_timestamp() FROM notification_deliveries WHERE event_id=$1 AND account_id=$2 FOR UPDATE`, c.event, c.account).Scan(&fence, &live)
		if e != nil {
			return e
		}
		if fence != c.fence || !live {
			return errFence
		}
		if !definition.Mandatory {
			var tasks, updates bool
			e = tx.QueryRow(ctx, `SELECT COALESCE((SELECT tasks FROM notification_preferences WHERE account_id=$1),true),COALESCE((SELECT updates FROM notification_preferences WHERE account_id=$1),true)`, c.account).Scan(&tasks, &updates)
			if e != nil {
				return e
			}
			if definition.Category == "tasks" && !tasks || definition.Category == "updates" && !updates {
				state, reason = "suppressed", "preference_disabled"
			}
		}
		if state == "delivered" {
			var exists bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_notifications WHERE account_id=$1 AND event_id=$2)`, c.account, c.event).Scan(&exists); e != nil {
				return e
			}
			if !exists {
				var seq int64
				if e = tx.QueryRow(ctx, `UPDATE notification_inbox_states SET sequence=sequence+1,revision=revision+1 WHERE account_id=$1 RETURNING sequence`, c.account).Scan(&seq); e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `INSERT INTO account_notifications(account_id,event_id,sequence) VALUES($1,$2,$3)`, c.account, c.event, seq); e != nil {
					return e
				}
			}
		}
		_, e = tx.Exec(ctx, `UPDATE notification_deliveries SET state=$3,reason_code=$4,lease_until=NULL,updated_at=clock_timestamp() WHERE event_id=$1 AND account_id=$2`, c.event, c.account, state, reason)
		return e
	})
}
func (s *Service) failure(ctx context.Context, c claim, cause error) error {
	state := "retry"
	if c.attempt >= s.options.MaxAttempts || errors.Is(cause, errTemplate) {
		state = "failed"
	}
	delay := time.Second * time.Duration(1<<min(c.attempt, 16))
	if delay > s.options.MaxBackoff {
		delay = s.options.MaxBackoff
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE notification_deliveries SET state=$4,reason_code=$5,lease_until=NULL,available_at=clock_timestamp()+$6::bigint*interval '1 millisecond',updated_at=clock_timestamp() WHERE event_id=$1 AND account_id=$2 AND fence=$3 AND state='processing'`, c.event, c.account, c.fence, state, sourceError(cause), delay.Milliseconds())
		return e
	})
}
func (s *Service) Tick(ctx context.Context) (int, error) {
	return s.tick(ctx, true, false)
}
func (s *Service) tick(ctx context.Context, scan, scheduled bool) (int, error) {
	maintain := func(ctx context.Context) error {
		var errs []error
		// A failing source bridge must not prevent independent account notifications.
		for _, operation := range s.maintenance {
			if e := operation(ctx); e != nil {
				errs = append(errs, e)
			}
		}
		return errors.Join(errs...)
	}
	var e error
	if scheduled {
		e = s.access.RunScheduled(ctx, "notification-maintenance", s.options.Poll, maintain)
	} else {
		e = maintain(ctx)
	}
	if !scan {
		return 0, e
	}
	n, delivery := s.deliverBatch(ctx, "")
	return n, errors.Join(e, delivery)
}

func eventKey(id string) any {
	if id == "" {
		return nil
	}
	return id
}
func (s *Service) deliverBatch(ctx context.Context, id string) (int, error) {
	var errs []error
	claims, e := s.claimDeliveries(ctx, id)
	if e != nil {
		return 0, errors.Join(append(errs, e)...)
	}
	for _, c := range claims {
		if e = s.deliver(ctx, c); e != nil && !errors.Is(e, errFence) {
			if failure := s.failure(ctx, c, e); failure != nil {
				errs = append(errs, failure)
			}
			errs = append(errs, e)
		}
	}
	return len(claims), errors.Join(errs...)
}

// DeliverEvent finishes a bounded page and tells the dispatcher whether this
// event still has recoverable deliveries. A failed/suppressed/de-duplicated
// recipient is a durable outcome handled by the existing delivery protocol.
func (s *Service) DeliverEvent(ctx context.Context, id string, version int64, scope string) (pending bool, err error) {
	if !access.ValidID(id) || version != 1 {
		return false, pgx.ErrNoRows
	}
	err = s.access.Transact(ctx, func(tx pgx.Tx) error {
		var authorized bool
		return tx.QueryRow(ctx, `SELECT true FROM notification_events WHERE id=$1 AND COALESCE(tenant_id::text,'platform')=$2`, id, scope).Scan(&authorized)
	})
	if err != nil {
		return false, err
	}
	if _, err = s.deliverBatch(ctx, id); err != nil {
		return false, err
	}
	err = s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_recipients WHERE event_id=e.id AND expanded_at IS NULL)
			OR EXISTS(SELECT 1 FROM notification_deliveries WHERE event_id=e.id AND state IN ('pending','processing','retry'))
			FROM notification_events e WHERE e.id=$1`, id).Scan(&pending)
	})
	return pending, err
}

func (s *Service) Run(ctx context.Context, worker string, report func(error), reporters ...diagnosticreport.Sink) {
	s.RunWithFallback(ctx, 0, worker, report, reporters...)
}
func (s *Service) RunWithFallback(ctx context.Context, interval time.Duration, worker string, report func(error), reporters ...diagnosticreport.Sink) {
	var nextScan time.Time
	reporter := diagnosticreport.Optional(reporters)
	ticker := time.NewTicker(s.options.Poll)
	defer ticker.Stop()
	for {
		reporter.Begin("delivery", diagnosticreport.Deadline(ctx))
		e := s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `INSERT INTO notification_worker_heartbeats(worker_id) VALUES($1) ON CONFLICT(worker_id) DO UPDATE SET heartbeat_at=clock_timestamp()`, worker)
			return e
		})
		reporter.Dependency("postgres", e == nil, diagnosticreport.Result(e))
		if e == nil {
			scan := !time.Now().Before(nextScan)
			_, e = s.tick(ctx, scan, true)
			if scan {
				nextScan = time.Now().Add(interval)
			}
		}
		reporter.End("delivery", diagnosticreport.Result(e), time.Now().Add(s.options.Poll))
		if e != nil && ctx.Err() == nil && report != nil {
			report(e)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type Failure struct {
	Event  string `json:"event_id"`
	Type   string `json:"type"`
	Reason string `json:"reason_code"`
	Count  int64  `json:"count"`
}
type Diagnostics struct {
	Failures         []Failure        `json:"failures"`
	Pending          int64            `json:"pending"`
	Failed           int64            `json:"failed"`
	Suppressed       int64            `json:"suppressed"`
	EmptyRecipients  int64            `json:"empty_recipients"`
	OldestAgeSeconds int64            `json:"oldest_age_seconds"`
	LastHeartbeat    *time.Time       `json:"last_heartbeat"`
	Reasons          map[string]int64 `json:"reasons"`
}

// Diagnostics and Replay are CLI-only capabilities, never exposed through a
// personnel/admin HTTP permission or a display ReadSubject.
func (s *Service) Diagnostics(ctx context.Context) (Diagnostics, error) {
	v := Diagnostics{Reasons: map[string]int64{}}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state IN ('pending','retry','processing')),count(*) FILTER(WHERE state='failed'),count(*) FILTER(WHERE state='suppressed'),COALESCE(extract(epoch FROM clock_timestamp()-min(e.created_at) FILTER(WHERE d.state IN ('pending','retry','processing')))::bigint,0),(SELECT count(*) FROM notification_events WHERE recipient_count=0),(SELECT max(heartbeat_at) FROM notification_worker_heartbeats) FROM notification_deliveries d JOIN notification_events e ON e.id=d.event_id`).Scan(&v.Pending, &v.Failed, &v.Suppressed, &v.OldestAgeSeconds, &v.EmptyRecipients, &v.LastHeartbeat); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT reason_code,count(*) FROM notification_deliveries WHERE reason_code<>'' GROUP BY reason_code`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var code string
			var n int64
			if e = rows.Scan(&code, &n); e != nil {
				return e
			}
			v.Reasons[code] = n
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		failures, e := tx.Query(ctx, `SELECT e.id::text,e.event_type,d.reason_code,count(*) FROM notification_deliveries d JOIN notification_events e ON e.id=d.event_id WHERE d.state='failed' GROUP BY e.id,e.event_type,d.reason_code ORDER BY e.id LIMIT 50`)
		if e != nil {
			return e
		}
		defer failures.Close()
		v.Failures = []Failure{}
		for failures.Next() {
			var f Failure
			if e = failures.Scan(&f.Event, &f.Type, &f.Reason, &f.Count); e != nil {
				return e
			}
			v.Failures = append(v.Failures, f)
		}
		var unexpanded, unexpandedAge int64
		if e = tx.QueryRow(ctx, `SELECT count(*),COALESCE(extract(epoch FROM clock_timestamp()-min(e.created_at))::bigint,0) FROM notification_recipients r JOIN notification_events e ON e.id=r.event_id WHERE r.expanded_at IS NULL`).Scan(&unexpanded, &unexpandedAge); e != nil {
			return e
		}
		v.Pending += unexpanded
		v.OldestAgeSeconds = max(v.OldestAgeSeconds, unexpandedAge)
		if v.EmptyRecipients > 0 {
			v.Reasons["no_qualified_recipients"] = v.EmptyRecipients
		}
		return failures.Err()
	})
	return v, e
}
func (s *Service) Replay(ctx context.Context, event, reason string) (int64, error) {
	if !access.ValidID(event) {
		return 0, access.ErrNotFound
	}
	if e := access.ValidReason(reason); e != nil {
		return 0, e
	}
	var count int64
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `UPDATE notification_deliveries SET state='pending',attempts=0,available_at=clock_timestamp(),lease_until=NULL,fence=fence+1 WHERE event_id=$1 AND state='failed'`, event)
		if e != nil {
			return e
		}
		count = tag.RowsAffected()
		return access.Audit(ctx, tx, "notification.delivery.replayed", "notification_event", event, "", nil, map[string]any{"count": count}, access.Meta{ActorType: "cli", Reason: reason})
	})
	return count, e
}
func (s *Service) Check(ctx context.Context) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		var v int
		e := tx.QueryRow(ctx, `SELECT version FROM notification_protocol WHERE id=1`).Scan(&v)
		if e != nil {
			return e
		}
		if v != 1 && v != 2 {
			return fmt.Errorf("notification protocol %d unsupported", v)
		}
		return nil
	})
}

// CheckWorker is separate from the protocol check: a newly started worker has
// not written its first heartbeat yet. Supervisors can use this after startup.
func (s *Service) CheckWorker(ctx context.Context, maxAge time.Duration) error {
	if maxAge < time.Second {
		return errors.New("worker heartbeat max age must be at least one second")
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		var alive bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_worker_heartbeats WHERE heartbeat_at>clock_timestamp()-$1::bigint*interval '1 millisecond')`, maxAge.Milliseconds()).Scan(&alive); e != nil {
			return e
		}
		if !alive {
			return errors.New("notification worker heartbeat missing or stale")
		}
		return nil
	})
}

func (s *Service) InspectEvent(ctx context.Context, id string) (map[string]any, error) {
	if !access.ValidID(id) {
		return nil, access.ErrNotFound
	}
	out := map[string]any{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var typ, module string
		var count int
		if e := tx.QueryRow(ctx, `SELECT event_type,source_module,recipient_count FROM notification_events WHERE id=$1`, id).Scan(&typ, &module, &count); e != nil {
			if e == pgx.ErrNoRows {
				return access.ErrNotFound
			}
			return e
		}
		out["id"], out["type"], out["source_module"], out["recipient_count"] = id, typ, module, count
		rows, e := tx.Query(ctx, `SELECT state,reason_code,count(*),max(attempts) FROM notification_deliveries WHERE event_id=$1 GROUP BY state,reason_code ORDER BY state,reason_code`, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		states := []map[string]any{}
		for rows.Next() {
			var state, reason string
			var n, attempts int
			if e = rows.Scan(&state, &reason, &n, &attempts); e != nil {
				return e
			}
			states = append(states, map[string]any{"state": state, "reason_code": reason, "count": n, "max_attempts": attempts})
		}
		out["deliveries"] = states
		return rows.Err()
	})
	return out, e
}
