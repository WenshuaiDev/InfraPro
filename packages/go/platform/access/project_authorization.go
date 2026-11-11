package access

import (
	"context"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
)

var ErrProjectNotFound = &Error{Code: "project_not_found", Message: "当前项目不可访问。", Status: 404}
var ErrProjectAdministrator = &Error{Code: "project_admin_transfer_required", Message: "该账号或成员仍承担项目管理员职责，请先交接或修复项目管理（含已归档项目）。", Status: 409}

type ProjectAuthorization struct {
	Snapshot
	TenantID               string           `json:"tenant_id"`
	ProjectID              string           `json:"project_id"`
	TenantMembershipID     string           `json:"tenant_membership_id"`
	TenantStatus           string           `json:"tenant_status"`
	TenantMembershipStatus string           `json:"tenant_membership_status"`
	ProjectStatus          string           `json:"project_status"`
	MembershipID           string           `json:"membership_id"`
	MembershipVersion      string           `json:"membership_version"`
	MembershipStatus       string           `json:"membership_status"`
	AccountID              string           `json:"account_id"`
	AccountStatus          string           `json:"account_status"`
	Username               string           `json:"username"`
	DisplayName            string           `json:"display_name"`
	Administrator          bool             `json:"is_current_administrator"`
	ConfiguredPermissions  []Code           `json:"configured_permissions"`
	RestorablePermissions  []Code           `json:"restorable_permissions"`
	InactiveReasons        []string         `json:"inactive_reasons"`
	AllowedOperations      []Operation      `json:"allowed_operations"`
	Bindings               []ProjectBinding `json:"bindings"`
}
type ProjectBinding struct {
	ID         string `json:"id"`
	RoleID     string `json:"role_id"`
	RoleName   string `json:"role_name"`
	AssignedAt string `json:"assigned_at"`
	RevokedAt  string `json:"revoked_at"`
}

