package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/dispatch"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type job struct {
	Event
	consumer string
	fence    int64
	attempt  int
}

func current(ctx context.Context, tx pgx.Tx, epoch string) error {
	err := dispatch.CheckEventEpoch(ctx, tx, epoch)
	if errors.Is(err, dispatch.ErrRetired) {
		return errors.Join(errEpoch, err)
	}
	return err
}
func quarantine(ctx context.Context, tx pgx.Tx, consumer string, e Event, reason string, attempt int) error {
	_, err := tx.Exec(ctx, `INSERT INTO event_quarantine(consumer,restore_epoch,event_id,aggregate_id,reason,attempts)
  VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(consumer,restore_epoch,event_id) DO UPDATE SET
  attempts=GREATEST(event_quarantine.attempts,EXCLUDED.attempts),updated_at=clock_timestamp(),
  reason=EXCLUDED.reason,state='open'`, consumer, e.Epoch, e.ID, optionalID(e.AggregateID), reason, attempt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE event_work SET state='quarantined',fence=fence+1,lease_until=NULL
 WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3 AND state<>'done'`, consumer, e.Epoch, e.ID)
	return err
}
func optionalID(id string) any {
	if validID(id) {
		return id
	}
	return nil
}
func messageIdentity(epoch, consumer, subject string, seq uint64) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%s/%s/%s/%d", epoch, consumer, subject, seq))).String()
}

func accept(ctx context.Context, tx pgx.Tx, consumer string, e Event, via string) (bool, error) {
	// Reading the assignment and writing its receipt share a single statement.
	// A duplicate may increase the receipt counter but cannot reset active work.
	var assigned, runnable bool
	err := tx.QueryRow(ctx, `WITH target AS MATERIALIZED (
   SELECT consumer,restore_epoch,event_id,state,lease_until FROM event_work WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3),
  wake AS (
   UPDATE event_work SET state='ready'
   WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3 AND state='awaiting'),
  receipt AS (
   INSERT INTO event_consumptions(consumer,restore_epoch,event_id,via)
   SELECT consumer,restore_epoch,event_id,$4 FROM target
   ON CONFLICT(consumer,restore_epoch,event_id) DO UPDATE SET duplicates=event_consumptions.duplicates+1 RETURNING event_id)
  SELECT EXISTS(SELECT FROM receipt),EXISTS(SELECT FROM target WHERE state IN ('awaiting','ready') OR state='running' AND lease_until<=statement_timestamp())`, consumer, e.Epoch, e.ID, via).Scan(&assigned, &runnable)
	if err == nil && !assigned {
		return false, ErrBadReference
	}
	return runnable, err
}

type handoff struct {
	key         string
	quarantined bool
	leased      *job
}

func receive(ctx context.Context, p *pgxpool.Pool, b *broker, name string, msg jetstream.Msg) (handoff, error) {
	return receiveDelivery(ctx, p, b, name, msg, false)
}

func directIngestionClaim(consumer string) bool {
	return consumer == "identify" || consumer == "parse"
}

// These two keyed machine stages have no ordered monitoring checkpoint. An
// available executor can take the lease in the receipt's own transaction.
func grantIngestion(ctx context.Context, tx pgx.Tx, c Config, consumer string, e Event) (handoff, error) {
	j := job{Event: e, consumer: consumer}
	var assigned, exhausted bool
	err := tx.QueryRow(ctx, `WITH target AS MATERIALIZED (
	  SELECT state,attempts,((state IN ('awaiting','ready') AND available_at<=statement_timestamp()) OR
	    (state='running' AND lease_until<=statement_timestamp())) due
	  FROM event_work WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3 FOR UPDATE),
	receipt AS (
	  INSERT INTO event_consumptions(consumer,restore_epoch,event_id,via)
	  SELECT $1,$2,$3,'broker' FROM target ON CONFLICT(consumer,restore_epoch,event_id)
	    DO UPDATE SET duplicates=event_consumptions.duplicates+1 RETURNING event_id),
	lease AS (
	  UPDATE event_work w SET state='running',fence=w.fence+1,attempts=w.attempts+1,
	    lease_until=clock_timestamp()+$4*interval '1 millisecond'
	  FROM target t WHERE w.consumer=$1 AND w.restore_epoch=$2 AND w.event_id=$3
	    AND t.due AND t.attempts<$5 RETURNING w.fence,w.attempts)
	SELECT EXISTS(SELECT FROM receipt),EXISTS(SELECT FROM target WHERE due AND attempts>=$5),
	  coalesce((SELECT fence FROM lease),0),coalesce((SELECT attempts FROM lease),0)`, consumer, e.Epoch, e.ID, c.AckWaitMS, c.MaxAttempts).Scan(&assigned, &exhausted, &j.fence, &j.attempt)
	if err != nil {
		return handoff{}, err
	}
	if !assigned {
		return handoff{}, ErrBadReference
	}
	if exhausted {
		return handoff{quarantined: true}, quarantine(ctx, tx, j.consumer, e, "work_exhausted", c.MaxAttempts)
	}
	if j.fence == 0 {
		return handoff{}, nil
	}
	return handoff{leased: &j}, nil
}

func receiveDelivery(ctx context.Context, p *pgxpool.Pool, b *broker, name string, msg jetstream.Msg, withSlot bool) (handoff, error) {
	var result handoff
	meta, err := msg.Metadata()
	if err != nil {
		return result, err
	}
	topic := consumerTopics[name]
	e, why := decode(msg.Data(), b.epoch, topic)
	if b.subject(topic) != msg.Subject() {
		why = "bad_reference"
	}
	key := e.ID
	if !validID(key) {
		key = messageIdentity(b.epoch, name, msg.Subject(), meta.Sequence.Stream)
	}
	err = pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error {
		if err := current(ctx, tx, b.epoch); err != nil {
			return err
		}
		original, err := canonical(ctx, tx, b.epoch, key)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// A legitimate delivery outside PG retention has no uncompleted slot.
			// Keep missing references observable and retain only IDs and reason.
			why = "bad_reference"
		case err != nil:
			return err
		case why == "" && !equal(original, e):
			why = "bad_reference"
		}
		if why != "" {
			evidence := Event{ID: key, Epoch: b.epoch, AggregateID: original.AggregateID}
			return quarantine(ctx, tx, name, evidence, why, int(meta.NumDelivered))
		}
		// A rebuilt event can omit this group because its old effect is complete.
		if original.Cause != "" {
			var assigned bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM event_work WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3)`, name, b.epoch, key).Scan(&assigned); err != nil || !assigned {
				return err
			}
		}
		var runnable bool
		if withSlot && directIngestionClaim(name) {
			result, err = grantIngestion(ctx, tx, b.cfg, name, original)
		} else {
			runnable, err = accept(ctx, tx, name, e, "broker")
		}
		if errors.Is(err, ErrBadReference) {
			why = "bad_reference"
			return quarantine(ctx, tx, name, original, why, int(meta.NumDelivered))
		}
		if err != nil {
			return err
		}
		if runnable {
			result.key = key
		}
		return dispatch.CheckTx(ctx, tx)
	})
	if err != nil {
		return handoff{}, err
	}
	// Even on an ambiguous DoubleAck, the durable handoff is already available.
	// Broker redelivery will confirm the existing effect rather than reset it.
	ack, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if why != "" {
		return handoff{quarantined: true}, msg.TermWithReason(why)
	}
	if result.quarantined {
		return result, msg.TermWithReason("work_exhausted")
	}
	return result, msg.DoubleAck(ack)
}
func checkpoint(ctx context.Context, tx pgx.Tx, name string, e Event) (int64, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO event_consumer_positions(consumer,restore_epoch,scope,position) VALUES($1,$2,$3,0) ON CONFLICT DO NOTHING`, name, e.Epoch, e.Scope); err != nil {
		return 0, err
	}
	var pos int64
	err := tx.QueryRow(ctx, `SELECT position FROM event_consumer_positions WHERE consumer=$1 AND restore_epoch=$2 AND scope=$3 FOR UPDATE`, name, e.Epoch, e.Scope).Scan(&pos)
	return pos, err
}
func claimWork(ctx context.Context, p *pgxpool.Pool, c Config, name, epoch, id string, reporters ...diagnosticreport.Sink) (*job, error) {
	var out *job
	quarantineReason := ""
	isolate := func(tx pgx.Tx, j job, reason string) error {
		quarantineReason = reason
		return quarantine(ctx, tx, name, j.Event, reason, j.attempt)
	}
	err := pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error {
		if err := current(ctx, tx, epoch); err != nil {
			return err
		}
		var state string
		j := job{consumer: name}
		args := []any{name, epoch}
		target := ""
		if id != "" {
			target = " AND w.event_id=$3"
			args = append(args, id)
		}
		query := `SELECT ` + columns + `,w.state,w.attempts FROM event_work w JOIN event_outbox o USING(restore_epoch,event_id)
          WHERE w.consumer=$1 AND w.restore_epoch=$2` + target + ` AND
          ((w.state IN ('awaiting','ready') AND w.available_at<=statement_timestamp()) OR
           (w.state='running' AND w.lease_until<=statement_timestamp())) FOR UPDATE OF w SKIP LOCKED`
		if id == "" {
			// Give fresh hints a delivery window before a healthy broker's
			// low-frequency repair scan competes for brand-new work.
			grace := 0
			if c.Mode == "jetstream" {
				grace = c.FallbackScanMS
			}
			args = append(args, grace)
			// Select the two indexed queue heads before joining the outbox.
			// Sorting the whole joined backlog for a single job grows with
			// history and penalizes PG fallback exactly when it is needed.
			query = `WITH awaiting AS MATERIALIZED(
               SELECT event_id,available_at AS due FROM event_work WHERE consumer=$1 AND restore_epoch=$2
               AND state='awaiting' AND available_at<=statement_timestamp()-$3::bigint*interval '1 millisecond'
               ORDER BY available_at,event_id LIMIT 1 FOR UPDATE SKIP LOCKED)
              , ready AS MATERIALIZED(
               SELECT event_id,available_at AS due FROM event_work WHERE consumer=$1 AND restore_epoch=$2 AND state='ready' AND available_at<=statement_timestamp()
               ORDER BY available_at,event_id LIMIT 1 FOR UPDATE SKIP LOCKED)
              , expired AS MATERIALIZED(
               SELECT event_id,lease_until AS due FROM event_work WHERE consumer=$1 AND restore_epoch=$2
               AND state='running' AND lease_until<=statement_timestamp()
               ORDER BY lease_until,event_id LIMIT 1 FOR UPDATE SKIP LOCKED)
            , picked AS MATERIALIZED(SELECT event_id FROM (TABLE awaiting UNION ALL TABLE ready UNION ALL TABLE expired) heads ORDER BY due,event_id LIMIT 1)
            SELECT ` + columns + `,w.state,w.attempts FROM picked k JOIN event_outbox o ON o.event_id=k.event_id AND o.restore_epoch=$2
            JOIN event_work w ON w.event_id=k.event_id AND w.restore_epoch=$2 AND w.consumer=$1`
		}
		err := tx.QueryRow(ctx, query, args...).Scan(append(j.targets(), &state, &j.attempt)...)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if j.Version != 1 {
			return isolate(tx, j, "unsupported_version")
		}
		if j.Type != consumerTopics[name] || !validID(j.AggregateID) || j.Generation < 0 || j.OccurredAt.IsZero() {
			return isolate(tx, j, "bad_reference")
		}
		if state == "awaiting" && j.Type == "monitoring.changed" {
			if _, err = accept(ctx, tx, name, j.Event, "pg"); err != nil {
				return err
			}
		}
		if j.Type == "monitoring.changed" {
			pos, err := checkpoint(ctx, tx, name, j.Event)
			if err != nil {
				return err
			}
			if j.Generation <= pos {
				_, err = tx.Exec(ctx, `UPDATE event_work SET state='done',lease_until=NULL WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3`, name, epoch, j.ID)
				return err
			}
			if j.Generation != pos+1 {
				var preceding bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_changes WHERE tenant_id=$1 AND source_position=$2)`, j.Scope, pos+1).Scan(&preceding); err != nil {
					return err
				}
				if !preceding {
					return isolate(tx, j, "position_gap")
				}
				_, err = tx.Exec(ctx, `UPDATE event_work SET state='ready',available_at=clock_timestamp()+$4*interval '1 millisecond',lease_until=NULL WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3`, name, epoch, j.ID, c.BackoffMS)
				return err
			}
		}
		// A work lease is separate from the broker acknowledgment. Both the normal
		// path and PG recovery consult the same original ID and domain generation.
		if j.attempt >= c.MaxAttempts {
			return isolate(tx, j, "work_exhausted")
		}
		// A PG-only handoff of an unordered key needs one durable transition.
		// Broker handoffs and ordered positions already have their receipt.
		pgHandoff := state == "awaiting" && j.Type != "monitoring.changed"
		err = tx.QueryRow(ctx, `WITH lease AS (
   UPDATE event_work SET state='running',fence=fence+1,attempts=attempts+1,lease_until=clock_timestamp()+$4*interval '1 millisecond'
   WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3 RETURNING consumer,restore_epoch,event_id,fence,attempts),
  receipt AS (
   INSERT INTO event_consumptions(consumer,restore_epoch,event_id,via)
   SELECT consumer,restore_epoch,event_id,'pg' FROM lease WHERE $5
   ON CONFLICT(consumer,restore_epoch,event_id) DO UPDATE SET duplicates=event_consumptions.duplicates+1)
  SELECT fence,attempts FROM lease`, name, epoch, j.ID, c.AckWaitMS, pgHandoff).Scan(&j.fence, &j.attempt)
		if err != nil {
			return err
		}
		out = &j
		return dispatch.CheckTx(ctx, tx)
	})
	if err == nil && quarantineReason != "" {
		r := diagnosticreport.Optional(reporters)
		diagnosticreport.Count(r, "event.quarantined", 1)
		if quarantineReason == "position_gap" {
			diagnosticreport.Count(r, "event.position_gaps", 1)
		}
	}
	return out, err
}
func finishWork(ctx context.Context, p *pgxpool.Pool, c Config, j job, cause error) error {
	return pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error {
		if err := current(ctx, tx, j.Epoch); err != nil {
			return err
		}
		if cause != nil {
			deferred := errors.Is(cause, ErrDeferred) || errors.Is(cause, context.Canceled) || errors.Is(cause, dispatch.ErrRetired)
			reason := ""
			if errors.Is(cause, ErrBadReference) {
				reason = "bad_reference"
			} else if !deferred && j.attempt >= c.MaxAttempts {
				reason = "work_exhausted"
			}
			if reason != "" {
				var live bool
				err := tx.QueryRow(ctx, `SELECT true FROM event_work WHERE consumer=$1 AND restore_epoch=$2
    AND event_id=$3 AND fence=$4 AND state='running' AND lease_until>clock_timestamp() FOR UPDATE`, j.consumer, j.Epoch, j.ID, j.fence).Scan(&live)
				if errors.Is(err, pgx.ErrNoRows) {
					return dispatch.ErrRetired
				}
				if err != nil {
					return err
				}
				return quarantine(ctx, tx, j.consumer, j.Event, reason, j.attempt)
			}
			tag, err := tx.Exec(ctx, `UPDATE event_work SET state='ready',lease_until=NULL,attempts=attempts-CASE WHEN $5 THEN 1 ELSE 0 END,
    available_at=clock_timestamp()+$6*interval '1 millisecond' WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3
    AND fence=$4 AND state='running' AND lease_until>clock_timestamp()`, j.consumer, j.Epoch, j.ID, j.fence, deferred, c.backoff(j.attempt).Milliseconds())
			if err == nil && tag.RowsAffected() != 1 {
				return dispatch.ErrRetired
			}
			return err
		}
		var done bool
		err := tx.QueryRow(ctx, `WITH finished AS (
    UPDATE event_work SET state='done',lease_until=NULL WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3
     AND fence=$4 AND state='running' AND lease_until>clock_timestamp() RETURNING consumer,restore_epoch,event_id),
   resolved AS (
    UPDATE event_quarantine q SET state='resolved',updated_at=clock_timestamp() FROM finished f
     WHERE (q.consumer,q.restore_epoch,q.event_id)=(f.consumer,f.restore_epoch,f.event_id) AND q.state='redriven')
   SELECT EXISTS(SELECT FROM finished)`, j.consumer, j.Epoch, j.ID, j.fence).Scan(&done)
		if err != nil {
			return err
		}
		if !done {
			return dispatch.ErrRetired
		}
		if j.Type == "monitoring.changed" {
			tag, err := tx.Exec(ctx, `UPDATE event_consumer_positions SET position=$4,updated_at=clock_timestamp() WHERE consumer=$1 AND restore_epoch=$2 AND scope=$3 AND position=$4-1`, j.consumer, j.Epoch, j.Scope, j.Generation)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return errors.New("event position changed")
			}
		}
		return dispatch.CheckTx(ctx, tx)
	})
}
func execute(ctx context.Context, p *pgxpool.Pool, c Config, j job, work func(context.Context, Event) error, r diagnosticreport.Sink) time.Time {
	run, cancel := context.WithTimeout(ctx, ms(c.WorkTimeoutMS))
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- work(dispatch.WithEventWork(run, j.consumer, j.Epoch, j.ID, j.fence), j.Event) }()
	ticker := time.NewTicker(ms(c.AckWaitMS) / 3)
	defer ticker.Stop()
	diagnosticreport.SetMetric(r, "event.delivery_lag_ms", max(int64(0), time.Since(j.OccurredAt).Milliseconds()))
	var err error
