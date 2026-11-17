package engineering

import (
	"fmt"
	"slices"
	"strings"
)

// SupportedMetamodel is shared by publication, single and batch dependency readers.
// Version 1 snapshots are never upgraded in place.
func SupportedMetamodel(version string) bool { return version == "1" || version == "2" }

func StructureCapabilityFor(c Content, kind string) *StructureCapability {
	if c.MetamodelVersion != "2" || c.Structure == nil {
		return nil
	}
	switch kind {
	case "monitoring_section":
		return c.Structure.MonitoringSection
	case "borehole":
		return c.Structure.Borehole
	}
	return nil
}

// StructureApplicability deliberately distinguishes an undeclared old definition
// from an explicit business decision that an object kind is not applicable.
func StructureApplicability(c Content, kind string) string {
	if slices.Contains([]string{"facility", "component", "point"}, kind) && SupportedMetamodel(c.MetamodelVersion) {
		return "applicable"
	}
	v := StructureCapabilityFor(c, kind)
	if v == nil || v.Supported == nil {
		return "undeclared"
	}
	if *v.Supported {
		return "applicable"
	}
	return "not_applicable"
}

func StructureFields(c Content, kind, category string) ([]Field, bool) {
	switch kind {
	case "facility":
		return c.Fields, true
	case "component":
		for _, v := range c.Components {
			if v.Key == category {
				return v.Fields, true
			}
		}
	case "point":
		return []Field{}, true
	case "monitoring_section", "borehole":
		if v := StructureCapabilityFor(c, kind); v != nil && v.Supported != nil && *v.Supported {
			return v.Fields, true
		}
	}
	return nil, false
}

var structureReservedFields = []string{"id", "kind", "tenant_id", "facility_id", "parent_ref", "code", "name", "status", "revision", "position", "section_type_id", "type_definition_id", "type_version_id", "design_depth_m", "measured_depth_m", "collar_elevation", "elevation_datum", "diameter_mm", "azimuth", "inclination", "borehole_id", "distance_m"}

func validateStructureContent(kind Kind, c Content) []Issue {
	out := []Issue{}
	if c.Structure != nil && (kind != TypeDefinition || c.MetamodelVersion != "2") {
		add(&out, "unsupported_structure", "结构能力仅用于元模型 2 的设施类型。", "/structure", "")
	}
	if kind == TypeDefinition && c.MetamodelVersion == "2" {
		for _, k := range []string{"monitoring_section", "borehole"} {
			p := "/structure/" + k
			v := StructureCapabilityFor(c, k)
			if v == nil || v.Supported == nil {
				add(&out, "structure_declaration_required", "请明确声明支持或不支持此类工程对象。", p+"/supported", k)
				continue
			}
			fields(&out, v.Fields, p+"/fields")
			if !*v.Supported && len(v.Fields) > 0 {
				add(&out, "unsupported_structure_fields", "不适用的节点不能声明属性。", p+"/fields", k)
			}
			for i, f := range v.Fields {
				if slices.Contains(structureReservedFields, f.Key) {
					add(&out, "reserved_structure_field", "扩展属性不能覆盖系统身份、归属或定位字段。", fmt.Sprintf("%s/fields/%d/key", p, i), f.Key)
				}
			}
		}
		for i, component := range c.Components {
			if slices.Contains([]string{"facility", "point", "monitoring_section", "borehole"}, component.Key) {
				add(&out, "reserved_component_key", "构件类别键不能占用结构种类键。", fmt.Sprintf("/components/%d/key", i), component.Key)
			}
		}
	}
	if c.Template != nil {
		for i, item := range c.Template.Items {
			if item.SuggestedCategory != nil && c.MetamodelVersion != "2" {
				add(&out, "unsupported_category_suggestion", "建议设备分类需要元模型 2。", fmt.Sprintf("/template/items/%d/suggested_category", i), item.Key)
			}
		}
	}
	return out
}

