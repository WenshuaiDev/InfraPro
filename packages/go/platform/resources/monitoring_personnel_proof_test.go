package resources_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
)

// The browser and monitoring worker create separate PG transactions for
// concurrent derived input reads. Keep both reads open to verify their lock
// mode and the behavior of the real logout operation behind them.
func TestConcurrentPersonnelRawInputReadAndLogout(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "RAW-PROOF")
	point, err := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"name": "测点", "code": "P"})
	must(t, err)
	channel, err := f.write(t, res.Target{Kind: res.Channel, Facility: facility.ID, Point: point.ID}, "create", map[string]any{
		"code": "OBS", "name": "位移", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time",
	})
	must(t, err)
	_, token, _, err := f.env.Service.Login(ctx, "ResourceOwner", password, identity.Meta{})
	must(t, err)
	principal, err := f.env.Service.Authenticate(ctx, token)
	must(t, err)
	actor := f.env.Service.AccessActor(principal)
	service := md.New(f.env.Service.Access())
	scope := md.TenantScope(f.tenant)
	needs := []md.RawNeed{{Channel: channel.ID}}

	work, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	results := make(chan error, 2)
	check := actor.Check
	actor.Check = func(c context.Context, tx pgx.Tx, recent bool) error {
		entered <- struct{}{}
		select {
		case <-release:
		case <-c.Done():
			return c.Err()
		}
		return check(c, tx, recent)
	}
	var readers []pgx.Tx
	var pids []int32
	var pending sync.WaitGroup
	defer func() {
		cancel()
		pending.Wait()
		for _, reader := range readers {
			reader.Rollback(ctx)
		}
	}()
	for range 2 {
		tx, err := f.env.Pool.Begin(work)
		must(t, err)
		readers = append(readers, tx)
		pids = append(pids, int32(tx.Conn().PgConn().PID()))
		pending.Add(1)
		go func() {
			defer pending.Done()
			results <- service.AuthorizeRawInputTx(work, tx, actor, scope, needs, false)
		}()
	}
	for range readers {
		select {
		case <-entered:
		case err := <-results:
			t.Fatalf("reader exited before checking its session: %v", err)
		case <-work.Done():
			t.Fatal(work.Err())
		}
	}
	close(release)
	for range readers {
		select {
		case err := <-results:
			must(t, err)
		case <-work.Done():
			t.Fatal(work.Err())
		}
	}
	// Logout locks the same account before revoking its session. Both read
	// proofs have completed, but the caller still holds their transactions.
	loggedOut := make(chan error, 1)
	go func() { loggedOut <- f.env.Service.Logout(work, token, identity.Meta{}) }()
	for {
		var blocked bool
		err := f.env.Pool.QueryRow(work, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity a
			WHERE a.datname=current_database() AND a.wait_event_type='Lock'
			  AND a.query LIKE '%FOR UPDATE OF a%' AND pg_blocking_pids(a.pid) && $1::int[])`, pids).Scan(&blocked)
		must(t, err)
		if blocked {
			break
		}
		select {
		case err := <-loggedOut:
			t.Fatalf("logout passed an open input proof: %v", err)
		case <-time.After(15 * time.Millisecond):
		case <-work.Done():
			t.Fatal(work.Err())
		}
	}
	for _, reader := range readers {
		must(t, reader.Commit(work))
	}
	select {
	case err := <-loggedOut:
		must(t, err)
	case <-work.Done():
		t.Fatal(work.Err())
	}
	err = pgx.BeginFunc(work, f.env.Pool, func(tx pgx.Tx) error {
		return service.AuthorizeRawInputTx(work, tx, f.env.Service.AccessActor(principal), scope, needs, false)
	})
	if !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("revoked session retained its raw input proof: %v", err)
	}
}
