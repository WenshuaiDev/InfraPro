package derivedmonitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

type trialCheckpoint struct {
	Schema    string            `json:"schema"`
	Run       string            `json:"run_id"`
	Plan      string            `json:"plan"`
	Snapshot  string            `json:"snapshot"`
	Pending   []NanosecondRange `json:"pending"`
	Completed uint64            `json:"completed_parts,string"`
	Result    TrialResult       `json:"result"`
}

func partitionBudget(e error) bool {
	if e != nil && strings.Contains(e.Error(), "temporary_budget_exceeded") {
		return false
	}
	var raw *monitoring.Error
	return errors.As(e, &raw) && raw.Code == "budget_exceeded" || e != nil && strings.Contains(e.Error(), "budget_exceeded")
}

// A depth-first split needs at most 64 pending ranges. Each partition resolves
// its own complete halo against the original immutable snapshot. Only bounded
// examples and exact counts survive the partition; there is no result truncation.
func (s *Service) calculateTrial(ctx context.Context, lease monitoring.TaskLease, run runRecord, cfg ChannelConfig, p *compute.Plan, description inputSnapshotDetails, snapshotID string) (TrialResult, error) {
	output, e := run.OutputRange.rangeValue()
	if e != nil {
		return TrialResult{}, e
	}
	if ctx.Value(inlineTrialKey{}) == true {
		return s.calculateTrialPart(ctx, lease, run, cfg, p, description, snapshotID, output)
	}
	state := trialCheckpoint{Schema: "derived-trial-checkpoint/1", Run: run.ID, Plan: p.PlanHash, Snapshot: description.Snapshot.Hash, Pending: []NanosecondRange{run.OutputRange}}
	saved, found, e := lease.Checkpoint(ctx, "trial-progress")
	if e != nil {
		return state.Result, e
	}
	if found {
		var ref struct {
			Document string           `json:"document"`
			Inline   *trialCheckpoint `json:"inline"`
		}
		if e = json.Unmarshal(saved.Result, &ref); e != nil {
			return state.Result, e
		}
		if ref.Document != "" {
			if s.results == nil {
				return state.Result, fmt.Errorf("calculation_storage_unavailable")
			}
			if e = s.results.ReadDocument(ctx, run.Scope.Tenant, "checkpoints", ref.Document, &state); e != nil {
				return state.Result, e
			}
		} else if ref.Inline != nil {
			state = *ref.Inline
		} else {
			return state.Result, fmt.Errorf("trial_checkpoint_incomplete")
		}
		if state.Schema != "derived-trial-checkpoint/1" || state.Run != run.ID || state.Plan != p.PlanHash || state.Snapshot != description.Snapshot.Hash || len(state.Pending) > 65 {
			return TrialResult{}, fmt.Errorf("trial_checkpoint_mismatch")
		}
	}
	for len(state.Pending) > 0 {
		part, e := state.Pending[0].rangeValue()
		if e != nil {
			return state.Result, e
		}
		needs, e := immediateInputNeeds(cfg, p, part, analysisSource(run.Analysis))
		if e != nil {
			return state.Result, e
		}
		resolved, e := s.resolveRunInputs(ctx, lease.Actor(), run, snapshotID, monitoring.RawFreezeRequest{Needs: needs, Prefixes: inputPrefixes(p, cfg, needs, part, analysisSource(run.Analysis))})
		if e != nil {
			return state.Result, e
		}
		result, e := s.calculateTrialPart(ctx, lease, run, cfg, p, resolved, snapshotID, part)
		if e != nil {
			if !partitionBudget(e) || p.Rule.Clock.Kind == "whole_range" {
				return state.Result, e
			}
			end := int64(math.MaxInt64)
			if part.To != nil {
				end = *part.To
			}
			mid := new(big.Int).Add(big.NewInt(part.From), big.NewInt(end))
			mid.Div(mid, big.NewInt(2))
			at := mid.Int64()
			if at <= part.From || at >= end {
				return state.Result, &monitoring.TaskError{Code: "budget_exceeded", Message: "单个试算输出及其完整输入窗口超过预算。", Disposition: "needs_action"}
			}
			state.Pending = append([]NanosecondRange{rangeDTO(part.From, &at), rangeDTO(at, part.To)}, state.Pending[1:]...)
			continue
		}
		if state.Completed == 0 {
			state.Result = result
			state.Result.OutputRange = run.OutputRange
			state.Result.InputRanges = description.Needs
			state.Result.Sources = description.Sources
		} else {
			mergeTrial(&state.Result, result, s.options.TrialExamples)
		}
		state.Completed++
		state.Pending = state.Pending[1:]
		if e = s.saveTrialProgress(ctx, lease, run, state); e != nil {
			return state.Result, e
		}
		if e = lease.Progress(ctx, map[string]string{"phase": "calculating", "completed_parts": strconv.FormatUint(state.Completed, 10), "output_count": state.Result.OutputCount, "input_rows": strconv.FormatUint(state.Result.InputCounts.Rows, 10)}); e != nil {
			return state.Result, e
		}
	}
	return state.Result, nil
}

