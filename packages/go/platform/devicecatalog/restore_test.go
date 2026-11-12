package devicecatalog_test

import (
	"bytes"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	src "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/url"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDeviceGraphBackupRestore(t *testing.T) {
	f := setup(t)
	b := runtime.Compose(f.env.Service.Access())
	scope := d.TenantScope(f.tenant)
	targets := f.sourceTargets(t, b)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "backup", Name: "备份公共厂家"}, access.Meta{})
	must(t, e)
	private, e := f.s.CreateManufacturer(ctx, f.owner, scope, c.ManufacturerCreate{Meta: f.meta(t), Code: "private", Name: "备份私有厂家"}, access.Meta{})
	must(t, e)
	p := f.product(t, c.PlatformScope(), f.actor(), m.ID, "sensor", []c.Slot{f.fixed(), {Key: "input", Name: "可配置", Mode: "configurable", AllowedShapes: []string{"decimal"}}})
	f.product(t, scope, f.owner, private.ID, "private", nil)
	_, e = f.s.CopyProduct(ctx, f.owner, scope, c.ProductCopy{Meta: f.meta(t), SourceVersionID: p.VersionID, ManufacturerID: private.ID, Code: "copy"}, access.Meta{})
	must(t, e)
	create := d.DeviceCreate{Meta: f.meta(t), Code: "BACKUP-DEVICE", Name: "含完整履历的设备", ProductVersionID: p.VersionID}
	device, e := b.Devices.Create(ctx, f.owner, scope, create, access.Meta{})
	must(t, e)
	slots, e := b.Devices.Slots(ctx, f.owner, scope, device.ID, d.Filter{})
	must(t, e)
	var fixed d.Output
	for _, slot := range slots.Items {
		if slot.Current != nil {
			fixed = *slot.Current
		}
	}
	output, e := b.Devices.ConfigureOutput(ctx, f.owner, scope, device.ID, d.OutputConfigure{Meta: f.meta(t), OutputRevision: device.OutputRevision, SlotKey: "input", OutputFields: d.OutputFields{Name: "配置输出", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}}, access.Meta{})
	must(t, e)
	replace := d.OutputReplace{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: output.Revision}, OutputRevision: output.OutputRevision, OutputFields: d.OutputFields{Name: "换代输出", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}}
	preview, e := b.Devices.PreviewOutput(ctx, f.owner, scope, device.ID, output.OutputID, "replace", replace)
	must(t, e)
	replace.PreviewHash = preview.Hash
	next, e := b.Devices.ReplaceOutput(ctx, f.owner, scope, device.ID, output.OutputID, "replace", replace, access.Meta{})
	must(t, e)
	t0 := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
	installed, e := b.Devices.Install(ctx, f.owner, scope, device.ID, d.InstallationCreateCommand(d.InstallationCreate{Meta: f.meta(t), InstallationFields: d.InstallationFields{Target: d.Target{Kind: "facility", ID: targets[0].FacilityID}}, InstallationRevision: "0", InstalledAt: t0}), access.Meta{})
	must(t, e)
	correction := d.InstallationCorrectCommand(installed.InstallationID, d.InstallationCorrect{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: installed.Revision}, InstallationRevision: installed.InstallationRevision, InstalledAt: t0, InstallationFields: d.InstallationFields{Target: d.Target{Kind: "location"}, LocationDescription: "纠错后的独立机柜"}})
	preview, e = b.Devices.PreviewInstallation(ctx, f.owner, scope, device.ID, correction)
	must(t, e)
	correction.PreviewHash = preview.Hash
	corrected, e := b.Devices.Install(ctx, f.owner, scope, device.ID, correction, access.Meta{})
	must(t, e)
	move := d.InstallationRelocateCommand(installed.InstallationID, d.InstallationRelocate{RevisionInput: d.RevisionInput{Meta: f.meta(t), Revision: corrected.Revision}, InstallationRevision: corrected.InstallationRevision, InstallationFields: d.InstallationFields{Target: d.Target{Kind: "facility", ID: targets[1].FacilityID}}})
	preview, e = b.Devices.PreviewInstallation(ctx, f.owner, scope, device.ID, move)
	must(t, e)
	move.PreviewHash = preview.Hash
	_, e = b.Devices.Install(ctx, f.owner, scope, device.ID, move, access.Meta{})
	must(t, e)
	apply := func(target src.Target, in src.Command) src.Result {
		t.Helper()
		p, e := b.Sources.Preview(ctx, f.owner, scope, target, in)
		must(t, e)
		in.PreviewHash = p.Hash
		r, e := b.Sources.Mutate(ctx, f.owner, scope, target, in, access.Meta{})
		must(t, e)
		return r
	}
	bound := apply(targets[0], src.Bind(src.BindInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: "0", ChannelRevision: "1"}, NewEnd: src.NewEnd{OutputID: fixed.ID, NewOutputRevision: "1", NewDeviceRevision: "1"}}))
	switched := apply(targets[0], src.Switch(src.SwitchInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: bound.SourceRevision, ChannelRevision: "1"}, OldEnd: src.OldEnd{BindingID: bound.BindingID, BindingRevision: bound.Revision, OldOutputRevision: "1", OldDeviceRevision: "1"}, NewEnd: src.NewEnd{OutputID: next.OutputID, NewOutputRevision: "1", NewDeviceRevision: "1"}}))
	apply(targets[0], src.Unbind(src.UnbindInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: switched.SourceRevision, ChannelRevision: "1"}, OldEnd: src.OldEnd{BindingID: switched.BindingID, BindingRevision: switched.Revision, OldOutputRevision: "1", OldDeviceRevision: "1"}}))
	apply(targets[1], src.Bind(src.BindInput{Revisions: src.Revisions{Meta: f.meta(t), SourceRevision: "0", ChannelRevision: "1"}, NewEnd: src.NewEnd{OutputID: fixed.ID, NewOutputRevision: "1", NewDeviceRevision: "1"}}))
	var member string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, f.tenant, f.owner.ID).Scan(&member))
	project, e := projects.New(f.env.Service.Access()).Create(ctx, f.owner, f.tenant, projects.CreateInput{WriteMeta: projects.WriteMeta{RequestID: f.meta(t).RequestID, Reason: "恢复演练"}, Fields: projects.Fields{Code: "backup", Name: "备份项目", Purpose: "恢复演练"}, Owner: member, Administrator: member}, access.Meta{})
	must(t, e)
	grant := d.GrantChangeSet{Meta: f.meta(t), ProjectRevision: project.Version, AccessRevision: "1", Add: []d.GrantAdd{{Kind: "device", ID: device.ID}, {Kind: "output", ID: fixed.ID, CanUseSource: true}, {Kind: "output", ID: next.OutputID}}}
	preview, e = b.Devices.PreviewGrants(ctx, f.owner, f.tenant, project.ProjectID, grant)
	must(t, e)
	grant.PreviewHash = preview.Hash
	_, e = b.Devices.ChangeGrants(ctx, f.owner, f.tenant, project.ProjectID, grant, access.Meta{})
	must(t, e)
	grants, e := b.Devices.Grants(ctx, f.owner, f.tenant, project.ProjectID, d.Filter{})
	must(t, e)
	var ended d.GrantEnd
	for _, g := range grants.Items {
		if g.OutputID != nil && *g.OutputID == next.OutputID {
			ended = d.GrantEnd{ID: g.ID, Revision: g.Revision}
		}
	}
	revoke := d.GrantChangeSet{Meta: f.meta(t), ProjectRevision: grants.ProjectRevision, AccessRevision: grants.AccessRevision, End: []d.GrantEnd{ended}}
	preview, e = b.Devices.PreviewGrants(ctx, f.owner, f.tenant, project.ProjectID, revoke)
	must(t, e)
	revoke.PreviewHash = preview.Hash
	_, e = b.Devices.ChangeGrants(ctx, f.owner, f.tenant, project.ProjectID, revoke, access.Meta{})
	must(t, e)
	// Capture every base table, including all three histories, receipts and audit.
	rows, e := f.env.Pool.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema=current_schema() AND table_type='BASE TABLE' ORDER BY table_name`)
	must(t, e)
	tables := []string{}
	for rows.Next() {
		var table string
		must(t, rows.Scan(&table))
		tables = append(tables, table)
	}
	must(t, rows.Err())
	rows.Close()
	checks := map[string]string{}
	counts := map[string]int{}
	for _, table := range tables {
		q := fmt.Sprintf(`SELECT count(*),md5(COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text,'[]')) FROM %s r`, pgx.Identifier{table}.Sanitize())
		var count int
		var sum string
		must(t, f.env.Pool.QueryRow(ctx, q).Scan(&count, &sum))
		checks[table] = sum
		counts[table] = count
	}
	for _, table := range []string{"device_manufacturers", "device_products", "device_product_drafts", "device_product_versions", "device_product_version_states", "device_product_dependencies", "devices", "device_outputs", "device_installations", "device_installation_target_refs", "monitoring_source_heads", "monitoring_source_bindings", "project_device_grants", "device_catalog_changes", "device_resource_changes", "project_device_access_changes", "device_operation_receipts"} {
		if counts[table] == 0 {
			t.Fatalf("restore coverage requires populated %s", table)
		}
	}
	u, e := url.Parse(f.env.Config.DatabaseURL)
	must(t, e)
	schema := u.Query().Get("search_path")
	db := "device_restore_" + schema
	compose, e := filepath.Abs("../../../../deploy/compose/test.yaml")
	must(t, e)
	run := func(input []byte, args ...string) []byte {
		t.Helper()
		cmd := exec.Command("docker", append([]string{"compose", "-f", compose, "exec", "-T", "postgres"}, args...)...)
		cmd.Stdin = bytes.NewReader(input)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, e := cmd.Output()
		if e != nil {
			t.Fatalf("restore utility: %v: %s", e, stderr.String())
		}
		return out
	}
	dump := run(nil, "pg_dump", "-U", "infrapro_test", "-d", "infrapro_test", "-n", schema, "-Fc")
	run(nil, "createdb", "-U", "infrapro_test", db)
	defer run(nil, "dropdb", "-U", "infrapro_test", db)
	run(nil, "psql", "-v", "ON_ERROR_STOP=1", "-U", "infrapro_test", "-d", db, "-c", "CREATE EXTENSION btree_gist WITH SCHEMA public")
	run(dump, "pg_restore", "--exit-on-error", "--no-owner", "-U", "infrapro_test", "-d", db)
	u.Path = "/" + db
	pool, e := pgxpool.New(ctx, u.String())
	must(t, e)
	defer pool.Close()
	for _, table := range tables {
		q := fmt.Sprintf(`SELECT md5(COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text,'[]')) FROM %s r`, pgx.Identifier{table}.Sanitize())
		var sum string
		must(t, pool.QueryRow(ctx, q).Scan(&sum))
		if checks[table] != sum {
			t.Fatalf("restored contents changed: %s", table)
		}
	}
	must(t, migrations.Check(ctx, pool))
	a := access.New(pool, slog.Default())
	must(t, a.Check(ctx))
	cfg := f.env.Config
	cfg.DatabaseURL = u.String()
	cfg.RedisKeyPrefix += "restored:"
	service, e := identity.NewService(pool, f.env.Redis, cfg, slog.Default())
	must(t, e)
	_, token, _, e := service.Login(ctx, "DeviceOwner", password, identity.Meta{})
	must(t, e)
	principal, e := service.Authenticate(ctx, token)
	must(t, e)
	restored := runtime.Compose(a)
	replay, e := restored.Devices.Create(ctx, service.AccessActor(principal), scope, create, access.Meta{})
	must(t, e)
	if !replay.Replayed || replay.ID != device.ID {
		t.Fatal("restored receipt was not reusable")
	}
	current, e := restored.Sources.Current(ctx, service.AccessActor(principal), scope, targets[1])
	must(t, e)
	if !current.Source.Configured || current.Source.Output.ID != fixed.ID {
		t.Fatal("current source was lost")
	}
	t.Logf("Full device graph restored and verified: %d tables", len(tables))
}
