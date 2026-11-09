// Package identity owns account credentials, invitations and revocable sessions.
package identity

import (
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type Error struct {
	Code    string
	Message string
	Status  int
	Fields  map[string]string
}

func (e *Error) Error() string { return e.Message }
func problem(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status}
}

var (
	ErrUnauthorized = problem("unauthenticated", "登录已失效，请重新登录。", http.StatusUnauthorized)
	ErrCredentials  = problem("invalid_credentials", "用户名或密码不正确，或账号暂不可用。", http.StatusUnauthorized)
	ErrUnavailable  = problem("unavailable", "服务暂时不可用，请稍后重试。", http.StatusServiceUnavailable)
	ErrLink         = problem("invalid_link", "链接已失效，请联系邀请人或维护人员获取新链接。", http.StatusNotFound)
	ErrNotFound     = problem("not_found", "未找到可操作的记录。", http.StatusNotFound)
	ErrRate         = problem("rate_limited", "操作过于频繁，请稍后重试。", http.StatusTooManyRequests)
	ErrReauth       = problem("reauthentication_required", "请先确认当前密码。", http.StatusForbidden)
	ErrConflict     = problem("username_taken", "用户名已被占用，请换一个用户名。", http.StatusConflict)
)

func FieldError(field, message string) error {
	return &Error{Code: "validation_failed", Message: message, Status: 400, Fields: map[string]string{field: message}}
}
func PublicError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	var ae *access.Error
	if errors.As(err, &ae) {
		return &Error{Code: ae.Code, Message: ae.Message, Status: ae.Status, Fields: ae.Fields}
	}
	return ErrUnavailable
}

type Account struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}
type Session struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	UserAgent      string    `json:"user_agent"`
	IP             string    `json:"ip"`
	Current        bool      `json:"current"`
}
type Principal struct {
	Account           Account   `json:"account"`
	Session           Session   `json:"session"`
	Version           int64     `json:"version"`
	ReauthenticatedAt time.Time `json:"reauthenticated_at"`
	TokenHash         string    `json:"-"`
}
type Invitation struct {
	ID        string    `json:"id"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Status    string    `json:"status"`
}
type SecurityEvent struct {
	ID        int64     `json:"id"`
	EventType string    `json:"event_type"`
	Outcome   string    `json:"outcome"`
	SourceIP  string    `json:"source_ip"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
}
type AuditEvent struct {
	SecurityEvent
	ActorType  string          `json:"actor_type"`
	ObjectType string          `json:"object_type"`
	Details    json.RawMessage `json:"details"`
	ActorID    *string         `json:"actor_id"`
	AccountID  *string         `json:"account_id"`
	ObjectID   string          `json:"object_id"`
	Reason     string          `json:"reason"`
	RequestID  string          `json:"request_id"`
}
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}
type Link struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Inspection struct {
	ExpiresAt time.Time `json:"expires_at"`
}
type Meta struct{ IP, UserAgent, RequestID, Reason string }

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,32}$`)
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validName(username, display string) (string, error) {
	if !usernamePattern.MatchString(username) {
		return "", FieldError("username", "用户名需为 3–32 位英文字母、数字、下划线或短横线。")
	}
	if display == "" {
		display = username
	}
	return validateDisplay(display)
}
func validateDisplay(display string) (string, error) {
	if !utf8.ValidString(display) || utf8.RuneCountInString(display) < 1 || utf8.RuneCountInString(display) > 50 || strings.TrimSpace(display) == "" {
		return "", FieldError("display_name", "显示名称需为 1–50 个字符。")
	}
	return display, nil
}
func trimMeta(m Meta) Meta {
	m.IP = limit(m.IP, 100)
	m.UserAgent = limit(m.UserAgent, 300)
	m.RequestID = limit(m.RequestID, 100)
	m.Reason = limit(m.Reason, 500)
	return m
}
func limit(value string, n int) string {
	r := []rune(value)
	if len(r) > n {
		return string(r[:n])
	}
	return value
}
