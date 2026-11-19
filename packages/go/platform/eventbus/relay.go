package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"sync"
	"time"
)

const columns = `o.event_id::text,o.type,o.version,o.scope,o.aggregate_id::text,o.generation_or_position,o.occurred_at,o.restore_epoch::text,COALESCE(o.cause_id::text,''),COALESCE(o.trace_id::text,'')`

func (e *Event) targets() []any {
	return []any{&e.ID, &e.Type, &e.Version, &e.Scope, &e.AggregateID, &e.Generation, &e.OccurredAt, &e.Epoch, &e.Cause, &e.Trace}
}

type publication struct {
	Event
	fence   int64
	attempt int
}

func claimPublications(ctx context.Context, p *pgxpool.Pool, c Config, epoch string) ([]publication, error) {
	rows, err := p.Query(ctx, `WITH active AS MATERIALIZED(SELECT restore_epoch FROM event_protocol WHERE id=1 AND restore_epoch=$1 AND enabled AND NOT frozen FOR SHARE),
 pending AS MATERIALIZED(
   SELECT o.event_id,o.available_at AS due FROM event_outbox o WHERE EXISTS(SELECT FROM active) AND o.restore_epoch=$1 AND o.state='pending' AND o.available_at<=statement_timestamp()
   ORDER BY o.available_at,o.event_id LIMIT $2 FOR UPDATE OF o SKIP LOCKED)
  , expired AS MATERIALIZED(
   SELECT o.event_id,o.lease_until AS due FROM event_outbox o WHERE EXISTS(SELECT FROM active) AND o.restore_epoch=$1 AND o.state='leased' AND o.lease_until<=statement_timestamp()
   ORDER BY o.lease_until,o.event_id LIMIT $2 FOR UPDATE OF o SKIP LOCKED),
 candidates AS(SELECT event_id FROM (TABLE pending UNION ALL TABLE expired) heads ORDER BY due,event_id LIMIT $2)
 UPDATE event_outbox o SET state='leased',fence=fence+1,attempts=attempts+1,lease_until=clock_timestamp()+interval '30 seconds'
 FROM candidates k WHERE o.event_id=k.event_id RETURNING `+columns+`,o.fence,o.attempts`, epoch, c.Batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []publication
	for rows.Next() {
		var v publication
		if err = rows.Scan(append(v.targets(), &v.fence, &v.attempt)...); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func publish(ctx context.Context, b *broker, v publication) (*jetstream.PubAck, error) {
	body, err := json.Marshal(v.Event)
	if err != nil || len(body) > MaxBytes {
		return nil, errors.New("invalid outbox event")
	}
	to, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return b.js.Publish(to, b.subject(v.Type), body, jetstream.WithMsgID(v.ID), jetstream.WithExpectStream(b.stream(family(v.Type))))
}

type published struct {
	item publication
	ack  *jetstream.PubAck
}

// Each row still has its own epoch, fence, lease and durable broker sequence.
// Coalescing confirmations lets a single PG connection keep up without holding
// a database transaction during any network publication.
func markPublications(ctx context.Context, p *pgxpool.Pool, c Config, confirmations []published) (int64, error) {
	ids := make([]string, 0, len(confirmations))
	fences := make([]int64, 0, len(confirmations))
	streams := make([]string, 0, len(confirmations))
	sequences := make([]int64, 0, len(confirmations))
	delays := make([]int64, 0, len(confirmations))
	epoch := confirmations[0].item.Epoch
	for _, result := range confirmations {
		if result.item.Epoch != epoch {
			return 0, errEpoch
		}
		ids = append(ids, result.item.ID)
		fences = append(fences, result.item.fence)
		delays = append(delays, c.backoff(result.item.attempt).Milliseconds())
		var stream string
		var sequence int64
		if result.ack != nil {
			stream, sequence = result.ack.Stream, int64(result.ack.Sequence)
		}
		streams = append(streams, stream)
		sequences = append(sequences, sequence)
	}
	var updated, confirmed int64
	err := p.QueryRow(ctx, `WITH active AS MATERIALIZED(SELECT restore_epoch FROM event_protocol WHERE restore_epoch=$1 AND NOT frozen FOR SHARE),
 result AS (SELECT * FROM unnest($2::text[],$3::bigint[],$4::text[],$5::bigint[],$6::bigint[]) r(id,fence,stream,seq,delay)),
 marked AS (UPDATE event_outbox o SET state=CASE WHEN r.seq>0 THEN 'sent' ELSE 'pending' END,
 last_error=CASE WHEN r.seq>0 THEN '' ELSE 'event_publish_failed' END,broker_stream=NULLIF(r.stream,''),broker_sequence=NULLIF(r.seq,0),
 published_at=CASE WHEN r.seq>0 THEN clock_timestamp() END,lease_until=NULL,
 available_at=CASE WHEN r.seq>0 THEN o.available_at ELSE clock_timestamp()+r.delay*interval '1 millisecond' END
 FROM active, result r WHERE o.event_id=r.id::uuid AND o.restore_epoch=active.restore_epoch AND o.fence=r.fence AND o.state='leased' AND o.lease_until>clock_timestamp()
 RETURNING o.broker_sequence)
 SELECT count(*), count(broker_sequence) FROM marked`, epoch, ids, fences, streams, sequences, delays).Scan(&updated, &confirmed)
	if err == nil && updated != int64(len(confirmations)) {
		err = errEpoch
	}
	return confirmed, err
}

func publishBatch(ctx context.Context, p *pgxpool.Pool, c Config, b *broker, batch []publication) (confirmed int64, failed bool) {
	if len(batch) == 0 {
		return 0, false
	}
	work := make(chan publication, len(batch))
	done := make(chan published, len(batch))
	for _, item := range batch {
		work <- item
	}
	close(work)
	for range min(c.Workers, len(batch)) {
		go func() {
			for item := range work {
				ack, err := publish(ctx, b, item)
				if err != nil {
					ack = nil
				}
				done <- published{item, ack}
			}
		}()
	}
	const maxConfirmations = 32
	results := make([]published, 0, min(maxConfirmations, len(batch)))
	for remaining := len(batch); remaining > 0; {
		results = append(results[:0], <-done)
		limit := min(cap(results), remaining)
		if limit > 1 {
			timer := time.NewTimer(5 * time.Millisecond)
		collect:
			for len(results) < limit {
				select {
				case result := <-done:
					results = append(results, result)
				case <-timer.C:
					break collect
				}
			}
			timer.Stop()
		}
		n, err := markPublications(ctx, p, c, results)
		confirmed += n
		failed = failed || err != nil || n != int64(len(results))
		remaining -= len(results)
	}
	return confirmed, failed
}

func samplePressure(ctx context.Context, p *pgxpool.Pool, epoch string, report diagnosticreport.Sink) error {
	var n, old int64
	err := p.QueryRow(ctx, `WITH sampled AS (
 SELECT f.family,(SELECT o.occurred_at FROM event_outbox o WHERE o.restore_epoch=$1 AND o.state IN ('pending','leased')
 AND CASE WHEN split_part(o.type,'.',1) IN ('monitoring','ingestion') THEN split_part(o.type,'.',1) ELSE 'auxiliary' END=f.family ORDER BY o.occurred_at LIMIT 1) AS oldest FROM event_pressure f),
 updated AS (UPDATE event_pressure m SET oldest_at=s.oldest,sampled_at=clock_timestamp() FROM sampled s WHERE m.family=s.family RETURNING m.pending,m.oldest_at)
 SELECT COALESCE(sum(pending),0)::bigint,COALESCE(extract(epoch FROM clock_timestamp()-min(oldest_at))*1000,0)::bigint FROM updated`, epoch).Scan(&n, &old)
	if err == nil {
		diagnosticreport.SetMetric(report, "event.outbox_pending", n)
		diagnosticreport.SetMetric(report, "event.outbox_oldest_ms", max(old, 0))
	}
	return err
}

// The relay may read the capacity metadata of its three streams. It has no
// permission to read stored messages or change stream limits.
func sampleDisk(ctx context.Context, p *pgxpool.Pool, b *broker) error {
	for _, f := range families {
		st, err := b.js.Stream(ctx, b.stream(f))
		if err != nil {
			return err
		}
		info := st.CachedInfo()
		if info == nil || info.Config.MaxBytes < 1 {
			return errors.New("missing stream capacity")
		}
		used, limit := int64(info.State.Bytes), info.Config.MaxBytes
		level := "normal"
		switch {
		case used*100 >= limit*int64(b.cfg.DiskHardPercent):
			level = "hard"
		case used*100 >= limit*int64(b.cfg.DiskSoftPercent):
			level = "soft"
		}
		_, err = p.Exec(ctx, `UPDATE event_pressure m SET broker_bytes=$3,broker_limit_bytes=$4,storage_level=$5,broker_sampled_at=clock_timestamp()
         FROM event_protocol ep WHERE ep.restore_epoch=$1 AND NOT ep.frozen AND m.family=$2`, b.epoch, f, used, limit, level)
		if err != nil {
			return err
		}
	}
	return nil
}
func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Empty, schema-specific PG notifications only shorten the relay's idle wait.
// A connection restart is handled by the indexed polling path.
func watchOutbox(ctx context.Context, p *pgxpool.Pool, wake chan<- struct{}) {
	for ctx.Err() == nil {
		conn, err := p.Acquire(ctx)
		if err != nil {
			pause(ctx, time.Second)
			continue
		}
		func() {
			defer conn.Release()
			var channel string
			if conn.QueryRow(ctx, "SELECT 'infrapro_events_'||md5(current_schema())").Scan(&channel) != nil {
				return
			}
			if _, err = conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
				return
			}
			defer func() {
				clean, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, _ = conn.Exec(clean, "UNLISTEN *")
			}()
			for ctx.Err() == nil {
				if _, err = conn.Conn().WaitForNotification(ctx); err != nil {
					return
				}
				select {
				case wake <- struct{}{}:
				default:
				}
			}
		}()
		pause(ctx, time.Second)
	}
}

// RunRelay lets each PG lease transaction finish before publishing on the wire.
// PubAck without a completed PG mark is retried with the same message identity.
func RunRelay(ctx context.Context, p *pgxpool.Pool, c Config, report diagnosticreport.Sink) error {
	if c.Mode == "off" {
		return errors.New("event relay needs an events file")
	}
	wake := make(chan struct{}, 1)
	watcher, stop := context.WithCancel(ctx)
	var watchers sync.WaitGroup
	// LISTEN keeps its connection until shutdown. Deployments with one
	// connection still publish through the indexed, bounded polling path.
	if c.Mode == "jetstream" && p.Config().MaxConns > 1 {
		watchers.Go(func() { watchOutbox(watcher, p, wake) })
	}
	defer func() { stop(); watchers.Wait() }()
	var b *broker
	defer func() {
		if b != nil {
			b.nc.Close()
		}
	}()
	var nextSample, nextPrune, nextDiskSample time.Time
	var tenants tenantQueueSample
	reporter, _ := report.(*diagnosticreport.Reporter)
	for ctx.Err() == nil {
		state, err := ProtocolState(ctx, p)
		if err != nil {
			report.Dependency("postgres", false, "dependency_unavailable")
			pause(ctx, ms(c.BackoffMS))
			continue
		}
		if !state.Enabled || state.Frozen {
			if tenants.epoch != "" {
				diagnosticreport.SetTenantQueue(reporter, nil)
				tenants = tenantQueueSample{}
			}
			pause(ctx, ms(c.FallbackScanMS))
			continue
		}
		if time.Now().After(nextSample) {
			err = samplePressure(ctx, p, state.Epoch, report)
			report.Dependency("postgres", err == nil, diagnosticreport.Result(err))
			nextSample = time.Now().Add(time.Second)
		}
		if reporter != nil && (tenants.epoch != state.Epoch || time.Now().After(tenants.next)) {
			tenants.collect(ctx, p, state.Epoch, reporter)
		}
		if c.Mode == "pg" {
			pause(ctx, ms(c.FallbackScanMS))
			continue
		}
		if time.Now().After(nextPrune) {
			_, _ = Prune(ctx, p, time.Duration(c.RetentionSeconds)*time.Second, 256)
			nextPrune = time.Now().Add(time.Minute)
		}
		if b == nil || b.epoch != state.Epoch {
			if b != nil {
				b.nc.Close()
			}
			b, err = connect(c, state.Epoch, "relay")
			if err != nil {
				report.Dependency("jetstream", false, "broker_unavailable")
				pause(ctx, c.backoff(4))
				continue
			}
		}
		if time.Now().After(nextDiskSample) {
			check, cancel := context.WithTimeout(ctx, 2*time.Second)
			if err := sampleDisk(check, p, b); err != nil {
				report.Dependency("jetstream", false, "broker_unavailable")
			}
			cancel()
			nextDiskSample = time.Now().Add(5 * time.Second)
		}
		report.Begin("publish", time.Now().Add(10*time.Second))
		batch, err := claimPublications(ctx, p, c, state.Epoch)
		if err != nil {
			report.End("publish", "dependency_unavailable", time.Now().Add(ms(c.BackoffMS)))
			pause(ctx, ms(c.BackoffMS))
			continue
		}
		confirmed, failures := publishBatch(ctx, p, c, b, batch)
		if confirmed > 0 {
			diagnosticreport.Count(report, "event.publish_acks", confirmed)
		}
		delay := ms(c.PollMS)
		if failures {
			report.Dependency("jetstream", false, "event_publish_failed")
			report.End("publish", "event_publish_failed", time.Now().Add(ms(c.BackoffMS)))
			delay = ms(c.BackoffMS)
		} else {
			if len(batch) > 0 {
				report.Dependency("jetstream", true, "")
				delay = 0
			}
			report.End("publish", "", time.Now().Add(delay))
		}
		if delay > 0 {
			first := time.NewTimer(delay)
			select {
			case <-ctx.Done():
			case <-wake:
			case <-first.C:
			}
			first.Stop()
		}
	}
	return nil
}

func canonical(ctx context.Context, tx pgx.Tx, epoch, id string) (Event, error) {
	var e Event
	err := tx.QueryRow(ctx, `SELECT `+columns+` FROM event_outbox o WHERE restore_epoch=$1 AND event_id=$2`, epoch, id).Scan(e.targets()...)
	return e, err
}