func validateTarget(version string, v Target, path string) []Issue {
	out := []Issue{}
	if version == "1" {
		if v.Applicability != "" || v.Reason != "" || v.AnchorKind != "" || v.PointScope != "" || v.QuantityBasis != "" || v.RequiresBoreholeAncestor {
			add(&out, "unsupported_target", "新目标映射需要元模型 2。", path, "")
		}
		return out
	}
	if v.Applicability == "not_applicable" {
		bounded(&out, v.Reason, path+"/reason", 2000, true)
		if v.AnchorKind != "" || v.Component != "" || v.PointScope != "" || v.QuantityBasis != "" || v.RequiresBoreholeAncestor {
			add(&out, "target_not_applicable", "不适用映射只保留原因，不得同时声明目标。", path, "")
		}
		return out
	}
	if v.Applicability != "applicable" {
		add(&out, "target_applicability", "每个类型须明确适用或不适用。", path+"/applicability", "")
	}
	if !slices.Contains([]string{"facility", "component", "monitoring_section", "borehole", "point"}, v.AnchorKind) {
		add(&out, "target_kind", "请选择正式工程对象种类。", path+"/anchor_kind", "")
	}
	if (v.AnchorKind == "component") != (v.Component != "") || v.Component == "root" {
		add(&out, "target_component", "仅构件锚点指定构件类别。", path+"/component", "")
	}
	expectedScope := "descendant_points"
	if v.AnchorKind == "point" {
		expectedScope = "self"
	}
	if v.PointScope != expectedScope {
		add(&out, "target_point_scope", "直接观测须通过锚点自身测点或其后代测点表达。", path+"/point_scope", "")
	}
	if v.QuantityBasis != "per_anchor" && v.QuantityBasis != "per_point" {
		add(&out, "target_quantity_basis", "请选择按锚点或按测点计算数量。", path+"/quantity_basis", "")
	}
	bounded(&out, v.Reason, path+"/reason", 2000, false)
	return out
}

// ValidateTargetType validates fixed dependencies even for default-disabled items.
func ValidateTargetType(version string, v Target, c Content, path string) []Issue {
	if version == "2" && v.Applicability == "not_applicable" {
		return nil
	}
	out := []Issue{}
	kind := v.AnchorKind
	if version == "1" {
		kind = "component"
		if v.Component == "root" {
			kind = "facility"
		}
	}
	if _, ok := StructureFields(c, kind, v.Component); !ok {
		add(&out, "target_not_supported", "固定类型版本不支持此监测目标或构件类别。", path, v.Component)
	}
	if v.RequiresBoreholeAncestor && StructureApplicability(c, "borehole") != "applicable" {
		add(&out, "target_borehole_not_supported", "该类型没有已声明的孔位能力，不能要求孔内测点。", path, "")
	}
	return out
}

func normalizedNode(n Node) Node {
	if n.Kind == "" {
		n.Kind = "component"
	}
	if n.ParentRef != "" {
		n.Parent = n.ParentRef
	}
	if n.ComponentCategoryKey != "" {
		n.Category = n.ComponentCategoryKey
	}
	return n
}

// ValidateStructureSample checks the actual mixed ancestor chain. Component
// cardinalities count the nearest component relation, never organizational layers.
func ValidateStructureSample(c Content, s Sample) []Issue {
	out := ValidateValues(c.Fields, s.Values, "/sample/values")
	if len(s.Nodes) > MaxNodes {
		add(&out, "input_limit", "建模样例最多 500 个节点；实际工程目录另行分页。", "/sample/nodes", "")
		return out
	}
	nodes := map[string]Node{"root": {ID: "root", Kind: "facility", Category: "root"}}
	for i, original := range s.Nodes {
		n := normalizedNode(original)
		p := fmt.Sprintf("/sample/nodes/%d", i)
		if n.ID == "" || len(n.ID) > 64 || n.ID == "root" {
			add(&out, "invalid_node_id", "样例节点需要独立标识。", p+"/id", n.ID)
		}
		if _, exists := nodes[n.ID]; exists {
			add(&out, "duplicate_node", "节点身份重复。", p+"/id", n.ID)
		}
		if original.Kind == "" || original.ParentRef == "" {
			add(&out, "structure_ref_required", "元模型 2 样例须明确 kind 和 parent_ref。", p, n.ID)
		}
		if original.Parent != "" && original.ParentRef != "" && original.Parent != original.ParentRef || original.Category != "" && original.ComponentCategoryKey != "" && original.Category != original.ComponentCategoryKey {
			add(&out, "conflicting_structure", "两种父级或类别表达不一致。", p, n.ID)
		}
		if n.Kind == "facility" || n.Kind != "component" && n.Category != "" {
			add(&out, "structure_kind", "设施只能作为根，只有构件可声明构件类别。", p, n.ID)
		}
		if fs, ok := StructureFields(c, n.Kind, n.Category); !ok {
			add(&out, "structure_not_supported", "该类型未允许此节点种类或构件类别。", p+"/kind", n.ID)
		} else {
			out = append(out, ValidateValues(fs, n.Values, p+"/values")...)
		}
		nodes[n.ID] = n
	}
	children := map[string]map[string]int{}
	for i, original := range s.Nodes {
		n := normalizedNode(original)
		p := fmt.Sprintf("/sample/nodes/%d/parent_ref", i)
		parent, exists := nodes[n.Parent]
		if !exists {
			add(&out, "parent_missing", "父级样例不存在。", p, n.ID)
		} else if parent.Kind == "point" {
			add(&out, "point_not_container", "测点不容纳结构下级。", p, n.ID)
		}
		seen := map[string]bool{n.ID: true}
		cursor, nearest := n.Parent, ""
		for cursor != "" {
			if seen[cursor] {
				add(&out, "sample_cycle", "工程结构存在环。", p, n.ID)
				nearest = ""
				break
			}
			seen[cursor] = true
			ancestor, ok := nodes[cursor]
			if !ok {
				break
			}
			if nearest == "" && (ancestor.Kind == "component" || ancestor.Kind == "facility") {
				nearest = cursor
			}
			if cursor == "root" {
				break
			}
			cursor = ancestor.Parent
		}
		if n.Kind != "component" || nearest == "" {
			continue
		}
		legal := false
		for _, r := range c.Relations {
			if r.Parent == nodes[nearest].Category && r.Child == n.Category {
				legal = true
				break
			}
		}
		if !legal {
			add(&out, "illegal_parent", "最近构件祖先的类别关系不被类型允许。", p, n.ID)
		}
		if children[nearest] == nil {
			children[nearest] = map[string]int{}
		}
		children[nearest][n.Category]++
	}
	for id, n := range nodes {
		if n.Kind != "component" && n.Kind != "facility" {
			continue
		}
		for _, r := range c.Relations {
			if r.Parent != n.Category {
				continue
			}
			count := children[id][r.Child]
			if count < r.MinCount || r.MaxCount != nil && count > *r.MaxCount {
				add(&out, "component_count", fmt.Sprintf("构件 %s 的数量 %d 不符合定义。", r.Child, count), "/sample/nodes/"+id, r.Child)
			}
		}
	}
	slices.SortFunc(out, func(a, b Issue) int { return strings.Compare(a.Path+"/"+a.Code, b.Path+"/"+b.Code) })
	return out
}

