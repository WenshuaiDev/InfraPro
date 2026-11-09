package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func baseEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"INFRAPRO_DATABASE_URL": "postgres://u:p@127.0.0.1/infrapro_test", "INFRAPRO_REDIS_URL": "redis://:p@127.0.0.1:6379/0", "INFRAPRO_SUPPORT_CONTACT": "维护员 8000", "INFRAPRO_CSRF_AUTH_KEY": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	} {
		t.Setenv(k, v)
	}
}
func TestConfigOverridesAndValidation(t *testing.T) {
	baseEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if e := os.WriteFile(path, []byte("http_addr: 127.0.0.1:8888\nsession_idle_timeout: 1h\ninvitation_limit: '3'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("INFRAPRO_SESSION_IDLE_TIMEOUT", "15m")
	c, e := Load(path)
	if e != nil || c.SessionIdle != 15*time.Minute || c.InvitationLimit != 3 || c.HTTPAddr != "127.0.0.1:8888" {
		t.Fatal("precedence mismatch", e)
	}
	for _, tc := range []struct{ k, v string }{
		{"INFRAPRO_CSRF_AUTH_KEY", "short"}, {"INFRAPRO_SUPPORT_CONTACT", ""}, {"INFRAPRO_COOKIE_SECURE", "true"}, {"INFRAPRO_TRUSTED_ORIGINS", "http://127.0.0.1:5173/path"}, {"INFRAPRO_PUBLIC_CONSOLE_URL", "http://another-host:5173/"}, {"INFRAPRO_REDIS_KEY_PREFIX", "infrapro:*:identity:v1:"}, {"INFRAPRO_SESSION_IDLE_TIMEOUT", "0s"}, {"INFRAPRO_TRUSTED_PROXIES", "*"},
	} {
		t.Run(tc.k, func(t *testing.T) {
			t.Setenv(tc.k, tc.v)
			if _, e := Load(""); e == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
	if e = os.WriteFile(path, []byte("typo_secret: unknown\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Load(path); e == nil {
		t.Fatal("unknown YAML accepted")
	}
}

func TestRetiredScreenConfigurationIsIgnored(t *testing.T) {
	baseEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("public_screen_url: obsolete-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INFRAPRO_PUBLIC_SCREEN_URL", "also-obsolete")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("legacy configuration must remain readable: %v", err)
	}
	if cfg.PublicConsoleURL != "http://127.0.0.1:5173/" || len(cfg.TrustedOrigins) != 1 || cfg.TrustedOrigins[0] != "http://127.0.0.1:5173" {
		t.Fatal("the retired app changed the active console configuration")
	}
}

func TestDatabaseMaintenanceOnlyRequiresDatabase(t *testing.T) {
	t.Setenv("INFRAPRO_DATABASE_URL", "postgres://u:p@127.0.0.1/infrapro_test")
	t.Setenv("INFRAPRO_REDIS_URL", "")
	t.Setenv("INFRAPRO_CSRF_AUTH_KEY", "")
	t.Setenv("INFRAPRO_SUPPORT_CONTACT", "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("database_url: postgres://u:p@127.0.0.1/other_test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadDatabase(path)
	if err != nil || cfg.URL != os.Getenv("INFRAPRO_DATABASE_URL") {
		t.Fatalf("database-only config/override failed: %v", err)
	}
	for _, value := range []string{"", "http://127.0.0.1/database", "postgres:///database"} {
		t.Setenv("INFRAPRO_DATABASE_URL", value)
		if _, err = LoadDatabase(""); err == nil {
			t.Fatal("invalid database configuration accepted")
		}
	}
}
