package derivedmonitoring

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
)

// Read actual schema-58 JSONB and its original Go checksum after upgrading
// the private schema. Corrupt fixtures exercise the same read path as restored
// data, without editing immutable evidence after the upgrade.
func TestFixedBaselineMetadataProjection(t *testing.T) {
	raw := os.Getenv("INFRAPRO_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("requires an isolated *_test PostgreSQL database")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("isolated test database required")
	}
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	admin, err := pgx.Connect(ctx, raw)
	must(err)
	defer admin.Close(ctx)
	schema := "baseline_projection_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	must(err)
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	params := u.Query()
	params.Set("search_path", schema)
	u.RawQuery = params.Encode()
	db, err := sql.Open("pgx", u.String())
	must(err)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS("../migrations"))
	must(err)
	_, err = provider.UpTo(ctx, 58)
	must(err)
	conn, err := pgx.Connect(ctx, u.String())
	must(err)
	defer conn.Close(ctx)

	tenant, config, snapshot := uuid.NewString(), uuid.NewString(), uuid.NewString()
	from, to := int64(math.MinInt64), int64(math.MaxInt64)
	base := fixedBaseline{
		Snapshot:  monitoring.RawSnapshot{ID: snapshot, Schema: "raw-input-snapshot/1", SourcePosition: "7", RouteRevision: "10", Hash: strings.Repeat("a", 64)},
		Needs:     []monitoring.RawNeed{{Channel: "second\n<&>中", From: &from}, {Channel: "first", To: &to}, {Channel: "third", From: &from, To: &to}},
		Numerator: "-303", Denominator: "140", At: math.MinInt64, Quality: "good", Count: math.MaxUint64, Excluded: 8,
		Observation: "quotation\" and literal \\u003c\x1f", Adoption: "边界<&>", Result: "\u2028", Generation: "\u2029",
	}
	type fixture struct {
		label    string
		actual   fixedBaseline
		checksum string
		valid    bool
	}
	var cases []fixture
	seed, err := conn.Begin(ctx)
	must(err)
	defer seed.Rollback(ctx)
	// Isolate this restored evidence from the domain fixture. Its immutable
	// parent graph and all other schemas keep their normal trigger behavior.
	_, err = seed.Exec(ctx, "SET LOCAL session_replication_role = replica")
	must(err)
	insert := func(label string, edit func(*fixedBaseline), checksumBeforeEdit bool, storedRef, storedSnapshot string, valid bool) {
		t.Helper()
		b := base
		b.Needs = slices.Clone(base.Needs)
		b.Reference = label
		oldHash := fingerprint(b)
		if edit != nil {
			edit(&b)
		}
		hash := fingerprint(b)
		if checksumBeforeEdit {
			hash = oldHash
		}
		if storedRef == "" {
			storedRef = b.Reference
		}
		if storedSnapshot == "" {
			storedSnapshot = snapshot
		}
		_, err := seed.Exec(ctx, `INSERT INTO derived_fixed_baselines (tenant_id,config_id,reference_key,snapshot_id,evidence,evidence_hash,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7)`, tenant, config, storedRef, storedSnapshot, jsonBytes(b), hash, uuid.NewString())
		must(err)
		cases = append(cases, fixture{storedRef, b, hash, valid})
	}
	insert("old-range/中<&>\u2028\u2029", nil, false, "", "", true)
	insert("window-without-optional-identifiers", func(b *fixedBaseline) {
		b.Observation, b.Adoption, b.Result, b.Generation = "", "", "", ""
		b.At, b.Count, b.Excluded = math.MaxInt64, 0, 0
		b.Needs = nil
	}, false, "", "", true)
	insert("empty-ranges", func(b *fixedBaseline) { b.Needs = []monitoring.RawNeed{} }, false, "", "", true)
	insert("signed-reversed-ranges", func(b *fixedBaseline) { slices.Reverse(b.Needs) }, false, "", "", true)
	insert("unsigned-reversed-ranges", func(b *fixedBaseline) { slices.Reverse(b.Needs) }, true, "", "", false)
	insert("different-range", func(b *fixedBaseline) { b.Needs[0].To = &to }, true, "", "", false)
	insert("changed-private-numerator", func(b *fixedBaseline) { b.Numerator = "-304" }, true, "", "", false)
	insert("changed-private-denominator", func(b *fixedBaseline) { b.Denominator = "141" }, true, "", "", false)
	insert("wrong-stored-reference", nil, false, "mismatching-record-key", "", false)
	insert("wrong-stored-snapshot", nil, false, "", uuid.NewString(), false)
	must(seed.Commit(ctx))
	must(migrations.Up(ctx, u.String()))

	tx, err := conn.Begin(ctx)
	must(err)
	defer tx.Rollback(ctx)
	// The historical audit role must work immediately after migrations.Up.
	_, err = tx.Exec(ctx, "SET LOCAL ROLE infrapro_audit_worker")
	must(err)
	_, found, err := loadBaselineMetadataTx(ctx, tx, tenant, config, cases[0].label, nil)
	must(err)
	if !found {
		t.Fatal("legacy audit role could not check its evidence")
	}
	_, err = tx.Exec(ctx, "RESET ROLE")
	must(err)
	// The split role may already exist from another installed schema. All
	// grants below are transaction-local and concern this private schema.
	_, err = tx.Exec(ctx, `DO $$ BEGIN IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='infrapro_display_runtime') THEN
		CREATE ROLE infrapro_display_runtime NOLOGIN; END IF; END $$`)
	must(err)
	_, err = tx.Exec(ctx, "GRANT USAGE ON SCHEMA "+schema+" TO infrapro_display_runtime")
	must(err)
	const signature = "architecture_fixed_baseline_metadata(uuid,uuid,text,text)"
	_, err = tx.Exec(ctx, "SET LOCAL ROLE infrapro_display_runtime")
	must(err)
	denied := func(query string) {
		t.Helper()
		savepoint, err := tx.Begin(ctx)
		must(err)
		_, err = savepoint.Exec(ctx, query)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "42501" {
			t.Fatalf("expected SQLSTATE 42501 for %s: %v", query, err)
		}
		must(savepoint.Rollback(ctx))
	}
	denied("SELECT * FROM architecture_fixed_baseline_metadata(NULL,NULL,NULL,NULL)")
	_, err = tx.Exec(ctx, "RESET ROLE")
	must(err)
	_, err = tx.Exec(ctx, "GRANT EXECUTE ON FUNCTION "+signature+" TO infrapro_display_runtime")
	must(err)
	_, err = tx.Exec(ctx, "SET LOCAL ROLE infrapro_display_runtime")
	must(err)
	denied("SELECT evidence FROM derived_fixed_baselines")
	denied("UPDATE derived_fixed_baselines SET evidence=evidence WHERE false")
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			got, found, err := loadBaselineMetadataTx(ctx, tx, tenant, config, c.label, &c.checksum)
			if !c.valid {
				if found || err == nil || err.Error() != "baseline_evidence_integrity" {
					t.Fatalf("damaged stored evidence was not rejected: found=%v err=%v", found, err)
				}
				return
			}
			if err != nil || !found || !reflect.DeepEqual(got, baselineMetadata{Snapshot: c.actual.Snapshot, Needs: c.actual.Needs}) {
				t.Fatalf("projected evidence differs from the stored Go snapshot or ordered ranges: found=%v err=%v, metadata=%+v", found, err, got)
			}
		})
	}
	for _, c := range []struct{ tenant, config, reference, expected string }{
		{tenant, config, cases[0].label, strings.Repeat("0", 64)},
		{uuid.NewString(), config, cases[0].label, cases[0].checksum},
		{tenant, uuid.NewString(), cases[0].label, cases[0].checksum},
		{tenant, config, "unknown", cases[0].checksum},
	} {
		_, found, err := loadBaselineMetadataTx(ctx, tx, c.tenant, c.config, c.reference, &c.expected)
		if err != nil || found {
			t.Fatalf("missing evidence or mismatching external proof accepted: found=%v err=%v", found, err)
		}
	}
	rows, err := tx.Query(ctx, "SELECT * FROM architecture_fixed_baseline_metadata($1,$2,$3,NULL)", tenant, config, cases[0].label)
	must(err)
	defer rows.Close()
	fields := rows.FieldDescriptions()
	if len(fields) != 2 || fields[0].Name != "snapshot" || fields[1].Name != "input_ranges" || !rows.Next() {
		t.Fatal("projection did not expose exactly the two authorization fields", fields)
	}
	t.Log("real schema 58→59, legacy audit and display runtime grants, three 42501 denials, four absent or mismatching external references, two authorized fields")
}
