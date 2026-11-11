package engineering

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var shapes = []string{"text", "integer", "decimal", "boolean", "date", "datetime", "enum", "multi_enum"}

func issue(code, message, path, key string) Issue {
	return Issue{Code: code, Message: message, Path: path, Key: key, Severity: "error"}
}
func add(out *[]Issue, code, message, path, key string) {
	*out = append(*out, issue(code, message, path, key))
}
func bounded(out *[]Issue, value, path string, max int, required bool) {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > max || (required && strings.TrimSpace(value) == "") {
		add(out, "text_constraint", fmt.Sprintf("请填写有效内容（最多 %d 字符）。", max), path, "")
	}
}
func key(out *[]Issue, value, path string, seen map[string]bool) {
	if !keyPattern.MatchString(value) || value == "root" {
		add(out, "invalid_key", "键须以小写字母开头，使用字母、数字、下划线，最多 64 字符；root 为保留键。", path, value)
	}
	if seen[value] {
		add(out, "duplicate_key", "稳定键重复。", path, value)
	}
	seen[value] = true
}
func counts(out *[]Issue, min int, max *int, path, k string) {
	if min < 0 || min > MaxNodes || (max != nil && (*max < min || *max > MaxNodes)) {
		add(out, "invalid_count", "数量必须自洽且在 0–500 内；不设业务上限时省略最大值。", path, k)
	}
}
func options(out *[]Issue, opts []Option, path string) {
	if len(opts) == 0 || len(opts) > 200 {
		add(out, "options_required", "需要 1–200 个选项。", path, "")
	}
	seen := map[string]bool{}
	for i, v := range opts {
		p := fmt.Sprintf("%s/%d", path, i)
		key(out, v.Key, p+"/key", seen)
		bounded(out, v.Name, p+"/name", 100, true)
	}
}
func fields(out *[]Issue, fs []Field, path string) {
	if len(fs) > MaxFields {
		add(out, "input_limit", "每个对象最多 100 个属性。", path, "")
	}
	seen := map[string]bool{}
	for i, f := range fs {
		p := fmt.Sprintf("%s/%d", path, i)
		key(out, f.Key, p+"/key", seen)
		bounded(out, f.Name, p+"/name", 100, true)
		bounded(out, f.Description, p+"/description", 2000, false)
		bounded(out, f.Group, p+"/group", 100, false)
		if !slices.Contains(shapes, f.Shape) {
			add(out, "unsupported_shape", "不支持的属性形态。", p+"/shape", f.Key)
			continue
		}
		numeric := f.Shape == "integer" || f.Shape == "decimal"
		enumerated := f.Shape == "enum" || f.Shape == "multi_enum"
		if (!numeric && (f.Minimum != nil || f.Maximum != nil || f.UnitVersion != "" || f.Precision != nil)) || (f.Shape != "text" && (f.MinLength != nil || f.MaxLength != nil || f.Multiline)) || (f.Shape != "multi_enum" && (f.MinItems != nil || f.MaxItems != nil)) || (!enumerated && len(f.Options) > 0) {
			add(out, "unsupported_constraint", "该属性形态不支持这些约束或控件。", p, f.Key)
		}
		if numeric {
			numberRange(out, f.Minimum, f.Maximum, p, f.Key)
			if f.Shape == "integer" {
				for _, v := range []*float64{f.Minimum, f.Maximum} {
					if v != nil && math.Trunc(*v) != *v {
						add(out, "integer_range", "整数边界须为安全整数。", p, f.Key)
					}
				}
			}
		}
		if f.Precision != nil && (*f.Precision < 0 || *f.Precision > 12) {
			add(out, "invalid_precision", "显示精度须为 0–12；不会改变实际值。", p+"/precision", f.Key)
		}
		if f.Shape == "text" {
			lengthRange(out, f.MinLength, f.MaxLength, p, f.Key, 10000)
		}
		if enumerated {
			options(out, f.Options, p+"/options")
		}
		if f.Shape == "multi_enum" {
			lengthRange(out, f.MinItems, f.MaxItems, p, f.Key, len(f.Options))
			if f.Required && f.MaxItems != nil && *f.MaxItems == 0 {
				add(out, "invalid_count", "必填多选至少选择一项。", p, f.Key)
			}
		}
		if len(f.Default) > 0 {
			var value any
			if json.Unmarshal(f.Default, &value) != nil || len(ValidateValues([]Field{f}, map[string]any{f.Key: value}, p+"/default")) > 0 {
				add(out, "invalid_default", "默认值必须通过该属性的全部规则。", p+"/default", f.Key)
			}
		}
	}
}
func lengthRange(out *[]Issue, min, max *int, path, k string, limit int) {
	if (min != nil && (*min < 0 || *min > limit)) || (max != nil && (*max < 0 || *max > limit)) || (min != nil && max != nil && *max < *min) {
		add(out, "invalid_range", "长度或选项数量上下限不合法。", path, k)
	}
}
func numberRange(out *[]Issue, min, max *float64, path, k string) {
	for _, v := range []*float64{min, max} {
		if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || math.Abs(*v) > MaxSafeInteger) {
			add(out, "numeric_range", "数值须在共同精确支持的范围内。", path, k)
		}
	}
	if min != nil && max != nil && *min > *max {
		add(out, "invalid_range", "上限不能小于下限。", path, k)
	}
}

