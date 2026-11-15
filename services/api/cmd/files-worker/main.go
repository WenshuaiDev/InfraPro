package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if e := run(); e != nil {
		slog.Error("file worker stopped; inspect files status")
		os.Exit(1)
	}
}
func run() (resultErr error) {
	reporter := diagnosticreport.FromEnv()
	defer func() {
		if resultErr != nil {
			reporter.Failed("dependency_unavailable")
		}
		reporter.Close()
	}()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	flags := flag.NewFlagSet("files-worker", flag.ContinueOnError)
	allowUnconfigured := flags.Bool("allow-unconfigured", false, "keep the local development process idle when file storage is not configured")
	if e := flags.Parse(os.Args[1:]); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected file worker arguments")
	}
	db, o, e := config.LoadFiles(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	if o.ObjectEndpoint == "" && *allowUnconfigured {
		reporter.Unconfigured()
		slog.Info("file worker idle; ordinary file storage is not configured")
		<-ctx.Done()
		return nil
	}
	pool, e := pgxpool.New(ctx, db.URL)
	if e != nil {
		return e
	}
	defer pool.Close()
	if e = migrations.Check(ctx, pool); e != nil {
		return e
	}
	a := access.New(pool, slog.Default())
	s := runtime.Compose(a).Files.WithOptions(o).WithChecker(files.SocketChecker{Socket: o.CheckerSocket, Timeout: o.CheckerTimeout})
	objects, e := blobstore.Open(o.ObjectEndpoint, o.ObjectAccessKey, o.ObjectSecretKey, o.Bucket, "files/v1")
	if e != nil {
		return e
	}
	s.WithStorage(objects)
	reporter.Ready()
	slog.Info("file worker ready", "node", o.NodeID)
	return s.RunWorker(ctx, o.NodeID+":"+uuid.NewString(), reporter)
}
