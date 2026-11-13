//go:build monitoringintegration

package resources_test

import (
	"testing"

	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/jackc/pgx/v5"
)

func TestDerivedRecoveryRejectsMissingDependencyAndActiveCycle(t *testing.T) {
	f := monitoringSetup(t)
	_, channel, cfg := derivedChannelSetup(t, f)
	must(t, dm.InitializeStorage(ctx, f.storage))
	recovery := dm.NewRecovery(f.storage)
	initial, e := recovery.Reconcile(ctx, f.env.Pool, f.tenant)
	must(t, e)
	if initial.State != "passed" {
		t.Fatal(initial)
	}
	// Simulate a damaged restore, bypassing immutable guards only in this
	// disposable schema. Normal APIs cannot erase or rewrite dependency history.
	mutate := func(fn func(pgx.Tx)) {
		tx, e := f.env.Pool.Begin(ctx)
		must(t, e)
		defer tx.Rollback(ctx)
		fn(tx)
		must(t, tx.Commit(ctx))
	}
	_, e = f.env.Pool.Exec(ctx, `CREATE TABLE recovery_saved_dependency AS SELECT * FROM derived_dependencies WHERE tenant_id=$1 AND config_id=$2`, f.tenant, cfg.ID)
	must(t, e)
	mutate(func(tx pgx.Tx) {
		_, e = tx.Exec(ctx, `ALTER TABLE derived_dependencies DISABLE TRIGGER derived_dependency_immutable`)
		must(t, e)
		_, e = tx.Exec(ctx, `DELETE FROM derived_dependencies WHERE tenant_id=$1 AND config_id=$2`, f.tenant, cfg.ID)
		must(t, e)
		_, e = tx.Exec(ctx, `ALTER TABLE derived_dependencies ENABLE TRIGGER derived_dependency_immutable`)
		must(t, e)
	})
	missing, e := recovery.Reconcile(ctx, f.env.Pool, f.tenant)
	must(t, e)
	if !hasRecoveryFinding(missing, "derived_channel_configs_integrity") {
		t.Fatal("missing reverse dependency passed reconciliation", missing)
	}
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO derived_dependencies SELECT * FROM recovery_saved_dependency; DROP TABLE recovery_saved_dependency`)
	must(t, e)
	mutate(func(tx pgx.Tx) {
		_, e = tx.Exec(ctx, `ALTER TABLE derived_dependencies DISABLE TRIGGER derived_dependency_immutable; ALTER TABLE derived_channel_configs DISABLE TRIGGER derived_configuration_immutable`)
		must(t, e)
		_, e = tx.Exec(ctx, `UPDATE derived_channel_configs SET input_bindings=jsonb_build_object('x',jsonb_build_object('channel_id',$3::text)) WHERE tenant_id=$1 AND id=$2`, f.tenant, cfg.ID, channel.Resource.ID)
		must(t, e)
		_, e = tx.Exec(ctx, `UPDATE derived_dependencies SET input_channel_id=$3,input_kind='derived' WHERE tenant_id=$1 AND config_id=$2`, f.tenant, cfg.ID, channel.Resource.ID)
		must(t, e)
		_, e = tx.Exec(ctx, `ALTER TABLE derived_channel_configs ENABLE TRIGGER derived_configuration_immutable; ALTER TABLE derived_dependencies ENABLE TRIGGER derived_dependency_immutable`)
		must(t, e)
	})
	cycle, e := recovery.Reconcile(ctx, f.env.Pool, f.tenant)
	must(t, e)
	if !hasRecoveryFinding(cycle, "derived_graph_states_integrity") {
		t.Fatal("active cycle passed reconciliation", cycle)
	}
}

func hasRecoveryFinding(report dm.Reconciliation, code string) bool {
	for _, finding := range report.Findings {
		if finding.Code == code {
			return report.State == "failed"
		}
	}
	return false
}
