package classification

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s *Service) AuditMetadataTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, kind, id string) (string, error) {
	kind = strings.TrimPrefix(kind, "device_classification_")
	action := "target"
	if kind == "dictionary" {
		action = "dictionary"
	}
	if kind == "release" {
		action = "categories"
	}
	auth, e := k.AuthorizeTx(ctx, tx, a, scope, scope.Operation("classification", action))
	if e != nil {
		return "", e
	}
	switch kind {
	case "dictionary":
		if scope.Entry != "platform" || id != "00000000-0000-4000-8000-000000000001" {
			return "", access.ErrDenied
		}
		return "Device classification dictionary", nil
	case "release":
		r, e := releaseTx(ctx, tx, id)
		return r.Content.DictionaryCode, e
	case "product_version", "device":
		t, e := targetTx(ctx, tx, auth, kind, id, false)
		return t.Name, e
	default:
		return "", access.ErrDenied
	}
}
