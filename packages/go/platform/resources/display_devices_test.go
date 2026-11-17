package resources_test

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	src "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/google/uuid"
)

func TestDisplayReadDevicesSourcesAndDistinctStatistics(t *testing.T) {
	f := resourcesSetup(t)
	p := f.project(t, "ReadDevices")
	b := runtime.Compose(displayReadPlanAccess(t, f))
	catalog := c.New(f.env.Service.Access())
	meta := func() c.Meta { return c.Meta{RequestID: uuid.NewString(), Reason: "合成 SDK 设备来源验收"} }
	admin := f.fixture.actor(f.p)
	manufacturer, e := catalog.CreateManufacturer(ctx, admin, c.PlatformScope(), c.ManufacturerCreate{Meta: meta(), Code: "read-maker", Name: "合成厂家"}, access.Meta{})
	must(t, e)
	product, e := catalog.CreateProduct(ctx, admin, c.PlatformScope(), c.ProductCreate{Meta: meta(), ManufacturerID: manufacturer.ID, Code: "read-product", Name: "合成位移传感器", Model: "Fixed-2026", OutputSlots: []c.Slot{{Key: "a", Name: "A", Mode: "fixed", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}, {Key: "b", Name: "B", Mode: "fixed", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}}}, access.Meta{})
	must(t, e)
	product, e = catalog.Publish(ctx, admin, c.PlatformScope(), product.ID, c.DraftRevision{Meta: meta(), Revision: "1"}, access.Meta{})
	must(t, e)
	devices := []d.Result{}
	outputs := [][]d.Output{}
	for i := range 4 {
		device, e := b.Devices.Create(ctx, f.actor, d.TenantScope(f.tenant), d.DeviceCreate{Meta: meta(), Code: "SDK-V" + strconv.Itoa(i+1), Name: "设备 " + strconv.Itoa(i+1), SerialNumber: "PRIVATE-SERIAL-" + strconv.Itoa(i), Description: "PRIVATE-DESCRIPTION", ProductVersionID: product.VersionID}, access.Meta{})
		must(t, e)
		devices = append(devices, device)
		slots, e := b.Devices.Slots(ctx, f.actor, d.TenantScope(f.tenant), device.ID, d.Filter{})
		must(t, e)
		outputs = append(outputs, []d.Output{*slots.Items[0].Current, *slots.Items[1].Current})
	}
	facility := f.facility(t, "SDK-F")
	point, e := f.write(t, target(facility, r.Point, ""), "create", map[string]any{"code": "SDK-P", "name": "合成测点"})
	must(t, e)
	channels := []string{}
	grant := f.accessInput(t, p, facility.ID, "create_link")
	grant.Add = []r.GrantInput{{Kind: r.Facility, Capabilities: []string{"read"}}, {Kind: r.Point, PointID: &point.ID, Capabilities: []string{"read"}}}
	for i := range 4 {
		ct := target(facility, r.Channel, "")
		ct.Point = point.ID
		channel, e := f.write(t, ct, "create", map[string]any{"code": "SDK-D" + strconv.Itoa(i), "name": "合成位移通道", "metric_version_id": f.metric, "unit_version_id": f.unit, "timestamp_meaning": "observation_time"})
		must(t, e)
		channels = append(channels, channel.ID)
		id := channel.ID
		grant.Add = append(grant.Add, r.GrantInput{Kind: r.Channel, ChannelID: &id, Capabilities: []string{"read"}})
	}
	f.grant(t, p, facility.ID, grant)
	for _, id := range channels {
		var resourceGrant, revision string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, p.id, id).Scan(&resourceGrant))
		must(t, f.env.Pool.QueryRow(ctx, `SELECT COALESCE((SELECT revision::text FROM monitoring_access_states WHERE tenant_id=$1 AND project_id=$2),'0')`, f.tenant, p.id).Scan(&revision))
		_, e = b.Monitoring.GrantData(ctx, f.actor, f.tenant, p.id, md.GrantInput{WriteMeta: md.WriteMeta{RequestID: uuid.NewString(), ExpectedRevision: revision, Reason: "SDK 来源读取"}, ChannelID: id, ResourceGrantID: resourceGrant, Range: &md.Range{}, Capabilities: []string{"read", "history"}})
		must(t, e)
	}
	for i, output := range []d.Output{outputs[0][0], outputs[0][1], outputs[2][0], outputs[3][0]} {
		target := src.Target{FacilityID: facility.ID, PointID: point.ID, ChannelID: channels[i]}
		cmd := src.Bind(src.BindInput{Revisions: src.Revisions{Meta: meta(), SourceRevision: "0", ChannelRevision: "1"}, NewEnd: src.NewEnd{OutputID: output.ID, NewOutputRevision: output.Revision, NewDeviceRevision: output.DeviceRevision}})
		preview, e := b.Sources.Preview(ctx, f.actor, d.TenantScope(f.tenant), target, cmd)
		must(t, e)
		cmd.PreviewHash = preview.Hash
		_, e = b.Sources.Mutate(ctx, f.actor, d.TenantScope(f.tenant), target, cmd, access.Meta{})
		must(t, e)
	}
	gs, e := b.Devices.Grants(ctx, f.actor, f.tenant, p.id, d.Filter{})
	must(t, e)
	dg := d.GrantChangeSet{Meta: meta(), AccessRevision: gs.AccessRevision, ProjectRevision: gs.ProjectRevision, Add: []d.GrantAdd{{Kind: "device", ID: devices[0].ID}, {Kind: "device", ID: devices[1].ID}, {Kind: "output", ID: outputs[0][0].ID}, {Kind: "output", ID: outputs[0][1].ID}, {Kind: "output", ID: outputs[2][0].ID}}}
	preview, e := b.Devices.PreviewGrants(ctx, f.actor, f.tenant, p.id, dg)
	must(t, e)
	dg.PreviewHash = preview.Hash
	_, e = b.Devices.ChangeGrants(ctx, f.actor, f.tenant, p.id, dg, access.Meta{})
	must(t, e)
	scopes := []dr.Definition{{ID: "devices", Domain: "devices", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"device.metadata", "device.outputs", "device.installations", "history", "statistics.counts"}}, {ID: "channels", Domain: "channels", Selector: dr.Selector{Kind: "ids", IDs: channels}, FieldGroups: []string{"resource.metadata", "source.binding", "history", "statistics.counts"}}, {ID: "resources", Domain: "resources", Selector: dr.Selector{Kind: "project"}, FieldGroups: []string{"resource.metadata", "statistics.counts"}}}
	views := map[string]dr.Definition{}
	for _, scope := range scopes {
		v := scope
		v.ScopeID = v.ID
		v.ID = ""
		views[scope.ID] = v
	}
	h := createReadDisplay(t, f, p, b, scopes, views, []string{"views.read", "queries.read", "devices.read", "devices.history", "sources.directory", "sources.history", "statistics.devices"})
	dt, ct, rt := dr.Target{View: "devices"}, dr.Target{View: "channels"}, dr.Target{View: "resources"}
	list := readData[dr.Page[dr.DeviceDetail]](t, readCall(t, h, "devices.list", dr.DevicesRequest{Target: dt}))
	if list.Total != "2" {
		t.Fatalf("full devices: %+v", list)
	}
	for _, item := range list.Items {
		if item.SerialNumber != nil || item.Description != nil || item.AdoptedModel != "Fixed-2026" {
			t.Fatalf("field/fixed product projection: %+v", item)
		}
	}
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "devices.get", Params: dr.Bytes(dr.DeviceGetRequest{Target: dt, ID: devices[2].ID})})
	if e == nil {
		t.Fatal("output-only device gained full record")
	}
	out := readData[dr.Page[dr.DeviceOutput]](t, readCall(t, h, "devices.outputs", dr.DeviceOutputsRequest{Target: dt, ID: devices[2].ID}))
	if out.Total != "1" || out.Items[0].Device.Visibility != "context" {
		t.Fatalf("independent output projection: %+v", out)
	}
	sources := readData[dr.Page[dr.SourceBindingItem]](t, readCall(t, h, "sources.bindings", dr.SourceBindingsRequest{Target: ct, DeviceTarget: &dt, PointIDs: []string{point.ID}}))
	if sources.Total != "4" {
		t.Fatalf("point channel pagination: %+v", sources)
	}
	states := map[string]dr.SourceBindingItem{}
	for _, v := range sources.Items {
		states[v.Channel.ID] = v
	}
	if states[channels[2]].Source.Device.Visibility != "context" || states[channels[3]].State != "restricted" {
		t.Fatalf("source states: %+v", sources)
	}
	hidden := dr.Bytes(states[channels[3]])
	for _, value := range []string{devices[3].ID, outputs[3][0].ID, "PRIVATE", "SDK-V4"} {
		if bytes.Contains(hidden, []byte(value)) {
			t.Fatal("restricted source identity leaked")
		}
	}
	for _, population := range []string{"authorized", "monitoring_bound"} {
		result := readCall(t, h, "statistics.devices", dr.DeviceStatisticsRequest{Target: dt, ChannelTarget: &ct, ResourceTarget: &rt, Population: population, GroupBy: "manufacturer"})
		var data struct {
			Totals dr.DeviceCounts                       `json:"totals"`
			Groups []dr.StatisticsGroup[dr.DeviceCounts] `json:"groups"`
		}
		data = readData[struct {
			Totals dr.DeviceCounts                       `json:"totals"`
			Groups []dr.StatisticsGroup[dr.DeviceCounts] `json:"groups"`
		}](t, result)
		want := "2"
		if population == "monitoring_bound" {
			want = "1"
		}
		if data.Totals.UniqueDeviceCount != want || len(data.Groups) != 1 || data.Groups[0].Totals.UniqueDeviceCount != want {
			t.Fatalf("%s: %+v", population, data)
		}
	}
	for _, kind := range []string{"device_change", "output_generation", "installation"} {
		history := readCall(t, h, "devices.history", dr.DeviceHistoryRequest{Target: dt, ID: devices[0].ID, Kind: kind, ResourceTarget: &rt})
		if bytes.Contains(dr.Bytes(history.Data), []byte("PRIVATE")) {
			t.Fatal("device history secret fields leaked")
		}
	}
	history := readData[dr.Page[dr.SourceBindingItem]](t, readCall(t, h, "sources.bindingHistory", dr.SourceHistoryRequest{Target: ct, ID: channels[0], DeviceTarget: &dt}))
	if history.Total != "1" || history.Items[0].Source.OutputSemantics == nil {
		t.Fatalf("typed historical semantics: %+v", history)
	}
	// Field widening is rejected, including a field omitted from both view/grant.
	_, e = h.s.Query(ctx, h.session, display.Call{Method: "devices.get", Params: dr.Bytes(dr.DeviceGetRequest{Target: dt, ID: devices[0].ID, ReadFields: dr.ReadFields{FieldGroups: []string{"device.metadata", "device.serial_number"}}})})
	if e == nil {
		t.Fatal("serial field widened")
	}
	query := dr.DeviceStatisticsRequest{Target: dt, Population: "authorized", GroupBy: "manufacturer", Paging: dr.Paging{Limit: 1}}
	sub, e := h.s.Subscribe(ctx, h.session, display.SubscriptionInput{Kind: "statistics.devices", Query: dr.Bytes(query), Generation: "device-sub"})
	must(t, e)
	events, e := h.s.Poll(ctx, h.session)
	must(t, e)
	if len(events) != 1 || events[0].Result == nil || readData[dr.DeviceStatistics](t, *events[0].Result).Totals.UniqueDeviceCount != "2" {
		t.Fatal("device subscription missing exact snapshot", events)
	}
	replay, e := h.s.Poll(ctx, h.session)
	must(t, e)
	if len(replay) != 1 || replay[0].Sequence != events[0].Sequence || replay[0].Result.Query.ID != events[0].Result.Query.ID {
		t.Fatal("device replay changed snapshot")
	}
	// Simulate an aged unacknowledged event without sleeping for retention.
	_, e = f.env.Pool.Exec(ctx, `UPDATE display_subscriptions SET updated_at=clock_timestamp()-interval '6 minutes' WHERE id=$1`, sub)
	must(t, e)
	events, e = h.s.Poll(ctx, h.session)
	must(t, e)
	if len(events) != 1 || events[0].Kind != "reset" || events[0].Result != nil {
		t.Fatal("expired device event was replayed", events)
	}
	events, e = h.s.Poll(ctx, h.session)
	must(t, e)
	if len(events) != 1 || events[0].Result == nil || readData[dr.DeviceStatistics](t, *events[0].Result).Totals.UniqueDeviceCount != "2" {
		t.Fatal("device reset did not create current snapshot", events)
	}
	must(t, h.s.CancelSubscription(ctx, h.session, sub))
}
