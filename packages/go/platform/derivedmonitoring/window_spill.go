package derivedmonitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/exactsort"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

type spilledWindows struct {
	Schema          string                             `json:"schema"`
	Run             string                             `json:"run_id"`
	Plan            string                             `json:"plan"`
	Snapshot        string                             `json:"snapshot"`
	Tasks           string                             `json:"tasks"`
	Summaries       []compute.WindowSummary            `json:"summaries"`
	Counts          monitoring.RawReadCount            `json:"counts"`
	Examples        []monitoring.RawSample             `json:"input_examples"`
	DerivedExamples []monitoring.CalculatedObservation `json:"derived_input_examples"`
}

// spillWindows reads a complete fixed input snapshot as a stream. Summaries
// stay exact; quantile runs spill to private local files and verified S3 objects.
// The existing executor consumes the resulting aggregate nodes, so downstream
// formulas, numeric encoding and publication use the normal contracts.
func (s *Service) spillWindows(ctx context.Context, lease monitoring.TaskLease, run runRecord, cfg ChannelConfig, p *compute.Plan, snapshotID string, description inputSnapshotDetails, options compute.RunOptions, auth inputAccess) (compute.Data, spilledWindows, bool, error) {
	data := compute.Data{Inputs: map[string][]compute.Sample{}, Windows: map[string][]compute.Sample{}, Complete: true}
	tasks, eligible, e := p.StreamingWindows(options)
	if e != nil || !eligible || s.results == nil {
		return data, spilledWindows{}, false, e
	}
	identity := fingerprint([]any{p.PlanHash, description.Snapshot.Hash, tasks})
	key := "window-stream:" + identity
	state := spilledWindows{Schema: "derived-window-stream/1", Run: run.ID, Plan: p.PlanHash, Snapshot: description.Snapshot.Hash, Tasks: identity, Summaries: []compute.WindowSummary{}, Examples: []monitoring.RawSample{}, DerivedExamples: []monitoring.CalculatedObservation{}}
	saved, found, e := lease.Checkpoint(ctx, key)
	if e != nil {
		return data, state, true, e
	}
	if found && saved.State == "completed" {
		var ref struct {
			Document string `json:"document"`
		}
		if e = json.Unmarshal(saved.Result, &ref); e != nil {
			return data, state, true, e
		}
		if e = s.results.ReadDocument(ctx, run.Scope.Tenant, "checkpoints", ref.Document, &state); e != nil {
			return data, state, true, e
		}
		if state.Schema != "derived-window-stream/1" || state.Plan != p.PlanHash || state.Snapshot != description.Snapshot.Hash || state.Tasks != identity || len(state.Summaries) != len(tasks) {
			return data, state, true, fmt.Errorf("window_checkpoint_mismatch")
		}
	} else {
		budget := &exactsort.Budget{Limit: s.options.TemporaryBytes}
		accumulators := make([]*compute.WindowAccumulator, len(tasks))
		defer func() {
			for _, a := range accumulators {
				if a != nil {
					_ = a.Close()
				}
			}
		}()
		memory := min(uint64(8<<20), s.options.MemoryBytes/uint64(max(1, len(tasks))*4))
		for i, task := range tasks {
			index := i
			seal := func(c context.Context, path string, segment exactsort.Segment) error {
				if e := lease.Check(c); e != nil {
					return e
				}
				object, e := s.results.SealSortSegment(c, run.Scope.Tenant, run.ID, path, segment)
				if e != nil {
					return e
				}
				// Each sealed run is recoverable even when a later spill exceeds the
				// disk budget; formal results are still unpublished at this point.
				return lease.SaveCheckpoint(c, "sort:"+identity[:24]+":"+strconv.Itoa(index)+":"+segment.SHA256, "completed", map[string]any{"object": object, "segment": segment, "plan_hash": p.PlanHash, "snapshot_id": snapshotID, "node_id": task.Node.ID, "at_ns": strconv.FormatInt(task.At, 10), "aggregate": accumulators[index].Summary()})
			}
			a, e := p.NewWindowAccumulator(task, exactsort.Options{Directory: s.results.TempDirectory(), MemoryBytes: memory, Budget: budget, Seal: seal})
			if e != nil {
				return data, state, true, e
			}
			accumulators[i] = a
		}
		resumeRows, e := s.restoreWindowSource(ctx, lease, run, &state, accumulators)
		if e != nil {
			return data, state, true, e
		}
		if e = lease.Progress(ctx, map[string]string{"phase": "sorting", "windows": strconv.Itoa(len(tasks)), "resumed_input_rows": strconv.FormatUint(resumeRows, 10)}); e != nil {
			return data, state, true, e
		}
		var seen uint64
		state.Counts, e = s.readInputs(ctx, auth, snapshotID, description, monitoring.RawReadBudget{Rows: math.MaxUint64, Bytes: s.options.TemporaryBytes}, func(row inputSample) error {
			seen++
			// Frozen input readers have deterministic order and verify every
			// skipped source block. Reused values are not sorted or counted twice.
			if seen <= resumeRows {
				return nil
			}
			if row.Raw != nil && len(state.Examples) < 200 {
				state.Examples = append(state.Examples, *row.Raw)
			}
			if row.Derived != nil && len(state.DerivedExamples) < 200 {
				state.DerivedExamples = append(state.DerivedExamples, *row.Derived)
			}
			for i, task := range tasks {
				slot := strings.TrimPrefix(task.Node.Inputs[0], "input:")
				if cfg.InputBindings[slot].ChannelID == row.Channel {
					if e := accumulators[i].Add(ctx, row.Sample); e != nil {
						return e
					}
				}
			}
			if seen%16384 == 0 {
				return s.saveWindowSource(ctx, lease, run, state, accumulators, seen)
			}
			return nil
		})
		if e != nil {
			return data, state, true, e
		}
		if seen < resumeRows {
			return data, state, true, fmt.Errorf("window_checkpoint_source_mismatch")
		}
		for _, a := range accumulators {
			v, e := a.Finish(ctx)
			if e != nil {
				return data, state, true, e
			}
			state.Summaries = append(state.Summaries, v)
		}
		hash, e := s.results.SealDocument(ctx, run.Scope.Tenant, "checkpoints", state)
		if e != nil {
			return data, state, true, e
		}
		if e = lease.SaveCheckpoint(ctx, key, "completed", map[string]string{"document": hash}); e != nil {
			return data, state, true, e
		}
		if e = lease.Progress(ctx, map[string]string{"phase": "calculating", "input_rows": strconv.FormatUint(state.Counts.Rows, 10), "temporary_peak_bytes": strconv.FormatUint(budget.Peak, 10)}); e != nil {
			return data, state, true, e
		}
	}
	for _, input := range p.Rule.Inputs {
		data.Inputs[input.Key] = []compute.Sample{}
	}
	for i, task := range tasks {
		sample, e := compute.WindowSample(task, state.Summaries[i])
		if e != nil {
			return data, state, true, e
		}
		data.Windows[task.Node.ID] = append(data.Windows[task.Node.ID], sample)
	}
	return data, state, true, nil
}
