package compute

import (
	"context"
	"fmt"
	"io"
	"math/big"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/exactsort"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

type WindowCheckpoint struct {
	Summary  WindowSummary       `json:"summary"`
	Previous *int64              `json:"previous_ns,string,omitempty"`
	Segments []exactsort.Segment `json:"segments"`
}

func (a *WindowAccumulator) Checkpoint(ctx context.Context) (WindowCheckpoint, error) {
	out := WindowCheckpoint{Summary: a.Summary(), Previous: a.previous, Segments: []exactsort.Segment{}}
	if a.sorter != nil {
		var e error
		out.Segments, e = a.sorter.Checkpoint(ctx)
		if e != nil {
			return out, e
		}
	}
	return out, nil
}

func (a *WindowAccumulator) Restore(ctx context.Context, saved WindowCheckpoint, fetch func(context.Context, exactsort.Segment) (io.ReadCloser, error)) error {
	v := saved.Summary
	if a.previous != nil || a.state.Count != 0 || v.Count > 0 && (v.First == nil || v.Last == nil || saved.Previous == nil || *v.First > *v.Last || *v.Last > *saved.Previous) {
		return fmt.Errorf("invalid_window_checkpoint")
	}
	parse := func(text string) (*big.Rat, error) {
		r, ok := new(big.Rat).SetString(text)
		if !ok {
			return nil, fmt.Errorf("invalid_window_checkpoint")
		}
		return r, monitoringvalue.Check(r)
	}
	sum, e := parse(v.Sum)
	if e != nil {
		return e
	}
	squares, e := parse(v.Squares)
	if e != nil {
		return e
	}
	if _, ok := a.maximumGap.SetString(v.MaximumGap, 10); !ok || a.maximumGap.Sign() < 0 {
		return fmt.Errorf("invalid_window_checkpoint")
	}
	if v.Count > 0 {
		a.minimum, e = parse(v.Min)
		if e != nil {
			return e
		}
		a.maximum, e = parse(v.Max)
		if e != nil {
			return e
		}
	}
	a.moments.N = v.Count
	a.moments.Sum.Set(sum)
	a.moments.Squares.Set(squares)
	var rows uint64
	for _, segment := range saved.Segments {
		if ^uint64(0)-rows < segment.Rows {
			return fmt.Errorf("invalid_window_checkpoint")
		}
		rows += segment.Rows
	}
	if a.sorter != nil {
		if rows != v.Count {
			return fmt.Errorf("sort_count_mismatch")
		}
		if e = a.sorter.Restore(ctx, saved.Segments, fetch); e != nil {
			return e
		}
	} else if len(saved.Segments) != 0 {
		return fmt.Errorf("invalid_window_checkpoint")
	}
	a.state, a.previous = v, saved.Previous
	return nil
}
