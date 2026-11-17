package parsing_test

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	parse "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdditiveMigrationPreservesLegacyObjects(t *testing.T) {
	f := setupAt(t, 43)
	scope := c.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), c.PlatformScope(), c.ManufacturerCreate{Meta: f.meta(t), Code: "legacy", Name: "原有厂家"}, access.Meta{})
	must(t, e)
	_, dev := seedLegacyProductDevice(t, f, m.ID, "legacy")
	tables := []string{"device_products", "device_product_versions", "device_product_version_slots", "devices", "device_outputs", "device_installations", "monitoring_channel_device_bindings"}
	before := map[string]string{}
	for _, table := range tables {
		var exists bool
		must(t, f.env.Pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists))
		if !exists {
			continue
		}
		var hash string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(COALESCE(string_agg(to_jsonb(t)::text,'' ORDER BY to_jsonb(t)::text),'')) FROM `+table+` t`).Scan(&hash))
		before[table] = hash
	}
	must(t, migrations.Up(ctx, f.env.Config.DatabaseURL))
	must(t, migrations.Check(ctx, f.env.Pool))
	for table, want := range before {
		var got string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(COALESCE(string_agg(to_jsonb(t)::text,'' ORDER BY to_jsonb(t)::text),'')) FROM `+table+` t`).Scan(&got))
		if got != want {
			t.Fatal("migration rewrote", table)
		}
	}
	state, e := f.ps.Device(ctx, f.owner, scope, dev.ID)
	must(t, e)
	if state.Revision != "0" || state.Mode != "unconfigured" {
		t.Fatal(state)
	}
	var adopted int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM parsing_device_adoptions`).Scan(&adopted))
	if adopted != 0 {
		t.Fatal("automatic adoption")
	}
	t.Logf("v43 -> v%d preserved legacy product/version/slot/device/output rows; no automatic adoption", migrations.Version)
}
func TestProjectReadOnlyTaskAtomicityAndAuditPrivacy(t *testing.T) {
	f := setup(t)
	f.sync(t)
	scope := c.TenantScope(f.tenant)
	platform := c.PlatformScope()
	m, e := f.s.CreateManufacturer(ctx, f.actor(), platform, c.ManufacturerCreate{Meta: f.meta(t), Code: "project", Name: "合成"}, access.Meta{})
	must(t, e)
	p := f.product(t, platform, f.actor(), m.ID, "project", []c.Slot{f.fixed()})
	cfg := f.config(t, p, "1.0.0")
	def, e := f.ps.SetDefault(ctx, f.actor(), platform, p.ID, p.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: "0", ConfigRevisionID: cfg.ConfigRevisionID})
	must(t, e)
	dev, e := f.b.Devices.Create(ctx, f.owner, scope, devices.DeviceCreate{Meta: f.meta(t), Code: "project", Name: "模拟设备", SerialNumber: "00012345678", ProductVersionID: p.VersionID, ParsingAdoption: &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: def.Revision}}, access.Meta{})
	must(t, e)
	var member string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2`, f.tenant, f.owner.ID).Scan(&member))
	project, e := projects.New(f.env.Service.Access()).Create(ctx, f.owner, f.tenant, projects.CreateInput{Fields: projects.Fields{Code: "readonly", Name: "只读", Purpose: "边界"}, WriteMeta: projects.WriteMeta{RequestID: f.meta(t).RequestID, Reason: "验收"}, Owner: member, Administrator: member}, access.Meta{})
	must(t, e)
	ps := devices.ProjectScope(f.tenant, project.ProjectID)
	_, e = f.ps.Device(ctx, f.owner, ps, dev.ID)
	if e == nil {
		t.Fatal("ungranted device visible")
	}
	grant := devices.GrantChangeSet{Meta: f.meta(t), AccessRevision: "1", ProjectRevision: project.Version, Add: []devices.GrantAdd{{Kind: "device", ID: dev.ID}}}
	preview, e := f.b.Devices.PreviewGrants(ctx, f.owner, f.tenant, project.ProjectID, grant)
	must(t, e)
	grant.PreviewHash = preview.Hash
	_, e = f.b.Devices.ChangeGrants(ctx, f.owner, f.tenant, project.ProjectID, grant, access.Meta{})
	must(t, e)
	_, e = f.ps.Device(ctx, f.owner, ps, dev.ID)
	must(t, e)
	_, e = f.ps.Command(ctx, f.owner, ps, dev.ID, parse.Command{Meta: f.meta(t), Revision: "1", Action: "clear"})
	if e == nil {
		t.Fatal("project wrote adoption")
	}
	sample := f.sample(t, scope, f.owner, "measurement.hex", "hex")
	in := parse.RunInput{Meta: f.meta(t), Kind: "device_test", DeviceID: dev.ID, SampleID: sample, Transport: "tcp"}
	_, e = f.ps.CreateRun(ctx, f.owner, ps, in)
	if e == nil {
		t.Fatal("project ran parsing")
	}
	_, e = f.ps.SampleContent(ctx, f.owner, ps, sample, false)
	if e == nil {
		t.Fatal("project downloaded raw")
	}
	r, e := f.ps.CreateRun(ctx, f.owner, scope, in)
	must(t, e)
	again, e := f.ps.CreateRun(ctx, f.owner, scope, in)
	must(t, e)
	if !again.Replayed || r.ID != again.ID {
		t.Fatal("lost response duplicated run")
	}
	in.Reason = "different payload"
	_, e = f.ps.CreateRun(ctx, f.owner, scope, in)
	wantCode(t, e, "request_conflict")
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	pubSample := f.sample(t, platform, f.actor(), "measurement.bin", "binary")
	pub := f.methodRun(t, platform, f.actor(), pubSample, "1.0.0")
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	for _, item := range []struct{ id, scope string }{{r.ID, "tenant"}, {pub.ID, "platform"}} {
		var change taskrecord.Change
		must(t, f.env.Pool.QueryRow(ctx, `SELECT task_snapshot FROM parsing_runs WHERE id=$1`, item.id).Scan(&change))
		if change.Scope.Kind != item.scope || change.State != "succeeded" {
			t.Fatal(change)
		}
		change.Scope = taskrecord.Scope{Kind: "project", Tenant: f.tenant, Project: project.ProjectID}
		if e := change.Validate(); e == nil {
			t.Fatal("project parsing task accepted")
		}
	}
	_, e = f.env.Pool.Exec(ctx, `CREATE FUNCTION parsing_reject_task() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.source_module='parsing' THEN RAISE EXCEPTION 'injected_task_failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER parsing_reject_task BEFORE INSERT ON task_records FOR EACH ROW EXECUTE FUNCTION parsing_reject_task()`)
	must(t, e)
	var before, after int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM parsing_runs`).Scan(&before))
	in.Meta = f.meta(t)
	_, e = f.ps.CreateRun(ctx, f.owner, scope, in)
	if e == nil {
		t.Fatal("task failure committed source")
	}
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM parsing_runs`).Scan(&after))
	if before != after {
		t.Fatal("partial task commit")
	}
	_, e = f.env.Pool.Exec(ctx, `DROP TRIGGER parsing_reject_task ON task_records; DROP FUNCTION parsing_reject_task()`)
	must(t, e)
	var raw string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT COALESCE(string_agg(to_jsonb(a)::text,''),'') FROM account_security_events a WHERE event_type LIKE 'parsing.%'`).Scan(&raw))
	if raw == "" {
		t.Fatal("parsing audit absent")
	}
	for _, secret := range []string{"12.34", "observed_at", "00012345678", "base64", "machine-token"} {
		if strings.Contains(raw, secret) {
			t.Fatal("audit exposed", secret)
		}
	}
	var formal int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM monitoring_publications)+(SELECT count(*) FROM monitoring_chunks)+(SELECT count(*) FROM monitoring_import_batches)`).Scan(&formal))
	if formal != 0 {
		t.Fatal("trial wrote formal monitoring data")
	}
	t.Log("project read-only; platform/tenant task facts; project task rejection; task registration rollback; idempotency and audit redaction")
}
func TestScopeConcurrencyAndFrozenAdoption(t *testing.T) {
	f := setup(t)
	f.sync(t)
	scope := c.TenantScope(f.tenant)
	sample := f.sample(t, scope, f.owner, "measurement.bin", "binary")
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	var calls atomic.Int32
	f.proxy(t, func(b []byte) ([]byte, int) { calls.Add(1); entered <- struct{}{}; <-release; return b, 200 })
	for i := 0; i < 3; i++ {
		f.methodRun(t, scope, f.owner, sample, "1.0.0")
	}
	done := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func() { _, e := f.ps.RunOne(ctx); done <- e }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("concurrency lane did not start")
		}
	}
	select {
	case <-entered:
		t.Fatal("scope exceeded two simultaneous parses")
	case <-time.After(500 * time.Millisecond):
	}
	close(release)
	for i := 0; i < 3; i++ {
		must(t, <-done)
	}
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
	_, e := f.ps.RunOne(ctx)
	must(t, e)
	list, e := f.ps.Runs(ctx, f.owner, scope, c.Filter{Limit: 1})
	must(t, e)
	if list.NextCursor == "" {
		t.Fatal("no bounded cursor")
	}
	_, e = f.ps.Runs(ctx, f.actor(), c.PlatformScope(), c.Filter{Limit: 1, Cursor: list.NextCursor})
	if e == nil {
		t.Fatal("cross-scope cursor accepted")
	}
	// All stored attempts carry the same frozen bytes and exact method artifact.
	rows, e := f.env.Pool.Query(ctx, `SELECT input_snapshot FROM parsing_runs`)
	must(t, e)
	defer rows.Close()
	for rows.Next() {
		var input json.RawMessage
		must(t, rows.Scan(&input))
		if !strings.Contains(string(input), "1.0.0") {
			t.Fatal("input version changed")
		}
	}
}

