package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
)

func (a *App) tasks(ctx context.Context, args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		return a.help("任务索引：tasks status | backfill --source all|module:kind | reconcile --source all|module:kind | replay-handoff --id UUID --reason 原因 | verify | restore-epoch --reason 原因\n仅同步元数据；不执行或重试业务任务。\n")
	}
	cfg, o, e := config.LoadTasks(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, cfg.URL)
	if e != nil {
		return e
	}
	defer pool.Close()
	index := runtime.ComposeTaskIndexer(access.New(pool, slog.Default())).WithOptions(tasks.Options(o))
	var out any
	flags := flag.NewFlagSet("tasks "+args[0], flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	source := flags.String("source", "all", "来源 module:kind")
	id := flags.String("id", "", "交接编号")
	reason := flags.String("reason", "", "人工理由")
	if e = flags.Parse(args[1:]); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected task command arguments")
	}
	switch args[0] {
	case "status":
		out, e = index.Diagnostics(ctx)
	case "backfill", "reconcile":
		e = index.Backfill(ctx, *source)
		out = map[string]bool{"reconciled": e == nil}
	case "replay-handoff":
		e = index.Replay(ctx, *id, *reason)
		out = map[string]bool{"requeued": e == nil}
	case "verify":
		e = index.Verify(ctx)
		out = map[string]bool{"verified": e == nil}
	case "restore-epoch":
		e = index.RestoreEpoch(ctx, *reason)
		out = map[string]bool{"requires_reconciliation": e == nil}
	default:
		return fmt.Errorf("unknown task command")
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(a.out).Encode(out)
}
