package derivedmonitoring_test

import (
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/google/uuid"
)

func TestDerivedWorkspaceTreeOwnersAndProjectCandidates(t *testing.T) {
	f := channelsSetup(t)
	actor := f.actor(f.admin)
	version := f.publish(t, f.create(t, "workspace").Definition)
	owners := []dm.Owner{{Kind: res.Facility, ID: f.facility, FacilityID: f.facility}, {Kind: res.Component, ID: f.component, FacilityID: f.facility}, {Kind: res.Point, ID: f.point, FacilityID: f.facility}}
	ids := map[res.Kind]string{}
	for _, owner := range owners {
		created, e := f.s.CreateChannel(ctx, actor, f.scope, dm.CreateChannelInput{WriteMeta: wm(""), Owner: owner, Code: "DERIVED", Name: "直属派生", DefinitionVersionID: version.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: f.direct}}}, access.Meta{})
		must(t, e)
		ids[owner.Kind] = created.Channel.Resource.ID
		tree, e := f.r.Tree(ctx, actor, f.scope.Tenant, res.TreeFilter{FacilityID: f.facility, ParentKind: owner.Kind, ParentID: owner.ID, Filter: res.Filter{Query: "直属派生", Status: "all"}})
		must(t, e)
		if len(tree.Items) != 1 || tree.Items[0].ID != ids[owner.Kind] || tree.Items[0].ParentID == nil || *tree.Items[0].ParentID != owner.ID || *tree.Items[0].ParentKind != owner.Kind {
			t.Fatalf("wrong direct owner in resource tree: %+v", tree)
		}
		page, e := f.r.List(ctx, actor, res.TenantScope(f.scope.Tenant), res.Target{Kind: res.Channel, Facility: f.facility}, res.Filter{OwnerKind: string(owner.Kind), OwnerID: owner.ID, Query: "直属派生"})
		must(t, e)
		if len(page.Items) != 1 || page.Items[0].ID != ids[owner.Kind] {
			t.Fatalf("mixed owner-scoped channel list: %+v", page)
		}
	}
	tenant, e := tenancy.New(f.e.Service.Access()).Context(ctx, actor, f.scope.Tenant)
	must(t, e)
	project, e := projects.New(f.e.Service.Access()).Create(ctx, actor, f.scope.Tenant, projects.CreateInput{Fields: projects.Fields{Name: "工作台项目", Code: "workspace", Purpose: "验证三类直属通道的项目可见性"}, WriteMeta: projects.WriteMeta{RequestID: uuid.NewString(), Reason: "合并验证"}, Owner: tenant.MembershipID, Administrator: tenant.MembershipID}, access.Meta{})
	must(t, e)
	for _, owner := range owners {
		candidates, e := f.r.AccessDetail(ctx, actor, f.scope.Tenant, project.ProjectID, f.facility, res.AccessFilter{CandidateKind: res.Channel, Candidate: res.Filter{OwnerKind: string(owner.Kind), OwnerID: owner.ID, Query: "直属派生"}})
		must(t, e)
		if len(candidates.Candidates.Items) != 1 || candidates.Candidates.Items[0].ID != ids[owner.Kind] || candidates.Candidates.Items[0].SourceKind != "derived" {
			t.Fatalf("wrong authorization candidates: %+v", candidates.Candidates)
		}
	}
	current, e := f.r.AccessDetail(ctx, actor, f.scope.Tenant, project.ProjectID, f.facility, res.AccessFilter{})
	must(t, e)
	componentChannel := ids[res.Component]
	grant := res.AccessInput{RequestID: uuid.NewString(), AccessRevision: current.Policy.Revision, Action: "create_link", Reason: "仅开放构件直属通道", Add: []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &componentChannel, Capabilities: []string{"read", "manage_calculation"}}}}
	preview, e := f.r.PreviewAccessChange(ctx, actor, f.scope.Tenant, project.ProjectID, f.facility, grant)
	must(t, e)
	grant.PreviewHash = preview.Hash
	_, e = f.r.ChangeAccess(ctx, actor, f.scope.Tenant, project.ProjectID, f.facility, grant, access.Meta{})
	must(t, e)
	for _, owner := range owners {
		page, e := f.r.List(ctx, actor, res.ProjectScope(f.scope.Tenant, project.ProjectID), res.Target{Kind: res.Channel, Facility: f.facility}, res.Filter{OwnerKind: string(owner.Kind), OwnerID: owner.ID})
		must(t, e)
		if owner.Kind == res.Component {
			if len(page.Items) != 1 || page.Items[0].ID != componentChannel {
				t.Fatal("project lost its authorized derived channel", page)
			}
		} else if len(page.Items) != 0 {
			t.Fatal("project directory fell back to tenant resources", page)
		}
	}
	_, e = f.r.List(ctx, actor, res.TenantScope(f.scope.Tenant), res.Target{Kind: res.Channel, Facility: f.facility}, res.Filter{OwnerKind: "component"})
	if e == nil {
		t.Fatal("partial owner filter accepted")
	}
}
