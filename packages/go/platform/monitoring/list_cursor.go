package monitoring

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

type listBoundary struct {
	At *time.Time `json:"at"`
	ID string     `json:"id"`
}

func listCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString(encodeJSON(listBoundary{&at, id}))
}
func parseListCursor(value string) (listBoundary, error) {
	var out listBoundary
	if value == "" {
		return out, nil
	}
	if len(value) > 256 {
		return out, invalid("cursor", "无效分页位置。")
	}
	data, e := base64.RawURLEncoding.DecodeString(value)
	if e != nil || json.Unmarshal(data, &out) != nil || out.At == nil || out.At.IsZero() || !access.ValidID(out.ID) {
		return listBoundary{}, invalid("cursor", "无效分页位置。")
	}
	return out, nil
}
