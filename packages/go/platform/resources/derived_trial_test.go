//go:build monitoringintegration

package resources_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Force the real interactive deadline boundary without relying on machine load
// or deep dependency timing. The background attempt uses the same executor.
type deadlineTrialExecutor struct {
	*dm.Service
	first, authorization bool
}

func (e *deadlineTrialExecutor) AuthorizeExecutionTx(ctx context.Context, tx pgx.Tx, task md.Task) error {
	if e.first && e.authorization {
		e.first = false
		<-ctx.Done()
		// A policy adapter may safely hide the database timeout. The task host
		// must still recognize its own interactive deadline at this boundary.
		return access.ErrCatalog
	}
	return e.Service.AuthorizeExecutionTx(ctx, tx, task)
}

func (e *deadlineTrialExecutor) ExecuteTask(ctx context.Context, lease md.TaskLease) error {
	if e.first {
		e.first = false
		<-ctx.Done()
	}
	return e.Service.ExecuteTask(ctx, lease)
}

func TestArchitectureTrialFinishesWithExistingClaim(t *testing.T) {
	f := monitoringSetup(t)
	service, channel, cfg := derivedChannelSetup(t, f)
	service.WithOptions(config.DefaultDerived())
	// Pause the ordinary worker after its FOR UPDATE selection but before its
	// state transition commits. The RPC must observe precisely the same job.
	_, err := f.env.Pool.Exec(ctx, `CREATE FUNCTION delay_trial_claim() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN PERFORM pg_advisory_xact_lock(4291,hashtext(current_schema())); RETURN NEW; END $$;
		CREATE TRIGGER delay_trial_claim BEFORE UPDATE OF state ON monitoring_jobs
		FOR EACH ROW WHEN (NEW.state='running' AND OLD.state='queued') EXECUTE FUNCTION delay_trial_claim()`)
	must(t, err)
	barrier, err := f.env.Pool.Begin(ctx)
	must(t, err)
	defer barrier.Rollback(ctx)
	_, err = barrier.Exec(ctx, `SELECT pg_advisory_xact_lock(4291,hashtext(current_schema()))`)
	must(t, err)
	done := make(chan error, 1)
	service.WithTrialAttempt(func(c context.Context, tenant, id string, budget time.Duration) (bool, error) {
		go func() {
			_, err := f.service.RunTask(c, "ordinary-incremental", tenant, id)
			done <- err
		}()
		until := time.Now().Add(budget)
		for {
			var waiting bool
			must(t, f.env.Pool.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, int32(barrier.Conn().PgConn().PID())).Scan(&waiting))
			if waiting {
				break
			}
			if time.Now().After(until) {
				t.Fatal("ordinary worker did not reach the claim barrier")
			}
			time.Sleep(10 * time.Millisecond)
		}
		claimed, err := service.AttemptTrial(c, tenant, id, budget)
		must(t, err)
		if claimed {
			t.Fatal("interactive call took a job already locked by the ordinary worker")
		}
		time.AfterFunc(100*time.Millisecond, func() { _ = barrier.Rollback(c) })
		return false, nil
	})
	end := "1704067260000000000"
	run, err := service.ChannelTrial(ctx, f.actor, dm.TenantScope(f.tenant), channel.Resource.ID, dm.TrialInput{
		WriteMeta: dm.WriteMeta{RequestID: uuid.NewString(), Revision: channel.ConfigRevision, Reason: "短试算已由同一作业的普通 worker 领取"},
		ConfigID:  cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: "1704067200000000000", To: &end},
	}, access.Meta{})
	must(t, err)
	must(t, <-done)
	if run.Job.State != "succeeded" || run.Job.Attempts != 1 || run.Trial == nil || run.Trial.OutputCount != "0" {
		t.Fatalf("completed short trial was handed to the background: %+v", run.Job)
	}
}

func TestDerivedInteractiveBudgetHandsOffWithoutFailure(t *testing.T) {
	for _, authorization := range []bool{false, true} {
		t.Run(fmt.Sprintf("authorization_%t", authorization), func(t *testing.T) {
			verifyInteractiveBudgetHandoff(t, authorization)
		})
	}
}

