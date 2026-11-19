package adapters

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/process"
)

func TestHostResourceCounterResetAndDeclaredFilesystemScope(t *testing.T) {
	m := testfixture.Manifest()
	p := m.Probes[0]
	p.Adapter, p.Facet, p.TargetKind, p.TargetID = "host_resources", "resource", "node", "node"
	r := New(m, settings.Bindings{}, uuid.NewString())
	root := testfixture.Directory(t)
	b := settings.Binding{ID: "resources", Adapter: p.Adapter, Scope: "host", MountPaths: []settings.Mount{{ID: "first", Path: root}, {ID: "same-filesystem", Path: root}, {ID: "missing", Path: filepath.Join(root, "not-present")}}, NetworkInterfaces: []settings.NetworkInterface{{ID: "not-present", Name: "diagnostics-nonexistent"}}}
	first := r.Collect(context.Background(), p, b)
	assertMissingCPU := func(metrics []model.Metric) {
		t.Helper()
		for _, v := range metrics {
			if v.Key == "cpu.utilization_ratio" {
				if v.Value != nil {
					t.Fatal("counter baseline fabricated a CPU ratio")
				}
				return
			}
		}
		t.Fatal("CPU availability missing")
	}
	assertMissingCPU(first.Metrics)
	// A prior counter above the real OS counter models a reset/wrap. The
	// resulting public sample must be null rather than negative or zero.
	r.samples[p.ID+":cpu"] = sample{at: time.Now().Add(-time.Second), identity: r.BootID, cpu: math.MaxFloat64, total: math.MaxFloat64}
	assertMissingCPU(r.Collect(context.Background(), p, b).Metrics)
	for _, v := range first.Metrics {
		if v.DimensionID == "same-filesystem" {
			t.Fatal("same filesystem counted twice")
		}
		if v.DimensionID == "missing" || v.DimensionID == "not-present" {
			if v.Value != nil || v.Availability == "available" {
				t.Fatal("unavailable resource represented as a value")
			}
		}
	}
}

