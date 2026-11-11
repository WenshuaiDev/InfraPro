package tenancy_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/jackc/pgx/v5"
)

func (f fixture) lifecycleInput(t *testing.T, id, target string) tenancy.StatusInput {
	t.Helper()
	p, e := f.s.PreviewStatus(ctx, f.actor(f.p), id, target)
	must(t, e)
	notice := "公开停用说明"
	in := tenancy.StatusInput{Version: p.Version, Hash: p.Hash, Reason: "平台内部原因-不可泄露"}
	if target == "disabled" {
		in.Notice = &notice
	}
	return in
}
func (f fixture) lifecycle(t *testing.T, id, target string) tenancy.StatusResult {
	t.Helper()
	v, e := f.s.ChangeStatus(ctx, f.actor(f.p), id, target, f.lifecycleInput(t, id, target), access.Meta{})
	must(t, e)
	return v
}
func rawRow(t *testing.T, f fixture, query string, args ...any) string {
	t.Helper()
	var v string
	must(t, f.e.Pool.QueryRow(ctx, query, args...).Scan(&v))
	return v
}
func TestTenantLifecycleIsolationPreservationAndPrivacy(t *testing.T) {
	f := setup(t)
	a := f.create(t, "lifecycle-a", f.a.Account.ID)
	b := f.create(t, "lifecycle-b", f.a.Account.ID)
	before := rawRow(t, f, `SELECT jsonb_agg(to_jsonb(m) ORDER BY id)::text FROM tenant_memberships m`)
	bindings := rawRow(t, f, `SELECT jsonb_agg(to_jsonb(m) ORDER BY id)::text FROM tenant_member_roles m`)
	sessions := rawRow(t, f, `SELECT jsonb_agg(to_jsonb(m) ORDER BY id)::text FROM account_sessions m`)
	in := f.lifecycleInput(t, a.TenantID, "disabled")
	rev := rawRow(t, f, `SELECT revision::text FROM authorization_state`)
	p, e := f.s.PreviewStatus(ctx, f.actor(f.p), a.TenantID, "disabled")
	must(t, e)
	if p.Impact.ActiveMembers != 1 || p.Impact.EligibleMembers != 1 || !p.CanSubmit {
		t.Fatalf("bad preview %+v", p)
	}
	disabled, e := f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, access.Meta{})
	must(t, e)
	if disabled.Status != "disabled" || disabled.Version != "2" || disabled.Revision == rev {
		t.Fatal(disabled)
	}
	_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, access.Meta{})
	code(t, e, "version_conflict")
	in = f.lifecycleInput(t, a.TenantID, "disabled")
	_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, access.Meta{})
	code(t, e, "tenant_status_unchanged")
	// Exhaust every registered ordinary tenant operation, including read-only POST previews.
	for _, policy := range access.Policies() {
		if policy.Class != "tenant" {
			continue
		}
		t.Run(string(policy.Operation), func(t *testing.T) {
			_, e := f.e.Service.Access().AuthorizeScoped(ctx, f.a.Account.ID, policy.Operation, a.TenantID)
			code(t, e, "tenant_disabled")
		})
	}
	_, e = f.s.Context(ctx, f.actor(f.a), a.TenantID)
	code(t, e, "tenant_disabled")
	_, e = f.s.Get(ctx, f.actor(f.a), a.TenantID, false)
	code(t, e, "tenant_disabled")
	_, e = f.e.Service.Access().TenantRoles(ctx, f.actor(f.a), a.TenantID, access.Filter{})
	code(t, e, "tenant_disabled")
	_, e = f.s.Get(ctx, f.actor(f.b), a.TenantID, false)
	code(t, e, "tenant_not_found")
	_, e = f.s.Availability(ctx, f.actor(f.b), a.TenantID)
	code(t, e, "tenant_not_found")
	av, e := f.s.Availability(ctx, f.actor(f.a), a.TenantID)
	must(t, e)
	if av.CanEnter || av.Notice != "公开停用说明" {
		t.Fatal(av)
	}
	list, e := f.s.Memberships(ctx, f.actor(f.a), "", tenancy.Filter{})
	must(t, e)
	if len(list.Items) != 2 {
		t.Fatal(list)
	}
	for _, v := range list.Items {
		if v.ID == a.TenantID && (v.CanEnter || len(v.Roles) != 0) {
			t.Fatal(v)
		}
	}
	_, e = f.s.Get(ctx, f.actor(f.a), b.TenantID, false)
	must(t, e)
	_, e = f.e.Service.ReadProfile(ctx, f.a)
	must(t, e)
	_, e = f.s.Get(ctx, f.actor(f.p), a.TenantID, true)
	must(t, e)
	_, e = f.s.Get(ctx, f.actor(f.p), a.TenantID, false)
	code(t, e, "tenant_not_found")
	desc := "停用期治理"
	_, e = f.s.Update(ctx, f.actor(f.p), a.TenantID, tenancy.UpdateInput{Description: &desc, Version: disabled.Version, Reason: "平台治理修改"}, true, access.Meta{})
	must(t, e)
	// Actor/session verification must precede disclosure even through direct domain entry points.
	sentinel := errors.New("revoked actor")
	actor := f.actor(f.a)
	actor.Check = func(_ctx context.Context, _tx pgx.Tx, _recent bool) error { return sentinel }
	_, e = f.s.Get(ctx, actor, a.TenantID, false)
	if !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	_, e = f.e.Service.Access().TenantRoles(ctx, actor, a.TenantID, access.Filter{})
	if !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	f.lifecycle(t, a.TenantID, "active")
	if before != rawRow(t, f, `SELECT jsonb_agg(to_jsonb(m) ORDER BY id)::text FROM tenant_memberships m`) || bindings != rawRow(t, f, `SELECT jsonb_agg(to_jsonb(m) ORDER BY id)::text FROM tenant_member_roles m`) {
		t.Fatal("subordinate history changed")
	}
	if sessions != rawRow(t, f, `SELECT jsonb_agg(to_jsonb(m) ORDER BY id)::text FROM account_sessions m`) {
		t.Fatal("sessions changed")
	}
	events, e := f.s.Audit(ctx, f.actor(f.a), a.TenantID, tenancy.Filter{})
	must(t, e)
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "不可泄露") || strings.Contains(string(encoded), "status_change_reason") {
		t.Fatal("internal status metadata leaked")
	}
	for _, event := range events.Items {
		if event.Event == "tenant.disabled" || event.Event == "tenant.enabled" {
			if event.ActorID != "" || event.RequestID != "" {
				t.Fatal("platform identity leaked")
			}
		}
	}
	events, e = f.s.Audit(ctx, f.actor(f.a), a.TenantID, tenancy.Filter{Query: "不可泄露"})
	must(t, e)
	if len(events.Items) != 0 {
		t.Fatal("private search oracle")
	}
	full, e := f.e.Service.Access().AuditEvents(ctx, f.actor(f.p), access.Filter{TenantID: a.TenantID, Query: "不可泄露"})
	must(t, e)
	if len(full.Items) != 2 {
		t.Fatal("missing complete audit", len(full.Items))
	}
	if rawRow(t, f, `SELECT count(*)::text FROM account_security_events WHERE tenant_id=$1 AND event_type IN ('tenant.disabled','tenant.enabled')`, a.TenantID) != "2" {
		t.Fatal("duplicate events")
	}
	page, e := f.s.List(ctx, f.actor(f.p), tenancy.Filter{Status: "active", Query: "lifecycle", Limit: 1})
	must(t, e)
	if len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal(page)
	}
	_, e = f.s.List(ctx, f.actor(f.p), tenancy.Filter{Status: "invalid"})
	code(t, e, "validation_failed")
}

