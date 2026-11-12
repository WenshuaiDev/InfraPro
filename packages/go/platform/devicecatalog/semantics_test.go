package devicecatalog_test

import (
	"encoding/json"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"os"
	"slices"
	"testing"
)

func TestSharedSemanticVectors(t *testing.T) {
	var fixture struct {
		Baseline c.Semantics
		Cases    []struct {
			Name   string
			Output c.Semantics
			Paths  []string
		}
		Normalization []struct{ Input, Expected string }
	}
	raw, e := os.ReadFile("../../../../contracts/fixtures/device-semantics.json")
	must(t, e)
	must(t, json.Unmarshal(raw, &fixture))
	for _, v := range fixture.Cases {
		t.Run(v.Name, func(t *testing.T) {
			got := []string{}
			for _, issue := range c.Compatible(v.Output, fixture.Baseline) {
				got = append(got, issue.Path)
			}
			if !slices.Equal(got, v.Paths) {
				t.Fatalf("%v != %v", got, v.Paths)
			}
		})
	}
	f := setup(t)
	for _, v := range fixture.Normalization {
		var got string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT device_semantic_text($1)`, v.Input).Scan(&got))
		meaning := c.Semantics{ReferenceBasis: v.Input}
		meaning.Normalize()
		if got != v.Expected || meaning.ReferenceBasis != v.Expected {
			t.Fatalf("normalization mismatch SQL=%q Go=%q expected=%q", got, meaning.ReferenceBasis, v.Expected)
		}
	}
}