func fixtureReport(t *testing.T) (*Runtime, settings.Probe, settings.Binding, diagnosticreport.Report) {
	t.Helper()
	m := testfixture.Manifest()
	root := testfixture.Directory(t)
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	exe, e = filepath.EvalSymlinks(exe)
	if e != nil {
		t.Fatal(e)
	}
	uid := os.Getuid()
	b := settings.Binding{ID: "report", Adapter: "self_report", ReportFile: filepath.Join(root, "report.json"), AllowedRoot: root, ExpectedExecutable: exe, ExpectedUID: &uid, ExpectedReplicaID: "api-1", PIDNamespace: "host"}
	proc, e := process.NewProcess(int32(os.Getpid()))
	if e != nil {
		t.Fatal(e)
	}
	created, e := proc.CreateTime()
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	v := diagnosticreport.Report{SchemaVersion: 1, InstanceID: "lab", ReplicaID: "api-1", NodeID: "node", Process: diagnosticreport.Identity{BootID: uuid.NewString(), PID: int32(os.Getpid()), OSStartTimeMS: strconv.FormatInt(created, 10), PIDNamespace: "host", BuildID: "dev"}, ReportSeq: "1", EmittedAt: now, UptimeMS: "1", Phase: "ready", Lanes: []diagnosticreport.Lane{}, Dependencies: []diagnosticreport.DependencyState{}}
	return New(m, settings.Bindings{Bindings: []settings.Binding{b}}, uuid.NewString()), m.Probes[1], b, v
}
func TestNativeIdentityAndSourceFreshness(t *testing.T) {
	r, p, b, report := fixtureReport(t)
	if e := settings.Save(b.ReportFile, report, true); e != nil {
		t.Fatal(e)
	}
	o := r.Collect(context.Background(), p, b)
	if o.Outcome != "ok" || o.Facets.Presence != "running" {
		t.Fatalf("unexpected %s %s", o.Outcome, o.ReasonCode)
	}
	again := r.Collect(context.Background(), p, b)
	if *o.AttemptID != *again.AttemptID || !o.ObservedAt.Equal(*again.ObservedAt) {
		t.Fatal("reading changed source attempt")
	}
	wrongExecutable := b
	wrongExecutable.ExpectedExecutable = "/not-the-owning-process"
	if got := r.Collect(context.Background(), p, wrongExecutable); got.ReasonCode != "identity_mismatch" || got.Facets.Presence != "unknown" {
		t.Fatal("different executable accepted")
	}
	wrongUID := b
	uid := os.Getuid() + 1
	wrongUID.ExpectedUID = &uid
	if got := r.Collect(context.Background(), p, wrongUID); got.Outcome == "ok" || got.Facets.Presence != "unknown" || !settings.OneOf(got.ReasonCode, "identity_mismatch", "permission_denied") {
		t.Fatal("different UID accepted")
	}
	report.Process.OSStartTimeMS = "1"
	if e := settings.Save(b.ReportFile, report, false); e != nil {
		t.Fatal(e)
	}
	o = r.Collect(context.Background(), p, b)
	if o.ReasonCode != "identity_mismatch" || o.Facets.Presence != "unknown" {
		t.Fatal("PID reuse accepted")
	}
	if e := os.Remove(b.ReportFile); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink("/etc/passwd", b.ReportFile); e != nil {
		t.Fatal(e)
	}
	o = r.Collect(context.Background(), p, b)
	if o.Outcome == "ok" {
		t.Fatal("symlink report accepted")
	}
	if e := os.Remove(b.ReportFile); e != nil {
		t.Fatal(e)
	}
	o = r.Collect(context.Background(), p, b)
	if o.ReasonCode != "process_locator_missing" || o.Facets.Presence != "unknown" {
		t.Fatal("missing locator claimed stopped")
	}
}
func TestLoopDeadlineUsesFreshSourceReportNotReadWallClock(t *testing.T) {
	r, p, b, report := fixtureReport(t)
	now := report.EmittedAt
	deadline := now.Add(-time.Second)
	attempt := now.Add(-time.Minute)
	report.Lanes = []diagnosticreport.Lane{{ID: "jobs", State: "working", LastAttemptAt: &attempt, ActiveSince: &attempt, DeadlineAt: &deadline, CheckGraceMS: 15000}}
	if e := settings.Save(b.ReportFile, report, true); e != nil {
		t.Fatal(e)
	}
	o := r.Collect(context.Background(), p, b)
	if o.Facets.Loop != "stalled" {
		t.Fatal("expired real deadline not stalled")
	}
	deadline = now.Add(time.Hour)
	report.Lanes[0].DeadlineAt = &deadline
	report.ReportSeq = "2"
	_ = settings.Save(b.ReportFile, report, false)
	o = r.Collect(context.Background(), p, b)
	if o.Facets.Loop != "working" {
		t.Fatal("long task marked stalled")
	}
	next := now.Add(24 * time.Hour)
	report.Lanes[0] = diagnosticreport.Lane{ID: "maintenance", State: "idle", LastAttemptAt: &attempt, LastSuccessAt: &attempt, NextAttemptAt: &next, CheckGraceMS: 15000}
	report.ReportSeq = "3"
	_ = settings.Save(b.ReportFile, report, false)
	o = r.Collect(context.Background(), p, b)
	if o.Facets.Loop != "idle" {
		t.Fatal("daily maintenance marked stalled")
	}
	// Reading a cached report whose deadline is in our past but after emitted_at
	// cannot invent a stall. Source freshness, handled by state, will expire it.
	report.EmittedAt = now.Add(-time.Minute)
	deadline = now.Add(-time.Second)
	report.Lanes[0] = diagnosticreport.Lane{ID: "jobs", State: "working", LastAttemptAt: &report.EmittedAt, ActiveSince: &report.EmittedAt, DeadlineAt: &deadline, CheckGraceMS: 15000}
	report.ReportSeq = "4"
	_ = settings.Save(b.ReportFile, report, false)
	o = r.Collect(context.Background(), p, b)
	if o.Facets.Loop != "working" || *o.SampleAgeMS < 59000 {
		t.Fatal("read time invented stall")
	}
}
func TestHTTPRedirectBodyLimitAndSecretRedaction(t *testing.T) {
	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; _, _ = w.Write([]byte("SECRET")) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, target.URL, 302)
		case "/huge":
			_, _ = w.Write([]byte(strings.Repeat("SECRET", 2000)))
		default:
			w.WriteHeader(500)
			_, _ = w.Write([]byte("SECRET-password"))
		}
	}))
	defer source.Close()
	m := testfixture.Manifest()
	p := m.Probes[0]
	p.Adapter = "http_ready"
	r := New(m, settings.Bindings{}, uuid.NewString())
	for _, path := range []string{"/redirect", "/huge", "/failed"} {
		o := r.Collect(context.Background(), p, settings.Binding{ID: "http", Adapter: "http_ready", URL: source.URL + path, ExpectedStatus: 200})
		raw, _ := json.Marshal(o)
		if o.Outcome == "ok" || strings.Contains(string(raw), "SECRET") {
			t.Fatal("unsafe http outcome")
		}
	}
	if hits != 0 {
		t.Fatal("followed redirect")
	}
}
func TestDockerReadAllowlistAndCardinality(t *testing.T) {
	root := testfixture.Directory(t)
	socket := filepath.Join(root, "docker.sock")
	listener, e := net.Listen("unix", socket)
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	id := strings.Repeat("a", 64)
	labels := map[string]string{"io.infrapro.instance": "lab", "io.infrapro.replica": "api-1"}
	var mu sync.Mutex
	paths := []string{}
	count := 1
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		paths = append(paths, request.Method+" "+request.URL.Path)
		n := count
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if request.Method != "GET" {
			t.Error("mutation request")
		}
		switch request.URL.Path {
		case "/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": "1.57", "MinAPIVersion": "1.41"})
		case "/v1.56/containers/json":
			list := []any{}
			for i := 0; i < n; i++ {
				list = append(list, map[string]any{"Id": id, "Labels": labels})
			}
			_ = json.NewEncoder(w).Encode(list)
		case "/v1.56/containers/" + id + "/json":
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Config": map[string]any{"Labels": labels, "Env": []string{"SECRET=value"}}, "State": map[string]any{"Running": true, "StartedAt": time.Now().UTC().Format(time.RFC3339Nano)}, "HostConfig": map[string]any{}, "RestartCount": 2})
		case "/v1.56/containers/" + id + "/stats":
			_, _ = w.Write([]byte(`{"cpu_stats":{"cpu_usage":{"total_usage":100}},"memory_stats":{"usage":1000}}`))
		default:
			t.Error("non-allowlisted request")
			w.WriteHeader(404)
		}
	})}
	go server.Serve(listener)
	defer server.Close()
	m := testfixture.Manifest()
	p := m.Probes[0]
	p.Adapter = "docker_container"
	r := New(m, settings.Bindings{}, uuid.NewString())
	b := settings.Binding{ID: "docker", Adapter: "docker_container", SocketPath: socket, SelectorLabels: labels, ExpectedReplicaID: "api-1", StatsEnabled: true}
	o := r.Collect(context.Background(), p, b)
	if o.Outcome != "ok" || o.Evidence.Health != "none" {
		t.Fatal(o.ReasonCode)
	}
	raw, _ := json.Marshal(o)
	if strings.Contains(string(raw), "SECRET") {
		t.Fatal("inspect leaked environment")
	}
	for _, metric := range o.Metrics {
		if metric.Key == "container.memory_limit_bytes" && (metric.Value != nil || metric.Availability != "unlimited") {
			t.Fatal("unlimited mapped to zero")
		}
	}
	for _, n := range []int{0, 2} {
		mu.Lock()
		count = n
		mu.Unlock()
		o = r.Collect(context.Background(), p, b)
		want := "process_not_found"
		if n == 2 {
			want = "ambiguous_identity"
		}
		if o.ReasonCode != want {
			t.Fatalf("%d: %s", n, o.ReasonCode)
		}
	}
}

