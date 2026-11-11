package resources_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func (f resourceFixture) projectIntent(t *testing.T, p projectFixture, target res.Target, fields map[string]any) map[string]any {
	t.Helper()
	d, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, p.id, target.Facility, res.AccessFilter{})
	must(t, e)
	fields["request_id"] = f.wm(t).RequestID
	fields["expected_access_revision"] = d.Policy.Revision
	if d.Link != nil {
		fields["expected_link_revision"] = d.Link.Revision
	}
	return fields
}
func (f resourceFixture) projectPreview(t *testing.T, p projectFixture, target res.Target, action string, fields map[string]any) (res.MutationPreview, json.RawMessage) {
	t.Helper()
	raw, _ := json.Marshal(fields)
	preview, e := f.srv.PreviewMutation(ctx, p.actor, res.ProjectScope(f.tenant, p.id), target, action, raw)
	must(t, e)
	fields["preview_hash"] = preview.Hash
	raw, _ = json.Marshal(fields)
	return preview, raw
}
func (f resourceFixture) projectStatus(t *testing.T, p projectFixture, op access.Operation) {
	t.Helper()
	s := projects.New(f.env.Service.Access())
	in := projects.Change{}
	pr, e := s.Preview(ctx, p.actor, f.tenant, p.id, "", op, in)
	must(t, e)
	if !pr.CanSubmit {
		t.Fatal(pr)
	}
	in.ChangeMeta = projects.ChangeMeta{WriteMeta: projects.WriteMeta{RequestID: f.wm(t).RequestID, Reason: "资源归档验证"}, Version: pr.Version, ProjectVersion: pr.ProjectVersion, Hash: pr.Hash}
	_, e = s.Change(ctx, p.actor, f.tenant, p.id, "", op, in, access.Meta{})
	must(t, e)
}
func TestSharedResourcePreviewConflictLifecycleArchiveAndReceipts(t *testing.T) {
	f := resourcesSetup(t)
	a := f.project(t, "ProjectA")
	b := f.project(t, "ProjectB")
	facility := f.facility(t, "SHARED")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"name": "公共测点", "code": "P"})
	must(t, e)
	pt := target(facility, res.Point, point.ID)
	for _, p := range []projectFixture{a, b} {
		in := f.accessInput(t, p, facility.ID, "create_link")
		in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Point, PointID: &point.ID, Capabilities: []string{"read", "update", "lifecycle"}}}
		f.grant(t, p, facility.ID, in)
	}
	fields := f.projectIntent(t, a, pt, map[string]any{"expected_revision": 1, "name": "A 修改公共资料", "reason": "A 私有原因"})
	raw, _ := json.Marshal(fields)
	_, e = f.srv.Mutate(ctx, a.actor, res.ProjectScope(f.tenant, a.id), pt, "update", raw, access.Meta{})
	code(t, e, "preview_required")
	pa, ra := f.projectPreview(t, a, pt, "update", fields)
	if !pa.Shared || !pa.Required || len(pa.Projects) > 0 {
		t.Fatal(pa)
	}
	_, rb := f.projectPreview(t, b, pt, "update", f.projectIntent(t, b, pt, map[string]any{"expected_revision": 1, "name": "B 并发修改", "reason": "B 私有原因"}))
	result, e := f.srv.Mutate(ctx, a.actor, res.ProjectScope(f.tenant, a.id), pt, "update", ra, access.Meta{})
	must(t, e)
	_, e = f.srv.Mutate(ctx, b.actor, res.ProjectScope(f.tenant, b.id), pt, "update", rb, access.Meta{})
	code(t, e, "resource_revision_conflict")
	replay, e := f.srv.Mutate(ctx, a.actor, res.ProjectScope(f.tenant, a.id), pt, "update", ra, access.Meta{})
	must(t, e)
	if !replay.Replayed || result.ID != point.ID {
		t.Fatal(replay)
	}
	h, e := f.srv.Changes(ctx, b.actor, res.ProjectScope(f.tenant, b.id), target(facility, res.Facility, facility.ID), res.Filter{})
	must(t, e)
	payload, _ := json.Marshal(h)
	if strings.Contains(string(payload), a.actor.ID) || strings.Contains(string(payload), "A 私有原因") || strings.Contains(string(payload), a.id) {
		t.Fatal("private history leaked", string(payload))
	}
	f.projectStatus(t, b, access.OpProjectArchive)
	d, e := f.srv.Detail(ctx, b.actor, res.ProjectScope(f.tenant, b.id), pt)
	must(t, e)
	if d.Actions.Edit || d.Actions.Retire {
		t.Fatal(d.Actions)
	}
	pr, retire := f.projectPreview(t, a, pt, "retire", f.projectIntent(t, a, pt, map[string]any{"expected_revision": 2, "reason": "退役共享测点"}))
	if pr.CanSubmit || pr.BlockedCode != "shared_resource_tenant_action_required" {
		t.Fatal(pr)
	}
	_, e = f.srv.Mutate(ctx, a.actor, res.ProjectScope(f.tenant, a.id), pt, "retire", retire, access.Meta{})
	code(t, e, "shared_resource_tenant_action_required")
	_, e = f.projectWrite(t, b, pt, "update", map[string]any{"expected_revision": 2, "name": "归档写"})
	code(t, e, "project_archived")
	in := f.accessInput(t, b, facility.ID, "revoke_link")
	f.grant(t, b, facility.ID, in)
	f.projectStatus(t, b, access.OpProjectRestore)
	_, e = f.srv.Detail(ctx, b.actor, res.ProjectScope(f.tenant, b.id), pt)
	code(t, e, "resource_not_found")
	pr, retire = f.projectPreview(t, a, pt, "retire", f.projectIntent(t, a, pt, map[string]any{"expected_revision": 2, "reason": "独用测点退役"}))
	if !pr.CanSubmit {
		t.Fatal(pr)
	}
	_, e = f.srv.Mutate(ctx, a.actor, res.ProjectScope(f.tenant, a.id), pt, "retire", retire, access.Meta{})
	must(t, e)
	in = f.accessInput(t, a, facility.ID, "revoke_link")
	f.grant(t, a, facility.ID, in)
	_, e = f.srv.Receipt(ctx, a.actor, res.ProjectScope(f.tenant, a.id), fields["request_id"].(string))
	code(t, e, "resource_not_found")
}
func TestProjectCreationRollsBackEveryCoupledWrite(t *testing.T) {
	for _, table := range []string{"project_resource_grants", "project_resource_access_changes", "engineering_resource_changes", "project_resource_operation_receipts"} {
		t.Run(table, func(t *testing.T) {
			f := resourcesSetup(t)
			a := f.project(t, "ProjectA")
			facility := f.facility(t, "ROLLBACK")
			in := f.accessInput(t, a, facility.ID, "create_link")
			in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read", "create_point"}}}
			f.grant(t, a, facility.ID, in)
			_, e := f.env.Pool.Exec(ctx, `CREATE FUNCTION reject_resource_test() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'deliberate test failure'; END$$; CREATE TRIGGER reject_resource BEFORE INSERT ON `+table+` FOR EACH ROW EXECUTE FUNCTION reject_resource_test()`)
			must(t, e)
			fields := f.projectIntent(t, a, target(facility, res.Point, ""), map[string]any{"name": "回滚点", "code": "ROLLBACK"})
			raw, _ := json.Marshal(fields)
			_, e = f.srv.Mutate(ctx, a.actor, res.ProjectScope(f.tenant, a.id), target(facility, res.Point, ""), "create", raw, access.Meta{})
			if e == nil {
				t.Fatal("expected atomic rollback")
			}
			var n int
			var rev string
			must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM engineering_points WHERE facility_id=$1),(SELECT revision::text FROM project_resource_policies WHERE project_id=$2)`, facility.ID, a.id).Scan(&n, &rev))
			if n != 0 || rev != "2" {
				t.Fatal("partial create", n, rev)
			}
		})
	}
}
func TestExplicitGrantPaginationCandidatesAndConstraints(t *testing.T) {
	f := resourcesSetup(t)
	a := f.project(t, "ProjectA")
	facility := f.facility(t, "LARGE")
	var ids []string
	// A larger fixture is inserted as setup; all authorization and reads use business interfaces.
	rows, e := f.env.Pool.Query(ctx, `INSERT INTO engineering_points(tenant_id,facility_id,code,name,created_by,updated_by) SELECT $1,$2,'P-'||n,'点 '||n,$3,$3 FROM generate_series(1,130) n RETURNING id::text`, f.tenant, facility.ID, f.actor.ID)
	must(t, e)
	for rows.Next() {
		var id string
		must(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	must(t, rows.Err())
	rows.Close()
	in := f.accessInput(t, a, facility.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	for i, id := range ids {
		if i%2 == 0 {
			in.Add = append(in.Add, res.GrantInput{Kind: res.Point, PointID: ptr(id), Capabilities: []string{"read"}})
		}
	}
	f.grant(t, a, facility.ID, in)
	count := 0
	cursor := ""
	for {
		page, e := f.srv.List(ctx, a.actor, res.ProjectScope(f.tenant, a.id), target(facility, res.Point, ""), res.Filter{Limit: 7, Cursor: cursor})
		must(t, e)
		count += len(page.Items)
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if count != 65 {
		t.Fatal("authorization after pagination", count)
	}
	d, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, a.id, facility.ID, res.AccessFilter{Limit: 3, CandidateKind: res.Point, Candidate: res.Filter{Limit: 9}})
	must(t, e)
	if len(d.Grants) != 3 || d.GrantNextCursor == "" || len(d.Candidates.Items) != 9 || len(d.CandidateGrants) > 9 {
		t.Fatal(d)
	}
	// Forbidden capability, wrong typed identity, immutable identity and cross-link project references.
	for _, sql := range []string{
		fmt.Sprintf(`UPDATE project_resource_grants SET can_create_channel=true WHERE link_id='%s' AND resource_kind='facility'`, d.Link.ID),
		fmt.Sprintf(`UPDATE project_resource_grants SET project_id=gen_random_uuid() WHERE link_id='%s'`, d.Link.ID),
		fmt.Sprintf(`UPDATE engineering_facilities SET created_project_id='%s' WHERE id='%s'`, a.id, facility.ID),
		fmt.Sprintf(`DELETE FROM project_facility_links WHERE id='%s'`, d.Link.ID),
	} {
		if _, e = f.env.Pool.Exec(ctx, sql); e == nil {
			t.Fatal("database accepted illegal write", sql)
		}
	}
	// Full current selection remains bounded and is not a silent first-page grant.
	in = f.accessInput(t, a, facility.ID, "change_grants")
	in.SelectCurrent = &res.CurrentSelection{RootKind: res.Facility, RootID: facility.ID, Kinds: map[res.Kind][]string{res.Point: {"read"}}}
	f.grant(t, a, facility.ID, in)
	page, e := f.srv.List(ctx, a.actor, res.ProjectScope(f.tenant, a.id), target(facility, res.Point, ""), res.Filter{Limit: 100})
	must(t, e)
	if len(page.Items) != 100 || page.NextCursor == "" {
		t.Fatal(page)
	}
}
func TestProjectReplayRequiresCurrentCreationPolicyButReceiptRemainsReadable(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "ProjectA")
	policy := f.accessInput(t, p, "", "policy")
	policy.AllowFacilityCreate = ptr(true)
	f.grant(t, p, "", policy)
	fields := f.projectIntent(t, p, res.Target{Kind: res.Facility}, map[string]any{"code": "REPLAY", "name": "原已创建设施", "type_version_id": f.typ, "values": map[string]any{"length": 0}})
	raw, _ := json.Marshal(fields)
	created, e := f.srv.Mutate(ctx, p.actor, res.ProjectScope(f.tenant, p.id), res.Target{Kind: res.Facility}, "create", raw, access.Meta{})
	must(t, e)
	replay, e := f.srv.Mutate(ctx, p.actor, res.ProjectScope(f.tenant, p.id), res.Target{Kind: res.Facility}, "create", raw, access.Meta{})
	must(t, e)
	if !replay.Replayed {
		t.Fatal(replay)
	}
	policy = f.accessInput(t, p, "", "policy")
	policy.AllowFacilityCreate = ptr(false)
	f.grant(t, p, "", policy)
	_, e = f.srv.Mutate(ctx, p.actor, res.ProjectScope(f.tenant, p.id), res.Target{Kind: res.Facility}, "create", raw, access.Meta{})
	code(t, e, "permission_denied")
	receipt, e := f.srv.Receipt(ctx, p.actor, res.ProjectScope(f.tenant, p.id), fields["request_id"].(string))
	must(t, e)
	if receipt.ID != created.ID {
		t.Fatal(receipt)
	}
}
func TestDualIdentityDoesNotFallbackAndCreationDoesNotInheritDownward(t *testing.T) {
	f := resourcesSetup(t)
	var member string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, f.tenant, f.actor.ID).Scan(&member))
	created, e := projects.New(f.env.Service.Access()).Create(ctx, f.actor, f.tenant, projects.CreateInput{Fields: projects.Fields{Name: "双身份项目", Code: "dual-scope", Purpose: "入口隔离"}, WriteMeta: projects.WriteMeta{RequestID: f.wm(t).RequestID, Reason: "入口隔离"}, Owner: member, Administrator: member}, access.Meta{})
	must(t, e)
	p := projectFixture{created.ProjectID, f.actor}
	facility := f.facility(t, "DUAL")
	pt, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"name": "隐藏测点", "code": "HIDDEN"})
	must(t, e)
	in := f.accessInput(t, p, facility.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, in)
	_, e = f.projectWrite(t, p, target(facility, res.Facility, facility.ID), "update", map[string]any{"expected_revision": 1, "name": "不得使用租户维护权"})
	code(t, e, "permission_denied")
	page, e := f.srv.List(ctx, p.actor, res.ProjectScope(f.tenant, p.id), target(facility, res.Point, ""), res.Filter{})
	must(t, e)
	if len(page.Items) != 0 {
		t.Fatal(page)
	}
	_, e = f.srv.Detail(ctx, p.actor, res.ProjectScope(f.tenant, p.id), target(facility, res.Point, pt.ID))
	code(t, e, "resource_not_found")
	_, e = f.srv.Detail(ctx, p.actor, res.TenantScope(f.tenant), target(facility, res.Point, pt.ID))
	must(t, e)
	usage, e := f.srv.FacilityUsage(ctx, f.actor, f.tenant, facility.ID, res.AccessFilter{Limit: 1})
	must(t, e)
	if len(usage.Items) != 1 || usage.Items[0].ProjectID != p.id {
		t.Fatal(usage)
	}
}
func TestCurrentSelectionLimitAndWrongGrantIdentities(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "ProjectA")
	fac := f.facility(t, "LIMIT")
	other := f.facility(t, "OTHER")
	point, e := f.write(t, target(other, res.Point, ""), "create", map[string]any{"name": "其他设施测点", "code": "P"})
	must(t, e)
	in := f.accessInput(t, p, fac.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Point, PointID: &point.ID, Capabilities: []string{"read"}}}
	_, e = f.srv.PreviewAccessChange(ctx, f.actor, f.tenant, p.id, fac.ID, in)
	code(t, e, "resource_not_found")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read", "lifecycle"}}}
	_, e = f.srv.PreviewAccessChange(ctx, f.actor, f.tenant, p.id, fac.ID, in)
	code(t, e, "grant_capability_invalid")
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO engineering_points(tenant_id,facility_id,code,name,created_by,updated_by) SELECT $1,$2,'P'||n,'测点'||n,$3,$3 FROM generate_series(1,1001) n`, f.tenant, fac.ID, f.actor.ID)
	must(t, e)
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}}
	in.SelectCurrent = &res.CurrentSelection{RootKind: res.Facility, RootID: fac.ID, Kinds: map[res.Kind][]string{res.Point: {"read"}}}
	_, e = f.srv.PreviewAccessChange(ctx, f.actor, f.tenant, p.id, fac.ID, in)
	code(t, e, "resource_invalid")
	var n int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM project_facility_links WHERE project_id=$1`, p.id).Scan(&n))
	if n != 0 {
		t.Fatal("truncated selection was committed")
	}
}
func TestPointMoveNeedsDestinationCapabilityAndContextFollowsTenantStructure(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "ProjectA")
	fac := f.facility(t, "MOVE")
	a, e := f.write(t, target(fac, res.Component, ""), "create", map[string]any{"name": "第一跨", "code": "A", "category_key": "span", "expected_structure_revision": 1})
	must(t, e)
	b, e := f.write(t, target(fac, res.Component, ""), "create", map[string]any{"name": "第二跨", "code": "B", "category_key": "span", "expected_structure_revision": 2})
	must(t, e)
	point, e := f.write(t, target(fac, res.Point, ""), "create", map[string]any{"name": "观测点", "code": "P", "component_id": a.ID})
	must(t, e)
	in := f.accessInput(t, p, fac.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Component, ComponentID: &b.ID, Capabilities: []string{"read", "update"}}, {Kind: res.Point, PointID: &point.ID, Capabilities: []string{"read", "update"}}}
	f.grant(t, p, fac.ID, in)
	_, e = f.projectWrite(t, p, target(fac, res.Point, point.ID), "update", map[string]any{"expected_revision": 1, "component_id": b.ID})
	code(t, e, "permission_denied")
	d, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, p.id, fac.ID, res.AccessFilter{})
	must(t, e)
	for _, g := range d.Grants {
		if g.ResourceID == b.ID {
			in = f.accessInput(t, p, fac.ID, "change_grants")
			in.Update = []res.GrantUpdate{{ID: g.ID, Revision: g.Revision, Capabilities: []string{"read", "update", "create_point"}}}
			f.grant(t, p, fac.ID, in)
		}
	}
	pr, raw := f.projectPreview(t, p, target(fac, res.Point, point.ID), "update", f.projectIntent(t, p, target(fac, res.Point, point.ID), map[string]any{"expected_revision": 1, "component_id": b.ID, "reason": "纠正归属"}))
	if !pr.Required {
		t.Fatal(pr)
	}
	_, e = f.srv.Mutate(ctx, p.actor, res.ProjectScope(f.tenant, p.id), target(fac, res.Point, point.ID), "update", raw, access.Meta{})
	must(t, e)
	_, e = f.projectWrite(t, p, target(fac, res.Component, b.ID), "update", map[string]any{"expected_revision": 1, "expected_structure_revision": 3, "parent_component_id": a.ID})
	code(t, e, "tenant_action_required")
	// Remove full component access; only the new necessary ancestor remains visible.
	d, e = f.srv.AccessDetail(ctx, f.actor, f.tenant, p.id, fac.ID, res.AccessFilter{})
	must(t, e)
	for _, g := range d.Grants {
		if g.ResourceID == b.ID {
			in = f.accessInput(t, p, fac.ID, "change_grants")
			in.End = []res.GrantEnd{{ID: g.ID, Revision: g.Revision}}
			pr, e := f.srv.PreviewAccessChange(ctx, f.actor, f.tenant, p.id, fac.ID, in)
			must(t, e)
			if len(pr.Context) != 1 {
				t.Fatal(pr)
			}
			f.grant(t, p, fac.ID, in)
		}
	}
	components, e := f.srv.List(ctx, p.actor, res.ProjectScope(f.tenant, p.id), target(fac, res.Component, ""), res.Filter{})
	must(t, e)
	if len(components.Items) != 1 || components.Items[0].ID != b.ID || components.Items[0].Visibility != "context" {
		t.Fatal(components)
	}
	// Tenant structural correction recomputes the project path without granting
	// either ancestor's full record, and history must not retain the hidden old ID.
	_, e = f.write(t, target(fac, res.Point, point.ID), "update", map[string]any{"expected_revision": 2, "component_id": a.ID, "reason": "租户按现场纠正归属"})
	must(t, e)
	components, e = f.srv.List(ctx, p.actor, res.ProjectScope(f.tenant, p.id), target(fac, res.Component, ""), res.Filter{})
	must(t, e)
	if len(components.Items) != 1 || components.Items[0].ID != a.ID || components.Items[0].Visibility != "context" {
		t.Fatal(components)
	}
	_, e = f.srv.Detail(ctx, p.actor, res.ProjectScope(f.tenant, p.id), target(fac, res.Component, a.ID))
	code(t, e, "resource_not_found")
	history, e := f.srv.Changes(ctx, p.actor, res.ProjectScope(f.tenant, p.id), target(fac, res.Facility, fac.ID), res.Filter{Kind: "point", ID: point.ID})
	must(t, e)
	projected, e := json.Marshal(history)
	must(t, e)
	if strings.Contains(string(projected), b.ID) || !strings.Contains(string(projected), `"historical_context_hidden":true`) {
		t.Fatal("old ungranted ancestor was not redacted", string(projected))
	}
}
