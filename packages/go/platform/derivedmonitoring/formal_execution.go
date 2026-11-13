package derivedmonitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) executeFormal(ctx context.Context, lease monitoring.TaskLease) error {
	if s.results == nil {
		return &monitoring.TaskError{Code: "calculation_storage_unavailable", Message: "派生结果存储尚未配置。", Disposition: "needs_action"}
	}
	task := lease.Task()
	var run runRecord
	var cfg ChannelConfig
	var p *compute.Plan
	var snapshotID string
	var target publicationTarget
	e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		run, e = scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.id=$2 AND r.job_id=$3`, task.Scope.Tenant, task.Job.Target, task.Job.ID))
		if e != nil {
			return e
		}
		if run.Mode != "automatic" && run.Mode != "backfill" {
			return &monitoring.TaskError{Code: "unsupported_execution_mode", Message: "此任务不是自动通道计算。", Disposition: "needs_action"}
		}
		cfg, e = scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND id=$3`, run.Scope.Tenant, run.ChannelID, run.ConfigID))
		if e != nil {
			return e
		}
		p, e = s.compile(cfg.Rule, "channel", cfg.Parameters)
		if e != nil {
			return e
		}
		if p.PlanHash != run.PlanHash {
			return problem("configuration_changed", "固定计算计划不匹配。", 409)
		}
		if e = tx.QueryRow(ctx, `SELECT generation_id::text,required_position::text,graph_revision::text FROM derived_run_publication_targets WHERE tenant_id=$1 AND run_id=$2`, run.Scope.Tenant, run.ID).Scan(&target.Generation, &target.RequiredPosition, &target.GraphRevision); e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `SELECT snapshot_id::text FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2`, run.Scope.Tenant, run.ID).Scan(&snapshotID)
		if e == pgx.ErrNoRows {
			return nil
		}
		return e
	})
	if e != nil {
		return e
	}
	var baselines map[string]compute.FixedBaseline
	var baselineProofs []BaselineProof
	if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		baselines, baselineProofs, e = s.runBaselinesTx(ctx, tx, lease.Actor(), run)
		return e
	}); e != nil {
		return e
	}
	prefixRange, _ := run.OutputRange.rangeValue()
	if snapshotID == "" {
		snapshot, e := s.freezeRunInputs(ctx, lease.Actor(), run, monitoring.RawFreezeRequest{Needs: run.Needs, Prefixes: inputPrefixes(p, cfg, run.Needs, prefixRange)})
		if e != nil {
			return e
		}
		if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
			if _, e := s.checkRunInputsTx(ctx, tx, lease.Actor(), run, snapshot.ID); e != nil {
				return e
			}
			_, e := tx.Exec(ctx, `INSERT INTO derived_run_raw_inputs(tenant_id,run_id,snapshot_id) VALUES($1,$2,$3)`, run.Scope.Tenant, run.ID, snapshot.ID)
			return e
		}); e != nil {
			return e
		}
		snapshotID = snapshot.ID
	}
	input, e := s.inspectRunInputs(ctx, lease.Actor(), run, snapshotID)
	if e != nil {
		return e
	}
	lineage, e := s.results.SealDocument(ctx, run.Scope.Tenant, "lineage", map[string]any{"schema": "derived-lineage/1", "run_id": run.ID, "config_id": cfg.ID, "config_hash": cfg.ConfigHash, "plan_hash": p.PlanHash, "engine_version": compute.EngineVersion, "compiler_version": compute.CompilerVersion, "raw_snapshot": input.Snapshot, "upstream_inputs": input.Sources, "input_ranges": input.Needs, "baselines": baselineProofs, "mandate_revision": run.MandateRevision, "output_range": run.OutputRange})
	if e != nil {
		return e
	}
	manifest := resultManifest{Schema: "derived-result-manifest/1", Generation: target.Generation, Run: run.ID, Channel: run.ChannelID, Config: cfg.ID, Plan: p.PlanHash, Range: run.OutputRange, Input: input.Snapshot, Lineage: lineage, Blocks: []resultstore.Block{}, States: map[string]string{}}
	manifest.Baselines = baselineProofs
	manifest.Sources = input.Sources
	monitoring.RecordTaskIdentity(ctx, map[string]string{"run_id": run.ID, "generation_id": target.Generation, "source_position": input.Snapshot.SourcePosition, "graph_revision": target.GraphRevision, "config_revision": run.ConfigRevision})
	if e = s.calculateManifestParts(ctx, lease, run, cfg, p, target, input, lineage, &manifest, baselines); e != nil {
		return e
	}
	if e = lease.Progress(ctx, map[string]string{"phase": "verifying", "output_rows": strconv.FormatUint(manifest.Rows, 10)}); e != nil {
		return e
	}
	root, e := s.results.SealDocument(ctx, run.Scope.Tenant, "manifests", manifest)
	if e != nil {
		return e
	}
	return s.publishFormal(ctx, lease, run, target, manifest, root)
}

