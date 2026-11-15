package devicecatalog

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s *Service) AuditMetadataTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, kind, id string) (string, error) {
	kind = strings.TrimPrefix(kind, "device_")
	if kind != "manufacturer" && kind != "product" {
		return "", access.ErrDenied
	}
	if _, e := k.AuthorizeTx(ctx, tx, a, scope, scope.Operation(kind, "detail")); e != nil {
		return "", e
	}
	if kind == "manufacturer" {
		v, e := manufacturerTx(ctx, tx, scope, id, false)
		return v.Name, e
	}
	v, e := productTx(ctx, tx, scope, id)
	return v.Code, e
}
