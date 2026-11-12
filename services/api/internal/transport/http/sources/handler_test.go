package sourceshttp

import "testing"

func TestSourceDTORejectsAmbiguousAndInapplicableFields(t *testing.T) {
	for _, raw := range []string{
		`{"action":"bind","expected_binding_id":"x"}`,
		`{"action":"unbind","output_id":"x"}`,
		`{"action":"switch","effective_at":"2026-11-11T00:00:00Z"}`,
		`{"action":"bind","action":"unbind"}`,
		`{"action":"bind","expected_source_revision":0}`,
		`{"action":"bind","request_id":null}`,
		`{"action":"unknown"}`,
		`null`,
		`{} {}`,
	} {
		if _, e := decode([]byte(raw), "preview"); e == nil {
			t.Fatalf("accepted ambiguous source: %s", raw)
		}
	}
	if _, e := decode([]byte(`{"action":"bind","expected_source_revision":"0","expected_channel_revision":"1","output_id":"test","expected_new_device_revision":"1","expected_new_output_revision":"1","request_id":"test"}`), "preview"); e != nil {
		t.Fatal(e)
	}
}
