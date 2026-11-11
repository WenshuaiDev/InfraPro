package projects_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/jackc/pgx/v5"
)

func (f fixture) lifecycle(t *testing.T, state string) {
	t.Helper()
	p, e := f.ts.PreviewStatus(ctx, f.actor(f.p), f.tid, state)
	must(t, e)
	in := tenancy.StatusInput{Version: p.Version, Hash: p.Hash, Reason: "项目上层状态验收"}
	if state == "disabled" {
		notice := "验收停用"
		in.Notice = &notice
	}
	_, e = f.ts.ChangeStatus(ctx, f.actor(f.p), f.tid, state, in, access.Meta{})
	must(t, e)
}
func (f fixture) parentState(t *testing.T, mid string, op access.Operation) {
	t.Helper()
	m, e := f.ts.Member(ctx, f.actor(f.a), f.tid, mid)
	must(t, e)
	_, e = f.ts.ChangeMember(ctx, f.actor(f.a), f.tid, mid, op, tenancy.MemberInput{Version: m.Version, Reason: "项目上层关系验收"}, access.Meta{})
	must(t, e)
}
func blocked(t *testing.T, p projects.Preview, e error) {
	t.Helper()
	if e == nil && p.CanSubmit {
		t.Fatal("unexpected allowed preview")
	}
}