func TestFrozenRunMappingRevisionsAndExplicitCopy(t *testing.T) {
	f := setup(t)
	f.sync(t)
	platform, scope := c.PlatformScope(), c.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), platform, c.ManufacturerCreate{Meta: f.meta(t), Code: "mapping", Name: "合成厂家"}, access.Meta{})
	must(t, e)
	timeSlot := f.fixed()
	timeSlot.TimestampMeaning = "interval_end"
	timeSlot.AggregationDescription = "hourly mean"
	bad := f.product(t, platform, f.actor(), m.ID, "bad-time", []c.Slot{timeSlot})
	_, e = f.ps.SaveConfig(ctx, f.actor(), platform, bad.ID, bad.VersionID, "", "", parse.ConfigInput{Meta: f.meta(t), Revision: "0", Name: "时间语义冲突", MethodID: example, MethodVersion: "1.0.0", Bindings: []parse.Binding{{OutputKey: "displacement", SlotKey: "x", Confirmed: true}}})
	if e == nil {
		t.Fatal("known time mismatch accepted")
	}
	p := f.product(t, platform, f.actor(), m.ID, "mapping", []c.Slot{{Key: "x", Name: "可配置", Mode: "configurable", AllowedShapes: []string{"decimal"}}})
	cfg := f.config(t, p, "1.0.0")
	def, e := f.ps.SetDefault(ctx, f.actor(), platform, p.ID, p.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: "0", ConfigRevisionID: cfg.ConfigRevisionID})
	must(t, e)
	dev, e := f.b.Devices.Create(ctx, f.owner, scope, devices.DeviceCreate{Meta: f.meta(t), Code: "mapping", Name: "模拟", ProductVersionID: p.VersionID, ParsingAdoption: &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: def.Revision}}, access.Meta{})
	must(t, e)
	initialDevice, e := f.b.Devices.Detail(ctx, f.owner, scope, dev.ID)
	must(t, e)
	configured, e := f.b.Devices.ConfigureOutput(ctx, f.owner, scope, dev.ID, devices.OutputConfigure{Meta: f.meta(t), SlotKey: "x", OutputRevision: initialDevice.OutputRevision, OutputFields: devices.OutputFields{Name: "实际位移", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}}, access.Meta{})
	must(t, e)
	sample := f.sample(t, scope, f.owner, "measurement.bin", "binary")
	run, e := f.ps.CreateRun(ctx, f.owner, scope, parse.RunInput{Meta: f.meta(t), Kind: "device_test", DeviceID: dev.ID, SampleID: sample, Transport: "tcp"})
	must(t, e)
	detail, e := f.ps.Run(ctx, f.owner, scope, run.ID)
	must(t, e)
	frozen := detail.Run.Input.Mapping[0]
	if frozen.OutputID != configured.OutputID {
		t.Fatal(frozen)
	}
	next, e := f.ps.SaveConfig(ctx, f.actor(), platform, p.ID, p.VersionID, "", cfg.ID, parse.ConfigInput{Meta: f.meta(t), Revision: cfg.Revision, Name: "新的固定修订", MethodID: example, MethodVersion: "1.1.0", Bindings: []parse.Binding{{OutputKey: "displacement", SlotKey: "x", Confirmed: true}}})
	must(t, e)
	_, e = f.ps.SetDefault(ctx, f.actor(), platform, p.ID, p.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: def.Revision, ConfigRevisionID: next.ConfigRevisionID})
	must(t, e)
	_, e = f.ps.Command(ctx, f.owner, scope, dev.ID, parse.Command{Meta: f.meta(t), Revision: "1", Action: "upgrade_default", DefaultRevision: "2"})
	must(t, e)
	curr, e := f.b.Devices.Detail(ctx, f.owner, scope, dev.ID)
	must(t, e)
	end := devices.OutputReplace{RevisionInput: devices.RevisionInput{Meta: f.meta(t), Revision: "1"}, OutputRevision: curr.OutputRevision}
	pv, e := f.b.Devices.PreviewOutput(ctx, f.owner, scope, dev.ID, configured.OutputID, "end", end)
	must(t, e)
	end.PreviewHash = pv.Hash
	_, e = f.b.Devices.ReplaceOutput(ctx, f.owner, scope, dev.ID, configured.OutputID, "end", end, access.Meta{})
	must(t, e)
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	detail, e = f.ps.Run(ctx, f.owner, scope, run.ID)
	must(t, e)
	if !detail.CurrentChanged || detail.Run.Input.Entry.MethodVersion != "1.0.0" || detail.Run.Input.Mapping[0].OutputID != frozen.OutputID {
		t.Fatal("in-flight snapshot rewritten")
	}
	newer, e := f.ps.CreateRun(ctx, f.owner, scope, parse.RunInput{Meta: f.meta(t), Kind: "device_test", DeviceID: dev.ID, SampleID: sample, Transport: "tcp"})
	must(t, e)
	d, e := f.ps.Run(ctx, f.owner, scope, newer.ID)
	must(t, e)
	if d.Run.Input.Entry.MethodVersion != "1.1.0" || d.Run.Input.Mapping[0].Status != "output_unconfigured" {
		t.Fatal("new run ignored mapping change")
	}
	list, e := f.ps.Configs(ctx, f.actor(), platform, p.ID, p.VersionID, false, c.Filter{})
	must(t, e)
	if len(list.Items) != 2 {
		t.Fatal("revision overwritten")
	}
	_, e = f.env.Pool.Exec(ctx, `DELETE FROM devices WHERE id=$1`, dev.ID)
	if e == nil {
		t.Fatal("deleted referenced device")
	}
	copyProduct, e := f.s.CopyProduct(ctx, f.actor(), platform, c.ProductCopy{Meta: f.meta(t), SourceVersionID: p.VersionID, ManufacturerID: m.ID, Code: "copied"}, access.Meta{})
	must(t, e)
	copyProduct, e = f.s.Publish(ctx, f.actor(), platform, copyProduct.ID, c.DraftRevision{Meta: f.meta(t), Revision: copyProduct.Revision}, access.Meta{})
	must(t, e)
	copiedDefault, e := f.ps.Default(ctx, f.actor(), platform, copyProduct.ID, copyProduct.VersionID, false)
	must(t, e)
	if copiedDefault.Config != nil || copiedDefault.Revision != "0" {
		t.Fatal("copy implicitly inherited parsing default")
	}
	copiedConfig, e := f.ps.SaveConfig(ctx, f.actor(), platform, copyProduct.ID, copyProduct.VersionID, "", "", parse.ConfigInput{Meta: f.meta(t), Revision: "0", Name: "显式复制并重验", SourceRevisionID: cfg.ConfigRevisionID})
	must(t, e)
	if copiedConfig.ConfigRevisionID == cfg.ConfigRevisionID {
		t.Fatal("copy reused owner-bound revision")
	}
	t.Log("new immutable config revision; in-flight 1.0/output generation retained; later 1.1 run explicitly unconfigured; referenced device deletion rejected")
}

