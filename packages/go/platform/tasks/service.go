// Package tasks presents currently authorized metadata. Sources retain execution,
// data, files and the final decision on every control command.
package tasks

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
)

type Source interface {
	ExistsTx(context.Context, pgx.Tx, taskrecord.SourceRef) (bool, error)
	ResolveTx(context.Context, pgx.Tx, access.Actor, []taskrecord.SourceRef, string) ([]View, error)
	Control(context.Context, access.Actor, taskrecord.SourceRef, Command) (CommandResult, error)
	TargetTx(context.Context, pgx.Tx, access.Actor, taskrecord.SourceRef, string, string) (Target, error)
	OperationsTx(context.Context, pgx.Tx, access.Actor, Filter) ([]Operation, error)
}

// ExecutionResultSource is optional; existing source contracts remain unchanged.
type ExecutionResultSource interface {
	ResultForExecutionTx(context.Context, pgx.Tx, access.Actor, taskrecord.SourceRef, string) (ExecutionResult, error)
}
type ExecutionResult struct {
	ResultState    string
	ResultReadable bool
	NextStep       string
}
type Options struct {
	PageSize, MaxPageSize, CandidateBatch, CandidateBudget, HandoffBatch, RetryLimit, ReconcileBatch int
	MetadataTimeout, Poll, ReconcileInterval, ShutdownGrace                                          time.Duration
}

func DefaultOptions() Options {
	return Options{50, 100, 200, 2000, 100, 8, 500, 5 * time.Second, time.Second, 5 * time.Minute, 10 * time.Second}
}

type Service struct {
	access  *access.Service
	sources map[string]Source
	options Options
}

func New(a *access.Service, sources map[string]Source) *Service {
	copy := map[string]Source{}
	for k, s := range sources {
		if !slices.Contains([]string{"monitoring", "derived", "spatial", "display", "files", "audit", "parsing", "ingestion"}, k) || s == nil {
			panic("unregistered task source")
		}
		copy[k] = s
	}
	return &Service{a, copy, DefaultOptions()}
}
func (s *Service) WithOptions(o Options) *Service { s.options = o; return s }
func Problem(code, message string, status int) error {
	return &access.Error{Code: code, Message: message, Status: status}
}
func Hidden() error { return Problem("task_not_found", "任务不存在或当前不可访问。", 404) }
func Denied(e error) bool {
	var status interface{ StatusCode() int }
	return errors.As(e, &status) && (status.StatusCode() == 403 || status.StatusCode() == 404)
}

func (s *Service) transaction(ctx context.Context, a access.Actor, fn func(pgx.Tx, string) error) error {
	if a.Check == nil || a.ReadSubject() != nil || !access.ValidID(a.ID) {
		return access.ErrDenied
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		rev, e := access.Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		if e = a.Check(ctx, tx, false); e != nil {
			return e
		}
		return fn(tx, rev)
	})
}

type Action struct {
	Kind           string `json:"kind"`
	Enabled        bool   `json:"enabled"`
	RequiresReason bool   `json:"requires_reason"`
	ReasonCode     string `json:"reason_code,omitempty"`
}
type View struct {
	RequestedBy       string               `json:"-"`
	ID                string               `json:"id"`
	Source            taskrecord.SourceRef `json:"source_ref"`
	Type              string               `json:"type"`
	Title             string               `json:"title"`
	Scope             taskrecord.Scope     `json:"scope"`
	ScopeLabel        string               `json:"scope_label"`
	ActorLabel        string               `json:"actor_label,omitempty"`
	Generation        string               `json:"generation"`
	ControlRevision   string               `json:"control_revision"`
	State             string               `json:"state"`
	SourceState       string               `json:"source_state"`
	Phase             string               `json:"phase"`
	Progress          taskrecord.Progress  `json:"progress"`
	ResultState       string               `json:"result_state"`
	NextStep          string               `json:"next_step,omitempty"`
	ErrorCode         string               `json:"error_code,omitempty"`
	Actions           []Action             `json:"actions"`
	CreatedAt         time.Time            `json:"created_at"`
	ObservedAt        time.Time            `json:"observed_at"`
	HistoryIncomplete bool                 `json:"history_incomplete"`
	CleanupState      string               `json:"cleanup_state,omitempty"`
	ResultReadable    bool                 `json:"result_readable"`
	SourceMissing     bool                 `json:"source_missing,omitempty"`
	Originator        string               `json:"-"`
}

