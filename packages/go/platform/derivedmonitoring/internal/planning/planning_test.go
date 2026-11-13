package planning

import (
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"math"
	"reflect"
	"testing"
)

func TestGraphUnionCycleAndEightLevels(t *testing.T) {
	graph := map[string][]string{}
	for i := 0; i < 8; i++ {
		source := "raw"
		if i > 0 {
			source = fmt.Sprint(i - 1)
		}
		graph[fmt.Sprint(i)] = []string{source, source}
	}
	order, e := Order(graph, 8)
	if e != nil || len(order) != 8 || order[0] != "0" || order[7] != "7" {
		t.Fatal(order, e)
	}
	graph["8"] = []string{"7"}
	_, e = Order(graph, 8)
	var ge *GraphError
	if !errors.As(e, &ge) || ge.Code != "dependency_depth_exceeded" {
		t.Fatal(e)
	}
	delete(graph, "8")
	graph["0"] = []string{"raw", "7"}
	_, e = Order(graph, 8)
	if !errors.As(e, &ge) || ge.Code != "dependency_cycle" {
		t.Fatal(e)
	}
	graph = map[string][]string{"A": {"raw"}, "B": {"A"}, "C": {"raw"}, "D": {"B", "C"}, "other": {"unrelated"}}
	got, e := Dependents(graph, []string{"raw"}, 8)
	if e != nil || !reflect.DeepEqual(got, []string{"A", "B", "C", "D"}) {
		t.Fatal(got, e)
	}
}
func TestRangeUnionCoverageAndMaxDomain(t *testing.T) {
	ranges := []Range{{30, End(40)}, {10, End(20)}, {20, End(30)}, {50, nil}}
	got, e := Union(ranges)
	if e != nil || !reflect.DeepEqual(got, []Range{{10, End(40)}, {50, nil}}) {
		t.Fatal(got, e)
	}
	required := []Range{{12, End(39)}, {math.MaxInt64, nil}}
	ok, e := Covered(required, ranges)
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	ok, e = Covered([]Range{{39, End(51)}}, ranges)
	if e != nil || ok {
		t.Fatal("permission gap ignored", ok, e)
	}
	halo, e := MatchHalo(Range{math.MaxInt64 - 3, nil}, 100, true)
	if e != nil || halo.To != nil || halo.From != math.MaxInt64-103 {
		t.Fatal(halo, e)
	}
	low := monitoringvalue.MinTime.UnixNano()
	halo, e = DurationHalo(Range{low + 1, End(low + 2)}, 100)
	if e != nil || halo.From != low {
		t.Fatal(halo, e)
	}
}
func TestDurationInfluenceAgainstDiscreteOracle(t *testing.T) {
	for from := int64(0); from < 10; from++ {
		for to := from + 1; to < 12; to++ {
			for width := int64(1); width < 8; width++ {
				impact, e := DurationImpact(Range{from, End(to)}, width)
				if e != nil {
					t.Fatal(e)
				}
				for at := int64(0); at < 25; at++ {
					hit := false
					for source := from; source < to; source++ {
						hit = hit || (source > at-width && source <= at)
					}
					if impact.Contains(at) != hit {
						t.Fatal(from, to, width, at, impact)
					}
				}
			}
		}
	}
}
func TestNewerDirtyIsNotClearedByOlderCompletion(t *testing.T) {
	remainder := Subtract(Range{0, End(100)}, Range{30, End(70)})
	if !reflect.DeepEqual(remainder, []Range{{0, End(30)}, {70, End(100)}}) {
		t.Fatal(remainder)
	}
	if got := Subtract(Range{math.MaxInt64 - 10, nil}, Range{math.MaxInt64, nil}); !reflect.DeepEqual(got, []Range{{math.MaxInt64 - 10, End(math.MaxInt64)}}) {
		t.Fatal(got)
	}
}
