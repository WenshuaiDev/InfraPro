package derivedmonitoring

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

func TestRecursiveInputPlanKeepsPerAssignmentLeafRanges(t *testing.T) {
	ctx := context.Background()
	window := func(channel, config, width string, r planning.Range) inputConfigSegment {
		return inputConfigSegment{Range: r, Config: ChannelConfig{ID: config, InputBindings: map[string]InputBinding{"x": {ChannelID: channel}}}, Plan: &compute.Plan{Rule: compute.Rule{Clock: compute.Clock{Kind: "primary_input", Input: "x"}, Inputs: []compute.Input{{Key: "x"}}, Nodes: []compute.Node{{ID: "mean", Op: "window_mean", Inputs: []string{"input:x"}, Params: compute.Params{Window: &compute.Window{Kind: "trailing_duration", DurationNS: width}}}}, Outputs: []compute.Output{{Node: "mean"}}}}}
	}
	from, to := int64(90), int64(110)
	resolve := func(_ context.Context, channel string, r planning.Range) (inputProfile, error) {
		if channel != "derived" {
			return inputProfile{Kind: "direct"}, nil
		}
		old := window("raw-old", "config-old", "10", planning.Range{From: 0, To: planning.End(100)})
		old.Baselines = []monitoring.RawNeed{{Channel: "basis", From: planning.End(1), To: planning.End(5)}}
		return inputProfile{Kind: "derived", Segments: []inputConfigSegment{old, window("raw-new", "config-new", "20", planning.Range{From: 100})}}, nil
	}
	plan, e := planInputRequirements(ctx, []monitoring.RawNeed{{Channel: "derived", From: &from, To: &to}}, resolve, 8)
	if e != nil {
		t.Fatal(e)
	}
	if len(plan.Raw) != 3 || len(plan.Derived) != 1 || len(plan.Configs) != 2 {
		t.Fatal(plan)
	}
	byChannel := map[string]monitoring.RawNeed{}
	for _, need := range plan.Raw {
		byChannel[need.Channel] = need
	}
	if *byChannel["raw-old"].From != 81 || *byChannel["raw-old"].To != 100 || *byChannel["raw-new"].From != 81 || *byChannel["raw-new"].To != 110 || *byChannel["basis"].From != 1 || *byChannel["basis"].To != 5 {
		t.Fatal(plan)
	}
}

func TestRecursiveInputPlanRejectsCycleAndExcessDepth(t *testing.T) {
	ctx := context.Background()
	from, to := int64(100), int64(101)
	pass := func(id, next string, r planning.Range) inputProfile {
		return inputProfile{Kind: "derived", Segments: []inputConfigSegment{{Range: r, Config: ChannelConfig{ID: id, InputBindings: map[string]InputBinding{"x": {ChannelID: next}}}, Plan: &compute.Plan{Rule: compute.Rule{Clock: compute.Clock{Kind: "primary_input", Input: "x"}, Inputs: []compute.Input{{Key: "x"}}, Nodes: []compute.Node{{ID: "value", Op: "formula", Params: compute.Params{Variables: map[string]string{"x": "input:x"}}}}, Outputs: []compute.Output{{Node: "value"}}}}}}}
	}
	resolve := func(_ context.Context, id string, r planning.Range) (inputProfile, error) {
		if id == "raw" {
			return inputProfile{Kind: "direct"}, nil
		}
		var n int
		if _, e := fmt.Sscanf(id, "node-%d", &n); e != nil {
			return inputProfile{}, e
		}
		next := "raw"
		if n < 8 {
			next = fmt.Sprintf("node-%d", n+1)
		}
		return pass(id, next, r), nil
	}
	needs := []monitoring.RawNeed{{Channel: "node-1", From: &from, To: &to}}
	plan, e := planInputRequirements(ctx, needs, resolve, 8)
	if e != nil || len(plan.Derived) != 8 || len(plan.Raw) != 1 {
		t.Fatal(plan, e)
	}
	_, e = planInputRequirements(ctx, needs, resolve, 7)
	var de *Error
	if !errors.As(e, &de) || de.Code != "dependency_depth_exceeded" {
		t.Fatal(e)
	}
	cycle := func(_ context.Context, id string, r planning.Range) (inputProfile, error) {
		next := "node-1"
		if id == "node-1" {
			next = "node-2"
		}
		return pass(id, next, r), nil
	}
	_, e = planInputRequirements(ctx, needs, cycle, 8)
	if !errors.As(e, &de) || de.Code != "dependency_cycle" {
		t.Fatal(e)
	}
}
