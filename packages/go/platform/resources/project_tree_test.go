package resources_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func TestProjectResourceTreeVisibilitySearchPagingAndRevocation(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "TreeProject")
	other := f.project(t, "OtherTreeProject")
	facility := f.facility(t, "VISIBLE")
	hidden := f.facility(t, "HIDDEN")
	component, e := f.write(t, target(facility, res.Component, ""), "create", map[string]any{"code": "SPAN", "name": "路径构件", "category_key": "span", "description": "仅完整授权可见的构件资料", "expected_structure_revision": 1})
	must(t, e)
	point, e := f.write(t, target(facility, res.Point, ""), "create", map[string]any{"code": "POINT", "name": "路径测点", "component_id": component.ID, "purpose": "仅完整授权可见的测点资料"})
	must(t, e)
	channels := []res.Result{}
	for i, name := range []string{"目标_%_一", "目标_%_二", "未授权通道"} {
		ch, err := f.write(t, res.Target{Kind: res.Channel, Facility: facility.ID, Point: point.ID}, "create", map[string]any{"code": fmt.Sprintf("CH%d", i), "name": name, "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
		must(t, err)
		channels = append(channels, ch)
	}
	in := f.accessInput(t, p, facility.ID, "create_link")
	in.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &channels[0].ID, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &channels[1].ID, Capabilities: []string{"read"}}}
	f.grant(t, p, facility.ID, in)
	scope := res.ProjectScope(f.tenant, p.id)
	read := func(filter res.TreeFilter) res.TreePage {
		t.Helper()
		page, err := f.srv.ScopedTree(ctx, p.actor, scope, filter)
		must(t, err)
		return page
	}
	root := read(res.TreeFilter{})
	if root.Total != 1 || root.Items[0].ID != facility.ID || len(root.Types) != 1 {
		t.Fatalf("project roots or types leaked: %+v", root)
	}
	children := read(res.TreeFilter{FacilityID: facility.ID, ParentKind: res.Facility, ParentID: facility.ID})
	if children.Total != 1 || children.Counts[res.Component] != 1 || children.Items[0].ID != component.ID || !children.Items[0].HasChildren || children.Items[0].TypeName != "" {
		t.Fatalf("visible context/counts incorrect: %+v", children)
	}
	filter := res.TreeFilter{Filter: res.Filter{Query: "_%_", Status: "all", Limit: 1}}
	page := read(filter)
	if page.Total != 2 || len(page.Items) != 1 || len(page.Items[0].Ancestors) != 3 || page.Items[0].Ancestors[2].ID != point.ID || page.Items[0].UnitSymbol != "mm" || page.NextCursor == "" {
		t.Fatalf("deep literal search/path incorrect: %+v", page)
	}
	filter.Cursor = page.NextCursor
	second := read(filter)
	if len(second.Items) != 1 || second.Items[0].ID == page.Items[0].ID || second.NextCursor != "" || second.Total != 2 {
		t.Fatal("incorrect next page", second)
	}
	_, e = f.srv.ScopedTree(ctx, other.actor, res.ProjectScope(f.tenant, other.id), filter)
	code(t, e, "invalid_request")
	path := read(res.TreeFilter{FacilityID: facility.ID, NodeID: point.ID})
	if len(path.Items) != 1 || len(path.Items[0].Ancestors) != 2 || path.Items[0].TypeDefinitionID != "" {
		t.Fatal("context deep-link path incorrect", path)
	}
	raw, e := json.Marshal(path)
	must(t, e)
	for _, private := range []string{"仅完整授权", "purpose", "description", "position", "created_by"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("context leaked %s", private)
		}
	}
	if got := read(res.TreeFilter{FacilityID: facility.ID, NodeID: channels[2].ID}); got.Total != 0 {
		t.Fatal("ungranted channel visible", got)
	}
	if got := read(res.TreeFilter{FacilityID: hidden.ID, NodeID: point.ID}); got.Total != 0 {
		t.Fatal("mismatched facility accepted", got)
	}
	if got := read(res.TreeFilter{Filter: res.Filter{TypeDefinition: root.Types[0].ID}}); got.Total != 1 {
		t.Fatal("type filter lost grant", got)
	}
	if got := read(res.TreeFilter{Filter: res.Filter{Status: "retired"}}); got.Total != 0 {
		t.Fatal("status filter ignored", got)
	}
	_, e = f.srv.ScopedTree(ctx, p.actor, scope, res.TreeFilter{FacilityID: hidden.ID, ParentKind: res.Facility, ParentID: hidden.ID})
	code(t, e, "resource_not_found")
	_, e = f.srv.ScopedTree(ctx, p.actor, scope, res.TreeFilter{Filter: res.Filter{Deleted: "only"}})
	code(t, e, "permission_denied")
	_, e = f.srv.Tree(ctx, p.actor, f.tenant, res.TreeFilter{})
	code(t, e, "permission_denied")
	f.grant(t, p, facility.ID, f.accessInput(t, p, facility.ID, "revoke_link"))
	if got := read(res.TreeFilter{}); got.Total != 0 || len(got.Types) != 0 {
		t.Fatal("revoked roots/types still visible", got)
	}
	if got := read(res.TreeFilter{FacilityID: facility.ID, NodeID: point.ID}); got.Total != 0 {
		t.Fatal("revoked path still visible", got)
	}
	_, e = f.srv.ScopedTree(ctx, p.actor, scope, filter)
	code(t, e, "invalid_request")
}
