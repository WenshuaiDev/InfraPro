package monitoring

import (
	"math"
	"testing"
)

func ns(n int64) *int64 { return &n }
func TestNanosecondRanges(t *testing.T) {
	ranges := UnionWindows([]Window{{ns(5), ns(8)}, {ns(0), ns(4)}, {ns(4), ns(6)}, {ns(12), nil}})
	if len(ranges) != 2 || *ranges[0].From != 0 || *ranges[0].To != 8 {
		t.Fatal(ranges)
	}
	if !ranges[1].Contains(math.MaxInt64, nil) || ranges[0].Contains(8, nil) {
		t.Fatal("half-open or unbounded end")
	}
	if ranges[0].Contains(7, ns(-1)) || !ranges[0].Contains(7, ns(1)) || ranges[0].Contains(7, ns(7)) {
		t.Fatal("full positive interval required")
	}
	if !ranges[0].Covers(Window{ns(1), ns(8)}) || ranges[0].Covers(Window{nil, ns(8)}) {
		t.Fatal("coverage")
	}
	if _, ok := ranges[0].Intersect(Window{ns(8), ns(12)}); ok {
		t.Fatal("touching ranges have no intersection")
	}
	max := "2262-04-11T23:47:16.854775807Z"
	w, e := (Range{From: &max}).window()
	if e != nil || !w.Contains(math.MaxInt64, nil) || w.pg() != "[9223372036854775807,)" {
		t.Fatal(w, e)
	}
	back := w.rangeDTO()
	if back.From == nil || *back.From != max || back.To != nil {
		t.Fatal("range round trip")
	}
}
