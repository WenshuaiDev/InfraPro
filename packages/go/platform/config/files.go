package config

import (
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Files is independent of monitoring storage and ClickHouse configuration.
type Files struct {
	AcceptUploads           bool
	ObjectEndpoint          string
	ObjectAccessKey         string
	ObjectSecretKey         string
	Bucket                  string
	NodeID                  string
	TempDirectory           string
	CheckerSocket           string
	MaxFileBytes            int64
	PartBytes               int64
	MaxActivePerAccount     int
	MaxActivePerTenant      int
	MaxQueuedPerTenant      int
	MaxQueuedPerInstance    int
	TemporaryObjectBudget   int64
	LocalTempBudget         int64
	Workers                 int
	MaxRunningPerTenant     int
	Lease                   time.Duration
	Heartbeat               time.Duration
	MaxAttempts             int
	WorkerHeartbeat         time.Duration
	StaleAfter              time.Duration
	ExecutionTimeout        time.Duration
	QueueTimeout            time.Duration
	ObjectIOTimeout         time.Duration
	PartRequestTimeout      time.Duration
	UploadIdleTTL           time.Duration
	UploadHardTTL           time.Duration
	PreparedTTL             time.Duration
	RetryInputTTL           time.Duration
	TrashTTL                time.Duration
	PreviewMaxPixels        int64
	PreviewEdge             int
	PreviewBytes            int64
	CheckerTimeout          time.Duration
	CheckerMemory           int64
	MaxDownloadsPerNode     int
	MaxDownloadsPerAccount  int
	ReadLease               time.Duration
	ReadHeartbeat           time.Duration
	ChunkBytes              int
	ReadTimeout             time.Duration
	CreateRequestsPerMinute int
	PartRequestsPerMinute   int
	ReadRequestsPerMinute   int
	CleanupInterval         time.Duration
	CleanupBatch            int
	ListPageSize            int
	MaxPageSize             int
}
type filesInput struct {
	AcceptUploads           string `yaml:"accept_uploads"`
	ObjectEndpoint          string `yaml:"object_endpoint"`
	ObjectAccessKey         string `yaml:"access_key"`
	ObjectSecretKey         string `yaml:"secret_key"`
	Bucket                  string `yaml:"bucket"`
	NodeID                  string `yaml:"node_id"`
	TempDirectory           string `yaml:"temp_directory"`
	CheckerSocket           string `yaml:"checker_socket"`
	MaxFileBytes            string `yaml:"max_file_bytes"`
	PartBytes               string `yaml:"part_bytes"`
	MaxActivePerAccount     string `yaml:"max_active_uploads_per_account"`
	MaxActivePerTenant      string `yaml:"max_active_uploads_per_tenant"`
	MaxQueuedPerTenant      string `yaml:"max_queued_per_tenant"`
	MaxQueuedPerInstance    string `yaml:"max_queued_per_instance"`
	TemporaryObjectBudget   string `yaml:"temporary_object_budget_bytes"`
	LocalTempBudget         string `yaml:"local_temp_budget_bytes"`
	Workers                 string `yaml:"workers"`
	MaxRunningPerTenant     string `yaml:"max_running_per_tenant"`
	Lease                   string `yaml:"lease"`
	Heartbeat               string `yaml:"heartbeat"`
	MaxAttempts             string `yaml:"max_attempts"`
	WorkerHeartbeat         string `yaml:"worker_heartbeat"`
	StaleAfter              string `yaml:"stale_after"`
	ExecutionTimeout        string `yaml:"execution_timeout"`
	QueueTimeout            string `yaml:"queue_timeout"`
	ObjectIOTimeout         string `yaml:"object_io_timeout"`
	PartRequestTimeout      string `yaml:"part_request_timeout"`
	UploadIdleTTL           string `yaml:"upload_idle_ttl"`
	UploadHardTTL           string `yaml:"upload_hard_ttl"`
	PreparedTTL             string `yaml:"prepared_ttl"`
	RetryInputTTL           string `yaml:"retry_input_ttl"`
	TrashTTL                string `yaml:"trash_ttl"`
	PreviewMaxPixels        string `yaml:"preview_max_pixels"`
	PreviewEdge             string `yaml:"preview_edge"`
	PreviewBytes            string `yaml:"preview_bytes"`
	CheckerTimeout          string `yaml:"checker_timeout"`
	CheckerMemory           string `yaml:"checker_memory"`
	MaxDownloadsPerNode     string `yaml:"max_downloads_per_node"`
	MaxDownloadsPerAccount  string `yaml:"max_downloads_per_account"`
	ReadLease               string `yaml:"read_lease"`
	ReadHeartbeat           string `yaml:"read_heartbeat"`
	ChunkBytes              string `yaml:"chunk_bytes"`
	ReadTimeout             string `yaml:"read_timeout"`
	CreateRequestsPerMinute string `yaml:"create_requests_per_minute"`
	PartRequestsPerMinute   string `yaml:"part_requests_per_minute"`
	ReadRequestsPerMinute   string `yaml:"read_requests_per_minute"`
	CleanupInterval         string `yaml:"cleanup_interval"`
	CleanupBatch            string `yaml:"cleanup_batch"`
	ListPageSize            string `yaml:"list_page_size"`
	MaxPageSize             string `yaml:"max_page_size"`
}

func defaultFilesInput() filesInput {
	return filesInput{
		AcceptUploads:           "true",
		ObjectEndpoint:          "",
		ObjectAccessKey:         "",
		ObjectSecretKey:         "",
		Bucket:                  "",
		NodeID:                  "local",
		TempDirectory:           ".local/files/tmp",
		CheckerSocket:           ".local/files/tmp/checker.sock",
		MaxFileBytes:            "104857600",
		PartBytes:               "8388608",
		MaxActivePerAccount:     "3",
		MaxActivePerTenant:      "20",
		MaxQueuedPerTenant:      "64",
		MaxQueuedPerInstance:    "256",
		TemporaryObjectBudget:   "21474836480",
		LocalTempBudget:         "2147483648",
		Workers:                 "2",
		MaxRunningPerTenant:     "2",
		Lease:                   "60s",
		Heartbeat:               "10s",
		MaxAttempts:             "5",
		WorkerHeartbeat:         "10s",
		StaleAfter:              "30s",
		ExecutionTimeout:        "120s",
		QueueTimeout:            "24h",
		ObjectIOTimeout:         "120s",
		PartRequestTimeout:      "120s",
		UploadIdleTTL:           "24h",
		UploadHardTTL:           "168h",
		PreparedTTL:             "24h",
		RetryInputTTL:           "24h",
		TrashTTL:                "720h",
		PreviewMaxPixels:        "25000000",
		PreviewEdge:             "1600",
		PreviewBytes:            "2097152",
		CheckerTimeout:          "15s",
		CheckerMemory:           "268435456",
		MaxDownloadsPerNode:     "8",
		MaxDownloadsPerAccount:  "2",
		ReadLease:               "60s",
		ReadHeartbeat:           "10s",
		ChunkBytes:              "65536",
		ReadTimeout:             "15m",
		CreateRequestsPerMinute: "30",
		PartRequestsPerMinute:   "120",
		ReadRequestsPerMinute:   "240",
		CleanupInterval:         "5m",
		CleanupBatch:            "100",
		ListPageSize:            "50",
		MaxPageSize:             "100",
	}
}
func DefaultFiles() Files {
	v, e := parseFiles(defaultFilesInput())
	if e != nil {
		panic(e)
	}
	return v
}
func filesEnvironment(raw *filesInput) {
	v := reflect.ValueOf(raw).Elem()
	t := v.Type()
	for n := 0; n < v.NumField(); n++ {
		if value, ok := os.LookupEnv("INFRAPRO_FILES_" + strings.ToUpper(t.Field(n).Tag.Get("yaml"))); ok {
			v.Field(n).SetString(value)
		}
	}
}
func parseFiles(raw filesInput) (Files, error) {
	var out Files
	values := reflect.ValueOf(raw)
	fields := reflect.ValueOf(&out).Elem()
	for n := 0; n < fields.NumField(); n++ {
		f := fields.Field(n)
		value := values.Field(n).String()
		name := values.Type().Field(n).Tag.Get("yaml")
		switch {
		case f.Type() == reflect.TypeOf(time.Duration(0)):
			v, e := time.ParseDuration(value)
			if e != nil || v <= 0 || v > 365*24*time.Hour {
				return out, fmt.Errorf("invalid files %s", name)
			}
			f.SetInt(int64(v))
		case f.Kind() == reflect.String:
			f.SetString(value)
		case f.Kind() == reflect.Bool:
			v, e := strconv.ParseBool(value)
			if e != nil {
				return out, fmt.Errorf("invalid files %s", name)
			}
			f.SetBool(v)
		default:
			v, e := strconv.ParseInt(value, 10, 64)
			if e != nil || v < 1 {
				return out, fmt.Errorf("invalid files %s", name)
			}
			f.SetInt(v)
		}
	}
	if out.Workers > 32 || out.MaxRunningPerTenant > 32 || out.MaxActivePerAccount > 100 || out.MaxActivePerTenant > 1000 || out.MaxQueuedPerTenant > 10000 || out.MaxQueuedPerInstance > 100000 || out.MaxDownloadsPerNode > 128 || out.MaxDownloadsPerAccount > 32 || out.CheckerMemory > 256<<20 || out.CheckerMemory < 32<<20 || out.ObjectIOTimeout > 120*time.Second || out.PartRequestTimeout > 120*time.Second || out.ReadTimeout > 15*time.Minute || out.PreparedTTL > 24*time.Hour || out.RetryInputTTL > 24*time.Hour || out.TrashTTL > 30*24*time.Hour {
		return out, fmt.Errorf("files limit out of bounds")
	}
	if out.MaxFileBytes > MaxUploadBytes || (out.MaxFileBytes-1)/out.PartBytes+1 > 2147483647 || out.PartBytes > 8<<20 || out.ChunkBytes > 65536 || out.MaxPageSize > 100 || out.ListPageSize > out.MaxPageSize || out.MaxAttempts > 5 || out.CleanupBatch > 100 || out.PreviewMaxPixels > 25000000 || out.PreviewEdge > 1600 || out.PreviewBytes > 2<<20 || out.CheckerTimeout > 15*time.Second || out.ExecutionTimeout > 120*time.Second || out.Heartbeat >= out.Lease || out.ReadHeartbeat >= out.ReadLease || out.UploadIdleTTL > out.UploadHardTTL || out.StaleAfter <= out.WorkerHeartbeat {
		return out, fmt.Errorf("files budget out of bounds")
	}
	if out.NodeID == "" || len(out.NodeID) > 100 || strings.ContainsAny(out.NodeID, "/\\ ") || out.TempDirectory == "" || out.CheckerSocket == "" {
		return out, fmt.Errorf("invalid files node or temporary directory")
	}
	if out.ObjectEndpoint != "" {
		u, e := url.Parse(out.ObjectEndpoint)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || out.Bucket == "" || out.ObjectAccessKey == "" || out.ObjectSecretKey == "" {
			return out, fmt.Errorf("invalid files storage configuration")
		}
	}
	return out, nil
}
func LoadFiles(path string) (Database, Files, error) {
	db, e := LoadDatabase(path)
	if e != nil {
		return db, Files{}, e
	}
	raw, e := loadInput(path)
	if e != nil {
		return db, Files{}, e
	}
	v, e := parseFiles(raw.Files)
	return db, v, e
}
