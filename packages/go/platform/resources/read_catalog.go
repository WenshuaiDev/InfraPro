package resources

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
	"strings"
)

// ReadCatalog is a bounded public projection of explicitly selected project
// objects. Counts are computed before pagination, after authorization/filtering.
type ReadCatalogItem struct {
	StructureModel      int    `json:"structure_model"`
	StructureProjection string `json:"structure_projection"`
	ID                  string `json:"id"`
	Kind                string `json:"kind"`
	FacilityID          string `json:"facility_id"`
	ParentID            string `json:"parent_id"`
	ParentKind          string `json:"parent_kind"`
	Code                string `json:"code"`
	Name                string `json:"name"`
	Status              string `json:"status"`
	Revision            string `json:"revision"`
}
type ReadCatalogPage struct {
	StructureModel      int               `json:"structure_model"`
	StructureProjection string            `json:"structure_projection"`
	Items               []ReadCatalogItem `json:"items"`
	NextCursor          string            `json:"next_cursor"`
	Count               string            `json:"count"`
	Counts              map[string]string `json:"counts"`
}

func (s *Service) ReadCatalog(ctx context.Context, a access.Actor, scope Scope, ids []string, kind, search, cursor string, limit int) (ReadCatalogPage, error) {
	out := ReadCatalogPage{StructureModel: 1, StructureProjection: "legacy", Items: []ReadCatalogItem{}, Counts: map[string]string{}, Count: "0"}
	if scope.Entry != "project" || len(ids) > 500 || len(search) > 120 || cursor != "" && !access.ValidID(cursor) {
		return out, notFound()
	}
	for _, id := range ids {
		if !access.ValidID(id) {
			return out, notFound()
		}
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return out, invalid("/limit", "invalid", "分页大小为 1–100。")
	}
	search = strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(search)
	e := s.transact(ctx, a, scope.operation("facility", "list"), scope, false, func(tx pgx.Tx, _ authorization) error {
		var unsupported bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM engineering_channels c JOIN project_resource_visibility($1,$2,NULL) v ON v.kind='channel' AND v.id=c.id AND v.full_access WHERE c.tenant_id=$1 AND c.id=ANY($3::uuid[]) AND c.owner_kind IN ('monitoring_section','borehole') AND c.deleted_at IS NULL)`, scope.Tenant, scope.Project, ids).Scan(&unsupported); e != nil {
			return e
		}
		if unsupported {
			return displayread.StructureUpgradeRequired()
		}
		const selected = ` FROM engineering_resource_nodes_legacy n WHERE n.tenant_id=$1 AND n.id=ANY($3::uuid[]) AND n.deleted_at IS NULL AND ($4='' OR n.kind=$4) AND ($5='' OR n.name ILIKE '%'||$5||'%' OR n.code ILIKE '%'||$5||'%') AND EXISTS(SELECT 1 FROM project_resource_visibility(n.tenant_id,$2,n.facility_id) v WHERE v.kind=n.kind AND v.id=n.id AND v.full_access)`
		rows, e := tx.Query(ctx, `SELECT n.kind,count(*)::text`+selected+` GROUP BY n.kind`, scope.Tenant, scope.Project, ids, kind, search)
		if e != nil {
			return e
		}
		var total int64
		for rows.Next() {
			var k, n string
			if e = rows.Scan(&k, &n); e != nil {
				rows.Close()
				return e
			}
			out.Counts[k] = n
			var value int64
			if _, e = fmt.Sscan(n, &value); e != nil {
				rows.Close()
				return e
			}
			total += value
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		out.Count = fmt.Sprint(total)
		rows, e = tx.Query(ctx, `SELECT n.id::text,n.kind,n.facility_id::text,COALESCE(n.parent_id::text,''),COALESCE(n.parent_kind,''),n.code,n.name,n.status,n.revision::text`+selected+` AND ($6='' OR n.id::text>$6) ORDER BY n.id LIMIT $7`, scope.Tenant, scope.Project, ids, kind, search, cursor, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v := ReadCatalogItem{StructureModel: 1, StructureProjection: "legacy"}
			if e = rows.Scan(&v.ID, &v.Kind, &v.FacilityID, &v.ParentID, &v.ParentKind, &v.Code, &v.Name, &v.Status, &v.Revision); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			out.NextCursor = out.Items[limit-1].ID
		}
		return rows.Err()
	})
	return out, e
}
