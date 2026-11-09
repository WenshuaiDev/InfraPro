package migrations

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConcurrentMigrationAndVersionCheck(t *testing.T) {
	raw := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("run pnpm test:identity")
	}
	u, e := url.Parse(raw)
	if e != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("refusing non-test database")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	schema := "migration_test_" + strings.ToLower(rand.Text())
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Go(func() { errs <- Up(ctx, u.String()) })
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	pool, e := pgxpool.New(ctx, u.String())
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if e = Check(ctx, pool); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = pool.QueryRow(ctx, "SELECT count(*) FROM system_initialization").Scan(&count); e != nil || count != 1 {
		t.Fatal("migration concurrency duplicated initialization", e)
	}
	if _, e = pool.Exec(ctx, "INSERT INTO goose_db_version(version_id,is_applied) VALUES(999,true)"); e != nil {
		t.Fatal(e)
	}
	if e = Check(ctx, pool); e == nil {
		t.Fatal("unknown migration accepted")
	}
}
