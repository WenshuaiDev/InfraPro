package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestCalculationSummaryRoundTripMergeMatchesExactScan(t *testing.T) {
	rows := []CalculatedObservation{}
	for i, v := range []string{"10000000000000000000000000.000000000001", "10000000000000000000000000.000000000002", "10000000000000000000000000.000000000001", "-0.000000000000000000000001", "0.000000000000000000000001"} {
		at := time.Date(2024, 1, 1, 0, 0, i, 1, time.UTC)
		q := "good"
		if i == 3 {
			q = "bad"
		}
		rows = append(rows, CalculatedObservation{SourceKind: "derived", Channel: "x", Result: fmt.Sprint(i), AtNS: at.UnixNano(), At: at.Format(time.RFC3339Nano), Value: &v, Status: "valid", Quality: q})
	}
	at := time.Date(2024, 1, 1, 0, 1, 0, 1, time.UTC)
	rows = append(rows, CalculatedObservation{SourceKind: "derived", Channel: "x", Result: "no-value", AtNS: at.UnixNano(), At: at.Format(time.RFC3339Nano), Status: "missing_input", Quality: "unmarked"})
	for _, qualities := range [][]string{allQualities, {"good"}, {"bad"}, {"unmarked"}} {
		exact, merged := newCalculationAccumulator(), newCalculationAccumulator()
		for _, r := range rows {
			if e := exact.add(r, qualities); e != nil {
				t.Fatal(e)
			}
		}
		// Deliberately merge the chunks out of temporal order after serialization.
		for _, chunk := range [][]CalculatedObservation{rows[3:], rows[:3]} {
			b := CalculationSummaryBuilder{}
			for _, r := range chunk {
				if e := b.Add(r); e != nil {
					t.Fatal(e)
				}
			}
			raw, e := json.Marshal(b.Summary())
			if e != nil {
				t.Fatal(e)
			}
			var summary CalculationSummary
			if e = json.Unmarshal(raw, &summary); e != nil {
				t.Fatal(e)
			}
			if e = merged.mergeSummary(summary, qualities); e != nil {
				t.Fatal(e)
			}
		}
		digits := 30
		in := QueryInput{Digits: &digits}
		want, e := exact.statistic(context.Background(), "x", in)
		if e != nil {
			t.Fatal(e)
		}
		got, e := merged.statistic(context.Background(), "x", in)
		if e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(got, want) {
			a, _ := json.Marshal(want)
			b, _ := json.Marshal(got)
			t.Fatalf("summary changed exact statistics for %v\nwant %s\ngot %s", qualities, a, b)
		}
	}
}
