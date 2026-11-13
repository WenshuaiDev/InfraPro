package compute

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/exactsort"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

func TestStreamingFixedWindowMatchesExactContract(t *testing.T) {
	for _, kind := range []string{"whole_range", "fixed_duration", "trailing_duration"} {
		for _, variant := range []string{"values", "excluded", "mixed_config", "empty"} {
			for _, op := range []string{"window_count", "window_sum", "window_mean", "window_variance", "window_stddev", "window_min", "window_max", "window_range", "window_median", "window_quantile"} {
				t.Run(kind+"/"+variant+"/"+op, func(t *testing.T) {
					w := &Window{Kind: kind}
					clock := Clock{Kind: "whole_range"}
					mode := "analysis"
					options := RunOptions{OutputRange: TimeRange{0, 100000000000}, SourceRange: TimeRange{0, 100000000000}, NowNS: 1000000000000}
					if kind != "whole_range" {
						w.DurationNS = "100000000000"
						if kind == "fixed_duration" {
							w.AnchorNS = "0"
						}
						clock = Clock{Kind: "fixed_grid", StepNS: "100000000000", AnchorNS: "0"}
						mode = "channel"
						options.OutputRange = TimeRange{100000000000, 100000000001}
						options.SourceRange = TimeRange{0, 100000000001}
					}
					params := Params{Window: w, MaxGapNS: "100000000000", MaxAgeNS: "100000000000"}
					if op == "window_quantile" {
						params.Percentile = "95"
					}
					r := baseRule(Node{ID: "stat", Op: op, Inputs: []string{"input:A"}, Params: params})
					r.Clock = clock
					if op == "window_count" {
						r.Outputs[0].Numeric.Scale = 0
					}
					p := compiled(t, r, mode)
					data := Data{Inputs: map[string][]Sample{"A": {}, "B": {}}, Complete: true}
					if variant != "empty" {
						for i, value := range []string{"14", "20"} {
							number, e := monitoringvalue.Decimal(value)
							if e != nil {
								t.Fatal(e)
							}
							sample := Sample{At: int64(i + 1), Value: number, Quality: "good", Status: "valid"}
							if variant == "excluded" && i == 0 {
								sample.Quality = "bad"
							}
							if variant == "mixed_config" && i == 1 {
								sample.ConfigID = "changed"
							}
							data.Inputs["A"] = append(data.Inputs["A"], sample)
						}
					}
					var want []Result
					if e := Run(t.Context(), p, data, options, func(row Result) error { want = append(want, row); return nil }); e != nil {
						t.Fatal(e)
					}
					tasks, ok, e := p.StreamingWindows(options)
					if e != nil || !ok {
						t.Fatal("not streaming eligible", e)
					}
					prepared := Data{Inputs: map[string][]Sample{"A": {}, "B": {}}, Windows: map[string][]Sample{}, Complete: true}
					for _, task := range tasks {
						acc, e := p.NewWindowAccumulator(task, exactsort.Options{Directory: t.TempDir(), MemoryBytes: 16 << 10, Budget: &exactsort.Budget{Limit: 1 << 20}, ChunkRows: 1, FanIn: 2})
						if e != nil {
							t.Fatal(e)
						}
						defer acc.Close()
						for _, row := range data.Inputs["A"] {
							if e = acc.Add(t.Context(), row); e != nil {
								t.Fatal(e)
							}
						}
						summary, e := acc.Finish(t.Context())
						if e != nil {
							t.Fatal(e)
						}
						raw, e := json.Marshal(summary)
						if e != nil {
							t.Fatal(e)
						}
						var restored WindowSummary
						if e = json.Unmarshal(raw, &restored); e != nil {
							t.Fatal(e)
						}
						sample, e := WindowSample(task, restored)
						if e != nil {
							t.Fatal(e)
						}
						prepared.Windows[task.Node.ID] = append(prepared.Windows[task.Node.ID], sample)
					}
					var got []Result
					if e = Run(t.Context(), p, prepared, options, func(row Result) error { got = append(got, row); return nil }); e != nil {
						t.Fatal(e)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("streaming changed complete result: got %+v want %+v", got, want)
					}
					if variant == "values" {
						expected := map[string]string{"window_count": "2", "window_sum": "34.000000000000000000", "window_mean": "17.000000000000000000", "window_variance": "9.000000000000000000", "window_stddev": "3.000000000000000000", "window_min": "14.000000000000000000", "window_max": "20.000000000000000000", "window_range": "6.000000000000000000", "window_median": "17.000000000000000000", "window_quantile": "19.700000000000000000"}
						if len(got) != 1 || got[0].Value == nil || *got[0].Value != expected[op] {
							t.Fatal("hand calculation mismatch", got)
						}
					}
				})
			}
		}
	}
}

func TestStreamingTrailingWindowBoundaryAndSampleBudget(t *testing.T) {
	r := baseRule(Node{ID: "count", Op: "window_count", Inputs: []string{"input:A"}, Params: Params{Window: &Window{Kind: "trailing_duration", DurationNS: "10000000000"}, MaxGapNS: "10000000000", MaxAgeNS: "10000000000"}})
	r.Clock = Clock{Kind: "fixed_grid", StepNS: "10000000000", AnchorNS: "0"}
	r.Outputs[0].Numeric.Scale = 0
	p := compiled(t, r, "channel")
	p.limits.WindowSamples = 2
	task := WindowTask{Node: r.Nodes[0], At: 10_000_000_000, Source: TimeRange{0, 10_000_000_001}}
	a, e := p.NewWindowAccumulator(task, exactsort.Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	v, e := monitoringvalue.Decimal("1")
	if e != nil {
		t.Fatal(e)
	}
	// (t-duration,t] excludes the left endpoint and includes the right.
	for _, at := range []int64{0, 1, 10_000_000_000, 10_000_000_001} {
		if e = a.Add(t.Context(), Sample{At: at, Value: v, Quality: "good", Status: "valid"}); e != nil {
			t.Fatal(e)
		}
	}
	if a.state.Count != 2 || *a.state.First != 1 || *a.state.Last != 10_000_000_000 {
		t.Fatal("streamed trailing window changed endpoint convention", a.state)
	}
	b, e := p.NewWindowAccumulator(task, exactsort.Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	for _, at := range []int64{1, 2} {
		if e = b.Add(t.Context(), Sample{At: at, Value: v, Quality: "good"}); e != nil {
			t.Fatal(e)
		}
	}
	if e = b.Add(t.Context(), Sample{At: 3, Value: v, Quality: "bad"}); e != nil {
		t.Fatal("excluded input consumed accepted-sample budget", e)
	}
	if e = b.Add(t.Context(), Sample{At: 4, Value: v, Quality: "good"}); e == nil || e.Error() != "window_budget_exceeded" {
		t.Fatal("oversized active window was silently accepted", e)
	}
}
