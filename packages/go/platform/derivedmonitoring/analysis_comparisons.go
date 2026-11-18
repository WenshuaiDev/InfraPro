package derivedmonitoring

import (
	"context"
	"slices"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func comparisonTargetTx(ctx context.Context, tx pgx.Tx, scope Scope, id string) (analysisComparisonTarget, error) {
	var out analysisComparisonTarget
	var from, to *int64
	e := tx.QueryRow(ctx, `SELECT c.left_version_id::text,c.right_version_id::text,lv.artifact_id::text,rv.artifact_id::text,c.comparison_hash,c.output_mapping,lower(c.comparison_range),upper(c.comparison_range),j.input->'left_selection',j.input->'right_selection' FROM analysis_comparisons c JOIN derived_runs r ON r.tenant_id=c.tenant_id AND r.id=c.run_id JOIN monitoring_jobs j ON j.tenant_id=r.tenant_id AND j.id=r.job_id JOIN analysis_artifact_versions lv ON lv.tenant_id=c.tenant_id AND lv.id=c.left_version_id JOIN analysis_artifact_versions rv ON rv.tenant_id=c.tenant_id AND rv.id=c.right_version_id WHERE c.tenant_id=$1 AND r.scope_key=$2 AND c.run_id=$3`, scope.Tenant, scope.key(), id).Scan(&out.LeftVersion, &out.RightVersion, &out.LeftArtifact, &out.RightArtifact, &out.Hash, &out.Outputs, &from, &to, &out.LeftSelection, &out.RightSelection)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	if from != nil {
		r := rangeDTO(*from, to)
		out.Range = &r
	}
	return out, e
}

// A comparison receipt describes the qualified output slices. The immutable
// versions retain their complete bounds for integrity checks, but those bounds
// are not necessarily readable by the caller.
func comparisonOutputRange(left, right analysisArtifactProof) NanosecondRange {
	rangeValue, _ := left.Detail.Version.OutputRange.rangeValue()
	other, _ := right.Detail.Version.OutputRange.rangeValue()
	rangeValue.From = min(rangeValue.From, other.From)
	if rangeValue.To != nil && other.To != nil {
		to := max(*rangeValue.To, *other.To)
		rangeValue.To = &to
	} else {
		rangeValue.To = nil
	}
	return rangeDTO(rangeValue.From, rangeValue.To)
}

func (s *Service) CompareAnalysis(ctx context.Context, a access.Actor, scope Scope, in AnalysisCompareInput, audit access.Meta) (Run, error) {
	var out Run
	if e := checkMeta(in.WriteMeta, false, false); e != nil {
		return out, e
	}
	if !access.ValidID(in.LeftVersionID) || !access.ValidID(in.RightVersionID) {
		return out, invalid("/left_version_id", "需要两个固定成果版本。")
	}
	var rangeSQLValue any
	if in.Range != nil {
		r, e := in.Range.rangeValue()
		if e != nil {
			return out, e
		}
		rangeSQLValue = rangeSQL(r)
	}
	var leftArtifact, rightArtifact string
	e := s.transact(ctx, a, scope, "analysis.comparisons", false, func(tx pgx.Tx) error {
		for _, item := range []struct {
			version string
			target  *string
		}{{in.LeftVersionID, &leftArtifact}, {in.RightVersionID, &rightArtifact}} {
			e := tx.QueryRow(ctx, `SELECT v.artifact_id::text FROM analysis_artifact_versions v JOIN analysis_artifacts a ON a.tenant_id=v.tenant_id AND a.id=v.artifact_id WHERE v.tenant_id=$1 AND a.scope_key=$2 AND a.deleted_at IS NULL AND v.id=$3`, scope.Tenant, scope.key(), item.version).Scan(item.target)
			if e == pgx.ErrNoRows {
				return notFound()
			}
			if e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	left, e := s.loadSelectedAnalysis(ctx, a, scope, leftArtifact, AnalysisArtifactQuery{VersionID: in.LeftVersionID, Range: in.Range, Outputs: comparisonOutputKeys(in.Outputs, true)}, "analysis.comparisons")
	if e != nil {
		return out, e
	}
	right, e := s.loadSelectedAnalysis(ctx, a, scope, rightArtifact, AnalysisArtifactQuery{VersionID: in.RightVersionID, Range: in.Range, Outputs: comparisonOutputKeys(in.Outputs, false)}, "analysis.comparisons")
	if e != nil {
		return out, e
	}
	leftKeys, rightKeys := map[string]bool{}, map[string]bool{}
	for _, o := range left.Manifest.Outputs {
		leftKeys[o.Key] = true
	}
	for _, o := range right.Manifest.Outputs {
		rightKeys[o.Key] = true
	}
	mapping := append([]AnalysisOutputMapping{}, in.Outputs...)
	if len(mapping) == 0 {
		for _, o := range left.Manifest.Outputs {
			pair := AnalysisOutputMapping{Left: o.Key}
			if rightKeys[o.Key] {
				pair.Right = o.Key
			}
			mapping = append(mapping, pair)
		}
		for _, o := range right.Manifest.Outputs {
			if !leftKeys[o.Key] {
				mapping = append(mapping, AnalysisOutputMapping{Right: o.Key})
			}
		}
	}
	if len(mapping) > 32 {
		return out, invalid("/output_mapping", "一次最多比较32组输出。")
	}
	lseen, rseen := map[string]bool{}, map[string]bool{}
	for _, pair := range mapping {
		if pair.Left == "" && pair.Right == "" || pair.Left != "" && (!leftKeys[pair.Left] || lseen[pair.Left]) || pair.Right != "" && (!rightKeys[pair.Right] || rseen[pair.Right]) {
			return out, invalid("/output_mapping", "映射需要存在且不重复的输出项。")
		}
		lseen[pair.Left] = true
		rseen[pair.Right] = true
	}
	slices.SortFunc(mapping, func(a, b AnalysisOutputMapping) int {
		if a.Left < b.Left {
			return -1
		}
		if a.Left > b.Left {
			return 1
		}
		if a.Right < b.Right {
			return -1
		}
		if a.Right > b.Right {
			return 1
		}
		return 0
	})
	hash := fingerprint([]any{"analysis-comparison/1", left.Version.Manifest, right.Version.Manifest, mapping, in.Range})
	e = s.transact(ctx, a, scope, "analysis.comparisons", false, func(tx pgx.Tx) error {
		found, e := analysisReceiptTx(ctx, tx, a, scope, "analysis.comparisons", in.WriteMeta, in, &out)
		if e != nil {
			return e
		}
		if found {
			out, e = s.readRunTx(ctx, tx, a, scope, out.ID, true)
			out.Replayed = true
			return e
		}
		for _, proof := range []analysisArtifactProof{left, right} {
			if _, e = s.analysisArtifactSelectedTx(ctx, tx, a, scope, proof.Version.ArtifactID, proof.Version.ID, proof.Selection); e != nil {
				return e
			}
		}
		output := comparisonOutputRange(left, right)
		rangeValue, _ := output.rangeValue()
		run, job := uuid.NewString(), uuid.NewString()
		e = tx.QueryRow(ctx, `INSERT INTO derived_runs(id,tenant_id,project_id,actor_id,mode,job_id,request_id,request_hash,output_range,config_revision,control_revision,mandate_revision,plan_hash,input_requirements,requested_at_ns,reason) VALUES($1,$2,$3,$4,'comparison',$5,$6,$7,$8::int8range,1,1,1,$9,'[]',$10,$11) RETURNING created_at`, run, scope.Tenant, nullable(scope.Project), a.ID, job, in.RequestID, fingerprint(in), rangeSQL(rangeValue), hash, time.Now().UnixNano(), in.Reason).Scan(&out.CreatedAt)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO analysis_comparisons(tenant_id,run_id,left_version_id,right_version_id,output_mapping,comparison_range,comparison_hash) VALUES($1,$2,$3,$4,$5,$6::int8range,$7)`, scope.Tenant, run, in.LeftVersionID, in.RightVersionID, jsonBytes(mapping), rangeSQLValue, hash)
		if e != nil {
			return e
		}
		out.Job, e = s.monitoring.EnqueueTaskTx(ctx, tx, monitoring.EnqueueTask{ID: job, Scope: monitoringScope(scope), ActorID: a.ID, Kind: "analysis_compare", Target: run, TargetRevision: "1", Input: jsonBytes(map[string]any{"mode": "comparison", "left_selection": left.Selection, "right_selection": right.Selection})})
		if e != nil {
			return e
		}
		out.ID = run
		out.Mode = "comparison"
		out.PlanHash = hash
		out.OutputRange = output
		return analysisCompleteTx(ctx, tx, a, scope, "analysis.comparisons", "derived_run", run, in.WriteMeta, in, nil, out, audit)
	})
	return out, e
}
func (s *Service) authorizeComparisonTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, run runRecord, manage bool) error {
	if run.Scope != scope {
		return notFound()
	}
	if e := requireAnyDerivedTx(ctx, tx, a, scope, "analysis.results.read"); e != nil {
		return e
	}
	if manage {
		if e := requireAnyDerivedTx(ctx, tx, a, scope, "runs.manage"); e != nil {
			return e
		}
	}
	target, e := comparisonTargetTx(ctx, tx, scope, run.ID)
	if e != nil {
		return e
	}
	left, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, target.LeftArtifact, target.LeftVersion, target.LeftSelection)
	if e != nil {
		return e
	}
	right, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, target.RightArtifact, target.RightVersion, target.RightSelection)
	if e != nil {
		return e
	}
	if target.Hash != run.PlanHash || target.Hash != fingerprint([]any{"analysis-comparison/1", left.Version.Manifest, right.Version.Manifest, target.Outputs, target.Range}) {
		return problem("result_integrity_unavailable", "固定比较目标不一致。", 503)
	}
	return nil
}
func (s *Service) readComparisonTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, run runRecord) (Run, error) {
	if e := s.authorizeComparisonTx(ctx, tx, a, scope, run, false); e != nil {
		return Run{}, e
	}
	target, e := comparisonTargetTx(ctx, tx, scope, run.ID)
	if e != nil {
		return Run{}, e
	}
	left, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, target.LeftArtifact, target.LeftVersion, target.LeftSelection)
	if e != nil {
		return Run{}, e
	}
	right, e := s.analysisArtifactSelectedTx(ctx, tx, a, scope, target.RightArtifact, target.RightVersion, target.RightSelection)
	if e != nil {
		return Run{}, e
	}
	// Re-project on reads too, including queued runs and idempotent replays
	// created before slice bounds were persisted in the run record.
	run.OutputRange = comparisonOutputRange(left, right)
	var result AnalysisComparisonResult
	e = tx.QueryRow(ctx, `SELECT result FROM analysis_comparison_receipts WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, run.ID).Scan(&result)
	if e == pgx.ErrNoRows {
		return run.Run, nil
	}
	if e != nil {
		return Run{}, e
	}
	result.Left = left.Detail.Version
	result.Right = right.Detail.Version
	if result.Left.Partial || result.Right.Partial {
		result.RangeChanged = fingerprint(result.Left.OutputRange) != fingerprint(result.Right.OutputRange)
	}
	run.Comparison = &result
	return run.Run, nil
}
