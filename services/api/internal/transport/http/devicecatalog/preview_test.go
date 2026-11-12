package devicecataloghttp

import "testing"

func TestPreviewDiscriminatedFields(t *testing.T) {
	for _, raw := range []string{`{"action":"delete","expected_revision":"1","target_status":""}`, `{"action":"state","name":"","target_status":"active"}`, `{"action":"update","target_status":"","name":"ok"}`, `{"action":"delete","action":"update","name":"x"}`} {
		if _, e := manufacturerPreviewInput([]byte(raw)); e == nil {
			t.Errorf("accepted manufacturer input %s", raw)
		}
	}
	for _, raw := range []string{`{"action":"delete","version_id":""}`, `{"action":"delete","enabled":false}`, `{"action":"state","enabled":null}`} {
		if _, e := productPreviewInput([]byte(raw)); e == nil {
			t.Errorf("accepted product input %s", raw)
		}
	}
	for _, raw := range []string{`{"action":"delete","expected_revision":"1"}`, `{"action":"update","name":"ok","expected_revision":"1"}`} {
		if _, e := manufacturerPreviewInput([]byte(raw)); e != nil {
			t.Fatal(e)
		}
	}
}
