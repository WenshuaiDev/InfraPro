package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
)

func main() {
	if e := run(); e != nil {
		var failure *parsingcontract.Error
		if errors.As(e, &failure) && failure.Code == "schema_incompatible" {
			fmt.Fprintf(os.Stderr, "ingestion verification rejected: schema_incompatible; database must match this release (expected schema %d)\n", migrations.Version)
		} else {
			fmt.Fprintln(os.Stderr, "ingestion verification rejected; inspect protected configuration, schema, artifact coverage or object integrity")
		}
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: ingestion-operator manifest FILE | release PACKAGE | objects REPORT | reconcile CLOUD_MANIFEST (INFRAPRO_CONFIG required)")
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
	s := runtime.Compose(access.New(pool, slog.Default())).Configure(cfg).Ingestion
	switch os.Args[1] {
	case "manifest":
		m, e := s.OperatorManifest(ctx)
		if e != nil {
			return e
		}
		b, e := m.Bytes()
		if e != nil {
			return e
		}
		f, e := os.OpenFile(os.Args[2], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = f.Write(b)
		return e
	case "release":
		v, e := s.VerifyRelease(ctx, os.Args[2])
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "objects":
		v, e := s.VerifyObjects(ctx)
		if e != nil {
			return e
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		if e = os.WriteFile(os.Args[2], b, 0600); e != nil {
			return e
		}
		if v["missing_or_corrupt"] > 0 {
			return fmt.Errorf("object gaps")
		}
		return json.NewEncoder(os.Stdout).Encode(v)
	case "reconcile":
		b, e := os.ReadFile(os.Args[2])
		if e != nil {
			return e
		}
		var cloud []json.RawMessage
		if e = json.Unmarshal(b, &cloud); e != nil {
			return e
		}
		v, e := s.ReconcileManifest(ctx, cloud)
		if e != nil {
			return e
		}
		json.NewEncoder(os.Stdout).Encode(v)
		if v["gaps"]+v["conflicts"] > 0 {
			return fmt.Errorf("receipt gaps")
		}
		return nil
	}
	return fmt.Errorf("unknown action")
}
