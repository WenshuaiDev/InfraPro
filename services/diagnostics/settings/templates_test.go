package settings_test

import (
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"os"
	"path/filepath"
	"testing"
)

func TestDeploymentExamplesUseStrictContracts(t *testing.T) {
	for _, profile := range []string{"native", "docker", "mixed"} {
		t.Run(profile, func(t *testing.T) {
			root := filepath.Join("../../../deploy/diagnostics", profile)
			read := func(name string, dst any) {
				t.Helper()
				raw, e := os.ReadFile(filepath.Join(root, name))
				if e != nil {
					t.Fatal(e)
				}
				if e = settings.Strict(raw, dst); e != nil {
					t.Fatal(name, e)
				}
			}
			var server settings.Server
			var manifest settings.Manifest
			var bindings settings.Bindings
			var auth settings.Auth
			var keys settings.Keyring
			read("server.json", &server)
			read("manifest.json", &manifest)
			read("bindings.json", &bindings)
			read("auth.json", &auth)
			read("node-keys.json", &keys)
			for _, e := range []error{server.Validate(false), manifest.Validate(settings.DefaultLimits()), bindings.Validate(manifest), auth.Validate(), keys.Validate(manifest, "local")} {
				if e != nil {
					t.Fatal(e)
				}
			}
			var split settings.Manifest
			read("split-manifest.json", &split)
			read("split-bindings.json", &bindings)
			if err := split.Validate(settings.DefaultLimits()); err != nil {
				t.Fatal("split manifest", err)
			}
			if err := bindings.Validate(split); err != nil {
				t.Fatal("split bindings", err)
			}
			if profile == "docker" {
				read("split-observer-edge-bindings.json", &bindings)
				if err := bindings.Validate(split); err != nil {
					t.Fatal("split native edge bindings", err)
				}
			}
			if profile != "native" {
				read("split-observer-bindings.json", &bindings)
				if err := bindings.Validate(split); err != nil {
					t.Fatal("split observer bindings", err)
				}
				var observer settings.Observer
				read("observer.json", &observer)
				read("observer-bindings.json", &bindings)
				if e := observer.Validate(); e != nil {
					t.Fatal(e)
				}
				if e := bindings.Validate(manifest); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}

func TestSplitReplicaBudgetsHaveSeparateProbes(t *testing.T) {
	for _, profile := range []string{"native", "docker", "mixed"} {
		t.Run(profile, func(t *testing.T) {
			roles := map[string][]string{"control-api": {"control-api-1", "control-api-2"}, "audit-worker": {"audit-worker-1", "audit-worker-2"}}
			options := settings.SplitOptions{BinDirectory: "/opt/infrapro/current/bin", ReportDirectory: "/var/run/reports", RuntimeUID: 10001,
				ReplicaSlots: roles, RoleUIDs: map[string]int{"control-api-2": 10012}}
			dir := filepath.Join(t.TempDir(), "split")
			server, err := settings.Init(dir, "quota-test", profile, "/opt/infrapro/current/bin/infrapro-diagnostics", options)
			if err != nil {
				t.Fatal(err)
			}
			if err = settings.ValidateFiles(server); err != nil {
				t.Fatal(err)
			}
			name := "bindings.json"
			if profile == "docker" {
				name = "observer-bindings.json"
			}
			var bindings settings.Bindings
			if _, err = settings.Load(filepath.Join(filepath.Dir(server), name), &bindings); err != nil {
				t.Fatal(err)
			}
			required := 0
			for _, b := range bindings.Bindings {
				for role, ids := range roles {
					for _, id := range ids {
						if b.ID != id+"-report" {
							continue
						}
						expectedUID := 10001
						if id == "control-api-2" {
							expectedUID = 10012
						}
						if b.ReportFile != filepath.Join(options.ReportDirectory, id, "process.json") || b.ExpectedReplicaID != id || b.ExpectedUID == nil || *b.ExpectedUID != expectedUID || filepath.Base(b.ExpectedExecutable) != role {
							t.Fatal("wrong replica identity", b.ID)
						}
						required++
					}
				}
			}
			if required != 4 {
				t.Fatal("missing independent replica probes", required)
			}
		})
	}
}

func TestSplitHandlesAssignedHostsAndBrokerQuorum(t *testing.T) {
	root := t.TempDir()
	options := settings.SplitOptions{BinDirectory: "/opt/infrapro/current/bin", RuntimeUID: 10001, BrokerReplicas: 3,
		ReplicaNodes: map[string]string{"ingest-relay-1": "edge", "nats-server-1": "msg-a", "nats-server-2": "msg-b", "nats-server-3": "msg-c"}}
	dir := filepath.Join(root, "multi")
	server, err := settings.Init(dir, "assigned", "native", "/opt/infrapro/current/bin/infrapro-diagnostics", options)
	if err != nil {
		t.Fatal(err)
	}
	if err = settings.ValidateFiles(server); err != nil {
		t.Fatal(err)
	}
	dir = filepath.Dir(server)
	var manifest settings.Manifest
	if _, err = settings.Load(filepath.Join(dir, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	for _, node := range []string{"edge", "msg-a", "msg-b", "msg-c"} {
		var b settings.Bindings
		if _, err := settings.Load(filepath.Join(dir, "observer-"+node+"-bindings.json"), &b); err != nil || b.Validate(manifest) != nil {
			t.Fatal(node, err)
		}
		if len(b.Bindings) < 2 {
			t.Fatal(node)
		}
	}
	for _, component := range manifest.Components {
		if component.ID == "nats-server" && component.MinReadyReplicas != 2 {
			t.Fatal(component)
		}
	}
	for _, item := range []struct{ replica, node string }{{"nats-server-2", "msg-a"}, {"ingest-worker-1", "surplus"}} {
		bad := options
		bad.ReplicaNodes = map[string]string{"nats-server-1": "msg-a", item.replica: item.node}
		if _, err = settings.Init(filepath.Join(root, item.node), "assigned", "native", "/opt/infrapro/current/bin/infrapro-diagnostics", bad); err == nil {
			t.Fatal("invalid assignment accepted", item)
		}
	}
}
