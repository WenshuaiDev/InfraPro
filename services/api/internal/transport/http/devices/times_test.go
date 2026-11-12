package deviceshttp

import "testing"

func TestExplicitInstallationTimes(t *testing.T) {
	for _, tc := range []struct {
		action, body string
		valid        bool
	}{
		{"create", `{}`, true}, {"create", `{"installed_at":"2026-11-11T00:00:00Z"}`, true},
		{"create", `{"installed_at":""}`, false}, {"create", `{"uninstalled_at":null}`, false},
		{"end", `{"uninstalled_at":""}`, false}, {"correct", `{"uninstalled_at":null}`, true},
		{"preview", `{"action":"installation_correct","uninstalled_at":null}`, true},
		{"preview", `{"action":"installation_create","uninstalled_at":null}`, false},
	} {
		if got := installationTimes([]byte(tc.body), tc.action) == nil; got != tc.valid {
			t.Errorf("%s %s: valid=%v", tc.action, tc.body, got)
		}
	}
}
