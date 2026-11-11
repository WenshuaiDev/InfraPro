package tenancy_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
)

func tokenOf(link string) string {
	u, _ := url.Parse(link)
	q, _ := url.ParseQuery(u.Fragment)
	return q.Get("token")
}
func (f fixture) uuid(t *testing.T) string {
	t.Helper()
	var id string
	must(t, f.e.Pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id))
	return id
}
func (f fixture) invite(t *testing.T, tenant string, inviter, target identity.Principal) tenancy.InvitationCreated {
	t.Helper()
	v, e := f.s.CreateInvitation(ctx, f.actor(inviter), tenant, tenancy.InvitationCreateInput{TargetID: target.Account.ID, RequestID: f.uuid(t), Reason: "join test"}, access.Meta{})
	must(t, e)
	return v
}
func (f fixture) accept(t *testing.T, link tenancy.InvitationCreated, p identity.Principal) tenancy.AcceptanceResult {
	t.Helper()
	v, e := f.s.AcceptInvitation(ctx, f.actor(p), tenancy.AcceptanceInput{Token: tokenOf(link.Link), Version: link.Invitation.Version}, access.Meta{})
	must(t, e)
	return v
}
func TestMembershipLifecycleAndMinimalAuthority(t *testing.T) {
	f := setup(t)
	a := f.create(t, "members-a", f.a.Account.ID)
	b := f.create(t, "members-b", f.a.Account.ID)
	target, e := f.e.Service.Access().Account(ctx, f.actor(f.p), f.b.Account.ID)
	must(t, e)
	_, e = f.e.Service.Access().AssignRoles(ctx, f.actor(f.p), target.ID, target.Version, []string{}, access.Meta{Reason: "tenant-only"})
	must(t, e)
	l := f.invite(t, a.TenantID, f.a, f.b)
	my, e := f.s.Memberships(ctx, f.actor(f.b), "", tenancy.Filter{})
	must(t, e)
	if len(my.Items) != 0 {
		t.Fatal("pending is not membership")
	}
	_, e = f.s.InspectInvitation(ctx, f.actor(f.p), tokenOf(l.Link), access.Meta{})
	code(t, e, "tenant_invitation_unavailable")
	i, e := f.s.InspectInvitation(ctx, f.actor(f.b), tokenOf(l.Link), access.Meta{})
	must(t, e)
	if !i.CanAccept || i.Preview.Account.ID != f.b.Account.ID {
		t.Fatal(i)
	}
	x := f.accept(t, l, f.b)
	f.accept(t, f.invite(t, b.TenantID, f.a, f.b), f.b)
	c, e := f.s.Context(ctx, f.actor(f.b), a.TenantID)
	must(t, e)
	if len(c.Permissions) != 1 || c.Permissions[0] != access.TenantProfileRead {
		t.Fatal(c)
	}
	for _, p := range []identity.Principal{f.p, f.b} {
		_, e = f.s.Members(ctx, f.actor(p), a.TenantID, tenancy.Filter{})
		if p.Account.ID == f.p.Account.ID {
			code(t, e, "tenant_not_found")
		} else {
			code(t, e, "permission_denied")
		}
	}
	_, e = f.s.Audit(ctx, f.actor(f.b), a.TenantID, tenancy.Filter{})
	code(t, e, "permission_denied")
	_, e = f.s.Invitations(ctx, f.actor(f.b), a.TenantID, tenancy.Filter{})
	code(t, e, "permission_denied")
	change := func(op access.Operation, version string) (tenancy.MembershipResult, error) {
		return f.s.ChangeMember(ctx, f.actor(f.a), a.TenantID, x.MembershipID, op, tenancy.MemberInput{Version: version, Reason: "lifecycle"}, access.Meta{})
	}
	suspended, e := change(access.OpTenantMemberSuspend, x.Version)
	must(t, e)
	_, e = f.s.Context(ctx, f.actor(f.b), a.TenantID)
	code(t, e, "tenant_not_found")
	_, e = f.s.Context(ctx, f.actor(f.b), b.TenantID)
	must(t, e)
	_, e = f.s.CreateInvitation(ctx, f.actor(f.a), a.TenantID, tenancy.InvitationCreateInput{TargetID: f.b.Account.ID, RequestID: f.uuid(t), Reason: "repeat"}, access.Meta{})
	code(t, e, "member_suspended")
	_, e = change(access.OpTenantMemberSuspend, suspended.Version)
	code(t, e, "member_state_conflict")
	resumed, e := change(access.OpTenantMemberResume, suspended.Version)
	must(t, e)
	if resumed.MembershipID != x.MembershipID {
		t.Fatal("new relation on resume")
	}
	ended, e := change(access.OpTenantMemberRemove, resumed.Version)
	must(t, e)
	_, e = change(access.OpTenantMemberResume, ended.Version)
	code(t, e, "member_state_conflict")
	old, e := f.s.Member(ctx, f.actor(f.a), a.TenantID, x.MembershipID)
	must(t, e)
	if old.Roles[0].RevokedAt == nil || old.EndedAt == nil {
		t.Fatal("unrevoked ended member")
	}
	inspection, e := f.s.InspectInvitation(ctx, f.actor(f.b), tokenOf(l.Link), access.Meta{})
	must(t, e)
	if inspection.MembershipStatus != "ended" || inspection.CanAccept {
		t.Fatal(inspection)
	}
	_, e = f.s.AcceptInvitation(ctx, f.actor(f.b), tenancy.AcceptanceInput{Token: tokenOf(l.Link), Version: "1"}, access.Meta{})
	code(t, e, "invitation_already_accepted")
	y := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
	if y.MembershipID == x.MembershipID {
		t.Fatal("historical relation reused")
	}
	current, e := f.s.Member(ctx, f.actor(f.a), a.TenantID, y.MembershipID)
	must(t, e)
	f.transfer(t, a, f.b.Account.ID)
	promoted, e := f.s.Member(ctx, f.actor(f.b), a.TenantID, y.MembershipID)
	must(t, e)
	if promoted.JoinedAt != current.JoinedAt || !promoted.Administrator || len(promoted.Roles) != 2 {
		t.Fatal(promoted)
	}
	_, e = f.s.ChangeMember(ctx, f.actor(f.b), a.TenantID, y.MembershipID, access.OpTenantMemberRemove, tenancy.MemberInput{Version: promoted.Version, Reason: "protect"}, access.Meta{})
	code(t, e, "tenant_admin_transfer_required")
	_, e = f.s.Member(ctx, f.actor(f.a), b.TenantID, y.MembershipID)
	code(t, e, "member_not_found")
	history, e := f.s.Members(ctx, f.actor(f.b), a.TenantID, tenancy.Filter{Status: "all", Query: "Bob", Limit: 1})
	must(t, e)
	if len(history.Items) != 1 || history.NextCursor == "" {
		t.Fatal(history)
	}
	next, e := f.s.Members(ctx, f.actor(f.b), a.TenantID, tenancy.Filter{Status: "all", Query: "Bob", Limit: 1, Cursor: history.NextCursor})
	must(t, e)
	if len(next.Items) != 1 || next.Items[0].ID == history.Items[0].ID {
		t.Fatal(next)
	}
	events, e := f.s.Audit(ctx, f.actor(f.b), a.TenantID, tenancy.Filter{ObjectType: "tenant_member", ObjectID: x.MembershipID})
	must(t, e)
	if len(events.Items) != 4 {
		t.Fatal(events)
	}
	must(t, f.e.Service.Access().Check(ctx))
}
func TestTenantInvitationIntentExpiryAndReplacement(t *testing.T) {
	f := setup(t)
	a := f.create(t, "invitation-intents", f.a.Account.ID)
	for _, username := range []string{"", "Bo", "!bad"} {
		_, e := f.s.ResolveMember(ctx, f.actor(f.a), a.TenantID, username)
		code(t, e, "validation_failed")
	}
	target, e := f.s.ResolveMember(ctx, f.actor(f.a), a.TenantID, "bOB")
	must(t, e)
	if target.ID != f.b.Account.ID {
		t.Fatal(target)
	}
	in := tenancy.InvitationCreateInput{TargetID: target.ID, RequestID: f.uuid(t), Reason: "  invitation  "}
	l, e := f.s.CreateInvitation(ctx, f.actor(f.a), a.TenantID, in, access.Meta{})
	must(t, e)
	if l.Invitation.ExpiresAt.Sub(l.Invitation.CreatedAt) != 24*time.Hour {
		t.Fatal("ttl")
	}
	r, e := f.s.CreateInvitation(ctx, f.actor(f.a), a.TenantID, in, access.Meta{})
	must(t, e)
	if !r.Replayed || !r.LinkUnavailable || r.Link != "" || r.Invitation.ID != l.Invitation.ID {
		t.Fatal("replay exposed link")
	}
	in.Reason = "different"
	_, e = f.s.CreateInvitation(ctx, f.actor(f.a), a.TenantID, in, access.Meta{})
	code(t, e, "idempotency_conflict")
	in.RequestID = f.uuid(t)
	_, e = f.s.CreateInvitation(ctx, f.actor(f.a), a.TenantID, in, access.Meta{})
	code(t, e, "invitation_pending_exists")
	re := tenancy.InvitationRegenerateInput{Version: l.Invitation.Version, Reason: "replace", RequestID: f.uuid(t)}
	r, e = f.s.RegenerateInvitation(ctx, f.actor(f.a), a.TenantID, l.Invitation.ID, re, access.Meta{})
	must(t, e)
	_, e = f.s.AcceptInvitation(ctx, f.actor(f.b), tenancy.AcceptanceInput{Token: tokenOf(l.Link), Version: "1"}, access.Meta{})
	code(t, e, "invitation_not_pending")
	old, e := f.s.Invitation(ctx, f.actor(f.a), a.TenantID, l.Invitation.ID)
	must(t, e)
	if old.Status != "replaced" || old.ReplacedByID != r.Invitation.ID {
		t.Fatal(old)
	}
	replayed, e := f.s.RegenerateInvitation(ctx, f.actor(f.a), a.TenantID, l.Invitation.ID, re, access.Meta{})
	must(t, e)
	if replayed.Link != "" || !replayed.Replayed {
		t.Fatal("regenerate replay")
	}
	v, e := f.s.RevokeInvitation(ctx, f.actor(f.a), a.TenantID, r.Invitation.ID, tenancy.MemberInput{Version: "1", Reason: "revoke"}, access.Meta{})
	must(t, e)
	r, e = f.s.RegenerateInvitation(ctx, f.actor(f.a), a.TenantID, v.ID, tenancy.InvitationRegenerateInput{Version: v.Version, Reason: "replace revoked", RequestID: f.uuid(t)}, access.Meta{})
	must(t, e)
	old, e = f.s.Invitation(ctx, f.actor(f.a), a.TenantID, v.ID)
	must(t, e)
	if old.Status != "revoked" {
		t.Fatal("terminal overwritten")
	}
	_, e = f.e.Pool.Exec(ctx, `UPDATE tenant_invitations SET created_at=clock_timestamp()-interval '2 days',expires_at=clock_timestamp()-interval '1 day' WHERE id=$1`, r.Invitation.ID)
	must(t, e)
	old, e = f.s.Invitation(ctx, f.actor(f.a), a.TenantID, r.Invitation.ID)
	must(t, e)
	if old.Status != "expired" || old.Version != "1" {
		t.Fatal(old)
	}
	_, e = f.s.RevokeInvitation(ctx, f.actor(f.a), a.TenantID, old.ID, tenancy.MemberInput{Version: "1", Reason: "late"}, access.Meta{})
	code(t, e, "invitation_not_pending")
	r, e = f.s.RegenerateInvitation(ctx, f.actor(f.a), a.TenantID, old.ID, tenancy.InvitationRegenerateInput{Version: "1", Reason: "expired", RequestID: f.uuid(t)}, access.Meta{})
	must(t, e)
	f.accept(t, r, f.b)
	must(t, f.e.Service.Access().Check(ctx))
}
func TestTenantInvitationRevocationCoupledToIdentity(t *testing.T) {
	for _, cause := range []string{"target-disable", "inviter-disable", "reset", "restore", "handover", "appointed"} {
		t.Run(cause, func(t *testing.T) {
			f := setup(t)
			a := f.create(t, "coupled", f.a.Account.ID)
			l := f.invite(t, a.TenantID, f.a, f.b)
			repo := identity.NewRepository(f.e.Pool)
			switch cause {
			case "target-disable":
				must(t, repo.SetStatus(ctx, "Bob", false, false, identity.Meta{Reason: "test"}))
				must(t, repo.SetStatus(ctx, "Bob", true, false, identity.Meta{Reason: "restore"}))
				f.b = login(t, f.e, "Bob")
			case "inviter-disable":
				must(t, repo.SetStatus(ctx, "Alice", false, true, identity.Meta{Reason: "test"}))
				must(t, repo.SetStatus(ctx, "Alice", true, false, identity.Meta{Reason: "restore"}))
				f.a = login(t, f.e, "Alice")
			case "reset":
				_, e := repo.ResetLink(ctx, "Alice", "test-reset-hash", time.Hour, identity.Meta{Reason: "test"})
				must(t, e)
				hash, e := identity.HashPassword(password)
				must(t, e)
				must(t, repo.CompleteReset(ctx, "test-reset-hash", hash, identity.Meta{}))
				f.a = login(t, f.e, "Alice")
			case "restore":
				must(t, repo.RevokeAfterRestore(ctx, identity.Meta{Reason: "restore"}))
				f.a = login(t, f.e, "Alice")
				f.b = login(t, f.e, "Bob")
			case "handover":
				f.transfer(t, a, f.p.Account.ID)
			case "appointed":
				f.transfer(t, a, f.b.Account.ID)
			}
			v, e := f.s.InspectInvitation(ctx, f.actor(f.b), tokenOf(l.Link), access.Meta{})
			must(t, e)
			if v.Status != "invalidated" || v.CanAccept {
				t.Fatal(v)
			}
			_, e = f.s.AcceptInvitation(ctx, f.actor(f.b), tenancy.AcceptanceInput{Token: tokenOf(l.Link), Version: "1"}, access.Meta{})
			code(t, e, "invitation_not_pending")
			must(t, f.e.Service.Access().Check(ctx))
		})
	}
}
func TestTenantAcceptRollbackAndConstraints(t *testing.T) {
	f := setup(t)
	a := f.create(t, "atomic", f.a.Account.ID)
	l := f.invite(t, a.TenantID, f.a, f.b)
	for _, table := range []string{"tenant_memberships", "tenant_member_roles", "tenant_invitations", "account_security_events"} {
		t.Run(table, func(t *testing.T) {
			var rev string
			must(t, f.e.Pool.QueryRow(ctx, `SELECT revision::text FROM authorization_state`).Scan(&rev))
			event := "INSERT"
			if table == "tenant_invitations" {
				event = "UPDATE"
			}
			_, e := f.e.Pool.Exec(ctx, `CREATE FUNCTION test_membership_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$; CREATE TRIGGER test_membership_fail BEFORE `+event+` ON `+table+` FOR EACH ROW EXECUTE FUNCTION test_membership_fail()`)
			must(t, e)
			_, e = f.s.AcceptInvitation(ctx, f.actor(f.b), tenancy.AcceptanceInput{Token: tokenOf(l.Link), Version: "1"}, access.Meta{})
			code(t, e, "internal_error")
			_, e = f.e.Pool.Exec(ctx, `DROP TRIGGER test_membership_fail ON `+table+`; DROP FUNCTION test_membership_fail()`)
			must(t, e)
			var n int
			var after string
			must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, a.TenantID, f.b.Account.ID).Scan(&n))
			must(t, f.e.Pool.QueryRow(ctx, `SELECT revision::text FROM authorization_state`).Scan(&after))
			if n != 0 || rev != after {
				t.Fatal("partial transaction")
			}
			v, e := f.s.Invitation(ctx, f.actor(f.a), a.TenantID, l.Invitation.ID)
			must(t, e)
			if v.Status != "pending" || v.Version != "1" {
				t.Fatal(v)
			}
		})
	}
	x := f.accept(t, l, f.b)
	for _, query := range []string{`UPDATE tenant_memberships SET status='suspended' WHERE id=(SELECT administrator_membership_id FROM tenants LIMIT 1)`, `UPDATE tenant_member_roles SET role_id=(SELECT id FROM roles WHERE builtin_key='tenant_admin') WHERE membership_id='` + x.MembershipID + `'`, `INSERT INTO tenant_memberships(tenant_id,account_id,joined_by,status_changed_by,status) SELECT tenant_id,account_id,joined_by,status_changed_by,'suspended' FROM tenant_memberships WHERE id='` + x.MembershipID + `'`} {
		_, e := f.e.Pool.Exec(ctx, query)
		if e == nil {
			t.Fatal("invalid structure committed")
		}
	}
	must(t, f.e.Service.Access().Check(ctx))
}
func TestTenantControlledRaceOrders(t *testing.T) {
	for _, race := range []string{"revoke", "replace", "handover", "disable", "appointed", "accept"} {
		for _, acceptFirst := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/accept-first-%v", race, acceptFirst), func(t *testing.T) {
				f := setup(t)
				a := f.create(t, "race-members", f.a.Account.ID)
				l := f.invite(t, a.TenantID, f.a, f.b)
				accept := func() error {
					_, e := f.s.AcceptInvitation(ctx, f.actor(f.b), tenancy.AcceptanceInput{Token: tokenOf(l.Link), Version: "1"}, access.Meta{})
					return e
				}
				requestID := f.uuid(t)
				other := func() error {
					switch race {
					case "revoke":
						_, e := f.s.RevokeInvitation(ctx, f.actor(f.a), a.TenantID, l.Invitation.ID, tenancy.MemberInput{Version: "1", Reason: "race"}, access.Meta{})
						return e
					case "replace":
						_, e := f.s.RegenerateInvitation(ctx, f.actor(f.a), a.TenantID, l.Invitation.ID, tenancy.InvitationRegenerateInput{Version: "1", Reason: "race", RequestID: requestID}, access.Meta{})
						return e
					case "handover", "appointed":
						id := f.p.Account.ID
						if race == "appointed" {
							id = f.b.Account.ID
						}
						_, e := f.s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: "1", AdministratorID: id, Reason: "race"}, access.Meta{})
						return e
					case "disable":
						return identity.NewRepository(f.e.Pool).SetStatus(ctx, "Bob", false, false, identity.Meta{Reason: "race"})
					default:
						return accept()
					}
				}
				gate, e := f.e.Pool.Begin(ctx)
				must(t, e)
				defer gate.Rollback(ctx)
				_, e = gate.Exec(ctx, `SELECT id FROM authorization_state WHERE id=1 FOR UPDATE`)
				must(t, e)
				first, second := accept, other
				if !acceptFirst {
					first, second = other, accept
				}
				one, two := make(chan error, 1), make(chan error, 1)
				go func() { one <- first() }()
				waitForGraphWaiter(t, f.e, 1)
				go func() { two <- second() }()
				waitForGraphWaiter(t, f.e, 2)
				must(t, gate.Commit(ctx))
				must(t, <-one)
				e = <-two
				if acceptFirst && oneOfTest(race, "handover", "disable", "appointed") {
					must(t, e)
				} else {
					want := "invitation_not_pending"
					if race == "accept" || acceptFirst && race == "replace" {
						want = "invitation_already_accepted"
					}
					if !acceptFirst && race == "disable" {
						want = "unauthenticated"
					}
					code(t, e, want)
				}
				var count int
				must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, a.TenantID, f.b.Account.ID).Scan(&count))
				if count > 1 {
					t.Fatal("duplicate relation")
				}
				must(t, f.e.Service.Access().Check(ctx))
			})
		}
	}
}
func oneOfTest(v string, options ...string) bool {
	for _, s := range options {
		if s == v {
			return true
		}
	}
	return false
}
func TestTenantExpiryAfterLockWait(t *testing.T) {
	f := setup(t)
	a := f.create(t, "expiry-wait", f.a.Account.ID)
	l := f.invite(t, a.TenantID, f.a, f.b)
	gate, e := f.e.Pool.Begin(ctx)
	must(t, e)
	defer gate.Rollback(ctx)
	_, e = gate.Exec(ctx, `SELECT id FROM authorization_state WHERE id=1 FOR UPDATE`)
	must(t, e)
	done := make(chan error, 1)
	go func() {
		_, e := f.s.AcceptInvitation(ctx, f.actor(f.b), tenancy.AcceptanceInput{Token: tokenOf(l.Link), Version: "1"}, access.Meta{})
		done <- e
	}()
	waitForGraphWaiter(t, f.e, 1)
	// The request transaction already started; expire while it waits, without a real-time sleep.
	_, e = gate.Exec(ctx, `UPDATE tenant_invitations SET created_at=clock_timestamp()-interval '2 days',expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, l.Invitation.ID)
	must(t, e)
	must(t, gate.Commit(ctx))
	code(t, <-done, "invitation_not_pending")
	_, e = f.s.Members(ctx, f.actor(f.a), a.TenantID, tenancy.Filter{Cursor: strings.Repeat("x", 100)})
	code(t, e, "validation_failed")
}

func TestMemberGovernanceRaceAndAccountAvailability(t *testing.T) {
	for _, action := range []access.Operation{access.OpTenantMemberSuspend, access.OpTenantMemberRemove} {
		for _, memberFirst := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s-first-%v", action, memberFirst), func(t *testing.T) {
				f := setup(t)
				a := f.create(t, "member-admin-race", f.a.Account.ID)
				x := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
				change := func() error {
					_, e := f.s.ChangeMember(ctx, f.actor(f.a), a.TenantID, x.MembershipID, action, tenancy.MemberInput{Version: "1", Reason: "race"}, access.Meta{})
					return e
				}
				transfer := func() error {
					_, e := f.s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: "1", AdministratorID: f.b.Account.ID, Reason: "race"}, access.Meta{})
					return e
				}
				gate, e := f.e.Pool.Begin(ctx)
				must(t, e)
				defer gate.Rollback(ctx)
				_, e = gate.Exec(ctx, `SELECT id FROM authorization_state WHERE id=1 FOR UPDATE`)
				must(t, e)
				first, second := transfer, change
				if memberFirst {
					first, second = change, transfer
				}
				one, two := make(chan error, 1), make(chan error, 1)
				go func() { one <- first() }()
				waitForGraphWaiter(t, f.e, 1)
				go func() { two <- second() }()
				waitForGraphWaiter(t, f.e, 2)
				must(t, gate.Commit(ctx))
				must(t, <-one)
				e = <-two
				if memberFirst && action == access.OpTenantMemberRemove {
					must(t, e)
				} else if memberFirst {
					code(t, e, "administrator_membership_suspended")
				} else {
					code(t, e, "tenant_not_found")
				}
				must(t, f.e.Service.Access().Check(ctx))
			})
		}
	}
	f := setup(t)
	a := f.create(t, "availability-a", f.a.Account.ID)
	b := f.create(t, "availability-b", f.a.Account.ID)
	c := f.create(t, "availability-c", f.a.Account.ID)
	x := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
	y := f.accept(t, f.invite(t, b.TenantID, f.a, f.b), f.b)
	z := f.accept(t, f.invite(t, c.TenantID, f.a, f.b), f.b)
	repo := identity.NewRepository(f.e.Pool)
	must(t, repo.SetStatus(ctx, "Bob", false, false, identity.Meta{Reason: "disabled"}))
	v, e := f.s.Member(ctx, f.actor(f.a), a.TenantID, x.MembershipID)
	must(t, e)
	if v.Available || v.Status != "active" {
		t.Fatal(v)
	}
	suspended, e := f.s.ChangeMember(ctx, f.actor(f.a), b.TenantID, y.MembershipID, access.OpTenantMemberSuspend, tenancy.MemberInput{Version: "1", Reason: "while disabled"}, access.Meta{})
	must(t, e)
	_, e = f.s.ChangeMember(ctx, f.actor(f.a), b.TenantID, y.MembershipID, access.OpTenantMemberResume, tenancy.MemberInput{Version: suspended.Version, Reason: "while disabled"}, access.Meta{})
	code(t, e, "target_account_unavailable")
	_, e = f.s.ChangeMember(ctx, f.actor(f.a), c.TenantID, z.MembershipID, access.OpTenantMemberRemove, tenancy.MemberInput{Version: "1", Reason: "while disabled"}, access.Meta{})
	must(t, e)
	must(t, repo.SetStatus(ctx, "Bob", true, false, identity.Meta{Reason: "restore"}))
	f.b = login(t, f.e, "Bob")
	_, e = f.s.Context(ctx, f.actor(f.b), a.TenantID)
	must(t, e)
	for _, id := range []string{b.TenantID, c.TenantID} {
		_, e = f.s.Context(ctx, f.actor(f.b), id)
		code(t, e, "tenant_not_found")
	}
}

func TestTenantInvitationRateLimitsAndFailClosed(t *testing.T) {
	f := setup(t)
	a := f.create(t, "rate-limited", f.a.Account.ID)
	s := tenancy.New(f.e.Service.Access(), tenancy.Options{ConsoleURL: f.e.Config.PublicConsoleURL, Limit: f.e.Service.Limit, CredentialLimit: f.e.Service.Rate})
	for i := 0; i < 20; i++ {
		_, e := s.ResolveMember(ctx, f.actor(f.a), a.TenantID, "Bob")
		must(t, e)
	}
	_, e := s.ResolveMember(ctx, f.actor(f.a), a.TenantID, "Bob")
	code(t, e, "rate_limited")
	failing := tenancy.New(f.e.Service.Access(), tenancy.Options{Limit: func(context.Context, string, string, int) error { return identity.ErrUnavailable }})
	_, e = failing.CreateInvitation(ctx, f.actor(f.a), a.TenantID, tenancy.InvitationCreateInput{TargetID: f.b.Account.ID, Reason: "failure", RequestID: f.uuid(t)}, access.Meta{})
	code(t, e, "unavailable")
	var n int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM tenant_invitations WHERE tenant_id=$1`, a.TenantID).Scan(&n))
	if n != 0 {
		t.Fatal("rate dependency bypassed")
	}
}

