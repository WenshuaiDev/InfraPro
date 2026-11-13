package derivedmonitoring

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) ReplayGeneration(ctx context.Context, a access.Actor, scope Scope, generation string, in ReplayInput, audit access.Meta) (Run, error) {
	if e := checkMeta(in.WriteMeta, false, false); e != nil {
		return Run{}, e
	}
	proof, e := s.historyGeneration(ctx, a, scope, generation, in.Range, "generations.replay")
	if e != nil {
		return Run{}, e
	}
	var id string
	replayed := false
	e = s.transact(ctx, a, scope, "generations.replay", true, func(tx pgx.Tx) error {
		if e := s.recheckHistoryTx(ctx, tx, a, scope, &proof); e != nil {
			return e
		}
		hash := fingerprint([]any{generation, in})
		var previousHash string
		e := tx.QueryRow(ctx, `SELECT id::text,request_hash FROM derived_runs WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.key(), a.ID, in.RequestID).Scan(&id, &previousHash)
		if e == nil {
			if previousHash != hash {
				return problem("request_id_reused", "请求编号已用于其他运行。", 409)
			}
			replayed = true
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		original, e := scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.id=$2`, scope.Tenant, proof.generation.RunID))
		if e != nil {
			return e
		}
		p, e := s.compile(proof.config.Rule, "channel", proof.config.Parameters)
		if e != nil {
			return e
		}
		if p.PlanHash != original.PlanHash || p.PlanHash != proof.manifest.Plan {
			return problem("engine_version_unavailable", "原编译计划与当前引擎不一致，无法验证此版本。", 409)
		}
		id = uuid.NewString()
		job := uuid.NewString()
		_, e = tx.Exec(ctx, `INSERT INTO derived_runs(id,tenant_id,project_id,actor_id,channel_id,config_id,mode,job_id,request_id,request_hash,output_range,config_revision,control_revision,mandate_revision,plan_hash,input_requirements,requested_at_ns,reason) VALUES($1,$2,$3,$4,$5,$6,'historical_replay',$7,$8,$9,$10::int8range,$11,$12,$13,$14,$15,$16,$17)`, id, scope.Tenant, nullable(scope.Project), a.ID, original.ChannelID, original.ConfigID, job, in.RequestID, hash, rangeSQL(proof.output), original.ConfigRevision, original.ControlRevision, original.MandateRevision, original.PlanHash, jsonBytes(proof.needs), original.RequestedAt, in.Reason)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO derived_replay_targets(tenant_id,run_id,generation_id,baselines,upstream_inputs) VALUES($1,$2,$3,$4,$5)`, scope.Tenant, id, generation, jsonBytes(proof.manifest.Baselines), jsonBytes(append([]inputSourceProof{}, proof.sources...)))
		if e != nil {
			return e
		}
		if _, e = s.monitoring.EnqueueTaskTx(ctx, tx, monitoring.EnqueueTask{ID: job, Scope: monitoringScope(scope), ActorID: a.ID, Kind: "derived_replay", Target: id, TargetRevision: "1", Input: jsonBytes(map[string]string{"mode": "historical_replay", "run_id": id, "generation_id": generation})}); e != nil {
			return e
		}
		audit.ActorID = a.ID
		audit.ActorType = "account"
		audit.ActorScope = scope.Entry
		audit.TenantID = scope.Tenant
		audit.ProjectID = scope.Project
		audit.Reason = in.Reason
		return access.Audit(ctx, tx, "derived.generations.replay", "derived_run", id, "", nil, map[string]string{"generation_id": generation, "job_id": job}, audit)
	})
	if e != nil {
		return Run{}, e
	}
	out, e := s.readRun(ctx, a, scope, id, "generations.replay")
	out.Replayed = replayed
	return out, e
}

// The replay target only points at immutable evidence. Current channel control
// or assignment revisions do not reinterpret it; current user access still does.
func (s *Service) replayProofTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, run runRecord) (historyProof, error) {
	var proof historyProof
	var generation string
	e := tx.QueryRow(ctx, `SELECT generation_id::text,baselines,upstream_inputs FROM derived_replay_targets WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, run.ID).Scan(&generation, &proof.manifest.Baselines, &proof.sources)
	if e != nil {
		return proof, e
	}
	proof.generation, e = scanGeneration(tx.QueryRow(ctx, generationSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, generation))
	if e != nil {
		return proof, e
	}
	if proof.generation.ChannelID != run.ChannelID || proof.generation.ConfigID != run.ConfigID {
		return proof, access.ErrDenied
	}
	proof.output, e = run.OutputRange.rangeValue()
	if e != nil {
		return proof, e
	}
	proof.needs = run.Needs
	if _, e = s.channelTx(ctx, tx, a, scope, run.ChannelID); e != nil {
		return proof, e
	}
	e = s.recheckHistoryTx(ctx, tx, a, scope, &proof)
	return proof, e
}
func (s *Service) authorizeReplayExecutionTx(ctx context.Context, tx pgx.Tx, task monitoring.Task, run runRecord) error {
	a := executionActor(task.ActorID)
	op := access.DerivedOperation(run.Scope.Entry, "generations.replay")
	if run.Scope.Entry == "project" {
		if _, e := access.RequireProjectTx(ctx, tx, a.ID, op, run.Scope.Tenant, run.Scope.Project); e != nil {
			return e
		}
	} else if e := access.RequireScopedTx(ctx, tx, a.ID, op, run.Scope.Tenant); e != nil {
		return e
	}
	_, e := s.replayProofTx(ctx, tx, a, run.Scope, run)
	return e
}
func (s *Service) authorizeReplayTaskTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, run runRecord, manage bool) error {
	for _, permission := range []string{"channels.read", "results.history", "runs.read"} {
		if e := requireAnyDerivedTx(ctx, tx, a, scope, permission); e != nil {
			return e
		}
	}
	if manage {
		if e := requireAnyDerivedTx(ctx, tx, a, scope, "runs.manage"); e != nil {
			return e
		}
	}
	_, e := s.replayProofTx(ctx, tx, a, scope, run)
	return e
}

