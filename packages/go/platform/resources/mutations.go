package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
)

func (s *Service) Mutate(ctx context.Context, a access.Actor, scope Scope, target Target, action string, raw json.RawMessage, meta access.Meta) (Result, error) {
	out, _, e := s.mutate(ctx, a, scope, target, action, raw, meta, false)
	return out, e
}
func authExclusive(t scopedTarget, action string, dryRun bool) bool {
	return t.Entry == "project" && action == "create" && !dryRun
}
func (s *Service) mutate(ctx context.Context, a access.Actor, scope Scope, target Target, action string, raw json.RawMessage, meta access.Meta, dryRun bool) (Result, MutationPreview, error) {
	t := scoped(scope, target)
	out := Result{}
	preview := MutationPreview{CanSubmit: true}
	if e := checkTarget(t); e != nil {
		return out, preview, e
	}
	if !slices.Contains([]string{"create", "update", "retire", "restore", "delete"}, action) {
		return out, preview, problem("invalid_request", "不支持的操作。", 400)
	}
	m, in, e := parse(t, action, raw)
	if e != nil {
		return out, preview, e
	}
	op := t.operation(string(t.Kind), action)
	e = s.transact(ctx, a, op, t.Scope, authExclusive(t, action, dryRun), func(tx pgx.Tx, auth authorization) error {
		if !dryRun {
			var replay bool
			out, replay, e = mutationReceipt(ctx, tx, auth, t, action, m, in)
			if e != nil || replay {
				return e
			}
		}

		r := Resource{Kind: t.Kind, TenantID: t.Tenant, FacilityID: t.Facility, PointID: t.Point, Status: "normal", Revision: 1, CreatedBy: a.ID, UpdatedBy: a.ID}
		var facility Resource
		var before *Resource
		if action != "create" || t.Kind != Facility {
			facility, e = lockFacility(ctx, tx, t, !dryRun, false)
			if e != nil {
				return e
			}
		}
		if action != "create" {
			if auth.Entry == "project" {
				if _, _, e = projectGrantTx(ctx, tx, auth, t, true); e != nil {
					return e
				}
			}
			r, e = load(ctx, tx, t, false)
			if e != nil {
				return e
			}
			if m.Revision < 1 || m.Revision != r.Revision {
				return problem("resource_revision_conflict", "对象已被修改，请重新读取并核对；当前输入已保留。", 409)
			}
			previous := r
			before = &previous
			ancestors, e := ancestorsTx(ctx, tx, t, r, facility)
			if e != nil {
				return e
			}
			if e = writable(r, ancestors, action); e != nil {
				return e
			}
		} else {
			if e = tx.QueryRow(ctx, `SELECT gen_random_uuid()::text,clock_timestamp()`).Scan(&r.ID, &r.CreatedAt); e != nil {
				return e
			}
			r.UpdatedAt = r.CreatedAt
			if t.Kind == Facility {
				r.FacilityID = ""
				r.StructureRevision = 1
			}
		}
		structural := t.Kind == Component && (action != "update" || in["parent_component_id"] != nil || hasKey(in, "parent_component_id"))
		if structural && (m.StructureRevision < 1 || m.StructureRevision != facility.StructureRevision) {
			return problem("structure_revision_conflict", "构件结构已变化，请刷新结构后核对重试。", 409)
		}
		if action == "create" || action == "update" {
			r, e = apply(r, in)
			if e != nil {
				return e
			}
			if e = validateBasic(r); e != nil {
				return e
			}
			if auth.Entry == "project" {
				if _, e = validateProjectWrite(ctx, tx, auth, t, action, m, r, before); e != nil {
					return e
				}
			}
			if e = validateResource(ctx, tx, t, r, facility, action == "create"); e != nil {
				return e
			}
		}
		if action == "retire" {
			r.Status = "retired"
		}
		if action == "restore" {
			r.Status = "normal"
			if t.Kind == Component {
				if e = validateResource(ctx, tx, t, r, facility, false); e != nil {
					return e
				}
			}
		}
		if action == "delete" {
			if e = s.checkDependencies(ctx, tx, t, r); e != nil {
				if e = previewBlock(e, &preview, dryRun); e != nil {
					return e
				}
			}
			if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&r.DeletedAt); e != nil {
				return e
			}
		}
		link, e := validateProjectWrite(ctx, tx, auth, t, action, m, r, before)
		if e != nil {
			if e = previewBlock(e, &preview, dryRun); e != nil {
				return e
			}
		}
		impact, e := s.mutationImpact(ctx, tx, auth, t, action, m, in, before, r)
		if e != nil {
			return e
		}
		impact.CanSubmit = preview.CanSubmit
		impact.BlockedCode = preview.BlockedCode
		impact.BlockedReason = preview.BlockedReason
		impact.Dependencies = preview.Dependencies
		preview = impact
		if dryRun {
			return nil
		}
		if preview.Required {
			if m.PreviewHash == "" {
				return problem("preview_required", "请先预览并确认本次修改影响。", 409)
			}
			if m.PreviewHash != preview.Hash {
				return previewChanged()
			}
			if m.Reason == "" {
				return invalid("/reason", "required", "请填写本次共享或结构修改的原因。")
			}
		}
		if action == "create" && auth.Entry == "project" {
			r.CreatedProjectID = &auth.Project
		}
		changed := before == nil || fingerprint(*before) != fingerprint(r)
		if changed {
			if before != nil {
				r.Revision++
				r.UpdatedBy = a.ID
				if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&r.UpdatedAt); e != nil {
					return e
				}
			}
			if e = storeResource(ctx, tx, r, before == nil); e != nil {
				return e
			}
			// Property edits do not advance structure; parent moves do, even at the root.
			structureChanged := t.Kind == Component && (before == nil || action != "update" || ptrValue(before.ParentComponentID) != ptrValue(r.ParentComponentID))
			if structureChanged {
				if e = tx.QueryRow(ctx, `UPDATE engineering_facilities SET structure_revision=structure_revision+1 WHERE tenant_id=$1 AND id=$2 RETURNING structure_revision`, t.Tenant, t.Facility).Scan(&facility.StructureRevision); e != nil {
					return e
				}
			}
		}
		fid := t.Facility
		sr := facility.StructureRevision
		if t.Kind == Facility {
			fid = r.ID
			sr = r.StructureRevision
		}
		out = Result{ID: r.ID, FacilityID: fid, Revision: r.Revision, StructureRevision: sr, Changed: changed}
		if auth.Entry == "project" {
			out.ProjectID = auth.Project
			out.AccessRevision = auth.AccessRevision
			if link != nil {
				out.LinkID = link.ID
				out.LinkRevision = link.Revision
			}
			if action == "create" {
				out.LinkID, out.LinkRevision, out.AccessRevision, e = projectCreateGrant(ctx, tx, auth, r, link, m.RequestID, meta)
				if e != nil {
					return e
				}
			}
		}
		if changed {
			if e = recordChange(ctx, tx, a, auth, t, action, m, before, r, fid, out.LinkID, meta); e != nil {
				return e
			}
		}
		return saveMutationReceipt(ctx, tx, auth, t, action, m, in, out)
	})
	return out, preview, e
}
func hasKey(m map[string]any, k string) bool { _, ok := m[k]; return ok }
func storeResource(ctx context.Context, tx pgx.Tx, r Resource, create bool) error {
	columns := []string{"name", "status", "revision", "updated_at", "updated_by", "deleted_at"}
	columns = append(columns, mutable[r.Kind][1:]...)
	if create {
		columns = append(columns, "id", "tenant_id", "created_at", "created_by", "created_project_id")
		columns = append(columns, fixed[r.Kind]...)
		if r.Kind != Facility {
			columns = append(columns, "facility_id")
		}
		if r.Kind == Channel {
			columns = append(columns, "point_id")
		}
		if r.Kind == Facility {
			columns = append(columns, "structure_revision")
		}
	}
	var data map[string]any
	_ = json.Unmarshal(jsonBytes(r), &data)
	if r.Kind == Facility || r.Kind == Component {
		data["values"] = r.Values
	}
	cs := strings.Join(columns, ",")
	table := tables[r.Kind]
	body := jsonBytes(data)
	if create {
		_, e := tx.Exec(ctx, `INSERT INTO `+table+` (`+cs+`) SELECT `+cs+` FROM jsonb_populate_record(NULL::`+table+`,$1::jsonb)`, body)
		return e
	}
	_, e := tx.Exec(ctx, `UPDATE `+table+` SET (`+cs+`)=(SELECT `+cs+` FROM jsonb_populate_record(NULL::`+table+`,$1::jsonb)) WHERE tenant_id=$2 AND id=$3`, body, r.TenantID, r.ID)
	return e
}
func validateResource(ctx context.Context, tx pgx.Tx, t scopedTarget, r, facility Resource, newUse bool) error {
	if r.Kind == Facility {
		v, _, e := engineering.ResolvePublicTx(ctx, tx, r.TypeVersionID, engineering.TypeDefinition, newUse)
		if e != nil {
			return definitionError(e, "/type_version_id")
		}
		return valuesValid(v.Content.Fields, r.Values)
	}
	ancestors, e := ancestorsTx(ctx, tx, t, r, facility)
	if e != nil {
		return e
	}
	for _, p := range ancestors {
		if p.Status != "normal" {
			return problem("ancestor_retired", "祖先已退役，请先恢复祖先。", 409)
		}
	}
	if r.Kind == Component {
		v, _, e := engineering.ResolvePublicTx(ctx, tx, facility.TypeVersionID, engineering.TypeDefinition, false)
		if e != nil {
			return e
		}
		found := false
		for _, c := range v.Content.Components {
			if c.Key == r.CategoryKey {
				found = true
				if e = valuesValid(c.Fields, r.Values); e != nil {
					return e
				}
			}
		}
		if !found {
			return invalid("/category_key", "category", "该类型没有此构件类别。")
		}
		parent := "root"
		for _, p := range ancestors {
			if p.ID == r.ID {
				return invalid("/parent_component_id", "cycle", "不能将构件移动到自身或后代。")
			}
			if p.Kind == Component {
				parent = p.CategoryKey
			}
		}
		for _, rel := range v.Content.Relations {
			if rel.Parent == parent && rel.Child == r.CategoryKey {
				if rel.MaxCount != nil && r.Status == "normal" {
					var count int
					e = tx.QueryRow(ctx, `SELECT count(*) FROM engineering_components WHERE tenant_id=$1 AND facility_id=$2 AND parent_component_id IS NOT DISTINCT FROM $3::uuid AND category_key=$4 AND status='normal' AND deleted_at IS NULL AND id<>$5`, t.Tenant, t.Facility, r.ParentComponentID, r.CategoryKey, r.ID).Scan(&count)
					if e != nil {
						return e
					}
					if count >= *rel.MaxCount {
						return invalid("/parent_component_id", "max_count", fmt.Sprintf("该父级下此类别的正常构件最多 %d 个。", *rel.MaxCount))
					}
				}
				return nil
			}
		}
		return invalid("/parent_component_id", "relation", "类型不允许该父子类别关系。")
	}
	if r.Kind == Channel {
		v, _, e := engineering.ResolvePublicTx(ctx, tx, r.MetricVersionID, engineering.MetricDefinition, newUse)
		if e != nil {
			return definitionError(e, "/metric_version_id")
		}
		m := v.Content.Metric
		if m == nil || !slices.Contains([]string{"integer", "decimal", "enum"}, m.Shape) {
			return invalid("/metric_version_id", "shape", "不支持该指标形态。")
		}
		if m.Shape == "enum" {
			if r.UnitVersionID != nil {
				return invalid("/unit_version_id", "metric_unit_mismatch", "枚举指标不能选择单位。")
			}
			return nil
		}
		if r.UnitVersionID == nil || !slices.Contains(m.AllowedUnits, *r.UnitVersionID) {
			return invalid("/unit_version_id", "metric_unit_mismatch", "请选择该指标固定允许的单位版本。")
		}
		u, _, e := engineering.ResolvePublicTx(ctx, tx, *r.UnitVersionID, engineering.UnitDefinition, newUse)
		if e != nil {
			return definitionError(e, "/unit_version_id")
		}
		if u.Content.Unit == nil || u.Content.Unit.Category != m.Category {
			return invalid("/unit_version_id", "unit_category_mismatch", "单位量纲不匹配。")
		}
	}
	return nil
}
func definitionError(e error, path string) error {
	if e == access.ErrNotFound {
		return invalid(path, "definition_reference", "请选择可访问的公共发布版本。")
	}
	return e
}
func valuesValid(fields []engineering.Field, values map[string]any) error {
	issues := engineering.ValidateValues(fields, values, "/values")
	if len(issues) > 0 {
		return &Error{Code: "resource_invalid", Message: "请修正工程属性。", Status: 422, Issues: issues}
	}
	return nil
}
func (s *Service) checkDependencies(ctx context.Context, tx pgx.Tx, t scopedTarget, r Resource) error {
	if s.external != nil {
		impact, e := s.external.ResourceImpactTx(ctx, tx, t.Scope, t.Target)
		if e != nil {
			return e
		}
		if impact.Referenced {
			return problem("resource_referenced", "资源存在安装或来源履历，不能删除。", 409)
		}
	}
	var referenced bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_resource_visibility($1,NULL,$2) WHERE kind=$3 AND id=$4)`, t.Tenant, t.Facility, t.Kind, r.ID).Scan(&referenced); e != nil {
		return e
	}
	if referenced {
		return problem("resource_referenced", "仍有项目使用此资源，请先由租户明确解除相应授权。", 409)
	}
	deps := map[string]int{}
	queries := map[string]string{}
	switch r.Kind {
	case Facility:
		queries = map[string]string{"components": "engineering_components WHERE facility_id=$2", "points": "engineering_points WHERE facility_id=$2"}
	case Component:
		queries = map[string]string{"components": "engineering_components WHERE parent_component_id=$2", "points": "engineering_points WHERE component_id=$2"}
	case Point:
		queries = map[string]string{"channels": "engineering_channels WHERE point_id=$2"}
	}
	for kind, q := range queries {
		var n int
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM `+q+` AND tenant_id=$1 AND deleted_at IS NULL`, t.Tenant, r.ID).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			deps[kind] = n
		}
	}
	if len(deps) > 0 {
		e := problem("resource_referenced", "仍有未删除的子项，请先处理列出的依赖。", 409)
		e.Dependencies = deps
		return e
	}
	return nil
}
func recordChange(ctx context.Context, tx pgx.Tx, a access.Actor, auth authorization, t scopedTarget, action string, m WriteMeta, before *Resource, after Resource, fid, link string, meta access.Meta) error {
	fields := []string{}
	for _, d := range engineering.Differences(before, after) {
		fields = append(fields, d.Path)
	}
	versions := []string{}
	for _, id := range []string{after.TypeVersionID, after.MetricVersionID, ptrValue(after.UnitVersionID)} {
		if id != "" {
			versions = append(versions, id)
		}
	}
	if t.Kind == Component || t.Kind == Point {
		var v string
		if e := tx.QueryRow(ctx, `SELECT type_version_id::text FROM engineering_facilities WHERE tenant_id=$1 AND id=$2`, t.Tenant, fid).Scan(&v); e != nil {
			return e
		}
		versions = append(versions, v)
	}
	var name string
	if e := tx.QueryRow(ctx, `SELECT display_name FROM accounts WHERE id=$1`, a.ID).Scan(&name); e != nil {
		return e
	}
	reason := m.Reason
	if reason == "" {
		reason = map[string]string{"create": "新增资源", "update": "更新资源资料"}[action]
	}
	_, e := tx.Exec(ctx, `INSERT INTO engineering_resource_changes(tenant_id,facility_id,resource_kind,resource_id,resource_revision,operation,actor_id,actor_name,request_id,reason,before,after,changed_fields,definition_versions,actor_scope,actor_project_id,actor_tenant_membership_id,actor_project_membership_id,actor_link_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,NULLIF($16,'')::uuid,NULLIF($17,'')::uuid,NULLIF($18,'')::uuid,NULLIF($19,'')::uuid)`, t.Tenant, fid, t.Kind, after.ID, after.Revision, action, a.ID, name, m.RequestID, reason, before, after, fields, versions, auth.Entry, auth.Project, auth.TenantMembership, auth.ProjectAuth.MembershipID, link)
	if e != nil {
		return e
	}
	meta.ActorID = a.ID
	meta.ActorScope = auth.Entry
	meta.TenantID = t.Tenant
	meta.ProjectID = auth.Project
	meta.Reason = reason
	event := "resource." + action
	if auth.Entry == "project" {
		event = "project." + event
	}
	return access.Audit(ctx, tx, event, "engineering_"+string(t.Kind), after.ID, "", nil, map[string]any{"id": after.ID, "code": after.Code, "name": after.Name, "changed_fields": fields}, meta)
}