loop:
	for {
		select {
		case err = <-result:
			break loop
		case <-run.Done():
			err = run.Err()
			<-result
			break loop
		case <-ticker.C:
			e := pgx.BeginFunc(run, p, func(tx pgx.Tx) error {
				if err := current(run, tx, j.Epoch); err != nil {
					return err
				}
				tag, err := tx.Exec(run, `UPDATE event_work SET lease_until=clock_timestamp()+$5*interval '1 millisecond'
  WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3 AND fence=$4 AND state='running' AND lease_until>clock_timestamp()`, j.consumer, j.Epoch, j.ID, j.fence, c.AckWaitMS)
				if err == nil && tag.RowsAffected() != 1 {
					return dispatch.ErrRetired
				}
				return err
			})
			if e != nil {
				cancel()
				<-result
				err = dispatch.ErrRetired
				break loop
			}
		}
	}
	finish, done := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer done()
	if e := finishWork(finish, p, c, j, err); e != nil {
		if errors.Is(e, dispatch.ErrRetired) || errors.Is(e, errEpoch) {
			diagnosticreport.Count(r, "dispatch.rejected_fences", 1)
		}
		return time.Time{}
	}
	if err == nil && j.Type == "monitoring.changed" {
		diagnosticreport.SetMetric(r, "event.consumer_position", j.Generation)
	}
	if errors.Is(err, ErrBadReference) || err != nil && !errors.Is(err, ErrDeferred) && !errors.Is(err, context.Canceled) && !errors.Is(err, dispatch.ErrRetired) && j.attempt >= c.MaxAttempts {
		diagnosticreport.Count(r, "event.quarantined", 1)
		return time.Time{}
	}
	if err != nil {
		return time.Now().Add(c.backoff(j.attempt))
	}
	return time.Time{}
}

