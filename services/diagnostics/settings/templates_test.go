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
			if profile != "native" {
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
