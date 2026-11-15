package config

import (
	"testing"
	"time"
)

func TestAuditDefaultsAndIndependentMetadataStartup(t *testing.T) {
	o := DefaultAudit()
	if o.ExportMaxRows != 50000 || o.ExportMaxBytes != 100<<20 || o.SnapshotMaxBytes != 128<<20 || o.SnapshotTimeout != 15*time.Second || o.ReadTimeout != 15*time.Minute || o.ActivePerAccount != 2 || o.ActivePerScope != 10 || o.ActivePerInstance != 100 || o.ResultTTL != 24*time.Hour || o.CleanupMode != "plan_only" {
		t.Fatal("audit budget drift")
	}
	t.Setenv("INFRAPRO_DATABASE_URL", "postgres://audit:synthetic@localhost/audit_test")
	t.Setenv("INFRAPRO_MONITORING_CLICKHOUSE_URL", "invalid://not-required")
	t.Setenv("INFRAPRO_FILES_CHECKER_SOCKET", "/unavailable/checker.sock")
	_, v, e := LoadAudit("")
	if e != nil || v.ObjectEndpoint != "" {
		t.Fatal("metadata startup unexpectedly needs object storage or parser", e)
	}
}
func TestAuditRejectsUnsafeConfiguration(t *testing.T) {
	for name, edit := range map[string]func(*auditInput){
		"rows": func(v *auditInput) { v.ExportMaxRows = "50001" }, "bytes": func(v *auditInput) { v.ExportMaxBytes = "104857601" }, "snapshot": func(v *auditInput) { v.SnapshotMaxBytes = "134217729" }, "window": func(v *auditInput) { v.MaxWindow = "9000h" }, "heartbeat": func(v *auditInput) { v.Heartbeat = "31s" }, "read": func(v *auditInput) { v.ReadTimeout = "16m" }, "lease": func(v *auditInput) { v.ReadLease = "2m" }, "cleanup": func(v *auditInput) { v.CleanupMode = "force" }, "credential_url": func(v *auditInput) {
			v.ObjectEndpoint = "https://user:secret@storage"
			v.ObjectAccessKey = "test"
			v.ObjectSecretKey = "test"
			v.Bucket = "test"
		}, "incomplete_storage": func(v *auditInput) { v.Bucket = "only-one" }, "candidate": func(v *auditInput) { v.CandidateBudget = "2001" }, "concurrency": func(v *auditInput) { v.ActivePerAccount = "3" }, "retry": func(v *auditInput) { v.MaxAttempts = "4" }, "result_ttl": func(v *auditInput) { v.ResultTTL = "25h" },
	} {
		t.Run(name, func(t *testing.T) {
			v := defaultAuditInput()
			edit(&v)
			if _, e := parseAudit(v); e == nil {
				t.Fatal("unsafe audit configuration accepted")
			}
		})
	}
}
