package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type PhysicalShard struct {
	Name     string   `json:"name"`
	Database string   `json:"database"`
	Replicas []string `json:"replicas"`
}
type Monitoring struct {
	TaskLease, TaskHeartbeat                                                time.Duration
	TaskAttempts                                                            int
	Shards                                                                  []PhysicalShard
	HotDays                                                                 int
	OnlineVolume                                                            string
	ExportRows, ExportBytes                                                 int64
	Replicas                                                                []string
	Database, User, Password, Policy                                        string
	TLS                                                                     bool
	ObjectEndpoint, ObjectAccessKey, ObjectSecretKey, Bucket, TempDirectory string
}

func LoadMonitoring(path string) (Monitoring, error) {
	raw, e := loadInput(path)
	if e != nil {
		return Monitoring{}, e
	}
	c, e := resolveMonitoring(raw)
	if e == nil && len(c.Replicas) == 0 {
		e = fmt.Errorf("monitoring storage is not configured")
	}
	return c, e
}
func resolveMonitoring(raw input) (Monitoring, error) {
	c := Monitoring{OnlineVolume: raw.MonitoringOnlineVolume, Database: raw.MonitoringDatabase, User: raw.MonitoringUser, Password: raw.MonitoringPassword, Policy: raw.MonitoringPolicy, ObjectEndpoint: raw.MonitoringObjectEndpoint, ObjectAccessKey: raw.MonitoringObjectAccessKey, ObjectSecretKey: raw.MonitoringObjectSecretKey, Bucket: raw.MonitoringBucket, TempDirectory: raw.MonitoringTemp}
	var taskError error
	c.TaskLease, taskError = time.ParseDuration(raw.MonitoringTaskLease)
	if taskError != nil || c.TaskLease < 10*time.Second || c.TaskLease > 5*time.Minute {
		return c, fmt.Errorf("monitoring_task_lease must be 10s-5m")
	}
	c.TaskHeartbeat, taskError = time.ParseDuration(raw.MonitoringTaskHeartbeat)
	if taskError != nil || c.TaskHeartbeat < time.Second || 3*c.TaskHeartbeat > c.TaskLease {
		return c, fmt.Errorf("monitoring_task_heartbeat must be at least 1s and at most a third of the lease")
	}
	c.TaskAttempts, taskError = strconv.Atoi(raw.MonitoringTaskAttempts)
	if taskError != nil || c.TaskAttempts < 1 || c.TaskAttempts > 10 {
		return c, fmt.Errorf("monitoring_task_attempts must be 1-10")
	}
	if raw.MonitoringShards != "" {
		d := json.NewDecoder(bytes.NewBufferString(raw.MonitoringShards))
		d.DisallowUnknownFields()
		if e := d.Decode(&c.Shards); e != nil {
			return c, fmt.Errorf("invalid monitoring physical shard configuration")
		}
		var extra any
		if e := d.Decode(&extra); e != io.EOF {
			return c, fmt.Errorf("invalid monitoring physical shard configuration")
		}
		if len(c.Shards) > 32 {
			return c, fmt.Errorf("at most 32 physical shards may be configured")
		}
		names := map[string]bool{"local": true}
		databases := map[string]bool{c.Database: true}
		for _, p := range c.Shards {
			if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(p.Name) || !regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`).MatchString(p.Database) || names[p.Name] || databases[p.Database] || len(p.Replicas) < 2 {
				return c, fmt.Errorf("invalid or duplicate monitoring physical shard")
			}
			names[p.Name] = true
			databases[p.Database] = true
		}
	}
	if raw.MonitoringReplicas == "" {
		if len(c.Shards) > 0 || c.ObjectEndpoint != "" || c.ObjectAccessKey != "" || c.ObjectSecretKey != "" || c.Bucket != "" {
			return c, fmt.Errorf("monitoring configuration is incomplete")
		}
		return c, nil
	}
	c.Replicas = strings.Split(raw.MonitoringReplicas, ",")
	seen := map[string]bool{}
	if len(c.Replicas) < 2 {
		return c, fmt.Errorf("monitoring requires at least two replica endpoints")
	}
	for i, addr := range c.Replicas {
		addr = strings.TrimSpace(addr)
		host, port, e := net.SplitHostPort(addr)
		n, parse := strconv.Atoi(port)
		if e != nil || host == "" || parse != nil || n < 1 || n > 65535 || seen[addr] {
			return c, fmt.Errorf("invalid or duplicate monitoring replica endpoint")
		}
		seen[addr] = true
		c.Replicas[i] = addr
	}
	identifier := regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	if !identifier.MatchString(c.Database) || !identifier.MatchString(c.Policy) {
		return c, fmt.Errorf("invalid monitoring database or storage policy")
	}
	var e error
	c.HotDays, e = strconv.Atoi(raw.MonitoringHotDays)
	if e != nil || c.HotDays < 1 || c.HotDays > 3650 || !identifier.MatchString(c.OnlineVolume) {
		return c, fmt.Errorf("invalid monitoring online tier configuration")
	}
	c.TLS, e = strconv.ParseBool(raw.MonitoringTLS)
	if e != nil {
		return c, fmt.Errorf("invalid monitoring TLS flag")
	}
	endpoint, e := url.Parse(c.ObjectEndpoint)
	if e != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") || c.ObjectAccessKey == "" || c.ObjectSecretKey == "" || !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`).MatchString(c.Bucket) {
		return c, fmt.Errorf("invalid monitoring object storage configuration")
	}
	c.ExportRows, e = strconv.ParseInt(raw.MonitoringExportRows, 10, 64)
	if e != nil || c.ExportRows < 1 || c.ExportRows > 1000000 {
		return c, fmt.Errorf("monitoring export rows per file must be 1–1000000")
	}
	c.ExportBytes, e = strconv.ParseInt(raw.MonitoringExportBytes, 10, 64)
	if e != nil || c.ExportBytes < 1<<20 || c.ExportBytes > 1<<30 {
		return c, fmt.Errorf("monitoring export bytes per file must be 1–1024 MiB")
	}
	return c, nil
}
