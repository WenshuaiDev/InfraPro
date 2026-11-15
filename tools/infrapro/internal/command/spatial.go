package command

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
)

const spatialHelp = `空间资料维护：infrapro spatial verify --tenant ID [--apply --reason TEXT]

逐版本核对 PostgreSQL 清单、原件、浏览文件的大小和 SHA-256。
默认只读；--apply 按核验结果隔离或重新开放固定版本并记录维护审计。
不会重新转换或替换已发布字节；存储暂时故障不会修改可用状态。
先恢复匹配的完整数据库、S3 原件和浏览产物，再核验后开放服务。
使用 INFRAPRO_CONFIG、INFRAPRO_DATABASE_URL 与 INFRAPRO_MONITORING_OBJECT_* 配置。
`

func (a *App) spatial(ctx context.Context, args []string) error {
	if len(args) == 0 || len(args) == 1 && isHelp(args[0]) {
		return a.help(spatialHelp)
	}
	if args[0] != "verify" {
		return fmt.Errorf("unknown spatial command; use infrapro spatial --help")
	}
	f := flag.NewFlagSet("spatial verify", flag.ContinueOnError)
	f.SetOutput(a.errOut)
	var tenant, reason string
	var apply bool
	f.StringVar(&tenant, "tenant", "", "tenant ID")
	f.StringVar(&reason, "reason", "", "maintenance reason")
	f.BoolVar(&apply, "apply", false, "apply verified availability")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	db, e := config.LoadDatabase(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	objects, e := config.LoadSpatialStorage(os.Getenv("INFRAPRO_CONFIG"))
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, db.URL)
	if e != nil {
		return fmt.Errorf("database unavailable")
	}
	defer pool.Close()
	if e = migrations.Check(ctx, pool); e != nil {
		return e
	}
	store, e := blobstore.Open(objects.ObjectEndpoint, objects.ObjectAccessKey, objects.ObjectSecretKey, objects.Bucket, "spatial")
	if e != nil {
		return e
	}
	failed := false
	e = spatial.VerifyContent(ctx, pool, store, tenant, apply, reason, func(r spatial.IntegrityResult) error {
		failed = failed || !r.Available
		return json.NewEncoder(a.out).Encode(r)
	})
	if e != nil {
		return e
	}
	if failed {
		return fmt.Errorf("spatial content verification found unavailable versions")
	}
	return nil
}
