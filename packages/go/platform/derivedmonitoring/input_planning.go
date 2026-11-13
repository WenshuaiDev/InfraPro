package derivedmonitoring

import (
	"context"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"github.com/jackc/pgx/v5"
)

// This plan is a prospective authorization boundary. Fixed reads subsequently
// resolve each upstream generation and actual rank prefix, then repeat checks
// against that evidence. A metadata plan never certifies computed coverage.
type inputRequirementPlan struct {
	Raw          []monitoring.RawNeed
	Derived      []monitoring.RawNeed
	Requirements []monitoring.RawNeed
	Prefixes     []monitoring.RawPrefix
	Configs      []string
}
type inputConfigSegment struct {
	Range     planning.Range
	Config    ChannelConfig
	Plan      *compute.Plan
	Baselines []monitoring.RawNeed
}
type inputProfile struct {
	Kind     string
	Segments []inputConfigSegment
}
type inputProfileResolver func(context.Context, string, planning.Range) (inputProfile, error)

func immediateInputNeeds(cfg ChannelConfig, p *compute.Plan, r planning.Range, sources ...*planning.Range) ([]monitoring.RawNeed, error) {
	var prefixes []planning.Predecessor
	var source *planning.Range
	if len(sources) > 0 {
		source = sources[0]
	}
	ranges, e := planning.Inputs(p, r, source, &prefixes)
	if e != nil {
		return nil, problem("plan_invalid", "输入范围规划失败。", 422)
	}
	needs := []monitoring.RawNeed{}
	for _, input := range p.Rule.Inputs {
		channel := cfg.InputBindings[input.Key].ChannelID
		if channel == "" {
			return nil, problem("plan_invalid", "固定输入槽未绑定。", 422)
		}
		for _, part := range ranges[input.Key] {
			from := part.From
			needs = append(needs, monitoring.RawNeed{Channel: channel, From: &from, To: part.To})
		}
	}
	return normalizedNeeds(needs)
}
func planInputRequirements(ctx context.Context, needs []monitoring.RawNeed, resolve inputProfileResolver, maxDepth int) (inputRequirementPlan, error) {
	out := inputRequirementPlan{Raw: []monitoring.RawNeed{}, Derived: []monitoring.RawNeed{}, Requirements: []monitoring.RawNeed{}, Prefixes: []monitoring.RawPrefix{}, Configs: []string{}}
	seen := map[string]bool{}
	path := map[string]bool{}
	visited := 0
	var visit func(monitoring.RawNeed, int) error
	visit = func(need monitoring.RawNeed, depth int) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		from := monitoringvalue.MinTime.UnixNano()
		if need.From != nil {
			from = *need.From
		}
		need.From = &from
		r := planning.Range{From: from, To: need.To}
		if !r.Valid() {
			return invalid("/input_ranges", "输入范围无效。")
		}
		if path[need.Channel] {
			return problem("dependency_cycle", "固定输入图存在循环。", 409)
		}
		key := need.Channel + ":" + rangeSQL(r) + ":" + strconv.Itoa(depth)
		if seen[key] {
			return nil
		}
		seen[key] = true
		visited++
		if visited > 10000 {
			return problem("plan_limit_exceeded", "输入范围展开超过规划预算。", 422)
		}
		profile, e := resolve(ctx, need.Channel, r)
		if e != nil {
			return e
		}
		out.Requirements = append(out.Requirements, need)
		if profile.Kind == "direct" {
			out.Raw = append(out.Raw, need)
			return nil
		}
		if profile.Kind != "derived" {
			return problem("input_type_mismatch", "计算输入必须是原始或正式派生通道。", 422)
		}
		if depth > maxDepth {
			return problem("dependency_depth_exceeded", "派生依赖深度超过上限。", 422)
		}
		out.Derived = append(out.Derived, need)
		path[need.Channel] = true
		defer delete(path, need.Channel)
		covered := []planning.Range{}
		for _, segment := range profile.Segments {
			part, ok := planning.Intersection(r, segment.Range)
			if !ok {
				continue
			}
			if segment.Plan == nil {
				return problem("plan_invalid", "缺少上游固定计划。", 422)
			}
			covered = append(covered, part)
			children, e := immediateInputNeeds(segment.Config, segment.Plan, part)
			if e != nil {
				return e
			}
			out.Prefixes = append(out.Prefixes, inputPrefixes(segment.Plan, segment.Config, children, part)...)
			out.Configs = append(out.Configs, segment.Config.ID)
			children = append(children, segment.Baselines...)
			for _, child := range children {
				if e = visit(child, depth+1); e != nil {
					return e
				}
			}
		}
		complete, e := planning.Covered([]planning.Range{r}, covered)
		if e != nil {
			return e
		}
		if !complete {
			return problem("upstream_pending", "上游所需范围缺少配置依据。", 409)
		}
		return nil
	}
	for _, need := range needs {
		if e := visit(need, 1); e != nil {
			return out, e
		}
	}
	var e error
	if out.Raw, e = normalizedNeeds(out.Raw); e != nil {
		return out, e
	}
	if out.Derived, e = normalizedNeeds(out.Derived); e != nil {
		return out, e
	}
	if out.Requirements, e = normalizedNeeds(out.Requirements); e != nil {
		return out, e
	}
	slices.Sort(out.Configs)
	out.Configs = slices.Compact(out.Configs)
	return out, nil
}

