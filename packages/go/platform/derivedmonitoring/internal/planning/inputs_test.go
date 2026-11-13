package planning

import (
	"math"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
)

func TestInputPlanComposesOperatorHalosAndPrimaryTimeline(t *testing.T) {
	p := &compute.Plan{Rule: compute.Rule{
		Inputs: []compute.Input{{Key: "x"}, {Key: "y"}}, Clock: compute.Clock{Kind: "primary_input", Input: "x"},
		Nodes: []compute.Node{
			{ID: "difference", Op: "difference", Inputs: []string{"input:x"}, Params: compute.Params{DifferenceMode: "adjacent", MaxGapNS: "5"}},
			{ID: "combined", Op: "subtract", Inputs: []string{"node:difference", "input:y"}, Params: compute.Params{Alignment: "previous", ToleranceNS: "7"}},
			{ID: "average", Op: "window_mean", Inputs: []string{"node:combined"}, Params: compute.Params{Window: &compute.Window{Kind: "trailing_duration", DurationNS: "3"}}},
		}, Outputs: []compute.Output{{Node: "average"}},
	}}
	inputs, e := Inputs(p, Range{100, End(101)}, nil)
	if e != nil {
		t.Fatal(e)
	}
	// Window: [98,101); previous match: [91,101); difference: [86,101).
	if len(inputs["x"]) != 1 || inputs["x"][0].From != 86 || *inputs["x"][0].To != 101 || len(inputs["y"]) != 1 || inputs["y"][0].From != 91 {
		t.Fatal(inputs)
	}
	// A nearest analysis candidate must include the closed maximum timestamp.
	p.Rule.Nodes = []compute.Node{{ID: "average", Op: "subtract", Inputs: []string{"input:x", "input:y"}, Params: compute.Params{Alignment: "nearest", ToleranceNS: "9"}}}
	inputs, e = Inputs(p, Range{math.MaxInt64 - 3, End(math.MaxInt64)}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if inputs["x"][0].To != nil || inputs["y"][0].From != math.MaxInt64-12 {
		t.Fatal(inputs)
	}
}
func TestInputPlanCalendarDSTAndWholeRange(t *testing.T) {
	location, e := time.LoadLocation("America/New_York")
	if e != nil {
		t.Fatal(e)
	}
	at := time.Date(2024, 3, 11, 0, 0, 0, 0, location).UnixNano()
	p := &compute.Plan{Rule: compute.Rule{Inputs: []compute.Input{{Key: "x"}}, Clock: compute.Clock{Kind: "calendar", Timezone: "America/New_York", Period: "day"}, Nodes: []compute.Node{{ID: "w", Op: "window_mean", Inputs: []string{"input:x"}, Params: compute.Params{Window: &compute.Window{Kind: "calendar", Timezone: "America/New_York", Period: "day"}}}}, Outputs: []compute.Output{{Node: "w"}}}}
	inputs, e := Inputs(p, Range{at, End(at + 1)}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if at-inputs["x"][0].From != int64(23*time.Hour) {
		t.Fatal("calendar halo replaced with fixed day", inputs)
	}
	p.Rule.Clock = compute.Clock{Kind: "whole_range"}
	p.Rule.Nodes[0].Params.Window = &compute.Window{Kind: "whole_range"}
	source := Range{10, End(90)}
	inputs, e = Inputs(p, Range{90, End(91)}, &source)
	if e != nil {
		t.Fatal(e)
	}
	if inputs["x"][0].From != 10 || *inputs["x"][0].To != 90 {
		t.Fatal("source window changed with output viewport", inputs)
	}
}

func TestActualRankPrefixDoesNotRequireUnusedTimeHalo(t *testing.T) {
	p := &compute.Plan{Rule: compute.Rule{Inputs: []compute.Input{{Key: "x"}}, Clock: compute.Clock{Kind: "primary_input", Input: "x"}, Nodes: []compute.Node{{ID: "d", Op: "difference", Inputs: []string{"input:x"}, Params: compute.Params{DifferenceMode: "adjacent", MaxGapNS: "10000"}}}, Outputs: []compute.Output{{Node: "d"}}}}
	var prefix []Predecessor
	inputs, e := Inputs(p, Range{100, End(200)}, nil, &prefix)
	if e != nil {
		t.Fatal(e)
	}
	if len(prefix) != 1 || prefix[0].Before != 100 || prefix[0].Count != 1 || inputs["x"][0].From != 100 {
		t.Fatal(inputs, prefix)
	}
	p.Rule.Nodes[0] = compute.Node{ID: "d", Op: "window_mean", Inputs: []string{"input:x"}, Params: compute.Params{Window: &compute.Window{Kind: "trailing_count", Count: "5", MaxSpanNS: "10000"}}}
	prefix = nil
	inputs, e = Inputs(p, Range{100, End(200)}, nil, &prefix)
	if e != nil {
		t.Fatal(e)
	}
	if len(prefix) != 1 || prefix[0].Before != 101 || prefix[0].Count != 5 || inputs["x"][0].From != 100 {
		t.Fatal(inputs, prefix)
	}
	// Duration windows remain complete even when rank resolution is enabled.
	p.Rule.Nodes[0].Params.Window = &compute.Window{Kind: "trailing_duration", DurationNS: "50"}
	inputs, e = Inputs(p, Range{100, End(200)}, nil, &prefix)
	if e != nil || inputs["x"][0].From != 51 {
		t.Fatal(inputs, e)
	}
}
