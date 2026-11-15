package audit_test

import (
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
)

func TestAuditBusinessCommandReceiptsConflictsAndAtomicFailure(t *testing.T) {
	f := setup(t)
	s := f.env.Service.Access()
	m := access.Meta{Reason: "真实业务命令与回滚自验", CommandRequired: true, RequestID: uuid.NewString()}
	_, e := s.CreateRole(ctx, f.actor, "命令角色", "", m)
	if !errors.Is(e, ar.ErrCommandRequired) {
		t.Fatal("missing command header accepted", e)
	}
	m.CommandID = uuid.NewString()
	m.RequestID = uuid.NewString()
	role, e := s.CreateRole(ctx, f.actor, "命令角色", "", m)
	must(t, e)
	m.RequestID = uuid.NewString()
	replay, e := s.CreateRole(ctx, f.actor, "命令角色", "", m)
	must(t, e)
	if replay.ID != role.ID {
		t.Fatal("replayed command created a second role")
	}
	m.RequestID = uuid.NewString()
	_, e = s.CreateRole(ctx, f.actor, "其他意图", "", m)
	if !errors.Is(e, ar.ErrCommandConflict) {
		t.Fatal("same command allowed altered payload", e)
	}
	var success int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events WHERE event_type='role_created' AND object_id=$1`, role.ID).Scan(&success))
	if success != 1 {
		t.Fatal("duplicate successful fact")
	}
	// Two genuine intents share one expected version; graph locking serializes
	// effects and the loser persists a conflict only after its transaction ends.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"并发甲", "并发乙"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			_, e := s.ChangeRole(ctx, f.actor, role.ID, access.OpRoleUpdate, access.RoleChange{ExpectedVersion: role.Version, Name: name}, access.Meta{CommandRequired: true, CommandID: uuid.NewString(), RequestID: uuid.NewString(), Reason: "并发修订自验"})
			results <- e
		}(name)
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for e := range results {
		if e == nil {
			ok++
		} else if errors.Is(e, access.ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal("concurrent intent result", ok, conflict)
	}
	var changes bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_security_events WHERE event_type='audit.command.conflict' AND outcome='conflict' AND changes='[]'::jsonb AND tenant_id IS NULL AND project_id IS NULL)`).Scan(&changes))
	if !changes {
		t.Fatal("conflict receipt is missing or claims applied changes")
	}
	// Rollback the actual role insert if its mandatory audit cannot persist.
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION fail_test_role_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'synthetic audit failure';END$$;CREATE TRIGGER fail_test_role_audit BEFORE INSERT ON account_security_events FOR EACH ROW WHEN(NEW.event_type='role_created') EXECUTE FUNCTION fail_test_role_audit()`)
	must(t, e)
	m.CommandID = uuid.NewString()
	m.RequestID = uuid.NewString()
	_, e = s.CreateRole(ctx, f.actor, "必须原子回滚", "", m)
	if e == nil {
		t.Fatal("business succeeded without audit")
	}
	var exists bool
	must(t, f.env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roles WHERE name='必须原子回滚')`).Scan(&exists))
	if exists {
		t.Fatal("business insert survived audit failure")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_security_events WHERE request_id=$1 AND event_type='audit.command.failure')`, m.RequestID).Scan(&exists))
	if !exists {
		t.Fatal("independent failure fact not persisted")
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER fail_test_role_audit ON account_security_events;CREATE TRIGGER fail_all_test_audit BEFORE INSERT ON account_security_events FOR EACH ROW EXECUTE FUNCTION fail_test_role_audit()`)
	must(t, e)
	before := access.AuditPersistFailures()
	failCtx := access.WithOperation(ctx, s, f.actor.ID, access.OpRoleUpdate, uuid.NewString())
	e = s.Transact(failCtx, func(tx pgx.Tx) error { return access.ErrConflict })
	if !errors.Is(e, access.ErrConflict) || access.AuditPersistFailures() != before+1 {
		t.Fatal("failed audit persistence hid original result or diagnostic")
	}
}
