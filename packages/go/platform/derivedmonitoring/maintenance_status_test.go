package derivedmonitoring_test

import (
	"encoding/json"
	"strings"
	"testing"

	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
)

func TestDerivedMaintenanceStatusScopesAggregates(t *testing.T) {
	f := setup(t)
	for _, tenant := range []string{"", f.scope.Tenant} {
		data, e := dm.MaintenanceStatus(ctx, f.e.Pool, tenant)
		must(t, e)
		var status map[string]json.RawMessage
		must(t, json.Unmarshal(data, &status))
		for _, key := range []string{"environment", "integrity", "retention", "queues", "dirty", "consumers", "control", "leases", "attempts", "automatic_latency", "clock", "generated_at"} {
			if _, ok := status[key]; !ok {
				t.Fatal("missing maintenance aggregate", key)
			}
		}
		if strings.Contains(string(data), "object_key") || strings.Contains(string(data), "password") {
			t.Fatal("sensitive maintenance output")
		}
	}
	if _, e := dm.MaintenanceStatus(ctx, f.e.Pool, "not-a-tenant-id"); e == nil {
		t.Fatal("invalid tenant accepted")
	}
}
