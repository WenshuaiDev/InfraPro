package resources

import (
	"bytes"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"io"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

var codePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var mutable = map[Kind][]string{
	Facility: {"name", "values", "position", "description", "source_reference"}, Component: {"name", "values", "position", "description", "parent_component_id"}, Point: {"name", "position", "purpose", "direction_description", "reference_description", "source_reference", "component_id"}, Channel: {"name", "description", "sampling"},
}
var fixed = map[Kind][]string{Facility: {"code", "type_version_id"}, Component: {"code", "category_key"}, Point: {"code"}, Channel: {"code", "metric_version_id", "unit_version_id", "observation_direction", "reference_basis", "timestamp_meaning", "aggregation_description"}}

func strict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var more any
	if e := d.Decode(&more); e != io.EOF {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func parse(t scopedTarget, action string, raw json.RawMessage) (WriteMeta, map[string]any, error) {
	m := WriteMeta{}
	var in map[string]any
	if len(raw) > 256*1024 {
		return m, nil, problem("input_too_large", "单次请求最多 256 KiB。", 413)
	}
	if e := strict(raw, &in); e != nil || in == nil {
		return m, nil, problem("invalid_request", "请提交有效的对象。", 400)
	}
	allowed := []string{"request_id"}
	if t.Entry == "project" {
		allowed = append(allowed, "expected_access_revision", "expected_link_revision")
	}
	if action != "create" {
		allowed = append(allowed, "preview_hash", "reason")
	}
	if action != "create" {
		allowed = append(allowed, "expected_revision")
	}
	if t.Kind == Component {
		allowed = append(allowed, "expected_structure_revision")
	}
	if action != "create" && action != "update" {
		allowed = append(allowed, "reason")
	}
	if action == "create" || action == "update" {
		allowed = append(allowed, mutable[t.Kind]...)
	}
	if action == "create" {
		allowed = append(allowed, fixed[t.Kind]...)
	}
	for k, v := range in {
		if !slices.Contains(allowed, k) {
			return m, nil, problem("invalid_request", "请求包含未知或不可修改字段："+k, 400)
		}
		if v == nil && !slices.Contains([]string{"parent_component_id", "component_id", "unit_version_id", "sampling", "position"}, k) {
			return m, nil, invalid("/"+k, "required", "此字段不能为 null。")
		}
	}
	if sampling, ok := in["sampling"].(map[string]any); ok {
		if _, present := sampling["interval_seconds"]; present && sampling["mode"] != "periodic" {
			return m, nil, invalid("/sampling/interval_seconds", "not_allowed", "事件或人工采样不接受周期参数。")
		}
	}
	meta := map[string]any{}
	for _, k := range []string{"request_id", "expected_revision", "expected_structure_revision", "reason", "preview_hash", "expected_access_revision", "expected_link_revision"} {
		if v, ok := in[k]; ok {
			meta[k] = v
			delete(in, k)
		}
	}
	if strict(jsonBytes(meta), &m) != nil || !access.ValidID(m.RequestID) {
		return m, nil, problem("invalid_request", "请求编号或修订无效。", 400)
	}
	if t.Entry == "project" && (!decimalRevision(m.AccessRevision) || (action != "create" || t.Kind != Facility) && !decimalRevision(m.LinkRevision)) {
		return m, nil, problem("invalid_request", "项目写入须提供当前资源授权与关联修订。", 400)
	}
	m.Reason = strings.TrimSpace(m.Reason)
	if utf8.RuneCountInString(m.Reason) > 2000 {
		return m, nil, invalid("/reason", "input_limit", "原因最多 2000 字符。")
	}
	if action != "create" && action != "update" && (m.Reason == "" || utf8.RuneCountInString(m.Reason) > 2000) {
		return m, nil, invalid("/reason", "required", "请填写原因，最多 2000 字符。")
	}
	for _, k := range []string{"name", "code"} {
		if v, ok := in[k].(string); ok {
			in[k] = strings.TrimSpace(v)
		}
	}
	return m, in, nil
}
func unmarshalResource(b []byte, r *Resource) error { return json.Unmarshal(b, r) }
func apply(r Resource, in map[string]any) (Resource, error) {
	// Decode each submitted field through the response model, without accepting server fields.
	// Strict nested decoding rejects unknown position and sampling members.
	var copy Resource
	_ = json.Unmarshal(jsonBytes(r), &copy)
	r = copy
	if hasKey(in, "values") {
		r.Values = nil
	}
	if hasKey(in, "position") {
		r.Position = nil
	}
	if hasKey(in, "sampling") {
		r.Sampling = nil
	}
	if e := strict(jsonBytes(in), &r); e != nil {
		return r, problem("invalid_request", "字段类型或嵌套内容无效。", 400)
	}
	if r.Position == nil && r.Kind != Channel {
		r.Position = &Position{}
	}
	if r.Position != nil {
		if r.Position.LinearReferences == nil {
			r.Position.LinearReferences = []LinearReference{}
		}
		if r.Position.Sections == nil {
			r.Position.Sections = []Section{}
		}
	}
	if r.Values == nil && (r.Kind == Facility || r.Kind == Component) {
		r.Values = map[string]any{}
	}
	return r, nil
}
func validateBasic(r Resource) error {
	if !codePattern.MatchString(r.Code) {
		return invalid("/code", "code", "编号为 1–64 位字母、数字、点、下划线或连字符，首位须为字母或数字。")
	}
	if strings.TrimSpace(r.Name) == "" || utf8.RuneCountInString(r.Name) > 200 {
		return invalid("/name", "name", "名称须为 1–200 字符。")
	}
	for k, v := range map[string]string{"description": r.Description, "source_reference": r.SourceReference, "purpose": r.Purpose, "direction_description": r.DirectionDescription, "reference_description": r.ReferenceDescription, "observation_direction": r.ObservationDirection, "reference_basis": r.ReferenceBasis, "aggregation_description": r.AggregationDescription} {
		max := 2000
		if k == "source_reference" {
			max = 1000
		}
		if utf8.RuneCountInString(v) > max {
			return invalid("/"+k, "input_limit", "说明超出长度限制。")
		}
	}
	for k, v := range map[string]*string{"parent_component_id": r.ParentComponentID, "component_id": r.ComponentID, "unit_version_id": r.UnitVersionID} {
		if v != nil && !access.ValidID(*v) {
			return invalid("/"+k, "reference", "请提供有效引用或 null。")
		}
	}
	if len(jsonBytes(r.Values)) > 128*1024 {
		return problem("input_too_large", "属性值最多 128 KiB。", 413)
	}
	if e := ValidatePosition(r.Position); e != nil {
		return e
	}
	if r.Kind == Channel {
		if r.TimestampMeaning != "observation_time" && r.TimestampMeaning != "interval_end" {
			return invalid("/timestamp_meaning", "timestamp_meaning", "请选择观测时刻或统计区间结束时刻。")
		}
		if r.TimestampMeaning == "interval_end" && strings.TrimSpace(r.AggregationDescription) == "" {
			return invalid("/aggregation_description", "required", "请说明统计区间含义。")
		}
		if p := r.Sampling; p != nil {
			if utf8.RuneCountInString(p.Description) > 2000 {
				return invalid("/sampling/description", "input_limit", "最多 2000 字符。")
			}
			switch p.Mode {
			case "periodic":
				if p.IntervalSeconds == nil || *p.IntervalSeconds <= 0 || math.IsNaN(*p.IntervalSeconds) || math.IsInf(*p.IntervalSeconds, 0) || *p.IntervalSeconds > engineering.MaxSafeInteger {
					return invalid("/sampling/interval_seconds", "positive", "周期须为安全范围内的有限正数。")
				}
			case "event", "manual":
				if p.IntervalSeconds != nil {
					return invalid("/sampling/interval_seconds", "not_allowed", "事件或人工采样不接受周期。")
				}
			default:
				return invalid("/sampling/mode", "sampling_mode", "请选择周期、事件或人工采样。")
			}
		}
	}
	return nil
}
