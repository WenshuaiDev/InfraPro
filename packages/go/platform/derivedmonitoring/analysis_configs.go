package derivedmonitoring

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const analysisConfigSelect = `SELECT id::text,tenant_id::text,project_id::text,code,name,description,status,revision::text,latest_version::text,created_at,updated_at,draft_revision::text,draft FROM analysis_configs`
const analysisVersionSelect = `SELECT v.id::text,v.config_id::text,v.version_no::text,v.name,v.body,v.config_hash,v.plan_hash,v.engine_version,v.compiler_version,COALESCE(v.base_version_id::text,''),v.reset_baselines,v.created_at FROM analysis_config_versions v JOIN analysis_configs c ON c.tenant_id=v.tenant_id AND c.id=v.config_id`

func scanAnalysisConfig(row pgx.Row) (AnalysisConfig, error) {
	var c AnalysisConfig
	c.Draft = &AnalysisDraft{}
	e := row.Scan(&c.ID, &c.TenantID, &c.ProjectID, &c.Code, &c.Name, &c.Description, &c.Status, &c.Revision, &c.LatestVersion, &c.CreatedAt, &c.UpdatedAt, &c.Draft.Revision, &c.Draft.Body)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	return c, e
}
func scanAnalysisVersion(row pgx.Row) (AnalysisVersion, error) {
	var v AnalysisVersion
	e := row.Scan(&v.ID, &v.ConfigID, &v.Number, &v.Name, &v.Body, &v.ConfigHash, &v.PlanHash, &v.EngineVersion, &v.CompilerVersion, &v.BaseVersionID, &v.ResetBaselines, &v.CreatedAt)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	return v, e
}
func analysisConfigTx(ctx context.Context, tx pgx.Tx, scope Scope, id string, lock bool) (AnalysisConfig, error) {
	if !access.ValidID(id) {
		return AnalysisConfig{}, notFound()
	}
	query := analysisConfigSelect + ` WHERE tenant_id=$1 AND scope_key=$2 AND id=$3`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanAnalysisConfig(tx.QueryRow(ctx, query, scope.Tenant, scope.key(), id))
}
func analysisVersionTx(ctx context.Context, tx pgx.Tx, scope Scope, id string) (AnalysisVersion, error) {
	if !access.ValidID(id) {
		return AnalysisVersion{}, notFound()
	}
	return scanAnalysisVersion(tx.QueryRow(ctx, analysisVersionSelect+` WHERE v.tenant_id=$1 AND c.scope_key=$2 AND v.id=$3`, scope.Tenant, scope.key(), id))
}
func (s *Service) analysisBindingsTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, body AnalysisBody) (map[string]resources.CalculationTarget, error) {
	targets := []resources.Target{}
	if len(body.InputBindings) > 16 {
		return nil, invalid("/body/input_bindings", "输入最多16个。")
	}
	for _, binding := range body.InputBindings {
		if !access.ValidID(binding.ChannelID) {
			return nil, invalid("/body/input_bindings", "输入通道编号无效。")
		}
		targets = append(targets, resources.Target{Kind: resources.Channel, ID: binding.ChannelID})
	}
	if len(targets) == 0 {
		return map[string]resources.CalculationTarget{}, nil
	}
	return s.resources.CalculationTargetsTx(ctx, tx, a, resourceScope(scope), targets, false)
}
func (s *Service) qualifyAnalysisConfigTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, c *AnalysisConfig) error {
	if c.Draft == nil {
		return nil
	}
	_, e := s.analysisBindingsTx(ctx, tx, a, scope, c.Draft.Body)
	var de *Error
	var re *resources.Error
	if errors.Is(e, access.ErrDenied) || errors.As(e, &re) && (re.Status == 404 || re.Status == 403) || errors.As(e, &de) && de.Status == 404 {
		c.Draft = nil
		c.InputsRestricted = true
		return nil
	}
	return e
}
func (s *Service) AnalysisConfig(ctx context.Context, a access.Actor, scope Scope, id string) (AnalysisConfig, error) {
	var out AnalysisConfig
	e := s.transact(ctx, a, scope, "analysis.configs.detail", false, func(tx pgx.Tx) error {
		var e error
		out, e = analysisConfigTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		return s.qualifyAnalysisConfigTx(ctx, tx, a, scope, &out)
	})
	return out, e
}
func (s *Service) AnalysisConfigs(ctx context.Context, a access.Actor, scope Scope, filter MetricFilter) (AnalysisConfigPage, error) {
	out := AnalysisConfigPage{Items: []AnalysisConfig{}}
	if filter.State != "" && filter.State != "active" && filter.State != "archived" {
		return out, invalid("/state", "不支持该分析状态。")
	}
	if filter.Cursor != "" && !access.ValidID(filter.Cursor) {
		return out, invalid("/cursor", "分页游标无效。")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	e := s.transact(ctx, a, scope, "analysis.configs.list", false, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, analysisConfigSelect+` WHERE tenant_id=$1 AND scope_key=$2 AND ($3::uuid IS NULL OR id>$3) AND ($4='' OR name ILIKE '%'||$4||'%' OR code ILIKE '%'||$4||'%') AND ($6='' OR status=$6) ORDER BY id LIMIT $5`, scope.Tenant, scope.key(), nullable(filter.Cursor), strings.TrimSpace(filter.Query), limit+1, filter.State)
		if e != nil {
			return e
		}
		for rows.Next() {
			c, e := scanAnalysisConfig(rows)
			if e != nil {
				rows.Close()
				return e
			}
			c.Draft = nil
			out.Items = append(out.Items, c)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			out.NextCursor = out.Items[limit-1].ID
		}
		return nil
	})
	return out, e
}
func (s *Service) AnalysisVersions(ctx context.Context, a access.Actor, scope Scope, id, cursor string) (AnalysisVersionPage, error) {
	out := AnalysisVersionPage{Items: []AnalysisVersion{}}
	var before int64 = 1<<63 - 1
	if cursor != "" {
		var e error
		before, e = revisionNumber(cursor)
		if e != nil {
			return out, e
		}
	}
	e := s.transact(ctx, a, scope, "analysis.configs.versions", false, func(tx pgx.Tx) error {
		if _, e := analysisConfigTx(ctx, tx, scope, id, false); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, analysisVersionSelect+` WHERE v.tenant_id=$1 AND c.scope_key=$2 AND v.config_id=$3 AND v.version_no<$4 ORDER BY v.version_no DESC LIMIT 51`, scope.Tenant, scope.key(), id, before)
		if e != nil {
			return e
		}
		for rows.Next() {
			v, e := scanAnalysisVersion(rows)
			if e != nil {
				rows.Close()
				return e
			}
			out.Items = append(out.Items, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, v := range out.Items {
			if _, e = s.analysisBindingsTx(ctx, tx, a, scope, v.Body); e != nil {
				return e
			}
		}
		if len(out.Items) > 50 {
			out.Items = out.Items[:50]
			out.NextCursor = out.Items[49].Number
		}
		return nil
	})
	return out, e
}
func analysisReceiptTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, action string, meta WriteMeta, payload any, result any) (bool, error) {
	var operation, hash string
	var data []byte
	e := tx.QueryRow(ctx, `SELECT operation,payload_hash,result FROM derived_operation_receipts WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, scope.Tenant, scope.key(), a.ID, meta.RequestID).Scan(&operation, &hash, &data)
	if e == pgx.ErrNoRows {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if operation != action || hash != fingerprint(payload) {
		return false, problem("idempotency_conflict", "请求编号已用于不同内容。", 409)
	}
	return true, json.Unmarshal(data, result)
}
func analysisCompleteTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, action, kind, id string, meta WriteMeta, payload, before, after any, audit access.Meta) error {
	audit.ActorID = a.ID
	audit.ActorType = "account"
	audit.ActorScope = scope.Entry
	audit.TenantID = scope.Tenant
	audit.ProjectID = scope.Project
	audit.Reason = meta.Reason
	if e := access.Audit(ctx, tx, "derived."+action, kind, id, "", before, after, audit); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `INSERT INTO derived_changes(tenant_id,project_id,actor_kind,actor_id,operation,target_kind,target_id,request_id,reason,before,after) VALUES($1,$2,'user',$3,$4,$5,$6,$7,$8,$9,$10)`, scope.Tenant, nullable(scope.Project), a.ID, action, kind, id, meta.RequestID, meta.Reason, jsonBytes(before), jsonBytes(after)); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `INSERT INTO derived_operation_receipts(tenant_id,project_id,actor_id,request_id,operation,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6,$7)`, scope.Tenant, nullable(scope.Project), a.ID, meta.RequestID, action, fingerprint(payload), jsonBytes(after))
	return e
}
func checkAnalysisBody(body AnalysisBody) error {
	if len(jsonBytes(body)) > 1<<20 || !json.Valid(body.Rule) {
		return invalid("/body/rule", "需要有效 JSON 规则，配置最多1 MiB。")
	}
	return nil
}
func (s *Service) CreateAnalysis(ctx context.Context, a access.Actor, scope Scope, in CreateAnalysisInput, audit access.Meta) (AnalysisConfigResult, error) {
	var out AnalysisConfigResult
	if e := checkMeta(in.WriteMeta, false, false); e != nil {
		return out, e
	}
	if !codePattern.MatchString(in.Code) {
		return out, invalid("/code", "分析编号格式不正确。")
	}
	if e := checkName(in.Name, in.Description); e != nil {
		return out, e
	}
	if e := checkAnalysisBody(in.Body); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "analysis.configs.create", true, func(tx pgx.Tx) error {
		found, e := analysisReceiptTx(ctx, tx, a, scope, "analysis.configs.create", in.WriteMeta, in, &out)
		if e != nil {
			return e
		}
		if found {
			out.Replayed = true
			return s.qualifyAnalysisConfigTx(ctx, tx, a, scope, &out.Config)
		}
		if _, e = s.analysisBindingsTx(ctx, tx, a, scope, in.Body); e != nil {
			return e
		}
		id := uuid.NewString()
		_, e = tx.Exec(ctx, `INSERT INTO analysis_configs(id,tenant_id,project_id,code,name,description,draft,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, scope.Tenant, nullable(scope.Project), in.Code, in.Name, in.Description, jsonBytes(in.Body), a.ID)
		if e != nil {
			return e
		}
		out.Config, e = analysisConfigTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		return analysisCompleteTx(ctx, tx, a, scope, "analysis.configs.create", "analysis_config", id, in.WriteMeta, in, nil, out, audit)
	})
	return out, e
}
func (s *Service) SaveAnalysis(ctx context.Context, a access.Actor, scope Scope, id string, in SaveAnalysisInput, audit access.Meta) (AnalysisConfigResult, error) {
	var out AnalysisConfigResult
	if e := checkMeta(in.WriteMeta, true, false); e != nil {
		return out, e
	}
	if e := checkName(in.Name, in.Description); e != nil {
		return out, e
	}
	if e := checkAnalysisBody(in.Body); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "analysis.configs.save", true, func(tx pgx.Tx) error {
		payload := []any{id, in}
		found, e := analysisReceiptTx(ctx, tx, a, scope, "analysis.configs.save", in.WriteMeta, payload, &out)
		if e != nil {
			return e
		}
		if found {
			out.Replayed = true
			return s.qualifyAnalysisConfigTx(ctx, tx, a, scope, &out.Config)
		}
		before, e := analysisConfigTx(ctx, tx, scope, id, true)
		if e != nil {
			return e
		}
		if before.Revision != in.Revision {
			return conflict()
		}
		if before.Status != "active" {
			return problem("analysis_archived", "分析配置已归档。", 409)
		}
		if _, e = s.analysisBindingsTx(ctx, tx, a, scope, in.Body); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE analysis_configs SET name=$3,description=$4,draft=$5,draft_revision=draft_revision+1,revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id, in.Name, in.Description, jsonBytes(in.Body))
		if e != nil {
			return e
		}
		out.Config, e = analysisConfigTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		return analysisCompleteTx(ctx, tx, a, scope, "analysis.configs.save", "analysis_config", id, in.WriteMeta, payload, before, out, audit)
	})
	return out, e
}
func (s *Service) compileAnalysisTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, body AnalysisBody, newUse bool) (*compute.Plan, AnalysisBody, error) {
	p, e := s.compile(body.Rule, "analysis", body.Parameters)
	if e != nil {
		return nil, body, e
	}
	if _, e = publicReferences(ctx, tx, p, newUse); e != nil {
		return nil, body, e
	}
	bound, e := s.analysisBindingsTx(ctx, tx, a, scope, body)
	if e != nil {
		return nil, body, e
	}
	if _, e = bindInputs(p, body.InputBindings, bound, true); e != nil {
		return nil, body, e
	}
	if e = validateBaselineBindings(p, body.BaselineBindings); e != nil {
		return nil, body, e
	}
	body.Rule = jsonBytes(p.Rule)
	body.Parameters = p.Values
	if body.BaselineBindings == nil {
		body.BaselineBindings = map[string]BaselineBinding{}
	}
	return p, body, nil
}
func analysisValidation(c AnalysisConfig, p *compute.Plan, body AnalysisBody) AnalysisValidation {
	configHash := fingerprint([]any{"analysis-config/1", body, p.PlanHash, compute.EngineVersion, compute.CompilerVersion})
	out := AnalysisValidation{DraftRevision: c.Draft.Revision, ConfigHash: configHash, PlanHash: p.PlanHash, Outputs: p.Rule.Outputs, IdentityParameters: []string{}, Issues: []Issue{}}
	for _, parameter := range p.Rule.Parameters {
		if parameter.IdentityAffecting {
			out.IdentityParameters = append(out.IdentityParameters, parameter.Key)
		}
	}
	out.ValidationHash = fingerprint([]any{c.ID, c.Revision, c.Draft.Revision, configHash})
	return out
}
func (s *Service) ValidateAnalysis(ctx context.Context, a access.Actor, scope Scope, id string, in ValidateInput) (AnalysisValidation, error) {
	var out AnalysisValidation
	e := s.transact(ctx, a, scope, "analysis.configs.validate", false, func(tx pgx.Tx) error {
		c, e := analysisConfigTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		if c.Draft.Revision != in.DraftRevision {
			return conflict()
		}
		if c.Status != "active" {
			return problem("analysis_archived", "分析配置已归档。", 409)
		}
		p, body, e := s.compileAnalysisTx(ctx, tx, a, scope, c.Draft.Body, true)
		if e != nil {
			return e
		}
		out = analysisValidation(c, p, body)
		return nil
	})
	return out, e
}
func (s *Service) PublishAnalysis(ctx context.Context, a access.Actor, scope Scope, id string, in PublishAnalysisInput, audit access.Meta) (AnalysisConfigResult, error) {
	var out AnalysisConfigResult
	if e := checkMeta(in.WriteMeta, true, true); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "analysis.configs.publish", true, func(tx pgx.Tx) error {
		payload := []any{id, in}
		found, e := analysisReceiptTx(ctx, tx, a, scope, "analysis.configs.publish", in.WriteMeta, payload, &out)
		if e != nil {
			return e
		}
		if found {
			out.Replayed = true
			if out.Version != nil {
				if _, e = s.analysisBindingsTx(ctx, tx, a, scope, out.Version.Body); e != nil {
					return e
				}
			}
			return s.qualifyAnalysisConfigTx(ctx, tx, a, scope, &out.Config)
		}
		c, e := analysisConfigTx(ctx, tx, scope, id, true)
		if e != nil {
			return e
		}
		if c.Revision != in.Revision || c.Draft.Revision != in.DraftRevision {
			return conflict()
		}
		if c.Status != "active" {
			return problem("analysis_archived", "分析配置已归档。", 409)
		}
		p, body, e := s.compileAnalysisTx(ctx, tx, a, scope, c.Draft.Body, true)
		if e != nil {
			return e
		}
		validation := analysisValidation(c, p, body)
		if validation.ValidationHash != in.ValidationHash {
			return problem("preview_stale", "配置校验已变化，请重新校验。", 409)
		}
		var base AnalysisVersion
		if in.BaseVersionID != "" {
			base, e = analysisVersionTx(ctx, tx, scope, in.BaseVersionID)
			if e != nil {
				return e
			}
			if base.ConfigID != id {
				return notFound()
			}
		}
		version := uuid.NewString()
		number, _ := strconv.ParseInt(c.LatestVersion, 10, 64)
		_, e = tx.Exec(ctx, `INSERT INTO analysis_config_versions(id,tenant_id,config_id,version_no,name,body,compiled_plan,config_hash,plan_hash,engine_version,compiler_version,base_version_id,reset_baselines,created_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, version, scope.Tenant, id, number+1, c.Name, jsonBytes(body), p.Canonical, validation.ConfigHash, p.PlanHash, compute.EngineVersion, compute.CompilerVersion, nullable(in.BaseVersionID), in.ResetBaselines, a.ID, in.Reason)
		if e != nil {
			return e
		}
		for key, binding := range body.InputBindings {
			if _, e = tx.Exec(ctx, `INSERT INTO analysis_version_inputs(tenant_id,version_id,input_key,channel_id) VALUES($1,$2,$3,$4)`, scope.Tenant, version, key, binding.ChannelID); e != nil {
				return e
			}
		}
		refs, e := publicReferences(ctx, tx, p, true)
		if e != nil {
			return e
		}
		for reference := range refs {
			if _, e = tx.Exec(ctx, `INSERT INTO analysis_version_references(tenant_id,version_id,engineering_version_id) VALUES($1,$2,$3)`, scope.Tenant, version, reference); e != nil {
				return e
			}
		}
		if base.ID != "" && !in.ResetBaselines && fingerprint(base.Body.BaselineBindings) == fingerprint(body.BaselineBindings) {
			oldPlan, e := s.compile(base.Body.Rule, "analysis", base.Body.Parameters)
			if e != nil {
				return e
			}
			if analysisBaselineIdentity(oldPlan, analysisCalculationConfig(base)) == analysisBaselineIdentity(p, ChannelConfig{InputBindings: body.InputBindings, BaselineBindings: body.BaselineBindings}) {
				_, e = tx.Exec(ctx, `INSERT INTO derived_fixed_baselines(tenant_id,config_id,reference_key,snapshot_id,evidence,evidence_hash,created_by) SELECT tenant_id,$3,reference_key,snapshot_id,evidence,evidence_hash,$4 FROM derived_fixed_baselines WHERE tenant_id=$1 AND config_id=$2`, scope.Tenant, base.ID, version, a.ID)
				if e != nil {
					return e
				}
			}
		}
		_, e = tx.Exec(ctx, `UPDATE analysis_configs SET latest_version=$3,revision=revision+1,updated_at=clock_timestamp() WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id, number+1)
		if e != nil {
			return e
		}
		out.Config, e = analysisConfigTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		v, e := analysisVersionTx(ctx, tx, scope, version)
		if e != nil {
			return e
		}
		out.Version = &v
		return analysisCompleteTx(ctx, tx, a, scope, "analysis.configs.publish", "analysis_config", id, in.WriteMeta, payload, c, out, audit)
	})
	return out, e
}
func analysisCalculationConfig(v AnalysisVersion) ChannelConfig {
	return ChannelConfig{ID: v.ID, Rule: v.Body.Rule, InputBindings: v.Body.InputBindings, BaselineBindings: v.Body.BaselineBindings, Parameters: v.Body.Parameters, ConfigHash: v.ConfigHash}
}
