package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const derivedHelp = `派生监测：infrapro monitoring derived <命令> [选项]

  check                         检查迁移、权限目录、物理布局、引擎和变化日志
  status [--tenant ID]           队列、dirty、consumer、租约和结果空间聚合状态
  reconcile [--tenant ID]        只读完整对账，不改写结果或权限
  recovery [--tenant ID]         恢复后状态及完整对账；开放写入用 monitoring restore open
  login --url ORIGIN --username NAME --session-file ABS_PATH
                                交互密码登录，生成权限 0600 的新会话文件
  pause/resume --tenant ID --channel ID --expected-revision N [--resume-mode MODE]
                                默认只预览；--apply 必须附原预览 ID/hash 和原因
  backfill --tenant ID --channel ID --expected-revision N --from-ns N --to-ns N
                                默认只预览；可用 --input ABS_PATH 提交完整依赖选择
  replay --compare --tenant ID --generation ID --apply --reason TEXT [--wait]
                                按原固定来源回放比较，不改正式结果

业务命令均需要 --session-file；可加 --project ID，严格使用该项目入口。
确认预览：--apply --preview-id ID --preview-hash HASH --expected-revision N --reason TEXT
幂等键：--request-id UUID（省略时生成并输出）；--wait 通过 API 等待已提交任务。
会话失效时重新登录。CLI 不接受账号 ID 冒充登录，不直写业务表。
存储初始化、独立备份、恢复和 worker 重启沿用 monitoring 命令组。
`

