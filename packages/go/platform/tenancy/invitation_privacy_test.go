package tenancy_test

import (
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
)

func TestAcceptedInvitationKeepsHistoricalSnapshotAfterMembershipLoss(t *testing.T) {
	for _, state := range []string{"suspended", "ended"} {
		t.Run(state, func(t *testing.T) {
			f := setup(t)
			tenant := f.create(t, "invitation-history", f.a.Account.ID)
			link := f.invite(t, tenant.TenantID, f.a, f.b)
			member := f.accept(t, link, f.b)
			inspect := func() tenancy.InvitationInspection {
				v, e := f.s.InspectInvitation(ctx, f.actor(f.b), tokenOf(link.Link), access.Meta{})
				must(t, e)
				return v
			}
			initial := inspect()
			if initial.Availability == nil || !initial.Availability.CanEnter {
				t.Fatal("active recipient must retain the current tenant entry")
			}
			op := access.OpTenantMemberSuspend
			if state == "ended" {
				op = access.OpTenantMemberRemove
			}
			_, e := f.s.ChangeMember(ctx, f.actor(f.a), tenant.TenantID, member.MembershipID, op, tenancy.MemberInput{Version: member.Version, Reason: "结束当前访问"}, access.Meta{})
			must(t, e)
			name, newCode := "成员失权后修改的名称", "renamed-after-loss"
			_, e = f.s.Update(ctx, f.actor(f.p), tenant.TenantID, tenancy.UpdateInput{Name: &name, Code: &newCode, Version: tenant.Version, Reason: "更新档案"}, true, access.Meta{})
			must(t, e)
			_, e = f.s.Get(ctx, f.actor(f.b), tenant.TenantID, false)
			code(t, e, "tenant_not_found")
			assertHistorical := func() {
				t.Helper()
				v := inspect()
				if v.Preview == nil || v.Preview.Tenant.Name != initial.Preview.Tenant.Name || v.Preview.Tenant.Code != initial.Preview.Tenant.Code {
					t.Fatal("accepted invitation must preserve its original display snapshot")
				}
				if v.MembershipStatus != state || v.CanAccept || v.Availability != nil || v.TenantStatus != "" || v.BlockedReason != "" {
					t.Fatalf("historical recipient must not read current availability: %+v", v)
				}
			}
			assertHistorical()
			f.lifecycle(t, tenant.TenantID, "disabled")
			assertHistorical()
		})
	}
}