func verifyInteractiveBudgetHandoff(t *testing.T, authorization bool) {
	f := monitoringSetup(t)
	service, channel, cfg := derivedChannelSetup(t, f)
	_, e := f.service.WithTaskExecutors(map[string]md.TaskExecutor{"derive": &deadlineTrialExecutor{Service: service, first: true, authorization: authorization}})
	must(t, e)
	to := "1704067260000000000"
	run, e := service.ChannelTrial(ctx, f.actor, dm.TenantScope(f.tenant), channel.Resource.ID, dm.TrialInput{WriteMeta: dm.WriteMeta{RequestID: uuid.NewString(), Revision: channel.ConfigRevision, Reason: "交互预算到期转后台"}, ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: "1704067200000000000", To: &to}}, access.Meta{})
	must(t, e)
	if run.Job.State != "queued" || run.Job.Attempts != 0 || run.Job.ErrorCode != "" {
		t.Fatalf("interactive handoff counted as failure: state=%s attempts=%d error=%s", run.Job.State, run.Job.Attempts, run.Job.ErrorCode)
	}
	worked, e := f.service.RunTask(ctx, "background-after-budget", f.tenant, run.Job.ID)
	must(t, e)
	if !worked {
		t.Fatal("background handoff could not be claimed immediately")
	}
	run, e = service.Run(ctx, f.actor, dm.TenantScope(f.tenant), run.ID)
	must(t, e)
	if run.Job.State != "succeeded" || run.Trial == nil || !run.Trial.WaitingForInput {
		t.Fatal("background attempt failed to finish the complete target", run)
	}
}

func TestDerivedBackgroundTrialPartitionsAndResumesCompleteSnapshot(t *testing.T) {
	f := monitoringSetup(t)
	var csv strings.Builder
	csv.WriteString("observed_at,value,quality\n")
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&csv, "%s,%d,good\n", base.Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano), i*i)
	}
	f.publish(t, f.validate(t, f.upload(t, csv.String())))
	s, channel, cfg := derivedChannelSetup(t, f)
	options := config.DefaultDerived()
	options.TrialRows = 3
	options.WriteRows = 8
	options.ReadBytes = 4096 // Force several complete-halo partitions.
	s.WithOptions(options)
	end := strconv.FormatInt(base.Add(40*time.Second).UnixNano(), 10)
	run, e := s.ChannelTrial(ctx, f.actor, dm.TenantScope(f.tenant), channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: strconv.FormatInt(base.UnixNano(), 10), To: &end}}, access.Meta{})
	must(t, e)
	if run.Job.State != "queued" || run.Job.Attempts != 0 {
		t.Fatal("oversized trial did not hand off", run.Job)
	}
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION fail_trial_checkpoint() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.step_key='trial-progress' AND (NEW.result#>>'{inline,completed_parts}')::integer=2 THEN RAISE EXCEPTION 'injected checkpoint commit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_trial_checkpoint BEFORE INSERT OR UPDATE ON monitoring_job_steps FOR EACH ROW EXECUTE FUNCTION fail_trial_checkpoint()`)
	must(t, e)
	_, e = f.service.RunTask(ctx, "trial-interrupted", f.tenant, run.Job.ID)
	if e == nil || !strings.Contains(e.Error(), "injected checkpoint commit failure") {
		t.Fatal("checkpoint fault was not exercised", e)
	}
	var completed string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT result#>>'{inline,completed_parts}' FROM monitoring_job_steps WHERE job_id=$1 AND step_key='trial-progress'`, run.Job.ID).Scan(&completed))
	if completed != "1" {
		t.Fatal("checkpoint was lost", completed)
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER fail_trial_checkpoint ON monitoring_job_steps; DROP FUNCTION fail_trial_checkpoint(); UPDATE monitoring_jobs SET available_at=clock_timestamp() WHERE state='retry_wait'`)
	must(t, e)
	_, e = f.service.RunTask(ctx, "trial-resumed", f.tenant, run.Job.ID)
	must(t, e)
	run, e = s.Run(ctx, f.actor, dm.TenantScope(f.tenant), run.ID)
	must(t, e)
	if run.Job.State != "succeeded" || run.Trial == nil || run.Trial.OutputCount != "40" || len(run.Trial.Examples) != 40 || run.Trial.States["valid"] != "39" || run.Trial.Nodes[0].Samples != "40" {
		t.Fatal("background trial was truncated or double counted", run)
	}
	for i, row := range run.Trial.Examples {
		if i == 0 {
			if row.Status != "missing_input" {
				t.Fatal(row)
			}
			continue
		}
		if row.Value == nil || *row.Value != fmt.Sprintf("%d.000000000000000000", 2*i-1) {
			t.Fatal("partition halo changed difference", i, row)
		}
	}
	var published int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_generations WHERE tenant_id=$1`, f.tenant).Scan(&published))
	if published != 0 {
		t.Fatal("trial published formal data")
	}
}

