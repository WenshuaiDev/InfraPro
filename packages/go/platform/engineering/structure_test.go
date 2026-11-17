package engineering_test

import (
	"encoding/json"
	"strings"
	"testing"

	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
)

func structureType() eng.Content {
	return eng.Content{MetamodelVersion: "2", Name: "混合结构测试类型", Structure: &eng.Structure{
		MonitoringSection: &eng.StructureCapability{Supported: ptr(true), Fields: []eng.Field{}},
		Borehole:          &eng.StructureCapability{Supported: ptr(true), Fields: []eng.Field{}},
	}, Components: []eng.Component{{Key: "beam", Name: "梁", Fields: []eng.Field{}}, {Key: "part", Name: "组成件", Fields: []eng.Field{}}}, Relations: []eng.Relation{{Parent: "root", Child: "beam"}, {Parent: "beam", Child: "part", MaxCount: ptr(1)}}}
}
func structureNode(id, kind, parent, category string) eng.Node {
	return eng.Node{ID: id, Kind: kind, ParentRef: parent, ComponentCategoryKey: category, Values: map[string]any{}}
}
func hasIssue(issues []eng.Issue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func TestStructureMetamodelCompatibilityAndApplicability(t *testing.T) {
	old := simple("旧类型")
	before, _ := json.Marshal(old)
	var copy eng.Content
	must(t, json.Unmarshal(before, &copy))
	after, _ := json.Marshal(copy)
	if string(before) != string(after) || strings.Contains(string(after), "structure") {
		t.Fatal("old serialization changed")
	}
	if eng.StructureApplicability(old, "borehole") != "undeclared" {
		t.Fatal("legacy type must be undeclared")
	}
	c := structureType()
	if issues := eng.ValidateContent(eng.TypeDefinition, c); len(issues) > 0 {
		t.Fatal(issues)
	}
	c.Structure.Borehole.Supported = ptr(false)
	if eng.StructureApplicability(c, "borehole") != "not_applicable" {
		t.Fatal("explicit unsupported")
	}
	if !hasIssue(eng.ValidateSample(c, eng.Sample{Nodes: []eng.Node{structureNode("h", "borehole", "root", "")}}), "structure_not_supported") {
		t.Fatal("unsupported hole accepted")
	}
	c.Structure.Borehole.Supported = nil
	if !hasIssue(eng.ValidateContent(eng.TypeDefinition, c), "structure_declaration_required") {
		t.Fatal("omitted support accepted")
	}
	c.MetamodelVersion = "999"
	if !hasIssue(eng.ValidateContent(eng.TypeDefinition, c), "unsupported_metamodel") {
		t.Fatal("unknown metamodel accepted")
	}
}

func TestMixedStructureNearestComponentAndAllContainerPairs(t *testing.T) {
	c := structureType()
	for _, parent := range []string{"component", "monitoring_section", "borehole"} {
		for _, child := range []string{"component", "monitoring_section", "borehole", "point"} {
			t.Run(parent+"_"+child, func(t *testing.T) {
				cat, childCat := "", ""
				if parent == "component" {
					cat = "beam"
				}
				if child == "component" {
					childCat = "beam"
					if parent == "component" {
						childCat = "part"
					}
				}
				sample := eng.Sample{Nodes: []eng.Node{structureNode("a", parent, "root", cat), structureNode("b", child, "a", childCat)}}
				if issues := eng.ValidateSample(c, sample); len(issues) > 0 {
					t.Fatal(issues)
				}
			})
		}
	}
	sample := eng.Sample{Nodes: []eng.Node{structureNode("c", "component", "root", "beam"), structureNode("h", "borehole", "c", ""), structureNode("s", "monitoring_section", "h", ""), structureNode("part", "component", "s", "part")}}
	if issues := eng.ValidateSample(c, sample); len(issues) > 0 {
		t.Fatal(issues)
	}
	sample.Nodes = append(sample.Nodes, structureNode("part2", "component", "h", "part"))
	if !hasIssue(eng.ValidateSample(c, sample), "component_count") {
		t.Fatal("organizational layer bypassed max count")
	}
	sample.Nodes[0].ParentRef = "s"
	if !hasIssue(eng.ValidateSample(c, sample), "sample_cycle") {
		t.Fatal("cycle accepted")
	}
	if !hasIssue(eng.ValidateSample(c, eng.Sample{Nodes: []eng.Node{structureNode("p", "point", "root", ""), structureNode("h", "borehole", "p", "")}}), "point_not_container") {
		t.Fatal("point accepted as container")
	}
}

func TestTemplateV2ApplicabilityAndRealAncestors(t *testing.T) {
	target := eng.Target{TypeVersion: "type", Applicability: "applicable", AnchorKind: "monitoring_section", PointScope: "descendant_points", RequiresBoreholeAncestor: true, QuantityBasis: "per_point"}
	c := eng.Content{MetamodelVersion: "2", Name: "孔内位移", Template: &eng.Template{TypeVersions: []string{"type"}, Items: []eng.Item{{Key: "displacement", Name: "位移", Enabled: false, Targets: []eng.Target{target}, Bindings: []eng.Binding{{MetricVersion: "metric", ResourceKind: "observation_channel"}}}}}}
	if issues := eng.ValidateContent(eng.MonitoringTemplate, c); len(issues) > 0 {
		t.Fatal(issues)
	}
	typ := structureType()
	typ.Structure.Borehole.Supported = ptr(false)
	if !hasIssue(eng.ValidateTargetType("2", target, typ, "/target"), "target_borehole_not_supported") {
		t.Fatal("disabled item accepted unsupported hole requirement")
	}
	sample := eng.Sample{Nodes: []eng.Node{structureNode("h", "borehole", "root", ""), structureNode("c", "component", "h", "beam"), structureNode("s", "monitoring_section", "c", ""), structureNode("p", "point", "s", ""), structureNode("outside", "point", "root", "")}}
	matches := eng.MatchTemplateSample(c, "type", sample)
	if len(matches) != 1 || len(matches[0].PointIDs) != 1 || matches[0].PointIDs[0] != "p" {
		t.Fatal(matches)
	}
	c.Template.Items[0].Targets[0] = eng.Target{TypeVersion: "type", Applicability: "not_applicable", Reason: "无孔类型"}
	if issues := eng.ValidateContent(eng.MonitoringTemplate, c); len(issues) > 0 {
		t.Fatal(issues)
	}
	if matches = eng.MatchTemplateSample(c, "type", sample); len(matches[0].PointIDs) != 0 || matches[0].Applicability != "not_applicable" {
		t.Fatal(matches)
	}
	c.Template.Items[0].Targets = nil
	if !hasIssue(eng.ValidateContent(eng.MonitoringTemplate, c), "target_mapping") {
		t.Fatal("missing mapping accepted")
	}
}

func TestMetamodel2PublicationAndBothDependencyReaders(t *testing.T) {
	f := setup(t)
	old := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "legacy", simple("旧类型")))
	var digest string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT digest FROM engineering_versions WHERE id=$1`, old.VersionID).Scan(&digest))
	published := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "mixed", structureType()))
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		v, _, e := eng.ResolvePublicTx(ctx, tx, published.VersionID, eng.TypeDefinition, true)
		if e != nil {
			return e
		}
		if v.SchemaVersion != "2" {
			t.Fatal(v.SchemaVersion)
		}
		all, e := eng.ResolvePublicBatchTx(ctx, tx, []string{old.VersionID, published.VersionID}, true)
		if e != nil {
			return e
		}
		if all[old.VersionID].Digest != digest || all[old.VersionID].Content.Structure != nil {
			t.Fatal("legacy snapshot changed")
		}
		return nil
	}))
	preview, e := f.s.Preview(ctx, f.actor(f.p), eng.Scope{Kind: eng.TypeDefinition}, published.DefinitionID, eng.PreviewInput{VersionID: published.VersionID, Sample: &eng.Sample{Nodes: []eng.Node{structureNode("h", "borehole", "root", "")}}})
	must(t, e)
	if !preview.Valid {
		t.Fatal(preview.Issues)
	}
	// Platform publication is independent of actual resource creation or adoption.
	var actual int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM engineering_facilities`).Scan(&actual))
	if actual != 0 {
		t.Fatal(actual)
	}
}
