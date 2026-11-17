package spatial

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
	"strconv"
)

func (s *Service) DisplayDefaultsTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, members []dr.Member) (dr.DefaultSummary, error) {
	out := dr.DefaultSummary{Total: "0", Items: []dr.DefaultVersion{}}
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.SpatialOperation("project", "assets.list"), tenant, project); e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `WITH assets AS(SELECT DISTINCT a.id,a.name,a.code FROM spatial_assets a JOIN spatial_asset_versions v ON v.tenant_id=a.tenant_id AND v.asset_id=a.id WHERE v.tenant_id=$1 AND v.id=ANY($3::uuid[]))
 SELECT a.id::text,a.name,CASE WHEN g.version_id IS NULL THEN 'unset' WHEN g.version_id=ANY($3::uuid[]) THEN 'set' ELSE 'unavailable' END,CASE WHEN g.version_id=ANY($3::uuid[]) THEN g.version_id::text ELSE '' END,CASE WHEN g.version_id=ANY($3::uuid[]) THEN v.number::text ELSE '' END,count(*) OVER() FROM assets a LEFT JOIN project_spatial_grants g ON g.tenant_id=$1 AND g.project_id=$2 AND g.asset_id=a.id AND g.is_default AND g.ended_at IS NULL LEFT JOIN spatial_asset_versions v ON v.tenant_id=g.tenant_id AND v.id=g.version_id ORDER BY a.code,a.id LIMIT 21`, tenant, project, (dr.Scope{Members: members}).IDs("version"))
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v dr.DefaultVersion
		var total int64
		if e = rows.Scan(&v.AssetID, &v.Name, &v.State, &v.VersionID, &v.Number, &total); e != nil {
			return out, e
		}
		out.Total = strconv.FormatInt(total, 10)
		if len(out.Items) < 20 {
			out.Items = append(out.Items, v)
		} else {
			out.HasMore = true
		}
	}
	return out, rows.Err()
}
