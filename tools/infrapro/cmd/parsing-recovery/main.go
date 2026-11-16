package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "parsing recovery failed; verify private configuration, package and object integrity")
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: parsing-recovery manifest FILE | verify RELEASE_DIRECTORY | reconcile RELEASE_DIRECTORY")
	}
	cfg, e := config.Load(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, cfg.DatabaseURL)
	if e != nil {
		return e
	}
	defer pool.Close()
	s := runtime.Compose(access.New(pool, slog.Default())).Configure(cfg).Parsing
	switch os.Args[1] {
	case "manifest":
		v, e := s.RecoveryManifest(ctx)
		if e != nil {
			return e
		}
		raw, e := v.Bytes()
		if e != nil {
			return e
		}
		f, e := os.OpenFile(os.Args[2], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = f.Write(raw)
		return e
	case "verify", "reconcile":
		v, e := s.VerifyRecovery(ctx, os.Args[2])
		if e != nil {
			return e
		}
		if v["missing_or_corrupt"] != 0 {
			return fmt.Errorf("incomplete objects")
		}
		if os.Args[1] == "reconcile" {
			n, e := s.ReconcileRecovery(ctx)
			if e != nil {
				return e
			}
			v["interrupted_generations"] = n
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	}
	return fmt.Errorf("unknown action")
}
