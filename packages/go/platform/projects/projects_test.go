package projects_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/jackc/pgx/v5"
)

var ctx = context.Background()

const password = "Project river 桥梁 2026!"

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func code(t *testing.T, e error, want string) {
	t.Helper()
	if e == nil || identity.PublicError(e).Code != want {
		t.Fatalf("got %v (%s), want %s", e, identity.PublicError(e).Code, want)
	}
}
func token(link string) string {
	u, _ := url.Parse(link)
	v, _ := url.ParseQuery(u.Fragment)
	return v.Get("token")
}

type fixture struct {
	e                       *testsupport.Environment
	s                       *projects.Service
	ts                      *tenancy.Service
	p, a, b, c, d           identity.Principal
	tid, bid, aid, cid, did string
}

func login(t *testing.T, e *testsupport.Environment, name string) identity.Principal {
	t.Helper()
	_, tok, _, err := e.Service.Login(ctx, name, password, identity.Meta{})
	must(t, err)
	p, err := e.Service.Authenticate(ctx, tok)
	must(t, err)
	return p
}
func (f fixture) actor(p identity.Principal) access.Actor { return f.e.Service.AccessActor(p) }
func (f fixture) uuid(t *testing.T) string {
	t.Helper()
	var id string
	must(t, f.e.Pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id))
	return id
}
func (f fixture) wm(t *testing.T) projects.WriteMeta {
	return projects.WriteMeta{RequestID: f.uuid(t), Reason: "项目验收"}
}
func (f fixture) join(t *testing.T, tenant string, admin, user identity.Principal) string {
	t.Helper()
	link, e := f.ts.CreateInvitation(ctx, f.actor(admin), tenant, tenancy.InvitationCreateInput{TargetID: user.Account.ID, RequestID: f.uuid(t), Reason: "项目测试加入"}, access.Meta{})
	must(t, e)
	v, e := f.ts.AcceptInvitation(ctx, f.actor(user), tenancy.AcceptanceInput{Token: token(link.Link), Version: link.Invitation.Version}, access.Meta{})
	must(t, e)
	return v.MembershipID
}
func setup(t *testing.T) fixture {
	t.Helper()
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:identity")
	}
	env, e := testsupport.New(ctx, slog.Default())
	must(t, e)
	t.Cleanup(env.Close)
	_, e = env.Service.Bootstrap(ctx, "Platform", "平台", password, identity.Meta{})
	must(t, e)
	f := fixture{e: env, s: projects.New(env.Service.Access()), ts: tenancy.New(env.Service.Access())}
	f.p = login(t, env, "Platform")
	for _, u := range []string{"Alice", "Bob", "Carol", "David"} {
		l, e := env.Service.CreateInvitation(ctx, f.p, "", "", identity.Meta{})
		must(t, e)
		must(t, env.Service.AcceptInvitation(ctx, token(l.URL), u, u, password, identity.Meta{}))
	}
	f.a = login(t, env, "Alice")
	f.b = login(t, env, "Bob")
	f.c = login(t, env, "Carol")
	f.d = login(t, env, "David")
	v, e := f.ts.Create(ctx, f.actor(f.p), tenancy.CreateInput{Fields: tenancy.Fields{Name: "项目租户 A", Code: "tenant-a"}, AdministratorID: f.a.Account.ID, RequestID: f.uuid(t), Reason: "项目测试"}, access.Meta{})
	must(t, e)
	f.tid = v.TenantID
	tc, e := f.ts.Context(ctx, f.actor(f.a), f.tid)
	must(t, e)
	f.aid = tc.MembershipID
	f.bid = f.join(t, f.tid, f.a, f.b)
	f.cid = f.join(t, f.tid, f.a, f.c)
	f.did = f.join(t, f.tid, f.a, f.d)
	return f
}
func (f fixture) input(t *testing.T, code, admin string) projects.CreateInput {
	return projects.CreateInput{Fields: projects.Fields{Name: "项目 " + code, Code: code, Purpose: "道路结构监测", Description: "纯文本 <b>不执行</b>"}, WriteMeta: f.wm(t), Owner: f.cid, Administrator: admin}
}
func (f fixture) create(t *testing.T, code, admin string) projects.Result {
	t.Helper()
	v, e := f.s.Create(ctx, f.actor(f.a), f.tid, f.input(t, code, admin), access.Meta{})
	must(t, e)
	return v
}
func (f fixture) prep(t *testing.T, who identity.Principal, pid, target string, op access.Operation, in projects.Change) projects.Change {
	t.Helper()
	p, e := f.s.Preview(ctx, f.actor(who), f.tid, pid, target, op, in)
	must(t, e)
	if !p.CanSubmit {
		t.Fatal("blocked preview", p.BlockedReason)
	}
	in.ChangeMeta = projects.ChangeMeta{WriteMeta: f.wm(t), Version: p.Version, ProjectVersion: p.ProjectVersion, Hash: p.Hash}
	if op == access.OpProjectMemberRoles {
		in.RoleVersions = p.RoleVersions
	}
	return in
}
func (f fixture) change(t *testing.T, who identity.Principal, pid, target string, op access.Operation, in projects.Change) projects.Result {
	t.Helper()
	in = f.prep(t, who, pid, target, op, in)
	v, e := f.s.Change(ctx, f.actor(who), f.tid, pid, target, op, in, access.Meta{})
	must(t, e)
	return v
}
func (f fixture) add(t *testing.T, who identity.Principal, pid, tm string) projects.Result {
	t.Helper()
	p, e := f.s.Context(ctx, f.actor(who), f.tid, pid)
	must(t, e)
	v, e := f.s.AddMember(ctx, f.actor(who), f.tid, pid, projects.AddMemberInput{WriteMeta: f.wm(t), ProjectVersion: p.Project.Version, Candidate: tm}, access.Meta{})
	must(t, e)
	return v
}
func (f fixture) role(t *testing.T, pid, name string, codes []access.Code) projects.Result {
	t.Helper()
	p, e := f.s.Context(ctx, f.actor(f.b), f.tid, pid)
	must(t, e)
	v, e := f.s.CreateRole(ctx, f.actor(f.b), f.tid, pid, "", projects.RoleInput{ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), ProjectVersion: p.Project.Version}, Name: name, Permissions: codes}, access.Meta{})
	must(t, e)
	return v
}
func (f fixture) preset(t *testing.T, pid, key string) access.Role {
	t.Helper()
	list, e := f.s.Roles(ctx, f.actor(f.b), f.tid, pid, projects.Filter{Limit: 100}, false)
	must(t, e)
	for _, r := range list.Items {
		if r.BuiltinKey == key {
			return r
		}
	}
	t.Fatal(key)
	return access.Role{}
}
func count(t *testing.T, f fixture, query string, args ...any) int {
	t.Helper()
	var n int
	must(t, f.e.Pool.QueryRow(ctx, query, args...).Scan(&n))
	return n
}
func TestProjectCreationIsolationAtomicityAndCodes(t *testing.T) {
	f := setup(t)
	in := f.input(t, "  ALPHA  ", f.bid)
	in.Name = " Ａ项目 "
	p, e := f.s.Create(ctx, f.actor(f.a), f.tid, in, access.Meta{})
	must(t, e)
	replay, e := f.s.Create(ctx, f.actor(f.a), f.tid, in, access.Meta{})
	must(t, e)
	if !replay.Replayed || replay.ProjectID != p.ProjectID {
		t.Fatal(replay)
	}
	in.Purpose = "changed"
	_, e = f.s.Create(ctx, f.actor(f.a), f.tid, in, access.Meta{})
	code(t, e, "request_payload_conflict")
	profile, e := f.s.Profile(ctx, f.actor(f.b), f.tid, p.ProjectID)
	must(t, e)
	if profile.Name != "A项目" || profile.Code != "alpha" || profile.Owner.MembershipID != f.cid {
		t.Fatal(profile)
	}
	for _, who := range []identity.Principal{f.p, f.a, f.c} {
		_, e = f.s.Profile(ctx, f.actor(who), f.tid, p.ProjectID)
		code(t, e, "project_not_found")
	}
	my, e := f.s.List(ctx, f.actor(f.a), "", projects.Filter{}, true)
	must(t, e)
	if len(my.Items) != 0 {
		t.Fatal(my)
	}
	gov, e := f.s.Governance(ctx, f.actor(f.a), f.tid, p.ProjectID)
	must(t, e)
	raw, _ := json.Marshal(gov)
	if strings.Contains(string(raw), "道路结构监测") {
		t.Fatal("governance leaked")
	}
	_, e = f.s.Create(ctx, f.actor(f.a), f.tid, f.input(t, "alpha", f.bid), access.Meta{})
	code(t, e, "project_code_taken")
	change := projects.UpdateInput{Fields: projects.Fields{Name: profile.Name, Code: "beta", Purpose: "新目的"}, ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), Version: profile.Version, ProjectVersion: profile.Version}}
	updated, e := f.s.Update(ctx, f.actor(f.b), f.tid, p.ProjectID, change, access.Meta{})
	must(t, e)
	again, e := f.s.Update(ctx, f.actor(f.b), f.tid, p.ProjectID, change, access.Meta{})
	must(t, e)
	if !again.Replayed {
		t.Fatal(again)
	}
	_, e = f.s.Create(ctx, f.actor(f.a), f.tid, f.input(t, "alpha", f.bid), access.Meta{})
	code(t, e, "project_code_taken")
	change.Code = "alpha"
	change.ChangeMeta = projects.ChangeMeta{WriteMeta: f.wm(t), Version: updated.Version, ProjectVersion: updated.Version}
	_, e = f.s.Update(ctx, f.actor(f.b), f.tid, p.ProjectID, change, access.Meta{})
	must(t, e)
	for _, bad := range []projects.Fields{{Name: "", Code: "valid", Purpose: "ok"}, {Name: "ok", Code: "xx", Purpose: "ok"}, {Name: "ok", Code: "valid", Purpose: ""}, {Name: "ok", Code: "valid", Purpose: "ok", PlannedStart: "2026-11-11", PlannedEnd: "2026-11-10"}} {
		in := f.input(t, "invalid", f.bid)
		in.Fields = bad
		_, e = f.s.Create(ctx, f.actor(f.a), f.tid, in, access.Meta{})
		code(t, e, "validation_failed")
	}
	before := count(t, f, `SELECT count(*) FROM projects`)
	_, e = f.e.Pool.Exec(ctx, `CREATE FUNCTION fail_project_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.project_id IS NOT NULL THEN RAISE EXCEPTION 'audit test'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_project_audit BEFORE INSERT ON account_security_events FOR EACH ROW EXECUTE FUNCTION fail_project_audit()`)
	must(t, e)
	in = f.input(t, "rollback", f.bid)
	_, e = f.s.Create(ctx, f.actor(f.a), f.tid, in, access.Meta{})
	if e == nil {
		t.Fatal("expected rollback")
	}
	if count(t, f, `SELECT count(*) FROM projects`) != before || count(t, f, `SELECT count(*) FROM project_codes WHERE code='rollback'`) != 0 {
		t.Fatal("partial create")
	}
	_, e = f.e.Pool.Exec(ctx, `DROP TRIGGER fail_project_audit ON account_security_events`)
	must(t, e)
	_, e = f.s.Create(ctx, f.actor(f.a), f.tid, in, access.Meta{})
	must(t, e)
	// Another tenant may use the same code; scope probes never reveal the first tenant.
	tv, e := f.ts.Create(ctx, f.actor(f.p), tenancy.CreateInput{Fields: tenancy.Fields{Name: "另一个租户", Code: "tenant-b"}, AdministratorID: f.a.Account.ID, RequestID: f.uuid(t), Reason: "隔离"}, access.Meta{})
	must(t, e)
	tc, e := f.ts.Context(ctx, f.actor(f.a), tv.TenantID)
	must(t, e)
	ci := f.input(t, "alpha", tc.MembershipID)
	ci.Owner = tc.MembershipID
	_, e = f.s.Create(ctx, f.actor(f.a), tv.TenantID, ci, access.Meta{})
	must(t, e)
	_, e = f.s.Profile(ctx, f.actor(f.b), tv.TenantID, p.ProjectID)
	code(t, e, "project_not_found")
}
func TestProjectMembersRolesUnionHistoryAndParentCascade(t *testing.T) {
	f := setup(t)
	p := f.create(t, "roles", f.bid)
	member := f.add(t, f.b, p.ProjectID, f.cid)
	_, e := f.s.AddMember(ctx, f.actor(f.b), f.tid, p.ProjectID, projects.AddMemberInput{WriteMeta: f.wm(t), ProjectVersion: "1", Candidate: f.cid}, access.Meta{})
	code(t, e, "membership_exists")
	r1 := f.role(t, p.ProjectID, "查看一", []access.Code{access.ProjectProfileRead})
	r2 := f.role(t, p.ProjectID, "查看二", []access.Code{access.ProjectProfileRead, access.ProjectAuditRead})
	f.change(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{r1.ObjectID, r2.ObjectID}})
	auth, e := f.s.Authorization(ctx, f.actor(f.b), f.tid, p.ProjectID, member.ObjectID)
	must(t, e)
	if len(auth.Sources[access.ProjectProfileRead]) != 2 {
		t.Fatal(auth)
	}
	preview, e := f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, r1.ObjectID, access.OpProjectRoleStatus, projects.Change{Status: "disabled"})
	must(t, e)
	if preview.LosingMembers != 0 || len(preview.Members) != 1 {
		t.Fatal(preview)
	}
	f.change(t, f.b, p.ProjectID, r1.ObjectID, access.OpProjectRoleStatus, projects.Change{Status: "disabled"})
	_, e = f.s.Profile(ctx, f.actor(f.c), f.tid, p.ProjectID)
	must(t, e)
	// A disabled existing binding can stay while another is revoked.
	f.change(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{r1.ObjectID}})
	_, e = f.s.Profile(ctx, f.actor(f.c), f.tid, p.ProjectID)
	code(t, e, "permission_denied")
	f.change(t, f.b, p.ProjectID, r1.ObjectID, access.OpProjectRoleStatus, projects.Change{Status: "active"})
	_, e = f.s.Profile(ctx, f.actor(f.c), f.tid, p.ProjectID)
	must(t, e)
	f.change(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberSuspend, projects.Change{})
	_, e = f.s.Profile(ctx, f.actor(f.c), f.tid, p.ProjectID)
	code(t, e, "permission_denied")
	f.change(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{}})
	f.change(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberResume, projects.Change{})
	context, e := f.s.Context(ctx, f.actor(f.c), f.tid, p.ProjectID)
	must(t, e)
	if context.CanEnter || len(context.Authorization.Roles) != 0 {
		t.Fatal(context)
	}
	f.change(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberRemove, projects.Change{})
	rejoined := f.add(t, f.b, p.ProjectID, f.cid)
	if rejoined.ObjectID == member.ObjectID {
		t.Fatal("reused ended member")
	}
	auth, e = f.s.Authorization(ctx, f.actor(f.b), f.tid, p.ProjectID, rejoined.ObjectID)
	must(t, e)
	if len(auth.Roles) != 1 || auth.Roles[0].BuiltinKey != access.ProjectMemberKey {
		t.Fatal(auth)
	}
	tm, e := f.ts.Member(ctx, f.actor(f.a), f.tid, f.cid)
	must(t, e)
	_, e = f.ts.ChangeMember(ctx, f.actor(f.a), f.tid, f.cid, access.OpTenantMemberRemove, tenancy.MemberInput{Version: tm.Version, Reason: "结束上层关系"}, access.Meta{})
	must(t, e)
	_, e = f.s.Context(ctx, f.actor(f.c), f.tid, p.ProjectID)
	code(t, e, "project_not_found")
	if count(t, f, `SELECT count(*) FROM project_member_roles WHERE membership_id=$1 AND revoked_at IS NULL`, rejoined.ObjectID) != 0 {
		t.Fatal("orphan bindings")
	}
	newParent := f.join(t, f.tid, f.a, f.c)
	if newParent == f.cid {
		t.Fatal("old parent reused")
	}
	my, e := f.s.List(ctx, f.actor(f.c), "", projects.Filter{}, true)
	must(t, e)
	if len(my.Items) != 0 {
		t.Fatal("old project revived")
	}
}
func TestProjectArchiveRevocationTransferAndRecovery(t *testing.T) {
	f := setup(t)
	p := f.create(t, "archive", f.bid)
	member := f.add(t, f.b, p.ProjectID, f.cid)
	r := f.role(t, p.ProjectID, "资料维护", []access.Code{access.ProjectProfileRead, access.ProjectProfileUpdate})
	f.change(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{r.ObjectID}})
	in := f.prep(t, f.b, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
	ar, e := f.s.Change(ctx, f.actor(f.b), f.tid, p.ProjectID, "", access.OpProjectArchive, in, access.Meta{})
	must(t, e)
	replay, e := f.s.Change(ctx, f.actor(f.b), f.tid, p.ProjectID, "", access.OpProjectArchive, in, access.Meta{})
	must(t, e)
	if !replay.Replayed {
		t.Fatal(replay)
	}
	_, e = f.s.AddMember(ctx, f.actor(f.b), f.tid, p.ProjectID, projects.AddMemberInput{WriteMeta: f.wm(t), ProjectVersion: ar.ProjectVersion, Candidate: f.did}, access.Meta{})
	code(t, e, "project_archived")
	profile, e := f.s.Profile(ctx, f.actor(f.c), f.tid, p.ProjectID)
	must(t, e)
	if profile.Status != "archived" {
		t.Fatal(profile)
	}
	in = f.prep(t, f.b, p.ProjectID, r.ObjectID, access.OpProjectRolePermissions, projects.Change{Permissions: []access.Code{access.ProjectProfileRead}})
	_, e = f.s.Change(ctx, f.actor(f.b), f.tid, p.ProjectID, r.ObjectID, access.OpProjectRolePermissions, in, access.Meta{})
	must(t, e)
	pr, e := f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, r.ObjectID, access.OpProjectRolePermissions, projects.Change{Permissions: []access.Code{access.ProjectProfileRead, access.ProjectProfileUpdate}})
	must(t, e)
	if pr.CanSubmit {
		t.Fatal("archived grant")
	}
	f.change(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberRemove, projects.Change{})
	f.change(t, f.b, p.ProjectID, "", access.OpProjectTransfer, projects.Change{Candidate: f.did})
	_, e = f.s.Context(ctx, f.actor(f.b), f.tid, p.ProjectID)
	code(t, e, "project_not_found")
	_, e = f.ts.Context(ctx, f.actor(f.b), f.tid)
	must(t, e)
	f.change(t, f.d, p.ProjectID, "", access.OpProjectRestore, projects.Change{})
	_, e = f.s.Context(ctx, f.actor(f.c), f.tid, p.ProjectID)
	code(t, e, "project_not_found")
	_, e = f.s.Context(ctx, f.actor(f.b), f.tid, p.ProjectID)
	code(t, e, "project_not_found")
	account, e := f.e.Service.Access().Account(ctx, f.actor(f.p), f.d.Account.ID)
	must(t, e)
	_, e = f.e.Service.SetAccountStatus(ctx, f.p, f.d.Account.ID, account.Version, false, access.Meta{Reason: "常规停用"})
	code(t, e, "project_admin_transfer_required")
	must(t, f.e.Service.SetStatus(ctx, "David", false, true, identity.Meta{Reason: "应急停用"}))
	pr, e = f.s.Preview(ctx, f.actor(f.a), f.tid, p.ProjectID, "", access.OpProjectRecoverAdmin, projects.Change{Candidate: f.bid})
	must(t, e)
	if !pr.CanSubmit {
		t.Fatal(pr)
	}
	f.change(t, f.a, p.ProjectID, "", access.OpProjectRecoverAdmin, projects.Change{Candidate: f.bid})
	_, e = f.s.Profile(ctx, f.actor(f.a), f.tid, p.ProjectID)
	code(t, e, "project_not_found")
	pr, e = f.s.Preview(ctx, f.actor(f.a), f.tid, p.ProjectID, "", access.OpProjectRecoverAdmin, projects.Change{Candidate: f.cid})
	must(t, e)
	if pr.CanSubmit {
		t.Fatal("takeover available admin")
	}
}
func TestProjectConcurrentCreationPreviewAndConstraints(t *testing.T) {
	f := setup(t)
	in := f.input(t, "concurrent", f.bid)
	var wg sync.WaitGroup
	results := make(chan projects.Result, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() { r, e := f.s.Create(ctx, f.actor(f.a), f.tid, in, access.Meta{}); results <- r; errs <- e })
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		must(t, e)
	}
	pid := ""
	for r := range results {
		if pid != "" && pid != r.ProjectID {
			t.Fatal("duplicate create")
		}
		pid = r.ProjectID
	}
	if count(t, f, `SELECT count(*) FROM projects`) != 1 {
		t.Fatal("duplicate")
	}
	pending := f.prep(t, f.b, pid, "", access.OpProjectArchive, projects.Change{})
	f.add(t, f.b, pid, f.cid)
	_, e := f.s.Change(ctx, f.actor(f.b), f.tid, pid, "", access.OpProjectArchive, pending, access.Meta{})
	code(t, e, "preview_stale")
	p, e := f.s.Profile(ctx, f.actor(f.b), f.tid, pid)
	must(t, e)
	update := projects.UpdateInput{Fields: projects.Fields{Name: "并发资料", Code: p.Code, Purpose: p.Purpose}, ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), Version: p.Version, ProjectVersion: p.Version}}
	errs = make(chan error, 2)
	for range 2 {
		copy := update
		copy.RequestID = f.uuid(t)
		wg.Go(func() { _, e := f.s.Update(ctx, f.actor(f.b), f.tid, pid, copy, access.Meta{}); errs <- e })
	}
	wg.Wait()
	close(errs)
	success, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if identity.PublicError(e).Code == "version_conflict" {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal(success, conflicts)
	}
	role := f.role(t, pid, "受约束角色", []access.Code{})
	_, e = f.e.Pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,'project.archive','project')`, role.ObjectID)
	if e == nil {
		t.Fatal("reserved permission SQL accepted")
	}
	_, e = f.e.Pool.Exec(ctx, `UPDATE projects SET administrator_membership_id=NULL WHERE id=$1`, pid)
	if e == nil {
		t.Fatal("unrecorded gap")
	}
	_, e = f.e.Pool.Exec(ctx, `UPDATE project_memberships SET status='suspended' WHERE id=$1`, p.AdministratorMembershipID)
	if e == nil {
		t.Fatal("suspended administrator")
	}
	sentinel := errors.New("revoked actor")
	a := f.actor(f.b)
	a.Check = func(context.Context, pgx.Tx, bool) error { return sentinel }
	_, e = f.s.Profile(ctx, a, f.tid, pid)
	if !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
}
func TestProjectAuditIsolationAndTenantRoleIndependence(t *testing.T) {
	f := setup(t)
	p := f.create(t, "audit", f.bid)
	member := f.add(t, f.b, p.ProjectID, f.cid)
	events, e := f.s.Audit(ctx, f.actor(f.b), f.tid, p.ProjectID, projects.Filter{})
	must(t, e)
	if len(events.Items) < 2 {
		t.Fatal(events)
	}
	platform, e := f.e.Service.Access().AuditEvents(ctx, f.actor(f.p), access.Filter{Limit: 100})
	must(t, e)
	for _, v := range platform.Items {
		if v.ProjectID != "" || strings.HasPrefix(v.EventType, "project.") || v.EventType == "tenant.project.governance" {
			t.Fatal("project leak", v)
		}
	}
	tenant, e := f.ts.Audit(ctx, f.actor(f.a), f.tid, tenancy.Filter{})
	must(t, e)
	for _, v := range tenant.Items {
		if strings.HasPrefix(v.Event, "project.") {
			t.Fatal(v)
		}
	}
	// Empty tenant roles preserve the parent relation and independent project rights.
	ta, e := f.e.Service.Access().TenantMemberAuthorization(ctx, f.actor(f.a), f.tid, f.bid)
	must(t, e)
	ti := access.TenantChange{ExpectedMembershipVersion: ta.MembershipVersion, RoleIDs: []string{}, Reason: "保留关系、清空租户角色"}
	tp, e := f.e.Service.Access().PreviewTenantChange(ctx, f.actor(f.a), f.tid, f.bid, access.OpTenantMemberRolesPreview, ti, access.Filter{})
	must(t, e)
	ti.PreviewHash = tp.Hash
	ti.RoleVersions = tp.RoleVersions
	_, e = f.e.Service.Access().ChangeTenantAuthorization(ctx, f.actor(f.a), f.tid, f.bid, access.OpTenantMemberRoles, ti, access.Meta{})
	must(t, e)
	_, e = f.s.Profile(ctx, f.actor(f.b), f.tid, p.ProjectID)
	must(t, e)
	_, e = f.ts.Get(ctx, f.actor(f.b), f.tid, false)
	code(t, e, "permission_denied")
	admin := f.preset(t, p.ProjectID, access.ProjectAdminKey)
	pr, e := f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, member.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{admin.ID}})
	code(t, e, "permission_denied")
	_ = pr
	// Owner changes never add a member and self/member protection remains independent.
	f.change(t, f.b, p.ProjectID, "", access.OpProjectOwnerChange, projects.Change{Candidate: f.did})
	_, e = f.s.Context(ctx, f.actor(f.d), f.tid, p.ProjectID)
	code(t, e, "project_not_found")
	list, e := f.s.Permissions(ctx, f.actor(f.b), f.tid, p.ProjectID)
	must(t, e)
	ordinary := 0
	for _, pc := range list {
		if pc.ProjectCustomRoleAssignable {
			ordinary++
		}
	}
	if len(list) != 21 || ordinary != 11 {
		t.Fatal(list)
	}
	auth, e := f.s.Authorization(ctx, f.actor(f.b), f.tid, p.ProjectID, member.ObjectID)
	must(t, e)
	if !slices.Contains(auth.Permissions, access.ProjectProfileRead) {
		t.Fatal(auth)
	}
}
