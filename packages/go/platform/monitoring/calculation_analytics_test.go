package monitoring

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCalculationStatisticsKeepCanonicalPrecisionAndSeparateLatest(t *testing.T) {
	acc := newCalculationAccumulator()
	values := []string{"10000000000000000000000000.000000000001", "10000000000000000000000000.000000000002"}
	for i := range values {
		row := CalculatedObservation{SourceKind: "derived", Channel: "x", Result: values[i], AtNS: int64(i), At: "2024-01-01T00:00:00Z", Value: &values[i], Status: "valid", Quality: "good"}
		if e := acc.add(row, []string{"good"}); e != nil {
			t.Fatal(e)
		}
	}
	if e := acc.add(CalculatedObservation{SourceKind: "derived", AtNS: 2, At: "2024-01-01T00:01:00Z", Status: "division_by_zero", Quality: "good"}, []string{"good"}); e != nil {
		t.Fatal(e)
	}
	digits := 18
	stat, e := acc.statistic(context.Background(), "x", QueryInput{Digits: &digits})
	if e != nil {
		t.Fatal(e)
	}
	if stat.Mean == nil || *stat.Mean != "10000000000000000000000000.000000000001500000" || stat.Stddev == nil || *stat.Stddev != "0.000000000000500000" {
		t.Fatal(stat.Mean, stat.Stddev)
	}
	if stat.N != "2" || stat.Derived.LogicalOutputs != "3" || stat.Derived.NoResults != "1" || stat.LatestOutput.Status != "division_by_zero" || stat.Last.Calculated.Status != "valid" {
		t.Fatal(stat)
	}
	// Cached result encoding must retain the full derived variant and identity.
	data, e := json.Marshal(stat)
	if e != nil {
		t.Fatal(e)
	}
	var replay ChannelStatistic
	if e = json.Unmarshal(data, &replay); e != nil {
		t.Fatal(e)
	}
	if replay.Last.Calculated == nil || *replay.Last.Calculated.Value != values[1] || replay.Last.Calculated.SourceKind != "derived" {
		t.Fatal(string(data), replay)
	}
}