func TestTenantLifecyclePreviewRollbackAndValidation(t *testing.T) {
	f := setup(t)
	a := f.create(t, "lifecycle-conflicts", f.a.Account.ID)
	in := f.lifecycleInput(t, a.TenantID, "disabled")
	for _, reason := range []string{" \u3000\n", strings.Repeat("桥", 501)} {
		bad := in
		bad.Reason = reason
		_, e := f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", bad, access.Meta{})
		code(t, e, "validation_failed")
	}
	for _, notice := range []string{" \u3000\n", strings.Repeat("桥", 501)} {
		bad := in
		bad.Notice = &notice
		_, e := f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", bad, access.Meta{})
		code(t, e, "validation_failed")
	}
	// Profile version and graph revisions are independently protected.
	name := "修改后名称"
	_, e := f.s.Update(ctx, f.actor(f.p), a.TenantID, tenancy.UpdateInput{Name: &name, Version: in.Version, Reason: "资料修改"}, true, access.Meta{})
	must(t, e)
	_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, access.Meta{})
	code(t, e, "version_conflict")
	in = f.lifecycleInput(t, a.TenantID, "disabled")
	f.create(t, "unrelated-revision", f.b.Account.ID)
	_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, access.Meta{})
	code(t, e, "preview_changed")
	in = f.lifecycleInput(t, a.TenantID, "disabled")
	before := rawRow(t, f, `SELECT to_jsonb(t)::text FROM tenants t WHERE id=$1`, a.TenantID)
	rev := rawRow(t, f, `SELECT revision::text FROM authorization_state`)
	_, e = f.e.Pool.Exec(ctx, `CREATE FUNCTION reject_lifecycle_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='tenant.disabled' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_lifecycle BEFORE INSERT ON account_security_events FOR EACH ROW EXECUTE FUNCTION reject_lifecycle_audit()`)
	must(t, e)
	_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, access.Meta{})
	if e == nil {
		t.Fatal("expected audit rollback")
	}
	if before != rawRow(t, f, `SELECT to_jsonb(t)::text FROM tenants t WHERE id=$1`, a.TenantID) || rev != rawRow(t, f, `SELECT revision::text FROM authorization_state`) {
		t.Fatal("partial status commit")
	}
	_, e = f.e.Pool.Exec(ctx, `DROP TRIGGER reject_lifecycle ON account_security_events`)
	must(t, e)
	// Two old confirmations produce exactly one commit.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, e := f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, access.Meta{})
			results <- e
		})
	}
	wg.Wait()
	close(results)
	ok := 0
	for e := range results {
		if e == nil {
			ok++
		} else {
			code(t, e, "version_conflict")
		}
	}
	if ok != 1 {
		t.Fatal(ok)
	}
	// DB metadata and the existing administrator structure stay enforced while disabled.
	for _, sql := range []string{`UPDATE tenants SET status='unknown' WHERE id=$1`, `UPDATE tenants SET status_notice='' WHERE id=$1`, `UPDATE tenants SET status_change_reason=NULL WHERE id=$1`, `UPDATE tenant_memberships SET status='suspended' WHERE id=(SELECT administrator_membership_id FROM tenants WHERE id=$1)`} {
		_, e = f.e.Pool.Exec(ctx, sql, a.TenantID)
		if e == nil {
			t.Fatal("constraint accepted", sql)
		}
	}
}

