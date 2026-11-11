package resources_test

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
)

type projectFixture struct {
	id    string
	actor access.Actor
}

func (f resourceFixture) project(t *testing.T, name string) projectFixture {
	t.Helper()
	u := f.member(t, name)
	actor := f.fixture.actor(u)
	ts := tenancy.New(f.env.Service.Access())
	inv, e := ts.CreateInvitation(ctx, f.actor, f.tenant, tenancy.InvitationCreateInput{TargetID: u.Account.ID, RequestID: f.wm(t).RequestID, Reason: "项目资源验证"}, access.Meta{})
	must(t, e)
	parsed, e := url.Parse(inv.Link)
	must(t, e)
	q, e := url.ParseQuery(parsed.Fragment)
	must(t, e)
	member, e := ts.AcceptInvitation(ctx, actor, tenancy.AcceptanceInput{Token: q.Get("token"), Version: inv.Invitation.Version}, access.Meta{})
	must(t, e)
	ps := projects.New(f.env.Service.Access())
	v, e := ps.Create(ctx, f.actor, f.tenant, projects.CreateInput{Fields: projects.Fields{Name: name, Code: name, Purpose: "资源共享验证"}, WriteMeta: projects.WriteMeta{RequestID: f.wm(t).RequestID, Reason: "项目资源验证"}, Owner: member.MembershipID, Administrator: member.MembershipID}, access.Meta{})
	must(t, e)
	return projectFixture{v.ProjectID, actor}
}
func (f resourceFixture) accessInput(t *testing.T, p projectFixture, facility, action string) res.AccessInput {
	t.Helper()
	d, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, p.id, facility, res.AccessFilter{})
	must(t, e)
	in := res.AccessInput{RequestID: f.wm(t).RequestID, Action: action, AccessRevision: d.Policy.Revision, Reason: "明确授权范围"}
	if d.Link != nil {
		in.LinkRevision = d.Link.Revision
	}
	return in
}
func (f resourceFixture) grant(t *testing.T, p projectFixture, facility string, in res.AccessInput) res.AccessResult {
	t.Helper()
	preview, e := f.srv.PreviewAccessChange(ctx, f.actor, f.tenant, p.id, facility, in)
	must(t, e)
	in.PreviewHash = preview.Hash
	out, e := f.srv.ChangeAccess(ctx, f.actor, f.tenant, p.id, facility, in, access.Meta{})
	must(t, e)
	return out
}
func (f resourceFixture) projectWrite(t *testing.T, p projectFixture, target res.Target, action string, in map[string]any) (res.Result, error) {
	t.Helper()
	d, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, p.id, target.Facility, res.AccessFilter{})
	if e != nil {
		return res.Result{}, e
	}
	in["request_id"] = f.wm(t).RequestID
	in["expected_access_revision"] = d.Policy.Revision
	if d.Link != nil {
		in["expected_link_revision"] = d.Link.Revision
	}
	raw, _ := json.Marshal(in)
	return f.srv.Mutate(ctx, p.actor, res.ProjectScope(f.tenant, p.id), target, action, raw, access.Meta{})
}
func TestProjectResourceExplicitGrantsContextAndCreation(t *testing.T) {
	f := resourcesSetup(t)
	a := f.project(t, "ProjectA")
	b := f.project(t, "ProjectB")
	facility := f.facility(t, "SHARED")
	component, e := f.write(t, target(facility, res.Component, ""), "create", map[string]any{"code": "C", "name": "完整构件资料", "category_key": "span", "description": "不应给上下文公开", "expected_structure_revision": 1})
	must(t, e)
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "测点", "component_id": component.ID, "purpose": "不应给上下文公开"})
	must(t, e)
	ct := target(facility, res.Channel, "")
	ct.Point = point.ID
	ch, e := f.write(t, ct, "create", map[string]any{"code": "CH", "name": "通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	root := res.GrantInput{Kind: res.Facility, Capabilities: []string{"read"}}
	in := f.accessInput(t, a, facility.ID, "create_link")
	in.Add = []res.GrantInput{root, {Kind: res.Point, PointID: &point.ID, Capabilities: []string{"read", "update", "create_channel", "lifecycle"}}, {Kind: res.Channel, ChannelID: &ch.ID, Capabilities: []string{"read", "update", "lifecycle"}}}
	f.grant(t, a, facility.ID, in)
	in = f.accessInput(t, b, facility.ID, "create_link")
	in.Add = []res.GrantInput{root, {Kind: res.Channel, ChannelID: &ch.ID, Capabilities: []string{"read"}}}
	f.grant(t, b, facility.ID, in)
	// Candidate channel pages follow the selected component through the point owner.
	for owner, count := range map[string]int{"root": 0, component.ID: 1} {
		candidates, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, a.id, facility.ID, res.AccessFilter{CandidateKind: res.Channel, Candidate: res.Filter{Owner: owner}})
		must(t, e)
		if len(candidates.Candidates.Items) != count {
			t.Fatal("channel candidates ignored component owner", owner, candidates.Candidates.Items)
		}
	}
	scope := res.ProjectScope(f.tenant, b.id)
	ct.ID = ch.ID
	detail, e := f.srv.Detail(ctx, b.actor, scope, ct)
	must(t, e)
	if detail.ID != ch.ID || detail.Actions.Edit || len(detail.Ancestors) != 3 || detail.Ancestors[1].Visibility != "context" || detail.Ancestors[2].Visibility != "context" {
		t.Fatal(detail)
	}
	raw, _ := json.Marshal(detail)
	if strings.Contains(string(raw), "不应给上下文公开") || strings.Contains(string(raw), f.actor.ID) {
		t.Fatal("private body/actor leaked", string(raw))
	}
	_, e = f.srv.Detail(ctx, b.actor, scope, target(facility, res.Point, point.ID))
	code(t, e, "resource_not_found")
	pages, e := f.srv.List(ctx, b.actor, scope, target(facility, res.Component, ""), res.Filter{})
	must(t, e)
	if len(pages.Items) != 1 || pages.Items[0].Visibility != "context" {
		t.Fatal(pages)
	}
	channels := ct
	channels.ID = ""
	page, e := f.srv.List(ctx, b.actor, scope, channels, res.Filter{})
	must(t, e)
	if len(page.Items) != 1 {
		t.Fatal(page)
	}
	history, e := f.srv.Changes(ctx, b.actor, scope, target(facility, res.Facility, facility.ID), res.Filter{})
	must(t, e)
	if len(history.Items) != 2 {
		t.Fatal("history not filtered", history)
	}
	raw, _ = json.Marshal(history)
	if strings.Contains(string(raw), "不应给上下文公开") || strings.Contains(string(raw), f.actor.ID) {
		t.Fatal("history leaked unrelated identity", string(raw))
	}
	// No tenant resource role is needed for project work, and tenant entry remains unavailable.
	_, e = f.srv.List(ctx, a.actor, res.TenantScope(f.tenant), res.Target{Kind: res.Facility}, res.Filter{})
	code(t, e, "permission_denied")
	added, e := f.projectWrite(t, a, channels, "create", map[string]any{"code": "NEW", "name": "A 新通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	if added.LinkID == "" || added.AccessRevision == "" {
		t.Fatal(added)
	}
	page, e = f.srv.List(ctx, b.actor, scope, channels, res.Filter{})
	must(t, e)
	if len(page.Items) != 1 {
		t.Fatal("future resource auto shared", page)
	}
	created := ct
	created.ID = added.ID
	_, e = f.srv.Detail(ctx, a.actor, res.ProjectScope(f.tenant, a.id), created)
	must(t, e)
	_, e = f.srv.Detail(ctx, b.actor, scope, created)
	code(t, e, "resource_not_found")
	// Revoke/relink uses new identity and removes old full history.
	before, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, b.id, facility.ID, res.AccessFilter{})
	must(t, e)
	in = f.accessInput(t, b, facility.ID, "revoke_link")
	f.grant(t, b, facility.ID, in)
	_, e = f.srv.Detail(ctx, b.actor, scope, ct)
	code(t, e, "resource_not_found")
	in = f.accessInput(t, b, facility.ID, "create_link")
	in.Add = []res.GrantInput{root}
	again := f.grant(t, b, facility.ID, in)
	if again.LinkID == before.Link.ID {
		t.Fatal("revoked link revived")
	}
}
func TestResourceAccessPreviewTracksCurrentSelection(t *testing.T) {
	f := resourcesSetup(t)
	a := f.project(t, "ProjectA")
	facility := f.facility(t, "F")
	in := f.accessInput(t, a, facility.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	in.SelectCurrent = &res.CurrentSelection{RootKind: res.Facility, RootID: facility.ID, Kinds: map[res.Kind][]string{res.Point: {"read"}}}
	preview, e := f.srv.PreviewAccessChange(ctx, f.actor, f.tenant, a.id, facility.ID, in)
	must(t, e)
	in.PreviewHash = preview.Hash
	_, e = f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "NEW", "name": "并发新增"})
	must(t, e)
	_, e = f.srv.ChangeAccess(ctx, f.actor, f.tenant, a.id, facility.ID, in, access.Meta{})
	code(t, e, "preview_changed")
	out := f.grant(t, a, facility.ID, in)
	if out.AccessRevision != "2" {
		t.Fatal(out)
	}
	// The default facility-create policy is off; opening and closing retains original grants.
	fields := map[string]any{"code": "FROMPROJECT", "name": "项目设施", "type_version_id": f.typ, "values": map[string]any{"length": 0}}
	_, e = f.projectWrite(t, a, res.Target{Kind: res.Facility}, "create", fields)
	code(t, e, "permission_denied")
	policy := f.accessInput(t, a, "", "policy")
	policy.AllowFacilityCreate = ptr(true)
	f.grant(t, a, "", policy)
	created, e := f.projectWrite(t, a, res.Target{Kind: res.Facility}, "create", fields)
	must(t, e)
	policy = f.accessInput(t, a, "", "policy")
	policy.AllowFacilityCreate = ptr(false)
	f.grant(t, a, "", policy)
	d, e := f.srv.Detail(ctx, a.actor, res.ProjectScope(f.tenant, a.id), target(created, res.Facility, created.ID))
	must(t, e)
	if ptrValueForTest(d.CreatedProjectID) != a.id || !d.Actions.CreatePoint {
		t.Fatal(d)
	}
}
func ptrValueForTest(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