func TestProjectControlledTenantRepairAndRecoveryChain(t *testing.T) {
	f := setup(t)
	p := f.create(t, "gap-chain", f.aid)
	q := f.create(t, "unaffected", f.bid)
	ordinary := f.add(t, f.b, q.ProjectID, f.aid)
	f.add(t, f.a, p.ProjectID, f.cid)
	f.change(t, f.a, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
	old, e := f.ts.Get(ctx, f.actor(f.p), f.tid, true)
	must(t, e)
	transfer := tenancy.TransferInput{Version: old.Version, AdministratorID: f.d.Account.ID, Reason: "修复租户管理"}
	_, e = f.ts.Transfer(ctx, f.actor(f.p), f.tid, transfer, access.Meta{})
	code(t, e, "project_admin_transfer_required")
	// Both an active and an archived project protect their administrator through upper operations.
	tm, e := f.ts.Member(ctx, f.actor(f.a), f.tid, f.bid)
	must(t, e)
	for _, op := range []access.Operation{access.OpTenantMemberSuspend, access.OpTenantMemberRemove} {
		_, e = f.ts.ChangeMember(ctx, f.actor(f.a), f.tid, f.bid, op, tenancy.MemberInput{Version: tm.Version, Reason: "常规保护"}, access.Meta{})
		code(t, e, "project_admin_transfer_required")
	}
	f.lifecycle(t, "disabled")
	must(t, f.e.Service.SetStatus(ctx, "Alice", false, true, identity.Meta{Reason: "应急禁用不可用租户管理员"}))
	old, e = f.ts.Get(ctx, f.actor(f.p), f.tid, true)
	must(t, e)
	transfer.Version = old.Version
	_, e = f.ts.Transfer(ctx, f.actor(f.p), f.tid, transfer, access.Meta{})
	code(t, e, "preview_stale")
	preview, e := f.ts.PreviewTransfer(ctx, f.actor(f.p), f.tid)
	must(t, e)
	if preview["requires_confirmation"] != true {
		t.Fatal(preview)
	}
	impacts := preview["projects"].([]access.ProjectManagementImpact)
	if len(impacts) != 2 {
		t.Fatal(impacts)
	}
	transfer.ConfirmProjectImpact = true
	transfer.ProjectPreviewHash = "obsolete"
	_, e = f.ts.Transfer(ctx, f.actor(f.p), f.tid, transfer, access.Meta{})
	code(t, e, "preview_stale")
	transfer.ProjectPreviewHash = preview["project_preview_hash"].(string)
	_, e = f.ts.Transfer(ctx, f.actor(f.p), f.tid, transfer, access.Meta{})
	must(t, e)
	if count(t, f, `SELECT count(*) FROM projects WHERE id=$1 AND administrator_membership_id IS NULL AND management_gap_reason IS NOT NULL`, p.ProjectID) != 1 {
		t.Fatal("gap missing")
	}
	if count(t, f, `SELECT count(*) FROM project_memberships WHERE tenant_membership_id=$1 AND status<>'ended'`, f.aid) != 0 || count(t, f, `SELECT count(*) FROM project_member_roles WHERE membership_id=$1 AND revoked_at IS NULL`, ordinary.ObjectID) != 0 {
		t.Fatal("old relations retained")
	}
	_, e = f.s.Governance(ctx, f.actor(f.d), f.tid, p.ProjectID)
	code(t, e, "tenant_disabled")
	f.lifecycle(t, "active")
	g, e := f.s.Governance(ctx, f.actor(f.d), f.tid, p.ProjectID)
	must(t, e)
	if !g.ManagementGap || g.Status != "archived" {
		t.Fatal(g)
	}
	_, e = f.s.Profile(ctx, f.actor(f.c), f.tid, p.ProjectID)
	must(t, e)
	_, e = f.s.Profile(ctx, f.actor(f.d), f.tid, p.ProjectID)
	code(t, e, "project_not_found")
	// Repair assigns only the chosen successor and does not grant the governance operator project access.
	f.change(t, f.d, p.ProjectID, "", access.OpProjectRecoverAdmin, projects.Change{Candidate: f.bid})
	f.change(t, f.b, p.ProjectID, "", access.OpProjectRestore, projects.Change{})
	_, e = f.s.Profile(ctx, f.actor(f.d), f.tid, p.ProjectID)
	code(t, e, "project_not_found")
	other, e := f.s.Profile(ctx, f.actor(f.b), f.tid, q.ProjectID)
	must(t, e)
	if other.Status != "active" {
		t.Fatal(other)
	}
	must(t, f.e.Service.SetStatus(ctx, "Alice", true, false, identity.Meta{Reason: "恢复账号"}))
	_, e = f.s.List(ctx, f.actor(f.a), "", projects.Filter{}, true)
	code(t, e, "unauthenticated")
	fresh := login(t, f.e, "Alice")
	list, e := f.s.List(ctx, f.actor(fresh), "", projects.Filter{}, true)
	must(t, e)
	if len(list.Items) != 0 {
		t.Fatal("old relation revived")
	}
	if count(t, f, `SELECT count(*) FROM account_security_events WHERE project_id=$1 AND event_type IN('project.administrator.gap','project.member.parent_ended','project.ProjectRecoverAdmin')`, p.ProjectID) != 3 {
		t.Fatal("repair history missing")
	}
	must(t, f.e.Service.Access().Check(ctx))
}

func TestProjectRoleBoundariesCopyAndCompleteImpact(t *testing.T) {
	f := setup(t)
	p := f.create(t, "role-boundaries", f.bid)
	q := f.create(t, "role-other", f.bid)
	carol := f.add(t, f.b, p.ProjectID, f.cid)
	david := f.add(t, f.b, p.ProjectID, f.did)
	r := f.role(t, p.ProjectID, "协作", []access.Code{access.ProjectProfileRead, access.ProjectProfileUpdate})
	backup := f.role(t, p.ProjectID, "只读备份", []access.Code{access.ProjectProfileRead})
	f.change(t, f.b, p.ProjectID, carol.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{r.ObjectID, backup.ObjectID}})
	f.change(t, f.b, p.ProjectID, david.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{r.ObjectID}})
	f.change(t, f.b, p.ProjectID, david.ObjectID, access.OpProjectMemberSuspend, projects.Change{})
	impact, e := f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, r.ObjectID, access.OpProjectRoleStatus, projects.Change{Status: "disabled"})
	must(t, e)
	if impact.MemberCount != 2 || impact.LosingMembers != 1 || len(impact.Members) != 2 {
		t.Fatal(impact)
	}
	for _, m := range impact.Members {
		if m.MembershipID == carol.ObjectID && !slices.Equal(m.Removed, []access.Code{access.ProjectProfileUpdate}) {
			t.Fatal(m)
		}
		if m.MembershipID == david.ObjectID && (len(m.Removed) != 0 || len(m.RestorableRemoved) != 2) {
			t.Fatal(m)
		}
	}
	for i, codes := range [][]access.Code{nil, {access.ProjectArchive}, {access.TenantProfileRead}, {access.ProfileRead}, {"project.retired"}, {access.ProjectProfileRead, access.ProjectProfileRead}} {
		_, e = f.s.CreateRole(ctx, f.actor(f.b), f.tid, p.ProjectID, "", projects.RoleInput{ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), ProjectVersion: "1"}, Name: fmt.Sprintf("非法%d", i), Permissions: codes}, access.Meta{})
		code(t, e, "validation_failed")
	}
	_, e = f.s.CreateRole(ctx, f.actor(f.b), f.tid, p.ProjectID, "", projects.RoleInput{ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), ProjectVersion: "1"}, Name: " 协作 ", Permissions: []access.Code{}}, access.Meta{})
	code(t, e, "role_name_taken")
	foreign := f.role(t, q.ProjectID, "协作", []access.Code{access.ProjectProfileRead})
	copyRole, e := f.s.CreateRole(ctx, f.actor(f.b), f.tid, p.ProjectID, r.ObjectID, projects.RoleInput{ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), ProjectVersion: "1"}, Name: "独立副本", SourceVersion: r.Version, Permissions: []access.Code{access.ProjectProfileRead, access.ProjectProfileUpdate}}, access.Meta{})
	must(t, e)
	f.change(t, f.b, p.ProjectID, r.ObjectID, access.OpProjectRolePermissions, projects.Change{Permissions: []access.Code{access.ProjectProfileRead}})
	copied, e := f.s.Role(ctx, f.actor(f.b), f.tid, p.ProjectID, copyRole.ObjectID, false)
	must(t, e)
	if copied.SourceRoleID != r.ObjectID || copied.SourceRoleVersion != r.Version || len(copied.Permissions) != 2 {
		t.Fatal(copied)
	}
	admin := f.preset(t, p.ProjectID, access.ProjectAdminKey)
	for _, id := range []string{admin.ID, foreign.ObjectID} {
		_, e = f.s.CreateRole(ctx, f.actor(f.b), f.tid, p.ProjectID, id, projects.RoleInput{ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), ProjectVersion: "1"}, Name: "禁止复制", SourceVersion: "1", Permissions: []access.Code{}}, access.Meta{})
		if e == nil {
			t.Fatal("cross scope/admin copy")
		}
	}
	for _, scope := range []string{"tenant", "platform"} {
		var id string
		must(t, f.e.Pool.QueryRow(ctx, `SELECT id::text FROM roles WHERE binding_scope=$1 LIMIT 1`, scope).Scan(&id))
		_, e = f.s.Role(ctx, f.actor(f.b), f.tid, p.ProjectID, id, false)
		code(t, e, "not_found")
	}
	f.change(t, f.b, p.ProjectID, r.ObjectID, access.OpProjectRoleStatus, projects.Change{Status: "disabled"})
	pv, e := f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, r.ObjectID, access.OpProjectRoleDelete, projects.Change{})
	blocked(t, pv, e)
	f.change(t, f.b, p.ProjectID, carol.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{backup.ObjectID}})
	pv, e = f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, carol.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{r.ObjectID}})
	blocked(t, pv, e)
	pv, e = f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, r.ObjectID, access.OpProjectRoleDelete, projects.Change{})
	blocked(t, pv, e)
	f.change(t, f.b, p.ProjectID, david.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{}})
	f.change(t, f.b, p.ProjectID, r.ObjectID, access.OpProjectRoleDelete, projects.Change{})
	if count(t, f, `SELECT count(*) FROM roles WHERE id=$1 AND deleted_at IS NOT NULL`, r.ObjectID) != 1 {
		t.Fatal("no soft-delete")
	}
	auth, e := f.s.Authorization(ctx, f.actor(f.b), f.tid, p.ProjectID, david.ObjectID)
	must(t, e)
	if len(auth.Bindings) < 2 || len(auth.Roles) != 0 {
		t.Fatal(auth)
	}
}