func pumpBroker(ctx context.Context, p *pgxpool.Pool, c Config, name, epoch string, keys chan<- string, r diagnosticreport.Sink, deliver func(*broker, jetstream.Msg) (handoff, error)) {
	for ctx.Err() == nil {
		b, e := connect(c, epoch, name)
		if e != nil {
			r.Dependency("jetstream", false, "broker_unavailable")
			pause(ctx, c.backoff(3))
			continue
		}
		func() {
			defer b.nc.Close()
			bind, cancel := context.WithTimeout(ctx, 3*time.Second)
			topic := consumerTopics[name]
			stream := b.stream(family(topic))
			con, err := b.js.Consumer(bind, stream, name)
			cancel()
			if err != nil {
				return
			}
			notices, stop := context.WithCancel(ctx)
			var reader sync.WaitGroup
			reader.Go(func() { watchAdvisories(notices, p, b, name, r) })
			defer func() { stop(); reader.Wait() }()
			for ctx.Err() == nil {
				batch, err := con.Fetch(c.Batch, jetstream.FetchMaxWait(time.Second))
				if err != nil {
					return
				}
				for m := range batch.Messages() {
					handoff, err := deliver(b, m)
					if err != nil {
						if errors.Is(err, errEpoch) {
							return
						}
						_ = m.NakWithDelay(ms(c.BackoffMS))
						continue
					}
					r.Dependency("jetstream", true, "")
					if meta, err := m.Metadata(); err == nil && meta.NumDelivered > 1 {
						diagnosticreport.Count(r, "event.redeliveries", 1)
					}
					if handoff.quarantined {
						diagnosticreport.Count(r, "event.quarantined", 1)
					}
					if handoff.key != "" {
						select {
						case keys <- handoff.key:
						case <-ctx.Done():
							return
						}
					}
				}
				if e = batch.Error(); e != nil && !errors.Is(e, nats.ErrTimeout) {
					return
				}
			}
		}()
		if ctx.Err() == nil {
			r.Dependency("jetstream", false, "broker_unavailable")
			pause(ctx, c.backoff(3))
		}
	}
}
func watchAdvisories(ctx context.Context, p *pgxpool.Pool, b *broker, name string, r diagnosticreport.Sink) {
	stream := b.stream(family(consumerTopics[name]))
	for ctx.Err() == nil {
		con, err := b.js.Consumer(ctx, stream, name+"-advisories")
		if err != nil {
			pause(ctx, b.cfg.backoff(4))
			continue
		}
		// This subscription normally stays idle. A cancelable long poll avoids
		// wakeups and permits a prompt shutdown even with a quiet stream.
		for ctx.Err() == nil {
			wait, done := context.WithTimeout(ctx, 20*time.Second)
			batch, fetchErr := con.Fetch(b.cfg.Batch, jetstream.FetchContext(wait))
			if fetchErr != nil {
				done()
				break
			}
			for m := range batch.Messages() {
				stored, err := recordAdvisory(ctx, p, b, name, stream, m)
				if err != nil {
					_ = m.NakWithDelay(b.cfg.backoff(5))
					continue
				}
				if stored {
					diagnosticreport.Count(r, "event.quarantined", 1)
				}
				ack, cancel := context.WithTimeout(ctx, 2*time.Second)
				_ = m.DoubleAck(ack)
				cancel()
			}
			done()
			if err = batch.Error(); err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, nats.ErrTimeout) {
				break
			}
		}
		pause(ctx, b.cfg.backoff(3))
	}
}

