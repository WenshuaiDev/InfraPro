package audit_test

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"testing"
	"time"
)

func rolePool(t *testing.T, f fixture, role string) *pgxpool.Pool {
	t.Helper()
	c, e := pgxpool.ParseConfig(f.env.Config.DatabaseURL)
	must(t, e)
	c.AfterConnect = func(ctx context.Context, c *pgx.Conn) error { _, e := c.Exec(ctx, "SET ROLE "+role); return e }
	p, e := pgxpool.NewWithConfig(ctx, c)
	must(t, e)
	t.Cleanup(p.Close)
	must(t, p.Ping(ctx))
	return p
}
func TestAuditNamedRolesRunAndDenyUnrelatedData(t *testing.T) {
	f := setup(t)
	store := exportStorage(t, f)
	s := f.business.Audit
	tenant := f.tenant(t, "audit-role-tenant")
	project := f.project(t, tenant, "audit-role-project")
	opts := config.DefaultAudit()
	opts.TempDirectory = t.TempDir()
	opts.NodeID = "restricted-audit-test"
	pool := rolePool(t, f, "infrapro_audit_worker")
	worker := runtime.Compose(access.New(pool, slog.Default())).Audit.WithOptions(opts).WithStorage(store)
	for _, scope := range []audit.Scope{{Kind: "platform"}, {Kind: "tenant", Tenant: tenant}, {Kind: "project", Tenant: tenant, Project: project}} {
		ex, e := s.CreateExport(ctx, f.actor, scope, uuid.NewString(), exportInput(t, s, f.actor, scope, audit.Filter{}))
		must(t, e)
		claimed, e := worker.RunExportOnce(ctx)
		must(t, e)
		if !claimed {
			t.Fatal("restricted worker did not claim")
		}
		result, e := s.Export(ctx, f.actor, scope, ex.ID, "1")
		must(t, e)
		if result.State != "succeeded" {
			t.Fatalf("restricted worker failed: %+v", result)
		}
	}
	for _, sql := range []string{"SELECT password_hash FROM account_passwords LIMIT 1", "SELECT * FROM account_sessions LIMIT 1", "SELECT * FROM file_objects LIMIT 1", "SELECT * FROM spatial_blobs LIMIT 1", "SELECT * FROM account_notifications LIMIT 1", "SELECT result FROM derived_trial_receipts LIMIT 1", "SELECT result FROM analysis_trial_receipts LIMIT 1", "SELECT manifest_object FROM monitoring_exports LIMIT 1", "UPDATE account_security_events SET reason='changed'", "DELETE FROM account_security_events"} {
		if _, e := pool.Exec(ctx, sql); e == nil {
			t.Fatalf("worker unexpectedly authorized: %s", sql)
		}
	}
	observerPool := rolePool(t, f, "infrapro_audit_observer")
	observer := runtime.Compose(access.New(observerPool, slog.Default())).Audit.WithStorage(store)
	_, e := observer.Status(ctx)
	must(t, e)
	v, e := observer.Verify(ctx, "metadata")
	must(t, e)
	if v.Invalid != "0" {
		t.Fatal(v)
	}
	if _, e = observer.Verify(ctx, "content"); e == nil {
		t.Fatal("metadata observer read CSV content")
	}
	if _, e = observer.Plan(ctx, "facts", "observer cannot plan writes", nil); e == nil {
		t.Fatal("observer changed cleanup plan")
	}
	content := runtime.Compose(access.New(rolePool(t, f, "infrapro_audit_content"), slog.Default())).Audit.WithStorage(store)
	v, e = content.Verify(ctx, "content")
	must(t, e)
	if v.Checked != "3" || v.Invalid != "0" {
		t.Fatal(v)
	}
	maintenance := runtime.Compose(access.New(rolePool(t, f, "infrapro_audit_maintenance"), slog.Default())).Audit
	duration := 90 * 24 * time.Hour
	plan, e := maintenance.Plan(ctx, "policy", "restricted maintenance import", &duration)
	must(t, e)
	_, e = maintenance.ApplyPlan(ctx, plan.ID, plan.Revision, uuid.NewString(), "restricted policy apply")
	must(t, e)
	restore := runtime.Compose(access.New(rolePool(t, f, "infrapro_audit_restore"), slog.Default())).Audit
	must(t, restore.SetMaintenanceMode(ctx, "reconciling", uuid.NewString(), "isolated role recovery"))
	_, e = restore.RestoreReconcile(ctx, uuid.NewString(), "coordinated role recovery")
	must(t, e)
}
