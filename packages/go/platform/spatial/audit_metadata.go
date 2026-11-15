package spatial

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

// AuditReferenceTx exposes finite identifiers and a currently authorized name.
// It never loads the manifest, geometry or private blob location.
func (s *Service) AuditReferenceTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, types ...string) (kind, target, name string, err error) {
	if len(types) > 0 && types[0] == "spatial_access" {
		if scope.Entry != "tenant" {
			err = access.ErrDenied
			return
		}
		_, err = k.AuthorizeTx(ctx, tx, a, scope, access.SpatialOperation("tenant", "access.detail"))
		if err == nil {
			err = tx.QueryRow(ctx, `SELECT name FROM projects WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&name)
		}
		kind, target = "spatial_access", id
		return
	}
	if len(types) > 0 && types[0] == "spatial_view" {
		if scope.Entry != "project" {
			err = access.ErrDenied
			return
		}
		_, err = k.AuthorizeTx(ctx, tx, a, scope, access.SpatialOperation("project", "views.detail"))
		if err != nil {
			return
		}
		var version string
		err = tx.QueryRow(ctx, `SELECT config->>'version_id' FROM project_spatial_views WHERE tenant_id=$1 AND project_id=$2 AND id=$3`, scope.Tenant, scope.Project, id).Scan(&version)
		if err != nil {
			return
		}
		return s.AuditReferenceTx(ctx, tx, a, scope, version)
	}
	_, err = k.AuthorizeTx(ctx, tx, a, scope, access.SpatialOperation(scope.Entry, "assets.detail"))
	if err != nil {
		return
	}
	var asset, version string
	err = tx.QueryRow(ctx, `SELECT asset,version FROM (
 SELECT id::text asset,''::text version,0 priority FROM spatial_assets WHERE tenant_id=$1 AND id::text=$2
 UNION ALL SELECT asset_id::text,id::text,1 FROM spatial_asset_versions WHERE tenant_id=$1 AND id::text=$2 AND deleted_at IS NULL
 UNION ALL SELECT v.asset_id::text,v.id::text,2 FROM spatial_processing_jobs j JOIN spatial_asset_versions v ON v.id=j.version_id WHERE j.tenant_id=$1 AND j.id::text=$2 AND v.deleted_at IS NULL
 UNION ALL SELECT v.asset_id::text,v.id::text,3 FROM spatial_uploads u JOIN spatial_asset_versions v ON v.id=u.version_id WHERE u.tenant_id=$1 AND u.id::text=$2 AND v.deleted_at IS NULL
 ) refs ORDER BY priority LIMIT 1`, scope.Tenant, id).Scan(&asset, &version)
	if err != nil {
		return
	}
	err = tx.QueryRow(ctx, `SELECT name FROM spatial_assets WHERE tenant_id=$1 AND id=$2`, scope.Tenant, asset).Scan(&name)
	if err != nil {
		return
	}
	if scope.Project != "" {
		var allowed bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_spatial_grants g JOIN spatial_asset_versions v ON v.id=g.version_id JOIN spatial_assets a ON a.id=g.asset_id LEFT JOIN project_facility_links l ON l.id=g.facility_link_id WHERE g.tenant_id=$1 AND g.project_id=$2 AND g.asset_id=$3 AND ($4='' OR g.version_id::text=$4) AND g.ended_at IS NULL AND v.state='published' AND (a.facility_id IS NULL OR (l.status='active' AND l.facility_id=a.facility_id)))`, scope.Tenant, scope.Project, asset, version).Scan(&allowed)
		if err == nil && !allowed {
			err = access.ErrDenied
		}
		if err != nil {
			return
		}
	}
	kind, target = "spatial_asset", asset
	if version != "" {
		kind, target = "spatial_version", version
	}
	return
}
