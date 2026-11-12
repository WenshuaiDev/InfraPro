package devices

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) Create(ctx context.Context, a access.Actor, scope Scope, in DeviceCreate, meta access.Meta) (Result, error) {
	var out Result
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	in.Code = k.Text(in.Code)
	in.Name = k.Text(in.Name)
	in.SerialNumber = k.Text(in.SerialNumber)
	in.Description = k.Text(in.Description)
	if in.Status == "" {
		in.Status = "enabled"
	}
	if in.Status != "enabled" && in.Status != "disabled" {
		return out, k.Invalid("/status", "enum", "请选择启用或停用。")
	}
	if !k.Code(in.Code) {
		return out, k.Invalid("/code", "code", "设备编号格式无效。")
	}
	for _, v := range []struct {
		value, path string
		max         int
		required    bool
	}{{in.Name, "name", 200, true}, {in.SerialNumber, "serial_number", 200, false}, {in.Description, "description", 2000, false}} {
		if e := k.String(v.value, "/"+v.path, v.max, v.required); e != nil {
			return out, e
		}
	}
	e := s.transact(ctx, a, scope, "device", "create", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("device", "create")
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, in)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		version, p, e := c.ResolveVersionTx(ctx, tx, scope.Tenant, in.ProductVersionID, true)
		if e != nil {
			return e
		}
		required := access.TenantDeviceCatalogRead
		if p.OwnerTenantID != nil {
			required = access.TenantDeviceDefinitionRead
		}
		if e = auth.Require(required); e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `INSERT INTO devices(tenant_id,code,name,product_id,product_version_id,serial_number,description,created_by,updated_by,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8,$9) RETURNING id::text,revision::text`, scope.Tenant, in.Code, in.Name, p.ID, version.ID, in.SerialNumber, in.Description, a.ID, in.Status).Scan(&out.ID, &out.Revision)
		if e != nil {
			return e
		}
		d, e := loadDevice(ctx, tx, scope.Tenant, out.ID, false)
		if e != nil {
			return e
		}
		var t time.Time
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&t); e != nil {
			return e
		}
		outputs := []Output{}
		for _, slot := range version.Content.OutputSlots {
			if slot.Mode != "fixed" {
				continue
			}
			semantic, _, e := c.ResolveSemanticsTx(ctx, tx, c.Semantics{MetricVersionID: slot.MetricVersionID, UnitVersionID: slot.UnitVersionID, TimestampMeaning: slot.TimestampMeaning, AggregationDescription: slot.AggregationDescription}, true)
			if e != nil {
				return e
			}
			output, e := insertOutput(ctx, tx, auth, d, slot.Key, slot.Name, slot.Description, semantic, t)
			if e != nil {
				return e
			}
			outputs = append(outputs, output)
		}
		out.OutputRevision = "0"
		out.InstallationRevision = "0"
		if len(outputs) > 0 {
			out.OutputRevision = "1"
			if _, e = tx.Exec(ctx, `UPDATE devices SET output_revision=1 WHERE id=$1`, out.ID); e != nil {
				return e
			}
		}
		out.Changed = true
		d.OutputRevision = out.OutputRevision
		return complete(ctx, tx, auth, in.Meta, op, in, "device", out.ID, out.ID, "", "", nil, map[string]any{"device": d, "outputs": outputs}, []string{"device", "outputs"}, out, meta)
	})
	return out, e
}

type impact struct {
	Shared         bool
	Referenced     bool
	CurrentSources bool
	Hash           string
}

