package devices

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
)

func (s *Service) accessTransact(ctx context.Context, a access.Actor, tenant, action string, write bool, fn func(pgx.Tx, k.Authorization) error) error {
	scope := TenantScope(tenant)
	return k.Transact(ctx, s.access, a, scope, scope.Operation("access", action), k.Locks{Governance: true, TenantWrite: write}, fn)
}
func grantStateTx(ctx context.Context, tx pgx.Tx, tenant, project string) (GrantState, error) {
	var out GrantState
	if !access.ValidID(project) {
		return out, access.ErrProjectNotFound
	}
	e := tx.QueryRow(ctx, `SELECT p.id::text,p.name,p.status,p.version::text,s.revision::text FROM projects p JOIN project_device_access_states s ON s.project_id=p.id WHERE p.tenant_id=$1 AND p.id=$2`, tenant, project).Scan(&out.ProjectID, &out.ProjectName, &out.ProjectStatus, &out.ProjectRevision, &out.AccessRevision)
	if e == pgx.ErrNoRows {
		return out, access.ErrProjectNotFound
	}
	return out, e
}

const grantJSON = `to_jsonb(g)||jsonb_build_object('revision',g.revision::text,'name',CASE WHEN g.kind='device' THEN d.name ELSE o.name END,'device_name',d.name,'device_code',d.code)`
const grantFrom = ` FROM project_device_grants g JOIN devices d ON d.id=g.device_id LEFT JOIN device_outputs o ON o.id=g.output_id `

func normalizeGrants(in *GrantChangeSet) error {
	if e := in.Meta.Normalize(); e != nil {
		return e
	}
	if e := in.RequireReason(); e != nil {
		return e
	}
	if !k.Revision(in.AccessRevision, false) || !k.Revision(in.ProjectRevision, false) {
		return k.Invalid("/expected_device_access_revision", "revision", "请提供当前项目和设备授权修订。")
	}
	if len(in.Add)+len(in.Update)+len(in.End) > 1000 || len(in.Add)+len(in.Update)+len(in.End) == 0 {
		return k.Invalid("/add", "input_limit", "每次提交 1–1,000 个明确对象。")
	}
	seen := map[string]bool{}
	for _, v := range in.Add {
		if !slices.Contains([]string{"device", "output"}, v.Kind) || !access.ValidID(v.ID) || v.Kind == "device" && v.CanUseSource {
			return k.Invalid("/add", "target", "请选择设备档案或具体输出；设备档案不含来源使用权。")
		}
		key := v.Kind + ":" + v.ID
		if seen[key] {
			return k.Invalid("/add", "duplicate", "同一对象不能重复。")
		}
		seen[key] = true
	}
	for _, v := range in.Update {
		if !access.ValidID(v.ID) || !k.Revision(v.Revision, false) || seen[v.ID] {
			return k.Invalid("/update", "grant", "授权身份、修订无效或重复。")
		}
		seen[v.ID] = true
	}
	for _, v := range in.End {
		if !access.ValidID(v.ID) || !k.Revision(v.Revision, false) || seen[v.ID] {
			return k.Invalid("/end", "grant", "授权身份、修订无效或重复。")
		}
		seen[v.ID] = true
	}
	return nil
}

type grantPlan struct {
	Preview                  Preview
	State                    GrantState
	Add, Update, End, Before []Grant
}

