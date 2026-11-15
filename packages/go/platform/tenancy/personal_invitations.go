package tenancy

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

func invitationNotification(ctx context.Context, tx pgx.Tx, v Invitation, status, actor string, accepted bool) error {
	ids := []string{v.Target.ID}
	if accepted {
		ids = append(ids, v.Inviter.ID)
	}
	return access.NotificationReceiptTx(ctx, tx, "tenancy", "invitation.changed", status, actor, ne.BusinessScope(v.TenantID, ""), ne.Target{Kind: "tenant_invitation", ID: v.ID}, ne.Personal(ids...))
}
func (s *Service) PersonalInvitation(ctx context.Context, a access.Actor, id string, m access.Meta) (InvitationInspection, error) {
	if a.ReadSubject() != nil {
		return InvitationInspection{}, access.ErrDenied
	}
	return s.inspectInvitation(ctx, a, "", id, m)
}
func (s *Service) AcceptPersonalInvitation(ctx context.Context, a access.Actor, id, version string, m access.Meta) (AcceptanceResult, error) {
	if a.ReadSubject() != nil {
		return AcceptanceResult{}, access.ErrDenied
	}
	if !access.ValidID(id) {
		return AcceptanceResult{}, errUnavailable
	}
	return s.AcceptInvitation(ctx, a, AcceptanceInput{Version: version, invitationID: id}, m)
}
func (s *Service) PersonalInvitations(ctx context.Context, a access.Actor, f Filter) (access.Page[InvitationInspection], error) {
	out := access.Page[InvitationInspection]{Items: []InvitationInspection{}}
	if a.ReadSubject() != nil {
		return out, access.ErrDenied
	}
	e := s.transact(ctx, a, access.OpTenantInvitationInspect, "", "", func(tx pgx.Tx, _ string) error {
		at, id, e := membershipFilter(&f)
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, invitationSelect+` WHERE i.target_account_id=$1 AND ($2::timestamptz IS NULL OR (i.created_at,i.id)<($2,$3::uuid)) ORDER BY i.created_at DESC,i.id DESC LIMIT $4`, a.ID, at, nullable(id), f.Limit+1)
		if e != nil {
			return e
		}
		items := []Invitation{}
		for rows.Next() {
			v, e := scanInvitation(rows)
			if e != nil {
				rows.Close()
				return e
			}
			items = append(items, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(items) > f.Limit {
			items = items[:f.Limit]
			last := items[len(items)-1]
			out.NextCursor = cursorFor(last.CreatedAt, last.ID)
		}
		for _, v := range items {
			item, e := inspectRecipientTx(ctx, tx, a, v)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, item)
		}
		return nil
	})
	return out, e
}

// ExpireInvitations persists expiry even when the recipient never opens a page.
// Authorization lock precedes invitation rows, just as in invitation commands.
func (s *Service) ExpireInvitations(ctx context.Context) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT DISTINCT tenant_id::text,target_account_id::text FROM tenant_invitations WHERE status='pending' AND expires_at<=clock_timestamp() ORDER BY tenant_id::text,target_account_id::text LIMIT 100`)
		if e != nil {
			return e
		}
		pairs := [][2]string{}
		for rows.Next() {
			var p [2]string
			if e = rows.Scan(&p[0], &p[1]); e != nil {
				rows.Close()
				return e
			}
			pairs = append(pairs, p)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, p := range pairs {
			if e = expireInvitations(ctx, tx, p[0], p[1], access.Meta{ActorType: "system", TenantID: p[0], Reason: "expired"}); e != nil {
				return e
			}
		}
		return nil
	})
}
