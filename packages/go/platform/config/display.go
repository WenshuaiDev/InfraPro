package config

import (
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Display struct {
	RuntimeOrigin, AssetOrigin, RuntimeAddr, AssetAddr, ShellDirectory                string
	Lease, SwitchGrace                                                                time.Duration
	MaxSessions, MaxSubscriptions, MaxQueries, MaxInstances, MaxPackageJobs, PoolSize int
	MaxResultBytes, MaxFileBytes                                                      int64
}
type displayInput struct {
	RuntimeOrigin    string `yaml:"runtime_origin"`
	AssetOrigin      string `yaml:"asset_origin"`
	RuntimeAddr      string `yaml:"runtime_addr"`
	AssetAddr        string `yaml:"asset_addr"`
	ShellDirectory   string `yaml:"shell_directory"`
	Lease            string `yaml:"lease"`
	SwitchGrace      string `yaml:"switch_grace"`
	MaxSessions      string `yaml:"max_sessions"`
	MaxSubscriptions string `yaml:"max_subscriptions"`
	MaxQueries       string `yaml:"max_queries"`
	MaxInstances     string `yaml:"max_instances"`
	MaxPackageJobs   string `yaml:"max_package_jobs"`
	PoolSize         string `yaml:"pool_size"`
	MaxResultBytes   string `yaml:"max_result_bytes"`
	MaxFileBytes     string `yaml:"max_file_bytes"`
}

func defaultDisplayInput() displayInput {
	return displayInput{"http://127.0.0.1:5180", "http://127.0.0.1:5181", "127.0.0.1:5180", "127.0.0.1:5181", "apps/display-runtime/dist", "60s", "30s", "100", "16", "8", "100", "8", "8", "8388608", "4194304"}
}
func displayEnvironment(raw *displayInput) {
	v := reflect.ValueOf(raw).Elem()
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if value, ok := os.LookupEnv("INFRAPRO_DISPLAY_" + strings.ToUpper(t.Field(i).Tag.Get("yaml"))); ok {
			v.Field(i).SetString(value)
		}
	}
}
func resolveDisplay(raw displayInput, console string, secure bool) (Display, error) {
	out := Display{RuntimeOrigin: raw.RuntimeOrigin, AssetOrigin: raw.AssetOrigin, RuntimeAddr: raw.RuntimeAddr, AssetAddr: raw.AssetAddr, ShellDirectory: raw.ShellDirectory}
	c, e := applicationURL(console, false)
	if e != nil || secure != (c.Scheme == "https") {
		return out, fmt.Errorf("display console URL and cookie_secure must agree on HTTP/HTTPS")
	}
	seen := map[string]bool{c.Scheme + "://" + c.Host: true}
	for _, origin := range []string{raw.RuntimeOrigin, raw.AssetOrigin} {
		u, e := applicationURL(origin, true)
		if e != nil || u.Scheme != c.Scheme || seen[origin] {
			return out, fmt.Errorf("display origins must be distinct exact HTTP/HTTPS origins with the console scheme")
		}
		seen[origin] = true
	}
	for _, addr := range []string{out.RuntimeAddr, out.AssetAddr} {
		if _, _, e = net.SplitHostPort(addr); e != nil {
			return out, fmt.Errorf("invalid display listen address")
		}
	}
	for _, f := range []struct {
		raw string
		dst *int
		max int
	}{{raw.MaxSessions, &out.MaxSessions, 10000}, {raw.MaxSubscriptions, &out.MaxSubscriptions, 64}, {raw.MaxQueries, &out.MaxQueries, 64}, {raw.MaxInstances, &out.MaxInstances, 100}, {raw.MaxPackageJobs, &out.MaxPackageJobs, 100}, {raw.PoolSize, &out.PoolSize, 32}} {
		v, e := strconv.Atoi(f.raw)
		if e != nil || v < 1 || v > f.max {
			return out, fmt.Errorf("invalid display resource budget")
		}
		*f.dst = v
	}
	for _, f := range []struct {
		raw string
		dst *int64
		max int64
	}{{raw.MaxResultBytes, &out.MaxResultBytes, 16 << 20}, {raw.MaxFileBytes, &out.MaxFileBytes, 4 << 20}} {
		v, e := strconv.ParseInt(f.raw, 10, 64)
		if e != nil || v < 1024 || v > f.max {
			return out, fmt.Errorf("invalid display byte budget")
		}
		*f.dst = v
	}
	out.Lease, e = time.ParseDuration(raw.Lease)
	if e != nil || out.Lease < 15*time.Second || out.Lease > 2*time.Minute {
		return out, fmt.Errorf("display lease must be 15s–2m")
	}
	out.SwitchGrace, e = time.ParseDuration(raw.SwitchGrace)
	if e != nil || out.SwitchGrace < 5*time.Second || out.SwitchGrace > time.Minute {
		return out, fmt.Errorf("display switch grace must be 5s–1m")
	}
	return out, nil
}

// Package workers can load their scoped storage configuration without opening
// personnel identity, ClickHouse, Redis or business providers.
func LoadDisplayWorker(path string) (Database, Monitoring, error) {
	db, e := LoadDatabase(path)
	if e != nil {
		return db, Monitoring{}, e
	}
	storage, e := LoadSpatialStorage(path)
	return db, storage, e
}

// LoadDisplayRetention is usable by the least-privilege worker without loading
// personnel identity or business data credentials.
func LoadDisplayRetention(path string) (time.Duration, error) {
	raw, e := loadInput(path)
	if e != nil {
		return 0, e
	}
	v := raw.AuditRetention
	if v == "" {
		v = "2160h"
	}
	d, e := time.ParseDuration(v)
	if e != nil || d < 24*time.Hour || d > 365*24*time.Hour {
		return 0, fmt.Errorf("display event retention must be 24h–8760h")
	}
	return d, nil
}
