package ingestion_test

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	wire "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/ingestion"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOperatorSchemaMatchesRuntime(t *testing.T) {
	raw := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("isolated test database required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("refusing non-test database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "operator_test_" + strings.ToLower(rand.Text())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	if err = migrations.Up(ctx, u.String()); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := ingestion.New(access.New(pool, slog.Default()), nil)
	for _, tc := range []struct {
		name    string
		version int64
	}{
		{"current_release", migrations.Version},
		{"previous_release", migrations.Version - 1},
		{"legacy_ingestion_release", 45},
		{"future_release", migrations.Version + 1},
		{"unapplied", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Change only the ledger in this disposable schema, retaining the
			// tables so rejection must come from the version check itself.
			if _, err := pool.Exec(ctx, `UPDATE goose_db_version SET is_applied=(version_id<=$1)`, tc.version); err != nil {
				t.Fatal(err)
			}
			if tc.version > migrations.Version {
				if _, err := pool.Exec(ctx, `INSERT INTO goose_db_version(version_id,is_applied) VALUES($1,true)`, tc.version); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				if _, err := pool.Exec(ctx, `DELETE FROM goose_db_version WHERE version_id>$1`, migrations.Version); err != nil {
					t.Error(err)
				}
				if _, err := pool.Exec(ctx, `UPDATE goose_db_version SET is_applied=true`); err != nil {
					t.Error(err)
				}
			})
			runtimeErr := migrations.Check(ctx, pool)
			manifest, manifestErr := s.OperatorManifest(ctx)
			objects, objectsErr := s.VerifyObjects(ctx)
			if tc.version == migrations.Version {
				if runtimeErr != nil || manifestErr != nil || objectsErr != nil {
					t.Fatalf("current release rejected: runtime=%v manifest=%v objects=%v", runtimeErr, manifestErr, objectsErr)
				}
				if int64(manifest.Schema) != migrations.Version || len(manifest.Receipts) != 0 || len(manifest.Objects) != 0 || objects["objects"] != 0 {
					t.Fatalf("unexpected empty-install manifest/report: %+v %+v", manifest, objects)
				}
				return
			}
			if runtimeErr == nil {
				t.Fatal("runtime accepted unsupported schema")
			}
			_, releaseErr := s.VerifyRelease(ctx, t.TempDir())
			for name, err := range map[string]error{"manifest": manifestErr, "release": releaseErr, "objects": objectsErr} {
				var failure *wire.Error
				if !errors.As(err, &failure) || failure.Code != "schema_incompatible" {
					t.Errorf("%s must reject schema %d before reading artifacts/objects: %v", name, tc.version, err)
				}
			}
		})
	}
}
