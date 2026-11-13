package compute

import (
	"fmt"
	"math/big"
	"math/rand/v2"
	"sort"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

// The independent oracle rescans small windows and sorts exact rationals. It
// catches stale sliding moments, duplicate-rank errors and wrong open bounds;
// it does not share the treap, sliding state or quantile implementation.
func TestSlidingStatisticsAgainstIndependentOracle(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261112, 42))
	data := Data{Complete: true, Inputs: map[string][]Sample{"A": {}, "B": {}}}
	for i := 0; i < 150; i++ {
		v := int64(rng.IntN(31) - 15)
		data.Inputs["A"] = append(data.Inputs["A"], Sample{At: int64(i) * minute, Value: monitoringvalue.Integer(v), Quality: "good", ID: fmt.Sprint(i)})
	}
	for _, op := range []string{"sum", "mean", "variance", "min", "max", "range", "median", "quantile"} {
		t.Run(op, func(t *testing.T) {
			n := rolling(op, "value", "input:A")
			n.Params.Window.DurationNS = "420000000000"
			n.Params.MinSamples = "1"
			if op == "quantile" {
				n.Params.Percentile = "95"
			}
			results := run(t, baseRule(n), data, "channel", RunOptions{OutputRange: TimeRange{0, 150 * minute}, NowNS: 150 * minute})
			for i, result := range results {
				samples := []*big.Rat{}
				sum, square := new(big.Rat), new(big.Rat)
				for _, s := range data.Inputs["A"] {
					if s.At <= result.At && s.At > result.At-7*minute {
						v, _ := s.Value.Exact()
						samples = append(samples, v)
						sum.Add(sum, v)
						square.Add(square, new(big.Rat).Mul(v, v))
					}
				}
				sort.Slice(samples, func(i, j int) bool { return samples[i].Cmp(samples[j]) < 0 })
				count := big.NewRat(int64(len(samples)), 1)
				want := new(big.Rat)
				switch op {
				case "sum":
					want.Set(sum)
				case "mean":
					want.Quo(sum, count)
				case "variance":
					want.Sub(new(big.Rat).Mul(count, square), new(big.Rat).Mul(sum, sum))
					want.Quo(want, new(big.Rat).Mul(count, count))
				case "min":
					want.Set(samples[0])
				case "max":
					want.Set(samples[len(samples)-1])
				case "range":
					want.Sub(samples[len(samples)-1], samples[0])
				case "median", "quantile":
					p := int64(50)
					if op == "quantile" {
						p = 95
					}
					numerator := int64(len(samples)-1) * p
					k := int(numerator / 100)
					want.Set(samples[k])
					if k+1 < len(samples) {
						want.Add(want, new(big.Rat).Mul(new(big.Rat).Sub(samples[k+1], samples[k]), big.NewRat(numerator%100, 100)))
					}
				}
				wantText := monitoringvalue.Format(monitoringvalue.RoundHalfEven(want, 18), 18)
				if result.Value == nil || *result.Value != wantText || result.Count != uint64(len(samples)) {
					t.Fatalf("row %d got %v want %s", i, result, wantText)
				}
			}
		})
	}
}
func TestAllTemplatesCompileWithoutCreatingBusinessRecords(t *testing.T) {
	for _, template := range Templates() {
		t.Run(template.Key, func(t *testing.T) {
			inputs := []Input{input("A")}
			if template.InputCount == 2 {
				inputs = append(inputs, input("B"))
			}
			rule, e := ExpandTemplate(template.Key, inputs)
			if e != nil {
				t.Fatal(e)
			}
			_ = compiled(t, rule, "channel")
		})
	}
}
