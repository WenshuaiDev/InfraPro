package devicekit

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicecoord"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"slices"
	"strings"
)

type Authorization struct {
	Scope
	Account, Name, TenantMembership, ProjectMembership, Revision                string
	Permissions                                                                 []access.Code
	ProjectStatus, ProjectVersion, ResourceAccessRevision, DeviceAccessRevision string
}

func (a Authorization) Has(c access.Code) bool { return slices.Contains(a.Permissions, c) }
func (a Authorization) Require(codes ...access.Code) error {
	for _, c := range codes {
		if !a.Has(c) {
			return access.ErrDenied
		}
	}
	return nil
}

// AuthorizeTx requires the caller's authorization graph and account locks. It
// never starts a transaction and always rechecks the explicit route scope.
func AuthorizeTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, op access.Operation) (Authorization, error) {
	out := Authorization{Scope: scope, Account: a.ID, Permissions: []access.Code{}}
	if !scope.Valid() {
		return out, NotFound("device")
	}
	var err error
	switch scope.Entry {
	case "project":
		var v access.ProjectAuthorization
		v, err = access.RequireProjectTx(ctx, tx, a.ID, op, scope.Tenant, scope.Project)
		out.TenantMembership = v.TenantMembershipID
		out.ProjectMembership = v.MembershipID
		out.Permissions = v.Permissions
		out.ProjectStatus = v.ProjectStatus
		if err == nil {
			err = tx.QueryRow(ctx, `SELECT p.version::text,r.revision::text,d.revision::text FROM projects p JOIN project_resource_policies r ON r.project_id=p.id JOIN project_device_access_states d ON d.project_id=p.id WHERE p.tenant_id=$1 AND p.id=$2`, scope.Tenant, scope.Project).Scan(&out.ProjectVersion, &out.ResourceAccessRevision, &out.DeviceAccessRevision)
		}
	case "tenant":
		err = access.RequireScopedTx(ctx, tx, a.ID, op, scope.Tenant)
		if err == nil {
			var snap access.Snapshot
			out.TenantMembership, snap, err = access.TenantSnapshotTx(ctx, tx, a.ID, scope.Tenant, "")
			out.Permissions = snap.Permissions
		}
	case "platform":
		if op != scope.Operation("request", "detail") {
			err = access.RequireTx(ctx, tx, a.ID, op)
		}
		if err == nil {
			for _, c := range access.Catalog() {
				if c.Scope == "platform" {
					var has bool
					has, err = access.HasTx(ctx, tx, a.ID, c.Code)
					if err != nil {
						break
					}
					if has {
						out.Permissions = append(out.Permissions, c.Code)
					}
				}
			}
		}
	}
	if err != nil {
		return out, err
	}
	err = tx.QueryRow(ctx, `SELECT COALESCE(NULLIF(display_name,''),username) FROM accounts WHERE id=$1`, a.ID).Scan(&out.Name)
	return out, err
}

type Locks struct{ CatalogWrite, TenantWrite, Governance bool }