func TestSplitMetricsPrecisePositionAndRejectPrivateDimensions(t *testing.T) {
	r, p, b, report := fixtureReport(t)
	report.Metrics = []diagnosticreport.ProcessMetric{
		{Key: "event.consumer_position", Value: 1<<54 + 1},
		{Key: "event.outbox_oldest_ms", Value: 800},
		{Key: "tenant.queue_depth", Dimension: "tenant-0123456789abcdef01234567", Value: 2},
	}
	if err := settings.Save(b.ReportFile, report, true); err != nil {
		t.Fatal(err)
	}
	o := r.Collect(context.Background(), p, b)
	if err := model.Validate(o, p, r.BootID); err != nil || o.Outcome != "ok" || len(o.Metrics) != 3 {
		t.Fatalf("valid metrics rejected: %s %v", o.ReasonCode, err)
	}
	if o.Metrics[0].Value != "18014398509481985" || o.Metrics[1].Value != "800" || o.Metrics[2].Value != float64(2) {
		t.Fatal("precision or unit mismatch", o.Metrics)
	}
	for _, bad := range []diagnosticreport.ProcessMetric{{Key: "tenant.queue_depth", Dimension: uuid.NewString(), Value: 1}, {Key: "event.payload", Value: 1}, report.Metrics[0]} {
		report.Metrics = append(report.Metrics, bad)
		if err := settings.Save(b.ReportFile, report, false); err != nil {
			t.Fatal(err)
		}
		o := r.Collect(context.Background(), p, b)
		if o.ReasonCode != "report_invalid" || len(o.Metrics) > 0 {
			t.Fatal("invalid metric exposed", o.ReasonCode)
		}
		report.Metrics = report.Metrics[:len(report.Metrics)-1]
	}
}

