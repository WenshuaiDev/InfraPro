package tenancy_test

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/jackc/pgx/v5"
)

func (f fixture) localRole(t *testing.T, tid, name string, codes ...access.Code) access.TenantRole {
	t.Helper()
	if codes == nil {
		codes = []access.Code{}
	}
	r, e := f.e.Service.Access().CreateTenantRole(ctx, f.actor(f.a), tid, "", access.TenantRoleCreateInput{Name: name, Permissions: codes, RequestID: f.uuid(t), Reason: "测试创建"}, access.Meta{})
	must(t, e)
	return r.Role
}
func (f fixture) preset(t *testing.T, tid, key string) access.TenantRole {
	t.Helper()
	p, e := f.e.Service.Access().TenantRoles(ctx, f.actor(f.a), tid, access.Filter{})
	must(t, e)
	for _, r := range p.Items {
		if r.BuiltinKey == key {
			return r
		}
	}
	t.Fatal(key)
	return access.TenantRole{}
}
func (f fixture) roleInput(t *testing.T, tid, mid string, ids ...string) access.TenantChange {
	t.Helper()
	if ids == nil {
		ids = []string{}
	}
	a, e := f.e.Service.Access().TenantMemberAuthorization(ctx, f.actor(f.a), tid, mid)
	must(t, e)
	return access.TenantChange{ExpectedMembershipVersion: a.MembershipVersion, RoleIDs: ids, Reason: "测试成员授权"}
}
func (f fixture) preview(t *testing.T, tid, id string, op access.Operation, in access.TenantChange) access.TenantChange {
	t.Helper()
	p, e := f.e.Service.Access().PreviewTenantChange(ctx, f.actor(f.a), tid, id, op, in, access.Filter{})
	must(t, e)
	in.PreviewHash = p.Hash
	in.RoleVersions = p.RoleVersions
	return in
}
func (f fixture) grant(t *testing.T, tid, mid string, ids ...string) access.TenantAuthorization {
	t.Helper()
	in := f.preview(t, tid, mid, access.OpTenantMemberRolesPreview, f.roleInput(t, tid, mid, ids...))
	v, e := f.e.Service.Access().ChangeTenantAuthorization(ctx, f.actor(f.a), tid, mid, access.OpTenantMemberRoles, in, access.Meta{})
	must(t, e)
	return *v.Authorization
}
func (f fixture) changeRole(t *testing.T, tid string, r access.TenantRole, op access.Operation, in access.TenantChange) access.TenantRole {
	t.Helper()
	in.ExpectedVersion = r.Version
	in.Reason = "测试角色变化"
	pre := access.OpTenantRolePermissionsPreview
	if op == access.OpTenantRoleStatus {
		pre = access.OpTenantRoleStatusPreview
	}
	in = f.preview(t, tid, r.ID, pre, in)
	v, e := f.e.Service.Access().ChangeTenantAuthorization(ctx, f.actor(f.a), tid, r.ID, op, in, access.Meta{})
	must(t, e)
	return *v.Role
}

