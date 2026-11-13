package derivedmonitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/exactsort"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

type windowSourceCheckpoint struct {
	Schema          string                             `json:"schema"`
	Run             string                             `json:"run_id"`
	Plan            string                             `json:"plan"`
	Snapshot        string                             `json:"snapshot"`
	Tasks           string                             `json:"tasks"`
	Rows            uint64                             `json:"processed_rows,string"`
	Windows         []compute.WindowCheckpoint         `json:"windows"`
	Examples        []monitoring.RawSample             `json:"input_examples"`
	DerivedExamples []monitoring.CalculatedObservation `json:"derived_input_examples"`
}

func (s *Service) restoreWindowSource(ctx context.Context, lease monitoring.TaskLease, run runRecord, state *spilledWindows, accumulators []*compute.WindowAccumulator) (uint64, error) {
	saved, found, e := lease.Checkpoint(ctx, "window-source:"+state.Tasks)
	if e != nil || !found {
		return 0, e
	}
	var ref struct {
		Document string `json:"document"`
	}
	if e = json.Unmarshal(saved.Result, &ref); e != nil {
		return 0, e
	}
	var checkpoint windowSourceCheckpoint
	if e = s.results.ReadDocument(ctx, run.Scope.Tenant, "checkpoints", ref.Document, &checkpoint); e != nil {
		return 0, e
	}
	if checkpoint.Schema != "derived-window-source/1" || checkpoint.Plan != state.Plan || checkpoint.Snapshot != state.Snapshot || checkpoint.Tasks != state.Tasks || len(checkpoint.Windows) != len(accumulators) || len(checkpoint.Examples) > 200 || len(checkpoint.DerivedExamples) > 200 {
		return 0, fmt.Errorf("window_checkpoint_mismatch")
	}
	for i, a := range accumulators {
		if e = a.Restore(ctx, checkpoint.Windows[i], func(c context.Context, segment exactsort.Segment) (io.ReadCloser, error) {
			if e := lease.Check(c); e != nil {
				return nil, e
			}
			return s.results.ReadSortSegment(c, run.Scope.Tenant, run.ID, segment)
		}); e != nil {
			return 0, e
		}
	}
	state.Examples, state.DerivedExamples = checkpoint.Examples, checkpoint.DerivedExamples
	return checkpoint.Rows, nil
}

func (s *Service) saveWindowSource(ctx context.Context, lease monitoring.TaskLease, run runRecord, state spilledWindows, accumulators []*compute.WindowAccumulator, rows uint64) error {
	checkpoint := windowSourceCheckpoint{Schema: "derived-window-source/1", Run: run.ID, Plan: state.Plan, Snapshot: state.Snapshot, Tasks: state.Tasks, Rows: rows, Examples: state.Examples, DerivedExamples: state.DerivedExamples}
	for _, a := range accumulators {
		part, e := a.Checkpoint(ctx)
		if e != nil {
			return e
		}
		checkpoint.Windows = append(checkpoint.Windows, part)
	}
	hash, e := s.results.SealDocument(ctx, run.Scope.Tenant, "checkpoints", checkpoint)
	if e != nil {
		return e
	}
	return lease.SaveCheckpoint(ctx, "window-source:"+state.Tasks, "running", map[string]string{"document": hash, "processed_rows": strconv.FormatUint(rows, 10)})
}
