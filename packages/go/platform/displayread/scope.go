package displayread

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

var domainFields = map[string][]string{
	"resources": {"resource.metadata", "resource.attributes", "resource.position", "statistics.counts"},
	"channels":  {"resource.metadata", "resource.attributes", "resource.position", "source.binding", "history", "monitoring.values", "monitoring.quality", "statistics.counts"},
	"devices":   {"device.classification", "statistics.classification", "device.metadata", "device.serial_number", "device.description", "device.outputs", "device.installations", "history", "statistics.counts"},
	"spatial":   {"spatial.metadata", "spatial.geometry", "spatial.properties"},
}

func DomainValid(domain string) bool { _, ok := domainFields[domain]; return ok }
func ValidateFormat(format int, hasNew bool, path string) error {
	if format != 0 && format != 1 && format != 2 {
		return Invalid(path, "未知格式版本。")
	}
	if format != 2 && hasNew {
		return Invalid(path, "命名读取视图需要格式 2。")
	}
	return nil
}
func ValidateSchemas(version int, schemas map[string]ViewSchema) error {
	if version != 2 && len(schemas) > 0 {
		return Invalid("/read_view_schema", "命名读取视图需要 Manifest 2。")
	}
	if len(schemas) > 64 {
		return Invalid("/read_view_schema", "最多声明 64 个视图。")
	}
	for name, s := range schemas {
		p := "/read_view_schema/" + pointer(name)
		if s.StructureModel < 0 || s.StructureModel > 2 {
			return Invalid(p+"/structure_model", "Unknown structure model.")
		}
		if !NamePattern.MatchString(name) {
			return Invalid(p, "视图名称无效。")
		}
		if !DomainValid(s.Domain) {
			return Invalid(p+"/domain", "未知领域。")
		}
		if len([]rune(s.Purpose)) > 240 {
			return Invalid(p+"/purpose", "用途最多 240 字符。")
		}
	}
	return nil
}
func ValidateIDs(ids []string, path string, max int) error {
	if len(ids) > max {
		return Problem("budget_exceeded", fmt.Sprintf("%s 最多 %d 个显式标识。", path, max), 429)
	}
	seen := map[string]bool{}
	for i, id := range ids {
		if !access.ValidID(id) || seen[id] {
			return Invalid(fmt.Sprintf("%s/%d", path, i), "标识无效或重复。")
		}
		seen[id] = true
	}
	return nil
}
func enums(values, allowed []string, path string) error {
	seen := map[string]bool{}
	for _, v := range values {
		if !slices.Contains(allowed, v) || seen[v] {
			return Invalid(path, "枚举无效或重复。")
		}
		seen[v] = true
	}
	return nil
}
func ValidateDefinition(d *Definition, view bool, path string) error {
	if !DomainValid(d.Domain) {
		return Invalid(path+"/domain", "未知读取领域。")
	}
	if view {
		if !NamePattern.MatchString(d.ScopeID) || d.ID != "" {
			return Invalid(path+"/scope_id", "配置须引用授权范围。")
		}
	} else if !NamePattern.MatchString(d.ID) || d.ScopeID != "" {
		return Invalid(path+"/id", "授权范围标识无效。")
	}
	if d.StructureModel < 0 || d.StructureModel > 2 {
		return Invalid(path+"/structure_model", "Unknown structure model.")
	}
	d.Selector.StructureModel = d.StructureModel
	if d.StructureModel != 2 {
		for _, kind := range d.Selector.Kinds {
			if NewStructureKind(kind) {
				return StructureUpgradeRequired()
			}
		}
		for _, ref := range d.Selector.Owners {
			if NewStructureKind(ref.Kind) {
				return StructureUpgradeRequired()
			}
		}
	}
	if e := ValidateSelector(d.Domain, &d.Selector, path+"/selector"); e != nil {
		return e
	}
	if e := enums(d.FieldGroups, domainFields[d.Domain], path+"/field_groups"); e != nil {
		return e
	}
	if len(d.FieldGroups) == 0 {
		return Invalid(path+"/field_groups", "必须明确授权字段。")
	}
	if d.TimeWindow != nil {
		if d.Domain != "channels" {
			return Invalid(path+"/time_window", "只有通道可设置时间范围。")
		}
		if _, e := ParseWindow(d.TimeWindow, path+"/time_window"); e != nil {
			return e
		}
	}
	return nil
}
func ValidateSelector(domain string, s *Selector, path string) error {
	if !DomainValid(domain) {
		return Invalid(path, "未知领域。")
	}
	if !slices.Contains([]string{"project", "facilities", "ids"}, s.Kind) || domain == "devices" && s.Kind == "facilities" {
		return Invalid(path+"/kind", "领域不支持该选择方式。")
	}
	if s.Kind == "ids" && len(s.IDs) == 0 && !(domain == "devices" && len(s.OutputIDs) > 0) || s.Kind != "ids" && s.IDs != nil {
		return Invalid(path+"/ids", "ids 选择需明确非空标识。")
	}
	if s.Kind == "facilities" && len(s.FacilityIDs) == 0 {
		return Invalid(path+"/facility_ids", "设施选择不能为空。")
	}
	for name, ids := range map[string][]string{"ids": s.IDs, "facility_ids": s.FacilityIDs, "type_definition_ids": s.TypeIDs, "manufacturer_ids": s.ManufacturerIDs, "product_ids": s.ProductIDs, "output_ids": s.OutputIDs} {
		if ids != nil && len(ids) == 0 {
			return Invalid(path+"/"+name, "显式筛选不能为空。")
		}
		if e := ValidateIDs(ids, path+"/"+name, MaxIDs); e != nil {
			return e
		}
	}
	for name, v := range map[string]bool{"type_definition_ids": s.TypeIDs != nil, "kinds": s.Kinds != nil, "source_kinds": s.SourceKinds != nil, "metric_refs": s.Metrics != nil, "owner_refs": s.Owners != nil, "manufacturer_ids": s.ManufacturerIDs != nil, "product_ids": s.ProductIDs != nil, "output_ids": s.OutputIDs != nil, "asset_kinds": s.AssetKinds != nil, "facility_ids": s.FacilityIDs != nil, "statuses": s.Statuses != nil} {
		if !v {
			continue
		}
		valid := false
		switch name {
		case "type_definition_ids", "kinds":
			valid = domain == "resources"
		case "source_kinds", "metric_refs", "owner_refs":
			valid = domain == "channels"
		case "manufacturer_ids", "product_ids", "output_ids":
			valid = domain == "devices"
		case "asset_kinds":
			valid = domain == "spatial"
		case "facility_ids":
			valid = domain != "devices"
		case "statuses":
			valid = domain == "resources" || domain == "devices"
		}
		if !valid {
			return Invalid(path+"/"+name, "字段不适用于此领域。")
		}
	}
	if e := enums(s.Kinds, []string{"facility", "component", "monitoring_section", "borehole", "point", "channel"}, path+"/kinds"); e != nil {
		return e
	}
	statuses := []string{"normal", "retired"}
	if domain == "devices" {
		statuses = []string{"enabled", "disabled"}
	}
	if e := enums(s.Statuses, statuses, path+"/statuses"); e != nil {
		return e
	}
	if e := enums(s.SourceKinds, []string{"direct", "derived"}, path+"/source_kinds"); e != nil {
		return e
	}
	if e := enums(s.AssetKinds, []string{"layout", "model", "geolayer"}, path+"/asset_kinds"); e != nil {
		return e
	}
	if domain == "channels" && s.Kind != "ids" && len(s.SourceKinds) == 0 {
		s.SourceKinds = []string{"direct"}
	}
	if domain == "channels" && s.Kind != "ids" && slices.Contains(s.SourceKinds, "derived") && len(s.Metrics) == 0 {
		return Invalid(path+"/metric_refs", "派生通道须选择固定指标版本或显式通道。")
	}
	if len(s.Metrics) > MaxIDs || len(s.Owners) > MaxIDs {
		return Problem("budget_exceeded", "指标或归属筛选超过预算。", 429)
	}
	seen := map[MetricRef]bool{}
	for i, m := range s.Metrics {
		if !slices.Contains([]string{"engineering", "derived"}, m.Kind) || !access.ValidID(m.VersionID) || seen[m] {
			return Invalid(fmt.Sprintf("%s/metric_refs/%d", path, i), "指标引用无效或重复。")
		}
		seen[m] = true
	}
	refs := map[Ref]bool{}
	for i, r := range s.Owners {
		if !slices.Contains([]string{"facility", "component", "monitoring_section", "borehole", "point"}, r.Kind) || !access.ValidID(r.ID) || refs[r] {
			return Invalid(fmt.Sprintf("%s/owner_refs/%d", path, i), "归属引用无效或重复。")
		}
		refs[r] = true
	}
	return nil
}

