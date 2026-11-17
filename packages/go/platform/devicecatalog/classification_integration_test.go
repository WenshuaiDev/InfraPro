package devicecatalog_test

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	cl "github.com/WenshuaiDev/InfraPro/packages/go/platform/classification"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestDeviceClassificationReleasesInheritanceAndFixedBatch(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	s := b.Classification
	actor := f.actor()
	platform := cl.PlatformScope()
	tenant := cl.TenantScope(f.tenant)
	root, branch, leaf, other := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	content := cl.Content{DictionaryCode: cl.DictionaryCode, SensorRootID: root, Nodes: []cl.Node{{ID: root, Code: "sensor", Name: "传感器", Status: "active", Selectable: true}, {ID: branch, Code: "displacement", Name: "位移监测", ParentID: root, Status: "active", Selectable: true}, {ID: leaf, Code: "laser", Name: "激光位移", ParentID: branch, Status: "active", Selectable: true}, {ID: other, Code: "gateway", Name: "通信设备", Status: "active", Selectable: true}}}
	publish := func(content cl.Content) string {
		t.Helper()
		dict, e := s.Dictionary(ctx, actor)
		must(t, e)
		saved, e := s.SaveDraft(ctx, actor, cl.SaveInput{Meta: f.meta(t), Revision: dict.DraftRevision, Content: content}, access.Meta{})
		must(t, e)
		in := cl.PublishInput{Meta: f.meta(t), Revision: saved.Revision}
		p, e := s.PreviewPublish(ctx, actor, in)
		must(t, e)
		in.PreviewHash = p.Hash
		r, e := s.Publish(ctx, actor, in, access.Meta{})
		must(t, e)
		return r.ReleaseID
	}
	release := publish(content)
	categories, e := s.Categories(ctx, actor, platform, release, "", "", 2)
	must(t, e)
	if len(categories.Items) != 2 || categories.NextCursor == "" {
		t.Fatal(categories)
	}
	second, e := s.Categories(ctx, actor, platform, release, categories.NextCursor, "", 2)
	must(t, e)
	if len(second.Items) != 2 || second.NextCursor != "" {
		t.Fatal(second)
	}
	m, e := f.s.CreateManufacturer(ctx, actor, c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "classfactory", Name: "厂家"}, access.Meta{})
	must(t, e)
	product := f.product(t, c.PlatformScope(), actor, m.ID, "multifunction", nil)
	first, e := b.Devices.Create(ctx, f.owner, d.TenantScope(f.tenant), d.DeviceCreate{Meta: f.meta(t), Code: "ONE", Name: "继承设备", ProductVersionID: product.VersionID}, access.Meta{})
	must(t, e)
	two, e := b.Devices.Create(ctx, f.owner, d.TenantScope(f.tenant), d.DeviceCreate{Meta: f.meta(t), Code: "TWO", Name: "覆盖设备", ProductVersionID: product.VersionID}, access.Meta{})
	must(t, e)
	var contentHash string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT content_hash FROM device_product_versions WHERE id=$1`, product.VersionID).Scan(&contentHash))
	makeBatch := func(a access.Actor, scope cl.Scope, kind, id string, selection cl.Selection) cl.BatchInput {
		t.Helper()
		v, e := s.Target(ctx, a, scope, kind, id)
		must(t, e)
		return cl.BatchInput{Meta: f.meta(t), Targets: []cl.Target{v.Target}, Selection: selection}
	}
	apply := func(a access.Actor, scope cl.Scope, in cl.BatchInput) cl.Result {
		t.Helper()
		p, e := s.PreviewBatch(ctx, a, scope, in)
		must(t, e)
		in.PreviewID = p.ID
		in.PreviewHash = p.Hash
		r, e := s.ApplyBatch(ctx, a, scope, in, access.Meta{})
		must(t, e)
		replay, e := s.ApplyBatch(ctx, a, scope, in, access.Meta{})
		must(t, e)
		if !replay.Replayed || replay.Applied != 1 {
			t.Fatal(replay)
		}
		return r
	}
	apply(actor, platform, makeBatch(actor, platform, "product_version", product.VersionID, cl.Selection{Mode: "classify", ReleaseID: release, CategoryID: branch}))
	apply(f.owner, tenant, makeBatch(f.owner, tenant, "device", two.ID, cl.Selection{Mode: "classify", ReleaseID: release, CategoryID: other}))
	one, e := b.Devices.Detail(ctx, f.owner, d.TenantScope(f.tenant), first.ID)
	must(t, e)
	if one.Classification == nil || one.Classification.CategoryID != branch || one.Classification.Sensor == nil || !*one.Classification.Sensor || one.OutputRevision != "0" {
		t.Fatal(one.Classification)
	}

	assertGroup := func(filter d.Filter, count int) {
		t.Helper()
		page, e := b.Devices.List(ctx, f.owner, d.TenantScope(f.tenant), filter)
		must(t, e)
		if len(page.Items) != count {
			t.Fatalf("classification filter %#v = %d, want %d", filter, len(page.Items), count)
		}
	}
	assertGroup(d.Filter{CategoryGroup: "sensor"}, 1)
	assertGroup(d.Filter{CategoryGroup: "non_sensor"}, 1)
	assertGroup(d.Filter{CategoryGroup: "unclassified"}, 0)
	assertGroup(d.Filter{CategoryID: root, CategoryScope: "exact"}, 0)
	assertGroup(d.Filter{CategoryID: root, CategoryScope: "descendants"}, 1)
	assertGroup(d.Filter{CategoryID: branch, CategoryScope: "exact"}, 1)
	priorPage, e := b.Devices.List(ctx, f.owner, d.TenantScope(f.tenant), d.Filter{Limit: 1})
	must(t, e)
	if priorPage.NextCursor == "" {
		t.Fatal("missing classification cursor")
	}
	// Adding a child does not move devices directly classified at its parent.
	deep := uuid.NewString()
	content.Nodes = append(content.Nodes, cl.Node{ID: deep, Code: "deep", Name: "第四层", ParentID: leaf, Status: "active", Selectable: true})
	content.Nodes[1].Name = "位移类（修订）"
	release2 := publish(content)
	_, e = b.Devices.List(ctx, f.owner, d.TenantScope(f.tenant), d.Filter{Limit: 1, Cursor: priorPage.NextCursor})
	if e == nil {
		t.Fatal("cursor crossed classification publication")
	}

	one, e = b.Devices.Detail(ctx, f.owner, d.TenantScope(f.tenant), first.ID)
	must(t, e)
	if one.Classification.CategoryID != branch || one.Classification.DictionaryReleaseID != release2 || one.Classification.Name != "位移类（修订）" || one.Classification.SelectedReleaseID != release {
		t.Fatal(one.Classification)
	}
	must(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		old, e := cl.ReadDeviceTx(ctx, tx, f.tenant, first.ID, release)
		if e == nil && (old.Name != "位移监测" || old.DictionaryReleaseID != release) {
			t.Fatal(old)
		}
		return e
	}))
	// A batch fixes both target set and revisions, and product changes never overwrite a device override.
	stale := makeBatch(f.owner, tenant, "device", first.ID, cl.Selection{Mode: "classify", ReleaseID: release2, CategoryID: leaf})
	preview, e := s.PreviewBatch(ctx, f.owner, tenant, stale)
	must(t, e)
	stale.PreviewID = preview.ID
	stale.PreviewHash = preview.Hash
	apply(actor, platform, makeBatch(actor, platform, "product_version", product.VersionID, cl.Selection{Mode: "classify", ReleaseID: release2, CategoryID: deep}))
	_, e = s.ApplyBatch(ctx, f.owner, tenant, stale, access.Meta{})
	if e == nil {
		t.Fatal("batch accepted changed inheritance basis")
	}
	overridden, e := b.Devices.Detail(ctx, f.owner, d.TenantScope(f.tenant), two.ID)
	must(t, e)
	if overridden.Classification.CategoryID != other || overridden.Classification.Origin != "device_override" {
		t.Fatal(overridden.Classification)
	}
	apply(f.owner, tenant, makeBatch(f.owner, tenant, "device", two.ID, cl.Selection{Mode: "inherit"}))
	inherited, e := b.Devices.Detail(ctx, f.owner, d.TenantScope(f.tenant), two.ID)
	must(t, e)
	if inherited.Classification.CategoryID != deep || inherited.Classification.Origin != "product_default" {
		t.Fatal(inherited.Classification)
	}
	hist, e := s.History(ctx, f.owner, tenant, "device", two.ID, "", 1)
	must(t, e)
	if len(hist.Items) != 1 || hist.NextCursor == "" {
		t.Fatal(hist)
	}
	// Deactivation preserves interpretation and counts, while rejecting new selection even through an old release.
	content.Nodes[2].Status = "inactive"
	release3 := publish(content)
	inherited, e = b.Devices.Detail(ctx, f.owner, d.TenantScope(f.tenant), two.ID)
	must(t, e)
	if inherited.Classification.CategoryID != deep || inherited.Classification.State != "classified" || inherited.Classification.DictionaryReleaseID != release3 {
		t.Fatal(inherited.Classification)
	}
	_, e = s.PreviewBatch(ctx, f.owner, tenant, makeBatch(f.owner, tenant, "device", two.ID, cl.Selection{Mode: "classify", ReleaseID: release2, CategoryID: deep}))
	if e == nil {
		t.Fatal("deactivated selection used old version")
	}
	_, e = s.PreviewBatch(ctx, f.owner, tenant, makeBatch(f.owner, tenant, "device", two.ID, cl.Selection{Mode: "classify", ReleaseID: release3, CategoryID: deep}))
	if e == nil {
		t.Fatal("inactive ancestor allowed new classification")
	}
	var hash string
	var versions int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT content_hash,(SELECT count(*) FROM device_product_versions WHERE product_id=$2) FROM device_product_versions WHERE id=$1`, product.VersionID, product.ID).Scan(&hash, &versions))
	if hash != contentHash || versions != 1 {
		t.Fatal("classification changed product output content")
	}
	// Tenant cannot classify a platform-owned product, even though it can read its metadata.
	target, e := s.Target(ctx, f.owner, tenant, "product_version", product.VersionID)
	must(t, e)
	_, e = s.PreviewBatch(ctx, f.owner, tenant, cl.BatchInput{Meta: f.meta(t), Targets: []cl.Target{target.Target}, Selection: cl.Selection{Mode: "unclassified"}})
	if e == nil {
		t.Fatal("tenant rewrote public product classification")
	}
}
