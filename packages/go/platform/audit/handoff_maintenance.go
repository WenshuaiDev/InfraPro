package audit

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

// RetryHandoff is a finite maintenance action, never a business-command retry.
// It reopens only the identified exhausted source handoff at its observed count.
func (s *Service) RetryHandoff(ctx context.Context, id string, attempts int, request, reason string) (map[string]any, error) {
	out := map[string]any{}
	if e := maintenanceInput(request, reason); e != nil {
		return out, e
	}
	if !ar.ID(id) || attempts < 8 || attempts > 100 || s.handoffReplay == nil {
		return out, Problem("audit_filter_invalid", 400)
	}
	intent := ar.Hash([]any{id, attempts, reason})
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,58414))`, request); e != nil {
			return e
		}
		e := tx.QueryRow(ctx, `SELECT summary FROM audit_maintenance_receipts WHERE request_id=$1 AND operation='handoff.retry'`, request).Scan(&out)
		if e == nil {
			if out["intent"] != intent {
				return Problem("request_conflict", 409)
			}
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		var mode string
		if e = tx.QueryRow(ctx, `SELECT mode FROM audit_protocol WHERE id=1 FOR SHARE`).Scan(&mode); e != nil {
			return e
		}
		if mode != "ready" {
			return Problem("audit_protocol_unavailable", 503)
		}
		changed, e := s.handoffReplay(ctx, tx, id, attempts)
		if e != nil {
			return e
		}
		if !changed {
			return Problem("audit_handoff_changed", 409)
		}
		out = map[string]any{"id": id, "intent": intent, "state": "pending", "attempts": attempts}
		if _, e = tx.Exec(ctx, `INSERT INTO audit_maintenance_receipts(request_id,operation,reason,summary) VALUES($1,'handoff.retry',$2,$3)`, request, safeReason(reason), ar.JSON(out)); e != nil {
			return e
		}
		return maintenanceFactTx(ctx, tx, "handoff_replayed", request, reason, map[string]any{"source": "display", "count": "1"})
	})
	return out, e
}