func ProjectRoleTx(ctx context.Context, tx pgx.Tx, tenant, project, id string) (Role, error) {
	if !ValidID(id) {
		return Role{}, ErrNotFound
	}
	r, e := roleTx(ctx, tx, id)
	if e == nil && (r.BindingScope != "project" || r.OwnerProjectID != "" && (r.OwnerProjectID != project || r.OwnerTenantID != tenant)) {
		return Role{}, ErrNotFound
	}
	if e != nil {
		return r, e
	}
	e = tx.QueryRow(ctx, `SELECT count(*) FROM project_member_roles WHERE role_id=$1 AND project_id::text=$2 AND revoked_at IS NULL`, id, project).Scan(&r.AccountCount)
	return r, e
}
func projectCatalogTx(ctx context.Context, tx pgx.Tx) (map[Code]Permission, error) {
	rows, e := tx.Query(ctx, `SELECT code,lifecycle,project_custom_role_assignable FROM permission_catalog WHERE scope='project' ORDER BY code`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[Code]Permission{}
	for rows.Next() {
		var p Permission
		p.Scope = "project"
		if e = rows.Scan(&p.Code, &p.Lifecycle, &p.ProjectCustomRoleAssignable); e != nil {
			return nil, e
		}
		out[p.Code] = p
	}
	return out, rows.Err()
}
func ExplainProject(v *ProjectAuthorization, catalog map[Code]Permission) {
	v.Permissions = []Code{}
	v.ConfiguredPermissions = []Code{}
	v.RestorablePermissions = []Code{}
	v.Sources = map[Code][]string{}
	v.InactiveReasons = []string{}
	v.AllowedOperations = []Operation{}
	if v.TenantStatus != "active" {
		v.InactiveReasons = append(v.InactiveReasons, "tenant_disabled")
	}
	if v.TenantMembershipStatus != "active" {
		v.InactiveReasons = append(v.InactiveReasons, "tenant_membership_"+v.TenantMembershipStatus)
	}
	if v.MembershipStatus != "active" {
		v.InactiveReasons = append(v.InactiveReasons, "membership_"+v.MembershipStatus)
	}
	if v.AccountStatus != "active" {
		v.InactiveReasons = append(v.InactiveReasons, "account_unavailable")
	}
	eligible := len(v.InactiveReasons) == 0
	for _, r := range v.Roles {
		for _, code := range r.Permissions {
			p, ok := catalog[code]
			if !ok || p.Lifecycle != "active" {
				continue
			}
			if !slices.Contains(v.ConfiguredPermissions, code) {
				v.ConfiguredPermissions = append(v.ConfiguredPermissions, code)
			}
			if r.Status != "active" {
				continue
			}
			if !slices.Contains(v.RestorablePermissions, code) {
				v.RestorablePermissions = append(v.RestorablePermissions, code)
			}
			if eligible {
				if !slices.Contains(v.Permissions, code) {
					v.Permissions = append(v.Permissions, code)
				}
				v.Sources[code] = append(v.Sources[code], r.ID)
			}
		}
	}
	slices.Sort(v.Permissions)
	slices.Sort(v.ConfiguredPermissions)
	slices.Sort(v.RestorablePermissions)
	if eligible && len(v.Permissions) == 0 {
		v.InactiveReasons = append(v.InactiveReasons, "no_project_permissions")
	}
	for _, p := range Policies() {
		if p.Class != "project" {
			continue
		}
		allowed := eligible
		for _, code := range p.Permissions {
			allowed = allowed && slices.Contains(v.Permissions, code)
		}
		if v.ProjectStatus == "archived" && ProjectActiveOnly(p.Operation) {
			allowed = false
		}
		if allowed {
			v.AllowedOperations = append(v.AllowedOperations, p.Operation)
		}
	}
}
func ProjectActiveOnly(op Operation) bool {
	if strings.HasPrefix(string(op), "ProjectResource:") {
		p, ok := PolicyFor(op)
		return ok && p.Method != "GET"
	}
	return slices.Contains([]Operation{OpProjectUpdate, OpProjectOwnerChange, OpProjectOwnerChangePreview, OpProjectMemberAdd, OpProjectMemberResume, OpProjectMemberResumePreview, OpProjectRoleCreate, OpProjectRoleCopy, OpProjectRoleUpdate, OpProjectArchive, OpProjectArchivePreview}, op)
}
func ProjectAuthorizationTx(ctx context.Context, tx pgx.Tx, tenant, project, member, revision string) (ProjectAuthorization, error) {
	v := ProjectAuthorization{Snapshot: Snapshot{Revision: revision, Roles: []Role{}}, Bindings: []ProjectBinding{}}
	if !ValidID(tenant) || !ValidID(project) || !ValidID(member) {
		return v, ErrProjectNotFound
	}
	e := tx.QueryRow(ctx, `SELECT m.tenant_id::text,m.project_id::text,m.tenant_membership_id::text,t.status,tm.status,p.status,m.id::text,m.version::text,m.status,a.id::text,a.status,a.username,a.display_name,COALESCE(m.id=p.administrator_membership_id,false) FROM project_memberships m JOIN projects p ON p.id=m.project_id JOIN tenants t ON t.id=m.tenant_id JOIN tenant_memberships tm ON tm.id=m.tenant_membership_id JOIN accounts a ON a.id=m.account_id WHERE m.tenant_id=$1 AND m.project_id=$2 AND m.id=$3`, tenant, project, member).Scan(&v.TenantID, &v.ProjectID, &v.TenantMembershipID, &v.TenantStatus, &v.TenantMembershipStatus, &v.ProjectStatus, &v.MembershipID, &v.MembershipVersion, &v.MembershipStatus, &v.AccountID, &v.AccountStatus, &v.Username, &v.DisplayName, &v.Administrator)
	if e != nil {
		return v, notFound(e)
	}
	rows, e := tx.Query(ctx, `SELECT mr.id::text,mr.role_id::text,r.name,mr.assigned_at::text,COALESCE(mr.revoked_at::text,'') FROM project_member_roles mr JOIN roles r ON r.id=mr.role_id WHERE mr.membership_id=$1 ORDER BY mr.assigned_at,mr.id`, member)
	if e != nil {
		return v, e
	}
	for rows.Next() {
		var b ProjectBinding
		if e = rows.Scan(&b.ID, &b.RoleID, &b.RoleName, &b.AssignedAt, &b.RevokedAt); e != nil {
			rows.Close()
			return v, e
		}
		v.Bindings = append(v.Bindings, b)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return v, e
	}
	for _, b := range v.Bindings {
		if b.RevokedAt != "" {
			continue
		}
		r, e := ProjectRoleTx(ctx, tx, tenant, project, b.RoleID)
		if e != nil {
			return v, ErrCatalog
		}
		v.Roles = append(v.Roles, r)
	}
	catalog, e := projectCatalogTx(ctx, tx)
	if e != nil {
		return v, e
	}
	ExplainProject(&v, catalog)
	return v, nil
}
func ProjectSnapshotTx(ctx context.Context, tx pgx.Tx, account, tenant, project, revision string) (ProjectAuthorization, error) {
	var v ProjectAuthorization
	if !ValidID(tenant) || !ValidID(project) {
		return v, ErrProjectNotFound
	}
	var member string
	e := tx.QueryRow(ctx, `SELECT m.id::text FROM project_memberships m JOIN tenant_memberships tm ON tm.id=m.tenant_membership_id JOIN accounts a ON a.id=m.account_id WHERE m.tenant_id=$1 AND m.project_id=$2 AND m.account_id=$3 AND m.status<>'ended' AND tm.status='active' AND a.status='active'`, tenant, project, account).Scan(&member)
	if e == pgx.ErrNoRows {
		return v, ErrProjectNotFound
	}
	if e != nil {
		return v, e
	}
	return ProjectAuthorizationTx(ctx, tx, tenant, project, member, revision)
}
func RequireProjectTx(ctx context.Context, tx pgx.Tx, account string, op Operation, tenant, project string) (ProjectAuthorization, error) {
	v, e := ProjectSnapshotTx(ctx, tx, account, tenant, project, "")
	if e != nil {
		return v, e
	}
	p, ok := PolicyFor(op)
	if !ok {
		return v, ErrDenied
	}
	if p.Class == "project_context" {
		return v, nil
	}
	if p.Class != "project" {
		return v, ErrDenied
	}
	if v.TenantStatus != "active" {
		return v, ErrTenantDisabled
	}
	if v.MembershipStatus != "active" {
		return v, ErrDenied
	}
	for _, code := range p.Permissions {
		if !slices.Contains(v.Permissions, code) {
			return v, ErrDenied
		}
	}
	if v.ProjectStatus == "archived" && ProjectActiveOnly(op) && op != OpProjectArchive && op != OpProjectArchivePreview {
		return v, &Error{Code: "project_archived", Message: "项目已归档，仅可读取当前获准资源。", Status: 409}
	}
	return v, nil
}
func CheckProjectStructure(ctx context.Context, tx pgx.Tx) error {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT project_roles_valid()`).Scan(&ok)
	if e == nil && !ok {
		return ErrCatalog
	}
	return e
}
