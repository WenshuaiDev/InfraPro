package parsing_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	wire "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	parse "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func (f fixture) options(url string) parse.Options {
	o := parse.DefaultOptions()
	o.Enabled = true
	o.URL = url
	o.TokenFile = os.Getenv("INFRAPRO_PARSING_TOKEN_FILE")
	return o
}
func (f fixture) methodRun(t *testing.T, scope c.Scope, a access.Actor, sample, version string) parse.Result {
	r, e := f.ps.CreateRun(ctx, a, scope, parse.RunInput{Meta: f.meta(t), Kind: "method_test", SampleID: sample, MethodID: example, MethodVersion: version, Transport: "tcp"})
	must(t, e)
	return r
}
func (f fixture) checkRun(t *testing.T, scope c.Scope, a access.Actor, id, state, code string) {
	d, e := f.ps.Run(ctx, a, scope, id)
	must(t, e)
	if d.Run.State != state || d.Run.ErrorCode != code {
		t.Fatalf("state=%s code=%s; want %s/%s", d.Run.State, d.Run.ErrorCode, state, code)
	}
}
func (f fixture) setMethod(t *testing.T, version, state string) {
	ms, e := f.ps.Methods(ctx, f.actor(), c.PlatformScope(), c.Filter{})
	must(t, e)
	for _, m := range ms.Items {
		if m.Entry.MethodID == example && m.Entry.MethodVersion == version {
			_, e = f.ps.SetMethodState(ctx, f.actor(), example, version, parse.StateInput{Meta: f.meta(t), Revision: m.Revision, State: state})
			must(t, e)
			return
		}
	}
	t.Fatal("method absent")
}
func proxyParser(t *testing.T, fn func(http.ResponseWriter, *http.Request, []byte)) { t.Helper() }
func (f fixture) proxy(t *testing.T, transform func([]byte) ([]byte, int)) *httptest.Server {
	upstream := os.Getenv("INFRAPRO_PARSING_URL")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		request, e := http.NewRequestWithContext(r.Context(), r.Method, upstream+r.URL.Path, bytes.NewReader(b))
		if e != nil {
			return
		}
		request.Header = r.Header.Clone()
		resp, e := http.DefaultClient.Do(request)
		if e != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		code := resp.StatusCode
		if r.URL.Path == "/v1/parse" {
			body, code = transform(body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	f.ps.WithOptions(f.options(server.URL))
	return server
}
func TestCatalogImmutableAndUnavailable(t *testing.T) {
	f := setup(t)
	platform := c.PlatformScope()
	status, e := f.ps.CatalogStatus(ctx, f.actor())
	must(t, e)
	if status.State != "never_synced" || status.CreatedAt != nil {
		t.Fatalf("unexpected initial catalog status: %+v", status)
	}
	first, e := f.ps.Sync(ctx, f.actor(), f.meta(t))
	must(t, e)
	if first.ID == "" {
		t.Fatal("missing sync")
	}
	status, e = f.ps.CatalogStatus(ctx, f.actor())
	must(t, e)
	if status.ID != first.ID || status.State != "succeeded" || status.Added != 3 || status.CreatedAt == nil {
		t.Fatalf("catalog success not exposed: %+v", status)
	}
	ms, e := f.ps.Methods(ctx, f.actor(), platform, c.Filter{})
	must(t, e)
	for _, m := range ms.Items {
		if m.State != "pending" {
			t.Fatal("new version automatically enabled")
		}
	}
	_, e = f.ps.Sync(ctx, f.actor(), f.meta(t))
	must(t, e)
	var count int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM parsing_method_versions`).Scan(&count))
	if count != 3 {
		t.Fatal(count)
	}
	token, e := os.ReadFile(os.Getenv("INFRAPRO_PARSING_TOKEN_FILE"))
	must(t, e)
	req, _ := http.NewRequest("GET", os.Getenv("INFRAPRO_PARSING_URL")+"/v1/catalog", nil)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	resp, e := http.DefaultClient.Do(req)
	must(t, e)
	defer resp.Body.Close()
	b, e := io.ReadAll(resp.Body)
	must(t, e)
	var cat wire.Catalog
	must(t, json.Unmarshal(b, &cat))
	for _, mode := range []string{"different", "incomplete", "unsupported", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			next := cat
			next.Entries = append([]wire.Entry{}, cat.Entries...)
			status := 200
			switch mode {
			case "different":
				var m wire.Manifest
				raw, _ := json.Marshal(next.Entries[0].Manifest)
				must(t, json.Unmarshal(raw, &m))
				m.ReleaseNotes += " altered"
				raw, _ = json.Marshal(m)
				next.Entries[0].Manifest = m
				next.Entries[0].ManifestDigest = wire.Digest(raw)
				next.Entries[0].ManifestBase64 = encode(raw)
			case "incomplete":
				next.Entries = next.Entries[:1]
			case "unsupported":
				next.ContractVersion = 999
			case "unavailable":
				status = 503
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(next)
			}))
			defer server.Close()
			f.ps.WithOptions(f.options(server.URL))
			result, e := f.ps.Sync(ctx, f.actor(), f.meta(t))
			if e == nil && result.State != "failed" {
				t.Fatalf("bad catalog accepted: %+v", result)
			}
			last, e := f.ps.CatalogStatus(ctx, f.actor())
			must(t, e)
			if last.ID != result.ID || last.State != "failed" || last.ErrorCode == "" || last.Added != 0 {
				t.Fatalf("catalog failure not preserved: %+v", last)
			}
			var n int
			must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM parsing_method_versions`).Scan(&n))
			if n != 3 {
				t.Fatal("old catalog cleared")
			}
		})
	}
	f.ps.WithOptions(f.options(os.Getenv("INFRAPRO_PARSING_URL")))
}
func TestAdoptionTransactionsAndSemanticBoundaries(t *testing.T) {
	f := setup(t)
	f.sync(t)
	pScope, scope := c.PlatformScope(), c.TenantScope(f.tenant)
	m, e := f.s.CreateManufacturer(ctx, f.actor(), pScope, c.ManufacturerCreate{Meta: f.meta(t), Code: "fixed", Name: "模拟厂家"}, access.Meta{})
	must(t, e)
	product := f.product(t, pScope, f.actor(), m.ID, "sensor", []c.Slot{f.fixed(), {Key: "optional", Name: "可配置", Mode: "configurable", AllowedShapes: []string{"decimal"}}})
	cfg := f.config(t, product, "1.0.0")
	def, e := f.ps.SetDefault(ctx, f.actor(), pScope, product.ID, product.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: "0", ConfigRevisionID: cfg.ConfigRevisionID})
	must(t, e)
	create := func(code string, intent *devices.ParsingAdoptionIntent) string {
		v, e := f.b.Devices.Create(ctx, f.owner, scope, devices.DeviceCreate{Meta: f.meta(t), ProductVersionID: product.VersionID, Code: code, Name: "模拟设备", ParsingAdoption: intent}, access.Meta{})
		must(t, e)
		return v.ID
	}
	ids := []string{create("one", &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: def.Revision}), create("two", &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: def.Revision}), create("three", nil)}
	st, e := f.ps.Device(ctx, f.owner, scope, ids[2])
	must(t, e)
	if st.Mode != "unconfigured" || st.Revision != "0" {
		t.Fatal(st)
	}
	cfg2 := f.config(t, product, "1.1.0")
	def2, e := f.ps.SetDefault(ctx, f.actor(), pScope, product.ID, product.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: def.Revision, ConfigRevisionID: cfg2.ConfigRevisionID})
	must(t, e)
	_, e = f.b.Devices.Create(ctx, f.owner, scope, devices.DeviceCreate{Meta: f.meta(t), ProductVersionID: product.VersionID, Code: "stale", Name: "模拟设备", ParsingAdoption: &devices.ParsingAdoptionIntent{Mode: "inherit_default", DefaultRevision: def.Revision}}, access.Meta{})
	wantCode(t, e, "revision_conflict")
	for _, id := range ids[:2] {
		v, e := f.ps.Device(ctx, f.owner, scope, id)
		must(t, e)
		if v.Current.Snapshot.Method.Version != "1.0.0" || !v.DefaultChanged {
			t.Fatal("implicit migration")
		}
	}
	batch := parse.BatchCommand{Command: parse.Command{Meta: f.meta(t), Action: "upgrade_default", DefaultRevision: def2.Revision}, Devices: []parse.BatchDevice{{DeviceID: ids[0], Revision: "1"}, {DeviceID: ids[1], Revision: "999"}}}
	_, _, e = f.ps.Batch(ctx, f.owner, scope, batch, false)
	wantCode(t, e, "revision_conflict")
	v, e := f.ps.Device(ctx, f.owner, scope, ids[0])
	must(t, e)
	if v.Revision != "1" {
		t.Fatal("partial batch commit")
	}
	cmd := parse.Command{Meta: f.meta(t), Action: "upgrade_default", Revision: "1", DefaultRevision: def2.Revision}
	var errs [2]error
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := cmd
			if i == 1 {
				in.RequestID = uuidString()
			}
			_, errs[i] = f.ps.Command(ctx, f.owner, scope, ids[0], in)
		}(i)
	}
	wg.Wait()
	success := 0
	for _, e := range errs {
		if e == nil {
			success++
		} else {
			wantCode(t, e, "revision_conflict")
		}
	}
	if success != 1 {
		t.Fatal(errs)
	}
	v, e = f.ps.Device(ctx, f.owner, scope, ids[1])
	must(t, e)
	if v.Current.Snapshot.Method.Version != "1.0.0" {
		t.Fatal("unselected device changed")
	}
	history, e := f.ps.History(ctx, f.owner, scope, ids[0], c.Filter{})
	must(t, e)
	old := history.Items[len(history.Items)-1]
	_, e = f.ps.Command(ctx, f.owner, scope, ids[0], parse.Command{Meta: f.meta(t), Action: "rollback", Revision: "2", AdoptionID: old.ID})
	must(t, e)
	v, e = f.ps.Device(ctx, f.owner, scope, ids[0])
	must(t, e)
	if v.Current.SourceDefaultRevision == nil || *v.Current.SourceDefaultRevision != def.Revision {
		t.Fatal("rollback provenance changed")
	}
	override, e := f.ps.SaveConfig(ctx, f.owner, scope, product.ID, product.VersionID, ids[0], "", parse.ConfigInput{Meta: f.meta(t), Revision: "0", Name: "设备特化", MethodID: example, MethodVersion: "1.1.0", Bindings: []parse.Binding{{OutputKey: "displacement", SlotKey: "optional", Confirmed: true}}})
	must(t, e)
	_, e = f.ps.Command(ctx, f.owner, scope, ids[0], parse.Command{Meta: f.meta(t), Action: "override", Revision: "3", ConfigRevisionID: override.ConfigRevisionID})
	must(t, e)
	v, e = f.ps.Device(ctx, f.owner, scope, ids[0])
	must(t, e)
	if v.Mode != "override" || v.Mapping[0].Status != "output_unconfigured" {
		t.Fatalf("bad override %+v", v)
	}
	_, e = f.ps.Command(ctx, f.owner, scope, ids[0], parse.Command{Meta: f.meta(t), Action: "restore_inheritance", Revision: "4", DefaultRevision: def2.Revision})
	must(t, e)
	_, e = f.ps.Command(ctx, f.owner, scope, ids[0], parse.Command{Meta: f.meta(t), Action: "clear", Revision: "5"})
	must(t, e)
	_, e = f.ps.SetDefault(ctx, f.actor(), pScope, product.ID, product.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: def2.Revision})
	must(t, e)
	v, e = f.ps.Device(ctx, f.owner, scope, ids[1])
	must(t, e)
	if v.Current.Snapshot.Method.Version != "1.0.0" {
		t.Fatal("cleared default cleared device")
	}
	for _, bindings := range [][]parse.Binding{{{OutputKey: "displacement", SlotKey: "absent", Confirmed: true}}, {{OutputKey: "displacement", SlotKey: "x", Confirmed: true}, {OutputKey: "displacement", SlotKey: "optional", Confirmed: true}}, {{OutputKey: "displacement", SlotKey: "x", Confirmed: false}}} {
		_, e = f.ps.SaveConfig(ctx, f.actor(), pScope, product.ID, product.VersionID, "", "", parse.ConfigInput{Meta: f.meta(t), Revision: "0", Name: "invalid", MethodID: example, MethodVersion: "1.0.0", Bindings: bindings})
		if e == nil {
			t.Fatal("invalid mapping accepted")
		}
	}
	_, e = f.ps.SetDefault(ctx, f.owner, scope, product.ID, product.VersionID, parse.DefaultInput{Meta: f.meta(t), Revision: "0", ConfigRevisionID: cfg.ConfigRevisionID})
	if e == nil {
		t.Fatal("tenant changed public default")
	}
	for _, sql := range []string{`UPDATE parsing_config_revisions SET snapshot='{}'`, `UPDATE parsing_device_adoptions SET snapshot='{}' WHERE snapshot IS NOT NULL`, `DELETE FROM parsing_device_adoptions`} {
		_, e = f.env.Pool.Exec(ctx, sql)
		if e == nil {
			t.Fatal("mutable history", sql)
		}
	}
	f.setMethod(t, "1.0.0", "deprecated")
	_, e = f.ps.Command(ctx, f.owner, scope, ids[0], parse.Command{Meta: f.meta(t), Action: "rollback", Revision: "6", AdoptionID: old.ID})
	if e == nil {
		t.Fatal("deprecated rollback accepted")
	}
	sample := f.sample(t, scope, f.owner, "measurement.bin", "binary")
	r, e := f.ps.CreateRun(ctx, f.owner, scope, parse.RunInput{Meta: f.meta(t), Kind: "device_test", SampleID: sample, DeviceID: ids[1], Transport: "tcp"})
	must(t, e)
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	f.checkRun(t, scope, f.owner, r.ID, "succeeded", "")
}
func TestResponseValidationRetryRevocationAndFences(t *testing.T) {
	f := setup(t)
	f.sync(t)
	scope := c.TenantScope(f.tenant)
	sample := f.sample(t, scope, f.owner, "measurement.bin", "binary")
	t.Run("corrupt-response", func(t *testing.T) {
		f.proxy(t, func(b []byte) ([]byte, int) {
			var r wire.Response
			_ = json.Unmarshal(b, &r)
			r.InputSHA256 = strings.Repeat("0", 64)
			b, _ = json.Marshal(r)
			return b, 200
		})
		r := f.methodRun(t, scope, f.owner, sample, "1.0.0")
		_, e := f.ps.RunOne(ctx)
		must(t, e)
		f.checkRun(t, scope, f.owner, r.ID, "failed", "invalid_response")
		v, _, e := f.ps.Result(ctx, f.owner, scope, r.ID, "1", false, 0, 100)
		must(t, e)
		if v.State != "quarantined" {
			t.Fatal(v)
		}
	})
	t.Run("lost-response-retry", func(t *testing.T) {
		var n atomic.Int32
		f.proxy(t, func(b []byte) ([]byte, int) {
			if n.Add(1) < 3 {
				return []byte(`{"code":"unavailable"}`), 503
			}
			return b, 200
		})
		r := f.methodRun(t, scope, f.owner, sample, "1.0.0")
		_, e := f.ps.RunOne(ctx)
		must(t, e)
		f.checkRun(t, scope, f.owner, r.ID, "succeeded", "")
		d, e := f.ps.Run(ctx, f.owner, scope, r.ID)
		must(t, e)
		if len(d.Executions[0].Attempts) != 3 {
			t.Fatal(d.Executions)
		}
	})
	for _, action := range []string{"cancel", "block", "revoke", "lease"} {
		t.Run(action, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			f.proxy(t, func(b []byte) ([]byte, int) { close(entered); <-release; return b, 200 })
			r := f.methodRun(t, scope, f.owner, sample, "1.0.0")
			done := make(chan error, 1)
			go func() { _, e := f.ps.RunOne(ctx); done <- e }()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("parser not called")
			}
			switch action {
			case "cancel":
				d, e := f.ps.Run(ctx, f.owner, scope, r.ID)
				must(t, e)
				_, e = f.ps.TaskControl(ctx, f.owner, taskrecord.SourceRef{Module: "parsing", Kind: "run", RootID: r.ID}, tasks.Command{RequestID: f.meta(t).RequestID, Action: "cancel", ExpectedGeneration: "1", ExpectedRevision: d.Run.ControlRevision, Reason: "取消迟到结果"})
				must(t, e)
			case "block":
				f.setMethod(t, "1.0.0", "blocked")
			case "revoke":
				_, e := f.env.Pool.Exec(ctx, `UPDATE accounts SET status='disabled' WHERE id=$1`, f.owner.ID)
				must(t, e)
			case "lease":
				_, e := f.env.Pool.Exec(ctx, `UPDATE parsing_run_executions SET lease_until=clock_timestamp()-interval '1 second',fence=fence+1 WHERE run_id=$1`, r.ID)
				must(t, e)
			}
			close(release)
			must(t, <-done)
			if action == "revoke" {
				if _, e := f.ps.Run(ctx, f.owner, scope, r.ID); e == nil {
					t.Fatal("revoked account read old run")
				}
				if _, e := f.ps.SampleContent(ctx, f.owner, scope, sample, false); e == nil {
					t.Fatal("revoked account read raw")
				}
				_, e := f.env.Pool.Exec(ctx, `UPDATE accounts SET status='active' WHERE id=$1`, f.owner.ID)
				must(t, e)
			}
			switch action {
			case "cancel":
				f.checkRun(t, scope, f.owner, r.ID, "canceled", "canceled")
				d, e := f.ps.Run(ctx, f.owner, scope, r.ID)
				must(t, e)
				f.ps.WithOptions(f.options(os.Getenv("INFRAPRO_PARSING_URL")))
				_, e = f.ps.TaskControl(ctx, f.owner, taskrecord.SourceRef{Module: "parsing", Kind: "run", RootID: r.ID}, tasks.Command{RequestID: f.meta(t).RequestID, Action: "retry", ExpectedGeneration: "1", ExpectedRevision: d.Run.ControlRevision, Reason: "新代重试"})
				must(t, e)
				_, e = f.ps.RunOne(ctx)
				must(t, e)
				d, e = f.ps.Run(ctx, f.owner, scope, r.ID)
				must(t, e)
				if d.Run.Generation != "2" || len(d.Executions) != 2 {
					t.Fatal("retry overwrote generation")
				}
			case "block":
				f.checkRun(t, scope, f.owner, r.ID, "failed", "method_blocked")
				f.setMethod(t, "1.0.0", "enabled")
			case "revoke":
				f.checkRun(t, scope, f.owner, r.ID, "failed", "authorization_revoked")
			case "lease":
				var n int
				must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM parsing_run_executions WHERE run_id=$1 AND state='succeeded'`, r.ID).Scan(&n))
				if n != 0 {
					t.Fatal("stale fence committed")
				}
				f.ps.WithOptions(f.options(os.Getenv("INFRAPRO_PARSING_URL")))
				_, e := f.ps.RunOne(ctx)
				must(t, e)
				f.checkRun(t, scope, f.owner, r.ID, "succeeded", "")
			}
		})
	}
}
func TestTenantIsolationBudgetsAndRawPrivileges(t *testing.T) {
	f := setup(t)
	f.sync(t)
	scope := c.TenantScope(f.tenant)
	sample := f.sample(t, scope, f.owner, "measurement.bin", "binary")
	other := f.member(t, "SecondOwner")
	created, e := tenancy.New(f.env.Service.Access()).Create(ctx, f.actor(), tenancy.CreateInput{Fields: tenancy.Fields{Name: "第二租户", Code: "second"}, AdministratorID: other.Account.ID, RequestID: f.meta(t).RequestID, Reason: "隔离"}, access.Meta{})
	must(t, e)
	a2 := f.env.Service.AccessActor(other)
	scope2 := c.TenantScope(created.TenantID)
	sample2 := f.sample(t, scope2, a2, "measurement.hex", "hex")
	s1, e := f.ps.Sample(ctx, f.owner, scope, sample)
	must(t, e)
	s2, e := f.ps.Sample(ctx, a2, scope2, sample2)
	must(t, e)
	if s1.SHA256 != s2.SHA256 || s1.ObjectID == s2.ObjectID {
		t.Fatal("representation hash/private storage")
	}
	_, e = f.ps.Sample(ctx, a2, scope2, sample)
	if e == nil {
		t.Fatal("cross tenant sample")
	}
	r := f.methodRun(t, scope, f.owner, sample, "1.0.0")
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	_, e = f.env.Pool.Exec(ctx, `DELETE FROM role_permissions WHERE permission_code=$1`, access.TenantParsingSampleRead)
	must(t, e)
	v, _, e := f.ps.Result(ctx, f.owner, scope, r.ID, "1", false, 0, 100)
	must(t, e)
	if v.Response == nil {
		t.Fatal("structured result hidden")
	}
	for _, field := range v.Response.Fields {
		if field.RawText != "" {
			t.Fatal("raw field leaked")
		}
	}
	for _, record := range v.Response.Records {
		for _, m := range record.Measurements {
			if m.RawText != "" {
				t.Fatal("raw measurement leaked")
			}
		}
	}
	_, _, e = f.ps.Result(ctx, f.owner, scope, r.ID, "1", true, 0, 100)
	if e == nil {
		t.Fatal("raw response leaked")
	}
	_, e = f.ps.SampleContent(ctx, f.owner, scope, sample, false)
	if e == nil {
		t.Fatal("raw sample leaked")
	}
	o := f.options(os.Getenv("INFRAPRO_PARSING_URL"))
	o.Queue = 1
	f.ps.WithOptions(o)
	queued := f.methodRun(t, scope, f.owner, sample, "1.0.0")
	_, e = f.ps.CreateRun(ctx, f.owner, scope, parse.RunInput{Meta: f.meta(t), Kind: "method_test", SampleID: sample, MethodID: example, MethodVersion: "1.0.0", Transport: "tcp"})
	wantCode(t, e, "queue_budget_exceeded")
	f.setMethod(t, "1.0.0", "blocked")
	_, e = f.ps.RunOne(ctx)
	must(t, e)
	f.checkRun(t, scope, f.owner, queued.ID, "failed", "method_blocked")
	o.StorageBudget = 1
	f.ps.WithOptions(o)
	_, e = f.ps.CreateSample(ctx, f.owner, scope, parse.SampleInput{Meta: f.meta(t), Representation: "binary", Source: "synthetic", ReceivedAt: time.Now().UTC()})
	wantCode(t, e, "storage_budget_exceeded")
	var n int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM task_records WHERE source_module='parsing' AND scope_kind='project'`).Scan(&n))
	if n != 0 {
		t.Fatal("project task")
	}
}

func encode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
func uuidString() string     { return uuid.NewString() }
