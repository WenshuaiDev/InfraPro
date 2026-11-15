// Package auditrecord owns append-only audit facts. It has no authorization,
// transport, task executor, notification inbox, or object storage dependency.
package auditrecord

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrConflict        = errors.New("audit_fact_conflict")
	ErrInvalid         = errors.New("audit_fact_invalid")
	ErrUnregistered    = errors.New("audit_event_unregistered")
	ErrProtocol        = errors.New("audit_protocol_unavailable")
	ErrCommandConflict = errors.New("request_conflict")
	ErrCommandRequired = errors.New("command_id_required")
	ErrReceiptExpired  = errors.New("command_receipt_expired")
)

type Scope struct {
	Kind    string `json:"kind"`
	Tenant  string `json:"tenant_id,omitempty"`
	Project string `json:"project_id,omitempty"`
}

func (s Scope) Key() string {
	switch s.Kind {
	case "platform":
		return "platform"
	case "tenant":
		return "tenant:" + s.Tenant
	default:
		return "project:" + s.Project
	}
}
func (s Scope) Valid() bool {
	switch s.Kind {
	case "platform":
		return s.Tenant == "" && s.Project == ""
	case "tenant":
		return ID(s.Tenant) && s.Project == ""
	case "project":
		return ID(s.Tenant) && ID(s.Project)
	}
	return false
}
func ID(v string) bool { id, e := uuid.Parse(v); return e == nil && id.String() == v && id != uuid.Nil }
func Number(v string, min int64) bool {
	n, e := strconv.ParseInt(v, 10, 64)
	return e == nil && n >= min && strconv.FormatInt(n, 10) == v
}
func JSON(v any) []byte { b, _ := json.Marshal(v); return b }
func Hash(v any) string { h := sha256.Sum256(JSON(v)); return hex.EncodeToString(h[:]) }
func null(v string) any {
	if v == "" {
		return nil
	}
	return v
}

type ValueState string

const (
	Recorded    ValueState = "recorded"
	NotRecorded ValueState = "not_recorded"
	Redacted    ValueState = "redacted"
	Truncated   ValueState = "truncated"
)

type Change struct {
	Key         string     `json:"key"`
	Label       string     `json:"label"`
	Type        string     `json:"type"`
	Before      any        `json:"before"`
	After       any        `json:"after"`
	BeforeState ValueState `json:"before_state"`
	AfterState  ValueState `json:"after_state"`
}
type Fact struct {
	Module            string         `json:"source_module"`
	SourceKey         string         `json:"source_key"`
	Effect            string         `json:"effect_key"`
	Event             string         `json:"event_type"`
	Scope             Scope          `json:"scope"`
	Visibility        string         `json:"visibility_class"`
	Category          string         `json:"category"`
	Phase             string         `json:"phase"`
	Outcome           string         `json:"outcome"`
	ActorType         string         `json:"actor_type"`
	ActorID           string         `json:"actor_id,omitempty"`
	AccountID         string         `json:"account_id,omitempty"`
	Initiator         string         `json:"initiator_id,omitempty"`
	RequestedBy       string         `json:"requested_by,omitempty"`
	ExecutorKind      string         `json:"executor_kind,omitempty"`
	ExecutorRef       string         `json:"executor_ref,omitempty"`
	ObjectType        string         `json:"object_type"`
	ObjectID          string         `json:"object_id"`
	ObjectRevision    string         `json:"object_revision,omitempty"`
	OperationID       string         `json:"operation_id,omitempty"`
	ParentOperationID string         `json:"parent_operation_id,omitempty"`
	RequestID         string         `json:"request_id,omitempty"`
	RequestKind       string         `json:"request_id_kind"`
	Reason            string         `json:"reason"`
	ErrorCode         string         `json:"error_code,omitempty"`
	IP                string         `json:"source_ip,omitempty"`
	UserAgent         string         `json:"user_agent,omitempty"`
	OccurredAt        time.Time      `json:"occurred_at,omitempty"`
	Changes           []Change       `json:"changes"`
	Context           map[string]any `json:"safe_context"`
	Details           map[string]any `json:"details"`
	Quality           string         `json:"evidence_quality"`
	Missing           []string       `json:"missing_fields"`
	Links             []Link         `json:"links,omitempty"`
}
type EventRef struct {
	ID          string `json:"event_id"`
	OperationID string `json:"operation_id"`
}
type Link struct {
	ID         string `json:"id,omitempty"`
	Kind       string `json:"kind"`
	Module     string `json:"source_module"`
	TargetKind string `json:"target_kind"`
	TargetID   string `json:"target_id"`
	Generation string `json:"generation,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Relation   string `json:"relation"`
	Scope      Scope  `json:"scope"`
}
type Writer interface {
	AppendTx(context.Context, pgx.Tx, Fact) (EventRef, error)
	LinkTx(context.Context, pgx.Tx, EventRef, []Link) error
}
type Store struct{}

func (Store) AppendTx(ctx context.Context, tx pgx.Tx, f Fact) (EventRef, error) {
	return AppendTx(ctx, tx, f)
}
func (Store) LinkTx(ctx context.Context, tx pgx.Tx, r EventRef, l []Link) error {
	return LinkTx(ctx, tx, r, l)
}

// CommandContext is written only by the command owner, never from a request ID.
// It replaces cross-module SQL guessing in access.Audit.
type CommandContext struct {
	TaskID      string `json:"task_id"`
	AfterState  string `json:"after_state"`
	OperationID string `json:"operation_id"`
	ActorID     string `json:"actor_id"`
	Initiator   string `json:"initiator_id"`
	RequestedBy string `json:"requested_by"`
	Module      string `json:"module"`
	Kind        string `json:"kind"`
	RootID      string `json:"root_id"`
	Generation  string `json:"generation"`
	BeforeState string `json:"before_state"`
	Action      string `json:"action"`
	RequestID   string `json:"request_id"`
	Scope       Scope  `json:"scope"`
}

func SetCommandTx(ctx context.Context, tx pgx.Tx, c CommandContext) error {
	if !ID(c.OperationID) || !ID(c.ActorID) || !ID(c.RootID) || !Number(c.Generation, 1) || !c.Scope.Valid() {
		return ErrInvalid
	}
	_, e := tx.Exec(ctx, `SELECT set_config('infrapro.audit_command',$1,true)`, string(JSON(c)))
	return e
}
