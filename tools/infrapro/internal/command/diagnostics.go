package command

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/services/diagnostics/maintenance"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
	"golang.org/x/term"
)

type diagnosticSlot struct {
	ID   string `json:"id"`
	Host string `json:"host"`
	Node string `json:"diagnostic_node"`
}

func (s diagnosticSlot) nodeID() string {
	if s.Node != "" {
		return s.Node
	}
	if s.Host == "" || settings.ValidID(s.Host) {
		return s.Host
	}
	hash := sha256.Sum256([]byte(s.Host))
	return fmt.Sprintf("node-%x", hash[:8])
}

func loadDiagnosticSlots(path string, options *settings.SplitOptions) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	var roles map[string]json.RawMessage
	var instances map[string][]diagnosticSlot
	if d.Decode(&roles) != nil || d.Decode(new(any)) != io.EOF || roles == nil {
		return false
	}
	if slots, ok := roles["replica_slots"]; ok {
		var version int
		if json.Unmarshal(roles["version"], &version) != nil || version != 1 {
			return false
		}
		if listeners, ok := roles["tcp_proxies"]; ok && !loadProxyListeners(listeners, options) {
			return false
		}
		if entries, ok := roles["instances"]; ok && json.Unmarshal(entries, &instances) != nil {
			return false
		}
		roles = nil
		if json.Unmarshal(slots, &roles) != nil || roles == nil {
			return false
		}
	}
	options.ReplicaNodes, options.ReplicaSlots = map[string]string{}, map[string][]string{}
	for role, rawEntries := range roles {
		var entries []json.RawMessage
		if json.Unmarshal(rawEntries, &entries) != nil || len(entries) == 0 {
			return false
		}
		// These identities belong to the independent diagnosis configuration
		// or a one-shot CLI. The deployment inventory uses the binary names.
		if role == "diagnostics" || role == "observer" || role == "infrapro" {
			if len(entries) != 1 {
				return false
			}
			continue
		}
		nodes := make(map[string]string, len(instances[role]))
		for _, slot := range instances[role] {
			nodes[slot.ID] = slot.nodeID()
		}
		for _, raw := range entries {
			var item diagnosticSlot
			if json.Unmarshal(raw, &item.ID) != nil {
				v := json.NewDecoder(bytes.NewReader(raw))
				v.DisallowUnknownFields()
				if v.Decode(&item) != nil {
					return false
				}
			}
			if !settings.ValidID(item.ID) {
				return false
			}
			options.ReplicaSlots[role] = append(options.ReplicaSlots[role], item.ID)
			node := nodes[item.ID]
			if declared := item.nodeID(); declared != "" {
				node = declared
			}
			if node != "" {
				options.ReplicaNodes[item.ID] = node
			}
		}
	}
	return true
}

