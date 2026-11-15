package derivedmonitoring

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type BackfillPreviewInput struct {
	Revision           string                        `json:"expected_revision"`
	Range              NanosecondRange               `json:"output_range"`
	UnassignedConfigID string                        `json:"unassigned_config_id,omitempty"`
	Dependencies       []BackfillDependencySelection `json:"dependencies,omitempty"`
}
type BackfillDependencySelection struct {
	ChannelID          string          `json:"channel_id"`
	Revision           string          `json:"expected_revision"`
	Range              NanosecondRange `json:"output_range"`
	UnassignedConfigID string          `json:"unassigned_config_id,omitempty"`
}
type BackfillTarget struct {
	ConfigID   string               `json:"config_id"`
	ConfigHash string               `json:"config_hash"`
	PlanHash   string               `json:"compiled_plan_hash"`
	Range      NanosecondRange      `json:"output_range"`
	Needs      []monitoring.RawNeed `json:"input_ranges"`
	Assign     bool                 `json:"creates_assignment"`
}
type BackfillPreview struct {
	Selections         []BackfillDependencySelection `json:"dependency_selections,omitempty"`
	Dependencies       []BackfillPreview             `json:"dependencies,omitempty"`
	ID                 string                        `json:"id"`
	Hash               string                        `json:"hash"`
	ChannelID          string                        `json:"channel_id"`
	ConfigRevision     string                        `json:"config_revision"`
	ControlRevision    string                        `json:"control_revision"`
	GraphRevision      string                        `json:"graph_revision"`
	Range              NanosecondRange               `json:"output_range"`
	UnassignedConfigID string                        `json:"unassigned_config_id,omitempty"`
	Targets            []BackfillTarget              `json:"targets"`
	ExpiresAt          time.Time                     `json:"expires_at"`
}
type BackfillResult struct {
	TaskRef  *taskrecord.Ref `json:"task_ref,omitempty"`
	Runs     []Run           `json:"runs"`
	Replayed bool            `json:"replayed"`
}

