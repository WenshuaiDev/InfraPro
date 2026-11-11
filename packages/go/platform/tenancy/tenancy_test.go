package tenancy_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
)

var ctx = context.Background()

const password = "Tenant river 桥梁 2026!"

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func code(t *testing.T, e error, want string) {
	t.Helper()
	if e == nil || identity.PublicError(e).Code != want {
		t.Fatalf("got %v, want %s", e, want)
	}
}

type fixture struct {
	e       *testsupport.Environment
	s       *tenancy.Service
	p, a, b identity.Principal
}

func login(t *testing.T, e *testsupport.Environment, user string) identity.Principal {
	t.Helper()
	_, token, _, err := e.Service.Login(ctx, user, password, identity.Meta{})
	must(t, err)
	p, err := e.Service.Authenticate(ctx, token)
	must(t, err)
	return p
}
func setup(t *testing.T) fixture {
	t.Helper()
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:identity")
	}
	e, err := testsupport.New(ctx, slog.Default())
	must(t, err)
	t.Cleanup(e.Close)
	_, err = e.Service.Bootstrap(ctx, "Platform", "平台", password, identity.Meta{})
	must(t, err)
	p := login(t, e, "Platform")
	for _, u := range []string{"Alice", "Bob"} {
		l, err := e.Service.CreateInvitation(ctx, p, "", "", identity.Meta{})
		must(t, err)
		parsed, _ := url.Parse(l.URL)
		q, _ := url.ParseQuery(parsed.Fragment)
		must(t, e.Service.AcceptInvitation(ctx, q.Get("token"), u, u, password, identity.Meta{}))
	}
	return fixture{e, tenancy.New(e.Service.Access()), p, login(t, e, "Alice"), login(t, e, "Bob")}
}
func (f fixture) actor(p identity.Principal) access.Actor { return f.e.Service.AccessActor(p) }
func (f fixture) input(t *testing.T, code, admin string) tenancy.CreateInput {
	t.Helper()
	var id string
	must(t, f.e.Pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&id))
	return tenancy.CreateInput{Fields: tenancy.Fields{Name: "租户 " + code, Code: code}, AdministratorID: admin, RequestID: id, Reason: "验收创建"}
}
func (f fixture) create(t *testing.T, code, admin string) tenancy.Result {
	t.Helper()
	v, e := f.s.Create(ctx, f.actor(f.p), f.input(t, code, admin), access.Meta{})
	must(t, e)
	return v
}
func (f fixture) transfer(t *testing.T, v tenancy.Result, admin string) tenancy.Result {
	t.Helper()
	out, e := f.s.Transfer(ctx, f.actor(f.p), v.TenantID, tenancy.TransferInput{AdministratorID: admin, Version: v.Version, Reason: "验收交接"}, access.Meta{})
	must(t, e)
	return out
}
func (f fixture) status(t *testing.T, p identity.Principal, enabled bool) error {
	t.Helper()
	target, e := f.e.Service.Access().Account(ctx, f.actor(f.p), p.Account.ID)
	must(t, e)
	_, e = f.e.Service.SetAccountStatus(ctx, f.p, p.Account.ID, target.Version, enabled, access.Meta{Reason: "验收启停"})
	return e
}
func TestLifecycleIsolationHistoryAndRecovery(t *testing.T) {
	f := setup(t)
	s := f.s
	empty, e := s.Memberships(ctx, f.actor(f.a), "", tenancy.Filter{})
	must(t, e)
	if len(empty.Items) != 0 {
		t.Fatal("invitation joined tenant")
	}
	in := f.input(t, "  ALPHA  ", f.a.Account.ID)
	a, e := s.Create(ctx, f.actor(f.p), in, access.Meta{})
	must(t, e)
	replay, e := s.Create(ctx, f.actor(f.p), in, access.Meta{})
	must(t, e)
	if !replay.Replayed || replay.TenantID != a.TenantID {
		t.Fatal(replay)
	}
	in.Name = "changed"
	_, e = s.Create(ctx, f.actor(f.p), in, access.Meta{})
	code(t, e, "idempotency_conflict")
	_, e = s.Create(ctx, f.actor(f.p), f.input(t, "alpha", f.b.Account.ID), access.Meta{})
	code(t, e, "tenant_code_taken")
	b := f.create(t, "bravo", f.a.Account.ID)
	for _, p := range []identity.Principal{f.p, f.b} {
		_, e = s.Get(ctx, f.actor(p), a.TenantID, false)
		code(t, e, "tenant_not_found")
		_, e = s.Context(ctx, f.actor(p), a.TenantID)
		code(t, e, "tenant_not_found")
		_, e = s.Audit(ctx, f.actor(p), a.TenantID, tenancy.Filter{})
		code(t, e, "tenant_not_found")
	}
	_, e = s.Get(ctx, f.actor(f.a), a.TenantID, true)
	code(t, e, "permission_denied")
	_, e = s.Candidates(ctx, f.actor(f.a), tenancy.Filter{Query: "Al"})
	code(t, e, "permission_denied")
	c, e := s.Context(ctx, f.actor(f.a), a.TenantID)
	must(t, e)
	if len(c.Permissions) != 31 || len(c.Roles) != 1 || len(c.Sources) != 31 {
		t.Fatal(c)
	}
	originalMember := c.MembershipID
	my, e := s.Memberships(ctx, f.actor(f.a), "", tenancy.Filter{Limit: 1})
	must(t, e)
	if len(my.Items) != 1 || my.NextCursor == "" {
		t.Fatal(my)
	}
	next, e := s.Memberships(ctx, f.actor(f.a), "", tenancy.Filter{Limit: 1, Cursor: my.NextCursor})
	must(t, e)
	if len(next.Items) != 1 || next.Items[0].ID == my.Items[0].ID {
		t.Fatal(next)
	}
	a, e = s.Update(ctx, f.actor(f.a), a.TenantID, update(tenancy.Fields{Name: "Ａlpha", Code: "alpha-new", Description: "<script>文本</script>"}, a.Version, "更名"), false, access.Meta{})
	must(t, e)
	_, e = s.Update(ctx, f.actor(f.p), b.TenantID, update(tenancy.Fields{Name: "B", Code: "alpha"}, b.Version, "抢用历史编号"), true, access.Meta{})
	code(t, e, "tenant_code_taken")
	a, e = s.Update(ctx, f.actor(f.p), a.TenantID, update(tenancy.Fields{Name: "Alpha", Code: "alpha"}, a.Version, "改回"), true, access.Meta{})
	must(t, e)
	unchanged, e := s.Update(ctx, f.actor(f.a), a.TenantID, update(tenancy.Fields{Name: "Alpha", Code: "alpha"}, a.Version, "不变"), false, access.Meta{})
	must(t, e)
	if unchanged.Version != a.Version {
		t.Fatal("no-op version")
	}
	code(t, f.status(t, f.a, false), "tenant_admin_transfer_required")
	_, e = f.e.Service.AuthContext(ctx, f.a)
	must(t, e)
	repo := identity.NewRepository(f.e.Pool)
	code(t, repo.SetStatus(ctx, "Alice", false, false, identity.Meta{Reason: "常规停用"}), "tenant_admin_transfer_required")
	must(t, repo.SetStatus(ctx, "Alice", false, true, identity.Meta{Reason: "应急停用"}))
	must(t, f.e.Service.Ready(ctx))
	_, e = f.e.Service.AuthContext(ctx, f.a)
	code(t, e, "unauthenticated")
	unavailable, e := s.List(ctx, f.actor(f.p), tenancy.Filter{Unavailable: true})
	must(t, e)
	if len(unavailable.Items) != 2 {
		t.Fatal(unavailable)
	}
	must(t, repo.SetStatus(ctx, "Alice", true, false, identity.Meta{Reason: "恢复"}))
	_, e = f.e.Service.AuthContext(ctx, f.a)
	code(t, e, "unauthenticated")
	f.a = login(t, f.e, "Alice")
	c, e = s.Context(ctx, f.actor(f.a), a.TenantID)
	must(t, e)
	if c.MembershipID != originalMember {
		t.Fatal("preserved member not restored")
	}
	_, e = s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: a.Version, AdministratorID: f.a.Account.ID, Reason: "不变"}, access.Meta{})
	code(t, e, "administrator_unchanged")
	a = f.transfer(t, a, f.b.Account.ID)
	_, e = s.Get(ctx, f.actor(f.a), a.TenantID, false)
	code(t, e, "tenant_not_found")
	_, e = s.Get(ctx, f.actor(f.b), a.TenantID, false)
	must(t, e)
	_, e = s.Get(ctx, f.actor(f.a), b.TenantID, false)
	must(t, e)
	a = f.transfer(t, a, f.a.Account.ID)
	c, e = s.Context(ctx, f.actor(f.a), a.TenantID)
	must(t, e)
	if c.MembershipID == originalMember {
		t.Fatal("resurrected membership")
	}
	must(t, repo.SetStatus(ctx, "Alice", false, true, identity.Meta{Reason: "再次应急"}))
	a = f.transfer(t, a, f.b.Account.ID)
	must(t, repo.SetStatus(ctx, "Alice", true, false, identity.Meta{Reason: "恢复不复活"}))
	f.a = login(t, f.e, "Alice")
	_, e = s.Context(ctx, f.actor(f.a), a.TenantID)
	code(t, e, "tenant_not_found")
	code(t, f.status(t, f.a, false), "tenant_admin_transfer_required")
	f.transfer(t, b, f.b.Account.ID)
	must(t, f.status(t, f.a, false))
	audit, e := s.Audit(ctx, f.actor(f.b), a.TenantID, tenancy.Filter{Limit: 2})
	must(t, e)
	if len(audit.Items) != 2 || audit.NextCursor == "" {
		t.Fatal(audit)
	}
	for _, ev := range audit.Items {
		if ev.TenantID != a.TenantID {
			t.Fatal("audit leak")
		}
	}
	global, e := f.e.Service.Access().AuditEvents(ctx, f.actor(f.p), access.Filter{TenantID: a.TenantID})
	must(t, e)
	if len(global.Items) < 7 {
		t.Fatal(global)
	}
	security, e := f.e.Service.SecurityEvents(ctx, f.b, "")
	must(t, e)
	for _, ev := range security.Items {
		if strings.HasPrefix(ev.EventType, "tenant.") {
			t.Fatal("personal event leak")
		}
	}
	must(t, f.e.Service.Access().Check(ctx))
	must(t, f.e.Service.Access().Sync(ctx))
}
func TestConstraintsRollbackAndValidation(t *testing.T) {
	f := setup(t)
	a := f.create(t, "alpha", f.a.Account.ID)
	var tenantRole, platformRole string
	must(t, f.e.Pool.QueryRow(ctx, "SELECT id::text FROM roles WHERE builtin_key=$1", access.TenantAdminKey).Scan(&tenantRole))
	must(t, f.e.Pool.QueryRow(ctx, "SELECT id::text FROM roles WHERE builtin_key=$1", access.MemberKey).Scan(&platformRole))
	for _, q := range []string{fmt.Sprintf("INSERT INTO account_roles(account_id,role_id,source) VALUES('%s','%s','test')", f.b.Account.ID, tenantRole), fmt.Sprintf("INSERT INTO role_permissions(role_id,permission_code) VALUES('%s','%s')", platformRole, access.TenantProfileRead)} {
		_, e := f.e.Pool.Exec(ctx, q)
		if e == nil {
			t.Fatal("scope constraint bypassed")
		}
	}
	target, e := f.e.Service.Access().Account(ctx, f.actor(f.p), f.b.Account.ID)
	must(t, e)
	_, e = f.e.Service.Access().AssignRoles(ctx, f.actor(f.p), f.b.Account.ID, target.Version, []string{tenantRole}, access.Meta{Reason: "错误范围"})
	code(t, e, "validation_failed")
	for _, fields := range []tenancy.Fields{{Name: "", Code: "bad"}, {Name: strings.Repeat("𠮷", 101), Code: "long"}, {Name: "X", Code: "-bad"}, {Name: "X", Code: "bad", Description: strings.Repeat("𠮷", 1001)}} {
		in := f.input(t, "valid", f.b.Account.ID)
		in.Fields = fields
		_, e = f.s.Create(ctx, f.actor(f.p), in, access.Meta{})
		code(t, e, "validation_failed")
	}
	// Each storage fault must roll back the profile, code claim, graph revision and event together.
	for _, table := range []string{"tenant_memberships", "tenant_member_roles", "account_security_events"} {
		t.Run(table, func(t *testing.T) {
			var before string
			must(t, f.e.Pool.QueryRow(ctx, "SELECT revision::text FROM authorization_state").Scan(&before))
			_, e := f.e.Pool.Exec(ctx, "CREATE FUNCTION test_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$; CREATE TRIGGER test_fail BEFORE INSERT ON "+table+" FOR EACH ROW EXECUTE FUNCTION test_fail()")
			must(t, e)
			_, e = f.s.Create(ctx, f.actor(f.p), f.input(t, "rollback", f.b.Account.ID), access.Meta{})
			code(t, e, "internal_error")
			_, e = f.e.Pool.Exec(ctx, "DROP TRIGGER test_fail ON "+table+"; DROP FUNCTION test_fail()")
			must(t, e)
			var n int
			var after string
			must(t, f.e.Pool.QueryRow(ctx, "SELECT count(*) FROM tenant_codes WHERE code='rollback'").Scan(&n))
			must(t, f.e.Pool.QueryRow(ctx, "SELECT revision::text FROM authorization_state").Scan(&after))
			if n != 0 || before != after {
				t.Fatal("partial transaction")
			}
		})
	}
	must(t, f.status(t, f.b, false))
	_, e = f.s.Create(ctx, f.actor(f.p), f.input(t, "disabled", f.b.Account.ID), access.Meta{})
	code(t, e, "administrator_unavailable")
	_, e = f.s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: a.Version, AdministratorID: f.b.Account.ID, Reason: "已停用"}, access.Meta{})
	code(t, e, "administrator_unavailable")
}
func TestConcurrency(t *testing.T) {
	f := setup(t)
	run := func(actions ...func() error) []error {
		var wg sync.WaitGroup
		out := make([]error, len(actions))
		for i, fn := range actions {
			wg.Go(func() { out[i] = fn() })
		}
		wg.Wait()
		return out
	}
	in := f.input(t, "same-request", f.a.Account.ID)
	var first, second tenancy.Result
	errs := run(func() error { v, e := f.s.Create(ctx, f.actor(f.p), in, access.Meta{}); first = v; return e }, func() error { v, e := f.s.Create(ctx, f.actor(f.p), in, access.Meta{}); second = v; return e })
	for _, e := range errs {
		must(t, e)
	}
	if first.TenantID != second.TenantID {
		t.Fatal("duplicate intent")
	}
	a := f.create(t, "edit-race", f.a.Account.ID)
	update := func(p identity.Principal, name string) func() error {
		return func() error {
			_, e := f.s.Update(ctx, f.actor(p), a.TenantID, update(tenancy.Fields{Name: name, Code: "edit-race"}, a.Version, "并发"), p.Account.ID == f.p.Account.ID, access.Meta{})
			return e
		}
	}
	assertOne := func(errs []error, want string) {
		t.Helper()
		ok, conflict := 0, 0
		for _, e := range errs {
			if e == nil {
				ok++
			} else {
				code(t, e, want)
				conflict++
			}
		}
		if ok != 1 || conflict != 1 {
			t.Fatal(errs)
		}
	}
	assertOne(run(update(f.a, "A"), update(f.p, "P")), "version_conflict")
	current, e := f.s.Get(ctx, f.actor(f.p), a.TenantID, true)
	must(t, e)
	a.Version = current.Version
	transfer := func(id string) func() error {
		return func() error {
			_, e := f.s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: a.Version, AdministratorID: id, Reason: "并发交接"}, access.Meta{})
			return e
		}
	}
	assertOne(run(transfer(f.b.Account.ID), transfer(f.p.Account.ID)), "version_conflict")
	// A creation and a normal disable cannot both succeed.
	target, e := f.e.Service.Access().Account(ctx, f.actor(f.p), f.b.Account.ID)
	must(t, e)
	errs = run(func() error {
		_, e := f.s.Create(ctx, f.actor(f.p), f.input(t, "disable-race", f.b.Account.ID), access.Meta{})
		return e
	}, func() error {
		_, e := f.e.Service.SetAccountStatus(ctx, f.p, f.b.Account.ID, target.Version, false, access.Meta{Reason: "并发停用"})
		return e
	})
	if errs[0] == nil && errs[1] == nil {
		t.Fatal("administrator vacancy")
	}
	for _, e := range errs {
		if e != nil {
			c := identity.PublicError(e).Code
			if c != "tenant_admin_transfer_required" && c != "administrator_unavailable" {
				t.Fatal(e)
			}
		}
	}
	must(t, f.e.Service.Access().Check(ctx))
	// Duplicate code with different requests also has a single winner.
	x, y := f.input(t, "code-race", f.a.Account.ID), f.input(t, "code-race", f.a.Account.ID)
	assertOne(run(func() error { _, e := f.s.Create(ctx, f.actor(f.p), x, access.Meta{}); return e }, func() error { _, e := f.s.Create(ctx, f.actor(f.p), y, access.Meta{}); return e }), "tenant_code_taken")
}