func Present(c taskrecord.Change, label string) View {
	d, _ := taskrecord.Lookup(c.Type)
	result := "none"
	if taskrecord.Active(c.State) {
		result = "preparing"
	}
	if c.State == "succeeded" {
		result = "available"
	}
	return View{ID: c.SourceRef.ID(), Source: c.SourceRef, Type: c.Type, Title: d.Title, Scope: c.Scope, ScopeLabel: label, Generation: c.Generation, ControlRevision: c.ControlRevision, State: c.State, SourceState: c.SourceState, Phase: c.Phase, Progress: c.Progress, ResultState: result, ErrorCode: c.ErrorCode, Actions: []Action{}, CreatedAt: c.CreatedAt, ObservedAt: c.ObservedAt, HistoryIncomplete: c.HistoryIncomplete, Originator: c.Originator, RequestedBy: c.RequestedBy}
}
func Capability(v *View, kind string, enabled, reason bool, code string) {
	v.Actions = append(v.Actions, Action{kind, enabled, reason, code})
}

type Target struct {
	Available  bool              `json:"available"`
	Kind       string            `json:"kind,omitempty"`
	Scope      taskrecord.Scope  `json:"scope"`
	ID         string            `json:"id,omitempty"`
	Generation string            `json:"generation,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
}
type Operation struct {
	Source     string           `json:"source_module"`
	Scope      taskrecord.Scope `json:"scope"`
	ScopeLabel string           `json:"scope_label"`
	State      string           `json:"state"`
	Queued     *string          `json:"queued"`
	Running    *string          `json:"running"`
	Failed     *string          `json:"failed"`
	ObservedAt time.Time        `json:"observed_at"`
	Target     string           `json:"target_kind,omitempty"`
}

// ScopeTx performs only current membership/role checks, never data reads.
func ScopeTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope taskrecord.Scope) ([]access.Code, string, string, error) {
	var codes []access.Code
	var name, status string
	if scope.Kind == "platform" {
		var e error
		codes, e = access.PermissionsTx(ctx, tx, a.ID)
		return codes, "平台", "active", e
	} else if scope.Kind == "project" {
		p, e := access.ProjectSnapshotTx(ctx, tx, a.ID, scope.Tenant, scope.Project, "")
		if e != nil {
			return nil, "", "", e
		}
		codes = p.Permissions
		status = p.ProjectStatus
		if e = tx.QueryRow(ctx, `SELECT name FROM projects WHERE tenant_id=$1 AND id=$2`, scope.Tenant, scope.Project).Scan(&name); e != nil {
			return nil, "", "", e
		}
	} else {
		_, p, e := access.TenantSnapshotTx(ctx, tx, a.ID, scope.Tenant, "")
		if e != nil {
			return nil, "", "", e
		}
		codes = p.Permissions
		if e = tx.QueryRow(ctx, `SELECT name,status FROM tenants WHERE id=$1`, scope.Tenant).Scan(&name, &status); e != nil {
			return nil, "", "", e
		}
	}
	return codes, name, status, nil
}
func Has(codes []access.Code, code string) bool { return slices.Contains(codes, access.Code(code)) }
func CanManage(codes []access.Code, d taskrecord.Definition, scope string) bool {
	switch d.Module {
	case "parsing":
		read, execute := access.ParsingRunRead, access.ParsingTestExecute
		if scope == "tenant" {
			read, execute = access.TenantParsingRunRead, access.TenantParsingTestExecute
		}
		return Has(codes, string(read)) && Has(codes, string(execute))
	case "audit":
		return Has(codes, scope+".audit.read") && Has(codes, scope+".audit.exports.manage")
	case "files":
		return Has(codes, scope+".profile.read") && Has(codes, scope+".attachments.read") && Has(codes, scope+".attachments.manage")
	case "monitoring":
		return Has(codes, scope+".monitoring.jobs.read") || Has(codes, scope+".monitoring.jobs.manage")
	case "derived":
		return Has(codes, scope+".derived.runs.read") || Has(codes, scope+".derived.runs.manage")
	case "spatial":
		return Has(codes, string(access.TenantSpatialManage))
	case "display":
		return Has(codes, string(access.ProjectDisplayUpload))
	}
	return false
}
func CanUse(codes []access.Code, d taskrecord.Definition, scope, mode string) bool {
	if !slices.Contains(d.Scopes, scope) {
		return false
	}
	if CanManage(codes, d, scope) {
		return true
	}
	if mode == "managed" {
		return false
	}
	switch d.Module {
	case "parsing":
		read := access.ParsingRunRead
		if scope == "tenant" {
			read = access.TenantParsingRunRead
		}
		return Has(codes, string(read))
	case "audit":
		return Has(codes, scope+".audit.read")
	case "files":
		return Has(codes, scope+".profile.read") && Has(codes, scope+".attachments.read")
	case "monitoring":
		if strings.Contains(d.Type, "import") {
			return Has(codes, scope+".monitoring.imports.read")
		}
		return Has(codes, scope+".monitoring.data.read")
	case "derived":
		return Has(codes, scope+".derived.runs.read")
	case "spatial":
		return Has(codes, string(access.TenantSpatialRead))
	case "display":
		return Has(codes, string(access.ProjectDisplayUpload))
	}
	return false
}
