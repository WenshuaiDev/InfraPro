package audit

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"time"
)

// reserveTemp is shared by encoding and downloads. Durable reservations bound
// simultaneous processes on the node; a crashed process cannot release another
// process's reservation or reserve the same capacity indefinitely.
func (s *Service) reserveTemp(ctx context.Context, size int64) (func(), error) {
	if size < 1 || size > s.options.ExportMaxBytes {
		return nil, Problem("audit_export_too_large", 422)
	}
	id := uuid.NewString()
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,834327123))`, s.options.NodeID); e != nil {
			return e
		}
		var used int64
		if e := tx.QueryRow(ctx, `SELECT COALESCE(sum(reserved_bytes),0)::bigint FROM audit_temp_reservations WHERE node_id=$1 AND released_at IS NULL AND expires_at>clock_timestamp()`, s.options.NodeID).Scan(&used); e != nil {
			return e
		}
		if used+size > s.options.TempBudget {
			return Problem("audit_temp_budget_exceeded", 503)
		}
		_, e := tx.Exec(ctx, `INSERT INTO audit_temp_reservations(id,node_id,reserved_bytes,expires_at) VALUES($1,$2,$3,clock_timestamp()+$4*interval '1 second')`, id, s.options.NodeID, size, (s.options.ReadTimeout + time.Minute).Seconds())
		return e
	})
	if e != nil {
		return nil, e
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE audit_temp_reservations SET released_at=clock_timestamp() WHERE id=$1 AND released_at IS NULL`, id)
			return e
		})
	}, nil
}
