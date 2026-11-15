package audit_test

import (
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestAuditSnapshotExcludesLateCommittedFact(t *testing.T) {
	f := setup(t)
	exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	fact := func(label string) ar.Fact {
		return ar.Fact{Module: "access", Event: "role_created", SourceKey: uuid.NewString(), Effect: "create", Scope: scope, ActorType: "system", ObjectType: "role", ObjectID: uuid.NewString(), Reason: label, Outcome: "success"}
	}
	var first ar.EventRef
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		var e error
		first, e = ar.AppendTx(ctx, tx, fact("已提交的原快照记录"))
		return e
	}))
	late, e := f.env.Pool.Begin(ctx)
	must(t, e)
	defer late.Rollback(ctx)
	delayed, e := ar.AppendTx(ctx, late, fact("快照建立后才提交"))
	must(t, e)
	gate, e := f.env.Pool.Acquire(ctx)
	must(t, e)
	defer gate.Release()
	lock := time.Now().UnixNano() % 2000000000
	_, e = gate.Exec(ctx, `SELECT pg_advisory_lock($1)`, lock)
	must(t, e)
	defer gate.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lock)
	_, e = f.env.Pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION audit_test_snapshot_gate() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(%d); RETURN NEW; END $$; CREATE TRIGGER audit_test_snapshot_gate BEFORE INSERT ON audit_export_rows FOR EACH ROW EXECUTE FUNCTION audit_test_snapshot_gate()`, lock))
	must(t, e)
	input := exportInput(t, s, f.actor, scope, audit.Filter{EventType: "role_created", To: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)})
	ex, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), input)
	must(t, e)
	finished := make(chan error, 1)
	go func() { _, e := s.RunExportOnce(ctx); finished <- e }()
	waiting := false
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		must(t, f.env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=$1::oid AND NOT granted)`, lock).Scan(&waiting))
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("snapshot did not reach the real materialization barrier")
	}
	must(t, late.Commit(ctx))
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error { _, e := ar.AppendTx(ctx, tx, fact("快照期间插入的新事实")); return e }))
	_, e = gate.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lock)
	must(t, e)
	must(t, <-finished)
	result, e := s.Export(ctx, f.actor, scope, ex.ID, "1")
	must(t, e)
	if result.State != "succeeded" || result.RowCount == nil || *result.RowCount != "1" {
		t.Fatalf("export observed multiple database views: %+v", result)
	}
	var id string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT event_id::text FROM audit_export_rows WHERE export_id=$1`, ex.ID).Scan(&id))
	if id != first.ID || id == delayed.ID {
		t.Fatal("late commit entered frozen record set", id)
	}
	t.Log("real PostgreSQL materialization barrier: preexisting committed fact included; late commit and later insertion excluded")
}
