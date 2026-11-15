package spatial

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

// Qualification binds in-flight reads, browser caches and camera files to the
// same resource, source, monitoring and spatial authorization generations.
func (s *Service) Qualification(ctx context.Context, a access.Actor, scope Scope, action string) (string, error) {
	var out string
	e := s.transact(ctx, a, scope, action, false, func(tx pgx.Tx, auth k.Authorization) error {
		resource, device, data, spatial := "0", "0", "0", "0"
		if scope.Project != "" {
			resource, device = auth.ResourceAccessRevision, auth.DeviceAccessRevision
			if e := tx.QueryRow(ctx, `SELECT COALESCE((SELECT revision FROM monitoring_access_states WHERE tenant_id=$1 AND project_id=$2),0)::text,COALESCE((SELECT revision FROM project_spatial_states WHERE tenant_id=$1 AND project_id=$2),0)::text`, scope.Tenant, scope.Project).Scan(&data, &spatial); e != nil {
				return e
			}
		}
		out = auth.Revision + ":" + resource + ":" + device + ":" + data + ":" + spatial
		return nil
	})
	return out, e
}
