package monitoring

import "testing"

func TestStrictMonitoringDTO(t *testing.T) {
	for _, raw := range []string{`{"reason":"a","reason":"b"}`, `{"reason":"a","Reason":"b"}`, `{"Expected_Revision":"2"}`, `{"range":{"from":null,"from":"2024-01-01T00:00:00Z"}}`, `{"channel_id":null}`, `{"expected_revision":9007199254740993}`, `{"unexpected":true}`, `[]`, `{} {}`} {
		var out GrantInput
		if DecodeRequest([]byte(raw), &out) == nil {
			t.Fatal("ambiguous DTO accepted", raw)
		}
	}
	var out GrantInput
	if e := DecodeRequest([]byte(`{"range":{"from":"2262-04-11T23:47:16.854775807Z","to":null},"expected_revision":"9007199254740993"}`), &out); e != nil || out.ExpectedRevision != "9007199254740993" || out.Range.To != nil {
		t.Fatal(out, e)
	}
}
