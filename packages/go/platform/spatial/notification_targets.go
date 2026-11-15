package spatial

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

func (s *Service) NotificationTargetsTx(ctx context.Context, tx pgx.Tx, account string, scope Scope, ids []string) (map[string]ne.TargetContext, error) {
	out := map[string]ne.TargetContext{}
	action := "versions.draft"
	if scope.Project != "" {
		action = "versions.manifest"
	}
	_, e := k.AuthorizeTx(ctx, tx, access.Actor{ID: account}, scope, access.SpatialOperation(scope.Entry, action))
	if restricted(e) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, `SELECT v.id::text,a.name,v.state,v.content_available FROM spatial_asset_versions v JOIN spatial_assets a ON a.tenant_id=v.tenant_id AND a.id=v.asset_id LEFT JOIN project_spatial_grants g ON g.tenant_id=v.tenant_id AND g.version_id=v.id AND g.project_id::text=$3 AND g.ended_at IS NULL LEFT JOIN project_facility_links l ON l.id=g.facility_link_id WHERE v.tenant_id=$1 AND v.deleted_at IS NULL AND v.id::text=ANY($2::text[]) AND ($3='' OR (v.state='published' AND g.id IS NOT NULL AND (a.facility_id IS NULL OR (l.status='active' AND g.facility_id=a.facility_id))))`, scope.Tenant, ids, scope.Project)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, state string
		var available bool
		if e = rows.Scan(&id, &name, &state, &available); e != nil {
			return nil, e
		}
		v := ne.TargetContext{Allowed: true, Available: available && state != "withdrawn", Name: name, State: state}
		if !v.Available {
			v.Reason = "object_unavailable"
		}
		out[id] = v
	}
	return out, rows.Err()
}
