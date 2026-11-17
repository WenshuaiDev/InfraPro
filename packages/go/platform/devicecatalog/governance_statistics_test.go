package devicecatalog_test

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"testing"
)

func TestGovernanceDeviceCountExcludesOutputAndEndedGrants(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	var member string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, f.tenant, f.owner.ID).Scan(&member))
	ps := projects.New(f.env.Service.Access())
	project, err := ps.Create(ctx, f.owner, f.tenant, projects.CreateInput{Fields: projects.Fields{Code: "counts", Name: "统计项目", Purpose: "授权统计"}, WriteMeta: projects.WriteMeta{RequestID: f.meta(t).RequestID, Reason: "统计验证"}, Owner: member, Administrator: member}, access.Meta{})
	must(t, err)
	manufacturer, err := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "factory", Name: "厂家"}, access.Meta{})
	must(t, err)
	product := f.product(t, c.PlatformScope(), f.actor(), manufacturer.ID, "sensor", []c.Slot{f.fixed()})
	device, err := b.Devices.Create(ctx, f.owner, d.TenantScope(f.tenant), d.DeviceCreate{Meta: f.meta(t), Code: "sensor", Name: "设备", ProductVersionID: product.VersionID}, access.Meta{})
	must(t, err)
	slots, err := b.Devices.Slots(ctx, f.owner, d.TenantScope(f.tenant), device.ID, d.Filter{})
	must(t, err)
	output := slots.Items[0].Current.ID
	change := func(add []d.GrantAdd, end []d.GrantEnd) {
		t.Helper()
		current, err := b.Devices.Grants(ctx, f.owner, f.tenant, project.ProjectID, d.Filter{})
		must(t, err)
		in := d.GrantChangeSet{Meta: f.meta(t), AccessRevision: current.AccessRevision, ProjectRevision: current.ProjectRevision, Add: add, End: end}
		preview, err := b.Devices.PreviewGrants(ctx, f.owner, f.tenant, project.ProjectID, in)
		must(t, err)
		in.PreviewHash = preview.Hash
		_, err = b.Devices.ChangeGrants(ctx, f.owner, f.tenant, project.ProjectID, in, access.Meta{})
		must(t, err)
	}
	check := func(want int) {
		t.Helper()
		g, err := ps.Governance(ctx, f.owner, f.tenant, project.ProjectID)
		must(t, err)
		if g.Statistics == nil || g.Statistics.Devices == nil || *g.Statistics.Devices != want {
			t.Fatalf("unexpected device count: %+v", g.Statistics)
		}
	}
	change([]d.GrantAdd{{Kind: "output", ID: output, CanUseSource: true}}, nil)
	check(0)
	change([]d.GrantAdd{{Kind: "device", ID: device.ID}}, nil)
	check(1)
	current, err := b.Devices.Grants(ctx, f.owner, f.tenant, project.ProjectID, d.Filter{})
	must(t, err)
	for _, grant := range current.Items {
		if grant.Kind == "device" {
			change(nil, []d.GrantEnd{{ID: grant.ID, Revision: grant.Revision}})
		}
	}
	check(0)
}
