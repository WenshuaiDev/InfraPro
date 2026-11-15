package notificationevent

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"testing"
)

func TestTaskReferencePreservesV1Bytes(t *testing.T) {
	const legacy = `{"module":"monitoring","key":"synthetic:1:failed","type":"task.result","scope":{"kind":"tenant","tenant_id":"11111111-1111-4111-8111-111111111111"},"target":{"kind":"monitoring_job","id":"22222222-2222-4222-8222-222222222222"},"outcome":"failed","reason_code":"job_attempts_exhausted","recipients":[{"account_id":"33333333-3333-4333-8333-333333333333","policy":"personal","relationship":"affected_account"}]}`
	var event Event
	if e := json.Unmarshal([]byte(legacy), &event); e != nil {
		t.Fatal(e)
	}
	if e := event.normalize(); e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(event)
	if e != nil {
		t.Fatal(e)
	}
	if string(raw) != legacy {
		t.Fatalf("v1 serialization changed: %s", raw)
	}
	event.TaskReference = &taskrecord.Ref{SourceRef: taskrecord.SourceRef{Module: "monitoring", Kind: "job", RootID: event.Target.ID}, Generation: "9223372036854775806"}
	if e = event.normalize(); e != nil {
		t.Fatal(e)
	}
	raw, e = json.Marshal(event)
	if e != nil {
		t.Fatal(e)
	}
	var restored Event
	if e = json.Unmarshal(raw, &restored); e != nil {
		t.Fatal(e)
	}
	if restored.TaskReference.Generation != event.TaskReference.Generation {
		t.Fatal("lost decimal generation")
	}
	event.TaskReference.TaskID = event.Target.ID
	if event.normalize() == nil {
		t.Fatal("source could assert public task identity")
	}
}
