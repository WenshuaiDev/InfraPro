package settings_test

import (
	"encoding/json"
	"testing"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/testfixture"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

func TestManifestRejectsAmbiguityAndInvalidAssignments(t *testing.T) {
	valid := testfixture.Manifest()
	if e := valid.Validate(settings.DefaultLimits()); e != nil {
		t.Fatal(e)
	}
	cases := map[string]func(*settings.Manifest){"duplicate": func(m *settings.Manifest) { m.Nodes = append(m.Nodes, m.Nodes[0]) }, "dangling": func(m *settings.Manifest) { m.Replicas[0].NodeID = "missing" }, "fast": func(m *settings.Manifest) { m.Probes[0].IntervalMS = 999 }, "stale": func(m *settings.Manifest) { m.Probes[0].StaleAfterMS = 14000 }, "unknown adapter": func(m *settings.Manifest) { m.Probes[0].Adapter = "exec" }, "foreign required": func(m *settings.Manifest) { m.Replicas[0].RequiredProbeIDs = []string{"other"} }, "parent cycle": func(m *settings.Manifest) { m.Nodes[0].ParentNodeID = m.Nodes[0].ID }, "vantage": func(m *settings.Manifest) {
		m.Dependencies = append(m.Dependencies, settings.Dependency{ID: "pg", FromReplicaID: "api-1", ToComponentID: "api", ProbeID: "presence", EvidenceMode: "self_report"})
	}}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := testfixture.Manifest()
			mutate(&m)
			if m.Validate(settings.DefaultLimits()) == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
}
func TestStrictInputAndFixedBindingUnion(t *testing.T) {
	for _, raw := range []string{`{"schema_version":1,"schema_version":1}`, `{"schema_version":1,"unknown":1}`, `{} {}`, `{"schema_version":null}`} {
		var m settings.Manifest
		e := settings.Strict([]byte(raw), &m)
		if e == nil && m.Validate(settings.DefaultLimits()) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, raw := range []string{`{"id":"x","adapter":"tcp","command":"sh"}`, `{"id":"x","adapter":"exec"}`, `{"id":"x","adapter":"http_live","socket_path":"/tmp/x"}`} {
		var b settings.Binding
		if settings.Strict([]byte(raw), &b) == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	var b settings.Binding
	if settings.Strict([]byte(`{"id":"redis","adapter":"redis","address":"127.0.0.1:6379","database":0,"tls":false}`), &b) != nil {
		t.Fatal("valid redis rejected")
	}
	raw, _ := json.Marshal(b)
	var values map[string]any
	_ = json.Unmarshal(raw, &values)
	if values["database"] != float64(0) {
		t.Fatal("redis database not integer")
	}
}
func TestSecretsAndTargetBoundaries(t *testing.T) {
	hash, e := settings.PasswordHash("test-password-1234")
	if e != nil || !settings.VerifyPassword("test-password-1234", hash) || settings.VerifyPassword("wrong-password-1234", hash) {
		t.Fatal("password verification")
	}
	if settings.VerifyPassword("x", "$argon2id$v=19$m=999999999,t=99,p=99$bad$bad") {
		t.Fatal("unbounded hash accepted")
	}
	for _, raw := range []string{"http://169.254.169.254/", "http://user:secret@localhost/", "http://localhost/path?token=secret", "javascript:alert(1)", "http://[fe80::1]/"} {
		if _, e := settings.TargetURL(raw, nil); e == nil {
			t.Fatalf("unsafe target %s", raw)
		}
	}
	if settings.Origin("http://192.168.1.1", true) || settings.Origin("https://host/path", true) || !settings.Origin("http://127.0.0.1:8096", true) {
		t.Fatal("origin validation")
	}
}
