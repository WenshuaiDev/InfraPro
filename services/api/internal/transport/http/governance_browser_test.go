package http_test

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
)

// Emergency maintenance cannot be invoked from the public HTTP API. Prepare
// that state through its owning service, exclusively in the isolated E2E DB.
func governanceBrowserFixture(t *testing.T, ctx context.Context, env *testsupport.Environment, platform identity.Principal, fixture map[string]string) {
	if os.Getenv("INFRAPRO_GOVERNANCE_E2E") != "1" {
		return
	}
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	uuid := func() string {
		var id string
		check(env.Pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&id))
		return id
	}
	token := func(link string) string {
		u, err := url.Parse(link)
		check(err)
		q, err := url.ParseQuery(u.Fragment)
		check(err)
		return q.Get("token")
	}
	actor := env.Service.AccessActor
	signup := func(name string) identity.Principal {
		link, err := env.Service.CreateInvitation(ctx, platform, "", "", identity.Meta{})
		check(err)
		check(env.Service.AcceptInvitation(ctx, token(link.URL), name, "治理验收", password, identity.Meta{}))
		_, session, _, err := env.Service.Login(ctx, name, password, identity.Meta{})
		check(err)
		principal, err := env.Service.Authenticate(ctx, session)
		check(err)
		return principal
	}
	ts, ps := tenancy.New(env.Service.Access()), projects.New(env.Service.Access())
	for _, mode := range []string{"desktop", "mobile"} {
		prefix := "governance_" + mode + "_"
		admin, former, successor := signup(prefix+"admin"), signup(prefix+"former"), signup(prefix+"successor")
		tenant, err := ts.Create(ctx, actor(platform), tenancy.CreateInput{Fields: tenancy.Fields{Name: "治理恢复验收", Code: prefix + "tenant"}, AdministratorID: admin.Account.ID, RequestID: uuid(), Reason: "隔离浏览器验收"}, access.Meta{})
		check(err)
		join := func(person identity.Principal) string {
			invite, err := ts.CreateInvitation(ctx, actor(admin), tenant.TenantID, tenancy.InvitationCreateInput{TargetID: person.Account.ID, RequestID: uuid(), Reason: "准备恢复场景"}, access.Meta{})
			check(err)
			joined, err := ts.AcceptInvitation(ctx, actor(person), tenancy.AcceptanceInput{Token: token(invite.Link), Version: invite.Invitation.Version}, access.Meta{})
			check(err)
			return joined.MembershipID
		}
		old, next := join(former), join(successor)
		project, err := ps.Create(ctx, actor(admin), tenant.TenantID, projects.CreateInput{Fields: projects.Fields{Name: "恢复管理员验收项目", Code: "recover-admin", Purpose: "治理恢复验证"}, WriteMeta: projects.WriteMeta{RequestID: uuid(), Reason: "准备恢复场景"}, Owner: next, Administrator: old}, access.Meta{})
		check(err)
		check(env.Service.SetStatus(ctx, former.Account.Username, false, true, identity.Meta{Reason: "应急停用，验证治理恢复入口"}))
		fixture[prefix+"tenant"], fixture[prefix+"project"] = tenant.TenantID, project.ProjectID
		fixture[prefix+"admin"], fixture[prefix+"successor"] = admin.Account.Username, successor.Account.Username
	}
}
