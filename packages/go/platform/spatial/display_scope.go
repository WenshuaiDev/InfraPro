package spatial

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func (s *Service) DisplayMembersTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, selector displayread.Selector) ([]displayread.Member, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.SpatialOperation("project", "assets.list"), tenant, project); e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, `SELECT v.id::text,'version' FROM spatial_asset_versions v JOIN spatial_assets a ON a.tenant_id=v.tenant_id AND a.id=v.asset_id
 JOIN project_spatial_grants g ON g.tenant_id=v.tenant_id AND g.version_id=v.id LEFT JOIN project_facility_links l ON l.id=g.facility_link_id AND l.tenant_id=g.tenant_id AND l.project_id=g.project_id
 WHERE v.tenant_id=$1 AND g.project_id=$2 AND g.ended_at IS NULL AND v.state='published' AND v.content_available AND v.deleted_at IS NULL
 AND (a.facility_id IS NULL OR l.status='active')
 AND (COALESCE(cardinality($3::uuid[]),0)=0 OR v.id=ANY($3)) AND (COALESCE(cardinality($4::uuid[]),0)=0 OR a.facility_id=ANY($4))
 AND (COALESCE(cardinality($5::text[]),0)=0 OR a.kind=ANY($5)) ORDER BY v.id LIMIT 50001`, tenant, project, selector.IDs, selector.FacilityIDs, selector.AssetKinds)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []displayread.Member{}
	for rows.Next() {
		var m displayread.Member
		if e = rows.Scan(&m.ID, &m.Kind); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	return displayread.NormalizeMembers(out)
}