func loadProxyListeners(data []byte, options *settings.SplitOptions) bool {
	type address struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	var proxies []struct {
		Role           string    `json:"role"`
		Protocol       string    `json:"protocol"`
		Listen         address   `json:"listen"`
		Backends       []address `json:"backends"`
		MaxConnections int       `json:"max_connections"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&proxies) != nil || d.Decode(new(any)) != io.EOF || len(proxies) > 16 {
		return false
	}
	options.ProxyListeners = nil
	for _, p := range proxies {
		host := p.Listen.Host
		if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
			if ip.To4() != nil {
				host = "127.0.0.1"
			} else {
				host = "::1"
			}
		}
		options.ProxyListeners = append(options.ProxyListeners, settings.ProxyListener{Role: p.Role, Protocol: p.Protocol, Host: host, Port: p.Listen.Port})
	}
	return true
}

// macOS /usr/bin/python3 may launch a framework executable, and pyenv can
// dispatch through a shim. Resolve the configured local interpreter once at
// initialization. The observer only uses its normal OS process checks.
func localProxyExecutable(ctx context.Context, configured string) (string, error) {
	if configured != "" {
		return filepath.EvalSymlinks(configured)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const locate = `import os, sys
if sys.platform == "darwin":
    import ctypes
    path = ctypes.create_string_buffer(4096)
    if ctypes.CDLL("/usr/lib/libproc.dylib").proc_pidpath(os.getpid(), path, len(path)) <= 0:
        raise SystemExit(1)
    print(os.fsdecode(path.value))
else:
    print(os.path.realpath("/proc/self/exe") if os.path.exists("/proc/self/exe") else os.path.realpath(sys.executable))`
	out, err := exec.CommandContext(ctx, "python3", "-I", "-S", "-c", locate).Output()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(strings.TrimSpace(string(out)))
}

const diagnosticsHelp = `独立运行诊断：infrapro diagnostics <组> <操作>
  config init --directory <新目录> --instance-id <标识> --profile native|docker|mixed [--topology split --bin-directory <目录> --report-directory <目录> --local-reports --proxy-listeners <文件> --proxy-executable <程序> --container-report-directory <目录> --container-roles <角色名列表> --runtime-uid <uid> --role-uids <文件> --replicas <文件> --replica-nodes <文件> --broker-replicas 1|3 --compat-api --compat-workers]
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
	topology := f.String("topology", "standalone", "standalone 或 split")
	binDir := f.String("bin-directory", "/opt/infrapro/current/bin", "业务程序目录")
	reportDir := f.String("report-directory", "", "各角色报告的根目录")
	flat := f.Bool("local-reports", false, "本机启动器以 <角色或副本>.json 保存报告")
	proxyListeners := f.String("proxy-listeners", "", "已生效的 replica-proxy 监听清单")
	python := f.String("proxy-executable", "", "本机 TCP 转发器实际 Python 可执行路径")
	containerReportDir := f.String("container-report-directory", "", "供容器观察器读取的宿主报告目录")
	containerRoles := f.String("container-roles", "", "混合部署中的容器角色，以逗号分隔")
	uid := f.Int("runtime-uid", 10001, "业务进程默认 UID")
	compat := f.Bool("compat-api", false, "兼容 API 已启用时登记探针")
	compatWorkers := f.Bool("compat-workers", false, "登记已启用的两个兼容 worker")
	roleUIDs := f.String("role-uids", "", "每个角色的 UID 映射")
	nodes := f.String("replica-nodes", "", "副本 ID 至观察器节点的 JSON 映射")
	replicas := f.String("replicas", "", "部署角色到副本 ID 或 {id,host} 列表的 JSON 文件")
	brokers := f.Int("broker-replicas", 0, "代理副本数 1 或 3")
	brokerPID := f.String("broker-pid-file", "", "本机单个 broker 的受保护 PID 文件")
	brokerBin := f.String("broker-executable", "", "本机 broker 实际可执行程序")
	brokerPort := f.Int("broker-monitor-port", 8222, "broker 本机 HTTP 健康端口")
	username := f.String("username", "", "独立诊断账号")
	enabled := f.String("enabled", "", "true 或 false")
	node := f.String("node-id", "", "节点")
	output := f.String("output-file", "", "新密钥文件")
	probe := f.Bool("probe", false, "执行固定检查")
	if f.Parse(args[2:]) != nil || f.NArg() != 0 {
		return bad()
	}
	allowed := map[string]map[string]bool{
		"config.init": {"directory": true, "instance-id": true, "profile": true, "topology": true, "bin-directory": true, "report-directory": true, "local-reports": true, "proxy-executable": true, "proxy-listeners": true, "container-report-directory": true, "container-roles": true, "runtime-uid": true, "role-uids": true, "replicas": true, "replica-nodes": true, "broker-replicas": true, "broker-pid-file": true, "broker-executable": true, "broker-monitor-port": true, "compat-api": true, "compat-workers": true}, "config.validate": {"server": true, "probe": true},
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
		var split []settings.SplitOptions
		switch *topology {
		case "standalone":
		case "split":
			opt := settings.SplitOptions{
				BinDirectory: *binDir, ReportDirectory: *reportDir, FlatLocalReports: *flat, ContainerReportDirectory: *containerReportDir,
				RuntimeUID: *uid, EnabledCompat: *compat, EnabledCompatWorkers: *compatWorkers,
				BrokerReplicas: *brokers, BrokerPIDFile: *brokerPID, BrokerExecutable: *brokerBin, BrokerMonitorPort: *brokerPort,
			}
			if *containerRoles != "" {
				opt.ContainerRoles = strings.Split(*containerRoles, ",")
			}
			if *replicas != "" && !loadDiagnosticSlots(*replicas, &opt) {
				return bad()
			}
			if *proxyListeners != "" {
				f, err := os.Open(*proxyListeners)
				if err != nil {
					return bad()
				}
				data, err := io.ReadAll(io.LimitReader(f, 128<<10))
				f.Close()
				if err != nil || !loadProxyListeners(data, &opt) {
					return bad()
				}
			}
			if *flat && len(opt.ProxyListeners) != 0 {
				opt.ProxyExecutable, err = localProxyExecutable(ctx, *python)
				if err != nil {
					return bad()
				}
			}
			for _, input := range []struct {
				path   string
				target any
			}{{*roleUIDs, &opt.RoleUIDs}, {*nodes, &opt.ReplicaNodes}} {
				if input.path == "" {
					continue
				}
				f, err := os.Open(input.path)
				if err != nil {
					return bad()
				}
				defer f.Close()
				d := json.NewDecoder(io.LimitReader(f, 64<<10))
				if d.Decode(input.target) != nil || d.Decode(new(any)) != io.EOF {
					return bad()
				}
			}
			split = append(split, opt)
		default:
			return bad()
		}
		_, err = settings.Init(*directory, *instance, *profile, diag, split...)
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
