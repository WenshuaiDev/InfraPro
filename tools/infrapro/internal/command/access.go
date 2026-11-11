package command

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
)

const accessHelp = `权限维护：infrapro access <子命令>
  sync [--dry-run] 同步当前发布的权限目录与内置角色；--dry-run 只读预览差异及影响
  check           只读检查目录一致性
  recover-admin   --username <已有用户名> --reason <原因>，添加管理员角色，不恢复账号或会话
  catalog         输出代码目录，供构建生成器使用；不连接数据库
`

func (a *App) access(ctx context.Context, args []string) error {
	if len(args) == 0 || (len(args) == 1 && isHelp(args[0])) {
		return a.help(accessHelp)
	}
	cmd := args[0]
	if cmd != "sync" && cmd != "check" && cmd != "recover-admin" && cmd != "catalog" {
		return fmt.Errorf("未知权限命令 %q", cmd)
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	var username, reason string
	var dryRun bool
	if cmd == "sync" {
		fs.BoolVar(&dryRun, "dry-run", false, "只读预览目录、角色及租户影响")
	}
	if cmd == "recover-admin" {
		fs.StringVar(&username, "username", "", "已有账号用户名")
		fs.StringVar(&reason, "reason", "", "恢复原因")
	}
	if e := fs.Parse(args[1:]); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("不接受额外参数")
	}
	if cmd == "catalog" {
		return json.NewEncoder(a.out).Encode(access.Catalog())
	}
	if cmd == "recover-admin" {
		if username == "" {
			return fmt.Errorf("必须填写 --username")
		}
		if e := access.ValidReason(reason); e != nil {
			return e
		}
	}
	cfg, e := config.LoadDatabase(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, cfg.URL)
	if e != nil {
		return fmt.Errorf("数据库配置无效")
	}
	defer pool.Close()
	if e = migrations.Check(ctx, pool); e != nil {
		return e
	}
	s := access.New(pool, slog.Default())
	switch cmd {
	case "sync":
		if dryRun {
			v, err := s.SyncPreview(ctx)
			if err != nil {
				return err
			}
			return json.NewEncoder(a.out).Encode(v)
		}
		e = s.Sync(ctx)
	case "check":
		e = s.Check(ctx)
	case "recover-admin":
		e = s.RecoverAdmin(ctx, username, reason)
	}
	if e != nil {
		var ae *access.Error
		if errors.As(e, &ae) {
			return ae
		}
		return fmt.Errorf("权限维护失败：%w", e)
	}
	_, e = fmt.Fprintln(a.out, "权限维护完成。")
	return e
}
