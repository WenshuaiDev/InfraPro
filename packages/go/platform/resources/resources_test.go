package resources_test

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
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
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
	var re *res.Error
	got := identity.PublicError(e).Code
	if errors.As(e, &de) {
		got = de.Code
	}
	if errors.As(e, &re) {
		got = re.Code
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

type resourceFixture struct {
	fixture
	srv                       *res.Service
	actor                     access.Actor
	tenant, typ, metric, unit string
}

func resourcesSetup(t *testing.T) resourceFixture {
	f := setup(t)
	a := f.actor(f.p)
	user := f.member(t, "ResourceOwner")
	ts := tenancy.New(f.env.Service.Access())
	tenant, e := ts.Create(ctx, a, tenancy.CreateInput{Fields: tenancy.Fields{Name: "资源测试租户", Code: "resources"}, AdministratorID: user.Account.ID, RequestID: f.wm(t).RequestID, Reason: "资源验证"}, access.Meta{})
	must(t, e)
	_, e = f.s.SaveCategory(ctx, a, eng.CategoryInput{WriteMeta: f.wm(t), Key: "length", Name: "长度", Description: "空间长度"}, access.Meta{})
	must(t, e)
	u := simple("毫米")
	u.Unit = &eng.Unit{Symbol: "mm", Category: "length"}
	unit := f.publish(t, eng.UnitDefinition, f.create(t, eng.UnitDefinition, "mm", u))
	c := simple("新配置类型")
	c.Fields = []eng.Field{{Key: "length", Name: "长度", Description: "空间长度", Shape: "decimal", Required: true}}
	c.Components = []eng.Component{{Key: "span", Name: "跨", Fields: []eng.Field{}}, {Key: "beam", Name: "梁", Fields: []eng.Field{}}}
	c.Relations = []eng.Relation{{Parent: "root", Child: "span", MinCount: 2, MaxCount: ptr(2)}, {Parent: "span", Child: "beam", MinCount: 0, MaxCount: ptr(3)}}
	typ := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "unknown_type", c))
	m := simple("位移")
	m.Metric = &eng.Metric{Shape: "decimal", Category: "length", StandardUnit: unit.VersionID, AllowedUnits: []string{unit.VersionID}}
	metric := f.publish(t, eng.MetricDefinition, f.create(t, eng.MetricDefinition, "displacement", m))
	return resourceFixture{f, res.New(f.env.Service.Access()), f.actor(user), tenant.TenantID, typ.VersionID, metric.VersionID, unit.VersionID}
}
func (f resourceFixture) write(t *testing.T, target res.Target, op string, in map[string]any) (res.Result, error) {
	t.Helper()

	if _, ok := in["request_id"]; !ok {
		in["request_id"] = f.wm(t).RequestID
	}
	if op != "create" && in["preview_hash"] == nil {
		if in["reason"] == nil {
			in["reason"] = "资源修改自验"
		}
		raw, _ := json.Marshal(in)
		preview, e := f.srv.PreviewMutation(ctx, f.actor, res.TenantScope(f.tenant), target, op, raw)
		if e != nil {
			return res.Result{}, e
		}
		if preview.Required {
			in["preview_hash"] = preview.Hash
		}
	}
	b, _ := json.Marshal(in)
	return f.srv.Mutate(ctx, f.actor, res.TenantScope(f.tenant), target, op, b, access.Meta{})
}
func (f resourceFixture) facility(t *testing.T, code string) res.Result {
	t.Helper()
	r, e := f.write(t, res.Target{Kind: res.Facility}, "create", map[string]any{"code": code, "name": "测试设施", "type_version_id": f.typ, "values": map[string]any{"length": 0}})
	must(t, e)
	return r
}
func target(f res.Result, k res.Kind, id string) res.Target {
	return res.Target{Kind: k, Facility: f.ID, ID: id}
}
func TestResourceLifecycleAndStableHistory(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "F-01")
	ft := target(facility, res.Facility, facility.ID)

	d, e := f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), ft)
	must(t, e)
	if d.Status != "normal" || d.Values["length"] != float64(0) {
		t.Fatal(d)
	}
	component, e := f.write(t, target(facility, res.Component, ""), "create", map[string]any{"code": "C.1", "name": "第一跨", "category_key": "span", "expected_structure_revision": 1})
	must(t, e) // min=2 intentionally does not block.
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P-1", "name": "测点", "component_id": component.ID})
	must(t, e)
	pt := target(facility, res.Point, point.ID)

	pd, e := f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), pt)
	must(t, e)
	if len(pd.Ancestors) != 2 || len(pd.Position.LinearReferences) != 0 {
		t.Fatal(pd)
	}
	ct := target(facility, res.Channel, "")
	ct.Point = point.ID
	payload := map[string]any{"code": "CH-1", "name": "横向位移", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time", "sampling": map[string]any{"mode": "periodic", "interval_seconds": 1}}
	ch, e := f.write(t, ct, "create", payload)
	must(t, e)
	again, e := f.write(t, ct, "create", payload)
	must(t, e)
	if again.ID != ch.ID || !again.Replayed {
		t.Fatal(again)
	}
	payload["name"] = "不同内容"
	_, e = f.write(t, ct, "create", payload)
	code(t, e, "request_payload_conflict")
	ct.ID = ch.ID
	_, e = f.write(t, ct, "update", map[string]any{"expected_revision": 1, "metric_version_id": f.metric})
	code(t, e, "invalid_request")
	// Reparent a point keeps channel identities and gives an independent history revision.
	moved, e := f.write(t, pt, "update", map[string]any{"expected_revision": 1, "component_id": nil, "position": map[string]any{"geographic": map[string]any{"crs": "EPSG:4326", "longitude": 0, "latitude": 0}}})
	must(t, e)
	_, e = f.write(t, pt, "update", map[string]any{"expected_revision": 1, "name": "过期"})
	code(t, e, "resource_revision_conflict")
	no, e := f.write(t, pt, "update", map[string]any{"expected_revision": moved.Revision, "name": "测点"})
	must(t, e)
	if no.Changed || no.Revision != moved.Revision {
		t.Fatal(no)
	}
	_, e = f.write(t, pt, "delete", map[string]any{"expected_revision": moved.Revision, "reason": "误建"})
	code(t, e, "resource_referenced")
	retired, e := f.write(t, ft, "retire", map[string]any{"expected_revision": 1, "reason": "停用设施"})
	must(t, e)
	_, e = f.write(t, pt, "update", map[string]any{"expected_revision": moved.Revision, "name": "不能修改"})
	code(t, e, "ancestor_retired")
	restored, e := f.write(t, ft, "restore", map[string]any{"expected_revision": retired.Revision, "reason": "恢复使用"})
	must(t, e)
	_, e = f.write(t, ct, "delete", map[string]any{"expected_revision": ch.Revision, "reason": "误建通道"})
	must(t, e)
	_, e = f.write(t, pt, "delete", map[string]any{"expected_revision": moved.Revision, "reason": "误建测点"})
	must(t, e)
	_, e = f.write(t, target(facility, res.Component, component.ID), "delete", map[string]any{"expected_revision": component.Revision, "expected_structure_revision": component.StructureRevision, "reason": "误建构件"})
	must(t, e)
	_, e = f.write(t, ft, "delete", map[string]any{"expected_revision": restored.Revision, "reason": "误建设施"})
	must(t, e)
	_, e = f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), ft)
	code(t, e, "resource_not_found")
	history, e := f.srv.Changes(ctx, f.actor, res.TenantScope(f.tenant), ft, res.Filter{})
	must(t, e)
	if len(history.Items) != 11 {
		t.Fatalf("history count %d", len(history.Items))
	}
	_, e = f.write(t, res.Target{Kind: res.Facility}, "create", map[string]any{"code": "f-01", "name": "不可复用", "type_version_id": f.typ, "values": map[string]any{"length": 0}})
	code(t, e, "resource_code_taken")
}
func TestStructureConcurrencyAndDatabaseGuards(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "F")
	in := func(code string) map[string]any {
		return map[string]any{"code": code, "name": code, "category_key": "span", "expected_structure_revision": 1}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, name := range []string{"C1", "C2"} {
		payload := in(name)
		payload["request_id"] = f.wm(t).RequestID
		wg.Go(func() { _, e := f.write(t, target(facility, res.Component, ""), "create", payload); errs <- e })
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else {
			code(t, e, "structure_revision_conflict")
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	c2, e := f.write(t, target(facility, res.Component, ""), "create", map[string]any{"code": "C3", "name": "跨", "category_key": "span", "expected_structure_revision": 2})
	must(t, e)
	_, e = f.write(t, target(facility, res.Component, ""), "create", map[string]any{"code": "C4", "name": "跨", "category_key": "span", "expected_structure_revision": 3})
	code(t, e, "resource_invalid")
	_, e = f.write(t, target(facility, res.Component, c2.ID), "update", map[string]any{"expected_revision": 1, "expected_structure_revision": 3, "parent_component_id": c2.ID})
	code(t, e, "resource_invalid")
	_, e = f.env.Pool.Exec(ctx, `UPDATE engineering_components SET parent_component_id=id WHERE id=$1`, c2.ID)
	if e == nil {
		t.Fatal("SQL cycle accepted")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE engineering_facilities SET code='changed' WHERE id=$1`, facility.ID)
	if e == nil {
		t.Fatal("SQL identity changed")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE engineering_resource_changes SET operation='erased' WHERE facility_id=$1`, facility.ID)
	if e == nil {
		t.Fatal("SQL history rewritten")
	}
	// A second tenant's ID cannot be used in a nested authorized path.
	other := f.facility(t, "OTHER")
	_, e = f.write(t, target(other, res.Point, ""), "create", map[string]any{"code": "P", "name": "跨设施测点", "component_id": c2.ID})
	code(t, e, "resource_not_found")
}