func update(f tenancy.Fields, v, r string) tenancy.UpdateInput {
	return tenancy.UpdateInput{Name: &f.Name, Code: &f.Code, Description: &f.Description, Version: v, Reason: r}
}

func TestHandoverDisableAndEditOrdering(t *testing.T) {
	f := setup(t)
	// Hold a real authorization-state lock to deterministically serialize competing requests.
	// Each order is tested; the second request begins while the first owns the graph lock.
	for _, disableFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("disable-first-%v", disableFirst), func(t *testing.T) {
			a := f.create(t, fmt.Sprintf("handover-%v", disableFirst), f.a.Account.ID)
			target, e := f.e.Service.Access().Account(ctx, f.actor(f.p), f.b.Account.ID)
			must(t, e)
			gate, e := f.e.Pool.Begin(ctx)
			must(t, e)
			defer gate.Rollback(ctx)
			_, e = gate.Exec(ctx, `SELECT id FROM authorization_state WHERE id=1 FOR UPDATE`)
			must(t, e)
			// Queue the first request and observe it blocked in PostgreSQL, then queue the second.
			transfer := func() error {
				_, e := f.s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: a.Version, AdministratorID: f.b.Account.ID, Reason: "受控并发"}, access.Meta{})
				return e
			}
			disable := func() error {
				_, e := f.e.Service.SetAccountStatus(ctx, f.p, f.b.Account.ID, target.Version, false, access.Meta{Reason: "受控并发停用"})
				return e
			}
			first, second := transfer, disable
			if disableFirst {
				first, second = disable, transfer
			}
			one, two := make(chan error, 1), make(chan error, 1)
			go func() { one <- first() }()
			waitForGraphWaiter(t, f.e, 1)
			go func() { two <- second() }()
			waitForGraphWaiter(t, f.e, 2)
			must(t, gate.Commit(ctx))
			must(t, <-one)
			e = <-two
			if disableFirst {
				code(t, e, "administrator_unavailable")
				must(t, identity.NewRepository(f.e.Pool).SetStatus(ctx, "Bob", true, false, identity.Meta{Reason: "继续测试"}))
				f.b = login(t, f.e, "Bob")
			} else {
				code(t, e, "tenant_admin_transfer_required")
			}
			must(t, f.e.Service.Access().Check(ctx))
		})
	}
	for _, editFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("edit-first-%v", editFirst), func(t *testing.T) {
			a := f.create(t, fmt.Sprintf("edit-order-%v", editFirst), f.a.Account.ID)
			gate, e := f.e.Pool.Begin(ctx)
			must(t, e)
			defer gate.Rollback(ctx)
			_, e = gate.Exec(ctx, `SELECT id FROM authorization_state WHERE id=1 FOR UPDATE`)
			must(t, e)
			edit := func() error {
				_, e := f.s.Update(ctx, f.actor(f.a), a.TenantID, update(tenancy.Fields{Name: "已编辑", Code: fmt.Sprintf("edit-order-%v", editFirst)}, a.Version, "先后顺序"), false, access.Meta{})
				return e
			}
			transfer := func() error {
				_, e := f.s.Transfer(ctx, f.actor(f.p), a.TenantID, tenancy.TransferInput{Version: a.Version, AdministratorID: f.b.Account.ID, Reason: "先后顺序"}, access.Meta{})
				return e
			}
			first, second := transfer, edit
			if editFirst {
				first, second = edit, transfer
			}
			one, two := make(chan error, 1), make(chan error, 1)
			go func() { one <- first() }()
			waitForGraphWaiter(t, f.e, 1)
			go func() { two <- second() }()
			waitForGraphWaiter(t, f.e, 2)
			must(t, gate.Commit(ctx))
			must(t, <-one)
			e = <-two
			if editFirst {
				code(t, e, "version_conflict")
				a.Version = "2"
				f.transfer(t, a, f.b.Account.ID)
			} else {
				code(t, e, "tenant_not_found")
			}
			_, e = f.s.Update(ctx, f.actor(f.a), a.TenantID, update(tenancy.Fields{Name: "旧权限不能落库", Code: "forbidden"}, "3", "撤权后"), false, access.Meta{})
			code(t, e, "tenant_not_found")
		})
	}
}
func waitForGraphWaiter(t *testing.T, e *testsupport.Environment, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		// A blocked pgx transaction is visible in the shared test DB; constrain it to this schema.
		err := e.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND query LIKE '%authorization_state%' AND pid IN (SELECT pid FROM pg_locks WHERE relation='authorization_state'::regclass)`).Scan(&n)
		must(t, err)
		if n >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected graph lock waiters", want)
}

func TestPlatformActionsAreIndependentAndTenantOnlyIdentity(t *testing.T) {
	f := setup(t)
	a := f.create(t, "matrix", f.b.Account.ID)
	as := f.e.Service.Access()
	actor := f.actor(f.p)
	for _, permission := range []access.Code{access.TenantRead, access.TenantCreate, access.TenantUpdate, access.TenantTransfer} {
		t.Run(string(permission), func(t *testing.T) {
			role, e := as.CreateRole(ctx, actor, "Only "+string(permission), "", access.Meta{Reason: "独立动作测试"})
			must(t, e)
			role, e = as.ChangeRole(ctx, actor, role.ID, access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: role.Version, Permissions: []access.Code{permission}}, access.Meta{Reason: "独立动作测试"})
			must(t, e)
			target, e := as.Account(ctx, actor, f.a.Account.ID)
			must(t, e)
			_, e = as.AssignRoles(ctx, actor, f.a.Account.ID, target.Version, []string{role.ID}, access.Meta{Reason: "仅此动作"})
			must(t, e)
			check := func(e error, allowed bool) {
				t.Helper()
				if allowed {
					must(t, e)
				} else {
					code(t, e, "permission_denied")
				}
			}
			_, e = f.s.List(ctx, f.actor(f.a), tenancy.Filter{})
			check(e, permission == access.TenantRead)
			_, e = f.s.Get(ctx, f.actor(f.a), a.TenantID, true)
			check(e, permission == access.TenantRead)
			_, e = f.s.Candidates(ctx, f.actor(f.a), tenancy.Filter{Query: "Bo"})
			check(e, permission == access.TenantCreate || permission == access.TenantTransfer)
			_, e = f.s.Create(ctx, f.actor(f.a), f.input(t, "matrix-created", f.b.Account.ID), access.Meta{})
			check(e, permission == access.TenantCreate)
			v, e := f.s.Update(ctx, f.actor(f.a), a.TenantID, update(tenancy.Fields{Name: "已改", Code: "matrix"}, a.Version, "独立编辑"), true, access.Meta{})
			check(e, permission == access.TenantUpdate)
			if e == nil {
				a = v
			}
			v, e = f.s.Transfer(ctx, f.actor(f.a), a.TenantID, tenancy.TransferInput{Version: a.Version, AdministratorID: f.a.Account.ID, Reason: "独立交接"}, access.Meta{})
			check(e, permission == access.TenantTransfer)
			if e == nil {
				a = v
			}
		})
	}
	target, e := as.Account(ctx, actor, f.a.Account.ID)
	must(t, e)
	_, e = as.AssignRoles(ctx, actor, f.a.Account.ID, target.Version, []string{}, access.Meta{Reason: "无平台业务角色"})
	must(t, e)
	auth, e := f.e.Service.AuthContext(ctx, f.a)
	must(t, e)
	if len(auth.Permissions) != 0 {
		t.Fatal(auth)
	}
	context, e := f.s.Context(ctx, f.actor(f.a), a.TenantID)
	must(t, e)
	if len(context.Permissions) != 31 {
		t.Fatal(context)
	}
	_, e = f.s.Memberships(ctx, f.actor(f.a), "", tenancy.Filter{})
	must(t, e)
	_, e = f.s.Memberships(ctx, f.actor(f.a), f.b.Account.ID, tenancy.Filter{})
	code(t, e, "permission_denied")
	// Platform admin does not obtain membership when creating on behalf of another account.
	mine, e := f.s.Memberships(ctx, actor, "", tenancy.Filter{})
	must(t, e)
	if len(mine.Items) != 0 {
		t.Fatal(mine)
	}
}
