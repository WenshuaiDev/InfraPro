package devices

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

func insertOutput(ctx context.Context, tx pgx.Tx, auth k.Authorization, d Device, key, name, description string, semantic c.Semantics, t time.Time) (Output, error) {
	var id string
	semantic.Normalize()
	e := tx.QueryRow(ctx, `INSERT INTO device_outputs(tenant_id,device_id,product_version_id,slot_key,generation,name,description,metric_version_id,unit_version_id,value_shape,timestamp_meaning,aggregation_description,observation_direction,reference_basis,semantic_hash,started_at,created_by,updated_by) SELECT $1,$2,$3,$4,COALESCE(MAX(generation),0)+1,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16 FROM device_outputs WHERE tenant_id=$1 AND device_id=$2 AND slot_key=$4 RETURNING id::text`, auth.Tenant, d.ID, d.ProductVersionID, key, name, description, semantic.MetricVersionID, semantic.UnitVersionID, semantic.ValueShape, semantic.TimestampMeaning, semantic.AggregationDescription, semantic.ObservationDirection, semantic.ReferenceBasis, semantic.Hash(), t, auth.Account).Scan(&id)
	if e != nil {
		return Output{}, e
	}
	return loadOutput(ctx, tx, auth.Tenant, d.ID, id)
}
func outputFields(in *OutputFields) error {
	in.Name = k.Text(in.Name)
	in.Description = k.Text(in.Description)
	in.AggregationDescription = k.Text(in.AggregationDescription)
	in.ObservationDirection = k.Text(in.ObservationDirection)
	in.ReferenceBasis = k.Text(in.ReferenceBasis)
	if e := k.String(in.Name, "/name", 200, true); e != nil {
		return e
	}
	return k.String(in.Description, "/description", 2000, false)
}
func resolveOutputFields(ctx context.Context, tx pgx.Tx, auth k.Authorization, slot c.Slot, in OutputFields, old *Output) (c.Semantics, error) {
	semantic := c.Semantics{MetricVersionID: in.MetricVersionID, UnitVersionID: in.UnitVersionID, TimestampMeaning: in.TimestampMeaning, AggregationDescription: in.AggregationDescription, ObservationDirection: in.ObservationDirection, ReferenceBasis: in.ReferenceBasis}
	newUse := old == nil || old.MetricVersionID != semantic.MetricVersionID || value(old.UnitVersionID) != value(semantic.UnitVersionID)
	if slot.Mode == "fixed" {
		if in.FixedCorePresent || in.MetricVersionID != "" || in.UnitVersionID != nil || in.TimestampMeaning != "" || in.AggregationDescription != "" {
			return semantic, k.Problem("invalid_request", "固定输出只能修改名称、说明与现场方向／参考含义。", 400)
		}
		semantic.MetricVersionID = slot.MetricVersionID
		semantic.UnitVersionID = slot.UnitVersionID
		semantic.TimestampMeaning = slot.TimestampMeaning
		semantic.AggregationDescription = slot.AggregationDescription
		newUse = false
	} else if newUse {
		if e := auth.Require(access.TenantEngineeringCatalogRead); e != nil {
			return semantic, e
		}
	}
	semantic, _, e := c.ResolveSemanticsTx(ctx, tx, semantic, newUse)
	if e != nil {
		return semantic, e
	}
	if slot.Mode == "configurable" && !slices.Contains(slot.AllowedShapes, semantic.ValueShape) {
		return semantic, k.Invalid("/metric_version_id", "shape_not_allowed", "所选指标的值形态不在此槽位允许范围内。")
	}
	return semantic, nil
}
func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func (s *Service) ConfigureOutput(ctx context.Context, a access.Actor, scope Scope, device string, in OutputConfigure, meta access.Meta) (Result, error) {
	var out Result
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	if e := outputFields(&in.OutputFields); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "output", "create", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("output", "create")
		input := []any{device, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		d, e := loadDevice(ctx, tx, auth.Tenant, device, false)
		if e != nil {
			return e
		}
		if e = k.Expect(in.OutputRevision, d.OutputRevision); e != nil {
			return e
		}
		slot, e := slotTx(ctx, tx, d, in.SlotKey)
		if e != nil {
			return e
		}
		if slot.Mode != "configurable" {
			return k.Problem("invalid_request", "固定输出由设备建档自动生成。", 400)
		}
		var occupied bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM device_outputs WHERE tenant_id=$1 AND device_id=$2 AND slot_key=$3 AND ended_at IS NULL)`, auth.Tenant, device, in.SlotKey).Scan(&occupied); e != nil {
			return e
		}
		if occupied {
			return k.Problem("revision_conflict", "槽位已有输出，请使用更改测量含义。", 409)
		}
		semantic, e := resolveOutputFields(ctx, tx, auth, slot, in.OutputFields, nil)
		if e != nil {
			return e
		}
		var t time.Time
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&t); e != nil {
			return e
		}
		output, e := insertOutput(ctx, tx, auth, d, slot.Key, in.Name, in.Description, semantic, t)
		if e != nil {
			return e
		}
		out = Result{ID: device, OutputID: output.ID, Revision: output.Revision, Changed: true}
		e = tx.QueryRow(ctx, `UPDATE devices SET output_revision=output_revision+1 WHERE id=$1 RETURNING output_revision::text`, device).Scan(&out.OutputRevision)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, in.Meta, op, input, "device_output", output.ID, device, output.ID, "", nil, output, []string{"outputs"}, out, meta)
	})
	return out, e
}
func outputPreviewTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, device, id, action string, in OutputReplace) (Preview, Device, Output, c.Semantics, error) {
	var preview Preview
	var semantic c.Semantics
	var old Output
	if e := auth.Require(access.TenantDeviceOutputManage); e != nil {
		return preview, Device{}, old, semantic, e
	}
	d, e := loadDevice(ctx, tx, auth.Tenant, device, false)
	if e != nil {
		return preview, d, old, semantic, e
	}
	if e = k.Expect(in.OutputRevision, d.OutputRevision); e != nil {
		return preview, d, old, semantic, e
	}
	old, e = loadOutput(ctx, tx, auth.Tenant, device, id)
	if e != nil {
		return preview, d, old, semantic, e
	}
	if e = k.Expect(in.Revision, old.Revision); e != nil {
		return preview, d, old, semantic, e
	}
	if old.EndedAt != nil {
		return preview, d, old, semantic, k.Problem("output_ended", "该输出代次已结束。", 409)
	}
	if action == "end" && old.Template.Mode != "configurable" {
		return preview, d, old, semantic, k.Problem("invalid_request", "固定槽位不支持清空。", 400)
	}
	if action == "replace" {
		semantic, e = resolveOutputFields(ctx, tx, auth, old.Template, in.OutputFields, &old)
		if e != nil {
			return preview, d, old, semantic, e
		}
	}
	impact, e := impactTx(ctx, tx, auth.Tenant, device)
	if e != nil {
		return preview, d, old, semantic, e
	}
	in.PreviewHash = ""
	preview = k.NewPreview([]any{device, id, action, in}, []any{auth.Scope, auth.Account, auth.TenantMembership, d, old, impact.Hash}, map[string]any{"before": old, "new_semantics": semantic, "new_output_does_not_inherit_grants": true}, true, impact.Shared)
	if old.Occupied {
		preview.CanSubmit = false
		preview.Blockers = append(preview.Blockers, Issue{Path: "/output_id", Code: "output_in_use", Message: "输出仍被当前来源使用，请先明确解除或换源。"})
	}
	return preview, d, old, semantic, nil
}
func (s *Service) PreviewOutput(ctx context.Context, a access.Actor, scope Scope, device, id, action string, in OutputReplace) (Preview, error) {
	var out Preview
	if action != "replace" && action != "end" {
		return out, k.Problem("invalid_request", "不支持的输出动作。", 400)
	}
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	if e := in.RequireReason(); e != nil {
		return out, e
	}
	if action == "replace" {
		if e := outputFields(&in.OutputFields); e != nil {
			return out, e
		}
	}
	e := s.transact(ctx, a, scope, "device", "preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, _, _, _, e = outputPreviewTx(ctx, tx, auth, device, id, action, in)
		return e
	})
	return out, e
}
func (s *Service) ReplaceOutput(ctx context.Context, a access.Actor, scope Scope, device, id, action string, in OutputReplace, meta access.Meta) (Result, error) {
	var out Result
	if action != "replace" && action != "end" {
		return out, k.Problem("invalid_request", "不支持的输出动作。", 400)
	}
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	if e := in.RequireReason(); e != nil {
		return out, e
	}
	if action == "replace" {
		if e := outputFields(&in.OutputFields); e != nil {
			return out, e
		}
	}
	hash := in.PreviewHash
	in.PreviewHash = ""
	e := s.transact(ctx, a, scope, "output", action, true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("output", action)
		input := []any{device, id, action, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		preview, d, old, semantic, e := outputPreviewTx(ctx, tx, auth, device, id, action, in)
		if e != nil {
			return e
		}
		m := in.Meta
		m.PreviewHash = hash
		if e = preview.Check(m); e != nil {
			return e
		}
		var t time.Time
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&t); e != nil {
			return e
		}
		if !t.After(old.StartedAt) {
			return k.Problem("effective_time_conflict", "服务器时间未晚于输出开始时间。", 409)
		}
		if _, e = tx.Exec(ctx, `UPDATE device_outputs SET ended_at=$2,revision=revision+1,updated_by=$3 WHERE id=$1`, id, t, a.ID); e != nil {
			return e
		}
		out = Result{ID: device, OutputID: id, Changed: true}
		var after any = map[string]any{"ended_at": t}
		if action == "replace" {
			output, e := insertOutput(ctx, tx, auth, d, old.SlotKey, in.Name, in.Description, semantic, t)
			if e != nil {
				return e
			}
			out.OutputID = output.ID
			out.Revision = output.Revision
			after = output
		} else {
			if e = tx.QueryRow(ctx, `SELECT revision::text FROM device_outputs WHERE id=$1`, id).Scan(&out.Revision); e != nil {
				return e
			}
		}
		e = tx.QueryRow(ctx, `UPDATE devices SET output_revision=output_revision+1 WHERE id=$1 RETURNING output_revision::text`, device).Scan(&out.OutputRevision)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, m, op, input, "device_output", out.OutputID, device, id, "", old, after, []string{"outputs", "semantics"}, out, meta)
	})
	return out, e
}
func (s *Service) UpdateOutput(ctx context.Context, a access.Actor, scope Scope, device, id string, in OutputUpdate, meta access.Meta) (Result, error) {
	var out Result
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	if in.Name == nil && in.Description == nil {
		return out, k.Problem("invalid_request", "请提供名称或说明。", 400)
	}
	if in.Name != nil {
		*in.Name = k.Text(*in.Name)
		if e := k.String(*in.Name, "/name", 200, true); e != nil {
			return out, e
		}
	}
	if in.Description != nil {
		*in.Description = k.Text(*in.Description)
		if e := k.String(*in.Description, "/description", 2000, false); e != nil {
			return out, e
		}
	}
	e := s.transact(ctx, a, scope, "output", "update", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("output", "update")
		input := []any{device, id, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		old, e := loadOutput(ctx, tx, auth.Tenant, device, id)
		if e != nil {
			return e
		}
		if e = k.Expect(in.Revision, old.Revision); e != nil {
			return e
		}
		if old.EndedAt != nil {
			return k.Problem("output_ended", "历史输出只读。", 409)
		}
		after := old
		if in.Name != nil {
			after.Name = *in.Name
		}
		if in.Description != nil {
			after.Description = *in.Description
		}
		out = Result{ID: device, OutputID: id, Revision: old.Revision, Changed: after.Name != old.Name || after.Description != old.Description}
		if !out.Changed {
			return k.SaveReceipt(ctx, tx, auth, in.Meta, op, input, "device_output", id, out)
		}
		e = tx.QueryRow(ctx, `UPDATE device_outputs SET name=$2,description=$3,revision=revision+1,updated_by=$4 WHERE id=$1 RETURNING revision::text`, id, after.Name, after.Description, a.ID).Scan(&out.Revision)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, in.Meta, op, input, "device_output", id, device, id, "", old, after, []string{"name", "description"}, out, meta)
	})
	return out, e
}

// CapturePresence preserves whether a fixed-only field was explicitly supplied,
// including empty strings or null, after strict transport decoding.
func (in *OutputFields) CapturePresence(raw []byte) {
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &fields) != nil {
		return
	}
	for _, key := range []string{"metric_version_id", "unit_version_id", "timestamp_meaning", "aggregation_description"} {
		if _, ok := fields[key]; ok {
			in.FixedCorePresent = true
		}
	}
}
