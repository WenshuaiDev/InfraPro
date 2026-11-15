package audit_test

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/google/uuid"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"
)

type observedStore struct {
	audit.BlobStore
	afterPut func(string) error
	get      func(string) (io.ReadCloser, error)
	remove   func(context.Context, string) error
}

func (s observedStore) Delete(c context.Context, k string) error {
	if s.remove != nil {
		return s.remove(c, k)
	}
	return s.BlobStore.Delete(c, k)
}

func (s observedStore) Put(c context.Context, k string, r io.Reader, n int64, m string) error {
	if e := s.BlobStore.Put(c, k, r, n, m); e != nil {
		return e
	}
	if s.afterPut != nil {
		return s.afterPut(k)
	}
	return nil
}
func (s observedStore) Get(c context.Context, k string) (io.ReadCloser, error) {
	if s.get != nil {
		return s.get(k)
	}
	return s.BlobStore.Get(c, k)
}

func TestAuditStorageFailureSnapshotReuseRevocationAndUnknownTransfer(t *testing.T) {
	f := setup(t)
	store := exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	_, e := f.env.Service.Access().CreateRole(ctx, f.actor, "真实来源写后故障样本", "", access.Meta{Reason: "故障恢复验证"})
	must(t, e)
	ex, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	s.WithStorage(observedStore{BlobStore: store, afterPut: func(string) error { return errors.New("synthetic lost storage reply") }})
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	pending, e := s.Export(ctx, f.actor, scope, ex.ID, "1")
	must(t, e)
	if pending.State != "retry_wait" || pending.SnapshotAt == nil {
		t.Fatalf("write failure lost reusable snapshot: %+v", pending)
	}
	var digest string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT snapshot_digest FROM audit_export_generations WHERE export_id=$1`, ex.ID).Scan(&digest))
	_, e = f.env.Pool.Exec(ctx, `UPDATE audit_export_generations SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE export_id=$1`, ex.ID)
	must(t, e)
	s.WithStorage(store)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	done, e := s.Export(ctx, f.actor, scope, ex.ID, "1")
	must(t, e)
	var again string
	var attempts, objects, published int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT snapshot_digest,attempt FROM audit_export_generations WHERE export_id=$1`, ex.ID).Scan(&again, &attempts))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE state='available') FROM audit_export_objects WHERE export_id=$1`, ex.ID).Scan(&objects, &published))
	if done.State != "succeeded" || digest != again || !pending.SnapshotAt.Equal(*done.SnapshotAt) || attempts != 2 || objects != 2 || published != 1 {
		t.Fatal("automatic recovery changed frozen view or duplicated publication", done, attempts, objects, published)
	}
	// Corruption is detected before headers, never replaced by an empty CSV.
	s.WithStorage(observedStore{BlobStore: store, get: func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("corrupt CSV")), nil }})
	_, e = s.OpenRead(ctx, f.actor, scope, ex.ID, "1", "GET", "corrupt-body")
	if e == nil {
		t.Fatal("corrupt result was readable")
	}
	s.WithStorage(store)
	// A newly captured successful write becomes unpublished if permission changes
	// between object IO and the final SQL publication boundary.
	second, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	s.WithStorage(observedStore{BlobStore: store, afterPut: func(string) error {
		_, e := f.env.Pool.Exec(ctx, `UPDATE authorization_state SET revision=revision+1 WHERE id=1`)
		return e
	}})
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	v, e := s.Export(ctx, f.actor, scope, second.ID, "1")
	must(t, e)
	if v.State != "failed" || v.File != nil || v.ErrorCode != "audit_export_invalidated" {
		t.Fatalf("revoked output published: %+v", v)
	}
	// Open transfer abandoned by a process is reconciled without guessed bytes.
	s.WithStorage(store)
	third, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	read, e := s.OpenRead(ctx, f.actor, scope, third.ID, "1", "GET", "abandoned-receipt")
	must(t, e)
	defer read.Close()
	s.WithClock(func() time.Time { return time.Now().Add(21 * time.Minute) })
	n, e := s.ReconcileTransfers(ctx)
	must(t, e)
	if n != 1 {
		t.Fatal("abandoned receipt not reconciled", n)
	}
	var outcome string
	var sent *int64
	must(t, f.env.Pool.QueryRow(ctx, `SELECT outcome,sent_bytes FROM audit_transfer_receipts WHERE request_id='abandoned-receipt'`).Scan(&outcome, &sent))
	if outcome != "interrupted_unknown" || sent != nil {
		t.Fatal(outcome, sent)
	}
}

func TestAuditObjectDeletionTimeoutMissingBodyAndUnknownKey(t *testing.T) {
	f := setup(t)
	store := exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	unknownKey := "audit-exports/unregistered-" + uuid.NewString()
	must(t, store.Put(ctx, unknownKey, strings.NewReader("unregistered"), 12, "text/plain"))
	for _, lostReply := range []bool{false, true} {
		ex, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
		must(t, e)
		_, e = s.RunExportOnce(ctx)
		must(t, e)
		var key string
		must(t, f.env.Pool.QueryRow(ctx, `UPDATE audit_export_objects SET expires_at=clock_timestamp()-interval '1 second' WHERE export_id=$1 RETURNING object_key`, ex.ID).Scan(&key))
		plan, e := s.Plan(ctx, "objects", "删除边界验收", nil)
		must(t, e)
		s.WithStorage(observedStore{BlobStore: store, remove: func(c context.Context, k string) error {
			if lostReply {
				if e := store.Delete(c, k); e != nil {
					return e
				}
			}
			return context.DeadlineExceeded
		}})
		result, e := s.ApplyObjects(ctx, plan.ID, plan.Revision, uuid.NewString(), "模拟删除超时或响应丢失")
		must(t, e)
		if result["failed"] != 1 {
			t.Fatal("delete failure was hidden", result)
		}
		var state string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT state FROM audit_export_objects WHERE export_id=$1`, ex.ID).Scan(&state))
		if state != "delete_pending" {
			t.Fatal("failed deletion returned to readable state", state)
		}
		_, e = s.OpenRead(ctx, f.actor, scope, ex.ID, "1", "HEAD", "delete-pending")
		if e == nil {
			t.Fatal("delete_pending exposed body")
		}
		s.WithStorage(store)
		plan, e = s.Plan(ctx, "objects", "按当前事实重新预览", nil)
		must(t, e)
		result, e = s.ApplyObjects(ctx, plan.ID, plan.Revision, uuid.NewString(), "重试登记对象删除")
		must(t, e)
		if result["deleted"] != 1 {
			t.Fatal("idempotent delete did not reconcile", result)
		}
		_, e = store.Get(ctx, key)
		if !errors.Is(e, fs.ErrNotExist) {
			t.Fatal("registered object survived", e)
		}
	}
	unknown, e := store.Get(ctx, unknownKey)
	must(t, e)
	body, e := io.ReadAll(unknown)
	must(t, e)
	must(t, unknown.Close())
	if string(body) != "unregistered" {
		t.Fatal("cleanup touched an unknown key")
	}
	ex, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	var key string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT object_key FROM audit_export_objects WHERE export_id=$1`, ex.ID).Scan(&key))
	must(t, store.Delete(ctx, key))
	_, e = s.OpenRead(ctx, f.actor, scope, ex.ID, "1", "GET", "body-missing")
	if e == nil {
		t.Fatal("missing body became an empty successful CSV")
	}
	t.Log("real S3 before-delete timeout and lost reply reconciled; missing body refused; unknown key preserved")
}

func TestAuditFiftyThousandRowBoundaryAndMidstreamRevoke(t *testing.T) {
	f := setup(t)
	exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	// Bulk fixture represents already classified historical facts, not mocked
	// adapters. The public export/worker/download flow uses the real database/S3.
	_, e := f.env.Pool.Exec(ctx, `INSERT INTO account_security_events(event_type,object_type,object_id,actor_type,outcome,visibility_class,reason,details) SELECT 'role_created','role',gen_random_uuid()::text,'system','success','platform','边界合成记录',jsonb_build_object('after',jsonb_build_object('name','历史名称 '||n)) FROM generate_series(1,50000) n`)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `ANALYZE account_security_events`)
	must(t, e)
	input := exportInput(t, s, f.actor, scope, audit.Filter{EventType: "role_created"})
	ex, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), input)
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	v, e := s.Export(ctx, f.actor, scope, ex.ID, "1")
	must(t, e)
	if v.State != "succeeded" || v.RowCount == nil || *v.RowCount != "50000" {
		t.Fatalf("50000-row export: %+v", v)
	}
	read, e := s.OpenRead(ctx, f.actor, scope, ex.ID, "1", "GET", "midstream-revoke")
	must(t, e)
	defer read.Close()
	must(t, read.Respond(ctx, func() error { return nil }))
	revoker, e := f.env.Pool.Acquire(ctx)
	must(t, e)
	defer revoker.Release()
	revoked := make(chan error, 1)
	blocks := 0
	e = read.Write(ctx, 0, read.Size, func(b []byte) (int, error) {
		blocks++
		if blocks == 1 {
			go func() {
				_, err := revoker.Exec(ctx, `UPDATE authorization_state SET revision=revision+1 WHERE id=1`)
				revoked <- err
			}()
			// Wait until the revocation is queued behind the currently authorized
			// block, then return it. PostgreSQL grants the writer before a new reader.
			waiting := false
			for end := time.Now().Add(3 * time.Second); time.Now().Before(end); {
				if err := f.env.Pool.QueryRow(ctx, `SELECT COALESCE(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, revoker.Conn().PgConn().PID()).Scan(&waiting); err != nil {
					return 0, err
				}
				if waiting {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if !waiting {
				return 0, errors.New("revocation did not queue at the real graph lock")
			}
		}
		return len(b), nil
	})
	must(t, <-revoked)
	if e == nil || blocks != 1 {
		t.Fatal("revocation released an additional block", blocks, e)
	}
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO account_security_events(event_type,object_type,object_id,actor_type,outcome,visibility_class,reason) VALUES('role_created','role',gen_random_uuid()::text,'system','success','platform','第50001条')`)
	must(t, e)
	input = exportInput(t, s, f.actor, scope, audit.Filter{EventType: "role_created"})
	ex, e = s.CreateExport(ctx, f.actor, scope, uuid.NewString(), input)
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	v, e = s.Export(ctx, f.actor, scope, ex.ID, "1")
	must(t, e)
	if v.State != "failed" || v.ErrorCode != "audit_export_too_large" || v.File != nil {
		t.Fatalf("50001 rows published partial output: %+v", v)
	}
	var rows int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_export_rows WHERE export_id=$1`, ex.ID).Scan(&rows))
	if rows != 0 {
		t.Fatal("oversize snapshot left partial rows", rows)
	}
	t.Log("50000 actual rows succeeded; 50001 rolled back snapshot; revoked GET sent exactly one authorized 64 KiB block")
}

func TestAuditConfiguredByteCeilingHasNoPartialSuccess(t *testing.T) {
	f := setup(t)
	exportStorage(t, f)
	s := f.business.Audit
	o := config.DefaultAudit()
	o.TempDirectory = t.TempDir()
	o.ExportMaxBytes = 128
	s.WithOptions(o)
	scope := audit.Scope{Kind: "platform"}
	ex, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	_, e = s.RunExportOnce(ctx)
	must(t, e)
	v, e := s.Export(ctx, f.actor, scope, ex.ID, "1")
	must(t, e)
	if v.State != "failed" || v.ErrorCode != "audit_export_too_large" || v.File != nil {
		t.Fatalf("CSV byte limit failed open: %+v", v)
	}
	// Time controls never change the host clock.
	s.WithClock(func() time.Time { return time.Now().Add(25 * time.Hour) })
}
