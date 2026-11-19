package parsing

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestArchitectureMultipleDispatcherParserBudget(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("isolated PG fixture required")
	}
	ctx := context.Background()
	env, err := testsupport.New(ctx, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	var replicas []*Service
	for range 3 {
		pool, e := pgxpool.New(ctx, env.Config.DatabaseURL)
		if e != nil {
			t.Fatal(e)
		}
		defer pool.Close()
		replicas = append(replicas, New(access.New(pool, slog.Default())))
	}
	acquire := func(replica int, scope string, automatic bool) (int, string) {
		t.Helper()
		id := uuid.NewString()
		slot, e := replicas[replica].machineSlot(ctx, machineWork{scope, id, automatic}, time.Second)
		if e != nil {
			t.Fatal(e)
		}
		return slot, id
	}
	s1, id1 := acquire(0, "tenant:first", true)
	s2, _ := acquire(1, "tenant:first", true)
	if _, e := replicas[2].machineSlot(ctx, machineWork{"tenant:first", uuid.NewString(), true}, 150*time.Millisecond); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("tenant cap did not hold across pools", e)
	}
	s3, _ := acquire(2, "tenant:second", true)
	if _, e := replicas[1].machineSlot(ctx, machineWork{"tenant:third", uuid.NewString(), true}, 150*time.Millisecond); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("automatic calls consumed manual reservation", e)
	}
	s4, _ := acquire(2, "tenant:first", false)
	if len(map[int]bool{s1: true, s2: true, s3: true, s4: true}) != ExecutorSlots || s4 != 4 {
		t.Fatal("incorrect aggregate slot assignment", s1, s2, s3, s4)
	}
	if _, e := replicas[0].machineSlot(ctx, machineWork{"platform", uuid.NewString(), false}, 100*time.Millisecond); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("exceeded global cap", e)
	}
	if _, e := env.Pool.Exec(ctx, `UPDATE parsing_executor_slots SET lease_until=clock_timestamp()-interval '1 second' WHERE slot=$1`, s1); e != nil {
		t.Fatal(e)
	}
	reclaimed, newID := acquire(2, "tenant:second", true)
	if reclaimed != s1 {
		t.Fatal("expired slot not recovered")
	}
	replicas[0].releaseMachineSlot(ctx, s1, id1)
	var live string
	if e := env.Pool.QueryRow(ctx, `SELECT execution_id::text FROM parsing_executor_slots WHERE slot=$1`, s1).Scan(&live); e != nil || live != newID {
		t.Fatal("late release cleared successor", e)
	}
}

func TestArchitectureLocalParserReservation(t *testing.T) {
	var queue localMachineAdmission
	enter := func(tenant string, automatic bool) func() {
		t.Helper()
		free, err := queue.enter(context.Background(), machineWork{scope: tenant, automatic: automatic})
		if err != nil {
			t.Fatal(err)
		}
		return free
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	blocked := func(tenant string, automatic bool) {
		t.Helper()
		free, err := queue.enter(cancelled, machineWork{scope: tenant, automatic: automatic})
		if err == nil {
			free()
			t.Fatal("admitted work past its allocation", tenant, automatic)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	first := enter("one", true)
	second := enter("one", true)
	blocked("one", true)
	third := enter("two", true)
	blocked("three", true)
	manual := enter("one", false)
	blocked("two", false)
	first()
	replacement := enter("one", true)
	replacement()
	second()
	third()
	manual()
	if queue.total != 0 || queue.automatic != 0 || len(queue.tenants) != 0 {
		t.Fatal("local slots not returned")
	}
}
