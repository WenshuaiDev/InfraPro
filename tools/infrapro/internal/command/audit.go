package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"time"
)

const auditHelp = `审计与操作追踪：
  audit status
  audit verify --mode metadata|content
  audit worker
  audit handoff retry --id UUID --attempts 8 --request-id UUID --reason 原因
  audit policy plan --reason 原因 [--config-present]
  audit policy apply --plan-id UUID --revision 1 --reason 原因
  audit backfill plan|apply --reason 原因 [--plan-id UUID --revision 1]
  audit cleanup plan --kind facts|objects --reason 原因
  audit cleanup apply --kind facts|objects --plan-id UUID --revision 1 --reason 原因
  audit backup manifest --snapshot-ref 冻结点编号 [--cursor UUID]
  audit backup hold --kind all|backup|event|export_object --object-id 标识 --until RFC3339 --reason 原因
  audit backup release --id UUID --reason 原因
  audit mode --mode read_only|reconciling|ready --reason 原因
  audit restore reconcile --reason 原因
修改命令可传 --request-id UUID 重放同一维护收据。无 force 或任意事件删除入口。
policy plan 只有明确 --config-present 才导入实际 audit_retention；否则生成无删除能力的待核对计划。
content 核验需独立 INFRAPRO_AUDIT_CONTENT_ACCESS_KEY / INFRAPRO_AUDIT_CONTENT_SECRET_KEY。
恢复需先停止业务写入并设置 reconciling；reconcile 后保持 read_only，核验后显式设 ready。
`

func (a *App) audit(ctx context.Context, args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		return a.help(auditHelp)
	}
	action := args[0]
	args = args[1:]
	if action == "policy" || action == "backfill" || action == "cleanup" || action == "backup" || action == "restore" || action == "handoff" {
		if len(args) == 0 {
			return a.help(auditHelp)
		}
		action += "." + args[0]
		args = args[1:]
	}
	f := flag.NewFlagSet("audit "+action, flag.ContinueOnError)
	f.SetOutput(a.errOut)
	mode := f.String("mode", "metadata", "模式")
	kind := f.String("kind", "facts", "计划或保护类型")
	reason := f.String("reason", "", "维护原因")
	plan := f.String("plan-id", "", "计划编号")
	revision := f.String("revision", "1", "预期计划修订")
	request := f.String("request-id", "", "维护请求编号")
	id := f.String("id", "", "保护编号")
	attempts := f.Int("attempts", 8, "交接的已观察失败次数")
	object := f.String("object-id", "", "保护目标")
	until := f.String("until", "", "保护截止时间")
	snapshot := f.String("snapshot-ref", "", "一致备份点编号")
	cursor := f.String("cursor", "", "清单分页位置")
	present := f.Bool("config-present", false, "确认读取实际部署配置")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected audit arguments")
	}
	if *request == "" {
		*request = uuid.NewString()
	}
	db, opts, e := config.LoadAudit(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, db.URL)
	if e != nil {
		return fmt.Errorf("audit database configuration unavailable")
	}
	defer pool.Close()
	if e = migrations.Check(ctx, pool); e != nil {
		return e
	}
	role := "infrapro_audit_maintenance"
	if action == "status" || action == "verify" && *mode == "metadata" {
		role = "infrapro_audit_observer"
	}
	if action == "worker" {
		role = "infrapro_audit_worker"
	}
	if action == "verify" && *mode == "content" {
		role = "infrapro_audit_content"
	}
	if action == "restore.reconcile" {
		role = "infrapro_audit_restore"
	}
	var allowed bool
	if e = pool.QueryRow(ctx, `SELECT pg_has_role(current_user,$1,'USAGE') OR (SELECT rolsuper FROM pg_roles WHERE rolname=current_user)`, role).Scan(&allowed); e != nil || !allowed {
		return fmt.Errorf("audit operation requires the %s database credential", role)
	}
	s := runtime.Compose(access.New(pool, slog.Default())).Audit.WithOptions(opts)
	if action == "verify" && *mode == "content" {
		opts.ObjectAccessKey = os.Getenv("INFRAPRO_AUDIT_CONTENT_ACCESS_KEY")
		opts.ObjectSecretKey = os.Getenv("INFRAPRO_AUDIT_CONTENT_SECRET_KEY")
		if opts.ObjectAccessKey == "" || opts.ObjectSecretKey == "" {
			return fmt.Errorf("audit content verification requires separate content-read storage credentials")
		}
	}
	if opts.ObjectEndpoint != "" {
		store, e := blobstore.Open(opts.ObjectEndpoint, opts.ObjectAccessKey, opts.ObjectSecretKey, opts.Bucket, "audit-exports")
		if e != nil {
			return e
		}
		s.WithStorage(store)
	}
	var out any
	switch action {
	case "status":
		out, e = s.Status(ctx)
	case "verify":
		out, e = s.Verify(ctx, *mode)
	case "worker":
		return s.RunWorker(ctx)
	case "handoff.retry":
		out, e = s.RetryHandoff(ctx, *id, *attempts, *request, *reason)
	case "policy.plan":
		var retention *time.Duration
		if *present {
			v, err := config.LoadAuditRetention(os.Getenv("INFRAPRO_CONFIG"))
			if err != nil {
				return err
			}
			retention = &v
		}
		out, e = s.Plan(ctx, "policy", *reason, retention)
	case "backfill.plan":
		out, e = s.Plan(ctx, "backfill", *reason, nil)
	case "cleanup.plan":
		out, e = s.Plan(ctx, *kind, *reason, nil)
	case "policy.apply", "backfill.apply":
		out, e = s.ApplyPlan(ctx, *plan, *revision, *request, *reason)
	case "cleanup.apply":
		if *kind == "objects" {
			out, e = s.ApplyObjects(ctx, *plan, *revision, *request, *reason)
		} else {
			out, e = s.ApplyPlan(ctx, *plan, *revision, *request, *reason)
		}
	case "backup.manifest":
		out, e = s.Manifest(ctx, *snapshot, *cursor)
	case "backup.hold":
		end, err := time.Parse(time.RFC3339, *until)
		if err != nil {
			return fmt.Errorf("hold requires a valid until timestamp")
		}
		out, e = s.Hold(ctx, *kind, *object, *reason, end)
	case "backup.release":
		e = s.ReleaseHold(ctx, *id, *request, *reason)
		out = map[string]string{"id": *id, "state": "released"}
	case "mode":
		e = s.SetMaintenanceMode(ctx, *mode, *request, *reason)
		out = map[string]string{"mode": *mode}
	case "restore.reconcile":
		out, e = s.RestoreReconcile(ctx, *request, *reason)
	default:
		return fmt.Errorf("unknown audit action; use audit --help")
	}
	if e != nil {
		return e
	}
	encoder := json.NewEncoder(a.out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}
