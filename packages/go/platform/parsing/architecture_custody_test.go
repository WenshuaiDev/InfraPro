package parsing_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	ig "github.com/WenshuaiDev/InfraPro/packages/go/platform/ingestion"
	parse "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/jackc/pgx/v5/pgxpool"
)

type custodyBoundary struct {
	parse.ObjectStore
	point, file string
}

func (b custodyBoundary) stopAt() { _ = os.WriteFile(b.file, []byte("at boundary"), 0600); select {} }
func (b custodyBoundary) Put(ctx context.Context, key string, reader io.Reader, size int64, mime string) error {
	if b.point == "pre-write" {
		b.stopAt()
	}
	return b.ObjectStore.Put(ctx, key, reader, size, mime)
}
func (b custodyBoundary) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if b.point == "failed-read" {
		return nil, errors.New("injected authenticated readback failure")
	}
	return b.ObjectStore.Get(ctx, key)
}

type custodyFixture struct {
	DB, Bucket, Point string
	Envelope          r.Envelope
}

func TestArchitectureCustodyBoundaryChild(t *testing.T) {
	dir := os.Getenv("INFRAPRO_ARCH_CUSTODY_CHILD")
	if dir == "" {
		t.Skip("parent owns fault injection")
	}
	b, e := os.ReadFile(filepath.Join(dir, "case.json"))
	must(t, e)
	var x custodyFixture
	must(t, json.Unmarshal(b, &x))
	pool, e := pgxpool.New(ctx, x.DB)
	must(t, e)
	defer pool.Close()
	store, e := blobstore.Open(fixtureObjectEndpoint(), "contract-admin", "contract-admin-test-only", x.Bucket, "ingestion")
	must(t, e)
	point := custodyBoundary{store, x.Point, filepath.Join(dir, "paused")}
	opts, e := ig.LoadOptions(os.Getenv("INFRAPRO_INGESTION_FILE"))
	must(t, e)
	s := ig.New(access.New(pool, slog.Default()), nil).WithOptions(opts).WithStorage(point)
	_, e = s.Accept(ctx, x.Envelope)
	if x.Point == "failed-read" {
		if e == nil {
			t.Fatal("accepted a failed readback")
		}
	} else {
		must(t, e)
	}
	point.stopAt()
}
func TestArchitectureCustodyCrashBoundaries(t *testing.T) {
	f := setupIngestion(t)
	batch, e := os.ReadFile("../../../../samples/ingestion/vendor-batch.json")
	must(t, e)
	var relayConfig struct {
		DatabaseURL string `json:"database_url"`
	}
	raw, e := os.ReadFile(os.Getenv("INFRAPRO_INGESTION_TEST_RELAY_CONFIG"))
	must(t, e)
	must(t, json.Unmarshal(raw, &relayConfig))
	cloud, e := pgxpool.New(ctx, relayConfig.DatabaseURL)
	must(t, e)
	defer cloud.Close()
	store, e := blobstore.Open(fixtureObjectEndpoint(), "contract-admin", "contract-admin-test-only", f.bucket, "ingestion")
	must(t, e)
	for _, point := range []string{"pre-write", "failed-read", "pre-commit", "committed-before-confirmation"} {
		t.Run(point, func(t *testing.T) {
			if f.send(t, batch, point) != 202 {
				t.Fatal("external rejection")
			}
			items := f.cloudPull(t)
			if len(items) != 1 {
				t.Fatalf("unexpected deliveries: %d", len(items))
			}
			env := items[0]
			dir := t.TempDir()
			archJSON(t, filepath.Join(dir, "case.json"), custodyFixture{f.env.Config.DatabaseURL, f.bucket, point, env})
			if point == "pre-commit" {
				// Hold INSERT after the authenticated object reread but before the client
				// can send COMMIT. Killing a client already in COMMIT is ambiguous.
				_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION arc_custody_wait() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(490018); RETURN NEW; END $$; CREATE TRIGGER arc_custody_wait BEFORE INSERT ON ingestion_receipts FOR EACH ROW EXECUTE FUNCTION arc_custody_wait()`)
				must(t, e)
			}
			lock, e := f.env.Pool.Acquire(ctx)
			must(t, e)
			defer lock.Release()
			if point == "pre-commit" {
				_, e = lock.Exec(ctx, `SELECT pg_advisory_lock(490018)`)
				must(t, e)
			}
			proc := exec.Command(os.Args[0], "-test.run", "^TestArchitectureCustodyBoundaryChild$")
			proc.Env = append(os.Environ(), "INFRAPRO_ARCH_CUSTODY_CHILD="+dir)
			must(t, proc.Start())
			defer func() { _ = proc.Process.Kill() }()
			archWait(t, 15*time.Second, func() bool {
				if point != "pre-commit" {
					_, e := os.Stat(filepath.Join(dir, "paused"))
					return e == nil
				}
				var waiting bool
				must(t, f.env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT FROM pg_locks WHERE locktype='advisory' AND objid=490018 AND NOT granted)`).Scan(&waiting))
				return waiting
			}, point)
			must(t, proc.Process.Kill())
			if e = proc.Wait(); e == nil {
				t.Fatal("child exited before injected stop")
			}
			if point == "pre-commit" {
				_, e = lock.Exec(ctx, `SELECT pg_advisory_unlock(490018);DROP TRIGGER arc_custody_wait ON ingestion_receipts;DROP FUNCTION arc_custody_wait()`)
				must(t, e)
			}
			var accepted, ackIntent, confirmed int
			must(t, f.env.Pool.QueryRow(ctx, `SELECT count(r.id),count(a.receipt_id) FROM ingestion_receipts r LEFT JOIN ingestion_ack_outbox a ON a.receipt_id=r.id WHERE r.receipt_id=$1`, env.ReceiptID).Scan(&accepted, &ackIntent))
			must(t, cloud.QueryRow(ctx, `SELECT count(*) FROM relay_ack_receipts WHERE receipt_id=$1`, env.ReceiptID).Scan(&confirmed))
			want := 0
			if point == "committed-before-confirmation" {
				want = 1
			}
			if accepted != want || ackIntent != want || confirmed != 0 {
				t.Fatal("premature or missing custody evidence", accepted, ackIntent, confirmed)
			}
			local, e := f.s.Accept(ctx, env)
			must(t, e)
			_, e = f.s.AckOne(ctx)
			must(t, e)
			var key, hash, pair string
			must(t, f.env.Pool.QueryRow(ctx, `SELECT o.object_key,o.sha256 FROM ingestion_receipts r JOIN ingestion_objects o ON o.id=r.object_id JOIN ingestion_ack_outbox a ON a.receipt_id=r.id WHERE r.id=$1 AND a.state='acknowledged'`, local).Scan(&key, &hash))
			rd, e := store.Get(ctx, key)
			must(t, e)
			body, e := io.ReadAll(rd)
			must(t, e)
			rd.Close()
			must(t, cloud.QueryRow(ctx, `SELECT local_receipt_id::text FROM relay_ack_receipts WHERE receipt_id=$1 AND sha256=$2`, env.ReceiptID, hash).Scan(&pair))
			if pair != local || hash != archHash(body) || hash != env.SHA256 {
				t.Fatal("confirmed evidence mismatch")
			}
		})
	}
}
