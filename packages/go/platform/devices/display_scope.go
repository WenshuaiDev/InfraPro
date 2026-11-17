package devices

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

// Full-device grants and output grants are separate populations in this domain.
// A device record grant does not implicitly authorize its outputs.
func (s *Service) DisplayMembersTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, selector displayread.Selector) ([]displayread.Member, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.DeviceOperation("project", "device", "list"), tenant, project); e != nil {
		return nil, e
	}
	rows, e := tx.Query(ctx, `SELECT DISTINCT CASE WHEN g.kind='device' THEN d.id ELSE g.output_id END::text,g.kind
 FROM project_device_grants g JOIN devices d ON d.tenant_id=g.tenant_id AND d.id=g.device_id JOIN device_products p ON p.id=d.product_id
 LEFT JOIN device_outputs o ON o.tenant_id=g.tenant_id AND o.id=g.output_id
 WHERE g.tenant_id=$1 AND g.project_id=$2 AND g.ended_at IS NULL AND d.deleted_at IS NULL
 AND ((g.kind='device' AND ($8<>'ids' OR d.id=ANY($3::uuid[]))) OR (g.kind='output' AND (CASE WHEN COALESCE(cardinality($7::uuid[]),0)>0 THEN o.id=ANY($7) ELSE $8<>'ids' OR d.id=ANY($3::uuid[]) END)))
 AND (COALESCE(cardinality($4::uuid[]),0)=0 OR p.manufacturer_id=ANY($4))
 AND (COALESCE(cardinality($5::uuid[]),0)=0 OR d.product_id=ANY($5))
 AND (COALESCE(cardinality($6::text[]),0)=0 OR d.status=ANY($6))
 AND (g.kind='device' OR COALESCE(cardinality($7::uuid[]),0)=0 OR o.id=ANY($7))
 ORDER BY 2,1 LIMIT 50001`, tenant, project, selector.IDs, selector.ManufacturerIDs, selector.ProductIDs, selector.Statuses, selector.OutputIDs, selector.Kind)
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
