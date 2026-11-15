package identity

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

func NotificationInvitationsTx(ctx context.Context, tx pgx.Tx, account string, ids []string) (map[string]ne.TargetContext, error) {
	out := map[string]ne.TargetContext{}
	if e := access.RequireTx(ctx, tx, account, access.OpInvitationRead); e != nil {
		if e == access.ErrDenied {
			return out, nil
		}
		return nil, e
	}
	rows, e := tx.Query(ctx, `SELECT id::text FROM account_invitations WHERE inviter_id=$1 AND id::text=ANY($2::text[])`, account, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out[id] = ne.TargetContext{Allowed: true, Available: true, State: "used"}
	}
	return out, rows.Err()
}
