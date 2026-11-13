package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDerivedDeploymentBudgetsAndSharedLease(t *testing.T) {
	baseEnv(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if e := os.WriteFile(path, []byte("derived:\n  historical_workers: '2'\n  memory_bytes: '2147483648'\n  write_rows: '40000'\n  trial_ttl: 1h\nmonitoring_task_lease: 12s\nmonitoring_task_heartbeat: 2s\nmonitoring_task_attempts: '3'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("INFRAPRO_DERIVED_HISTORICAL_WORKERS", "3")
	c, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	if c.Derived.HistoricalWorkers != 3 || c.Derived.ForegroundWorkers != 1 || c.Derived.WriteRows != 40000 || c.Derived.TrialTTL != time.Hour || c.Derived.MemoryBytes != 2<<30 || c.Monitoring.TaskLease != 12*time.Second || c.Monitoring.TaskHeartbeat != 2*time.Second || c.Monitoring.TaskAttempts != 3 {
		t.Fatal("deployment configuration precedence mismatch", c.Derived, c.Monitoring.TaskLease)
	}
	// CLI-only loaders must use exactly the same configuration sources.
	d, e := LoadDerived(path)
	if e != nil || d != c.Derived {
		t.Fatal("CLI derived budget mismatch", e)
	}
	for _, tc := range []struct{ key, value string }{
		{"INFRAPRO_DERIVED_FOREGROUND_WORKERS", "0"}, {"INFRAPRO_DERIVED_RULE_INPUTS", "17"}, {"INFRAPRO_DERIVED_GRAPH_DEPTH", "9"},
		{"INFRAPRO_DERIVED_MEMORY_BYTES", "67108864"}, {"INFRAPRO_DERIVED_TEMPORARY_BYTES", "-1"}, {"INFRAPRO_DERIVED_TRIAL_ROWS", "100001"},
		{"INFRAPRO_DERIVED_TRIAL_DEADLINE", "4s"}, {"INFRAPRO_MONITORING_TASK_LEASE", "9s"}, {"INFRAPRO_MONITORING_TASK_HEARTBEAT", "5s"}, {"INFRAPRO_MONITORING_TASK_ATTEMPTS", "0"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, e := Load(path); e == nil {
				t.Fatal("invalid or contradictory budget accepted")
			}
		})
	}
	if e := os.WriteFile(path, []byte("derived:\n  numerical_scale: '5'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(path); e == nil {
		t.Fatal("unknown semantic override accepted as deployment budget")
	}
}
