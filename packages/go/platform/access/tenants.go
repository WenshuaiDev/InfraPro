package access

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
)

var ErrTenantDisabled = &Error{Code: "tenant_disabled", Message: "该租户已停用，暂时无法使用。请查看租户状态或联系维护人员。", Status: 403}

var ErrTenantNotFound = &Error{Code: "tenant_not_found", Message: "当前租户不可访问。", Status: 404}

// TenantSnapshotTx deliberately never consults account_roles. Caller holds the authorization lock.
func TenantSnapshotTx(ctx context.Context, tx pgx.Tx, account, tenant, revision string) (string, Snapshot, error) {
	v := Snapshot{Revision: revision, Permissions: []Code{}, Roles: []Role{}, Sources: map[Code][]string{}}
	if !ValidID(tenant) {
		return "", v, ErrTenantNotFound
	}
	var member string
	e := tx.QueryRow(ctx, `SELECT m.id::text FROM tenant_memberships m JOIN accounts a ON a.id=m.account_id WHERE m.tenant_id=$1 AND m.account_id=$2 AND m.status='active' AND a.status='active'`, tenant, account).Scan(&member)
	if e == pgx.ErrNoRows {
		return "", v, ErrTenantNotFound
	}
	if e != nil {
		return "", v, e
	}
	auth, e := TenantAuthorizationTx(ctx, tx, tenant, member, revision)
	if e != nil {
		return "", v, e
	}
	if auth.TenantStatus != "active" {
		return "", v, ErrTenantDisabled
	}
	return member, auth.Snapshot, nil
}
func RequireScopedTx(ctx context.Context, tx pgx.Tx, account string, op Operation, tenant string) error {
	p, ok := PolicyFor(op)
	if !ok {
		return ErrDenied
	}
	if p.Class == "tenant" {
		_, v, e := TenantSnapshotTx(ctx, tx, account, tenant, "")
		if e != nil {
			return e
		}
		for _, code := range p.Permissions {
			if !slices.Contains(v.Permissions, code) {
				return ErrDenied
			}
		}
		return nil
	}
	if op == OpTenantAvailability || op == OpMyTenants || op == OpTenantInvitationInspect || op == OpTenantInvitationAccept {
		return nil
	}
	return RequireTx(ctx, tx, account, op)
}
func (s *Service) AuthorizeScoped(ctx context.Context, id string, op Operation, tenant string) (string, error) {
	var rev string
	e := s.Transact(ctx, func(tx pgx.Tx) error {
		var e error
		rev, e = Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		return RequireScopedTx(ctx, tx, id, op, tenant)
	})
	s.LogDenial(op, id, e)
	return rev, e
}
func ProtectTenantAdministrator(ctx context.Context, tx pgx.Tx, id string) error {
	if e := ProtectProjectAdministrator(ctx, tx, id, ""); e != nil {
		return e
	}
	var n int
	e := tx.QueryRow(ctx, `SELECT count(*) FROM tenants t JOIN tenant_memberships m ON m.id=t.administrator_membership_id AND m.tenant_id=t.id WHERE m.account_id=$1 AND m.status='active' AND EXISTS(SELECT 1 FROM tenant_member_roles mr JOIN roles r ON r.id=mr.role_id JOIN role_permissions rp ON rp.role_id=r.id WHERE mr.membership_id=m.id AND mr.tenant_id=t.id AND mr.revoked_at IS NULL AND r.status='active' AND r.deleted_at IS NULL AND rp.permission_code=$2)`, id, TenantProfileUpdate).Scan(&n)
	if e != nil {
		return e
	}
	if n > 0 {
		return &Error{Code: "tenant_admin_transfer_required", Message: fmt.Sprintf("请先交接该账号负责的 %d 个租户，再停用账号。", n), Status: 409}
	}
	return nil
}
func AuditTenantAvailability(ctx context.Context, tx pgx.Tx, id string, enabled bool, m Meta) error {
	if e := AuditProjectAvailability(ctx, tx, id, enabled, m); e != nil {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT t.id::text FROM tenants t JOIN tenant_memberships tm ON tm.id=t.administrator_membership_id WHERE tm.account_id=$1 AND tm.status='active' ORDER BY t.id`, id)
	if e != nil {
		return e
	}
	ids, e := readIDs(rows)
	if e != nil {
		return e
	}
	event := "tenant.administrator.unavailable"
	if enabled {
		event = "tenant.administrator.available"
	}
	for _, tid := range ids {
		var name, code, user, display string
		if e = tx.QueryRow(ctx, `SELECT t.name,t.code,a.username,a.display_name FROM tenants t CROSS JOIN accounts a WHERE t.id=$1 AND a.id=$2`, tid, id).Scan(&name, &code, &user, &display); e != nil {
			return e
		}
		m.TenantID = tid
		snapshot := map[string]any{"tenant_name": name, "tenant_code": code, "account_id": id, "username": user, "display_name": display, "available": enabled}
		if e = Audit(ctx, tx, event, "tenant", tid, "", nil, snapshot, m); e != nil {
			return e
		}
	}
	return nil
}
func CheckTenantStructure(ctx context.Context, tx pgx.Tx) error {
	var broken bool
	e := tx.QueryRow(ctx, `SELECT NOT tenant_roles_valid()`).Scan(&broken)
	if e != nil {
		return e
	}
	if broken {
		return ErrCatalog
	}
	return nil
}
