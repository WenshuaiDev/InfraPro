package parsing

import (
	"context"
	"testing"
)

func TestRemoteEndpointRequiresTLS(t *testing.T) {
	for _, endpoint := range []string{"http://192.168.1.20:8091", "http://10.0.0.2:8091", "http://172.16.1.2:8091", "http://example.test:8091", "ftp://127.0.0.1:8091"} {
		s := &Service{options: Options{Enabled: true, URL: endpoint}}
		_, err := s.call(context.Background(), "/v1/catalog", nil)
		if codeOf(err) != "service_tls_required" {
			t.Fatalf("remote endpoint was not rejected before credentials/network: %s: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"http://127.0.0.1:8091", "http://[::1]:8091", "http://localhost:8091", "http://parser:8091", "https://example.test:8091"} {
		s := &Service{options: Options{Enabled: true, URL: endpoint}}
		_, err := s.call(context.Background(), "/v1/catalog", nil)
		if codeOf(err) != "service_auth_failed" {
			t.Fatalf("valid endpoint should reach missing-credential check: %s: %v", endpoint, err)
		}
	}
}
