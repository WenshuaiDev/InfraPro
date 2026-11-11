package access_test

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/jackc/pgx/v5"
)

const secret = "Role test river 桥梁 2026!"

var ctx = context.Background()

func env(t *testing.T) *testsupport.Environment {
	t.Helper()
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:identity")
	}
	e, err := testsupport.New(ctx, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	if _, err = e.Service.Bootstrap(ctx, "Admin", "管理", secret, identity.Meta{}); err != nil {
		t.Fatal(err)
	}
	return e
}
func principal(t *testing.T, e *testsupport.Environment, user string) identity.Principal {
	t.Helper()
	_, token, _, err := e.Service.Login(ctx, user, secret, identity.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.Service.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func member(t *testing.T, e *testsupport.Environment, p identity.Principal, user string) identity.Principal {
	t.Helper()
	l, err := e.Service.CreateInvitation(ctx, p, "", "", identity.Meta{})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(l.URL)
	q, _ := url.ParseQuery(u.Fragment)
	if err = e.Service.AcceptInvitation(ctx, q.Get("token"), user, user, secret, identity.Meta{}); err != nil {
		t.Fatal(err)
	}
	return principal(t, e, user)
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func code(t *testing.T, e error, want string) {
	t.Helper()
	if e == nil || identity.PublicError(e).Code != want {
		t.Fatalf("got %v want %s", e, want)
	}
}
func meta() access.Meta { return access.Meta{Reason: "验证授权变更"} }
func role(t *testing.T, e *testsupport.Environment, p identity.Principal, name string, perms []access.Code) access.Role {
	t.Helper()
	s := e.Service.Access()
	r, err := s.CreateRole(ctx, e.Service.AccessActor(p), name, "", meta())
	must(t, err)
	r, err = s.ChangeRole(ctx, e.Service.AccessActor(p), r.ID, access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: r.Version, Permissions: perms}, meta())
	must(t, err)
	return r
}
func assign(t *testing.T, e *testsupport.Environment, p identity.Principal, id string, roles []string) access.Account {
	t.Helper()
	a := e.Service.AccessActor(p)
	s := e.Service.Access()
	target, err := s.Account(ctx, a, id)
	must(t, err)
	out, err := s.AssignRoles(ctx, a, id, target.Version, roles, meta())
	must(t, err)
	return out
}
func TestCatalogPolicy(t *testing.T) {
	codes := map[access.Code]bool{}
	for _, p := range access.Catalog() {
		if codes[p.Code] || p.Name == "" || p.Description == "" || p.Scope == "" || p.Risk == "" || p.Group == "" {
			t.Fatal(p)
		}
		codes[p.Code] = true
	}
	if len(codes) != 90 {
		t.Fatal(len(codes))
	}
	routes := map[string]bool{}
	for _, p := range access.Policies() {
		if !slices.Contains([]string{"public", "credential", "authenticated", "permission", "permission_any", "tenant", "project", "project_context"}, p.Class) {
			t.Fatal("unknown policy class", p.Class)
		}
		key := p.Method + " " + p.Path
		if routes[key] {
			t.Fatal(key)
		}
		routes[key] = true
		for _, c := range p.Permissions {
			if !codes[c] {
				t.Fatal(c)
			}
		}
		if p.Class == "permission" && len(p.Permissions) == 0 {
			t.Fatal(p)
		}
	}
}
func TestIntegrationAuthorizationLifecycle(t *testing.T) {
	e := env(t)
	p := principal(t, e, "Admin")
	b := member(t, e, p, "Member")
	s := e.Service.Access()
	actor := e.Service.AccessActor(p)
	snap, err := s.Snapshot(ctx, p.Account.ID)
	must(t, err)
	if len(snap.Permissions) != len(access.ManagementCodes()) {
		t.Fatal(snap)
	}
	snap, err = s.Snapshot(ctx, b.Account.ID)
	must(t, err)
	if len(snap.Permissions) != 6 {
		t.Fatal(snap)
	}
	_, err = e.Service.CreateInvitation(ctx, b, "", "", identity.Meta{})
	code(t, err, "permission_denied")
	r := role(t, e, p, "邀请办理", []access.Code{access.InvitationCreate, access.InvitationRead})
	r2 := role(t, e, p, "邀请备份", []access.Code{access.InvitationCreate})
	a := assign(t, e, p, b.Account.ID, []string{snap.Roles[0].ID, r.ID, r2.ID})
	_, err = s.AssignRoles(ctx, actor, b.Account.ID, "1", []string{}, meta())
	code(t, err, "version_conflict")
	l, err := e.Service.CreateInvitation(ctx, b, "", "", identity.Meta{})
	must(t, err)
	u, _ := url.Parse(l.URL)
	q, _ := url.ParseQuery(u.Fragment)
	assign(t, e, p, b.Account.ID, []string{r2.ID})
	_, err = e.Service.Inspect(ctx, q.Get("token"), false, identity.Meta{})
	must(t, err)
	r2, err = s.ChangeRole(ctx, actor, r2.ID, access.OpRoleSetStatus, access.RoleChange{ExpectedVersion: r2.Version, Status: "disabled"}, meta())
	must(t, err)
	_, err = e.Service.Inspect(ctx, q.Get("token"), false, identity.Meta{})
	code(t, err, "invalid_link")
	r2, err = s.ChangeRole(ctx, actor, r2.ID, access.OpRoleSetStatus, access.RoleChange{ExpectedVersion: r2.Version, Status: "active"}, meta())
	must(t, err)
	_, err = e.Service.Inspect(ctx, q.Get("token"), false, identity.Meta{})
	code(t, err, "invalid_link")
	_, err = s.ChangeRole(ctx, actor, r2.ID, access.OpRoleDelete, access.RoleChange{ExpectedVersion: r2.Version}, meta())
	code(t, err, "role_in_use")
	assign(t, e, p, b.Account.ID, []string{})
	_, err = e.Service.AuthContext(ctx, b)
	must(t, err)
	_, err = e.Service.ReadProfile(ctx, b)
	code(t, err, "permission_denied")
	_, err = s.ChangeRole(ctx, actor, r2.ID, access.OpRoleDelete, access.RoleChange{ExpectedVersion: r2.Version}, meta())
	must(t, err)
	_, err = s.ChangeRole(ctx, actor, r.ID, access.OpRoleUpdate, access.RoleChange{ExpectedVersion: r.Version, Name: "重命名", Description: ""}, meta())
	must(t, err)
	_, err = s.ChangeRole(ctx, actor, r.ID, access.OpRoleUpdate, access.RoleChange{ExpectedVersion: r.Version, Name: "过期编辑"}, meta())
	code(t, err, "version_conflict")
	_ = a
}
func TestIntegrationScopeAndManager(t *testing.T) {
	e := env(t)
	p := principal(t, e, "Admin")
	b := member(t, e, p, "Limited")
	s := e.Service.Access()
	pa := e.Service.AccessActor(p)
	r := role(t, e, p, "有限授权", []access.Code{access.RoleCreate, access.RoleUpdate, access.RoleSetPermissions, access.RoleSetStatus, access.RoleDelete, access.AccountAssignRoles, access.AccountDisable, access.AccountEnable, access.InvitationCreate})
	assign(t, e, p, b.Account.ID, []string{r.ID})
	ba := e.Service.AccessActor(b)
	target := role(t, e, p, "高级角色", []access.Code{access.AuditRead})
	_, err := s.ChangeRole(ctx, ba, target.ID, access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: target.Version, Permissions: []access.Code{}}, meta())
	code(t, err, "grant_scope_exceeded")
	for _, op := range []access.Operation{access.OpRoleUpdate, access.OpRoleSetStatus, access.OpRoleDelete} {
		_, err = s.ChangeRole(ctx, ba, target.ID, op, access.RoleChange{ExpectedVersion: target.Version, Name: "changed", Status: "disabled"}, meta())
		code(t, err, "grant_scope_exceeded")
	}
	target, err = s.ChangeRole(ctx, pa, target.ID, access.OpRoleSetStatus, access.RoleChange{ExpectedVersion: target.Version, Status: "disabled"}, meta())
	must(t, err)
	_, err = s.ChangeRole(ctx, ba, target.ID, access.OpRoleSetStatus, access.RoleChange{ExpectedVersion: target.Version, Status: "active"}, meta())
	code(t, err, "grant_scope_exceeded")
	admin, err := s.Account(ctx, pa, p.Account.ID)
	must(t, err)
	_, err = s.AssignRoles(ctx, ba, b.Account.ID, "2", []string{admin.Roles[0].ID}, meta())
	code(t, err, "grant_scope_exceeded")
	_, err = e.Service.SetAccountStatus(ctx, b, p.Account.ID, admin.Version, false, meta())
	code(t, err, "grant_scope_exceeded")
	_, err = s.AssignRoles(ctx, pa, p.Account.ID, admin.Version, []string{}, meta())
	code(t, err, "last_platform_manager")
	_, err = e.Service.SetAccountStatus(ctx, p, p.Account.ID, admin.Version, false, meta())
	code(t, err, "last_platform_manager")
	_, err = s.ChangeRole(ctx, pa, admin.Roles[0].ID, access.OpRoleSetStatus, access.RoleChange{ExpectedVersion: admin.Roles[0].Version, Status: "disabled"}, meta())
	code(t, err, "validation_failed")
	// Management completeness is a union, independent of role identity and name.
	all := access.ManagementCodes()
	x := role(t, e, p, "自定义甲", slices.Clone(all[:10]))
	y := role(t, e, p, "自定义乙", slices.Clone(all[10:]))
	assign(t, e, p, b.Account.ID, []string{x.ID, y.ID})
	assign(t, e, p, p.Account.ID, []string{})
	bp := principal(t, e, "Limited")
	_, err = s.ChangeRole(ctx, e.Service.AccessActor(bp), x.ID, access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: x.Version, Permissions: []access.Code{}}, meta())
	code(t, err, "last_platform_manager")
	must(t, e.Service.SetStatus(ctx, "Limited", false, true, identity.Meta{Reason: "确认失陷"}))
	must(t, s.RecoverAdmin(ctx, "Admin", "恢复管理"))
	must(t, e.Service.SetStatus(ctx, "Admin", true, false, identity.Meta{Reason: "恢复账号"}))
	must(t, s.Check(ctx))
}
func TestIntegrationAuthorizationConcurrencyAndRollback(t *testing.T) {
	e := env(t)
	p := principal(t, e, "Admin")
	b := member(t, e, p, "Second")
	s := e.Service.Access()
	must(t, s.RecoverAdmin(ctx, "Second", "测试并发"))
	a, err := s.Account(ctx, e.Service.AccessActor(p), p.Account.ID)
	must(t, err)
	bb, err := s.Account(ctx, e.Service.AccessActor(p), b.Account.ID)
	must(t, err)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, item := range []struct {
		p      identity.Principal
		target access.Account
	}{{p, bb}, {b, a}} {
		wg.Go(func() {
			_, err := s.AssignRoles(ctx, e.Service.AccessActor(item.p), item.target.ID, item.target.Version, []string{}, meta())
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, access.ErrDenied) && !errors.Is(err, access.ErrLastManager) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	must(t, s.RecoverAdmin(ctx, "Admin", "回滚测试"))
	p = principal(t, e, "Admin")
	before, err := s.Snapshot(ctx, p.Account.ID)
	must(t, err)
	_, err = e.Pool.Exec(ctx, `CREATE FUNCTION reject_access_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='role_created' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_access_audit BEFORE INSERT ON account_security_events FOR EACH ROW EXECUTE FUNCTION reject_access_audit()`)
	must(t, err)
	_, err = s.CreateRole(ctx, e.Service.AccessActor(p), "应回滚", "", meta())
	if err == nil {
		t.Fatal("audit failure did not roll back")
	}
	var n int
	must(t, e.Pool.QueryRow(ctx, "SELECT count(*) FROM roles WHERE name='应回滚'").Scan(&n))
	if n != 0 {
		t.Fatal(n)
	}
	after, err := s.Snapshot(ctx, p.Account.ID)
	must(t, err)
	if before.Revision != after.Revision {
		t.Fatal("revision leaked")
	}
	must(t, s.Sync(ctx))
	after, err = s.Snapshot(ctx, p.Account.ID)
	must(t, err)
	if before.Revision != after.Revision {
		t.Fatal("sync not idempotent")
	}
	// Shared lock protects the whole business transaction; a graph writer cannot pass it.
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.Transact(ctx, func(tx pgx.Tx) error {
			if err := access.Require(ctx, tx, p.Account.ID, access.OpInvitationCreate); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	tx, err := e.Pool.Begin(ctx)
	must(t, err)
	_, err = tx.Exec(ctx, "SELECT id FROM authorization_state WHERE id=1 FOR UPDATE NOWAIT")
	if err == nil {
		t.Fatal("authorization lock not held")
	}
	_ = tx.Rollback(ctx)
	close(release)
	must(t, <-done)
	_, err = e.Pool.Exec(ctx, "UPDATE permission_catalog SET name='drift' WHERE code=$1", access.ProfileRead)
	must(t, err)
	code(t, s.Check(ctx), "unavailable")
	must(t, s.Sync(ctx))
	must(t, s.Check(ctx))
}

func TestIntegrationInvitationRaceAndBindingRollback(t *testing.T) {
	e := env(t)
	p := principal(t, e, "Admin")
	b := member(t, e, p, "Inviter")
	s := e.Service.Access()
	r := role(t, e, p, "开户许可", []access.Code{access.InvitationCreate})
	assign(t, e, p, b.Account.ID, []string{r.ID})
	link, err := e.Service.CreateInvitation(ctx, b, "", "", identity.Meta{})
	must(t, err)
	u, _ := url.Parse(link.URL)
	q, _ := url.ParseQuery(u.Fragment)
	token := q.Get("token")
	// Failure during initial role binding rolls back account/password/invitation consumption together.
	_, err = e.Pool.Exec(ctx, `CREATE FUNCTION reject_binding() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.source='account' THEN RAISE EXCEPTION 'binding fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_binding BEFORE INSERT ON account_roles FOR EACH ROW EXECUTE FUNCTION reject_binding()`)
	must(t, err)
	if err = e.Service.AcceptInvitation(ctx, token, "Atomic", "", secret, identity.Meta{}); err == nil {
		t.Fatal("binding failure accepted")
	}
	var n int
	must(t, e.Pool.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE username='Atomic'").Scan(&n))
	if n != 0 {
		t.Fatal("partial account")
	}
	_, err = e.Service.Inspect(ctx, token, false, identity.Meta{})
	must(t, err)
	_, err = e.Pool.Exec(ctx, "DROP TRIGGER reject_binding ON account_roles; DROP FUNCTION reject_binding()")
	must(t, err)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	wg.Go(func() { results <- e.Service.AcceptInvitation(ctx, token, "Atomic", "", secret, identity.Meta{}) })
	wg.Go(func() {
		_, err := s.ChangeRole(ctx, e.Service.AccessActor(p), r.ID, access.OpRoleSetStatus, access.RoleChange{ExpectedVersion: r.Version, Status: "disabled"}, meta())
		results <- err
	})
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && identity.PublicError(err).Code != "invalid_link" {
			t.Fatal(err)
		}
	}
	must(t, e.Pool.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE username='Atomic'").Scan(&n))
	if n == 1 {
		var bound int
		must(t, e.Pool.QueryRow(ctx, "SELECT count(*) FROM account_roles ar JOIN accounts a ON a.id=ar.account_id WHERE a.username='Atomic'").Scan(&bound))
		if bound != 1 {
			t.Fatal("missing member role")
		}
	}
	_, err = e.Service.Inspect(ctx, token, false, identity.Meta{})
	code(t, err, "invalid_link")
}
func TestIntegrationRecentSessionVersionAndStatus(t *testing.T) {
	e := env(t)
	p := principal(t, e, "Admin")
	b := member(t, e, p, "Member")
	s := e.Service.Access()
	a := e.Service.AccessActor(p)
	_, err := e.Pool.Exec(ctx, "UPDATE account_sessions SET reauthenticated_at=now()-interval '6 minutes' WHERE id=$1", p.Session.ID)
	must(t, err)
	_, err = s.CreateRole(ctx, a, "过期复验", "", meta())
	code(t, err, "reauthentication_required")
	must(t, e.Service.Reauthenticate(ctx, p, secret, identity.Meta{}))
	r, err := s.CreateRole(ctx, a, "同名角色", "", meta())
	must(t, err)
	_, err = s.CreateRole(ctx, a, " 同名角色 ", "", meta())
	code(t, err, "name_taken")
	_, err = s.ChangeRole(ctx, a, r.ID, access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: r.Version, Permissions: []access.Code{"unknown"}}, meta())
	code(t, err, "validation_failed")
	_, err = s.ChangeRole(ctx, a, r.ID, access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: r.Version, Permissions: []access.Code{access.AuditRead, access.AuditRead}}, meta())
	code(t, err, "validation_failed")
	target, err := s.Account(ctx, a, b.Account.ID)
	must(t, err)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() { _, err := s.AssignRoles(ctx, a, b.Account.ID, target.Version, []string{}, meta()); errs <- err })
	}
	wg.Wait()
	close(errs)
	ok, conflict := 0, 0
	for err := range errs {
		if err == nil {
			ok++
		} else if errors.Is(err, access.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal(ok, conflict)
	}
	target, err = s.Account(ctx, a, b.Account.ID)
	must(t, err)
	target, err = e.Service.SetAccountStatus(ctx, p, b.Account.ID, target.Version, false, meta())
	must(t, err)
	must(t, s.RecoverAdmin(ctx, "Member", "预配置停用账号"))
	target, err = s.Account(ctx, a, b.Account.ID)
	must(t, err)
	if target.Status != "disabled" {
		t.Fatal("recovery enabled account")
	}
	_, err = e.Service.AuthContext(ctx, b)
	code(t, err, "unauthenticated")
	_, err = e.Service.SetAccountStatus(ctx, p, b.Account.ID, target.Version, true, meta())
	must(t, err)
	_, err = e.Service.AuthContext(ctx, b)
	code(t, err, "unauthenticated")
}

func TestIntegrationCatalogPreflightPreservesCustomRoles(t *testing.T) {
	e := env(t)
	p := principal(t, e, "Admin")
	b := member(t, e, p, "CustomManager")
	s := e.Service.Access()
	r := role(t, e, p, "自定义完整管理", access.ManagementCodes())
	assign(t, e, p, b.Account.ID, []string{r.ID})
	assign(t, e, p, p.Account.ID, []string{})
	// Simulate a release in which the custom role lacks one required permission.
	_, err := e.Pool.Exec(ctx, "DELETE FROM role_permissions WHERE role_id=$1 AND permission_code=$2", r.ID, access.AuditRead)
	must(t, err)
	_, err = e.Pool.Exec(ctx, "UPDATE permission_catalog SET description='release drift' WHERE code=$1", access.AuditRead)
	must(t, err)
	code(t, s.Sync(ctx), "last_platform_manager")
	var n int
	must(t, e.Pool.QueryRow(ctx, "SELECT count(*) FROM role_permissions WHERE role_id=$1", r.ID).Scan(&n))
	if n != len(access.ManagementCodes())-1 {
		t.Fatal("sync silently expanded custom role")
	}
	var bindings int
	must(t, e.Pool.QueryRow(ctx, "SELECT count(*) FROM account_roles WHERE account_id=$1", b.Account.ID).Scan(&bindings))
	if bindings != 1 {
		t.Fatal("sync rebound account")
	}
	must(t, s.RecoverAdmin(ctx, "CustomManager", "显式预置发布管理权限"))
	must(t, s.Sync(ctx))
	must(t, s.Check(ctx))
	must(t, e.Pool.QueryRow(ctx, "SELECT count(*) FROM role_permissions WHERE role_id=$1", r.ID).Scan(&n))
	if n != len(access.ManagementCodes())-1 {
		t.Fatal("custom configuration overwritten")
	}
}