func (s *Service) calculateManifestParts(ctx context.Context, lease monitoring.TaskLease, run runRecord, cfg ChannelConfig, p *compute.Plan, target publicationTarget, input inputSnapshotDetails, lineage string, manifest *resultManifest, baselines map[string]compute.FixedBaseline) error {
	output, e := run.OutputRange.rangeValue()
	if e != nil {
		return e
	}
	var process func(planning.Range, int) error
	process = func(part planning.Range, depth int) error {
		if depth > 64 {
			return fmt.Errorf("partition_budget_exceeded")
		}
		key := "output:" + rangeSQL(part) + ":" + fingerprint(manifest.Baselines)
		saved, found, e := lease.Checkpoint(ctx, key)
		if e != nil {
			return e
		}
		checkpoint := resultPart{Schema: "derived-result-part/1", Plan: p.PlanHash, Snapshot: input.Snapshot.Hash, Range: rangeDTO(part.From, part.To), Blocks: []resultstore.Block{}, States: map[string]string{}}
		valid := found && saved.State == "completed" && json.Unmarshal(saved.Result, &checkpoint) == nil && checkpoint.Schema == "derived-result-part/1" && checkpoint.Plan == p.PlanHash && checkpoint.Snapshot == input.Snapshot.Hash && fingerprint(checkpoint.Range) == fingerprint(rangeDTO(part.From, part.To))
		if valid && checkpoint.Split != nil {
			at, e := strconv.ParseInt(*checkpoint.Split, 10, 64)
			if e != nil || at <= part.From || part.To != nil && at >= *part.To {
				return fmt.Errorf("invalid_partition_checkpoint")
			}
			if e = process(planning.Range{From: part.From, To: &at}, depth+1); e != nil {
				return e
			}
			return process(planning.Range{From: at, To: part.To}, depth+1)
		}
		if valid {
			for _, block := range checkpoint.Blocks {
				if e = s.results.Verify(ctx, block); e != nil {
					valid = false
					break
				}
			}
		}
		if !valid {
			checkpoint, e = s.calculatePart(ctx, lease, run, cfg, p, target, input.Snapshot, lineage, part, baselines)
			if e != nil {
				if !partitionBudget(e) || p.Rule.Clock.Kind == "whole_range" {
					return e
				}
				end := int64(math.MaxInt64)
				if part.To != nil {
					end = *part.To
				}
				middle := new(big.Int).Add(big.NewInt(part.From), big.NewInt(end))
				middle.Div(middle, big.NewInt(2))
				at := middle.Int64()
				if at <= part.From || at >= end {
					return &monitoring.TaskError{Code: "budget_exceeded", Message: "单个输出及其完整输入窗口超过任务预算。", Disposition: "needs_action"}
				}
				split := strconv.FormatInt(at, 10)
				checkpoint = resultPart{Schema: "derived-result-part/1", Plan: p.PlanHash, Snapshot: input.Snapshot.Hash, Range: rangeDTO(part.From, part.To), Split: &split, Blocks: []resultstore.Block{}, States: map[string]string{}}
				if e = lease.SaveCheckpoint(ctx, key, "completed", checkpoint); e != nil {
					return e
				}
				if e = process(planning.Range{From: part.From, To: &at}, depth+1); e != nil {
					return e
				}
				return process(planning.Range{From: at, To: part.To}, depth+1)
			}
			if e = lease.SaveCheckpoint(ctx, key, "completed", checkpoint); e != nil {
				return e
			}
		}
		manifest.Blocks = append(manifest.Blocks, checkpoint.Blocks...)
		manifest.Rows += checkpoint.Rows
		for status, text := range checkpoint.States {
			n, e := strconv.ParseUint(text, 10, 64)
			if e != nil {
				return e
			}
			old, _ := strconv.ParseUint(manifest.States[status], 10, 64)
			manifest.States[status] = strconv.FormatUint(old+n, 10)
		}
		return nil
	}
	return process(output, 0)
}

