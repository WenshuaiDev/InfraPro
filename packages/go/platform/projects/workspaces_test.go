package projects_test

import (
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/jackc/pgx/v5"
)

func TestWorkspaceDiscoveryAuthorizationAndUnfilteredSummary(t *testing.T) {
	f := setup(t)
	s := f.e.Service.Access()
	read := func(who identity.Principal, filter access.WorkspaceFilter) access.Workspaces {
		t.Helper()
		v, e := s.MyWorkspaces(ctx, f.actor(who), filter)
		must(t, e)
		return v
	}
	clearTenantRoles := func(member string) {
		t.Helper()
		auth, e := s.TenantMemberAuthorization(ctx, f.actor(f.a), f.tid, member)
		must(t, e)
		in := access.TenantChange{ExpectedMembershipVersion: auth.MembershipVersion, RoleIDs: []string{}, Reason: "仅授权项目入口"}
		preview, e := s.PreviewTenantChange(ctx, f.actor(f.a), f.tid, member, access.OpTenantMemberRolesPreview, in, access.Filter{})
		must(t, e)
		in.PreviewHash, in.RoleVersions = preview.Hash, preview.RoleVersions
		_, e = s.ChangeTenantAuthorization(ctx, f.actor(f.a), f.tid, member, access.OpTenantMemberRoles, in, access.Meta{})
		must(t, e)
	}
	if v := read(f.b, access.WorkspaceFilter{}); v.Summary.Mode != "single" || v.Summary.Single.Kind != "tenant" {
		t.Fatal(v)
	}
	clearTenantRoles(f.bid)
	if v := read(f.b, access.WorkspaceFilter{}); v.Summary.Mode != "none" || len(v.Items) != 0 {
		t.Fatal("membership inferred entry", v)
	}
	tenantContext, e := f.ts.Context(ctx, f.actor(f.b), f.tid)
	must(t, e)
	if tenantContext.CanEnter {
		t.Fatal("tenant context disagrees with discovery")
	}
	p := f.create(t, "only-project", f.bid)
	v := read(f.b, access.WorkspaceFilter{Query: "not-a-match", Kind: "tenant", Limit: 1})
	if len(v.Items) != 0 || v.Summary.Mode != "single" || v.Summary.Single.ID != p.ProjectID || v.Summary.TenantCount != 0 || v.Summary.Platform != nil {
		t.Fatal("filters changed the full entry summary", v)
	}
	if v := read(f.a, access.WorkspaceFilter{}); v.Summary.Single == nil || v.Summary.Single.Kind != "tenant" {
		t.Fatal("tenant governance conferred project entry", v)
	}
	if v := read(f.p, access.WorkspaceFilter{}); v.Summary.Single == nil || v.Summary.Single.Kind != "platform" {
		t.Fatal("platform rights conferred tenant/project entry", v)
	}
	q := f.create(t, "second-project", f.bid)
	f.create(t, "hidden-project", f.did)
	v = read(f.b, access.WorkspaceFilter{Limit: 1})
	if v.Summary.Mode != "multiple" || v.Summary.Single != nil || len(v.Items) != 1 || v.NextCursor == "" {
		t.Fatal(v)
	}
	next := read(f.b, access.WorkspaceFilter{Limit: 1, Cursor: v.NextCursor})
	if len(next.Items) != 1 || next.Items[0].ID == v.Items[0].ID || next.NextCursor != "" || next.Summary.ProjectCount != 2 {
		t.Fatal(next)
	}
	if v := read(f.b, access.WorkspaceFilter{Query: "hidden-project"}); len(v.Items) != 0 || v.Summary.ProjectCount != 2 {
		t.Fatal("hidden target or count leaked", v)
	}
	f.change(t, f.b, q.ProjectID, "", access.OpProjectArchive, projects.Change{})
	v = read(f.b, access.WorkspaceFilter{Status: "archived"})
	if len(v.Items) != 1 || v.Items[0].ID != q.ProjectID || v.Summary.Mode != "multiple" {
		t.Fatal("archive/read access or summary mismatch", v)
	}
	clearTenantRoles(f.cid)
	// Ownership alone does not authorize the project; explicit membership does.
	if v := read(f.c, access.WorkspaceFilter{}); v.Summary.Mode != "none" {
		t.Fatal(v)
	}
	m := f.add(t, f.b, p.ProjectID, f.cid)
	if v := read(f.c, access.WorkspaceFilter{}); v.Summary.Mode != "single" || v.Summary.Single.ID != p.ProjectID {
		t.Fatal(v)
	}
	f.change(t, f.b, p.ProjectID, m.ObjectID, access.OpProjectMemberSuspend, projects.Change{})
	if v := read(f.c, access.WorkspaceFilter{}); v.Summary.Mode != "none" {
		t.Fatal("suspended membership visible", v)
	}
	f.change(t, f.b, p.ProjectID, m.ObjectID, access.OpProjectMemberResume, projects.Change{})
	f.change(t, f.b, p.ProjectID, m.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{}})
	if v := read(f.c, access.WorkspaceFilter{}); v.Summary.Mode != "none" {
		t.Fatal("revoked roles visible", v)
	}
	projectContext, e := f.s.Context(ctx, f.actor(f.c), f.tid, p.ProjectID)
	must(t, e)
	if projectContext.CanEnter {
		t.Fatal("project context disagrees with discovery")
	}
	// Fixture lifecycle change under the same authorization lock as production.
	must(t, s.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "UPDATE tenants SET status='disabled',status_notice='入口测试停用',status_changed_at=now(),status_changed_by=$2,status_change_reason='入口测试' WHERE id=$1", f.tid, f.p.Account.ID); e != nil {
			return e
		}
		return access.Bump(ctx, tx)
	}))
	if v := read(f.b, access.WorkspaceFilter{}); v.Summary.Mode != "none" || len(v.Items) != 0 {
		t.Fatal("disabled ancestor visible", v)
	}
}

func TestWorkspaceLandingUsesScopedBusinessPermissions(t *testing.T) {
	for _, c := range []struct {
		kind, status string
		codes        []access.Code
		want         string
	}{
		{"tenant", "active", nil, ""}, {"project", "active", []access.Code{access.TenantProfileRead}, ""},
		{"platform", "active", []access.Code{access.ProfileRead, access.SessionRead}, ""},
		{"tenant", "disabled", []access.Code{access.TenantProfileRead}, ""},
		{"project", "archived", []access.Code{access.ProjectProfileUpdate}, ""},
		{"project", "active", []access.Code{access.ProjectEngineeringResourceRead}, "/t/t/p/p/resources/facilities"},
		{"project", "archived", []access.Code{access.ProjectProfileRead}, "/t/t/p/p/profile"},
		{"platform", "active", []access.Code{access.DeviceDefinitionRead}, "/platform/device-products"},
		{"tenant", "active", []access.Code{access.TenantDeviceRead}, "/t/t/devices"},
		{"tenant", "active", []access.Code{access.TenantDeviceDefinitionRead}, "/t/t/device-products"},
		{"tenant", "active", []access.Code{access.TenantDeviceCatalogRead}, "/t/t/device-products"},
		{"project", "active", []access.Code{access.ProjectDeviceRead}, "/t/t/p/p/devices"},
		{"project", "archived", []access.Code{access.ProjectDeviceRead}, "/t/t/p/p/devices"},
	} {
		if got := access.WorkspacePath(c.kind, "t", "p", c.status, c.codes); got != c.want {
			t.Fatalf("%+v: got %q", c, got)
		}
	}
}
