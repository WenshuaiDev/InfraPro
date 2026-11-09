// Package command organizes project operations by command group.
package command

import (
	"context"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

type App struct {
	in     *os.File
	out    io.Writer
	errOut io.Writer
}

func New(in *os.File, out, errOut io.Writer) *App {
	return &App{in: in, out: out, errOut: errOut}
}

const rootHelp = `InfraPro 项目统一运维工具

用法：infrapro <命令组> <子命令> [选项]

命令组：
  access      权限目录与管理员角色维护
  database    项目数据库迁移
  account     账号初始化、查找、恢复、状态维护及审计

使用 infrapro <命令组> --help 查看可用子命令。
配置：INFRAPRO_CONFIG 指定 YAML，环境变量覆盖同名配置。
密码只通过交互终端输入，不接受参数或环境变量。
`

const accountHelp = `账号维护：infrapro account <子命令> [选项]

  bootstrap       交互创建首个账号，密码不回显
  find            --username <用户名>
  reset-link      --username <用户名> --reason <核验原因>
  disable         --username <用户名> --reason <原因> [--emergency]
  enable          --username <用户名> --reason <原因>
  audit           [--cursor <分页位置>]
  revoke-access   --reason <原因>，备份恢复后撤销历史访问能力
`

const databaseHelp = `数据库维护：infrapro database <子命令>

  migrate    执行尚未应用的项目数据库迁移

本命令只需要 INFRAPRO_DATABASE_URL（或 YAML 的 database_url）。
`

func isHelp(arg string) bool { return arg == "help" || arg == "--help" || arg == "-h" }

func (a *App) help(text string) error {
	_, err := fmt.Fprint(a.out, text)
	return err
}

func isTerminalOutput(out io.Writer) bool {
	file, ok := out.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 || (len(args) == 1 && isHelp(args[0])) {
		return a.help(rootHelp)
	}
	if args[0] == "help" && len(args) == 2 {
		args = []string{args[1], "--help"}
	}
	switch args[0] {
	case "access":
		return a.access(ctx, args[1:])
	case "database":
		return a.database(ctx, args[1:])
	case "account":
		return a.account(ctx, args[1:])
	default:
		return fmt.Errorf("未知命令组 %q；使用 infrapro --help 查看帮助", args[0])
	}
}
