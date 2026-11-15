package monitoring

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// AuditRevisionTx exposes only this module's current authorization version.
func (s *Service) AuditRevisionTx(ctx context.Context, tx pgx.Tx, tenant, project string) (string, error) {
	var revision string
	e := tx.QueryRow(ctx, `SELECT COALESCE((SELECT revision::text FROM monitoring_access_states WHERE tenant_id=$1 AND project_id=$2),'0')`, tenant, project).Scan(&revision)
	return revision, e
}