func derivedChannelSetup(t *testing.T, f monitoringFixture, configure ...func(*dm.Rule)) (*dm.Service, dm.Channel, dm.ChannelConfig) {
	return derivedChannelSetupBindings(t, f, nil, configure...)
}
func derivedChannelSetupBindings(t *testing.T, f monitoringFixture, baselines map[string]dm.BaselineBinding, configure ...func(*dm.Rule)) (*dm.Service, dm.Channel, dm.ChannelConfig) {
	t.Helper()
	service := dm.New(f.env.Service.Access()).WithResources(f.srv).WithMonitoring(f.service)
	must(t, service.RegisterTasks())
	scope := dm.TenantScope(f.tenant)
	meta := func(revision string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: revision, Reason: "真实输入试算验证"}
	}
	rule, e := service.ExpandTemplate(ctx, f.actor, scope, "difference", dm.ExpandTemplateInput{Inputs: []dm.InputSpec{{Key: "x", Shape: "numeric", SourceKinds: []string{"direct"}, MetricVersionID: f.metric, Quantity: dm.Quantity{BaseUnitVersionID: f.unit, BasePower: 1, Mode: "level", LevelScale: "ratio"}, Time: dm.TimeSemantics{Kind: "instant"}, Direction: "not_applicable", Reference: "not_applicable"}}})
	must(t, e)
	for _, change := range configure {
		change(&rule)
	}
	raw, e := json.Marshal(rule)
	must(t, e)
	d, e := service.CreateMetric(ctx, f.actor, scope, dm.CreateMetricInput{WriteMeta: meta(""), Code: "DIFFERENCE", Name: "相邻差分", Rule: raw}, access.Meta{})
	must(t, e)
	validation, e := service.ValidateMetric(ctx, f.actor, scope, d.Definition.ID, dm.ValidateInput{DraftRevision: d.Definition.Draft.Revision})
	must(t, e)
	published, e := service.PublishMetric(ctx, f.actor, scope, d.Definition.ID, dm.PublishMetricInput{WriteMeta: meta(d.Definition.Revision), DraftRevision: d.Definition.Draft.Revision, ValidationHash: validation.ValidationHash}, access.Meta{})
	must(t, e)
	var facility, point string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text,point_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility, &point))
	channel, e := service.CreateChannel(ctx, f.actor, scope, dm.CreateChannelInput{WriteMeta: meta(""), Code: "DIFFERENCE", Name: "位移相邻变化", Owner: dm.Owner{Kind: res.Point, ID: point, FacilityID: facility}, DefinitionVersionID: published.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: f.channel}}, BaselineBindings: baselines, Parameters: map[string]string{}}, access.Meta{})
	must(t, e)
	configs, e := service.ChannelConfigs(ctx, f.actor, scope, channel.Channel.Resource.ID, dm.MetricFilter{})
	must(t, e)
	cfg := configs.Items[0]
	return service, channel.Channel, cfg
}

