package resources_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestStructureInstallationsAndLegacyResolution(t *testing.T) {
	f := resourcesSetup(t)
	b := runtime.Compose(displayReadPlanAccess(t, f))
	f.srv = b.Resources
	x := structureDisplaySetup(t, f, b)
	x.addDevices(t)
	scope := d.TenantScope(f.tenant)
	meta := func() d.Meta { return d.Meta{RequestID: uuid.NewString(), Reason: "安装与旧断面迁移核对"} }
	install := func(index int, node string) d.Result {
		current, err := b.Devices.Detail(ctx, f.actor, scope, x.devices[index].ID)
		structureDisplayMust(t, err)
		in := d.InstallationCreateCommand(d.InstallationCreate{Meta: meta(), InstallationRevision: current.InstallationRevision, InstalledAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano), InstallationFields: d.InstallationFields{Target: d.Target{Kind: string(x.kinds[node]), ID: x.nodes[node].ID}}})
		preview, err := b.Devices.PreviewInstallation(ctx, f.actor, scope, current.ID, in)
		structureDisplayMust(t, err)
		in.PreviewHash = preview.Hash
		out, err := b.Devices.Install(ctx, f.actor, scope, current.ID, in, access.Meta{})
		structureDisplayMust(t, err)
		return out
	}
	installed := install(3, "S1")
	install(4, "H1")
	for index, node := range map[int]string{3: "S1", 4: "H1"} {
		page, err := b.Devices.Installations(ctx, x.p.actor, d.ProjectScope(f.tenant, x.p.id), x.devices[index].ID, d.Filter{})
		structureDisplayMust(t, err)
		if len(page.Items) != 1 || page.Items[0].TargetKind != string(x.kinds[node]) || page.Items[0].Visibility != "full" {
			t.Fatal(page)
		}
	}
	var beforeSources int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_source_bindings WHERE tenant_id=$1`, f.tenant).Scan(&beforeSources))
	if beforeSources != 6 {
		t.Fatal(beforeSources)
	}
	x.display(t, true)
	rt, dt := dr.Target{View: "resources"}, dr.Target{View: "devices"}
	statistics := readData[dr.DeviceStatistics](t, readCall(t, x.h, "statistics.devices", dr.DeviceStatisticsRequest{Target: dt, ResourceTarget: &rt, Population: "authorized", Relation: "installation", GroupBy: "monitoring_section", Membership: "nearest"}))
	if statistics.Totals.UniqueDeviceCount != "6" {
		t.Fatal(statistics)
	}
	counts := map[string]string{}
	for _, group := range statistics.Groups {
		counts[group.Key] = group.Totals.UniqueDeviceCount
	}
	if counts[x.nodes["S1"].ID] != "2" || counts["unassigned"] != "4" {
		t.Fatal(counts)
	}
	// Installation relocation changes its own history without changing sources.
	move := d.InstallationRelocateCommand(installed.InstallationID, d.InstallationRelocate{RevisionInput: d.RevisionInput{Meta: meta(), Revision: installed.Revision}, InstallationRevision: installed.InstallationRevision, InstallationFields: d.InstallationFields{Target: d.Target{Kind: "borehole", ID: x.nodes["H2"].ID}}})
	preview, err := b.Devices.PreviewInstallation(ctx, f.actor, scope, x.devices[3].ID, move)
	structureDisplayMust(t, err)
	move.PreviewHash = preview.Hash
	_, err = b.Devices.Install(ctx, f.actor, scope, x.devices[3].ID, move, access.Meta{})
	structureDisplayMust(t, err)
	history, err := b.Devices.Installations(ctx, f.actor, scope, x.devices[3].ID, d.Filter{})
	structureDisplayMust(t, err)
	if len(history.Items) != 2 || history.Items[0].TargetKind != "borehole" || history.Items[1].TargetKind != "monitoring_section" || !history.Items[1].UninstalledAt.Equal(history.Items[0].InstalledAt) {
		t.Fatal(history)
	}
	// Capture distinct occurrences exactly as the migration does. Their equal
	// text does not identify a section or combine historical records.
	for i := 0; i < 3; i++ {
		_, err = f.env.Pool.Exec(ctx, `INSERT INTO engineering_legacy_section_evidence(tenant_id,facility_id,resource_kind,resource_id,device_id,source_table,source_id,source_revision,source_path,entry_index,original,historical) VALUES($1,$2,'installation',$3,$4,'device_installations',$3,'1','position.sections',$5,'{"code":"same","name":"重复断面文字"}',$6)`, f.tenant, x.nodes["A"].ID, installed.InstallationID, x.devices[3].ID, i, i == 2)
		structureDisplayMust(t, err)
	}
	page, err := b.Devices.LegacySections(ctx, f.actor, scope, x.devices[3].ID, d.Filter{Limit: 2})
	structureDisplayMust(t, err)
	if len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatal(page)
	}
	next, err := b.Devices.LegacySections(ctx, f.actor, scope, x.devices[3].ID, d.Filter{Limit: 2, Cursor: page.NextCursor})
	structureDisplayMust(t, err)
	if len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatal(next)
	}
	evidence := page.Items[0]
	in := d.LegacySectionResolution{Meta: meta(), Status: "mapped", Target: &d.Target{Kind: "monitoring_section", ID: x.nodes["S4"].ID}}
	_, err = b.Devices.PreviewLegacySection(ctx, f.actor, scope, x.devices[3].ID, evidence.ID, in)
	if err == nil {
		t.Fatal("mapped text across known facilities")
	}
	in.Target.ID = x.nodes["S2"].ID
	preview, err = b.Devices.PreviewLegacySection(ctx, f.actor, scope, x.devices[3].ID, evidence.ID, in)
	structureDisplayMust(t, err)
	in.PreviewHash = preview.Hash
	stale := in
	stale.RequestID = uuid.NewString()
	// A second preview is not permission to overwrite a newer resolution.
	stalePreview, err := b.Devices.PreviewLegacySection(ctx, f.actor, scope, x.devices[3].ID, evidence.ID, stale)
	structureDisplayMust(t, err)
	stale.PreviewHash = stalePreview.Hash
	result, err := b.Devices.ResolveLegacySection(ctx, f.actor, scope, x.devices[3].ID, evidence.ID, in, access.Meta{})
	structureDisplayMust(t, err)
	if result.ID != x.devices[3].ID || result.Revision == "" {
		t.Fatal(result)
	}
	replay, err := b.Devices.ResolveLegacySection(ctx, f.actor, scope, x.devices[3].ID, evidence.ID, in, access.Meta{})
	structureDisplayMust(t, err)
	if !replay.Replayed {
		t.Fatal(replay)
	}
	_, err = b.Devices.ResolveLegacySection(ctx, f.actor, scope, x.devices[3].ID, evidence.ID, stale, access.Meta{})
	if err == nil {
		t.Fatal("stale resolution preview accepted")
	}
	page, err = b.Devices.LegacySections(ctx, f.actor, scope, x.devices[3].ID, d.Filter{})
	structureDisplayMust(t, err)
	mapped := 0
	for _, v := range page.Items {
		if v.Status == "mapped" {
			mapped++
			if v.TargetID == nil || *v.TargetID != x.nodes["S2"].ID || !bytes.Equal(v.Original, evidence.Original) {
				t.Fatal(v)
			}
		}
	}
	if mapped != 1 || len(page.Items) != 3 {
		t.Fatal(page)
	}
	_, err = b.Devices.LegacySections(ctx, x.p.actor, d.ProjectScope(f.tenant, x.p.id), x.devices[3].ID, d.Filter{})
	if err == nil {
		t.Fatal("project read tenant migration evidence")
	}
	var afterSources, resolutions int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM monitoring_source_bindings WHERE tenant_id=$1`, f.tenant).Scan(&afterSources))
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM engineering_legacy_section_resolutions WHERE evidence_id=$1`, evidence.ID).Scan(&resolutions))
	if afterSources != beforeSources || resolutions != 1 {
		t.Fatal(afterSources, resolutions)
	}
	// A mapped historical target remains protected even after the result is
	// reclassified as historical evidence.
	in = d.LegacySectionResolution{Meta: meta(), Status: "historical"}
	preview, err = b.Devices.PreviewLegacySection(ctx, f.actor, scope, x.devices[3].ID, evidence.ID, in)
	structureDisplayMust(t, err)
	in.PreviewHash = preview.Hash
	_, err = b.Devices.ResolveLegacySection(ctx, f.actor, scope, x.devices[3].ID, evidence.ID, in, access.Meta{})
	structureDisplayMust(t, err)
	var impact d.ResourceImpact
	structureDisplayMust(t, f.env.Service.Access().Transact(ctx, func(tx pgx.Tx) error {
		var e error
		impact, e = b.Devices.ResourceImpactTx(ctx, tx, f.tenant, "monitoring_section", x.nodes["S2"].ID)
		return e
	}))
	if !impact.Referenced {
		t.Fatal("lost retained resolution target")
	}
}
