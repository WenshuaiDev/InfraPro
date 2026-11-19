package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var files embed.FS

const Version int64 = 59

func Up(ctx context.Context, databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files, goose.WithSessionLocker(locker))
	if err != nil {
		return err
	}
	if _, err = provider.Up(ctx); err != nil {
		return err
	}
	return ensureMetadataReaders(ctx, db)
}

// Older schema-59 databases may lack a metadata reader. Dumps restored
// without ACLs also restore PostgreSQL's default PUBLIC EXECUTE.
func ensureMetadataReaders(ctx context.Context, db *sql.DB) error {
	readers := []struct{ section, signature, role string }{
		{"fixed-baseline-metadata", "architecture_fixed_baseline_metadata(uuid,uuid,text,text)", "infrapro_audit_worker"},
		{"notification-invitation-targets", "architecture_notification_invitation_targets(uuid,uuid,uuid[])", "infrapro_control_api"},
	}
	const access = `WITH target AS (
		SELECT p.proowner,p.proacl,p.proconfig FROM pg_catalog.pg_proc p
		WHERE p.oid=to_regprocedure(format('%I.%s',current_schema(),$1::text))
	), permissions AS (
		SELECT a.grantee FROM target t
		CROSS JOIN LATERAL pg_catalog.aclexplode(coalesce(t.proacl,pg_catalog.acldefault('f',t.proowner))) a
		WHERE a.privilege_type='EXECUTE'
	)
	SELECT EXISTS(SELECT FROM target),
		EXISTS(SELECT FROM target WHERE proconfig @> ARRAY[format('search_path=pg_catalog, %I, pg_temp',current_schema())])
		AND NOT EXISTS(SELECT FROM permissions WHERE grantee=0)
		AND (to_regrole($2) IS NULL OR
			EXISTS(SELECT FROM permissions WHERE grantee=to_regrole($2)::oid))`
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('infrapro:baseline-metadata:' || current_schema(), 0))`); err != nil {
		return err
	}
	source, err := files.ReadFile("00059_architecture_events.sql")
	if err != nil {
		return err
	}
	for _, reader := range readers {
		var installed, restricted bool
		if err = tx.QueryRowContext(ctx, access, reader.signature, reader.role).Scan(&installed, &restricted); err != nil {
			return err
		}
		if installed && restricted {
			continue
		}
		marker := "-- architecture:" + reader.section + ":"
		_, section, found := strings.Cut(string(source), marker+"begin")
		body, _, end := strings.Cut(section, marker+"end")
		if !found || !end {
			return fmt.Errorf("incomplete metadata migration for %s", reader.signature)
		}
		if installed {
			_, body, found = strings.Cut(body, marker+"acl")
			if !found {
				return fmt.Errorf("missing metadata permissions for %s", reader.signature)
			}
		}
		if _, err = tx.ExecContext(ctx, body); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func Check(ctx context.Context, pool *pgxpool.Pool) error {
	var version int64
	if err := pool.QueryRow(ctx, "SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied").Scan(&version); err != nil {
		return fmt.Errorf("database migrations unavailable")
	}
	if version != Version {
		return fmt.Errorf("database migration version mismatch: run infrapro database migrate")
	}
	return nil
}
