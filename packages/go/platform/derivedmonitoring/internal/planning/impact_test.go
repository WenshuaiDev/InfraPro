package planning

import (
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
)

func TestImpactRetainsDistantSuccessorsAndComposesWindows(t *testing.T) {
	p := &compute.Plan{Rule: compute.Rule{Clock: compute.Clock{Kind: "primary_input", Input: "x"}, Nodes: []compute.Node{{ID: "d", Op: "difference", Inputs: []string{"input:x"}, Params: compute.Params{DifferenceMode: "adjacent", MaxGapNS: "5"}}, {ID: "mean", Op: "window_mean", Inputs: []string{"node:d"}, Params: compute.Params{Window: &compute.Window{Kind: "trailing_duration", DurationNS: "3"}}}}, Outputs: []compute.Output{{Node: "mean"}}}}
	got, wide, e := Impact(p, map[string][]Range{"x": {{10, End(11)}}}, Range{0, End(1000)})
	if e != nil || !wide || len(got) != 1 || got[0].From != 10 || *got[0].To != 1000 {
		t.Fatal("distant successor lost because max_gap was treated as identity horizon", got, wide, e)
	}
	p.Rule.Nodes = []compute.Node{{ID: "d", Op: "window_mean", Inputs: []string{"input:x"}, Params: compute.Params{Window: &compute.Window{Kind: "trailing_duration", DurationNS: "5"}}}, {ID: "mean", Op: "window_mean", Inputs: []string{"node:d"}, Params: compute.Params{Window: &compute.Window{Kind: "trailing_duration", DurationNS: "3"}}}}
	got, wide, e = Impact(p, map[string][]Range{"x": {{10, End(11)}}}, Range{13, End(1000)})
	// Changed input at 10 influences the first window at 10..14, then the
	// second at 10..16. An assignment starting at 13 must retain 13..16.
	if e != nil || wide || len(got) != 1 || got[0].From != 13 || *got[0].To != 17 {
		t.Fatal("internal halo clipped at assignment boundary", got, wide, e)
	}
}
