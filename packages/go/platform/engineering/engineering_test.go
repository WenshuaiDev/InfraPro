package engineering_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
)

var ctx = context.Background()

const password = "Engineering river 橋梁 2026!"

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func code(t *testing.T, e error, want string) {
	t.Helper()
	var de *eng.Error
	got := identity.PublicError(e).Code
	if errors.As(e, &de) {
		got = de.Code
	}
	if e == nil || got != want {
		t.Fatalf("want %s, got %s: %v", want, got, e)
	}
}

type fixture struct {
	env *testsupport.Environment
	s   *eng.Service
	p   identity.Principal
}

func setup(t *testing.T) fixture {
	t.Helper()
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("requires isolated test database")
	}
	e, err := testsupport.New(ctx, slog.Default())
	must(t, err)
	t.Cleanup(e.Close)
	_, err = e.Service.Bootstrap(ctx, "Platform", "平台", password, identity.Meta{})
	must(t, err)
	_, token, _, err := e.Service.Login(ctx, "Platform", password, identity.Meta{})
	must(t, err)
	p, err := e.Service.Authenticate(ctx, token)
	must(t, err)
	return fixture{e, eng.New(e.Service.Access()), p}
}
func (f fixture) actor(p identity.Principal) access.Actor { return f.env.Service.AccessActor(p) }
func (f fixture) wm(t *testing.T) eng.WriteMeta {
	t.Helper()
	var id string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id))
	return eng.WriteMeta{RequestID: id, Reason: "工程定义验证"}
}
func (f fixture) create(t *testing.T, kind eng.Kind, code string, c eng.Content) eng.Result {
	t.Helper()
	v, e := f.s.Create(ctx, f.actor(f.p), eng.Scope{Kind: kind}, eng.CreateInput{WriteMeta: f.wm(t), Code: code, Content: c}, access.Meta{})
	must(t, e)
	return v
}
func (f fixture) publish(t *testing.T, kind eng.Kind, v eng.Result) eng.Result {
	t.Helper()
	out, e := f.s.Publish(ctx, f.actor(f.p), eng.Scope{Kind: kind}, v.DefinitionID, eng.RevisionInput{WriteMeta: f.wm(t), Revision: v.Revision}, access.Meta{})
	must(t, e)
	return out
}
func (f fixture) member(t *testing.T, name string) identity.Principal {
	t.Helper()
	l, e := f.env.Service.CreateInvitation(ctx, f.p, "", "", identity.Meta{})
	must(t, e)
	u, _ := url.Parse(l.URL)
	q, _ := url.ParseQuery(u.Fragment)
	must(t, f.env.Service.AcceptInvitation(ctx, q.Get("token"), name, name, password, identity.Meta{}))
	_, token, _, e := f.env.Service.Login(ctx, name, password, identity.Meta{})
	must(t, e)
	p, e := f.env.Service.Authenticate(ctx, token)
	must(t, e)
	return p
}
func simple(name string) eng.Content { return eng.Content{MetamodelVersion: "1", Name: name} }
func ptr[T any](v T) *T              { return &v }
func TestPublishedHistoryDependenciesAndIsolation(t *testing.T) {
	f := setup(t)
	a := f.actor(f.p)
	_, e := f.s.SaveCategory(ctx, a, eng.CategoryInput{WriteMeta: f.wm(t), Key: "length", Name: "长度", Description: "空间长度"}, access.Meta{})
	must(t, e)
	unit := simple("毫米")
	unit.Unit = &eng.Unit{Symbol: "mm", Category: "length"}
	u := f.publish(t, eng.UnitDefinition, f.create(t, eng.UnitDefinition, "mm", unit))
	c := simple("任意新类型")
	c.Fields = []eng.Field{{Key: "name", Name: "名称", Shape: "text", Required: true}, {Key: "length", Name: "长度", Shape: "decimal", Minimum: ptr(0.0), UnitVersion: u.VersionID}}
	c.Components = []eng.Component{{Key: "layer", Name: "结构层", Fields: []eng.Field{{Key: "thickness", Name: "厚度", Shape: "decimal", UnitVersion: u.VersionID, Required: true}}}}
	c.Relations = []eng.Relation{{Parent: "root", Child: "layer", MinCount: 1, MaxCount: ptr(3)}}
	typ := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "arbitrary_type_971", c))
	metric := simple("位移")
	metric.Metric = &eng.Metric{Shape: "decimal", Category: "length", StandardUnit: u.VersionID, AllowedUnits: []string{u.VersionID}}
	m := f.publish(t, eng.MetricDefinition, f.create(t, eng.MetricDefinition, "displacement", metric))
	template := simple("道路监测")
	template.Template = &eng.Template{TypeVersions: []string{typ.VersionID}, Items: []eng.Item{{Key: "movement", Name: "位移", Enabled: true, Targets: []eng.Target{{TypeVersion: typ.VersionID, Component: "layer"}}, Bindings: []eng.Binding{{MetricVersion: m.VersionID, UnitVersion: u.VersionID, ResourceKind: "observation_channel", MinCount: 1}}}}}
	tpl := f.publish(t, eng.MonitoringTemplate, f.create(t, eng.MonitoringTemplate, "road_monitoring", template))
	alice := f.member(t, "Alice")
	bob := f.member(t, "Bob")
	ts := tenancy.New(f.env.Service.Access())
	ta, e := ts.Create(ctx, a, tenancy.CreateInput{Fields: tenancy.Fields{Name: "租户 A", Code: "tenant-a"}, AdministratorID: alice.Account.ID, RequestID: f.wm(t).RequestID, Reason: "工程测试"}, access.Meta{})
	must(t, e)
	tb, e := ts.Create(ctx, a, tenancy.CreateInput{Fields: tenancy.Fields{Name: "租户 B", Code: "tenant-b"}, AdministratorID: bob.Account.ID, RequestID: f.wm(t).RequestID, Reason: "工程测试"}, access.Meta{})
	must(t, e)
	scope := eng.Scope{Tenant: ta.TenantID, Kind: eng.MonitoringTemplate}
	copy, e := f.s.Copy(ctx, f.actor(alice), scope, eng.CopyInput{WriteMeta: f.wm(t), Code: "local_template", SourceVersion: tpl.VersionID}, access.Meta{})
	must(t, e)
	private, e := f.s.Publish(ctx, f.actor(alice), scope, copy.DefinitionID, eng.RevisionInput{WriteMeta: f.wm(t), Revision: copy.Revision}, access.Meta{})
	must(t, e)
	_, e = f.s.Detail(ctx, f.actor(bob), eng.Scope{Tenant: tb.TenantID, Kind: eng.MonitoringTemplate}, copy.DefinitionID)
	code(t, e, "not_found")
	otherScope := eng.Scope{Tenant: tb.TenantID, Kind: eng.MonitoringTemplate}
	other := f.actor(bob)
	page, e := f.s.List(ctx, other, otherScope, eng.Filter{Query: "local_template"})
	must(t, e)
	if page.Total != 0 {
		t.Fatal("cross-tenant search disclosure")
	}
	_, e = f.s.Preview(ctx, other, otherScope, copy.DefinitionID, eng.PreviewInput{VersionID: private.VersionID})
	code(t, e, "not_found")
	_, e = f.s.Diff(ctx, other, otherScope, copy.DefinitionID, private.VersionID, private.VersionID, 0)
	code(t, e, "not_found")
	_, e = f.s.Impact(ctx, other, otherScope, copy.DefinitionID, private.VersionID)
	code(t, e, "not_found")
	_, e = f.s.Copy(ctx, other, otherScope, eng.CopyInput{WriteMeta: f.wm(t), Code: "stolen", SourceVersion: private.VersionID}, access.Meta{})
	code(t, e, "not_found")
	_, e = f.s.Detail(ctx, a, scope, copy.DefinitionID)
	code(t, e, "tenant_not_found")
	// Source status is provenance, not an actual dependency of the copy.
	_, e = f.s.SetState(ctx, a, eng.Scope{Kind: eng.MonitoringTemplate}, tpl.DefinitionID, tpl.VersionID, eng.StateInput{WriteMeta: f.wm(t), Revision: 1, Enabled: false}, access.Meta{})
	must(t, e)
	d, e := f.s.Detail(ctx, f.actor(alice), scope, copy.DefinitionID)
	must(t, e)
	if !d.Versions[0].Usable {
		t.Fatal("copy depends on provenance")
	}
	// Every transitive dependency is checked, while historical interpretation remains readable.
	_, e = f.s.SetState(ctx, a, eng.Scope{Kind: eng.UnitDefinition}, u.DefinitionID, u.VersionID, eng.StateInput{WriteMeta: f.wm(t), Revision: 1, Enabled: false}, access.Meta{})
	must(t, e)
	d, e = f.s.Detail(ctx, f.actor(alice), scope, copy.DefinitionID)
	must(t, e)
	if d.Versions[0].Usable || len(d.Dependencies) != 3 {
		t.Fatal("transitive closure", d)
	}
	_, e = f.s.Copy(ctx, f.actor(alice), scope, eng.CopyInput{WriteMeta: f.wm(t), Code: "bypass", SourceVersion: private.VersionID}, access.Meta{})
	code(t, e, "dependency_unavailable")
	_, e = f.s.Detail(ctx, f.actor(alice), eng.Scope{Tenant: ta.TenantID, Catalog: true, Kind: eng.TypeDefinition}, typ.DefinitionID)
	code(t, e, "not_found")
	preview, e := f.s.Preview(ctx, f.actor(alice), scope, copy.DefinitionID, eng.PreviewInput{VersionID: private.VersionID, TypeVersion: typ.VersionID, Sample: &eng.Sample{Values: map[string]any{"name": "路段"}, Nodes: []eng.Node{{ID: "n1", Parent: "root", Category: "layer", Values: map[string]any{"thickness": 0}}}}})
	must(t, e)
	if !preview.Valid {
		t.Fatal(preview.Issues)
	}
	_, e = f.s.SetState(ctx, a, eng.Scope{Kind: eng.UnitDefinition}, u.DefinitionID, u.VersionID, eng.StateInput{WriteMeta: f.wm(t), Revision: 2, Enabled: true}, access.Meta{})
	must(t, e)
	draft, e := f.s.NewDraft(ctx, a, eng.Scope{Kind: eng.TypeDefinition}, typ.DefinitionID, eng.NewDraftInput{RevisionInput: eng.RevisionInput{WriteMeta: f.wm(t), Revision: typ.Revision}, VersionID: typ.VersionID}, access.Meta{})
	must(t, e)
	c.Name = "新名称"
	saved, e := f.s.Save(ctx, a, eng.Scope{Kind: eng.TypeDefinition}, typ.DefinitionID, eng.SaveInput{WriteMeta: f.wm(t), Revision: draft.Revision, Content: c}, access.Meta{})
	must(t, e)
	f.publish(t, eng.TypeDefinition, saved)
	d, e = f.s.Detail(ctx, f.actor(alice), scope, copy.DefinitionID)
	must(t, e)
	for _, v := range d.Dependencies {
		if v.ID == typ.VersionID && v.Content.Name != "任意新类型" {
			t.Fatal("historical name changed")
		}
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE engineering_versions SET content='{}' WHERE id=$1`, typ.VersionID)
	if e == nil {
		t.Fatal("published mutation allowed")
	}
	_, e = f.s.SaveCategory(ctx, a, eng.CategoryInput{WriteMeta: f.wm(t), Key: "length", Name: "改义", Description: "新含义", Revision: 1}, access.Meta{})
	code(t, e, "engineering_constraint")
}
func TestConcurrencyReceiptsAndRollback(t *testing.T) {
	f := setup(t)
	a := f.actor(f.p)
	scope := eng.Scope{Kind: eng.TypeDefinition}
	v := f.create(t, eng.TypeDefinition, "parallel", simple("并发类型"))
	inputs := []eng.SaveInput{{WriteMeta: f.wm(t), Revision: 1, Content: simple("编辑 A")}, {WriteMeta: f.wm(t), Revision: 1, Content: simple("编辑 B")}}
	results := make([]eng.Result, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = f.s.Save(ctx, a, scope, v.DefinitionID, inputs[i], access.Meta{})
		}(i)
	}
	wg.Wait()
	success := 0
	for _, e := range errs {
		if e == nil {
			success++
		} else {
			code(t, e, "version_conflict")
		}
	}
	if success != 1 {
		t.Fatal(errs)
	}
	in := eng.RevisionInput{WriteMeta: f.wm(t), Revision: 2}
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = f.s.Publish(ctx, a, scope, v.DefinitionID, in, access.Meta{})
		}(i)
	}
	wg.Wait()
	for _, e := range errs {
		must(t, e)
	}
	if results[0].VersionID != results[1].VersionID {
		t.Fatal("duplicate publication")
	}
	in.Reason = "different"
	_, e := f.s.Publish(ctx, a, scope, v.DefinitionID, in, access.Meta{})
	code(t, e, "request_payload_conflict")
	var n int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events WHERE event_type='engineering.publish' AND object_id=$1`, v.DefinitionID).Scan(&n))
	if n != 1 {
		t.Fatal("duplicate audit", n)
	}
	// Failed publication consumes neither draft nor receipt and cannot leave a version.
	bad := simple("结构错误")
	bad.Components = []eng.Component{{Key: "orphan", Name: "孤立"}}
	b := f.create(t, eng.TypeDefinition, "invalid_tree", bad)
	_, e = f.s.Publish(ctx, a, scope, b.DefinitionID, eng.RevisionInput{WriteMeta: f.wm(t), Revision: 1}, access.Meta{})
	code(t, e, "definition_invalid")
	d, e := f.s.Detail(ctx, a, scope, b.DefinitionID)
	must(t, e)
	if d.Draft == nil || len(d.Versions) != 0 {
		t.Fatal("partial publication")
	}
	del := eng.RevisionInput{WriteMeta: f.wm(t), Revision: 1}
	_, e = f.s.DeleteDraft(ctx, a, scope, b.DefinitionID, del, access.Meta{})
	must(t, e)
	_, e = f.s.DeleteDraft(ctx, a, scope, b.DefinitionID, del, access.Meta{})
	must(t, e)
	_, e = f.s.Create(ctx, a, scope, eng.CreateInput{WriteMeta: f.wm(t), Code: "invalid_tree", Content: simple("reuse")}, access.Meta{})
	code(t, e, "engineering_conflict")
}
func TestFieldAndStructureRules(t *testing.T) {
	fs := []eng.Field{{Key: "number", Name: "数值", Shape: "integer", Required: true}, {Key: "enabled", Name: "布尔", Shape: "boolean", Required: true}, {Key: "name", Name: "名称", Shape: "text", Required: true, MinLength: ptr(1), MaxLength: ptr(1)}, {Key: "date", Name: "日期", Shape: "date"}, {Key: "time", Name: "时间", Shape: "datetime"}, {Key: "tags", Name: "枚举", Shape: "multi_enum", Required: true, Options: []eng.Option{{Key: "ok", Name: "正常"}}}}
	good := map[string]any{"number": 0, "enabled": false, "name": "😀", "date": "2024-02-29", "time": "2026-11-10T09:30:00+08:00", "tags": []string{"ok"}}
	if issues := eng.ValidateValues(fs, good, "/sample"); len(issues) > 0 {
		t.Fatal(issues)
	}
	for k, v := range map[string]any{"number": 9007199254740992.0, "enabled": nil, "name": "😀好", "date": "2025-02-29", "time": "2026-11-10T09:30:00", "tags": []string{"ok", "ok"}} {
		b, _ := json.Marshal(good)
		var values map[string]any
		must(t, json.Unmarshal(b, &values))
		values[k] = v
		if issues := eng.ValidateValues(fs, values, "/sample"); len(issues) == 0 {
			t.Fatal("invalid accepted", k)
		}
	}
	delete(good, "enabled")
	if len(eng.ValidateValues(fs, good, "/sample")) == 0 {
		t.Fatal("missing false accepted")
	}
	c := simple("树")
	c.Components = []eng.Component{{Key: "child", Name: "子节点"}}
	c.Relations = []eng.Relation{{Parent: "root", Child: "child", MinCount: 1, MaxCount: ptr(2)}}
	if len(eng.ValidateContent(eng.TypeDefinition, c)) != 0 {
		t.Fatal("valid definition")
	}
	if len(eng.ValidateSample(c, eng.Sample{})) == 0 {
		t.Fatal("required child ignored")
	}
	for name, nodes := range map[string][]eng.Node{
		"duplicate parent": {{ID: "one", Parent: "root", Category: "child"}, {ID: "one", Parent: "one", Category: "child"}},
		"missing parent":   {{ID: "one", Parent: "missing", Category: "child"}},
		"illegal parent":   {{ID: "one", Parent: "root", Category: "child"}, {ID: "two", Parent: "one", Category: "child"}},
		"cycle":            {{ID: "one", Parent: "two", Category: "child"}, {ID: "two", Parent: "one", Category: "child"}},
		"too many":         {{ID: "one", Parent: "root", Category: "child"}, {ID: "two", Parent: "root", Category: "child"}, {ID: "three", Parent: "root", Category: "child"}},
	} {
		if len(eng.ValidateSample(c, eng.Sample{Nodes: nodes})) == 0 {
			t.Fatal("invalid structure accepted", name)
		}
	}
	c.Relations = append(c.Relations, eng.Relation{Parent: "child", Child: "child"})
	if len(eng.ValidateContent(eng.TypeDefinition, c)) == 0 {
		t.Fatal("cycle accepted")
	}
}