func TestMappingRejectsKnownUnitAndShapeConflict(t *testing.T) {
	f := setup(t)
	f.sync(t)
	scope := c.PlatformScope()
	wm := func() eng.WriteMeta { m := f.meta(t); return eng.WriteMeta{RequestID: m.RequestID, Reason: m.Reason} }
	unit, e := f.e.Create(ctx, f.actor(), eng.Scope{Kind: eng.UnitDefinition}, eng.CreateInput{WriteMeta: wm(), Code: "cm", Content: eng.Content{MetamodelVersion: "1", Name: "厘米", Unit: &eng.Unit{Symbol: "cm", Category: "length"}}}, access.Meta{})
	must(t, e)
	unit, e = f.e.Publish(ctx, f.actor(), eng.Scope{Kind: eng.UnitDefinition}, unit.DefinitionID, eng.RevisionInput{WriteMeta: wm(), Revision: unit.Revision}, access.Meta{})
	must(t, e)
	metric, e := f.e.Create(ctx, f.actor(), eng.Scope{Kind: eng.MetricDefinition}, eng.CreateInput{WriteMeta: wm(), Code: "cm_displacement", Content: eng.Content{MetamodelVersion: "1", Name: "厘米位移", Metric: &eng.Metric{Shape: "decimal", Category: "length", StandardUnit: unit.VersionID, AllowedUnits: []string{unit.VersionID}}}}, access.Meta{})
	must(t, e)
	metric, e = f.e.Publish(ctx, f.actor(), eng.Scope{Kind: eng.MetricDefinition}, metric.DefinitionID, eng.RevisionInput{WriteMeta: wm(), Revision: metric.Revision}, access.Meta{})
	must(t, e)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), scope, c.ManufacturerCreate{Meta: f.meta(t), Code: "units", Name: "合成"}, access.Meta{})
	must(t, e)
	for i, slot := range []c.Slot{{Key: "x", Name: "厘米", Mode: "fixed", MetricVersionID: metric.VersionID, UnitVersionID: &unit.VersionID, TimestampMeaning: "observation_time"}, {Key: "x", Name: "整数", Mode: "configurable", AllowedShapes: []string{"integer"}}} {
		p := f.product(t, scope, f.actor(), m.ID, []string{"unit-conflict", "shape-conflict"}[i], []c.Slot{slot})
		_, e = f.ps.SaveConfig(ctx, f.actor(), scope, p.ID, p.VersionID, "", "", parse.ConfigInput{Meta: f.meta(t), Revision: "0", Name: "不可自动换算", MethodID: example, MethodVersion: "1.0.0", Bindings: []parse.Binding{{OutputKey: "displacement", SlotKey: "x", Confirmed: true}}})
		if e == nil {
			t.Fatal("incompatible measurement accepted", i)
		}
	}
}

