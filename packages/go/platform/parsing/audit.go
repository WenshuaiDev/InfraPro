package parsing

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func (s *Service) AuditMetadataTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, kind, id string) (string, error) {
	operation := op(scope, "methods", "list")
	if scope.Entry == "project" {
		return "", access.ErrDenied
	}
	if kind == "parsing_runs" || kind == "parsing.test" {
		operation = op(scope, "runs", "detail")
	}
	if kind == "parsing_samples" {
		operation = op(scope, "samples", "detail")
	}
	auth, e := k.AuthorizeTx(ctx, tx, a, scope, operation)
	if e != nil {
		return "", e
	}
	switch kind {
	case "parsing_runs", "parsing.test":
		r, e := runTx(ctx, tx, scope, id)
		if e != nil {
			return "", e
		}
		if e = s.authorizeRunTx(ctx, tx, auth, r, false); e != nil {
			return "", e
		}
		return "样本试解析", nil
	case "parsing_samples":
		_, e := sampleTx(ctx, tx, scope, id)
		return "解析样本", e
	case "parsing_device", "parsing_batch":
		_, e := deviceTx(ctx, tx, auth, id)
		return "设备解析采用", e
	case "parsing_configs", "parsing_device-configs":
		var revision string
		e := tx.QueryRow(ctx, `SELECT id::text FROM parsing_config_revisions WHERE config_id=$1 ORDER BY number DESC LIMIT 1`, id).Scan(&revision)
		if e != nil {
			return "", e
		}
		v, e := configTx(ctx, tx, auth, revision, false)
		return v.Name, e
	case "parsing_defaults":
		_, _, e := productTx(ctx, tx, auth, "", id, false, false)
		return "产品解析默认", e
	case "parsing_catalog", "parsing_methods":
		if scope.Entry != "platform" {
			return "", access.ErrDenied
		}
		return "解析方法目录", nil
	}
	return "", access.ErrDenied
}
