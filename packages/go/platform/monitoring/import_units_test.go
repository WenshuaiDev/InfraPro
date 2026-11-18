package monitoring

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUnitColumnForNumericAndEnumRows(t *testing.T) {
	const tenant = "00000000-0000-4000-8000-000000000011"
	const channel = "00000000-0000-4000-8000-000000000012"
	for _, tc := range []struct {
		kind, fixed, file, value string
		valid                    bool
	}{
		{"decimal", "cm", "cm", "12.5", true}, {"decimal", "cm", "mm", "12.5", false},
		{"decimal", "cm", "", "12.5", false}, {"enum", "", "", "on", true}, {"enum", "", "cm", "on", false},
	} {
		in := validationInput{mapping: Mapping{UnitColumn: "unit"}, snapshot: validationSnapshot{
			Semantics: map[string]channelSemantics{channel: {Kind: tc.kind, Symbol: tc.fixed, Definition: json.RawMessage(`{"options":[{"key":"on"}]}`)}},
			Windows:   map[string][]Window{channel: {{}}},
		}}
		_, err := normalizeInput(in, Job{tenant: tenant, Target: "00000000-0000-4000-8000-000000000001"}, ChannelMapping{Channel: channel, Unit: tc.fixed}, 2, "value",
			inputCell{Text: tc.value}, inputCell{Text: "2024-01-01T00:00:00Z"}, inputCell{}, inputCell{}, inputCell{Text: tc.file}, inputCell{}, time.UTC, false)
		if (err == nil) != tc.valid {
			t.Errorf("%s unit %q with file %q: %v", tc.kind, tc.fixed, tc.file, err)
		}
	}
}
