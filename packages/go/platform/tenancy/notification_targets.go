package tenancy

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

func NotificationInvitationsTx(ctx context.Context, tx pgx.Tx, account string, ids []string) (map[string]ne.TargetContext, error) {
	out := map[string]ne.TargetContext{}
	rows, e := tx.Query(ctx, invitationSelect+` WHERE i.id::text=ANY($1::text[]) AND (i.target_account_id=$2 OR i.inviter_account_id=$2 AND i.status='accepted')`, ids, account)
	if e != nil {
		return nil, e
	}
	items := []Invitation{}
	for rows.Next() {
		v, e := scanInvitation(rows)
		if e != nil {
			rows.Close()
			return nil, e
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
