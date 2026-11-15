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

type Audit struct {
	ObjectEndpoint      string
	ObjectAccessKey     string
	ObjectSecretKey     string
	Bucket              string
	TempDirectory       string
	NodeID              string
	PageSize            int
	MaxPageSize         int
	CandidateBudget     int
	TraceMaxNodes       int
	TraceMaxEdges       int
	ExportMaxRows       int
	ExportMaxBytes      int64
	SnapshotMaxBytes    int64
	Workers             int
	ActivePerAccount    int
	ActivePerScope      int
	ActivePerInstance   int
	MaxAttempts         int
	TempBudget          int64
	DownloadsPerAccount int
	DownloadsPerNode    int
	FactBatch           int
	ObjectBatch         int
	DefaultWindow       time.Duration
	MaxWindow           time.Duration
	MetadataTimeout     time.Duration
	SnapshotTimeout     time.Duration
	Lease               time.Duration
	Heartbeat           time.Duration
	ResultTTL           time.Duration
	ReadLease           time.Duration
	ReadHeartbeat       time.Duration
	ReadTimeout         time.Duration
	FailedFactTimeout   time.Duration
	TransferStaleAfter  time.Duration
	CleanupMode         string
}
type auditInput struct {
	ObjectEndpoint      string `yaml:"object_endpoint"`
	ObjectAccessKey     string `yaml:"access_key"`
	ObjectSecretKey     string `yaml:"secret_key"`
	Bucket              string `yaml:"bucket"`
	TempDirectory       string `yaml:"temp_directory"`
	NodeID              string `yaml:"node_id"`
	PageSize            string `yaml:"page_size"`
	MaxPageSize         string `yaml:"max_page_size"`
	CandidateBudget     string `yaml:"candidate_budget"`
	TraceMaxNodes       string `yaml:"trace_max_nodes"`
	TraceMaxEdges       string `yaml:"trace_max_edges"`
	ExportMaxRows       string `yaml:"export_max_rows"`
	ExportMaxBytes      string `yaml:"export_max_bytes"`
	SnapshotMaxBytes    string `yaml:"export_snapshot_max_bytes"`
	Workers             string `yaml:"export_worker_concurrency"`
	ActivePerAccount    string `yaml:"active_per_account"`
	ActivePerScope      string `yaml:"active_per_scope"`
	ActivePerInstance   string `yaml:"active_per_instance"`
	MaxAttempts         string `yaml:"export_max_attempts"`
	TempBudget          string `yaml:"export_temp_budget"`
	DownloadsPerAccount string `yaml:"download_per_account"`
	DownloadsPerNode    string `yaml:"download_per_node"`
	FactBatch           string `yaml:"fact_batch"`
	ObjectBatch         string `yaml:"object_batch"`
	DefaultWindow       string `yaml:"query_default_window"`
	MaxWindow           string `yaml:"max_window"`
	MetadataTimeout     string `yaml:"metadata_timeout"`
	SnapshotTimeout     string `yaml:"export_snapshot_timeout"`
	Lease               string `yaml:"export_lease"`
	Heartbeat           string `yaml:"export_heartbeat"`
	ResultTTL           string `yaml:"export_result_ttl"`
	ReadLease           string `yaml:"read_lease"`
	ReadHeartbeat       string `yaml:"read_heartbeat"`
	ReadTimeout         string `yaml:"read_timeout"`
	FailedFactTimeout   string `yaml:"failed_fact_timeout"`
	TransferStaleAfter  string `yaml:"transfer_stale_after"`
	CleanupMode         string `yaml:"cleanup_mode"`
}

