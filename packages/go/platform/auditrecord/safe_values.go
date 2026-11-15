package auditrecord

import (
	"bytes"
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Credentials are never stored, including inside otherwise allowed free text.
// A URL is not an audit target; remove it wholesale rather than trying to
// enumerate every provider's signed query parameters.
var unsafeText = regexp.MustCompile(`(?i)(?:https?|s3|postgres(?:ql)?|redis)://[^\s<>"']+|(?:bearer\s+[^\s,;]+)|(?:(?:password|passwd|secret|token|cookie|authorization|api[_-]?key|access[_-]?key|密码|令牌|密钥)\s*[:=：]\s*[^\s,;]+)|-----BEGIN [^-]*PRIVATE KEY-----[\s\S]*`)

func SafeText(v string, max int) (string, ValueState) {
	state := Recorded
	if unsafeText.MatchString(v) {
		v = unsafeText.ReplaceAllString(v, "[redacted]")
		state = Redacted
	}
	v = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, v)
	if !utf8.ValidString(v) {
		v = strings.ToValidUTF8(v, "�")
	}
	if utf8.RuneCountInString(v) > max {
		v = string([]rune(v)[:max])
		if state != Redacted {
			state = Truncated
		}
	}
	return v, state
}
func Snapshot(v any, fields []string) (any, error) {
	if v == nil {
		return nil, nil
	}
	b, e := json.Marshal(v)
	if e != nil {
		return nil, ErrInvalid
	}
	if len(b) > 4*1024*1024 {
		return nil, ErrInvalid
	}
	var value any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if dec.Decode(&value) != nil {
		return nil, ErrInvalid
	}
	return safeValue(value, fields, 0), nil
}
func safeValue(v any, fields []string, depth int) any {
	if depth > 6 {
		return map[string]any{"value_state": Truncated}
	}
	switch x := v.(type) {
	case string:
		s, state := SafeText(x, 500)
		if state != Recorded {
			out := map[string]any{"value": s, "value_state": state}
			if utf8.RuneCountInString(x) > 500 {
				// Hash the fully sanitized text, so two truncated management values
				// cannot silently replay as the same fact. Never hash credentials.
				full, _ := SafeText(x, utf8.RuneCountInString(x))
				out["sha256"], out["count"] = Hash(full), utf8.RuneCountInString(full)
			}
			return out
		}
		return s
	case json.Number:
		return x.String()
	case []any:
		out := make([]any, 0, min(len(x), 20))
		for _, item := range x[:min(len(x), 20)] {
			out = append(out, safeValue(item, fields, depth+1))
		}
		if len(x) > 20 {
			all := make([]any, len(x))
			for i, item := range x {
				all[i] = safeValue(item, fields, depth+1)
			}
			return map[string]any{"count": len(x), "sample": out, "sha256": Hash(all), "value_state": Truncated}
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, value := range x {
			if slices.Contains(fields, k) {
				out[k] = safeValue(value, fields, depth+1)
			}
		}
		return out
	case bool, nil:
		return x
	default:
		return nil
	}
}
func stateOf(v any, present bool) ValueState {
	if !present {
		return NotRecorded
	}
	if m, ok := v.(map[string]any); ok {
		switch m["value_state"] {
		case Redacted, string(Redacted):
			return Redacted
		case Truncated, string(Truncated):
			return Truncated
		}
	}
	return Recorded
}

var fieldLabels = map[string]string{"name": "名称", "display_name": "显示名称", "status": "状态", "state": "状态", "enabled": "是否启用", "revision": "修订", "current_revision": "正文修订", "permissions": "权限", "roles": "角色", "reason": "原因", "code": "编号", "description": "说明", "account_id": "账号", "version": "版本", "file_id": "文件", "row_count": "记录数", "size_bytes": "字节数", "generation": "执行代次", "filename": "文件名称", "count": "数量", "value": "值"}

func Label(key string) string {
	if v := fieldLabels[key]; v != "" {
		return v
	}
	return key
}
func Diff(before, after any, beforeRecorded, afterRecorded bool) []Change {
	b, bok := before.(map[string]any)
	a, aok := after.(map[string]any)
	if !bok && !aok {
		return []Change{change("value", before, after, beforeRecorded, afterRecorded)}
	}
	keys := []string{}
	for k := range b {
		keys = append(keys, k)
	}
	for k := range a {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	out := []Change{}
	for _, k := range keys {
		bv, bp := b[k]
		av, ap := a[k]
		if bp && ap && reflect.DeepEqual(bv, av) {
			continue
		}
		out = append(out, change(k, bv, av, bp && beforeRecorded, ap && afterRecorded))
	}
	return out
}
func change(k string, b, a any, bp, ap bool) Change {
	typ := "text"
	switch a.(type) {
	case bool:
		typ = "boolean"
	case []any:
		typ = "set"
	case map[string]any:
		typ = "summary"
	}
	return Change{k, Label(k), typ, b, a, stateOf(b, bp), stateOf(a, ap)}
}
