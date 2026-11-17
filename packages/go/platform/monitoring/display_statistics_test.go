package monitoring

import (
	"context"
	"testing"

	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
)

func TestDisplayCalculatedCountsUseQualitySelectedPopulation(t *testing.T) {
	value := "9007199254740993.000001"
	acc := newCalculationAccumulator()
	for i, quality := range []string{"good", "good", "bad"} {
		if e := acc.add(CalculatedObservation{SourceKind: "derived", Channel: "x", Result: value, AtNS: int64(i), At: "2026-11-01T00:00:00Z", Value: &value, Status: "valid", Quality: quality}, []string{"good"}); e != nil {
			t.Fatal(e)
		}
	}
	if e := acc.add(CalculatedObservation{SourceKind: "derived", Channel: "x", AtNS: 4, At: "2026-11-01T00:01:00Z", Status: "division_by_zero", Quality: "good"}, []string{"good"}); e != nil {
		t.Fatal(e)
	}
	stat, e := acc.statistic(context.Background(), "x", QueryInput{})
	if e != nil {
		t.Fatal(e)
	}
	got := displayCounts([]dr.ChannelSummary{{ID: "x", SourceKind: "derived"}}, map[string]ChannelStatistic{"x": stat}, []string{"derived"})
	if got.Raw.State != "not_requested" || *got.Derived.Logical != "4" || *got.Derived.Valid != "3" || *got.Derived.NoResult != "1" || *got.Derived.Samples != "2" {
		t.Fatalf("logical/valid/sample populations conflated: %s", dr.Bytes(got))
	}
}
