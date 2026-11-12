package devicecatalog_test

import (
	"errors"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
)

func TestDeviceCreationRollbackAndOutputValidation(t *testing.T) {
	f := setup(t)
	s := runtime.Compose(f.env.Service.Access()).Devices
	scope := d.TenantScope(f.tenant)
	m, err := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "rollback", Name: "厂家"}, access.Meta{})
	must(t, err)
	p := f.product(t, c.PlatformScope(), f.actor(), m.ID, "mixed", []c.Slot{f.fixed(), {Key: "numeric", Name: "数值输入", Mode: "configurable", AllowedShapes: []string{"decimal"}}, {Key: "state", Name: "状态输入", Mode: "configurable", AllowedShapes: []string{"enum"}}})
	input := d.DeviceCreate{Meta: f.meta(t), Code: "Device-A", Name: "相同名称", ProductVersionID: p.VersionID}
	_, err = f.env.Pool.Exec(ctx, `CREATE FUNCTION reject_fixed_output() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected output failure'; END $$; CREATE TRIGGER reject_fixed_output BEFORE INSERT ON device_outputs FOR EACH ROW EXECUTE FUNCTION reject_fixed_output()`)
	must(t, err)
	_, err = s.Create(ctx, f.owner, scope, input, access.Meta{})
	if err == nil {
		t.Fatal("fixed-output failure committed device")
	}
	var devices, history, receipts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM devices), (SELECT count(*) FROM device_resource_changes), (SELECT count(*) FROM device_operation_receipts WHERE request_id=$1)`, input.RequestID).Scan(&devices, &history, &receipts))
	if devices != 0 || history != 0 || receipts != 0 {
		t.Fatalf("partial creation: device=%d history=%d receipt=%d", devices, history, receipts)
	}
	_, err = f.env.Pool.Exec(ctx, `DROP TRIGGER reject_fixed_output ON device_outputs; DROP FUNCTION reject_fixed_output()`)
	must(t, err)
	created, err := s.Create(ctx, f.owner, scope, input, access.Meta{})
	must(t, err)
	input.Meta = f.meta(t)
	input.Code = "device-a"
	_, err = s.Create(ctx, f.owner, scope, input, access.Meta{})
	wantCode(t, err, "code_taken")
	input.Meta = f.meta(t)
	input.Code = "Device-B"
	_, err = s.Create(ctx, f.owner, scope, input, access.Meta{})
	must(t, err) // Names do not merge identities or reserve codes.
	wm := func() eng.WriteMeta { m := f.meta(t); return eng.WriteMeta{RequestID: m.RequestID, Reason: m.Reason} }
	metric, err := f.e.Create(ctx, f.actor(), eng.Scope{Kind: eng.MetricDefinition}, eng.CreateInput{WriteMeta: wm(), Code: "state", Content: eng.Content{MetamodelVersion: "1", Name: "状态", Metric: &eng.Metric{Shape: "enum", Options: []eng.Option{{Key: "normal", Name: "正常"}, {Key: "alarm", Name: "异常"}}}}}, access.Meta{})
	must(t, err)
	metric, err = f.e.Publish(ctx, f.actor(), eng.Scope{Kind: eng.MetricDefinition}, metric.DefinitionID, eng.RevisionInput{WriteMeta: wm(), Revision: metric.Revision}, access.Meta{})
	must(t, err)
	for _, tc := range []struct {
		name, slot, metric, path, code string
		unit                           *string
	}{
		{"disallowed shape", "numeric", metric.VersionID, "/metric_version_id", "shape_not_allowed", nil},
		{"missing numeric unit", "numeric", f.metric, "/unit_version_id", "unit", nil},
		{"enum unit", "state", metric.VersionID, "/unit_version_id", "enum_unit", &f.unit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.ConfigureOutput(ctx, f.owner, scope, created.ID, d.OutputConfigure{Meta: f.meta(t), SlotKey: tc.slot, OutputRevision: created.OutputRevision, OutputFields: d.OutputFields{Name: "现场输入", MetricVersionID: tc.metric, UnitVersionID: tc.unit, TimestampMeaning: "observation_time"}}, access.Meta{})
			var failure *c.Error
			if !errors.As(err, &failure) || len(failure.Issues) != 1 || failure.Issues[0].Path != tc.path || failure.Issues[0].Code != tc.code {
				t.Fatalf("field validation: %+v, %v", failure, err)
			}
		})
	}
	slots, err := s.Slots(ctx, f.owner, scope, created.ID, d.Filter{})
	must(t, err)
	for _, slot := range slots.Items {
		if slot.Template.Mode == "configurable" && slot.Current != nil {
			t.Fatal("failed configuration materialized output")
		}
	}
	current, err := s.Detail(ctx, f.owner, scope, created.ID)
	must(t, err)
	if current.OutputRevision != created.OutputRevision {
		t.Fatal("failed configuration changed output collection")
	}
}