// SelectorSubset deliberately proves only finite, structural implications.
// Equal current memberships never prove a broader future predicate safe.
func SelectorSubset(child, parent Selector) bool {
	if parent.Kind == "ids" && (child.Kind != "ids" || !subset(child.IDs, parent.IDs)) {
		return false
	}
	if parent.Kind == "facilities" && len(child.FacilityIDs) == 0 {
		return false
	}
	for _, sets := range [][2][]string{{child.FacilityIDs, parent.FacilityIDs}, {child.TypeIDs, parent.TypeIDs}, {child.Kinds, parent.Kinds}, {child.Statuses, parent.Statuses}, {child.SourceKinds, parent.SourceKinds}, {child.ManufacturerIDs, parent.ManufacturerIDs}, {child.ProductIDs, parent.ProductIDs}, {child.OutputIDs, parent.OutputIDs}, {child.AssetKinds, parent.AssetKinds}} {
		if len(sets[1]) > 0 && (len(sets[0]) == 0 || !subset(sets[0], sets[1])) {
			return false
		}
	}
	if len(parent.Metrics) > 0 && (len(child.Metrics) == 0 || !subset(child.Metrics, parent.Metrics)) {
		return false
	}
	if len(parent.Owners) > 0 && (len(child.Owners) == 0 || !subset(child.Owners, parent.Owners)) {
		return false
	}
	return true
}
func subset[T comparable](a, b []T) bool {
	set := map[T]bool{}
	for _, v := range b {
		set[v] = true
	}
	for _, v := range a {
		if !set[v] {
			return false
		}
	}
	return true
}
func DefinitionSubset(child, parent Definition) bool {
	if StructureVersion(child.StructureModel) > StructureVersion(parent.StructureModel) || child.Domain != parent.Domain || child.IncludeFuture && !parent.IncludeFuture || !subset(child.FieldGroups, parent.FieldGroups) {
		return false
	}
	if child.IncludeFuture && !SelectorSubset(child.Selector, parent.Selector) {
		return false
	}
	a, e := ParseWindow(child.TimeWindow, "/time_window")
	if e != nil {
		return false
	}
	b, e := ParseWindow(parent.TimeWindow, "/time_window")
	return e == nil && Covers(b, a)
}
func Fields(available, requested, minimum []string) ([]string, error) {
	if len(requested) == 0 {
		requested = minimum
	}
	if !subset(minimum, requested) {
		return nil, Invalid("/field_groups", "缺少方法的最低字段组。")
	}
	if !subset(requested, available) {
		return nil, Problem("field_not_granted", "请求的字段组未获授权。", 403)
	}
	out := slices.Clone(requested)
	slices.Sort(out)
	return slices.Compact(out), nil
}
func ParseWindow(w *Window, path string) (access.ReadWindow, error) {
	out := access.ReadWindow{}
	if w == nil {
		return out, nil
	}
	for _, v := range []struct {
		name string
		from *string
		to   **int64
	}{{"from", w.From, &out.From}, {"to", w.To, &out.To}} {
		if v.from == nil {
			continue
		}
		t, e := time.Parse(time.RFC3339Nano, *v.from)
		if e != nil || t.Year() < 1900 || t.Year() > 2261 {
			return out, Invalid(path+"/"+v.name, "需要 1900–2261 年内的 RFC3339Nano 时刻。")
		}
		n := t.UnixNano()
		*v.to = &n
	}
	if out.From != nil && out.To != nil && *out.From >= *out.To {
		return out, Invalid(path, "结束时刻必须大于开始时刻。")
	}
	return out, nil
}
func Covers(a, b access.ReadWindow) bool {
	return (a.From == nil || b.From != nil && *a.From <= *b.From) && (a.To == nil || b.To != nil && *a.To >= *b.To)
}
func Intersect(a, b access.ReadWindow) (access.ReadWindow, bool) {
	o := a
	if b.From != nil && (o.From == nil || *b.From > *o.From) {
		o.From = b.From
	}
	if b.To != nil && (o.To == nil || *b.To < *o.To) {
		o.To = b.To
	}
	return o, o.From == nil || o.To == nil || *o.From < *o.To
}
func Union(windows []access.ReadWindow) []access.ReadWindow {
	items := slices.Clone(windows)
	slices.SortFunc(items, func(a, b access.ReadWindow) int {
		if a.From == nil {
			if b.From == nil {
				return 0
			}
			return -1
		}
		if b.From == nil {
			return 1
		}
		if *a.From < *b.From {
			return -1
		}
		if *a.From > *b.From {
			return 1
		}
		return 0
	})
	out := []access.ReadWindow{}
	for _, w := range items {
		if w.From != nil && w.To != nil && *w.From >= *w.To {
			continue
		}
		if len(out) == 0 {
			out = append(out, w)
			continue
		}
		last := &out[len(out)-1]
		if last.To == nil || w.From == nil || *last.To >= *w.From {
			if last.To != nil && (w.To == nil || *w.To > *last.To) {
				last.To = w.To
			}
		} else {
			out = append(out, w)
		}
	}
	return out
}
func NormalizeMembers(members []Member) ([]Member, error) {
	slices.SortFunc(members, func(a, b Member) int {
		if n := strings.Compare(a.Kind, b.Kind); n != 0 {
			return n
		}
		return strings.Compare(a.ID, b.ID)
	})
	members = slices.Compact(members)
	if len(members) > MaxMembers {
		return nil, Problem("budget_exceeded", "读取视图成员超过 50,000。", 429)
	}
	return members, nil
}
func IntersectMembers(a, b []Member) []Member {
	set := map[Member]bool{}
	for _, m := range b {
		set[m] = true
	}
	out := []Member{}
	for _, m := range a {
		if set[m] {
			out = append(out, m)
		}
	}
	return out
}