func recordAdvisory(ctx context.Context, p *pgxpool.Pool, b *broker, name, stream string, notice jetstream.Msg) (bool, error) {
	var a struct {
		Stream     string `json:"stream"`
		Consumer   string `json:"consumer"`
		StreamSeq  uint64 `json:"stream_seq"`
		Deliveries int    `json:"deliveries"`
	}
	meta, err := notice.Metadata()
	if err != nil {
		return false, err
	}
	if notice.Subject() != advisorySubject(stream, name) || json.Unmarshal(notice.Data(), &a) != nil || a.StreamSeq == 0 || a.Stream != stream || a.Consumer != name || a.Deliveries < 1 {
		return false, ErrBadReference
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	stored := false
	err = pgx.BeginFunc(deadline, p, func(tx pgx.Tx) error {
		if err := current(deadline, tx, b.epoch); err != nil {
			return err
		}
		var e Event
		err := tx.QueryRow(deadline, `SELECT `+columns+` FROM event_outbox o WHERE restore_epoch=$1 AND type=$2 AND broker_stream=$3 AND broker_sequence=$4`, b.epoch, consumerTopics[name], stream, int64(a.StreamSeq)).Scan(e.targets()...)
		if errors.Is(err, pgx.ErrNoRows) {
			// PubAck may not yet have been marked in PG. Retry across the whole
			// relay lease. For an unmapped broker position preserve a stable,
			// payload-free receipt; the source's indexed PG work remains eligible.
			if time.Since(meta.Timestamp) < time.Minute {
				return ErrDeferred
			}
			e = Event{Epoch: b.epoch, ID: messageIdentity(b.epoch, name, stream, a.StreamSeq)}
		} else if err != nil {
			return err
		} else {
			var state string
			err = tx.QueryRow(deadline, `SELECT state FROM event_work WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3 FOR UPDATE`, name, b.epoch, e.ID).Scan(&state)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil || state != "awaiting" {
				return err
			}
		}
		stored = true
		return quarantine(deadline, tx, name, e, "delivery_exhausted", a.Deliveries)
	})
	return stored && err == nil, err
}
func consumeEpoch(ctx context.Context, p *pgxpool.Pool, c Config, name, epoch string, work func(context.Context, Event) error, r diagnosticreport.Sink) error {
	run, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	keys := make(chan string, c.MaxAckPending)
	// A bounded scan drains durable unfinished work even if all broker history
	// expired. Separate from the existing domain queues for pre-upgrade jobs.
	scan := time.NewTimer(0)
	defer scan.Stop()
	next := time.Now()
	schedule := func(delay time.Duration) {
		next = time.Now().Add(delay)
		scan.Reset(max(0, delay))
	}
	slots := make(chan struct{}, c.Workers)
	retries := make(chan time.Time, c.Workers)
	var lastClaim time.Time
	var brokerProgress atomic.Int64
	executeInSlot := func(j *job) {
		wg.Go(func() {
			due := execute(run, p, c, *j, work, r)
			<-slots
			if !due.IsZero() {
				// Coalesce retry wakeups. PG still owns every deadline if this
				// process restarts or a busy consumer already has a wakeup.
				select {
				case retries <- due:
				default:
				}
			}
		})
	}
	process := func(id string) (bool, error) {
		select {
		case slots <- struct{}{}:
		case <-run.Done():
			return false, run.Err()
		}
		if id == "" {
			diagnosticreport.Count(r, "event.pg_reconciliations", 1)
		}
		j, err := claimWork(run, p, c, name, epoch, id, r)
		if err != nil || j == nil {
			<-slots
			return false, err
		}
		lastClaim = time.Now()
		executeInSlot(j)
		return true, nil
	}
	if c.Mode == "jetstream" {
		deliver := func(b *broker, m jetstream.Msg) (handoff, error) {
			if !directIngestionClaim(name) {
				return receive(run, p, b, name, m)
			}
			select {
			case slots <- struct{}{}:
			case <-run.Done():
				return handoff{}, run.Err()
			}
			h, err := receiveDelivery(run, p, b, name, m, true)
			if h.leased != nil {
				executeInSlot(h.leased)
			} else {
				<-slots
			}
			if err == nil {
				brokerProgress.Store(time.Now().UnixNano())
			}
			return h, err
		}
		wg.Go(func() { pumpBroker(run, p, c, name, epoch, keys, r, deliver) })
	}
	lane := "events-" + name
	idle := ms(c.FallbackScanMS)
	for {
		select {
		case <-ctx.Done():
			return nil
		case due := <-retries:
			if due.Before(next) {
				schedule(time.Until(due))
			}
		case id := <-keys:
			if _, err := process(id); err != nil {
				return err
			}
		case <-scan.C:
			r.Begin(lane, time.Now().Add(ms(c.WorkTimeoutMS)))
			until := time.Now().Add(250 * time.Millisecond)
			delay := idle
			// Repair a lost key even during continuous broker traffic. While
			// the machine subscription is making progress, reserve the other
			// execution slots for its referenced keys. A stalled subscription
			// returns to the ordinary bounded PG drain after one scan interval.
			healthyDirect := directIngestionClaim(name) && c.Mode == "jetstream" && time.Since(time.Unix(0, brokerProgress.Load())) < idle
			for n := 0; n < c.Batch; n++ {
				claimed, err := process("")
				if err != nil {
					r.End(lane, "dependency_unavailable", time.Now().Add(ms(c.BackoffMS)))
					return err
				}
				if !claimed {
					// In PG mode a preceding stage may be about to commit the
					// next key. Poll briefly after real work, then go fully idle.
					if c.Mode == "pg" && time.Since(lastClaim) < 2*idle {
						delay = min(idle, max(50*time.Millisecond, min(250*time.Millisecond, ms(c.BackoffMS)/2)))
					}
					break
				}
				if healthyDirect {
					break
				}
				if n+1 == c.Batch || time.Now().After(until) {
					delay = 10 * time.Millisecond
					break
				}
			}
			schedule(delay)
			r.End(lane, "", next)
		}
	}
}

// RunConsumer accepts each broker hint after durable PG handoff and executes
// only the referenced domain operation. With no events file the role's original
// domain scan and timers operate until shutdown.
func RunConsumer(ctx context.Context, p *pgxpool.Pool, name string, work func(context.Context, Event) error, reporters ...diagnosticreport.Sink) error {
	if consumerTopics[name] == "" || work == nil {
		return errors.New("unknown consumer or missing domain worker")
	}
	c, err := Load(os.Getenv("INFRAPRO_EVENTS_FILE"))
	if err != nil {
		return err
	}
	if c.Mode == "off" {
		<-ctx.Done()
		return nil
	}
	r := diagnosticreport.Optional(reporters)
	for ctx.Err() == nil {
		state, err := ProtocolState(ctx, p)
		if err != nil {
			r.Dependency("postgres", false, "dependency_unavailable")
			pause(ctx, ms(c.BackoffMS))
			continue
		}
		if !state.Enabled || state.Frozen {
			pause(ctx, ms(c.FallbackScanMS))
			continue
		}
		err = consumeEpoch(ctx, p, c, name, state.Epoch, work, r)
		if err != nil && !errors.Is(err, errEpoch) {
			r.Dependency("postgres", false, "dependency_unavailable")
			pause(ctx, ms(c.BackoffMS))
		}
	}
	return nil
}
