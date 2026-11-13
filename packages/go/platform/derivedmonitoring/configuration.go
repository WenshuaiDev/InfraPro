package derivedmonitoring

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) SaveChannelConfig(ctx context.Context, a access.Actor, scope Scope, id string, in SaveConfigInput, audit access.Meta) (ConfigResult, error) {
	var out ConfigResult
	in.BaselineBindings = normalizeBaselineBindings(in.BaselineBindings)
	if e := checkMeta(in.WriteMeta, true, true); e != nil {
		return out, e
	}
	if !access.ValidID(id) || !access.ValidID(in.BaseConfigID) || !access.ValidID(in.DefinitionVersionID) {
		return out, invalid("/config_id", "需要有效的基础配置和定义版本。")
	}
	if !slices.Contains([]string{"ordinary", "correction"}, in.ChangeKind) || in.ChangeKind == "correction" && !in.SemanticIntentConfirmed || in.ChangeKind != "correction" && (in.SemanticIntentConfirmed || in.ResetBaselines) {
		return out, invalid("/change_kind", "算法修正必须明确确认保持原有业务意图；重新选择基准也须使用修正操作。")
	}
	if len(in.InputBindings) < 1 || len(in.InputBindings) > 16 {
		return out, invalid("/input_bindings", "需要1–16个输入。")
	}
	e := s.transact(ctx, a, scope, "channels.configs.create", true, func(tx pgx.Tx) error {
		channel, e := s.maintenanceTargetTx(ctx, tx, a, scope, id, in.ChangeKind == "correction")
		if e != nil {
			return e
		}
		if in.ChangeKind == "correction" {
			if e = requireAnyDerivedTx(ctx, tx, a, scope, "channels.backfill"); e != nil {
				return e
			}
		}
		var saved []byte
		var operation, hash string
		e = tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM derived_operation_receipts WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.key(), a.ID, in.RequestID).Scan(&operation, &hash, &saved)
		if e == nil {
			if operation != "channels.configs.create" || hash != fingerprint([]any{id, in}) {
				return problem("idempotency_conflict", "请求编号已用于其他内容。", 409)
			}
			if e = json.Unmarshal(saved, &out); e != nil {
				return e
			}
			out.Replayed = true
			return s.configVisible(ctx, tx, a, scope, &out.Config)
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if channel.ConfigRevision != in.Revision {
			return conflict()
		}
		_, base, basePlan, e := s.calculationConfigTx(ctx, tx, a, scope, id, in.BaseConfigID)
		if e != nil {
			return e
		}
		v, e := versionTx(ctx, tx, scope, in.DefinitionVersionID, true)
		if e != nil {
			return e
		}
		initial, e := versionTx(ctx, tx, scope, channel.Resource.DerivedMetricVersionID, false)
		if e != nil {
			return e
		}
		if v.DefinitionID != initial.DefinitionID {
			return problem("new_channel_required", "更换指标定义需要新建通道。", 409)
		}
		if v.EngineVersion != compute.EngineVersion || v.CompilerVersion != compute.CompilerVersion {
			return problem("engine_version_unavailable", "配置要求匹配的计算引擎。", 409)
		}
		var rule Rule
		if e = compute.Decode(v.Rule, &rule); e != nil {
			return e
		}
		if in.Quality != nil {
			rule.Policies.Quality = in.Quality
		}
		if in.NumericContract != nil {
			rule.Outputs[0].Numeric = *in.NumericContract
		}
		p, e := s.compile(jsonBytes(rule), "channel", in.Parameters)
		if e != nil {
			return e
		}
		if _, e = publicReferences(ctx, tx, p, true); e != nil {
			return e
		}
		targets := []resources.Target{}
		ids := []string{}
		for _, binding := range in.InputBindings {
			if !access.ValidID(binding.ChannelID) {
				return invalid("/input_bindings", "输入通道编号无效。")
			}
			targets = append(targets, resources.Target{Kind: resources.Channel, ID: binding.ChannelID})
			ids = append(ids, binding.ChannelID)
		}
		resolved, e := s.resources.CalculationTargetsTx(ctx, tx, a, resourceScope(scope), targets, false)
		if e != nil {
			return e
		}
		semantics, e := bindInputs(p, in.InputBindings, resolved, in.ChangeKind == "correction")
		if e != nil {
			return e
		}
		if in.BaselineBindings == nil {
			in.BaselineBindings = map[string]BaselineBinding{}
		}
		if e = validateBaselineBindings(p, in.BaselineBindings); e != nil {
			return e
		}
		baseSemantics := map[string]bindingSemantic{}
		rows, e := tx.Query(ctx, `SELECT slot_key,source_semantics FROM derived_dependencies WHERE tenant_id=$1 AND config_id=$2 ORDER BY slot_key`, scope.Tenant, base.ID)
		if e != nil {
			return e
		}
		for rows.Next() {
			var slot string
			var value bindingSemantic
			if e = rows.Scan(&slot, &value); e != nil {
				rows.Close()
				return e
			}
			baseSemantics[slot] = value
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		// A different channel can replace the same business position only when
		// all immutable slot semantics, including owner identity, are identical.
		if fingerprint(baseSemantics) != fingerprint(semantics) {
			return problem("new_channel_required", "输入对象组合、单位、方向或基准语义发生变化，需要新建通道。", 409)
		}
		if in.ChangeKind == "ordinary" && p.OutputContractHash != basePlan.OutputContractHash || in.ChangeKind == "correction" && p.CorrectionIdentity() != basePlan.CorrectionIdentity() {
			return problem("new_channel_required", "业务含义、窗口或身份参数发生变化，需要新建通道。", 409)
		}
		if !in.ResetBaselines && fingerprint(in.BaselineBindings) != fingerprint(normalizeBaselineBindings(base.BaselineBindings)) {
			return problem("baseline_selection_required", "改变固定基准需要明确选择重新建立基准。", 409)
		}
		for ref, b := range base.BaselineBindings {
			if !in.ResetBaselines && base.InputBindings[b.Input].ChannelID != in.InputBindings[b.Input].ChannelID {
				return invalid("/baseline_bindings/"+ref, "固定基准来源保持原采用身份；更换基准输入必须明确重新建立基准。")
			}
		}
		if e = s.graphTx(ctx, tx, scope.Tenant, id, ids); e != nil {
			return e
		}
		var graph string
		if e = tx.QueryRow(ctx, `UPDATE derived_graph_states SET revision=revision+1 WHERE tenant_id=$1 RETURNING revision::text`, scope.Tenant).Scan(&graph); e != nil {
			return e
		}
		config := uuid.NewString()
		bound := fingerprint([]any{"derived-bound-output/1", p.OutputContractHash, sourceOwner(channel.Resource), semantics, in.BaselineBindings})
		configHash := fingerprint([]any{"derived-config/1", p.PlanHash, in.InputBindings, semantics, in.BaselineBindings, in.ChangeKind, in.ResetBaselines, base.ID})
		_, e = tx.Exec(ctx, `INSERT INTO derived_channel_configs(id,tenant_id,channel_id,definition_version_id,input_bindings,parameters,rule,compiled_plan,semantic_hash,config_hash,output_contract_hash,graph_revision,created_by,reason,baseline_bindings,base_config_id,change_kind,semantic_intent_confirmed,reset_baselines) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, config, scope.Tenant, id, v.ID, jsonBytes(in.InputBindings), jsonBytes(p.Values), jsonBytes(p.Rule), p.Canonical, bound, configHash, channel.Resource.OutputContractHash, graph, a.ID, in.Reason, jsonBytes(in.BaselineBindings), base.ID, in.ChangeKind, in.SemanticIntentConfirmed, in.ResetBaselines)
		if e != nil {
			return e
		}
		keys := []string{}
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
		if !in.ResetBaselines {
			_, e = tx.Exec(ctx, `INSERT INTO derived_fixed_baselines(tenant_id,config_id,reference_key,snapshot_id,evidence,evidence_hash,created_by) SELECT tenant_id,$3,reference_key,snapshot_id,evidence,evidence_hash,$4 FROM derived_fixed_baselines WHERE tenant_id=$1 AND config_id=$2`, scope.Tenant, base.ID, config, a.ID)
			if e != nil {
				return e
			}
		}
		if channel.ControlState == "draft" {
			_, e = tx.Exec(ctx, `UPDATE derived_channels SET draft_config_id=$3,config_revision=config_revision+1 WHERE tenant_id=$1 AND channel_id=$2`, scope.Tenant, id, config)
			if e != nil {
				return e
			}
		}
		out.Config, e = scanConfig(tx.QueryRow(ctx, configSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, config))
		if e != nil {
			return e
		}
		out.Channel, e = s.channelTx(ctx, tx, a, scope, id)
		if e != nil {
			return e
		}
		return configOperationTx(ctx, tx, a, scope, id, "channels.configs.create", in.WriteMeta, fingerprint([]any{id, in}), base, out, audit)
	})
	return out, e
}
func normalizeBaselineBindings(bindings map[string]BaselineBinding) map[string]BaselineBinding {
	if bindings == nil {
		return map[string]BaselineBinding{}
	}
	return bindings
}
func configOperationTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id, action string, meta WriteMeta, hash string, before, after any, audit access.Meta) error {
	audit.ActorID = a.ID
	audit.ActorType = "account"
	audit.ActorScope = scope.Entry
	audit.TenantID = scope.Tenant
	audit.ProjectID = scope.Project
	audit.Reason = meta.Reason
	if e := access.Audit(ctx, tx, "derived."+action, "derived_channel", id, "", before, after, audit); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `INSERT INTO derived_changes(tenant_id,project_id,actor_kind,actor_id,operation,target_kind,target_id,request_id,reason,before,after) VALUES($1,$2,'user',$3,$4,'channel',$5,$6,$7,$8,$9)`, scope.Tenant, nullable(scope.Project), a.ID, action, id, meta.RequestID, meta.Reason, jsonBytes(before), jsonBytes(after)); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `INSERT INTO derived_operation_receipts(tenant_id,project_id,actor_id,request_id,operation,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6,$7)`, scope.Tenant, nullable(scope.Project), a.ID, meta.RequestID, action, hash, jsonBytes(after))
	return e
}

func scanAssignment(row pgx.Row) (ConfigAssignment, error) {
	var v ConfigAssignment
	var from int64
	var to *int64
	e := row.Scan(&v.ID, &v.ConfigID, &from, &to, &v.CreatedRevision, &v.SupersededRevision)
	v.Range = rangeDTO(from, to)
	return v, e
}

const assignmentSelect = `SELECT id::text,config_id::text,lower(effective_range),upper(effective_range),created_revision::text,superseded_revision::text FROM derived_config_assignments`

func (s *Service) ChannelAssignments(ctx context.Context, a access.Actor, scope Scope, id, revision, cursor string) (AssignmentPage, error) {
	out := AssignmentPage{Items: []ConfigAssignment{}}
	if cursor != "" && !access.ValidID(cursor) {
		return out, invalid("/cursor", "分配游标无效。")
	}
	e := s.transact(ctx, a, scope, "channels.assignments", false, func(tx pgx.Tx) error {
		c, e := s.channelTx(ctx, tx, a, scope, id)
		if e != nil {
			return e
		}
		out.Revision = revision
		if revision == "" {
			out.Revision = c.ConfigRevision
		}
		r, e := revisionNumber(out.Revision)
		if e != nil {
			return e
		}
		current, _ := strconv.ParseInt(c.ConfigRevision, 10, 64)
		if r > current {
			return invalid("/revision", "配置修订尚不存在。")
		}
		rows, e := tx.Query(ctx, assignmentSelect+` WHERE tenant_id=$1 AND channel_id=$2 AND created_revision<=$3 AND (superseded_revision IS NULL OR superseded_revision>$3) AND ($4='' OR id::text>$4) ORDER BY id LIMIT 101`, scope.Tenant, id, r, cursor)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := scanAssignment(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > 100 {
			out.Items = out.Items[:100]
			out.NextCursor = out.Items[99].ID
		}
		return nil
	})
	return out, e
}
