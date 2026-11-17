package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
)

const monitoringHelp = `监测维护：infrapro monitoring <子命令> [选项]

  derived                 派生计算检查、状态、对账、恢复核验及经业务 API 执行控制

  storage init/check/status        显式初始化或检查布局、已配置节点、在线对象层与缓存
  verify                    --tenant ID；只读对账
                            --apply --expected-watermark N --reason TEXT 排队对账及隔离修复
  summaries rebuild         --tenant ID --channel ID 显示固定重建计划
                            --apply --plan-hash HASH --reason TEXT 执行
  routes plan               --tenant ID --logical N --target NAME
  routes move               同 plan，加 --plan-hash HASH --reason TEXT
                            或 --id ID --expected-revision N --phase copy|switch --reason TEXT
  routes status             --tenant ID --id ID
  routes pause/continue     --tenant ID --id ID --expected-revision N --reason TEXT
  routes rollback           同上；创建反向复制任务，含切换后新增数据
  backup                    --directory ABS_PATH --reason TEXT [--pg-tools-container NAME]
  restore                   --directory ABS_PATH --manifest-sha256 HASH 显示备份清单
                            --apply --reason TEXT 恢复到独立空环境，完成后只读
  restore open              --backup-id ID --reason TEXT 开放已核验恢复环境写入
  jobs inspect              --tenant ID --id ID [--report]
  jobs retry                --tenant ID --id ID --expected-revision N --reason TEXT

任务命令可加 --wait，由当前 CLI 执行持久 worker 并等待指定任务。
独立任务执行器：go run ./services/api/cmd/monitoring-worker
配置使用 INFRAPRO_CONFIG 和 INFRAPRO_MONITORING_*；不打印连接凭据。
本轮仅本机合成样本验证，目标部署的 PITR、异地灾备与规模 SLA 尚未验证。
`

