package access

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"sync"
	"sync/atomic"
	"time"
)

type operationContextKey struct{}
type operationContext struct {
	service                *Service
	actor, action, request string
	once                   sync.Once
}

var persistFailureCount atomic.Uint64

// WithOperation accepts only verified actor identity, a registered route action,
// and the server transport request ID. It intentionally has no body/target input.
func WithOperation(ctx context.Context, s *Service, actor string, op Operation, request string) context.Context {
	if ctx.Value(operationContextKey{}) != nil {
		return ctx
	}
	if _, ok := PolicyFor(op); !ok || !ar.ID(actor) {
		return ctx
	}
	return context.WithValue(ctx, operationContextKey{}, &operationContext{s, actor, string(op), ar.SafeRequestID(request), sync.Once{}})
}
func AuditPersistFailures() uint64 { return persistFailureCount.Load() }
func (s *Service) AuditDiagnostic(request, reference, code string) {
	persistFailureCount.Add(1)
	if s.logger != nil {
		s.logger.Error("audit_persist_failure", "request_id", ar.SafeRequestID(request), "reference", ar.SafeRequestID(reference), "error_code", code)
	}
}
func ReportOperationFailure(ctx context.Context, err error) {
	v, ok := ctx.Value(operationContextKey{}).(*operationContext)
	if !ok || err == nil || v.request == "" {
		return
	}
	v.once.Do(func() { v.service.recordFailure(v.actor, v.action, v.request, err) })
}
func (s *Service) recordFailure(actor, action, request string, cause error) {
	classified := ar.ClassifyFailure(cause)
	event := "audit.command.failure"
	if classified.Outcome == "denied" || classified.Outcome == "conflict" || classified.Outcome == "unknown" {
		event = "audit.command." + classified.Outcome
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tx, e := s.pool.Begin(ctx)
	if e == nil {
		defer tx.Rollback(context.Background())
		_, e = tx.Exec(ctx, `SET LOCAL statement_timeout='1500ms'; SET LOCAL lock_timeout='500ms'`)
		if e == nil {
			_, e = ar.AppendTx(ctx, tx, ar.Fact{Module: "audit", Event: event, SourceKey: request + ":" + action + ":command", Effect: "failure", Scope: ar.Scope{Kind: "platform"}, ActorType: "account", ActorID: actor, ObjectType: "operation", ObjectID: "", Outcome: classified.Outcome, RequestID: request, RequestKind: "http", ErrorCode: classified.Code, Context: map[string]any{"action": action, "error_code": classified.Code}})
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
	}
	if e != nil {
		persistFailureCount.Add(1)
		if s.logger != nil {
			s.logger.Error("audit_persist_failure", "request_id", request, "action", action, "error_code", classified.Code)
		}
	}
}
