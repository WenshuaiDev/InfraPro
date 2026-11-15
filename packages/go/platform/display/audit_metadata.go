package display

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

func (s *Service) AuditPackageTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string) (string, string, error) {
	var instance string
	e := tx.QueryRow(ctx, `SELECT instance_id::text FROM display_packages WHERE tenant_id=$1 AND project_id=$2 AND id=$3`, scope.Tenant, scope.Project, id).Scan(&instance)
	if e != nil {
		return "", "", e
	}
	values, e := s.NotificationTargetsTx(ctx, tx, a.ID, scope, []string{instance})
	if e != nil {
		return "", "", e
	}
	v := values[instance]
	if !v.Allowed {
		return "", "", access.ErrDenied
	}
	return instance, v.Name, nil
}
