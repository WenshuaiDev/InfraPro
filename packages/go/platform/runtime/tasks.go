package runtime

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
)

type taskSource struct {
	resolve    func(context.Context, pgx.Tx, access.Actor, []taskrecord.SourceRef, string) ([]tasks.View, error)
	control    func(context.Context, access.Actor, taskrecord.SourceRef, tasks.Command) (tasks.CommandResult, error)
	target     func(context.Context, pgx.Tx, access.Actor, taskrecord.SourceRef, string, string) (tasks.Target, error)
	operations func(context.Context, pgx.Tx, access.Actor, tasks.Filter) ([]tasks.Operation, error)
}

// ExistsTx reads only the source identity whitelist; lack of authorization must
// never be interpreted as deletion of an existing business record.
func (s taskSource) ExistsTx(ctx context.Context, tx pgx.Tx, ref taskrecord.SourceRef) (bool, error) {
	var exists bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_retained_sources WHERE source_module=$1 AND source_kind=$2 AND source_root_id=$3)`, ref.Module, ref.Kind, ref.RootID).Scan(&exists)
	return exists, e
}

func (s taskSource) ResolveTx(c context.Context, t pgx.Tx, a access.Actor, r []taskrecord.SourceRef, m string) ([]tasks.View, error) {
	return s.resolve(c, t, a, r, m)
}
func (s taskSource) Control(c context.Context, a access.Actor, r taskrecord.SourceRef, v tasks.Command) (tasks.CommandResult, error) {
	return s.control(c, a, r, v)
}
func (s taskSource) TargetTx(c context.Context, t pgx.Tx, a access.Actor, r taskrecord.SourceRef, g, k string) (tasks.Target, error) {
	return s.target(c, t, a, r, g, k)
}
func (s taskSource) OperationsTx(c context.Context, t pgx.Tx, a access.Actor, f tasks.Filter) ([]tasks.Operation, error) {
	return s.operations(c, t, a, f)
}
func ComposeTasks(a *access.Service, business ...Business) *tasks.Service {
	b := Business{}
	if len(business) > 0 {
		b = business[0]
	} else {
		b = Compose(a)
	}
	sources := map[string]tasks.Source{}
	for _, module := range []string{"monitoring", "derived"} {
		sources[module] = taskSource{b.Monitoring.TaskResolveTx, b.Monitoring.TaskControl, b.Monitoring.TaskTargetTx, func(c context.Context, t pgx.Tx, a access.Actor, f tasks.Filter) ([]tasks.Operation, error) {
			return b.Monitoring.TaskOperationsTx(c, t, a, f, module)
		}}
	}
	sources["derived"] = taskSource{b.DerivedMonitoring.TaskResolveTx, b.Monitoring.TaskControl, b.Monitoring.TaskTargetTx, func(c context.Context, t pgx.Tx, a access.Actor, f tasks.Filter) ([]tasks.Operation, error) {
		return b.Monitoring.TaskOperationsTx(c, t, a, f, "derived")
	}}
	sources["parsing"] = taskSource{b.Parsing.TaskResolveTx, b.Parsing.TaskControl, b.Parsing.TaskTargetTx, b.Parsing.TaskOperationsTx}
	sources["spatial"] = taskSource{b.Spatial.TaskResolveTx, b.Spatial.TaskControl, b.Spatial.TaskTargetTx, b.Spatial.TaskOperationsTx}
	sources["display"] = taskSource{b.Displays.TaskResolveTx, b.Displays.TaskControl, b.Displays.TaskTargetTx, b.Displays.TaskOperationsTx}
	sources["files"] = fileTaskSource{taskSource: taskSource{b.Files.TaskResolveTx, b.Files.TaskControl, b.Files.TaskTargetTx, b.Files.TaskOperationsTx}, files: b.Files}
	sources["audit"] = auditTaskSource{taskSource: taskSource{b.Audit.TaskResolveTx, b.Audit.TaskControl, b.Audit.TaskTargetTx, b.Audit.TaskOperationsTx}, audit: b.Audit}
	return tasks.New(a, sources)
}

// Only the files wrapper implements the optional per-execution result seam.
type fileTaskSource struct {
	taskSource
	files *files.Service
}

func (s fileTaskSource) ResultForExecutionTx(ctx context.Context, tx pgx.Tx, a access.Actor, ref taskrecord.SourceRef, generation string) (tasks.ExecutionResult, error) {
	return s.files.ResultForExecutionTx(ctx, tx, a, ref, generation)
}

type auditTaskSource struct {
	taskSource
	audit *audit.Service
}

func (s auditTaskSource) ResultForExecutionTx(ctx context.Context, tx pgx.Tx, a access.Actor, ref taskrecord.SourceRef, generation string) (tasks.ExecutionResult, error) {
	return s.audit.ResultForExecutionTx(ctx, tx, a, ref, generation)
}