func backfillHash(v BackfillPreview) string {
	v.ID = ""
	v.Hash = ""
	v.ExpiresAt = time.Time{}
	return fingerprint(v)
}
func (s *Service) prepareBackfillTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string, in BackfillPreviewInput) (BackfillPreview, error) {
	out := BackfillPreview{ChannelID: id, Range: in.Range, UnassignedConfigID: in.UnassignedConfigID, Targets: []BackfillTarget{}}
	channel, e := s.maintenanceTargetTx(ctx, tx, a, scope, id, true)
	if e != nil {
		return out, e
	}
	if channel.ConfigRevision != in.Revision {
		return out, conflict()
	}
	if channel.ControlState == "draft" {
		return out, problem("invalid_control_transition", "先启用并建立配置分配，再申请正式回算。", 409)
	}
	r, e := in.Range.rangeValue()
	if e != nil {
		return out, e
	}
	if r.To == nil || *r.To > time.Now().UnixNano() {
		return out, invalid("/output_range", "手动回算需要已经到达的有限时间范围。")
	}
	if e = checkRetiredRange(channel, r); e != nil {
		return out, e
	}
	if e = s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, a, monitoringScope(scope), id, []monitoring.Window{rangeWindow(r)}, false); e != nil {
		return out, e
	}
	assignments, e := activeAssignmentsTx(ctx, tx, scope.Tenant, id)
	if e != nil {
		return out, e
	}
	covered := []planning.Range{}
	for _, assignment := range assignments {
		interval, _ := assignment.Range.rangeValue()
		part, ok := planning.Intersection(interval, r)
		if !ok {
			continue
		}
		_, cfg, p, e := s.calculationConfigTx(ctx, tx, a, scope, id, assignment.ConfigID)
		if e != nil {
			return out, e
		}
		needs, e := s.plannedInputsTx(ctx, tx, scope.Tenant, cfg, p, part)
		if e != nil {
			return out, e
		}
		if e = s.authorizeInputsTx(ctx, tx, a, monitoringScope(scope), needs, false); e != nil {
			return out, e
		}
		if e = s.authorizeBaselineNeedsTx(ctx, tx, a, monitoringScope(scope), cfg.ID); e != nil {
			return out, e
		}
		out.Targets = append(out.Targets, BackfillTarget{ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, PlanHash: p.PlanHash, Range: rangeDTO(part.From, part.To), Needs: needs})
		covered = append(covered, part)
	}
	complete, e := planning.Covered([]planning.Range{r}, covered)
	if e != nil {
		return out, e
	}
	if !complete {
		if !access.ValidID(in.UnassignedConfigID) {
			return out, invalid("/unassigned_config_id", "此历史范围没有配置分配，必须明确选择所用配置。")
		}
		if e = requireAnyDerivedTx(ctx, tx, a, scope, "channels.manage"); e != nil {
			return out, e
		}
		_, cfg, p, e := s.calculationConfigTx(ctx, tx, a, scope, id, in.UnassignedConfigID)
		if e != nil {
			return out, e
		}
		if _, _, e = s.baselinesTx(ctx, tx, a, monitoringScope(scope), nil, cfg.ID); e != nil {
			return out, e
		}
		missing := []planning.Range{r}
		for _, part := range covered {
			next := []planning.Range{}
			for _, hole := range missing {
				next = append(next, planning.Subtract(hole, part)...)
			}
			missing = next
		}
		for _, part := range missing {
			needs, e := s.plannedInputsTx(ctx, tx, scope.Tenant, cfg, p, part)
			if e != nil {
				return out, e
			}
			if e = s.authorizeInputsTx(ctx, tx, a, monitoringScope(scope), needs, false); e != nil {
				return out, e
			}
			out.Targets = append(out.Targets, BackfillTarget{ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, PlanHash: p.PlanHash, Range: rangeDTO(part.From, part.To), Needs: needs, Assign: true})
		}
	}
	if len(out.Targets) > 1000 {
		return out, problem("plan_limit_exceeded", "本次回算涉及超过1000个配置段，请分段申请。", 422)
	}
	out.ConfigRevision = channel.ConfigRevision
	out.ControlRevision = channel.ControlRevision
	e = tx.QueryRow(ctx, `SELECT revision::text FROM derived_graph_states WHERE tenant_id=$1`, scope.Tenant).Scan(&out.GraphRevision)
	if e == nil && len(in.Dependencies) > 0 {
		e = s.prepareBackfillDependenciesTx(ctx, tx, a, scope, &out, in.Dependencies)
	}
	return out, e
}
func (s *Service) PreviewBackfill(ctx context.Context, a access.Actor, scope Scope, id string, in BackfillPreviewInput) (BackfillPreview, error) {
	var out BackfillPreview
	if s.monitoring == nil {
		return out, problem("dependency_unavailable", "监测任务宿主尚未配置。", 503)
	}
	if !access.ValidID(id) {
		return out, notFound()
	}
	if _, e := revisionNumber(in.Revision); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "channels.backfill-preview", true, func(tx pgx.Tx) error {
		var e error
		out, e = s.prepareBackfillTx(ctx, tx, a, scope, id, in)
		if e != nil {
			return e
		}
		out.Hash = backfillHash(out)
		out.ID = uuid.NewString()
		return tx.QueryRow(ctx, `INSERT INTO derived_backfill_previews(id,tenant_id,project_id,actor_id,channel_id,proposal,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING expires_at`, out.ID, scope.Tenant, nullable(scope.Project), a.ID, id, jsonBytes(out), out.Hash).Scan(&out.ExpiresAt)
	})
	return out, e
}
func (s *Service) BackfillChannel(ctx context.Context, a access.Actor, scope Scope, id string, in ControlInput, audit access.Meta) (BackfillResult, error) {
	out := BackfillResult{Runs: []Run{}}
	if e := checkMeta(in.WriteMeta, true, true); e != nil {
		return out, e
	}
	if !access.ValidID(id) || !access.ValidID(in.PreviewID) || len(in.PreviewHash) != 64 {
		return out, invalid("/preview_id", "需要完整回算预览。")
	}
	e := s.transact(ctx, a, scope, "channels.backfill", true, func(tx pgx.Tx) error {
		_, e := s.maintenanceTargetTx(ctx, tx, a, scope, id, true)
		if e != nil {
			return e
		}
		hash := fingerprint([]any{id, in})
		var action, oldHash string
		var raw []byte
		e = tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM derived_operation_receipts WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.key(), a.ID, in.RequestID).Scan(&action, &oldHash, &raw)
		if e == nil {
			if action != "channels.backfill" || oldHash != hash {
				return problem("idempotency_conflict", "请求编号已用于不同操作。", 409)
			}
			if e = json.Unmarshal(raw, &out); e != nil {
				return e
			}
			for _, run := range out.Runs {
				if _, e = s.monitoring.TaskTx(ctx, tx, a, monitoringScope(scope), run.Job.ID, false); e != nil {
					return e
				}
			}
			out.Replayed = true
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		var saved BackfillPreview
		var savedHash string
		var fresh bool
		e = tx.QueryRow(ctx, `SELECT proposal,content_hash,expires_at>clock_timestamp() FROM derived_backfill_previews WHERE tenant_id=$1 AND project_id IS NOT DISTINCT FROM $2::uuid AND actor_id=$3 AND channel_id=$4 AND id=$5`, scope.Tenant, nullable(scope.Project), a.ID, id, in.PreviewID).Scan(&saved, &savedHash, &fresh)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		if e != nil {
			return e
		}
		if !fresh {
			return problem("preview_expired", "回算预览已过期。", 409)
		}
		if savedHash != in.PreviewHash {
			return problem("preview_changed", "回算预览不匹配。", 409)
		}
		preview, e := s.prepareBackfillTx(ctx, tx, a, scope, id, BackfillPreviewInput{Revision: in.Revision, Range: saved.Range, UnassignedConfigID: saved.UnassignedConfigID, Dependencies: saved.Selections})
		if e != nil {
			return e
		}
		if backfillHash(preview) != savedHash {
			return problem("preview_changed", "配置分配或依赖图已变化。", 409)
		}
		out.Runs, e = s.enqueueBackfillGroupTx(ctx, tx, a, scope, preview, in.WriteMeta)
		if e != nil {
			return e
		}

		out.TaskRef, e = s.registerTaskGroupTx(ctx, tx, a, scope, in.WriteMeta, out.Runs)
		if e != nil {
			return e
		}
		return configOperationTx(ctx, tx, a, scope, id, "channels.backfill", in.WriteMeta, hash, preview, out, audit)
	})
	return out, e
}