func (a *App) monitoring(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "derived" {
		return a.monitoringDerived(ctx, args[1:])
	}
	if len(args) == 0 || slices.ContainsFunc(args, isHelp) {
		return a.help(monitoringHelp)
	}
	ctx = md.TraceContext(ctx, "")
	defer func() { md.LogOperation(ctx, "operator."+args[0], 0, 0, ctx.Err() != nil) }()
	group, action := args[0], ""
	rest := args[1:]
	if slices.Contains([]string{"storage", "summaries", "routes", "jobs"}, group) || group == "restore" && len(rest) > 0 && rest[0] == "open" {
		if len(rest) == 0 {
			return fmt.Errorf("缺少监测子命令")
		}
		action = rest[0]
		rest = rest[1:]
	}
	allowed := map[string][]string{"storage": {"init", "check", "status"}, "verify": {""}, "summaries": {"rebuild"}, "routes": {"plan", "move", "status", "pause", "continue", "rollback"}, "backup": {""}, "restore": {"", "open"}, "jobs": {"inspect", "retry"}}
	if !slices.Contains(allowed[group], action) {
		return fmt.Errorf("未知监测维护命令；使用 infrapro monitoring --help")
	}
	flags := flag.NewFlagSet("monitoring", flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	var tenant, channel, target, id, revision, phase, reason, planHash, watermark, directory, manifestSHA, pgContainer, backupID string
	var apply, wait, report bool
	logical := -1
	flags.StringVar(&tenant, "tenant", "", "租户 ID")
	flags.StringVar(&channel, "channel", "", "通道 ID")
	flags.StringVar(&target, "target", "", "物理分片名")
	flags.StringVar(&id, "id", "", "任务或迁移 ID")
	flags.StringVar(&revision, "expected-revision", "", "当前修订号")
	flags.StringVar(&phase, "phase", "copy", "copy 或 switch")
	flags.StringVar(&reason, "reason", "", "操作原因")
	flags.StringVar(&planHash, "plan-hash", "", "固定计划 hash")
	flags.StringVar(&watermark, "expected-watermark", "", "发布水位")
	flags.StringVar(&directory, "directory", "", "新备份目录或恢复目录")
	flags.StringVar(&manifestSHA, "manifest-sha256", "", "备份清单 SHA-256")
	flags.StringVar(&pgContainer, "pg-tools-container", "", "包含 PG 17 客户端的 Docker 容器")
	flags.StringVar(&backupID, "backup-id", "", "已核验恢复的备份 ID")
	flags.IntVar(&logical, "logical", -1, "0–255 逻辑分片")
	flags.BoolVar(&apply, "apply", false, "执行预览对应变更")
	flags.BoolVar(&wait, "wait", false, "执行并等待任务")
	flags.BoolVar(&report, "report", false, "读取对账报告")
	if e := flags.Parse(rest); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("不支持额外的位置参数")
	}
	if slices.Contains([]string{"verify", "summaries", "routes", "jobs"}, group) && !access.ValidID(tenant) {
		return fmt.Errorf("必须填写有效的 --tenant")
	}
	write := group == "backup" || group == "restore" && (apply || action == "open") || group == "verify" && apply || group == "summaries" && apply || group == "routes" && action != "plan" && action != "status" || group == "jobs" && action == "retry"
	if write {
		if e := access.ValidReason(reason); e != nil {
			return fmt.Errorf("变更命令必须提供有效的 --reason")
		}
	}
	output := func(v any) error { enc := json.NewEncoder(a.out); enc.SetIndent("", "  "); return enc.Encode(v) }
	if group == "restore" && action == "" && !apply {
		m, e := md.ReadBackupManifest(directory, manifestSHA)
		if e != nil {
			return e
		}
		return output(m)
	}
	path := os.Getenv("INFRAPRO_CONFIG")
	cfg, e := config.LoadMonitoring(path)
	if e != nil {
		return e
	}
	storage, e := md.OpenStorage(ctx, runtime.MonitoringStorageConfig(cfg))
	if e != nil {
		return e
	}
	defer storage.Close()
	if group == "storage" && action != "status" {
		if action == "init" {
			e = storage.Initialize(ctx)
			if e == nil {
				e = storage.Check(ctx)
			}
			if e == nil {
				e = derivedmonitoring.InitializeStorage(ctx, storage)
			}
		} else {
			e = storage.Check(ctx)
			if e == nil {
				e = derivedmonitoring.CheckStorage(ctx, storage)
			}
		}
		if e != nil {
			return e
		}
		return output(map[string]string{"state": "passed", "operation": "storage." + action})
	}
	db, e := config.LoadDatabase(path)
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, db.URL)
	if e != nil {
		return fmt.Errorf("控制库配置无效")
	}
	defer pool.Close()
	if group != "restore" || action == "open" {
		if e = migrations.Check(ctx, pool); e != nil {
			return e
		}
	}
	derivedCfg, e := config.LoadDerived(path)
	if e != nil {
		return e
	}
	business := runtime.Compose(access.New(pool, slog.Default()), storage).Configure(config.Config{Monitoring: cfg, Derived: derivedCfg})
	service := business.Monitoring
	waitJob := func(job string) error {
		if !wait {
			return output(map[string]string{"job_id": job, "state": "queued"})
		}
		for {
			v, e := service.InspectOperatorJob(ctx, tenant, job)
			if e != nil {
				return e
			}
			if v.State == "succeeded" {
				return output(v)
			}
			if v.State == "failed" || v.State == "needs_action" || v.State == "cancelled" {
				_ = output(v)
				return fmt.Errorf("任务未完成，请按收据修复原因后重试")
			}
			worked, e := service.RunOne(ctx, "operator-cli")
			if e != nil {
				return e
			}
			if !worked {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
				}
			}
		}
	}
	switch group {
	case "storage":
		v, e := service.MaintenanceStatus(ctx)
		if e != nil {
			return e
		}
		return output(v)
	case "verify":
		if apply {
			job, e := service.QueueVerification(ctx, tenant, watermark, reason)
			if e != nil {
				return e
			}
			return waitJob(job)
		}
		v, e := service.Verify(ctx, tenant)
		if e != nil {
			return e
		}
		if e = output(v); e != nil {
			return e
		}
		if v.State != "passed" {
			return fmt.Errorf("对账发现异常；只读检查未修改任何数据")
		}
		return nil
	case "summaries":
		p, e := service.PlanSummaryRepair(ctx, tenant, channel)
		if e != nil {
			return e
		}
		if !apply {
			return output(p)
		}
		if p.Hash != planHash {
			return fmt.Errorf("计划已变化，请重新预览")
		}
		job, e := service.QueueSummaryRepair(ctx, p, reason)
		if e != nil {
			return e
		}
		return waitJob(job)
	case "jobs":
		if action == "inspect" {
			if report {
				v, e := service.OperatorJobReport(ctx, tenant, id)
				if e != nil {
					return e
				}
				return output(v)
			}
			v, e := service.InspectOperatorJob(ctx, tenant, id)
			if e != nil {
				return e
			}
			return output(v)
		}
		if e = service.RetryOperatorJob(ctx, tenant, id, revision, reason); e != nil {
			return e
		}
		return waitJob(id)
	case "routes":
		if action == "status" {
			v, e := service.Relocation(ctx, tenant, id)
			if e != nil {
				return e
			}
			return output(v)
		}
		if action == "plan" || action == "move" && id == "" {
			p, e := service.PlanRoute(ctx, tenant, logical, target)
			if e != nil {
				return e
			}
			if action == "plan" {
				return output(p)
			}
			if p.Hash != planHash {
				return fmt.Errorf("迁移计划已变化，请重新预览")
			}
			r, e := service.CreateRelocation(ctx, p, reason)
			if e != nil {
				return e
			}
			id, revision = r.ID, r.Revision
			_ = output(r)
		}
		if action == "pause" {
			if e = service.PauseRoute(ctx, tenant, id, revision, reason); e != nil {
				return e
			}
			r, e := service.Relocation(ctx, tenant, id)
			if e != nil {
				return e
			}
			return output(r)
		}
		if action == "rollback" {
			r, e := service.RollbackRoute(ctx, tenant, id, revision, reason)
			if e != nil {
				return e
			}
			id, revision, phase = r.ID, r.Revision, "switch"
			_ = output(r)
		}
		job, e := service.QueueRoute(ctx, tenant, id, revision, phase, reason)
		if e != nil {
			return e
		}
		return waitJob(job)
	case "backup", "restore":
		if action == "open" {
			if e = md.OpenRestoredWrites(ctx, pool, storage, backupID, reason, derivedmonitoring.NewRecovery(storage)); e != nil {
				return e
			}
			return output(map[string]string{"backup_id": backupID, "state": "ready"})
		}
		bc, e := config.LoadMonitoringBackup(path)
		if e != nil {
			return e
		}
		opt := md.BackupOptions{Directory: directory, DatabaseURL: db.URL, PGContainer: pgContainer, Reason: reason, Store: md.BackupConfig{Endpoint: bc.Endpoint, AccessKey: bc.AccessKey, SecretKey: bc.SecretKey, Bucket: bc.Bucket, NamedCollection: bc.NamedCollection}, Calculations: derivedmonitoring.NewRecovery(storage)}
		var v md.BackupManifest
		if group == "backup" {
			v, e = md.Backup(ctx, pool, storage, opt)
		} else {
			v, e = md.Restore(ctx, pool, storage, opt, manifestSHA)
		}
		if e != nil {
			return e
		}
		if group == "restore" {
			return output(map[string]any{"backup": v, "state": "read_only", "verified": true})
		}
		return output(v)
	}
	return fmt.Errorf("未知监测操作")
}
