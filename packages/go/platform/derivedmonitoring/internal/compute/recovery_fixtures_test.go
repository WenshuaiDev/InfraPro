package compute

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

func TestRecoveryFixturesExecuteWithRecordedEngine(t *testing.T) {
	raw, hash := RecoveryFixtures()
	if len(hash) != 64 {
		t.Fatal("missing fixture digest")
	}
	var fixtures struct {
		Schema string `json:"schema"`
		Cases  []struct {
			Operator   string    `json:"operator"`
			Percentile string    `json:"percentile"`
			Values     []string  `json:"values"`
			Times      []string  `json:"at_ns"`
			Scale      int       `json:"scale"`
			Expected   []*string `json:"expected"`
		} `json:"cases"`
	}
	if e := json.Unmarshal(raw, &fixtures); e != nil {
		t.Fatal(e)
	}
	if fixtures.Schema != "derived-numeric-fixtures/1" || len(fixtures.Cases) != 4 {
		t.Fatal("unexpected fixture contract")
	}
	for _, c := range fixtures.Cases {
		t.Run(c.Operator, func(t *testing.T) {
			n := Node{ID: "result", Op: c.Operator, Inputs: []string{"input:A"}, Params: Params{Window: &Window{Kind: "whole_range"}, Percentile: c.Percentile}}
			r := baseRule(n)
			r.Clock = Clock{Kind: "whole_range"}
			options := RunOptions{OutputRange: TimeRange{0, 1800000000001}, SourceRange: TimeRange{0, 1800000000001}, NowNS: 1800000000001}
			if c.Operator == "rate" {
				r.Clock = Clock{Kind: "primary_input", Input: "A"}
				r.Nodes[0].Params = Params{DifferenceMode: "adjacent", MaxGapNS: "1800000000000", RateTimeUnit: "minute", InvalidPolicy: "break"}
			}
			r.Outputs[0].Numeric.Scale = c.Scale
			data := Data{Inputs: map[string][]Sample{"A": {}, "B": {}}, Complete: true}
			for i, value := range c.Values {
				number, e := monitoringvalue.Decimal(value)
				if e != nil {
					t.Fatal(e)
				}
				at, e := strconv.ParseInt(c.Times[i], 10, 64)
				if e != nil {
					t.Fatal(e)
				}
				data.Inputs["A"] = append(data.Inputs["A"], Sample{At: at, Value: number, Status: "valid", Quality: "good"})
			}
			rows := run(t, r, data, "analysis", options)
			var values []*string
			for _, row := range rows {
				values = append(values, row.Value)
			}
			if !reflect.DeepEqual(values, c.Expected) {
				t.Fatal("backup fixture no longer matches this engine", rows, c.Expected)
			}
		})
	}
}
