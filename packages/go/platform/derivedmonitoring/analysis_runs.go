package derivedmonitoring

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AnalysisRunInput struct {
	WriteMeta
	ConfigVersionID string           `json:"config_version_id"`
	ConfigHash      string           `json:"config_hash"`
	OutputRange     NanosecondRange  `json:"output_range"`
	SourceRange     *NanosecondRange `json:"source_range,omitempty"`
	Outputs         []string         `json:"outputs,omitempty"`
	Quality         []string         `json:"quality,omitempty"`
	ArtifactID      string           `json:"artifact_id,omitempty"`
	ArtifactName    string           `json:"artifact_name,omitempty"`
}
type AnalysisRunInfo struct {
	Quality           []string         `json:"quality"`
	ConfigID          string           `json:"config_id"`
	ConfigVersionID   string           `json:"config_version_id"`
	ArtifactID        string           `json:"artifact_id,omitempty"`
	ArtifactVersionID string           `json:"artifact_version_id,omitempty"`
	Trial             bool             `json:"trial"`
	Outputs           []string         `json:"outputs"`
	SourceRange       *NanosecondRange `json:"source_range,omitempty"`
}
type analysisRunTarget struct {
	AnalysisRunInfo
	Rule json.RawMessage
}

func analysisRunTargetTx(ctx context.Context, tx pgx.Tx, tenant, id string) (analysisRunTarget, error) {
	var out analysisRunTarget
	var from, to *int64
	e := tx.QueryRow(ctx, `SELECT config_id::text,config_version_id::text,COALESCE(artifact_id::text,''),COALESCE(artifact_version_id::text,''),trial,outputs,lower(source_range),upper(source_range),run_rule FROM analysis_runs WHERE tenant_id=$1 AND run_id=$2`, tenant, id).Scan(&out.ConfigID, &out.ConfigVersionID, &out.ArtifactID, &out.ArtifactVersionID, &out.Trial, &out.Outputs, &from, &to, &out.Rule)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	if from != nil {
		r := rangeDTO(*from, to)
		out.SourceRange = &r
	}
	if e == nil {
		var rule Rule
		e = compute.Decode(out.Rule, &rule)
		out.Quality = rule.Policies.Quality
	}
	return out, e
}
func analysisSource(target *analysisRunTarget) *planning.Range {
	if target == nil || target.SourceRange == nil {
		return nil
	}
	r, _ := target.SourceRange.rangeValue()
	return &r
}
func (s *Service) StartAnalysis(ctx context.Context, a access.Actor, scope Scope, in AnalysisRunInput, trial bool, audit access.Meta) (Run, error) {
	var out Run
	if s.monitoring == nil || s.raw == nil || s.results == nil {
		return out, problem("dependency_unavailable", "分析任务与结果存储尚未配置。", 503)
	}
	if e := checkMeta(in.WriteMeta, false, false); e != nil {
		return out, e
	}
	if !access.ValidID(in.ConfigVersionID) || len(in.ConfigHash) != 64 {
		return out, invalid("/config_version_id", "需要固定配置版本及其 hash。")
	}
	output, e := in.OutputRange.rangeValue()
	if e != nil {
		return out, e
	}
	if output.To == nil || *output.To > time.Now().UnixNano() {
		return out, invalid("/output_range", "历史分析需要明确的过去范围。")
	}
	var source *planning.Range
	if in.SourceRange != nil {
		r, e := in.SourceRange.rangeValue()
		if e != nil {
			return out, e
		}
		source = &r
	}
	action := "analysis.runs"
	if trial {
		action = "analysis.trials"
		if in.ArtifactID != "" || in.ArtifactName != "" {
			return out, invalid("/artifact_id", "试算不发布成果。")
		}
	}
	e = s.transact(ctx, a, scope, action, true, func(tx pgx.Tx) error {
		found, e := analysisReceiptTx(ctx, tx, a, scope, action, in.WriteMeta, in, &out)
		if e != nil {
			return e
		}
		if found {
			out, e = s.readRunTx(ctx, tx, a, scope, out.ID, true)
			out.Replayed = true
			return e
		}
		version, e := analysisVersionTx(ctx, tx, scope, in.ConfigVersionID)
		if e != nil {
			return e
		}
		if version.ConfigHash != in.ConfigHash {
			return conflict()
		}
		config, e := analysisConfigTx(ctx, tx, scope, version.ConfigID, false)
		if e != nil {
			return e
		}
		if config.Status != "active" {
			return problem("analysis_archived", "分析配置已归档，不能启动新运行。", 409)
		}
		if version.EngineVersion != compute.EngineVersion || version.CompilerVersion != compute.CompilerVersion {
			return problem("engine_version_unavailable", "执行版本所需计算引擎不可用。", 409)
		}
		body := version.Body
		if in.Quality != nil {
			var rule Rule
			if e = compute.Decode(body.Rule, &rule); e != nil {
				return e
			}
			rule.Policies.Quality = in.Quality
			body.Rule = jsonBytes(rule)
		}
		p, body, e := s.compileAnalysisTx(ctx, tx, a, scope, body, false)
		if e != nil {
			return e
		}
		if p.Rule.Clock.Kind == "whole_range" {
			if source == nil || source.To == nil || *source.To != *output.To {
				return invalid("/source_range", "全范围统计需要明确源范围，其结束时刻必须等于输出标签。")
			}
		} else if source != nil {
			return invalid("/source_range", "仅全范围统计填写独立源范围。")
		}
		outputs := append([]string{}, in.Outputs...)
		if len(outputs) == 0 {
			for _, o := range p.Rule.Outputs {
				outputs = append(outputs, o.Key)
			}
		}
		known := map[string]bool{}
		for _, o := range p.Rule.Outputs {
			known[o.Key] = true
		}
		seen := map[string]bool{}
		for _, key := range outputs {
			if !known[key] || seen[key] {
				return invalid("/outputs", "需要已保存规则中不重复的输出项。")
			}
			seen[key] = true
		}
		slices.Sort(outputs)
		cfg := analysisCalculationConfig(version)
		cfg.Rule = body.Rule
		needs, e := immediateInputNeeds(cfg, p, output, source)
		if e != nil {
			return e
		}
		if e = s.authorizeInputsTx(ctx, tx, a, monitoringScope(scope), needs, false); e != nil {
			return e
		}
		if e = s.authorizeBaselineNeedsTx(ctx, tx, a, monitoringScope(scope), cfg.ID); e != nil {
			return e
		}
		artifact, artifactVersion := "", ""
		if !trial {
			artifact = in.ArtifactID
			artifactVersion = uuid.NewString()
			if artifact == "" {
				artifact = uuid.NewString()
				name := in.ArtifactName
				if name == "" {
					name = config.Name
				}
				if e = checkName(name, ""); e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `INSERT INTO analysis_artifacts(id,tenant_id,project_id,config_id,name,created_by) VALUES($1,$2,$3,$4,$5,$6)`, artifact, scope.Tenant, nullable(scope.Project), config.ID, name, a.ID); e != nil {
					return e
				}
			} else {
				if !access.ValidID(artifact) {
					return notFound()
				}
				var active bool
				e = tx.QueryRow(ctx, `SELECT status='active' FROM analysis_artifacts WHERE tenant_id=$1 AND scope_key=$2 AND config_id=$3 AND id=$4 FOR UPDATE`, scope.Tenant, scope.key(), config.ID, artifact).Scan(&active)
				if e == pgx.ErrNoRows {
					return notFound()
				}
				if e != nil {
					return e
				}
				if !active {
					return problem("analysis_archived", "成果已归档，不能添加新版本。", 409)
				}
			}
		}
		run, job := uuid.NewString(), uuid.NewString()
		var sourceSQL any
		if source != nil {
			sourceSQL = rangeSQL(*source)
		}
		e = tx.QueryRow(ctx, `INSERT INTO derived_runs(id,tenant_id,project_id,actor_id,mode,job_id,request_id,request_hash,output_range,source_range,config_revision,control_revision,mandate_revision,plan_hash,input_requirements,requested_at_ns,reason) VALUES($1,$2,$3,$4,'analysis',$5,$6,$7,$8::int8range,$9::int8range,$10,1,1,$11,$12,$13,$14) RETURNING created_at`, run, scope.Tenant, nullable(scope.Project), a.ID, job, in.RequestID, fingerprint(in), rangeSQL(output), sourceSQL, version.Number, p.PlanHash, jsonBytes(needs), time.Now().UnixNano(), in.Reason).Scan(&out.CreatedAt)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO analysis_runs(tenant_id,run_id,config_id,config_version_id,artifact_id,artifact_version_id,trial,outputs,source_range,run_rule) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::int8range,$10)`, scope.Tenant, run, config.ID, version.ID, nullable(artifact), nullable(artifactVersion), trial, jsonBytes(outputs), sourceSQL, body.Rule)
		if e != nil {
			return e
		}
		out.Job, e = s.monitoring.EnqueueTaskTx(ctx, tx, monitoring.EnqueueTask{ID: job, Scope: monitoringScope(scope), ActorID: a.ID, Kind: "analysis", Target: run, TargetRevision: version.Number, Input: jsonBytes(map[string]any{"mode": "analysis", "run_id": run, "trial": trial})})
		if e != nil {
			return e
		}
		out.ID = run
		out.Mode = "analysis"
		out.OutputRange = in.OutputRange
		out.PlanHash = p.PlanHash
		out.Analysis = &AnalysisRunInfo{ConfigID: config.ID, ConfigVersionID: version.ID, ArtifactID: artifact, ArtifactVersionID: artifactVersion, Trial: trial, Outputs: outputs, SourceRange: in.SourceRange}
		out.Analysis.Quality = p.Rule.Policies.Quality
		return analysisCompleteTx(ctx, tx, a, scope, action, "derived_run", run, in.WriteMeta, in, nil, out, audit)
	})
	return out, e
}
func (s *Service) authorizeAnalysisExecutionTx(ctx context.Context, tx pgx.Tx, task monitoring.Task, run runRecord) error {
	a := executionActor(task.ActorID)
	op := access.DerivedOperation(run.Scope.Entry, "analysis.runs")
	if run.Scope.Entry == "project" {
		if _, e := access.RequireProjectTx(ctx, tx, a.ID, op, run.Scope.Tenant, run.Scope.Project); e != nil {
			return e
		}
	} else if e := access.RequireScopedTx(ctx, tx, a.ID, op, run.Scope.Tenant); e != nil {
		return e
	}
	target, e := analysisRunTargetTx(ctx, tx, run.Scope.Tenant, run.ID)
	if e != nil {
		return e
	}
	v, e := analysisVersionTx(ctx, tx, run.Scope, target.ConfigVersionID)
	if e != nil {
		return e
	}
	if v.EngineVersion != compute.EngineVersion || v.CompilerVersion != compute.CompilerVersion {
		return problem("engine_version_unavailable", "原分析引擎不可用。", 409)
	}
	p, e := s.compile(target.Rule, "analysis", v.Body.Parameters)
	if e != nil {
		return e
	}
	if p.PlanHash != run.PlanHash {
		return problem("configuration_changed", "固定分析计划不匹配。", 409)
	}
	if !target.Trial {
		var active bool
		if e = tx.QueryRow(ctx, `SELECT status='active' FROM analysis_artifacts WHERE tenant_id=$1 AND id=$2 AND scope_key=$3`, run.Scope.Tenant, target.ArtifactID, run.Scope.key()).Scan(&active); e != nil {
			return e
		}
		if !active {
			return problem("analysis_archived", "成果已归档。", 409)
		}
	}
	if e = s.authorizeInputsTx(ctx, tx, a, task.Scope, run.Needs, false); e != nil {
		return e
	}
	if e = s.authorizeBaselineNeedsTx(ctx, tx, a, task.Scope, v.ID); e != nil {
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
	_, e = s.checkAnalysisInputsTx(ctx, tx, a, task.Scope, snapshot, false)
	return e
}
func (s *Service) checkAnalysisInputsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, id string, history bool, capabilities ...string) (monitoring.RawSnapshotState, error) {
	state, e := s.monitoring.CheckRawSnapshotTx(ctx, tx, a, scope, id, history, capabilities...)
	if e != nil {
		return state, e
	}
	capability := ""
	if len(capabilities) > 0 {
		capability = capabilities[0]
	}
	upstream, e := s.checkInputLedgerTx(ctx, tx, inputAccess{Actor: a, Scope: scope, History: history, AllowChanged: true, Capability: capability}, id)
	if e != nil {
		return state, e
	}
	state.Changed = state.Changed || upstream.Changed || !upstream.Current
	state.Revoked = state.Revoked || upstream.Revoked
	if state.Revoked && !history {
		return state, problem("snapshot_stale", "固定来源已撤销，请建立新的分析运行。", 409)
	}
	return state, nil
}
func (s *Service) authorizeAnalysisTaskTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, run runRecord, manage bool) error {
	if run.Scope != scope {
		return notFound()
	}
	if e := requireAnyDerivedTx(ctx, tx, a, scope, "runs.read"); e != nil {
		return e
	}
	if e := requireAnyDerivedTx(ctx, tx, a, scope, "analysis.read"); e != nil {
		return e
	}
	if manage {
		if e := requireAnyDerivedTx(ctx, tx, a, scope, "runs.manage"); e != nil {
			return e
		}
		if e := requireAnyDerivedTx(ctx, tx, a, scope, "analysis.run"); e != nil {
			return e
		}
	}
	target, e := analysisRunTargetTx(ctx, tx, scope.Tenant, run.ID)
	if e != nil {
		return e
	}
	v, e := analysisVersionTx(ctx, tx, scope, target.ConfigVersionID)
	if e != nil {
		return e
	}
	if _, e = s.analysisBindingsTx(ctx, tx, a, scope, v.Body); e != nil {
		return e
	}
	if e = s.authorizeInputsTx(ctx, tx, a, monitoringScope(scope), run.Needs, false); e != nil {
		return e
	}
	return s.authorizeBaselineNeedsTx(ctx, tx, a, monitoringScope(scope), v.ID)
}
func (s *Service) readAnalysisRunTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, run runRecord) (Run, error) {
	target, e := analysisRunTargetTx(ctx, tx, scope.Tenant, run.ID)
	if e != nil {
		return Run{}, e
	}
	run.Run.Analysis = &target.AnalysisRunInfo
	var snapshot string
	e = tx.QueryRow(ctx, `SELECT snapshot_id::text FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, run.ID).Scan(&snapshot)
	if e == pgx.ErrNoRows {
		return run.Run, nil
	}
	if e != nil {
		return Run{}, e
	}
	state, e := s.checkAnalysisInputsTx(ctx, tx, a, monitoringScope(scope), snapshot, false)
	var raw *monitoring.Error
	var domain *Error
	if errors.As(e, &raw) && raw.Code == "snapshot_stale" || errors.As(e, &domain) && domain.Code == "snapshot_stale" {
		run.Job.ResultState = "input_invalidated"
		run.Job.Progress = jsonBytes(map[string]string{"phase": "input_invalidated"})
		return run.Run, nil
	}
	if e != nil {
		return Run{}, e
	}
	if target.Trial {
		var trial TrialResult
		e = tx.QueryRow(ctx, `SELECT result FROM analysis_trial_receipts WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, run.ID).Scan(&trial)
		if e == pgx.ErrNoRows {
			if run.Job.State == "succeeded" {
				run.Job.ResultState = "expired"
				run.Job.Progress = jsonBytes(map[string]string{"phase": "expired"})
			}
			return run.Run, nil
		}
		if e != nil {
			return Run{}, e
		}
		_, currentBaselines, e := s.baselinesTx(ctx, tx, a, monitoringScope(scope), nil, target.ConfigVersionID)
		if e != nil {
			return Run{}, e
		}
		if fingerprint(currentBaselines) != fingerprint(trial.Baselines) {
			run.Job.ResultState = "input_invalidated"
			run.Job.Progress = jsonBytes(map[string]string{"phase": "baseline_invalidated"})
			return run.Run, nil
		}
		trial.Stale = trial.Stale || state.Changed
		run.Trial = &trial
	}
	return run.Run, nil
}
