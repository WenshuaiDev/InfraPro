package parsing_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	parse "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	store                *blobstore.Store
	bucket               string
	b                    runtime.Business
	ps                   *parse.Service
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
func setup(t *testing.T) fixture { return setupAt(t, migrations.Version) }
func setupAt(t *testing.T, version int64) fixture {
	t.Helper()
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("isolated test database required")
	}
	env, e := testsupport.NewAtMigration(ctx, slog.Default(), version)
	must(t, e)
	keepLoad := os.Getenv("INFRAPRO_INGESTION_LOAD") == "1" && os.Getenv("INFRAPRO_INGESTION_KEEP_LOAD") == "1"
	if keepLoad {
		env.PreserveSchema()
	}
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
	f.b = runtime.Compose(env.Service.Access())
	f.ps = f.b.Parsing
	opts := parse.DefaultOptions()
	opts.Enabled = true
	opts.URL = os.Getenv("INFRAPRO_PARSING_URL")
	opts.TokenFile = os.Getenv("INFRAPRO_PARSING_TOKEN_FILE")
	if opts.URL == "" {
		t.Fatal("real parser URL required")
	}
	f.ps.WithOptions(opts)
	bucket := "parsing-" + uuid.NewString()
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	must(t, e)
	must(t, client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}))
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", bucket, "parsing")
	must(t, e)
	f.ps.WithStorage(store)
	f.store = store
	f.bucket = bucket
	if keepLoad {
		var schema string
		must(t, env.Pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema))
		data := fmt.Sprintf(`{"synthetic":true,"schema":%q,"bucket":%q,"business_container":"infrapro-ingestion-lab-business-1"}`, schema, bucket)
		must(t, os.WriteFile(filepath.Join(os.Getenv("INFRAPRO_INGESTION_EVIDENCE"), "retained-fixture.json"), []byte(data), 0600))
	}
	t.Cleanup(func() {
		if keepLoad {
			return
		}
		objects := client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true})
		for failure := range client.RemoveObjects(ctx, bucket, objects, minio.RemoveObjectsOptions{}) {
			t.Errorf("owned fixture object cleanup: %v", failure.Err)
		}
		if e := client.RemoveBucket(ctx, bucket); e != nil {
			t.Errorf("owned fixture bucket cleanup: %v", e)
		}
	})
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

const example = "example.hongdian.ddp.tcp.monitoring-json"

