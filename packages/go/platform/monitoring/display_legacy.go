package monitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

// Legacy SDK bindings do not declare a structural model. Unrepresentable owners
// must produce an upgrade requirement, including before statistical reads.
func (s *Service) CheckLegacyStructure(ctx context.Context, a access.Actor, scope Scope, ids []string) error {
	return s.transact(ctx, a, scope, "channels.list", false, false, func(tx pgx.Tx, auth authorization) error {
		var unsupported bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM engineering_channels c WHERE c.tenant_id=$1 AND c.id=ANY($3::uuid[]) AND c.owner_kind IN ('monitoring_section','borehole') AND `+visibleDataChannel+`)`, scope.Tenant, scope.Project, ids).Scan(&unsupported); e != nil {
			return e
		}
		if unsupported {
			return displayread.StructureUpgradeRequired()
		}
		return nil
	})
}
