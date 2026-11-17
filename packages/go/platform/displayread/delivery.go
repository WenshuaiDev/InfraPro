package displayread

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
)

// ValidateStoredTx is the final delivery gate for persisted subscription events.
// The caller holds the access lock in the same transaction as the delivery claim.
func (s *Service) ValidateStoredTx(ctx context.Context, tx pgx.Tx, a Authority, id string) error {
	var method, state string
	var evidence Evidence
	var proof, requestRaw json.RawMessage
	var scopes map[string]Scope
	var info QueryInfo
	e := tx.QueryRow(ctx, `SELECT method,state,expires_at,source_vector,proof,request,scopes FROM display_read_queries WHERE id=$1 AND subject_id=$2 AND session_id=$3 AND config_id=$4 AND environment=$5`, id, a.Owner.Subject, a.Owner.Session, a.Owner.Config, a.Owner.Environment).Scan(&method, &state, &info.ExpiresAt, &evidence, &proof, &requestRaw, &scopes)
	if e == pgx.ErrNoRows {
		return Hidden()
	}
	if e != nil {
		return e
	}
	if !info.ExpiresAt.After(time.Now()) || state == "expired" {
		return Problem("query_expired", "读取快照已过期。", 409)
	}
	if state == "canceled" || state == "failed" {
		return Problem("reset_required", "读取快照已失效。", 409)
	}
	if e = s.checkParentTx(ctx, tx, id); e != nil {
		return e
	}
	if _, e = a.Validate(ctx, tx, method, &evidence); e != nil {
		return e
	}
	if validate := s.validators[method]; validate != nil && len(proof) > 0 {
		request, e := DecodeRequest(method, requestRaw)
		if e != nil {
			return e
		}
		info.ID = id
		info.Method = method
		return validate(ctx, tx, Plan{Authority: a, Method: method, Request: request, Scopes: scopes, Evidence: evidence, Query: info}, proof)
	}
	return nil
}