func Transact(ctx context.Context, s *access.Service, a access.Actor, scope Scope, op access.Operation, locks Locks, fn func(pgx.Tx, Authorization) error) error {
	if !scope.Valid() {
		return NotFound("device")
	}
	err := s.Transact(ctx, func(tx pgx.Tx) error {
		policy, ok := access.PolicyFor(op)
		if !ok || a.Check == nil {
			return access.ErrDenied
		}
		revision, e := access.Lock(ctx, tx, locks.Governance)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, a.ID); e != nil {
			return e
		}
		if e = a.Check(ctx, tx, policy.Recent); e != nil {
			return e
		}
		auth, e := AuthorizeTx(ctx, tx, a, scope, op)
		if e != nil {
			return e
		}
		auth.Revision = revision
		if _, e = tx.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR SHARE`); e != nil {
			return e
		}
		if e = devicecoord.Catalog(ctx, tx, locks.CatalogWrite); e != nil {
			return e
		}
		if scope.Tenant != "" {
			if e = devicecoord.Tenant(ctx, tx, scope.Tenant, locks.TenantWrite); e != nil {
				return e
			}
		}
		return fn(tx, auth)
	})
	s.LogDenial(op, a.ID, err)
	return PublicError(err)
}
func PublicError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			code := "revision_conflict"
			switch {
			case strings.Contains(pg.ConstraintName, "code"):
				code = "code_taken"
			case strings.Contains(pg.ConstraintName, "receipt"):
				code = "request_conflict"
			case strings.Contains(pg.ConstraintName, "source_channel"):
				code = "source_channel_occupied"
			case strings.Contains(pg.ConstraintName, "source_output"):
				code = "source_output_occupied"
			}
			return Problem(code, "编号、修订或关系已被使用，请重新核对。", 409)
		case "23P01":
			code := "revision_conflict"
			switch pg.ConstraintName {
			case "device_installation_period_exclusion":
				code = "installation_overlap"
			case "monitoring_source_channel_exclusion":
				code = "source_channel_occupied"
			case "monitoring_source_output_exclusion":
				code = "source_output_occupied"
			}
			return Problem(code, "所选期间或来源与已有关系冲突。", 409)
		case "23503", "23514":
			return Problem("validation_failed", "操作违反对象归属、固定定义或历史引用规则。", 422)
		default:
			out := Problem("unavailable", "操作暂未完成，请核对原请求收据后重试。", 503)
			out.Cause = err
			return out
		}
	}
	var ee *engineering.Error
	if errors.As(err, &ee) {
		out := Problem(ee.Code, ee.Message, ee.Status)
		for _, i := range ee.Issues {
			out.Issues = append(out.Issues, Issue{i.Path, i.Code, i.Message})
		}
		return out
	}
	return err
}

// ReceiptTx returns only a minimal result, scoped to the same account and the
// original membership identities. The caller has already required the current
// operation and must check target readability before returning it.
func ReceiptTx(ctx context.Context, tx pgx.Tx, a Authorization, request string) (Result, string, string, string, bool, error) {
	var result Result
	var op, hash, kind, target, tm, pm string
	e := tx.QueryRow(ctx, `SELECT result,operation,payload_hash,target_kind,target_id::text,COALESCE(tenant_membership_id::text,''),COALESCE(project_membership_id::text,'') FROM device_operation_receipts WHERE entry_scope=$1 AND tenant_id IS NOT DISTINCT FROM $2::uuid AND project_id IS NOT DISTINCT FROM $3::uuid AND actor_id=$4 AND request_id=$5`, a.Entry, Nullable(a.Tenant), Nullable(a.Project), a.Account, request).Scan(&result, &op, &hash, &kind, &target, &tm, &pm)
	if e == pgx.ErrNoRows {
		return result, "", "", "", false, nil
	}
	if e != nil {
		return result, "", "", "", false, e
	}
	if tm != a.TenantMembership || pm != a.ProjectMembership {
		return result, "", "", "", false, NotFound("device")
	}
	result.Replayed = true
	return result, op, hash, kind + ":" + target, true, nil
}
func Replay(ctx context.Context, tx pgx.Tx, a Authorization, request string, op access.Operation, input any) (Result, bool, error) {
	result, old, hash, _, found, e := ReceiptTx(ctx, tx, a, request)
	if e != nil || !found {
		return result, found, e
	}
	if old != string(op) || hash != Hash(input) {
		return Result{}, false, Problem("request_conflict", "相同请求 ID 已用于其他内容。", 409)
	}
	return result, true, nil
}
func SaveReceipt(ctx context.Context, tx pgx.Tx, a Authorization, m Meta, op access.Operation, input any, kind, target string, result Result) error {
	_, e := tx.Exec(ctx, `INSERT INTO device_operation_receipts(entry_scope,tenant_id,project_id,actor_id,tenant_membership_id,project_membership_id,request_id,operation,payload_hash,target_kind,target_id,result) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, a.Entry, Nullable(a.Tenant), Nullable(a.Project), a.Account, Nullable(a.TenantMembership), Nullable(a.ProjectMembership), m.RequestID, string(op), Hash(input), kind, target, JSON(result))
	return e
}
func Audit(ctx context.Context, tx pgx.Tx, a Authorization, m Meta, operation, kind, target string, result Result, meta access.Meta) error {
	meta.ActorID = a.Account
	meta.TenantID = a.Tenant
	meta.ProjectID = a.Project
	meta.Reason = m.Reason
	meta.RequestID = m.RequestID
	// Security events carry only the minimum identity/result. Domain history owns content.
	return access.Audit(ctx, tx, operation, kind, target, "", nil, result, meta)
}
