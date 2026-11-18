package migrations

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Old-row snapshots compare only the original columns. Independently assert
// the new columns so a fabricated avatar or wrong initial revision still fails.
func assertAvatarDefaults(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var valid bool
	if err := pool.QueryRow(ctx, `SELECT bool_and(avatar_object_id IS NULL AND avatar_revision=0) FROM accounts`).Scan(&valid); err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("migration fabricated an avatar or revision for a legacy account")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM account_avatar_objects`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("migration fabricated avatar objects")
	}
}