func (s *Service) authorizeBackfillExecutionTx(ctx context.Context, tx pgx.Tx, task monitoring.Task, run runRecord) error {
	a := executionActor(task.ActorID)
	if e := a.Check(ctx, tx, false); e != nil {
		return e
	}
	op := access.DerivedOperation(run.Scope.Entry, "channels.backfill")
	if run.Scope.Entry == "project" {
		if _, e := access.RequireProjectTx(ctx, tx, a.ID, op, run.Scope.Tenant, run.Scope.Project); e != nil {
			return e
		}
	} else if e := access.RequireScopedTx(ctx, tx, a.ID, op, run.Scope.Tenant); e != nil {
		return e
	}
	c, _, p, e := s.calculationConfigTx(ctx, tx, a, run.Scope, run.ChannelID, run.ConfigID)
	if e != nil {
		return e
	}
	if c.ConfigRevision != run.ConfigRevision || c.ControlRevision != run.ControlRevision || c.ControlState == "draft" || p.PlanHash != run.PlanHash {
		return &monitoring.TaskError{Code: "configuration_changed", Message: "回算期间配置或控制状态已经变化。", Disposition: "cancelled"}
	}
	r, e := run.OutputRange.rangeValue()
	if e != nil {
		return e
	}
	if e = checkRetiredRange(c, r); e != nil {
		return e
	}
	var active bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_config_assignments WHERE tenant_id=$1 AND channel_id=$2 AND config_id=$3 AND superseded_revision IS NULL AND effective_range @> $4::int8range)`, run.Scope.Tenant, run.ChannelID, run.ConfigID, rangeSQL(r)).Scan(&active); e != nil {
		return e
	}
	if !active {
		return access.ErrDenied
	}
	if e = s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, a, task.Scope, run.ChannelID, []monitoring.Window{rangeWindow(r)}, false); e != nil {
		return e
	}
	if e = s.authorizeInputsTx(ctx, tx, a, task.Scope, run.Needs, false); e != nil {
		return e
	}
	if e = s.authorizeBaselineNeedsTx(ctx, tx, a, task.Scope, run.ConfigID); e != nil {
		return e
	}
	if e = s.requireRunDependenciesTx(ctx, tx, run.Scope.Tenant, run.ID); e != nil {
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
	_, e = s.checkRunInputsTx(ctx, tx, a, run, snapshot)
	return e
}

func checkRetiredRange(channel Channel, r planning.Range) error {
	if channel.ControlState != "retired" {
		return nil
	}
	if channel.StopNS == nil {
		return problem("result_integrity_unavailable", "退役通道缺少停止边界。", 503)
	}
	stop, e := strconv.ParseInt(*channel.StopNS, 10, 64)
	if e != nil {
		return e
	}
	if r.To == nil || *r.To > stop {
		return invalid("/output_range", "退役通道只能回算或修正停止边界之前的历史。")
	}
	return nil
}

func (s *Service) assignBackfillGapsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, channel Channel, preview BackfillPreview, meta WriteMeta) (Channel, string, error) {
	changes := []monitoring.ChangeRange{}
	for _, target := range preview.Targets {
		if target.Assign {
			r, _ := target.Range.rangeValue()
			changes = append(changes, monitoring.ChangeRange{Channel: channel.Resource.ID, From: &r.From, To: r.To})
		}
	}
	if len(changes) == 0 {
		return channel, preview.GraphRevision, nil
	}
	if e := requireAnyDerivedTx(ctx, tx, a, scope, "channels.manage"); e != nil {
		return channel, "", e
	}
	revision, e := strconv.ParseInt(channel.ConfigRevision, 10, 64)
	if e != nil {
		return channel, "", e
	}
	revision++
	for _, target := range preview.Targets {
		if target.Assign {
			r, _ := target.Range.rangeValue()
			if _, e = tx.Exec(ctx, `INSERT INTO derived_config_assignments(tenant_id,channel_id,config_id,effective_range,created_revision,request_id) VALUES($1,$2,$3,$4::int8range,$5,$6)`, scope.Tenant, channel.Resource.ID, target.ConfigID, rangeSQL(r), revision, meta.RequestID); e != nil {
				return channel, "", e
			}
		}
	}
	var graph string
	if e = tx.QueryRow(ctx, `UPDATE derived_graph_states SET revision=revision+1 WHERE tenant_id=$1 RETURNING revision::text`, scope.Tenant).Scan(&graph); e != nil {
		return channel, "", e
	}
	if _, e = tx.Exec(ctx, `UPDATE derived_channels SET config_revision=$3 WHERE tenant_id=$1 AND channel_id=$2`, scope.Tenant, channel.Resource.ID, revision); e != nil {
		return channel, "", e
	}
	channel.ConfigRevision = strconv.FormatInt(revision, 10)
	if _, e = tx.Exec(ctx, `UPDATE monitoring_jobs j SET state='cancelled',cancel_requested=true,fence=fence+1,lease_owner=NULL,lease_until=NULL,revision=j.revision+1,updated_at=clock_timestamp() FROM derived_runs r WHERE r.tenant_id=$1 AND r.channel_id=$2 AND r.job_id=j.id AND r.mode='automatic' AND j.state IN ('queued','running','retry_wait','needs_action')`, scope.Tenant, channel.Resource.ID); e != nil {
		return channel, "", e
	}
	var position int64
	if e = tx.QueryRow(ctx, `UPDATE monitoring_tenant_states SET commit_sequence=commit_sequence+1 WHERE tenant_id=$1 RETURNING commit_sequence`, scope.Tenant).Scan(&position); e != nil {
		return channel, "", e
	}
	_, e = monitoring.AppendChangeTx(ctx, tx, scope.Tenant, position, "config_assignment_change", "derived", channel.Resource.ID, changes)
	return channel, graph, e
}

func (s *Service) enqueueBackfillTargetsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, channel Channel, preview BackfillPreview, meta WriteMeta, position string) ([]Run, error) {
	out := []Run{}
	var e error
	for _, target := range preview.Targets {
		runID, jobID, generation := uuid.NewString(), uuid.NewString(), uuid.NewString()
		r, _ := target.Range.rangeValue()
		targetHash := fingerprint([]any{"manual-backfill/1", scope, a.ID, meta.RequestID, target})
		var created time.Time
		e = tx.QueryRow(ctx, `INSERT INTO derived_runs(id,tenant_id,project_id,actor_id,channel_id,config_id,mode,job_id,request_id,request_hash,output_range,config_revision,control_revision,mandate_revision,plan_hash,input_requirements,requested_at_ns,reason) VALUES($1,$2,$3,$4,$5,$6,'backfill',$7,$8,$9,$10::int8range,$11,$12,$13,$14,$15,$16,$17) RETURNING created_at`, runID, scope.Tenant, nullable(scope.Project), a.ID, channel.Resource.ID, target.ConfigID, jobID, uuid.NewString(), targetHash, rangeSQL(r), channel.ConfigRevision, channel.ControlRevision, channel.MandateRevision, target.PlanHash, jsonBytes(target.Needs), time.Now().UnixNano(), meta.Reason).Scan(&created)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO derived_run_publication_targets(tenant_id,run_id,channel_id,generation_id,target_hash,required_position,graph_revision) VALUES($1,$2,$3,$4,$5,$6,$7)`, scope.Tenant, runID, channel.Resource.ID, generation, targetHash, position, preview.GraphRevision); e != nil {
			return nil, e
		}
		job, e := s.monitoring.EnqueueTaskTx(ctx, tx, monitoring.EnqueueTask{ID: jobID, Scope: monitoringScope(scope), ActorID: a.ID, Kind: "derive", Target: runID, TargetRevision: channel.ConfigRevision, Input: jsonBytes(map[string]string{"mode": "backfill", "run_id": runID})})
		if e != nil {
			return nil, e
		}
		out = append(out, Run{ID: runID, ChannelID: channel.Resource.ID, ConfigID: target.ConfigID, Mode: "backfill", OutputRange: target.Range, PlanHash: target.PlanHash, CreatedAt: created, Job: job})
	}
	return out, nil
}
