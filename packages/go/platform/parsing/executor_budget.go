package parsing

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// MachineWork carries the fixed, server-selected budget scope to the HTTP
// adapter. A parser client or a user-supplied request cannot choose it.
type machineWork struct {
	scope, id string
	automatic bool
}
type machineWorkKey struct{}

func MachineWork(ctx context.Context, scope, id string, automatic bool) context.Context {
	return context.WithValue(ctx, machineWorkKey{}, machineWork{scope, id, automatic})
}

const ExecutorSlots = 4

// Coalesce same-process waiters before they reach the shared slot table.
// The PG lease remains authoritative across replicas, including after death.
type localMachineAdmission struct {
	mu               sync.Mutex
	total, automatic int
	tenants          map[string]int
	released         chan struct{}
}

func (a *localMachineAdmission) enter(ctx context.Context, w machineWork) (func(), error) {
	for {
		a.mu.Lock()
		if a.total < ExecutorSlots && (!w.automatic || a.automatic < ExecutorSlots-1 && a.tenants[w.scope] < 2) {
			a.total++
			if w.automatic {
				a.automatic++
				if a.tenants == nil {
					a.tenants = make(map[string]int)
				}
				a.tenants[w.scope]++
			}
			a.mu.Unlock()
			return func() {
				a.mu.Lock()
				a.total--
				if w.automatic {
					a.automatic--
					a.tenants[w.scope]--
					if a.tenants[w.scope] == 0 {
						delete(a.tenants, w.scope)
					}
				}
				if a.released != nil {
					close(a.released)
					a.released = nil
				}
				a.mu.Unlock()
			}, nil
		}
		if a.released == nil {
			a.released = make(chan struct{})
		}
		wait := a.released
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}
}

// Slots are held for just the machine call, independently of job leases. Slot
// four is available to personnel requests even with several dispatchers and
// parsers. The existing automatic 4/tenant 2 job-claim limits remain in force.
func (s *Service) machineSlot(ctx context.Context, w machineWork, timeout time.Duration) (int, error) {
	wait, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		var slot int
		err := s.access.Transact(wait, func(tx pgx.Tx) error {
			// The second statement sees any reservation committed by the previous
			// lock owner. Both statements can travel on the same PG round trip.
			var reserve pgx.Batch
			reserve.Queue(`SELECT pg_advisory_xact_lock(754049)`)
			reserve.Queue(`UPDATE parsing_executor_slots SET scope=$1,execution_id=$2,automatic=$3,lease_until=clock_timestamp()+$4*interval '1 millisecond'
			    WHERE slot=(SELECT slot FROM parsing_executor_slots WHERE (lease_until IS NULL OR lease_until<=clock_timestamp())
			       AND (NOT $3 OR slot<4)
			       AND (NOT $3 OR (SELECT count(*) FROM parsing_executor_slots WHERE automatic AND scope=$1 AND lease_until>clock_timestamp())<2)
			       ORDER BY slot DESC LIMIT 1) RETURNING slot`, w.scope, w.id, w.automatic, (timeout + 2*time.Second).Milliseconds()).
				QueryRow(func(row pgx.Row) error {
					err := row.Scan(&slot)
					if err == pgx.ErrNoRows {
						return nil
					}
					return err
				})
			return tx.SendBatch(wait, &reserve).Close()
		})
		if err != nil {
			return 0, err
		}
		if slot > 0 {
			return slot, nil
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-timer.C:
		case <-wait.Done():
			timer.Stop()
			return 0, wait.Err()
		}
	}
}
func (s *Service) releaseMachineSlot(ctx context.Context, slot int, id string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_ = s.access.Transact(cleanup, func(tx pgx.Tx) error {
		_, err := tx.Exec(cleanup, `UPDATE parsing_executor_slots SET scope=NULL,execution_id=NULL,automatic=NULL,lease_until=NULL WHERE slot=$1 AND execution_id=$2`, slot, id)
		return err
	})
}
