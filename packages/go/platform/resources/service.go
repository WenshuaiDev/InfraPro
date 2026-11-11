package resources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
)

type Service struct{ access *access.Service }

func New(a *access.Service) *Service { return &Service{a} }
func fingerprint(v any) string       { h := sha256.Sum256(jsonBytes(v)); return hex.EncodeToString(h[:]) }
func (s *Service) transact(ctx context.Context, a access.Actor, op access.Operation, scope Scope, exclusive bool, fn func(pgx.Tx, authorization) error) error {
	if !scope.valid() {
		return notFound()
	}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		p, ok := access.PolicyFor(op)
		if !ok || a.Check == nil {
			return access.ErrDenied
		}
		if _, e := access.Lock(ctx, tx, exclusive); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM accounts WHERE id::text=$1 FOR UPDATE`, a.ID); e != nil {
			return e
		}
		if e := a.Check(ctx, tx, p.Recent); e != nil {
			return e
		}
		auth, e := s.authorizeTx(ctx, tx, a, op, scope)
		if e != nil {
			return e
		}
		// This lock always precedes facility locks, including historical interpretation.
		if _, e := tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
			return e
		}
		return fn(tx, auth)
	})
	s.access.LogDenial(op, a.ID, e)
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "23505":
			return problem("resource_code_taken", "该编号已占用或保留，请使用其他编号。", 409)
		case "55P03", "57014", "40P01", "40001":
			return problem("unavailable", "操作繁忙，请核对收据后重试。", 503)
		case "23514", "23503":
			return problem("resource_invalid", "操作违反结构或引用保护规则。", 422)
		}
		return problem("unavailable", "数据库操作未完成，请核对结果后重试。", 503)
	}
	var ee *engineering.Error
	if errors.As(e, &ee) {
		return &Error{Code: ee.Code, Message: ee.Message, Status: ee.Status, Issues: ee.Issues}
	}
	return e
}
func checkTarget(t scopedTarget) error {
	if tables[t.Kind] == "" || !t.Scope.valid() {
		return problem("resource_not_found", "资源不存在或不可访问。", 404)
	}
	for _, id := range []string{t.Facility, t.Point, t.ID} {
		if id != "" && !access.ValidID(id) {
			return notFound()
		}
	}
	if t.Kind != Facility && t.Facility == "" {
		return notFound()
	}
	if t.Kind == Channel && t.Point == "" {
		return notFound()
	}
	return nil
}
func notFound() *Error {
	return problem("resource_not_found", "资源不存在或不可访问。", 404)
}
func lockFacility(ctx context.Context, tx pgx.Tx, t scopedTarget, write, deleted bool) (Resource, error) {
	suffix := " FOR SHARE"
	if write {
		suffix = " FOR UPDATE"
	}
	where := " AND deleted_at IS NULL"
	if deleted {
		where = ""
	}
	var raw []byte
	r := Resource{Kind: Facility}
	e := tx.QueryRow(ctx, `SELECT to_jsonb(f) FROM engineering_facilities f WHERE tenant_id::text=$1 AND id::text=$2`+where+suffix, t.Tenant, t.Facility).Scan(&raw)
	if e == pgx.ErrNoRows {
		return r, notFound()
	}
	if e != nil {
		return r, e
	}
	e = unmarshalResource(raw, &r)
	return r, e
}
func load(ctx context.Context, tx pgx.Tx, t scopedTarget, deleted bool) (Resource, error) {
	r := Resource{Kind: t.Kind}
	where := ""
	if t.Kind != Facility {
		where += " AND facility_id::text=$3"
	}
	if t.Kind == Channel {
		where += " AND point_id::text=$4"
	}
	if !deleted {
		where += " AND deleted_at IS NULL"
	}
	args := []any{t.Tenant, t.ID}
	if t.Kind != Facility {
		args = append(args, t.Facility)
	}
	if t.Kind == Channel {
		args = append(args, t.Point)
	}
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT to_jsonb(r) FROM `+tables[t.Kind]+` r WHERE tenant_id::text=$1 AND id::text=$2`+where, args...).Scan(&raw)
	if e == pgx.ErrNoRows {
		return r, notFound()
	}
	if e != nil {
		return r, e
	}
	e = unmarshalResource(raw, &r)
	return r, e
}
func (s *Service) Receipt(ctx context.Context, a access.Actor, scope Scope, id string) (Result, error) {
	out := Result{}
	tenant := scope.Tenant
	e := s.transact(ctx, a, scope.operation("request", "detail"), scope, false, func(tx pgx.Tx, auth authorization) error {
		if !access.ValidID(id) {
			return notFound()
		}
		if scope.Entry == "project" {
			e := tx.QueryRow(ctx, `SELECT result FROM project_resource_operation_receipts WHERE entry_scope='project' AND tenant_id=$1 AND project_id=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.Project, a.ID, id).Scan(&out)
			if e == pgx.ErrNoRows {
				return notFound()
			}
			if e != nil {
				return e
			}
			if e = projectReceiptReadable(ctx, tx, auth, id, &out); e != nil {
				return e
			}
			out.Replayed = true
			return nil
		}
		var op string
		e := tx.QueryRow(ctx, `SELECT operation,result FROM engineering_resource_receipts WHERE tenant_id::text=$1 AND actor_id=$2 AND request_id=$3`, tenant, a.ID, id).Scan(&op, &out)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		if e != nil {
			return e
		}
		if e = access.RequireScopedTx(ctx, tx, a.ID, access.Operation(op), tenant); e != nil {
			return e
		}
		out.Replayed = true
		return nil
	})
	return out, e
}

// HasResourceReferences answers only existence, never names, tenant IDs or counts.
func (s *Service) HasResourceReferences(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	var out bool
	e := tx.QueryRow(ctx, `WITH RECURSIVE referencing(id) AS (SELECT $1::uuid UNION SELECT d.version_id FROM engineering_dependencies d JOIN referencing r ON d.target_id=r.id) SELECT EXISTS(SELECT 1 FROM engineering_facilities WHERE type_version_id IN (SELECT id FROM referencing)) OR EXISTS(SELECT 1 FROM engineering_channels WHERE metric_version_id IN (SELECT id FROM referencing) OR unit_version_id IN (SELECT id FROM referencing))`, id).Scan(&out)
	return out, e
}
func ptrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func actionAllowed(r Resource, ancestors []Resource, manage bool) Actions {
	out := Actions{}
	if !manage {
		out.Reason = "当前角色只有查看权限。"
		return out
	}
	if r.DeletedAt != nil {
		out.Reason = "对象已删除，仅保留历史。"
		return out
	}
	for _, p := range ancestors {
		if p.Status == "retired" {
			out.Reason = "祖先已退役，请先恢复祖先。"
			return out
		}
	}
	out.Delete = true
	if r.Status == "retired" {
		out.Restore = true
		out.Reason = "对象已退役，请先恢复。"
		return out
	}
	out.Edit = true
	out.Add = r.Kind != Channel
	out.Retire = true
	return out
}
func writable(r Resource, ancestors []Resource, action string) error {
	for _, p := range ancestors {
		if p.Status == "retired" {
			return problem("ancestor_retired", "祖先已退役，请先恢复祖先。", 409)
		}
	}
	if r.Status == "retired" && action != "restore" && action != "delete" {
		return problem("resource_retired", "对象已退役，请先恢复。", 409)
	}
	return nil
}
func operationKind(op string) Kind {
	parts := strings.Split(op, ":")
	if len(parts) == 3 {
		return Kind(parts[1])
	}
	return ""
}
