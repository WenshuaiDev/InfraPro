package devicecatalog

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func (s *Service) CreateManufacturer(ctx context.Context, a access.Actor, scope Scope, in ManufacturerCreate, meta access.Meta) (Result, error) {
	var out Result
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	in.Code = k.Text(in.Code)
	in.Name = k.Text(in.Name)
	in.Description = k.Text(in.Description)
	if !k.Code(in.Code) {
		return out, k.Invalid("/code", "code", "编号为 1–64 位字母、数字、点、下划线或连字符，首位为字母或数字。")
	}
	if e := k.String(in.Name, "/name", 200, true); e != nil {
		return out, e
	}
	if e := k.String(in.Description, "/description", 2000, false); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "manufacturer", "create", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("manufacturer", "create")
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, in)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		e = tx.QueryRow(ctx, `INSERT INTO device_manufacturers(owner_tenant_id,code,name,description,created_by,updated_by) VALUES($1,$2,$3,$4,$5,$5) RETURNING id::text,revision::text`, owner(scope), in.Code, in.Name, in.Description, a.ID).Scan(&out.ID, &out.Revision)
		if e != nil {
			return e
		}
		out.Changed = true
		return complete(ctx, tx, auth, in.Meta, op, in, "manufacturer", out.ID, out.ID, "", "", nil, in, out, meta)
	})
	return out, e
}
func manufacturerInput(in *ManufacturerMutation) error {
	if e := in.Meta.Normalize(); e != nil {
		return e
	}
	switch in.Action {
	case "update":
		if in.Status != "" {
			return k.Problem("invalid_request", "编辑资料不接受状态字段。", 400)
		}
		if in.Name == nil && in.Description == nil {
			return k.Problem("invalid_request", "请提供需要修改的字段。", 400)
		}
	case "state":
		if in.Name != nil || in.Description != nil || in.Status != "active" && in.Status != "archived" {
			return k.Problem("invalid_request", "厂家状态输入无效。", 400)
		}
		return in.Meta.RequireReason()
	case "delete":
		if in.Name != nil || in.Description != nil || in.Status != "" {
			return k.Problem("invalid_request", "删除不接受资料或状态字段。", 400)
		}
		return in.Meta.RequireReason()
	default:
		return k.Problem("invalid_request", "不支持的厂家操作。", 400)
	}
	if in.Name != nil {
		v := k.Text(*in.Name)
		in.Name = &v
		if e := k.String(v, "/name", 200, true); e != nil {
			return e
		}
	}
	if in.Description != nil {
		v := k.Text(*in.Description)
		in.Description = &v
		if e := k.String(v, "/description", 2000, false); e != nil {
			return e
		}
	}
	return nil
}
func manufacturerPreviewTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, id string, in ManufacturerMutation) (Preview, Manufacturer, Manufacturer, error) {
	var preview Preview
	before, e := manufacturerTx(ctx, tx, auth.Scope, id, false)
	if e != nil {
		return preview, before, before, e
	}
	if e = k.Expect(in.Revision, before.Revision); e != nil {
		return preview, before, before, e
	}
	after := before
	if in.Name != nil {
		after.Name = *in.Name
	}
	if in.Description != nil {
		after.Description = *in.Description
	}
	if in.Action == "state" {
		after.Status = in.Status
	}
	var refs bool
	var graph string
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM device_products WHERE manufacturer_id=$1),COALESCE((SELECT md5(jsonb_agg(jsonb_build_array(id,revision,deleted_at) ORDER BY id)::text) FROM device_products WHERE manufacturer_id=$1),'')`, id).Scan(&refs, &graph)
	if e != nil {
		return preview, before, after, e
	}
	in.PreviewHash = ""
	preview = k.NewPreview([]any{id, in}, []any{auth.Scope, auth.Account, auth.TenantMembership, before, graph}, map[string]any{"before": before, "after": after, "has_references": refs}, in.Action != "update", false)
	if in.Action == "delete" && refs {
		preview.CanSubmit = false
		preview.Blockers = append(preview.Blockers, Issue{Path: "/id", Code: "device_referenced", Message: "存在产品引用，不能删除厂家。"})
	}
	return preview, before, after, nil
}
func (s *Service) PreviewManufacturer(ctx context.Context, a access.Actor, scope Scope, id string, in ManufacturerMutation) (Preview, error) {
	var out Preview
	if e := manufacturerInput(&in); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "manufacturer", "preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, _, _, e = manufacturerPreviewTx(ctx, tx, auth, id, in)
		return e
	})
	return out, e
}
func (s *Service) MutateManufacturer(ctx context.Context, a access.Actor, scope Scope, id string, in ManufacturerMutation, meta access.Meta) (Result, error) {
	var out Result
	if e := manufacturerInput(&in); e != nil {
		return out, e
	}
	hash := in.PreviewHash
	in.PreviewHash = ""
	e := s.transact(ctx, a, scope, "manufacturer", in.Action, true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("manufacturer", in.Action)
		input := []any{id, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		preview, before, after, e := manufacturerPreviewTx(ctx, tx, auth, id, in)
		if e != nil {
			return e
		}
		m := in.Meta
		m.PreviewHash = hash
		if e = preview.Check(m); e != nil {
			return e
		}
		out = Result{ID: id, Revision: before.Revision, Changed: in.Action == "delete" || before.Name != after.Name || before.Description != after.Description || before.Status != after.Status}
		if out.Changed {
			e = tx.QueryRow(ctx, `UPDATE device_manufacturers SET name=$2,description=$3,status=$4,deleted_at=CASE WHEN $5 THEN clock_timestamp() ELSE NULL END,revision=revision+1,updated_at=clock_timestamp(),updated_by=$6 WHERE id=$1 RETURNING revision::text`, id, after.Name, after.Description, after.Status, in.Action == "delete", a.ID).Scan(&out.Revision)
			if e != nil {
				return e
			}
			return complete(ctx, tx, auth, m, op, input, "manufacturer", id, id, "", "", before, after, out, meta)
		}
		return k.SaveReceipt(ctx, tx, auth, m, op, input, "manufacturer", id, out)
	})
	return out, e
}