func TestIngestionMigrationPreservesV44ParsingEvidence(t *testing.T) {
	f := setupAt(t, 44)
	// Reproduce the old composition while the new ingestion tables do not exist.
	f.ps.WithIngress(nil)
	f.sync(t)
	platform, scope := c.PlatformScope(), c.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), platform, c.ManufacturerCreate{Meta: f.meta(t), Code: "pre_ingestion", Name: "迁移前厂家"}, access.Meta{})
	must(t, e)
	product, device := seedLegacyProductDevice(t, f, m.ID, "pre_ingestion")
	sample := f.sample(t, scope, f.owner, "measurement.bin", "binary")
	run := seedLegacyParsingRun(t, f, product, device, sample)
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	beforeResult, _, e := f.ps.Result(ctx, f.owner, scope, run.ID, "1", false, 0, 100)
	must(t, e)
	if beforeResult.Response == nil || *beforeResult.Response.Records[0].Measurements[0].Value != "12.34" {
		t.Fatal("historical fixture did not execute before upgrade")
	}
	tables := []string{"parsing_method_versions", "parsing_configs", "parsing_config_revisions", "parsing_product_defaults", "parsing_device_adoptions", "parsing_samples", "parsing_runs", "parsing_run_executions", "parsing_objects"}
	hashes := map[string]string{}
	for _, table := range tables {
		var hash string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(COALESCE(string_agg(to_jsonb(t)::text,'' ORDER BY to_jsonb(t)::text),'')) FROM `+table+` t`).Scan(&hash))
		hashes[table] = hash
	}
	must(t, migrations.Up(ctx, f.env.Config.DatabaseURL))
	must(t, migrations.Check(ctx, f.env.Pool))
	for table, want := range hashes {
		var got string
		must(t, f.env.Pool.QueryRow(ctx, `SELECT md5(COALESCE(string_agg(to_jsonb(t)::text,'' ORDER BY to_jsonb(t)::text),'')) FROM `+table+` t`).Scan(&got))
		if got != want {
			t.Fatal("upgrade rewrote", table)
		}
	}
	f.ps.WithIngress(f.b.Ingestion)
	replay, e := f.ps.CreateRun(ctx, f.owner, scope, parse.RunInput{Meta: f.meta(t), ReplayRunID: run.ID})
	must(t, e)
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	result, _, e := f.ps.Result(ctx, f.owner, scope, replay.ID, "1", false, 0, 100)
	must(t, e)
	if result.Response == nil || *result.Response.Records[0].Measurements[0].Value != "12.34" {
		t.Fatal("legacy replay changed")
	}
	var sources, receipts int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ingestion_sources),(SELECT count(*) FROM ingestion_receipts)`).Scan(&sources, &receipts))
	if sources+receipts != 0 {
		t.Fatal("migration implicitly enabled ingestion")
	}
	t.Logf("v44 -> v%d preserves exact method/config/default/adoption/sample/run/object rows and replays the original method; ingestion remains empty and disabled", migrations.Version)
}