// ValidateContent checks the supported metamodel, independently of storage or business facility names.
func ValidateContent(kind Kind, c Content) []Issue {
	out := []Issue{}
	if c.MetamodelVersion != MetamodelVersion {
		add(&out, "unsupported_metamodel", "不支持的元模型格式版本。", "/metamodel_version", "")
	}
	bounded(&out, c.Name, "/name", 200, true)
	bounded(&out, c.Description, "/description", 4000, false)
	bounded(&out, c.ReleaseNotes, "/release_notes", 2000, false)
	if (kind != TypeDefinition && (len(c.Fields) > 0 || len(c.Components) > 0 || len(c.Relations) > 0)) || (kind != UnitDefinition && c.Unit != nil) || (kind != MetricDefinition && c.Metric != nil) || (kind != MonitoringTemplate && c.Template != nil) {
		add(&out, "unsupported_content", "该目录不接受其他种类的定义内容。", "/", "")
	}
	switch kind {
	case TypeDefinition:
		fields(&out, c.Fields, "/fields")
		cats := map[string]bool{"root": true}
		if len(c.Components) > MaxComponents || len(c.Relations) > 500 {
			add(&out, "input_limit", "构件类别最多 100 个，组成关系最多 500 条。", "/components", "")
		}
		for i, v := range c.Components {
			p := fmt.Sprintf("/components/%d", i)
			key(&out, v.Key, p+"/key", cats)
			bounded(&out, v.Name, p+"/name", 100, true)
			bounded(&out, v.Description, p+"/description", 2000, false)
			fields(&out, v.Fields, p+"/fields")
		}
		pairs := map[string]bool{}
		graph := map[string][]string{}
		for i, r := range c.Relations {
			p := fmt.Sprintf("/relations/%d", i)
			counts(&out, r.MinCount, r.MaxCount, p, r.Child)
			if !cats[r.Parent] || !cats[r.Child] || r.Child == "root" {
				add(&out, "invalid_relation", "父级和子级必须存在，根对象不能作为子级。", p, r.Child)
			}
			pair := r.Parent + "/" + r.Child
			if pairs[pair] {
				add(&out, "duplicate_relation", "组成关系重复。", p, r.Child)
			}
			pairs[pair] = true
			graph[r.Parent] = append(graph[r.Parent], r.Child)
		}
		visited := map[string]int{}
		var visit func(string)
		visit = func(k string) {
			if visited[k] == 1 {
				add(&out, "relation_cycle", "构件类别关系存在环。", "/relations", k)
				return
			}
			if visited[k] == 2 {
				return
			}
			visited[k] = 1
			for _, child := range graph[k] {
				visit(child)
			}
			visited[k] = 2
		}
		visit("root")
		for _, v := range c.Components {
			if visited[v.Key] == 0 {
				add(&out, "unreachable_component", "构件类别不能从根对象到达。", "/relations", v.Key)
				visit(v.Key)
			}
		}
	case UnitDefinition:
		if c.Unit == nil {
			add(&out, "unit_required", "请配置单位。", "/unit", "")
		} else {
			bounded(&out, c.Unit.Symbol, "/unit/symbol", 30, true)
			if !keyPattern.MatchString(c.Unit.Category) {
				add(&out, "category_required", "请选择单位类别。", "/unit/category", c.Unit.Category)
			}
		}
	case MetricDefinition:
		if c.Metric == nil {
			add(&out, "metric_required", "请配置指标。", "/metric", "")
			break
		}
		m := c.Metric
		if m.Shape == "enum" {
			options(&out, m.Options, "/metric/options")
			if m.Category != "" || m.StandardUnit != "" || len(m.AllowedUnits) > 0 || m.Minimum != nil || m.Maximum != nil {
				add(&out, "unsupported_constraint", "枚举状态不配置物理单位和数值范围。", "/metric", "")
			}
		} else if m.Shape == "integer" || m.Shape == "decimal" {
			numberRange(&out, m.Minimum, m.Maximum, "/metric", "")
			if len(m.Options) > 0 {
				add(&out, "unsupported_constraint", "数值指标不配置枚举选项。", "/metric/options", "")
			}
			if m.Category == "" || m.StandardUnit == "" || !slices.Contains(m.AllowedUnits, m.StandardUnit) {
				add(&out, "standard_unit_required", "数值指标需要单位类别及属于允许单位的标准单位。", "/metric/standard_unit", "")
			}
			seen := map[string]bool{}
			for _, u := range m.AllowedUnits {
				if seen[u] {
					add(&out, "duplicate_reference", "允许单位重复。", "/metric/allowed_units", u)
				}
				seen[u] = true
			}
			if m.Shape == "integer" {
				for _, v := range []*float64{m.Minimum, m.Maximum} {
					if v != nil && math.Trunc(*v) != *v {
						add(&out, "integer_range", "整数边界须为安全整数。", "/metric", "")
					}
				}
			}
		} else {
			add(&out, "unsupported_shape", "指标仅支持单值整数、小数和枚举状态。", "/metric/shape", "")
		}
	case MonitoringTemplate:
		if c.Template == nil {
			add(&out, "template_required", "请配置监测模板。", "/template", "")
			break
		}
		t := c.Template
		bounded(&out, t.Purpose, "/template/purpose", 2000, false)
		if len(t.TypeVersions) == 0 || len(t.TypeVersions) > 20 {
			add(&out, "targets_required", "请选择 1–20 个适用类型版本。", "/template/type_versions", "")
		}
		targets := map[string]bool{}
		for _, id := range t.TypeVersions {
			if targets[id] {
				add(&out, "duplicate_reference", "适用类型版本重复。", "/template/type_versions", id)
			}
			targets[id] = true
		}
		gs := map[string]bool{}
		for i, g := range t.Groups {
			p := fmt.Sprintf("/template/groups/%d", i)
			key(&out, g.Key, p+"/key", gs)
			bounded(&out, g.Name, p+"/name", 100, true)
			bounded(&out, g.Description, p+"/description", 2000, false)
		}
		if len(t.Items) == 0 || len(t.Items) > 200 || len(t.Groups) > 100 {
			add(&out, "items_required", "需要 1–200 个监测项，分组最多 100 个。", "/template/items", "")
		}
		items := map[string]bool{}
		for i, item := range t.Items {
			p := fmt.Sprintf("/template/items/%d", i)
			key(&out, item.Key, p+"/key", items)
			bounded(&out, item.Name, p+"/name", 100, true)
			bounded(&out, item.Description, p+"/description", 2000, false)
			if item.Group != "" && !gs[item.Group] {
				add(&out, "group_missing", "监测分组不存在。", p+"/group", item.Key)
			}
			seen := map[string]bool{}
			for j, v := range item.Targets {
				tp := fmt.Sprintf("%s/targets/%d", p, j)
				if !targets[v.TypeVersion] || seen[v.TypeVersion] {
					add(&out, "target_mapping", "映射须对应一个适用版本且不可重复。", tp, item.Key)
				}
				seen[v.TypeVersion] = true
				if v.Component == "" {
					add(&out, "target_missing", "请选择根对象或构件类别。", tp, item.Key)
				}
			}
			for _, id := range t.TypeVersions {
				if !seen[id] {
					add(&out, "target_mapping", "每个适用类型版本均需显式目标映射。", p+"/targets", item.Key)
				}
			}
			if len(item.Bindings) == 0 || len(item.Bindings) > 20 {
				add(&out, "binding_required", "每项需要 1–20 条指标与资源绑定要求。", p+"/bindings", item.Key)
			}
			for j, b := range item.Bindings {
				bp := fmt.Sprintf("%s/bindings/%d", p, j)
				counts(&out, b.MinCount, b.MaxCount, bp, item.Key)
				if b.MetricVersion == "" {
					add(&out, "metric_required", "请选择指标发布版本。", bp+"/metric_version", item.Key)
				}
				if b.ResourceKind != "observation_channel" {
					add(&out, "unsupported_resource", "本轮支持观测通道绑定要求，不包含实际资源身份。", bp+"/resource_kind", item.Key)
				}
			}
		}
	default:
		add(&out, "unsupported_kind", "不支持的定义目录。", "/", "")
	}
	return out
}
