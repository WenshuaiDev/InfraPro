package resources_test

import (
	"encoding/json"
	"testing"

	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func TestTenantResourceTreePagingSearchAndIsolation(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "TREE")
	span, e := f.write(t, target(facility, res.Component, ""), "create", map[string]any{"code": "SPAN", "name": "第一跨", "category_key": "span", "expected_structure_revision": 1})
	must(t, e)
	beam, e := f.write(t, target(facility, res.Component, ""), "create", map[string]any{"code": "BEAM", "name": "第一梁", "category_key": "beam", "parent_component_id": span.ID, "expected_structure_revision": span.StructureRevision})
	must(t, e)
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "POINT", "name": "梁端测点", "component_id": beam.ID})
	must(t, e)
	rootPoint, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "ROOT", "name": "环境测点"})
	must(t, e)
	channel, e := f.write(t, res.Target{Kind: res.Channel, Facility: facility.ID, Point: point.ID}, "create", map[string]any{"code": "WY-01", "name": "位移_%_01", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
	must(t, e)
	browse := res.TreeFilter{Filter: res.Filter{Status: "all", Limit: 1}, FacilityID: facility.ID, ParentKind: res.Facility, ParentID: facility.ID}
	page, e := f.srv.Tree(ctx, f.actor, f.tenant, browse)
	must(t, e)
	if len(page.Items) != 1 || page.Total != 2 || page.Counts[res.Component] != 1 || page.Counts[res.Point] != 1 || page.NextCursor == "" || page.Items[0].ID != span.ID {
		t.Fatalf("unexpected first page: %+v", page)
	}
	browse.Cursor = page.NextCursor
	page, e = f.srv.Tree(ctx, f.actor, f.tenant, browse)
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].ID != rootPoint.ID || page.NextCursor != "" {
		t.Fatal(page)
	}
	browse.Status = "retired"
	_, e = f.srv.Tree(ctx, f.actor, f.tenant, browse)
	code(t, e, "invalid_request")
	search, e := f.srv.Tree(ctx, f.actor, f.tenant, res.TreeFilter{Filter: res.Filter{Query: "_%_", Status: "all"}})
	must(t, e)
	if len(search.Items) != 1 || search.Items[0].ID != channel.ID || len(search.Items[0].Ancestors) != 4 || search.Items[0].Ancestors[2].ID != beam.ID || search.Items[0].Ancestors[3].ID != point.ID || search.Items[0].UnitSymbol != "mm" || search.Items[0].MetricName != "位移" {
		t.Fatalf("search path or literal matching incorrect: %+v", search)
	}
	outsider := f.fixture.actor(f.member(t, "TreeOutsider"))
	_, e = f.srv.Tree(ctx, outsider, f.tenant, res.TreeFilter{})
	if e == nil {
		t.Fatal("tree leaked across membership boundary")
	}
	_, e = f.srv.Tree(ctx, f.actor, f.tenant, res.TreeFilter{FacilityID: facility.ID, ParentKind: res.Component, ParentID: point.ID})
	code(t, e, "resource_not_found")
	other := f.facility(t, "OTHER")
	_, e = f.srv.Tree(ctx, f.actor, f.tenant, res.TreeFilter{FacilityID: other.ID, ParentKind: res.Component, ParentID: beam.ID})
	code(t, e, "resource_not_found")
	b, e := json.Marshal(search)
	must(t, e)
	var wire map[string]any
	must(t, json.Unmarshal(b, &wire))
	item := wire["items"].([]any)[0].(map[string]any)
	for _, key := range []string{"values", "position", "created_by", "source_reference", "source", "tenant_id"} {
		if _, exists := item[key]; exists {
			t.Fatalf("unexpected private/tree-unrelated field %s", key)
		}
	}
}

func TestTenantResourceTreeStatusAndDeletedHistory(t *testing.T) {
	f := resourcesSetup(t)
	facility := f.facility(t, "RETIRED")
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "POINT", "name": "正常下级"})
	must(t, e)
	_, e = f.write(t, target(facility, res.Facility, facility.ID), "retire", map[string]any{"expected_revision": 1, "reason": "状态筛选验证"})
	must(t, e)
	page, e := f.srv.Tree(ctx, f.actor, f.tenant, res.TreeFilter{Filter: res.Filter{Query: "正常下级", Status: "normal"}})
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].ID != point.ID || len(page.Items[0].Ancestors) != 1 || page.Items[0].Ancestors[0].Status != "retired" {
		t.Fatal("retired ancestors must remain in a matching child's path", page)
	}
	deleted := f.facility(t, "DELETED")
	_, e = f.write(t, target(deleted, res.Facility, deleted.ID), "delete", map[string]any{"expected_revision": 1, "reason": "历史入口验证"})
	must(t, e)
	page, e = f.srv.Tree(ctx, f.actor, f.tenant, res.TreeFilter{Filter: res.Filter{Deleted: "only", Status: "all"}})
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].ID != deleted.ID || page.Items[0].HasChildren || page.Items[0].DeletedAt == nil || page.Items[0].TypeName != "" {
		t.Fatal(page)
	}
}
