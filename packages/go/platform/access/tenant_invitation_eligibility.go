package access

import (
	"context"
	"github.com/jackc/pgx/v5"
	"slices"
)

// InvitationQualificationPermissions excludes only the temporary tenant restriction.
// It must never be used to authorize an ordinary tenant operation.
func InvitationQualificationPermissions(v TenantAuthorization) []Code {
	if v.AccountStatus != "active" || v.MembershipStatus != "active" {
		return []Code{}
	}
	return v.RestorablePermissions
}
func InvitationGrantEligible(v TenantAuthorization, fixed []Code) bool {
	have := InvitationQualificationPermissions(v)
	if !slices.Contains(have, TenantInvitationCreate) {
		return false
	}
	for _, code := range fixed {
		if !slices.Contains(have, code) {
			return false
		}
	}
	return true
}
func InvitationQualificationTx(ctx context.Context, tx pgx.Tx, account, tenant string) (string, []Code, error) {
	var member string
	e := tx.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2 AND status='active'`, tenant, account).Scan(&member)
	if e == pgx.ErrNoRows {
		return "", nil, ErrTenantNotFound
	}
	if e != nil {
		return "", nil, e
	}
	v, e := TenantAuthorizationTx(ctx, tx, tenant, member, "")
	return member, InvitationQualificationPermissions(v), e
}

// RequireActiveTenantTx is only for callers that already verified a membership or targeted invitation.
func RequireActiveTenantTx(ctx context.Context, tx pgx.Tx, tenant string) error {
	var status string
	if e := tx.QueryRow(ctx, `SELECT status FROM tenants WHERE id=$1`, tenant).Scan(&status); e != nil {
		return e
	}
	if status != "active" {
		return ErrTenantDisabled
	}
	return nil
}

func TenantInvitationFixedCodesTx(ctx context.Context, tx pgx.Tx) ([]Code, error) {
	rows, e := tx.Query(ctx, `SELECT rp.permission_code FROM roles r JOIN role_permissions rp ON rp.role_id=r.id JOIN permission_catalog p ON p.code=rp.permission_code WHERE r.builtin_key=$1 AND r.status='active' AND r.deleted_at IS NULL AND p.lifecycle='active' ORDER BY rp.permission_code`, TenantMemberKey)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Code{}
	for rows.Next() {
		var c Code
		if e = rows.Scan(&c); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
