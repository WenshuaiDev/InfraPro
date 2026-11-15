package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const filesHelp = `文件与附件：
  files check | status
  files worker
  files checker --directory 私有目录 --socket 私有UnixSocket
  files verify --mode metadata|content [--apply --reason 原因]
  files cleanup plan --reason 原因
  files cleanup apply --plan-id UUID --reason 原因
  files backup prepare --directory 备份目录 --snapshot-ref 冻结点编号 --reason 原因 [--external-dump | --pg-container 本地测试PG容器]
  files backup copy|seal --id UUID --directory 备份目录 --reason 原因
  files backup release --id UUID --reason 原因
  files restore objects --directory 备份目录 --reason 原因
  files restore reconcile --reason 原因
只操作普通文件命名空间。未知对象不删除；不支持 force。
`

func (a *App) files(ctx context.Context, args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		return a.help(filesHelp)
	}
	action := args[0]
	args = args[1:]
	if action == "cleanup" || action == "backup" || action == "restore" {
		if len(args) == 0 {
			return a.help(filesHelp)
		}
		action += "." + args[0]
		args = args[1:]
	}
	flags := flag.NewFlagSet("files "+action, flag.ContinueOnError)
	flags.SetOutput(a.errOut)
	id := flags.String("id", "", "备份编号")
	plan := flags.String("plan-id", "", "清理计划编号")
	reason := flags.String("reason", "", "运维理由")
	directory := flags.String("directory", "", "受限目录")
	socket := flags.String("socket", "", "本地 Unix socket")
	snapshot := flags.String("snapshot-ref", "", "与数据库一致备份关联的冻结点编号")
	mode := flags.String("mode", "metadata", "核对模式")
	apply := flags.Bool("apply", false, "应用已核验完整性状态")
	externalDump := flags.Bool("external-dump", false, "冻结后由独立数据库备份身份保存 database.dump")
	container := flags.String("pg-container", "", "仅本机测试数据库容器")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected files arguments")
	}
	if action == "checker" {
		executable := os.Getenv("INFRAPRO_FILES_CHECKER_EXECUTABLE")
		if executable == "" {
			var e error
			executable, e = exec.LookPath("files-checker")
			if e != nil {
				return fmt.Errorf("files-checker executable is required")
			}
		}
		cmd := exec.CommandContext(ctx, executable, "--directory", *directory, "--socket", *socket)
		cmd.Env = []string{"GOMEMLIMIT=256MiB", "GOMAXPROCS=2", "TZ=UTC"}
		cmd.Stdout = a.out
		cmd.Stderr = a.errOut
		return cmd.Run()
	}
	db, options, e := config.LoadFiles(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, db.URL)
	if e != nil {
		return e
	}
	defer pool.Close()
	if e = migrations.Check(ctx, pool); e != nil {
		return e
	}
	accessService := access.New(pool, slog.Default())
	business := runtime.Compose(accessService)
	s := business.Files.WithOptions(options).WithChecker(files.SocketChecker{Socket: options.CheckerSocket, Timeout: options.CheckerTimeout})
	if options.ObjectEndpoint != "" {
		store, e := blobstore.Open(options.ObjectEndpoint, options.ObjectAccessKey, options.ObjectSecretKey, options.Bucket, "files/v1")
		if e != nil {
			return e
		}
		s.WithStorage(store)
	}
	var out any
	switch action {
	case "worker":
		return s.RunWorker(ctx, options.NodeID+":"+uuid.NewString())
	case "status":
		out, e = s.Status(ctx)
	case "check":
		if e = accessService.Check(ctx); e != nil {
			return e
		}
		status, err := s.Status(ctx)
		if err != nil {
			return err
		}
		out = status
		if e = s.CheckDependencies(ctx); e != nil {
			return e
		}
		if options.ObjectEndpoint == "" || !status.CheckerAvailable || status.WorkerLastSeen == nil {
			return fmt.Errorf("files dependencies unavailable; inspect files status")
		}
	case "verify":
		out, e = s.Verify(ctx, *mode, *apply, *reason)
	case "cleanup.plan":
		out, e = s.CleanupPlan(ctx, "infrapro", *reason)
	case "cleanup.apply":
		out, e = s.CleanupApply(ctx, *plan, *reason)
	case "backup.prepare":
		if *directory == "" {
			return fmt.Errorf("backup directory is required")
		}
		if e = os.MkdirAll(*directory, 0700); e != nil {
			return e
		}
		manifest, err := s.BackupPrepare(ctx, *snapshot, *reason)
		out, e = manifest, err
		if e == nil && !*externalDump {
			e = dumpFilesDatabase(ctx, db.URL, *directory, *container)
		}
	case "backup.copy":
		out, e = s.BackupCopy(ctx, *id, *directory)
	case "backup.seal":
		out, e = s.BackupSeal(ctx, *id, *directory, *reason)
	case "backup.release":
		e = s.BackupRelease(ctx, *id, *reason)
		out = map[string]bool{"released": e == nil}
	case "restore.objects":
		out, e = s.RestoreObjects(ctx, *directory, *reason)
	case "restore.reconcile":
		out, e = s.RestoreReconcile(ctx, *reason)
	default:
		return fmt.Errorf("unknown files command")
	}
	if out != nil {
		if err := json.NewEncoder(a.out).Encode(out); err != nil {
			return err
		}
	}
	if e != nil {
		return e
	}
	if v, ok := out.(files.Verification); ok && !v.Complete {
		return fmt.Errorf("files verification incomplete; maintenance barrier preserved")
	}
	return nil
}
func dumpFilesDatabase(ctx context.Context, databaseURL, directory, container string) error {
	u, e := url.Parse(databaseURL)
	if e != nil {
		return fmt.Errorf("invalid backup database configuration")
	}
	// Password is passed through the process environment, never command arguments
	// or output. Production uses the installed pg_dump or the existing backup tool.
	password, _ := u.User.Password()
	user := u.User.Username()
	database := strings.TrimPrefix(u.Path, "/")
	args := []string{"--format=custom", "--no-owner", "--no-acl", "--username", user, "--dbname", database}
	if schema := u.Query().Get("search_path"); schema != "" {
		args = append(args, "--schema", schema)
	}
	var cmd *exec.Cmd
	if container != "" {
		if !strings.HasSuffix(database, "_test") || !strings.Contains(container, "test") {
			return fmt.Errorf("container pg_dump is restricted to local test databases")
		}
		cmd = exec.CommandContext(ctx, "docker", append([]string{"exec", container, "pg_dump"}, args...)...)
	} else {
		args = append(args, "--host", u.Hostname())
		if u.Port() != "" {
			args = append(args, "--port", u.Port())
		}
		cmd = exec.CommandContext(ctx, "pg_dump", args...)
	}
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)
	file, e := os.OpenFile(filepath.Join(directory, "database.dump"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer file.Close()
	cmd.Stdout = file
	if e = cmd.Run(); e != nil {
		return fmt.Errorf("database snapshot failed; file write barrier remains read_only")
	}
	return file.Sync()
}
