// Package replicabudget admits only one live process per declared deployment
// slot. Its ordinary business connections hold the session locks. No extra PG
// connection or per-request SQL is required.
package replicabudget

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgconn/ctxwatch"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOccupied = errors.New("replica budget slot is occupied")
var ErrLost = errors.New("replica budget ownership lost")

type idleProbeKey struct{}

type Pool struct {
	*pgxpool.Pool
	Context context.Context
	stop    context.CancelCauseFunc
}

func (p *Pool) Close() {
	p.stop(context.Canceled)
	p.Pool.Close()
}

func (p *Pool) CloseResult(result *error) {
	if errors.Is(context.Cause(p.Context), ErrLost) {
		*result = ErrLost
	}
	p.Close()
}

// Requests preserves ordinary graceful shutdown but cancels in-flight work
// immediately when the registered replica loses all its PostgreSQL sessions.
func Requests(owner context.Context) (context.Context, func()) {
	req, cancel := context.WithCancel(context.WithoutCancel(owner))
	stop := context.AfterFunc(owner, func() {
		if errors.Is(context.Cause(owner), ErrLost) {
			cancel()
		}
	})
	return req, func() { stop(); cancel() }
}

func Open(parent context.Context, pc *pgxpool.Config, slot string) (*Pool, error) {
	ctx, stop := context.WithCancelCause(parent)
	guarded := slot != ""
	if guarded {
		// The callbacks and instance marker below belong to this pool alone.
		pc = pc.Copy()
		if pc.MaxConns < 1 {
			stop(context.Canceled)
			return nil, errors.New("replica slot has no PostgreSQL allocation")
		}
		instance := uuid.NewString()
		// An idle owner must retain at least one session. Other idle connections
		// still use the configured idle lifetime. Process-local authority ends
		// if every backend vanishes, including an administrative termination.
		pc.MinConns = min(pc.MaxConns, max(pc.MinConns, 1))
		pc.MinIdleConns = min(pc.MaxConns, pc.MinIdleConns)
		pc.MaxConnLifetime = 0
		// The default pgx watcher immediately times out the TCP read and
		// discards this backend when an HTTP caller cancels a query. Let
		// PostgreSQL cancel the statement and send ReadyForQuery instead.
		// An unresponsive backend still has a bounded cleanup deadline.
		pc.ConnConfig.BuildContextWatcherHandler = func(conn *pgconn.PgConn) ctxwatch.Handler {
			return &pgconn.CancelRequestContextWatcherHandler{Conn: conn, DeadlineDelay: 5 * time.Second}
		}
		// A deadline on pgxpool's checkout ping can evict a session before
		// it is handed to the caller. The slot already watches idle backends
		// and active operations observe any genuine transport failure.
		checkoutPing := pc.ShouldPing
		pc.ShouldPing = func(ctx context.Context, params pgxpool.ShouldPingParams) bool {
			return checkoutPing != nil && ctx.Value(idleProbeKey{}) == nil && checkoutPing(ctx, params)
		}
		beforeConnect := pc.BeforeConnect
		pc.BeforeConnect = func(ctx context.Context, cfg *pgx.ConnConfig) error {
			if ctx.Value(idleProbeKey{}) != nil {
				return context.Canceled
			}
			if beforeConnect != nil {
				return beforeConnect(ctx, cfg)
			}
			return nil
		}
		var mu sync.Mutex
		joined := false
		active := make(map[*pgx.Conn]bool)
		previousConnect, previousClose := pc.AfterConnect, pc.BeforeClose
		pc.AfterConnect = func(check context.Context, conn *pgx.Conn) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if previousConnect != nil {
				if err := previousConnect(check, conn); err != nil {
					return err
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := attach(check, conn, slot, instance, joined); err != nil {
				if errors.Is(err, ErrLost) {
					stop(err)
				}
				return err
			}
			joined = true
			active[conn] = true
			return nil
		}
		pc.BeforeClose = func(conn *pgx.Conn) {
			mu.Lock()
			if active[conn] {
				delete(active, conn)
				if len(active) == 0 {
					stop(ErrLost)
				}
			}
			mu.Unlock()
			if previousClose != nil {
				previousClose(conn)
			}
		}
	}
	p, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		stop(err)
		return nil, err
	}
	out := &Pool{p, ctx, stop}
	if guarded {
		if err = p.Ping(ctx); err != nil {
			out.Close()
			return nil, err
		}
		go out.checkIdleOwner(pc.MaxConns == 1)
	}
	return out, nil
}

// Process death releases every shared lock at PostgreSQL. Newly opening
// backends can share a slot only if the random marker of their own process
// is still present. An unrelated process must obtain the slot exclusively
// first. The transaction lock makes that check and downgrade atomic.
func attach(ctx context.Context, conn *pgx.Conn, slot, instance string, joined bool) error {
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var key, marker int64
		err := tx.QueryRow(ctx, `SELECT hashtextextended('infrapro/slot/'||current_schema()||'/'||$1,0),
			hashtextextended('infrapro/instance/'||$2,0)`, slot, instance).Scan(&key, &marker)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1::bigint # 87420231)", key); err != nil {
			return err
		}
		var ours bool
		err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks
			WHERE locktype='advisory' AND granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database())
			AND objsubid=1 AND classid=($1::bigint >> 32 & 4294967295)::oid AND objid=($1::bigint & 4294967295)::oid)`, marker).Scan(&ours)
		if err != nil {
			return err
		}
		if !ours {
			if joined {
				return ErrLost
			}
			var free bool
			if err = tx.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&free); err != nil {
				return err
			}
			if !free {
				return ErrOccupied
			}
		}
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_lock_shared($1), pg_advisory_lock_shared($2)", key, marker); err != nil {
			return err
		}
		if !ours {
			_, err = tx.Exec(ctx, "SELECT pg_advisory_unlock($1)", key)
		}
		return err
	})
}

func (p *Pool) checkIdleOwner(single bool) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	probe := context.WithValue(p.Context, idleProbeKey{}, true)
	for {
		select {
		case <-p.Context.Done():
			return
		case <-tick.C:
			if p.Stat().IdleConns() == 0 {
				continue
			}
			check, cancel := context.WithTimeout(probe, time.Millisecond)
			var c *pgxpool.Conn
			if single {
				// Never enqueue an idle probe ahead of a one-connection publisher.
				if idle := p.AcquireAllIdle(check); len(idle) == 1 {
					c = idle[0]
				}
			} else {
				// Borrow one existing session. Acquiring and releasing the entire
				// idle set would continually reset every backend's idle lifetime.
				// BeforeConnect prevents a competing checkout from letting this
				// probe open a new physical connection.
				c, _ = p.Acquire(check)
			}
			if c != nil {
				// An idle server sends nothing. A short read can observe an
				// administrative FATAL or EOF without initiating a query.
				// A paused server or scheduler must keep its healthy session.
				conn := c.Conn().PgConn()
				err := conn.Conn().SetReadDeadline(time.Now().Add(time.Millisecond))
				if err == nil {
					// A cancellable context would invoke the SQL cancel watcher on
					// every idle poll. This read uses only its socket read deadline.
					_, err = conn.ReceiveMessage(context.Background())
					reset := conn.Conn().SetReadDeadline(time.Time{})
					if err == nil || pgconn.Timeout(err) {
						err = reset
					}
				}
				if err != nil {
					_ = c.Conn().Close(check)
				}
				c.Release()
			}
			cancel()
		}
	}
}
