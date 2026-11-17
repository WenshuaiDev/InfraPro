package projects_test

import (
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
)

func TestGovernanceOverviewFiltersPaginationAndTenantIsolation(t *testing.T) {
	f := setup(t)
	f.create(t, "live-a", f.aid)
	in := f.input(t, "live-b", f.bid)
	in.Owner = f.did
	_, err := f.s.Create(ctx, f.actor(f.a), f.tid, in, access.Meta{})
	must(t, err)
	archived := f.create(t, "closed", f.bid)
	f.change(t, f.b, archived.ProjectID, "", access.OpProjectArchive, projects.Change{})
	must(t, f.e.Service.SetStatus(ctx, "Bob", false, true, identity.Meta{Reason: "不可用管理员统计"}))
	other, err := f.ts.Create(ctx, f.actor(f.p), tenancy.CreateInput{Fields: tenancy.Fields{Name: "另一租户", Code: "other"}, AdministratorID: f.a.Account.ID, RequestID: f.uuid(t), Reason: "隔离统计"}, access.Meta{})
	must(t, err)
	tc, err := f.ts.Context(ctx, f.actor(f.a), other.TenantID)
	must(t, err)
	in = f.input(t, "outside", tc.MembershipID)
	in.Owner = tc.MembershipID
	_, err = f.s.Create(ctx, f.actor(f.a), other.TenantID, in, access.Meta{})
	must(t, err)

	check := func(filter projects.Filter, want int) projects.ProjectPage {
		t.Helper()
		page, err := f.s.List(ctx, f.actor(f.a), f.tid, filter, false)
		must(t, err)
		v := page.Overview
		if v == nil || v.Total != 3 || v.Active != 2 || v.Archived != 1 || v.Attention != 2 || v.FilteredTotal != want || len(v.Owners) != 2 {
			t.Fatalf("incorrect tenant totals for %+v: %+v", filter, v)
		}
		for _, owner := range v.Owners {
			if owner.MembershipID != f.cid && owner.MembershipID != f.did {
				t.Fatal("foreign owner", owner)
			}
		}
		return page
	}
	first := check(projects.Filter{Status: "all", Limit: 1}, 3)
	if len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatal("missing first page")
	}
	second := check(projects.Filter{Status: "all", Limit: 1, Cursor: first.NextCursor}, 3)
	if len(second.Items) != 1 || second.Items[0].Project.ID == first.Items[0].Project.ID {
		t.Fatal("cursor repeated a project")
	}
	check(projects.Filter{Status: "active"}, 2)
	check(projects.Filter{Status: "archived"}, 1)
	check(projects.Filter{Status: "all", Owner: f.cid}, 2)
	check(projects.Filter{Status: "all", Query: "live", Unavailable: "true"}, 1)
	check(projects.Filter{Status: "all", Unavailable: "false"}, 1)
	check(projects.Filter{Status: "all", Query: "no-match"}, 0)
	isolated, err := f.s.List(ctx, f.actor(f.a), other.TenantID, projects.Filter{Status: "all"}, false)
	must(t, err)
	if isolated.Overview.Total != 1 || len(isolated.Items) != 1 {
		t.Fatal("cross-tenant aggregation")
	}
	mine, err := f.s.List(ctx, f.actor(f.a), "", projects.Filter{Status: "all"}, true)
	must(t, err)
	if mine.Overview != nil {
		t.Fatal("governance overview leaked through my projects")
	}
	for _, item := range mine.Items {
		if item.Project.Statistics != nil {
			t.Fatal("statistics leaked through minimal context")
		}
	}
}

func TestGovernanceStatisticsRespectDomainAndProjectMembershipPermissions(t *testing.T) {
	f := setup(t)
	own := f.create(t, "own", f.aid)
	foreign := f.create(t, "not-member", f.bid)
	m := f.add(t, f.a, own.ProjectID, f.cid)
	f.change(t, f.a, own.ProjectID, m.ObjectID, access.OpProjectMemberSuspend, projects.Change{})
	stats := func(id string) *projects.GovernanceStatistics {
		t.Helper()
		p, err := f.s.Governance(ctx, f.actor(f.a), f.tid, id)
		must(t, err)
		if p.Statistics == nil {
			t.Fatal("statistics missing")
		}
		return p.Statistics
	}
	current := stats(own.ProjectID)
	if current.Members == nil || *current.Members != 2 {
		t.Fatal("current members must include suspended, exclude ended", current)
	}
	if current.Facilities == nil || *current.Facilities != 0 || current.Points == nil || *current.Points != 0 || current.Devices == nil || *current.Devices != 0 {
		t.Fatal("authorized empty counts must be zero", current)
	}
	if stats(foreign.ProjectID).Members != nil {
		t.Fatal("tenant administrator bypassed project member-read permission")
	}
	f.change(t, f.a, own.ProjectID, m.ObjectID, access.OpProjectMemberRemove, projects.Change{})
	if *stats(own.ProjectID).Members != 1 {
		t.Fatal("ended member remained in count")
	}

	as := f.e.Service.Access()
	role, err := as.CreateTenantRole(ctx, f.actor(f.a), f.tid, "", access.TenantRoleCreateInput{Name: "仅项目治理", Permissions: []access.Code{access.TenantProjectRead}, RequestID: f.uuid(t), Reason: "验证最小可见范围"}, access.Meta{})
	must(t, err)
	auth, err := as.TenantMemberAuthorization(ctx, f.actor(f.a), f.tid, f.cid)
	must(t, err)
	change := access.TenantChange{ExpectedMembershipVersion: auth.MembershipVersion, RoleIDs: []string{role.Role.ID}, Reason: "治理可见性验证"}
	preview, err := as.PreviewTenantChange(ctx, f.actor(f.a), f.tid, f.cid, access.OpTenantMemberRolesPreview, change, access.Filter{})
	must(t, err)
	change.PreviewHash, change.RoleVersions = preview.Hash, preview.RoleVersions
	_, err = as.ChangeTenantAuthorization(ctx, f.actor(f.a), f.tid, f.cid, access.OpTenantMemberRoles, change, access.Meta{})
	must(t, err)
	page, err := f.s.List(ctx, f.actor(f.c), f.tid, projects.Filter{}, false)
	must(t, err)
	if len(page.Items) != 2 {
		t.Fatal("governance-only reader lost project summaries")
	}
	for _, item := range page.Items {
		v := item.Project.Statistics
		if v == nil || v.Facilities != nil || v.Points != nil || v.Devices != nil || v.Members != nil {
			t.Fatal("domain counts disclosed without permission", v)
		}
	}
	_, err = f.s.Governance(ctx, f.actor(f.d), f.tid, own.ProjectID)
	code(t, err, "permission_denied")
}
