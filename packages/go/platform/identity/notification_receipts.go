package identity

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

func securityReceipt(ctx context.Context, tx pgx.Tx, event, actor, account, object string, m Meta, fields ...map[string]any) error {
	if e := audit(ctx, tx, event, actor, account, object, "success", m, fields...); e != nil {
		return e
	}
	if event == "reauthentication" {
		return nil
	}
	return access.NotificationReceiptTx(ctx, tx, "identity", "security.changed", event, actor, ne.Scope{Kind: "account"}, ne.Target{Kind: "security"}, ne.Personal(account))
}