func TestTenantLifecycleInvitationRetentionExpiryAndTransfer(t *testing.T) {
	for _, scenario := range []string{"retain", "expire", "handover", "target_disabled"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t)
			a := f.create(t, "lifecycle-invite-"+scenario, f.a.Account.ID)
			l := f.invite(t, a.TenantID, f.a, f.b)
			before := rawRow(t, f, `SELECT to_jsonb(i)::text FROM tenant_invitations i WHERE id=$1`, l.Invitation.ID)
			disabled := f.lifecycle(t, a.TenantID, "disabled")
			if before != rawRow(t, f, `SELECT to_jsonb(i)::text FROM tenant_invitations i WHERE id=$1`, l.Invitation.ID) {
				t.Fatal("disable consumed invitation")
			}
			ins, e := f.s.InspectInvitation(ctx, f.actor(f.b), tokenOf(l.Link), access.Meta{})
			must(t, e)
			if ins.CanAccept || ins.BlockedReason != "tenant_disabled" || ins.Preview != nil || ins.Availability == nil || ins.Availability.Name == "" {
				t.Fatal(ins)
			}
			_, e = f.s.InspectInvitation(ctx, f.actor(f.p), tokenOf(l.Link), access.Meta{})
			code(t, e, "tenant_invitation_unavailable")
			_, e = f.s.AcceptInvitation(ctx, f.actor(f.b), tenancy.AcceptanceInput{Token: tokenOf(l.Link), Version: l.Invitation.Version}, access.Meta{})
			code(t, e, "tenant_disabled")
			if before != rawRow(t, f, `SELECT to_jsonb(i)::text FROM tenant_invitations i WHERE id=$1`, l.Invitation.ID) {
				t.Fatal("accept consumed disabled invitation")
			}
			switch scenario {
			case "expire":
				in := f.lifecycleInput(t, a.TenantID, "active")
				_, e = f.e.Pool.Exec(ctx, `UPDATE tenant_invitations SET created_at=clock_timestamp()-interval '2 days',expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, l.Invitation.ID)
				must(t, e)
				_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "active", in, access.Meta{})
				code(t, e, "preview_changed")
			case "handover":
				_, e = f.s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: disabled.Version, AdministratorID: f.p.Account.ID, Reason: "停用期间交接"}, access.Meta{})
				must(t, e)
				v, e := f.s.Get(ctx, f.actor(f.p), a.TenantID, true)
				must(t, e)
				if v.Status != "disabled" {
					t.Fatal("handover implicitly enabled")
				}
			case "target_disabled":
				must(t, f.status(t, f.b, false))
				must(t, f.status(t, f.b, true))
				f.b = login(t, f.e, "Bob")
			}
			f.lifecycle(t, a.TenantID, "active")
			ins, e = f.s.InspectInvitation(ctx, f.actor(f.b), tokenOf(l.Link), access.Meta{})
			must(t, e)
			switch scenario {
			case "retain":
				if !ins.CanAccept || ins.Version != l.Invitation.Version {
					t.Fatal(ins)
				}
				m := f.accept(t, l, f.b)
				f.lifecycle(t, a.TenantID, "disabled")
				ins, e = f.s.InspectInvitation(ctx, f.actor(f.b), tokenOf(l.Link), access.Meta{})
				must(t, e)
				if ins.Status != "accepted" || ins.Preview == nil || ins.TenantStatus != "disabled" {
					t.Fatal(ins)
				}
				f.lifecycle(t, a.TenantID, "active")
				c, e := f.s.Context(ctx, f.actor(f.b), a.TenantID)
				must(t, e)
				if c.MembershipID != m.MembershipID {
					t.Fatal("identity changed")
				}
			case "expire":
				if ins.Status != "expired" || ins.CanAccept {
					t.Fatal(ins)
				}
			default:
				if ins.Status != "invalidated" || ins.CanAccept {
					t.Fatal(ins)
				}
			}
		})
	}
}
func TestTenantLifecycleRestoreAdministratorAndPreservedRestrictions(t *testing.T) {
	f := setup(t)
	a := f.create(t, "lifecycle-restore", f.a.Account.ID)
	m := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
	_, e := f.s.ChangeMember(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberSuspend, tenancy.MemberInput{Version: m.Version, Reason: "保留暂停"}, access.Meta{})
	must(t, e)
	disabled := f.lifecycle(t, a.TenantID, "disabled")
	code(t, f.status(t, f.a, false), "tenant_admin_transfer_required")
	// Isolated fixture simulates an emergency-unavailable administrator, not a normal account operation.
	_, e = f.e.Pool.Exec(ctx, `UPDATE accounts SET status='disabled' WHERE id=$1`, f.a.Account.ID)
	must(t, e)
	p, e := f.s.PreviewStatus(ctx, f.actor(f.p), a.TenantID, "active")
	must(t, e)
	if p.CanSubmit || p.AdministratorAvailable {
		t.Fatal(p)
	}
	_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "active", f.lifecycleInput(t, a.TenantID, "active"), access.Meta{})
	code(t, e, "tenant_restore_blocked")
	_, e = f.s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: disabled.Version, AdministratorID: f.b.Account.ID, Reason: "不能隐式恢复暂停成员"}, access.Meta{})
	code(t, e, "administrator_membership_suspended")
	_, e = f.s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: disabled.Version, AdministratorID: f.p.Account.ID, Reason: "合法修复管理员"}, access.Meta{})
	must(t, e)
	f.lifecycle(t, a.TenantID, "active")
	_, e = f.s.Context(ctx, f.actor(f.b), a.TenantID)
	code(t, e, "tenant_not_found")
	_, e = f.s.Availability(ctx, f.actor(f.b), a.TenantID)
	code(t, e, "tenant_not_found")
	if rawRow(t, f, `SELECT status FROM tenant_memberships WHERE id=$1`, m.MembershipID) != "suspended" {
		t.Fatal("suspended member revived")
	}
	if rawRow(t, f, `SELECT status FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, a.TenantID, f.a.Account.ID) != "ended" {
		t.Fatal("old admin revived")
	}
}
func TestTenantLifecycleDisabledCatalogLossIsPermanent(t *testing.T) {
	for _, loss := range []string{"create", "fixed_grant"} {
		t.Run(loss, func(t *testing.T) {
			f := setup(t)
			a := f.create(t, "lifecycle-sync-"+loss, f.a.Account.ID)
			m := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
			preset := f.preset(t, a.TenantID, "tenant_profile_maintainer")
			if loss == "fixed_grant" {
				preset = f.preset(t, a.TenantID, "tenant_invitation_coordinator")
			}
			f.grant(t, a.TenantID, m.MembershipID, preset.ID)
			if loss == "create" {
				_, e := f.e.Pool.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,'tenant')`, preset.ID, access.TenantInvitationCreate)
				must(t, e)
			}
			// Simulate old fixed member role being empty; publication adds profile.read while inviter loses coverage.
			if loss == "fixed_grant" {
				_, e := f.e.Pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND permission_code=$2`, preset.ID, access.TenantProfileRead)
				must(t, e)
				_, e = f.e.Pool.Exec(ctx, `DELETE FROM role_permissions WHERE role_id IN(SELECT id FROM roles WHERE builtin_key=$1)`, access.TenantMemberKey)
				must(t, e)
				// Keep the inviter's missing profile permission in a local role through publication.
				r := f.localRole(t, a.TenantID, "仅邀请创建", access.TenantInvitationCreate)
				f.grant(t, a.TenantID, m.MembershipID, r.ID)
			}
			l := f.invite(t, a.TenantID, f.b, f.p)
			f.lifecycle(t, a.TenantID, "disabled")
			report, e := f.e.Service.Access().SyncPreview(ctx)
			must(t, e)
			if len(report.Tenants) != 1 || report.Tenants[0].InvalidatedInvitations != 1 {
				t.Fatal(report)
			}
			must(t, f.e.Service.Access().Sync(ctx))
			rev := rawRow(t, f, `SELECT revision::text FROM authorization_state`)
			must(t, f.e.Service.Access().Sync(ctx))
			if rev != rawRow(t, f, `SELECT revision::text FROM authorization_state`) {
				t.Fatal("noop sync wrote")
			}
			if rawRow(t, f, `SELECT status FROM tenant_invitations WHERE id=$1`, l.Invitation.ID) != "invalidated" {
				t.Fatal("lost grant was hidden by disabled tenant")
			}
			f.lifecycle(t, a.TenantID, "active")
			f.grant(t, a.TenantID, m.MembershipID, f.preset(t, a.TenantID, "tenant_invitation_coordinator").ID)
			ins, e := f.s.InspectInvitation(ctx, f.actor(f.p), tokenOf(l.Link), access.Meta{})
			must(t, e)
			if ins.CanAccept || ins.Status != "invalidated" {
				t.Fatal(ins)
			}
		})
	}
}
func TestTenantLifecycleAcceptSerialization(t *testing.T) {
	for _, first := range []string{"disable", "accept"} {
		t.Run(first, func(t *testing.T) {
			f := setup(t)
			a := f.create(t, "lifecycle-race-"+first, f.a.Account.ID)
			l := f.invite(t, a.TenantID, f.a, f.b)
			in := f.lifecycleInput(t, a.TenantID, "disabled")
			locked := make(chan struct{})
			release := make(chan struct{})
			done := make(chan error, 1)
			actor := f.actor(f.p)
			if first == "accept" {
				actor = f.actor(f.b)
			}
			original := actor.Check
			actor.Check = func(c context.Context, tx pgx.Tx, recent bool) error {
				if e := original(c, tx, recent); e != nil {
					return e
				}
				close(locked)
				<-release
				return nil
			}
			go func() {
				var e error
				if first == "disable" {
					_, e = f.s.ChangeStatus(ctx, actor, a.TenantID, "disabled", in, access.Meta{})
				} else {
					_, e = f.s.AcceptInvitation(ctx, actor, tenancy.AcceptanceInput{Token: tokenOf(l.Link), Version: l.Invitation.Version}, access.Meta{})
				}
				done <- e
			}()
			<-locked
			second := make(chan error, 1)
			go func() {
				var e error
				if first == "disable" {
					_, e = f.s.AcceptInvitation(ctx, f.actor(f.b), tenancy.AcceptanceInput{Token: tokenOf(l.Link), Version: l.Invitation.Version}, access.Meta{})
				} else {
					_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, access.Meta{})
				}
				second <- e
			}()
			close(release)
			must(t, <-done)
			e := <-second
			if first == "disable" {
				code(t, e, "tenant_disabled")
				if rawRow(t, f, `SELECT count(*)::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, a.TenantID, f.b.Account.ID) != "0" {
					t.Fatal("partial member")
				}
			} else {
				code(t, e, "preview_changed")
				f.lifecycle(t, a.TenantID, "disabled")
				if rawRow(t, f, `SELECT status FROM tenant_invitations WHERE id=$1`, l.Invitation.ID) != "accepted" {
					t.Fatal("accept lost")
				}
			}
		})
	}
}

func TestTenantLifecycleIndependentPermissionsAndTransactionGuards(t *testing.T) {
	f := setup(t)
	a := f.create(t, "lifecycle-permissions", f.a.Account.ID)
	s := f.e.Service.Access()
	m := access.Meta{Reason: "独立生命周期权限"}
	r, e := s.CreateRole(ctx, f.actor(f.p), "只授生命周期", "", m)
	must(t, e)
	target, e := s.Account(ctx, f.actor(f.p), f.b.Account.ID)
	must(t, e)
	_, e = s.AssignRoles(ctx, f.actor(f.p), target.ID, target.Version, []string{r.ID}, m)
	must(t, e)
	set := func(codes ...access.Code) {
		t.Helper()
		var e error
		r, e = s.ChangeRole(ctx, f.actor(f.p), r.ID, access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: r.Version, Permissions: codes}, m)
		must(t, e)
	}
	set(access.TenantDisable)
	_, e = f.s.PreviewStatus(ctx, f.actor(f.b), a.TenantID, "disabled")
	code(t, e, "permission_denied")
	set(access.TenantRead)
	_, e = f.s.PreviewStatus(ctx, f.actor(f.b), a.TenantID, "disabled")
	code(t, e, "permission_denied")
	set(access.TenantRead, access.TenantDisable)
	_, e = f.s.PreviewStatus(ctx, f.actor(f.b), a.TenantID, "active")
	code(t, e, "permission_denied")
	_, e = f.s.ChangeStatus(ctx, f.actor(f.b), a.TenantID, "disabled", f.lifecycleInput(t, a.TenantID, "disabled"), m)
	must(t, e)
	set(access.TenantRead, access.TenantEnable)
	_, e = f.s.PreviewStatus(ctx, f.actor(f.b), a.TenantID, "disabled")
	code(t, e, "permission_denied")
	_, e = f.s.ChangeStatus(ctx, f.actor(f.b), a.TenantID, "active", f.lifecycleInput(t, a.TenantID, "active"), m)
	must(t, e)
	_, e = f.e.Pool.Exec(ctx, `UPDATE account_sessions SET reauthenticated_at=now()-interval '1 hour' WHERE account_id=$1`, f.p.Account.ID)
	must(t, e)
	in := f.lifecycleInput(t, a.TenantID, "disabled") // previews do not require recent verification
	_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, m)
	code(t, e, "reauthentication_required")
	f.p = login(t, f.e, "Platform")
	tx, e := f.e.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT id FROM authorization_state WHERE id=1 FOR UPDATE`)
	must(t, e)
	_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, m)
	code(t, e, "operation_busy")
	must(t, tx.Rollback(ctx))
	if rawRow(t, f, `SELECT status FROM tenants WHERE id=$1`, a.TenantID) != "active" {
		t.Fatal("failed guard changed status")
	}
	_, e = f.e.Pool.Exec(ctx, `UPDATE authorization_state SET catalog_checksum='unsynced' WHERE id=1`)
	must(t, e)
	_, e = f.s.ChangeStatus(ctx, f.actor(f.p), a.TenantID, "disabled", in, m)
	code(t, e, "unavailable")
}

func TestTenantLifecycleDoesNotReviveRestrictedGrants(t *testing.T) {
	for _, restriction := range []string{"role_disabled", "binding_revoked", "member_ended", "account_disabled"} {
		t.Run(restriction, func(t *testing.T) {
			f := setup(t)
			a := f.create(t, "preserve-"+restriction, f.a.Account.ID)
			m := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
			switch restriction {
			case "role_disabled":
				r := f.localRole(t, a.TenantID, "保留停用角色", access.TenantProfileRead)
				f.grant(t, a.TenantID, m.MembershipID, r.ID)
				f.changeRole(t, a.TenantID, r, access.OpTenantRoleStatus, access.TenantChange{Status: "disabled"})
			case "binding_revoked":
				f.grant(t, a.TenantID, m.MembershipID)
			case "member_ended":
				_, e := f.s.ChangeMember(ctx, f.actor(f.a), a.TenantID, m.MembershipID, access.OpTenantMemberRemove, tenancy.MemberInput{Version: m.Version, Reason: "保留结束"}, access.Meta{})
				must(t, e)
			case "account_disabled":
				must(t, f.status(t, f.b, false))
			}
			before, e := f.e.Service.Access().TenantMemberAuthorization(ctx, f.actor(f.a), a.TenantID, m.MembershipID)
			must(t, e)
			f.lifecycle(t, a.TenantID, "disabled")
			f.lifecycle(t, a.TenantID, "active")
			after, e := f.e.Service.Access().TenantMemberAuthorization(ctx, f.actor(f.a), a.TenantID, m.MembershipID)
			must(t, e)
			if len(after.Permissions) != 0 || after.MembershipID != before.MembershipID || after.MembershipStatus != before.MembershipStatus || after.AccountStatus != before.AccountStatus {
				t.Fatal(after)
			}
			before.Revision = ""
			after.Revision = ""
			b, _ := json.Marshal(before)
			c, _ := json.Marshal(after)
			if string(b) != string(c) {
				t.Fatal("lifecycle changed preserved grants")
			}
		})
	}
}
