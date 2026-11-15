// Package audit provides authorized personnel projections and bounded exports.
// Source modules retain ownership of business authorization, execution and files.
package audit

import (
	"context"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

type Scope = ar.Scope
type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Status     int    `json:"-"`
	Retryable  bool   `json:"retryable"`
	NextAction string `json:"next_action"`
	cause      error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.cause }
func sourceUnavailable(cause error) *Error {
	e := Problem("audit_source_unavailable", 503)
	e.cause = cause
	return e
}
func Problem(code string, status int) *Error {
	messages := map[string]string{"audit_filter_invalid": "筛选条件无效，请核对时间、标识及分页。", "audit_reference_invalid": "关联标识无效。", "audit_not_found": "当前记录不可访问。", "audit_cursor_stale": "查询资格或分页位置已变化，请刷新第一页。", "audit_epoch_changed": "服务恢复版本已变化，请刷新后重新提交。", "audit_source_unavailable": "关联来源暂不可核实，请稍后重试。", "audit_query_budget_exceeded": "候选记录过多，请缩小查询范围。", "audit_storage_unavailable": "审计成果存储暂不可用。", "audit_export_invalidated": "生成后的授权已变化，请重新生成。", "audit_export_expired": "审计成果已超过 24 小时有效期。", "audit_export_limit": "活动审计导出已达到限额。", "audit_export_too_large": "导出超出行数或体积上限，请缩小范围。", "audit_protocol_unavailable": "审计正在维护，暂不接受此操作。", "request_conflict": "该命令编号已用于不同内容。", "command_id_required": "该操作需要命令编号。", "command_receipt_expired": "该命令的详细收据已过期，不会重复执行。", "operation_status_unknown": "暂时无法核实提交结果，请使用原命令编号查询。"}
	message := messages[code]
	if message == "" {
		message = "操作未完成，请通过排障编号联系维护人员。"
	}
	next := "refresh"
	if code == "audit_query_budget_exceeded" || code == "audit_export_too_large" {
		next = "narrow_filter"
	}
	if status == 503 {
		next = "retry_later"
	}
	return &Error{code, message, status, status == 503, next, nil}
}

