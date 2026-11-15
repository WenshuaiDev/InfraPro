package auditrecord

import (
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
)

// CommitUnknown never reports a failed business effect: the server may have
// committed even though the acknowledgement was lost.
type CommitUnknown struct{ Cause error }

func (e *CommitUnknown) Error() string { return "operation_status_unknown" }
func (e *CommitUnknown) Unwrap() error { return e.Cause }

type Classification struct {
	Code, Outcome string
	Status        int
}

func ClassifyFailure(err error) Classification {
	var unknown *CommitUnknown
	if errors.As(err, &unknown) {
		return Classification{"operation_status_unknown", "unknown", 503}
	}
	for _, v := range []struct {
		e             error
		code, outcome string
		status        int
	}{
		{ErrCommandRequired, "command_id_required", "failure", 428},
		{ErrCommandConflict, "request_conflict", "conflict", 409},
		{ErrReceiptExpired, "command_receipt_expired", "conflict", 410},
		{ErrConflict, "audit_fact_conflict", "conflict", 409},
		{ErrInvalid, "audit_fact_invalid", "failure", 400},
		{ErrUnregistered, "audit_event_unregistered", "failure", 503},
		{ErrProtocol, "audit_protocol_unavailable", "failure", 503},
	} {
		if errors.Is(err, v.e) {
			return Classification{v.code, v.outcome, v.status}
		}
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		if p.Code == "23505" || p.Code == "40001" || p.Code == "40P01" {
			return Classification{"revision_conflict", "conflict", 409}
		}
		return Classification{"operation_unavailable", "failure", 503}
	}
	status := 500
	var public interface{ StatusCode() int }
	if errors.As(err, &public) {
		status = public.StatusCode()
	}
	outcome, code := "failure", "operation_failed"
	if status == 401 || status == 403 || status == 404 {
		outcome, code = "denied", "operation_not_accessible"
	}
	if status == 409 || status == 410 || status == 412 {
		outcome, code = "conflict", "revision_conflict"
	}
	if status == 400 || status == 422 || status == 428 {
		code = "validation_failed"
	}
	if status == 429 {
		outcome, code = "rate_limited", "rate_limited"
	}
	return Classification{code, outcome, status}
}
