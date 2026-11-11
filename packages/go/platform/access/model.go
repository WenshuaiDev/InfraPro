package access

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type Error struct {
	Code, Message string
	Status        int
	Fields        map[string]string
}

func (e *Error) Error() string { return e.Message }

var (
	ErrDenied      = &Error{Code: "permission_denied", Message: "没有执行此操作的权限。", Status: 403}
	ErrScope       = &Error{Code: "grant_scope_exceeded", Message: "不能处置超出本人权限范围的授权。", Status: 403}
	ErrLastManager = &Error{Code: "last_platform_manager", Message: "请先为另一个正常账号配置完整平台管理权限。", Status: 409}
	ErrConflict    = &Error{Code: "version_conflict", Message: "记录已被修改，请重新加载并比较差异后提交。", Status: 409}
	ErrNotFound    = &Error{Code: "not_found", Message: "未找到可操作的记录。", Status: 404}
	ErrCatalog     = &Error{Code: "unavailable", Message: "权限目录不一致，请由维护人员运行 access sync/check。", Status: 503}
)

func Validation(field, message string) error {
	return &Error{Code: "validation_failed", Message: message, Status: 400, Fields: map[string]string{field: message}}
}
func ValidReason(s string) error {
	if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || utf8.RuneCountInString(s) > 500 {
		return Validation("reason", "请填写 1–500 字符的变更原因。")
	}
	return nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ValidID(id string) bool { return uuidPattern.MatchString(id) }

type Role struct {
	OwnerProjectID    string    `json:"owner_project_id"`
	OwnerTenantID     string    `json:"owner_tenant_id"`
	SourceRoleID      string    `json:"source_role_id"`
	SourceRoleVersion string    `json:"source_role_version"`
	AssignmentPolicy  string    `json:"assignment_policy"`
	ID                string    `json:"id"`
	BindingScope      string    `json:"binding_scope"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	BuiltinKey        string    `json:"builtin_key"`
	Status            string    `json:"status"`
	Version           string    `json:"version"`
	Permissions       []Code    `json:"permission_codes"`
	AccountCount      int       `json:"account_count"`
	CreatedAt         time.Time `json:"created_at"`
}
type Snapshot struct {
	Revision    string            `json:"authorization_revision"`
	Permissions []Code            `json:"permissions"`
	Roles       []Role            `json:"roles"`
	Sources     map[Code][]string `json:"sources"`
}
type Account struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	Version     string    `json:"management_version"`
	CreatedAt   time.Time `json:"created_at"`
	Roles       []Role    `json:"roles"`
}
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}
type Meta struct{ ActorID, ActorType, IP, UserAgent, RequestID, Reason, TenantID, ProjectID, ActorScope string }
type AuditEvent struct {
	ID         string         `json:"id"`
	ProjectID  string         `json:"project_id"`
	TenantID   string         `json:"tenant_id"`
	ActorScope string         `json:"actor_scope"`
	EventType  string         `json:"event_type"`
	ActorType  string         `json:"actor_type"`
	ActorID    string         `json:"actor_id"`
	AccountID  string         `json:"account_id"`
	ObjectType string         `json:"object_type"`
	ObjectID   string         `json:"object_id"`
	Reason     string         `json:"reason"`
	RequestID  string         `json:"request_id"`
	CreatedAt  time.Time      `json:"created_at"`
	Details    map[string]any `json:"details"`
	Display    *AuditDisplay  `json:"display,omitempty"`
}

// AuditDisplay is a read projection, never a replacement for the persisted event.
// Historical names take precedence; current-name fallbacks are explicitly marked.
type AuditDisplay struct {
	Category         string            `json:"category"`
	Outcome          string            `json:"outcome"`
	ActorName        string            `json:"actor_name"`
	ActorUsername    string            `json:"actor_username"`
	ActorNameSource  string            `json:"actor_name_source"`
	TargetName       string            `json:"target_name"`
	TargetUsername   string            `json:"target_username"`
	TargetNameSource string            `json:"target_name_source"`
	TenantName       string            `json:"tenant_name"`
	SourceIP         string            `json:"source_ip"`
	UserAgent        string            `json:"user_agent"`
	PermissionNames  map[string]string `json:"permission_names"`
	RoleNames        map[string]string `json:"role_names"`
}

func denied(err error) bool { return errors.Is(err, ErrDenied) || errors.Is(err, ErrScope) }
