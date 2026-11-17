package config

import "testing"

func monitoringEnv(t *testing.T) {
	t.Helper()
	for key, value := range map[string]string{
		"INFRAPRO_MONITORING_CLICKHOUSE_REPLICAS": "127.0.0.1:59000",
		"INFRAPRO_MONITORING_PHYSICAL_SHARDS":     "[]",
		"INFRAPRO_MONITORING_OBJECT_ENDPOINT":     "http://127.0.0.1:58333",
		"INFRAPRO_MONITORING_OBJECT_ACCESS_KEY":   "test-access",
		"INFRAPRO_MONITORING_OBJECT_SECRET_KEY":   "test-secret",
		"INFRAPRO_MONITORING_SOURCE_BUCKET":       "test-monitoring",
	} {
		t.Setenv(key, value)
	}
}

func TestMonitoringSingleAndMultipleEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name, endpoints, shards string
		want                    int
	}{
		{"single", "127.0.0.1:59000", "[]", 1},
		{"multiple", "127.0.0.1:59000,127.0.0.1:59001", "[]", 2},
		{"single_additional_shard", "127.0.0.1:59000", `[{"name":"extra","database":"monitoring_extra","replicas":["127.0.0.1:59001"]}]`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			monitoringEnv(t)
			t.Setenv("INFRAPRO_MONITORING_CLICKHOUSE_REPLICAS", tc.endpoints)
			t.Setenv("INFRAPRO_MONITORING_PHYSICAL_SHARDS", tc.shards)
			cfg, err := LoadMonitoring("")
			if err != nil || len(cfg.Replicas) != tc.want {
				t.Fatalf("valid endpoint configuration rejected: count=%d error=%v", len(cfg.Replicas), err)
			}
			if tc.name == "single_additional_shard" && (len(cfg.Shards) != 1 || len(cfg.Shards[0].Replicas) != 1) {
				t.Fatal("single endpoint additional shard was not retained")
			}
		})
	}
}

func TestMonitoringStillRejectsMissingAndInvalidEndpoints(t *testing.T) {
	for _, endpoints := range []string{"", " ", "127.0.0.1", "127.0.0.1:0", "127.0.0.1:59000,", "127.0.0.1:59000,127.0.0.1:59000"} {
		t.Run(endpoints, func(t *testing.T) {
			monitoringEnv(t)
			t.Setenv("INFRAPRO_MONITORING_CLICKHOUSE_REPLICAS", endpoints)
			if _, err := LoadMonitoring(""); err == nil {
				t.Fatal("missing, invalid or duplicate endpoints accepted")
			}
		})
	}
	t.Run("empty_additional_shard", func(t *testing.T) {
		monitoringEnv(t)
		t.Setenv("INFRAPRO_MONITORING_PHYSICAL_SHARDS", `[{"name":"extra","database":"monitoring_extra","replicas":[]}]`)
		if _, err := LoadMonitoring(""); err == nil {
			t.Fatal("additional shard without an endpoint accepted")
		}
	})
}
