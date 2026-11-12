package devicecatalog_test

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
)

var ctx = context.Background()

const password = "Device river 橋梁 2026!"

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		var de *c.Error
		if errors.As(e, &de) {
			t.Fatalf("%s: %+v (cause: %v)", de.Code, de.Issues, de.Cause)
		}
		var ee *eng.Error
		if errors.As(e, &ee) {
			t.Fatalf("%s: %+v", ee.Code, ee.Issues)
		}
		t.Fatal(e)
	}
}
func wantCode(t *testing.T, e error, want string) {
	t.Helper()
	var de *c.Error
	var ae *access.Error
	got := ""
	if errors.As(e, &de) {
		got = de.Code
	}
	if errors.As(e, &ae) {
		got = ae.Code
	}
	if got != want {
		t.Fatalf("want %s, got %s: %v", want, got, e)
	}
}
func ptr[T any](v T) *T { return &v }

type fixture struct {
	env                  *testsupport.Environment
	s                    *c.Service
	e                    *eng.Service
	admin                identity.Principal
	owner                access.Actor
	tenant, metric, unit string
}

func (f fixture) actor() access.Actor { return f.env.Service.AccessActor(f.admin) }
func (f fixture) meta(t *testing.T) c.Meta {
	var id string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&id))
	return c.Meta{RequestID: id, Reason: "设备功能验证"}
}
func (f fixture) member(t *testing.T, name string) identity.Principal {
	l, e := f.env.Service.CreateInvitation(ctx, f.admin, "", "", identity.Meta{})
	must(t, e)
	u, _ := url.Parse(l.URL)
	q, _ := url.ParseQuery(u.Fragment)
	must(t, f.env.Service.AcceptInvitation(ctx, q.Get("token"), name, name, password, identity.Meta{}))
	_, token, _, e := f.env.Service.Login(ctx, name, password, identity.Meta{})
	must(t, e)
	p, e := f.env.Service.Authenticate(ctx, token)
	must(t, e)
	return p
}
func setup(t *testing.T) fixture {
	t.Helper()
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("isolated test database required")
	}
	env, e := testsupport.New(ctx, slog.Default())
	must(t, e)
	t.Cleanup(env.Close)
	_, e = env.Service.Bootstrap(ctx, "Platform", "平台", password, identity.Meta{})
	must(t, e)
	_, token, _, e := env.Service.Login(ctx, "Platform", password, identity.Meta{})
	must(t, e)
	p, e := env.Service.Authenticate(ctx, token)
	must(t, e)
	f := fixture{env: env, s: c.New(env.Service.Access()), e: eng.New(env.Service.Access()), admin: p}
	user := f.member(t, "DeviceOwner")
	f.owner = env.Service.AccessActor(user)
	tid, e := tenancy.New(env.Service.Access()).Create(ctx, f.actor(), tenancy.CreateInput{Fields: tenancy.Fields{Name: "设备测试租户", Code: "devices"}, AdministratorID: user.Account.ID, RequestID: f.meta(t).RequestID, Reason: "验证"}, access.Meta{})
	must(t, e)
	f.tenant = tid.TenantID
	wm := func() eng.WriteMeta { m := f.meta(t); return eng.WriteMeta{RequestID: m.RequestID, Reason: m.Reason} }
	_, e = f.e.SaveCategory(ctx, f.actor(), eng.CategoryInput{WriteMeta: wm(), Key: "length", Name: "长度", Description: "空间长度"}, access.Meta{})
	must(t, e)
	unit, e := f.e.Create(ctx, f.actor(), eng.Scope{Kind: eng.UnitDefinition}, eng.CreateInput{WriteMeta: wm(), Code: "mm", Content: eng.Content{MetamodelVersion: "1", Name: "毫米", Unit: &eng.Unit{Symbol: "mm", Category: "length"}}}, access.Meta{})
	must(t, e)
	unit, e = f.e.Publish(ctx, f.actor(), eng.Scope{Kind: eng.UnitDefinition}, unit.DefinitionID, eng.RevisionInput{WriteMeta: wm(), Revision: unit.Revision}, access.Meta{})
	must(t, e)
	f.unit = unit.VersionID
	metric, e := f.e.Create(ctx, f.actor(), eng.Scope{Kind: eng.MetricDefinition}, eng.CreateInput{WriteMeta: wm(), Code: "displacement", Content: eng.Content{MetamodelVersion: "1", Name: "位移", Metric: &eng.Metric{Shape: "decimal", Category: "length", StandardUnit: f.unit, AllowedUnits: []string{f.unit}}}}, access.Meta{})
	must(t, e)
	metric, e = f.e.Publish(ctx, f.actor(), eng.Scope{Kind: eng.MetricDefinition}, metric.DefinitionID, eng.RevisionInput{WriteMeta: wm(), Revision: metric.Revision}, access.Meta{})
	must(t, e)
	f.metric = metric.VersionID
	return f
}
func (f fixture) product(t *testing.T, scope c.Scope, actor access.Actor, manufacturer, code string, slots []c.Slot) c.Result {
	p, e := f.s.CreateProduct(ctx, actor, scope, c.ProductCreate{Meta: f.meta(t), ManufacturerID: manufacturer, Code: code, Name: "位移传感器", Model: "Sensor-X", OutputSlots: slots}, access.Meta{})
	must(t, e)
	p, e = f.s.Publish(ctx, actor, scope, p.ID, c.DraftRevision{Meta: f.meta(t), Revision: "1"}, access.Meta{})
	must(t, e)
	return p
}
func (f fixture) fixed() c.Slot {
	return c.Slot{Key: "x", Name: "X 输出", Mode: "fixed", MetricVersionID: f.metric, UnitVersionID: &f.unit, TimestampMeaning: "observation_time"}
}
func TestCatalogOwnershipPublicationAndReplay(t *testing.T) {
	f := setup(t)
	platform := c.PlatformScope()
	tenant := c.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), platform, c.ManufacturerCreate{Meta: f.meta(t), Code: "Acme", Name: "公共厂家"}, access.Meta{})
	must(t, e)
	private, e := f.s.CreateManufacturer(ctx, f.owner, tenant, c.ManufacturerCreate{Meta: f.meta(t), Code: "acme", Name: "同名私有厂家"}, access.Meta{})
	must(t, e)
	_, e = f.s.CreateManufacturer(ctx, f.actor(), platform, c.ManufacturerCreate{Meta: f.meta(t), Code: "ACME", Name: "重复编号"}, access.Meta{})
	wantCode(t, e, "code_taken")
	public := f.product(t, platform, f.actor(), m.ID, "sensor", []c.Slot{f.fixed(), {Key: "input", Name: "可配置", Mode: "configurable", AllowedShapes: []string{"decimal", "enum"}}})
	p := f.product(t, tenant, f.owner, m.ID, "private", nil)
	list, e := f.s.Products(ctx, f.actor(), platform, c.Filter{ManufacturerID: m.ID}, false)
	must(t, e)
	if len(list.Items) != 1 || list.Items[0].ID != public.ID {
		t.Fatalf("private product leaked: %+v", list.Items)
	}
	_, e = f.s.Product(ctx, f.actor(), platform, p.ID, c.Filter{})
	wantCode(t, e, "definition_not_found")
	_, e = f.s.CreateProduct(ctx, f.actor(), platform, c.ProductCreate{Meta: f.meta(t), ManufacturerID: private.ID, Code: "invalid", Name: "不可公开", Model: "X"}, access.Meta{})
	wantCode(t, e, "definition_not_found")
	version, e := f.s.Version(ctx, f.owner, tenant, public.ID, public.VersionID, true)
	must(t, e)
	if !version.Usable || len(version.Content.OutputSlots) != 2 {
		t.Fatalf("bad version: %+v", version)
	}
	// Archived manufacturers do not remotely disable existing products.
	state := c.ManufacturerMutation{Action: "state", ManufacturerUpdate: c.ManufacturerUpdate{RevisionInput: c.RevisionInput{Meta: f.meta(t), Revision: m.Revision}}, Status: "archived"}
	preview, e := f.s.PreviewManufacturer(ctx, f.actor(), platform, m.ID, state)
	must(t, e)
	state.PreviewHash = preview.Hash
	_, e = f.s.MutateManufacturer(ctx, f.actor(), platform, m.ID, state, access.Meta{})
	must(t, e)
	_, e = f.s.NewDraft(ctx, f.actor(), platform, public.ID, c.DraftCreate{RevisionInput: c.RevisionInput{Meta: f.meta(t), Revision: public.Revision}, VersionID: public.VersionID}, access.Meta{})
	must(t, e)
	publish := c.DraftRevision{Meta: f.meta(t), Revision: "1"}
	var results [2]c.Result
	var errs [2]error
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = f.s.Publish(ctx, f.actor(), platform, public.ID, publish, access.Meta{})
		}(i)
	}
	wg.Wait()
	for _, e := range errs {
		must(t, e)
	}
	if results[0].VersionID != results[1].VersionID || results[0].Replayed == results[1].Replayed {
		t.Fatal("duplicate publication")
	}
	receipt, e := f.s.Receipt(ctx, f.actor(), platform, publish.RequestID)
	must(t, e)
	if receipt.VersionID != results[0].VersionID {
		t.Fatal("receipt mismatch")
	}
	_, e = f.s.CreateProduct(ctx, f.owner, tenant, c.ProductCreate{Meta: f.meta(t), ManufacturerID: m.ID, Code: "after_archive", Name: "测试", Model: "X"}, access.Meta{})
	wantCode(t, e, "definition_unavailable")
	// SQL must preserve publication and cross-owner invariants independently of Go.
	_, e = f.env.Pool.Exec(ctx, `UPDATE device_product_versions SET content='{}' WHERE id=$1`, public.VersionID)
	if e == nil {
		t.Fatal("published content mutated")
	}
	_, e = f.env.Pool.Exec(ctx, `INSERT INTO device_products(manufacturer_id,code,created_by,updated_by) VALUES($1,'cross_owner',$2,$2)`, private.ID, f.admin.Account.ID)
	if e == nil {
		t.Fatal("SQL accepted public product with private parent")
	}
}
func TestProductSlotMeaningAndVersionAvailability(t *testing.T) {
	f := setup(t)
	scope := c.PlatformScope()
	m, e := f.s.CreateManufacturer(ctx, f.actor(), scope, c.ManufacturerCreate{Meta: f.meta(t), Code: "maker", Name: "厂家"}, access.Meta{})
	must(t, e)
	p := f.product(t, scope, f.actor(), m.ID, "sensor", []c.Slot{f.fixed()})
	v, e := f.s.Version(ctx, f.actor(), scope, p.ID, p.VersionID, false)
	must(t, e)
	_, e = f.s.NewDraft(ctx, f.actor(), scope, p.ID, c.DraftCreate{RevisionInput: c.RevisionInput{Meta: f.meta(t), Revision: p.Revision}, VersionID: v.ID}, access.Meta{})
	must(t, e)
	v.Content.OutputSlots[0].TimestampMeaning = "interval_end"
	v.Content.OutputSlots[0].AggregationDescription = "每小时平均"
	saved, e := f.s.SaveDraft(ctx, f.actor(), scope, p.ID, c.DraftSave{Meta: f.meta(t), Revision: "1", Content: v.Content}, access.Meta{})
	must(t, e)
	validation, e := f.s.Validate(ctx, f.actor(), scope, p.ID)
	must(t, e)
	if validation.Valid || len(validation.Issues) == 0 || validation.Issues[0].Code != "slot_semantics_changed" {
		t.Fatal(validation)
	}
	_, e = f.s.Publish(ctx, f.actor(), scope, p.ID, c.DraftRevision{Meta: f.meta(t), Revision: saved.Revision}, access.Meta{})
	wantCode(t, e, "validation_failed")
	state := c.ProductMutation{Action: "state", VersionID: p.VersionID, VersionState: c.VersionState{RevisionInput: c.RevisionInput{Meta: f.meta(t), Revision: "1"}, Enabled: ptr(false)}}
	preview, e := f.s.PreviewProduct(ctx, f.actor(), scope, p.ID, state)
	must(t, e)
	state.PreviewHash = preview.Hash
	_, e = f.s.MutateProduct(ctx, f.actor(), scope, p.ID, state, access.Meta{})
	must(t, e)
	tx, e := f.env.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	_, _, e = c.ResolveVersionTx(ctx, tx, f.tenant, p.VersionID, true)
	wantCode(t, e, "definition_unavailable")
	old, _, e := c.ResolveVersionTx(ctx, tx, f.tenant, p.VersionID, false)
	must(t, e)
	if old.Content.OutputSlots[0].TimestampMeaning != "observation_time" {
		t.Fatal("draft rewrote adopted semantics")
	}
}
