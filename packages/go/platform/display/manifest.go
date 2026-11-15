package display

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

type Manifest struct {
	Version    int    `json:"manifest_version"`
	AppKey     string `json:"app_key"`
	Name       string `json:"name,omitempty"`
	AppVersion string `json:"version"`
	Entry      string `json:"entry"`
	Routing    struct {
		Mode     string   `json:"mode"`
		Fallback []string `json:"fallback,omitempty"`
	} `json:"routing"`
	Protocol struct {
		Major int `json:"major"`
	} `json:"sdk_protocol"`
	Required     map[string]int           `json:"required_capabilities"`
	Optional     map[string]int           `json:"optional_capabilities,omitempty"`
	ConfigSchema Schema                   `json:"config_schema"`
	Bindings     map[string]BindingSchema `json:"binding_schema"`
	Files        []File                   `json:"files"`
}
type File struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
}
type BindingSchema struct {
	Kind          string `json:"kind"`
	Required      bool   `json:"required"`
	Min           int    `json:"min_items,omitempty"`
	Max           int    `json:"max_items,omitempty"`
	ValueKind     string `json:"value_kind,omitempty"`
	SourceKind    string `json:"source_kind,omitempty"`
	MetricID      string `json:"metric_version_id,omitempty"`
	UnitDimension string `json:"unit_dimension,omitempty"`
	Direction     string `json:"observation_direction,omitempty"`
	Basis         string `json:"reference_basis,omitempty"`
}
type Schema struct {
	Type        string            `json:"type"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Default     any               `json:"default,omitempty"`
	Properties  map[string]Schema `json:"properties,omitempty"`
	Required    []string          `json:"required,omitempty"`
	Additional  bool              `json:"additionalProperties,omitempty"`
	Enum        []any             `json:"enum,omitempty"`
	MinLength   int               `json:"minLength,omitempty"`
	MaxLength   int               `json:"maxLength,omitempty"`
	Minimum     *json.Number      `json:"minimum,omitempty"`
	Maximum     *json.Number      `json:"maximum,omitempty"`
	Items       *Schema           `json:"items,omitempty"`
	MaxItems    int               `json:"maxItems,omitempty"`
}

var keyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,79}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var mediaTypes = map[string]string{".html": "text/html", ".js": "text/javascript", ".mjs": "text/javascript", ".css": "text/css", ".json": "application/json", ".wasm": "application/wasm", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp", ".gif": "image/gif", ".svg": "image/svg+xml", ".woff": "font/woff", ".woff2": "font/woff2", ".ttf": "font/ttf", ".glb": "model/gltf-binary", ".gltf": "model/gltf+json", ".bin": "application/octet-stream", ".ico": "image/x-icon", ".txt": "text/plain", ".map": "application/json"}

func SafePath(p string) bool {
	return p != "" && len(p) <= 240 && utf8.ValidString(p) && path.Clean(p) == p && !strings.HasPrefix(p, "/") && !strings.ContainsAny(p, "\\:%?#\x00") && !strings.HasPrefix(p, ".") && !strings.Contains(p, "/.") && len(strings.Split(p, "/")) <= 12
}

// Decode rejects duplicate properties as well as unknown fields. JSON with
// duplicate keys must never have different meanings in Go and JavaScript.
func Decode(b []byte, out any) error {
	if len(b) > 1<<20 {
		return invalid("/", "JSON 超过限制。")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var check func() error
	check = func() error {
		t, e := d.Token()
		if e != nil {
			return e
		}
		v, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch v {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return fmt.Errorf("duplicate key")
				}
				seen[key] = true
				if e = check(); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = check(); e != nil {
					return e
				}
			}
		default:
			return fmt.Errorf("unexpected delimiter")
		}
		_, e = d.Token()
		return e
	}
	if e := check(); e != nil {
		return invalid("/", "JSON 无效或含重复字段。")
	}
	if _, e := d.Token(); e != io.EOF {
		return invalid("/", "JSON 含多余内容。")
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	d.UseNumber()
	if e := d.Decode(out); e != nil {
		return invalid("/", "JSON 字段或类型无效。")
	}
	return nil
}
func ValidateManifest(m Manifest) error {
	if m.Version != 1 || m.Protocol.Major != 1 {
		return problem("version_incompatible", "不支持此包或运行协议主版本。", 422)
	}
	if !keyPattern.MatchString(m.AppKey) || len(m.AppVersion) == 0 || len(m.AppVersion) > 64 || !SafePath(m.Entry) || path.Ext(m.Entry) != ".html" {
		return invalid("/manifest", "应用标识、版本或入口无效。")
	}
	if m.Routing.Mode != "hash" && m.Routing.Mode != "history" {
		return invalid("/routing", "路由必须为 hash 或 history。")
	}
	for _, p := range m.Routing.Fallback {
		if !SafePath(p) {
			return invalid("/routing/fallback", "仅支持明确的包内相对路径。")
		}
	}
	for _, set := range []map[string]int{m.Required, m.Optional} {
		for name, v := range set {
			if _, ok := capabilities[name]; !ok || v != 1 {
				return invalid("/capabilities", "能力名称或主版本不支持："+name)
			}
		}
	}
	if len(m.Bindings) > 64 || len(m.Files) == 0 || len(m.Files) > 2048 {
		return invalid("/manifest", "绑定或文件数超出限制。")
	}
	if m.ConfigSchema.Type != "object" {
		return invalid("/config_schema", "配置根节点必须为对象。")
	}
	if e := validateSchema(m.ConfigSchema, 0); e != nil {
		return e
	}
	for name, b := range m.Bindings {
		if !keyPattern.MatchString(name) || !slices.Contains([]string{"channels", "resources", "spatial", "artifacts"}, b.Kind) || b.Min < 0 || b.Max < 0 || b.Max > 500 || b.Max > 0 && b.Min > b.Max {
			return invalid("/binding_schema/"+name, "绑定声明无效。")
		}
	}
	found := false
	seen := map[string]bool{}
	var total int64
	for _, f := range m.Files {
		lower := strings.ToLower(f.Path)
		if !SafePath(f.Path) || f.Path == "manifest.json" || seen[lower] || !digestPattern.MatchString(f.SHA256) || f.Size < 0 || f.Size > 32<<20 || mediaTypes[strings.ToLower(path.Ext(f.Path))] != f.MediaType || f.MediaType == "" {
			return invalid("/files", "文件路径、类型、长度或摘要无效。")
		}
		seen[lower] = true
		total += f.Size
		found = found || f.Path == m.Entry
	}
	if !found || total > 256<<20 {
		return invalid("/files", "入口不在清单中或解压内容超出限制。")
	}
	return nil
}
func validateSchema(s Schema, depth int) error {
	if depth > 6 || !slices.Contains([]string{"object", "array", "string", "number", "integer", "boolean"}, s.Type) || s.Additional || len(s.Properties) > 64 || s.MaxItems < 0 || s.MaxItems > 1000 || s.MaxLength < 0 || s.MaxLength > 10000 {
		return invalid("/config_schema", "超出受限 JSON Schema 子集。")
	}
	if depth == 0 && s.Type != "object" {
		return invalid("/config_schema/type", "根节点必须是 object。")
	}
	if s.Type == "array" && (s.Items == nil || s.MaxItems == 0) {
		return invalid("/config_schema/items", "数组必须有 items 与 maxItems。")
	}
	for k, v := range s.Properties {
		if !keyPattern.MatchString(k) {
			return invalid("/config_schema/properties", "参数名称无效。")
		}
		if e := validateSchema(v, depth+1); e != nil {
			return e
		}
	}
	for _, k := range s.Required {
		if _, ok := s.Properties[k]; !ok {
			return invalid("/config_schema/required", "必需字段未声明。")
		}
	}
	if s.Items != nil {
		if e := validateSchema(*s.Items, depth+1); e != nil {
			return e
		}
	}
	return nil
}
func validateValue(s Schema, v any, p string) error {
	bad := func() error { return invalid(p, "值不符合包声明的类型或范围。") }
	if len(s.Enum) > 0 {
		ok := false
		for _, item := range s.Enum {
			ok = ok || hash(item) == hash(v)
		}
		if !ok {
			return bad()
		}
	}
	switch s.Type {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			return bad()
		}
		for _, k := range s.Required {
			if _, ok := m[k]; !ok {
				return invalid(p+"/"+k, "需要填写此参数。")
			}
		}
		for k, v := range m {
			schema, ok := s.Properties[k]
			if !ok {
				return invalid(p+"/"+k, "未声明的参数。")
			}
			if e := validateValue(schema, v, p+"/"+k); e != nil {
				return e
			}
		}
	case "string":
		n, ok := v.(string)
		if !ok || utf8.RuneCountInString(n) < s.MinLength || s.MaxLength > 0 && utf8.RuneCountInString(n) > s.MaxLength {
			return bad()
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return bad()
		}
	case "number", "integer":
		if !validNumber(s, v) {
			return bad()
		}
	case "array":
		a, ok := v.([]any)
		if !ok || len(a) > s.MaxItems {
			return bad()
		}
		for i, v := range a {
			if e := validateValue(*s.Items, v, fmt.Sprintf("%s/%d", p, i)); e != nil {
				return e
			}
		}
	}
	return nil
}
func ValidateConfig(m Manifest, parameters map[string]any) error {
	for k, v := range m.ConfigSchema.Properties {
		if _, ok := parameters[k]; !ok && v.Default != nil {
			parameters[k] = v.Default
		}
	}
	return validateValue(m.ConfigSchema, parameters, "/parameters")
}

type ValidatedPackage struct {
	Manifest Manifest
	Files    map[string][]byte
}

func ValidateZIP(ctx context.Context, r io.ReaderAt, size int64) (ValidatedPackage, error) {
	out := ValidatedPackage{Files: map[string][]byte{}}
	if size < 1 || size > 128<<20 {
		return out, invalid("/zip", "压缩包大小超出限制。")
	}
	z, e := zip.NewReader(r, size)
	if e != nil {
		return out, invalid("/zip", "无法读取 ZIP。")
	}
	if len(z.File) < 2 || len(z.File) > 2049 {
		return out, invalid("/zip", "文件数量超出限制。")
	}
	seen := map[string]bool{}
	var total uint64
	for _, f := range z.File {
		if e := ctx.Err(); e != nil {
			return out, e
		}
		if !SafePath(f.Name) || seen[strings.ToLower(f.Name)] || f.Mode()&fs.ModeType != 0 || f.Flags&1 != 0 || f.UncompressedSize64 > 32<<20 || f.CompressedSize64 == 0 && f.UncompressedSize64 > 0 || f.CompressedSize64 > 0 && f.UncompressedSize64/f.CompressedSize64 > 200 {
			return out, invalid("/zip", "ZIP 包含不安全路径、重复文件、特殊文件或异常压缩比。")
		}
		seen[strings.ToLower(f.Name)] = true
		total += f.UncompressedSize64
		if total > 256<<20 {
			return out, invalid("/zip", "解压大小超出限制。")
		}
		reader, e := f.Open()
		if e != nil {
			return out, invalid("/zip", "无法读取文件。")
		}
		b, e := io.ReadAll(io.LimitReader(reader, int64(f.UncompressedSize64)+1))
		reader.Close()
		if e != nil || uint64(len(b)) != f.UncompressedSize64 {
			return out, invalid("/zip", "文件长度或 CRC 校验失败。")
		}
		out.Files[f.Name] = b
	}
	if e = Decode(out.Files["manifest.json"], &out.Manifest); e != nil {
		return out, e
	}
	if e = ValidateManifest(out.Manifest); e != nil {
		return out, e
	}
	if len(out.Files) != len(out.Manifest.Files)+1 {
		return out, invalid("/files", "ZIP 与文件清单不一致。")
	}
	for _, f := range out.Manifest.Files {
		b, ok := out.Files[f.Path]
		if !ok || int64(len(b)) != f.Size || hashBytes(b) != f.SHA256 {
			return out, invalid("/files/"+f.Path, "文件长度或 SHA-256 与清单不一致。")
		}
		if path.Ext(f.Path) == ".html" || path.Ext(f.Path) == ".css" {
			if externalReference.Match(b) {
				return out, invalid("/files/"+f.Path, "静态依赖必须使用包内相对路径。")
			}
		}
	}
	return out, nil
}

var externalReference = regexp.MustCompile(`(?i)(?:src|href)\s*=\s*["']\s*(?:https?:|//|/)|url\(\s*["']?\s*(?:https?:|//|/)`)
