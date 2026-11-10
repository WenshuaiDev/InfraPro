// Package tenancy owns tenant profiles, permanent code ownership and administrator handover.
package tenancy

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"golang.org/x/text/unicode/norm"
)

type Account struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}
type Summary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}
type Tenant struct {
	Summary
	Description            string    `json:"description"`
	Administrator          Account   `json:"administrator"`
	AdministratorAvailable bool      `json:"administrator_available"`
	MembershipID           string    `json:"administrator_membership_id"`
	Version                string    `json:"version"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	CreatedBy              string    `json:"created_by"`
	UpdatedBy              string    `json:"updated_by"`
}
type TenantRole struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BindingScope string `json:"binding_scope"`
}
type Context struct {
	Tenant       Summary                  `json:"tenant"`
	MembershipID string                   `json:"membership_id"`
	Roles        []TenantRole             `json:"roles"`
	Permissions  []access.Code            `json:"permissions"`
	Sources      map[access.Code][]string `json:"permission_sources"`
	Revision     string                   `json:"authorization_revision"`
}
type MyTenant struct {
	Summary
	MembershipID string       `json:"membership_id"`
	Roles        []TenantRole `json:"roles"`
}
type Fields struct {
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
}
type CreateInput struct {
	Fields
	AdministratorID string `json:"administrator_account_id"`
	RequestID       string `json:"client_request_id"`
	Reason          string `json:"reason"`
}
type UpdateInput struct {
	Name        *string `json:"name,omitempty"`
	Code        *string `json:"code,omitempty"`
	Description *string `json:"description,omitempty"`
	Version     string  `json:"expected_version"`
	Reason      string  `json:"reason"`
}
type TransferInput struct {
	AdministratorID string `json:"new_administrator_account_id"`
	Version         string `json:"expected_version"`
	Reason          string `json:"reason"`
}
type Result struct {
	TenantID string `json:"tenant_id"`
	Version  string `json:"version"`
	Revision string `json:"authorization_revision"`
	Replayed bool   `json:"replayed"`
}
type Filter struct {
	Query, Cursor, Event, From, To string
	Limit                          int
	Unavailable                    bool
}
type AuditEvent struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenant_id"`
	Event      string         `json:"event_type"`
	ActorID    string         `json:"actor_id"`
	ActorType  string         `json:"actor_type"`
	ActorScope string         `json:"actor_scope"`
	Reason     string         `json:"reason"`
	RequestID  string         `json:"request_id"`
	CreatedAt  time.Time      `json:"created_at"`
	Details    map[string]any `json:"details"`
}

var codePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,31}$`)

func (f *Fields) normalize() error {
	f.Name = strings.TrimSpace(norm.NFKC.String(f.Name))
	f.Code = strings.ToLower(strings.TrimSpace(f.Code))
	if !utf8.ValidString(f.Name) || utf8.RuneCountInString(f.Name) < 1 || utf8.RuneCountInString(f.Name) > 100 {
		return access.Validation("name", "名称需为 1–100 个字符。")
	}
	if !codePattern.MatchString(f.Code) {
		return access.Validation("code", "编号需为 3–32 位字母、数字、短横线或下划线，首位为字母或数字。")
	}
	if !utf8.ValidString(f.Description) || utf8.RuneCountInString(f.Description) > 1000 {
		return access.Validation("description", "说明最多 1000 个字符。")
	}
	return nil
}
func (f *Filter) validate(candidate bool) error {
	if f.Limit == 0 {
		f.Limit = 20
	}
	max := 100
	if candidate {
		max = 20
	}
	if f.Limit < 1 || f.Limit > max {
		return access.Validation("limit", "每页数量超出范围。")
	}
	f.Query = strings.TrimSpace(f.Query)
	if !utf8.ValidString(f.Query) || utf8.RuneCountInString(f.Query) > 200 || candidate && utf8.RuneCountInString(f.Query) < 2 {
		return access.Validation("q", "请输入 2–200 个字符查找账号。")
	}
	if f.Cursor != "" && !access.ValidID(f.Cursor) {
		return access.Validation("cursor", "分页位置无效。")
	}
	return nil
}
func version(have, want string) error {
	n, e := strconv.ParseInt(want, 10, 64)
	if e != nil || n < 1 || strconv.FormatInt(n, 10) != want {
		return access.Validation("expected_version", "版本号无效。")
	}
	if have != want {
		return access.ErrConflict
	}
	return nil
}
func problem(code, message string, status int) error {
	return &access.Error{Code: code, Message: message, Status: status}
}

var errAdmin = problem("administrator_unavailable", "接任账号不存在或已不可用，请重新选择。", 409)
var errCode = problem("tenant_code_taken", "编号已被使用或保留，请更换编号或查验已创建结果。", 409)
