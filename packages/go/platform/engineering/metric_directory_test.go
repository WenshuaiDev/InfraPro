package engineering_test

import (
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
)

func TestMetricDirectoryAndFixedUnitChoices(t *testing.T) {
	f := setup(t)
	a := f.actor(f.p)
	units := eng.Scope{Kind: eng.UnitDefinition}
	metrics := eng.Scope{Kind: eng.MetricDefinition}
	for _, category := range []string{"length", "indicator"} {
		_, e := f.s.SaveCategory(ctx, a, eng.CategoryInput{WriteMeta: f.wm(t), Key: category, Name: category, Description: category}, access.Meta{})
		must(t, e)
	}
	unit := simple("毫米")
	unit.Unit = &eng.Unit{Symbol: "mm", Category: "length"}
	mm := f.publish(t, units.Kind, f.create(t, units.Kind, "mm", unit))
	unit.Unit.Category = "indicator"
	f.publish(t, units.Kind, f.create(t, units.Kind, "other_mm", unit))
	unit.Name = "草稿毫米"
	f.create(t, units.Kind, "draft_mm", unit)
	unfiltered, e := f.s.List(ctx, a, units, eng.Filter{Query: "mm", UnitChoices: true})
	must(t, e)
	page, e := f.s.List(ctx, a, units, eng.Filter{Query: "毫米", UnitChoices: true, UnitCategory: "length"})
	must(t, e)
	if unfiltered.Total != 2 || page.Total != 1 || len(page.Items[0].UnitVersions) != 1 || page.Items[0].UnitVersions[0].ID != mm.VersionID {
		t.Fatalf("choices: all=%+v filtered=%+v", unfiltered, page)
	}
	metric := simple("位移")
	metric.Description = "定义：测点位置变化。\n适用设施：桥梁、隧道、桥梁。\n类别：基础观测。"
	metric.Metric = &eng.Metric{Shape: "decimal", StandardUnit: mm.VersionID, AllowedUnits: []string{mm.VersionID}, Category: "length"}
	release := f.publish(t, metrics.Kind, f.create(t, metrics.Kind, "legacy", metric))
	metric.Metric.FacilityTopics = ptr([]string{"隧道", "边坡"})
	f.create(t, metrics.Kind, "tagged", metric)
	metric.Metric.FacilityTopics = ptr([]string{})
	f.create(t, metrics.Kind, "untagged", metric)
	list := func(filter eng.Filter) eng.Page {
		result, err := f.s.List(ctx, a, metrics, filter)
		must(t, err)
		return result
	}
	all := list(eng.Filter{FacilityTopic: "隧道", Limit: 1, Offset: 1})
	if all.Total != 2 || len(all.Items) != 1 || all.FacilityCounts["__all"] != 3 || all.FacilityCounts["桥梁"] != 1 || all.FacilityCounts["__unassigned"] != 1 {
		t.Fatalf("bad full-set facets on page two: %+v", all)
	}
	bridge := list(eng.Filter{Query: "位移", FacilityTopic: "桥梁", Status: "usable"})
	if bridge.Total != 1 || bridge.Items[0].ID != release.DefinitionID || bridge.Items[0].MetricIndex.StandardSymbol != "mm" || !bridge.Items[0].MetricIndex.LatestUsable {
		t.Fatalf("bad published index: %+v", bridge)
	}
	if unassigned := list(eng.Filter{FacilityTopic: "__unassigned", Status: "draft"}); unassigned.Total != 1 || unassigned.Items[0].Code != "untagged" {
		t.Fatalf("empty explicit tags must override legacy description: %+v", unassigned)
	}
	if empty := list(eng.Filter{Query: "missing"}); empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatalf("bad empty list: %+v", empty)
	}
}
