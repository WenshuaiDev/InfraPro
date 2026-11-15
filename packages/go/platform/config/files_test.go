package config

import (
	"testing"
	"time"
)

func TestFilesDefaultsAndIndependentLoader(t *testing.T) {
	d := DefaultFiles()
	if d.MaxFileBytes != 100<<20 || d.PartBytes != 8<<20 || d.TemporaryObjectBudget != 20<<30 || d.LocalTempBudget != 2<<30 || d.Workers != 2 || d.ChunkBytes != 65536 || d.TrashTTL != 30*24*time.Hour || d.PreviewEdge != 1600 {
		t.Fatal(d)
	}
	t.Setenv("INFRAPRO_DATABASE_URL", "postgres://files:local@localhost/files_test")
	t.Setenv("INFRAPRO_MONITORING_CLICKHOUSE_URL", "invalid://must-not-be-read")
	t.Setenv("INFRAPRO_FILES_MAX_FILE_BYTES", "16777216")
	_, f, e := LoadFiles("")
	if e != nil || f.MaxFileBytes != 16<<20 {
		t.Fatal(f.MaxFileBytes, e)
	}
}
func TestFilesRejectsUnsafeBudgetsAndCredentialURL(t *testing.T) {
	for _, edit := range []func(*filesInput){func(f *filesInput) { f.MaxFileBytes = "104857601" }, func(f *filesInput) { f.PartBytes = "16777216" }, func(f *filesInput) { f.Workers = "999999999999" }, func(f *filesInput) { f.CheckerMemory = "9999999999" }, func(f *filesInput) { f.CheckerTimeout = "16s" }, func(f *filesInput) { f.ChunkBytes = "65537" }, func(f *filesInput) { f.ReadTimeout = "16m" }, func(f *filesInput) { f.PreparedTTL = "48h" }, func(f *filesInput) { f.Heartbeat = "60s" }, func(f *filesInput) { f.ObjectEndpoint = "https://user:password@host" }} {
		raw := defaultFilesInput()
		edit(&raw)
		if _, e := parseFiles(raw); e == nil {
			t.Fatal("unsafe files config accepted")
		}
	}
}
