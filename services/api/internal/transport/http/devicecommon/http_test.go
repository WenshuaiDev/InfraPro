package devicecommon

import (
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"testing"
)

func TestDeviceStrictNestedDTO(t *testing.T) {
	for _, raw := range []string{
		`{"content":{"schema_version":1,"output_slots":[{"key":"x","name":"X","mode":"configurable","allowed_shapes":["decimal"],"metric_version_id":"x"}]}}`,
		`{"content":{"schema_version":1,"output_slots":[{"key":"x","name":"X","mode":"fixed","allowed_shapes":["decimal"]}]}}`,
		`{"content":{"schema_version":1,"schema_version":1}}`,
		`{"content":{"schema_version":1,"output_slots":null}}`,
		`{"content":{"schema_version":1,"protocol_id":"x"}}`,
	} {
		var in c.DraftSave
		if e := Decode([]byte(raw), &in); e == nil {
			t.Fatalf("accepted nested ambiguity: %s", raw)
		}
	}
	var update d.DeviceUpdate
	if e := Decode([]byte(`{"name":null}`), &update); e == nil {
		t.Fatal("null update interpreted as omission")
	}
	var install d.InstallationCorrect
	if e := Decode([]byte(`{"target":{"kind":"location"},"position":null,"uninstalled_at":null}`), &install); e != nil {
		t.Fatal("explicit open period/absent position", e)
	}
}
