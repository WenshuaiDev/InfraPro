package sources

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

func normalize(in *Command, scope Scope) error {
	if e := in.Meta.Normalize(); e != nil {
		return e
	}
	if in.Action != "bind" && in.Action != "switch" && in.Action != "unbind" {
		return k.Problem("invalid_request", "不支持的来源动作。", 400)
	}
	if !k.Revision(in.SourceRevision, true) || !k.Revision(in.ChannelRevision, false) {
		return k.Invalid("/expected_source_revision", "revision", "请提供当前来源及通道修订号。")
	}
	if in.Action != "unbind" {
		if !access.ValidID(in.OutputID) || !k.Revision(in.NewOutputRevision, false) || !k.Revision(in.NewDeviceRevision, false) {
			return k.Invalid("/output_id", "revision", "请提供新输出身份和两项修订号。")
		}
	} else if in.OutputID != "" || in.NewOutputRevision != "" || in.NewDeviceRevision != "" {
		return k.Problem("invalid_request", "解除来源不接受新输出字段。", 400)
	}
	if in.Action != "bind" {
		if !access.ValidID(in.BindingID) || !k.Revision(in.BindingRevision, false) || !k.Revision(in.OldOutputRevision, false) || !k.Revision(in.OldDeviceRevision, false) {
			return k.Invalid("/expected_binding_id", "revision", "请提供当前绑定身份及旧端修订号。")
		}
		if e := in.RequireReason(); e != nil {
			return e
		}
	} else if in.BindingID != "" || in.BindingRevision != "" || in.OldOutputRevision != "" || in.OldDeviceRevision != "" {
		return k.Problem("invalid_request", "首次配置不接受旧端字段。", 400)
	}
	if scope.Entry == "project" {
		if !k.Revision(in.ResourceAccessRevision, false) || !k.Revision(in.LinkRevision, false) || !k.Revision(in.DeviceAccessRevision, false) {
			return k.Invalid("/expected_device_access_revision", "revision", "请提供当前项目的两类授权及设施关联修订。")
		}
	} else if in.ResourceAccessRevision != "" || in.LinkRevision != "" || in.DeviceAccessRevision != "" {
		return k.Problem("invalid_request", "租户入口不接受项目授权修订。", 400)
	}
	return nil
}

type prepared struct {
	Preview         Preview
	Old             *Binding
	OldOutput       *devices.Output
	NewOutput       *devices.Output
	Semantic        c.Semantics
	Revision        string
	ChannelRevision string
}