func (a *App) monitoringDerived(ctx context.Context, args []string) error {
	if len(args) == 0 || slices.ContainsFunc(args, isHelp) {
		return a.help(derivedHelp)
	}
	action := args[0]
	if !slices.Contains([]string{"check", "status", "reconcile", "recovery", "login", "pause", "resume", "backfill", "replay"}, action) {
		return fmt.Errorf("未知派生命令；使用 infrapro monitoring derived --help")
	}
	flags := flag.NewFlagSet("monitoring derived", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	var tenant, project, channel, generation, revision, preview, hash, reason, session, origin, username, requestID, from, to, input, resume, configuration, trialReceipt string
	flags.StringVar(&trialReceipt, "trial-receipt-id", "", "恢复所依据的有效试算回执")
	for _, f := range []struct {
		target     *string
		name, help string
	}{{&tenant, "tenant", "租户 ID"}, {&project, "project", "项目 ID"}, {&channel, "channel", "通道 ID"}, {&generation, "generation", "原代次 ID"}, {&revision, "expected-revision", "当前修订"}, {&preview, "preview-id", "原预览 ID"}, {&hash, "preview-hash", "原预览 hash"}, {&reason, "reason", "操作原因"}, {&session, "session-file", "受保护的登录会话文件"}, {&origin, "url", "控制台 origin"}, {&username, "username", "用户名"}, {&requestID, "request-id", "请求幂等键"}, {&from, "from-ns", "起点纳秒字符串"}, {&to, "to-ns", "终点纳秒字符串"}, {&input, "input", "完整预览 JSON 文件"}, {&resume, "resume-mode", "catch_up 或 new_start"}, {&configuration, "config", "无配置区间使用的配置 ID"}} {
		flags.StringVar(f.target, f.name, "", f.help)
	}
	var apply, wait, compare bool
	var allowWaiting bool
	flags.BoolVar(&allowWaiting, "allow-waiting", false, "确认无数据时等待输入")
	flags.BoolVar(&apply, "apply", false, "执行已预览操作")
	flags.BoolVar(&wait, "wait", false, "等待已提交任务")
	flags.BoolVar(&compare, "compare", false, "原依据回放并比较")
	if e := flags.Parse(args[1:]); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("不支持额外位置参数")
	}
	output := func(v any) error { enc := json.NewEncoder(a.out); enc.SetIndent("", "  "); return enc.Encode(v) }
	if action == "login" {
		return a.derivedLogin(ctx, origin, username, session)
	}
	if slices.Contains([]string{"check", "status", "reconcile", "recovery"}, action) {
		if apply || wait || project != "" || session != "" {
			return fmt.Errorf("运维检查为只读，不能附业务执行参数")
		}
		path := os.Getenv("INFRAPRO_CONFIG")
		db, e := config.LoadDatabase(path)
		if e != nil {
			return e
		}
		pool, e := pgxpool.New(ctx, db.URL)
		if e != nil {
			return fmt.Errorf("控制库配置无效")
		}
		defer pool.Close()
		if e = migrations.Check(ctx, pool); e != nil {
			return e
		}
		if action == "status" {
			v, e := dm.MaintenanceStatus(ctx, pool, tenant)
			if e != nil {
				return e
			}
			return output(v)
		}
		cfg, e := config.LoadMonitoring(path)
		if e != nil {
			return e
		}
		storage, e := md.OpenStorage(ctx, runtime.MonitoringStorageConfig(cfg))
		if e != nil {
			return e
		}
		defer storage.Close()
		if e = runtime.Compose(access.New(pool, slog.Default()), storage).DerivedMonitoring.Ready(ctx); e != nil {
			return e
		}
		if action == "check" {
			return output(map[string]any{"state": "passed", "versions": dm.NewRecovery(storage).Descriptor()})
		}
		v, e := dm.NewRecovery(storage).Reconcile(ctx, pool, tenant)
		if e != nil {
			return e
		}
		if action == "recovery" {
			status, e := dm.MaintenanceStatus(ctx, pool, tenant)
			if e != nil {
				return e
			}
			e = output(map[string]any{"status": status, "reconciliation": v})
			if e != nil {
				return e
			}
		} else {
			if e = output(v); e != nil {
				return e
			}
		}
		if v.State != "passed" {
			return fmt.Errorf("对账发现异常；原证据未修改，需恢复缺失依据或通过获准回算发布新代次")
		}
		return nil
	}
	if !access.ValidID(tenant) || project != "" && !access.ValidID(project) {
		return fmt.Errorf("需要有效 --tenant，可选有效 --project")
	}
	c, e := loadDerivedClient(session, origin)
	if e != nil {
		return e
	}
	base := "/api/v1/tenants/" + tenant
	if project != "" {
		base += "/projects/" + project
	}
	base += "/monitoring/derived"
	if requestID == "" {
		requestID = uuid.NewString()
	}
	if !access.ValidID(requestID) {
		return fmt.Errorf("无效 request_id")
	}
	if apply {
		if e = access.ValidReason(reason); e != nil {
			return fmt.Errorf("执行必须提供 --reason")
		}
		fmt.Fprintln(a.errOut, "request_id="+requestID)
	}
	if action == "replay" {
		if !compare || !access.ValidID(generation) {
			return fmt.Errorf("replay 需要 --compare 和有效 --generation")
		}
		if !apply {
			return output(map[string]string{"operation": "replay_compare", "generation_id": generation, "state": "ready_to_submit", "request_id": requestID})
		}
		var run dm.Run
		if e = c.request(ctx, "POST", base+"/generations/"+generation+"/replay", dm.ReplayInput{WriteMeta: dm.WriteMeta{RequestID: requestID, Reason: reason}}, &run); e != nil {
			return e
		}
		if wait {
			run, e = c.waitDerivedRun(ctx, base, run)
			if e != nil {
				_ = output(run)
				return e
			}
		}
		return output(run)
	}
	if !access.ValidID(channel) || revision == "" {
		return fmt.Errorf("需要有效 --channel 和 --expected-revision")
	}
	path := base + "/channels/" + channel
	if apply {
		if !access.ValidID(preview) || len(hash) != 64 {
			return fmt.Errorf("执行需要原 --preview-id 和 --preview-hash")
		}
		body := dm.ControlInput{WriteMeta: dm.WriteMeta{RequestID: requestID, Revision: revision, Reason: reason}, PreviewID: preview, PreviewHash: hash}
		if action == "backfill" {
			var result dm.BackfillResult
			if e = c.request(ctx, "POST", path+"/backfill", body, &result); e != nil {
				return e
			}
			if wait {
				for i, run := range result.Runs {
					result.Runs[i], e = c.waitDerivedRun(ctx, base, run)
					if e != nil {
						_ = output(result)
						return e
					}
				}
			}
			return output(result)
		}
		var result dm.ChannelResult
		if e = c.request(ctx, "POST", path+"/control", body, &result); e != nil {
			return e
		}
		return output(result)
	}
	if wait {
		return fmt.Errorf("--wait 只用于 --apply 后的任务")
	}
	if action == "backfill" {
		body := dm.BackfillPreviewInput{Revision: revision, Range: dm.NanosecondRange{From: from, To: &to}, UnassignedConfigID: configuration}
		if input != "" {
			if e = decodeDerivedInput(input, &body); e != nil {
				return e
			}
			if body.Revision != revision {
				return fmt.Errorf("输入文件修订与 --expected-revision 不一致")
			}
		}
		var result dm.BackfillPreview
		if e = c.request(ctx, "POST", path+"/backfill-preview", body, &result); e != nil {
			return e
		}
		return output(result)
	}
	var result dm.ControlPreview
	body := dm.ControlPreviewInput{Revision: revision, ControlProposal: dm.ControlProposal{Action: action, ResumeMode: resume, TrialReceiptID: trialReceipt, AllowWaiting: allowWaiting}}
	if from != "" {
		body.AutoStartNS = &from
	}
	if to != "" {
		body.StopNS = &to
	}
	if input != "" {
		if e = decodeDerivedInput(input, &body); e != nil {
			return e
		}
		if body.Revision != revision || body.Action != action {
			return fmt.Errorf("输入文件动作或修订不匹配")
		}
	}
	if e = c.request(ctx, "POST", path+"/control-preview", body, &result); e != nil {
		return e
	}
	return output(result)
}
func decodeDerivedInput(path string, out any) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return fmt.Errorf("预览输入须是至多 1 MiB 的普通 JSON 文件")
	}
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if e = d.Decode(out); e != nil {
		return e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return fmt.Errorf("输入文件必须只有一个 JSON 对象")
	}
	return nil
}
func (c *derivedClient) waitDerivedRun(ctx context.Context, base string, run dm.Run) (dm.Run, error) {
	for {
		if e := c.request(ctx, "GET", base+"/runs/"+run.ID, nil, &run); e != nil {
			return run, e
		}
		switch run.Job.State {
		case "succeeded":
			return run, nil
		case "failed", "cancelled", "needs_action":
			return run, fmt.Errorf("任务 %s：%s", run.Job.State, run.Job.ErrorCode)
		}
		select {
		case <-ctx.Done():
			return run, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
