// Package dispatch fences worker ownership across compatible topologies. Job
// leases retain their domain semantics; the longer-lived process ticket cannot
// authorize a commit made after its topology was retired.
package dispatch

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRetired = errors.New("dispatch ownership expired")

type ticket struct {
	role, mode, instance, epoch string
	generation                  int64
}
type ticketKey struct{}

const ttl = 30 * time.Second

// CheckTx takes the shared ownership lock at commit, after any transaction
// isolation options have been set by the caller. An exclusive cutover waits
// for this commit or makes it fail. No lock survives the transaction.
func CheckTx(ctx context.Context, tx pgx.Tx) error {
	return checkTx(ctx, tx, "")
}

// CheckEventEpoch also requires active capture in the specified namespace. A
// single lock query covers the protocol, process ticket and event-work ticket.
// Domain commits without tickets keep their original zero-query path.
func CheckEventEpoch(ctx context.Context, tx pgx.Tx, epoch string) error {
	if epoch == "" {
		return ErrRetired
	}
	return checkTx(ctx, tx, epoch)
}

func checkTx(ctx context.Context, tx pgx.Tx, epoch string) error {
	t, owned := ctx.Value(ticketKey{}).(ticket)
	w, working := ctx.Value(workKey{}).(workTicket)
	events := epoch != "" || working
	if epoch == "" {
		switch {
		case owned:
			epoch = t.epoch
		case working:
			epoch = w.epoch
		default:
			return nil
		}
	}
	if owned && t.epoch != epoch || working && w.epoch != epoch {
		return ErrRetired
	}
	args := pgx.NamedArgs{"epoch": epoch}
	query := "SELECT true FROM event_protocol p"
	locks := "p"
	if owned {
		query += ` JOIN dispatch_ownership o ON o.restore_epoch=p.restore_epoch
  AND o.partition='all' AND o.role=@role AND o.mode=@mode AND o.generation=@generation
 JOIN dispatch_leases l ON l.role=o.role AND l.partition=o.partition AND l.restore_epoch=o.restore_epoch
  AND l.generation=o.generation AND l.instance=@instance AND l.lease_until>clock_timestamp()`
		locks += ",o,l"
		args["role"], args["mode"], args["generation"], args["instance"] = t.role, t.mode, t.generation, t.instance
	}
	if working {
		query += ` JOIN event_work w ON w.restore_epoch=p.restore_epoch AND w.consumer=@consumer
  AND w.event_id=@event AND w.fence=@fence AND w.state='running' AND w.lease_until>clock_timestamp()`
		locks += ",w"
		args["consumer"], args["event"], args["fence"] = w.consumer, w.event, w.fence
	}
	query += " WHERE p.id=1 AND p.restore_epoch=@epoch AND NOT p.frozen"
	if events {
		query += " AND p.enabled"
	}
	var live bool
	err := tx.QueryRow(ctx, query+" FOR SHARE OF "+locks, args).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRetired
	}
	return err
}

func acquire(ctx context.Context, pool *pgxpool.Pool, role, mode, instance string) (ticket, error) {
	t := ticket{role: role, mode: mode, instance: instance}
	e := pool.QueryRow(ctx, `INSERT INTO dispatch_leases(role,partition,instance,generation,restore_epoch,lease_until)
 SELECT o.role,o.partition,$3,o.generation,o.restore_epoch,clock_timestamp()+$4*interval '1 millisecond'
 FROM dispatch_ownership o JOIN event_protocol p USING(restore_epoch)
 WHERE role=$1 AND partition='all' AND mode=$2 AND NOT p.frozen FOR SHARE OF o,p
 ON CONFLICT(role,partition,instance) DO UPDATE SET generation=EXCLUDED.generation,restore_epoch=EXCLUDED.restore_epoch,lease_until=EXCLUDED.lease_until
 RETURNING generation,restore_epoch::text`, role, mode, instance, ttl.Milliseconds()).Scan(&t.generation, &t.epoch)
	return t, e
}
func runLease(ctx context.Context, pool *pgxpool.Pool, t ticket, run func(context.Context) error) error {
	work, cancel := context.WithCancel(context.WithValue(ctx, ticketKey{}, t))
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(work) }()
	tick := time.NewTicker(ttl / 3)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			cancel()
			<-done
			return nil
		case err := <-done:
			return err
		case <-tick.C:
			renew, finish := context.WithTimeout(ctx, ttl/4)
			tag, err := pool.Exec(renew, `UPDATE dispatch_leases l SET lease_until=clock_timestamp()+$6*interval '1 millisecond'
    FROM dispatch_ownership o,event_protocol p WHERE l.instance=$1 AND l.role=$2 AND l.partition='all'
    AND l.lease_until>clock_timestamp() AND o.role=l.role AND o.partition=l.partition AND o.mode=$3 AND o.generation=$4
    AND o.restore_epoch=$5 AND l.generation=o.generation AND p.restore_epoch=o.restore_epoch AND NOT p.frozen`, t.instance, t.role, t.mode, t.generation, t.epoch, ttl.Milliseconds())
			finish()
			if err != nil || tag.RowsAffected() != 1 {
				cancel()
				<-done
				return ErrRetired
			}
		}
	}
}

// Run waits until the operational grant selects mode, renews it while run is
// active, and drains its goroutines before acquiring a new generation.
func Run(ctx context.Context, pool *pgxpool.Pool, role, mode string, run func(context.Context) error) error {
	instance := uuid.NewString()
	for ctx.Err() == nil {
		t, err := acquire(ctx, pool, role, mode, instance)
		if err == nil {
			err = runLease(ctx, pool, t, run)
			cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			_, _ = pool.Exec(cleanup, `DELETE FROM dispatch_leases WHERE role=$1 AND partition='all' AND instance=$2 AND generation=$3`, role, instance, t.generation)
			done()
			if !errors.Is(err, ErrRetired) {
				return err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
	return nil
}

// Try runs a single inline CLI/interactive attempt only if the role currently
// owns this topology. The same heartbeat and commit fence apply to the attempt.
func Try(ctx context.Context, pool *pgxpool.Pool, role, mode string, run func(context.Context) error) (bool, error) {
	t, err := acquire(ctx, pool, role, mode, uuid.NewString())
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer done()
		_, _ = pool.Exec(cleanup, `DELETE FROM dispatch_leases WHERE role=$1 AND partition='all' AND instance=$2 AND generation=$3`, t.role, t.instance, t.generation)
	}()
	return true, runLease(ctx, pool, t, run)
}

// WithEventWork binds a domain transaction to the durable wakeup it is
// executing. This also fences auxiliary consumers without a topology lease.
type workKey struct{}
type workTicket struct {
	consumer, epoch, event string
	fence                  int64
}

func WithEventWork(ctx context.Context, consumer, epoch, event string, fence int64) context.Context {
	return context.WithValue(ctx, workKey{}, workTicket{consumer, epoch, event, fence})
}
