package engineering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Service struct {
	access *access.Service
	usage  UsageReader
}

func New(a *access.Service) *Service { return &Service{access: a} }

// Scope comes from an authorized route, never from a request body.
// Catalog is the tenant view of public, currently adoptable versions.
type Scope struct {
	Tenant  string
	Catalog bool
	Kind    Kind
}

func (v Scope) owner() string {
	if v.Catalog {
		return ""
	}
	return v.Tenant
}
func (v Scope) key() string {
	if v.owner() == "" {
		return "public"
	}
	return v.owner()
}
func (v Scope) operation(action string) access.Operation {
	return access.EngineeringOperation(v.Tenant != "" && !v.Catalog, v.Catalog, string(v.Kind), action)
}
func (s *Service) transact(ctx context.Context, a access.Actor, op access.Operation, tenant string, exclusive bool, fn func(pgx.Tx) error) error {
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		p, ok := access.PolicyFor(op)
		if !ok || a.Check == nil {
			return access.ErrDenied
		}
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM accounts WHERE id::text=$1 FOR UPDATE`, a.ID); e != nil {
			return e
		}
		if e := a.Check(ctx, tx, p.Recent); e != nil {
			return e
		}
		if e := access.RequireScopedTx(ctx, tx, a.ID, op, tenant); e != nil {
			return e
		}
		mode := " FOR SHARE"
		if exclusive {
			mode = " FOR UPDATE"
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1`+mode); e != nil {
			return e
		}
		return fn(tx)
	})
	s.access.LogDenial(op, a.ID, e)
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "23505":
			return problem("engineering_conflict", "编号已保留或操作发生冲突，请重新读取。", 409)
		case "55P03", "57014", "40P01":
			return problem("unavailable", "操作繁忙，请核对结果后重试。", 503)
		case "23514":
			return problem("engineering_constraint", "操作违反历史或引用保护规则。", 409)
		}
		return problem("internal_error", "操作未完成，请通过请求编号联系维护人员。", 500)
	}
	return e
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func checkMeta(m WriteMeta) error {
	if !access.ValidID(m.RequestID) {
		return access.Validation("request_id", "请提供有效的请求编号。")
	}
	return access.ValidReason(m.Reason)
}
func checkCode(code string) error {
	if !keyPattern.MatchString(code) {
		return access.Validation("code", "编号须以小写字母开头，使用字母、数字、下划线，最多 64 字符。")
	}
	return nil
}
func contentBoundary(kind Kind, c Content) error {
	b, e := json.Marshal(c)
	if e != nil || len(b) > MaxContentBytes {
		return access.Validation("content", "定义内容过大或包含无效数值。")
	}
	for _, i := range ValidateContent(kind, c) {
		if strings.HasPrefix(i.Code, "unsupported_") || i.Code == "input_limit" {
			return &Error{Code: "invalid_request", Message: i.Message, Status: 400, Issues: []Issue{i}}
		}
	}
	return nil
}
func receipt(ctx context.Context, tx pgx.Tx, a access.Actor, scope, op string, m WriteMeta, payload any) (Result, bool, error) {
	out := Result{Issues: []Issue{}}
	if e := checkMeta(m); e != nil {
		return out, false, e
	}
	var old, oldOp string
	e := tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM engineering_operation_receipts WHERE scope_key=$1 AND actor_id=$2 AND request_id=$3`, scope, a.ID, m.RequestID).Scan(&oldOp, &old, &out)
	if e == pgx.ErrNoRows {
		return out, false, nil
	}
	if e != nil {
		return out, false, e
	}
	if old != hash(payload) || oldOp != op {
		return out, false, problem("request_payload_conflict", "该请求编号已用于不同操作或内容。", 409)
	}
	out.Replayed = true
	return out, true, nil
}
func complete(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, op string, m WriteMeta, payload any, result Result, before, after any, meta access.Meta) (Result, error) {
	meta.ActorID = a.ID
	meta.TenantID = scope.owner()
	meta.ProjectID = ""
	meta.ActorScope = "platform"
	if meta.TenantID != "" {
		meta.ActorScope = "tenant"
	}
	meta.Reason = m.Reason
	if e := access.Audit(ctx, tx, "engineering."+op, "engineering_definition", result.DefinitionID, "", before, after, meta); e != nil {
		return result, e
	}
	_, e := tx.Exec(ctx, `INSERT INTO engineering_operation_receipts(scope_key,actor_id,request_id,operation,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6)`, scope.key(), a.ID, m.RequestID, op, hash(payload), result)
	return result, e
}
func definitionTx(ctx context.Context, tx pgx.Tx, scope Scope, id string, lock bool) (Definition, error) {
	out := Definition{Versions: []Version{}, Dependencies: []Version{}}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	e := tx.QueryRow(ctx, `SELECT id::text,kind,code,COALESCE(tenant_id::text,''),COALESCE(source_version::text,''),source_snapshot,created_at,revision FROM engineering_definitions WHERE id::text=$1 AND kind=$2 AND scope_key=$3 AND deleted_at IS NULL`+suffix, id, scope.Kind, scope.key()).Scan(&out.ID, &out.Kind, &out.Code, &out.TenantID, &out.SourceVersion, &out.Source, &out.CreatedAt, &out.Revision)
	if e == pgx.ErrNoRows {
		return out, access.ErrNotFound
	}
	if e != nil {
		return out, e
	}
	d := Draft{}
	e = tx.QueryRow(ctx, `SELECT revision,COALESCE(base_version::text,''),content,updated_at FROM engineering_drafts WHERE definition_id=$1`, id).Scan(&d.Revision, &d.BaseVersion, &d.Content, &d.UpdatedAt)
	if e == nil {
		out.Draft = &d
	} else if e != pgx.ErrNoRows {
		return out, e
	}
	return out, nil
}
func revision(d Definition, want int) error {
	if want < 1 || d.Revision != want {
		return access.ErrConflict
	}
	return nil
}
