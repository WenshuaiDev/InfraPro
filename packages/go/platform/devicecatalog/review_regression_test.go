package devicecatalog_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	src "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
)

func (f fixture) deviceReader(t *testing.T, codes []access.Code) access.Actor {
	t.Helper()
	reader := f.env.Service.AccessActor(f.member(t, "DeviceReader"))
	role, e := f.env.Service.Access().CreateTenantRole(ctx, f.owner, f.tenant, "", access.TenantRoleCreateInput{Name: "设备权限", Permissions: codes, RequestID: f.meta(t).RequestID, Reason: "回归验证"}, access.Meta{})
	must(t, e)
	var member string
	must(t, f.env.Pool.QueryRow(ctx, `INSERT INTO tenant_memberships(tenant_id,account_id,joined_by,status_changed_by) VALUES($1,$2,$3,$3) RETURNING id::text`, f.tenant, reader.ID, f.owner.ID).Scan(&member))
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) VALUES($1,$2,$3,$4)`, f.tenant, member, role.Role.ID, f.owner.ID)
	must(t, e)
	return reader
}

func TestTenantInstallationHistoryAndMutationRequireTargetRead(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	scope := d.TenantScope(f.tenant)
	target := f.sourceTargets(t, b)[0]
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "history", Name: "厂家"}, access.Meta{})
	must(t, e)
	p := f.product(t, c.PlatformScope(), f.actor(), m.ID, "history", []c.Slot{f.fixed()})
	device, e := b.Devices.Create(ctx, f.owner, scope, d.DeviceCreate{Meta: f.meta(t), Code: "history", Name: "设备", ProductVersionID: p.VersionID}, access.Meta{})
	must(t, e)
	installedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond).Format(time.RFC3339Nano)
	install, e := b.Devices.Install(ctx, f.owner, scope, device.ID, d.InstallationCreateCommand(d.InstallationCreate{Meta: f.meta(t), InstallationRevision: "0", InstalledAt: installedAt, InstallationFields: d.InstallationFields{Target: d.Target{Kind: "point", ID: target.PointID}, LocationDescription: "受限测点位置"}}), access.Meta{})
	must(t, e)
	slots, e := b.Devices.Slots(ctx, f.owner, scope, device.ID, d.Filter{})
	must(t, e)
	bind := src.Bind(src.BindInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: "0", ChannelRevision: "1"}, NewEnd: src.NewEnd{OutputID: slots.Items[0].Current.ID, NewOutputRevision: "1", NewDeviceRevision: "1"}})
	preview, e := b.Sources.Preview(ctx, f.owner, scope, target, bind)
	must(t, e)
	bind.PreviewHash = preview.Hash
	_, e = b.Sources.Mutate(ctx, f.owner, scope, target, bind, access.Meta{})
	must(t, e)
	reader := f.deviceReader(t, []access.Code{access.TenantDeviceRead, access.TenantDeviceHistoryRead, access.TenantDeviceInstall})
	workspaces, e := f.env.Service.Access().MyWorkspaces(ctx, reader, access.WorkspaceFilter{})
	must(t, e)
	if workspaces.Summary.Mode != "single" || workspaces.Summary.Single.DefaultPath != "/t/"+f.tenant+"/devices" {
		t.Fatal("device-only account has no usable landing", workspaces)
	}

	t.Run("history is projected with current target rights", func(t *testing.T) {
		current, e := b.Devices.Installations(ctx, reader, scope, device.ID, d.Filter{State: "current"})
		must(t, e)
		if len(current.Items) != 1 || current.Items[0].Visibility != "restricted" {
			t.Fatal(current)
		}
		history, e := b.Devices.Changes(ctx, reader, scope, device.ID, "", d.Filter{Kind: "installation"})
		must(t, e)
		raw, e := json.Marshal(history)
		must(t, e)
		if len(history.Items) != 1 || strings.Contains(string(raw), target.PointID) || strings.Contains(string(raw), "受限测点位置") || !strings.Contains(string(raw), "restricted") {
			t.Fatalf("restricted installation leaked: %s", raw)
		}
		ownerHistory, e := b.Devices.Changes(ctx, f.owner, scope, device.ID, "", d.Filter{Kind: "installation"})
		must(t, e)
		raw, e = json.Marshal(ownerHistory)
		must(t, e)
		if !strings.Contains(string(raw), target.PointID) || !strings.Contains(string(raw), "受限测点位置") || ownerHistory.Items[0].Reason == "" {
			t.Fatalf("authorized history lost its target or audit fields: %s", raw)
		}
	})
	t.Run("source history is filtered before pagination", func(t *testing.T) {
		page, e := b.Devices.Changes(ctx, reader, scope, device.ID, "", d.Filter{Kind: "source", Limit: 1})
		must(t, e)
		if len(page.Items) != 0 || page.NextCursor != "" {
			t.Fatal("source history leaked without resource history rights", page)
		}
		page, e = b.Devices.Changes(ctx, f.owner, scope, device.ID, "", d.Filter{Kind: "source", Limit: 1})
		must(t, e)
		if len(page.Items) != 1 {
			t.Fatal("authorized source history missing", page)
		}
	})
	for _, action := range []string{"end", "relocate", "correct"} {
		t.Run(action+" checks the existing structured target", func(t *testing.T) {
			in := d.InstallationCommand{Action: action, ID: install.InstallationID, RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: install.Revision}, InstallationRevision: install.InstallationRevision, InstallationFields: d.InstallationFields{Target: d.Target{Kind: "location"}, LocationDescription: "独立位置"}, InstalledAt: installedAt}
			if action == "end" {
				in.UninstalledAt = ptr("")
			}
			_, e := b.Devices.PreviewInstallation(ctx, reader, scope, device.ID, in)
			wantCode(t, e, "permission_denied")
			ownerPreview, e := b.Devices.PreviewInstallation(ctx, f.owner, scope, device.ID, in)
			must(t, e)
			in.PreviewHash = ownerPreview.Hash
			_, e = b.Devices.Install(ctx, reader, scope, device.ID, in, access.Meta{})
			wantCode(t, e, "permission_denied")
		})
	}
	current, e := b.Devices.Installations(ctx, f.owner, scope, device.ID, d.Filter{State: "current"})
	must(t, e)
	if len(current.Items) != 1 || current.Items[0].PointID == nil || *current.Items[0].PointID != target.PointID || current.Items[0].Revision != install.Revision {
		t.Fatal("rejected mutations changed the installation", current)
	}
	// Qualified removal is allowed; independent locations do not require resource rights.
	end := d.InstallationEndCommand(install.InstallationID, d.InstallationEnd{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: install.Revision}, InstallationRevision: install.InstallationRevision})
	endPreview, e := b.Devices.PreviewInstallation(ctx, f.owner, scope, device.ID, end)
	must(t, e)
	end.PreviewHash = endPreview.Hash
	ended, e := b.Devices.Install(ctx, f.owner, scope, device.ID, end, access.Meta{})
	must(t, e)
	independent := d.InstallationCreateCommand(d.InstallationCreate{Meta: f.meta(t), InstallationRevision: ended.InstallationRevision, InstallationFields: d.InstallationFields{Target: d.Target{Kind: "location"}, LocationDescription: "独立位置"}})
	independentPreview, e := b.Devices.PreviewInstallation(ctx, reader, scope, device.ID, independent)
	must(t, e)
	independent.PreviewHash = independentPreview.Hash
	_, e = b.Devices.Install(ctx, reader, scope, device.ID, independent, access.Meta{})
	must(t, e)
}

func TestArchivedManufacturerKeepsExistingProductsAdoptable(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	platform, tenant := c.PlatformScope(), c.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), platform, c.ManufacturerCreate{Meta: f.meta(t), Code: "archived", Name: "厂家"}, access.Meta{})
	must(t, e)
	public := f.product(t, platform, f.actor(), m.ID, "public", nil)
	private := f.product(t, tenant, f.owner, m.ID, "private", nil)
	state := c.ManufacturerMutation{Action: "state", ManufacturerUpdate: c.ManufacturerUpdate{RevisionInput: c.RevisionInput{Meta: f.meta(t), Revision: m.Revision}}, Status: "archived"}
	preview, e := f.s.PreviewManufacturer(ctx, f.actor(), platform, m.ID, state)
	must(t, e)
	state.PreviewHash = preview.Hash
	_, e = f.s.MutateManufacturer(ctx, f.actor(), platform, m.ID, state, access.Meta{})
	must(t, e)
	t.Run("public catalog and versions", func(t *testing.T) {
		products, e := f.s.Products(ctx, f.owner, tenant, c.Filter{ManufacturerID: m.ID}, true)
		must(t, e)
		if len(products.Items) != 1 || products.Items[0].ID != public.ID {
			t.Fatalf("archival hid published products: %+v", products)
		}
		detail, e := f.s.Versions(ctx, f.owner, tenant, public.ID, c.Filter{})
		must(t, e)
		if len(detail.Items) != 1 || !detail.Items[0].Usable {
			t.Fatal(detail)
		}
	})
	t.Run("device candidates", func(t *testing.T) {
		versions, e := b.Devices.ProductCandidates(ctx, f.owner, d.TenantScope(f.tenant), d.Filter{})
		must(t, e)
		if len(versions.Items) != 2 {
			t.Fatalf("archival hid adoption candidates: %+v", versions)
		}
	})
	for name, product := range map[string]c.Result{"public": public, "private": private} {
		t.Run("adopt "+name, func(t *testing.T) {
			_, e := b.Devices.Create(ctx, f.owner, d.TenantScope(f.tenant), d.DeviceCreate{Meta: f.meta(t), Code: name, Name: "设备", ProductVersionID: product.VersionID}, access.Meta{})
			must(t, e)
		})
	}
	_, e = f.s.CreateProduct(ctx, f.owner, tenant, c.ProductCreate{Meta: f.meta(t), ManufacturerID: m.ID, Code: "new", Name: "禁止新建", Model: "X"}, access.Meta{})
	wantCode(t, e, "definition_unavailable")
	_, e = f.s.CopyProduct(ctx, f.owner, tenant, c.ProductCopy{Meta: f.meta(t), SourceVersionID: public.VersionID, ManufacturerID: m.ID, Code: "copy"}, access.Meta{})
	wantCode(t, e, "definition_unavailable")
	// Manufacturer archival must not override a version's own disabled state.
	disable := c.ProductMutation{Action: "state", VersionID: public.VersionID, VersionState: c.VersionState{RevisionInput: c.RevisionInput{Meta: f.meta(t), Revision: "1"}, Enabled: ptr(false)}}
	versionPreview, e := f.s.PreviewProduct(ctx, f.actor(), platform, public.ID, disable)
	must(t, e)
	disable.PreviewHash = versionPreview.Hash
	_, e = f.s.MutateProduct(ctx, f.actor(), platform, public.ID, disable, access.Meta{})
	must(t, e)
	_, e = b.Devices.Create(ctx, f.owner, d.TenantScope(f.tenant), d.DeviceCreate{Meta: f.meta(t), Code: "disabled", Name: "禁用产品", ProductVersionID: public.VersionID}, access.Meta{})
	wantCode(t, e, "definition_unavailable")
	versions, e := b.Devices.ProductCandidates(ctx, f.owner, d.TenantScope(f.tenant), d.Filter{})
	must(t, e)
	if len(versions.Items) != 1 || versions.Items[0].ID != private.VersionID {
		t.Fatal("disabled product remained adoptable", versions)
	}
}