func planGrants(ctx context.Context, tx pgx.Tx, auth k.Authorization, project string, in GrantChangeSet) (grantPlan, error) {
	out := grantPlan{Add: []Grant{}, Update: []Grant{}, End: []Grant{}, Before: []Grant{}}
	state, e := grantStateTx(ctx, tx, auth.Tenant, project)
	if e != nil {
		return out, e
	}
	out.State = state
	if state.AccessRevision != in.AccessRevision || state.ProjectRevision != in.ProjectRevision {
		return out, k.Problem("access_revision_conflict", "项目或设备授权已变化，请重新读取。", 409)
	}
	ids := []string{}
	for _, g := range in.Update {
		ids = append(ids, g.ID)
	}
	for _, g := range in.End {
		ids = append(ids, g.ID)
	}
	rows, e := tx.Query(ctx, `SELECT `+grantJSON+grantFrom+` WHERE g.tenant_id=$1 AND g.project_id=$2 AND g.id=ANY($3::uuid[]) AND g.ended_at IS NULL`, auth.Tenant, project, ids)
	if e != nil {
		return out, e
	}
	current := map[string]Grant{}
	for rows.Next() {
		var raw []byte
		var g Grant
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return out, e
		}
		if e = json.Unmarshal(raw, &g); e != nil {
			rows.Close()
			return out, e
		}
		current[g.ID] = g
		out.Before = append(out.Before, g)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}

	deviceIDs, outputIDs := []string{}, []string{}
	for _, v := range in.Add {
		if v.Kind == "device" {
			deviceIDs = append(deviceIDs, v.ID)
		} else {
			outputIDs = append(outputIDs, v.ID)
		}
	}
	additions := map[string]Grant{}
	already := map[string]bool{}
	rows, e = tx.Query(ctx, `SELECT 'device',d.id::text,d.id::text,NULL::text,d.name,d.name,d.code,EXISTS(SELECT 1 FROM project_device_grants g WHERE g.tenant_id=d.tenant_id AND g.project_id=$2 AND g.kind='device' AND g.device_id=d.id AND g.ended_at IS NULL) FROM devices d WHERE d.tenant_id=$1 AND d.id=ANY($3::uuid[]) AND d.deleted_at IS NULL
 UNION ALL SELECT 'output',o.id::text,d.id::text,o.id::text,o.name,d.name,d.code,EXISTS(SELECT 1 FROM project_device_grants g WHERE g.tenant_id=o.tenant_id AND g.project_id=$2 AND g.kind='output' AND g.output_id=o.id AND g.ended_at IS NULL) FROM device_outputs o JOIN devices d ON d.id=o.device_id WHERE o.tenant_id=$1 AND o.id=ANY($4::uuid[]) AND d.deleted_at IS NULL`, auth.Tenant, project, deviceIDs, outputIDs)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var g Grant
		var id string
		var exists bool
		if e = rows.Scan(&g.Kind, &id, &g.DeviceID, &g.OutputID, &g.Name, &g.DeviceName, &g.DeviceCode, &exists); e != nil {
			rows.Close()
			return out, e
		}
		key := g.Kind + ":" + id
		additions[key] = g
		already[key] = exists
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, v := range in.Add {
		key := v.Kind + ":" + v.ID
		g, ok := additions[key]
		if !ok {
			return out, k.NotFound("device")
		}
		if already[key] {
			return out, k.Invalid("/add", "already_granted", "该对象已有授权，请修改对应授权。")
		}
		if state.ProjectStatus == "archived" {
			return out, k.Problem("project_archived", "已归档项目不能增加设备授权。", 409)
		}
		g.CanUseSource = v.CanUseSource
		out.Add = append(out.Add, g)
	}

	for _, v := range in.Update {
		g, ok := current[v.ID]
		if !ok || g.Revision != v.Revision {
			return out, k.Problem("access_revision_conflict", "授权已变化。", 409)
		}
		if g.Kind == "device" && v.CanUseSource {
			return out, k.Invalid("/update", "capability", "设备档案授权不包含输出使用权。")
		}
		if state.ProjectStatus == "archived" && v.CanUseSource && !g.CanUseSource {
			return out, k.Problem("project_archived", "已归档项目不能扩大输出使用权。", 409)
		}
		if g.CanUseSource != v.CanUseSource {
			g.CanUseSource = v.CanUseSource
			out.Update = append(out.Update, g)
		}
	}
	for _, v := range in.End {
		g, ok := current[v.ID]
		if !ok || g.Revision != v.Revision {
			return out, k.Problem("access_revision_conflict", "授权已变化。", 409)
		}
		out.End = append(out.End, g)
	}
	var graph string
	e = tx.QueryRow(ctx, `SELECT md5(jsonb_build_array((SELECT jsonb_agg(to_jsonb(g) ORDER BY g.id) FROM project_device_grants g WHERE g.tenant_id=$1 AND g.project_id=$2),(SELECT jsonb_agg(to_jsonb(b) ORDER BY b.id) FROM monitoring_source_bindings b WHERE b.tenant_id=$1 AND b.output_id IN(SELECT output_id FROM project_device_grants WHERE tenant_id=$1 AND project_id=$2)))::text)`, auth.Tenant, project).Scan(&graph)
	if e != nil {
		return out, e
	}
	in.PreviewHash = ""
	out.Preview = k.NewPreview([]any{project, in}, []any{auth.Scope, auth.Account, auth.TenantMembership, state, graph, out.Add, out.Update, out.End}, map[string]any{"add": out.Add, "update": out.Update, "end": out.End, "preserves_sources": true, "no_future_outputs": true}, true, true)
	return out, nil
}
func (s *Service) PreviewGrants(ctx context.Context, a access.Actor, tenant, project string, in GrantChangeSet) (Preview, error) {
	var out Preview
	if e := normalizeGrants(&in); e != nil {
		return out, e
	}
	e := s.accessTransact(ctx, a, tenant, "preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		p, e := planGrants(ctx, tx, auth, project, in)
		out = p.Preview
		return e
	})
	return out, e
}
func (s *Service) ChangeGrants(ctx context.Context, a access.Actor, tenant, project string, in GrantChangeSet, meta access.Meta) (Result, error) {
	var out Result
	if e := normalizeGrants(&in); e != nil {
		return out, e
	}
	hash := in.PreviewHash
	in.PreviewHash = ""
	e := s.accessTransact(ctx, a, tenant, "change", true, func(tx pgx.Tx, auth k.Authorization) error {
		op := auth.Operation("access", "change")
		input := []any{project, in}
		v, ok, e := k.Replay(ctx, tx, auth, in.RequestID, op, input)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		p, e := planGrants(ctx, tx, auth, project, in)
		if e != nil {
			return e
		}
		m := in.Meta
		m.PreviewHash = hash
		if e = p.Preview.Check(m); e != nil {
			return e
		}
		out = Result{ID: project, Revision: p.State.ProjectRevision, AccessRevision: p.State.AccessRevision, Changed: len(p.Add)+len(p.Update)+len(p.End) > 0}
		if !out.Changed {
			return k.SaveReceipt(ctx, tx, auth, m, op, input, "project_device_grant_set", project, out)
		}
		after := []Grant{}
		for _, g := range p.Add {
			e = tx.QueryRow(ctx, `INSERT INTO project_device_grants(tenant_id,project_id,kind,device_id,output_id,can_use_source,granted_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text,revision::text,granted_at`, tenant, project, g.Kind, g.DeviceID, g.OutputID, g.CanUseSource, a.ID).Scan(&g.ID, &g.Revision, &g.GrantedAt)
			if e != nil {
				return e
			}
			after = append(after, g)
		}
		for _, g := range p.Update {
			e = tx.QueryRow(ctx, `UPDATE project_device_grants SET can_use_source=$2,revision=revision+1 WHERE id=$1 RETURNING revision::text`, g.ID, g.CanUseSource).Scan(&g.Revision)
			if e != nil {
				return e
			}
			after = append(after, g)
		}
		for _, g := range p.End {
			e = tx.QueryRow(ctx, `UPDATE project_device_grants SET ended_at=clock_timestamp(),ended_by=$2,end_reason=$3,revision=revision+1 WHERE id=$1 RETURNING revision::text,ended_at`, g.ID, a.ID, in.Reason).Scan(&g.Revision, &g.EndedAt)
			if e != nil {
				return e
			}
			after = append(after, g)
		}
		e = tx.QueryRow(ctx, `UPDATE project_device_access_states SET revision=revision+1,updated_at=clock_timestamp(),updated_by=$3 WHERE tenant_id=$1 AND project_id=$2 RETURNING revision::text`, tenant, project, a.ID).Scan(&out.AccessRevision)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO project_device_access_changes(tenant_id,project_id,access_revision,actor_id,actor_name,tenant_membership_id,request_id,reason,instructions,before,after) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, tenant, project, out.AccessRevision, a.ID, auth.Name, auth.TenantMembership, in.RequestID, in.Reason, k.JSON(in), k.JSON(p.Before), k.JSON(after))
		if e != nil {
			return e
		}
		if e = k.Audit(ctx, tx, auth, m, string(op), "project_device_grant_set", project, out, meta); e != nil {
			return e
		}
		return k.SaveReceipt(ctx, tx, auth, m, op, input, "project_device_grant_set", project, out)
	})
	return out, e
}