func TestProjectUpperEligibilityCoverageAndCandidateRecheck(t *testing.T) {
	f := setup(t)
	p := f.create(t, "upper-states", f.bid)
	carol := f.add(t, f.b, p.ProjectID, f.cid)
	david := f.add(t, f.b, p.ProjectID, f.did)
	manager := f.role(t, p.ProjectID, "成员操作员", []access.Code{access.ProjectMemberRead, access.ProjectMemberSuspend, access.ProjectMemberRemove})
	powerful := f.role(t, p.ProjectID, "资料修改", []access.Code{access.ProjectProfileRead, access.ProjectProfileUpdate})
	f.change(t, f.b, p.ProjectID, carol.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{manager.ObjectID}})
	f.change(t, f.b, p.ProjectID, david.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{powerful.ObjectID}})
	f.change(t, f.b, p.ProjectID, powerful.ObjectID, access.OpProjectRoleStatus, projects.Change{Status: "disabled"})
	_, e := f.s.Preview(ctx, f.actor(f.c), f.tid, p.ProjectID, david.ObjectID, access.OpProjectMemberSuspend, projects.Change{})
	code(t, e, "grant_scope_exceeded")
	pv, e := f.s.Preview(ctx, f.actor(f.c), f.tid, p.ProjectID, carol.ObjectID, access.OpProjectMemberSuspend, projects.Change{})
	blocked(t, pv, e)
	f.change(t, f.b, p.ProjectID, david.ObjectID, access.OpProjectMemberSuspend, projects.Change{})
	// Project suspension permits preconfiguration without silently resuming eligibility.
	ordinary := f.preset(t, p.ProjectID, access.ProjectMemberKey)
	f.change(t, f.b, p.ProjectID, david.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{ordinary.ID}})
	f.parentState(t, f.did, access.OpTenantMemberSuspend)
	pv, e = f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, david.ObjectID, access.OpProjectMemberResume, projects.Change{})
	blocked(t, pv, e)
	pv, e = f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, david.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{ordinary.ID, manager.ObjectID}})
	blocked(t, pv, e)
	f.change(t, f.b, p.ProjectID, david.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{}})
	f.parentState(t, f.did, access.OpTenantMemberResume)
	auth, e := f.s.Authorization(ctx, f.actor(f.b), f.tid, p.ProjectID, david.ObjectID)
	must(t, e)
	if auth.MembershipStatus != "suspended" || len(auth.Permissions) != 0 {
		t.Fatal(auth)
	}
	f.change(t, f.b, p.ProjectID, david.ObjectID, access.OpProjectMemberResume, projects.Change{})
	// A previously valid candidate may become invalid before create/owner/admin submit.
	pending := f.prep(t, f.b, p.ProjectID, "", access.OpProjectOwnerChange, projects.Change{Candidate: f.did})
	f.parentState(t, f.did, access.OpTenantMemberSuspend)
	_, e = f.s.Change(ctx, f.actor(f.b), f.tid, p.ProjectID, "", access.OpProjectOwnerChange, pending, access.Meta{})
	code(t, e, "candidate_unavailable")
	_, e = f.s.Create(ctx, f.actor(f.a), f.tid, f.input(t, "invalid-candidate", f.did), access.Meta{})
	code(t, e, "candidate_unavailable")
	if count(t, f, `SELECT count(*) FROM projects WHERE code='invalid-candidate'`) != 0 {
		t.Fatal("partial candidate create")
	}
	// Invalid owners are responsibility warnings; they do not prevent safe archive/recovery.
	f.parentState(t, f.cid, access.OpTenantMemberSuspend)
	pr, e := f.s.Profile(ctx, f.actor(f.b), f.tid, p.ProjectID)
	must(t, e)
	if pr.Owner.Available {
		t.Fatal(pr)
	}
	f.change(t, f.b, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
	f.change(t, f.b, p.ProjectID, "", access.OpProjectRestore, projects.Change{})
}

