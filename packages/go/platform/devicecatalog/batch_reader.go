package devicecatalog

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

// ResolveVersionsTx enforces ownership and resolves fixed definitions in batches.
// Callers must already authorize catalog reads or existing device references.
func ResolveVersionsTx(ctx context.Context, tx pgx.Tx, tenant string, ids []string, newUse bool) (map[string]Version, map[string]Product, error) {
	versions, products := map[string]Version{}, map[string]Product{}
	if len(ids) == 0 {
		return versions, products, nil
	}
	for _, id := range ids {
		if !access.ValidID(id) {
			return nil, nil, k.NotFound("definition")
		}
	}
	rows, e := tx.Query(ctx, `SELECT to_jsonb(v)||jsonb_build_object('enabled',s.enabled,'revision',s.revision::text,'manufacturer_id',m.id,'manufacturer_name',m.name,'usable',m.deleted_at IS NULL),to_jsonb(p)||jsonb_build_object('revision',p.revision::text,'manufacturer_name',m.name) FROM device_product_versions v JOIN device_product_version_states s ON s.version_id=v.id JOIN device_products p ON p.id=v.product_id JOIN device_manufacturers m ON m.id=p.manufacturer_id WHERE v.id=ANY($1::uuid[]) AND p.deleted_at IS NULL AND (p.owner_tenant_id IS NULL OR p.owner_tenant_id=$2::uuid)`, ids, k.Nullable(tenant))
	if e != nil {
		return nil, nil, e
	}
	refs := []string{}
	for rows.Next() {
		var vr, pr []byte
		var v Version
		var p Product
		if e = rows.Scan(&vr, &pr); e != nil {
			rows.Close()
			return nil, nil, e
		}
		if e = json.Unmarshal(vr, &v); e != nil {
			rows.Close()
			return nil, nil, e
		}
		if e = json.Unmarshal(pr, &p); e != nil {
			rows.Close()
			return nil, nil, e
		}
		p.Name = v.Content.Name
		p.Model = v.Content.Model
		v.Issues = []Issue{}
		v.Usable = v.Usable && v.Enabled
		if !v.Usable {
			v.Issues = append(v.Issues, Issue{Path: "/id", Code: "definition_unavailable", Message: "厂家或该产品版本不可新采用。"})
		}
		for _, slot := range v.Content.OutputSlots {
			if slot.Mode == "fixed" {
				refs = append(refs, slot.MetricVersionID)
				if slot.UnitVersionID != nil {
					refs = append(refs, *slot.UnitVersionID)
				}
			}
		}
		versions[v.ID] = v
		products[v.ID] = p
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, nil, e
	}
	defs, e := engineering.ResolvePublicBatchTx(ctx, tx, refs, false)
	if e != nil {
		return nil, nil, e
	}
	for _, id := range ids {
		v, ok := versions[id]
		if !ok {
			return nil, nil, k.NotFound("definition")
		}
		roots := []string{}
		for i, slot := range v.Content.OutputSlots {
			if slot.Mode != "fixed" {
				continue
			}
			roots = append(roots, slot.MetricVersionID)
			if slot.UnitVersionID != nil {
				roots = append(roots, *slot.UnitVersionID)
			}
			if !defs[slot.MetricVersionID].Usable || slot.UnitVersionID != nil && !defs[*slot.UnitVersionID].Usable {
				v.Usable = false
				v.Issues = append(v.Issues, Issue{Path: fmtPath(i), Code: "definition_unavailable", Message: "固定依赖不可新采用。"})
			}
		}
		v.Definitions = engineering.PublicDependencySet(defs, roots)
		if newUse && !v.Usable {
			return nil, nil, k.Problem("definition_unavailable", "该产品版本或固定依赖已不可新采用。", 409)
		}
		versions[id] = v
	}
	return versions, products, nil
}
