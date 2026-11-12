//go:build monitoringintegration

package resources_test

import (
	"testing"

	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
)

func TestMonitoringOperatorStatusAndDurableTrace(t *testing.T) {
	f := monitoringSetup(t)
	b := f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,1,good\n")
	traced := md.TraceContext(ctx, uuid.NewString())
	queued, e := f.service.QueueImport(traced, f.actor, f.scope, b.ID, "validate", f.meta(b.Revision))
	must(t, e)
	job, e := f.service.InspectOperatorJob(ctx, f.tenant, queued.JobID)
	must(t, e)
	if job.TraceID != md.TraceID(traced) {
		t.Fatal("HTTP-to-job trace lost")
	}
	var auditTrace string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT trace_id FROM monitoring_audit WHERE target_id=$1 AND operation='imports.validate' ORDER BY created_at DESC LIMIT 1`, b.ID).Scan(&auditTrace))
	if auditTrace != job.TraceID {
		t.Fatal(auditTrace, job.TraceID)
	}
	worked, e := f.service.RunOne(ctx, "status-worker")
	must(t, e)
	if !worked {
		t.Fatal("validation not claimed")
	}
	b, e = f.service.Import(ctx, f.actor, f.scope, b.ID)
	must(t, e)
	f.publish(t, b)
	status, e := f.service.MaintenanceStatus(ctx)
	must(t, e)
	if status.Mode != "ready" || len(status.Replicas) != 2 || status.Attempts["sealed"] != "1" || status.TempFree == "" || status.Quarantined != "0" {
		t.Fatal(status)
	}
	for _, r := range status.Replicas {
		if r.State != "ready" || r.PhysicalRows != "1" {
			t.Fatal("operator metrics unavailable", r)
		}
	}
	md.LogOperation(traced, "synthetic.trace-proof", 200, 0, false)
}
