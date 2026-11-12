package devicekit

import (
	"encoding/base64"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"time"
	"unicode/utf8"
)

type Filter struct {
	Level            string `json:"level,omitempty"`
	ParentID         string `json:"parent_id,omitempty"`
	Query            string `json:"q"`
	Cursor           string `json:"-"`
	Limit            int    `json:"limit"`
	ManufacturerID   string `json:"manufacturer_id,omitempty"`
	ProductID        string `json:"product_id,omitempty"`
	ProductVersionID string `json:"product_version_id,omitempty"`
	DeviceID         string `json:"device_id,omitempty"`
	Status           string `json:"status,omitempty"`
	Source           string `json:"source,omitempty"`
	State            string `json:"state,omitempty"`
	SlotKey          string `json:"slot_key,omitempty"`
	Kind             string `json:"kind,omitempty"`
	Installation     string `json:"installation,omitempty"`
	MetricID         string `json:"metric_version_id,omitempty"`
	UnitID           string `json:"unit_version_id,omitempty"`
	CompatibleOnly   *bool  `json:"compatible_only,omitempty"`
}

func (f *Filter) Validate() error {
	f.Query = Text(f.Query)
	if f.Limit == 0 {
		f.Limit = 25
	}
	if f.Limit < 1 || f.Limit > 100 || utf8.RuneCountInString(f.Query) > 200 {
		return Problem("invalid_request", "分页大小为 1–100，搜索最多 200 字符。", 400)
	}
	for _, id := range []string{f.ParentID, f.ManufacturerID, f.ProductID, f.ProductVersionID, f.DeviceID, f.MetricID, f.UnitID} {
		if id != "" && !access.ValidID(id) {
			return Problem("invalid_request", "筛选身份无效。", 400)
		}
	}
	return nil
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}
type Cursor struct {
	Scope  string    `json:"scope"`
	At     time.Time `json:"at"`
	ID     string    `json:"id"`
	Number int       `json:"number,omitempty"`
	Key    string    `json:"key,omitempty"`
}

func CursorContext(a Authorization, f Filter, parent any) string {
	return Hash([]any{a.Scope, a.Account, a.TenantMembership, a.ProjectMembership, a.Revision, a.ResourceAccessRevision, a.DeviceAccessRevision, f, parent})
}
func DecodeCursor(raw, scope string) (Cursor, error) {
	c := Cursor{Scope: scope}
	if raw == "" {
		return c, nil
	}
	b, e := base64.RawURLEncoding.DecodeString(raw)
	if e == nil {
		e = json.Unmarshal(b, &c)
	}
	if e != nil || c.Scope != scope || !access.ValidID(c.ID) {
		return Cursor{}, Problem("invalid_cursor", "分页上下文已变化，请回到第一页。", 400)
	}
	return c, nil
}
func (c Cursor) Encode() string { return base64.RawURLEncoding.EncodeToString(JSON(c)) }
func (c Cursor) TimeValue() any {
	if c.ID == "" {
		return nil
	}
	return c.At
}
