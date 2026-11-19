package migrations

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The new nullable column grants a narrowly scoped SQL lock privilege. It has
// no value on any migrated row, including existing sessions and permissions.
func assertLegacyLockMarkers(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, table := range []string{"accounts", "tenants", "projects", "authorization_state", "platform_security_policy", "account_sessions", "account_mfa", "device_tenant_states", "display_instances"} {
		var marked bool
		query := "SELECT EXISTS (SELECT FROM " + pgx.Identifier{table}.Sanitize() + " WHERE architecture_lock IS NOT NULL)"
		if err := pool.QueryRow(ctx, query).Scan(&marked); err != nil {
			t.Fatal(err)
		}
		if marked {
			t.Fatalf("unexpected lock marker in migrated %s", table)
		}
	}
}
