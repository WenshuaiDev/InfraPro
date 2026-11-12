package devicecatalog

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func productMutationInput(in *ProductMutation) error {
	if e := in.Meta.Normalize(); e != nil {
		return e
	}
	if e := in.Meta.RequireReason(); e != nil {
		return e
	}
	if in.Action == "state" {
		if in.Enabled == nil || !access.ValidID(in.VersionID) {
			return k.Problem("invalid_request", "版本启停须指定版本和目标状态。", 400)
		}
	} else if in.Action == "delete" {
		if in.Enabled != nil || in.VersionID != "" {
			return k.Problem("invalid_request", "删除身份不接受版本状态字段。", 400)
		}
	} else {
		return k.Problem("invalid_request", "不支持的产品操作。", 400)
	}
	return nil
}
func productPreviewTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, id string, in ProductMutation) (Preview, Product, *Version, error) {
	var out Preview
	p, e := productTx(ctx, tx, auth.Scope, id)
	if e != nil {
		return out, p, nil, e
	}
	permission := access.DeviceProductEdit
	if auth.Entry == "tenant" {
		permission = access.TenantDeviceProductEdit
	}
	if in.Action == "state" {
		permission = access.DeviceProductState
		if auth.Entry == "tenant" {
			permission = access.TenantDeviceProductState
		}
	}
	if e = auth.Require(permission); e != nil {
		return out, p, nil, e
	}
	var v *Version
	var state any
	var refs bool
	var graph string
	if in.Action == "state" {
		version, _, e := ResolveVersionTx(ctx, tx, auth.Tenant, in.VersionID, false)
		if e != nil {
			return out, p, v, e
		}
		if version.ProductID != id {
			return out, p, v, k.NotFound("definition")
		}
		v = &version
		if e = k.Expect(in.Revision, v.Revision); e != nil {
			return out, p, v, e
		}
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE product_version_id=$1),COALESCE((SELECT md5(jsonb_agg(jsonb_build_array(id,revision,status,deleted_at) ORDER BY id)::text) FROM devices WHERE product_version_id=$1),'')`, v.ID).Scan(&refs, &graph)
		if e != nil {
			return out, p, v, e
		}
		state = map[string]any{"version_id": v.ID, "before_enabled": v.Enabled, "enabled": *in.Enabled, "has_references": refs, "existing_devices_unchanged": true}
	} else {
		if e = k.Expect(in.Revision, p.Revision); e != nil {
			return out, p, v, e
		}
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM device_product_versions WHERE product_id=$1)`, id).Scan(&refs)
		if e != nil {
			return out, p, v, e
		}
		state = map[string]any{"product_id": id, "delete": true, "has_references": refs}
	}
	in.PreviewHash = ""
	out = k.NewPreview([]any{id, in}, []any{auth.Scope, auth.Account, auth.TenantMembership, p, v, refs, graph}, state, true, false)
	if in.Action == "delete" && refs {
		out.CanSubmit = false
		out.Blockers = append(out.Blockers, Issue{Path: "/id", Code: "device_referenced", Message: "产品已经发布，请通过停用版本停止新采用。"})
	}
	return out, p, v, nil
}
func (s *Service) PreviewProduct(ctx context.Context, a access.Actor, scope Scope, id string, in ProductMutation) (Preview, error) {
	var out Preview
	if e := productMutationInput(&in); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "product", "preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, _, _, e = productPreviewTx(ctx, tx, auth, id, in)
		return e
	})
	return out, e
}
func (s *Service) MutateProduct(ctx context.Context, a access.Actor, scope Scope, id string, in ProductMutation, meta access.Meta) (Result, error) {
	var out Result
	if e := productMutationInput(&in); e != nil {
		return out, e
	}
	hash := in.PreviewHash
	in.PreviewHash = ""
	e := s.transact(ctx, a, scope, "product", in.Action, true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("product", in.Action)
		input := []any{id, in}
		result, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = result
			return nil
		}
		preview, p, v, e := productPreviewTx(ctx, tx, auth, id, in)
		if e != nil {
			return e
		}
		m := in.Meta
		m.PreviewHash = hash
		if e = preview.Check(m); e != nil {
			return e
		}
		out = Result{ID: id, Revision: p.Revision, Changed: true}
		if in.Action == "state" {
			out.VersionID = v.ID
			out.Revision = v.Revision
			out.Changed = v.Enabled != *in.Enabled
			if out.Changed {
				e = tx.QueryRow(ctx, `UPDATE device_product_version_states SET enabled=$2,revision=revision+1,updated_at=clock_timestamp(),updated_by=$3 WHERE version_id=$1 RETURNING revision::text`, v.ID, *in.Enabled, a.ID).Scan(&out.Revision)
			}
		} else {
			e = tx.QueryRow(ctx, `UPDATE device_products SET deleted_at=clock_timestamp(),revision=revision+1,updated_at=clock_timestamp(),updated_by=$2 WHERE id=$1 RETURNING revision::text`, id, a.ID).Scan(&out.Revision)
			if e == nil {
				_, e = tx.Exec(ctx, `DELETE FROM device_product_drafts WHERE product_id=$1`, id)
			}
		}
		if e != nil {
			return e
		}
		if !out.Changed {
			return k.SaveReceipt(ctx, tx, auth, m, op, input, "product", id, out)
		}
		return complete(ctx, tx, auth, m, op, input, "product", id, p.ManufacturerID, id, out.VersionID, preview.Changes, out, out, meta)
	})
	return out, e
}
