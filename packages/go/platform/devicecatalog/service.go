package devicecatalog

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

type Service struct{ access *access.Service }

func New(a *access.Service) *Service { return &Service{access: a} }
func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, kind, action string, write bool, fn func(pgx.Tx, k.Authorization) error) error {
	if scope.Entry == "project" {
		return k.NotFound("definition")
	}
	return k.Transact(ctx, s.access, a, scope, scope.Operation(kind, action), k.Locks{CatalogWrite: write}, fn)
}
func owner(scope Scope) any { return k.Nullable(scope.Tenant) }
func manufacturerTx(ctx context.Context, tx pgx.Tx, scope Scope, id string, publicParent bool) (Manufacturer, error) {
	var out Manufacturer
	if !access.ValidID(id) {
		return out, k.NotFound("definition")
	}
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT to_jsonb(m)||jsonb_build_object('revision',revision::text) FROM device_manufacturers m WHERE id=$1 AND deleted_at IS NULL AND (owner_tenant_id IS NOT DISTINCT FROM $2::uuid OR ($3 AND owner_tenant_id IS NULL))`, id, owner(scope), publicParent).Scan(&raw)
	if e == pgx.ErrNoRows {
		return out, k.NotFound("definition")
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out)
	return out, e
}
func productTx(ctx context.Context, tx pgx.Tx, scope Scope, id string) (Product, error) {
	var out Product
	if !access.ValidID(id) {
		return out, k.NotFound("definition")
	}
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT to_jsonb(p)||jsonb_build_object('revision',p.revision::text,'manufacturer_name',m.name) FROM device_products p JOIN device_manufacturers m ON m.id=p.manufacturer_id WHERE p.id=$1 AND p.deleted_at IS NULL AND p.owner_tenant_id IS NOT DISTINCT FROM $2::uuid`, id, owner(scope)).Scan(&raw)
	if e == pgx.ErrNoRows {
		return out, k.NotFound("definition")
	}
	if e != nil {
		return out, e
	}
	e = json.Unmarshal(raw, &out)
	out.Versions = []Version{}
	return out, e
}
func draftTx(ctx context.Context, tx pgx.Tx, id string) (*Draft, error) {
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT to_jsonb(d)||jsonb_build_object('revision',revision::text) FROM device_product_drafts d WHERE product_id=$1`, id).Scan(&raw)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var d Draft
	e = json.Unmarshal(raw, &d)
	return &d, e
}
func actions(a k.Authorization, kind string) Actions {

	manufacturer, edit, publish, state := access.DeviceManufacturerManage, access.DeviceProductEdit, access.DeviceProductPublish, access.DeviceProductState
	if a.Entry == "tenant" {
		manufacturer, edit, publish, state = access.TenantDeviceManufacturerManage, access.TenantDeviceProductEdit, access.TenantDeviceProductPublish, access.TenantDeviceProductState
	}
	out := Actions{}
	if kind == "manufacturer" {
		out.Edit = a.Has(manufacturer)
		out.State = out.Edit
		out.Delete = out.Edit
		return out
	}
	out.Edit = a.Has(edit)
	out.Copy = out.Edit
	out.Draft = out.Edit
	out.Delete = out.Edit
	out.Publish = a.Has(publish)
	out.State = a.Has(state)
	return out
}

// ResolveVersionTx checks legal ownership and resolves immutable dependencies in
// the caller's transaction. newUse must be true for a new device, false only for
// interpreting an already authorized retained reference. It does not grant reads.
func ResolveVersionTx(ctx context.Context, tx pgx.Tx, tenant, id string, newUse bool) (Version, Product, error) {
	versions, products, e := ResolveVersionsTx(ctx, tx, tenant, []string{id}, newUse)
	return versions[id], products[id], e
}

func catalogChange(ctx context.Context, tx pgx.Tx, a k.Authorization, m Meta, op access.Operation, manufacturer, product, version string, before, after any) error {
	_, e := tx.Exec(ctx, `INSERT INTO device_catalog_changes(owner_tenant_id,manufacturer_id,product_id,version_id,operation,actor_id,actor_name,tenant_membership_id,request_id,reason,before,after) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, owner(a.Scope), k.Nullable(manufacturer), k.Nullable(product), k.Nullable(version), string(op), a.Account, a.Name, k.Nullable(a.TenantMembership), m.RequestID, m.Reason, k.JSON(before), k.JSON(after))
	return e
}
func complete(ctx context.Context, tx pgx.Tx, a k.Authorization, m Meta, op access.Operation, input any, kind, id, manufacturer, product, version string, before, after any, result Result, meta access.Meta) error {
	if e := catalogChange(ctx, tx, a, m, op, manufacturer, product, version, before, after); e != nil {
		return e
	}
	if e := k.Audit(ctx, tx, a, m, string(op), "device_"+kind, id, result, meta); e != nil {
		return e
	}
	return k.SaveReceipt(ctx, tx, a, m, op, input, kind, id, result)
}

// HasResourceReferences exposes existence only, preserving private tenant data.
func (s *Service) HasResourceReferences(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	var out bool
	e := tx.QueryRow(ctx, `WITH RECURSIVE referencing(id) AS (SELECT $1::uuid UNION SELECT d.version_id FROM engineering_dependencies d JOIN referencing r ON d.target_id=r.id) SELECT EXISTS(SELECT 1 FROM device_product_dependencies WHERE engineering_version_id IN(SELECT id FROM referencing)) OR EXISTS(SELECT 1 FROM device_outputs WHERE metric_version_id IN(SELECT id FROM referencing) OR unit_version_id IN(SELECT id FROM referencing))`, id).Scan(&out)
	return out, e
}
