// Package monitoring owns monitoring data authorization, immutable observations
// and publication visibility. Physical storage is not exposed to HTTP handlers.
package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

type Scope struct {
	Entry   string `json:"kind"`
	Tenant  string `json:"tenant_id"`
	Project string `json:"project_id,omitempty"`
}

func TenantScope(tenant string) Scope { return Scope{Entry: "tenant", Tenant: tenant} }
func ProjectScope(tenant, project string) Scope {
	return Scope{Entry: "project", Tenant: tenant, Project: project}
}
func (s Scope) valid() bool {
	return access.ValidID(s.Tenant) && ((s.Entry == "tenant" && s.Project == "") || (s.Entry == "project" && access.ValidID(s.Project)))
}
func (s Scope) scopeID() string {
	if s.Entry == "project" {
		return s.Project
	}
	return s.Tenant
}

type Error struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Status  int     `json:"-"`
	Issues  []Issue `json:"issues,omitempty"`
}
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }
func problem(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status}
}
func invalid(path, message string) *Error {
	return &Error{Code: "monitoring_invalid", Message: "请修正监测数据配置。", Status: 422, Issues: []Issue{{Code: "invalid", Path: path, Message: message}}}
}
func notFound() *Error {
	return problem("monitoring_not_found", "监测数据不存在或当前不可访问。", 404)
}
func revisionChanged() *Error {
	return problem("revision_changed", "内容或授权已变化，请刷新后重新核对。", 409)
}
func digest(value any) string {
	b, _ := json.Marshal(value)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type WriteMeta struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision string `json:"expected_revision"`
	Reason           string `json:"reason"`
}

func (m WriteMeta) validate() error {
	if !access.ValidID(m.RequestID) {
		return invalid("request_id", "需要有效请求标识。")
	}
	if len(strings.TrimSpace(m.Reason)) == 0 || len([]rune(m.Reason)) > 2000 {
		return invalid("reason", "请提供 1–2000 字原因。")
	}
	return nil
}

type Range struct {
	From *string `json:"from"`
	To   *string `json:"to"`
}
type DataGrant struct {
	ChannelName     string     `json:"channel_name"`
	ChannelCode     string     `json:"channel_code"`
	PointName       string     `json:"point_name"`
	FacilityName    string     `json:"facility_name"`
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	ProjectID       string     `json:"project_id"`
	ChannelID       string     `json:"channel_id"`
	ResourceGrantID string     `json:"resource_grant_id"`
	Range           Range      `json:"range"`
	Capabilities    []string   `json:"capabilities"`
	Revision        string     `json:"revision"`
	GrantedAt       time.Time  `json:"granted_at"`
	RevokedAt       *time.Time `json:"revoked_at"`
	Effective       bool       `json:"effective"`
}
type GrantInput struct {
	WriteMeta
	ChannelID       string   `json:"channel_id"`
	ResourceGrantID string   `json:"resource_grant_id"`
	Range           *Range   `json:"range"`
	Capabilities    []string `json:"capabilities"`
}
type GrantResult struct {
	ID             string `json:"id"`
	Revision       string `json:"revision"`
	AccessRevision string `json:"access_revision"`
	Replayed       bool   `json:"replayed"`
}
type GrantPage struct {
	Items          []DataGrant `json:"items"`
	AccessRevision string      `json:"access_revision"`
	NextCursor     string      `json:"next_cursor"`
}
type RevokeInput struct {
	WriteMeta
	PreviewHash string `json:"preview_hash"`
}
type GrantPreview struct {
	Grant          DataGrant `json:"grant"`
	Hash           string    `json:"preview_hash"`
	AccessRevision string    `json:"access_revision"`
}

// StatusCode allows cross-module metadata adapters to distinguish hidden data from outages.
func (e *Error) StatusCode() int { return e.Status }
