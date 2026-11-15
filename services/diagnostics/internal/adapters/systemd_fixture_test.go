package adapters

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"
)

type systemdFixture struct {
	mu    sync.Mutex
	state string
	calls []string
}

func (f *systemdFixture) GetUnit(unit string) (dbus.ObjectPath, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "GetUnit:"+unit)
	if f.state == "denied" {
		return "", dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", nil)
	}
	if f.state == "missing" {
		return "", dbus.NewError("org.freedesktop.systemd1.NoSuchUnit", nil)
	}
	return "/org/freedesktop/systemd1/unit/fixture_2eservice", nil
}
func (f *systemdFixture) Get(iface, property string) (dbus.Variant, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Get:"+iface+":"+property)
	values := map[string]any{"LoadState": "loaded", "ActiveState": f.state, "SubState": "running", "MainPID": uint32(0), "ExecMainCode": int32(0), "ExecMainStatus": int32(0), "Result": "success", "ExecMainStartTimestamp": uint64(0)}
	value, ok := values[property]
	if !ok {
		return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.UnknownProperty", nil)
	}
	return dbus.MakeVariant(value), nil
}
func TestPrivateDBusSystemdContract(t *testing.T) {
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("L4 runner supplies an isolated Linux dbus-daemon")
	}
	cmd := exec.Command(daemon, "--session", "--nofork", "--print-address=1")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	scanner := bufio.NewScanner(pipe)
	if !scanner.Scan() {
		t.Fatal("private bus address missing")
	}
	address := scanner.Text()
	owner, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err = owner.RequestName("org.freedesktop.systemd1", dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	fixture := &systemdFixture{}
	if err = owner.Export(fixture, "/org/freedesktop/systemd1", "org.freedesktop.systemd1.Manager"); err != nil {
		t.Fatal(err)
	}
	if err = owner.Export(fixture, "/org/freedesktop/systemd1/unit/fixture_2eservice", "org.freedesktop.DBus.Properties"); err != nil {
		t.Fatal(err)
	}
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	m := testfixture.Manifest()
	p := m.Probes[0]
	p.Adapter = "systemd_unit"
	r := New(m, settings.Bindings{}, uuid.NewString())
	r.Systemd = &dbusReader{conn}
	for _, c := range []struct{ state, presence, readiness, outcome string }{{"active", "running", "ready", "ok"}, {"activating", "running", "starting", "ok"}, {"failed", "stopped", "not_ready", "failed"}, {"missing", "missing", "unknown", "failed"}, {"denied", "unknown", "unknown", "permission_denied"}} {
		t.Run(c.state, func(t *testing.T) {
			fixture.mu.Lock()
			fixture.state = c.state
			fixture.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			o := r.Collect(ctx, p, settings.Binding{ID: "unit", Adapter: p.Adapter, UnitName: "fixture.service"})
			if o.Outcome != c.outcome || o.Facets.Presence != c.presence || o.Facets.Readiness != c.readiness {
				t.Fatalf("mapped %s: %+v", c.state, o)
			}
		})
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.calls) == 0 {
		t.Fatal("no D-Bus traffic")
	}
	for _, call := range fixture.calls {
		if !strings.HasPrefix(call, "GetUnit:") && !strings.HasPrefix(call, "Get:") {
			t.Fatal("non-read method", call)
		}
	}
	t.Logf("L4 private D-Bus contract: %d GetUnit/Get requests; real systemd units remain unverified", len(fixture.calls))
}
func TestLinuxContainerResourceCollection(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getenv("DIAGNOSTICS_LINUX_CONTAINER_TEST") != "1" {
		t.Skip("requires bounded L4 container")
	}
	m := testfixture.Manifest()
	p := m.Probes[0]
	p.Adapter = "host_resources"
	p.Facet = "resource"
	p.TargetKind = "node"
	p.TargetID = "node"
	r := New(m, settings.Bindings{}, uuid.NewString())
	b := settings.Binding{ID: "own-cgroup", Adapter: p.Adapter, Scope: "container", MountPaths: []settings.Mount{{ID: "lab", Path: "/tmp"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	first := r.Collect(ctx, p, b)
	time.Sleep(50 * time.Millisecond)
	second := r.Collect(ctx, p, b)
	found := map[string]bool{}
	for _, v := range first.Metrics {
		found[v.Key] = true
		if v.Key == "container.cpu_cores" && v.Value != nil {
			t.Fatal("first CPU sample fabricated")
		}
		if v.Key == "container.memory_limit_bytes" && v.Value != "268435456" {
			t.Fatalf("wrong actual memory limit: %+v", v)
		}
	}
	if !found["container.memory_limit_bytes"] || !found["disk.total_bytes"] || second.Evidence.Scope != "container" {
		t.Fatal("missing scoped resource evidence")
	}
	t.Log("Linux /proc + cgroup v2 + declared /tmp collection: 256 MiB limit, first CPU null, second sample available")
}
