package devicecatalog_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
)

func TestDeviceWorkspaceCurrentInstallationFiltersAndProjection(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	scope := d.TenantScope(f.tenant)
	targets := f.sourceTargets(t, b)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "workspace", Name: "厂家"}, access.Meta{})
	must(t, e)
	product := f.product(t, c.PlatformScope(), f.actor(), m.ID, "workspace", nil)
	ids := []string{}
	for index := range 6 {
		device, e := b.Devices.Create(ctx, f.owner, scope, d.DeviceCreate{Meta: f.meta(t), Code: fmt.Sprintf("WORK-%d", index), Name: "工作区设备", ProductVersionID: product.VersionID}, access.Meta{})
		must(t, e)
		ids = append(ids, device.ID)
		if index == 5 {
			continue
		}
		fields := d.InstallationFields{Target: d.Target{Kind: "location"}, LocationDescription: "独立机柜"}
		if index == 0 {
			fields = d.InstallationFields{Target: d.Target{Kind: "facility", ID: targets[0].FacilityID}}
		}
		if index == 1 {
			fields = d.InstallationFields{Target: d.Target{Kind: "point", ID: targets[0].PointID}}
		}
		if index == 2 {
			fields = d.InstallationFields{Target: d.Target{Kind: "point", ID: targets[1].PointID}}
		}
		in := d.InstallationCreate{Meta: f.meta(t), InstallationRevision: "0", InstallationFields: fields}
		if index == 4 {
			in.InstalledAt = time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
			in.UninstalledAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
		}
		_, e = b.Devices.Install(ctx, f.owner, scope, device.ID, d.InstallationCreateCommand(in), access.Meta{})
		must(t, e)
	}
	for _, tc := range []struct {
		name, installation, facility string
		count                        int
	}{
		{"all", "", "", 6}, {"recorded", "recorded", "", 4}, {"unrecorded includes ended history", "unrecorded", "", 2},
		{"independent", "location", "", 1}, {"structured", "resource", "", 3}, {"facility and descendants", "resource", targets[0].FacilityID, 2},
		{"other facility", "", targets[1].FacilityID, 1}, {"incompatible filters", "location", targets[0].FacilityID, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filter := d.Filter{Installation: tc.installation, FacilityID: tc.facility, Limit: 1, ManufacturerID: m.ID, ProductID: product.ID, Query: "WORK-", Source: "public"}
			seen := map[string]bool{}
			for {
				page, e := b.Devices.List(ctx, f.owner, scope, filter)
				must(t, e)
				for _, item := range page.Items {
					if seen[item.ID] {
						t.Fatal("duplicate device across pages")
					}
					seen[item.ID] = true
					if item.CurrentInstallation != nil && item.CurrentInstallation.Visibility != "full" {
						t.Fatal("owner installation projection missing")
					}
					if item.CurrentInstallation != nil && item.CurrentInstallation.TargetKind != "location" && item.CurrentInstallation.TargetName == "" {
						t.Fatal("missing target label")
					}
				}
				if page.NextCursor == "" {
					break
				}
				if filter.Cursor == "" {
					changed := filter
					changed.Cursor = page.NextCursor
					changed.FacilityID = targets[1].FacilityID
					if changed.FacilityID == filter.FacilityID {
						changed.FacilityID = targets[0].FacilityID
					}
					_, e = b.Devices.List(ctx, f.owner, scope, changed)
					wantCode(t, e, "invalid_cursor")
				}
				filter.Cursor = page.NextCursor
			}
			if len(seen) != tc.count {
				t.Fatalf("want %d, got %d", tc.count, len(seen))
			}
		})
	}
	reader := f.deviceReader(t, []access.Code{access.TenantDeviceRead})
	page, e := b.Devices.List(ctx, reader, scope, d.Filter{Installation: "recorded"})
	must(t, e)
	if len(page.Items) != 4 {
		t.Fatal("device-only reader lost devices")
	}
	for _, item := range page.Items {
		installation := item.CurrentInstallation
		if installation == nil {
			t.Fatal("missing current installation")
		}
		if item.ID == ids[3] {
			if installation.LocationDescription != "独立机柜" {
				t.Fatal("independent location hidden")
			}
			continue
		}
		if installation.Visibility != "restricted" || installation.FacilityID != nil || installation.PointID != nil || installation.TargetName != "" || installation.TargetKind != "" {
			t.Fatalf("resource target leaked: %+v", installation)
		}
	}
	_, e = b.Devices.List(ctx, reader, scope, d.Filter{FacilityID: targets[0].FacilityID})
	wantCode(t, e, "permission_denied")
	_, e = b.Devices.List(ctx, f.owner, scope, d.Filter{FacilityID: "not-a-uuid"})
	wantCode(t, e, "invalid_request")
	_, e = b.Devices.List(ctx, f.owner, scope, d.Filter{Installation: "not-a-location-kind"})
	wantCode(t, e, "invalid_request")
}
