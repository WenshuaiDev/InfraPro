package projects

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
)

var changes = map[access.Operation]access.Operation{
	access.OpProjectMemberSuspend: access.OpProjectMemberSuspendPreview, access.OpProjectMemberResume: access.OpProjectMemberResumePreview, access.OpProjectMemberRemove: access.OpProjectMemberRemovePreview,
	access.OpProjectOwnerChange: access.OpProjectOwnerChangePreview, access.OpProjectTransfer: access.OpProjectTransferPreview, access.OpProjectRecoverAdmin: access.OpProjectRecoverAdminPreview,
	access.OpProjectArchive: access.OpProjectArchivePreview, access.OpProjectRestore: access.OpProjectRestorePreview, access.OpProjectRolePermissions: access.OpProjectRolePermissionsPreview, access.OpProjectRoleStatus: access.OpProjectRoleStatusPreview, access.OpProjectRoleDelete: access.OpProjectRoleDeletePreview, access.OpProjectMemberRoles: access.OpProjectMemberRolesPreview,
}

func diff(a, b []access.Code) []access.Code {
	out := []access.Code{}
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}
func coverage(have, want []access.Code) error {
	if len(diff(want, have)) > 0 {
		return access.ErrScope
	}
	return nil
}
func normalizeChange(in *Change) {
	in.Candidate = strings.ToLower(in.Candidate)
	if in.Roles != nil {
		for i := range in.Roles {
			in.Roles[i] = strings.ToLower(in.Roles[i])
		}
		slices.Sort(in.Roles)
	}
	if in.Permissions != nil {
		slices.Sort(in.Permissions)
	}
}
func graphHash(ctx context.Context, tx pgx.Tx, project string, op access.Operation, target string, in Change) (string, error) {
	var graph string
	e := tx.QueryRow(ctx, `SELECT jsonb_build_object('project',to_jsonb(p),'tenant_status',t.status,
 'members',(SELECT COALESCE(jsonb_agg(jsonb_build_array(to_jsonb(m),tm.status,a.status) ORDER BY m.id),'[]'::jsonb) FROM project_memberships m JOIN tenant_memberships tm ON tm.id=m.tenant_membership_id JOIN accounts a ON a.id=m.account_id WHERE m.project_id=p.id),
 'owner',(SELECT jsonb_build_array(tm.status,a.status) FROM tenant_memberships tm JOIN accounts a ON a.id=tm.account_id WHERE tm.id=p.owner_tenant_membership_id),
 'candidate',(SELECT jsonb_build_array(tm.id,tm.status,a.status,(SELECT jsonb_agg(to_jsonb(m) ORDER BY m.id) FROM project_memberships m WHERE m.project_id=p.id AND m.tenant_membership_id=tm.id)) FROM tenant_memberships tm JOIN accounts a ON a.id=tm.account_id WHERE tm.tenant_id=p.tenant_id AND tm.id::text=$2),
 'roles',(SELECT COALESCE(jsonb_agg(jsonb_build_array(to_jsonb(r),(SELECT jsonb_agg(rp.permission_code ORDER BY rp.permission_code) FROM role_permissions rp WHERE rp.role_id=r.id)) ORDER BY r.id),'[]'::jsonb) FROM roles r WHERE r.binding_scope='project' AND (r.owner_project_id=p.id OR r.builtin_key IS NOT NULL)),
 'bindings',(SELECT COALESCE(jsonb_agg(to_jsonb(b) ORDER BY b.id),'[]'::jsonb) FROM project_member_roles b WHERE b.project_id=p.id))::text FROM projects p JOIN tenants t ON t.id=p.tenant_id WHERE p.id=$1`, project, in.Candidate).Scan(&graph)
	if e != nil {
		return "", e
	}
	return hash([]any{op, target, in.Candidate, in.Permissions, in.Roles, in.Status, graph, access.Checksum()}), nil
}
func (s *Service) Preview(ctx context.Context, a access.Actor, tenant, project, target string, op access.Operation, in Change) (Preview, error) {
	out := Preview{}
	preview, ok := changes[op]
	if !ok {
		return out, access.ErrDenied
	}
	normalizeChange(&in)
	e := s.transact(ctx, a, preview, tenant, project, func(tx pgx.Tx, _ string, auth access.ProjectAuthorization) error {
		var e error
		out, e = planChange(ctx, tx, a, tenant, project, target, op, in, auth)
		var ae *access.Error
		if errors.As(e, &ae) && ae.Status != 403 && ae.Status != 404 {
			out.CanSubmit = false
			out.BlockedReason = ae.Message
			return nil
		}
		return e
	})
	return out, e
}
func planChange(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project, target string, op access.Operation, in Change, auth access.ProjectAuthorization) (Preview, error) {
	p, e := projectTx(ctx, tx, tenant, project)
	out := Preview{Version: p.Version, ProjectVersion: p.Version, CatalogChecksum: access.Checksum(), CanSubmit: true, Members: []MemberImpact{}, RoleVersions: map[string]string{}, Notes: []string{}}
	if e != nil {
		return out, e
	}
	out.Hash, e = graphHash(ctx, tx, project, op, target, in)
	if e != nil {
		return out, e
	}
	if access.ProjectActiveOnly(op) {
		if e = active(p); e != nil {
			return out, e
		}
	}
	switch op {
	case access.OpProjectArchive, access.OpProjectRestore:
		out.Before = p.Summary
		after := p.Summary
		after.Status = "archived"
		if op == access.OpProjectRestore {
			after.Status = "active"
			if p.Status != "archived" {
				return out, problem("state_conflict", "项目当前未归档。", 409)
			}
			if !p.Administrator.Available {
				return out, problem("administrator_unavailable", "请先从租户治理入口修复项目管理员。", 409)
			}
			if e = access.CheckProjectStructure(ctx, tx); e != nil {
				return out, e
			}
		}
		out.After = after
		out.Notes = []string{"归档后可按权限查看资料和历史、撤销授权及交接管理；禁止编辑、添加成员、恢复成员和扩权。", "恢复沿用当前关系，已移除成员、撤销绑定和停用角色不会复活。"}
		var n int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM project_memberships WHERE project_id=$1 AND status<>'ended'`, project).Scan(&n); e != nil {
			return out, e
		}
		out.MemberCount = n
		if !p.Owner.Available {
			out.Notes = append(out.Notes, "业务负责人不可用；恢复不会自动更换负责人。")
		}
	case access.OpProjectOwnerChange, access.OpProjectTransfer, access.OpProjectRecoverAdmin:
		candidate, e := candidateTx(ctx, tx, tenant, in.Candidate)
		if e != nil {
			return out, e
		}
		out.Before = p.Summary
		out.After = candidate
		out.Notes = append(out.Notes, "接任人员："+candidate.DisplayName+"（"+candidate.Username+"）")
		if op == access.OpProjectOwnerChange {
			if p.Owner.MembershipID == in.Candidate {
				return out, problem("state_conflict", "负责人未发生变化。", 409)
			}
			out.Notes = append(out.Notes, "负责人变更不授予成员身份或项目权限。")
		} else {
			if op == access.OpProjectRecoverAdmin && p.Administrator.Available {
				return out, problem("state_conflict", "当前管理员可用，请由现任管理员正常交接。", 409)
			}
			if op == access.OpProjectTransfer && (!auth.Administrator || p.Administrator.AccountID != a.ID) {
				return out, access.ErrDenied
			}
			if p.Administrator.AccountID == candidate.AccountID {
				return out, problem("candidate_unavailable", "不能交接给当前管理员。", 409)
			}
			if e = access.CheckProjectStructure(ctx, tx); e != nil {
				return out, e
			}
			var mid, status string
			e = tx.QueryRow(ctx, `SELECT id::text,status FROM project_memberships WHERE project_id=$1 AND account_id=$2 AND status<>'ended'`, project, candidate.AccountID).Scan(&mid, &status)
			if e != nil && e != pgx.ErrNoRows {
				return out, e
			}
			if status == "suspended" {
				return out, problem("candidate_unavailable", "接任者项目成员关系已暂停，请先恢复或另选成员。", 409)
			}
			if mid != "" {
				before, e := access.ProjectAuthorizationTx(ctx, tx, tenant, project, mid, "")
				if e != nil {
					return out, e
				}
				out.Before = map[string]any{"project": p.Summary, "successor_authorization": before}
				names := []string{}
				for _, r := range before.Roles {
					names = append(names, r.Name)
				}
				out.Notes = append(out.Notes, "接任者将被替换的原角色："+strings.Join(names, "、"))
			}
			out.Notes = append(out.Notes, "接任者现有项目角色将被固定管理员角色替换；原管理员项目关系结束，租户关系保留。")
		}
	case access.OpProjectMemberSuspend, access.OpProjectMemberResume, access.OpProjectMemberRemove:
		member, e := memberTx(ctx, tx, tenant, project, target)
		if e != nil {
			return out, e
		}
		out.Version = member.Version
		out.Before = member
		if member.Administrator || member.AccountID == a.ID {
			return out, problem("member_protected", "不能通过普通成员操作处置自己或当前管理员。", 409)
		}
		before, e := access.ProjectAuthorizationTx(ctx, tx, tenant, project, target, "")
		if e != nil {
			return out, e
		}
		if e = coverage(auth.Permissions, before.ConfiguredPermissions); e != nil {
			return out, e
		}
		after := member
		switch op {
		case access.OpProjectMemberSuspend:
			if member.Status != "active" {
				return out, problem("state_conflict", "成员状态已变化。", 409)
			}
			after.Status = "suspended"
		case access.OpProjectMemberResume:
			if member.Status != "suspended" {
				return out, problem("state_conflict", "成员状态已变化。", 409)
			}
			if _, e = candidateTx(ctx, tx, tenant, member.TenantMembershipID); e != nil {
				return out, e
			}
			after.Status = "active"
		case access.OpProjectMemberRemove:
			if member.Status == "ended" {
				return out, problem("state_conflict", "成员关系已经结束。", 409)
			}
			after.Status = "ended"
		}
		out.After = after
		afterAuth := before
		afterAuth.MembershipStatus = after.Status
		if after.Status == "ended" {
			afterAuth.Roles = []access.Role{}
		}
		catalog := map[access.Code]access.Permission{}
		for _, pc := range access.Catalog() {
			catalog[pc.Code] = pc
		}
		access.ExplainProject(&afterAuth, catalog)
		impact := MemberImpact{MembershipID: member.ID, DisplayName: member.DisplayName, Status: member.Status, Before: before, After: afterAuth, Added: diff(afterAuth.Permissions, before.Permissions), Removed: diff(before.Permissions, afterAuth.Permissions), RestorableAdded: diff(afterAuth.RestorablePermissions, before.RestorablePermissions), RestorableRemoved: diff(before.RestorablePermissions, afterAuth.RestorablePermissions)}
		out.Members = append(out.Members, impact)
		out.MemberCount = 1
		if len(impact.Removed) > 0 {
			out.LosingMembers = 1
		}
	case access.OpProjectRolePermissions, access.OpProjectRoleStatus, access.OpProjectRoleDelete, access.OpProjectMemberRoles:
		return planAuthorizationChange(ctx, tx, tenant, project, target, op, in, p, auth, out)
	default:
		return out, access.ErrDenied
	}
	return out, nil
}
func (s *Service) Change(ctx context.Context, a access.Actor, tenant, project, target string, op access.Operation, in Change, m access.Meta) (Result, error) {
	var out Result
	if _, ok := changes[op]; !ok {
		return out, access.ErrDenied
	}
	normalizeChange(&in)
	e := s.transact(ctx, a, op, tenant, project, func(tx pgx.Tx, _ string, auth access.ProjectAuthorization) error {
		if e := in.WriteMeta.normalize(); e != nil {
			return e
		}
		payload := struct {
			Target string
			Change Change
		}{target, in}
		var e error
		var replay bool
		out, replay, e = s.receipt(ctx, tx, a, tenant, project, target, op, in.WriteMeta, payload)
		if e != nil || replay {
			return e
		}
		p, e := planChange(ctx, tx, a, tenant, project, target, op, in, auth)
		if e != nil {
			return e
		}
		if e = version(p.Version, in.Version); e != nil {
			return e
		}
		if e = version(p.ProjectVersion, in.ProjectVersion); e != nil {
			return e
		}
		if p.Hash != in.Hash || in.Hash == "" {
			return problem("preview_stale", "变更影响已经变化，请重新预览并确认。", 409)
		}
		if op == access.OpProjectMemberRoles && hash(p.RoleVersions) != hash(in.RoleVersions) {
			return problem("preview_stale", "相关角色版本已经变化，请重新预览。", 409)
		}
		old, e := projectTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		object := target
		if object == "" {
			object = project
		}
		ver := ""
		switch op {
		case access.OpProjectMemberSuspend, access.OpProjectMemberResume, access.OpProjectMemberRemove:
			if op == access.OpProjectMemberRemove {
				e = access.EndProjectMembershipTx(ctx, tx, target, a.ID, in.Reason)
			} else {
				status := "suspended"
				if op == access.OpProjectMemberResume {
					status = "active"
				}
				_, e = tx.Exec(ctx, `UPDATE project_memberships SET status=$2,version=version+1,status_changed_at=clock_timestamp(),status_changed_by=$3,status_reason=$4 WHERE id=$1`, target, status, a.ID, in.Reason)
			}
			if e != nil {
				return e
			}
			e = tx.QueryRow(ctx, `SELECT version::text FROM project_memberships WHERE id=$1`, target).Scan(&ver)
		case access.OpProjectOwnerChange:
			_, e = tx.Exec(ctx, `UPDATE projects SET owner_tenant_membership_id=$2,version=version+1,updated_at=clock_timestamp(),updated_by=$3 WHERE id=$1`, project, in.Candidate, a.ID)
		case access.OpProjectArchive:
			_, e = tx.Exec(ctx, `UPDATE projects SET status='archived',archived_at=clock_timestamp(),archived_by=$2,archive_reason=$3,version=version+1,updated_at=clock_timestamp(),updated_by=$2 WHERE id=$1`, project, a.ID, in.Reason)
		case access.OpProjectRestore:
			_, e = tx.Exec(ctx, `UPDATE projects SET status='active',archived_at=NULL,archived_by=NULL,archive_reason=NULL,version=version+1,updated_at=clock_timestamp(),updated_by=$2 WHERE id=$1`, project, a.ID)
		case access.OpProjectTransfer, access.OpProjectRecoverAdmin:
			candidate, e := candidateTx(ctx, tx, tenant, in.Candidate)
			if e != nil {
				return e
			}
			var mid string
			e = tx.QueryRow(ctx, `SELECT id::text FROM project_memberships WHERE project_id=$1 AND account_id=$2 AND status='active'`, project, candidate.AccountID).Scan(&mid)
			if e == pgx.ErrNoRows {
				mid, e = newID(ctx, tx)
				if e != nil {
					return e
				}
				e = bindTx(ctx, tx, tenant, project, mid, in.Candidate, candidate.AccountID, a.ID, access.ProjectAdminKey)
			} else if e == nil {
				if _, e = tx.Exec(ctx, `UPDATE project_member_roles SET revoked_at=clock_timestamp(),revoked_by=$2 WHERE membership_id=$1 AND revoked_at IS NULL`, mid, a.ID); e != nil {
					return e
				}
				if e = bindRoleTx(ctx, tx, tenant, project, mid, a.ID, access.ProjectAdminKey); e != nil {
					return e
				}
				_, e = tx.Exec(ctx, `UPDATE project_memberships SET version=version+1 WHERE id=$1`, mid)
			}
			if e != nil {
				return e
			}
			if old.AdministratorMembershipID != "" {
				if e = access.EndProjectMembershipTx(ctx, tx, old.AdministratorMembershipID, a.ID, in.Reason); e != nil {
					return e
				}
			}
			_, e = tx.Exec(ctx, `UPDATE projects SET administrator_membership_id=$2,management_gap_reason=NULL,management_gap_at=NULL,management_gap_by=NULL,previous_administrator_membership_id=NULL,version=version+1,updated_at=clock_timestamp(),updated_by=$3 WHERE id=$1`, project, mid, a.ID)
		case access.OpProjectRolePermissions, access.OpProjectRoleStatus, access.OpProjectRoleDelete, access.OpProjectMemberRoles:
			ver, e = applyAuthorizationChange(ctx, tx, tenant, project, target, op, in, a.ID)
		default:
			return access.ErrDenied
		}
		if e != nil {
			return e
		}
		if ver == "" {
			e = tx.QueryRow(ctx, `SELECT version::text FROM projects WHERE id=$1`, project).Scan(&ver)
			if e != nil {
				return e
			}
		}
		var after any
		switch op {
		case access.OpProjectMemberSuspend, access.OpProjectMemberResume, access.OpProjectMemberRemove, access.OpProjectMemberRoles:
			after, e = access.ProjectAuthorizationTx(ctx, tx, tenant, project, target, "")
		case access.OpProjectRolePermissions, access.OpProjectRoleStatus:
			after, e = access.ProjectRoleTx(ctx, tx, tenant, project, target)
		case access.OpProjectRoleDelete:
			after = map[string]string{"id": target, "version": ver, "status": "deleted"}
		default:
			after, e = projectTx(ctx, tx, tenant, project)
		}
		if e != nil {
			return e
		}
		out, e = complete(ctx, tx, a, tenant, project, object, ver, op, in.WriteMeta, payload, p.Before, after, m)
		return e
	})
	return out, e
}
func (s *Service) AddMember(ctx context.Context, a access.Actor, tenant, project string, in AddMemberInput, m access.Meta) (Result, error) {
	var out Result
	in.Candidate = strings.ToLower(in.Candidate)
	e := s.transact(ctx, a, access.OpProjectMemberAdd, tenant, project, func(tx pgx.Tx, _ string, auth access.ProjectAuthorization) error {
		if e := in.WriteMeta.normalize(); e != nil {
			return e
		}
		var e error
		var replay bool
		out, replay, e = s.receipt(ctx, tx, a, tenant, project, "", access.OpProjectMemberAdd, in.WriteMeta, in)
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
		if e = coverage(auth.Permissions, []access.Code{access.ProjectProfileRead}); e != nil {
			return e
		}
		candidate, e := candidateTx(ctx, tx, tenant, in.Candidate)
		if e != nil {
			return e
		}
		mid, e := newID(ctx, tx)
		if e != nil {
			return e
		}
		if e = bindTx(ctx, tx, tenant, project, mid, in.Candidate, candidate.AccountID, a.ID, access.ProjectMemberKey); e != nil {
			return e
		}
		after, e := memberTx(ctx, tx, tenant, project, mid)
		if e != nil {
			return e
		}
		out, e = complete(ctx, tx, a, tenant, project, mid, "1", access.OpProjectMemberAdd, in.WriteMeta, in, nil, after, m)
		return e
	})
	return out, e
}
