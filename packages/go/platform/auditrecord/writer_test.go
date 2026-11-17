package auditrecord_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ctx = context.Background()

func environment(t *testing.T) *testsupport.Environment {
	t.Helper()
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run pnpm test:audit with real PostgreSQL")
	}
	e, err := testsupport.New(ctx, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.Close)
	_, err = e.Service.Bootstrap(ctx, "AuditAdmin", "审计管理员", "Audit fixture river bridge 2026!", identity.Meta{})
	must(t, err)
	return e
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func sample(key string) ar.Fact {
	return ar.Fact{Event: "audit.export.requested", SourceKey: key, Effect: "requested", Scope: ar.Scope{Kind: "platform"}, ActorType: "system", ObjectType: "audit_export", ObjectID: uuid.NewString(), Reason: "核对审计事实", Changes: []ar.Change{}}
}
func TestAuditSourceIdentityAtomicityAndBounds(t *testing.T) {
	e := environment(t)
	s := e.Service.Access()
	f := sample("source-identity")
	var first ar.EventRef
	must(t, s.Transact(ctx, func(tx pgx.Tx) error { var err error; first, err = ar.AppendTx(ctx, tx, f); return err }))
	var replay ar.EventRef
	must(t, s.Transact(ctx, func(tx pgx.Tx) error { var err error; replay, err = ar.AppendTx(ctx, tx, f); return err }))
	if first != replay {
		t.Fatal("source replay changed immutable identity")
	}
	var before, after string
	must(t, e.Pool.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&before))
	changed := f
	changed.Reason = "different immutable evidence"
	err := s.Transact(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE authorization_state SET revision=revision+1 WHERE id=1`); err != nil {
			return err
		}
		_, err := ar.AppendTx(ctx, tx, changed)
		return err
	})
	if !errors.Is(err, ar.ErrConflict) {
		t.Fatalf("same key different content: %v", err)
	}
	must(t, e.Pool.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&after))
	if before != after {
		t.Fatal("failed fact allowed partial business commit")
	}
	for name, mutate := range map[string]func(*ar.Fact){
		"unregistered":    func(f *ar.Fact) { f.Event = "new.undeclared.event" },
		"request_control": func(f *ar.Fact) { f.RequestID = "evil\nforged-entry" },
		"source_bound":    func(f *ar.Fact) { f.SourceKey = strings.Repeat("s", 241) },
		"change_count":    func(f *ar.Fact) { f.Changes = make([]ar.Change, 129) },
		"link_count":      func(f *ar.Fact) { f.Links = make([]ar.Link, 33) },
		"invalid_actor":   func(f *ar.Fact) { f.ActorType = "arbitrary" },
		"invalid_outcome": func(f *ar.Fact) { f.Outcome = "maybe" },
		"cross_scope":     func(f *ar.Fact) { f.Scope = ar.Scope{Kind: "platform", Tenant: uuid.NewString()} },
		"invalid_change_type": func(f *ar.Fact) {
			f.Changes = []ar.Change{{Key: "name", Type: "html", BeforeState: ar.Recorded, AfterState: ar.Recorded}}
		},
		"self_cause": func(f *ar.Fact) { f.OperationID = uuid.NewString(); f.ParentOperationID = f.OperationID },
	} {
		t.Run(name, func(t *testing.T) {
			bad := sample(name)
			mutate(&bad)
			err := s.Transact(ctx, func(tx pgx.Tx) error { _, err := ar.AppendTx(ctx, tx, bad); return err })
			if err == nil {
				t.Fatal("invalid fact accepted")
			}
		})
	}
	for _, q := range []string{`UPDATE account_security_events SET reason='rewrite' WHERE id=$1`, `DELETE FROM account_security_events WHERE id=$1`} {
		if _, err = e.Pool.Exec(ctx, q, first.ID); err == nil {
			t.Fatal("history mutation accepted")
		}
	}
}

func TestAuditTruncatedValuesAndBoundedCausalChain(t *testing.T) {
	e := environment(t)
	s := e.Service.Access()
	f := sample("truncated-identity")
	f.Changes = []ar.Change{{Key: "name", Type: "text", BeforeState: ar.NotRecorded, AfterState: ar.Recorded, After: strings.Repeat("长", 600) + "A"}}
	must(t, s.Transact(ctx, func(tx pgx.Tx) error { _, err := ar.AppendTx(ctx, tx, f); return err }))
	f.Changes[0].After = strings.Repeat("长", 600) + "B"
	err := s.Transact(ctx, func(tx pgx.Tx) error { _, err := ar.AppendTx(ctx, tx, f); return err })
	if !errors.Is(err, ar.ErrConflict) {
		t.Fatalf("truncation concealed a changed source fact: %v", err)
	}
	var last string
	for i := 0; i < 33; i++ {
		f := sample(uuid.NewString())
		f.OperationID = uuid.NewString()
		f.ParentOperationID = last
		must(t, s.Transact(ctx, func(tx pgx.Tx) error { _, err := ar.AppendTx(ctx, tx, f); return err }))
		last = f.OperationID
	}
	f = sample("too-deep")
	f.ParentOperationID = last
	err = s.Transact(ctx, func(tx pgx.Tx) error { _, err := ar.AppendTx(ctx, tx, f); return err })
	if !errors.Is(err, ar.ErrInvalid) {
		t.Fatalf("unbounded parent chain: %v", err)
	}
}

func TestAuditConcurrentReplayReturnsOneFact(t *testing.T) {
	e := environment(t)
	f := sample("concurrent-source")
	var wg sync.WaitGroup
	results := make(chan ar.EventRef, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var ref ar.EventRef
			err := e.Service.Access().Transact(ctx, func(tx pgx.Tx) error { var err error; ref, err = ar.AppendTx(ctx, tx, f); return err })
			if err != nil {
				failures <- err
			} else {
				results <- ref
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	id := ""
	for r := range results {
		if id == "" {
			id = r.ID
		}
		if id != r.ID {
			t.Fatal("concurrent duplicate facts")
		}
	}
}

func TestAuditAllowListedValuesAndMissingStates(t *testing.T) {
	e := environment(t)
	var ref ar.EventRef
	err := e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		var err error
		ref, err = ar.AppendLegacyTx(ctx, tx, ar.LegacyInput{Event: "role_created", ObjectType: "role", ObjectID: uuid.NewString(), ActorType: "maintenance", Reason: "password=SECRET_REASON https://user:URL_SECRET@example.test/path?token=SECRET_URL", Before: map[string]any{"name": nil, "permissions": []string{}}, After: map[string]any{"name": "中文\"\n=SUM(1,2)", "description": strings.Repeat("长", 600), "password": "SECRET_PASSWORD", "access_token": "SECRET_TOKEN", "body": "SECRET_BODY", "permissions": []string{}, "status": "active"}})
		return err
	})
	must(t, err)
	var stored string
	must(t, e.Pool.QueryRow(ctx, `SELECT row_to_json(e)::text FROM account_security_events e WHERE id=$1`, ref.ID).Scan(&stored))
	for _, secret := range []string{"SECRET_REASON", "URL_SECRET", "SECRET_URL", "SECRET_PASSWORD", "SECRET_TOKEN", "SECRET_BODY"} {
		if strings.Contains(stored, secret) {
			t.Fatalf("forbidden value persisted: %s", secret)
		}
	}
	var changes []ar.Change
	must(t, e.Pool.QueryRow(ctx, `SELECT changes FROM account_security_events WHERE id=$1`, ref.ID).Scan(&changes))
	states := map[string]ar.Change{}
	for _, c := range changes {
		states[c.Key] = c
	}
	if states["name"].BeforeState != ar.Recorded || states["name"].Before != nil || states["status"].BeforeState != ar.NotRecorded || states["description"].AfterState != ar.Truncated {
		t.Fatalf("value states collapsed: %+v", changes)
	}
	snap, err := ar.Snapshot(map[string]any{"permissions": []string{}}, []string{"permissions"})
	must(t, err)
	if string(ar.JSON(snap)) != `{"permissions":[]}` {
		t.Fatal("empty set was not recorded")
	}
}

func TestAuditTopLevelCollectionSummaryCommits(t *testing.T) {
	env := environment(t)
	for _, count := range []int{20, 21, 100} {
		values := make([]map[string]any, count)
		for i := range values {
			values[i] = map[string]any{"id": uuid.NewString(), "name": "工程设备授权"}
		}
		tx, err := env.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := ar.AppendLegacyTx(ctx, tx, ar.LegacyInput{Event: "role_created", ObjectType: "role", ObjectID: uuid.NewString(), ActorType: "maintenance", Reason: "批量授权摘要", Before: []any{}, After: values})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(count, err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var changes []ar.Change
		if err = env.Pool.QueryRow(ctx, `SELECT changes FROM account_security_events WHERE id=$1`, ref.ID).Scan(&changes); err != nil {
			t.Fatal(err)
		}
		if len(changes) != 1 || changes[0].Key != "value" {
			t.Fatal(changes)
		}
		if count > 20 && changes[0].AfterState != ar.Truncated {
			t.Fatal("large collection lost its truncation state", changes)
		}
	}
}

func TestAuditCommandReceiptsAndTombstones(t *testing.T) {
	e := environment(t)
	var actor string
	must(t, e.Pool.QueryRow(ctx, `SELECT id::text FROM accounts WHERE username='AuditAdmin'`).Scan(&actor))
	command := ar.Command{Module: "access", Action: "role_created", ActorID: actor, Scope: ar.Scope{Kind: "platform"}, ID: uuid.NewString(), Payload: map[string]any{"name": "一次命令"}}
	execute := func(c ar.Command) (ar.Receipt, error) {
		var r ar.Receipt
		err := e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
			var err error
			r, err = ar.BeginCommandTx(ctx, tx, c)
			if err != nil || r.Replayed {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE authorization_state SET revision=revision+1 WHERE id=1`); err != nil {
				return err
			}
			return ar.CompleteCommandTx(ctx, tx, r, map[string]string{"id": actor, "token": "NEVER_STORE"}, "applied", actor)
		})
		return r, err
	}
	first, err := execute(command)
	must(t, err)
	again, err := execute(command)
	must(t, err)
	if !again.Replayed || first.OperationID != again.OperationID || strings.Contains(string(again.Response), "NEVER_STORE") {
		t.Fatal("unsafe or unstable command receipt")
	}
	changed := command
	changed.Payload = map[string]any{"name": "另一载荷"}
	_, err = execute(changed)
	if !errors.Is(err, ar.ErrCommandConflict) {
		t.Fatal("different intent reexecuted", err)
	}
	changed = command
	changed.ID = ""
	_, err = execute(changed)
	if !errors.Is(err, ar.ErrCommandRequired) {
		t.Fatal("missing command accepted")
	}
	_, err = e.Pool.Exec(ctx, `UPDATE audit_command_receipts SET safe_response='{}',tombstoned_at=clock_timestamp() WHERE operation_id=$1`, first.OperationID)
	must(t, err)
	_, err = execute(command)
	if !errors.Is(err, ar.ErrReceiptExpired) {
		t.Fatal("expired command executed", err)
	}
	if _, err = e.Pool.Exec(ctx, `DELETE FROM audit_command_receipts WHERE operation_id=$1`, first.OperationID); err == nil {
		t.Fatal("deduplication tombstone removable")
	}
}
