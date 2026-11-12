package devicecatalog_test

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	src "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"strconv"
	"sync"
	"testing"
)

func (f fixture) sourceTargets(t *testing.T, b runtime.Business) []src.Target {
	wm := func() eng.WriteMeta { m := f.meta(t); return eng.WriteMeta{RequestID: m.RequestID, Reason: m.Reason} }
	typ, e := f.e.Create(ctx, f.actor(), eng.Scope{Kind: eng.TypeDefinition}, eng.CreateInput{WriteMeta: wm(), Code: "facility", Content: eng.Content{MetamodelVersion: "1", Name: "设施类型"}}, access.Meta{})
	must(t, e)
	typ, e = f.e.Publish(ctx, f.actor(), eng.Scope{Kind: eng.TypeDefinition}, typ.DefinitionID, eng.RevisionInput{WriteMeta: wm(), Revision: typ.Revision}, access.Meta{})
	must(t, e)
	create := func(target r.Target, fields map[string]any) r.Result {
		fields["request_id"] = f.meta(t).RequestID
		raw, _ := json.Marshal(fields)
		v, e := b.Resources.Mutate(ctx, f.owner, r.TenantScope(f.tenant), target, "create", raw, access.Meta{})
		must(t, e)
		return v
	}
	out := []src.Target{}
	for i := range 2 {
		facility := create(r.Target{Kind: r.Facility}, map[string]any{"code": "facility" + strconv.Itoa(i), "name": "测试设施", "type_version_id": typ.VersionID})
		point := create(r.Target{Kind: r.Point, Facility: facility.ID}, map[string]any{"code": "point", "name": "测点"})
		channel := create(r.Target{Kind: r.Channel, Facility: facility.ID, Point: point.ID}, map[string]any{"code": "channel", "name": "位移通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
		out = append(out, src.Target{FacilityID: facility.ID, PointID: point.ID, ChannelID: channel.ID})
	}
	return out
}
func TestSourceAtomicSwitchOccupancyAndRollback(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	scope := d.TenantScope(f.tenant)
	targets := f.sourceTargets(t, b)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "factory", Name: "厂家"}, access.Meta{})
	must(t, e)
	p := f.product(t, c.PlatformScope(), f.actor(), m.ID, "sensor", []c.Slot{f.fixed()})
	outputs := []d.Output{}
	devices := []d.Result{}
	for i := range 2 {
		device, e := b.Devices.Create(ctx, f.owner, scope, d.DeviceCreate{Meta: f.meta(t), Code: "sensor" + strconv.Itoa(i), Name: "设备", ProductVersionID: p.VersionID}, access.Meta{})
		must(t, e)
		slots, e := b.Devices.Slots(ctx, f.owner, scope, device.ID, d.Filter{})
		must(t, e)
		outputs = append(outputs, *slots.Items[0].Current)
		devices = append(devices, device)
	}
	candidates, e := b.Sources.Candidates(ctx, f.owner, scope, targets[0], src.Filter{})
	must(t, e)
	if len(candidates.Items) != 2 || !candidates.Items[0].Available {
		t.Fatal(candidates)
	}

	reverse, e := b.Sources.ChannelCandidates(ctx, f.owner, scope, outputs[0].ID, src.Filter{Limit: 1})
	must(t, e)
	if len(reverse.Items) != 1 || !reverse.Items[0].Available || reverse.NextCursor == "" {
		t.Fatal(reverse)
	}
	next, e := b.Sources.ChannelCandidates(ctx, f.owner, scope, outputs[0].ID, src.Filter{Limit: 1, Cursor: reverse.NextCursor})
	must(t, e)
	if len(next.Items) != 1 || next.Items[0].ChannelID == reverse.Items[0].ChannelID || next.NextCursor != "" {
		t.Fatal(next)
	}
	// Same output races for independent channels. Exactly one can commit.
	commands := []src.Command{}
	for _, target := range targets {
		in := src.Bind(src.BindInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: "0", ChannelRevision: "1"}, NewEnd: src.NewEnd{OutputID: outputs[0].ID, NewOutputRevision: "1", NewDeviceRevision: "1"}})
		preview, e := b.Sources.Preview(ctx, f.owner, scope, target, in)
		must(t, e)
		in.PreviewHash = preview.Hash
		commands = append(commands, in)
	}
	var result [2]src.Result
	var errs [2]error
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result[i], errs[i] = b.Sources.Mutate(ctx, f.owner, scope, targets[i], commands[i], access.Meta{})
		}(i)
	}
	wg.Wait()
	winner := 0
	if errs[0] != nil {
		winner = 1
	}
	must(t, errs[winner])
	wantCode(t, errs[1-winner], "source_output_occupied")
	current, e := b.Sources.Current(ctx, f.owner, scope, targets[winner])
	must(t, e)
	if !current.Source.Configured || current.Source.Output == nil {
		t.Fatal(current)
	}
	in := src.Switch(src.SwitchInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: current.SourceRevision, ChannelRevision: current.ChannelRevision}, OldEnd: src.OldEnd{BindingID: current.Source.ID, BindingRevision: current.Source.Revision, OldOutputRevision: "1", OldDeviceRevision: "1"}, NewEnd: src.NewEnd{OutputID: outputs[1].ID, NewOutputRevision: "1", NewDeviceRevision: "1"}})
	preview, e := b.Sources.Preview(ctx, f.owner, scope, targets[winner], in)
	must(t, e)
	in.PreviewHash = preview.Hash
	// A history failure after closing the old source must roll the entire command back.
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION reject_source_history() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test history failure'; END $$; CREATE TRIGGER reject_source_history BEFORE INSERT ON device_resource_changes FOR EACH ROW EXECUTE FUNCTION reject_source_history()`)
	must(t, e)
	_, e = b.Sources.Mutate(ctx, f.owner, scope, targets[winner], in, access.Meta{})
	wantCode(t, e, "unavailable")
	unchanged, e := b.Sources.Current(ctx, f.owner, scope, targets[winner])
	must(t, e)
	if unchanged.Source.ID != current.Source.ID || unchanged.SourceRevision != current.SourceRevision {
		t.Fatal("failed switch lost original source")
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER reject_source_history ON device_resource_changes; DROP FUNCTION reject_source_history()`)
	must(t, e)
	switched, e := b.Sources.Mutate(ctx, f.owner, scope, targets[winner], in, access.Meta{})
	must(t, e)
	replayed, e := b.Sources.Mutate(ctx, f.owner, scope, targets[winner], in, access.Meta{})
	must(t, e)
	if !replayed.Replayed || replayed.BindingID != switched.BindingID {
		t.Fatal("switch replay duplicated source")
	}
	history, e := b.Sources.History(ctx, f.owner, scope, targets[winner], src.Filter{})
	must(t, e)
	if len(history.Items) != 2 || history.Items[1].EndedAt == nil || !history.Items[1].EndedAt.Equal(*history.Items[0].StartedAt) {
		t.Fatal("switch has time gap or overlap")
	}
	// Stopping a source device retains occupancy and permits explicitly authorized cleanup.
	state := d.DeviceMutation{Action: "state", DeviceUpdate: d.DeviceUpdate{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: devices[1].Revision}}, Status: "disabled"}
	preview, e = b.Devices.PreviewDevice(ctx, f.owner, scope, devices[1].ID, state)
	must(t, e)
	state.PreviewHash = preview.Hash
	disabled, e := b.Devices.Mutate(ctx, f.owner, scope, devices[1].ID, state, access.Meta{})
	must(t, e)
	current, e = b.Sources.Current(ctx, f.owner, scope, targets[winner])
	must(t, e)
	if !current.Source.Configured || current.Source.Output.DeviceStatus != "disabled" || !current.Actions.Unbind {
		t.Fatal("disabled source was erased or cannot be cleaned")
	}
	unbind := src.Unbind(src.UnbindInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: current.SourceRevision, ChannelRevision: current.ChannelRevision}, OldEnd: src.OldEnd{BindingID: current.Source.ID, BindingRevision: current.Source.Revision, OldOutputRevision: "1", OldDeviceRevision: disabled.Revision}})
	preview, e = b.Sources.Preview(ctx, f.owner, scope, targets[winner], unbind)
	must(t, e)
	unbind.PreviewHash = preview.Hash
	_, e = b.Sources.Mutate(ctx, f.owner, scope, targets[winner], unbind, access.Meta{})
	must(t, e)
	current, e = b.Sources.Current(ctx, f.owner, scope, targets[winner])
	must(t, e)
	if current.Source.Configured {
		t.Fatal("unbind did not close relation")
	}
}
