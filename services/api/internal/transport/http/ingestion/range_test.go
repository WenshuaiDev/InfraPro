package ingestionhttp

import "testing"

func TestRangeCannotEscapeVerifiedUnit(t *testing.T) {
	for _, v := range []struct {
		header     string
		start, end int
		ok         bool
	}{{"", 0, 81, true}, {"bytes=15-80", 15, 81, true}, {"bytes=80-", 80, 81, true}, {"bytes=-5", 76, 81, true}, {"bytes=0-81", 0, 0, false}, {"bytes=0-999999", 0, 0, false}, {"bytes=-82", 0, 0, false}, {"bytes=4-2", 0, 0, false}, {"bytes=0-2,4-7", 0, 0, false}, {"bytes=999999999999999999999999-", 0, 0, false}} {
		a, b, ok := rangeBounds(v.header, 81)
		if a != v.start || b != v.end || ok != v.ok {
			t.Fatal(v.header, a, b, ok)
		}
	}
}