func TestProjectCatalogPublicationAuditCleanupAndPaging(t *testing.T) {
	f := setup(t)
	p := f.create(t, "catalog", f.bid)
	c := f.add(t, f.b, p.ProjectID, f.cid)
	memberRole := f.preset(t, p.ProjectID, access.ProjectMemberKey)
	// Simulate an older published role with less authority, using its real projection.
	must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_code=$2`, memberRole.ID, access.ProjectProfileRead)
		return e
	}))
	f.change(t, f.b, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
	report, e := f.e.Service.Access().SyncPreview(ctx)
	must(t, e)
	if len(report.Projects) == 0 || len(report.Conflicts) == 0 {
		t.Fatal(report)
	}
	if e = f.e.Service.Access().Sync(ctx); e == nil {
		t.Fatal("archived preset expansion")
	}
	f.change(t, f.b, p.ProjectID, "", access.OpProjectRestore, projects.Change{})
	must(t, f.e.Service.Access().Sync(ctx))
	must(t, f.e.Service.Access().Check(ctx))
	auth, e := f.s.Authorization(ctx, f.actor(f.b), f.tid, p.ProjectID, c.ObjectID)
	must(t, e)
	if !slices.Contains(auth.Permissions, access.ProjectProfileRead) {
		t.Fatal(auth)
	}
	platformAudit, e := f.e.Service.Access().AuditEvents(ctx, f.actor(f.p), access.Filter{Limit: 100})
	must(t, e)
	raw, _ := json.Marshal(platformAudit)
	if strings.Contains(string(raw), c.ObjectID) {
		t.Fatal("publication leaked project member grants")
	}
	if count(t, f, `SELECT count(*) FROM account_security_events WHERE project_id=$1 AND event_type='project.authorization.preset_synced'`, p.ProjectID) != 1 {
		t.Fatal("project preset audit missing")
	}
	before := count(t, f, `SELECT count(*) FROM account_security_events WHERE project_id=$1`, p.ProjectID)
	must(t, f.e.Service.Access().Sync(ctx))
	if count(t, f, `SELECT count(*) FROM account_security_events WHERE project_id=$1`, p.ProjectID) != before {
		t.Fatal("sync duplicated audit")
	}
	// Verify all pages by primary ID, including historical objects, instead of trusting a first-page count.
	for i := 0; i < 23; i++ {
		f.role(t, p.ProjectID, fmt.Sprintf("分页角色%02d", i), []access.Code{})
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		page, e := f.s.Roles(ctx, f.actor(f.b), f.tid, p.ProjectID, projects.Filter{Limit: 7, Cursor: cursor}, false)
		must(t, e)
		for _, r := range page.Items {
			if seen[r.ID] {
				t.Fatal("duplicate cursor row")
			}
			seen[r.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 28 {
		t.Fatal("incomplete role pages", len(seen))
	}
	auditIDs := map[string]bool{}
	cursor = ""
	for {
		page, e := f.s.Audit(ctx, f.actor(f.b), f.tid, p.ProjectID, projects.Filter{Limit: 5, Cursor: cursor})
		must(t, e)
		for _, a := range page.Items {
			if auditIDs[a.ID] {
				t.Fatal("duplicate audit")
			}
			auditIDs[a.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(auditIDs) != count(t, f, `SELECT count(*) FROM account_security_events WHERE project_id=$1`, p.ProjectID) {
		t.Fatal("incomplete audit")
	}
	_, e = f.e.Pool.Exec(ctx, `UPDATE account_security_events SET created_at=clock_timestamp()-interval '200 days' WHERE project_id=$1`, p.ProjectID)
	must(t, e)
	_, e = f.e.Pool.Exec(ctx, `INSERT INTO account_security_events(event_type,outcome,created_at) VALUES('expired.nonproject','success',clock_timestamp()-interval '200 days')`)
	must(t, e)
	must(t, identity.NewRepository(f.e.Pool).Cleanup(ctx, 90*24*time.Hour))
	if count(t, f, `SELECT count(*) FROM account_security_events WHERE project_id=$1`, p.ProjectID) != len(auditIDs) || count(t, f, `SELECT count(*) FROM account_security_events WHERE event_type='expired.nonproject'`) != 0 {
		t.Fatal("retention mismatch")
	}
}

func TestProjectSerializedRacesAndReplayAfterRevocation(t *testing.T) {
	f := setup(t)
	p := f.create(t, "race", f.bid)
	member := f.add(t, f.b, p.ProjectID, f.cid)
	// A held graph lock gives deterministic overlap without timing assumptions.
	race := func(first, second func() error) []error {
		t.Helper()
		tx, e := f.e.Pool.Begin(ctx)
		must(t, e)
		_, e = access.Lock(ctx, tx, true)
		must(t, e)
		ready := make(chan struct{}, 2)
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for _, fn := range []func() error{first, second} {
			wg.Go(func() { ready <- struct{}{}; errs <- fn() })
		}
		<-ready
		<-ready
		must(t, tx.Commit(ctx))
		wg.Wait()
		close(errs)
		out := []error{}
		for e := range errs {
			out = append(out, e)
		}
		return out
	}
	archive := f.prep(t, f.b, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
	add := projects.AddMemberInput{WriteMeta: f.wm(t), ProjectVersion: "1", Candidate: f.did}
	errs := race(func() error {
		_, e := f.s.Change(ctx, f.actor(f.b), f.tid, p.ProjectID, "", access.OpProjectArchive, archive, access.Meta{})
		return e
	}, func() error {
		_, e := f.s.AddMember(ctx, f.actor(f.b), f.tid, p.ProjectID, add, access.Meta{})
		return e
	})
	successes := 0
	for _, e := range errs {
		if e == nil {
			successes++
		} else if !slices.Contains([]string{"preview_stale", "project_archived"}, identity.PublicError(e).Code) {
			t.Fatal(e)
		}
	}
	if successes != 1 {
		t.Fatal(errs)
	}
	state, e := f.s.Profile(ctx, f.actor(f.b), f.tid, p.ProjectID)
	must(t, e)
	if state.Status == "archived" {
		f.change(t, f.b, p.ProjectID, "", access.OpProjectRestore, projects.Change{})
	}
	r := f.role(t, p.ProjectID, "并发角色", []access.Code{access.ProjectProfileRead})
	remove := f.prep(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberRemove, projects.Change{})
	assign := f.prep(t, f.b, p.ProjectID, member.ObjectID, access.OpProjectMemberRoles, projects.Change{Roles: []string{r.ObjectID}})
	errs = race(func() error {
		_, e := f.s.Change(ctx, f.actor(f.b), f.tid, p.ProjectID, member.ObjectID, access.OpProjectMemberRemove, remove, access.Meta{})
		return e
	}, func() error {
		_, e := f.s.Change(ctx, f.actor(f.b), f.tid, p.ProjectID, member.ObjectID, access.OpProjectMemberRoles, assign, access.Meta{})
		return e
	})
	successes = 0
	for _, e := range errs {
		if e == nil {
			successes++
		} else if !slices.Contains([]string{"preview_stale", "state_conflict", "version_conflict"}, identity.PublicError(e).Code) {
			t.Fatal(e)
		}
	}
	if successes != 1 {
		t.Fatal(errs)
	}
	if count(t, f, `SELECT count(*) FROM project_member_roles b JOIN project_memberships m ON m.id=b.membership_id WHERE m.status='ended' AND b.revoked_at IS NULL`) != 0 {
		t.Fatal("ended binding race")
	}
	// A transferred actor cannot use a receipt to retrieve an old success or duplicate an audit.
	handover := f.prep(t, f.b, p.ProjectID, "", access.OpProjectTransfer, projects.Change{Candidate: f.aid})
	_, e = f.s.Change(ctx, f.actor(f.b), f.tid, p.ProjectID, "", access.OpProjectTransfer, handover, access.Meta{})
	must(t, e)
	_, e = f.s.Change(ctx, f.actor(f.b), f.tid, p.ProjectID, "", access.OpProjectTransfer, handover, access.Meta{})
	code(t, e, "project_not_found")
	// Project handover and removal of its prospective tenant parent serialize to one valid result.
	q := f.create(t, "handover-parent-race", f.bid)
	handover = f.prep(t, f.b, q.ProjectID, "", access.OpProjectTransfer, projects.Change{Candidate: f.did})
	parent, e := f.ts.Member(ctx, f.actor(f.a), f.tid, f.did)
	must(t, e)
	errs = race(func() error {
		_, e := f.s.Change(ctx, f.actor(f.b), f.tid, q.ProjectID, "", access.OpProjectTransfer, handover, access.Meta{})
		return e
	}, func() error {
		_, e := f.ts.ChangeMember(ctx, f.actor(f.a), f.tid, f.did, access.OpTenantMemberRemove, tenancy.MemberInput{Version: parent.Version, Reason: "并发结束上层成员"}, access.Meta{})
		return e
	})
	successes = 0
	for _, e := range errs {
		if e == nil {
			successes++
		} else if !slices.Contains([]string{"candidate_unavailable", "project_admin_transfer_required"}, identity.PublicError(e).Code) {
			t.Fatal(e)
		}
	}
	if successes != 1 {
		t.Fatal(errs)
	}
	// Release synchronization and a different project's edit share the same lock order.
	current, e := f.s.Profile(ctx, f.actor(f.a), f.tid, p.ProjectID)
	must(t, e)
	edit := projects.UpdateInput{Fields: projects.Fields{Name: "同步争用后的资料", Code: current.Code, Purpose: current.Purpose}, ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), Version: current.Version, ProjectVersion: current.Version}}
	errs = race(func() error { return f.e.Service.Access().Sync(ctx) }, func() error { _, e := f.s.Update(ctx, f.actor(f.a), f.tid, p.ProjectID, edit, access.Meta{}); return e })
	for _, e := range errs {
		must(t, e)
	}
	// Recent verification is checked from the live session, even when the actor was captured earlier.
	recent := f.prep(t, f.a, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
	_, e = f.e.Pool.Exec(ctx, `UPDATE account_sessions SET reauthenticated_at=clock_timestamp()-interval '6 minutes' WHERE id=$1`, f.a.Session.ID)
	must(t, e)
	_, e = f.s.Change(ctx, f.actor(f.a), f.tid, p.ProjectID, "", access.OpProjectArchive, recent, access.Meta{})
	code(t, e, "reauthentication_required")
	must(t, f.e.Service.Reauthenticate(ctx, f.a, password, identity.Meta{}))
	state, e = f.s.Profile(ctx, f.actor(f.a), f.tid, p.ProjectID)
	must(t, e)
	if state.Status != "active" {
		t.Fatal("reauthentication replayed operation")
	}
	// Timeout after taking no write lock leaves no receipt or partial mutation.
	tx, e := f.e.Pool.Begin(ctx)
	must(t, e)
	_, e = access.Lock(ctx, tx, true)
	must(t, e)
	limited, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	_, e = f.s.Create(limited, f.actor(f.a), f.tid, f.input(t, "timeout", f.aid), access.Meta{})
	if e == nil {
		t.Fatal("timeout succeeded")
	}
	must(t, tx.Rollback(ctx))
	if count(t, f, `SELECT count(*) FROM projects WHERE code='timeout'`) != 0 {
		t.Fatal("partial timeout")
	}
	must(t, f.e.Service.Access().Check(ctx))
}

func TestProjectArchiveFullOperationMatrixAndIdentityConstraints(t *testing.T) {
	f := setup(t)
	p := f.create(t, "archive-matrix", f.bid)
	m := f.add(t, f.b, p.ProjectID, f.cid)
	r := f.role(t, p.ProjectID, "归档角色", []access.Code{access.ProjectProfileRead})
	f.change(t, f.b, p.ProjectID, m.ObjectID, access.OpProjectMemberSuspend, projects.Change{})
	f.change(t, f.b, p.ProjectID, r.ObjectID, access.OpProjectRoleStatus, projects.Change{Status: "disabled"})
	f.change(t, f.b, p.ProjectID, "", access.OpProjectArchive, projects.Change{})
	for _, tc := range []struct {
		op     access.Operation
		target string
		in     projects.Change
	}{
		{access.OpProjectOwnerChange, "", projects.Change{Candidate: f.did}},
		{access.OpProjectMemberResume, m.ObjectID, projects.Change{}},
		{access.OpProjectRoleStatus, r.ObjectID, projects.Change{Status: "active"}},
		{access.OpProjectRolePermissions, r.ObjectID, projects.Change{Permissions: []access.Code{access.ProjectProfileRead, access.ProjectProfileUpdate}}},
		{access.OpProjectMemberRoles, m.ObjectID, projects.Change{Roles: []string{r.ObjectID}}},
	} {
		pv, e := f.s.Preview(ctx, f.actor(f.b), f.tid, p.ProjectID, tc.target, tc.op, tc.in)
		blocked(t, pv, e)
	}
	for _, source := range []string{"", r.ObjectID} {
		_, e := f.s.CreateRole(ctx, f.actor(f.b), f.tid, p.ProjectID, source, projects.RoleInput{ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), ProjectVersion: "2"}, Name: "归档新角色", Permissions: []access.Code{}}, access.Meta{})
		code(t, e, "project_archived")
	}
	_, e := f.s.Update(ctx, f.actor(f.b), f.tid, p.ProjectID, projects.UpdateInput{Fields: projects.Fields{Name: "归档修改", Code: "archive-matrix", Purpose: "不能修改"}, ChangeMeta: projects.ChangeMeta{WriteMeta: f.wm(t), Version: "2", ProjectVersion: "2"}}, access.Meta{})
	code(t, e, "project_archived")
	// SQL rejects fabricated gaps, changed membership parents and deleted historical codes.
	for _, q := range []string{
		`UPDATE projects SET previous_administrator_membership_id=administrator_membership_id,administrator_membership_id=NULL,management_gap_reason='fake',management_gap_at=now(),management_gap_by=created_by WHERE id=$1`,
		`DELETE FROM project_codes WHERE project_id=$1`,
		`UPDATE project_memberships SET tenant_membership_id=(SELECT id FROM tenant_memberships WHERE tenant_id=project_memberships.tenant_id AND account_id=(SELECT id FROM accounts WHERE username='David')),account_id=(SELECT id FROM accounts WHERE username='David') WHERE project_id=$1 AND account_id=(SELECT id FROM accounts WHERE username='Carol')`,
	} {
		_, e = f.e.Pool.Exec(ctx, q, p.ProjectID)
		if e == nil {
			t.Fatal("invalid SQL accepted", q)
		}
	}
	f.lifecycle(t, "disabled")
	for _, policy := range access.Policies() {
		if policy.Class != "project" {
			continue
		}
		e = f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			_, e := access.RequireProjectTx(ctx, tx, f.b.Account.ID, policy.Operation, f.tid, p.ProjectID)
			return e
		})
		code(t, e, "tenant_disabled")
	}
	status, e := f.s.Availability(ctx, f.actor(f.b), f.tid, p.ProjectID)
	must(t, e)
	if status.CanEnter || len(status.Authorization.Permissions) != 0 {
		t.Fatal(status)
	}
	f.lifecycle(t, "active")
	status, e = f.s.Context(ctx, f.actor(f.b), f.tid, p.ProjectID)
	must(t, e)
	if status.Project.Status != "archived" {
		t.Fatal("tenant resume changed project")
	}
	f.change(t, f.b, p.ProjectID, "", access.OpProjectRestore, projects.Change{})
	auth, e := f.s.Authorization(ctx, f.actor(f.b), f.tid, p.ProjectID, m.ObjectID)
	must(t, e)
	if auth.MembershipStatus != "suspended" {
		t.Fatal("restore revived member")
	}
	role, e := f.s.Role(ctx, f.actor(f.b), f.tid, p.ProjectID, r.ObjectID, false)
	must(t, e)
	if role.Status != "disabled" {
		t.Fatal("restore revived role")
	}
}
