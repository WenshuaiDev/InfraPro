package access

import (
	"context"
	"github.com/jackc/pgx/v5"
	"slices"
)

// Upper-layer callers hold the exclusive graph lock; no caller gains project business access.
func ProtectProjectAdministrator(ctx context.Context, tx pgx.Tx, account, tenantMember string) error {
	var exists bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects p JOIN project_memberships m ON m.id=p.administrator_membership_id WHERE ($1<>'' AND m.account_id::text=$1) OR ($2<>'' AND m.tenant_membership_id::text=$2))`, account, tenantMember).Scan(&exists)
	if e == nil && exists {
		return ErrProjectAdministrator
	}
	return e
}
func EndProjectMembershipTx(ctx context.Context, tx pgx.Tx, member, actor, reason string) error {
	if _, e := tx.Exec(ctx, `UPDATE project_member_roles SET revoked_at=clock_timestamp(),revoked_by=$2 WHERE membership_id=$1 AND revoked_at IS NULL`, member, actor); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `UPDATE project_memberships SET status='ended',version=version+1,status_changed_at=clock_timestamp(),status_changed_by=$2,status_reason=$3,ended_at=clock_timestamp(),ended_by=$2,end_reason=$3 WHERE id=$1 AND status<>'ended'`, member, actor, reason)
	return e
}
func CascadeProjectMemberships(ctx context.Context, tx pgx.Tx, tenantMember string, m Meta) error {
	if e := ProtectProjectAdministrator(ctx, tx, "", tenantMember); e != nil {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT id::text,tenant_id::text,project_id::text FROM project_memberships WHERE tenant_membership_id=$1 AND status<>'ended' ORDER BY project_id,id`, tenantMember)
	if e != nil {
		return e
	}
	type target struct{ id, tenant, project string }
	list := []target{}
	for rows.Next() {
		var v target
		if e = rows.Scan(&v.id, &v.tenant, &v.project); e != nil {
			rows.Close()
			return e
		}
		list = append(list, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range list {
		before, e := ProjectAuthorizationTx(ctx, tx, v.tenant, v.project, v.id, "")
		if e != nil {
			return e
		}
		if e = EndProjectMembershipTx(ctx, tx, v.id, m.ActorID, m.Reason); e != nil {
			return e
		}
		m.TenantID = v.tenant
		m.ProjectID = v.project
		if e = Audit(ctx, tx, "project.member.parent_ended", "project_member", v.id, "", before, map[string]string{"status": "ended"}, m); e != nil {
			return e
		}
	}
	return nil
}

type ProjectManagementImpact struct {
	ProjectID     string `json:"project_id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	MembershipID  string `json:"membership_id"`
	Administrator bool   `json:"administrator"`
}

func ProjectParentImpactTx(ctx context.Context, tx pgx.Tx, tenantMember string) ([]ProjectManagementImpact, string, error) {
	rows, e := tx.Query(ctx, `SELECT p.id::text,p.name,p.version::text,m.id::text,COALESCE(p.administrator_membership_id=m.id,false) FROM project_memberships m JOIN projects p ON p.id=m.project_id WHERE m.tenant_membership_id=$1 AND m.status<>'ended' ORDER BY p.id,m.id`, tenantMember)
	if e != nil {
		return nil, "", e
	}
	defer rows.Close()
	out := []ProjectManagementImpact{}
	for rows.Next() {
		var v ProjectManagementImpact
		if e = rows.Scan(&v.ProjectID, &v.Name, &v.Version, &v.MembershipID, &v.Administrator); e != nil {
			return nil, "", e
		}
		out = append(out, v)
	}
	if e = rows.Err(); e != nil {
		return nil, "", e
	}
	rows.Close()
	var graph string
	e = tx.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_array(to_jsonb(m),tm.status,a.status,(SELECT jsonb_agg(to_jsonb(b) ORDER BY b.id) FROM project_member_roles b WHERE b.membership_id=m.id)) ORDER BY m.id),'[]'::jsonb)::text FROM project_memberships m JOIN tenant_memberships tm ON tm.id=m.tenant_membership_id JOIN accounts a ON a.id=m.account_id WHERE m.tenant_membership_id=$1 AND m.status<>'ended'`, tenantMember).Scan(&graph)
	return out, hashValue([]any{out, graph}), e
}