// Full retained sets participate in the hash. Only booleans are exposed unless
// a caller performs a separate, currently authorized detail projection.
func impactTx(ctx context.Context, tx pgx.Tx, tenant, device string) (impact, error) {
	var out impact
	e := tx.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM project_device_grants WHERE tenant_id=$1 AND device_id=$2 AND ended_at IS NULL) OR EXISTS(SELECT 1 FROM monitoring_source_bindings b JOIN project_resource_grants g ON g.channel_id=b.channel_id JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE b.tenant_id=$1 AND b.device_id=$2 AND b.ended_at IS NULL AND g.ended_at IS NULL),
 EXISTS(SELECT 1 FROM device_installations WHERE tenant_id=$1 AND device_id=$2) OR EXISTS(SELECT 1 FROM monitoring_source_bindings WHERE tenant_id=$1 AND device_id=$2) OR EXISTS(SELECT 1 FROM project_device_grants WHERE tenant_id=$1 AND device_id=$2),
 EXISTS(SELECT 1 FROM monitoring_source_bindings WHERE tenant_id=$1 AND device_id=$2 AND ended_at IS NULL),
 md5(jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(g) ORDER BY g.id) FROM project_device_grants g WHERE g.tenant_id=$1 AND g.device_id=$2),
 (SELECT jsonb_agg(to_jsonb(b) ORDER BY b.id) FROM monitoring_source_bindings b WHERE b.tenant_id=$1 AND b.device_id=$2),
 (SELECT jsonb_agg(to_jsonb(i) ORDER BY i.id) FROM device_installations i WHERE i.tenant_id=$1 AND i.device_id=$2),
 (SELECT jsonb_agg(jsonb_build_array(to_jsonb(g),l.revision,l.status,p.status,rs.revision,ds.revision) ORDER BY g.id) FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id JOIN projects p ON p.id=g.project_id JOIN project_resource_policies rs ON rs.project_id=p.id JOIN project_device_access_states ds ON ds.project_id=p.id WHERE g.tenant_id=$1 AND g.channel_id IN(SELECT channel_id FROM monitoring_source_bindings WHERE tenant_id=$1 AND device_id=$2)),
 (SELECT jsonb_agg(jsonb_build_array(p.id,p.status,s.revision) ORDER BY p.id) FROM projects p JOIN project_device_access_states s ON s.project_id=p.id WHERE p.tenant_id=$1 AND p.id IN(SELECT project_id FROM project_device_grants WHERE device_id=$2)))::text)`, tenant, device).Scan(&out.Shared, &out.Referenced, &out.CurrentSources, &out.Hash)
	return out, e
}
func normalizeDeviceMutation(in *DeviceMutation) error {
	if e := in.Meta.Normalize(); e != nil {
		return e
	}
	switch in.Action {
	case "update":
		if in.Status != "" {
			return k.Problem("invalid_request", "资料编辑不接受状态字段。", 400)
		}
		if in.Name == nil && in.SerialNumber == nil && in.Description == nil {
			return k.Problem("invalid_request", "请提供修改字段。", 400)
		}
	case "state":
		if in.Status != "enabled" && in.Status != "disabled" || in.Name != nil || in.SerialNumber != nil || in.Description != nil {
			return k.Problem("invalid_request", "设备状态输入无效。", 400)
		}
		return in.Meta.RequireReason()
	case "delete":
		if in.Status != "" || in.Name != nil || in.SerialNumber != nil || in.Description != nil {
			return k.Problem("invalid_request", "删除不接受资料字段。", 400)
		}
		return in.Meta.RequireReason()
	default:
		return k.Problem("invalid_request", "不支持的设备操作。", 400)
	}
	for _, v := range []struct {
		value    *string
		path     string
		max      int
		required bool
	}{{in.Name, "name", 200, true}, {in.SerialNumber, "serial_number", 200, false}, {in.Description, "description", 2000, false}} {
		if v.value != nil {
			*v.value = k.Text(*v.value)
			if e := k.String(*v.value, "/"+v.path, v.max, v.required); e != nil {
				return e
			}
		}
	}
	return nil
}
func devicePreviewTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, id string, in DeviceMutation) (Preview, Device, Device, error) {
	var out Preview
	before, e := loadDevice(ctx, tx, auth.Tenant, id, false)
	if e != nil {
		return out, before, before, e
	}
	if e = k.Expect(in.Revision, before.Revision); e != nil {
		return out, before, before, e
	}
	permission := access.TenantDeviceManage
	if in.Action == "state" {
		permission = access.TenantDeviceState
	}
	if e = auth.Require(permission); e != nil {
		return out, before, before, e
	}
	after := before
	if in.Name != nil {
		after.Name = *in.Name
	}
	if in.Description != nil {
		after.Description = *in.Description
	}
	if in.SerialNumber != nil {
		after.SerialNumber = *in.SerialNumber
	}
	if in.Action == "state" {
		after.Status = in.Status
	}
	impact, e := impactTx(ctx, tx, auth.Tenant, id)
	if e != nil {
		return out, before, after, e
	}
	in.PreviewHash = ""
	out = k.NewPreview([]any{id, in}, []any{auth.Scope, auth.Account, auth.TenantMembership, before, impact.Hash}, map[string]any{"before": before, "after": after, "preserves_sources": impact.CurrentSources}, in.Action != "update" || impact.Shared, impact.Shared)
	if impact.Shared && in.Reason == "" {
		out.CanSubmit = false
		out.Blockers = append(out.Blockers, Issue{Path: "/reason", Code: "required", Message: "共享修改须填写原因。"})
	}
	if in.Action == "delete" && impact.Referenced {
		out.CanSubmit = false
		out.Blockers = append(out.Blockers, Issue{Path: "/id", Code: "device_referenced", Message: "设备存在安装、来源或项目授权履历，请停用保留。"})
	}
	return out, before, after, nil
}
func (s *Service) PreviewDevice(ctx context.Context, a access.Actor, scope Scope, id string, in DeviceMutation) (Preview, error) {
	var out Preview
	if e := normalizeDeviceMutation(&in); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "device", "preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, _, _, e = devicePreviewTx(ctx, tx, auth, id, in)
		return e
	})
	return out, e
}
func (s *Service) Mutate(ctx context.Context, a access.Actor, scope Scope, id string, in DeviceMutation, meta access.Meta) (Result, error) {
	var out Result
	if e := normalizeDeviceMutation(&in); e != nil {
		return out, e
	}
	hash := in.PreviewHash
	in.PreviewHash = ""
	e := s.transact(ctx, a, scope, "device", in.Action, true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("device", in.Action)
		input := []any{id, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		preview, before, after, e := devicePreviewTx(ctx, tx, auth, id, in)
		if e != nil {
			return e
		}
		m := in.Meta
		m.PreviewHash = hash
		if e = preview.Check(m); e != nil {
			return e
		}
		out = Result{ID: id, Revision: before.Revision, Changed: in.Action == "delete" || before.Name != after.Name || before.Description != after.Description || before.SerialNumber != after.SerialNumber || before.Status != after.Status}
		if !out.Changed {
			return k.SaveReceipt(ctx, tx, auth, m, op, input, "device", id, out)
		}
		affected := []string{}
		if in.Action == "delete" {
			var t time.Time
			if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&t); e != nil {
				return e
			}
			rows, e := tx.Query(ctx, `UPDATE device_outputs SET ended_at=$3,revision=revision+1,updated_by=$4 WHERE tenant_id=$1 AND device_id=$2 AND ended_at IS NULL AND started_at<$3 RETURNING id::text`, auth.Tenant, id, t, a.ID)
			if e != nil {
				return e
			}
			for rows.Next() {
				var id string
				if e = rows.Scan(&id); e != nil {
					rows.Close()
					return e
				}
				affected = append(affected, id)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			var remaining bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM device_outputs WHERE tenant_id=$1 AND device_id=$2 AND ended_at IS NULL)`, auth.Tenant, id).Scan(&remaining); e != nil {
				return e
			}
			if remaining {
				return k.Problem("effective_time_conflict", "服务器时间未晚于输出开始时间，请核对后重试。", 409)
			}
		}
		e = tx.QueryRow(ctx, `UPDATE devices SET name=$2,serial_number=$3,description=$4,status=$5,deleted_at=CASE WHEN $6 THEN clock_timestamp() ELSE NULL END,revision=revision+1,output_revision=output_revision+CASE WHEN $7 THEN 1 ELSE 0 END,updated_at=clock_timestamp(),updated_by=$8 WHERE id=$1 RETURNING revision::text,output_revision::text`, id, after.Name, after.SerialNumber, after.Description, after.Status, in.Action == "delete", len(affected) > 0, a.ID).Scan(&out.Revision, &out.OutputRevision)
		if e != nil {
			return e
		}
		fields := []string{}
		if before.Name != after.Name {
			fields = append(fields, "name")
		}
		if before.Description != after.Description {
			fields = append(fields, "description")
		}
		if before.SerialNumber != after.SerialNumber {
			fields = append(fields, "serial_number")
		}
		if before.Status != after.Status {
			fields = append(fields, "status")
		}
		if in.Action == "delete" {
			fields = append(fields, "deleted_at", "outputs")
		}
		return complete(ctx, tx, auth, m, op, input, "device", id, id, "", "", before, map[string]any{"device": after, "ended_output_ids": affected}, fields, out, meta)
	})
	return out, e
}
