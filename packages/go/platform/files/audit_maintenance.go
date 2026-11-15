package files

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// Maintenance facts contain logical identifiers and safe state summaries only.
// They do not grant platform audit readers access to tenant file metadata.
func maintenanceAuditTx(ctx context.Context, tx pgx.Tx, action, id, reason string, before, after any) error {
	return access.Audit(ctx, tx, "files.maintenance."+action, "file_maintenance", id, "", before, after, access.Meta{ActorType: "maintenance", Reason: reason})
}
