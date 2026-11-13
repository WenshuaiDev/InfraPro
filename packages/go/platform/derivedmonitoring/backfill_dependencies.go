package derivedmonitoring

import (
	"context"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
)

func (s *Service) backfillDependencyPlanTx(ctx context.Context, tx pgx.Tx, tenant string, p BackfillPreview) (inputRequirementPlan, error) {
	needs := []monitoring.RawNeed{}
	for _, target := range p.Targets {
		needs = append(needs, target.Needs...)
	}
	return s.inputRequirementsTx(ctx, tx, tenant, needs)
}
func (s *Service) prepareBackfillDependenciesTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, parent *BackfillPreview, selections []BackfillDependencySelection) error {
	if len(selections) > 32 {
		return invalid("/dependencies", "一次最多明确选择32个依赖通道。")
	}
	required, e := s.backfillDependencyPlanTx(ctx, tx, scope.Tenant, *parent)
	if e != nil {
		return e
	}
	allowed := map[string]bool{}
	for _, need := range required.Derived {
		allowed[need.Channel] = true
	}
	all := map[string]BackfillPreview{}
	graph := map[string][]string{}
	total := len(parent.Targets)
	for _, selection := range selections {
		if !allowed[selection.ChannelID] || selection.ChannelID == parent.ChannelID {
			return invalid("/dependencies", "只能选择此运行实际依赖的上游通道。")
		}
		if _, exists := all[selection.ChannelID]; exists {
			return invalid("/dependencies", "同一依赖通道只选择一次明确范围。")
		}
		if _, e = revisionNumber(selection.Revision); e != nil {
			return e
		}
		child, e := s.prepareBackfillTx(ctx, tx, a, scope, selection.ChannelID, BackfillPreviewInput{Revision: selection.Revision, Range: selection.Range, UnassignedConfigID: selection.UnassignedConfigID})
		if e != nil {
			return e
		}
		total += len(child.Targets)
		if total > 1000 {
			return problem("plan_limit_exceeded", "父子回算超过1000个配置段，请缩小所选范围。", 422)
		}
		all[selection.ChannelID] = child
		dependencies, e := s.backfillDependencyPlanTx(ctx, tx, scope.Tenant, child)
		if e != nil {
			return e
		}
		graph[selection.ChannelID] = []string{}
		for _, need := range dependencies.Derived {
			graph[selection.ChannelID] = append(graph[selection.ChannelID], need.Channel)
		}
	}
	order, e := planning.Order(graph, 8)
	if e != nil {
		return e
	}
	parent.Selections = append([]BackfillDependencySelection{}, selections...)
	slices.SortFunc(parent.Selections, func(a, b BackfillDependencySelection) int {
		if a.ChannelID < b.ChannelID {
			return -1
		}
		if a.ChannelID > b.ChannelID {
			return 1
		}
		return 0
	})
	parent.Dependencies = []BackfillPreview{}
	for _, id := range order {
		if child, ok := all[id]; ok {
			parent.Dependencies = append(parent.Dependencies, child)
		}
	}
	return nil
}

// Explicit children and their parent are created atomically. Every child uses
// the same requesting actor and scope; no upstream tenant mandate is borrowed.
func (s *Service) enqueueBackfillGroupTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, preview BackfillPreview, meta WriteMeta) ([]Run, error) {
	plans := append(append([]BackfillPreview{}, preview.Dependencies...), preview)
	channels := map[string]Channel{}
	for _, plan := range plans {
		channel, e := s.maintenanceTargetTx(ctx, tx, a, scope, plan.ChannelID, true)
		if e != nil {
			return nil, e
		}
		channel, _, e = s.assignBackfillGapsTx(ctx, tx, a, scope, channel, plan, meta)
		if e != nil {
			return nil, e
		}
		channels[plan.ChannelID] = channel
	}
	var position, graph string
	if e := tx.QueryRow(ctx, `SELECT m.commit_sequence::text,g.revision::text FROM monitoring_tenant_states m JOIN derived_graph_states g USING(tenant_id) WHERE m.tenant_id=$1`, scope.Tenant).Scan(&position, &graph); e != nil {
		return nil, e
	}
	out := []Run{}
	links := 0
	for _, plan := range plans {
		plan.GraphRevision = graph
		needed, e := s.backfillDependencyPlanTx(ctx, tx, scope.Tenant, plan)
		if e != nil {
			return nil, e
		}
		inputs := map[string]bool{}
		for _, need := range needed.Derived {
			inputs[need.Channel] = true
		}
		runs, e := s.enqueueBackfillTargetsTx(ctx, tx, a, scope, channels[plan.ChannelID], plan, meta, position)
		if e != nil {
			return nil, e
		}
		for _, run := range runs {
			for _, previous := range out {
				if !inputs[previous.ChannelID] {
					continue
				}
				links++
				if links > 8192 {
					return nil, problem("plan_limit_exceeded", "回算依赖关系超过预算，请缩小范围。", 422)
				}
				if _, e = tx.Exec(ctx, `INSERT INTO derived_run_dependencies(tenant_id,run_id,dependency_run_id) VALUES($1,$2,$3)`, scope.Tenant, run.ID, previous.ID); e != nil {
					return nil, e
				}
			}
		}
		out = append(out, runs...)
	}
	return out, nil
}
func (s *Service) requireRunDependenciesTx(ctx context.Context, tx pgx.Tx, tenant, run string) error {
	var pending, failed bool
	e := tx.QueryRow(ctx, `SELECT COALESCE(bool_or(j.state<>'succeeded'),false),COALESCE(bool_or(j.state IN ('failed','needs_action','cancelled')),false) FROM derived_run_dependencies d JOIN derived_runs r ON r.tenant_id=d.tenant_id AND r.id=d.dependency_run_id JOIN monitoring_jobs j ON j.tenant_id=r.tenant_id AND j.id=r.job_id WHERE d.tenant_id=$1 AND d.run_id=$2`, tenant, run).Scan(&pending, &failed)
	if e != nil {
		return e
	}
	if failed {
		return &monitoring.TaskError{Code: "dependency_failed", Message: "明确提交的依赖回算未完成，请处理子任务后重试。", Disposition: "needs_action"}
	}
	if pending {
		return &monitoring.TaskError{Code: "dependency_pending", Message: "等待明确提交的上游回算完成。", Disposition: "waiting"}
	}
	return nil
}
func (s *Service) wakeDependentRunsTx(ctx context.Context, tx pgx.Tx, tenant, run string) error {
	_, e := tx.Exec(ctx, `UPDATE monitoring_jobs j SET available_at=clock_timestamp() FROM derived_runs r JOIN derived_run_dependencies d ON d.tenant_id=r.tenant_id AND d.run_id=r.id WHERE d.tenant_id=$1 AND d.dependency_run_id=$2 AND r.job_id=j.id AND j.state='retry_wait' AND j.error_code='dependency_pending'`, tenant, run)
	return e
}
