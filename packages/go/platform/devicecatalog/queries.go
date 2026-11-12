package devicecatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
)

func fmtPath(i int) string { return fmt.Sprintf("/content/output_slots/%d", i) }
func (s *Service) Manufacturers(ctx context.Context, a access.Actor, scope Scope, f Filter, public bool) (k.Page[Manufacturer], error) {
	out := k.Page[Manufacturer]{Items: []Manufacturer{}}
	kind := "manufacturer"
	if public {
		kind = "catalog-manufacturer"
	}
	e := s.transact(ctx, a, scope, kind, "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := f.Validate(); e != nil {
			return e
		}
		if f.Status != "" && f.Status != "active" && f.Status != "archived" {
			return k.Problem("invalid_request", "厂家状态无效。", 400)
		}
		c, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, kind))
		if e != nil {
			return e
		}
		targetOwner := owner(scope)
		if public {
			targetOwner = nil
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(m)||jsonb_build_object('revision',revision::text) FROM device_manufacturers m WHERE owner_tenant_id IS NOT DISTINCT FROM $1::uuid AND deleted_at IS NULL AND ($2='' OR name ILIKE '%'||$2||'%' OR code ILIKE '%'||$2||'%') AND ($3='' OR status=$3) AND ($4::timestamptz IS NULL OR (created_at,id)>($4,$5::uuid)) ORDER BY created_at,id LIMIT $6`, targetOwner, f.Query, f.Status, c.TimeValue(), k.Nullable(c.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var v Manufacturer
			if e = rows.Scan(&raw); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &v); e != nil {
				return e
			}
			if !public {
				v.Actions = actions(auth, "manufacturer")
			}
			out.Items = append(out.Items, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			c.At = last.CreatedAt
			c.ID = last.ID
			out.NextCursor = c.Encode()
		}
		return nil
	})
	return out, e
}
func (s *Service) Manufacturer(ctx context.Context, a access.Actor, scope Scope, id string) (Manufacturer, error) {
	var out Manufacturer
	e := s.transact(ctx, a, scope, "manufacturer", "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, e = manufacturerTx(ctx, tx, scope, id, false)
		out.Actions = actions(auth, "manufacturer")
		return e
	})
	return out, e
}

// Public adoptability is evaluated in SQL before pagination, including every
// transitive engineering dependency. No private draft participates in the list.
const adoptableCTE = `WITH RECURSIVE blocked(id) AS (
 SELECT v.id FROM engineering_versions v JOIN engineering_version_states s ON s.version_id=v.id WHERE NOT s.enabled
 UNION SELECT d.version_id FROM engineering_dependencies d JOIN blocked b ON b.id=d.target_id
), usable_device_versions AS (
 SELECT v.* FROM device_product_versions v JOIN device_product_version_states s ON s.version_id=v.id WHERE s.enabled AND NOT EXISTS(SELECT 1 FROM device_product_dependencies d JOIN blocked b ON b.id=d.engineering_version_id WHERE d.product_version_id=v.id)
) `

func (s *Service) Products(ctx context.Context, a access.Actor, scope Scope, f Filter, public bool) (k.Page[Product], error) {
	out := k.Page[Product]{Items: []Product{}}
	kind := "product"
	if public {
		kind = "catalog-product"
	}
	e := s.transact(ctx, a, scope, kind, "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := f.Validate(); e != nil {
			return e
		}
		if !slices.Contains([]string{"", "draft", "enabled", "disabled"}, f.Status) {
			return k.Problem("invalid_request", "产品状态筛选无效。", 400)
		}
		c, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, kind))
		if e != nil {
			return e
		}
		targetOwner := owner(scope)
		if public {
			targetOwner = nil
		}
		rows, e := tx.Query(ctx, adoptableCTE+`SELECT to_jsonb(p)||jsonb_build_object('revision',p.revision::text,'manufacturer_name',m.name,'has_draft',NOT $6 AND d.product_id IS NOT NULL,'latest_version_number',v.number,'latest_version_enabled',(SELECT enabled FROM device_product_version_states WHERE version_id=v.id),'name',COALESCE(CASE WHEN NOT $6 THEN d.content->>'name' END,v.content->>'name',''),'model',COALESCE(CASE WHEN NOT $6 THEN d.content->>'model' END,v.content->>'model',''))
   FROM device_products p JOIN device_manufacturers m ON m.id=p.manufacturer_id LEFT JOIN device_product_drafts d ON d.product_id=p.id
   LEFT JOIN LATERAL(SELECT v.* FROM device_product_versions v WHERE v.product_id=p.id AND (NOT $6 OR v.id IN(SELECT id FROM usable_device_versions)) ORDER BY number DESC LIMIT 1)v ON true
   WHERE p.owner_tenant_id IS NOT DISTINCT FROM $1::uuid AND p.deleted_at IS NULL AND (NOT $6 OR (v.id IS NOT NULL AND m.deleted_at IS NULL)) AND ($2='' OR p.code ILIKE '%'||$2||'%' OR COALESCE(CASE WHEN NOT $6 THEN d.content->>'name' END,v.content->>'name','') ILIKE '%'||$2||'%' OR COALESCE(CASE WHEN NOT $6 THEN d.content->>'model' END,v.content->>'model','') ILIKE '%'||$2||'%') AND ($3::uuid IS NULL OR p.manufacturer_id=$3) AND ($4::timestamptz IS NULL OR (p.created_at,p.id)>($4,$5::uuid)) AND ($8='' OR ($8='draft' AND NOT $6 AND d.product_id IS NOT NULL) OR ($8='enabled' AND EXISTS(SELECT 1 FROM device_product_versions v2 JOIN device_product_version_states s2 ON s2.version_id=v2.id WHERE v2.product_id=p.id AND s2.enabled)) OR ($8='disabled' AND v.id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM device_product_versions v2 JOIN device_product_version_states s2 ON s2.version_id=v2.id WHERE v2.product_id=p.id AND s2.enabled))) ORDER BY p.created_at,p.id LIMIT $7`, targetOwner, f.Query, k.Nullable(f.ManufacturerID), c.TimeValue(), k.Nullable(c.ID), public, f.Limit+1, f.Status)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var p Product
			if e = rows.Scan(&raw); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &p); e != nil {
				return e
			}
			p.Versions = []Version{}
			if !public {
				p.Actions = actions(auth, "product")
			}
			out.Items = append(out.Items, p)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			c.At = last.CreatedAt
			c.ID = last.ID
			out.NextCursor = c.Encode()
		}
		return nil
	})
	return out, e
}
func versionsTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, scope Scope, id string, f Filter, public bool) (k.Page[Version], error) {
	out := k.Page[Version]{Items: []Version{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	c, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, []any{"versions", id, public}))
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, adoptableCTE+`SELECT v.id::text FROM device_product_versions v WHERE product_id=$1 AND (NOT $2 OR (v.id IN(SELECT id FROM usable_device_versions) AND EXISTS(SELECT 1 FROM device_products p JOIN device_manufacturers m ON m.id=p.manufacturer_id WHERE p.id=v.product_id AND m.deleted_at IS NULL))) AND ($3::uuid IS NULL OR (number,id)<($4,$3::uuid)) ORDER BY number DESC,id DESC LIMIT $5`, id, public, k.Nullable(c.ID), c.Number, f.Limit+1)
	if e != nil {
		return out, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return out, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	more := len(ids) > f.Limit
	if more {
		ids = ids[:f.Limit]
	}

	versions, _, e := ResolveVersionsTx(ctx, tx, scope.Tenant, ids, false)
	if e != nil {
		return out, e
	}
	for _, id := range ids {
		out.Items = append(out.Items, versions[id])
	}

	if more {
		last := out.Items[len(out.Items)-1]
		c.ID = last.ID
		c.Number = last.Number
		out.NextCursor = c.Encode()
	}
	return out, nil
}
func (s *Service) Product(ctx context.Context, a access.Actor, scope Scope, id string, f Filter) (Product, error) {
	var out Product
	e := s.transact(ctx, a, scope, "product", "detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, e = productTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		out.Draft, e = draftTx(ctx, tx, id)
		if e != nil {
			return e
		}
		versions, e := versionsTx(ctx, tx, auth, scope, id, f, false)
		if e != nil {
			return e
		}
		out.HasDraft = out.Draft != nil
		out.Versions = versions.Items
		if len(out.Versions) > 0 {
			out.LatestVersionNumber = &out.Versions[0].Number
			out.LatestVersionEnabled = &out.Versions[0].Enabled
		}
		out.NextCursor = versions.NextCursor
		out.Actions = actions(auth, "product")
		if out.Draft != nil {
			out.Name = out.Draft.Content.Name
			out.Model = out.Draft.Content.Model
		} else if len(out.Versions) > 0 {
			out.Name = out.Versions[0].Content.Name
			out.Model = out.Versions[0].Content.Model
		}
		return nil
	})
	return out, e
}
func (s *Service) Versions(ctx context.Context, a access.Actor, scope Scope, id string, f Filter) (k.Page[Version], error) {
	out := k.Page[Version]{Items: []Version{}}
	e := s.transact(ctx, a, scope, "catalog-product", "versions", false, func(tx pgx.Tx, auth k.Authorization) error {
		if _, e := productTx(ctx, tx, k.Platform(), id); e != nil {
			return e
		}
		var e error
		out, e = versionsTx(ctx, tx, auth, k.Platform(), id, f, true)
		return e
	})
	return out, e
}
func (s *Service) Version(ctx context.Context, a access.Actor, scope Scope, product, version string, public bool) (Version, error) {
	var out Version
	kind := "product"
	if public {
		kind = "catalog-product"
	}
	e := s.transact(ctx, a, scope, kind, "version", false, func(tx pgx.Tx, _ k.Authorization) error {
		target := scope
		if public {
			target = k.Platform()
		}
		if _, e := productTx(ctx, tx, target, product); e != nil {
			return e
		}
		var e error
		out, _, e = ResolveVersionTx(ctx, tx, target.Tenant, version, false)
		if e != nil {
			return e
		}
		if out.ProductID != product || public && !out.Usable {
			return k.NotFound("definition")
		}
		return nil
	})
	return out, e
}
func (s *Service) Changes(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[Change], error) {
	out := k.Page[Change]{Items: []Change{}}
	e := s.transact(ctx, a, scope, "catalog", "changes", false, func(tx pgx.Tx, auth k.Authorization) error {
		if e := f.Validate(); e != nil {
			return e
		}
		c, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, "catalog-changes"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(c) FROM device_catalog_changes c WHERE owner_tenant_id IS NOT DISTINCT FROM $1::uuid AND ($2::uuid IS NULL OR product_id=$2) AND ($3::uuid IS NULL OR manufacturer_id=$3) AND ($4::timestamptz IS NULL OR (changed_at,id)<($4,$5::uuid)) ORDER BY changed_at DESC,id DESC LIMIT $6`, owner(scope), k.Nullable(f.ProductID), k.Nullable(f.ManufacturerID), c.TimeValue(), k.Nullable(c.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var v Change
			if e = rows.Scan(&raw); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &v); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[f.Limit-1]
			c.ID = last.ID
			c.At = last.ChangedAt
			out.NextCursor = c.Encode()
		}
		return nil
	})
	return out, e
}
