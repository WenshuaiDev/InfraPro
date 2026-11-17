package spatial

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func (s *Service) DisplayOptionsTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, members []dr.Member) ([]dr.ScopeOption, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.SpatialOperation("project", "assets.list"), tenant, project); e != nil {
		return nil, e
	}
	_, facilityPermission := access.RequireActorProjectTx(ctx, tx, a, access.ProjectResourceOperation("facility", "list"), tenant, project)
	rows, e := tx.Query(ctx, `SELECT v.id::text,a.code||' / v'||v.number::text,a.name,v.id::text,COALESCE(f.id::text,''),COALESCE(f.code,''),COALESCE(f.name,'') FROM spatial_asset_versions v JOIN spatial_assets a ON a.tenant_id=v.tenant_id AND a.id=v.asset_id LEFT JOIN engineering_facilities f ON f.id=a.facility_id AND f.tenant_id=a.tenant_id AND f.deleted_at IS NULL AND $4::boolean AND EXISTS(SELECT 1 FROM project_resource_visibility($1,$3,a.facility_id) p WHERE p.kind='facility' AND p.id=a.facility_id AND p.full_access) WHERE v.tenant_id=$1 AND v.id=ANY($2::uuid[]) AND v.state='published' ORDER BY a.code,v.number,v.id`, tenant, (dr.Scope{Members: members}).IDs("version"), project, facilityPermission == nil)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []dr.ScopeOption{}
	facilities := map[string]bool{}
	for rows.Next() {
		v := dr.ScopeOption{Kind: "version", Visibility: "full"}
		var facility, code, name string
		if e = rows.Scan(&v.ID, &v.Code, &v.Name, &v.VersionID, &facility, &code, &name); e != nil {
			return nil, e
		}
		out = append(out, v)
		if facility != "" && !facilities[facility] {
			facilities[facility] = true
			out = append(out, dr.ScopeOption{ID: facility, Kind: "facility", Code: code, Name: name, Visibility: "full"})
		}
	}
	return out, rows.Err()
}
