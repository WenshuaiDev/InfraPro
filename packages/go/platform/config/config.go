package config

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	foundation "github.com/WenshuaiDev/InfraPro/packages/go/foundation/config"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	foundation.Config
	DatabaseURL, RedisURL, RedisKeyPrefix                                                     string
	PublicConsoleURL, SupportContact                                                          string
	TrustedOrigins                                                                            []string
	TrustedProxies                                                                            []string
	CSRFKey                                                                                   []byte
	CookieSecure                                                                              bool
	SessionIdle, SessionAbsolute, SessionCacheTTL, InvitationTTL, ResetTTL                    time.Duration
	AuditRetention                                                                            time.Duration
	InvitationRate, InvitationLimit, IPRequestLimit, CredentialRequestLimit, PasswordFailures int
}

type input struct {
	foundation.Input       `yaml:",inline"`
	DatabaseURL            string `yaml:"database_url"`
	RedisURL               string `yaml:"redis_url"`
	RedisKeyPrefix         string `yaml:"redis_key_prefix"`
	PublicConsoleURL       string `yaml:"public_console_url"`
	DeprecatedScreenURL    string `yaml:"public_screen_url"` // Decode old strict YAML without using it at runtime.
	TrustedOrigins         string `yaml:"trusted_origins"`
	TrustedProxies         string `yaml:"trusted_proxies"`
	CSRFKey                string `yaml:"csrf_auth_key"`
	CookieSecure           string `yaml:"cookie_secure"`
	SessionIdle            string `yaml:"session_idle_timeout"`
	SessionAbsolute        string `yaml:"session_absolute_timeout"`
	SessionCacheTTL        string `yaml:"session_cache_ttl"`
	InvitationTTL          string `yaml:"invitation_ttl"`
	ResetTTL               string `yaml:"password_reset_ttl"`
	SupportContact         string `yaml:"support_contact"`
	AuditRetention         string `yaml:"audit_retention"`
	InvitationRate         string `yaml:"invitation_rate"`
	InvitationLimit        string `yaml:"invitation_limit"`
	IPRequestLimit         string `yaml:"ip_request_limit"`
	CredentialRequestLimit string `yaml:"credential_request_limit"`
	PasswordFailures       string `yaml:"password_failures"`
}

func loadInput(path string) (input, error) {
	raw := input{Input: foundation.Defaults(), RedisKeyPrefix: "infrapro:dev:identity:v1:",
		PublicConsoleURL: "http://127.0.0.1:5173/",
		TrustedOrigins:   "http://127.0.0.1:5173", CookieSecure: "false",
		SessionIdle: "2h", SessionAbsolute: "12h", SessionCacheTTL: "5m", InvitationTTL: "24h", ResetTTL: "30m",
		AuditRetention: "2160h", InvitationRate: "5", InvitationLimit: "20", IPRequestLimit: "120", CredentialRequestLimit: "20", PasswordFailures: "5"}
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return input{}, fmt.Errorf("open config: %w", err)
		}
		defer f.Close()
		d := yaml.NewDecoder(f)
		d.KnownFields(true)
		if err = d.Decode(&raw); err != nil {
			return input{}, fmt.Errorf("decode config: %w", err)
		}
	}
	if _, legacyEnv := os.LookupEnv("INFRAPRO_PUBLIC_SCREEN_URL"); legacyEnv || raw.DeprecatedScreenURL != "" {
		slog.Warn("public_screen_url / INFRAPRO_PUBLIC_SCREEN_URL is deprecated and ignored; remove it after migrating to the single console application")
	}
	v := reflect.ValueOf(&raw).Elem()
	typ := v.Type()
	for i := 1; i < v.NumField(); i++ {
		if typ.Field(i).Tag.Get("yaml") == "public_screen_url" {
			continue
		}
		if value, ok := os.LookupEnv("INFRAPRO_" + strings.ToUpper(typ.Field(i).Tag.Get("yaml"))); ok {
			v.Field(i).SetString(value)
		}
	}
	return raw, nil
}

// Database contains only settings required by project database maintenance.
type Database struct{ URL string }

func LoadDatabase(path string) (Database, error) {
	raw, err := loadInput(path)
	if err != nil {
		return Database{}, err
	}
	if raw.DatabaseURL == "" {
		return Database{}, fmt.Errorf("database_url is required")
	}
	if err = validateConnectionURL("database_url", raw.DatabaseURL); err != nil {
		return Database{}, err
	}
	return Database{URL: raw.DatabaseURL}, nil
}

func validateConnectionURL(name, value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (name == "database_url" && u.Scheme != "postgres" && u.Scheme != "postgresql") || (name == "redis_url" && u.Scheme != "redis" && u.Scheme != "rediss") {
		return fmt.Errorf("invalid %s", name)
	}
	return nil
}

