package derivedmonitoring_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func queueAutomaticFixture(t *testing.T, f resourceFixture, host *monitoring.Service, channel dm.Channel, planHash string, needs []monitoring.RawNeed, from, to int64) (string, string) {
	t.Helper()
	runID, jobID := uuid.NewString(), uuid.NewString()
	raw, e := json.Marshal(needs)
	must(t, e)
	must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO derived_runs(id,tenant_id,channel_id,config_id,mode,job_id,request_id,request_hash,output_range,config_revision,control_revision,mandate_revision,plan_hash,input_requirements,requested_at_ns,reason) SELECT $1,$2,$3,draft_config_id,'automatic',$4,$5,$6,int8range($7,$8,'[)'),config_revision,control_revision,mandate_revision,$6,$9,$8,'系统委托边界验证' FROM derived_channels WHERE tenant_id=$2 AND channel_id=$3`, runID, f.scope.Tenant, channel.Resource.ID, jobID, uuid.NewString(), planHash, from, to, raw)
		if e != nil {
			return e
		}
		_, e = host.EnqueueTaskTx(ctx, tx, monitoring.EnqueueTask{ID: jobID, Scope: monitoring.TenantScope(f.scope.Tenant), Kind: "derive", Target: runID, TargetRevision: channel.ConfigRevision, Input: []byte(`{"mode":"automatic"}`)})
		return e
	}))
	return runID, jobID
}

// No physical samples are used here: this qualifies transactional control,
// empty-input opt-in and tenant execution authority. Real payloads have their
// separate CH/S3 integration qualification.
func TestDerivedControlReceiptMandateAndPauseFence(t *testing.T) {
	f := channelsSetup(t)
	actor := f.actor(f.admin)
	host := monitoring.New(f.e.Service.Access()).WithStorage(new(monitoring.Storage))
	f.s.WithMonitoring(host)
	must(t, f.s.RegisterTasks())
	definition := f.publish(t, f.create(t, "control").Definition)
	created, e := f.s.CreateChannel(ctx, actor, f.scope, dm.CreateChannelInput{WriteMeta: wm(""), Code: "CONTROL", Name: "计算启停", Owner: dm.Owner{Kind: resources.Point, FacilityID: f.facility, ID: f.point}, DefinitionVersionID: definition.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: f.direct}}, Parameters: map[string]string{}}, access.Meta{})
	must(t, e)
	channel := created.Channel
	configs, e := f.s.ChannelConfigs(ctx, actor, f.scope, channel.Resource.ID, dm.MetricFilter{})
	must(t, e)
	cfg := configs.Items[0]
	start, end := "100", "200"
	trial, e := f.s.ChannelTrial(ctx, actor, f.scope, channel.Resource.ID, dm.TrialInput{WriteMeta: wm(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: start, To: &end}}, access.Meta{})
	must(t, e)
	if trial.Trial == nil || !trial.Trial.WaitingForInput {
		t.Fatal("empty input did not produce waiting receipt", trial)
	}
	in := dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &start, TrialReceiptID: trial.Trial.ReceiptID}}
	_, e = f.s.PreviewControl(ctx, actor, f.scope, channel.Resource.ID, in)
	code(t, e, "waiting_confirmation_required")
	in.AllowWaiting = true
	preview, e := f.s.PreviewControl(ctx, actor, f.scope, channel.Resource.ID, in)
	must(t, e)
	if len(preview.OutputRanges) != 1 || preview.OutputRanges[0].From != start || preview.OutputRanges[0].To != nil || len(preview.InputRanges) != 1 || preview.InputRanges[0].To != nil {
		t.Fatal("continuous authority was silently limited", preview)
	}
	command := dm.ControlInput{WriteMeta: wm(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}
	dependency := errors.New("test storage unavailable")
	f.s.WithReadiness(func(context.Context) error { return dependency })
	if _, e = f.s.PreviewControl(ctx, actor, f.scope, channel.Resource.ID, in); !errors.Is(e, dependency) {
		t.Fatal("enable preview ignored readiness", e)
	}
	if _, e = f.s.ControlChannel(ctx, actor, f.scope, channel.Resource.ID, command, access.Meta{}); !errors.Is(e, dependency) {
		t.Fatal("enable submission ignored readiness", e)
	}
	f.s.WithReadiness(nil)
	enabled, e := f.s.ControlChannel(ctx, actor, f.scope, channel.Resource.ID, command, access.Meta{})
	must(t, e)
	if enabled.Channel.ControlState != "enabled" || enabled.Channel.ControlRevision != "2" || enabled.Channel.MandateRevision != "2" {
		t.Fatal(enabled)
	}
	repeated, e := f.s.ControlChannel(ctx, actor, f.scope, channel.Resource.ID, command, access.Meta{})
	must(t, e)
	if !repeated.Replayed {
		t.Fatal("enable was not idempotent")
	}
	var mandates, assignments, dirty int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM derived_execution_mandates WHERE tenant_id=$1),(SELECT count(*) FROM derived_config_assignments WHERE tenant_id=$1),(SELECT count(*) FROM derived_dirty_ranges WHERE tenant_id=$1)`, f.scope.Tenant).Scan(&mandates, &assignments, &dirty))
	if mandates != 1 || assignments != 1 || dirty != 1 {
		t.Fatal("control did not atomically preserve its execution basis", mandates, assignments, dirty)
	}
	_, e = f.e.Pool.Exec(ctx, `UPDATE derived_execution_mandates SET input_requirements='[]' WHERE tenant_id=$1`, f.scope.Tenant)
	if e == nil {
		t.Fatal("execution evidence was mutable")
	}
	runID, jobID := uuid.NewString(), uuid.NewString()
	must(t, f.e.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO derived_runs(id,tenant_id,channel_id,config_id,mode,job_id,request_id,request_hash,output_range,config_revision,control_revision,mandate_revision,plan_hash,input_requirements,requested_at_ns,reason) SELECT $1,$2,$3,$4,'automatic',$5,$6,$7,'[100,200)'::int8range,config_revision,control_revision,mandate_revision,$7,$8,200,'系统委托边界验证' FROM derived_channels WHERE tenant_id=$2 AND channel_id=$3`, runID, f.scope.Tenant, channel.Resource.ID, cfg.ID, jobID, uuid.NewString(), trial.PlanHash, preview.InputRanges)
		if e != nil {
			return e
		}
		_, e = host.EnqueueTaskTx(ctx, tx, monitoring.EnqueueTask{ID: jobID, Scope: monitoring.TenantScope(f.scope.Tenant), Kind: "derive", Target: runID, TargetRevision: "1", Input: []byte(`{"mode":"automatic"}`)})
		return e
	}))
	permit := monitoring.RawSystemPermit{Tenant: f.scope.Tenant, RunID: runID, MandateRevision: "2"}
	// This checks a real persisted automatic run, not a client-supplied actor.
	from, to := int64(100), int64(200)
	snapshot, e := host.RawReader().FreezeSystem(ctx, permit, monitoring.RawFreezeRequest{Needs: []monitoring.RawNeed{{Channel: f.direct, From: &from, To: &to}}})
	must(t, e)
	_, e = host.RawReader().FreezeSystem(ctx, monitoring.RawSystemPermit{Tenant: f.scope.Tenant, RunID: uuid.NewString(), MandateRevision: "2"}, monitoring.RawFreezeRequest{Needs: preview.InputRanges})
	if e == nil {
		t.Fatal("forged automatic run accepted")
	}
	f.s.WithReadiness(func(context.Context) error { return dependency })
	pause, e := f.s.PreviewControl(ctx, actor, f.scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: enabled.Channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "pause"}})
	must(t, e)
	paused, e := f.s.ControlChannel(ctx, actor, f.scope, channel.Resource.ID, dm.ControlInput{WriteMeta: wm(enabled.Channel.ControlRevision), PreviewID: pause.ID, PreviewHash: pause.Hash}, access.Meta{})
	must(t, e)
	f.s.WithReadiness(nil)
	var state string
	var fence int64
	must(t, f.e.Pool.QueryRow(ctx, `SELECT state,fence FROM monitoring_jobs WHERE id=$1`, jobID).Scan(&state, &fence))
	if state != "cancelled" || fence != 1 || paused.Channel.ControlState != "paused" {
		t.Fatal("pause did not fence prior work", state, fence, paused)
	}
	_, e = host.RawReader().CheckSystem(ctx, permit, snapshot.ID, false)
	if e == nil {
		t.Fatal("old mandate survived pause")
	}
	resumeIn := dm.ControlPreviewInput{Revision: paused.Channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "resume", TrialReceiptID: trial.Trial.ReceiptID, AllowWaiting: true}}
	resume, e := f.s.PreviewControl(ctx, actor, f.scope, channel.Resource.ID, resumeIn)
	must(t, e)
	if resume.Proposal.ResumeMode != "catch_up" || resume.Proposal.AutoStartNS == nil || *resume.Proposal.AutoStartNS != pause.EffectiveAtNS || len(resume.OutputRanges) != 1 || resume.OutputRanges[0].From != start {
		t.Fatal("resume did not preserve earlier range and paused gap", resume)
	}
	future := strconv.FormatInt(time.Now().Add(time.Hour).UnixNano(), 10)
	resumeIn.ResumeMode = "new_start"
	resumeIn.AutoStartNS = &future
	restart, e := f.s.PreviewControl(ctx, actor, f.scope, channel.Resource.ID, resumeIn)
	must(t, e)
	if len(restart.OutputRanges) != 2 || restart.OutputRanges[0].To == nil || *restart.OutputRanges[0].To != pause.EffectiveAtNS || restart.OutputRanges[1].From != future {
		t.Fatal("new start hid the uncovered gap", restart)
	}
	restarted, e := f.s.ControlChannel(ctx, actor, f.scope, channel.Resource.ID, dm.ControlInput{WriteMeta: wm(paused.Channel.ControlRevision), PreviewID: restart.ID, PreviewHash: restart.Hash}, access.Meta{})
	must(t, e)
	if restarted.Channel.ControlState != "enabled" || restarted.Channel.MandateRevision != "4" {
		t.Fatal(restarted)
	}
	_, e = f.s.ControlChannel(ctx, actor, f.scope, channel.Resource.ID, dm.ControlInput{WriteMeta: wm(paused.Channel.ControlRevision), PreviewID: resume.ID, PreviewHash: resume.Hash}, access.Meta{})
	code(t, e, "revision_changed")
}