func TestProxyListenerDistinguishesTCPFromBusinessReadiness(t *testing.T) {
	m := testfixture.Manifest()
	p := m.Probes[0]
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	r := New(m, settings.Bindings{}, uuid.NewString())
	for _, pair := range []struct{ adapter, readiness string }{{"tcp", "unknown"}, {"proxy_listener", "ready"}} {
		p.Adapter = pair.adapter
		b := settings.Binding{ID: p.ID, Adapter: p.Adapter, Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port}
		v := r.Collect(t.Context(), p, b)
		if v.Outcome != "ok" || v.Facets.Readiness != pair.readiness {
			t.Fatal(pair.adapter, v.Outcome, v.Facets.Readiness)
		}
	}
}

func TestJetStreamProbeChecksOnlyLocalReadiness(t *testing.T) {
	m := testfixture.Manifest()
	p := m.Probes[0]
	p.Adapter = "jetstream_health"
	p.Facet = "readiness"
	served := "ok"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" || req.URL.Path != "/healthz" || req.URL.RawQuery != "js-enabled-only=true" || req.Header.Get("Authorization") != "" {
			t.Errorf("unexpected probe %s %s", req.Method, req.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": served})
	}))
	defer s.Close()
	_, portText, e := net.SplitHostPort(s.Listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	port, e := strconv.Atoi(portText)
	if e != nil {
		t.Fatal(e)
	}
	b := settings.Binding{ID: p.ID, Adapter: p.Adapter, Host: "127.0.0.1", Port: port}
	r := New(m, settings.Bindings{}, uuid.NewString())
	if got := r.Collect(context.Background(), p, b); got.Outcome != "ok" || got.Facets.Readiness != "ready" {
		t.Fatal("healthy JetStream", got)
	}
	served = "error"
	if got := r.Collect(context.Background(), p, b); got.ReasonCode != "response_mismatch" || got.Facets.Readiness != "not_ready" {
		t.Fatal("health JSON ignored", got)
	}
}

func TestContainerReportRequiresRuntimeIdentity(t *testing.T) {
	ts := time.Now().Truncate(time.Millisecond)
	id := 10001
	b := settings.Binding{ExpectedUID: &id, ExpectedExecutable: "/usr/local/bin/files-checker", AllowedRoot: "/srv/checker", ReportFile: "/srv/checker/process.json"}
	good := dockerInspect{Path: b.ExpectedExecutable}
	good.Config.User = "10001:10001"
	good.State.Running = true
	good.State.StartedAt = ts
	mounts := []byte(`[{"Source":"/srv/checker","Destination":"/run/infrapro-report","RW":true}]`)
	if err := json.Unmarshal(mounts, &good.Mounts); err != nil {
		t.Fatal(err)
	}
	report := diagnosticreport.Report{EmittedAt: ts.Add(time.Second), Process: diagnosticreport.Identity{PID: 1, PIDNamespace: "container", OSStartTimeMS: strconv.FormatInt(ts.Add(-500*time.Millisecond).UnixMilli(), 10)}}
	if !matchesReportIdentity(good, b, report) {
		t.Fatal("current attested process rejected")
	}
	for _, change := range []func(*dockerInspect, *diagnosticreport.Report){
		func(i *dockerInspect, _ *diagnosticreport.Report) { i.Path = "/usr/local/bin/another" },
		func(i *dockerInspect, _ *diagnosticreport.Report) { i.Config.User = "10002" },
		func(i *dockerInspect, _ *diagnosticreport.Report) { i.HostConfig.PidMode = "host" },
		func(i *dockerInspect, _ *diagnosticreport.Report) { enable := true; i.HostConfig.Init = &enable },
		func(i *dockerInspect, _ *diagnosticreport.Report) { i.State.Running = false },
		func(i *dockerInspect, _ *diagnosticreport.Report) { i.Mounts = nil },
		func(_ *dockerInspect, r *diagnosticreport.Report) { r.Process.PID = 7 },
		func(_ *dockerInspect, r *diagnosticreport.Report) { r.EmittedAt = ts.Add(-time.Second) },
		func(_ *dockerInspect, r *diagnosticreport.Report) {
			r.Process.OSStartTimeMS = strconv.FormatInt(ts.Add(-10*time.Second).UnixMilli(), 10)
		},
	} {
		i, r := good, report
		change(&i, &r)
		if matchesReportIdentity(i, b, r) {
			t.Fatal("wrong container accepted")
		}
	}
}
