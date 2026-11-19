// Package eventbus carries bounded references to already committed PostgreSQL
// facts. JetStream never holds a personnel credential or a domain payload.
package eventbus

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Mode             string `json:"mode"`
	Environment      string `json:"environment"`
	NATSURL          string `json:"nats_url"`
	CAFile           string `json:"ca_file"`
	CertFile         string `json:"cert_file"`
	KeyFile          string `json:"key_file"`
	CredentialsFile  string `json:"credentials_file,omitempty"`
	LoopbackTest     bool   `json:"loopback_test,omitempty"`
	Replicas         int    `json:"replicas"`
	Batch            int    `json:"batch"`
	MaxAckPending    int    `json:"max_ack_pending"`
	MaxDeliver       int    `json:"max_deliver"`
	Workers          int    `json:"work_concurrency"`
	MaxAttempts      int    `json:"max_attempts"`
	AckWaitMS        int    `json:"ack_wait_ms"`
	WorkTimeoutMS    int    `json:"work_timeout_ms"`
	PollMS           int    `json:"relay_poll_ms"`
	FallbackScanMS   int    `json:"fallback_scan_ms"`
	BackoffMS        int    `json:"backoff_ms"`
	MaxBackoffMS     int    `json:"max_backoff_ms"`
	IngestionBytes   int64  `json:"ingestion_bytes"`
	MonitoringBytes  int64  `json:"monitoring_bytes"`
	AuxiliaryBytes   int64  `json:"auxiliary_bytes"`
	DiskSoftPercent  int    `json:"disk_soft_percent"`
	DiskHardPercent  int    `json:"disk_hard_percent"`
	RetentionSeconds int    `json:"retention_seconds"`
	DedupSeconds     int    `json:"dedup_seconds"`
}

var environmentName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func Defaults() Config {
	return Config{Mode: "jetstream", Replicas: 1, Batch: 32, MaxAckPending: 64, MaxDeliver: 8, Workers: 4, DiskSoftPercent: 80, DiskHardPercent: 95, MaxAttempts: 8, AckWaitMS: 30000, WorkTimeoutMS: 120000, PollMS: 100, FallbackScanMS: 5000, BackoffMS: 500, MaxBackoffMS: 60000, IngestionBytes: 1 << 30, MonitoringBytes: 1 << 29, AuxiliaryBytes: 1 << 28, RetentionSeconds: 7 * 86400, DedupSeconds: 120}
}
func Load(path string) (Config, error) {
	c := Defaults()
	if path == "" {
		c.Mode = "off"
		return c, nil
	}
	f, e := os.Open(path)
	if e != nil {
		return c, e
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 64<<10))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	if d.Decode(new(any)) != io.EOF {
		return c, errors.New("invalid events file")
	}
	for _, v := range []*string{&c.CAFile, &c.CertFile, &c.KeyFile, &c.CredentialsFile} {
		if *v != "" && !filepath.IsAbs(*v) {
			*v = filepath.Join(filepath.Dir(path), *v)
		}
	}
	if !environmentName.MatchString(c.Environment) || (c.Mode != "jetstream" && c.Mode != "pg") {
		return c, errors.New("invalid event environment or transport mode")
	}
	if c.Mode == "jetstream" {
		for _, endpoint := range strings.Split(c.NATSURL, ",") {
			u, e := url.Parse(endpoint)
			if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
				return c, errors.New("invalid broker endpoint")
			}
			if c.LoopbackTest && net.ParseIP(u.Hostname()).IsLoopback() && u.Scheme == "nats" {
				continue
			}
			if u.Scheme != "tls" || c.CAFile == "" || c.CertFile == "" || c.KeyFile == "" {
				return c, errors.New("broker requires mutual TLS")
			}
		}
	}
	if (c.Replicas != 1 && c.Replicas != 3) || c.Batch < 1 || c.Batch > 128 || c.MaxAckPending < c.Batch || c.MaxAckPending > 1024 || c.MaxDeliver < 2 || c.MaxDeliver > 50 || c.MaxAttempts < 1 || c.MaxAttempts > 50 || c.Workers < 1 || c.Workers > 32 {
		return c, errors.New("invalid event concurrency or attempts")
	}
	if c.PollMS < 10 || c.PollMS > 300000 || c.FallbackScanMS < 100 || c.FallbackScanMS > 300000 || c.BackoffMS < 10 || c.MaxBackoffMS < c.BackoffMS || c.MaxBackoffMS > 3600000 || c.AckWaitMS < 100 || c.AckWaitMS > 600000 || c.WorkTimeoutMS < 1000 || c.WorkTimeoutMS > 1800000 {
		return c, errors.New("invalid event deadlines")
	}
	for _, v := range []int64{c.IngestionBytes, c.MonitoringBytes, c.AuxiliaryBytes} {
		if v < 1<<16 || v > 1<<45 {
			return c, fmt.Errorf("invalid stream capacity")
		}
	}
	if c.DiskSoftPercent < 1 || c.DiskHardPercent <= c.DiskSoftPercent || c.DiskHardPercent > 100 {
		return c, errors.New("invalid stream disk watermarks")
	}
	if c.DedupSeconds < 1 || c.RetentionSeconds < c.DedupSeconds || c.RetentionSeconds > 365*86400 {
		return c, errors.New("invalid stream retention")
	}
	return c, nil
}
func ms(v int) time.Duration { return time.Duration(v) * time.Millisecond }
func (c Config) backoff(attempt int) time.Duration {
	return min(ms(c.MaxBackoffMS), ms(c.BackoffMS)*(1<<min(max(attempt-1, 0), 19)))
}
func FallbackInterval() time.Duration {
	c, err := Load(os.Getenv("INFRAPRO_EVENTS_FILE"))
	if err != nil || c.Mode == "off" {
		return 0
	}
	return ms(c.FallbackScanMS)
}
