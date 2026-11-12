package devicecatalog_test

import (
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"testing"
)

func TestDeviceTreePaginationAndCatalogState(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	scope := d.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "tree", Name: "分页厂家"}, access.Meta{})
	must(t, e)
	p := f.product(t, c.PlatformScope(), f.actor(), m.ID, "zero", nil)
	for i := range 31 {
		_, e = b.Devices.Create(ctx, f.owner, scope, d.DeviceCreate{Meta: f.meta(t), Code: fmt.Sprintf("PAGE-%02d", i), Name: fmt.Sprintf("设备 %02d", i), ProductVersionID: p.VersionID}, access.Meta{})
		must(t, e)
	}
	cursor := ""
	seen := map[string]bool{}
	for {
		page, e := b.Devices.Tree(ctx, f.owner, scope, d.Filter{Level: "device", ParentID: p.ID, Limit: 7, Cursor: cursor})
		must(t, e)
		if len(page.Items) > 7 {
			t.Fatal("page overflow")
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate tree row")
			}
			seen[item.ID] = true
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 31 {
		t.Fatal("tree silently truncated", len(seen))
	}
	first, e := b.Devices.List(ctx, f.owner, scope, d.Filter{Limit: 7, Source: "public"})
	must(t, e)
	if len(first.Items) != 7 || first.NextCursor == "" {
		t.Fatal(first)
	}
	_, e = b.Devices.List(ctx, f.owner, scope, d.Filter{Limit: 7, Source: "private", Cursor: first.NextCursor})
	wantCode(t, e, "invalid_cursor")
	private, e := b.Devices.List(ctx, f.owner, scope, d.Filter{Source: "private"})
	must(t, e)
	if len(private.Items) != 0 {
		t.Fatal("source filter ignored")
	}
	state := c.ManufacturerMutation{Action: "state", ManufacturerUpdate: c.ManufacturerUpdate{RevisionInput: c.RevisionInput{Meta: f.meta(t), Revision: m.Revision}}, Status: "archived"}
	preview, e := f.s.PreviewManufacturer(ctx, f.actor(), c.PlatformScope(), m.ID, state)
	must(t, e)
	state.PreviewHash = preview.Hash
	_, e = f.s.MutateManufacturer(ctx, f.actor(), c.PlatformScope(), m.ID, state, access.Meta{})
	must(t, e)
	candidates, e := b.Devices.ProductCandidates(ctx, f.owner, scope, d.Filter{ProductID: p.ID})
	must(t, e)
	if len(candidates.Items) != 1 || candidates.Items[0].ID != p.VersionID {
		t.Fatal("manufacturer archival must preserve adoption of existing versions", candidates)
	}
	roots, e := b.Devices.Tree(ctx, f.owner, scope, d.Filter{})
	must(t, e)
	if len(roots.Items) != 1 || roots.Items[0].Visibility != "full" {
		t.Fatal("catalog permission must still expose the archived manufacturer", roots)
	}
	products, e := b.Devices.Tree(ctx, f.owner, scope, d.Filter{Level: "product", ParentID: m.ID})
	must(t, e)
	if len(products.Items) != 1 || products.Items[0].ID != p.ID || products.Items[0].Visibility != "full" {
		t.Fatal("archival hid the existing catalog product", products)
	}
	old, e := b.Devices.Detail(ctx, f.owner, scope, first.Items[0].ID)
	must(t, e)
	if old.ProductVersionID != p.VersionID {
		t.Fatal("archive rewrote adopted product")
	}
}
