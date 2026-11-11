package projects

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
	"slices"
	"strings"
)

func permissionSet(codes []access.Code, have []access.Code) error {
	if codes == nil {
		return access.Validation("permission_codes", "请提交明确的权限数组，空数组表示清空。")
	}
	seen := map[access.Code]bool{}
	for _, c := range codes {
		if seen[c] {
			return access.Validation("permission_codes", "权限不能重复。")
		}
		seen[c] = true
		valid := false
		for _, p := range access.Catalog() {
			if p.Code == c && p.Scope == "project" && p.Lifecycle == "active" && p.ProjectCustomRoleAssignable {
				valid = true
				break
			}
		}
		if !valid {
			return access.Validation("permission_codes", "仅可配置当前有效的项目普通权限，管理保留权限不能下放。")
		}
	}
	return coverage(have, codes)
}
func roleName(ctx context.Context, tx pgx.Tx, project, except string, name, description *string) error {
	*name = strings.TrimSpace(norm.NFKC.String(*name))
	*description = strings.TrimSpace(*description)
	if e := textField("name", *name, 1, 50); e != nil {
		return e
	}
	if e := textField("description", *description, 0, 500); e != nil {
		return e
	}
	var taken bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roles WHERE binding_scope='project' AND deleted_at IS NULL AND (owner_project_id IS NULL OR owner_project_id::text=$1) AND id::text<>$2 AND name_normalized=$3)`, project, except, strings.ToLower(*name)).Scan(&taken)
	if e == nil && taken {
		return problem("role_name_taken", "本项目或预置角色名称已被使用。", 409)
	}
	return e
}
func (s *Service) CreateRole(ctx context.Context, a access.Actor, tenant, project, source string, in RoleInput, m access.Meta) (Result, error) {
	var out Result
	op := access.OpProjectRoleCreate
	if source != "" {
		op = access.OpProjectRoleCopy
	}
	e := s.transact(ctx, a, op, tenant, project, func(tx pgx.Tx, _ string, auth access.ProjectAuthorization) error {
		if e := in.WriteMeta.normalize(); e != nil {
			return e
		}
		in.Name = strings.TrimSpace(norm.NFKC.String(in.Name))
		in.Description = strings.TrimSpace(in.Description)
		if in.Permissions != nil {
			slices.Sort(in.Permissions)
		}
		payload := struct {
			Source string
			Input  RoleInput
		}{source, in}
		var e error
		var replay bool
		out, replay, e = s.receipt(ctx, tx, a, tenant, project, source, op, in.WriteMeta, payload)
		if e != nil || replay {
			return e
		}
		p, e := projectTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		if e = active(p); e != nil {
			return e
		}
		if e = version(p.Version, in.ProjectVersion); e != nil {
			return e
		}
		if e = roleName(ctx, tx, project, "", &in.Name, &in.Description); e != nil {
			return e
		}
		if e = permissionSet(in.Permissions, auth.Permissions); e != nil {
			return e
		}
		if source != "" {
			r, e := access.ProjectRoleTx(ctx, tx, tenant, project, source)
			if e != nil {
				return e
			}
			if r.BuiltinKey == access.ProjectAdminKey {
				return access.ErrDenied
			}
			if e = version(r.Version, in.SourceVersion); e != nil {
				return e
			}
		} else if in.SourceVersion != "" {
			return access.Validation("source_version", "无复制来源时不应提供来源版本。")
		}
		var id string
		e = tx.QueryRow(ctx, `INSERT INTO roles(name,name_normalized,description,binding_scope,owner_tenant_id,owner_project_id,source_role_id,source_role_version,created_by,updated_by) VALUES($1,$2,$3,'project',$4,$5,NULLIF($6,'')::uuid,NULLIF($7,'')::bigint,$8,$8) RETURNING id::text`, in.Name, strings.ToLower(in.Name), in.Description, tenant, project, source, in.SourceVersion, a.ID).Scan(&id)
		if e != nil {
			return e
		}
		for _, code := range in.Permissions {
			if _, e = tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,'project')`, id, code); e != nil {
				return e
			}
		}
		after, e := access.ProjectRoleTx(ctx, tx, tenant, project, id)
		if e != nil {
			return e
		}
		out, e = complete(ctx, tx, a, tenant, project, id, "1", op, in.WriteMeta, payload, nil, after, m)
		return e
	})
	return out, e
}
func (s *Service) UpdateRole(ctx context.Context, a access.Actor, tenant, project, id string, in RoleInput, m access.Meta) (Result, error) {
	var out Result
	e := s.transact(ctx, a, access.OpProjectRoleUpdate, tenant, project, func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		if e := in.WriteMeta.normalize(); e != nil {
			return e
		}
		in.Name = strings.TrimSpace(norm.NFKC.String(in.Name))
		in.Description = strings.TrimSpace(in.Description)
		payload := struct {
			ID    string
			Input RoleInput
		}{id, in}
		var e error
		var replay bool
		out, replay, e = s.receipt(ctx, tx, a, tenant, project, id, access.OpProjectRoleUpdate, in.WriteMeta, payload)
		if e != nil || replay {
			return e
		}
		p, e := projectTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		if e = active(p); e != nil {
			return e
		}
		if e = version(p.Version, in.ProjectVersion); e != nil {
			return e
		}
		old, e := access.ProjectRoleTx(ctx, tx, tenant, project, id)
		if e != nil {
			return e
		}
		if old.BuiltinKey != "" {
			return access.ErrDenied
		}
		if e = version(old.Version, in.Version); e != nil {
			return e
		}
		if e = roleName(ctx, tx, project, id, &in.Name, &in.Description); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE roles SET name=$2,name_normalized=$3,description=$4,version=version+1,updated_by=$5,updated_at=clock_timestamp() WHERE id=$1`, id, in.Name, strings.ToLower(in.Name), in.Description, a.ID)
		if e != nil {
			return e
		}
		after, e := access.ProjectRoleTx(ctx, tx, tenant, project, id)
		if e != nil {
			return e
		}
		out, e = complete(ctx, tx, a, tenant, project, id, after.Version, access.OpProjectRoleUpdate, in.WriteMeta, payload, old, after, m)
		return e
	})
	return out, e
}
func planAuthorizationChange(ctx context.Context, tx pgx.Tx, tenant, project, target string, op access.Operation, in Change, p Project, actor access.ProjectAuthorization, out Preview) (Preview, error) {
	affected := []string{}
	var role, afterRole access.Role
	var member access.ProjectAuthorization
	newRoles := []access.Role{}
	if op == access.OpProjectMemberRoles {
		var e error
		member, e = access.ProjectAuthorizationTx(ctx, tx, tenant, project, target, "")
		if e != nil {
			return out, e
		}
		out.Version = member.MembershipVersion
		out.Before = member
		if member.Administrator {
			return out, problem("member_protected", "固定管理员角色仅通过管理交接流程修改。", 409)
		}
		if member.MembershipStatus == "ended" {
			return out, problem("state_conflict", "成员关系已经结束。", 409)
		}
		if in.Roles == nil {
			return out, access.Validation("role_ids", "请提交明确的角色数组；空数组表示清空。")
		}
		oldIDs := []string{}
		for _, r := range member.Roles {
			oldIDs = append(oldIDs, r.ID)
			out.RoleVersions[r.ID] = r.Version
		}
		seen := map[string]bool{}
		for _, id := range in.Roles {
			if !access.ValidID(id) || seen[id] {
				return out, access.Validation("role_ids", "角色编号无效或重复。")
			}
			seen[id] = true
			r, e := access.ProjectRoleTx(ctx, tx, tenant, project, id)
			if e != nil {
				return out, e
			}
			if r.BuiltinKey == access.ProjectAdminKey {
				return out, access.ErrDenied
			}
			out.RoleVersions[r.ID] = r.Version
			if !slices.Contains(oldIDs, id) {
				if p.Status == "archived" {
					return out, problem("project_archived", "归档项目只能减少已有角色绑定。", 409)
				}
				if member.AccountStatus != "active" || member.TenantMembershipStatus != "active" {
					return out, problem("candidate_unavailable", "上层资格不可用时只能减少已有绑定。", 409)
				}
				if r.Status != "active" {
					return out, problem("state_conflict", "不能新增停用角色绑定。", 409)
				}
				if e = permissionSet(r.Permissions, actor.Permissions); e != nil {
					return out, e
				}
			}
			newRoles = append(newRoles, r)
		}
		if len(in.Roles) == len(oldIDs) {
			same := true
			for _, id := range in.Roles {
				same = same && slices.Contains(oldIDs, id)
			}
			if same {
				return out, problem("state_conflict", "角色集合未发生变化。", 409)
			}
		}
		affected = append(affected, target)
	} else {
		var e error
		role, e = access.ProjectRoleTx(ctx, tx, tenant, project, target)
		if e != nil {
			return out, e
		}
		if role.BuiltinKey != "" {
			return out, access.ErrDenied
		}
		out.Version = role.Version
		out.Before = role
		afterRole = role
		switch op {
		case access.OpProjectRolePermissions:
			if e = permissionSet(in.Permissions, actor.Permissions); e != nil {
				return out, e
			}
			if slices.Equal(in.Permissions, role.Permissions) {
				return out, problem("state_conflict", "权限集合未发生变化。", 409)
			}
			if p.Status == "archived" && len(diff(in.Permissions, role.Permissions)) > 0 {
				return out, problem("project_archived", "归档项目只能减少已有角色权限。", 409)
			}
			afterRole.Permissions = in.Permissions
		case access.OpProjectRoleStatus:
			if in.Status != "active" && in.Status != "disabled" {
				return out, access.Validation("status", "状态应为 active 或 disabled。")
			}
			if in.Status == role.Status {
				return out, problem("state_conflict", "角色状态未发生变化。", 409)
			}
			if in.Status == "active" {
				if e = active(p); e != nil {
					return out, e
				}
				if e = permissionSet(role.Permissions, actor.Permissions); e != nil {
					return out, e
				}
			}
			afterRole.Status = in.Status
		case access.OpProjectRoleDelete:
			if role.AccountCount > 0 {
				return out, problem("role_in_use", "仍有当前绑定（包括暂停成员），请先全部解绑。", 409)
			}
			afterRole.Status = "deleted"
		}
		out.After = afterRole
		rows, e := tx.Query(ctx, `SELECT membership_id::text FROM project_member_roles WHERE project_id=$1 AND role_id=$2 AND revoked_at IS NULL ORDER BY membership_id`, project, target)
		if e != nil {
			return out, e
		}
		affected, e = ids(rows)
		if e != nil {
			return out, e
		}
	}
	catalog := map[access.Code]access.Permission{}
	for _, pc := range access.Catalog() {
		catalog[pc.Code] = pc
	}
	for _, mid := range affected {
		before, e := access.ProjectAuthorizationTx(ctx, tx, tenant, project, mid, "")
		if e != nil {
			return out, e
		}
		after := before
		after.Roles = slices.Clone(before.Roles)
		if op == access.OpProjectMemberRoles {
			after.Roles = newRoles
		} else {
			for i, r := range after.Roles {
				if r.ID == target {
					after.Roles[i] = afterRole
				}
			}
		}
		access.ExplainProject(&after, catalog)
		impact := MemberImpact{MembershipID: mid, DisplayName: before.DisplayName, Status: before.MembershipStatus, Added: diff(after.Permissions, before.Permissions), Removed: diff(before.Permissions, after.Permissions), RestorableAdded: diff(after.RestorablePermissions, before.RestorablePermissions), RestorableRemoved: diff(before.RestorablePermissions, after.RestorablePermissions), Before: before, After: after}
		out.Members = append(out.Members, impact)
		if len(impact.Removed) > 0 {
			out.LosingMembers++
		}
		if op == access.OpProjectMemberRoles {
			out.After = after
		}
	}
	out.MemberCount = len(affected)
	return out, nil
}
func applyAuthorizationChange(ctx context.Context, tx pgx.Tx, tenant, project, target string, op access.Operation, in Change, actor string) (string, error) {
	var e error
	var ver string
	switch op {
	case access.OpProjectMemberRoles:
		// Preserve existing disabled bindings; do not revoke/reinsert unchanged role IDs.
		_, e = tx.Exec(ctx, `UPDATE project_member_roles SET revoked_at=clock_timestamp(),revoked_by=$2 WHERE membership_id=$1 AND revoked_at IS NULL AND NOT(role_id::text=ANY($3::text[]))`, target, actor, in.Roles)
		if e != nil {
			return "", e
		}
		for _, id := range in.Roles {
			_, e = tx.Exec(ctx, `INSERT INTO project_member_roles(tenant_id,project_id,membership_id,role_id,assigned_by) SELECT $1,$2,$3,$4,$5 WHERE NOT EXISTS(SELECT 1 FROM project_member_roles WHERE membership_id=$3 AND role_id=$4 AND revoked_at IS NULL)`, tenant, project, target, id, actor)
			if e != nil {
				return "", e
			}
		}
		e = tx.QueryRow(ctx, `UPDATE project_memberships SET version=version+1 WHERE id=$1 RETURNING version::text`, target).Scan(&ver)
		return ver, e
	case access.OpProjectRolePermissions:
		_, e = tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND NOT(permission_code=ANY($2::text[]))`, target, in.Permissions)
		if e != nil {
			return "", e
		}
		for _, c := range in.Permissions {
			_, e = tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,'project') ON CONFLICT DO NOTHING`, target, c)
			if e != nil {
				return "", e
			}
		}
	case access.OpProjectRoleStatus:
		_, e = tx.Exec(ctx, `UPDATE roles SET status=$2 WHERE id=$1`, target, in.Status)
	case access.OpProjectRoleDelete:
		_, e = tx.Exec(ctx, `UPDATE roles SET deleted_at=clock_timestamp() WHERE id=$1`, target)
	default:
		return "", access.ErrDenied
	}
	if e != nil {
		return "", e
	}
	e = tx.QueryRow(ctx, `UPDATE roles SET version=version+1,updated_by=$2,updated_at=clock_timestamp() WHERE id=$1 RETURNING version::text`, target, actor).Scan(&ver)
	return ver, e
}
