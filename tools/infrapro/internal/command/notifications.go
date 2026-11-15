package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"time"
)

func (a *App) notifications(ctx context.Context, args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		return a.help("通知运维：infrapro notifications status | inspect --event <ID> | check [--worker --max-age 1m] | replay --event <ID> --reason <原因> | replay --handoff <ID> --reason <原因>\n只显示数量和原因码；重放仅处理失败投递，不重发已送达或主动抑制记录。\n")
	}
	cfg, _, e := config.LoadNotifications(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, cfg.URL)
	if e != nil {
		return e
	}
	defer pool.Close()
	aService := access.New(pool, slog.Default())
	s := runtime.ComposeNotifications(aService)
	var out any
	switch args[0] {
	case "status":
		diagnostics, err := s.Diagnostics(ctx)
		if err != nil {
			return err
		}
		handoffs, err := runtime.DisplayNotificationDiagnostics(ctx, aService)
		if err != nil {
			return err
		}
		out = map[string]any{"notifications": diagnostics, "display_handoffs": handoffs}
	case "inspect":
		flags := flag.NewFlagSet("notifications inspect", flag.ContinueOnError)
		flags.SetOutput(a.errOut)
		event := flags.String("event", "", "事件编号")
		if e = flags.Parse(args[1:]); e != nil {
			return e
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		out, e = s.InspectEvent(ctx, *event)
	case "check":
		flags := flag.NewFlagSet("notifications check", flag.ContinueOnError)
		flags.SetOutput(a.errOut)
		worker := flags.Bool("worker", false, "同时检查 worker 心跳")
		age := flags.Duration("max-age", time.Minute, "最大心跳间隔")
		if e = flags.Parse(args[1:]); e != nil {
			return e
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		e = s.Check(ctx)
		if e == nil && *worker {
			e = s.CheckWorker(ctx, *age)
		}
		out = map[string]bool{"ready": e == nil}
	case "replay":
		flags := flag.NewFlagSet("notifications replay", flag.ContinueOnError)
		flags.SetOutput(a.errOut)
		event := flags.String("event", "", "事件编号")
		handoff := flags.String("handoff", "", "大屏交接编号")
		reason := flags.String("reason", "", "操作原因")
		if e = flags.Parse(args[1:]); e != nil {
			return e
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		if (*event == "") == (*handoff == "") {
			return fmt.Errorf("provide exactly one --event or --handoff")
		}
		if *handoff != "" {
			e = runtime.ReplayDisplayNotification(ctx, aService, *handoff, *reason)
			out = map[string]bool{"requeued": e == nil}
		} else {
			out, e = s.Replay(ctx, *event, *reason)
		}
	default:
		return fmt.Errorf("unknown notification command")
	}
	if e != nil {
		return e
	}
	return json.NewEncoder(a.out).Encode(out)
}