func (s *Service) calculatePart(ctx context.Context, lease monitoring.TaskLease, run runRecord, cfg ChannelConfig, p *compute.Plan, target publicationTarget, snapshot monitoring.RawSnapshot, lineage string, part planning.Range, baselines map[string]compute.FixedBaseline) (resultPart, error) {
	out := resultPart{Schema: "derived-result-part/1", Plan: p.PlanHash, Snapshot: snapshot.Hash, Range: rangeDTO(part.From, part.To), Blocks: []resultstore.Block{}, States: map[string]string{}}
	var needs []monitoring.RawNeed
	e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		needs, e = immediateInputNeeds(cfg, p, part, analysisSource(run.Analysis))
		return e
	})
	if e != nil {
		return out, e
	}
	slots := map[string][]string{}
	data := compute.Data{Inputs: map[string][]compute.Sample{}, Baselines: baselines}
	for key, binding := range cfg.InputBindings {
		slots[binding.ChannelID] = append(slots[binding.ChannelID], key)
		data.Inputs[key] = []compute.Sample{}
	}
	budget := s.workingReadBudget(p)
	if e = lease.Progress(ctx, map[string]string{"phase": "reading", "from_ns": strconv.FormatInt(part.From, 10)}); e != nil {
		return out, e
	}
	resolved, e := s.resolveRunInputs(ctx, lease.Actor(), run, snapshot.ID, monitoring.RawFreezeRequest{Needs: needs, Prefixes: inputPrefixes(p, cfg, needs, part, analysisSource(run.Analysis))})
	if e != nil {
		return out, e
	}
	needs = resolved.Candidates
	source := part
	for _, n := range needs {
		if p.Mode == "analysis" && n.To != nil && source.To != nil && *n.To > *source.To {
			source.To = n.To
		}
		if n.From != nil && *n.From < source.From {
			source.From = *n.From
		}
	}
	if fixedSource := analysisSource(run.Analysis); fixedSource != nil {
		source = *fixedSource
	}
	options := runOptions(part, &source, run.RequestedAt)
	options.MaxSeriesRows = int(budget.Rows)
	_, e = s.readRunInputs(ctx, lease.Actor(), run, snapshot.ID, resolved, budget, func(row inputSample) error {
		for _, slot := range slots[row.Channel] {
			data.Inputs[slot] = append(data.Inputs[slot], row.Sample)
		}
		return nil
	})
	if partitionBudget(e) {
		// Release the rejected in-memory partition before allocating sort runs.
		data.Inputs = nil
		spilled, _, used, spillError := s.spillWindows(ctx, lease, run, cfg, p, snapshot.ID, resolved, options, runInputAccess(lease.Actor(), run))
		if used || spillError != nil {
			data, e = spilled, spillError
			data.Baselines = baselines
		}
	}
	if e != nil {
		return out, e
	}
	data.Complete = true
	type outputBuffer struct {
		rows  []resultstore.Row
		bytes int
		month string
	}
	buffers := map[string]*outputBuffer{}
	var bufferedRows, bufferedBytes int
	flush := func(key string) error {
		buffer := buffers[key]
		if buffer == nil || len(buffer.rows) == 0 {
			return nil
		}
		block, e := s.writeResultBlock(ctx, lease, run, part, buffer.rows)
		if e != nil {
			return e
		}
		out.Blocks = append(out.Blocks, block)
		bufferedRows -= len(buffer.rows)
		bufferedBytes -= buffer.bytes
		buffer.rows = nil
		buffer.bytes = 0
		return nil
	}
	flushAll := func() error {
		for _, output := range p.Rule.Outputs {
			if e := flush(output.Key); e != nil {
				return e
			}
		}
		return nil
	}
	if e = lease.Progress(ctx, map[string]string{"phase": "calculating"}); e != nil {
		return out, e
	}
	e = compute.Run(ctx, p, data, options, func(result compute.Result) error {
		if run.Analysis != nil && !slices.Contains(run.Analysis.Outputs, result.OutputKey) {
			return nil
		}
		row, e := channelResultRow(run, target.Generation, lineage, result)
		if e != nil {
			return e
		}
		nextMonth := time.Unix(0, result.At).UTC().Format("200601")
		size := len(jsonBytes(row))
		monitoring.RecordCalculationResult(ctx, result.Status, uint64(size))
		buffer := buffers[result.OutputKey]
		if buffer == nil {
			buffer = &outputBuffer{}
			buffers[result.OutputKey] = buffer
		}
		if nextMonth != buffer.month {
			if e = flush(result.OutputKey); e != nil {
				return e
			}
			buffer.month = nextMonth
		}
		if bufferedRows >= s.options.WriteRows || uint64(bufferedBytes+size) > s.options.WriteBytes {
			if e = flushAll(); e != nil {
				return e
			}
		}
		buffer.rows = append(buffer.rows, row)
		buffer.bytes += size
		bufferedRows++
		bufferedBytes += size
		out.Rows++
		incrementCount(out.States, result.Status)
		return nil
	})
	if e != nil {
		return out, e
	}
	return out, flushAll()
}

