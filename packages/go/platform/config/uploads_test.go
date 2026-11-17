package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUploadLimitsConfigurationAndIndependentWorker(t *testing.T) {
	baseEnv(t)
	path := filepath.Join(t.TempDir(), "uploads.yaml")
	err := os.WriteFile(path, []byte("files:\n  max_file_bytes: 209715200\nuploads:\n  spatial_max_file_bytes: 1073741824\n  monitoring_csv_max_bytes: 68719476736\n  monitoring_xlsx_max_bytes: 1073741824\n  display_max_package_bytes: 268435456\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Files.MaxFileBytes != 200<<20 || c.Uploads != (Uploads{1 << 30, 64 << 30, 1 << 30, 256 << 20}) {
		t.Fatal(c.Files.MaxFileBytes, c.Uploads)
	}
	t.Setenv("INFRAPRO_UPLOADS_SPATIAL_MAX_FILE_BYTES", "17")
	t.Setenv("INFRAPRO_REDIS_URL", "invalid")
	u, err := LoadUploads(path)
	if err != nil || u.SpatialMaxFileBytes != 17 {
		t.Fatal(u, err)
	}
	t.Setenv("INFRAPRO_FILES_MAX_FILE_BYTES", "17")
	_, files, err := LoadFiles(path)
	if err != nil || files.MaxFileBytes != 17 {
		t.Fatal(files.MaxFileBytes, err)
	}
}

func TestUploadLimitsRejectInvalidByteCounts(t *testing.T) {
	for _, field := range []string{"SPATIAL_MAX_FILE_BYTES", "MONITORING_CSV_MAX_BYTES", "MONITORING_XLSX_MAX_BYTES", "DISPLAY_MAX_PACKAGE_BYTES"} {
		for _, value := range []string{"0", "-1", "1.5", "unlimited", "9007199254740992", "9223372036854775808"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				t.Setenv("INFRAPRO_UPLOADS_"+field, value)
				if _, err := LoadUploads(""); err == nil {
					t.Fatal("invalid upload limit accepted")
				}
			})
		}
	}
}
