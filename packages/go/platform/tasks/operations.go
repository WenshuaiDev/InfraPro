package tasks

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"time"
)

func ScopedOperationsTx(ctx context.Context, tx pgx.Tx, a access.Actor, f Filter, module string, read func(context.Context, pgx.Tx, taskrecord.Scope) (string, string, string, error)) ([]Operation, error) {
	scopes, e := scopesTx(ctx, tx, a, f)
	if e != nil {
		return nil, e
	}
	out := []Operation{}
	for _, scope := range scopes {
		allowed := false
		for _, d := range taskrecord.Catalog() {
			if d.Module == module && CanManage(scope.Codes, d, scope.Scope.Kind) {
				allowed = true
			}
		}
		if !allowed {
			continue
		}
		q, r, failed, e := read(ctx, tx, scope.Scope)
		if e != nil {
			return nil, e
		}
		out = append(out, Operation{Source: module, Scope: scope.Scope, ScopeLabel: scope.Name, State: "available", Queued: &q, Running: &r, Failed: &failed, ObservedAt: time.Now().UTC(), Target: module})
	}
	return out, nil
}
