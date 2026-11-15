package audit_test

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type auditProcessFixture struct {
	Database string
	Options  config.Audit
	Marker   string
}

func TestAuditWorkerSubprocess(t *testing.T) {
	file := os.Getenv("INFRAPRO_AUDIT_WORKER_PROCESS_FIXTURE")
	if file == "" {
		return
	}
	raw, e := os.ReadFile(file)
	must(t, e)
	var v auditProcessFixture
	must(t, json.Unmarshal(raw, &v))
	pool, e := pgxpool.New(ctx, v.Database)
	must(t, e)
	defer pool.Close()
	store, e := blobstore.Open(v.Options.ObjectEndpoint, v.Options.ObjectAccessKey, v.Options.ObjectSecretKey, v.Options.Bucket, "audit-exports")
	must(t, e)
	blocked := observedStore{BlobStore: store, afterPut: func(string) error {
		if e := os.WriteFile(v.Marker, []byte("object-written"), 0600); e != nil {
			return e
		}
		select {}
	}}
	_, e = runtime.Compose(access.New(pool, slog.Default())).Audit.WithOptions(v.Options).WithStorage(blocked).RunExportOnce(ctx)
	must(t, e)
}
func TestAuditWorkerKilledAndLateFenceCannotPublishTwice(t *testing.T) {
	f := setup(t)
	store := exportStorage(t, f)
	s := f.business.Audit
	scope := audit.Scope{Kind: "platform"}
	in := exportInput(t, s, f.actor, scope, audit.Filter{})
	ex, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), in)
	must(t, e)
	dir := t.TempDir()
	marker := filepath.Join(dir, "written")
	file := filepath.Join(dir, "worker.json")
	raw, e := json.Marshal(auditProcessFixture{f.env.Config.DatabaseURL, f.env.Config.Audit, marker})
	must(t, e)
	must(t, os.WriteFile(file, raw, 0600))
	command := exec.Command(os.Args[0], "-test.run=^TestAuditWorkerSubprocess$", "-test.timeout=2m")
	command.Env = append(os.Environ(), "INFRAPRO_AUDIT_WORKER_PROCESS_FIXTURE="+file)
	must(t, command.Start())
	defer func() { _ = command.Process.Kill() }()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, e = os.Stat(marker); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not reach durable S3 write")
		}
		time.Sleep(10 * time.Millisecond)
	}
	must(t, command.Process.Kill())
	_ = command.Wait()
	var digest string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT snapshot_digest FROM audit_export_generations WHERE export_id=$1`, ex.ID).Scan(&digest))
	if digest == "" {
		t.Fatal("process died without materialized snapshot")
	}
	// The lease timeout is the injected crash boundary, without changing host time.
	_, e = f.env.Pool.Exec(ctx, `UPDATE audit_export_generations SET lease_until=clock_timestamp()-interval '1 second' WHERE export_id=$1`, ex.ID)
	must(t, e)
	restarted := runtime.Compose(f.env.Service.Access()).Audit.WithOptions(f.env.Config.Audit).WithStorage(store)
	_, e = restarted.RunExportOnce(ctx)
	must(t, e)
	v, e := s.Export(ctx, f.actor, scope, ex.ID, "1")
	must(t, e)
	if v.State != "succeeded" {
		t.Fatalf("restarted worker %+v", v)
	}
	var after string
	var attempts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT snapshot_digest,attempt FROM audit_export_generations WHERE export_id=$1`, ex.ID).Scan(&after, &attempts))
	if after != digest || attempts != 2 {
		t.Fatal("restart replaced snapshot or generation", attempts)
	}
	// Let an expired process return after a new owner has already published.
	next, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
	must(t, e)
	old := runtime.Compose(f.env.Service.Access()).Audit.WithOptions(f.env.Config.Audit).WithStorage(observedStore{BlobStore: store, afterPut: func(string) error {
		_, e := f.env.Pool.Exec(ctx, `UPDATE audit_export_generations SET lease_until=clock_timestamp()-interval '1 second' WHERE export_id=$1`, next.ID)
		if e != nil {
			return e
		}
		_, e = restarted.RunExportOnce(context.Background())
		return e
	}})
	_, _ = old.RunExportOnce(ctx)
	var outputs, terminal int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_export_objects WHERE export_id=$1 AND state='available'),(SELECT count(*) FROM account_security_events WHERE object_id=$1::text AND event_type='audit.export.succeeded')`, next.ID).Scan(&outputs, &terminal))
	if outputs != 1 || terminal != 1 {
		t.Fatal("expired fence duplicated publication", outputs, terminal)
	}
	t.Log("killed actual worker process after S3 write; restart reuses digest in attempt 2; late old fence cannot republish")
}
