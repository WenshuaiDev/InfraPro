package derivedmonitoring_test

import (
	"errors"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func monitoringCode(t *testing.T, e error, want string) {
	t.Helper()
	var p *monitoring.Error
	if !errors.As(e, &p) || p.Code != want {
		t.Fatalf("want %s, got %v", want, e)
	}
}
func TestRawSnapshotEvidenceAndRelevantChanges(t *testing.T) {
	f := channelsSetup(t)
	s := monitoring.New(f.e.Service.Access())
	reader := s.RawReader()
	scope := monitoring.TenantScope(f.scope.Tenant)
	actor := f.actor(f.admin)
	lo, hi := int64(100), int64(200)
	snapshot, e := reader.FreezeUser(ctx, actor, scope, monitoring.RawFreezeRequest{Needs: []monitoring.RawNeed{{Channel: f.direct, From: &lo, To: &hi}}})
	must(t, e)
	if snapshot.SourcePosition != "0" || snapshot.Hash == "" {
		t.Fatal(snapshot)
	}
	change := func(kind string, from, to int64) {
		t.Helper()
		must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			var position int64
			if e := tx.QueryRow(ctx, `INSERT INTO monitoring_tenant_states(tenant_id,commit_sequence) VALUES($1,1) ON CONFLICT(tenant_id) DO UPDATE SET commit_sequence=monitoring_tenant_states.commit_sequence+1 RETURNING commit_sequence`, scope.Tenant).Scan(&position); e != nil {
				return e
			}
			_, e := monitoring.AppendChangeTx(ctx, tx, scope.Tenant, position, kind, "direct", uuid.NewString(), []monitoring.ChangeRange{{Channel: f.direct, From: &from, To: &to}})
			return e
		}))
	}
	change("raw_publish", 200, 300)
	state, e := reader.CheckUser(ctx, actor, scope, snapshot.ID, false)
	must(t, e)
	if state.Changed || state.Revoked {
		t.Fatal("unrelated half-open append invalidated snapshot", state)
	}
	change("raw_publish", 150, 151)
	state, e = reader.CheckUser(ctx, actor, scope, snapshot.ID, false)
	must(t, e)
	if !state.Changed || state.Revoked || state.SourcePosition != "2" {
		t.Fatal(state)
	}
	change("raw_revoke", 300, 400)
	_, e = reader.CheckUser(ctx, actor, scope, snapshot.ID, false)
	must(t, e)
	change("raw_revoke", 199, 200)
	_, e = reader.CheckUser(ctx, actor, scope, snapshot.ID, false)
	monitoringCode(t, e, "snapshot_stale")
	state, e = reader.CheckUser(ctx, actor, scope, snapshot.ID, true)
	must(t, e)
	if !state.Revoked {
		t.Fatal(state)
	}
	// A caller cannot manufacture a system mandate, and platform membership is
	// not a fallback for the tenant's monitoring data.
	_, e = reader.FreezeSystem(ctx, monitoring.RawSystemPermit{Tenant: scope.Tenant, RunID: uuid.NewString(), MandateRevision: "1"}, monitoring.RawFreezeRequest{Needs: []monitoring.RawNeed{{Channel: f.direct}}})
	if e == nil {
		t.Fatal("client-shaped system permit acquired authority")
	}
	_, e = reader.CheckUser(ctx, f.actor(f.platform), scope, snapshot.ID, true)
	if e == nil {
		t.Fatal("platform read tenant snapshot")
	}
	_, e = f.e.Pool.Exec(ctx, `UPDATE monitoring_raw_snapshots SET source_position=10 WHERE id=$1`, snapshot.ID)
	if e == nil {
		t.Fatal("snapshot evidence was mutable")
	}
	_, e = f.e.Pool.Exec(ctx, `DELETE FROM monitoring_changes WHERE tenant_id=$1`, scope.Tenant)
	if e == nil {
		t.Fatal("source event evidence was deletable")
	}
	// Integrity quarantine is a transport failure, never a successful empty scan.
	_, e = f.e.Pool.Exec(ctx, `INSERT INTO monitoring_channel_versions(tenant_id,channel_id,quarantined) VALUES($1,$2,true)`, scope.Tenant, f.direct)
	must(t, e)
	_, e = reader.CheckUser(ctx, actor, scope, snapshot.ID, true)
	monitoringCode(t, e, "monitoring_integrity_unavailable")
}
