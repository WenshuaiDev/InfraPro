package resources_test

import (
	"encoding/json"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"os"
	"testing"
)

func TestSharedPositionRules(t *testing.T) {
	b, e := os.ReadFile("../../../../tests/fixtures/resource-positions.json")
	must(t, e)
	var cases []struct {
		Name     string
		Position res.Position
		Valid    bool
	}
	must(t, json.Unmarshal(b, &cases))
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			e := res.ValidatePosition(&c.Position)
			if (e == nil) != c.Valid {
				t.Fatalf("valid=%v: %v", c.Valid, e)
			}
		})
	}
}
