// Development-only bootstrap for the dedicated synthetic acceptance stack.
// Normal maintenance belongs to the infrapro CLI. This program refuses any
// database, object endpoint or file outside the local acceptance configuration.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg, e := config.Load("")
	if e != nil {
		return e
	}
	u, e := url.Parse(cfg.DatabaseURL)
	if e != nil || u.Host != "127.0.0.1:58432" || u.Path != "/acceptance_local" || cfg.Monitoring.ObjectEndpoint != "http://127.0.0.1:61333" || os.Getenv("INFRAPRO_LOCAL_SYNTHETIC") != "1" {
		return fmt.Errorf("requires the dedicated synthetic local stack")
	}
	for _, store := range []struct {
		endpoint, access, secret string
		buckets                  []string
	}{
		{"127.0.0.1:61333", "acceptance-admin", "acceptance-admin-test-only", []string{"acceptance-source", "acceptance-ch"}},
		{"127.0.0.1:61334", "acceptance-backup-admin", "acceptance-backup-admin-test-only", []string{"acceptance-backups"}},
	} {
		c, e := minio.New(store.endpoint, &minio.Options{Creds: credentials.NewStaticV4(store.access, store.secret, ""), BucketLookup: minio.BucketLookupPath})
		if e != nil {
			return e
		}
		for _, name := range store.buckets {
			exists, e := c.BucketExists(ctx, name)
			if e != nil {
				return e
			}
			if !exists {
				if e = c.MakeBucket(ctx, name, minio.MakeBucketOptions{}); e != nil {
					return e
				}
			}
		}
	}
	// Account initialization does not depend on the CH schema initialization.
	cfg.Monitoring.Replicas = nil
	r, e := runtime.Open(ctx, cfg, slog.Default(), true)
	if e != nil {
		return e
	}
	defer r.Close()
	done, e := r.Identity.Initialized(ctx)
	if e != nil || done {
		return e
	}
	path := filepath.Join(".artifacts", "monitoring-local", "login.json")
	raw, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	var user struct{ Username, Password string }
	if e = json.Unmarshal(raw, &user); e != nil {
		return e
	}
	_, e = r.Identity.Bootstrap(ctx, user.Username, "本机合成验收管理员", user.Password, identity.Meta{Reason: "首次初始化独立本机合成验收库", UserAgent: "monitoring-local-fixture"})
	return e
}
