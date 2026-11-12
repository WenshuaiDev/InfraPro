package monitoring

import (
	"encoding/json"
	"math/rand"
	"testing"
	"time"
)

func TestSummaryMergeMatchesExactObservations(t *testing.T) {
	rows := []rawObservation{}
	for i := 0; i < 257; i++ {
		rows = append(rows, rawObservation{At: int64(i), Observation: countText(int64(i)), Adoption: countText(int64(i)), Kind: "decimal", Value: []string{"99999999999999999999999999.000000000001", "99999999999999999999999999.000000000003", "-0.000000000001"}[i%3], Quality: allQualities[i%4], First: true})
	}
	want := summaryTotals{}
	for _, r := range rows {
		want.add(r, true)
	}
	for seed := int64(0); seed < 10; seed++ {
		random := rand.New(rand.NewSource(seed))
		shuffled := append([]rawObservation(nil), rows...)
		random.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		parts := make([]summaryTotals, 13)
		for i, r := range shuffled {
			parts[i%len(parts)].add(r, true)
		}
		got := summaryTotals{}
		for _, part := range parts {
			raw, _ := json.Marshal(part)
			var decoded summaryTotals
			if e := json.Unmarshal(raw, &decoded); e != nil {
				t.Fatal(e)
			}
			got.merge(decoded)
		}
		if digest(got) != digest(want) {
			t.Fatal("merge depends on partition or order", seed)
		}
	}
	state := exactState{}
	for i, value := range []string{"99999999999999999999999999.000000000001", "99999999999999999999999999.000000000003"} {
		state.add(&summaryPoint{At: int64(i), ID: countText(int64(i)), Kind: "decimal", Value: value})
	}
	mean, std, e := state.moments(12)
	if e != nil || *mean != "99999999999999999999999999.000000000002" || *std != "0.000000000001" {
		t.Fatal(mean, std, e)
	}
}
func TestSummaryBoundariesAndCalendar(t *testing.T) {
	loc, e := time.LoadLocation("America/New_York")
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		start, period string
		hours         int
	}{{"2024-03-10", "day", 23}, {"2024-11-03", "day", 25}, {"2024-02-01", "month", 696}, {"2024-01-01", "year", 8784}} {
		start, e := time.ParseInLocation("2006-01-02", tc.start, loc)
		if e != nil {
			t.Fatal(e)
		}
		if calendarNext(start, tc.period).Sub(start) != time.Duration(tc.hours)*time.Hour {
			t.Fatal(tc)
		}
	}
	max := int64(1<<63 - 1)
	window := minuteWindow(minuteKey(max))
	if window.To != nil || !window.Contains(max, nil) {
		t.Fatal("maximum minute overflow", window)
	}
	for _, ns := range []int64{-2208988800000000000, -60000000001, -60000000000, -1, 0, 1, max} {
		if !minuteWindow(minuteKey(ns)).Contains(ns, nil) {
			t.Fatal(ns)
		}
	}
	s := summaryTotals{}
	s.add(rawObservation{At: 30, Start: ptrNS(10), Kind: "integer", Value: "2", Quality: "good", First: true}, true)
	auth := []Window{{From: ptrNS(0), To: ptrNS(40)}}
	query := Window{From: ptrNS(20), To: ptrNS(40)}
	if !s.covered(auth, &query) {
		t.Fatal("observed-at filter was mistaken for interval authorization")
	}
	if s.covered([]Window{{From: ptrNS(20), To: ptrNS(40)}}, &query) {
		t.Fatal("partial interval incorrectly authorized")
	}
}
func ptrNS(n int64) *int64 { return &n }
