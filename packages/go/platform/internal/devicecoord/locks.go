// Package devicecoord contains only the shared lock order, never authorization.
package devicecoord

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// Catalog follows the engineering catalog lock and precedes all tenant and
// business row locks. Callers select the final mode up front; never upgrade.
func Catalog(ctx context.Context, tx pgx.Tx, write bool) error {
	mode := " FOR SHARE"
	if write {
		mode = " FOR UPDATE"
	}
	var id int
	return tx.QueryRow(ctx, `SELECT id FROM device_catalog_state WHERE id=1`+mode).Scan(&id)
}

// Tenant serializes related writes, including resource retirement/deletion.
// A missing coordination row is an error, never an authorization fallback.
func Tenant(ctx context.Context, tx pgx.Tx, tenant string, write bool) error {
	mode := " FOR SHARE"
	if write {
		mode = " FOR UPDATE"
	}
	var id string
	return tx.QueryRow(ctx, `SELECT tenant_id::text FROM device_tenant_states WHERE tenant_id=$1`+mode, tenant).Scan(&id)
}
