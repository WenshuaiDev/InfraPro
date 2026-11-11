package tenancy

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

type Options struct {
	ConsoleURL      string
	Limit           func(context.Context, string, string, int) error
	CredentialLimit func(context.Context, string, string, string) error
}
type Service struct {
	access  *access.Service
	options Options
}

func New(a *access.Service, options ...Options) *Service {
	s := &Service{access: a}
	if len(options) > 0 {
		s.options = options[0]
	}
	return s
}

// All entry points revalidate the operation and the live session in the business transaction.
func (s *Service) transact(ctx context.Context, a access.Actor, op access.Operation, tenant, target string, fn func(pgx.Tx, string) error) error {
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		p, ok := access.PolicyFor(op)
		if !ok || a.Check == nil {
			return access.ErrDenied
		}
		rev, e := access.Lock(ctx, tx, p.ChangesGraph)
		if e != nil {
			return e
		}
		credential := op == access.OpTenantInvitationInspect || op == access.OpTenantInvitationAccept
		if target != "" && !credential && !access.ValidID(target) {
			return errAdmin
		}
		// Graph writers discover the old account only after obtaining the exclusive graph lock.
		rows, e := tx.Query(ctx, `SELECT id::text FROM accounts WHERE id::text=$1 OR id::text=$2
 OR id IN(SELECT account_id FROM tenant_memberships WHERE tenant_id::text=$3 AND id::text=$2)
 OR id IN(SELECT unnest(ARRAY[target_account_id,inviter_account_id]) FROM tenant_invitations WHERE (tenant_id::text=$3 AND id::text=$2) OR ($4 AND token_hash=$2))
 OR ($3<>'' AND id IN(SELECT m.account_id FROM tenants t JOIN tenant_memberships m ON m.id=t.administrator_membership_id WHERE t.id::text=$3)) ORDER BY id FOR UPDATE`, a.ID, target, func() string {
			if p.ChangesGraph {
				return tenant
			}
			return ""
		}(), credential)
		if e != nil {
			return e
		}
		for rows.Next() {
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if e = a.Check(ctx, tx, p.Recent); e != nil {
			return e
		}
		if e = access.RequireScopedTx(ctx, tx, a.ID, op, tenant); e != nil {
			return e
		}
		return fn(tx, rev)
	})
	s.access.LogDenial(op, a.ID, e)
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "23505":
			switch pe.ConstraintName {
			case "tenants_code_key", "tenant_codes_pkey":
				return errCode
			case "tenant_one_current_member":
				return problem("member_already_exists", "该账号已有当前成员关系。", 409)
			case "tenant_invitation_pending":
				return errPending
			default:
				return access.ErrConflict
			}
		case "55P03", "57014", "40P01":
			return problem("operation_busy", "操作繁忙，请稍后核实结果后重试。", 503)
		default:
			if strings.HasPrefix(pe.Code, "08") || pe.Code == "57P01" || pe.Code == "57P02" || pe.Code == "57P03" {
				return access.ErrCatalog
			}
			return problem("internal_error", "操作未完成，请通过请求编号联系维护人员。", 500)
		}
	}
	return e
}
func meta(a access.Actor, m access.Meta, tenant, reason, scope string) access.Meta {
	m.ActorID = a.ID
	m.TenantID = tenant
	m.Reason = strings.TrimSpace(reason)
	m.ActorScope = scope
	return m
}
func result(ctx context.Context, tx pgx.Tx, id string, replayed bool) (Result, error) {
	v := Result{TenantID: id, Replayed: replayed}
	e := tx.QueryRow(ctx, `SELECT t.version::text,s.revision::text FROM tenants t CROSS JOIN authorization_state s WHERE t.id=$1 AND s.id=1`, id).Scan(&v.Version, &v.Revision)
	return v, e
}
func activeAccount(ctx context.Context, tx pgx.Tx, id string) error {
	var active bool
	e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1`, id).Scan(&active)
	if e == pgx.ErrNoRows || e == nil && !active {
		return errAdmin
	}
	return e
}
func bind(ctx context.Context, tx pgx.Tx, tenant, member, account, actor string) error {
	if _, e := tx.Exec(ctx, `INSERT INTO tenant_memberships(id,tenant_id,account_id,joined_by,status_changed_by) VALUES($1,$2,$3,$4,$4)`, member, tenant, account, actor); e != nil {
		return e
	}
	var role string
	if e := tx.QueryRow(ctx, `SELECT id::text FROM roles WHERE builtin_key=$1 AND binding_scope='tenant' AND status='active' AND deleted_at IS NULL`, access.TenantAdminKey).Scan(&role); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) VALUES($1,$2,$3,$4)`, tenant, member, role, actor)
	return e
}
func reserve(ctx context.Context, tx pgx.Tx, id, code string) error {
	var owner string
	e := tx.QueryRow(ctx, `INSERT INTO tenant_codes(code,tenant_id) VALUES($1,$2) ON CONFLICT(code) DO UPDATE SET code=excluded.code RETURNING tenant_id::text`, code, id).Scan(&owner)
	if e != nil {
		return e
	}
	if owner != id {
		return errCode
	}
	return nil
}
func (s *Service) Create(ctx context.Context, a access.Actor, in CreateInput, m access.Meta) (Result, error) {
	var out Result
	e := s.transact(ctx, a, access.OpTenantCreate, "", in.AdministratorID, func(tx pgx.Tx, _ string) error {
		if e := in.Fields.normalize(); e != nil {
			return e
		}
		in.Reason = strings.TrimSpace(in.Reason)
		if e := access.ValidReason(in.Reason); e != nil {
			return e
		}
		if !access.ValidID(in.RequestID) {
			return access.Validation("client_request_id", "请求编号需为 UUID。")
		}
		in.AdministratorID = strings.ToLower(in.AdministratorID)
		in.RequestID = strings.ToLower(in.RequestID)
		payload, _ := json.Marshal(in)
		sum := sha256.Sum256(payload)
		hash := hex.EncodeToString(sum[:])
		var id, oldHash string
		e := tx.QueryRow(ctx, `SELECT id::text,creation_payload_hash FROM tenants WHERE created_by=$1 AND creation_request_id=$2`, a.ID, in.RequestID).Scan(&id, &oldHash)
		if e == nil {
			if oldHash != hash {
				return problem("idempotency_conflict", "相同请求编号的内容已改变，请查验原结果或明确重新创建。", 409)
			}
			out, e = result(ctx, tx, id, true)
			return e
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if e = activeAccount(ctx, tx, in.AdministratorID); e != nil {
			return e
		}
		var member string
		if e = tx.QueryRow(ctx, `SELECT gen_random_uuid()::text,gen_random_uuid()::text`).Scan(&id, &member); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO tenants(id,name,code,description,administrator_membership_id,created_by,updated_by,creation_request_id,creation_payload_hash) VALUES($1,$2,$3,$4,$5,$6,$6,$7,$8)`, id, in.Name, in.Code, in.Description, member, a.ID, in.RequestID, hash)
		if e != nil {
			return e
		}
		if e = reserve(ctx, tx, id, in.Code); e != nil {
			return e
		}
		if e = bind(ctx, tx, id, member, in.AdministratorID, a.ID); e != nil {
			return e
		}
		if e = access.Bump(ctx, tx); e != nil {
			return e
		}
		v, e := tenantTx(ctx, tx, id, false)
		if e != nil {
			return e
		}
		if e = access.Audit(ctx, tx, "tenant.created", "tenant", id, "", nil, v, meta(a, m, id, in.Reason, "platform")); e != nil {
			return e
		}
		out, e = result(ctx, tx, id, false)
		return e
	})
	return out, e
}
func (s *Service) Update(ctx context.Context, a access.Actor, id string, in UpdateInput, platform bool, m access.Meta) (Result, error) {
	id = strings.ToLower(id)
	var out Result
	op := access.OpTenantProfileUpdate
	scope := "tenant"
	if platform {
		op = access.OpTenantUpdate
		scope = "platform"
	}
	e := s.transact(ctx, a, op, id, "", func(tx pgx.Tx, _ string) error {
		old, e := tenantTx(ctx, tx, id, true)
		if e != nil {
			return e
		}
		if e = version(old.Version, in.Version); e != nil {
			return e
		}
		if in.Name == nil && in.Code == nil && in.Description == nil {
			return access.Validation("request", "请至少提交一个需要修改的业务字段。")
		}
		fields := Fields{Name: old.Name, Code: old.Code, Description: old.Description}
		if in.Name != nil {
			fields.Name = *in.Name
		}
		if in.Code != nil {
			fields.Code = *in.Code
		}
		if in.Description != nil {
			fields.Description = *in.Description
		}
		if e = fields.normalize(); e != nil {
			return e
		}
		if e = access.ValidReason(in.Reason); e != nil {
			return e
		}
		if old.Name == fields.Name && old.Code == fields.Code && old.Description == fields.Description {
			out, e = result(ctx, tx, id, false)
			return e
		}
		if e = reserve(ctx, tx, id, fields.Code); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE tenants SET name=$2,code=$3,description=$4,version=version+1,updated_by=$5,updated_at=now() WHERE id=$1`, id, fields.Name, fields.Code, fields.Description, a.ID); e != nil {
			return e
		}
		v, e := tenantTx(ctx, tx, id, false)
		if e != nil {
			return e
		}
		if e = access.Audit(ctx, tx, "tenant.profile.updated", "tenant", id, "", old, v, meta(a, m, id, in.Reason, scope)); e != nil {
			return e
		}
		out, e = result(ctx, tx, id, false)
		return e
	})
	return out, e
}
func (s *Service) Transfer(ctx context.Context, a access.Actor, id string, in TransferInput, m access.Meta) (Result, error) {
	id = strings.ToLower(id)
	var out Result
	e := s.transact(ctx, a, access.OpTenantTransfer, id, in.AdministratorID, func(tx pgx.Tx, _ string) error {
		old, e := tenantTx(ctx, tx, id, true)
		if e != nil {
			return e
		}
		if e = version(old.Version, in.Version); e != nil {
			return e
		}
		if e = access.ValidReason(in.Reason); e != nil {
			return e
		}
		if strings.EqualFold(old.Administrator.ID, in.AdministratorID) {
			return problem("administrator_unchanged", "接任账号就是当前管理员，无需交接。", 409)
		}
		if e = activeAccount(ctx, tx, in.AdministratorID); e != nil {
			return e
		}
		var member, status string
		e = tx.QueryRow(ctx, `SELECT id::text,status FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2 AND status IN ('active','suspended')`, id, in.AdministratorID).Scan(&member, &status)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		if status == "suspended" {
			return problem("administrator_membership_suspended", "接任成员已暂停，请先恢复或另选账号。", 409)
		}
		reused := member != ""
		if e = access.RepairProjectParentTx(ctx, tx, old.MembershipID, in.ProjectPreviewHash, in.ConfirmProjectImpact, meta(a, m, id, in.Reason, "platform")); e != nil {
			return e
		}
		if e = changeMemberTx(ctx, tx, id, old.MembershipID, "ended", a.ID, strings.TrimSpace(in.Reason)); e != nil {
			return e
		}
		if e = access.InvalidateTenantInvitations(ctx, tx, access.InvitationRevocation{InviterMembershipID: old.MembershipID}, meta(a, m, id, "administrator_transferred", "platform")); e != nil {
			return e
		}
		if reused {
			if _, e = tx.Exec(ctx, `UPDATE tenant_member_roles SET revoked_at=clock_timestamp(),revoked_by=$2 WHERE membership_id=$1 AND revoked_at IS NULL`, member, a.ID); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) SELECT $1,$2,id,$3 FROM roles WHERE builtin_key=$4`, id, member, a.ID, access.TenantAdminKey); e != nil {
				return e
			}
			if e = changeMemberTx(ctx, tx, id, member, "active", a.ID, strings.TrimSpace(in.Reason)); e != nil {
				return e
			}
		} else {
			if e = tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&member); e != nil {
				return e
			}
			if e = bind(ctx, tx, id, member, in.AdministratorID, a.ID); e != nil {
				return e
			}
		}
		if e = access.InvalidateTenantInvitations(ctx, tx, access.InvitationRevocation{AccountID: in.AdministratorID, AppointedTenantID: id}, meta(a, m, id, "administrator_appointed", "platform")); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE tenants SET administrator_membership_id=$2,version=version+1,updated_by=$3,updated_at=now() WHERE id=$1`, id, member, a.ID); e != nil {
			return e
		}
		if e = access.Bump(ctx, tx); e != nil {
			return e
		}
		v, e := tenantTx(ctx, tx, id, false)
		if e != nil {
			return e
		}
		if e = access.Audit(ctx, tx, "tenant.administrator.transferred", "tenant", id, "", old, map[string]any{"tenant": v, "membership_reused": reused}, meta(a, m, id, in.Reason, "platform")); e != nil {
			return e
		}
		out, e = result(ctx, tx, id, false)
		return e
	})
	return out, e
}

// PreviewTransfer exposes only the governance impact of ending the old tenant relation.
func (s *Service) PreviewTransfer(ctx context.Context, a access.Actor, id string) (map[string]any, error) {
	var out map[string]any
	e := s.transact(ctx, a, access.OpTenantTransferPreview, id, "", func(tx pgx.Tx, _ string) error {
		old, e := tenantTx(ctx, tx, id, false)
		if e != nil {
			return e
		}
		impacts, hash, e := access.ProjectParentImpactTx(ctx, tx, old.MembershipID)
		if e != nil {
			return e
		}
		var disabled bool
		if e = tx.QueryRow(ctx, `SELECT status='disabled' FROM accounts WHERE id=$1`, old.Administrator.ID).Scan(&disabled); e != nil {
			return e
		}
		out = map[string]any{"projects": impacts, "project_preview_hash": hash, "requires_confirmation": disabled && len(impacts) > 0, "expected_version": old.Version, "administrator_disabled": disabled}
		return nil
	})
	return out, e
}
