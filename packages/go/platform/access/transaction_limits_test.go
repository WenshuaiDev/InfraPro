package access_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The same backend is reused so a leaked SET or failed rollback cannot be
// masked by the next caller getting a different pooled connection.
func TestArchitectureTransactionLimitsAndReuse(t *testing.T) {
	raw := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("isolated PG required")
	}
	config, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	svc := access.New(pool, slog.Default())
	const current = "SELECT pg_backend_pid(),current_setting('lock_timeout'),current_setting('statement_timeout')"
	var backend int
	var lockBefore, statementBefore string
	if err = pool.QueryRow(ctx, current).Scan(&backend, &lockBefore, &statementBefore); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, "CREATE TEMP TABLE arch_tx_reuse(effect text)"); err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("cancelled effect")
	run := func(effect string, rollback bool) error {
		return svc.Transact(ctx, func(tx pgx.Tx) error {
			var pid int
			var lock, statement string
			if e := tx.QueryRow(ctx, current).Scan(&pid, &lock, &statement); e != nil {
				return e
			}
			if pid != backend || lock != "5s" || statement != "10s" || tx.Conn().PgConn().TxStatus() != 'T' {
				t.Fatalf("invalid active transaction %d %s %s", pid, lock, statement)
			}
			if _, e := tx.Exec(ctx, "INSERT INTO arch_tx_reuse VALUES ($1)", effect); e != nil {
				return e
			}
			if rollback {
				return rejected
			}
			return nil
		})
	}
	for _, flag := range []struct {
		effect   string
		rollback bool
	}{{"rolled_back", true}, {"committed", false}} {
		rollback := flag.rollback
		err = run(flag.effect, rollback)
		if rollback && !errors.Is(err, rejected) || !rollback && err != nil {
			t.Fatal(err)
		}
		conn, e := pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		var pid int
		var lock, statement string
		e = conn.QueryRow(ctx, current).Scan(&pid, &lock, &statement)
		status := conn.Conn().PgConn().TxStatus()
		conn.Release()
		if e != nil || pid != backend || lock != lockBefore || statement != statementBefore || status != 'I' {
			t.Fatalf("pool settings after transaction %v %d %s %s %c", e, pid, lock, statement, status)
		}
	}
	var committed []string
	if err = pool.QueryRow(ctx, "SELECT array_agg(effect ORDER BY effect) FROM arch_tx_reuse").Scan(&committed); err != nil || len(committed) != 1 || committed[0] != "committed" {
		t.Fatalf("persisted effects %v %v", committed, err)
	}
}
