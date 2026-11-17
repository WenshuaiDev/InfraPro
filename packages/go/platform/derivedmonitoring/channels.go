package derivedmonitoring

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func resourceScope(scope Scope) resources.Scope {
	if scope.Entry == "project" {
		return resources.ProjectScope(scope.Tenant, scope.Project)
	}
	return resources.TenantScope(scope.Tenant)
}
func sourceOwner(r resources.Resource) Owner {
	owner := Owner{Kind: resources.Kind(r.OwnerKind), FacilityID: r.FacilityID}
	switch r.OwnerKind {
	case "facility":
		owner.ID = r.FacilityID
	case "component":
		if r.ComponentID != nil {
			owner.ID = *r.ComponentID
		}
	case "monitoring_section":
		if r.MonitoringSectionID != nil {
			owner.ID = *r.MonitoringSectionID
		}
	case "borehole":
		if r.BoreholeID != nil {
			owner.ID = *r.BoreholeID
		}
	default:
		owner.Kind = resources.Point
		owner.ID = r.PointID
	}
	return owner
}
func semanticText(v string) string { return strings.Join(strings.Fields(v), " ") }

type bindingSemantic struct {
	Owner              Owner         `json:"owner"`
	Kind               string        `json:"source_kind"`
	MetricVersionID    string        `json:"metric_version_id,omitempty"`
	OutputContractHash string        `json:"output_contract_hash,omitempty"`
	Quantity           Quantity      `json:"quantity_spec"`
	Time               TimeSemantics `json:"time_semantics"`
	Direction          string        `json:"direction"`
	Reference          string        `json:"reference"`
	Aggregation        string        `json:"aggregation"`
}
type outputSemantics struct {
	Time      TimeSemantics `json:"time_semantics"`
	Direction string        `json:"direction"`
	Reference string        `json:"reference"`
}

func bindInputs(p *compute.Plan, bindings map[string]InputBinding, targets map[string]resources.CalculationTarget, allowRetired ...bool) (map[string]bindingSemantic, error) {
	out := map[string]bindingSemantic{}
	if len(bindings) != len(p.Rule.Inputs) {
		return out, invalid("/input_bindings", "每个输入槽必须且只能绑定一次。")
	}
	direction, reference := "", ""
	for _, in := range p.Rule.Inputs {
		binding, ok := bindings[in.Key]
		if !ok {
			return out, invalid("/input_bindings/"+in.Key, "缺少输入绑定。")
		}
		target, ok := targets["channel:"+binding.ChannelID]
		if !ok {
			return out, notFound()
		}
		r := target.Resource
		path := "/input_bindings/" + in.Key
		if target.Retired && (len(allowRetired) == 0 || !allowRetired[0]) {
			return out, problem("dependency_unavailable", "输入通道或其祖先已退役。", 409)
		}
		if !slices.Contains(in.SourceKinds, r.SourceKind) {
			return out, invalid(path, "输入来源类型不符合定义。")
		}
		s := bindingSemantic{Owner: sourceOwner(r), Kind: r.SourceKind, MetricVersionID: r.MetricVersionID, OutputContractHash: r.OutputContractHash, Quantity: in.Quantity, Time: in.Time, Direction: semanticText(r.ObservationDirection), Reference: semanticText(r.ReferenceBasis), Aggregation: semanticText(r.AggregationDescription)}
		if in.MetricVersionID != "" && in.MetricVersionID != r.MetricVersionID {
			return out, invalid(path, "输入公共指标版本与定义不一致。")
		}
		if r.SourceKind == "direct" {
			if r.UnitVersionID == nil || *r.UnitVersionID != in.Quantity.BaseUnitVersionID || in.Quantity.BasePower != 1 || in.Quantity.TimePower != 0 {
				return out, problem("unit_mismatch", "直接输入的固定公共单位与声明不一致。", 422)
			}
			if r.TimestampMeaning == "observation_time" && in.Time.Kind != "instant" || r.TimestampMeaning == "interval_end" && in.Time.Kind != "interval_end" {
				return out, invalid(path, "输入时间语义与原通道不一致。")
			}
		} else {
			var q Quantity
			var tm outputSemantics
			if json.Unmarshal(r.QuantitySpec, &q) != nil || json.Unmarshal(r.OutputSemantics, &tm) != nil || q != in.Quantity || tm.Time != in.Time {
				return out, problem("unit_mismatch", "派生输入的固定单位或时间语义不一致。", 422)
			}
			s.Direction, s.Reference = tm.Direction, tm.Reference
		}
		if in.Direction == "must_match" {
			if s.Direction == "" || direction != "" && direction != s.Direction {
				return out, invalid(path, "输入观测方向必须明确且一致。")
			}
			direction = s.Direction
		}
		if in.Reference == "must_match" {
			if s.Reference == "" || reference != "" && reference != s.Reference {
				return out, invalid(path, "输入参考基准必须明确且一致。")
			}
			reference = s.Reference
		}
		out[in.Key] = s
	}
	return out, nil
}
func (s *Service) graphTx(ctx context.Context, tx pgx.Tx, tenant, channel string, inputs []string) error {
	graph, e := dependencyGraphTx(ctx, tx, tenant)
	if e != nil {
		return e
	}
	graph[channel] = append(graph[channel], inputs...)
	_, e = planning.Order(graph, s.options.GraphDepth)
	var ge *planning.GraphError
	if errors.As(e, &ge) {
		return problem(ge.Code, "依赖会形成循环或超过八层计算深度。", 409)
	}
	return e
}

