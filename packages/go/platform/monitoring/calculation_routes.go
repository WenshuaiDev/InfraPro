package monitoring

import (
	"context"
	"github.com/jackc/pgx/v5"
)

type CalculationRoute struct {
	Physical, Revision string
	Logical            uint16
}

func (s *Service) CalculationRouteTx(ctx context.Context, tx pgx.Tx, tenant, series string) (CalculationRoute, error) {
	var out CalculationRoute
	var e error
	out.Logical, e = CalculationLogicalShard(tenant, series)
	if e != nil {
		return out, e
	}
	if e = tx.QueryRow(ctx, `SELECT route_revision::text FROM monitoring_tenant_states WHERE tenant_id=$1`, tenant).Scan(&out.Revision); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO monitoring_routes(tenant_id,logical_shard,revision,physical_shard,state) SELECT $1,n,$2,'local','active' FROM generate_series(0,255) n ON CONFLICT DO NOTHING`, tenant, out.Revision); e != nil {
		return out, e
	}
	var state string
	if e = tx.QueryRow(ctx, `SELECT physical_shard,state FROM monitoring_routes WHERE tenant_id=$1 AND logical_shard=$2 AND revision=$3`, tenant, out.Logical, out.Revision).Scan(&out.Physical, &state); e != nil {
		return out, e
	}
	if state != "active" {
		return out, problem("route_paused", "此逻辑分片当前不能写入新结果。", 409)
	}
	return out, nil
}
