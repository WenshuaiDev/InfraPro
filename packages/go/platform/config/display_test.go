package config

import "testing"

func TestDisplayHTTPOrigins(t *testing.T) {
	raw := defaultDisplayInput()
	raw.RuntimeOrigin = "http://runtime.infrapro.lan:8081"
	raw.AssetOrigin = "http://assets.infrapro.lan:8082"
	if _, e := resolveDisplay(raw, "http://console.infrapro.lan:8080/console/", false); e != nil {
		t.Fatal("explicit LAN HTTP rejected", e)
	}
	for _, origin := range []string{"http://console.infrapro.lan:8080", "http://runtime.infrapro.lan:8081/path", "https://runtime.infrapro.lan", "http://user:password@runtime.infrapro.lan", "http://runtime.infrapro.lan?token=x"} {
		copy := raw
		copy.RuntimeOrigin = origin
		if _, e := resolveDisplay(copy, "http://console.infrapro.lan:8080/console/", false); e == nil {
			t.Fatal("invalid origin accepted", origin)
		}
	}
	if _, e := resolveDisplay(raw, "http://console.infrapro.lan:8080/", true); e == nil {
		t.Fatal("Secure cookie accepted for HTTP")
	}
}