func (s *Service) writeResultBlock(ctx context.Context, lease monitoring.TaskLease, run runRecord, part planning.Range, rows []resultstore.Row) (resultstore.Block, error) {
	monitoring.RecordTaskPhase(ctx, "writing")
	defer monitoring.RecordTaskPhase(ctx, "calculating")
	var route monitoring.CalculationRoute
	chunk, attempt := uuid.NewString(), uuid.NewString()
	task := lease.Task()
	e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		route, e = s.monitoring.CalculationRouteTx(ctx, tx, run.Scope.Tenant, run.resultSeries())
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE derived_write_attempts SET state='abandoned' WHERE tenant_id=$1 AND run_id=$2 AND state='writing' AND fence<$3`, run.Scope.Tenant, run.ID, task.Fence); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO derived_write_attempts(tenant_id,id,run_id,chunk_id,output_range,fence,state,physical_shard,route_revision) VALUES($1,$2,$3,$4,$5::int8range,$6,'writing',$7,$8)`, run.Scope.Tenant, attempt, run.ID, chunk, rangeSQL(part), task.Fence, route.Physical, route.Revision)
		return e
	})
	if e != nil {
		return resultstore.Block{}, e
	}
	block, e := s.results.Write(ctx, route.Physical, chunk, attempt, rows, lease.Check)
	monitoring.RecordTemporaryPeak(ctx, block.Bytes)
	if e != nil {
		return block, e
	}
	e = lease.Transaction(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE derived_write_attempts SET state='sealed',manifest=$4,sealed_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2 AND fence=$3 AND state='writing'`, run.Scope.Tenant, attempt, task.Fence, jsonBytes(block))
		return e
	})
	return block, e
}
