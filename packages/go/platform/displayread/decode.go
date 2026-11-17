package displayread

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// Decode checks the JSON token stream before decoding so Go, JavaScript and
// PostgreSQL cannot disagree about duplicate keys. Paths are JSON pointers.
func Decode(raw []byte, out any) error {
	if len(raw) > 1<<20 {
		return Invalid("/", "JSON 超过限制。")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var scan func(string) error
	scan = func(path string) error {
		t, e := d.Token()
		if e != nil {
			return Invalid(path, "JSON 无效。")
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return Invalid(path, "JSON 无效。")
				}
				key, ok := k.(string)
				if !ok {
					return Invalid(path, "JSON 对象键无效。")
				}
				p := path + "/" + pointer(key)
				if seen[key] {
					return Invalid(p, "重复 JSON 字段。")
				}
				seen[key] = true
				if e = scan(p); e != nil {
					return e
				}
			}
		case '[':
			for i := 0; d.More(); i++ {
				if e := scan(fmt.Sprintf("%s/%d", path, i)); e != nil {
					return e
				}
			}
		default:
			return Invalid(path, "JSON 分隔符无效。")
		}
		_, e = d.Token()
		if e != nil {
			return Invalid(path, "JSON 无效。")
		}
		return nil
	}
	if e := scan(""); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return Invalid("/", "JSON 含多余内容。")
	}
	var value any
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if e := d.Decode(&value); e != nil {
		return Invalid("/", "JSON 无效。")
	}
	if e := checkFields(value, reflect.TypeOf(out), ""); e != nil {
		return e
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	d.UseNumber()
	if e := d.Decode(out); e != nil {
		if v, ok := e.(*json.UnmarshalTypeError); ok {
			return Invalid("/"+strings.ReplaceAll(v.Field, ".", "/"), "字段类型无效。")
		}
		return Invalid("/", "JSON 字段或类型无效。")
	}
	return nil
}
func pointer(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }
func checkFields(v any, t reflect.Type, path string) error {
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	// Custom scalar decoders (time, RawMessage, exact numbers) own their format.
	if reflect.PointerTo(t).Implements(reflect.TypeFor[json.Unmarshaler]()) {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		fields := map[string]reflect.Type{}
		var collect func(reflect.Type)
		collect = func(st reflect.Type) {
			for i := 0; i < st.NumField(); i++ {
				f := st.Field(i)
				if !f.IsExported() {
					continue
				}
				tag := strings.Split(f.Tag.Get("json"), ",")[0]
				if tag == "-" {
					continue
				}
				if f.Anonymous && tag == "" {
					ft := f.Type
					for ft.Kind() == reflect.Pointer {
						ft = ft.Elem()
					}
					if ft.Kind() == reflect.Struct {
						collect(ft)
						continue
					}
				}
				if tag == "" {
					tag = f.Name
				}
				fields[tag] = f.Type
			}
		}
		collect(t)
		for k, x := range m {
			p := path + "/" + pointer(k)
			ft, ok := fields[k]
			if !ok {
				return Invalid(p, "未知字段。")
			}
			if e := checkFields(x, ft, p); e != nil {
				return e
			}
		}
	case reflect.Map:
		if m, ok := v.(map[string]any); ok {
			for k, x := range m {
				if e := checkFields(x, t.Elem(), path+"/"+pointer(k)); e != nil {
					return e
				}
			}
		}
	case reflect.Slice, reflect.Array:
		if a, ok := v.([]any); ok {
			for i, x := range a {
				if e := checkFields(x, t.Elem(), fmt.Sprintf("%s/%d", path, i)); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
