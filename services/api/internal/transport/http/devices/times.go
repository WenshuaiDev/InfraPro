package deviceshttp

import (
	"encoding/json"
	h "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/devicecommon"
)

// Omission requests server time. Explicit empty/null values must not silently
// request a different time; only correction may explicitly clear the end.
func installationTimes(raw []byte, action string) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return h.Invalid("请求格式无效。")
	}
	if action == "preview" {
		var value string
		_ = json.Unmarshal(fields["action"], &value)
		action = value
	}
	for _, key := range []string{"installed_at", "uninstalled_at", "moved_at"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		if string(value) == "null" && key == "uninstalled_at" && (action == "correct" || action == "installation_correct") {
			continue
		}
		var timestamp string
		if json.Unmarshal(value, &timestamp) != nil || timestamp == "" {
			return h.Invalid("显式提供的时间必须是带时区的有效时间；使用服务器当前时间时请省略该字段。")
		}
	}
	return nil
}
