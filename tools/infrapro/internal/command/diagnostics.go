package command

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/maintenance"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"golang.org/x/term"
)

const diagnosticsHelp = `独立运行诊断：infrapro diagnostics <组> <操作>
  config init --directory <新目录> --instance-id <标识> --profile native|docker|mixed
  config validate --server <server.json> [--probe]
  auth init|add|reset-password --server <server.json> --username <账号>
  auth set-enabled --server <server.json> --username <账号> --enabled true|false
  observer-key issue|rotate --server <server.json> --node-id <节点> --output-file <新文件>
  observer-key revoke --server <server.json> --node-id <节点>
  state inspect|compact --server <server.json>
独立 JSON 配置，不读取业务数据库配置。密码仅通过交互终端输入；密钥仅写入 0600 文件。
validate 退出码：0 通过、2 配置失败、3 固定检查未全部通过。
compact 必须先停止本实例诊断 server，保留所有上报顺序记录。
`

type diagnosticExit struct {
	code   int
	reason string
}

func (e diagnosticExit) Error() string { return e.reason }
func (e diagnosticExit) ExitCode() int { return e.code }

func (a *App) diagnostics(ctx context.Context, args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		return a.help(diagnosticsHelp)
	}
	bad := func() error { return diagnosticExit{2, "configuration_invalid"} }
	if len(args) < 2 {
		return bad()
	}
	group, action := args[0], args[1]
	f := flag.NewFlagSet("diagnostics", flag.ContinueOnError)
	// flag errors echo unknown argument values. Keep them out of diagnostics logs.
	f.SetOutput(io.Discard)
	server := f.String("server", "", "独立配置文件")
	directory := f.String("directory", "", "新配置目录")
	instance := f.String("instance-id", "", "实例标识")
	profile := f.String("profile", "native", "部署方式")
	username := f.String("username", "", "独立诊断账号")
	enabled := f.String("enabled", "", "true 或 false")
	node := f.String("node-id", "", "节点")
	output := f.String("output-file", "", "新密钥文件")
	probe := f.Bool("probe", false, "执行固定检查")
	if f.Parse(args[2:]) != nil || f.NArg() != 0 {
		return bad()
	}
	allowed := map[string]map[string]bool{
		"config.init": {"directory": true, "instance-id": true, "profile": true}, "config.validate": {"server": true, "probe": true},
		"auth.init": {"server": true, "username": true}, "auth.add": {"server": true, "username": true}, "auth.reset-password": {"server": true, "username": true}, "auth.set-enabled": {"server": true, "username": true, "enabled": true},
		"observer-key.issue": {"server": true, "node-id": true, "output-file": true}, "observer-key.rotate": {"server": true, "node-id": true, "output-file": true}, "observer-key.revoke": {"server": true, "node-id": true}, "state.inspect": {"server": true}, "state.compact": {"server": true},
	}[group+"."+action]
	if allowed == nil {
		return bad()
	}
	valid := true
	f.Visit(func(v *flag.Flag) { valid = valid && allowed[v.Name] })
	if !valid {
		return bad()
	}
	var err error
	if group == "config" && action == "init" {
		if *directory == "" || *instance == "" {
			return bad()
		}
		exe, _ := os.Executable()
		diag := filepath.Join(filepath.Dir(exe), "infrapro-diagnostics")
		_, err = settings.Init(*directory, *instance, *profile, diag)
	} else {
		if !settings.Absolute(*server) {
			return bad()
		}
		switch group {
		case "config":
			err = settings.ValidateFiles(*server)
			if err == nil && *probe {
				var results []maintenance.ProbeResult
				var passed bool
				results, passed, err = maintenance.Probe(ctx, *server)
				if err == nil {
					if e := json.NewEncoder(a.out).Encode(results); e != nil {
						return e
					}
					if !passed {
						return diagnosticExit{3, "checks_incomplete"}
					}
				}
			}
		case "auth":
			var value *bool
			password := ""
			if action == "set-enabled" {
				if *enabled != "true" && *enabled != "false" {
					return bad()
				}
				v, _ := strconv.ParseBool(*enabled)
				value = &v
			} else {
				password, err = a.diagnosticPassword()
				if err != nil {
					return err
				}
			}
			err = settings.EditAuth(*server, action, *username, password, value)
		case "observer-key":
			err = settings.EditNodeKey(*server, action, *node, *output)
		case "state":
			if action == "compact" {
				err = maintenance.Compact(*server)
			} else {
				var result maintenance.Inspection
				result, err = maintenance.Inspect(*server)
				if err == nil {
					return json.NewEncoder(a.out).Encode(result)
				}
			}
		}
	}
	if err != nil {
		return diagnosticExit{2, settings.Code(err)}
	}
	_, err = fmt.Fprintln(a.out, "独立诊断操作已完成。")
	return err
}
func (a *App) diagnosticPassword() (string, error) {
	if a.in == nil || !term.IsTerminal(int(a.in.Fd())) {
		return "", diagnosticExit{2, "密码必须在交互终端输入，不接受管道、参数或环境变量。"}
	}
	fmt.Fprint(a.out, "诊断密码（12–128 字符，不回显）：")
	first, e := term.ReadPassword(int(a.in.Fd()))
	fmt.Fprintln(a.out)
	if e != nil {
		return "", diagnosticExit{2, "password_input_failed"}
	}
	defer clear(first)
	fmt.Fprint(a.out, "再次输入密码：")
	second, e := term.ReadPassword(int(a.in.Fd()))
	fmt.Fprintln(a.out)
	defer clear(second)
	if e != nil || !bytes.Equal(first, second) {
		return "", diagnosticExit{2, "两次密码不一致。"}
	}
	return string(first), nil
}
