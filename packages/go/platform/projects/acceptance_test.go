package projects_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
)

func TestProjectCandidateHandoverAndIndependentPresetCopy(t *testing.T) {
	f := setup(t)
	p := f.create(t, "acceptance-edges", f.bid)
	m := f.add(t, f.b, p.ProjectID, f.cid)
	preset := f.preset(t, p.ProjectID, access.ProjectMemberKey)
	copied, e := f.s.CreateRole(ctx, f.actor(f.b), f.tid, p.ProjectID, preset.ID, projects.RoleInput{ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), ProjectVersion: "1"}, Name: "普通角色独立副本", SourceVersion: preset.Version, Permissions: append([]access.Code{}, preset.Permissions...)}, access.Meta{})
	must(t, e)
	f.change(t, f.b, p.ProjectID, m.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{preset.ID, copied.ObjectID}})
	f.change(t, f.b, p.ProjectID, m.ObjectID, access.OpProjectMemberSuspend, projects.Change{})
	_, e = f.s.AddMember(ctx, f.actor(f.b), f.tid, p.ProjectID, projects.AddMemberInput{WriteMeta: f.wm(t), ProjectVersion: "1", Candidate: f.cid}, access.Meta{})
	code(t, e, "membership_exists")
	for _, candidate := range []string{f.bid, f.cid} {
		preview, err := f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, "", access.OpProjectTransfer, projects.Change{Candidate: candidate})
		blocked(t, preview, err)
	}
	admin := f.preset(t, p.ProjectID, access.ProjectAdminKey)
	old, e := f.s.Profile(ctx, f.actor(f.b), f.tid, p.ProjectID)
	must(t, e)
	for _, op := range []access.Operation{access.OpProjectMemberSuspend, access.OpProjectMemberRemove, access.OpProjectMemberRoles} {
		preview, err := f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, old.AdministratorMembershipID, op, projects.Change{Roles: []string{}})
		blocked(t, preview, err)
	}
	for _, candidate := range []string{f.uuid(t), f.p.Account.ID} {
		_, e = f.s.AddMember(ctx, f.actor(f.b), f.tid, p.ProjectID, projects.AddMemberInput{WriteMeta: f.wm(t), ProjectVersion: "1", Candidate: candidate}, access.Meta{})
		if e == nil {
			t.Fatal("unrelated candidate admitted")
		}
	}
	f.change(t, f.b, p.ProjectID, m.ObjectID, access.OpProjectMemberResume, projects.Change{})
	f.change(t, f.b, p.ProjectID, "", access.OpProjectTransfer, projects.Change{Candidate: f.cid})
	current, e := f.s.Profile(ctx, f.actor(f.c), f.tid, p.ProjectID)
	must(t, e)
	if current.AdministratorMembershipID != m.ObjectID {
		t.Fatal("existing membership not reused")
	}
	auth, e := f.s.Authorization(ctx, f.actor(f.c), f.tid, p.ProjectID, m.ObjectID)
	must(t, e)
	if len(auth.Roles) != 1 || auth.Roles[0].ID != admin.ID {
		t.Fatal("successor roles not replaced")
	}
	if count(t, f, `SELECT count(*) FROM project_member_roles WHERE membership_id=$1 AND revoked_at IS NOT NULL`, m.ObjectID) != 2 {
		t.Fatal("prior role history lost")
	}
	_, e = f.s.Context(ctx, f.actor(f.b), f.tid, p.ProjectID)
	code(t, e, "project_not_found")
	governance, e := f.s.List(ctx, f.actor(f.a), f.tid, projects.Filter{}, false)
	must(t, e)
	if len(governance.Items) != 1 || governance.Items[0].Authorization.Roles == nil || governance.Items[0].Authorization.Permissions == nil || governance.Items[0].Authorization.Sources == nil {
		t.Fatal("governance empty authorization violates the collection contract")
	}
	// A past planned date is only business information, never a lifecycle trigger.
	_, e = f.s.Update(ctx, f.actor(f.c), f.tid, p.ProjectID, projects.UpdateInput{Fields: projects.Fields{Name: current.Name, Code: current.Code, Purpose: current.Purpose, PlannedEnd: "2000-01-01"}, ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), Version: current.Version, ProjectVersion: current.Version}}, access.Meta{})
	must(t, e)
	current, e = f.s.Profile(ctx, f.actor(f.c), f.tid, p.ProjectID)
	must(t, e)
	if current.Status != "active" {
		t.Fatal("planned date changed lifecycle")
	}
	for _, field := range []string{"name", "purpose", "description"} {
		in := f.input(t, "long-field", f.bid)
		switch field {
		case "name":
			in.Name = strings.Repeat("长", 101)
		case "purpose":
			in.Purpose = strings.Repeat("长", 1001)
		case "description":
			in.Description = strings.Repeat("长", 2001)
		}
		_, e = f.s.Create(ctx, f.actor(f.a), f.tid, in, access.Meta{})
		code(t, e, "validation_failed")
	}
}

func TestProjectDistinctRequestsCompeteForSameCode(t *testing.T) {
	f := setup(t)
	inputs := []projects.CreateInput{f.input(t, "same-code", f.bid), f.input(t, "same-code", f.bid)}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, in := range inputs {
		wg.Go(func() { _, e := f.s.Create(ctx, f.actor(f.a), f.tid, in, access.Meta{}); errs <- e })
	}
	wg.Wait()
	close(errs)
	successes := 0
	for e := range errs {
		if e == nil {
			successes++
		} else {
			code(t, e, "project_code_taken")
		}
	}
	if successes != 1 || count(t, f, `SELECT count(*) FROM projects`) != 1 || count(t, f, `SELECT count(*) FROM project_memberships`) != 1 || count(t, f, `SELECT count(*) FROM project_operation_receipts`) != 1 {
		t.Fatal("partial conflicting creation")
	}
}
