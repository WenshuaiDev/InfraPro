package devicecatalog_test

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"testing"
	"time"
)

func TestDeviceOutputsAndInstallationPeriods(t *testing.T) {
	f := setup(t)
	business := runtime.Compose(f.env.Service.Access())
	s := business.Devices
	scope := d.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "factory", Name: "厂家"}, access.Meta{})
	must(t, e)
	p := f.product(t, c.PlatformScope(), f.actor(), m.ID, "sensor", []c.Slot{f.fixed(), {Key: "input", Name: "采集槽位", Mode: "configurable", AllowedShapes: []string{"decimal"}}})
	create := d.DeviceCreate{Meta: f.meta(t), Code: "DEV-1", Name: "真实设备", ProductVersionID: p.VersionID}
	device, e := s.Create(ctx, f.owner, scope, create, access.Meta{})
	must(t, e)
	replay, e := s.Create(ctx, f.owner, scope, create, access.Meta{})
	must(t, e)
	if !replay.Replayed || device.ID != replay.ID {
		t.Fatal("duplicate hardware registration")
	}
	detail, e := s.Detail(ctx, f.owner, scope, device.ID)
	must(t, e)
	if detail.Status != "enabled" || detail.InstallationRevision != "0" {
		t.Fatal(detail)
	}
	slots, e := s.Slots(ctx, f.owner, scope, device.ID, d.Filter{})
	must(t, e)
	if len(slots.Items) != 2 {
		t.Fatal(slots)
	}
	var fixed *d.Output
	empty := 0
	for _, slot := range slots.Items {
		if slot.Current != nil {
			fixed = slot.Current
		} else {
			empty++
		}
	}
	if fixed == nil || empty != 1 || fixed.ValueShape != "decimal" {
		t.Fatal("fixed/configurable materialization")
	}
	input := d.OutputConfigure{Meta: f.meta(t), OutputFields: d.OutputFields{Name: "现场位移", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}, SlotKey: "input", OutputRevision: device.OutputRevision}
	output, e := s.ConfigureOutput(ctx, f.owner, scope, device.ID, input, access.Meta{})
	must(t, e)
	replace := d.OutputReplace{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: output.Revision}, OutputRevision: output.OutputRevision, OutputFields: d.OutputFields{Name: "现场横向位移", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time", ObservationDirection: "桥梁横向"}}
	preview, e := s.PreviewOutput(ctx, f.owner, scope, device.ID, output.OutputID, "replace", replace)
	must(t, e)
	replace.PreviewHash = preview.Hash
	next, e := s.ReplaceOutput(ctx, f.owner, scope, device.ID, output.OutputID, "replace", replace, access.Meta{})
	must(t, e)
	if next.OutputID == output.OutputID {
		t.Fatal("output semantic identity reused")
	}
	generations, e := s.Generations(ctx, f.owner, scope, device.ID, d.Filter{SlotKey: "input"})
	must(t, e)
	if len(generations.Items) != 2 || generations.Items[1].EndedAt == nil || !generations.Items[1].EndedAt.Equal(generations.Items[0].StartedAt) {
		t.Fatal("generation boundaries")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE device_outputs SET observation_direction='rewrite' WHERE id=$1`, next.OutputID)
	if e == nil {
		t.Fatal("SQL rewrote output semantics")
	}
	t0 := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Microsecond)
	t1 := t0.Add(time.Hour)
	t2 := t1.Add(time.Hour)
	install := d.InstallationCreateCommand(d.InstallationCreate{Meta: f.meta(t), InstallationFields: d.InstallationFields{Target: d.Target{Kind: "location"}, LocationDescription: "独立机柜"}, InstallationRevision: "0", InstalledAt: t0.Format(time.RFC3339Nano), UninstalledAt: t1.Format(time.RFC3339Nano)})
	installed, e := s.Install(ctx, f.owner, scope, device.ID, install, access.Meta{})
	must(t, e)
	install.Meta = f.meta(t)
	install.InstallationRevision = installed.InstallationRevision
	install.InstalledAt = t1.Format(time.RFC3339Nano)
	install.UninstalledAt = nil
	current, e := s.Install(ctx, f.owner, scope, device.ID, install, access.Meta{})
	must(t, e)
	// Adjacent intervals pass; overlap is rejected even by direct SQL.
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO device_installations(tenant_id,device_id,target_kind,location_description,installed_at,uninstalled_at,recorded_by,updated_by) VALUES($1,$2,'location','overlap',$3,$4,$5,$5)`, f.tenant, device.ID, t0.Add(time.Minute), t2, f.owner.ID)
	if e == nil {
		t.Fatal("overlapping installation accepted by database")
	}
	move := d.InstallationRelocateCommand(current.InstallationID, d.InstallationRelocate{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: current.Revision}, InstallationFields: d.InstallationFields{Target: d.Target{Kind: "location"}, LocationDescription: "另一机柜"}, InstallationRevision: current.InstallationRevision, MovedAt: t2.Format(time.RFC3339Nano)})
	preview, e = s.PreviewInstallation(ctx, f.owner, scope, device.ID, move)
	must(t, e)
	move.PreviewHash = preview.Hash
	moved, e := s.Install(ctx, f.owner, scope, device.ID, move, access.Meta{})
	must(t, e)
	history, e := s.Installations(ctx, f.owner, scope, device.ID, d.Filter{})
	must(t, e)
	if len(history.Items) != 3 || history.Items[0].ID != moved.InstallationID || !history.Items[1].UninstalledAt.Equal(history.Items[0].InstalledAt) {
		t.Fatal("relocation not atomic adjacent periods")
	}
	state := d.DeviceMutation{Action: "state", DeviceUpdate: d.DeviceUpdate{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: device.Revision}}, Status: "disabled"}
	preview, e = s.PreviewDevice(ctx, f.owner, scope, device.ID, state)
	must(t, e)
	state.PreviewHash = preview.Hash
	_, e = s.Mutate(ctx, f.owner, scope, device.ID, state, access.Meta{})
	must(t, e)
	detail, e = s.Detail(ctx, f.owner, scope, device.ID)
	must(t, e)
	if detail.CurrentInstallation == nil || detail.CurrentInstallation.ID != moved.InstallationID || detail.Status != "disabled" {
		t.Fatal("disabling lost installation")
	}
	deletion := d.DeviceMutation{Action: "delete", DeviceUpdate: d.DeviceUpdate{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: detail.Revision}}}
	preview, e = s.PreviewDevice(ctx, f.owner, scope, device.ID, deletion)
	must(t, e)
	if preview.CanSubmit || len(preview.Blockers) == 0 || preview.Blockers[0].Code != "device_referenced" {
		t.Fatal("business history deletion allowed")
	}
}
func TestInstallationCorrectionRetainsResourceReferences(t *testing.T) {
	f := setup(t)
	business := runtime.Compose(f.env.Service.Access())
	s := business.Devices
	scope := d.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "factory", Name: "厂家"}, access.Meta{})
	must(t, e)
	product := f.product(t, c.PlatformScope(), f.actor(), m.ID, "zero", nil)
	device, e := s.Create(ctx, f.owner, scope, d.DeviceCreate{Meta: f.meta(t), Code: "zero", Name: "无输出设备", ProductVersionID: product.VersionID}, access.Meta{})
	must(t, e)
	if device.OutputRevision != "0" {
		t.Fatal("zero-output device")
	}
	wm := func() eng.WriteMeta {
		meta := f.meta(t)
		return eng.WriteMeta{RequestID: meta.RequestID, Reason: meta.Reason}
	}
	typ, e := f.e.Create(ctx, f.actor(), eng.Scope{Kind: eng.TypeDefinition}, eng.CreateInput{WriteMeta: wm(), Code: "facility", Content: eng.Content{MetamodelVersion: "1", Name: "设施类型"}}, access.Meta{})
	must(t, e)
	typ, e = f.e.Publish(ctx, f.actor(), eng.Scope{Kind: eng.TypeDefinition}, typ.DefinitionID, eng.RevisionInput{WriteMeta: wm(), Revision: typ.Revision}, access.Meta{})
	must(t, e)
	raw, _ := json.Marshal(map[string]any{"request_id": f.meta(t).RequestID, "code": "facility", "name": "原安装设施", "type_version_id": typ.VersionID})
	facility, e := business.Resources.Mutate(ctx, f.owner, r.TenantScope(f.tenant), r.Target{Kind: r.Facility}, "create", raw, access.Meta{})
	must(t, e)
	install := d.InstallationCreateCommand(d.InstallationCreate{Meta: f.meta(t), InstallationFields: d.InstallationFields{Target: d.Target{Kind: "facility", ID: facility.ID}}, InstallationRevision: "0"})
	current, e := s.Install(ctx, f.owner, scope, device.ID, install, access.Meta{})
	must(t, e)
	history, e := s.Installations(ctx, f.owner, scope, device.ID, d.Filter{})
	must(t, e)
	correction := d.InstallationCorrectCommand(current.InstallationID, d.InstallationCorrect{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: current.Revision}, InstallationFields: d.InstallationFields{Target: d.Target{Kind: "location"}, LocationDescription: "纠正原录入错误，实际为独立位置"}, InstallationRevision: current.InstallationRevision, InstalledAt: history.Items[0].InstalledAt.Format(time.RFC3339Nano)})
	preview, e := s.PreviewInstallation(ctx, f.owner, scope, device.ID, correction)
	must(t, e)
	correction.PreviewHash = preview.Hash
	_, e = s.Install(ctx, f.owner, scope, device.ID, correction, access.Meta{})
	must(t, e)
	raw, _ = json.Marshal(map[string]any{"request_id": f.meta(t).RequestID, "expected_revision": facility.Revision, "reason": "误建删除"})
	impact, e := business.Resources.PreviewMutation(ctx, f.owner, r.TenantScope(f.tenant), r.Target{Kind: r.Facility, ID: facility.ID}, "delete", raw)
	must(t, e)
	if impact.CanSubmit || impact.BlockedCode != "resource_referenced" {
		t.Fatal("corrected-away installation target was unprotected")
	}
}
