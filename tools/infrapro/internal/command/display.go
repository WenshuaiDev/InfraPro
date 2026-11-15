package command

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
)

func (a *App) display(ctx context.Context, args []string) error {
	if len(args) != 1 || isHelp(args[0]) {
		return a.help("大屏运维：infrapro display backup-manifest | verify-storage | reclaim | restore-invalidate\n恢复数据库和私有对象后，启动服务之前执行 restore-invalidate；旧会话与终端需重新配对。\n")
	}
	db, storage, e := config.LoadDisplayWorker(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, db.URL)
	if e != nil {
		return e
	}
	defer pool.Close()
	retention, e := config.LoadDisplayRetention(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	options := display.DefaultOptions()
	options.EventRetention = retention
	s := display.New(access.New(pool, slog.Default())).WithOptions(options)
	store, e := blobstore.Open(storage.ObjectEndpoint, storage.ObjectAccessKey, storage.ObjectSecretKey, storage.Bucket, "display")
	if e != nil {
		return e
	}
	s.WithStorage(store)
	var out any
	switch args[0] {
	case "backup-manifest":
		out, e = s.BackupManifest(ctx)
	case "verify-storage":
		out, e = s.VerifyStorage(ctx)
	case "reclaim":
		e = s.Maintain(ctx)
		out = map[string]bool{"completed": e == nil}
	case "restore-invalidate":
		out, e = s.InvalidateAfterRestore(ctx)
	default:
		return fmt.Errorf("unknown display command")
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(a.out).Encode(out)
}