func TestTenantRoleUnionIsolationAndInvitationLoss(t *testing.T) {
	f := setup(t)
	s := f.e.Service.Access()
	a := f.create(t, "role-union-a", f.a.Account.ID)
	b := f.create(t, "role-union-b", f.a.Account.ID)
	joined := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
	other := f.accept(t, f.invite(t, b.TenantID, f.a, f.b), f.b)
	catalog, e := s.TenantPermissions(ctx, f.actor(f.a), a.TenantID)
	must(t, e)
	n := 0
	for _, p := range catalog {
		if p.Selectable {
			n++
		}
	}
	if len(catalog) != 46 || n != 37 {
		t.Fatal(catalog)
	}
	_, e = s.TenantRoles(ctx, f.actor(f.b), a.TenantID, access.Filter{})
	code(t, e, "permission_denied")
	profile := f.preset(t, a.TenantID, "tenant_profile_maintainer")
	inviter := f.preset(t, a.TenantID, "tenant_invitation_coordinator")
	backup := f.localRole(t, a.TenantID, "邀请备份", access.TenantProfileRead, access.TenantInvitationCreate)
	snap := f.grant(t, a.TenantID, joined.MembershipID, profile.ID, inviter.ID, backup.ID)
	if len(snap.Sources[access.TenantProfileRead]) != 3 || !slices.Contains(snap.Permissions, access.TenantProfileUpdate) {
		t.Fatal(snap)
	}
	f.grant(t, b.TenantID, other.MembershipID, inviter.ID)
	link := f.invite(t, a.TenantID, f.b, f.p)
	linkB := f.invite(t, b.TenantID, f.b, f.p)
	f.grant(t, a.TenantID, joined.MembershipID, backup.ID)
	_, e = f.s.InspectInvitation(ctx, f.actor(f.p), tokenOf(link.Link), access.Meta{})
	must(t, e)
	backup = f.changeRole(t, a.TenantID, backup, access.OpTenantRolePermissions, access.TenantChange{Permissions: []access.Code{access.TenantProfileRead}})
	inspection, inspectErr := f.s.InspectInvitation(ctx, f.actor(f.p), tokenOf(link.Link), access.Meta{})
	must(t, inspectErr)
	if inspection.CanAccept || inspection.Status != "invalidated" {
		t.Fatal(inspection)
	}
	_, e = f.s.InspectInvitation(ctx, f.actor(f.p), tokenOf(linkB.Link), access.Meta{})
	must(t, e)
	f.changeRole(t, a.TenantID, backup, access.OpTenantRolePermissions, access.TenantChange{Permissions: []access.Code{access.TenantProfileRead, access.TenantInvitationCreate}})
	{
		inspection, inspectErr := f.s.InspectInvitation(ctx, f.actor(f.p), tokenOf(link.Link), access.Meta{})
		must(t, inspectErr)
		if inspection.CanAccept || inspection.Status != "invalidated" {
			t.Fatal(inspection)
		}
	}
	f.grant(t, a.TenantID, joined.MembershipID)
	c, e := f.s.Context(ctx, f.actor(f.b), a.TenantID)
	must(t, e)
	if len(c.Permissions) != 0 || len(c.Authorization.Roles) != 0 {
		t.Fatal(c)
	}
	_, e = f.s.Get(ctx, f.actor(f.b), a.TenantID, false)
	code(t, e, "permission_denied")
	_, e = s.TenantRole(ctx, f.actor(f.a), b.TenantID, backup.ID)
	code(t, e, "not_found")
	_, e = s.ChangeRole(ctx, f.actor(f.p), backup.ID, access.OpRoleUpdate, access.RoleChange{ExpectedVersion: backup.Version, Name: "入侵"}, access.Meta{Reason: "跨范围"})
	code(t, e, "not_found")
	must(t, s.Check(ctx))
}
func TestTenantRoleCreationProtectionAndVersions(t *testing.T) {
	f := setup(t)
	s := f.e.Service.Access()
	a := f.create(t, "role-create-a", f.a.Account.ID)
	b := f.create(t, "role-create-b", f.a.Account.ID)
	r := f.localRole(t, a.TenantID, " 同名 ")
	f.localRole(t, b.TenantID, "同名")
	in := access.TenantRoleCreateInput{Name: "幂等", Permissions: []access.Code{}, RequestID: f.uuid(t), Reason: "创建原因"}
	one, e := s.CreateTenantRole(ctx, f.actor(f.a), a.TenantID, "", in, access.Meta{})
	must(t, e)
	two, e := s.CreateTenantRole(ctx, f.actor(f.a), a.TenantID, "", in, access.Meta{})
	must(t, e)
	if one.Role.ID != two.Role.ID || !two.Replayed {
		t.Fatal(two)
	}
	var auditCount int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events WHERE event_type='tenant.role.created' AND object_id=$1`, one.Role.ID).Scan(&auditCount))
	if auditCount != 1 {
		t.Fatal("replayed creation wrote duplicate audit", auditCount)
	}
	in.Name = "不同"
	_, e = s.CreateTenantRole(ctx, f.actor(f.a), a.TenantID, "", in, access.Meta{})
	code(t, e, "request_conflict")
	for _, codes := range [][]access.Code{nil, {access.ProfileRead}, {access.TenantRoleRead}, {"tenant.unknown"}, {access.TenantProfileRead, access.TenantProfileRead}} {
		in.Permissions = codes
		in.RequestID = f.uuid(t)
		_, e = s.CreateTenantRole(ctx, f.actor(f.a), a.TenantID, "", in, access.Meta{})
		code(t, e, "validation_failed")
	}
	for _, name := range []string{"同名", "租户普通成员"} {
		in.Permissions = []access.Code{}
		in.Name = name
		in.RequestID = f.uuid(t)
		_, e = s.CreateTenantRole(ctx, f.actor(f.a), a.TenantID, "", in, access.Meta{})
		if name == "同名" {
			code(t, e, "name_taken")
		} else {
			code(t, e, "name_reserved")
		}
	}
	source := f.preset(t, a.TenantID, "tenant_invitation_coordinator")
	in = access.TenantRoleCreateInput{Name: "独立副本", Permissions: []access.Code{access.TenantInvitationCreate}, RequestID: f.uuid(t), SourceVersion: source.Version, Reason: "复制"}
	copied, e := s.CreateTenantRole(ctx, f.actor(f.a), a.TenantID, source.ID, in, access.Meta{})
	must(t, e)
	if copied.Role.SourceRoleID != source.ID || copied.Role.MemberCount != 0 || len(copied.Role.Permissions) != 1 {
		t.Fatal(copied)
	}
	admin := f.preset(t, a.TenantID, access.TenantAdminKey)
	in.RequestID = f.uuid(t)
	in.SourceVersion = admin.Version
	_, e = s.CreateTenantRole(ctx, f.actor(f.a), a.TenantID, admin.ID, in, access.Meta{})
	code(t, e, "role_protected")
	change := access.TenantChange{ExpectedVersion: r.Version, Name: "改名", Description: "说明", Reason: "编辑"}
	v, e := s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRoleUpdate, change, access.Meta{})
	must(t, e)
	if len(v.Role.Permissions) != 0 || v.Role.ID != r.ID {
		t.Fatal(v)
	}
	var preciseAudit bool
	must(t, f.e.Pool.QueryRow(ctx, `SELECT tenant_id=$2::uuid AND actor_id=$3::uuid AND actor_scope='tenant' AND reason='编辑' AND details->'before'=$4::jsonb AND details->'after'=$5::jsonb AND jsonb_array_length(details->'permission_definitions')=46 FROM account_security_events WHERE event_type='tenant.role.updated' AND object_id=$1`, r.ID, a.TenantID, f.a.Account.ID, fmt.Sprintf(`{"version":%q,"name":"同名","description":""}`, r.Version), fmt.Sprintf(`{"version":%q,"name":"改名","description":"说明"}`, v.Role.Version)).Scan(&preciseAudit))
	if !preciseAudit {
		t.Fatal("role metadata audit omitted identity or included unrelated changes")
	}
	_, e = s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRoleUpdate, change, access.Meta{})
	code(t, e, "version_conflict")
	_, e = s.PreviewTenantChange(ctx, f.actor(f.a), a.TenantID, source.ID, access.OpTenantRoleArchivePreview, access.TenantChange{ExpectedVersion: source.Version}, access.Filter{})
	code(t, e, "role_protected")
}
func TestTenantRoleDisabledSuspendedAndDatabaseGuards(t *testing.T) {
	f := setup(t)
	s := f.e.Service.Access()
	a := f.create(t, "role-state-a", f.a.Account.ID)
	b := f.create(t, "role-state-b", f.a.Account.ID)
	m := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
	r := f.localRole(t, a.TenantID, "本地职责", access.TenantProfileRead, access.TenantProfileUpdate)
	f.grant(t, a.TenantID, m.MembershipID, r.ID)
	r = f.changeRole(t, a.TenantID, r, access.OpTenantRoleStatus, access.TenantChange{Status: "disabled"})
	snap := f.grant(t, a.TenantID, m.MembershipID, r.ID)
	if len(snap.Roles) != 1 || len(snap.Permissions) != 0 {
		t.Fatal(snap)
	}
	in := access.TenantChange{ExpectedVersion: r.Version, Reason: "删除"}
	pre, e := s.PreviewTenantChange(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRoleArchivePreview, in, access.Filter{})
	must(t, e)
	if len(pre.BlockedReasons) != 1 {
		t.Fatal(pre)
	}
	f.grant(t, a.TenantID, m.MembershipID)
	_, e = s.PreviewTenantChange(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberRolesPreview, f.roleInput(t, a.TenantID, m.MembershipID, r.ID), access.Filter{})
	code(t, e, "role_disabled")
	r = f.changeRole(t, a.TenantID, r, access.OpTenantRoleStatus, access.TenantChange{Status: "active"})
	snap = f.grant(t, a.TenantID, m.MembershipID, r.ID)
	suspended, e := f.s.ChangeMember(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberSuspend, tenancy.MemberInput{Version: snap.MembershipVersion, Reason: "暂停"}, access.Meta{})
	must(t, e)
	extra := f.preset(t, a.TenantID, "tenant_auditor")
	snap = f.grant(t, a.TenantID, m.MembershipID, extra.ID, r.ID)
	if len(snap.Permissions) != 0 || len(snap.RestorablePermissions) != 3 {
		t.Fatal(snap)
	}
	impact, e := s.PreviewTenantChange(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRoleStatusPreview, access.TenantChange{ExpectedVersion: r.Version, Status: "disabled"}, access.Filter{Limit: 1})
	must(t, e)
	if impact.BoundMembers != 1 || impact.EffectiveMembers != 0 || impact.LosingMembers != 0 || impact.RestorableMembers != 1 || len(impact.Members.Items) != 1 {
		t.Fatal("suspended member impact conflated current and restorable rights", impact)
	}
	blocked, e := s.PreviewTenantChange(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRoleArchivePreview, access.TenantChange{ExpectedVersion: r.Version}, access.Filter{})
	must(t, e)
	if len(blocked.BlockedReasons) != 1 {
		t.Fatal("suspended member's role can be deleted", blocked)
	}
	_, e = f.s.ChangeMember(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberResume, tenancy.MemberInput{Version: suspended.Version, Reason: "旧版本"}, access.Meta{})
	code(t, e, "version_conflict")
	_, e = f.s.ChangeMember(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberResume, tenancy.MemberInput{Version: snap.MembershipVersion, Reason: "恢复"}, access.Meta{})
	must(t, e)
	// Database rejects cross-tenant grants, protected rights, deleting used roles and forged admins.
	foreign := f.localRole(t, b.TenantID, "外部职责", access.TenantProfileRead)
	admin := f.preset(t, a.TenantID, access.TenantAdminKey)
	for _, sql := range []string{fmt.Sprintf(`INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) VALUES('%s','%s','%s','%s')`, a.TenantID, m.MembershipID, foreign.ID, f.a.Account.ID), fmt.Sprintf(`INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES('%s','%s','tenant')`, r.ID, access.TenantRoleRead), fmt.Sprintf(`UPDATE roles SET deleted_at=now() WHERE id='%s'`, r.ID), fmt.Sprintf(`INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) VALUES('%s','%s','%s','%s')`, a.TenantID, m.MembershipID, admin.ID, f.a.Account.ID)} {
		if _, e = f.e.Pool.Exec(ctx, sql); e == nil {
			t.Fatal("database accepted invalid authorization", sql)
		}
	}
	must(t, f.status(t, f.b, false))
	snap = f.grant(t, a.TenantID, m.MembershipID, r.ID)
	if len(snap.Permissions) != 0 {
		t.Fatal(snap)
	}
	_, e = s.PreviewTenantChange(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberRolesPreview, f.roleInput(t, a.TenantID, m.MembershipID, r.ID, extra.ID), access.Filter{})
	code(t, e, "target_account_unavailable")
	f.grant(t, a.TenantID, m.MembershipID)
	in.ExpectedVersion = r.Version
	in = f.preview(t, a.TenantID, r.ID, access.OpTenantRoleArchivePreview, in)
	_, e = s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRoleArchive, in, access.Meta{})
	must(t, e)
	must(t, s.Check(ctx))
}
func identityCode(e error) string {
	if v, ok := e.(*access.Error); ok {
		return v.Code
	}
	return ""
}

func TestTenantRolePreviewConcurrencyAndAuditRollback(t *testing.T) {
	f := setup(t)
	s := f.e.Service.Access()
	a := f.create(t, "role-races-a", f.a.Account.ID)
	b := f.create(t, "role-races-b", f.a.Account.ID)
	m := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
	r := f.localRole(t, a.TenantID, "邀请人", access.TenantProfileRead, access.TenantInvitationCreate)
	f.grant(t, a.TenantID, m.MembershipID, r.ID)
	in := access.TenantChange{ExpectedVersion: r.Version, Permissions: []access.Code{access.TenantProfileRead}, Reason: "撤权"}
	in = f.preview(t, a.TenantID, r.ID, access.OpTenantRolePermissionsPreview, in)
	f.localRole(t, b.TenantID, "无关变化", access.TenantProfileRead)
	// An unrelated tenant revision must not invalidate the preview.
	v, e := s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRolePermissions, in, access.Meta{})
	must(t, e)
	r = *v.Role
	r = f.changeRole(t, a.TenantID, r, access.OpTenantRolePermissions, access.TenantChange{Permissions: []access.Code{access.TenantProfileRead, access.TenantInvitationCreate}})
	in.ExpectedVersion = r.Version
	in = f.preview(t, a.TenantID, r.ID, access.OpTenantRolePermissionsPreview, in)
	link := f.invite(t, a.TenantID, f.b, f.p)
	_, e = s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRolePermissions, in, access.Meta{})
	code(t, e, "impact_changed")
	in = f.preview(t, a.TenantID, r.ID, access.OpTenantRolePermissionsPreview, in)
	_, e = f.e.Pool.Exec(ctx, `CREATE FUNCTION fail_role_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='tenant.role.permissions_changed' THEN RAISE EXCEPTION 'audit fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_role_audit BEFORE INSERT ON account_security_events FOR EACH ROW EXECUTE FUNCTION fail_role_audit()`)
	must(t, e)
	_, e = s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRolePermissions, in, access.Meta{})
	if e == nil {
		t.Fatal("audit failure committed")
	}
	_, e = f.s.InspectInvitation(ctx, f.actor(f.p), tokenOf(link.Link), access.Meta{})
	must(t, e)
	_, e = f.e.Pool.Exec(ctx, `DROP TRIGGER fail_role_audit ON account_security_events; DROP FUNCTION fail_role_audit()`)
	must(t, e)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, e := s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRolePermissions, in, access.Meta{})
			errs <- e
		})
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			code(t, e, "version_conflict")
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	{
		inspection, inspectErr := f.s.InspectInvitation(ctx, f.actor(f.p), tokenOf(link.Link), access.Meta{})
		must(t, inspectErr)
		if inspection.CanAccept || inspection.Status != "invalidated" {
			t.Fatal(inspection)
		}
	}
	must(t, s.Check(ctx))
}
func TestTenantRoleSyncPreviewNoopAndOldFacts(t *testing.T) {
	f := setup(t)
	s := f.e.Service.Access()
	a := f.create(t, "role-sync", f.a.Account.ID)
	m := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
	r := f.preset(t, a.TenantID, "tenant_profile_maintainer")
	copied, e := s.CreateTenantRole(ctx, f.actor(f.a), a.TenantID, r.ID, access.TenantRoleCreateInput{Name: "发布前独立副本", Permissions: slices.Clone(r.Permissions), RequestID: f.uuid(t), SourceVersion: r.Version, Reason: "副本不跟随"}, access.Meta{})
	must(t, e)
	f.grant(t, a.TenantID, m.MembershipID, r.ID, copied.Role.ID)
	// Emulate the previous release giving create to a shared preset. Sync must use stored old rights.
	_, e = f.e.Pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,'tenant')`, r.ID, access.TenantInvitationCreate)
	must(t, e)
	link := f.invite(t, a.TenantID, f.b, f.p)
	var revision string
	must(t, f.e.Pool.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&revision))
	report, e := s.SyncPreview(ctx)
	must(t, e)
	if !report.Changed || len(report.Tenants) != 1 || report.Tenants[0].InvalidatedInvitations != 1 {
		t.Fatal(report)
	}
	var current string
	must(t, f.e.Pool.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&current))
	if current != revision {
		t.Fatal("dry run wrote state")
	}
	must(t, s.Sync(ctx))
	{
		inspection, inspectErr := f.s.InspectInvitation(ctx, f.actor(f.p), tokenOf(link.Link), access.Meta{})
		must(t, inspectErr)
		if inspection.CanAccept || inspection.Status != "invalidated" {
			t.Fatal(inspection)
		}
	}
	report, e = s.SyncPreview(ctx)
	must(t, e)
	if report.Changed {
		t.Fatal(report)
	}
	r, e = s.TenantRole(ctx, f.actor(f.a), a.TenantID, r.ID)
	must(t, e)
	must(t, s.Sync(ctx))
	again, e := s.TenantRole(ctx, f.actor(f.a), a.TenantID, r.ID)
	must(t, e)
	if again.Version != r.Version {
		t.Fatal("noop bumped version")
	}
	independent, e := s.TenantRole(ctx, f.actor(f.a), a.TenantID, copied.Role.ID)
	must(t, e)
	if independent.Version != copied.Role.Version || !slices.Equal(independent.Permissions, copied.Role.Permissions) {
		t.Fatal("release changed independent copy")
	}
	must(t, s.Check(ctx))
	// Both statements must roll back together when structural validation fails.
	e = s.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE roles SET owner_tenant_id=$2 WHERE id=$1`, r.ID, a.TenantID)
		return e
	})
	if e == nil {
		t.Fatal("builtin accepted owner")
	}
}