func (s *Service) executeReplay(ctx context.Context, lease monitoring.TaskLease) error {
	task := lease.Task()
	a := lease.Actor()
	scope := Scope{Entry: task.Scope.Entry, Tenant: task.Scope.Tenant, Project: task.Scope.Project}
	var run, original runRecord
	var generation string
	e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		run, e = scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.id=$2`, scope.Tenant, task.Job.Target))
		if e != nil {
			return e
		}
		proof, e := s.replayProofTx(ctx, tx, a, scope, run)
		if e != nil {
			return e
		}
		generation = proof.generation.ID
		original, e = scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.id=$2`, scope.Tenant, proof.generation.RunID))
		return e
	})
	if e != nil {
		return e
	}
	proof, e := s.historyGeneration(ctx, a, scope, generation, &run.OutputRange, "generations.replay")
	if e != nil {
		return e
	}
	p, e := s.compile(proof.config.Rule, "channel", proof.config.Parameters)
	if e != nil {
		return e
	}
	if p.PlanHash != run.PlanHash || p.PlanHash != proof.manifest.Plan {
		return problem("engine_version_unavailable", "无法用当前引擎复现原计划。", 409)
	}
	baselines := map[string]compute.FixedBaseline{}
	e = lease.Transaction(ctx, func(tx pgx.Tx) error {
		for _, b := range proof.manifest.Baselines {
			fixed, found, e := loadBaselineTx(ctx, tx, scope.Tenant, run.ConfigID, b.Reference)
			if e != nil {
				return e
			}
			if !found || fingerprint(fixed) != b.Hash {
				return problem("result_integrity_unavailable", "固定基准证据不一致。", 503)
			}
			rat, ok := new(big.Rat).SetString(fixed.Numerator + "/" + fixed.Denominator)
			if !ok {
				return problem("result_integrity_unavailable", "固定基准数值不完整。", 503)
			}
			value, e := monitoringvalue.Rational(rat)
			if e != nil {
				return e
			}
			id := fixed.Adoption
			if fixed.Result != "" {
				id = fixed.Result + ":" + fixed.Generation
			}
			if id == "" {
				id = fixed.Snapshot.ID
			}
			baselines[b.Reference] = compute.FixedBaseline{Value: value, At: fixed.At, Valid: b.Valid, Proven: true, Quality: fixed.Quality, ID: id}
		}
		return nil
	})
	if e != nil {
		return e
	}
	result := ReplayResult{GenerationID: generation, Range: run.OutputRange, PlanHash: p.PlanHash, ManifestHash: proof.generation.manifest, DifferenceExamples: []ReplayDifference{}, Examples: []TrialValue{}}
	var process func(planning.Range, int) error
	process = func(part planning.Range, depth int) error {
		if depth > 64 {
			return problem("budget_exceeded", "回放无法在有界工作集中完成。", 422)
		}
		key := "replay:" + rangeSQL(part)
		saved, found, e := lease.Checkpoint(ctx, key)
		if e != nil {
			return e
		}
		var piece ReplayResult
		valid := found && saved.State == "completed" && json.Unmarshal(saved.Result, &piece) == nil && piece.GenerationID == generation && piece.PlanHash == p.PlanHash && piece.ManifestHash == result.ManifestHash && fingerprint(piece.Range) == fingerprint(rangeDTO(part.From, part.To))
		if !valid {
			piece, e = s.replayPart(ctx, lease, original, proof, p, baselines, part)
			if e != nil {
				var raw *monitoring.Error
				var issue *compute.Issue
				var domain *Error
				budget := errors.Is(e, monitoringvalue.ErrBudget) || errors.As(e, &raw) && raw.Code == "budget_exceeded" || errors.As(e, &issue) && issue.Code == "budget_exceeded" || errors.As(e, &domain) && domain.Code == "budget_exceeded"
				if !budget || part.To == nil || *part.To-part.From <= 1 {
					return e
				}
				middle := new(big.Int).Add(big.NewInt(part.From), big.NewInt(*part.To))
				middle.Quo(middle, big.NewInt(2))
				at := middle.Int64()
				if at <= part.From || at >= *part.To {
					return e
				}
				if e = process(planning.Range{From: part.From, To: &at}, depth+1); e != nil {
					return e
				}
				return process(planning.Range{From: at, To: part.To}, depth+1)
			}
			if e = lease.SaveCheckpoint(ctx, key, "completed", piece); e != nil {
				return e
			}
		}
		result.Expected += piece.Expected
		result.Actual += piece.Actual
		result.Differences += piece.Differences
		result.DifferenceExamples = append(result.DifferenceExamples, piece.DifferenceExamples[:min(len(piece.DifferenceExamples), 100-len(result.DifferenceExamples))]...)
		result.Examples = append(result.Examples, piece.Examples[:min(len(piece.Examples), 100-len(result.Examples))]...)
		return lease.Progress(ctx, map[string]string{"phase": "replaying", "compared_rows": strconv.FormatUint(result.Actual, 10), "differences": strconv.FormatUint(result.Differences, 10)})
	}
	if e = process(proof.output, 0); e != nil {
		return e
	}
	result.Matched = result.Differences == 0 && result.Expected == result.Actual
	return lease.Transaction(ctx, func(tx pgx.Tx) error {
		current, e := s.replayProofTx(ctx, tx, a, scope, run)
		if e != nil {
			return e
		}
		result.InputState = current.generation.InputState
		if _, e = tx.Exec(ctx, `INSERT INTO derived_replay_receipts(tenant_id,run_id,result) VALUES($1,$2,$3)`, scope.Tenant, run.ID, jsonBytes(result)); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET state='succeeded',revision=revision+1,lease_owner=NULL,lease_until=NULL,progress=$3,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, scope.Tenant, task.Job.ID, jsonBytes(map[string]string{"phase": "completed", "matched": strconv.FormatBool(result.Matched), "generation_id": generation}))
		return e
	})
}

