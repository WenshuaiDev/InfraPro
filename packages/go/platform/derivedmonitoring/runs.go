package derivedmonitoring

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func monitoringScope(scope Scope) monitoring.Scope {
	return monitoring.Scope{Entry: scope.Entry, Tenant: scope.Tenant, Project: scope.Project}
}
func (s *Service) WithMonitoring(host *monitoring.Service) *Service {
	s.monitoring = host
	host.WithRawSystemAuthorizer(s)
	host.WithCalculationReader(s)
	host.WithCalculationMaintenance(s)
	host.WithTaskScheduler(s)
	s.raw = host.RawReader()
	return s
}
func (s *Service) RegisterTasks() error {
	if s.monitoring == nil {
		return problem("dependency_unavailable", "共享监测任务宿主尚未配置。", 503)
	}
	_, e := s.monitoring.WithTaskExecutors(map[string]monitoring.TaskExecutor{"derive": s, "analysis": s, "derived_replay": s, "analysis_compare": s, "analysis_export": s, "derived_evidence_export": s})
	return e
}
func requireAnyDerivedTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, suffixes ...string) error {
	revision, e := access.Lock(ctx, tx, false)
	if e != nil {
		return e
	}
	var codes []access.Code
	if scope.Entry == "project" {
		snapshot, e := access.ProjectSnapshotTx(ctx, tx, a.ID, scope.Tenant, scope.Project, revision)
		if e != nil {
			return e
		}
		codes = snapshot.Permissions
	} else {
		_, snapshot, e := access.TenantSnapshotTx(ctx, tx, a.ID, scope.Tenant, revision)
		if e != nil {
			return e
		}
		codes = snapshot.Permissions
	}
	for _, suffix := range suffixes {
		if slices.Contains(codes, access.DerivedCode(scope.Entry, suffix)) {
			return nil
		}
	}
	return access.ErrDenied
}
func (s *Service) calculationConfigTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, channel, configID string) (Channel, ChannelConfig, *compute.Plan, error) {
	var c Channel
	cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND id=$3`, scope.Tenant, channel, configID))
	if e == pgx.ErrNoRows {
		return c, cfg, nil, notFound()
	}
	if e != nil {
		return c, cfg, nil, e
	}
	targets := []resources.Target{{Kind: resources.Channel, ID: channel}}
	for _, b := range cfg.InputBindings {
		targets = append(targets, resources.Target{Kind: resources.Channel, ID: b.ChannelID})
	}
	bound, e := s.resources.CalculationTargetsTx(ctx, tx, a, resourceScope(scope), targets, false)
	if e != nil {
		return c, cfg, nil, e
	}
	if !bound["channel:"+channel].CanManage {
		return c, cfg, nil, access.ErrDenied
	}
	c, e = s.channelTx(ctx, tx, a, scope, channel)
	if e != nil {
		return c, cfg, nil, e
	}
	p, e := s.compile(cfg.Rule, "channel", cfg.Parameters)
	if e != nil {
		return c, cfg, nil, e
	}
	return c, cfg, p, nil
}
func (s *Service) plannedInputsTx(ctx context.Context, tx pgx.Tx, tenant string, cfg ChannelConfig, p *compute.Plan, r planning.Range) ([]monitoring.RawNeed, error) {
	needs, e := immediateInputNeeds(cfg, p, r)
	if e != nil {
		return nil, e
	}
	if _, _, e = splitInputKindsTx(ctx, tx, tenant, needs); e != nil {
		return nil, e
	}
	return needs, nil
}

type runRecord struct {
	Run
	Analysis                                         *analysisRunTarget
	Scope                                            Scope
	Actor, JobID                                     string
	ConfigRevision, ControlRevision, MandateRevision string
	Needs                                            []monitoring.RawNeed
	RequestedAt                                      int64
}

const runSelect = `SELECT r.id::text,COALESCE(r.channel_id::text,''),COALESCE(r.config_id::text,''),r.mode,lower(r.output_range),upper(r.output_range),r.plan_hash,r.created_at,r.tenant_id::text,COALESCE(r.project_id::text,''),COALESCE(r.actor_id::text,''),r.job_id::text,r.config_revision::text,r.control_revision::text,r.mandate_revision::text,r.input_requirements,r.requested_at_ns FROM derived_runs r`

func scanRun(row pgx.Row) (runRecord, error) {
	var r runRecord
	var from int64
	var to *int64
	e := row.Scan(&r.ID, &r.ChannelID, &r.ConfigID, &r.Mode, &from, &to, &r.PlanHash, &r.CreatedAt, &r.Scope.Tenant, &r.Scope.Project, &r.Actor, &r.JobID, &r.ConfigRevision, &r.ControlRevision, &r.MandateRevision, &r.Needs, &r.RequestedAt)
	r.Scope.Entry = "tenant"
	if r.Scope.Project != "" {
		r.Scope.Entry = "project"
	}
	r.OutputRange = rangeDTO(from, to)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	return r, e
}
func (s *Service) ChannelTrial(ctx context.Context, a access.Actor, scope Scope, id string, in TrialInput, audit access.Meta) (Run, error) {
	var out Run
	if s.monitoring == nil || s.raw == nil {
		return out, problem("dependency_unavailable", "监测数据与任务宿主尚未配置。", 503)
	}
	if e := checkMeta(in.WriteMeta, true, false); e != nil {
		return out, e
	}
	if !access.ValidID(id) || !access.ValidID(in.ConfigID) || len(in.ConfigHash) != 64 {
		return out, invalid("/config_id", "需要固定配置及其内容 hash。")
	}
	output, e := in.OutputRange.rangeValue()
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "channels.trials", true, func(tx pgx.Tx) error {
		if e := requireAnyDerivedTx(ctx, tx, a, scope, "channels.manage", "channels.control"); e != nil {
			return e
		}
		channel, cfg, p, e := s.calculationConfigTx(ctx, tx, a, scope, id, in.ConfigID)
		if e != nil {
			return e
		}
		if cfg.ConfigHash != in.ConfigHash || channel.ConfigRevision != in.Revision {
			return problem("revision_changed", "配置已变化，请读取当前配置后重新试算。", 409)
		}
		needs, e := s.plannedInputsTx(ctx, tx, scope.Tenant, cfg, p, output)
		if e != nil {
			return e
		}
		if e = s.authorizeInputsTx(ctx, tx, a, monitoringScope(scope), needs, false); e != nil {
			return e
		}
		baselineNeeds, e := s.baselineNeedsTx(ctx, tx, scope.Tenant, cfg, p)
		if e != nil {
			return e
		}
		if len(baselineNeeds) > 0 {
			if e = s.authorizeInputsTx(ctx, tx, a, monitoringScope(scope), baselineNeeds, false); e != nil {
				return e
			}
		}
		var oldID, oldHash string
		e = tx.QueryRow(ctx, `SELECT id::text,request_hash FROM derived_runs WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.key(), a.ID, in.RequestID).Scan(&oldID, &oldHash)
		hash := fingerprint([]any{id, in})
		if e == nil {
			if oldHash != hash {
				return problem("request_id_reused", "同一请求编号不能用于不同试算。", 409)
			}
			out.ID = oldID
			out.Replayed = true
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		out.ID = uuid.NewString()
		jobID := uuid.NewString()
		_, e = tx.Exec(ctx, `INSERT INTO derived_runs(id,tenant_id,project_id,actor_id,channel_id,config_id,mode,job_id,request_id,request_hash,output_range,config_revision,control_revision,mandate_revision,plan_hash,input_requirements,requested_at_ns,reason) VALUES($1,$2,$3,$4,$5,$6,'trial',$7,$8,$9,$10::int8range,$11,$12,$13,$14,$15,$16,$17)`, out.ID, scope.Tenant, nullable(scope.Project), a.ID, id, cfg.ID, jobID, in.RequestID, hash, rangeSQL(output), channel.ConfigRevision, channel.ControlRevision, channel.MandateRevision, p.PlanHash, jsonBytes(needs), time.Now().UnixNano(), in.Reason)
		if e != nil {
			return e
		}
		out.Job, e = s.monitoring.EnqueueTaskTx(ctx, tx, monitoring.EnqueueTask{ID: jobID, Scope: monitoringScope(scope), ActorID: a.ID, Kind: "derive", Target: out.ID, TargetRevision: channel.ConfigRevision, Input: jsonBytes(map[string]string{"mode": "trial", "run_id": out.ID})})
		if e != nil {
			return e
		}
		audit.ActorID = a.ID
		audit.ActorType = "account"
		audit.ActorScope = scope.Entry
		audit.TenantID = scope.Tenant
		audit.ProjectID = scope.Project
		audit.Reason = in.Reason
		return access.Audit(ctx, tx, "derived.channels.trials", "derived_run", out.ID, "", nil, map[string]string{"channel_id": id, "config_id": cfg.ID, "job_id": jobID}, audit)
	})
	if e != nil {
		return Run{}, e
	}
	replayed := out.Replayed
	// The full target remains queued if its duration/input/time budget exceeds
	// the interactive allowance. No truncated prefix is reported as a trial.
	if !replayed && output.To != nil && uint64(*output.To)-uint64(output.From) <= uint64(s.options.TrialRange) {
		attempt := s.trialAttempt
		if attempt == nil {
			attempt = s.AttemptTrial
		}
		deadline := time.Now().Add(s.options.TrialDeadline)
		claimed, _ := attempt(ctx, scope.Tenant, out.Job.ID, s.options.TrialDeadline)
		if !claimed && s.trialAttempt != nil && time.Now().Before(deadline) {
			s.waitClaimedTrial(ctx, scope.Tenant, out.Job.ID, deadline)
		}
	}
	result, e := s.readRun(ctx, a, scope, out.ID, "channels.trials")
	result.Replayed = replayed
	return result, e
}

