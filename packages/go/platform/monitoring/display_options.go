package monitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func (s *Service) DisplayOptionsTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, members []dr.Member) ([]dr.ScopeOption, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.MonitoringOperation("project", "selections.create"), tenant, project); e != nil {
		return nil, e
	}
	ids := (dr.Scope{Members: members}).IDs("channel")
	_, facilityPermission := access.RequireActorProjectTx(ctx, tx, a, access.ProjectResourceOperation("facility", "list"), tenant, project)
	rows, e := tx.Query(ctx, `SELECT c.id::text,c.code,c.name,c.source_kind,COALESCE(c.derived_metric_version_id,c.metric_version_id)::text,COALESCE(d.name,m.content->>'name',''),COALESCE(f.id::text,''),COALESCE(f.code,''),COALESCE(f.name,'') FROM engineering_channels c LEFT JOIN engineering_facilities f ON f.id=c.facility_id AND f.tenant_id=c.tenant_id AND f.deleted_at IS NULL AND $4::boolean AND EXISTS(SELECT 1 FROM project_resource_visibility($1,$3,c.facility_id) p WHERE p.kind='facility' AND p.id=c.facility_id AND p.full_access) LEFT JOIN engineering_versions m ON m.id=c.metric_version_id LEFT JOIN derived_metric_versions d ON d.id=c.derived_metric_version_id AND d.tenant_id=c.tenant_id WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[]) AND c.deleted_at IS NULL ORDER BY c.code,c.id`, tenant, ids, project, facilityPermission == nil)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []dr.ScopeOption{}
	metrics := map[string]bool{}
	facilities := map[string]bool{}
	for rows.Next() {
		v := dr.ScopeOption{Kind: "channel", Visibility: "full"}
		var metric, facility, facilityCode, facilityName string
		if e = rows.Scan(&v.ID, &v.Code, &v.Name, &v.SourceKind, &v.VersionID, &metric, &facility, &facilityCode, &facilityName); e != nil {
			return nil, e
		}
		out = append(out, v)
		if facility != "" && !facilities[facility] {
			facilities[facility] = true
			out = append(out, dr.ScopeOption{ID: facility, Kind: "facility", Code: facilityCode, Name: facilityName, Visibility: "full"})
		}
		if !metrics[v.VersionID] {
			metrics[v.VersionID] = true
			out = append(out, dr.ScopeOption{ID: v.VersionID, Kind: "metric_version", Name: metric, SourceKind: v.SourceKind, Visibility: "full"})
		}
	}
	return out, rows.Err()
}
