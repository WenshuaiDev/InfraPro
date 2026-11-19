package config_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/replicabudget"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReplicaBudgetBrowserCancellationPreservesOwnership(t *testing.T) {
	dsn := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("requires isolated test database")
	}
	admin, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "http-cancel-" + uuid.NewString()
	pc.ConnConfig.RuntimeParams["application_name"] = name
	pc.MaxConns = 12
	pool, err := replicabudget.Open(t.Context(), pc, name)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	svc := access.New(pool.Pool, slog.New(slog.DiscardHandler))
	const requests = 3
	finished := make(chan error, requests)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" {
			if context.Cause(pool.Context) != nil || pool.Ping(r.Context()) != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			return
		}
		finished <- svc.Transact(r.Context(), func(tx pgx.Tx) error {
			_, err := tx.Exec(r.Context(), "SELECT pg_sleep(20)")
			return err
		})
	}))
	defer server.Close()
	var cancels []context.CancelFunc
	for range requests {
		ctx, cancel := context.WithCancel(t.Context())
		cancels = append(cancels, cancel)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/work", nil)
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			if resp, err := server.Client().Do(req); err == nil {
				resp.Body.Close()
			}
		}()
	}
	backends := func(working bool) []int32 {
		t.Helper()
		var pids []int32
		err := admin.QueryRow(t.Context(), `SELECT COALESCE(array_agg(pid ORDER BY pid),'{}'::int[])
			FROM pg_stat_activity WHERE datname=current_database() AND application_name=$1
			AND (NOT $2 OR wait_event='PgSleep')`, name, working).Scan(&pids)
		if err != nil {
			t.Fatal(err)
		}
		return pids
	}
	var original []int32
	for until := time.Now().Add(7 * time.Second); time.Now().Before(until); {
		original = backends(true)
		if len(original) == requests {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(original) != requests {
		t.Fatal("three HTTP handlers did not start real PG work", original)
	}
	allBefore := backends(false)
	t.Log("three real HTTP transactions ready on PostgreSQL backends", original, "all allocated", allBefore)
	for _, cancel := range cancels {
		cancel()
	}
	for range requests {
		select {
		case err := <-finished:
			if err == nil {
				t.Fatal("canceled transaction completed successfully")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("canceled transaction did not finish")
		}
	}
	response, err := server.Client().Get(server.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("three canceled browser requests returned readiness %d: %v", response.StatusCode, context.Cause(pool.Context))
	}
	if after := backends(false); !slices.Equal(after, allBefore) {
		t.Fatalf("healthy PostgreSQL sessions were replaced: %v -> %v", allBefore, after)
	}
	other, err := replicabudget.Open(t.Context(), pc, name)
	if other != nil {
		other.Close()
	}
	if !errors.Is(err, replicabudget.ErrOccupied) {
		t.Fatal("second instance acquired the surviving slot", err)
	}
	for _, pid := range allBefore {
		if _, err := admin.Exec(t.Context(), "SELECT pg_terminate_backend($1)", pid); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-pool.Context.Done():
		if !errors.Is(context.Cause(pool.Context), replicabudget.ErrLost) {
			t.Fatal(context.Cause(pool.Context))
		}
	case <-time.After(9 * time.Second):
		t.Fatal("administrative termination of all backends did not fence the owner")
	}
	t.Log("normal cancellation kept all three backend PIDs and the slot; explicit total backend termination fenced it")
}

type heldNetwork struct {
	net.Conn
	hold, attempted *atomic.Bool
}

type delayedReply struct {
	enabled       atomic.Bool
	local         string
	held, release chan struct{}
}

type delayedReadConn struct {
	net.Conn
	reply *delayedReply
}

func (c delayedReadConn) Read(data []byte) (int, error) {
	n, err := c.Conn.Read(data)
	if n > 0 && c.reply.enabled.Load() && c.LocalAddr().String() == c.reply.local {
		select {
		case c.reply.held <- struct{}{}:
		default:
		}
		<-c.reply.release
	}
	return n, err
}

func TestReplicaBudgetDelayedCancelPreservesBackend(t *testing.T) {
	dsn := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("requires isolated test database")
	}
	admin, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pc.MaxConns = 1
	reply := &delayedReply{held: make(chan struct{}, 1), release: make(chan struct{})}
	dial := pc.ConnConfig.DialFunc
	pc.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		c, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return delayedReadConn{c, reply}, nil
	}
	slot, err := replicabudget.Open(t.Context(), pc, "cancel-latency/"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	defer slot.Close()
	released := false
	defer func() {
		if !released {
			close(reply.release)
		}
	}()
	conn, err := slot.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	reply.local = conn.Conn().PgConn().Conn().LocalAddr().String()
	pid := int32(conn.Conn().PgConn().PID())
	conn.Release()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := slot.Exec(ctx, "SELECT pg_sleep(20)"); done <- err }()
	working := false
	for until := time.Now().Add(4 * time.Second); time.Now().Before(until); {
		err = admin.QueryRow(t.Context(), "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event='PgSleep')", pid).Scan(&working)
		if err != nil {
			t.Fatal(err)
		}
		if working {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !working {
		t.Fatal("the cancellation target did not begin work")
	}
	reply.enabled.Store(true)
	cancel()
	select {
	case <-reply.held:
	case <-time.After(3 * time.Second):
		t.Fatal("the server cancellation reply never arrived")
	}
	select {
	case <-slot.Context.Done():
		t.Fatal("a delayed cancellation reply fenced a live backend")
	case <-time.After(1250 * time.Millisecond):
	}
	close(reply.release)
	released = true
	var pgErr *pgconn.PgError
	select {
	case err = <-done:
		if !errors.As(err, &pgErr) || pgErr.Code != "57014" {
			t.Fatal("real PG query cancellation was not observed", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("query did not release its connection")
	}
	var after int32
	if err := slot.QueryRow(t.Context(), "SELECT pg_backend_pid()").Scan(&after); err != nil || after != pid {
		t.Fatal("the 1.25s delayed acknowledgement replaced its backend", pid, after, err)
	}
	t.Logf("PostgreSQL cancellation %s arrived after 1.25s; transaction resumed on backend %d", pgErr.Code, after)
}

type countingNetwork struct {
	net.Conn
	observe *atomic.Bool
	writes  *atomic.Int64
}

func (c countingNetwork) Write(data []byte) (int, error) {
	if c.observe.Load() {
		c.writes.Add(1)
	}
	return c.Conn.Write(data)
}

func TestReplicaBudgetIdleCheckoutAndShrink(t *testing.T) {
	dsn := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("requires isolated test database")
	}
	for _, guarded := range []bool{false, true} {
		label := "ordinary-pgx-default"
		if guarded {
			label = "registered-slot"
		}
		t.Run(label, func(t *testing.T) {
			pc, err := pgxpool.ParseConfig(dsn)
			if err != nil {
				t.Fatal(err)
			}
			const count = 3
			pc.MaxConns = count
			pc.HealthCheckPeriod = 100 * time.Millisecond
			pc.MaxConnIdleTime = 2500 * time.Millisecond
			var observe atomic.Bool
			var writes atomic.Int64
			dial := pc.ConnConfig.DialFunc
			pc.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
				c, err := dial(ctx, network, address)
				if err != nil {
					return nil, err
				}
				return countingNetwork{c, &observe, &writes}, nil
			}
			key := ""
			if guarded {
				key = "idle-checkout/" + uuid.NewString()
			}
			pool, err := replicabudget.Open(t.Context(), pc, key)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			var opened []*pgxpool.Conn
			for range count {
				conn, err := pool.Acquire(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				opened = append(opened, conn)
			}
			for _, conn := range opened {
				conn.Release()
			}
			time.Sleep(1100 * time.Millisecond)
			observe.Store(true)
			conn, err := pool.Acquire(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			conn.Release()
			observe.Store(false)
			t.Logf("acquiring a connection idle for over 1s produced %d protocol writes", writes.Load())
			if !guarded {
				if writes.Load() == 0 {
					t.Fatal("the default idle ping was not exercised")
				}
				return
			}
			if writes.Load() != 0 {
				t.Fatal("a normal checkout issued an extra protocol request")
			}
			for until := time.Now().Add(6500 * time.Millisecond); time.Now().Before(until) && pool.Stat().TotalConns() != 1; {
				time.Sleep(100 * time.Millisecond)
			}
			if pool.Stat().TotalConns() != 1 || pool.Stat().MaxIdleDestroyCount() != 2 {
				t.Fatalf("three available backends did not shrink to their one owner: %d active, %d retired", pool.Stat().TotalConns(), pool.Stat().MaxIdleDestroyCount())
			}
			if err := pool.Ping(t.Context()); err != nil || pool.Context.Err() != nil {
				t.Fatal("idle reclamation interrupted the surviving owner", err)
			}
		})
	}
}

func (c heldNetwork) Write(data []byte) (int, error) {
	if c.hold.Load() {
		c.attempted.Store(true)
		// A stalled TCP path may accept a local write before any reply is
		// delivered. The real database session remains open behind it.
		return len(data), nil
	}
	return c.Conn.Write(data)
}

func TestReplicaBudgetIdleStallPreservesSession(t *testing.T) {
	dsn := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("requires isolated test database")
	}
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pc.MaxConns = 1
	var hold, attempted atomic.Bool
	dial := pc.ConnConfig.DialFunc
	pc.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		c, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return heldNetwork{c, &hold, &attempted}, nil
	}
	slot, err := replicabudget.Open(t.Context(), pc, "idle-stall/"+uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	defer slot.Close()
	pid := func() int32 {
		var backend int32
		if err := slot.QueryRow(t.Context(), "SELECT pg_backend_pid()").Scan(&backend); err != nil {
			t.Fatal(err)
		}
		return backend
	}
	before := pid()
	hold.Store(true)
	select {
	case <-slot.Context.Done():
		t.Fatal("idle stall fenced an existing database session")
	case <-time.After(4*time.Second + 250*time.Millisecond):
	}
	hold.Store(false)
	if attempted.Load() || before != pid() {
		t.Fatal("idle guard sent a query or replaced a healthy backend")
	}
}

func workerPool(t *testing.T, dsn string) (*replicabudget.Pool, error) {
	t.Helper()
	spec, err := config.WorkerArchitecture(config.Control)
	if err != nil {
		t.Fatal(err)
	}
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pc.MaxConns = spec.Connections(config.Control)
	return replicabudget.Open(t.Context(), pc, spec.SlotKey())
}

func TestReplicaBudgetHolder(t *testing.T) {
	if os.Getenv("INFRAPRO_SLOT_TEST_CHILD") != "1" {
		return
	}
	slot, err := workerPool(t, os.Getenv("INFRAPRO_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer slot.Close()
	conns := make([]*pgxpool.Conn, 0, slot.Config().MaxConns)
	for range slot.Config().MaxConns {
		c, err := slot.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	for _, c := range conns {
		c.Release()
	}
	if err := os.WriteFile(os.Getenv("INFRAPRO_SLOT_TEST_READY"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	<-slot.Context.Done()
}

func TestReplicaBudgetRejectsDuplicateAndRecovers(t *testing.T) {
	dsn := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("requires isolated test database")
	}
	admin, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "quota_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA "+schema)
	q := u.Query()
	q.Set("search_path", schema)
	q.Set("application_name", schema)
	u.RawQuery = q.Encode()
	dsn = u.String()
	directory := t.TempDir()
	manifest := filepath.Join(directory, "architecture.json")
	ready := filepath.Join(directory, "ready")
	endpoints := map[string]string{}
	for _, owner := range config.Owners {
		endpoints[owner] = "https://127.0.0.1:65534"
	}
	data, _ := json.Marshal(map[string]any{"endpoints": endpoints, "cert_directory": directory,
		"replica_slots":  map[string][]string{config.Control: {"control-api-1", "control-api-2"}},
		"pg_connections": map[string]int{config.Control: 3}, "max_inflight": map[string]int{config.Control: 5}})
	if err = os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INFRAPRO_ARCHITECTURE_FILE", manifest)
	t.Setenv("INFRAPRO_REPLICA_ID", "control-api-1")
	child := exec.Command(os.Args[0], "-test.run=^TestReplicaBudgetHolder$")
	child.Env = append(os.Environ(), "INFRAPRO_SLOT_TEST_CHILD=1", "INFRAPRO_TEST_DATABASE_URL="+dsn, "INFRAPRO_SLOT_TEST_READY="+ready)
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	timeout := time.After(15 * time.Second)
	for {
		if _, err = os.Stat(ready); err == nil {
			break
		}
		select {
		case <-timeout:
			t.Fatal("first process did not admit its replica")
		case <-time.After(10 * time.Millisecond):
		}
	}
	first, err := config.WorkerArchitecture(config.Control)
	if err != nil || first.Connections(config.Control) != 2 || first.Inflight(config.Control) != 3 {
		t.Fatal("wrong first replica budget")
	}
	duplicate, err := workerPool(t, dsn)
	if duplicate != nil {
		duplicate.Close()
	}
	if !errors.Is(err, replicabudget.ErrOccupied) {
		t.Fatal("duplicate process was not rejected", err)
	}
	t.Setenv("INFRAPRO_REPLICA_ID", "control-api-2")
	second, err := config.WorkerArchitecture(config.Control)
	if err != nil || second.Connections(config.Control) != 1 || second.Inflight(config.Control) != 2 {
		t.Fatal("wrong second replica budget")
	}
	peer, err := workerPool(t, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	var pid int32
	if err = peer.QueryRow(t.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	// Both backends held by the child and the single peer backend must be
	// real server sessions. The unsuccessful duplicate must have disconnected.
	var sessions int
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); {
		err = admin.QueryRow(t.Context(), "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND application_name=$1", schema).Scan(&sessions)
		if err != nil {
			t.Fatal(err)
		}
		if sessions == 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if sessions != 3 {
		t.Fatalf("expected three physical PostgreSQL sessions, got %d", sessions)
	}
	// Terminate the authorized backend while the Go process remains alive.
	// No stale server should continue after losing its final shared lock.
	if _, err = admin.Exec(t.Context(), "SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatal(err)
	}
	select {
	case <-peer.Context.Done():
		if !errors.Is(context.Cause(peer.Context), replicabudget.ErrLost) {
			t.Fatal("slot loss was not distinguished")
		}
	case <-time.After(7 * time.Second):
		t.Fatal("lost backend did not fence the live process")
	}
	if err = child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	t.Setenv("INFRAPRO_REPLICA_ID", "control-api-1")
	var replacement *replicabudget.Pool
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		replacement, err = workerPool(t, dsn)
		if err == nil {
			break
		}
		if !errors.Is(err, replicabudget.ErrOccupied) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("slot not recovered after process death")
	}
	defer replacement.Close()
	t.Log("PG total 3, in-flight total 5, duplicate refused, terminated SQL owner fenced, terminated process slot recovered")
}