func DefaultAudit() Audit {
	return Audit{TempDirectory: ".artifacts/audit/tmp", NodeID: "audit-local", PageSize: 50, MaxPageSize: 100, CandidateBudget: 2000, TraceMaxNodes: 100, TraceMaxEdges: 200, ExportMaxRows: 50000, ExportMaxBytes: 100 << 20, SnapshotMaxBytes: 128 << 20, Workers: 2, ActivePerAccount: 2, ActivePerScope: 10, ActivePerInstance: 100, MaxAttempts: 3, TempBudget: 512 << 20, DownloadsPerAccount: 2, DownloadsPerNode: 8, FactBatch: 500, ObjectBatch: 100, DefaultWindow: 7 * 24 * time.Hour, MaxWindow: 366 * 24 * time.Hour, MetadataTimeout: 3 * time.Second, SnapshotTimeout: 15 * time.Second, Lease: 60 * time.Second, Heartbeat: 10 * time.Second, ResultTTL: 24 * time.Hour, ReadLease: 30 * time.Second, ReadHeartbeat: 10 * time.Second, ReadTimeout: 15 * time.Minute, FailedFactTimeout: 2 * time.Second, TransferStaleAfter: 20 * time.Minute, CleanupMode: "plan_only"}
}
func defaultAuditInput() auditInput {
	var raw auditInput
	v := reflect.ValueOf(&raw).Elem()
	d := reflect.ValueOf(DefaultAudit())
	for i := 0; i < v.NumField(); i++ {
		f := d.Field(i)
		var value string
		if f.Type() == reflect.TypeFor[time.Duration]() {
			value = time.Duration(f.Int()).String()
		} else if f.Kind() == reflect.String {
			value = f.String()
		} else {
			value = strconv.FormatInt(f.Int(), 10)
		}
		v.Field(i).SetString(value)
	}
	return raw
}
func auditEnvironment(raw *auditInput) {
	v := reflect.ValueOf(raw).Elem()
	for i := 0; i < v.NumField(); i++ {
		if value, ok := os.LookupEnv("INFRAPRO_AUDIT_" + strings.ToUpper(v.Type().Field(i).Tag.Get("yaml"))); ok {
			v.Field(i).SetString(value)
		}
	}
}
func parseAudit(raw auditInput) (Audit, error) {
	var out Audit
	v := reflect.ValueOf(&out).Elem()
	in := reflect.ValueOf(raw)
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		s := in.Field(i).String()
		if f.Kind() == reflect.String {
			f.SetString(s)
			continue
		}
		var n int64
		var err error
		if f.Type() == reflect.TypeFor[time.Duration]() {
			var d time.Duration
			d, err = time.ParseDuration(s)
			n = int64(d)
		} else {
			n, err = strconv.ParseInt(s, 10, 64)
		}
		if err != nil || n < 1 {
			return out, fmt.Errorf("invalid audit setting: %s", in.Type().Field(i).Tag.Get("yaml"))
		}
		f.SetInt(n)
	}
	if out.MaxPageSize > 100 || out.PageSize > out.MaxPageSize || out.CandidateBudget > 2000 || out.TraceMaxNodes > 100 || out.TraceMaxEdges > 200 || out.ExportMaxRows > 50000 || out.ExportMaxBytes > 100<<20 || out.SnapshotMaxBytes > 128<<20 || out.Workers > 8 || out.ActivePerAccount > 2 || out.ActivePerScope > 10 || out.ActivePerInstance > 100 || out.MaxAttempts > 3 || out.TempBudget < out.ExportMaxBytes || out.TempBudget > 512<<20 || out.DownloadsPerAccount > 2 || out.DownloadsPerNode > 8 || out.FactBatch > 500 || out.ObjectBatch > 100 || out.DefaultWindow > out.MaxWindow || out.MaxWindow > 366*24*time.Hour || out.MetadataTimeout > 3*time.Second || out.SnapshotTimeout > 15*time.Second || out.Heartbeat*2 >= out.Lease || out.Lease > 5*time.Minute || out.ResultTTL > 24*time.Hour || out.ReadHeartbeat*2 >= out.ReadLease || out.ReadLease > time.Minute || out.ReadTimeout > 15*time.Minute || out.FailedFactTimeout > 2*time.Second || out.TransferStaleAfter < out.ReadTimeout || out.CleanupMode != "plan_only" {
		return out, fmt.Errorf("audit budgets out of bounds")
	}
	configured := 0
	for _, s := range []string{out.ObjectEndpoint, out.ObjectAccessKey, out.ObjectSecretKey, out.Bucket} {
		if s != "" {
			configured++
		}
	}
	if configured != 0 && configured != 4 {
		return out, fmt.Errorf("audit private storage configuration incomplete")
	}
	if configured == 4 {
		u, e := url.Parse(out.ObjectEndpoint)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return out, fmt.Errorf("invalid audit private storage endpoint")
		}
	}
	if out.NodeID == "" || len(out.NodeID) > 80 || out.TempDirectory == "" {
		return out, fmt.Errorf("invalid audit worker configuration")
	}
	return out, nil
}
func LoadAudit(path string) (Database, Audit, error) {
	db, e := LoadDatabase(path)
	if e != nil {
		return db, Audit{}, e
	}
	raw, e := loadInput(path)
	if e != nil {
		return db, Audit{}, e
	}
	o, e := parseAudit(raw.Audit)
	return db, o, e
}

// LoadAuditRetention reads the historical deployed configuration without requiring web-session secrets.
func LoadAuditRetention(path string) (time.Duration, error) {
	raw, e := loadInput(path)
	if e != nil {
		return 0, e
	}
	v, e := time.ParseDuration(raw.AuditRetention)
	if e != nil || v <= 0 {
		return 0, fmt.Errorf("invalid historical audit_retention")
	}
	return v, nil
}