func (f fixture) sync(t *testing.T) {
	_, e := f.ps.Sync(ctx, f.actor(), f.meta(t))
	must(t, e)
	list, e := f.ps.Methods(ctx, f.actor(), c.PlatformScope(), c.Filter{})
	must(t, e)
	if len(list.Items) != 4 {
		t.Fatalf("catalog: %+v", list)
	}
	for _, m := range list.Items {
		_, e = f.ps.SetMethodState(ctx, f.actor(), m.Entry.MethodID, m.Entry.MethodVersion, parse.StateInput{Meta: f.meta(t), Revision: m.Revision, State: "enabled"})
		must(t, e)
	}
}
func (f fixture) sample(t *testing.T, scope c.Scope, a access.Actor, name, representation string) string {
	b, e := os.ReadFile(filepath.Join("../../../../samples/parsing", name))
	must(t, e)
	out, e := f.ps.CreateSample(ctx, a, scope, parse.SampleInput{Meta: f.meta(t), Representation: representation, Source: "synthetic", ReceivedAt: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)})
	must(t, e)
	_, e = f.ps.UploadSample(ctx, a, scope, out.ID, f.meta(t), bytes.NewReader(b))
	must(t, e)
	_, e = f.ps.ConfirmSample(ctx, a, scope, out.ID, f.meta(t))
	must(t, e)
	return out.ID
}
func (f fixture) config(t *testing.T, p c.Result, version string) parse.Result {
	x, e := f.ps.SaveConfig(ctx, f.actor(), c.PlatformScope(), p.ID, p.VersionID, "", "", parse.ConfigInput{Meta: f.meta(t), Revision: "0", Name: "联调示例，非厂家测量协议", MethodID: example, MethodVersion: version, Bindings: []parse.Binding{{OutputKey: "displacement", SlotKey: "x", Confirmed: true}}})
	must(t, e)
	return x
}
func TestRealServiceAdoptionExecutionAndReplay(t *testing.T) {
	f := setup(t)
	f.sync(t)
	scope := c.PlatformScope()
	tenant := c.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), scope, c.ManufacturerCreate{Meta: f.meta(t), Code: "demo", Name: "模拟厂家"}, access.Meta{})
	must(t, e)
	p := f.product(t, scope, f.actor(), m.ID, "demo", []c.Slot{f.fixed()})
	cfg := f.config(t, p, "1.0.0")
	def, e := f.ps.SetDefault(ctx, f.actor(), scope, p.ID, p.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: "0", ConfigRevisionID: cfg.ConfigRevisionID})
	must(t, e)
	input := devices.DeviceCreate{Meta: f.meta(t), ProductVersionID: p.VersionID, Code: "sim-1", Name: "模拟设备", ParsingAdoption: &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: def.Revision}}
	dev, e := f.b.Devices.Create(ctx, f.owner, tenant, input, access.Meta{})
	must(t, e)
	state, e := f.ps.Device(ctx, f.owner, tenant, dev.ID)
	must(t, e)
	if state.Current == nil || state.Current.Snapshot.Method.Version != "1.0.0" {
		t.Fatal(state)
	}
	cfg2 := f.config(t, p, "1.1.0")
	_, e = f.ps.SetDefault(ctx, f.actor(), scope, p.ID, p.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: def.Revision, ConfigRevisionID: cfg2.ConfigRevisionID})
	must(t, e)
	state, e = f.ps.Device(ctx, f.owner, tenant, dev.ID)
	must(t, e)
	if !state.DefaultChanged || state.Current.Snapshot.Method.Version != "1.0.0" {
		t.Fatal("default rewrote adoption")
	}
	sample := f.sample(t, tenant, f.owner, "measurement.bin", "binary")
	runInput := parse.RunInput{Meta: f.meta(t), Kind: "device_test", SampleID: sample, DeviceID: dev.ID, Transport: "tcp"}
	run, e := f.ps.CreateRun(ctx, f.owner, tenant, runInput)
	must(t, e)
	replay, e := f.ps.CreateRun(ctx, f.owner, tenant, runInput)
	must(t, e)
	if !replay.Replayed || replay.ID != run.ID {
		t.Fatal("idempotency")
	}
	worked, e := f.ps.RunOne(ctx)
	must(t, e)
	if !worked {
		t.Fatal("worker did not claim")
	}
	detail, e := f.ps.Run(ctx, f.owner, tenant, run.ID)
	must(t, e)
	if detail.Run.State != "succeeded" {
		t.Fatalf("run %+v", detail)
	}
	result, _, e := f.ps.Result(ctx, f.owner, tenant, run.ID, "1", false, 0, 100)
	must(t, e)
	if result.Response == nil || len(result.Response.Records) != 1 {
		t.Fatalf("result %+v", result)
	}
	_, e = f.ps.CreateRun(ctx, f.owner, tenant, parse.RunInput{Meta: f.meta(t), ReplayRunID: run.ID})
	must(t, e)
	worked, e = f.ps.RunOne(ctx)
	must(t, e)
	if !worked {
		t.Fatal("replay not claimed")
	}
	_, e = f.ps.Sample(ctx, f.actor(), tenant, sample)
	if e == nil {
		t.Fatal("platform crossed tenant")
	}
	var n int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM parsing_runs`).Scan(&n))
	if n != 2 {
		t.Fatalf("runs=%d", n)
	}
}
