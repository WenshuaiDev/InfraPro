package display

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

func (s *Service) NotificationTargetsTx(ctx context.Context, tx pgx.Tx, account string, scope Scope, ids []string) (map[string]ne.TargetContext, error) {
	out := map[string]ne.TargetContext{}
	auth, e := access.ProjectSnapshotTx(ctx, tx, account, scope.Tenant, scope.Project, "")
	if e != nil {
		var ae *access.Error
		if errors.As(e, &ae) && (ae.Status == 403 || ae.Status == 404) {
			return out, nil
		}
		return nil, e
	}
	if !manager(auth) {
		return out, nil
	}
	rows, e := tx.Query(ctx, `SELECT id::text,name,state FROM display_instances WHERE tenant_id=$1 AND project_id=$2 AND id::text=ANY($3::text[])`, scope.Tenant, scope.Project, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, state string
		if e = rows.Scan(&id, &name, &state); e != nil {
			return nil, e
		}
		out[id] = ne.TargetContext{Allowed: true, Available: true, Name: name, State: state}
	}
	return out, rows.Err()
}
