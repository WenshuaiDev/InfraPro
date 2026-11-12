package devices

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

// TreeNode is deliberately independent of Device. Context nodes cannot carry
// serial numbers, positions, hidden sibling counts, or device actions.
type TreeNode struct {
	ID                 string    `json:"id"`
	ParentID           *string   `json:"parent_id"`
	Kind               string    `json:"kind"`
	Code               string    `json:"code"`
	Name               string    `json:"name"`
	Model              string    `json:"model"`
	Source             string    `json:"source"`
	Visibility         string    `json:"visibility"`
	HasVisibleChildren bool      `json:"has_visible_children"`
	CreatedAt          time.Time `json:"created_at"`
}

func (s *Service) Tree(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[TreeNode], error) {
	out := k.Page[TreeNode]{Items: []TreeNode{}}
	e := s.transact(ctx, a, scope, "tree", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := f.Validate(); e != nil {
			return e
		}
		if f.Level == "" {
			f.Level = "manufacturer"
		}
		if !slices.Contains([]string{"manufacturer", "product", "device", "output"}, f.Level) || !slices.Contains([]string{"", "all", "public", "private"}, f.Source) || !slices.Contains([]string{"", "enabled", "disabled", "active", "archived"}, f.Status) {
			return k.Problem("invalid_request", "目录筛选条件无效。", 400)
		}
		if (f.Level == "manufacturer") != (f.ParentID == "") {
			return k.Problem("invalid_request", "请选择与目录层级对应的父节点。", 400)
		}
		pub, private, read := auth.Has(access.TenantDeviceCatalogRead), auth.Has(access.TenantDeviceDefinitionRead), auth.Has(access.TenantDeviceRead)
		if scope.Entry == "project" {
			pub = false
			private = false
			read = auth.Has(access.ProjectDeviceRead)
		}
		if !pub && !private && !read {
			return access.ErrDenied
		}
		cursor, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, "device-tree"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `WITH RECURSIVE blocked(id) AS (
   SELECT version_id FROM engineering_version_states WHERE NOT enabled UNION SELECT d.version_id FROM engineering_dependencies d JOIN blocked b ON b.id=d.target_id
  ), usable AS (SELECT v.id FROM device_product_versions v JOIN device_product_version_states s ON s.version_id=v.id WHERE s.enabled AND NOT EXISTS(SELECT 1 FROM device_product_dependencies dep JOIN blocked b ON b.id=dep.engineering_version_id WHERE dep.product_version_id=v.id)),
  visible_outputs AS (SELECT o.* FROM device_outputs o WHERE o.tenant_id=$1 AND $5 AND ($2::uuid IS NULL OR EXISTS(SELECT 1 FROM project_device_grants g WHERE g.project_id=$2 AND g.tenant_id=$1 AND g.kind='output' AND g.output_id=o.id AND g.ended_at IS NULL))),
  visible_devices AS (SELECT d.*,($2::uuid IS NULL OR EXISTS(SELECT 1 FROM project_device_grants g WHERE g.project_id=$2 AND g.tenant_id=$1 AND g.kind='device' AND g.device_id=d.id AND g.ended_at IS NULL)) AS full_read FROM devices d WHERE d.tenant_id=$1 AND $5 AND d.deleted_at IS NULL AND ($2::uuid IS NULL OR EXISTS(SELECT 1 FROM project_device_grants g WHERE g.project_id=$2 AND g.tenant_id=$1 AND g.device_id=d.id AND g.ended_at IS NULL))),
  visible_products AS (
   SELECT p.*,m.name AS manufacturer_name,m.code AS manufacturer_code,
    (p.owner_tenant_id=$1 AND $4 OR p.owner_tenant_id IS NULL AND $3 AND m.deleted_at IS NULL AND EXISTS(SELECT 1 FROM device_product_versions v JOIN usable u ON u.id=v.id WHERE v.product_id=p.id)) AS full_read,
    COALESCE(CASE WHEN p.owner_tenant_id=$1 AND $4 THEN draft.content->>'name' END,v.content->>'name','') AS name,
    COALESCE(CASE WHEN p.owner_tenant_id=$1 AND $4 THEN draft.content->>'model' END,v.content->>'model','') AS model
   FROM device_products p JOIN device_manufacturers m ON m.id=p.manufacturer_id LEFT JOIN device_product_drafts draft ON draft.product_id=p.id
   LEFT JOIN LATERAL (SELECT v.content FROM device_product_versions v WHERE v.product_id=p.id AND ((p.owner_tenant_id=$1 AND $4) OR (p.owner_tenant_id IS NULL AND $3 AND m.deleted_at IS NULL AND v.id IN(SELECT id FROM usable)) OR v.id IN(SELECT product_version_id FROM visible_devices)) ORDER BY number DESC LIMIT 1)v ON true
   WHERE p.deleted_at IS NULL AND (p.owner_tenant_id=$1 OR p.owner_tenant_id IS NULL) AND ($6 IN ('','all') OR ($6='public')=(p.owner_tenant_id IS NULL))
    AND ((p.owner_tenant_id=$1 AND $4) OR (p.owner_tenant_id IS NULL AND $3 AND m.deleted_at IS NULL AND EXISTS(SELECT 1 FROM device_product_versions v JOIN usable u ON u.id=v.id WHERE v.product_id=p.id)) OR EXISTS(SELECT 1 FROM visible_devices d WHERE d.product_id=p.id))
    AND ($7::uuid IS NULL OR p.manufacturer_id=$7) AND ($8::uuid IS NULL OR p.id=$8)
  ), matching_products AS (
   SELECT p.* FROM visible_products p WHERE $9='' OR concat_ws(' ',p.code,p.name,p.model,p.manufacturer_name,p.manufacturer_code) ILIKE '%'||$9||'%' OR EXISTS(SELECT 1 FROM visible_devices d WHERE d.product_id=p.id AND (concat_ws(' ',d.name,d.code) ILIKE '%'||$9||'%' OR EXISTS(SELECT 1 FROM visible_outputs o WHERE o.device_id=d.id AND o.name ILIKE '%'||$9||'%')))
  ), nodes AS (
   SELECT m.id,NULL::uuid parent_id,'manufacturer'::text kind,m.code,m.name,''::text model,CASE WHEN m.owner_tenant_id IS NULL THEN 'public' ELSE 'private' END source,
    CASE WHEN (m.owner_tenant_id IS NULL AND $3 AND m.deleted_at IS NULL OR m.owner_tenant_id=$1 AND $4) THEN 'full' ELSE 'context' END visibility,
    EXISTS(SELECT 1 FROM matching_products p WHERE p.manufacturer_id=m.id) has_visible_children,m.created_at
    FROM device_manufacturers m WHERE $10='manufacturer' AND m.deleted_at IS NULL AND (($11 IN ('','active','archived') AND ($11='' OR m.status=$11)) OR $11 IN ('enabled','disabled')) AND ($7::uuid IS NULL OR m.id=$7)
    AND (EXISTS(SELECT 1 FROM matching_products p WHERE p.manufacturer_id=m.id) OR ($8::uuid IS NULL AND ($6 IN ('','all') OR ($6='public')=(m.owner_tenant_id IS NULL)) AND (m.owner_tenant_id IS NULL AND $3 AND m.deleted_at IS NULL OR m.owner_tenant_id=$1 AND $4) AND ($9='' OR concat_ws(' ',m.name,m.code) ILIKE '%'||$9||'%')))
   UNION ALL SELECT p.id,p.manufacturer_id,'product',p.code,p.name,p.model,CASE WHEN p.owner_tenant_id IS NULL THEN 'public' ELSE 'private' END,CASE WHEN p.full_read THEN 'full' ELSE 'context' END,EXISTS(SELECT 1 FROM visible_devices d WHERE d.product_id=p.id),p.created_at FROM matching_products p WHERE $10='product' AND p.manufacturer_id=$12
   UNION ALL SELECT d.id,d.product_id,'device',d.code,d.name,COALESCE(v.content->>'model',''),CASE WHEN p.owner_tenant_id IS NULL THEN 'public' ELSE 'private' END,CASE WHEN d.full_read THEN 'full' ELSE 'context' END,EXISTS(SELECT 1 FROM visible_outputs o WHERE o.device_id=d.id),d.created_at FROM visible_devices d JOIN matching_products p ON p.id=d.product_id JOIN device_product_versions v ON v.id=d.product_version_id WHERE $10='device' AND d.product_id=$12 AND ($11 NOT IN ('enabled','disabled') OR d.status=$11) AND ($9='' OR concat_ws(' ',d.name,d.code,p.name,p.code,p.manufacturer_name) ILIKE '%'||$9||'%' OR EXISTS(SELECT 1 FROM visible_outputs o WHERE o.device_id=d.id AND o.name ILIKE '%'||$9||'%'))
   UNION ALL SELECT o.id,o.device_id,'output',o.slot_key,o.name,''::text,CASE WHEN p.owner_tenant_id IS NULL THEN 'public' ELSE 'private' END,'full',false,o.started_at FROM visible_outputs o JOIN visible_devices d ON d.id=o.device_id JOIN matching_products p ON p.id=d.product_id WHERE $10='output' AND o.device_id=$12 AND ($9='' OR concat_ws(' ',o.name,d.name,d.code,p.name,p.code,p.manufacturer_name) ILIKE '%'||$9||'%')
  ) SELECT * FROM nodes WHERE ($13::timestamptz IS NULL OR (created_at,id)>($13,$14::uuid)) ORDER BY created_at,id LIMIT $15`, auth.Tenant, k.Nullable(auth.Project), pub, private, read, f.Source, k.Nullable(f.ManufacturerID), k.Nullable(f.ProductID), f.Query, f.Level, f.Status, k.Nullable(f.ParentID), cursor.TimeValue(), k.Nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var n TreeNode
			if e = rows.Scan(&n.ID, &n.ParentID, &n.Kind, &n.Code, &n.Name, &n.Model, &n.Source, &n.Visibility, &n.HasVisibleChildren, &n.CreatedAt); e != nil {
				return e
			}
			out.Items = append(out.Items, n)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			cursor.ID = last.ID
			cursor.At = last.CreatedAt
			out.NextCursor = cursor.Encode()
		}
		return nil
	})
	return out, e
}
