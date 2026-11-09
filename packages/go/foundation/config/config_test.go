package config_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/config"
)

func TestEnvironmentOverridesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("http_addr: 127.0.0.1:8081\nlog_level: debug\nshutdown_timeout: 3s\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INFRAPRO_HTTP_ADDR", "127.0.0.1:9090")
	t.Setenv("INFRAPRO_LOG_LEVEL", "warn")
	t.Setenv("INFRAPRO_SHUTDOWN_TIMEOUT", "2s")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:9090" || cfg.LogLevel != slog.LevelWarn || cfg.ShutdownTimeout != 2*time.Second {
		t.Fatalf("environment overrides were not applied: %+v", cfg)
	}
}

func TestRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ name, key, value string }{
		{"missing port", "INFRAPRO_HTTP_ADDR", "127.0.0.1"},
		{"invalid port", "INFRAPRO_HTTP_ADDR", "127.0.0.1:70000"},
		{"invalid level", "INFRAPRO_LOG_LEVEL", "verbose"},
		{"invalid timeout", "INFRAPRO_SHUTDOWN_TIMEOUT", "0s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("INFRAPRO_HTTP_ADDR", "127.0.0.1:8080")
			t.Setenv("INFRAPRO_LOG_LEVEL", "info")
			t.Setenv("INFRAPRO_SHUTDOWN_TIMEOUT", "5s")
			t.Setenv(tc.key, tc.value)
			if _, err := config.Load(""); err == nil {
				t.Fatal("expected invalid configuration to fail before starting the server")
			}
		})
	}
}

func TestRejectsUnknownYAMLField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("http_adrr: 127.0.0.1:8080\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected a misspelled YAML field to fail")
	}
}
