package monitoring

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
)

// DecodeRequest rejects duplicate keys before strict typed decoding. Only
// explicit range endpoints and snapshot/interval references may be null.
func DecodeRequest(raw []byte, out any) error {
	bad := func() error {
		return problem("invalid_request", "请求包含未知、重复、空值或类型不符的字段。", 400)
	}
	if len(raw) > 256*1024 {
		return problem("input_too_large", "单次 JSON 请求最多 256 KiB。", 413)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return bad()
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(string, int) error
	walk = func(key string, depth int) error {
		if depth > 64 {
			return bad()
		}
		token, e := d.Token()
		if e != nil {
			return bad()
		}
		if token == nil && key != "from" && key != "to" && key != "snapshot_id" && key != "interval_start" {
			return bad()
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				v, e := d.Token()
				if e != nil {
					return bad()
				}
				k, ok := v.(string)
				if !ok || seen[k] {
					return bad()
				}
				seen[k] = true
				if e = walk(k, depth+1); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = walk("item", depth+1); e != nil {
					return e
				}
			}
		default:
			return bad()
		}
		_, e = d.Token()
		if e != nil {
			return bad()
		}
		return nil
	}
	if e := walk("", 0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return bad()
	}
	var tree any
	shapeDecoder := json.NewDecoder(bytes.NewReader(raw))
	shapeDecoder.UseNumber()
	if e := shapeDecoder.Decode(&tree); e != nil || !exactJSONKeys(tree, reflect.TypeOf(out)) {
		return bad()
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return bad()
	}
	return nil
}

// encoding/json accepts case-insensitive struct keys. The public contract does
// not: alternate spellings could otherwise bypass duplicate-key rejection.
func exactJSONKeys(value any, t reflect.Type) bool {
	if t == nil {
		return false
	}
	if value == nil {
		return t.Kind() == reflect.Pointer
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeFor[json.RawMessage]() {
		return true
	}
	switch t.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return false
		}
		fields := map[string]reflect.Type{}
		jsonFields(t, fields)
		for key, child := range object {
			kind, ok := fields[key]
			if ok && key == "snapshot_id" && child == nil && kind.Kind() == reflect.String {
				continue
			}
			if !ok || !exactJSONKeys(child, kind) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		values, ok := value.([]any)
		if !ok {
			return false
		}
		for _, child := range values {
			if !exactJSONKeys(child, t.Elem()) {
				return false
			}
		}
	case reflect.Map:
		object, ok := value.(map[string]any)
		if !ok || t.Key().Kind() != reflect.String {
			return false
		}
		for _, child := range object {
			if !exactJSONKeys(child, t.Elem()) {
				return false
			}
		}
	}
	return true
}
func jsonFields(t reflect.Type, out map[string]reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if field.Anonymous && name == "" {
			child := field.Type
			for child.Kind() == reflect.Pointer {
				child = child.Elem()
			}
			if child.Kind() == reflect.Struct {
				jsonFields(child, out)
				continue
			}
		}
		if name == "" {
			name = field.Name
		}
		out[name] = field.Type
	}
}