func (s *Service) replayPart(ctx context.Context, lease monitoring.TaskLease, original runRecord, proof historyProof, p *compute.Plan, baselines map[string]compute.FixedBaseline, part planning.Range) (ReplayResult, error) {
	out := ReplayResult{GenerationID: proof.generation.ID, Range: rangeDTO(part.From, part.To), PlanHash: p.PlanHash, ManifestHash: proof.generation.manifest, DifferenceExamples: []ReplayDifference{}, Examples: []TrialValue{}}
	var needs []monitoring.RawNeed
	e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		needs, e = s.plannedInputsTx(ctx, tx, original.Scope.Tenant, proof.config, p, part)
		return e
	})
	if e != nil {
		return out, e
	}
	task := lease.Task()
	scope := task.Scope
	resolved, e := s.resolveInputs(ctx, inputAccess{Actor: lease.Actor(), Scope: scope, History: true}, proof.generation.snapshot, monitoring.RawFreezeRequest{Needs: needs, Prefixes: inputPrefixes(p, proof.config, needs, part), History: true})
	if e != nil {
		return out, e
	}
	data := compute.Data{Inputs: map[string][]compute.Sample{}, Baselines: baselines, Complete: true}
	slots := map[string][]string{}
	for key, binding := range proof.config.InputBindings {
		data.Inputs[key] = []compute.Sample{}
		slots[binding.ChannelID] = append(slots[binding.ChannelID], key)
	}
	budget := s.workingReadBudget(p)
	source := part
	for _, n := range resolved.Candidates {
		if n.From != nil && *n.From < source.From {
			source.From = *n.From
		}
	}
	options := runOptions(part, &source, original.RequestedAt)
	options.MaxSeriesRows = int(budget.Rows)
	_, e = s.readInputs(ctx, inputAccess{Actor: lease.Actor(), Scope: scope, History: true}, proof.generation.snapshot, resolved, budget, func(row inputSample) error {
		for _, slot := range slots[row.Channel] {
			data.Inputs[slot] = append(data.Inputs[slot], row.Sample)
		}
		return nil
	})
	if partitionBudget(e) {
		data.Inputs = nil
		// Checkpoints belong to this replay, while logical result identities and
		// the immutable plan still belong to the original published run.
		replay := original
		replay.ID = task.Job.Target
		replay.Scope = Scope{Entry: scope.Entry, Tenant: scope.Tenant, Project: scope.Project}
		spilled, _, used, spillError := s.spillWindows(ctx, lease, replay, proof.config, p, proof.generation.snapshot, resolved, options, inputAccess{Actor: lease.Actor(), Scope: scope, History: true})
		if used || spillError != nil {
			data, e = spilled, spillError
			data.Baselines = baselines
		}
	}
	if e != nil {
		return out, e
	}
	expected := map[string]resultstore.Row{}
	for _, block := range proof.manifest.Blocks {
		if e = lease.Check(ctx); e != nil {
			return out, e
		}
		e = s.results.ReadBlock(ctx, block, func(row resultstore.Row) error {
			if row.Generation != proof.generation.ID || row.Run != original.ID || row.Series != original.ChannelID || row.Config != original.ConfigID || row.Lineage != proof.generation.lineage {
				return problem("result_integrity_unavailable", "原结果块与回放目标不一致。", 503)
			}
			if !part.Contains(row.At) {
				return nil
			}
			if _, exists := expected[row.Result]; exists {
				return problem("result_integrity_unavailable", "原结果包含重复逻辑身份。", 503)
			}
			if uint64(len(expected)) >= budget.Rows {
				return problem("budget_exceeded", "回放结果超出单块工作集。", 422)
			}
			expected[row.Result] = row
			out.Expected++
			return nil
		})
		if e != nil {
			return out, e
		}
	}
	addDifference := func(row resultstore.Row, kind, want, got string) {
		out.Differences++
		if len(out.DifferenceExamples) < 100 {
			out.DifferenceExamples = append(out.DifferenceExamples, ReplayDifference{row.Result, strconv.FormatInt(row.At, 10), kind, want, got})
		}
	}
	e = compute.Run(ctx, p, data, options, func(result compute.Result) error {
		row, e := channelResultRow(original, proof.generation.ID, proof.generation.lineage, result)
		if e != nil {
			return e
		}
		out.Actual++
		if len(out.Examples) < 100 {
			out.Examples = append(out.Examples, trialValue(result))
		}
		old, found := expected[row.Result]
		if !found {
			addDifference(row, "added", "", row.Hash())
		} else if old.Hash() != row.Hash() {
			addDifference(row, "changed", old.Hash(), row.Hash())
		}
		delete(expected, row.Result)
		return nil
	})
	if e != nil {
		return out, e
	}
	keys := make([]string, 0, len(expected))
	for key := range expected {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		row := expected[key]
		addDifference(row, "missing", row.Hash(), "")
	}
	out.Matched = out.Differences == 0 && out.Expected == out.Actual
	return out, nil
}