func (s *Service) metadataInputProfileTx(ctx context.Context, tx pgx.Tx, tenant, channel string, r planning.Range) (inputProfile, error) {
	var out inputProfile
	var draft *string
	e := tx.QueryRow(ctx, `SELECT c.source_kind,d.draft_config_id::text FROM engineering_channels c LEFT JOIN derived_channels d ON d.tenant_id=c.tenant_id AND d.channel_id=c.id WHERE c.tenant_id=$1 AND c.id=$2 AND c.deleted_at IS NULL`, tenant, channel).Scan(&out.Kind, &draft)
	if e == pgx.ErrNoRows {
		return out, notFound()
	}
	if e != nil || out.Kind == "direct" {
		return out, e
	}
	if draft == nil {
		return out, problem("result_integrity_unavailable", "派生通道缺少配置依据。", 503)
	}
	assignments, e := activeAssignmentsTx(ctx, tx, tenant, channel)
	if e != nil {
		return out, e
	}
	missing := []planning.Range{r}
	appendConfig := func(config string, part planning.Range) error {
		cfg, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND id=$3`, tenant, channel, config))
		if e != nil {
			return e
		}
		p, e := s.compile(cfg.Rule, "channel", cfg.Parameters)
		if e != nil {
			return e
		}
		baselines, e := s.baselineNeedsTx(ctx, tx, tenant, cfg, p)
		if e != nil {
			return e
		}
		out.Segments = append(out.Segments, inputConfigSegment{part, cfg, p, baselines})
		return nil
	}
	for _, assignment := range assignments {
		interval, e := assignment.Range.rangeValue()
		if e != nil {
			return out, e
		}
		part, ok := planning.Intersection(r, interval)
		if !ok {
			continue
		}
		if e = appendConfig(assignment.ConfigID, part); e != nil {
			return out, e
		}
		next := []planning.Range{}
		for _, hole := range missing {
			next = append(next, planning.Subtract(hole, part)...)
		}
		missing = next
	}
	// Uncomputed/future segments still require their declared input permissions.
	// The fixed input reader separately rejects holes in formal result coverage.
	for _, hole := range missing {
		if e = appendConfig(*draft, hole); e != nil {
			return out, e
		}
	}
	return out, nil
}
func (s *Service) inputRequirementsTx(ctx context.Context, tx pgx.Tx, tenant string, needs []monitoring.RawNeed) (inputRequirementPlan, error) {
	return planInputRequirements(ctx, needs, func(ctx context.Context, channel string, r planning.Range) (inputProfile, error) {
		return s.metadataInputProfileTx(ctx, tx, tenant, channel, r)
	}, 8)
}
func (s *Service) authorizeInputsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, needs []monitoring.RawNeed, history bool, capabilities ...string) error {
	_, e := s.authorizeInputRequirementsTx(ctx, tx, a, scope, needs, history, capabilities...)
	return e
}

func (s *Service) expandInputMandateTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, needs []monitoring.RawNeed) ([]monitoring.RawNeed, error) {
	for depth := 0; depth <= 8; depth++ {
		plan, e := s.authorizeInputRequirementsTx(ctx, tx, a, scope, needs, false)
		if e != nil {
			return nil, e
		}
		channels := []string{}
		for _, prefix := range plan.Prefixes {
			channels = append(channels, prefix.Channel)
		}
		slices.Sort(channels)
		channels = slices.Compact(channels)
		expanded := plan.Requirements
		if len(channels) > 0 {
			expanded, e = s.monitoring.CalculationPrefixAuthorityTx(ctx, tx, a, scope, expanded, channels)
			if e != nil {
				return nil, e
			}
		}
		expanded, e = normalizedNeeds(expanded)
		if e != nil {
			return nil, e
		}
		if fingerprint(expanded) == fingerprint(needs) {
			return expanded, nil
		}
		needs = expanded
	}
	return nil, problem("plan_limit_exceeded", "自动输入委托的递归展开超过预算。", 422)
}
func (s *Service) authorizeInputRequirementsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope monitoring.Scope, needs []monitoring.RawNeed, history bool, capabilities ...string) (inputRequirementPlan, error) {
	plan, e := s.inputRequirementsTx(ctx, tx, scope.Tenant, needs)
	if e != nil {
		return plan, e
	}
	if len(plan.Raw) > 0 {
		if e = s.monitoring.AuthorizeRawInputTx(ctx, tx, a, scope, plan.Raw, history, capabilities...); e != nil {
			return plan, e
		}
	}
	for _, need := range plan.Derived {
		if e = s.monitoring.AuthorizeCalculationOutputTx(ctx, tx, a, scope, need.Channel, []monitoring.Window{{From: need.From, To: need.To}}, history, capabilities...); e != nil {
			return plan, e
		}
	}
	return plan, nil
}