func TestTenantRoleTargetCoverageAndMultiRoleHandover(t *testing.T) {
	f := setup(t)
	s := f.e.Service.Access()
	a := f.create(t, "role-coverage", f.a.Account.ID)
	manager := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
	target := f.accept(t, f.invite(t, a.TenantID, f.a, f.p), f.p)
	managers := f.preset(t, a.TenantID, "tenant_member_manager")
	inviters := f.preset(t, a.TenantID, "tenant_invitation_coordinator")
	f.grant(t, a.TenantID, manager.MembershipID, managers.ID)
	f.grant(t, a.TenantID, target.MembershipID, managers.ID, inviters.ID)
	limited, e := s.TenantMemberAuthorization(ctx, f.actor(f.b), a.TenantID, target.MembershipID)
	must(t, e)
	for _, r := range limited.Roles {
		if r.BindingScope != "tenant" {
			t.Fatal("platform role leak")
		}
	}
	targetAuth, e := s.TenantMemberAuthorization(ctx, f.actor(f.a), a.TenantID, target.MembershipID)
	must(t, e)
	_, e = f.s.ChangeMember(ctx, f.actor(f.b), a.TenantID, target.MembershipID, access.OpTenantMemberSuspend, tenancy.MemberInput{Version: targetAuth.MembershipVersion, Reason: "超出覆盖"}, access.Meta{})
	code(t, e, "grant_scope_exceeded")
	self, e := s.TenantMemberAuthorization(ctx, f.actor(f.a), a.TenantID, manager.MembershipID)
	must(t, e)
	_, e = f.s.ChangeMember(ctx, f.actor(f.b), a.TenantID, manager.MembershipID, access.OpTenantMemberRemove, tenancy.MemberInput{Version: self.MembershipVersion, Reason: "自己"}, access.Meta{})
	code(t, e, "self_membership_protected")
	f.grant(t, a.TenantID, manager.MembershipID, managers.ID, inviters.ID)
	suspended, e := f.s.ChangeMember(ctx, f.actor(f.b), a.TenantID, target.MembershipID, access.OpTenantMemberSuspend, tenancy.MemberInput{Version: targetAuth.MembershipVersion, Reason: "完整覆盖"}, access.Meta{})
	must(t, e)
	_, e = f.s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: a.Version, AdministratorID: f.p.Account.ID, Reason: "拒绝暂停接任"}, access.Meta{})
	code(t, e, "administrator_membership_suspended")
	_, e = f.s.ChangeMember(ctx, f.actor(f.b), a.TenantID, target.MembershipID, access.OpTenantMemberResume, tenancy.MemberInput{Version: suspended.Version, Reason: "恢复"}, access.Meta{})
	must(t, e)
	candidates, e := f.s.Candidates(ctx, f.actor(f.p), tenancy.Filter{TenantID: a.TenantID, Query: "Platform"})
	must(t, e)
	if len(candidates.Items) != 1 || len(candidates.Items[0].ReplacedRoles) != 2 {
		t.Fatal(candidates)
	}
	f.transfer(t, a, f.p.Account.ID)
	promoted, e := s.TenantMemberAuthorization(ctx, f.actor(f.p), a.TenantID, target.MembershipID)
	must(t, e)
	if !promoted.Administrator || len(promoted.Roles) != 1 || promoted.Roles[0].BuiltinKey != access.TenantAdminKey {
		t.Fatal(promoted)
	}
	_, e = f.s.Context(ctx, f.actor(f.a), a.TenantID)
	code(t, e, "tenant_not_found")
	must(t, s.Check(ctx))
}
func TestTenantRoleAssignmentRacesAndRejoin(t *testing.T) {
	for _, scenario := range []string{"remove", "delete", "accept"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t)
			s := f.e.Service.Access()
			a := f.create(t, "role-race-"+scenario, f.a.Account.ID)
			m := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
			r := f.localRole(t, a.TenantID, "竞争角色", access.TenantProfileRead, access.TenantInvitationCreate)
			var first, second func() error
			switch scenario {
			case "remove":
				in := f.preview(t, a.TenantID, m.MembershipID, access.OpTenantMemberRolesPreview, f.roleInput(t, a.TenantID, m.MembershipID, r.ID))
				first = func() error {
					_, e := s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberRoles, in, access.Meta{})
					return e
				}
				second = func() error {
					_, e := f.s.ChangeMember(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberRemove, tenancy.MemberInput{Version: m.Version, Reason: "结束"}, access.Meta{})
					return e
				}
			case "delete":
				in := f.preview(t, a.TenantID, m.MembershipID, access.OpTenantMemberRolesPreview, f.roleInput(t, a.TenantID, m.MembershipID, r.ID))
				del := f.preview(t, a.TenantID, r.ID, access.OpTenantRoleArchivePreview, access.TenantChange{ExpectedVersion: r.Version, Reason: "删除"})
				first = func() error {
					_, e := s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberRoles, in, access.Meta{})
					return e
				}
				second = func() error {
					_, e := s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, r.ID, access.OpTenantRoleArchive, del, access.Meta{})
					return e
				}
			case "accept":
				f.grant(t, a.TenantID, m.MembershipID, r.ID)
				link := f.invite(t, a.TenantID, f.b, f.p)
				in := f.preview(t, a.TenantID, m.MembershipID, access.OpTenantMemberRolesPreview, f.roleInput(t, a.TenantID, m.MembershipID))
				first = func() error {
					_, e := s.ChangeTenantAuthorization(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberRoles, in, access.Meta{})
					return e
				}
				second = func() error {
					_, e := f.s.AcceptInvitation(ctx, f.actor(f.p), tenancy.AcceptanceInput{Token: tokenOf(link.Link), Version: link.Invitation.Version}, access.Meta{})
					return e
				}
			}
			var wg sync.WaitGroup
			errors := make(chan error, 2)
			for _, fn := range []func() error{first, second} {
				wg.Go(func() { errors <- fn() })
			}
			wg.Wait()
			close(errors)
			passed := 0
			for e := range errors {
				if e == nil {
					passed++
				} else if !slices.Contains([]string{"version_conflict", "member_state_conflict", "role_in_use", "not_found", "impact_changed", "tenant_invitation_unavailable", "invitation_not_pending"}, identityCode(e)) {
					t.Fatal(e)
				}
			}
			if passed != 1 {
				t.Fatal("expected one winner", passed)
			}
			must(t, s.Check(ctx))
			// Removing after the race revokes every current grant; a new relation gets only the default.
			current, e := f.s.Member(ctx, f.actor(f.a), a.TenantID, m.MembershipID)
			must(t, e)
			if current.Status != "ended" {
				_, e = f.s.ChangeMember(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberRemove, tenancy.MemberInput{Version: current.Version, Reason: "重入前移除"}, access.Meta{})
				must(t, e)
			}
			rejoined := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
			if rejoined.MembershipID == m.MembershipID {
				t.Fatal("reused relation")
			}
			auth, e := s.TenantMemberAuthorization(ctx, f.actor(f.a), a.TenantID, rejoined.MembershipID)
			must(t, e)
			if len(auth.Roles) != 1 || auth.Roles[0].BuiltinKey != access.TenantMemberKey {
				t.Fatal(auth)
			}
		})
	}
}
func TestTenantRoleSyncConflictsAndReadiness(t *testing.T) {
	f := setup(t)
	s := f.e.Service.Access()
	a := f.create(t, "role-release-conflict", f.a.Account.ID)
	preset := f.preset(t, a.TenantID, "tenant_auditor")
	_, e := f.e.Pool.Exec(ctx, `UPDATE roles SET name='旧预置名称',name_normalized='旧预置名称' WHERE id=$1`, preset.ID)
	must(t, e)
	f.localRole(t, a.TenantID, preset.Name)
	report, e := s.SyncPreview(ctx)
	must(t, e)
	if len(report.Conflicts) != 1 {
		t.Fatal(report)
	}
	if e = s.Sync(ctx); e == nil {
		t.Fatal("sync overwrote reserved name collision")
	}
	if e = s.Check(ctx); e == nil {
		t.Fatal("mismatch ready")
	}
	_, e = f.e.Pool.Exec(ctx, `UPDATE authorization_state SET catalog_checksum='old-release' WHERE id=1`)
	must(t, e)
	_, e = s.AuthorizeScoped(ctx, f.a.Account.ID, access.OpTenantRoleList, a.TenantID)
	code(t, e, "unavailable")
}

