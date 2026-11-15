//go:build monitoringintegration

package resources_test

import (
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"testing"
)

func TestTaskDerivedManagerCannotReplaceLostExecutor(t *testing.T) {
	f := monitoringSetup(t)
	derived, channel, cfg := derivedChannelSetup(t, f)
	manager := f.project(t, "TaskDerivedManager")
	taskTenantRole(t, f.resourceFixture, manager.actor, []access.Code{access.DerivedCode("tenant", "runs.manage")})
	to := "1704240000000000000"
	run, e := derived.ChannelTrial(ctx, f.actor, dm.TenantScope(f.tenant), channel.Resource.ID, dm.TrialInput{WriteMeta: dm.WriteMeta{RequestID: uuid.NewString(), Revision: channel.ConfigRevision, Reason: "固定原始执行资格"}, ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: "1704067200000000000", To: &to}}, access.Meta{})
	must(t, e)
	service := runtime.ComposeTasks(f.env.Service.Access())
	ref := taskrecord.SourceRef{Module: "derived", Kind: "run", RootID: run.ID}
	before, e := service.Get(ctx, manager.actor, ref.ID(), "")
	must(t, e)
	if before.ResultReadable {
		t.Fatal("run manage granted source data read")
	}
	_, e = service.Command(ctx, manager.actor, ref.ID(), tasks.Command{RequestID: uuid.NewString(), Action: "cancel", ExpectedGeneration: before.Generation, ExpectedRevision: before.ControlRevision, Reason: "最小管理资格停止试算"})
	must(t, e)
	// Simulate the owning identity service's committed account loss. This source
	// fixture contains no running worker and cannot affect any other environment.
	_, e = f.env.Pool.Exec(ctx, `UPDATE accounts SET status='disabled' WHERE id=$1`, f.actor.ID)
	must(t, e)
	after, e := service.Get(ctx, manager.actor, ref.ID(), "")
	must(t, e)
	var disabled bool
	for _, cap := range after.Actions {
		if cap.Kind == "retry" && !cap.Enabled && cap.ReasonCode == "execution_principal_unavailable" {
			disabled = true
		}
	}
	if !disabled {
		t.Fatal("lost executor advertised retry", after.Actions)
	}
	_, e = service.Command(ctx, manager.actor, ref.ID(), tasks.Command{RequestID: uuid.NewString(), Action: "retry", ExpectedGeneration: after.Generation, ExpectedRevision: after.ControlRevision, Reason: "不允许借管理员身份重试"})
	var problem *access.Error
	if !errors.As(e, &problem) || problem.Code != "execution_principal_unavailable" {
		t.Fatalf("unexplained retry rejection: %v", e)
	}
	var generation string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT notification_generation::text FROM monitoring_jobs WHERE id=$1`, run.Job.ID).Scan(&generation))
	if generation != "1" {
		t.Fatal("rejection started an execution")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE accounts SET status='active' WHERE id=$1`, f.actor.ID)
	must(t, e)
}