func (s *Service) AuthorizeExecutionTx(ctx context.Context, tx pgx.Tx, task monitoring.Task) error {
	if task.Job.Kind == "derived_evidence_export" {
		return s.authorizeGenerationExportTaskTx(ctx, tx, task, executionActor(task.ActorID), task.Scope, true, false)
	}
	if task.Job.Kind == "analysis_export" {
		return s.authorizeAnalysisExportTaskTx(ctx, tx, task, executionActor(task.ActorID), task.Scope, true, false)
	}
	run, e := scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.id=$2 AND r.job_id=$3`, task.Scope.Tenant, task.Job.Target, task.Job.ID))
	if e != nil {
		return e
	}
	if run.Actor != task.ActorID || monitoringScope(run.Scope) != task.Scope {
		return access.ErrDenied
	}
	if run.Mode == "automatic" {
		permit := monitoring.RawSystemPermit{Tenant: run.Scope.Tenant, RunID: run.ID, MandateRevision: run.MandateRevision}
		if e = s.AuthorizeRawSystemTx(ctx, tx, permit, run.Needs, false); e != nil {
			return e
		}
		var snapshot string
		e = tx.QueryRow(ctx, `SELECT snapshot_id::text FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2`, run.Scope.Tenant, run.ID).Scan(&snapshot)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		_, e = s.checkSystemInputsTx(ctx, tx, permit, snapshot)
		return e
	}
	if run.Mode == "backfill" {
		return s.authorizeBackfillExecutionTx(ctx, tx, task, run)
	}
	if run.Mode == "analysis" {
		return s.authorizeAnalysisExecutionTx(ctx, tx, task, run)
	}
	if run.Mode == "comparison" {
		return s.authorizeComparisonTx(ctx, tx, executionActor(task.ActorID), run.Scope, run, false)
	}
	if run.Mode == "historical_replay" {
		return s.authorizeReplayExecutionTx(ctx, tx, task, run)
	}
	if run.Mode != "trial" {
		return access.ErrDenied
	}
	actor := executionActor(task.ActorID)
	op := access.DerivedOperation(run.Scope.Entry, "channels.trials")
	if run.Scope.Entry == "project" {
		if _, e = access.RequireProjectTx(ctx, tx, actor.ID, op, run.Scope.Tenant, run.Scope.Project); e != nil {
			return e
		}
	} else if e = access.RequireScopedTx(ctx, tx, actor.ID, op, run.Scope.Tenant); e != nil {
		return e
	}
	if e = requireAnyDerivedTx(ctx, tx, actor, run.Scope, "channels.manage", "channels.control"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `SELECT revision FROM derived_graph_states WHERE tenant_id=$1 FOR SHARE`, run.Scope.Tenant); e != nil {
		return e
	}
	channel, _, p, e := s.calculationConfigTx(ctx, tx, actor, run.Scope, run.ChannelID, run.ConfigID)
	if e != nil {
		return e
	}
	if channel.ConfigRevision != run.ConfigRevision || p.PlanHash != run.PlanHash {
		return &monitoring.TaskError{Code: "configuration_changed", Message: "试算配置已变化。", Disposition: "needs_action"}
	}
	if e = s.authorizeInputsTx(ctx, tx, actor, task.Scope, run.Needs, false); e != nil {
		return e
	}
	if e = s.authorizeBaselineNeedsTx(ctx, tx, actor, task.Scope, run.ConfigID); e != nil {
		return e
	}
	var snapshot string
	e = tx.QueryRow(ctx, `SELECT snapshot_id::text FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2`, run.Scope.Tenant, run.ID).Scan(&snapshot)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	_, e = s.checkUserInputsTx(ctx, tx, actor, task.Scope, snapshot, false)
	return e
}
func (s *Service) AuthorizeTaskTx(ctx context.Context, tx pgx.Tx, task monitoring.Task, a access.Actor, scope monitoring.Scope, manage bool) error {
	if task.Job.Kind == "derived_evidence_export" {
		return s.authorizeGenerationExportTaskTx(ctx, tx, task, a, scope, false, manage)
	}
	if task.Job.Kind == "analysis_export" {
		return s.authorizeAnalysisExportTaskTx(ctx, tx, task, a, scope, false, manage)
	}
	if task.Scope.Tenant != scope.Tenant {
		return notFound()
	}
	run, e := scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.id=$2 AND r.job_id=$3`, scope.Tenant, task.Job.Target, task.Job.ID))
	if e != nil {
		return e
	}
	if run.Mode == "analysis" {
		return s.authorizeAnalysisTaskTx(ctx, tx, a, Scope{Tenant: scope.Tenant, Project: scope.Project, Entry: scope.Entry}, run, manage)
	}
	if run.Mode == "comparison" {
		if e := requireAnyDerivedTx(ctx, tx, a, Scope{Tenant: scope.Tenant, Project: scope.Project, Entry: scope.Entry}, "runs.read"); e != nil {
			return e
		}
		return s.authorizeComparisonTx(ctx, tx, a, Scope{Tenant: scope.Tenant, Project: scope.Project, Entry: scope.Entry}, run, manage)
	}
	if run.Mode != "trial" && run.Mode != "automatic" && run.Mode != "backfill" && run.Mode != "historical_replay" {
		return access.ErrDenied
	}
	if run.Mode != "automatic" && task.Scope != scope {
		return notFound()
	}
	viewScope := Scope{Tenant: scope.Tenant, Project: scope.Project, Entry: scope.Entry}
	if run.Mode == "historical_replay" {
		return s.authorizeReplayTaskTx(ctx, tx, a, viewScope, run, manage)
	}
	if manage {
		// Managing a run never grants its read permission implicitly.
		if e = requireAnyDerivedTx(ctx, tx, a, viewScope, "runs.manage"); e != nil {
			return e
		}
		if run.Mode == "automatic" {
			if e = requireAnyDerivedTx(ctx, tx, a, viewScope, "channels.control"); e != nil {
				return e
			}
		} else if run.Mode == "backfill" {
			if e = requireAnyDerivedTx(ctx, tx, a, viewScope, "channels.backfill"); e != nil {
				return e
			}
		} else if e = requireAnyDerivedTx(ctx, tx, a, viewScope, "channels.manage", "channels.control"); e != nil {
			return e
		}
		if _, _, _, e = s.calculationConfigTx(ctx, tx, a, viewScope, run.ChannelID, run.ConfigID); e != nil {
			return e
		}
	}
	if e = requireAnyDerivedTx(ctx, tx, a, viewScope, "runs.read"); e != nil {
		return e
	}
	if e = requireAnyDerivedTx(ctx, tx, a, viewScope, "channels.read"); e != nil {
		return e
	}
	if _, e = s.channelTx(ctx, tx, a, viewScope, run.ChannelID); e != nil {
		return e
	}
	if run.Mode == "automatic" || run.Mode == "backfill" {
		output, e := run.OutputRange.rangeValue()
		if e != nil {
			return e
		}
		if e = s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, a, scope, run.ChannelID, []monitoring.Window{rangeWindow(output)}, false); e != nil {
			return e
		}
	}
	if e = s.authorizeInputsTx(ctx, tx, a, scope, run.Needs, false); e != nil {
		return e
	}
	if e = s.authorizeBaselineNeedsTx(ctx, tx, a, scope, run.ConfigID); e != nil {
		return e
	}
	var snapshot string
	e = tx.QueryRow(ctx, `SELECT snapshot_id::text FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, run.ID).Scan(&snapshot)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	_, e = s.checkUserInputsTx(ctx, tx, a, scope, snapshot, false)
	var stale *monitoring.Error
	if errors.As(e, &stale) && stale.Code == "snapshot_stale" {
		return nil
	}
	return e
}
func (s *Service) readRunTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, commandResult ...bool) (Run, error) {
	r, e := scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.id=$2 AND (r.scope_key=$3 OR r.mode='automatic')`, scope.Tenant, id, scope.key()))
	if e != nil {
		return Run{}, e
	}
	var task monitoring.Task
	if len(commandResult) > 0 && commandResult[0] {
		task, e = s.monitoring.TaskCommandResultTx(ctx, tx, a, monitoringScope(scope), r.JobID)
	} else {
		task, e = s.monitoring.TaskTx(ctx, tx, a, monitoringScope(scope), r.JobID, false)
	}
	if e != nil {
		return Run{}, e
	}
	r.Job = task.Job
	if r.Mode == "analysis" {
		return s.readAnalysisRunTx(ctx, tx, a, scope, r)
	}
	if r.Mode == "comparison" {
		return s.readComparisonTx(ctx, tx, a, scope, r)
	}
	if r.Mode == "historical_replay" {
		proof, e := s.replayProofTx(ctx, tx, a, scope, r)
		if e != nil {
			return Run{}, e
		}
		var result ReplayResult
		e = tx.QueryRow(ctx, `SELECT result FROM derived_replay_receipts WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, id).Scan(&result)
		if e == nil {
			result.InputState = proof.generation.InputState
			r.Replay = &result
		} else if e != pgx.ErrNoRows {
			return Run{}, e
		}
		return r.Run, nil
	}
	var snapshot string
	e = tx.QueryRow(ctx, `SELECT snapshot_id::text FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, id).Scan(&snapshot)
	var state monitoring.RawSnapshotState
	if e == nil {
		state, e = s.checkUserInputsTx(ctx, tx, a, monitoringScope(scope), snapshot, false)
		var stale *monitoring.Error
		if errors.As(e, &stale) && stale.Code == "snapshot_stale" {
			r.Job.ResultState = "input_invalidated"
			r.Job.Progress = jsonBytes(map[string]string{"phase": "input_invalidated"})
			return r.Run, nil
		}
		if e != nil {
			return Run{}, e
		}
	} else if e != pgx.ErrNoRows {
		return Run{}, e
	}
	var result TrialResult
	e = tx.QueryRow(ctx, `SELECT result,id::text,expires_at FROM derived_trial_receipts WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, id).Scan(&result, &result.ReceiptID, &result.ExpiresAt)
	if e == nil {
		result.Stale = result.Stale || state.Changed
		_, baselines, e := s.baselinesTx(ctx, tx, a, monitoringScope(scope), nil, r.ConfigID)
		if e != nil {
			return Run{}, e
		}
		if fingerprint(baselines) != fingerprint(result.Baselines) {
			r.Job.ResultState = "input_invalidated"
			r.Job.Progress = jsonBytes(map[string]string{"phase": "baseline_invalidated"})
			return r.Run, nil
		}
		r.Trial = &result
	} else if e != pgx.ErrNoRows {
		return Run{}, e
	} else if r.Mode == "trial" && r.Job.State == "succeeded" {
		r.Job.ResultState = "expired"
		r.Job.Progress = jsonBytes(map[string]string{"phase": "expired"})
	}
	return r.Run, nil
}

// Check the same resource, channel and execution mandate as the command endpoint.
func (s *Service) automaticControlTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, run Run) (string, error) {
	action := "retry"
	switch run.Job.State {
	case "queued", "running", "retry_wait":
		action = "cancel"
	case "failed", "needs_action", "cancelled":
	default:
		return "", nil
	}
	task, e := s.monitoring.TaskTx(ctx, tx, a, monitoringScope(scope), run.Job.ID, true)
	if e == nil && action == "retry" {
		e = s.AuthorizeExecutionTx(ctx, tx, task)
	}
	if e != nil {
		var domain interface{ StatusCode() int }
		if errors.As(e, &domain) && domain.StatusCode() < 500 {
			return "", nil
		}
		return "", e
	}
	if action == "cancel" {
		var pending bool
		if e = tx.QueryRow(ctx, `SELECT cancel_requested FROM monitoring_jobs WHERE id=$1`, run.Job.ID).Scan(&pending); e != nil || pending {
			return "", e
		}
	}
	return action, nil
}

func (s *Service) readRun(ctx context.Context, a access.Actor, scope Scope, id, action string) (Run, error) {
	var out Run
	if !access.ValidID(id) || s.monitoring == nil {
		return out, notFound()
	}
	e := s.transact(ctx, a, scope, action, false, func(tx pgx.Tx) error {
		var e error
		out, e = s.readRunTx(ctx, tx, a, scope, id, action == "channels.trials" || action == "generations.replay")
		if e == nil && action == "runs.detail" && out.Mode == "automatic" {
			if out.Job.State == "succeeded" {
				out.PublishedGenerationID, e = s.publishedRunGenerationTx(ctx, tx, a, scope, id)
				if errors.Is(e, pgx.ErrNoRows) || tasks.Denied(e) {
					return nil
				}
			} else {
				out.ControlAction, e = s.automaticControlTx(ctx, tx, a, scope, out)
			}
		}
		return e
	})
	return out, e
}
func (s *Service) Run(ctx context.Context, a access.Actor, scope Scope, id string) (Run, error) {
	return s.readRun(ctx, a, scope, id, "runs.detail")
}
func (s *Service) Runs(ctx context.Context, a access.Actor, scope Scope, f RunFilter) (RunPage, error) {
	out := RunPage{Items: []Run{}}
	if f.Limit == 0 {
		f.Limit = 30
	}
	if f.Limit < 1 || f.Limit > 100 || f.Mode != "" && f.Mode != "automatic" || f.ChannelID != "" && !access.ValidID(f.ChannelID) || !slices.Contains([]string{"", "queued", "running", "retry_wait", "needs_action", "succeeded", "failed", "cancelled"}, f.State) {
		return out, invalid("/filter", "运行筛选无效。")
	}
	keyFilter := f
	keyFilter.Cursor = ""
	key := fingerprint([]any{a.ID, scope, keyFilter})
	cursor, e := decodeCursor(f.Cursor, key)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "runs.list", false, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT r.id::text,r.created_at FROM derived_runs r JOIN monitoring_jobs j ON j.id=r.job_id WHERE r.tenant_id=$1 AND (r.scope_key=$2 OR r.mode='automatic') AND ($8='' OR r.mode=$8) AND ($3::uuid IS NULL OR r.channel_id=$3) AND ($4='' OR j.state=$4) AND ($5::timestamptz IS NULL OR (r.created_at,r.id)<($5::timestamptz,$6::uuid)) ORDER BY r.created_at DESC,r.id DESC LIMIT $7`, scope.Tenant, scope.key(), nullable(f.ChannelID), f.State, cursor.At, nullable(cursor.ID), f.Limit+1, f.Mode)
		if e != nil {
			return e
		}
		type item struct {
			id string
			at time.Time
		}
		items := []item{}
		for rows.Next() {
			var v item
			if e = rows.Scan(&v.id, &v.at); e != nil {
				rows.Close()
				return e
			}
			items = append(items, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(items) > f.Limit {
			out.NextCursor = encodeCursor(items[f.Limit-1].at, items[f.Limit-1].id, key)
			items = items[:f.Limit]
		}
		for _, v := range items {
			r, e := s.readRunTx(ctx, tx, a, scope, v.id)
			if e != nil {
				var ae *access.Error
				var de *Error
				var me *monitoring.Error
				var re *resources.Error
				if errors.As(e, &ae) || errors.As(e, &de) && (de.Status == 403 || de.Status == 404) || errors.As(e, &me) && (me.Status == 403 || me.Status == 404) || errors.As(e, &re) && (re.Status == 403 || re.Status == 404) {
					continue
				}
				return e
			}
			out.Items = append(out.Items, r)
		}
		return nil
	})
	return out, e
}
func (s *Service) ControlRun(ctx context.Context, a access.Actor, scope Scope, id, action string, in WriteMeta, audit access.Meta) (Run, error) {
	var out Run
	if e := checkMeta(in, true, true); e != nil {
		return out, e
	}
	if !access.ValidID(id) || !slices.Contains([]string{"cancel", "retry"}, action) {
		return out, notFound()
	}
	e := s.transact(ctx, a, scope, "runs."+action, true, func(tx pgx.Tx) error {
		r, e := scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.id=$2 AND (r.scope_key=$3 OR r.mode='automatic')`, scope.Tenant, id, scope.key()))
		if e != nil {
			return e
		}
		// Replays still recheck the task's original qualification.
		if _, e = s.monitoring.TaskTx(ctx, tx, a, monitoringScope(scope), r.JobID, true); e != nil {
			return e
		}
		var oldHash, oldAction string
		var old Run
		e = tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM derived_operation_receipts WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.key(), a.ID, in.RequestID).Scan(&oldAction, &oldHash, &old)
		hash := fingerprint([]any{id, action, in})
		if e == nil {
			if oldAction != "runs."+action || oldHash != hash {
				return problem("request_id_reused", "请求编号已用于不同操作。", 409)
			}
			out = old
			out.Replayed = true
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if action == "retry" {
			task, e := s.monitoring.TaskTx(ctx, tx, a, monitoringScope(scope), r.JobID, true)
			if e != nil {
				return e
			}
			if e = s.AuthorizeExecutionTx(ctx, tx, task); e != nil {
				return e
			}
		}
		job, e := s.monitoring.ControlTaskTx(ctx, tx, a, monitoringScope(scope), r.JobID, in.Revision, action, tasks.Command{RequestID: in.RequestID, Reason: in.Reason})
		if e != nil {
			return e
		}
		r.Job = job
		out = r.Run
		audit.ActorID = a.ID
		audit.ActorType = "account"
		audit.ActorScope = scope.Entry
		audit.TenantID = scope.Tenant
		audit.ProjectID = scope.Project
		audit.Reason = in.Reason
		if e = access.Audit(ctx, tx, "derived.runs."+action, "derived_run", id, "", nil, out, audit); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO derived_operation_receipts(tenant_id,project_id,actor_id,request_id,operation,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6,$7)`, scope.Tenant, nullable(scope.Project), a.ID, in.RequestID, "runs."+action, hash, jsonBytes(out))
		return e
	})
	return out, e
}

type inlineTrialKey struct{}

func incrementCount(m map[string]string, key string) {
	v, _ := strconv.ParseUint(m[key], 10, 64)
	m[key] = strconv.FormatUint(v+1, 10)
}
