package compute

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

func pointer(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }

// Decode rejects duplicate, case-alias and unknown fields before typed decode.
// The token walk also bounds nesting and string/number allocation, and reports
// JSON pointers rather than silently dropping misspelled calculation options.
func Decode(raw []byte, out any) error {
	if len(raw) > 256*1024 {
		return issue("plan_limit_exceeded", "", "JSON exceeds 256 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(reflect.Type, string, int) error
	walk = func(t reflect.Type, path string, depth int) error {
		if depth > 64 {
			return issue("plan_limit_exceeded", path, "JSON nesting exceeds 64")
		}
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		token, err := d.Token()
		if err != nil {
			return issue("malformed_request", path, "invalid JSON")
		}
		if token == nil {
			return issue("invalid_rule", path, "null is not allowed")
		}
		delimiter, compound := token.(json.Delim)
		if !compound {
			valid := false
			switch t.Kind() {
			case reflect.String:
				s, ok := token.(string)
				valid = ok && len(s) <= 4096
			case reflect.Int:
				n, ok := token.(json.Number)
				if ok && len(n) <= 4 {
					_, e := n.Int64()
					valid = e == nil
				}
			case reflect.Bool:
				_, valid = token.(bool)
			}
			if !valid {
				return issue("invalid_rule", path, "value has an invalid type or length")
			}
			return nil
		}
		switch delimiter {
		case '{':
			if t.Kind() != reflect.Struct && t.Kind() != reflect.Map {
				return issue("invalid_rule", path, "object not allowed")
			}
			fields := map[string]reflect.Type{}
			if t.Kind() == reflect.Struct {
				for i := 0; i < t.NumField(); i++ {
					f := t.Field(i)
					fields[strings.Split(f.Tag.Get("json"), ",")[0]] = f.Type
				}
			}
			seen := map[string]bool{}
			for d.More() {
				tok, e := d.Token()
				if e != nil {
					return issue("malformed_request", path, "invalid object")
				}
				key, ok := tok.(string)
				if !ok || len(key) > 128 {
					return issue("invalid_rule", path, "invalid key")
				}
				child := path + "/" + pointer(key)
				if seen[key] {
					return issue("duplicate_field", child, "duplicate JSON key")
				}
				seen[key] = true
				var kind reflect.Type
				if t.Kind() == reflect.Map {
					kind = t.Elem()
				} else {
					kind = fields[key]
				}
				if kind == nil {
					return issue("unknown_field", child, "field is not defined")
				}
				if e := walk(kind, child, depth+1); e != nil {
					return e
				}
			}
		case '[':
			if t.Kind() != reflect.Slice {
				return issue("invalid_rule", path, "array not allowed")
			}
			for i := 0; d.More(); i++ {
				if e := walk(t.Elem(), fmt.Sprintf("%s/%d", path, i), depth+1); e != nil {
					return e
				}
			}
		default:
			return issue("malformed_request", path, "unexpected delimiter")
		}
		_, err = d.Token()
		if err != nil {
			return issue("malformed_request", path, "unclosed value")
		}
		return nil
	}
	t := reflect.TypeOf(out)
	if t == nil || t.Kind() != reflect.Pointer {
		return issue("invalid_rule", "", "decoder requires a pointer")
	}
	if err := walk(t, "", 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return issue("malformed_request", "", "trailing JSON")
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return issue("invalid_rule", "", err.Error())
	}
	return nil
}
