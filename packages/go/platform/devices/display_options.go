package devices

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func (s *Service) DisplayOptionsTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, members []dr.Member) ([]dr.ScopeOption, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.DeviceOperation("project", "device", "list"), tenant, project); e != nil {
		return nil, e
	}
	scope := dr.Scope{Members: members}
	rows, e := tx.Query(ctx, `SELECT `+deviceJSON+deviceFrom+` WHERE d.tenant_id=$1 AND d.id=ANY($2::uuid[]) AND d.deleted_at IS NULL ORDER BY d.code,d.id`, tenant, scope.IDs("device"))
	if e != nil {
		return nil, e
	}
	out := []dr.ScopeOption{}
	seen := map[string]bool{}
	add := func(v dr.ScopeOption) {
		if !seen[v.Kind+v.ID] {
			seen[v.Kind+v.ID] = true
			v.Visibility = "full"
			out = append(out, v)
		}
	}
	for rows.Next() {
		var raw []byte
		var d Device
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return nil, e
		}
		if e = json.Unmarshal(raw, &d); e != nil {
			rows.Close()
			return nil, e
		}
		add(dr.ScopeOption{ID: d.ID, Kind: "device", Code: d.Code, Name: d.Name})
		add(dr.ScopeOption{ID: d.ProductID, Kind: "product", Code: d.Product.Code, Name: d.Product.Name})
		add(dr.ScopeOption{ID: d.Product.ManufacturerID, Kind: "manufacturer", Name: d.Product.ManufacturerName})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	rows, e = tx.Query(ctx, `SELECT id::text,slot_key,name FROM device_outputs WHERE tenant_id=$1 AND id=ANY($2::uuid[]) AND ended_at IS NULL ORDER BY slot_key,id`, tenant, scope.IDs("output"))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		v := dr.ScopeOption{Kind: "output", Visibility: "full"}
		if e = rows.Scan(&v.ID, &v.Code, &v.Name); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
