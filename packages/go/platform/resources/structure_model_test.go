package resources_test

import (
	"encoding/json"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func (f resourceFixture) structureVersion(t *testing.T, base string, holes bool) string {
	t.Helper()
	var definition string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT definition_id::text FROM engineering_versions WHERE id=$1`, base).Scan(&definition))
	scope := eng.Scope{Kind: eng.TypeDefinition}
	d, e := f.s.Detail(ctx, f.fixture.actor(f.p), scope, definition)
	must(t, e)
	draft, e := f.s.NewDraft(ctx, f.fixture.actor(f.p), scope, definition, eng.NewDraftInput{RevisionInput: eng.RevisionInput{WriteMeta: f.wm(t), Revision: d.Revision}, VersionID: base}, access.Meta{})
	must(t, e)
	var content eng.Content
	for _, v := range d.Versions {
		if v.ID == base {
			content = v.Content
		}
	}
	content.MetamodelVersion = "2"
	content.Structure = &eng.Structure{MonitoringSection: &eng.StructureCapability{Supported: ptr(true), Fields: []eng.Field{}}, Borehole: &eng.StructureCapability{Supported: ptr(holes), Fields: []eng.Field{}}}
	saved, e := f.s.Save(ctx, f.fixture.actor(f.p), scope, definition, eng.SaveInput{WriteMeta: f.wm(t), Revision: draft.Revision, Content: content}, access.Meta{})
	must(t, e)
	return f.publish(t, eng.TypeDefinition, saved).VersionID
}
func (f resourceFixture) structureCreate(t *testing.T, facility res.Result, kind res.Kind, code string, parent res.StructureRef, extra map[string]any) res.Result {
	t.Helper()
	d, e := f.srv.Detail(ctx, f.actor, res.TenantScope(f.tenant), target(facility, res.Facility, facility.ID))
	must(t, e)
	in := map[string]any{"code": code, "name": code, "parent_ref": parent, "expected_structure_revision": d.StructureRevision}
	for k, v := range extra {
		in[k] = v
	}
	r, e := f.write(t, target(facility, kind, ""), "create", in)
	must(t, e)
	return r
}
func (f resourceFixture) moveInput(t *testing.T, facility res.Result, kind res.Kind, id string, parent res.StructureRef) res.MoveInput {
	t.Helper()
	scope := res.TenantScope(f.tenant)
	d, e := f.srv.Detail(ctx, f.actor, scope, target(facility, kind, id))
	must(t, e)
	root, e := f.srv.Detail(ctx, f.actor, scope, target(facility, res.Facility, facility.ID))
	must(t, e)
	return res.MoveInput{WriteMeta: res.WriteMeta{RequestID: f.wm(t).RequestID, Revision: d.Revision, StructureRevision: root.StructureRevision, Reason: "调整现场工程组织"}, ParentRef: parent}
}
func TestStructureModelAdoptionMixedTreeMovesAndProjectContext(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "STRUCTURE")
	scope := res.TenantScope(f.tenant)
	version := f.structureVersion(t, f.typ, true)
	old, e := f.srv.Detail(ctx, f.actor, scope, target(facility, res.Facility, facility.ID))
	must(t, e)
	if old.TypeVersionID != f.typ || old.Actions.CreateBorehole {
		t.Fatal("publication adopted implicitly", old.Actions)
	}
	_, e = f.write(t, target(facility, res.Borehole, ""), "create", map[string]any{"code": "INVALID", "name": "不应创建", "expected_structure_revision": 1})
	if e == nil {
		t.Fatal("legacy type allowed hole")
	}
	adopt := res.TypeAdoptionInput{WriteMeta: res.WriteMeta{RequestID: f.wm(t).RequestID, Revision: old.Revision, StructureRevision: old.StructureRevision, Reason: "采用已核对的孔位能力"}, TypeVersionID: version}
	preview, e := f.srv.PreviewTypeAdoption(ctx, f.actor, f.tenant, facility.ID, adopt)
	must(t, e)
	if preview.BeforeApplicability["borehole"] != "undeclared" || preview.AfterApplicability["borehole"] != "applicable" {
		t.Fatal(preview)
	}
	adopt.PreviewHash = preview.Hash
	adopted, e := f.srv.AdoptType(ctx, f.actor, f.tenant, facility.ID, adopt, access.Meta{})
	must(t, e)
	if adopted.ID != facility.ID {
		t.Fatal("facility identity changed")
	}
	root := res.StructureRef{Kind: res.Facility, ID: facility.ID}
	section := f.structureCreate(t, facility, res.MonitoringSection, "S1", root, nil)
	hole := f.structureCreate(t, facility, res.Borehole, "H1", res.StructureRef{Kind: res.MonitoringSection, ID: section.ID}, map[string]any{"borehole": map[string]any{"measured_depth_m": 10, "collar_elevation": 0, "elevation_datum": "工程高程基准"}})
	component := f.structureCreate(t, facility, res.Component, "C1", res.StructureRef{Kind: res.Borehole, ID: hole.ID}, map[string]any{"category_key": "span"})
	nested := f.structureCreate(t, facility, res.MonitoringSection, "S2", res.StructureRef{Kind: res.Component, ID: component.ID}, nil)
	point := f.structureCreate(t, facility, res.Point, "P1", res.StructureRef{Kind: res.MonitoringSection, ID: nested.ID}, nil)
	detail, e := f.srv.Detail(ctx, f.actor, scope, target(facility, res.Point, point.ID))
	must(t, e)
	if len(detail.Ancestors) != 5 || detail.ParentRef.Kind != res.MonitoringSection || detail.ComponentID == nil || *detail.ComponentID != component.ID {
		t.Fatal(detail)
	}
	_, e = f.write(t, target(facility, res.Point, point.ID), "update", map[string]any{"name": "仅改名", "expected_revision": 1})
	must(t, e)
	after, e := f.srv.Detail(ctx, f.actor, scope, target(facility, res.Point, point.ID))
	must(t, e)
	if after.ParentRef.ID != nested.ID {
		t.Fatal("omitted parent flattened tree")
	}
	_, e = f.write(t, target(facility, res.Point, point.ID), "update", map[string]any{"component_id": nil, "expected_revision": after.Revision})
	code(t, e, "structure_model_required")
	// A project can see the point and minimal context, never the ungranted containers.
	project := f.project(t, "StructureReader")
	in := f.accessInput(t, project, facility.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Point, PointID: &point.ID, Capabilities: []string{"read", "update"}}}
	f.grant(t, project, facility.ID, in)
	pd, e := f.srv.Detail(ctx, project.actor, res.ProjectScope(f.tenant, project.id), target(facility, res.Point, point.ID))
	must(t, e)
	if pd.Ancestors[1].Visibility != "context" || pd.Ancestors[2].Visibility != "context" {
		t.Fatal(pd.Ancestors)
	}
	_, e = f.srv.Detail(ctx, project.actor, res.ProjectScope(f.tenant, project.id), target(facility, res.Borehole, hole.ID))
	code(t, e, "resource_not_found")
	// Moving the complete section under its own descendant fails, with no partial update.
	cycle := f.moveInput(t, facility, res.MonitoringSection, section.ID, res.StructureRef{Kind: res.MonitoringSection, ID: nested.ID})
	_, e = f.srv.PreviewMove(ctx, f.actor, scope, target(facility, res.MonitoringSection, section.ID), cycle)
	if e == nil {
		t.Fatal("cycle accepted")
	}
	// Component ancestry cannot be bypassed by a section.
	invalid := f.moveInput(t, facility, res.Component, component.ID, root)
	// This is legal: a span remains rooted, and all descendants retain identities.
	mp, e := f.srv.PreviewMove(ctx, f.actor, scope, target(facility, res.Component, component.ID), invalid)
	must(t, e)
	if len(mp.Affected) != 3 {
		t.Fatal(mp)
	}
	invalid.PreviewHash = mp.Hash
	moved, e := f.srv.Move(ctx, f.actor, scope, target(facility, res.Component, component.ID), invalid, access.Meta{})
	must(t, e)
	if moved.ID != component.ID {
		t.Fatal(moved)
	}
	after, e = f.srv.Detail(ctx, f.actor, scope, target(facility, res.Point, point.ID))
	must(t, e)
	if after.ID != point.ID || len(after.Ancestors) != 3 || after.ParentRef.ID != nested.ID {
		t.Fatal(after)
	}
	// Existing borehole identity (even an empty one) prevents losing its type semantics.
	noHole := f.structureVersion(t, version, false)
	rootDetail, e := f.srv.Detail(ctx, f.actor, scope, target(facility, res.Facility, facility.ID))
	must(t, e)
	adopt = res.TypeAdoptionInput{WriteMeta: res.WriteMeta{RequestID: f.wm(t).RequestID, Revision: rootDetail.Revision, StructureRevision: rootDetail.StructureRevision, Reason: "不应采用无孔版本"}, TypeVersionID: noHole}
	_, e = f.srv.PreviewTypeAdoption(ctx, f.actor, f.tenant, facility.ID, adopt)
	if e == nil {
		t.Fatal("retained hole lost semantics")
	}
	tree, e := f.srv.Tree(ctx, f.actor, f.tenant, res.TreeFilter{Filter: res.Filter{Status: "all"}, FacilityID: facility.ID, ParentKind: res.Facility, ParentID: facility.ID})
	must(t, e)
	if tree.Counts[res.MonitoringSection] != 1 || tree.Counts[res.Component] != 1 {
		t.Fatal(tree)
	}
	raw, _ := json.Marshal(tree)
	if len(raw) == 0 {
		t.Fatal("empty projection")
	}
}

func (f resourceFixture) typedFacility(t *testing.T) res.Result {
	t.Helper()
	version := f.structureVersion(t, f.typ, true)
	r, e := f.write(t, res.Target{Kind: res.Facility}, "create", map[string]any{"code": "TYPED", "name": "结构定位试验", "type_version_id": version, "values": map[string]any{"length": 120}})
	must(t, e)
	return r
}
func (f resourceFixture) recordInput(t *testing.T, facility res.Result, kind res.Kind, id, action string) res.StructureRecordInput {
	t.Helper()
	m := f.moveInput(t, facility, kind, id, res.StructureRef{Kind: res.Facility, ID: facility.ID})
	return res.StructureRecordInput{WriteMeta: m.WriteMeta, Action: action}
}
func (f resourceFixture) saveRecord(t *testing.T, facility res.Result, kind res.Kind, id string, in res.StructureRecordInput) res.Result {
	t.Helper()
	scope := res.TenantScope(f.tenant)
	target := target(facility, kind, id)
	p, e := f.srv.PreviewStructureRecord(ctx, f.actor, scope, target, in)
	must(t, e)
	in.PreviewHash = p.Hash
	r, e := f.srv.WriteStructureRecord(ctx, f.actor, scope, target, in, access.Meta{})
	must(t, e)
	return r
}
func TestBoreholePositionReferenceAndMoveEvidence(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.typedFacility(t)
	scope := res.TenantScope(f.tenant)
	root := res.StructureRef{Kind: res.Facility, ID: facility.ID}
	h := f.structureCreate(t, facility, res.Borehole, "H", root, map[string]any{"borehole": map[string]any{"measured_depth_m": 10}})
	s := f.structureCreate(t, facility, res.MonitoringSection, "S", root, nil)
	p := f.structureCreate(t, facility, res.Point, "P", res.StructureRef{Kind: res.Borehole, ID: h.ID}, nil)
	in := f.recordInput(t, facility, res.Point, p.ID, "position")
	in.Position = &res.BoreholePosition{BoreholeID: h.ID, BoreholeRevision: 1, DistanceM: ptr(0.0), ReferenceStart: "孔口", Direction: "沿孔向下", Unit: "m"}
	f.saveRecord(t, facility, res.Point, p.ID, in)
	page, e := f.srv.StructureRecords(ctx, f.actor, scope, target(facility, res.Point, p.ID), "positions", "", 100)
	must(t, e)
	if page.CurrentPosition == nil || page.CurrentPosition.DistanceM == nil || *page.CurrentPosition.DistanceM != 0 || page.CurrentPosition.VerticalDepthM != nil || len(page.Items) != 1 {
		t.Fatal(page)
	}
	in = f.recordInput(t, facility, res.Point, p.ID, "position")
	in.Position = &res.BoreholePosition{BoreholeID: h.ID, BoreholeRevision: 1, DistanceM: ptr(11.0), ReferenceStart: "孔口", Direction: "沿孔向下", Unit: "m", VerticalDepthM: ptr(8.0), VerticalDatum: "路面"}
	_, e = f.srv.PreviewStructureRecord(ctx, f.actor, scope, target(facility, res.Point, p.ID), in)
	if e == nil {
		t.Fatal("depth discrepancy accepted without reason")
	}
	in.Position.ConsistencyReason = "实测孔深资料待复核，按测点测量记录暂存"
	f.saveRecord(t, facility, res.Point, p.ID, in)
	in = f.recordInput(t, facility, res.Point, p.ID, "reference")
	in.Target = &res.StructureRef{Kind: res.MonitoringSection, ID: s.ID}
	in.Purpose = "图纸所属断面"
	f.saveRecord(t, facility, res.Point, p.ID, in)
	refs, e := f.srv.StructureRecords(ctx, f.actor, scope, target(facility, res.Point, p.ID), "references", "", 1)
	must(t, e)
	if len(refs.Items) != 1 {
		t.Fatal(refs)
	}
	// Moving the entire hole preserves the nearest-hole identity and measured distances.
	move := f.moveInput(t, facility, res.Borehole, h.ID, res.StructureRef{Kind: res.MonitoringSection, ID: s.ID})
	preview, e := f.srv.PreviewMove(ctx, f.actor, scope, target(facility, res.Borehole, h.ID), move)
	must(t, e)
	if len(preview.PositionsRequiringReview) != 0 {
		t.Fatal(preview)
	}
	move.PreviewHash = preview.Hash
	_, e = f.srv.Move(ctx, f.actor, scope, target(facility, res.Borehole, h.ID), move, access.Meta{})
	must(t, e)
	// A point leaving its closest hole requires an explicit historical disposition.
	move = f.moveInput(t, facility, res.Point, p.ID, root)
	preview, e = f.srv.PreviewMove(ctx, f.actor, scope, target(facility, res.Point, p.ID), move)
	must(t, e)
	if len(preview.PositionsRequiringReview) != 1 {
		t.Fatal(preview)
	}
	move.PreviewHash = preview.Hash
	_, e = f.srv.Move(ctx, f.actor, scope, target(facility, res.Point, p.ID), move, access.Meta{})
	code(t, e, "position_disposition_required")
	move.PositionDisposition = "history_pending"
	preview, e = f.srv.PreviewMove(ctx, f.actor, scope, target(facility, res.Point, p.ID), move)
	must(t, e)
	move.PreviewHash = preview.Hash
	_, e = f.srv.Move(ctx, f.actor, scope, target(facility, res.Point, p.ID), move, access.Meta{})
	must(t, e)
	page, e = f.srv.StructureRecords(ctx, f.actor, scope, target(facility, res.Point, p.ID), "positions", "", 1)
	must(t, e)
	if page.CurrentPosition != nil || page.NearestBorehole != nil || page.NextCursor == "" {
		t.Fatal(page)
	}
	page2, e := f.srv.StructureRecords(ctx, f.actor, scope, target(facility, res.Point, p.ID), "positions", page.NextCursor, 1)
	must(t, e)
	if len(page2.Items) != 1 || page2.NextCursor != "" {
		t.Fatal(page2)
	}
	changes, e := f.srv.StructureRecords(ctx, f.actor, scope, target(facility, res.Point, p.ID), "changes", "", 100)
	must(t, e)
	if len(changes.Items) != 4 {
		t.Fatal(changes)
	}
	// Formal secondary references never change the canonical parent.
	detail, e := f.srv.Detail(ctx, f.actor, scope, target(facility, res.Point, p.ID))
	must(t, e)
	if detail.ParentRef.ID != facility.ID {
		t.Fatal(detail.ParentRef)
	}
	// Current clients cannot continue the parallel text model, even with an empty array.
	_, e = f.write(t, target(facility, res.Point, p.ID), "update", map[string]any{"expected_revision": detail.Revision, "position": map[string]any{"sections": []any{}}})
	if e == nil {
		t.Fatal("legacy sections still writable")
	}
}