// Only the explicit disabled-tenant-administrator repair may create recorded management gaps.
func RepairProjectParentTx(ctx context.Context, tx pgx.Tx, tenantMember, preview string, confirmed bool, m Meta) error {
	var disabled bool
	if e := tx.QueryRow(ctx, `SELECT a.status='disabled' FROM tenant_memberships tm JOIN tenants t ON t.administrator_membership_id=tm.id JOIN accounts a ON a.id=tm.account_id WHERE tm.id=$1`, tenantMember).Scan(&disabled); e != nil {
		return e
	}
	impacts, hash, e := ProjectParentImpactTx(ctx, tx, tenantMember)
	if e != nil {
		return e
	}
	if len(impacts) == 0 {
		return nil
	}
	if !disabled {
		return ProtectProjectAdministrator(ctx, tx, "", tenantMember)
	}
	if !confirmed || preview != hash {
		return &Error{Code: "preview_stale", Message: "请预览并明确确认原租户管理员的项目关系结束及管理空缺。", Status: 409}
	}
	for _, v := range impacts {
		if !v.Administrator {
			continue
		}
		_, e = tx.Exec(ctx, `UPDATE projects SET previous_administrator_membership_id=administrator_membership_id,administrator_membership_id=NULL,management_gap_reason=$2,management_gap_at=clock_timestamp(),management_gap_by=$3,version=version+1,updated_at=clock_timestamp(),updated_by=$3 WHERE id=$1`, v.ProjectID, m.Reason, m.ActorID)
		if e != nil {
			return e
		}
		m.ProjectID = v.ProjectID
		if e = Audit(ctx, tx, "project.administrator.gap", "project", v.ProjectID, "", v, map[string]string{"management_gap_reason": m.Reason}, m); e != nil {
			return e
		}
	}
	return nil
}
func AuditProjectAvailability(ctx context.Context, tx pgx.Tx, account string, enabled bool, m Meta) error {
	rows, e := tx.Query(ctx, `SELECT p.id::text,p.tenant_id::text FROM projects p JOIN project_memberships pm ON pm.id=p.administrator_membership_id WHERE pm.account_id=$1 ORDER BY p.id`, account)
	if e != nil {
		return e
	}
	type target struct{ pid, tid string }
	list := []target{}
	for rows.Next() {
		var v target
		if e = rows.Scan(&v.pid, &v.tid); e != nil {
			rows.Close()
			return e
		}
		list = append(list, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	event := "project.administrator.unavailable"
	if enabled {
		event = "project.administrator.available"
	}
	for _, v := range list {
		m.ProjectID = v.pid
		m.TenantID = v.tid
		if e = Audit(ctx, tx, event, "project", v.pid, "", nil, map[string]any{"account_id": account, "available": enabled}, m); e != nil {
			return e
		}
	}
	return nil
}
func prepareProjectSync(ctx context.Context, tx pgx.Tx, plan *syncPlan, catalog map[Code]Permission, builtins map[string]Builtin) error {
	report := &plan.Report
	rows, e := tx.Query(ctx, `SELECT id::text,tenant_id::text,project_id::text FROM project_memberships WHERE status<>'ended' ORDER BY project_id,id`)
	if e != nil {
		return e
	}
	type target struct{ id, tenant, project string }
	list := []target{}
	for rows.Next() {
		var v target
		if e = rows.Scan(&v.id, &v.tenant, &v.project); e != nil {
			rows.Close()
			return e
		}
		list = append(list, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range list {
		before, e := ProjectAuthorizationTx(ctx, tx, v.tenant, v.project, v.id, "")
		if e != nil {
			return e
		}
		after := before
		after.Roles = slices.Clone(before.Roles)
		for i, r := range after.Roles {
			if b, ok := builtins[r.BuiltinKey]; ok {
				after.Roles[i].Permissions = slices.Clone(b.Permissions)
				slices.Sort(after.Roles[i].Permissions)
				after.Roles[i].Name = b.Name
				after.Roles[i].Description = b.Description
				after.Roles[i].AssignmentPolicy = b.AssignmentPolicy
				after.Roles[i].Status = "active"
			}
		}
		ExplainProject(&after, catalog)
		if hashValue(before) != hashValue(after) {
			plan.ProjectBefore[v.id] = before
			report.Projects = append(report.Projects, CatalogProjectImpact{TenantID: v.tenant, ProjectID: v.project, MembershipID: v.id, Added: difference(after.ConfiguredPermissions, before.ConfiguredPermissions), Removed: difference(before.ConfiguredPermissions, after.ConfiguredPermissions)})
		}
		if (before.ProjectStatus == "archived" || before.TenantStatus != "active") && len(difference(after.ConfiguredPermissions, before.ConfiguredPermissions)) > 0 {
			report.Conflicts = append(report.Conflicts, "归档项目或停用租户禁止预置扩权："+v.tenant+" / "+v.project)
		}
	}
	return nil
}

type CatalogProjectImpact struct {
	TenantID     string `json:"tenant_id"`
	ProjectID    string `json:"project_id"`
	MembershipID string `json:"membership_id"`
	Added        []Code `json:"added"`
	Removed      []Code `json:"removed"`
}