func TestConcurrentInvitationCreationAndRecentPassword(t *testing.T) {
	f := setup(t)
	a := f.create(t, "creation-guards", f.a.Account.ID)
	in := tenancy.InvitationCreateInput{TargetID: f.b.Account.ID, Reason: "creation guards", RequestID: f.uuid(t)}
	_, e := f.e.Pool.Exec(ctx, `UPDATE account_sessions SET reauthenticated_at=now()-interval '6 minutes' WHERE id=$1`, f.a.Session.ID)
	must(t, e)
	_, e = f.s.CreateInvitation(ctx, f.actor(f.a), a.TenantID, in, access.Meta{})
	code(t, e, "reauthentication_required")
	var n int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM tenant_invitations WHERE tenant_id=$1`, a.TenantID).Scan(&n))
	if n != 0 {
		t.Fatal("stale password confirmation wrote an invitation")
	}
	must(t, f.e.Service.Reauthenticate(ctx, f.a, password, identity.Meta{}))
	other := in
	other.RequestID = f.uuid(t)
	gate, e := f.e.Pool.Begin(ctx)
	must(t, e)
	defer gate.Rollback(ctx)
	_, e = gate.Exec(ctx, `SELECT id FROM authorization_state WHERE id=1 FOR UPDATE`)
	must(t, e)
	one, two := make(chan error, 1), make(chan error, 1)
	go func() { _, e := f.s.CreateInvitation(ctx, f.actor(f.a), a.TenantID, in, access.Meta{}); one <- e }()
	waitForGraphWaiter(t, f.e, 1)
	go func() { _, e := f.s.CreateInvitation(ctx, f.actor(f.a), a.TenantID, other, access.Meta{}); two <- e }()
	waitForGraphWaiter(t, f.e, 2)
	must(t, gate.Commit(ctx))
	must(t, <-one)
	code(t, <-two, "invitation_pending_exists")
	must(t, f.e.Pool.QueryRow(ctx, `SELECT count(*) FROM tenant_invitations WHERE tenant_id=$1 AND status='pending'`, a.TenantID).Scan(&n))
	if n != 1 {
		t.Fatal("duplicate pending invitation")
	}
}

func TestNormalPasswordAndLogoutPreserveInvitation(t *testing.T) {
	f := setup(t)
	a := f.create(t, "normal-account-actions", f.a.Account.ID)
	l := f.invite(t, a.TenantID, f.a, f.b)
	const nextPassword = "Another tenant river 2026!"
	must(t, f.e.Service.ChangePassword(ctx, f.a, password, nextPassword, identity.Meta{}))
	_, token, _, e := f.e.Service.Login(ctx, "Alice", nextPassword, identity.Meta{})
	must(t, e)
	must(t, f.e.Service.Logout(ctx, token, identity.Meta{}))
	v, e := f.s.InspectInvitation(ctx, f.actor(f.b), tokenOf(l.Link), access.Meta{})
	must(t, e)
	if !v.CanAccept || v.Status != "pending" {
		t.Fatal("normal password change or logout invalidated invitation")
	}
	x := f.accept(t, l, f.b)
	f.transfer(t, a, f.p.Account.ID)
	c, e := f.s.Context(ctx, f.actor(f.b), a.TenantID)
	must(t, e)
	if c.MembershipID != x.MembershipID {
		t.Fatal("handover removed accepted member")
	}
	must(t, f.e.Service.Access().Check(ctx))
}

func TestInvitationPurposeAndCrossTenantBinding(t *testing.T) {
	f := setup(t)
	a := f.create(t, "purpose-a", f.a.Account.ID)
	b := f.create(t, "purpose-b", f.a.Account.ID)
	signup, e := f.e.Service.CreateInvitation(ctx, f.p, "", "purpose test", identity.Meta{})
	must(t, e)
	reset, e := f.e.Service.ResetLink(ctx, "Bob", identity.Meta{Reason: "purpose test"})
	must(t, e)
	for _, link := range []string{signup.URL, reset.URL} {
		_, e = f.s.InspectInvitation(ctx, f.actor(f.b), tokenOf(link), access.Meta{})
		code(t, e, "tenant_invitation_unavailable")
		_, e = f.s.AcceptInvitation(ctx, f.actor(f.b), tenancy.AcceptanceInput{Token: tokenOf(link), Version: "1"}, access.Meta{})
		code(t, e, "tenant_invitation_unavailable")
	}
	x := f.accept(t, f.invite(t, a.TenantID, f.a, f.b), f.b)
	_, e = f.e.Pool.Exec(ctx, `UPDATE tenant_member_roles SET tenant_id=$1 WHERE membership_id=$2`, b.TenantID, x.MembershipID)
	if e == nil {
		t.Fatal("cross-tenant role binding committed")
	}
	must(t, f.e.Service.Access().Check(ctx))
}
