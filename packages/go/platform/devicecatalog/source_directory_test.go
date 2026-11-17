package devicecatalog_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	r "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	src "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
)

func TestSourceDirectoryPagingFilteringAndVisibility(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	scope := d.TenantScope(f.tenant)
	targets := f.sourceTargets(t, b)
	page, e := b.Sources.Directory(ctx, f.owner, scope, src.Filter{Limit: 1})
	must(t, e)
	if len(page.Items) != 1 || page.Counts["all"] != 2 || page.Counts["unbound"] != 2 || page.NextCursor == "" {
		t.Fatal(page)
	}
	next, e := b.Sources.Directory(ctx, f.owner, scope, src.Filter{Limit: 1, Cursor: page.NextCursor})
	must(t, e)
	if len(next.Items) != 1 || next.Items[0].ChannelID == page.Items[0].ChannelID || next.NextCursor != "" || next.Counts["all"] != 2 {
		t.Fatal(next)
	}
	_, e = b.Sources.Directory(ctx, f.owner, scope, src.Filter{Limit: 1, Cursor: page.NextCursor, State: "bound"})
	wantCode(t, e, "invalid_cursor")
	_, e = b.Sources.Directory(ctx, f.owner, scope, src.Filter{State: "invalid"})
	wantCode(t, e, "invalid_request")
	_, e = b.Sources.Directory(ctx, f.owner, scope, src.Filter{ChannelID: "invalid"})
	wantCode(t, e, "invalid_request")
	exact, e := b.Sources.Directory(ctx, f.owner, scope, src.Filter{ChannelID: targets[1].ChannelID})
	must(t, e)
	if len(exact.Items) != 1 || exact.Items[0].ChannelID != targets[1].ChannelID || exact.Items[0].UnitSymbol != "mm" || exact.Items[0].MetricName != "位移" {
		t.Fatal(exact)
	}
	filtered, e := b.Sources.Directory(ctx, f.owner, scope, src.Filter{FacilityID: targets[1].FacilityID, ParentID: targets[0].PointID})
	must(t, e)
	if len(filtered.Items) != 0 || filtered.Counts["all"] != 0 {
		t.Fatal(filtered)
	}
	filtered, e = b.Sources.Directory(ctx, f.owner, scope, src.Filter{Query: "facility1"})
	must(t, e)
	if len(filtered.Items) != 1 || filtered.Items[0].FacilityID != targets[1].FacilityID {
		t.Fatal(filtered)
	}
	_, e = b.Sources.Directory(ctx, f.actor(), scope, src.Filter{})
	wantCode(t, e, "tenant_not_found")
	_, e = b.Sources.Directory(ctx, f.owner, d.ProjectScope(f.tenant, targets[0].FacilityID), src.Filter{})
	wantCode(t, e, "source_not_found")

	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "binding", Name: "厂家"}, access.Meta{})
	must(t, e)
	product := f.product(t, c.PlatformScope(), f.actor(), m.ID, "binding", []c.Slot{f.fixed()})
	device, e := b.Devices.Create(ctx, f.owner, scope, d.DeviceCreate{Meta: f.meta(t), Code: "secret-device", Name: "保密设备名称", ProductVersionID: product.VersionID}, access.Meta{})
	must(t, e)
	slots, e := b.Devices.Slots(ctx, f.owner, scope, device.ID, d.Filter{})
	must(t, e)
	output := slots.Items[0].Current
	bind := src.Bind(src.BindInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: "0", ChannelRevision: "1"}, NewEnd: src.NewEnd{OutputID: output.ID, NewOutputRevision: "1", NewDeviceRevision: "1"}})
	preview, e := b.Sources.Preview(ctx, f.owner, scope, targets[0], bind)
	must(t, e)
	bind.PreviewHash = preview.Hash
	_, e = b.Sources.Mutate(ctx, f.owner, scope, targets[0], bind, access.Meta{})
	must(t, e)
	bound, e := b.Sources.Directory(ctx, f.owner, scope, src.Filter{State: "bound"})
	must(t, e)
	if len(bound.Items) != 1 || bound.Counts["all"] != 2 || bound.Counts["unbound"] != 1 || bound.Items[0].Source.Output.ID != output.ID {
		t.Fatal(bound)
	}
	reader := f.deviceReader(t, []access.Code{access.TenantEngineeringResourceRead})
	restricted, e := b.Sources.Directory(ctx, reader, scope, src.Filter{})
	must(t, e)
	raw, e := json.Marshal(restricted)
	must(t, e)
	if restricted.Counts["restricted"] != 1 || strings.Contains(string(raw), device.ID) || strings.Contains(string(raw), output.ID) || strings.Contains(string(raw), "保密设备") {
		t.Fatal(string(raw))
	}
	hiddenSearch, e := b.Sources.Directory(ctx, reader, scope, src.Filter{Query: "保密设备名称"})
	must(t, e)
	if hiddenSearch.Counts["all"] != 0 {
		t.Fatal("device leaked through search")
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE devices SET status='disabled' WHERE id=$1`, device.ID)
	must(t, e)
	attention, e := b.Sources.Directory(ctx, f.owner, scope, src.Filter{State: "attention"})
	must(t, e)
	if len(attention.Items) != 1 {
		t.Fatal(attention)
	}
	restricted, e = b.Sources.Directory(ctx, reader, scope, src.Filter{})
	must(t, e)
	if restricted.Counts["restricted"] != 1 || restricted.Counts["attention"] != 0 {
		t.Fatal("device state leaked", restricted)
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE engineering_facilities SET status='retired' WHERE id=$1`, targets[0].FacilityID)
	must(t, e)
	retired, e := b.Sources.Directory(ctx, f.owner, scope, src.Filter{State: "retired"})
	must(t, e)
	if len(retired.Items) != 1 || retired.Items[0].ChannelID != targets[0].ChannelID {
		t.Fatal(retired)
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE engineering_channels SET status='retired',deleted_at=clock_timestamp() WHERE id=$1`, targets[1].ChannelID)
	must(t, e)
	page, e = b.Sources.Directory(ctx, f.owner, scope, src.Filter{})
	must(t, e)
	if page.Counts["all"] != 1 {
		t.Fatal("deleted channel remained visible", page)
	}
}

func TestSourceDirectoryCompleteComponentPathAndAncestorSearch(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	scope := d.TenantScope(f.tenant)
	targets := f.sourceTargets(t, b)
	create := func(kind r.Kind, fields map[string]any) r.Result {
		fields["request_id"] = f.meta(t).RequestID
		raw, err := json.Marshal(fields)
		must(t, err)
		result, err := b.Resources.Mutate(ctx, f.owner, r.TenantScope(f.tenant), r.Target{Kind: kind, Facility: targets[0].FacilityID}, "create", raw, access.Meta{})
		must(t, err)
		return result
	}
	facility, err := b.Resources.Detail(ctx, f.owner, r.TenantScope(f.tenant), r.Target{Kind: r.Facility, Facility: targets[0].FacilityID, ID: targets[0].FacilityID})
	must(t, err)
	span := create(r.Component, map[string]any{"code": "SPAN-ANCESTOR", "name": "第一主跨", "category_key": "span", "expected_structure_revision": facility.StructureRevision})
	beam := create(r.Component, map[string]any{"code": "BEAM-CHILD", "name": "跨中主梁", "category_key": "beam", "parent_component_id": span.ID, "expected_structure_revision": span.StructureRevision})
	_, e := f.env.Pool.Exec(ctx, `UPDATE engineering_points SET component_id=$1 WHERE id=$2`, beam.ID, targets[0].PointID)
	must(t, e)
	for _, query := range []string{"第一主跨", "SPAN-ANCESTOR", "跨中主梁", "BEAM-CHILD"} {
		page, err := b.Sources.Directory(ctx, f.owner, scope, src.Filter{Query: query, Limit: 1})
		must(t, err)
		if page.Counts["all"] != 1 || len(page.Items) != 1 || page.Items[0].ChannelID != targets[0].ChannelID {
			t.Fatalf("ancestor query %q: %+v", query, page)
		}
		path := page.Items[0].ComponentPath
		if len(path) != 2 || path[0].ID != span.ID || path[1].ID != beam.ID || path[0].Name != "第一主跨" || path[1].Code != "BEAM-CHILD" {
			t.Fatalf("incomplete or reversed path: %+v", path)
		}
	}
	direct, e := b.Sources.Directory(ctx, f.owner, scope, src.Filter{ChannelID: targets[1].ChannelID})
	must(t, e)
	if len(direct.Items) != 1 || direct.Items[0].ComponentPath == nil || len(direct.Items[0].ComponentPath) != 0 {
		t.Fatal("facility-owned point must have an empty array", direct)
	}
	_, e = f.env.Pool.Exec(ctx, `UPDATE engineering_components SET status='retired' WHERE id=$1`, span.ID)
	must(t, e)
	retired, e := b.Sources.Directory(ctx, f.owner, scope, src.Filter{State: "retired", Query: "SPAN-ANCESTOR"})
	must(t, e)
	if len(retired.Items) != 1 || len(retired.Items[0].ComponentPath) != 2 {
		t.Fatal("retired ancestors must remain in the path", retired)
	}
}
