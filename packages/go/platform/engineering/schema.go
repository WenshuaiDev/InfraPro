package engineering

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type noExternalSchemas struct{}

func (noExternalSchemas) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema loading is disabled")
}

// FieldSchema is generated solely from the bounded metamodel. User-authored schemas,
// remote references, executable keywords and dynamic defaults never enter the compiler.
func FieldSchema(f Field) map[string]any {
	s := map[string]any{}
	switch f.Shape {
	case "text":
		s["type"] = "string"
		if f.MinLength != nil {
			s["minLength"] = *f.MinLength
		}
		if f.Required && (f.MinLength == nil || *f.MinLength < 1) {
			s["minLength"] = 1
		}
		if f.MaxLength != nil {
			s["maxLength"] = *f.MaxLength
		} else {
			s["maxLength"] = 10000
		}
	case "integer", "decimal":
		s["type"] = "number"
		if f.Shape == "integer" {
			s["type"] = "integer"
		}
		s["minimum"] = -MaxSafeInteger
		s["maximum"] = MaxSafeInteger
		if f.Minimum != nil {
			s["minimum"] = *f.Minimum
		}
		if f.Maximum != nil {
			s["maximum"] = *f.Maximum
		}
	case "boolean":
		s["type"] = "boolean"
	case "date", "datetime":
		s["type"] = "string"
		s["format"] = "date"
		if f.Shape == "datetime" {
			s["format"] = "date-time"
		}
	case "enum", "multi_enum":
		opts := []string{}
		for _, o := range f.Options {
			opts = append(opts, o.Key)
		}
		if f.Shape == "enum" {
			s["type"] = "string"
			s["enum"] = opts
		} else {
			s["type"] = "array"
			s["items"] = map[string]any{"type": "string", "enum": opts}
			s["uniqueItems"] = true
			if f.MinItems != nil {
				s["minItems"] = *f.MinItems
			}
			if f.Required && (f.MinItems == nil || *f.MinItems < 1) {
				s["minItems"] = 1
			}
			if f.MaxItems != nil {
				s["maxItems"] = *f.MaxItems
			} else {
				s["maxItems"] = len(opts)
			}
		}
	}
	return s
}
func FieldsSchema(fs []Field) map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, f := range fs {
		props[f.Key] = FieldSchema(f)
		if f.Required {
			required = append(required, f.Key)
		}
	}
	return map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "properties": props, "required": required, "additionalProperties": false}
}

var schemaCache = struct {
	sync.Mutex
	values map[string]*jsonschema.Schema
}{values: map[string]*jsonschema.Schema{}}

