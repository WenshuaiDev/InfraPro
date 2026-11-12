// Package devicekit shares the device HTTP value contract and transaction metadata.
// Business decisions remain in devicecatalog, devices and sources.
package devicekit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Error struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Status  int     `json:"-"`
	Issues  []Issue `json:"issues"`
	Cause   error   `json:"-"`
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }
func Problem(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status, Issues: []Issue{}}
}
func Invalid(path, code, message string) *Error {
	return &Error{Code: "validation_failed", Message: "请修正标记的问题后重试。", Status: 422, Issues: []Issue{{path, code, message}}}
}
func NotFound(kind string) *Error {
	return Problem(kind+"_not_found", "对象不存在或不可访问。", 404)
}
func JSON(v any) []byte    { b, _ := json.Marshal(v); return b }
func Hash(v any) string    { h := sha256.Sum256(JSON(v)); return hex.EncodeToString(h[:]) }
func Text(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")) }
func String(s, path string, max int, required bool) error {
	if (required && Text(s) == "") || utf8.RuneCountInString(s) > max {
		return Invalid(path, "length", "内容为空或超过允许长度。")
	}
	return nil
}

var codePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func Code(s string) bool { return codePattern.MatchString(s) }
func Key(s string) bool  { return keyPattern.MatchString(s) }
func Revision(s string, zero bool) bool {
	n, e := strconv.ParseInt(s, 10, 64)
	return e == nil && n >= 0 && (zero || n > 0) && strconv.FormatInt(n, 10) == s
}
func Expect(want, have string) error {
	if !Revision(want, true) {
		return Invalid("/expected_revision", "revision", "请提供十进制字符串修订号。")
	}
	if want != have {
		return Problem("revision_conflict", "对象已变化，请重新读取后核对；当前输入已保留。", 409)
	}
	return nil
}
func Nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func Strict(raw []byte, v any) error {
	if len(raw) > 256*1024 {
		return Problem("input_too_large", "单次请求最多 256 KiB。", 413)
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return Problem("invalid_request", "请提交单个 JSON 对象。", 400)
	}
	if e := requestJSON(raw); e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return Problem("invalid_request", "请求字段或类型无效。", 400)
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return Problem("invalid_request", "请提交单个 JSON 对象。", 400)
	}
	return nil
}

// Reject ambiguous objects before decoding: duplicate keys and null scalars are
// never interpreted as omitted fields. Nullable position sections keep the
// established coordinate contract, including explicit zero versus absence.
func requestJSON(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var walk func(string) error
	walk = func(path string) error {
		token, e := dec.Token()
		if e != nil {
			return Problem("invalid_request", "请求 JSON 无效。", 400)
		}
		if token == nil && !strings.HasSuffix(path, "/unit_version_id") && !strings.HasSuffix(path, "/uninstalled_at") && !strings.HasSuffix(path, "/position") && !strings.Contains(path, "/position/") {
			return Problem("invalid_request", "此字段不能为 null。", 400)
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				v, e := dec.Token()
				if e != nil {
					return Problem("invalid_request", "请求 JSON 无效。", 400)
				}
				key, ok := v.(string)
				if !ok || seen[key] {
					return Problem("invalid_request", "请求包含重复字段。", 400)
				}
				seen[key] = true
				if e = walk(path + "/" + key); e != nil {
					return e
				}
			}
		case '[':
			for dec.More() {
				if e = walk(path + "/item"); e != nil {
					return e
				}
			}
		default:
			return Problem("invalid_request", "请求 JSON 无效。", 400)
		}
		_, e = dec.Token()
		if e != nil {
			return Problem("invalid_request", "请求 JSON 无效。", 400)
		}
		return nil
	}
	return walk("")
}

type Scope struct {
	Entry   string `json:"entry_scope"`
	Tenant  string `json:"tenant_id,omitempty"`
	Project string `json:"project_id,omitempty"`
}

