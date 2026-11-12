package access

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const OpMyWorkspaces Operation = "MyWorkspaces"

type WorkspaceEntry struct {
	Kind        string   `json:"kind"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Code        string   `json:"code"`
	TenantID    string   `json:"tenant_id,omitempty"`
	TenantName  string   `json:"tenant_name,omitempty"`
	Status      string   `json:"status"`
	Roles       []string `json:"roles"`
	DefaultPath string   `json:"default_path"`
}
type WorkspaceSummary struct {
	Mode         string          `json:"mode"`
	Single       *WorkspaceEntry `json:"single,omitempty"`
	Platform     *WorkspaceEntry `json:"platform,omitempty"`
	TenantCount  int             `json:"tenant_count"`
	ProjectCount int             `json:"project_count"`
}
type Workspaces struct {
	Items      []WorkspaceEntry `json:"items"`
	NextCursor string           `json:"next_cursor,omitempty"`
	Summary    WorkspaceSummary `json:"summary"`
	Revision   string           `json:"authorization_revision"`
}
type WorkspaceFilter struct {
	Kind, Query, Status, TenantID, Cursor string
	Limit                                 int
}

// WorkspacePath consumes effective permissions, never configured roles or membership alone.
// Contexts and the discovery projection share this entry predicate. Object grants
// continue to be enforced by each resource operation, including empty/new projects.
func WorkspacePath(kind, tenant, project, status string, permissions []Code) string {
	type destination struct {
		path  string
		codes []Code
	}
	var destinations []destination
	base := "/t/" + tenant
	switch kind {
	case "platform":
		destinations = []destination{
			{"/platform/tenants", []Code{TenantRead, TenantCreate, TenantUpdate, TenantTransfer}},
			{"/platform/engineering/type-definitions", []Code{EngineeringTypeRead}},
			{"/platform/engineering/metrics", []Code{EngineeringMetricRead}},
			{"/platform/engineering/monitoring-templates", []Code{EngineeringTemplateRead}},
			{"/platform/device-products", []Code{DeviceDefinitionRead}},
			{"/access/accounts", []Code{AccountRead}}, {"/invitations", []Code{InvitationRead}},
			{"/access/roles", []Code{RoleRead}}, {"/access/audit", []Code{AuditRead}},
		}
	case "tenant":
		if status != "active" {
			return ""
		}
		destinations = []destination{
			{base + "/profile", []Code{TenantProfileRead}}, {base + "/projects", []Code{TenantProjectRead}},
			{base + "/resources/facilities", []Code{TenantEngineeringResourceRead}},
			{base + "/devices", []Code{TenantDeviceRead}},
			{base + "/device-products", []Code{TenantDeviceDefinitionRead, TenantDeviceCatalogRead}},
			{base + "/members", []Code{TenantMemberRead}}, {base + "/invitations", []Code{TenantInvitationRead}},
			{base + "/roles", []Code{TenantRoleRead}}, {base + "/audit", []Code{TenantAuditRead}},
			{base + "/engineering/catalog/type-definitions", []Code{TenantEngineeringCatalogRead}},
			{base + "/engineering/templates", []Code{TenantEngineeringTemplateRead}},
		}
	case "project":
		if status != "active" && status != "archived" {
			return ""
		}
		base += "/p/" + project
		destinations = []destination{
			{base + "/profile", []Code{ProjectProfileRead}}, {base + "/resources/facilities", []Code{ProjectEngineeringResourceRead}},
			{base + "/devices", []Code{ProjectDeviceRead}},
			{base + "/members", []Code{ProjectMemberRead}}, {base + "/roles", []Code{ProjectRoleRead}},
			{base + "/permissions", []Code{ProjectPermissionRead}}, {base + "/audit", []Code{ProjectAuditRead}},
		}
	default:
		return ""
	}
	for _, d := range destinations {
		for _, code := range d.codes {
			if slices.Contains(permissions, code) {
				return d.path
			}
		}
	}
	// A write-only role can still enter to inspect its own business authorization.
	// Metadata/status operations without business permissions never confer entry.
	if kind == "tenant" || kind == "project" {
		for _, p := range Policies() {
			if p.Class != kind || len(p.Permissions) == 0 || (kind == "project" && status == "archived" && ProjectActiveOnly(p.Operation)) {
				continue
			}
			if !slices.ContainsFunc(p.Permissions, func(c Code) bool { return !slices.Contains(permissions, c) }) {
				return base + "/profile"
			}
		}
	}
	return ""
}

func workspaceKey(v WorkspaceEntry) string { return v.Kind + ":" + v.ID }

// MyWorkspaces projects only actual business entry rights. The authorization lock
// keeps the full summary and paginated results on the same revision. Batched
// membership/role reads avoid a context request for every candidate workspace.
func (s *Service) MyWorkspaces(ctx context.Context, a Actor, f WorkspaceFilter) (Workspaces, error) {
	out := Workspaces{Items: []WorkspaceEntry{}, Summary: WorkspaceSummary{Mode: "none"}}
	if f.Limit == 0 {
		f.Limit = 50
	}
	if f.Limit < 1 || f.Limit > 100 {
		return out, Validation("limit", "每页数量为 1–100。")
	}
	if !slices.Contains([]string{"", "all", "tenant", "project", "platform"}, f.Kind) {
		return out, Validation("kind", "工作空间类型无效。")
	}
	if !slices.Contains([]string{"", "all", "active", "archived"}, f.Status) {
		return out, Validation("status", "项目状态无效。")
	}
	if !utf8.ValidString(f.Query) || utf8.RuneCountInString(f.Query) > 100 {
		return out, Validation("q", "搜索内容最多 100 字符。")
	}
	if f.TenantID != "" && !ValidID(f.TenantID) {
		return out, Validation("tenant_id", "租户编号无效。")
	}
	if f.Cursor != "" {
		kind, id, ok := strings.Cut(f.Cursor, ":")
		if !ok || !slices.Contains([]string{"tenant", "project", "platform"}, kind) || !(ValidID(id) || kind == "platform" && id == "platform") {
			return out, Validation("cursor", "分页位置无效。")
		}
	}
	err := s.Transact(ctx, func(tx pgx.Tx) error {
		rev, e := Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		if a.Check == nil {
			return ErrDenied
		}
		if e = a.Check(ctx, tx, false); e != nil {
			return e
		}
		out.Revision = rev
		all := []WorkspaceEntry{}
		permissions, e := effective(ctx, tx, a.ID, false)
		if e != nil {
			return e
		}
		if path := WorkspacePath("platform", "", "", "active", permissions); path != "" {
			entry := WorkspaceEntry{Kind: "platform", ID: "platform", Name: "平台工作空间", Code: "platform", Status: "active", Roles: []string{}, DefaultPath: path}
			out.Summary.Platform = &entry
			all = append(all, entry)
		}
		rows, e := tx.Query(ctx, `
SELECT 'tenant',t.id::text,t.name,t.code,t.id::text,t.name,t.status,
 array_agg(DISTINCT r.name ORDER BY r.name),array_agg(DISTINCT pc.code ORDER BY pc.code)
FROM tenant_memberships m JOIN accounts a ON a.id=m.account_id JOIN tenants t ON t.id=m.tenant_id
JOIN tenant_member_roles mr ON mr.membership_id=m.id AND mr.revoked_at IS NULL
JOIN roles r ON r.id=mr.role_id AND r.status='active' AND r.deleted_at IS NULL
JOIN role_permissions rp ON rp.role_id=r.id JOIN permission_catalog pc ON pc.code=rp.permission_code AND pc.scope='tenant' AND pc.lifecycle='active'
WHERE m.account_id=$1 AND a.status='active' AND m.status='active' AND t.status='active'
GROUP BY t.id
UNION ALL
SELECT 'project',p.id::text,p.name,p.code,t.id::text,t.name,p.status,
 array_agg(DISTINCT r.name ORDER BY r.name),array_agg(DISTINCT pc.code ORDER BY pc.code)
FROM project_memberships m JOIN accounts a ON a.id=m.account_id JOIN projects p ON p.id=m.project_id AND p.tenant_id=m.tenant_id
JOIN tenants t ON t.id=m.tenant_id JOIN tenant_memberships tm ON tm.id=m.tenant_membership_id AND tm.tenant_id=m.tenant_id AND tm.account_id=m.account_id
JOIN project_member_roles mr ON mr.membership_id=m.id AND mr.revoked_at IS NULL
JOIN roles r ON r.id=mr.role_id AND r.status='active' AND r.deleted_at IS NULL
JOIN role_permissions rp ON rp.role_id=r.id JOIN permission_catalog pc ON pc.code=rp.permission_code AND pc.scope='project' AND pc.lifecycle='active'
WHERE m.account_id=$1 AND a.status='active' AND m.status='active' AND tm.status='active' AND t.status='active'
GROUP BY p.id,t.id`, a.ID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v WorkspaceEntry
			var codes []Code
			if e = rows.Scan(&v.Kind, &v.ID, &v.Name, &v.Code, &v.TenantID, &v.TenantName, &v.Status, &v.Roles, &codes); e != nil {
				return e
			}
			v.DefaultPath = WorkspacePath(v.Kind, v.TenantID, v.ID, v.Status, codes)
			if v.DefaultPath == "" {
				continue
			}
			if v.Kind == "tenant" {
				out.Summary.TenantCount++
			} else {
				out.Summary.ProjectCount++
			}
			all = append(all, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		slices.SortFunc(all, func(a, b WorkspaceEntry) int { return strings.Compare(workspaceKey(a), workspaceKey(b)) })
		if len(all) == 1 {
			out.Summary.Mode = "single"
			out.Summary.Single = &all[0]
		} else if len(all) > 1 {
			out.Summary.Mode = "multiple"
		}
		q := strings.ToLower(strings.TrimSpace(f.Query))
		for _, v := range all {
			if f.Kind != "" && f.Kind != "all" && f.Kind != v.Kind || f.TenantID != "" && v.TenantID != f.TenantID || v.Kind == "project" && f.Status != "" && f.Status != "all" && v.Status != f.Status {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(v.Name), q) && !strings.Contains(strings.ToLower(v.Code), q) {
				continue
			}
			if workspaceKey(v) <= f.Cursor {
				continue
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = workspaceKey(out.Items[len(out.Items)-1])
				break
			}
			out.Items = append(out.Items, v)
		}
		return nil
	})
	return out, err
}