func Load(path string) (Config, error) {
	raw, err := loadInput(path)
	if err != nil {
		return Config{}, err
	}
	base, err := foundation.Resolve(raw.Input)
	if err != nil {
		return Config{}, err
	}
	c := Config{Config: base, DatabaseURL: raw.DatabaseURL, RedisURL: raw.RedisURL, RedisKeyPrefix: raw.RedisKeyPrefix, PublicConsoleURL: raw.PublicConsoleURL, SupportContact: strings.TrimSpace(raw.SupportContact)}
	for name, value := range map[string]string{"database_url": c.DatabaseURL, "redis_url": c.RedisURL, "support_contact": c.SupportContact} {
		if value == "" {
			return c, fmt.Errorf("%s is required", name)
		}
	}
	if !strings.HasPrefix(c.RedisKeyPrefix, "infrapro:") || !strings.HasSuffix(c.RedisKeyPrefix, ":identity:v1:") || strings.ContainsAny(c.RedisKeyPrefix, "*?[] \n") {
		return c, fmt.Errorf("redis_key_prefix must be infrapro:<environment>:identity:v1:")
	}
	for name, value := range map[string]string{"database_url": c.DatabaseURL, "redis_url": c.RedisURL} {
		if e := validateConnectionURL(name, value); e != nil {
			return c, e
		}
	}
	for name, value := range map[string]string{"public_console_url": c.PublicConsoleURL} {
		u, e := url.Parse(value)
		if e != nil || u.Scheme != "http" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return c, fmt.Errorf("%s must be a full HTTP application base URL", name)
		}
	}
	for _, origin := range strings.Split(raw.TrustedOrigins, ",") {
		origin = strings.TrimSpace(origin)
		u, e := url.Parse(origin)
		if e != nil || u.Scheme != "http" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return c, fmt.Errorf("trusted_origins must contain exact HTTP origins without paths")
		}
		c.TrustedOrigins = append(c.TrustedOrigins, origin)
	}
	consoleURL, _ := url.Parse(c.PublicConsoleURL)
	for _, site := range []*url.URL{consoleURL} {
		matched := false
		for _, origin := range c.TrustedOrigins {
			if origin == site.Scheme+"://"+site.Host {
				matched = true
			}
		}
		if !matched {
			return c, fmt.Errorf("trusted_origins must include the console application origin")
		}
	}
	if raw.TrustedProxies != "" {
		c.TrustedProxies = strings.Split(raw.TrustedProxies, ",")
		for _, proxy := range c.TrustedProxies {
			if _, e := netip.ParsePrefix(proxy); e != nil {
				if _, e = netip.ParseAddr(proxy); e != nil {
					return c, fmt.Errorf("trusted_proxies must contain IP addresses or CIDRs")
				}
			}
		}
	}
	c.CSRFKey, err = hex.DecodeString(raw.CSRFKey)
	if err != nil || len(c.CSRFKey) != 32 {
		return c, fmt.Errorf("csrf_auth_key must be 64 hexadecimal characters (32 random bytes)")
	}
	c.CookieSecure, err = strconv.ParseBool(raw.CookieSecure)
	if err != nil || c.CookieSecure {
		return c, fmt.Errorf("cookie_secure must be false for this HTTP deployment")
	}
	for name, pair := range map[string]struct {
		raw string
		dst *time.Duration
	}{
		"session_idle_timeout": {raw.SessionIdle, &c.SessionIdle}, "session_absolute_timeout": {raw.SessionAbsolute, &c.SessionAbsolute}, "session_cache_ttl": {raw.SessionCacheTTL, &c.SessionCacheTTL}, "invitation_ttl": {raw.InvitationTTL, &c.InvitationTTL}, "password_reset_ttl": {raw.ResetTTL, &c.ResetTTL}, "audit_retention": {raw.AuditRetention, &c.AuditRetention},
	} {
		*pair.dst, err = time.ParseDuration(pair.raw)
		if err != nil || *pair.dst <= 0 {
			return c, fmt.Errorf("%s must be a positive duration", name)
		}
	}
	for name, pair := range map[string]struct {
		raw string
		dst *int
	}{"invitation_rate": {raw.InvitationRate, &c.InvitationRate}, "invitation_limit": {raw.InvitationLimit, &c.InvitationLimit}, "ip_request_limit": {raw.IPRequestLimit, &c.IPRequestLimit}, "credential_request_limit": {raw.CredentialRequestLimit, &c.CredentialRequestLimit}, "password_failures": {raw.PasswordFailures, &c.PasswordFailures}} {
		*pair.dst, err = strconv.Atoi(pair.raw)
		if err != nil || *pair.dst < 1 {
			return c, fmt.Errorf("%s must be a positive integer", name)
		}
	}
	return c, nil
}
