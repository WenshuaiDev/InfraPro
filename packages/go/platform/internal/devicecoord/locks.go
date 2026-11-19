// Package devicecoord contains only the shared lock order, never authorization.
package devicecoord

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const catalogStateLock = "SELECT id FROM device_catalog_state WHERE id=1"
const tenantStateLock = "SELECT tenant_id::text FROM device_tenant_states WHERE tenant_id=$1"

func lockMode(write bool) string {
	if write {
		return " FOR UPDATE"
	}
	return " FOR SHARE"
}
func requireState(row pgx.Row) error {
	var id any
	return row.Scan(&id)
}

// Catalog follows the engineering catalog lock and precedes all tenant and
// business row locks. Callers select the final mode up front; never upgrade.
func Catalog(ctx context.Context, tx pgx.Tx, write bool) error {
	return requireState(tx.QueryRow(ctx, catalogStateLock+lockMode(write)))
}

// Tenant serializes related writes, including resource retirement/deletion.
// A missing coordination row is an error, never an authorization fallback.
func Tenant(ctx context.Context, tx pgx.Tx, tenant string, write bool) error {
	return requireState(tx.QueryRow(ctx, tenantStateLock+lockMode(write), tenant))
}

// QueueStateLocks follows the authorization lock in the same batch.
// PostgreSQL executes them in batch order; no domain work starts until all
// responses, including a missing device or tenant state, have been checked.
func QueueStateLocks(batch *pgx.Batch, tenant string, write bool) {
	batch.Queue("SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE")
	batch.Queue(catalogStateLock + lockMode(false)).QueryRow(requireState)
	if tenant != "" {
		batch.Queue(tenantStateLock+lockMode(write), tenant).QueryRow(requireState)
	}
}