func TestTenantRoleReauthenticationAndLockTimeoutAreAtomic(t *testing.T) {
	f := setup(t)
	s := f.e.Service.Access()
	a := f.create(t, "role-atomic", f.a.Account.ID)
	in := access.TenantRoleCreateInput{Name: "事务保护", Permissions: []access.Code{}, RequestID: f.uuid(t), Reason: "测试复验"}
	_, e := f.e.Pool.Exec(ctx, `UPDATE account_sessions SET reauthenticated_at=now()-interval '1 hour' WHERE account_id=$1`, f.a.Account.ID)
	must(t, e)
	_, e = s.CreateTenantRole(ctx, f.actor(f.a), a.TenantID, "", in, access.Meta{})
	code(t, e, "reauthentication_required")
	f.a = login(t, f.e, "Alice")
	tx, e := f.e.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT id FROM authorization_state WHERE id=1 FOR UPDATE`)
	must(t, e)
	_, e = s.CreateTenantRole(ctx, f.actor(f.a), a.TenantID, "", in, access.Meta{})
	code(t, e, "operation_busy")
	must(t, tx.Rollback(ctx))
	var count int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM roles WHERE owner_tenant_id=$1`, a.TenantID).Scan(&count))
	if count != 0 {
		t.Fatal("partial role after rejection")
	}
}
