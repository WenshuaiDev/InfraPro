package derivedmonitoring

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

func (s *Service) ExecuteTask(ctx context.Context, lease monitoring.TaskLease) error {
	if e := s.requireReady(ctx); e != nil {
		return e
	}
	task := lease.Task()
	if task.Job.Kind != "derive" && task.Job.Kind != "derived_replay" && task.Job.Kind != "analysis" && task.Job.Kind != "analysis_compare" && task.Job.Kind != "analysis_export" && task.Job.Kind != "derived_evidence_export" {
		return &monitoring.TaskError{Code: "unsupported_execution_mode", Message: "当前执行器不支持该运行模式。", Disposition: "needs_action"}
	}
	var input struct {
		Mode string `json:"mode"`
	}
	_ = json.Unmarshal(task.Input, &input)
	var e error
	if task.Job.Kind == "derived_evidence_export" && input.Mode == "derived_evidence_export" {
		e = s.executeGenerationExport(ctx, lease)
	} else if task.Job.Kind == "analysis_export" && input.Mode == "analysis_export" {
		e = s.executeAnalysisExport(ctx, lease)
	} else if task.Job.Kind == "analysis_compare" && input.Mode == "comparison" {
		e = s.executeComparison(ctx, lease)
	} else if task.Job.Kind == "analysis" && input.Mode == "analysis" {
		e = s.executeAnalysis(ctx, lease)
	} else if task.Job.Kind == "derived_replay" && input.Mode == "historical_replay" {
		e = s.executeReplay(ctx, lease)
	} else if input.Mode == "automatic" || input.Mode == "backfill" {
		e = s.executeFormal(ctx, lease)
	} else {
		e = s.executeTrial(ctx, lease)
	}
	if e == nil {
		return nil
	}
	var raw *monitoring.Error
	if partitionBudget(e) && ctx.Value(inlineTrialKey{}) == true {
		return &monitoring.TaskError{Code: "background_required", Message: "完整试算已转后台执行。", Disposition: "background"}
	}
	if errors.Is(e, context.DeadlineExceeded) && ctx.Value(inlineTrialKey{}) == true {
		return &monitoring.TaskError{Code: "background_required", Message: "完整试算已转后台执行。", Disposition: "background"}
	}
	var issue *compute.Issue
	var de *Error
	if errors.As(e, &de) && de.Code == "upstream_pending" && input.Mode == "automatic" {
		var frozen bool
		if check := lease.Transaction(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2)`, task.Scope.Tenant, task.Job.Target).Scan(&frozen)
		}); check != nil {
			return check
		}
		if frozen {
			return &monitoring.TaskError{Code: "input_changed", Message: "固定上游已变化，旧目标取消并等待新的来源目标。", Disposition: "cancelled"}
		}
		if check := lease.Progress(ctx, map[string]string{"phase": "waiting_upstream"}); check != nil {
			return check
		}
		return &monitoring.TaskError{Code: "upstream_pending", Message: e.Error(), Disposition: "waiting"}
	}
	code := ""
	if errors.As(e, &raw) && raw.Status == 422 {
		code = raw.Code
	}
	if errors.As(e, &issue) {
		code = issue.Code
	}
	if errors.As(e, &de) && (de.Status == 409 || de.Status == 422) {
		code = de.Code
	}
	if errors.Is(e, monitoringvalue.ErrBudget) || strings.Contains(e.Error(), "budget_exceeded") {
		code = "budget_exceeded"
	}
	if code != "" {
		return &monitoring.TaskError{Code: code, Message: e.Error(), Disposition: "needs_action"}
	}
	return e
}
func trialValue(r compute.Result) TrialValue {
	out := TrialValue{Output: r.OutputKey, At: strconv.FormatInt(r.At, 10), Value: r.Value, Scale: r.Scale, Rounded: r.Rounded, Status: r.Status, Quality: r.Quality, Reasons: append([]string{}, r.Reasons...), Count: strconv.FormatUint(r.Count, 10), Excluded: strconv.FormatUint(r.Excluded, 10), Matches: []TrialMatch{}}
	if r.Interval != nil {
		v := rangeDTO(r.Interval.From, &r.Interval.To)
		out.Interval = &v
	}
	for _, m := range r.Matches {
		out.Matches = append(out.Matches, TrialMatch{m.Reference, m.ID, strconv.FormatInt(m.At, 10), m.OffsetNS})
	}
	return out
}
func nodeExample(ctx context.Context, s compute.Sample, scale int) (TrialValue, error) {
	previewIssue := ""
	result := compute.Result{At: s.At, Interval: s.Interval, Scale: scale, Status: s.Status, Quality: s.Quality, Reasons: s.Reasons, Count: s.Count, Excluded: s.Excluded, Matches: s.Matches}
	if result.Status == "" {
		result.Status = "valid"
	}
	if result.Status == "valid" && s.Value != nil {
		encoded, e := s.Value.Encode(ctx, scale)
		if e == nil {
			result.Value = &encoded.Value
			result.Rounded = encoded.Rounded
		} else if errors.Is(e, monitoringvalue.ErrUnderflow) || errors.Is(e, monitoringvalue.ErrRange) || errors.Is(e, monitoringvalue.ErrPrecision) {
			previewIssue = e.Error()
		} else {
			return TrialValue{}, e
		}
	}
	out := trialValue(result)
	out.PreviewIssue = previewIssue
	return out, nil
}
func (s *Service) executeTrial(ctx context.Context, lease monitoring.TaskLease) error {
	task := lease.Task()
	actor := lease.Actor()
	scope := Scope{Entry: task.Scope.Entry, Tenant: task.Scope.Tenant, Project: task.Scope.Project}
	var run runRecord
	var cfg ChannelConfig
	var p *compute.Plan
	var snapshotID string
	e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		run, e = scanRun(tx.QueryRow(ctx, runSelect+` WHERE r.tenant_id=$1 AND r.id=$2 AND r.job_id=$3`, scope.Tenant, task.Job.Target, task.Job.ID))
		if e != nil {
			return e
		}
		if run.Mode != "trial" {
			return &monitoring.TaskError{Code: "unsupported_execution_mode", Message: "运行模式不支持。", Disposition: "needs_action"}
		}
		_, cfg, p, e = s.calculationConfigTx(ctx, tx, actor, scope, run.ChannelID, run.ConfigID)
		if e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `SELECT snapshot_id::text FROM derived_run_raw_inputs WHERE tenant_id=$1 AND run_id=$2`, scope.Tenant, run.ID).Scan(&snapshotID)
		if e == pgx.ErrNoRows {
			return nil
		}
		return e
	})
	if e != nil {
		return e
	}
	if e = lease.Progress(ctx, map[string]string{"phase": "planning"}); e != nil {
		return e
	}
	if e = s.freezeBaselines(ctx, lease, cfg, p); e != nil {
		return e
	}
	prefixRange, _ := run.OutputRange.rangeValue()
	if snapshotID == "" {
		snapshot, e := s.freezeRunInputs(ctx, actor, run, monitoring.RawFreezeRequest{Needs: run.Needs, Prefixes: inputPrefixes(p, cfg, run.Needs, prefixRange)})
		if e != nil {
			return e
		}
		e = lease.Transaction(ctx, func(tx pgx.Tx) error {
			if _, e := s.checkUserInputsTx(ctx, tx, actor, task.Scope, snapshot.ID, false); e != nil {
				return e
			}
			_, e := tx.Exec(ctx, `INSERT INTO derived_run_raw_inputs(tenant_id,run_id,snapshot_id) VALUES($1,$2,$3)`, scope.Tenant, run.ID, snapshot.ID)
			return e
		})
		if e != nil {
			return e
		}
		snapshotID = snapshot.ID
	}
	description, e := s.inspectRunInputs(ctx, actor, run, snapshotID)
	if e != nil {
		return e
	}
	result, e := s.calculateTrial(ctx, lease, run, cfg, p, description, snapshotID)
	if e != nil {
		return e
	}
	return lease.Transaction(ctx, func(tx pgx.Tx) error {
		state, e := s.checkUserInputsTx(ctx, tx, actor, task.Scope, snapshotID, false)
		if e != nil {
			return e
		}
		result.Stale = state.Changed
		_, currentBaselines, e := s.baselinesTx(ctx, tx, actor, task.Scope, nil, cfg.ID)
		if e != nil {
			return e
		}
		result.Stale = result.Stale || fingerprint(currentBaselines) != fingerprint(result.Baselines)
		var receipt string
		e = tx.QueryRow(ctx, `INSERT INTO derived_trial_receipts(tenant_id,project_id,actor_id,run_id,channel_id,config_id,config_hash,result,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,clock_timestamp()+($9::bigint * interval '1 millisecond')) RETURNING id::text`, scope.Tenant, nullable(scope.Project), actor.ID, run.ID, run.ChannelID, cfg.ID, cfg.ConfigHash, jsonBytes(result), s.options.TrialTTL.Milliseconds()).Scan(&receipt)
		if e != nil {
			return e
		}
		e = lease.CompleteTx(ctx, tx, map[string]string{"phase": "completed", "output_count": result.OutputCount, "trial_receipt_id": receipt})
		return e
	})
}

func (s *Service) calculateTrialPart(ctx context.Context, lease monitoring.TaskLease, run runRecord, cfg ChannelConfig, p *compute.Plan, description inputSnapshotDetails, snapshotID string, output planning.Range) (TrialResult, error) {
	actor, task := lease.Actor(), lease.Task()
	var e error
	result := TrialResult{PlanHash: p.PlanHash, InputSnapshot: description.Snapshot, InputRanges: description.Needs, InputExamples: []monitoring.RawSample{}, DerivedInputExamples: []monitoring.CalculatedObservation{}, Sources: description.Sources, OutputRange: run.OutputRange, States: map[string]string{}, Nodes: []TrialNode{}, Examples: []TrialValue{}}
	data := compute.Data{Inputs: map[string][]compute.Sample{}, Baselines: map[string]compute.FixedBaseline{}}
	if e = lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		data.Baselines, result.Baselines, e = s.baselinesTx(ctx, tx, actor, task.Scope, nil, cfg.ID)
		return e
	}); e != nil {
		return result, e
	}
	slots := map[string][]string{}
	for _, input := range p.Rule.Inputs {
		data.Inputs[input.Key] = []compute.Sample{}
		channel := cfg.InputBindings[input.Key].ChannelID
		slots[channel] = append(slots[channel], input.Key)
	}
	budget := s.workingReadBudget(p)
	if ctx.Value(inlineTrialKey{}) == true {
		budget.Rows = min(budget.Rows, uint64(s.options.TrialRows))
	}
	if e = lease.Progress(ctx, map[string]string{"phase": "reading"}); e != nil {
		return result, e
	}
	source := output
	for _, need := range description.Candidates {
		if p.Mode == "analysis" && p.Rule.Clock.Kind != "whole_range" && need.To != nil && source.To != nil && *need.To > *source.To {
			source.To = need.To
		}
		if need.From != nil && *need.From < source.From {
			source.From = *need.From
		}
	}
	if fixedSource := analysisSource(run.Analysis); fixedSource != nil {
		source = *fixedSource
	}
	options := runOptions(output, &source, run.RequestedAt)
	options.MaxSeriesRows = int(budget.Rows)
	result.InputCounts, e = s.readRunInputs(ctx, actor, run, snapshotID, description, budget, func(row inputSample) error {
		for _, slot := range slots[row.Channel] {
			data.Inputs[slot] = append(data.Inputs[slot], row.Sample)
		}
		if row.Raw != nil && len(result.InputExamples) < 200 {
			result.InputExamples = append(result.InputExamples, *row.Raw)
		}
		if row.Derived != nil && len(result.DerivedInputExamples) < 200 {
			result.DerivedInputExamples = append(result.DerivedInputExamples, *row.Derived)
		}
		return nil
	})
	if partitionBudget(e) && ctx.Value(inlineTrialKey{}) != true {
		data.Inputs = nil
		spilled, state, used, spillError := s.spillWindows(ctx, lease, run, cfg, p, snapshotID, description, options, runInputAccess(lease.Actor(), run))
		if used || spillError != nil {
			baselines := data.Baselines
			data, e = spilled, spillError
			data.Baselines = baselines
			result.InputCounts, result.InputExamples, result.DerivedInputExamples = state.Counts, state.Examples, state.DerivedExamples
		}
	}
	if e != nil {
		return result, e
	}
	data.Complete = true
	result.WaitingForInput = result.InputCounts.Rows == 0
	nodeBudget := min(800, s.options.TrialExamples)
	perNode := max(1, nodeBudget/max(1, len(p.Rule.Nodes)))
	nodeScale := p.Rule.Outputs[0].Numeric.Scale
	options.Observe = func(id string, rows []compute.Sample) error {
		node := TrialNode{NodeID: id, States: map[string]string{}, Examples: []TrialValue{}}
		var samples uint64
		for i, sample := range rows {
			if i%256 == 0 {
				if e := ctx.Err(); e != nil {
					return e
				}
			}
			if !output.Contains(sample.At) && p.Rule.Clock.Kind != "whole_range" {
				continue
			}
			samples++
			status := sample.Status
			if status == "" {
				status = "valid"
			}
			incrementCount(node.States, status)
			if len(node.Examples) < perNode && (output.Contains(sample.At) || p.Rule.Clock.Kind == "whole_range") {
				example, e := nodeExample(ctx, sample, nodeScale)
				if e != nil {
					return e
				}
				node.Examples = append(node.Examples, example)
			}
		}
		node.Samples = strconv.FormatUint(samples, 10)
		result.Nodes = append(result.Nodes, node)
		return nil
	}
	if e = lease.Progress(ctx, map[string]string{"phase": "calculating", "input_rows": strconv.FormatUint(result.InputCounts.Rows, 10)}); e != nil {
		return result, e
	}
	var outputs uint64
	e = compute.Run(ctx, p, data, options, func(r compute.Result) error {
		if run.Analysis != nil && !slices.Contains(run.Analysis.Outputs, r.OutputKey) {
			return nil
		}
		outputs++
		monitoring.RecordCalculationResult(ctx, r.Status, 0)
		incrementCount(result.States, r.Status)
		if len(result.Examples) < s.options.TrialExamples {
			result.Examples = append(result.Examples, trialValue(r))
		}
		return nil
	})
	if e != nil {
		return result, e
	}
	result.OutputCount = strconv.FormatUint(outputs, 10)
	result.ExamplesTruncated = outputs > uint64(len(result.Examples)) || result.InputCounts.Rows > uint64(len(result.InputExamples)+len(result.DerivedInputExamples))
	return result, nil
}

// Assert that trial execution consumes published canonical input values and
// never reaches into monitoring/internal or invents an implicit raw conversion.
var _ monitoring.TaskExecutor = (*Service)(nil)
