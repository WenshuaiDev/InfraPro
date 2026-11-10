package command

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"golang.org/x/term"
)

func (a *App) account(ctx context.Context, args []string) error {
	if len(args) == 0 || (len(args) == 1 && isHelp(args[0])) {
		return a.help(accountHelp)
	}
	cmd := args[0]
	switch cmd {
	case "bootstrap", "find", "reset-link", "disable", "enable", "audit", "revoke-access":
	default:
		return fmt.Errorf("未知账号命令 %q；使用 infrapro account --help 查看帮助", cmd)
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	var username, reason, cursor string
	var emergency bool
	switch cmd {
	case "find", "reset-link", "disable", "enable":
		fs.StringVar(&username, "username", "", "目标用户名（必填）")
	}
	switch cmd {
	case "reset-link", "disable", "enable", "revoke-access":
		fs.StringVar(&reason, "reason", "", "维护操作原因（必填）")
	}
	if cmd == "disable" {
		fs.BoolVar(&emergency, "emergency", false, "应急停用（允许暂时失去平台或租户管理员）")
	}
	if cmd == "audit" {
		fs.StringVar(&cursor, "cursor", "", "审计分页位置")
	}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("不接受位置参数；密码只能通过终端交互输入")
	}
	cfg, e := config.Load(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	resources, e := runtime.Open(ctx, cfg, slog.Default(), false)
	if e != nil {
		return e
	}
	defer resources.Close()
	s := resources.Identity
	m := identity.Meta{Reason: reason, UserAgent: "infrapro"}
	output := func(v any) error {
		encoder := json.NewEncoder(a.out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(v)
	}
	switch cmd {
	case "bootstrap":
		done, e := s.Initialized(ctx)
		if e != nil {
			return fmt.Errorf("无法读取初始化状态")
		}
		if done {
			return fmt.Errorf("系统已初始化，请使用账号恢复流程")
		}
		if a.in == nil || !term.IsTerminal(int(a.in.Fd())) {
			return fmt.Errorf("初始化必须在交互终端运行，密码不会从管道读取")
		}
		reader := bufio.NewReader(a.in)
		read := func(prompt string) (string, error) {
			fmt.Fprint(a.out, prompt)
			v, e := reader.ReadString('\n')
			return strings.TrimRight(v, "\r\n"), e
		}
		name, e := read("用户名：")
		if e != nil {
			return e
		}
		display, e := read("显示名称（留空使用用户名）：")
		if e != nil {
			return e
		}
		fmt.Fprint(a.out, "密码（8–128 字符，不回显）：")
		first, e := term.ReadPassword(int(a.in.Fd()))
		fmt.Fprintln(a.out)
		if e != nil {
			return e
		}
		fmt.Fprint(a.out, "再次输入密码：")
		second, e := term.ReadPassword(int(a.in.Fd()))
		fmt.Fprintln(a.out)
		if e != nil {
			return e
		}
		if string(first) != string(second) {
			return fmt.Errorf("两次密码不一致")
		}
		_, e = s.Bootstrap(ctx, name, display, string(first), m)
		clear(first)
		clear(second)
		if e != nil {
			return e
		}
		fmt.Fprintln(a.out, "首个账号创建成功，请通过网页登录。")
		return nil
	case "find":
		a, e := s.Find(ctx, username)
		if e != nil {
			return e
		}
		return output(a)
	case "audit":
		a, e := s.Audit(ctx, cursor)
		if e != nil {
			return e
		}
		return output(a)
	case "reset-link":
		if !isTerminalOutput(a.out) {
			return fmt.Errorf("重置链接仅在受控交互终端展示，请勿重定向到日志或文件")
		}
		link, e := s.ResetLink(ctx, username, m)
		if e != nil {
			return e
		}
		fmt.Fprintln(a.out, "请先核验本人身份，再通过可信渠道交付；生成链接未退出任何设备。")
		return output(link)
	case "disable", "enable":
		if e = s.SetStatus(ctx, username, cmd == "enable", emergency, m); e != nil {
			return e
		}
	case "revoke-access":
		if e = s.RestoreRevoke(ctx, m); e != nil {
			return e
		}
	default:
		return fmt.Errorf("未知账号命令")
	}
	cleanup, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if e = s.DrainInvalidations(cleanup); e != nil {
		fmt.Fprintln(a.errOut, "数据库变更已生效；缓存清理未完成，将由 API 后台任务重试。")
	}
	if cmd == "revoke-access" {
		if e = s.ClearSessionCache(cleanup); e != nil {
			fmt.Fprintln(a.errOut, "数据库访问凭证已撤销；对应环境的会话缓存清理仍需重试。")
		}
	}
	fmt.Fprintln(a.out, "操作完成，历史会话和链接不会因恢复账号而重新生效。")
	return nil
}
