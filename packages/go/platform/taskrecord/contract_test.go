package taskrecord

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"testing"
	"time"
)

func validTaskChange() Change {
	id := uuid.NewString()
	return Change{SourceRef: SourceRef{Module: "monitoring", Kind: "job", RootID: id}, Type: "monitoring.export", Scope: Scope{Kind: "tenant", Tenant: uuid.NewString()}, OriginKind: "person", Originator: uuid.NewString(), ExecutorKind: "person", Generation: "9007199254740993", Revision: "1", ControlRevision: "1", State: "queued", Phase: "queued", InputRef: map[string]string{"job_id": id}, InputHash: "fixed-reference-hash", Fence: "0", CreatedAt: time.Now(), ObservedAt: time.Now()}
}
func TestTaskContractRejectsUnsafeFacts(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Change)
	}{
		{"unknown-source", func(c *Change) { c.Module = "arbitrary" }},
		{"scope-confusion", func(c *Change) { c.Scope.Project = uuid.NewString() }},
		{"unattributed-person", func(c *Change) { c.Originator = "" }},
		{"noncanonical-generation", func(c *Change) { c.Generation = "01" }},
		{"overflow", func(c *Change) { c.Generation = "9223372036854775808" }},
		{"raw-payload", func(c *Change) { c.InputRef["body"] = "sensitive input" }},
		{"download-url", func(c *Change) {
			c.ResultRef = map[string]string{"job_id": "https://example.invalid/download?token=secret"}
		}},
		{"fake-progress", func(c *Change) { n := "5"; c.Progress = Progress{Completed: "6", Total: &n, Unit: "rows"} }},
		{"missing-unit", func(c *Change) { c.Progress = Progress{Completed: "5"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := validTaskChange()
			if e := c.Validate(); e != nil {
				t.Fatal(e)
			}
			test.edit(&c)
			if c.Validate() == nil {
				t.Fatal("unsafe fact accepted")
			}
		})
	}
	c := validTaskChange()
	decoded, e := Decode(JSON(c))
	if e != nil || decoded.Generation != c.Generation {
		t.Fatal("lossy JSON integer", e)
	}
	if c.SourceRef.ID() != decoded.SourceRef.ID() {
		t.Fatal("unstable source identity")
	}
}

func TestOptionalEventActorPreservesLegacyFingerprint(t *testing.T) {
	c := validTaskChange()
	legacy := JSON(c)
	if bytes.Contains(legacy, []byte("event_actor")) {
		t.Fatal("empty optional field changed legacy bytes")
	}
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(legacy, &fields); e != nil {
		t.Fatal(e)
	}
	// Legacy representation is the exact producer JSON without the optional field.
	c.EventActor = uuid.NewString()
	withActor := JSON(c)
	if !bytes.Contains(withActor, []byte(`"event_actor"`)) || Hash(c) == Hash(json.RawMessage(legacy)) {
		t.Fatal("actual event actor not represented")
	}
	c.EventActor = ""
	if !bytes.Equal(JSON(c), legacy) {
		t.Fatal("legacy serialization changed")
	}
	c.EventActor = "not-an-account"
	if c.Validate() == nil {
		t.Fatal("invalid event actor accepted")
	}
}
