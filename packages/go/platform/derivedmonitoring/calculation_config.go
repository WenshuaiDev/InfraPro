package derivedmonitoring

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Both immutable execution configurations use the same numerical and fixed
// baseline contracts; their ownership and public permissions remain separate.
func calculationConfigKeyTx(ctx context.Context, tx pgx.Tx, tenant, id string) (ChannelConfig, string, error) {
	var kind string
	e := tx.QueryRow(ctx, `SELECT kind FROM derived_calculation_config_keys WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&kind)
	if e != nil {
		return ChannelConfig{}, "", e
	}
	if kind == "channel" {
		cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND id=$2`, tenant, id))
		return cfg, kind, e
	}
	v, e := scanAnalysisVersion(tx.QueryRow(ctx, analysisVersionSelect+` WHERE v.tenant_id=$1 AND v.id=$2`, tenant, id))
	return analysisCalculationConfig(v), kind, e
}
