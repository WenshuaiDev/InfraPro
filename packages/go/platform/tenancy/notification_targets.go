package tenancy

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

func NotificationInvitationsTx(ctx context.Context, tx pgx.Tx, account, tenant string, eventIDs []string) (map[string]ne.TargetContext, error) {
	out := map[string]ne.TargetContext{}
	if !access.ValidID(tenant) {
		return out, nil
	}
	rows, e := tx.Query(ctx, `SELECT invitation_id::text,is_recipient,inviter_id,inviter_membership_id,
		effective_status,membership_status,accepted_tenant_name
		FROM architecture_notification_invitation_targets($1,$2,$3)`, account, tenant, eventIDs)
	if e != nil {
		return nil, e
	}
	items := []Invitation{}
	for rows.Next() {
		v := Invitation{TenantID: tenant}
		var recipient bool
		var acceptedName *string
		if e := rows.Scan(&v.ID, &recipient, &v.Inviter.ID, &v.InviterMembershipID, &v.Status, &v.MembershipStatus, &acceptedName); e != nil {
			rows.Close()
			return nil, e
		}
		if recipient {
			v.Target.ID = account
		}
		if acceptedName != nil {
			v.Snapshot = &InvitationPreview{Tenant: Summary{Name: *acceptedName}}
		}
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for _, v := range items {
		if v.Target.ID != account {
			_, _, e := access.TenantSnapshotTx(ctx, tx, account, v.TenantID, "")
			if e == access.ErrTenantNotFound || e == access.ErrTenantDisabled {
				continue
			}
			if e != nil {
				return nil, e
			}
			out[v.ID] = ne.TargetContext{Allowed: true, State: v.Status, Available: false, Reason: "invitation_accepted"}
			continue
		}
		inspection, e := inspectRecipientTx(ctx, tx, access.Actor{ID: account}, v)
		if e != nil {
			return nil, e
		}
		allowed := inspection.Preview != nil && (inspection.Status != "accepted" || inspection.Availability != nil && inspection.Availability.CanEnter && inspection.MembershipStatus == "active")
		result := ne.TargetContext{Allowed: allowed, Available: allowed, State: inspection.Status, Reason: inspection.BlockedReason}
		if allowed {
			result.Name = inspection.Preview.Tenant.Name
		}
		if !result.Available {
			result.Reason = "invitation_" + inspection.Status
		}
		out[v.ID] = result
	}
	return out, nil
}
