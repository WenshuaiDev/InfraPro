package derivedmonitoring

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

func (s *Service) executeAnalysis(ctx context.Context, lease monitoring.TaskLease) error {
	if s.results == nil {
		return problem("calculation_storage_unavailable", "分析结果存储不可用。", 503)
	}
	task := lease.Task()
	actor := lease.Actor()
	var run runRecord
	var version AnalysisVersion
	var target analysisRunTarget
	var p *compute.Plan
	var cfg ChannelConfig
	var snapshotID string
	e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		run, e = scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.id=$2 AND r.job_id=$3`, task.Scope.Tenant, task.Job.Target, task.Job.ID))
		if e != nil {
			return e
		}
		target, e = analysisRunTargetTx(ctx, tx, task.Scope.Tenant, run.ID)
		if e != nil {
			return e
		}
		version, e = analysisVersionTx(ctx, tx, run.Scope, target.ConfigVersionID)
		if e != nil {
			return e
		}
		cfg = analysisCalculationConfig(version)
		cfg.Rule = target.Rule
		p, e = s.compile(target.Rule, "analysis", version.Body.Parameters)
		if e != nil {
			return e
		}
		if p.PlanHash != run.PlanHash {
			return problem("configuration_changed", "分析计划不匹配。", 409)
		}
		run.ConfigID = version.ID
		run.Analysis = &target
		e = tx.QueryRow(ctx, `SELECT snapshot_id::text FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2`, run.Scope.Tenant, run.ID).Scan(&snapshotID)
		if e == pgx.ErrNoRows {
			return nil
		}
		return e
	})
	if e != nil {
		return e
	}
	basePlan, e := s.compile(version.Body.Rule, "analysis", version.Body.Parameters)
	if e != nil {
		return e
	}
	if e = s.freezeBaselines(ctx, lease, analysisCalculationConfig(version), basePlan); e != nil {
		return e
	}
	output, e := run.OutputRange.rangeValue()
	if e != nil {
		return e
	}
	if snapshotID == "" {
		snapshot, e := s.freezeRunInputs(ctx, actor, run, monitoring.RawFreezeRequest{Needs: run.Needs, Prefixes: inputPrefixes(p, cfg, run.Needs, output, analysisSource(&target))})
		if e != nil {
			return e
		}
		if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
			if _, e := s.checkAnalysisInputsTx(ctx, tx, actor, task.Scope, snapshot.ID, false); e != nil {
				return e
			}
			_, e := tx.Exec(ctx, `INSERT INTO derived_run_raw_inputs(tenant_id,run_id,snapshot_id) VALUES($1,$2,$3)`, run.Scope.Tenant, run.ID, snapshot.ID)
			return e
		}); e != nil {
			return e
		}
		snapshotID = snapshot.ID
	}
	input, e := s.inspectRunInputs(ctx, actor, run, snapshotID)
	if e != nil {
		return e
	}
	monitoring.RecordTaskIdentity(ctx, map[string]string{"run_id": run.ID, "generation_id": target.ArtifactVersionID, "source_position": input.Snapshot.SourcePosition})
	if target.Trial {
		trial, e := s.calculateTrial(ctx, lease, run, cfg, p, input, snapshotID)
		if e != nil {
			return e
		}
		return lease.Transaction(ctx, func(tx pgx.Tx) error {
			state, e := s.checkAnalysisInputsTx(ctx, tx, actor, task.Scope, snapshotID, false)
			if e != nil {
				return e
			}
			_, currentBaselines, e := s.runBaselinesTx(ctx, tx, actor, run)
			if e != nil {
				return e
			}
			if fingerprint(currentBaselines) != fingerprint(trial.Baselines) {
				return problem("snapshot_stale", "试算期间固定基准已变化，请重新试算。", 409)
			}
			trial.Stale = state.Changed
			trial.ReceiptID = run.ID
			trial.ExpiresAt = time.Now().Add(s.options.TrialTTL)
			if _, e = tx.Exec(ctx, `INSERT INTO analysis_trial_receipts(tenant_id,run_id,result) VALUES($1,$2,$3)`, run.Scope.Tenant, run.ID, jsonBytes(trial)); e != nil {
				return e
			}
			e = lease.CompleteTx(ctx, tx, map[string]string{"phase": "completed", "output_count": trial.OutputCount})
			return e
		})
	}
	var baselines map[string]compute.FixedBaseline
	var proofs []BaselineProof
	if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		baselines, proofs, e = s.runBaselinesTx(ctx, tx, actor, run)
		return e
	}); e != nil {
		return e
	}
	lineage, e := s.results.SealDocument(ctx, run.Scope.Tenant, "lineage", map[string]any{"schema": "analysis-lineage/1", "run_id": run.ID, "config_version_id": cfg.ID, "config_hash": cfg.ConfigHash, "plan_hash": p.PlanHash, "engine_version": version.EngineVersion, "compiler_version": version.CompilerVersion, "raw_snapshot": input.Snapshot, "upstream_inputs": input.Sources, "input_ranges": input.Needs, "baselines": proofs, "output_range": run.OutputRange, "source_range": target.SourceRange, "outputs": target.Outputs})
	if e != nil {
		return e
	}
	manifest := resultManifest{Schema: "analysis-result-manifest/1", Generation: target.ArtifactVersionID, Run: run.ID, Artifact: target.ArtifactID, Config: cfg.ID, Plan: p.PlanHash, Range: run.OutputRange, SourceRange: target.SourceRange, Input: input.Snapshot, Lineage: lineage, Baselines: proofs, Sources: input.Sources, Blocks: []resultstore.Block{}, States: map[string]string{}, Outputs: []Output{}}
	for _, o := range p.Rule.Outputs {
		if slices.Contains(target.Outputs, o.Key) {
			manifest.Outputs = append(manifest.Outputs, o)
		}
	}
	if e = s.calculateManifestParts(ctx, lease, run, cfg, p, publicationTarget{Generation: target.ArtifactVersionID}, input, lineage, &manifest, baselines); e != nil {
		return e
	}
	if e = lease.Progress(ctx, map[string]string{"phase": "verifying", "output_rows": strconv.FormatUint(manifest.Rows, 10)}); e != nil {
		return e
	}
	root, e := s.results.SealDocument(ctx, run.Scope.Tenant, "manifests", manifest)
	if e != nil {
		return e
	}
	return s.publishAnalysis(ctx, lease, run, manifest, root, input)
}
func (s *Service) publishAnalysis(ctx context.Context, lease monitoring.TaskLease, run runRecord, manifest resultManifest, root string, input inputSnapshotDetails) error {
	// Sealed result blocks are still invisible until this one short PG commit.
	// The lease fence and current input rights are rechecked in the transaction.
	return lease.Transaction(ctx, func(tx pgx.Tx) error {
		state, e := s.checkAnalysisInputsTx(ctx, tx, lease.Actor(), lease.Task().Scope, input.Snapshot.ID, false)
		if e != nil {
			return e
		}
		_, current, e := s.runBaselinesTx(ctx, tx, lease.Actor(), run)
		if e != nil {
			return e
		}
		if fingerprint(current) != fingerprint(manifest.Baselines) {
			return problem("snapshot_stale", "运行期间固定基准状态已改变，请重新运行。", 409)
		}
		for _, block := range manifest.Blocks {
			var sealed bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_write_attempts WHERE tenant_id=$1 AND run_id=$2 AND id=$3 AND chunk_id=$4 AND state='sealed' AND manifest=$5::jsonb)`, run.Scope.Tenant, run.ID, block.Attempt, block.Chunk, jsonBytes(block)).Scan(&sealed); e != nil {
				return e
			}
			if !sealed {
				return problem("result_integrity_unavailable", "分析输出没有完整封存依据。", 503)
			}
		}
		var number int64
		if e = tx.QueryRow(ctx, `SELECT latest_version FROM analysis_artifacts WHERE tenant_id=$1 AND id=$2 AND scope_key=$3 AND status='active' FOR UPDATE`, run.Scope.Tenant, manifest.Artifact, run.Scope.key()).Scan(&number); e != nil {
			return e
		}
		output, _ := run.OutputRange.rangeValue()
		_, e = tx.Exec(ctx, `INSERT INTO analysis_artifact_versions(id,tenant_id,artifact_id,version_no,run_id,config_version_id,output_range,raw_snapshot_id,source_position,manifest_root,lineage_root,output_rows,stale_at_publication) VALUES($1,$2,$3,$4,$5,$6,$7::int8range,$8,$9,$10,$11,$12,$13)`, manifest.Generation, run.Scope.Tenant, manifest.Artifact, number+1, run.ID, run.ConfigID, rangeSQL(output), manifest.Input.ID, manifest.Input.SourcePosition, root, manifest.Lineage, int64(manifest.Rows), state.Changed)
		if e != nil {
			return e
		}
		needs, e := s.analysisAffectedNeedsTx(ctx, tx, run.Scope.Tenant, input, run.ConfigID, manifest.Baselines)
		if e != nil {
			return e
		}
		for _, need := range needs {
			r, e := rawNeedRange(need)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO analysis_artifact_input_ranges(tenant_id,version_id,channel_id,required_range) VALUES($1,$2,$3,$4::int8range)`, run.Scope.Tenant, manifest.Generation, need.Channel, rangeSQL(r)); e != nil {
				return e
			}
		}
		if e = markAnalysisChangesTx(ctx, tx, run.Scope.Tenant, manifest.Generation, ""); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE analysis_artifacts SET latest_version=$3,revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, run.Scope.Tenant, manifest.Artifact, number+1); e != nil {
			return e
		}
		if e = access.Audit(ctx, tx, "derived.analysis.published", "analysis_artifact", manifest.Artifact, "", nil, map[string]any{"version_id": manifest.Generation, "run_id": run.ID, "manifest_root": root, "stale": state.Changed}, access.Meta{ActorID: lease.Actor().ID, ActorType: "account", ActorScope: run.Scope.Entry, TenantID: run.Scope.Tenant, ProjectID: run.Scope.Project, Reason: "完整分析成果发布"}); e != nil {
			return e
		}
		e = lease.CompleteTx(ctx, tx, map[string]string{"phase": "completed", "artifact_id": manifest.Artifact, "artifact_version_id": manifest.Generation, "output_rows": strconv.FormatUint(manifest.Rows, 10)})
		return e
	})
}
func analysisAffectedNeeds(input inputSnapshotDetails) ([]monitoring.RawNeed, error) {
	needs := append([]monitoring.RawNeed{}, input.Needs...)
	var visit func([]inputSourceProof) error
	visit = func(proofs []inputSourceProof) error {
		for _, proof := range proofs {
			r, e := proof.Range.rangeValue()
			if e != nil {
				return e
			}
			needs = append(needs, monitoring.RawNeed{Channel: proof.Channel, From: &r.From, To: r.To})
			needs = append(needs, proof.RawNeeds...)
			if e = visit(proof.Inputs); e != nil {
				return e
			}
		}
		return nil
	}
	if e := visit(input.Sources); e != nil {
		return nil, e
	}
	return normalizedNeeds(needs)
}
func markAnalysisChangesTx(ctx context.Context, tx pgx.Tx, tenant, version, event string) error {
	_, e := tx.Exec(ctx, `INSERT INTO analysis_artifact_changes(tenant_id,version_id,event_id) SELECT DISTINCT v.tenant_id,v.id,e.id FROM analysis_artifact_versions v JOIN analysis_artifact_input_ranges i ON i.tenant_id=v.tenant_id AND i.version_id=v.id JOIN monitoring_change_ranges r ON r.tenant_id=i.tenant_id AND r.channel_id=i.channel_id AND r.affected&&i.required_range JOIN monitoring_changes e ON e.tenant_id=r.tenant_id AND e.id=r.event_id WHERE v.tenant_id=$1 AND ($2::uuid IS NULL OR v.id=$2) AND ($3::uuid IS NULL OR e.id=$3) AND e.source_position>v.source_position ON CONFLICT DO NOTHING`, tenant, nullable(version), nullable(event))
	return e
}
func rawNeedRange(need monitoring.RawNeed) (planning.Range, error) {
	from := monitoringvalue.MinTime.UnixNano()
	if need.From != nil {
		from = *need.From
	}
	r := planning.Range{From: from, To: need.To}
	if !r.Valid() {
		return r, invalid("/input_ranges", "输入范围无效。")
	}
	return r, nil
}

// Change tracking includes fixed reference windows and their recursive sources,
// even when those ranges lie outside the artifact's ordinary source interval.
func (s *Service) analysisAffectedNeedsTx(ctx context.Context, tx pgx.Tx, tenant string, input inputSnapshotDetails, config string, baselines []BaselineProof) ([]monitoring.RawNeed, error) {
	needs, e := analysisAffectedNeeds(input)
	if e != nil {
		return nil, e
	}
	visited := map[string]bool{}
	var fixed func(string, []BaselineProof) error
	var sources func([]inputSourceProof) error
	sources = func(proofs []inputSourceProof) error {
		for _, proof := range proofs {
			if e := fixed(proof.Config, proof.Baselines); e != nil {
				return e
			}
			if e := sources(proof.Inputs); e != nil {
				return e
			}
		}
		return nil
	}
	fixed = func(config string, proofs []BaselineProof) error {
		for _, proof := range proofs {
			key := config + ":" + proof.Reference
			if visited[key] {
				continue
			}
			visited[key] = true
			if len(visited) > 10000 {
				return problem("plan_limit_exceeded", "基准来源超过规划预算。", 422)
			}
			basis, found, e := loadBaselineTx(ctx, tx, tenant, config, proof.Reference)
			if e != nil {
				return e
			}
			if !found || fingerprint(basis) != proof.Hash {
				return problem("result_integrity_unavailable", "基准范围证据缺失。", 503)
			}
			needs = append(needs, basis.Needs...)
			ledger, found, e := loadInputSourcesTx(ctx, tx, tenant, basis.Snapshot.ID)
			if e != nil {
				return e
			}
			if found {
				additional, e := analysisAffectedNeeds(inputSnapshotDetails{Sources: ledger.Sources})
				if e != nil {
					return e
				}
				needs = append(needs, additional...)
				if e = sources(ledger.Sources); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e = fixed(config, baselines); e != nil {
		return nil, e
	}
	if e = sources(input.Sources); e != nil {
		return nil, e
	}
	return normalizedNeeds(needs)
}
