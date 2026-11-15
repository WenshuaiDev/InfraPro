package sources

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func (s *Service) AuditMetadataTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string) (string, error) {
	auth, e := k.AuthorizeTx(ctx, tx, a, scope, scope.Operation("source", "history"))
	if e != nil {
		return "", e
	}
	var target Target
	if e = tx.QueryRow(ctx, `SELECT facility_id::text,point_id::text,channel_id::text FROM monitoring_source_bindings WHERE tenant_id=$1 AND id::text=$2`, scope.Tenant, id).Scan(&target.FacilityID, &target.PointID, &target.ChannelID); e != nil {
		return "", e
	}
	channel, _, e := s.channelTx(ctx, tx, a, scope, target, false)
	if e != nil {
		return "", e
	}
	binding, e := bindingTx(ctx, tx, scope, target, id)
	if e != nil {
		return "", e
	}
	v, e := s.projectBinding(ctx, tx, a, auth, &binding, true)
	if e != nil {
		return "", e
	}
	if v.Visibility != "full" {
		return "", access.ErrDenied
	}
	return channel.Resource.Name, nil
}

func (s *Service) AuditLocationTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string) (map[string]string, error) {
	if _, e := s.AuditMetadataTx(ctx, tx, a, scope, id); e != nil {
		return nil, e
	}
	var facility, point, channel string
	e := tx.QueryRow(ctx, `SELECT facility_id::text,point_id::text,channel_id::text FROM monitoring_source_bindings WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&facility, &point, &channel)
	return map[string]string{"facility": facility, "point": point, "channel": channel}, e
}
