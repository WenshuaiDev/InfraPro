// Package config loads process configuration from defaults, optional YAML and environment.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"time"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	HTTPAddr        string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
}

type fileConfig struct {
	HTTPAddr        string `yaml:"http_addr"`
	LogLevel        string `yaml:"log_level"`
	ShutdownTimeout string `yaml:"shutdown_timeout"`
}

// Load applies defaults < YAML < INFRAPRO_* environment variables.
// An empty path uses defaults and environment only.
func Load(path string) (Config, error) {
	raw := fileConfig{
		HTTPAddr:        "127.0.0.1:8080",
		LogLevel:        "info",
		ShutdownTimeout: "5s",
	}
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return Config{}, fmt.Errorf("open config: %w", err)
		}
		defer file.Close()
		decoder := yaml.NewDecoder(file)
		decoder.KnownFields(true)
		if err := decoder.Decode(&raw); err != nil {
			return Config{}, fmt.Errorf("decode config: %w", err)
		}
	}
	for key, target := range map[string]*string{
		"INFRAPRO_HTTP_ADDR":        &raw.HTTPAddr,
		"INFRAPRO_LOG_LEVEL":        &raw.LogLevel,
		"INFRAPRO_SHUTDOWN_TIMEOUT": &raw.ShutdownTimeout,
	} {
		if value, ok := os.LookupEnv(key); ok {
			*target = value
		}
	}
	_, port, err := net.SplitHostPort(raw.HTTPAddr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid http_addr %q: %w", raw.HTTPAddr, err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Config{}, fmt.Errorf("http_addr port must be between 1 and 65535")
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(raw.LogLevel)); err != nil {
		return Config{}, fmt.Errorf("invalid log_level: %w", err)
	}
	timeout, err := time.ParseDuration(raw.ShutdownTimeout)
	if err != nil || timeout <= 0 {
		return Config{}, fmt.Errorf("shutdown_timeout must be a positive duration")
	}
	return Config{HTTPAddr: raw.HTTPAddr, LogLevel: level, ShutdownTimeout: timeout}, nil
}