func (s *Service) authorizeCommandTargets(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, target Target, in Command) error {
	if _, _, e := s.channelTx(ctx, tx, a, scope, target, true); e != nil {
		return e
	}
	if in.OutputID != "" {
		if _, e := s.devices.ResolveOutputTx(ctx, tx, a, scope, in.OutputID, true); e != nil {
			return e
		}
	}
	if in.BindingID != "" {
		b, e := bindingTx(ctx, tx, scope, target, in.BindingID)
		if e != nil {
			return e
		}
		if _, e = s.devices.ResolveOutputTx(ctx, tx, a, scope, b.OutputID, true); e != nil {
			return e
		}
	}
	return nil
}
func sourceImpactTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, target Target, outputs []string) (bool, string, error) {
	var shared bool
	var hash string
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE g.tenant_id=$1 AND g.channel_id=$2 AND g.ended_at IS NULL AND ($3::uuid IS NULL OR g.project_id<>$3)),
 md5(jsonb_build_array(
 (SELECT jsonb_agg(jsonb_build_array(to_jsonb(g),l.status,l.revision,p.status,p.version,s.revision,ds.revision) ORDER BY g.id) FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id JOIN projects p ON p.id=g.project_id JOIN project_resource_policies s ON s.project_id=p.id JOIN project_device_access_states ds ON ds.project_id=p.id WHERE g.tenant_id=$1 AND g.channel_id=$2),
 (SELECT jsonb_agg(jsonb_build_array(to_jsonb(g),p.status,p.version,s.revision) ORDER BY g.id) FROM project_device_grants g JOIN projects p ON p.id=g.project_id JOIN project_device_access_states s ON s.project_id=p.id WHERE g.tenant_id=$1 AND g.output_id=ANY($4::uuid[])),
 (SELECT jsonb_agg(to_jsonb(b) ORDER BY b.id) FROM monitoring_source_bindings b WHERE b.tenant_id=$1 AND (b.channel_id=$2 OR b.output_id=ANY($4::uuid[]))),
 (SELECT jsonb_agg(to_jsonb(i) ORDER BY i.id) FROM device_installations i WHERE i.tenant_id=$1 AND i.device_id IN(SELECT device_id FROM device_outputs WHERE id=ANY($4::uuid[]))))::text)`, auth.Tenant, target.ChannelID, k.Nullable(auth.Project), outputs).Scan(&shared, &hash)
	return shared, hash, e
}
func (s *Service) prepare(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, target Target, in Command, write bool) (prepared, error) {
	var out prepared
	channel, semantic, e := s.channelTx(ctx, tx, a, auth.Scope, target, true)
	if e != nil {
		return out, e
	}
	out.Semantic = semantic
	out.ChannelRevision = strconv.Itoa(channel.Resource.Revision)
	old, revision, e := currentBindingTx(ctx, tx, auth.Scope, target)
	if e != nil {
		return out, e
	}
	out.Old = old
	out.Revision = revision
	if e = k.Expect(in.SourceRevision, revision); e != nil {
		return out, e
	}
	if e = k.Expect(in.ChannelRevision, out.ChannelRevision); e != nil {
		return out, e
	}
	if auth.Entry == "project" && (in.ResourceAccessRevision != auth.ResourceAccessRevision || in.DeviceAccessRevision != auth.DeviceAccessRevision || in.LinkRevision != channel.LinkRevision) {
		return out, k.Problem("access_revision_conflict", "项目授权已变化，请重新读取并预览。", 409)
	}
	if in.Action == "bind" && old != nil {
		return out, k.Problem("source_channel_occupied", "通道已有来源，请使用更换来源。", 409)
	}
	if in.Action != "bind" && (old == nil || old.ID != in.BindingID || old.Revision != in.BindingRevision) {
		return out, k.Problem("revision_conflict", "当前来源已变化，请重新核对。", 409)
	}
	ids := []string{}
	if old != nil {
		ids = append(ids, old.OutputID)
	}
	if in.OutputID != "" {
		ids = append(ids, in.OutputID)
	}
	lock := " FOR SHARE"
	if write {
		lock = " FOR UPDATE"
	}
	if _, e = tx.Exec(ctx, `SELECT id FROM devices WHERE tenant_id=$1 AND id IN(SELECT device_id FROM device_outputs WHERE id=ANY($2::uuid[])) ORDER BY id`+lock, auth.Tenant, ids); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `SELECT id FROM device_outputs WHERE tenant_id=$1 AND id=ANY($2::uuid[]) ORDER BY id`+lock, auth.Tenant, ids); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `SELECT channel_id FROM monitoring_source_heads WHERE tenant_id=$1 AND channel_id=$2`+lock, auth.Tenant, target.ChannelID); e != nil {
		return out, e
	}
	if old != nil {
		v, e := s.devices.ResolveOutputTx(ctx, tx, a, auth.Scope, old.OutputID, true)
		if e != nil {
			return out, e
		}
		out.OldOutput = &v
		if e = k.Expect(in.OldOutputRevision, v.Revision); e != nil {
			return out, e
		}
		if e = k.Expect(in.OldDeviceRevision, v.DeviceRevision); e != nil {
			return out, e
		}
	}
	issues := []Issue{}
	if in.Action != "unbind" {
		if channel.Retired {
			return out, k.Problem("target_retired", "通道或其祖先已退役，不能建立新来源。", 409)
		}
		v, e := s.devices.ResolveOutputTx(ctx, tx, a, auth.Scope, in.OutputID, true)
		if e != nil {
			return out, e
		}
		out.NewOutput = &v
		if e = k.Expect(in.NewOutputRevision, v.Revision); e != nil {
			return out, e
		}
		if e = k.Expect(in.NewDeviceRevision, v.DeviceRevision); e != nil {
			return out, e
		}
		if old != nil && old.OutputID == in.OutputID {
			return out, k.Invalid("/output_id", "same_output", "新旧输出相同，无需换源。")
		}
		if v.DeviceStatus != "enabled" {
			return out, k.Problem("device_disabled", "来源设备已停用。", 409)
		}
		if v.EndedAt != nil {
			return out, k.Problem("output_ended", "该输出代次已结束。", 409)
		}
		if v.Occupied {
			return out, k.Problem("source_output_occupied", "该输出当前已被使用。", 409)
		}
		issues = c.Compatible(v.Semantics, semantic)
	}
	shared, hash, e := sourceImpactTx(ctx, tx, auth, target, ids)
	if e != nil {
		return out, e
	}
	in.PreviewHash = ""
	out.Preview = k.NewPreview([]any{target, in}, []any{auth.Scope, auth.Account, auth.TenantMembership, auth.ProjectMembership, auth.ResourceAccessRevision, auth.DeviceAccessRevision, channel, out.Old, out.OldOutput, out.NewOutput, hash}, map[string]any{"channel_id": target.ChannelID, "old_output": out.OldOutput, "new_output": out.NewOutput, "effective": "immediate"}, true, shared)
	if len(issues) > 0 {
		out.Preview.CanSubmit = false
		out.Preview.Blockers = issues
	}
	if shared && in.Reason == "" {
		out.Preview.CanSubmit = false
		out.Preview.Blockers = append(out.Preview.Blockers, Issue{Path: "/reason", Code: "required", Message: "此次变更影响共享配置，请填写原因。"})
	}
	return out, nil
}
func (s *Service) Preview(ctx context.Context, a access.Actor, scope Scope, target Target, in Command) (Preview, error) {
	var out Preview
	if e := normalize(&in, scope); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "source", "preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		p, e := s.prepare(ctx, tx, a, auth, target, in, false)
		out = p.Preview
		return e
	})
	return out, e
}
func (s *Service) Mutate(ctx context.Context, a access.Actor, scope Scope, target Target, in Command, meta access.Meta) (Result, error) {
	var out Result
	if e := normalize(&in, scope); e != nil {
		return out, e
	}
	hash := in.PreviewHash
	in.PreviewHash = ""
	e := s.transact(ctx, a, scope, "source", in.Action, true, func(tx pgx.Tx, auth k.Authorization) error {
		op := scope.Operation("source", in.Action)
		input := []any{target, in}
		// Replays still require both endpoints and the original membership identity.
		if e := s.authorizeCommandTargets(ctx, tx, a, scope, target, in); e != nil {
			return e
		}
		result, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = result
			return nil
		}
		p, e := s.prepare(ctx, tx, a, auth, target, in, true)
		if e != nil {
			return e
		}
		m := in.Meta
		m.PreviewHash = hash
		if !p.Preview.CanSubmit && len(p.Preview.Blockers) > 0 && p.Preview.Blockers[0].Code != "required" {
			return &Error{Code: "source_incompatible", Message: "输出与通道含义不兼容。", Status: 422, Issues: p.Preview.Blockers}
		}
		if e = p.Preview.Check(m); e != nil {
			return e
		}
		var t time.Time
		if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&t); e != nil {
			return e
		}
		if p.Old != nil && !t.After(p.Old.StartedAt) || p.NewOutput != nil && t.Before(p.NewOutput.StartedAt) {
			return k.Problem("effective_time_conflict", "服务器当前时间不能形成有效的来源期间，请核对后重试。", 409)
		}
		if _, e = tx.Exec(ctx, `INSERT INTO monitoring_source_heads(tenant_id,facility_id,point_id,channel_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, auth.Tenant, target.FacilityID, target.PointID, target.ChannelID); e != nil {
			return e
		}
		if p.Old != nil {
			if _, e = tx.Exec(ctx, `UPDATE monitoring_source_bindings SET ended_at=$2,ended_by=$3,ended_reason=$4,revision=revision+1 WHERE id=$1`, p.Old.ID, t, a.ID, in.Reason); e != nil {
				return e
			}
		}
		out = Result{ID: target.ChannelID, Changed: true}
		if p.NewOutput != nil {
			o := p.NewOutput
			e = tx.QueryRow(ctx, `INSERT INTO monitoring_source_bindings(tenant_id,device_id,output_id,facility_id,point_id,channel_id,started_at,semantic_hash,output_snapshot,channel_snapshot,created_actor_scope,created_project_id,created_by,created_reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id::text,revision::text`, auth.Tenant, o.DeviceID, o.ID, target.FacilityID, target.PointID, target.ChannelID, t, p.Semantic.Hash(), k.JSON(map[string]any{"output_id": o.ID, "device": o.Device, "slot_key": o.SlotKey, "name": o.Name, "semantics": o.Semantics}), k.JSON(map[string]any{"target": target, "revision": p.ChannelRevision, "semantics": p.Semantic}), auth.Entry, k.Nullable(auth.Project), a.ID, in.Reason).Scan(&out.BindingID, &out.Revision)
			if e != nil {
				return e
			}
			out.OutputID = o.ID
		} else {
			out.BindingID = p.Old.ID
			out.OutputID = p.Old.OutputID
			if e = tx.QueryRow(ctx, `SELECT revision::text FROM monitoring_source_bindings WHERE id=$1`, out.BindingID).Scan(&out.Revision); e != nil {
				return e
			}
		}
		e = tx.QueryRow(ctx, `UPDATE monitoring_source_heads SET revision=revision+1 WHERE tenant_id=$1 AND channel_id=$2 RETURNING revision::text`, auth.Tenant, target.ChannelID).Scan(&out.SourceRevision)
		if e != nil {
			return e
		}
		var after Binding
		after, e = bindingTx(ctx, tx, scope, target, out.BindingID)
		if e != nil {
			return e
		}
		outputs := []*devices.Output{}
		if p.OldOutput != nil {
			outputs = append(outputs, p.OldOutput)
		}
		if p.NewOutput != nil {
			outputs = append(outputs, p.NewOutput)
		}
		for _, o := range outputs {
			_, e = tx.Exec(ctx, `INSERT INTO device_resource_changes(tenant_id,device_id,output_id,binding_id,operation,actor_id,actor_name,actor_scope,actor_project_id,tenant_membership_id,project_membership_id,request_id,reason,before,after,changed_fields) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'["source"]')`, auth.Tenant, o.DeviceID, o.ID, out.BindingID, string(op), auth.Account, auth.Name, auth.Entry, k.Nullable(auth.Project), auth.TenantMembership, k.Nullable(auth.ProjectMembership), in.RequestID, in.Reason, k.JSON(p.Old), k.JSON(after))
			if e != nil {
				return e
			}
		}
		if e = k.Audit(ctx, tx, auth, m, string(op), "monitoring_source", out.BindingID, out, meta); e != nil {
			return e
		}
		return k.SaveReceipt(ctx, tx, auth, m, op, input, "monitoring_source", out.BindingID, out)
	})
	return out, e
}
