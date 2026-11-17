package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// MaxUploadBytes is the interoperable integer boundary, not a deployment quota.
// Upload sizes cross JSON/JavaScript, Go and PostgreSQL; part indexes are int32.
const MaxUploadBytes int64 = (1 << 53) - 1

type Uploads struct {
	SpatialMaxFileBytes    int64 `json:"spatial_max_file_bytes,string"`
	MonitoringCSVMaxBytes  int64 `json:"monitoring_csv_max_bytes,string"`
	MonitoringXLSXMaxBytes int64 `json:"monitoring_xlsx_max_bytes,string"`
	DisplayMaxPackageBytes int64 `json:"display_max_package_bytes,string"`
}
type uploadsInput struct {
	SpatialMaxFileBytes    string `yaml:"spatial_max_file_bytes"`
	MonitoringCSVMaxBytes  string `yaml:"monitoring_csv_max_bytes"`
	MonitoringXLSXMaxBytes string `yaml:"monitoring_xlsx_max_bytes"`
	DisplayMaxPackageBytes string `yaml:"display_max_package_bytes"`
}

func DefaultUploads() Uploads {
	return Uploads{256 << 20, 32 << 30, 512 << 20, 128 << 20}
}
func defaultUploadsInput() uploadsInput {
	return uploadsInput{"268435456", "34359738368", "536870912", "134217728"}
}
func uploadsEnvironment(raw *uploadsInput) {
	v := reflect.ValueOf(raw).Elem()
	for i := 0; i < v.NumField(); i++ {
		if value, ok := os.LookupEnv("INFRAPRO_UPLOADS_" + strings.ToUpper(v.Type().Field(i).Tag.Get("yaml"))); ok {
			v.Field(i).SetString(value)
		}
	}
}
func parseUploads(raw uploadsInput) (Uploads, error) {
	var out Uploads
	v, dst := reflect.ValueOf(raw), reflect.ValueOf(&out).Elem()
	for i := 0; i < v.NumField(); i++ {
		n, e := strconv.ParseInt(v.Field(i).String(), 10, 64)
		if e != nil || n < 1 || n > MaxUploadBytes {
			return out, fmt.Errorf("uploads.%s must be a positive exact byte count no greater than %d", v.Type().Field(i).Tag.Get("yaml"), MaxUploadBytes)
		}
		dst.Field(i).SetInt(n)
	}
	return out, nil
}

// EffectiveUploads supplies defaults for programmatic test/embedded configs.
// Explicit zero/negative YAML and environment values are rejected by parseUploads.
func (c Config) EffectiveUploads() Uploads {
	u, defaults := c.Uploads, DefaultUploads()
	v, d := reflect.ValueOf(&u).Elem(), reflect.ValueOf(defaults)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Int() == 0 {
			v.Field(i).SetInt(d.Field(i).Int())
		}
	}
	return u
}

func LoadUploads(path string) (Uploads, error) {
	raw, e := loadInput(path)
	if e != nil {
		return Uploads{}, e
	}
	return parseUploads(raw.Uploads)
}
