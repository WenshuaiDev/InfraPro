package devices

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
)

func outputDefinitions(ctx context.Context, tx pgx.Tx, outputs []*Output) error {
	ids := []string{}
	for _, o := range outputs {
		ids = append(ids, o.MetricVersionID)
		if o.UnitVersionID != nil {
			ids = append(ids, *o.UnitVersionID)
		}
	}
	all, e := engineering.ResolvePublicBatchTx(ctx, tx, ids, false)
	if e != nil {
		return e
	}
	for _, o := range outputs {
		roots := []string{o.MetricVersionID}
		if o.UnitVersionID != nil {
			roots = append(roots, *o.UnitVersionID)
		}
		o.Definitions = engineering.PublicDependencySet(all, roots)
	}
	return nil
}
func outputPageDefinitions(ctx context.Context, tx pgx.Tx, items []Output) error {
	outputs := make([]*Output, 0, len(items))
	for i := range items {
		outputs = append(outputs, &items[i])
	}
	return outputDefinitions(ctx, tx, outputs)
}
