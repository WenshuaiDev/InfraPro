package command

import (
	"context"
	"fmt"
	"os"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
)

func (a *App) database(ctx context.Context, args []string) error {
	if len(args) == 0 || (len(args) == 1 && isHelp(args[0])) {
		return a.help(databaseHelp)
	}
	if args[0] != "migrate" {
		return fmt.Errorf("未知数据库命令 %q；使用 infrapro database --help 查看帮助", args[0])
	}
	if len(args) == 2 && isHelp(args[1]) {
		return a.help(databaseHelp)
	}
	if len(args) != 1 {
		return fmt.Errorf("database migrate 不接受额外参数")
	}
	cfg, err := config.LoadDatabase(os.Getenv("INFRAPRO_CONFIG"))
	if err != nil {
		return err
	}
	if err = migrations.Up(ctx, cfg.URL); err != nil {
		return fmt.Errorf("迁移失败，请检查数据库连接与迁移状态")
	}
	_, err = fmt.Fprintln(a.out, "数据库迁移完成。")
	return err
}