type Name struct {
	ID         string `json:"id,omitempty"`
	Type       string `json:"type"`
	Ref        string `json:"ref,omitempty"`
	Name       string `json:"name"`
	Username   string `json:"username,omitempty"`
	NameSource string `json:"name_source"`
}
type Object struct {
	Type        string        `json:"type"`
	ID          string        `json:"id"`
	Revision    string        `json:"revision,omitempty"`
	Label       string        `json:"label"`
	LabelState  ar.ValueState `json:"label_state"`
	LabelSource string        `json:"label_source"`
}
type Technical struct {
	TaskReferences []TaskReference `json:"task_reference,omitempty"`
	SourceModule   string          `json:"source_module"`
	SourceKey      string          `json:"source_key"`
	OperationID    string          `json:"operation_id,omitempty"`
	RequestID      string          `json:"request_id,omitempty"`
	RequestKind    string          `json:"request_id_kind"`
	ErrorCode      string          `json:"error_code,omitempty"`
	ExecutorKind   string          `json:"executor_kind,omitempty"`
	ExecutorRef    string          `json:"executor_ref,omitempty"`
	IP             string          `json:"source_ip,omitempty"`
	UserAgent      string          `json:"user_agent,omitempty"`
}
type Event struct {
	ID              string       `json:"id"`
	Scope           Scope        `json:"scope"`
	EventType       string       `json:"event_type"`
	ActionLabel     string       `json:"action_label"`
	Category        string       `json:"category"`
	Phase           string       `json:"phase"`
	Outcome         string       `json:"outcome"`
	Actor           Name         `json:"actor"`
	Initiator       Name         `json:"initiator"`
	RequestedBy     Name         `json:"requested_by"`
	Object          Object       `json:"object"`
	OperationID     string       `json:"operation_id,omitempty"`
	Reason          string       `json:"reason"`
	OccurredAt      time.Time    `json:"occurred_at"`
	RecordedAt      time.Time    `json:"recorded_at"`
	TimeSource      string       `json:"time_source"`
	Changes         []ar.Change  `json:"changes"`
	EvidenceQuality string       `json:"evidence_quality"`
	MissingFields   []string     `json:"missing_fields"`
	Technical       *Technical   `json:"technical,omitempty"`
	SourceState     string       `json:"source_state"`
	Actions         EventActions `json:"actions"`
}
type EventActions struct {
	Trace     bool `json:"trace"`
	Technical bool `json:"technical"`
}
type EventPage struct {
	Items                 []Event   `json:"items"`
	NextCursor            string    `json:"next_cursor"`
	ObservedAt            time.Time `json:"observed_at"`
	Coverage              string    `json:"coverage"`
	MissingFields         []string  `json:"missing_fields"`
	AuthorizationRevision string    `json:"authorization_revision"`
}
type EventDetail struct {
	Event
	Links                 []ResolvedLink `json:"links"`
	ObservedAt            time.Time      `json:"observed_at"`
	AuthorizationRevision string         `json:"authorization_revision"`
}
type Filter struct {
	ReferenceID string `json:"reference_id,omitempty"`
	From        string `json:"from,omitempty"`
	To          string `json:"to,omitempty"`
	ActorID     string `json:"actor_id,omitempty"`
	ObjectType  string `json:"object_type,omitempty"`
	ObjectID    string `json:"object_id,omitempty"`
	EventType   string `json:"event_type,omitempty"`
	Outcome     string `json:"outcome,omitempty"`
	Category    string `json:"category,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
	Query       string `json:"q,omitempty"`
	TaskModule  string `json:"task_module,omitempty"`
	TaskKind    string `json:"task_kind,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	Generation  string `json:"generation,omitempty"`
	Cursor      string `json:"cursor,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}
type Capabilities struct {
	Read                  bool     `json:"read"`
	Export                bool     `json:"export"`
	Technical             bool     `json:"technical"`
	Manage                bool     `json:"manage"`
	FieldProfiles         []string `json:"field_profiles"`
	Epoch                 string   `json:"epoch"`
	AuthorizationRevision string   `json:"authorization_revision"`
	ProjectionVersion     string   `json:"projection_version"`
	MembershipKey         string   `json:"membership_key"`
	MaxRows               string   `json:"max_rows"`
	MaxBytes              string   `json:"max_bytes"`
	ResultTTLSeconds      string   `json:"result_ttl_seconds"`
	StorageAvailable      bool     `json:"storage_available"`
}
type SubjectRef struct {
	Key        string
	Module     string
	ObjectType string
	ObjectID   string
	Revision   string
	Scope      Scope
	Metadata   string
}
type Disclosure struct {
	Allowed     bool
	CurrentName string
	State       string
}
type DisclosureResolver interface {
	ResolveTx(context.Context, pgx.Tx, access.Actor, Scope, []SubjectRef) (map[string]Disclosure, error)
}
type Target struct {
	Parameters map[string]string `json:"parameters,omitempty"`
	Available  bool              `json:"available"`
	Kind       string            `json:"kind,omitempty"`
	Scope      Scope             `json:"scope"`
	ID         string            `json:"id,omitempty"`
	Generation string            `json:"generation,omitempty"`
	Revision   string            `json:"revision,omitempty"`
	ReasonCode string            `json:"reason_code,omitempty"`
}
type DeliverySummary struct {
	State      string    `json:"state"`
	ObservedAt time.Time `json:"observed_at"`
	Count      *string   `json:"count,omitempty"`
	ErrorCode  string    `json:"error_code,omitempty"`
}
type ResolvedLink struct {
	ID         string           `json:"id"`
	Kind       string           `json:"kind"`
	Label      string           `json:"label"`
	State      string           `json:"state"`
	Available  bool             `json:"available"`
	ReasonCode string           `json:"reason_code,omitempty"`
	Target     *Target          `json:"target,omitempty"`
	Delivery   *DeliverySummary `json:"delivery,omitempty"`
}
type TraceResolver interface {
	ResolveTx(context.Context, pgx.Tx, access.Actor, []ar.Link, bool) (map[string]ResolvedLink, error)
}
type TraceFilter struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}
type TracePage struct {
	Items      []Event        `json:"items"`
	Edges      []Edge         `json:"edges"`
	Links      []ResolvedLink `json:"links"`
	NextCursor string         `json:"next_cursor"`
	ObservedAt time.Time      `json:"observed_at"`
	Partial    bool           `json:"partial"`
}
type Reader interface {
	Capabilities(context.Context, access.Actor, Scope) (Capabilities, error)
	Events(context.Context, access.Actor, Scope, Filter) (EventPage, error)
	Event(context.Context, access.Actor, Scope, string) (EventDetail, error)
	Trace(context.Context, access.Actor, Scope, string, TraceFilter) (TracePage, error)
	Target(context.Context, access.Actor, Scope, string, string) (Target, error)
}

type TaskReference struct {
	Module     string `json:"source_module"`
	Kind       string `json:"source_kind"`
	RootID     string `json:"source_root_id"`
	Generation string `json:"generation"`
}

func (e *Error) PublicCode() string { return e.Code }
