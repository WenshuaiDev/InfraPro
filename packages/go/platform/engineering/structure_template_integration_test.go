package engineering_test

import (
	"encoding/json"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	cl "github.com/WenshuaiDev/InfraPro/packages/go/platform/classification"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/google/uuid"
)

func TestStructureTemplatesPublicTenantCopyAndCategoryBasis(t *testing.T) {
	f := setup(t)
	a := f.actor(f.p)
	dictionary := cl.New(f.env.Service.Access())
	meta := func() cl.Meta { return cl.Meta{RequestID: uuid.NewString(), Reason: "模板建议分类固定依据"} }
	root, leaf := uuid.NewString(), uuid.NewString()
	content := cl.Content{DictionaryCode: cl.DictionaryCode, SensorRootID: root, Nodes: []cl.Node{{ID: root, Code: "sensor", Name: "传感器", Status: "active"}, {ID: leaf, ParentID: root, Code: "movement", Name: "位移传感器", Status: "active", Selectable: true}}}
	d, err := dictionary.Dictionary(ctx, a)
	must(t, err)
	draft, err := dictionary.SaveDraft(ctx, a, cl.SaveInput{Meta: meta(), Revision: d.DraftRevision, Content: content}, access.Meta{})
	must(t, err)
	in := cl.PublishInput{Meta: meta(), Revision: draft.Revision}
	p, err := dictionary.PreviewPublish(ctx, a, in)
	must(t, err)
	in.PreviewHash = p.Hash
	release, err := dictionary.Publish(ctx, a, in, access.Meta{})
	must(t, err)
	_, err = f.s.SaveCategory(ctx, a, eng.CategoryInput{WriteMeta: f.wm(t), Key: "length", Name: "长度", Description: "空间长度"}, access.Meta{})
	must(t, err)
	unit := simple("毫米")
	unit.Unit = &eng.Unit{Symbol: "mm", Category: "length"}
	u := f.publish(t, eng.UnitDefinition, f.create(t, eng.UnitDefinition, "mm", unit))
	metric := simple("位移")
	metric.Metric = &eng.Metric{Shape: "decimal", Category: "length", StandardUnit: u.VersionID, AllowedUnits: []string{u.VersionID}}
	m := f.publish(t, eng.MetricDefinition, f.create(t, eng.MetricDefinition, "move", metric))
	withHole := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "with_hole", structureType()))
	noHoleContent := structureType()
	noHoleContent.Name = "明确无孔"
	noHoleContent.Structure.Borehole.Supported = ptr(false)
	noHole := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "no_hole", noHoleContent))
	template := eng.Content{MetamodelVersion: "2", Name: "混合类型监测", Template: &eng.Template{TypeVersions: []string{withHole.VersionID, noHole.VersionID}, Items: []eng.Item{{Key: "movement", Name: "孔内位移", Enabled: false, SuggestedCategory: &eng.CategorySuggestion{DictionaryReleaseID: release.ReleaseID, CategoryID: leaf}, Targets: []eng.Target{{TypeVersion: withHole.VersionID, Applicability: "applicable", AnchorKind: "monitoring_section", PointScope: "descendant_points", RequiresBoreholeAncestor: true, QuantityBasis: "per_point"}, {TypeVersion: noHole.VersionID, Applicability: "not_applicable", Reason: "此固定类型不支持孔位"}}, Bindings: []eng.Binding{{MetricVersion: m.VersionID, UnitVersion: u.VersionID, ResourceKind: "observation_channel", MinCount: 1}}}}}}
	public := f.publish(t, eng.MonitoringTemplate, f.create(t, eng.MonitoringTemplate, "mixed_template", template))
	owner := f.member(t, "StructureTemplateOwner")
	tenant, err := tenancy.New(f.env.Service.Access()).Create(ctx, a, tenancy.CreateInput{Fields: tenancy.Fields{Name: "模板租户", Code: "structure-template"}, AdministratorID: owner.Account.ID, RequestID: uuid.NewString(), Reason: "验证租户副本"}, access.Meta{})
	must(t, err)
	scope := eng.Scope{Tenant: tenant.TenantID, Kind: eng.MonitoringTemplate}
	actor := f.actor(owner)
	copy, err := f.s.Copy(ctx, actor, scope, eng.CopyInput{WriteMeta: f.wm(t), Code: "private_template", SourceVersion: public.VersionID}, access.Meta{})
	must(t, err)
	private, err := f.s.Publish(ctx, actor, scope, copy.DefinitionID, eng.RevisionInput{WriteMeta: f.wm(t), Revision: copy.Revision}, access.Meta{})
	must(t, err)
	// Real ancestry: hole → component → section → point; the reference layout
	// does not require the section to precede its hole.
	sample := eng.Sample{Nodes: []eng.Node{structureNode("h", "borehole", "root", ""), structureNode("c", "component", "h", "beam"), structureNode("s", "monitoring_section", "c", ""), structureNode("p", "point", "s", ""), structureNode("outside", "point", "root", "")}}
	preview, err := f.s.Preview(ctx, actor, scope, copy.DefinitionID, eng.PreviewInput{VersionID: private.VersionID, TypeVersion: withHole.VersionID, Sample: &sample})
	must(t, err)
	if !preview.Valid || !preview.SampleOnly || len(preview.Matches) != 1 || len(preview.Matches[0].PointIDs) != 1 || preview.Matches[0].PointIDs[0] != "p" {
		t.Fatal(preview)
	}
	preview, err = f.s.Preview(ctx, actor, scope, copy.DefinitionID, eng.PreviewInput{VersionID: private.VersionID, TypeVersion: noHole.VersionID, Sample: &eng.Sample{Nodes: []eng.Node{structureNode("p", "point", "root", "")}}})
	must(t, err)
	if !preview.Valid || preview.Matches[0].Applicability != "not_applicable" || len(preview.Matches[0].PointIDs) != 0 {
		t.Fatal(preview)
	}
	var invalid eng.Content
	raw, _ := json.Marshal(template)
	must(t, json.Unmarshal(raw, &invalid))
	invalid.Template.Items[0].Targets[1] = invalid.Template.Items[0].Targets[0]
	invalid.Template.Items[0].Targets[1].TypeVersion = noHole.VersionID
	_, err = f.s.Create(ctx, a, eng.Scope{Kind: eng.MonitoringTemplate}, eng.CreateInput{WriteMeta: f.wm(t), Code: "invalid_hole", Content: invalid}, access.Meta{})
	code(t, err, "definition_invalid")
	invalid.Template.Items[0].Targets = invalid.Template.Items[0].Targets[:1]
	missing := f.create(t, eng.MonitoringTemplate, "missing_mapping", invalid)
	_, err = f.s.Publish(ctx, a, eng.Scope{Kind: eng.MonitoringTemplate}, missing.DefinitionID, eng.RevisionInput{WriteMeta: f.wm(t), Revision: missing.Revision}, access.Meta{})
	code(t, err, "definition_invalid")
	// Suggestions cannot bypass the independently fixed metric/unit relation.
	must(t, json.Unmarshal(raw, &invalid))
	invalid.Template.Items[0].Bindings[0].UnitVersion = withHole.VersionID
	_, err = f.s.Create(ctx, a, eng.Scope{Kind: eng.MonitoringTemplate}, eng.CreateInput{WriteMeta: f.wm(t), Code: "invalid_metric", Content: invalid}, access.Meta{})
	if err == nil {
		t.Fatal("suggested classification bypassed output unit semantics")
	}
	must(t, json.Unmarshal(raw, &invalid))
	invalid.Template.Items[0].SuggestedCategory.CategoryID = uuid.NewString()
	_, err = f.s.Create(ctx, a, eng.Scope{Kind: eng.MonitoringTemplate}, eng.CreateInput{WriteMeta: f.wm(t), Code: "invalid_category", Content: invalid}, access.Meta{})
	code(t, err, "definition_invalid")
	var digest string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT digest FROM engineering_versions WHERE id=$1`, private.VersionID).Scan(&digest))
	newDraft, err := f.s.NewDraft(ctx, a, eng.Scope{Kind: eng.MonitoringTemplate}, public.DefinitionID, eng.NewDraftInput{RevisionInput: eng.RevisionInput{WriteMeta: f.wm(t), Revision: public.Revision}, VersionID: public.VersionID}, access.Meta{})
	must(t, err)
	template.Name = "公共后续版本"
	template.Template.Items[0].Name = "更新后的公共推荐名"
	newDraft, err = f.s.Save(ctx, a, eng.Scope{Kind: eng.MonitoringTemplate}, public.DefinitionID, eng.SaveInput{WriteMeta: f.wm(t), Revision: newDraft.Revision, Content: template}, access.Meta{})
	must(t, err)
	f.publish(t, eng.MonitoringTemplate, newDraft)
	detail, err := f.s.Detail(ctx, actor, scope, copy.DefinitionID)
	must(t, err)
	if detail.Versions[0].Digest != digest || detail.Versions[0].Content.Name == template.Name || detail.Versions[0].Content.Template.Items[0].SuggestedCategory.DictionaryReleaseID != release.ReleaseID {
		t.Fatal("public edit changed fixed private copy")
	}
	var dependencies, entities, bindings int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM engineering_category_dependencies WHERE version_id=$1 AND release_id=$2 AND category_id=$3`, private.VersionID, release.ReleaseID, leaf).Scan(&dependencies))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM engineering_structure_nodes`).Scan(&entities))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_source_bindings`).Scan(&bindings))
	if dependencies != 1 || entities != 0 || bindings != 0 {
		t.Fatal(dependencies, entities, bindings)
	}
}
