package access

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A clock-triggered action may span several short transactions. Holding a
// session advisory lock keeps one active scheduler without holding a row lock
// across those transactions. Reusing its connection also supports a pool of one.
// The callback must run its transactions sequentially on the supplied context.
type schedulerConnection struct{}
type scheduleSession struct {
	svc  *Service
	conn *pgxpool.Conn
}

func (s *Service) beginTx(ctx context.Context) (pgx.Tx, error) {
	// BeginTx still owns the pgx transaction and pool release. PostgreSQL runs
	// these session-local defaults inside BEGIN, so commit/rollback resets them.
	options := pgx.TxOptions{BeginQuery: "BEGIN; SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='10s'"}
	if v, ok := ctx.Value(schedulerConnection{}).(scheduleSession); ok && v.svc == s {
		return v.conn.BeginTx(ctx, options)
	}
	return s.pool.BeginTx(ctx, options)
}
func (s *Service) RunScheduled(ctx context.Context, lane string, period time.Duration, fn func(context.Context) error) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	const lock = "infrapro/schedule/"
	var locked bool
	err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, lock+lane).Scan(&locked)
	if err != nil || !locked {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var unlocked bool
		if err := conn.QueryRow(cleanup, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, lock+lane).Scan(&unlocked); err != nil || !unlocked {
			_ = conn.Conn().Close(cleanup)
		}
	}()
	ctx = context.WithValue(ctx, schedulerConnection{}, scheduleSession{s, conn})
	var due bool
	err = s.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `WITH claimed AS (
		INSERT INTO monitoring_scheduler_leases(lane,next_run) VALUES($1,clock_timestamp()+$2 * interval '1 millisecond')
		ON CONFLICT(lane) DO UPDATE SET next_run=EXCLUDED.next_run WHERE monitoring_scheduler_leases.next_run<=clock_timestamp()
		RETURNING 1) SELECT EXISTS(SELECT 1 FROM claimed)`, lane, period.Milliseconds()).Scan(&due)
	})
	if err != nil || !due {
		return err
	}
	return fn(ctx)
}
