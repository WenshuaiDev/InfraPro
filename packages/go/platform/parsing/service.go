package parsing

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

type Service struct {
	access  *access.Service
	options Options
	objects ObjectStore
}

func New(a *access.Service) *Service                  { return &Service{access: a, options: DefaultOptions()} }
func (s *Service) WithOptions(o Options) *Service     { s.options = o; return s }
func (s *Service) WithStorage(o ObjectStore) *Service { s.objects = o; return s }
func scopeKey(s Scope) string {
	if s.Entry == "platform" {
		return "platform"
	}
	return "tenant:" + s.Tenant
}
func platform() Scope        { return k.Platform() }
func tenant(id string) Scope { return k.Tenant(id) }
func op(s Scope, kind, action string) access.Operation {
	return access.ParsingOperation(s.Entry, kind, action)
}
func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, kind, action string, write bool, fn func(pgx.Tx, k.Authorization) error) error {
	return k.Transact(ctx, s.access, a, scope, op(scope, kind, action), k.Locks{CatalogWrite: write && scope.Entry == "platform", TenantWrite: write && scope.Entry == "tenant"}, fn)
}
func validateMeta(m *Meta, reason bool) error {
	if e := m.Normalize(); e != nil {
		return e
	}
	if reason {
		return m.RequireReason()
	}
	return nil
}
func replay(ctx context.Context, tx pgx.Tx, auth k.Authorization, operation string, m Meta, input any) (Result, bool, error) {
	var out Result
	var hash string
	e := tx.QueryRow(ctx, `SELECT payload_hash,result FROM parsing_operation_receipts WHERE scope_key=$1 AND actor_id=$2 AND operation=$3 AND request_id=$4`, scopeKey(auth.Scope), auth.Account, operation, m.RequestID).Scan(&hash, &out)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, false, nil
	}
	if e != nil {
		return out, false, e
	}
	if hash != k.Hash(input) {
		return out, false, k.Problem("request_conflict", "相同请求 ID 已用于不同内容。", 409)
	}
	out.Replayed = true
	return out, true, nil
}
func complete(ctx context.Context, tx pgx.Tx, auth k.Authorization, operation string, m Meta, input any, out Result, before, after any) error {
	_, e := tx.Exec(ctx, `INSERT INTO parsing_operation_receipts(scope_key,actor_id,operation,request_id,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6)`, scopeKey(auth.Scope), auth.Account, operation, m.RequestID, k.Hash(input), k.JSON(out))
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO parsing_changes(scope_key,actor_id,operation,target_id,request_id,reason,before_snapshot,after_snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, scopeKey(auth.Scope), auth.Account, operation, out.ID, m.RequestID, m.Reason, k.JSON(before), k.JSON(after))
	if e != nil {
		return e
	}
	return access.Audit(ctx, tx, "parsing."+operation, "parsing_"+auditKind(operation), out.ID, "", before, after, access.Meta{ActorID: auth.Account, ActorScope: auth.Entry, TenantID: auth.Tenant, RequestID: m.RequestID, Reason: m.Reason})
}
func auditKind(op string) string {
	for i, c := range op {
		if c == '.' {
			return op[:i]
		}
	}
	return op
}
func readJSON[T any](row pgx.Row) (T, error) {
	var v T
	var raw []byte
	e := row.Scan(&raw)
	if e == pgx.ErrNoRows {
		return v, k.NotFound("parsing")
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(raw, &v)
	return v, e
}
func workerActor(id string) access.Actor {
	return access.Actor{ID: id, Check: func(ctx context.Context, tx pgx.Tx, _ bool) error {
		var active bool
		if e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, id).Scan(&active); e != nil {
			return e
		}
		if !active {
			return access.ErrDenied
		}
		return nil
	}}
}
func require(auth k.Authorization, platformCode, tenantCode access.Code) error {
	if auth.Entry == "platform" {
		return auth.Require(platformCode)
	}
	return auth.Require(tenantCode)
}

func decode(raw []byte, out any) error { return json.Unmarshal(raw, out) }