func TestSharedFieldCorpus(t *testing.T) {
	raw, e := os.ReadFile("../../../../tests/fixtures/engineering-values.json")
	must(t, e)
	var cases []struct {
		Name   string         `json:"name"`
		Fields []eng.Field    `json:"fields"`
		Values map[string]any `json:"values"`
		Valid  bool           `json:"valid"`
	}
	must(t, json.Unmarshal(raw, &cases))
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			issues := eng.ValidateValues(c.Fields, c.Values, "/sample/values")
			if (len(issues) == 0) != c.Valid {
				t.Fatalf("expected %v, got %v", c.Valid, issues)
			}
		})
	}
}

func TestCurrentPermissionsReceiptsAndAuditScope(t *testing.T) {
	f := setup(t)
	admin := f.actor(f.p)
	person := f.member(t, "Editor")
	actor := f.actor(person)
	as := f.env.Service.Access()
	meta := access.Meta{Reason: "工程权限测试"}
	role, e := as.CreateRole(ctx, admin, "仅编制", "", meta)
	must(t, e)
	role, e = as.ChangeRole(ctx, admin, role.ID, access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: role.Version, Permissions: []access.Code{access.EngineeringTypeRead, access.EngineeringTypeEdit}}, meta)
	must(t, e)
	target, e := as.Account(ctx, admin, person.Account.ID)
	must(t, e)
	_, e = as.AssignRoles(ctx, admin, target.ID, target.Version, []string{role.ID}, meta)
	must(t, e)
	scope := eng.Scope{Kind: eng.TypeDefinition}
	in := eng.CreateInput{WriteMeta: f.wm(t), Code: "editable_only", Content: simple("编辑权限不能发布")}
	created, e := f.s.Create(ctx, actor, scope, in, meta)
	must(t, e)
	receipt, e := f.s.Receipt(ctx, actor, scope, in.RequestID)
	must(t, e)
	if receipt.DefinitionID != created.DefinitionID {
		t.Fatal("wrong receipt")
	}
	_, e = f.s.Publish(ctx, actor, scope, created.DefinitionID, eng.RevisionInput{WriteMeta: f.wm(t), Revision: created.Revision}, meta)
	code(t, e, "permission_denied")
	_, e = as.ChangeRole(ctx, admin, role.ID, access.OpRoleSetStatus, access.RoleChange{ExpectedVersion: role.Version, Status: "disabled"}, meta)
	must(t, e)
	_, e = f.s.Detail(ctx, actor, scope, created.DefinitionID)
	code(t, e, "permission_denied")
	_, e = f.s.Create(ctx, actor, scope, in, meta)
	code(t, e, "permission_denied")
	_, e = f.s.Receipt(ctx, actor, scope, in.RequestID)
	code(t, e, "permission_denied")
	// The same platform administrator has platform tenant-audit privileges, but no private template access.
	alice := f.member(t, "TenantAdmin")
	ts := tenancy.New(as)
	tenant, e := ts.Create(ctx, admin, tenancy.CreateInput{Fields: tenancy.Fields{Name: "模板审计租户", Code: "audit-tenant"}, AdministratorID: alice.Account.ID, RequestID: f.wm(t).RequestID, Reason: "工程审计"}, meta)
	must(t, e)
	privateScope := eng.Scope{Tenant: tenant.TenantID, Kind: eng.MonitoringTemplate}
	private, e := f.s.Create(ctx, f.actor(alice), privateScope, eng.CreateInput{WriteMeta: f.wm(t), Code: "private_secret", Content: simple("租户私有标题")}, meta)
	must(t, e)
	platformEvents, e := as.AuditEvents(ctx, admin, access.Filter{ObjectID: private.DefinitionID})
	must(t, e)
	if len(platformEvents.Items) != 0 {
		t.Fatal("platform audit disclosed private engineering change")
	}
	tenantEvents, e := ts.Audit(ctx, f.actor(alice), tenant.TenantID, tenancy.Filter{ObjectID: private.DefinitionID})
	must(t, e)
	if len(tenantEvents.Items) != 1 || tenantEvents.Items[0].Event != "engineering.create" {
		t.Fatal("missing tenant audit", tenantEvents)
	}
	impact, e := ts.PreviewStatus(ctx, admin, tenant.TenantID, "disabled")
	must(t, e)
	_, e = ts.ChangeStatus(ctx, admin, tenant.TenantID, "disabled", tenancy.StatusInput{Version: impact.Version, Hash: impact.Hash, Reason: "立即限制工程目录", Notice: ptr("工程访问暂时停止")}, meta)
	must(t, e)
	_, e = f.s.Detail(ctx, f.actor(alice), privateScope, private.DefinitionID)
	code(t, e, "tenant_disabled")
}
func TestFailedAuditRollsBackWholePublication(t *testing.T) {
	f := setup(t)
	v := f.create(t, eng.TypeDefinition, "atomic", simple("原子发布"))
	a := f.actor(f.p)
	scope := eng.Scope{Kind: eng.TypeDefinition}
	in := eng.RevisionInput{WriteMeta: f.wm(t), Revision: 1}
	_, e := f.env.Pool.Exec(ctx, `CREATE FUNCTION reject_engineering_publish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='engineering.publish' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_engineering_publish BEFORE INSERT ON account_security_events FOR EACH ROW EXECUTE FUNCTION reject_engineering_publish()`)
	must(t, e)
	_, e = f.s.Publish(ctx, a, scope, v.DefinitionID, in, access.Meta{})
	code(t, e, "internal_error")
	d, e := f.s.Detail(ctx, a, scope, v.DefinitionID)
	must(t, e)
	if d.Draft == nil || len(d.Versions) != 0 || d.Revision != 1 {
		t.Fatal("publication leaked after rollback")
	}
	_, e = f.s.Receipt(ctx, a, scope, in.RequestID)
	code(t, e, "not_found")
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER reject_engineering_publish ON account_security_events; DROP FUNCTION reject_engineering_publish()`)
	must(t, e)
	_, e = f.s.Publish(ctx, a, scope, v.DefinitionID, in, access.Meta{})
	must(t, e)
}