func dependencyGraphTx(ctx context.Context, tx pgx.Tx, tenant string) (map[string][]string, error) {
	graph := map[string][]string{}
	rows, e := tx.Query(ctx, `SELECT c.channel_id::text,d.input_channel_id::text FROM derived_channels c JOIN engineering_channels r ON r.tenant_id=c.tenant_id AND r.id=c.channel_id LEFT JOIN derived_dependencies d ON d.tenant_id=c.tenant_id AND (d.config_id=c.draft_config_id OR d.config_id IN(SELECT a.config_id FROM derived_config_assignments a WHERE a.tenant_id=c.tenant_id AND a.channel_id=c.channel_id AND a.superseded_revision IS NULL)) WHERE c.tenant_id=$1 AND r.deleted_at IS NULL ORDER BY c.channel_id,d.input_channel_id`, tenant)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var input *string
		if e = rows.Scan(&id, &input); e != nil {
			return nil, e
		}
		if _, ok := graph[id]; !ok {
			graph[id] = []string{}
		}
		if input != nil {
			graph[id] = append(graph[id], *input)
		}
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	return graph, nil
}
func (s *Service) channelTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string) (Channel, error) {
	var out Channel
	targets, e := s.resources.CalculationTargetsTx(ctx, tx, a, resourceScope(scope), []resources.Target{{Kind: resources.Channel, ID: id}}, false)
	if e != nil {
		return out, e
	}
	r := targets["channel:"+id].Resource
	if r.SourceKind != "derived" {
		return out, notFound()
	}
	if scope.Entry == "project" {
		r.CreatedBy = ""
		r.UpdatedBy = ""
		if r.CreatedProjectID != nil && *r.CreatedProjectID != scope.Project {
			r.CreatedProjectID = nil
		}
	}
	out.Resource = r
	owner := sourceOwner(r)
	if e = tx.QueryRow(ctx, `SELECT n.name,n.code,f.name FROM engineering_resource_nodes n JOIN engineering_facilities f ON f.tenant_id=n.tenant_id AND f.id=$4 WHERE n.tenant_id=$1 AND n.kind=$2 AND n.id=$3`, scope.Tenant, owner.Kind, owner.ID, owner.FacilityID).Scan(&out.OwnerName, &out.OwnerCode, &out.FacilityName); e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `SELECT control_state,control_revision::text,config_revision::text,mandate_revision::text,draft_config_id::text,auto_start_ns::text,stop_ns::text,health,blocked_reason FROM derived_channels WHERE tenant_id=$1 AND channel_id=$2`, scope.Tenant, id).Scan(&out.ControlState, &out.ControlRevision, &out.ConfigRevision, &out.MandateRevision, &out.DraftConfigID, &out.AutoStartNS, &out.StopNS, &out.Health, &out.BlockedReason)
	if e == nil {
		e = tx.QueryRow(ctx, `SELECT v.name,(SELECT a.config_id::text FROM derived_config_assignments a WHERE a.tenant_id=c.tenant_id AND a.channel_id=c.channel_id AND a.superseded_revision IS NULL AND a.effective_range @> $3::bigint LIMIT 1) FROM derived_channel_configs c JOIN derived_metric_versions v ON v.tenant_id=c.tenant_id AND v.id=c.definition_version_id WHERE c.tenant_id=$1 AND c.id=$2`, scope.Tenant, out.DraftConfigID, time.Now().UnixNano()).Scan(&out.MetricName, &out.CurrentConfigID)
	}
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	return out, e
}
func (s *Service) Channel(ctx context.Context, a access.Actor, scope Scope, id string) (Channel, error) {
	var out Channel
	e := s.transact(ctx, a, scope, "channels.detail", false, func(tx pgx.Tx) error { var e error; out, e = s.channelTx(ctx, tx, a, scope, id); return e })
	return out, e
}
func (s *Service) CreateChannel(ctx context.Context, a access.Actor, scope Scope, in CreateChannelInput, audit access.Meta) (ChannelResult, error) {
	var out ChannelResult
	if e := checkMeta(in.WriteMeta, false, false); e != nil {
		return out, e
	}
	if !codePattern.MatchString(in.Code) {
		return out, invalid("/code", "通道编号格式不正确。")
	}
	if e := checkName(in.Name, in.Description); e != nil {
		return out, e
	}
	if !access.ValidID(in.Owner.ID) || !access.ValidID(in.Owner.FacilityID) || !slices.Contains([]resources.Kind{resources.Facility, resources.Component, resources.MonitoringSection, resources.Borehole, resources.Point}, in.Owner.Kind) {
		return out, invalid("/owner", "需要有效且完整的工程所属对象。")
	}
	if in.Owner.Kind == resources.Facility && in.Owner.ID != in.Owner.FacilityID {
		return out, invalid("/owner", "设施所属对象必须等于设施编号。")
	}
	if len(in.InputBindings) < 1 || len(in.InputBindings) > 16 {
		return out, invalid("/input_bindings", "需要 1–16 个输入绑定。")
	}
	targets := []resources.Target{{Kind: in.Owner.Kind, ID: in.Owner.ID, Facility: in.Owner.FacilityID}}
	inputIDs := []string{}
	for _, binding := range in.InputBindings {
		if !access.ValidID(binding.ChannelID) {
			return out, invalid("/input_bindings", "输入通道编号无效。")
		}
		targets = append(targets, resources.Target{Kind: resources.Channel, ID: binding.ChannelID})
		inputIDs = append(inputIDs, binding.ChannelID)
	}
	e := s.transact(ctx, a, scope, "channels.create", true, func(tx pgx.Tx) error {
		resolved, e := s.resources.CalculationTargetsTx(ctx, tx, a, resourceScope(scope), targets, true)
		if e != nil {
			return e
		}
		owner := resolved[string(in.Owner.Kind)+":"+in.Owner.ID]
		if !owner.CanCreate {
			return access.ErrDenied
		}
		var previousAction, previousHash string
		var previous []byte
		e = tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM derived_operation_receipts WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.key(), a.ID, in.RequestID).Scan(&previousAction, &previousHash, &previous)
		if e == nil {
			if previousAction != "channels.create" || previousHash != fingerprint(in) {
				return problem("idempotency_conflict", "请求编号已用于不同内容。", 409)
			}
			if e = json.Unmarshal(previous, &out); e != nil {
				return e
			}
			if _, e = s.channelTx(ctx, tx, a, scope, out.Channel.Resource.ID); e != nil {
				return e
			}
			out.Replayed = true
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		v, e := versionTx(ctx, tx, scope, in.DefinitionVersionID, true)
		if e != nil {
			return e
		}
		if v.EngineVersion != compute.EngineVersion || v.CompilerVersion != compute.CompilerVersion {
			return problem("engine_version_unavailable", "此规则需要匹配的计算引擎版本。", 409)
		}
		var rule Rule
		if e = compute.Decode(v.Rule, &rule); e != nil {
			return e
		}
		if in.Quality != nil {
			rule.Policies.Quality = in.Quality
		}
		if in.NumericContract != nil && len(rule.Outputs) == 1 {
			rule.Outputs[0].Numeric = *in.NumericContract
		}
		p, e := s.compile(jsonBytes(rule), "channel", in.Parameters)
		if e != nil {
			return e
		}
		if _, e = publicReferences(ctx, tx, p, true); e != nil {
			return e
		}
		semantics, e := bindInputs(p, in.InputBindings, resolved)
		if e != nil {
			return e
		}
		if e = validateBaselineBindings(p, in.BaselineBindings); e != nil {
			return e
		}
		id, config := uuid.NewString(), uuid.NewString()
		if e = s.graphTx(ctx, tx, scope.Tenant, id, inputIDs); e != nil {
			return e
		}
		var graphRevision string
		if e = tx.QueryRow(ctx, `UPDATE derived_graph_states SET revision=revision+1 WHERE tenant_id=$1 RETURNING revision::text`, scope.Tenant).Scan(&graphRevision); e != nil {
			return e
		}
		hash := fingerprint([]any{"derived-bound-output/1", p.OutputContractHash, in.Owner, semantics, in.BaselineBindings})
		output := p.Rule.Outputs[0]
		tm := outputSemantics{Time: p.Times["node:"+output.Node]}
		for _, input := range p.Rule.Inputs {
			if input.Direction == "must_match" {
				tm.Direction = semantics[input.Key].Direction
			}
			if input.Reference == "must_match" {
				tm.Reference = semantics[input.Key].Reference
			}
		}
		r := resources.Resource{Kind: resources.Channel, ID: id, TenantID: scope.Tenant, FacilityID: in.Owner.FacilityID, SourceKind: "derived", OwnerKind: string(in.Owner.Kind), DerivedMetricVersionID: v.ID, QuantitySpec: jsonBytes(output.Quantity), OutputContractHash: hash, OutputSemantics: jsonBytes(tm), Code: in.Code, Name: in.Name, Description: in.Description, TimestampMeaning: "observation_time", ObservationDirection: tm.Direction, ReferenceBasis: tm.Reference}
		switch in.Owner.Kind {
		case resources.Component:
			r.ComponentID = &in.Owner.ID
		case resources.MonitoringSection:
			r.MonitoringSectionID = &in.Owner.ID
		case resources.Borehole:
			r.BoreholeID = &in.Owner.ID
		case resources.Point:
			r.PointID = in.Owner.ID
		}
		if tm.Time.Kind != "instant" {
			r.TimestampMeaning = "interval_end"
			r.AggregationDescription = tm.Time.Aggregation
		}
		r, e = s.resources.CreateCalculationChannelTx(ctx, tx, a, resourceScope(scope), targets[0], r, in.RequestID, in.Reason, audit)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO derived_channels(tenant_id,channel_id,draft_config_id) VALUES($1,$2,$3)`, scope.Tenant, id, config)
		if e != nil {
			return e
		}
		configHash := fingerprint([]any{"derived-config/1", p.PlanHash, in.InputBindings, semantics, in.BaselineBindings})
		baselines := in.BaselineBindings
		if baselines == nil {
			baselines = map[string]BaselineBinding{}
		}
		_, e = tx.Exec(ctx, `INSERT INTO derived_channel_configs(id,tenant_id,channel_id,definition_version_id,input_bindings,parameters,rule,compiled_plan,semantic_hash,config_hash,output_contract_hash,graph_revision,created_by,reason,baseline_bindings) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$9,$11,$12,$13,$14)`, config, scope.Tenant, id, v.ID, jsonBytes(in.InputBindings), jsonBytes(p.Values), jsonBytes(p.Rule), p.Canonical, hash, configHash, graphRevision, a.ID, in.Reason, jsonBytes(baselines))
		if e != nil {
			return e
		}
		keys := make([]string, 0, len(in.InputBindings))
		for key := range in.InputBindings {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			_, e = tx.Exec(ctx, `INSERT INTO derived_dependencies(tenant_id,config_id,slot_key,input_channel_id,input_kind,source_semantics,impact) VALUES($1,$2,$3,$4,$5,$6,$7)`, scope.Tenant, config, key, in.InputBindings[key].ChannelID, semantics[key].Kind, jsonBytes(semantics[key]), jsonBytes(map[string]string{"kind": "compiled_rule", "compiled_plan_hash": p.PlanHash}))
			if e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `INSERT INTO derived_channel_heads(tenant_id,channel_id) VALUES($1,$2)`, scope.Tenant, id); e != nil {
			return e
		}
		out.Channel, e = s.channelTx(ctx, tx, a, scope, id)
		if e != nil {
			return e
		}
		audit.ActorID = a.ID
		audit.ActorScope = scope.Entry
		audit.TenantID = scope.Tenant
		audit.ProjectID = scope.Project
		audit.Reason = in.Reason
		if e = access.Audit(ctx, tx, "derived.channels.create", "derived_channel", id, "", nil, out, audit); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO derived_changes(tenant_id,project_id,actor_kind,actor_id,operation,target_kind,target_id,request_id,reason,after) VALUES($1,$2,'user',$3,'channels.create','channel',$4,$5,$6,$7)`, scope.Tenant, nullable(scope.Project), a.ID, id, in.RequestID, in.Reason, jsonBytes(out))
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO derived_operation_receipts(tenant_id,project_id,actor_id,request_id,operation,payload_hash,result) VALUES($1,$2,$3,$4,'channels.create',$5,$6)`, scope.Tenant, nullable(scope.Project), a.ID, in.RequestID, fingerprint(in), jsonBytes(out))
		return e
	})
	return out, e
}

func (s *Service) Channels(ctx context.Context, a access.Actor, scope Scope, f ChannelFilter) (ChannelPage, error) {
	out := ChannelPage{Items: []Channel{}}
	if f.Limit == 0 {
		f.Limit = 30
	}
	if f.Limit < 1 || f.Limit > 100 || len(f.Query) > 400 || !slices.Contains([]string{"", "draft", "enabled", "paused", "retired"}, f.ControlState) || f.FacilityID != "" && !access.ValidID(f.FacilityID) || !slices.Contains([]string{"", "facility", "component", "monitoring_section", "borehole", "point"}, f.OwnerKind) || f.OwnerID != "" && (!access.ValidID(f.OwnerID) || f.OwnerKind == "") {
		return out, invalid("/filter", "通道筛选无效。")
	}
	keyFilter := f
	keyFilter.Cursor = ""
	key := fingerprint([]any{a.ID, scope, keyFilter})
	cursor, e := decodeCursor(f.Cursor, key)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "channels.list", false, func(tx pgx.Tx) error {
		if _, e := s.resources.CalculationTargetsTx(ctx, tx, a, resourceScope(scope), nil, false); e != nil {
			return e
		}
		where := ""
		if scope.Entry == "project" {
			where = ` AND EXISTS(SELECT 1 FROM project_resource_visibility($1,$10,NULL) v WHERE v.kind='channel' AND v.id=c.id AND v.full_access)`
		}
		args := []any{scope.Tenant, f.Query, cursor.At, nullable(cursor.ID), f.Limit + 1, f.ControlState, nullable(f.FacilityID), f.OwnerKind, nullable(f.OwnerID)}
		if scope.Entry == "project" {
			args = append(args, scope.Project)
		}
		rows, e := tx.Query(ctx, `SELECT c.id::text,c.created_at FROM engineering_channels c JOIN derived_channels d ON d.tenant_id=c.tenant_id AND d.channel_id=c.id WHERE c.tenant_id=$1 AND c.deleted_at IS NULL AND ($2='' OR c.name ILIKE '%'||$2||'%' OR c.code ILIKE '%'||$2||'%') AND ($3::timestamptz IS NULL OR (c.created_at,c.id)<($3,$4::uuid)) AND ($6='' OR d.control_state=$6) AND ($7::uuid IS NULL OR c.facility_id=$7) AND ($8='' OR c.owner_kind=$8) AND ($9::uuid IS NULL OR CASE c.owner_kind WHEN 'facility' THEN c.facility_id WHEN 'component' THEN c.component_id WHEN 'monitoring_section' THEN c.monitoring_section_id WHEN 'borehole' THEN c.borehole_id ELSE c.point_id END=$9)`+where+` ORDER BY c.created_at DESC,c.id DESC LIMIT $5`, args...)
		if e != nil {
			return e
		}
		type item struct {
			id string
			at time.Time
		}
		ids := []item{}
		for rows.Next() {
			var i item
			if e = rows.Scan(&i.id, &i.at); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, i)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(ids) > f.Limit {
			ids = ids[:f.Limit]
			last := ids[len(ids)-1]
			out.NextCursor = encodeCursor(last.at, last.id, key)
		}
		for _, i := range ids {
			v, e := s.channelTx(ctx, tx, a, scope, i.id)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		return nil
	})
	return out, e
}

const configSelect = `SELECT id::text,channel_id::text,definition_version_id::text,input_bindings,parameters,rule,semantic_hash,config_hash,output_contract_hash,graph_revision::text,created_at,baseline_bindings,COALESCE(base_config_id::text,''),change_kind,semantic_intent_confirmed,reset_baselines FROM derived_channel_configs`

func scanConfig(row pgx.Row) (ChannelConfig, error) {
	var c ChannelConfig
	e := row.Scan(&c.ID, &c.ChannelID, &c.DefinitionVersionID, &c.InputBindings, &c.Parameters, &c.Rule, &c.SemanticHash, &c.ConfigHash, &c.OutputContractHash, &c.GraphRevision, &c.CreatedAt, &c.BaselineBindings, &c.BaseConfigID, &c.ChangeKind, &c.SemanticIntentConfirmed, &c.ResetBaselines)
	return c, e
}
func (s *Service) configVisible(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, c *ChannelConfig) error {
	for _, b := range c.InputBindings {
		_, e := s.resources.CalculationTargetsTx(ctx, tx, a, resourceScope(scope), []resources.Target{{Kind: resources.Channel, ID: b.ChannelID}}, false)
		if e == nil {
			continue
		}
		var re *resources.Error
		var ae *access.Error
		if errors.As(e, &re) && (re.Status == 403 || re.Status == 404) || errors.As(e, &ae) && (ae.Status == 403 || ae.Status == 404) {
			c.InputsRestricted = true
			c.InputBindings = nil
			c.BaselineBindings = nil
			c.Parameters = nil
			c.Rule = nil
			return nil
		}
		return e
	}
	return nil
}
func (s *Service) ChannelConfigs(ctx context.Context, a access.Actor, scope Scope, id string, f MetricFilter) (ConfigPage, error) {
	out := ConfigPage{Items: []ChannelConfig{}}
	f, e := normalizeFilter(f)
	if e != nil {
		return out, e
	}
	key := fingerprint([]any{filterKey(a, scope, f), id, "configs"})
	cursor, e := decodeCursor(f.Cursor, key)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "channels.configs.list", false, func(tx pgx.Tx) error {
		if _, e := s.channelTx(ctx, tx, a, scope, id); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, configSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, scope.Tenant, id, cursor.At, nullable(cursor.ID), f.Limit+1)
		if e != nil {
			return e
		}
		for rows.Next() {
			c, e := scanConfig(rows)
			if e != nil {
				rows.Close()
				return e
			}
			out.Items = append(out.Items, c)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[len(out.Items)-1]
			out.NextCursor = encodeCursor(last.CreatedAt, last.ID, key)
		}
		for i := range out.Items {
			if e = s.configVisible(ctx, tx, a, scope, &out.Items[i]); e != nil {
				return e
			}
		}
		return nil
	})
	return out, e
}
func (s *Service) ChannelDependencies(ctx context.Context, a access.Actor, scope Scope, id string) (DependencyPage, error) {
	out := DependencyPage{Items: []ChannelDependency{}}
	e := s.transact(ctx, a, scope, "channels.dependencies", false, func(tx pgx.Tx) error {
		channel, e := s.channelTx(ctx, tx, a, scope, id)
		if e != nil {
			return e
		}
		c, e := scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, channel.DraftConfigID))
		if e != nil {
			return e
		}
		if e = s.configVisible(ctx, tx, a, scope, &c); e != nil {
			return e
		}
		if c.InputsRestricted {
			out.Restricted = true
			return nil
		}
		rows, e := tx.Query(ctx, `SELECT config_id::text,slot_key,input_channel_id::text,input_kind,source_semantics FROM derived_dependencies WHERE tenant_id=$1 AND config_id=$2 ORDER BY slot_key`, scope.Tenant, c.ID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var d ChannelDependency
			if e = rows.Scan(&d.ConfigID, &d.Slot, &d.ChannelID, &d.SourceKind, &d.SourceSemantics); e != nil {
				return e
			}
			out.Items = append(out.Items, d)
		}
		return rows.Err()
	})
	return out, e
}