func compile(doc map[string]any) (*jsonschema.Schema, error) {
	cacheKey := hash(doc)
	schemaCache.Lock()
	cached := schemaCache.values[cacheKey]
	schemaCache.Unlock()
	if cached != nil {
		return cached, nil
	}
	// Convert Go slices/numbers into the JSON data model expected by the compiler.
	raw, e := json.Marshal(doc)
	if e != nil {
		return nil, e
	}
	var v any
	if e = json.Unmarshal(raw, &v); e != nil {
		return nil, e
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	c.UseLoader(noExternalSchemas{})
	const url = "https://infrapro.invalid/internal-schema"
	if e = c.AddResource(url, v); e != nil {
		return nil, e
	}
	compiled, e := c.Compile(url)
	if e == nil {
		schemaCache.Lock()
		if len(schemaCache.values) >= 1024 {
			schemaCache.values = map[string]*jsonschema.Schema{}
		}
		schemaCache.values[cacheKey] = compiled
		schemaCache.Unlock()
	}
	return compiled, e
}
func Schemas(c Content) map[string]any {
	out := map[string]any{"root": FieldsSchema(c.Fields)}
	for _, v := range c.Components {
		out[v.Key] = FieldsSchema(v.Fields)
	}
	return out
}

// ValidateValues never changes input, fills defaults or coerces types.
func ValidateValues(fs []Field, values map[string]any, path string) []Issue {
	out := []Issue{}
	known := map[string]bool{}
	for _, f := range fs {
		known[f.Key] = true
		v, exists := values[f.Key]
		p := path + "/" + f.Key
		if !exists {
			if f.Required {
				add(&out, "required", "请填写必填属性。", p, f.Key)
			}
			continue
		}
		sc, e := compile(FieldSchema(f))
		if e != nil {
			add(&out, "invalid_field_definition", "属性定义尚不完整。", p, f.Key)
			continue
		}
		raw, e := json.Marshal(v)
		var normalized any
		if e == nil {
			e = json.Unmarshal(raw, &normalized)
		}
		if e != nil || sc.Validate(normalized) != nil {
			add(&out, "invalid_value", "值不符合属性的数据形态、选项或范围约束。", p, f.Key)
			continue
		}
		if f.Shape == "datetime" || f.Shape == "date" {
			s, ok := v.(string)
			layout := "2006-01-02"
			if f.Shape == "datetime" {
				layout = time.RFC3339Nano
			}
			_, e := time.Parse(layout, s)
			if !ok || e != nil {
				add(&out, "invalid_date", "请使用真实日期；日期时间须包含时区偏移。", p, f.Key)
			}
		}
		if n, ok := normalized.(float64); ok && (math.Abs(n) > MaxSafeInteger || math.IsInf(n, 0) || math.IsNaN(n)) {
			add(&out, "numeric_range", "数值超出共同支持的范围。", p, f.Key)
		}
	}
	for k := range values {
		if !known[k] {
			add(&out, "unknown_property", "该属性未在定义中声明。", path+"/"+k, k)
		}
	}
	slices.SortFunc(out, func(a, b Issue) int { return strings.Compare(a.Path, b.Path) })
	return out
}
func ValidateSample(c Content, s Sample) []Issue {
	out := ValidateValues(c.Fields, s.Values, "/sample/values")
	if len(s.Nodes) > MaxNodes {
		add(&out, "input_limit", "样例最多 500 个构件。", "/sample/nodes", "")
		return out
	}
	nodes := map[string]Node{"root": {ID: "root", Category: "root"}}
	indices := map[string]int{}
	cats := map[string][]Field{}
	for _, c := range c.Components {
		cats[c.Key] = c.Fields
	}
	pairs := map[string]Relation{}
	for _, r := range c.Relations {
		pairs[r.Parent+"/"+r.Child] = r
	}
	for i, n := range s.Nodes {
		p := fmt.Sprintf("/sample/nodes/%d", i)
		if n.ID == "" || len(n.ID) > 64 || n.ID == "root" {
			add(&out, "invalid_node_id", "构件样例需要独立标识。", p+"/id", n.ID)
		}
		if _, ok := nodes[n.ID]; ok {
			add(&out, "duplicate_node", "构件样例标识重复，不能重复父级。", p+"/id", n.ID)
		}
		nodes[n.ID] = n
		indices[n.ID] = i
		fs, ok := cats[n.Category]
		if !ok {
			add(&out, "unknown_component", "未定义的构件类别。", p+"/category", n.ID)
		} else {
			out = append(out, ValidateValues(fs, n.Values, p+"/values")...)
		}
	}
	children := map[string]map[string]int{}
	for i, n := range s.Nodes {
		p := fmt.Sprintf("/sample/nodes/%d", i)
		parent, ok := nodes[n.Parent]
		if !ok {
			add(&out, "parent_missing", "父级样例不存在。", p+"/parent", n.ID)
		} else if _, ok = pairs[parent.Category+"/"+n.Category]; !ok {
			add(&out, "illegal_parent", "此父子类别关系不被类型定义允许。", p+"/parent", n.ID)
		}
		if children[n.Parent] == nil {
			children[n.Parent] = map[string]int{}
		}
		children[n.Parent][n.Category]++
		seen := map[string]bool{}
		current := n.ID
		for current != "root" {
			if seen[current] {
				add(&out, "sample_cycle", "样例组成存在环。", p+"/parent", n.ID)
				break
			}
			seen[current] = true
			v, ok := nodes[current]
			if !ok {
				break
			}
			current = v.Parent
		}
	}
	for id, n := range nodes {
		p := "/sample/nodes"
		if id != "root" {
			p = fmt.Sprintf("/sample/nodes/%d", indices[id])
		}
		for _, r := range c.Relations {
			if r.Parent != n.Category {
				continue
			}
			count := children[id][r.Child]
			if count < r.MinCount || (r.MaxCount != nil && count > *r.MaxCount) {
				add(&out, "component_count", fmt.Sprintf("构件 %s 的数量 %d 不符合定义。", r.Child, count), p, r.Child)
			}
		}
	}
	return out
}