func TestDerivedTrialUsesRealSnapshotAndSharedTask(t *testing.T) {
	f := monitoringSetup(t)
	t.Cleanup(func() { verifyTerminalNotifications(t, f.env) })
	service, currentChannel, cfg := derivedChannelSetup(t, f)
	channel := dm.ChannelResult{Channel: currentChannel}
	scope := dm.TenantScope(f.tenant)
	meta := func(revision string) dm.WriteMeta {
		return dm.WriteMeta{RequestID: uuid.NewString(), Revision: revision, Reason: "真实输入试算验证"}
	}
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	end := strconv.FormatInt(start+int64(3*time.Minute), 10)
	input := dm.TrialInput{WriteMeta: meta(channel.Channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: strconv.FormatInt(start, 10), To: &end}}
	empty, e := service.ChannelTrial(ctx, f.actor, scope, channel.Channel.Resource.ID, input, access.Meta{})
	must(t, e)
	if empty.Job.State != "succeeded" || empty.Trial == nil || !empty.Trial.WaitingForInput || empty.Trial.InputCounts.Rows != 0 || empty.Trial.OutputCount != "0" {
		t.Fatal("empty input became a failed or fabricated trial", empty)
	}
	input.RequestID = uuid.NewString()
	batch := f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	run, e := service.ChannelTrial(ctx, f.actor, scope, channel.Channel.Resource.ID, input, access.Meta{})
	must(t, e)
	if run.Job.State != "succeeded" {
		t.Fatalf("small trial did not finish inline: %+v", run)
	}
	if run.Trial == nil || run.Trial.ReceiptID == "" || run.Trial.InputCounts.Rows != 3 || run.Trial.OutputCount != "3" || run.Trial.WaitingForInput || run.Trial.Stale || len(run.Trial.Examples) != 3 {
		t.Fatalf("incomplete trial: %+v", run.Trial)
	}
	values := run.Trial.Examples
	if values[0].Status != "missing_input" || values[0].Value != nil || values[1].Value == nil || *values[1].Value != "3.000000000000000000" || *values[2].Value != "4.000000000000000000" {
		t.Fatal("trial changed hand-computable difference", values)
	}
	if len(values[1].Matches) != 2 || values[1].Matches[0].Offset != "-60000000000" || len(run.Trial.Nodes) != 1 || run.Trial.Nodes[0].States["valid"] != "2" {
		t.Fatal("missing node/time evidence", run.Trial)
	}
	replay, e := service.ChannelTrial(ctx, f.actor, scope, channel.Channel.Resource.ID, input, access.Meta{})
	must(t, e)
	if !replay.Replayed || replay.ID != run.ID || replay.Trial.ReceiptID != run.Trial.ReceiptID {
		t.Fatal("trial request lost idempotency")
	}
	var formal int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM derived_channel_heads WHERE tenant_id=$1 AND channel_id=$2 AND current_root IS NOT NULL`, f.tenant, channel.Channel.Resource.ID).Scan(&formal))
	if formal != 0 {
		t.Fatal("trial published formal monitoring values")
	}
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-02T00:00:00Z,12,good\n")))
	current, e := service.Run(ctx, f.actor, scope, run.ID)
	must(t, e)
	if current.Trial == nil || current.Trial.Stale {
		t.Fatal("unrelated append invalidated receipt")
	}
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:01:30Z,7,good\n")))
	current, e = service.Run(ctx, f.actor, scope, run.ID)
	must(t, e)
	if current.Trial == nil || !current.Trial.Stale || current.Trial.OutputCount != "3" {
		t.Fatal("fixed trial did not retain its original input view")
	}
	// A long target is represented as the complete queued range and can be
	// cancelled through the canonical shared task before any output is produced.
	long := input
	long.RequestID = uuid.NewString()
	longEnd := strconv.FormatInt(start+int64(48*time.Hour), 10)
	long.OutputRange.To = &longEnd
	queued, e := service.ChannelTrial(ctx, f.actor, scope, channel.Channel.Resource.ID, long, access.Meta{})
	must(t, e)
	if queued.Job.State != "queued" || queued.Trial != nil || *queued.OutputRange.To != longEnd {
		t.Fatal("long trial was truncated", queued)
	}
	cancelInput := meta(queued.Job.Revision)
	cancelled, e := service.ControlRun(ctx, f.actor, scope, queued.ID, "cancel", cancelInput, access.Meta{})
	must(t, e)
	if cancelled.Job.State != "cancelled" {
		t.Fatal(cancelled)
	}
	cancelled, e = service.ControlRun(ctx, f.actor, scope, queued.ID, "cancel", cancelInput, access.Meta{})
	must(t, e)
	if !cancelled.Replayed {
		t.Fatal("cancel receipt lost")
	}
	preview, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: preview.Hash})
	must(t, e)
	current, e = service.Run(ctx, f.actor, scope, run.ID)
	must(t, e)
	if os.Getenv("INFRAPRO_TASK_ASSERTIONS") == "1" {
		tasks := runtime.ComposeTasks(f.env.Service.Access())
		view, err := tasks.Get(ctx, f.actor, taskrecord.SourceRef{Module: "derived", Kind: "run", RootID: run.ID}.ID(), "")
		must(t, err)
		if view.ResultState != "invalidated" {
			t.Fatal("task hid fixed-input invalidation", view.ResultState)
		}
	}
	if current.Trial != nil || current.Job.ResultState != "input_invalidated" {
		t.Fatal("revoked current trial values remained readable", current)
	}
	page, e := service.Runs(ctx, f.actor, scope, dm.RunFilter{ChannelID: channel.Channel.Resource.ID})
	must(t, e)
	if len(page.Items) != 3 {
		t.Fatal("run metadata disappeared on input invalidation", page)
	}
}
