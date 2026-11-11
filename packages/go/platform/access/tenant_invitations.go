package access

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// InvitationRevocation is used only within an existing exclusive authorization transaction.
// AppointedTenantID narrows AccountID to the target of a direct administrator appointment.
type InvitationRevocation struct {
	AccountID, InviterMembershipID, AppointedTenantID string
	InviterOnly, All                                  bool
}

func InvalidateTenantInvitations(ctx context.Context, tx pgx.Tx, scope InvitationRevocation, m Meta) error {
	rows, e := tx.Query(ctx, `UPDATE tenant_invitations SET
 status=CASE WHEN expires_at<=clock_timestamp() THEN 'expired' ELSE 'invalidated' END,
 closed_at=clock_timestamp(),closed_by=$6::uuid,close_reason=CASE WHEN expires_at<=clock_timestamp() THEN 'expired' ELSE $7 END,version=version+1
 WHERE status='pending' AND ($5 OR (inviter_membership_id::text=$2 AND $2<>'') OR
 ($1<>'' AND (($3='' AND (inviter_account_id::text=$1 OR (NOT $4 AND target_account_id::text=$1))) OR ($3<>'' AND tenant_id::text=$3 AND target_account_id::text=$1))))
 RETURNING id::text,tenant_id::text,target_account_id::text,inviter_account_id::text,inviter_membership_id::text,status,close_reason,expires_at,closed_at`, scope.AccountID, scope.InviterMembershipID, scope.AppointedTenantID, scope.InviterOnly, scope.All, nullID(m.ActorID), m.Reason)
	if e != nil {
		return e
	}
	type event struct {
		id, tenant, status string
		data               map[string]any
	}
	events := []event{}
	for rows.Next() {
		var v event
		var target, inviter, member, reason string
		var expiry, closed any
		if e = rows.Scan(&v.id, &v.tenant, &target, &inviter, &member, &v.status, &reason, &expiry, &closed); e != nil {
			rows.Close()
			return e
		}
		v.data = map[string]any{"status": v.status, "target_account_id": target, "inviter_account_id": inviter, "inviter_membership_id": member, "cause": reason, "expires_at": expiry, "closed_at": closed}
		events = append(events, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range events {
		m.TenantID = v.tenant
		if e = Audit(ctx, tx, "tenant.invitation."+v.status, "tenant_invitation", v.id, "", map[string]string{"status": "pending"}, v.data, m); e != nil {
			return e
		}
	}
	return nil
}
func nullID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
