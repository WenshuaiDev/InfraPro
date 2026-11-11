package resources_test

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func (f resourceFixture) reader(t *testing.T) (access.Actor, string, string) {
	t.Helper()
	p := f.member(t, "ResourceReader")
	tx, e := f.env.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	_, e = access.Lock(ctx, tx, true)
	must(t, e)
	var membership, role string
	must(t, tx.QueryRow(ctx, `INSERT INTO tenant_memberships(tenant_id,account_id,joined_by,status_changed_by) VALUES($1,$2,$3,$3) RETURNING id::text`, f.tenant, p.Account.ID, f.actor.ID).Scan(&membership))
	must(t, tx.QueryRow(ctx, `INSERT INTO roles(binding_scope,owner_tenant_id,name,name_normalized) VALUES('tenant',$1,'Resource reader','resource reader') RETURNING id::text`, f.tenant).Scan(&role))
	_, e = tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,'tenant')`, role, access.TenantEngineeringResourceRead)
	must(t, e)
	_, e = tx.Exec(ctx, `INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) SELECT $1,$2,id,$3 FROM roles WHERE builtin_key='tenant_member' OR id=$4`, f.tenant, membership, f.actor.ID, role)
	must(t, e)
	must(t, tx.Commit(ctx))
	return f.fixture.actor(p), role, membership
}
func TestAuthorizationHistoryInterpretationAndAuditIsolation(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "Private.Facility")
	ft := target(facility, res.Facility, facility.ID)

	reader, role, member := f.reader(t)
	d, e := f.srv.Detail(ctx, reader, res.TenantScope(f.tenant), ft)
	must(t, e)
	if len(d.Definitions) == 0 {
		t.Fatal("reader cannot interpret adopted definition")
	}
	_, e = f.s.Detail(ctx, reader, eng.Scope{Tenant: f.tenant, Catalog: true, Kind: eng.TypeDefinition}, d.Definitions[0].DefinitionID)
	code(t, e, "permission_denied")
	raw, _ := json.Marshal(map[string]any{"request_id": f.wm(t).RequestID, "expected_revision": 1, "name": "unauthorized"})
	_, e = f.srv.Mutate(ctx, reader, res.TenantScope(f.tenant), ft, "update", raw, access.Meta{})
	code(t, e, "permission_denied")
	_, e = f.srv.Detail(ctx, f.fixture.actor(f.p), res.TenantScope(f.tenant), ft)
	code(t, e, "tenant_not_found")
	// Tenant security audit is independent from business history and excluded before platform filters.
	platform := f.fixture.actor(f.p)
	for _, filter := range []access.Filter{{Query: "Private.Facility"}, {ObjectType: "engineering_facility", ObjectID: facility.ID}, {Event: "resource.create"}} {
		p, e := f.env.Service.Access().AuditEvents(ctx, platform, filter)
		must(t, e)
		if len(p.Items) != 0 {
			t.Fatal("platform leaked private event", p)
		}
	}
	var event string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM account_security_events WHERE object_id=$1`, facility.ID).Scan(&event))
	page, e := f.env.Service.Access().AuditEvents(ctx, platform, access.Filter{Cursor: event})
	if e == nil && len(page.Items) > 0 {
		for _, item := range page.Items {
			if item.ObjectType == "engineering_facility" {
				t.Fatal("cursor leaked resource")
			}
		}
	}
	// Removing security events never erases resource history.
	_, e = f.env.Pool.Exec(ctx, `DELETE FROM account_security_events WHERE object_id=$1`, facility.ID)
	must(t, e)
	h, e := f.srv.Changes(ctx, reader, res.TenantScope(f.tenant), ft, res.Filter{})
	must(t, e)
	if len(h.Items) != 1 {
		t.Fatal(h)
	}
	tx, e := f.env.Pool.Begin(ctx)
	must(t, e)
	_, e = access.Lock(ctx, tx, true)
	must(t, e)
	_, e = tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1`, role)
	must(t, e)
	done := make(chan error, 1)
	go func() { _, err := f.srv.Detail(ctx, reader, res.TenantScope(f.tenant), ft); done <- err }()
	select {
	case e := <-done:
		t.Fatalf("authorization lock bypassed: %v", e)
	case <-time.After(30 * time.Millisecond):
	}
	must(t, tx.Commit(ctx))
	code(t, <-done, "permission_denied")
	_, e = f.env.Pool.Exec(ctx, `UPDATE tenant_memberships SET status='suspended' WHERE id=$1`, member)
	must(t, e)
	_, e = f.srv.Detail(ctx, reader, res.TenantScope(f.tenant), ft)
	code(t, e, "tenant_not_found")
}
func TestDefinitionAdoptionLocksAndRetainedResources(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "F")
	ft := target(facility, res.Facility, facility.ID)

	tx, e := f.env.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR UPDATE`)
	must(t, e)
	_, e = tx.Exec(ctx, `UPDATE engineering_version_states SET enabled=false,revision=revision+1 WHERE version_id=$1`, f.typ)
	must(t, e)
	req := map[string]any{"code": "BLOCKED", "name": "停用竞争", "type_version_id": f.typ, "values": map[string]any{"length": 0}, "request_id": f.wm(t).RequestID}
	done := make(chan error, 1)
	go func() { _, err := f.write(t, res.Target{Kind: res.Facility}, "create", req); done <- err }()
	select {
	case e := <-done:
		t.Fatalf("catalog lock bypassed: %v", e)
	case <-time.After(30 * time.Millisecond):
	}
	must(t, tx.Commit(ctx))
	code(t, <-done, "definition_unavailable")
	_, e = f.write(t, ft, "update", map[string]any{"expected_revision": 1, "name": "既有设施仍可维护"})
	must(t, e)
	_, e = f.write(t, target(facility, res.Component, ""), "create", map[string]any{"expected_structure_revision": 1, "code": "C", "name": "既有类型构件", "category_key": "span"})
	must(t, e)
	var def string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT definition_id::text FROM engineering_versions WHERE id=$1`, f.typ).Scan(&def))
	impact, e := f.s.WithUsageReader(f.srv).Impact(ctx, f.fixture.actor(f.p), eng.Scope{Kind: eng.TypeDefinition}, def, f.typ)
	must(t, e)
	if !impact.ResourceReferences {
		t.Fatal("missing reference existence")
	}
}
func TestFailureRollbackAndReceiptAuthorization(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "F")
	ft := target(facility, res.Facility, facility.ID)

	for _, table := range []string{"engineering_resource_changes", "account_security_events"} {
		_, e := f.env.Pool.Exec(ctx, `CREATE FUNCTION fail_resource_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test failure'; END $$; CREATE TRIGGER fail_resource BEFORE INSERT ON `+table+` FOR EACH ROW EXECUTE FUNCTION fail_resource_write()`)
		must(t, e)
		req := map[string]any{"expected_revision": 1, "name": "Atomic change", "request_id": f.wm(t).RequestID}
		_, e = f.write(t, ft, "update", req)
		code(t, e, "unavailable")
		d, e := f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), ft)
		must(t, e)
		if d.Revision != 1 || d.Name == "Atomic change" {
			t.Fatal("partial commit")
		}
		_, e = f.srv.Receipt(ctx, f.actor, res.TenantScope(f.tenant), req["request_id"].(string))
		code(t, e, "resource_not_found")
		_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER fail_resource ON `+table+`; DROP FUNCTION fail_resource_write()`)
		must(t, e)
	}
	req := map[string]any{"expected_revision": 1, "name": "Atomic change", "request_id": f.wm(t).RequestID}
	out, e := f.write(t, ft, "update", req)
	must(t, e)
	receipt, e := f.srv.Receipt(ctx, f.actor, res.TenantScope(f.tenant), req["request_id"].(string))
	must(t, e)
	if receipt.ID != out.ID {
		t.Fatal(receipt)
	}
	reader, _, _ := f.reader(t)
	_, e = f.srv.Receipt(ctx, reader, res.TenantScope(f.tenant), req["request_id"].(string))
	code(t, e, "resource_not_found")
	// A current session check is required again even for a committed receipt.
	denied := f.actor
	denied.Check = func(_ context.Context, _ pgx.Tx, _ bool) error { return access.ErrDenied }
	_, e = f.srv.Receipt(ctx, denied, res.TenantScope(f.tenant), req["request_id"].(string))
	code(t, e, "permission_denied")
}
func TestPagedStructureBeyondPreviewLimit(t *testing.T) {
	f := resourcesSetup(t)
	c := simple("不限数量的新类型")
	c.Components = []eng.Component{{Key: "part", Name: "部件", Fields: []eng.Field{}}}
	c.Relations = []eng.Relation{{Parent: "root", Child: "part"}}
	v := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "large_type", c))
	f.typ = v.VersionID
	facility, e := f.write(t, res.Target{Kind: res.Facility}, "create", map[string]any{"code": "MANY", "name": "大结构", "type_version_id": f.typ})
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO engineering_components(tenant_id,facility_id,code,name,category_key,created_by,updated_by) SELECT $1,$2,'C'||n,'部件'||n,'part',$3,$3 FROM generate_series(1,501) n`, f.tenant, facility.ID, f.actor.ID)
	must(t, e)
	target := target(facility, res.Component, "")

	count := 0
	filter := res.Filter{Parent: "root", Limit: 100}
	seen := map[string]bool{}
	for {
		page, e := f.srv.List(ctx, f.actor, res.TenantScope(f.tenant), target, filter)
		must(t, e)
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate page item")
			}
			seen[item.ID] = true
			count++
		}
		if page.NextCursor == "" {
			break
		}
		filter.Cursor = page.NextCursor
	}
	if count != 501 {
		t.Fatalf("truncated tree: %d", count)
	}
	filter.Query = "changed filter"
	_, e = f.srv.List(ctx, f.actor, res.TenantScope(f.tenant), target, filter)
	code(t, e, "invalid_request")
}
func TestMoveRestoreVersionPinningAndScopeGuards(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "F")
	ft := target(facility, res.Facility, facility.ID)

	makeComponent := func(code, kind string, parent any, structure int) res.Result {
		t.Helper()
		v, e := f.write(t, target(facility, res.Component, ""), "create", map[string]any{"code": code, "name": code, "category_key": kind, "parent_component_id": parent, "expected_structure_revision": structure})
		must(t, e)
		return v
	}
	a := makeComponent("A", "span", nil, 1)
	b := makeComponent("B", "span", nil, 2)
	child := makeComponent("CHILD", "beam", a.ID, 3)
	p, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "P", "name": "测点", "component_id": child.ID})
	must(t, e)
	moved, e := f.write(t, target(facility, res.Component, child.ID), "update", map[string]any{"expected_revision": 1, "expected_structure_revision": 4, "parent_component_id": b.ID})
	must(t, e)
	pt := target(facility, res.Point, p.ID)

	d, e := f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), pt)
	must(t, e)
	if len(d.Ancestors) != 3 || d.Ancestors[1].ID != b.ID || d.Ancestors[2].ID != child.ID {
		t.Fatal(d.Ancestors)
	}
	own, e := f.write(t, pt, "retire", map[string]any{"expected_revision": 1, "reason": "自行退役"})
	must(t, e)
	off, e := f.write(t, ft, "retire", map[string]any{"expected_revision": 1, "reason": "设施退役"})
	must(t, e)
	_, e = f.write(t, pt, "restore", map[string]any{"expected_revision": own.Revision, "reason": "不能越过祖先"})
	code(t, e, "ancestor_retired")
	_, e = f.write(t, ft, "restore", map[string]any{"expected_revision": off.Revision, "reason": "恢复设施"})
	must(t, e)
	d, e = f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), pt)
	must(t, e)
	if d.Status != "retired" {
		t.Fatal("restoring ancestor resurrected descendant")
	}
	retired, e := f.write(t, target(facility, res.Component, a.ID), "retire", map[string]any{"expected_revision": 1, "expected_structure_revision": moved.StructureRevision, "reason": "退出使用"})
	must(t, e)
	_ = makeComponent("C", "span", nil, retired.StructureRevision)
	_, e = f.write(t, target(facility, res.Component, a.ID), "restore", map[string]any{"expected_revision": retired.Revision, "expected_structure_revision": retired.StructureRevision + 1, "reason": "不能绕过上限"})
	code(t, e, "resource_invalid")
	// Publish a changed type without reinterpreting an adopted V1.
	var definition string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT definition_id::text FROM engineering_versions WHERE id=$1`, f.typ).Scan(&definition))
	engScope := eng.Scope{Kind: eng.TypeDefinition}
	platform := f.fixture.actor(f.p)
	original, e := f.s.Detail(ctx, platform, engScope, definition)
	must(t, e)
	draft, e := f.s.NewDraft(ctx, platform, engScope, definition, eng.NewDraftInput{RevisionInput: eng.RevisionInput{WriteMeta: f.wm(t), Revision: original.Revision}, VersionID: f.typ}, access.Meta{})
	must(t, e)
	content := original.Versions[0].Content
	content.Name = "发布后的新解释"
	saved, e := f.s.Save(ctx, platform, engScope, definition, eng.SaveInput{WriteMeta: f.wm(t), Revision: draft.Revision, Content: content}, access.Meta{})
	must(t, e)
	f.publish(t, eng.TypeDefinition, saved)
	old, e := f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), ft)
	must(t, e)
	if old.TypeVersionID != f.typ {
		t.Fatal("adopted type followed latest")
	}
	for _, v := range old.Definitions {
		if v.ID == f.typ && v.Content.Name == content.Name {
			t.Fatal("V1 reinterpreted")
		}
	}
	other, e := tenancy.New(f.env.Service.Access()).Create(ctx, platform, tenancy.CreateInput{Fields: tenancy.Fields{Name: "另一租户", Code: "another-tenant"}, AdministratorID: f.actor.ID, RequestID: f.wm(t).RequestID, Reason: "隔离验证"}, access.Meta{})
	must(t, e)
	foreign := ft

	_, e = f.srv.Detail(ctx, f.actor, res.TenantScope(other.TenantID), foreign)
	code(t, e, "resource_not_found")
	_, e = f.srv.Changes(ctx, f.actor, res.TenantScope(other.TenantID), foreign, res.Filter{})
	code(t, e, "resource_not_found")
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO engineering_points(tenant_id,facility_id,code,name,created_by,updated_by) VALUES($1,$2,'X','非法跨租户',$3,$3)`, other.TenantID, facility.ID, f.actor.ID)
	if e == nil {
		t.Fatal("cross-tenant database reference accepted")
	}
}
