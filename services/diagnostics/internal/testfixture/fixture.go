package testfixture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

func Directory(t testing.TB) string {
	t.Helper()
	dir, e := os.MkdirTemp("", "ipd-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	p, e := filepath.EvalSymlinks(dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(p, 0700); e != nil {
		t.Fatal(e)
	}
	return p
}
func Manifest() settings.Manifest {
	return settings.Manifest{SchemaVersion: 1, InstanceID: "lab", Revision: "1", DisplayName: "Diagnostic lab", Nodes: []settings.Node{{ID: "node", DisplayName: "Logical node", Kind: "logical_test", ResourceScope: "none", AssignmentRevision: "1", Enabled: true}}, Components: []settings.Component{{ID: "api", DisplayName: "API", Kind: "application", Expectation: "continuous", MinReadyReplicas: 1}}, Replicas: []settings.Replica{{ID: "api-1", ComponentID: "api", NodeID: "node", Expectation: "continuous", RequiredProbeIDs: []string{"presence", "report"}}}, Endpoints: []settings.Endpoint{}, Dependencies: []settings.Dependency{}, Probes: []settings.Probe{{ID: "presence", NodeID: "node", TargetKind: "replica", TargetID: "api-1", Adapter: "native_process", BindingID: "native", Facet: "presence", Required: true, IntervalMS: 5000, TimeoutMS: 1000, StaleAfterMS: 20000}, {ID: "report", NodeID: "node", TargetKind: "replica", TargetID: "api-1", Adapter: "self_report", BindingID: "report", Facet: "loop", Required: true, IntervalMS: 5000, TimeoutMS: 1000, StaleAfterMS: 20000}}}
}
func Keyring() (settings.Keyring, string) {
	secret := settings.NewSecret()
	return settings.Keyring{SchemaVersion: 1, InstanceID: "lab", Revision: "1", Keys: []settings.NodeKey{{KeyID: "key-one", NodeID: "node", Generation: "1", SecretSHA256: settings.Digest([]byte(secret)), Enabled: true, AssignmentRevision: "1"}}}, "key-one." + secret
}
