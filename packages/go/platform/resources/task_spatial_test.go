//go:build monitoringintegration

package resources_test

import (
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"sync"
	"testing"
)

func TestTaskSpatialManagersRetryHistoryAndOriginalIdentity(t *testing.T) {
	f := spatialSetup(t)
	facility := f.facility(t, "TASK-SPATIAL")
	asset := f.newAsset(t, facility.ID, "model", "TASK-SPATIAL")
	version := f.newVersion(t, asset.ID, "", false)
	job := f.upload(t, version, "invalid/required-extension.glb")
	_, e := f.s.WorkOne(ctx, "task-failed-conversion")
	if e == nil {
		t.Fatal("invalid required extension unexpectedly converted")
	}
	one, two := f.project(t, "TaskSpatialM1"), f.project(t, "TaskSpatialM2")
	for _, a := range []access.Actor{one.actor, two.actor} {
		taskTenantRole(t, f.resourceFixture, a, []access.Code{access.TenantSpatialRead, access.TenantSpatialManage})
	}
	business := runtime.Compose(f.env.Service.Access())
	business.Spatial = f.s
	service := runtime.ComposeTasks(f.env.Service.Access(), business)
	ref := taskrecord.SourceRef{Module: "spatial", Kind: "job", RootID: job.ID}
	failed, e := service.Get(ctx, one.actor, ref.ID(), "")
	must(t, e)
	if failed.State != "failed" {
		t.Fatal(failed.State)
	}
	if _, e = f.s.OpenFile(ctx, one.actor, f.scope, version.ID, "", true); e == nil {
		t.Fatal("task manager gained original file download")
	}
	gate := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, a := range []access.Actor{one.actor, two.actor} {
		wg.Add(1)
		go func(a access.Actor) {
			defer wg.Done()
			<-gate
			_, e := service.Command(ctx, a, ref.ID(), tasks.Command{RequestID: uuid.NewString(), Action: "retry", ExpectedGeneration: failed.Generation, ExpectedRevision: failed.ControlRevision, Reason: "两位管理员并发重试同一原件"})
			results <- e
		}(a)
	}
	close(gate)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else {
			conflict++
			t.Logf("retry outcome %T: %v; cause: %v", e, e, errors.Unwrap(e))
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent retries: %d/%d", success, conflict)
	}
	current, e := service.Get(ctx, f.actor, ref.ID(), "")
	must(t, e)
	if current.Generation != "2" || current.Originator != f.actor.ID {
		t.Fatal("originator or generation changed", current)
	}
	executor := current.SelectedExecution.ExecutorRef
	if executor != one.actor.ID && executor != two.actor.ID || current.SelectedExecution.RequestedBy != executor {
		t.Fatal("current source executor identity not explicit")
	}
	old, e := service.Get(ctx, one.actor, ref.ID(), "1")
	must(t, e)
	if old.State != "failed" || old.ErrorCode != "required_extension" || old.SelectedExecution.ExecutorRef != f.actor.ID {
		t.Fatal("old failure overwritten", old)
	}
	var report json.RawMessage
	must(t, f.env.Pool.QueryRow(ctx, `SELECT report FROM spatial_task_execution_evidence WHERE job_id=$1 AND generation=1`, job.ID).Scan(&report))
	if len(report) <= 2 {
		t.Fatal("old report was not sealed")
	}
	var audits int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events WHERE details->'task'->>'task_id'=$1 AND details->'task'->>'generation'='2' AND details->'task'->>'before_state'='failed' AND details->'task'->>'after_state'='queued' AND details->'task'->>'original_actor'=$2`, ref.ID(), f.actor.ID).Scan(&audits))
	if audits != 1 {
		t.Fatal("source audit missing or duplicated", audits)
	}
	// A manager with only processing management can stop the queued generation,
	// without reading original files or professional result bodies.
	minimal := f.project(t, "TaskSpatialStop")
	taskTenantRole(t, f.resourceFixture, minimal.actor, []access.Code{access.TenantSpatialManage})
	v, e := service.Get(ctx, minimal.actor, ref.ID(), "")
	must(t, e)
	if v.ResultReadable {
		t.Fatal("minimal management became result read")
	}
	_, e = service.Command(ctx, minimal.actor, ref.ID(), tasks.Command{RequestID: uuid.NewString(), Action: "cancel", ExpectedGeneration: v.Generation, ExpectedRevision: v.ControlRevision, Reason: "仅停止权限下停止转换"})
	must(t, e)
	stopped, e := service.Get(ctx, f.actor, ref.ID(), "")
	must(t, e)
	if stopped.State != "cancelled" || stopped.CleanupState != "unknown" {
		t.Fatal("cleanup claim was fabricated")
	}
	verifyTaskLegacyUpgrade(t, f.resourceFixture)
}