func (s *Service) saveTrialProgress(ctx context.Context, lease monitoring.TaskLease, run runRecord, state trialCheckpoint) error {
	progress := "running"
	if len(state.Pending) == 0 {
		progress = "completed"
	}
	if len(jsonBytes(state)) < 200<<10 {
		return lease.SaveCheckpoint(ctx, "trial-progress", progress, map[string]any{"inline": state})
	}
	if s.results == nil {
		return fmt.Errorf("calculation_storage_unavailable")
	}
	hash, e := s.results.SealDocument(ctx, run.Scope.Tenant, "checkpoints", state)
	if e != nil {
		return e
	}
	return lease.SaveCheckpoint(ctx, "trial-progress", progress, map[string]string{"document": hash})
}

func addDecimalCounts(a, b string) string {
	left, _ := strconv.ParseUint(a, 10, 64)
	right, _ := strconv.ParseUint(b, 10, 64)
	return strconv.FormatUint(left+right, 10)
}
func mergeTrial(total *TrialResult, part TrialResult, exampleLimit int) {
	total.OutputCount = addDecimalCounts(total.OutputCount, part.OutputCount)
	total.InputCounts.Rows += part.InputCounts.Rows
	total.InputCounts.Bytes += part.InputCounts.Bytes
	total.WaitingForInput = total.WaitingForInput && part.WaitingForInput
	for status, count := range part.States {
		total.States[status] = addDecimalCounts(total.States[status], count)
	}
	total.Examples = append(total.Examples, part.Examples[:min(len(part.Examples), max(0, exampleLimit-len(total.Examples)))]...)
	for _, row := range part.InputExamples {
		if len(total.InputExamples) >= 200 {
			break
		}
		duplicate := false
		for _, old := range total.InputExamples {
			duplicate = duplicate || old.Adoption == row.Adoption
		}
		if !duplicate {
			total.InputExamples = append(total.InputExamples, row)
		}
	}
	for _, row := range part.DerivedInputExamples {
		if len(total.DerivedInputExamples) >= 200 {
			break
		}
		duplicate := false
		for _, old := range total.DerivedInputExamples {
			duplicate = duplicate || old.Result == row.Result && old.Generation == row.Generation
		}
		if !duplicate {
			total.DerivedInputExamples = append(total.DerivedInputExamples, row)
		}
	}
	perNode := max(1, min(800, exampleLimit)/max(1, len(part.Nodes)))
	for _, node := range part.Nodes {
		for i := range total.Nodes {
			old := &total.Nodes[i]
			if old.NodeID != node.NodeID {
				continue
			}
			old.Samples = addDecimalCounts(old.Samples, node.Samples)
			for status, count := range node.States {
				old.States[status] = addDecimalCounts(old.States[status], count)
			}
			old.Examples = append(old.Examples, node.Examples[:min(len(node.Examples), max(0, perNode-len(old.Examples)))]...)
			break
		}
	}
	outputs, _ := strconv.ParseUint(total.OutputCount, 10, 64)
	total.ExamplesTruncated = outputs > uint64(len(total.Examples)) || total.InputCounts.Rows > uint64(len(total.InputExamples)+len(total.DerivedInputExamples))
}