func Platform() Scope        { return Scope{Entry: "platform"} }
func Tenant(id string) Scope { return Scope{Entry: "tenant", Tenant: id} }
func Project(tenant, project string) Scope {
	return Scope{Entry: "project", Tenant: tenant, Project: project}
}
func (s Scope) Valid() bool {
	return s.Entry == "platform" && s.Tenant == "" && s.Project == "" || access.ValidID(s.Tenant) && (s.Entry == "tenant" && s.Project == "" || s.Entry == "project" && access.ValidID(s.Project))
}
func (s Scope) Operation(kind, action string) access.Operation {
	return access.DeviceOperation(s.Entry, kind, action)
}

type Meta struct {
	RequestID   string `json:"request_id"`
	Reason      string `json:"reason,omitempty"`
	PreviewHash string `json:"preview_hash,omitempty"`
}

func (m *Meta) Normalize() error {
	m.Reason = Text(m.Reason)
	if !access.ValidID(m.RequestID) {
		return Invalid("/request_id", "uuid", "请提供有效请求 ID。")
	}
	return String(m.Reason, "/reason", 2000, false)
}
func (m Meta) RequireReason() error {
	if m.Reason == "" {
		return Invalid("/reason", "required", "请填写变更原因。")
	}
	return nil
}

type RevisionInput struct {
	Meta
	Revision string `json:"expected_revision"`
}
type Result struct {
	Issues               []Issue `json:"issues,omitempty"`
	ID                   string  `json:"id"`
	Revision             string  `json:"revision"`
	VersionID            string  `json:"version_id,omitempty"`
	Number               int     `json:"number,omitempty"`
	OutputID             string  `json:"output_id,omitempty"`
	BindingID            string  `json:"binding_id,omitempty"`
	InstallationID       string  `json:"installation_id,omitempty"`
	OutputRevision       string  `json:"output_revision,omitempty"`
	InstallationRevision string  `json:"installation_revision,omitempty"`
	SourceRevision       string  `json:"source_revision,omitempty"`
	AccessRevision       string  `json:"device_access_revision,omitempty"`
	Changed              bool    `json:"changed"`
	Replayed             bool    `json:"replayed"`
}
type Preview struct {
	CanSubmit  bool    `json:"can_submit"`
	Required   bool    `json:"required"`
	Hash       string  `json:"preview_hash"`
	Changes    any     `json:"changes"`
	Blockers   []Issue `json:"blockers"`
	Shared     bool    `json:"shared"`
	Impacts    []any   `json:"visible_impacts"`
	NextCursor string  `json:"next_cursor"`
}

func NewPreview(input, state, changes any, required, shared bool) Preview {
	return Preview{CanSubmit: true, Required: required, Hash: Hash([]any{input, state}), Changes: changes, Blockers: []Issue{}, Shared: shared, Impacts: []any{}}
}
func (p Preview) Check(m Meta) error {
	if !p.CanSubmit {
		return &Error{Code: "validation_failed", Message: "当前操作存在阻断。", Status: 422, Issues: p.Blockers}
	}
	if p.Required && m.PreviewHash == "" {
		return Problem("preview_required", "请先预览本次变更。", 409)
	}
	if m.PreviewHash != "" && m.PreviewHash != p.Hash {
		return Problem("preview_changed", "关联状态已变化，请重新预览。", 409)
	}
	return nil
}

// Time accepts only zoned RFC3339 timestamps with PostgreSQL microsecond precision.
var timestampPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,6})?(Z|[+-][0-9]{2}:[0-9]{2})$`)

func Time(raw, path string) (time.Time, error) {
	if !timestampPattern.MatchString(raw) {
		return time.Time{}, Invalid(path, "timestamp", "请填写带时区且小数秒不超过 6 位的 RFC3339 时间。")
	}
	t, e := time.Parse(time.RFC3339Nano, raw)
	_, offset := t.Zone()
	if e != nil || t.Year() < 1 || t.Nanosecond()%1000 != 0 || offset <= -24*3600 || offset >= 24*3600 || (len(raw) > 6 && raw[len(raw)-1] != 'Z' && raw[len(raw)-2:] >= "60") {
		return t, Invalid(path, "timestamp", "请填写带时区且小数秒不超过 6 位的 RFC3339 时间。")
	}
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		n := 0
		for _, c := range raw[i+1:] {
			if c < '0' || c > '9' {
				break
			}
			n++
		}
		if n > 6 {
			return t, Invalid(path, "precision", "小数秒最多 6 位。")
		}
	}
	return t.UTC(), nil
}
