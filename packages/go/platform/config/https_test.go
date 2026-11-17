package config

import "testing"

func httpsEnv(t *testing.T) {
	t.Helper()
	baseEnv(t)
	for key, value := range map[string]string{
		"PUBLIC_CONSOLE_URL":     "https://10.23.52.152:18443/console/",
		"TRUSTED_ORIGINS":        "https://10.23.52.152:18443",
		"COOKIE_SECURE":          "true",
		"DISPLAY_RUNTIME_ORIGIN": "https://10.23.52.152:18444",
		"DISPLAY_ASSET_ORIGIN":   "https://10.23.52.152:18445",
		"TRUSTED_PROXIES":        "127.0.0.1, ::1",
	} {
		t.Setenv("INFRAPRO_"+key, value)
	}
}

func TestHTTPSConfiguration(t *testing.T) {
	httpsEnv(t)
	c, err := Load("")
	if err != nil || !c.CookieSecure || c.TrustedProxies[1] != "::1" || c.Display.RuntimeAddr != "127.0.0.1:5180" {
		t.Fatalf("HTTPS external origins with HTTP loopback backend: %v", err)
	}
	for _, tc := range []struct{ key, value string }{
		{"COOKIE_SECURE", "false"}, {"COOKIE_SECURE", "invalid"},
		{"PUBLIC_CONSOLE_URL", "http://10.23.52.152:18443/console/"},
		{"TRUSTED_ORIGINS", "https://10.23.52.152:18443,http://extra.example"},
		{"TRUSTED_ORIGINS", "https://10.23.52.152:18444"},
		{"TRUSTED_ORIGINS", "https://10.23.52.152:18443,https://10.23.52.152:18444"},
		{"TRUSTED_ORIGINS", "https://10.23.52.152:18443,https://10.23.52.152:18445"},
		{"DISPLAY_RUNTIME_ORIGIN", "http://10.23.52.152:18444"},
		{"DISPLAY_ASSET_ORIGIN", "http://10.23.52.152:18445"},
		{"DISPLAY_ASSET_ORIGIN", "https://10.23.52.152:18444"},
		{"DISPLAY_RUNTIME_ORIGIN", "https://10.23.52.152:18443"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Setenv("INFRAPRO_"+tc.key, tc.value)
			if _, err := Load(""); err == nil {
				t.Fatal("inconsistent HTTPS configuration accepted")
			}
		})
	}
}

func TestHTTPSNamesAndOriginSyntax(t *testing.T) {
	httpsEnv(t)
	t.Setenv("INFRAPRO_PUBLIC_CONSOLE_URL", "https://console.example/console/")
	t.Setenv("INFRAPRO_TRUSTED_ORIGINS", "https://console.example")
	t.Setenv("INFRAPRO_DISPLAY_RUNTIME_ORIGIN", "https://runtime.example")
	t.Setenv("INFRAPRO_DISPLAY_ASSET_ORIGIN", "https://assets.example")
	if _, err := Load(""); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"https://user:secret@runtime.example", "https://runtime.example/", "https://runtime.example/path", "https://runtime.example?", "https://runtime.example#", "https://runtime.example?x=1", "https://runtime.example#x", "https://runtime.example:0", "https://runtime.example:65536", "https://runtime.example:", "https://runtime.example:abc", "https://runtime.example:443", "https://console.example:443", "https://RUNTIME.example", "https://:18444", "//runtime.example", "ftp://runtime.example"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("INFRAPRO_DISPLAY_RUNTIME_ORIGIN", value)
			if _, err := Load(""); err == nil {
				t.Fatal("invalid origin accepted")
			}
		})
	}
}

func TestHTTPSDeploymentExample(t *testing.T) {
	baseEnv(t)
	c, err := Load("../../../../deploy/display/https.example.yaml")
	if err != nil || !c.CookieSecure || c.HTTPAddr != "127.0.0.1:18000" || c.PublicConsoleURL != "https://10.23.52.152:18443/console/" || c.Display.AssetOrigin != "https://10.23.52.152:18445" {
		t.Fatalf("deployment example: %v", err)
	}
	for _, origin := range []string{"http://localhost:5173", "https://[::1]:18443", "https://console.example"} {
		if _, err := applicationURL(origin, true); err != nil {
			t.Fatal(origin, err)
		}
	}
	for _, origin := range []string{"HTTPS://console.example", "https://[0:0:0:0:0:0:0:1]:18443", "https://127.1", "https://2130706433", "https://0x7f000001", "https://0177.0.0.1", "https://例子.example"} {
		if _, err := applicationURL(origin, true); err == nil {
			t.Fatal("noncanonical origin accepted", origin)
		}
	}
}