type TemplateMatch struct {
	ItemKey       string   `json:"item_key"`
	Applicability string   `json:"applicability"`
	Reason        string   `json:"reason"`
	AnchorIDs     []string `json:"anchor_ids"`
	PointIDs      []string `json:"point_ids"`
	QuantityBasis string   `json:"quantity_basis"`
}

// MatchTemplateSample provides a reference layout, not persisted resource creation
// or automatic source binding. Intermediate kinds never constrain the path.
func MatchTemplateSample(c Content, typeVersion string, s Sample) []TemplateMatch {
	out := []TemplateMatch{}
	if c.Template == nil {
		return out
	}
	nodes := map[string]Node{"root": {ID: "root", Kind: "facility", Category: "root"}}
	for _, n := range s.Nodes {
		nodes[n.ID] = normalizedNode(n)
	}
	for _, item := range c.Template.Items {
		for _, target := range item.Targets {
			if target.TypeVersion != typeVersion {
				continue
			}
			match := TemplateMatch{ItemKey: item.Key, Applicability: target.Applicability, Reason: target.Reason, AnchorIDs: []string{}, PointIDs: []string{}, QuantityBasis: target.QuantityBasis}
			if match.Applicability == "" {
				match.Applicability = "applicable"
			}
			if match.Applicability == "not_applicable" {
				out = append(out, match)
				continue
			}
			kind := target.AnchorKind
			if c.MetamodelVersion == "1" {
				kind = "component"
				if target.Component == "root" {
					kind = "facility"
				}
			}
			anchors := map[string]bool{}
			for id, n := range nodes {
				if n.Kind == kind && (kind != "component" || n.Category == target.Component) {
					anchors[id] = true
					match.AnchorIDs = append(match.AnchorIDs, id)
				}
			}
			for id, n := range nodes {
				if n.Kind != "point" {
					continue
				}
				selected, inHole := false, false
				seen := map[string]bool{}
				for cursor := id; cursor != "" && !seen[cursor]; {
					seen[cursor] = true
					current, ok := nodes[cursor]
					if !ok {
						break
					}
					selected = selected || anchors[cursor]
					inHole = inHole || current.Kind == "borehole"
					cursor = current.Parent
				}
				if selected && (!target.RequiresBoreholeAncestor || inHole) {
					match.PointIDs = append(match.PointIDs, id)
				}
			}
			slices.Sort(match.AnchorIDs)
			slices.Sort(match.PointIDs)
			out = append(out, match)
		}
	}
	return out
}
