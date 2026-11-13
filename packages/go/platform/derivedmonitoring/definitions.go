package derivedmonitoring

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const usableVersion = `v.enabled AND NOT EXISTS(SELECT 1 FROM derived_metric_references r JOIN engineering_version_states es ON es.version_id=r.engineering_version_id WHERE r.tenant_id=v.tenant_id AND r.version_id=v.id AND NOT es.enabled)`
const definitionSelect = `SELECT d.id::text,d.tenant_id::text,d.code,d.name,d.description,d.status,d.revision::text,d.created_at,d.updated_at,COALESCE((SELECT max(v.version_no)::text FROM derived_metric_versions v WHERE v.tenant_id=d.tenant_id AND v.definition_id=d.id),'0') FROM derived_metric_definitions d`
const projectDefinitionSelect = `SELECT d.id::text,d.tenant_id::text,d.code,pv.name,'' AS description,d.status,'0' AS revision,d.created_at,pv.published_at,pv.version_no::text FROM derived_metric_definitions d JOIN LATERAL(SELECT v.name,v.version_no,v.published_at FROM derived_metric_versions v WHERE v.tenant_id=d.tenant_id AND v.definition_id=d.id AND ` + usableVersion + ` ORDER BY v.version_no DESC LIMIT 1) pv ON true`

func scanDefinition(row pgx.Row) (MetricDefinition, error) {
	var d MetricDefinition
	e := row.Scan(&d.ID, &d.TenantID, &d.Code, &d.Name, &d.Description, &d.Status, &d.Revision, &d.CreatedAt, &d.UpdatedAt, &d.LatestVersion)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	return d, e
}
func definitionTx(ctx context.Context, tx pgx.Tx, scope Scope, id string, deleted bool) (MetricDefinition, error) {
	if !access.ValidID(id) {
		return MetricDefinition{}, notFound()
	}
	where := " AND d.deleted_at IS NULL"
	if deleted {
		where = ""
	}
	selectSQL := definitionSelect
	if scope.Entry == "project" {
		selectSQL = projectDefinitionSelect
	}
	d, e := scanDefinition(tx.QueryRow(ctx, selectSQL+` WHERE d.tenant_id=$1 AND d.id=$2`+where, scope.Tenant, id))
	if e != nil {
		return d, e
	}
	if scope.Entry == "tenant" {
		var draft MetricDraft
		e = tx.QueryRow(ctx, `SELECT revision::text,COALESCE(base_version_id::text,''),rule,updated_at FROM derived_metric_drafts WHERE tenant_id=$1 AND definition_id=$2`, scope.Tenant, id).Scan(&draft.Revision, &draft.BaseVersionID, &draft.Rule, &draft.UpdatedAt)
		if e == nil {
			d.Draft = &draft
		} else if e == pgx.ErrNoRows {
			e = nil
		}
	}
	return d, e
}

const versionSelect = `SELECT v.id::text,v.tenant_id::text,v.definition_id::text,v.version_no::text,v.name,v.rule,v.schema_version,v.compiler_version,v.engine_version,v.content_hash,v.output_contract_hash,v.enabled,(` + usableVersion + `),v.state_revision::text,v.published_at FROM derived_metric_versions v`

func scanVersion(row pgx.Row) (MetricVersion, error) {
	var v MetricVersion
	e := row.Scan(&v.ID, &v.TenantID, &v.DefinitionID, &v.Number, &v.Name, &v.Rule, &v.SchemaVersion, &v.CompilerVersion, &v.EngineVersion, &v.ContentHash, &v.OutputContractHash, &v.Enabled, &v.Usable, &v.StateRevision, &v.PublishedAt)
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	return v, e
}
func versionTx(ctx context.Context, tx pgx.Tx, scope Scope, id string, newUse bool) (MetricVersion, error) {
	if !access.ValidID(id) {
		return MetricVersion{}, notFound()
	}
	where := ""
	if scope.Entry == "project" {
		where = " AND (" + usableVersion + ")"
	}
	v, e := scanVersion(tx.QueryRow(ctx, versionSelect+` WHERE v.tenant_id=$1 AND v.id=$2`+where, scope.Tenant, id))
	if e == nil && newUse && !v.Usable {
		e = problem("dependency_unavailable", "版本已停用或固定依赖不可新采用。", 409)
	}
	return v, e
}
func (s *Service) Metric(ctx context.Context, a access.Actor, scope Scope, id string) (MetricDefinition, error) {
	var d MetricDefinition
	e := s.transact(ctx, a, scope, "metrics.detail", false, func(tx pgx.Tx) error { var e error; d, e = definitionTx(ctx, tx, scope, id, false); return e })
	return d, e
}
func (s *Service) MetricVersion(ctx context.Context, a access.Actor, scope Scope, id string) (MetricVersion, error) {
	var v MetricVersion
	e := s.transact(ctx, a, scope, "versions.detail", false, func(tx pgx.Tx) error { var e error; v, e = versionTx(ctx, tx, scope, id, false); return e })
	return v, e
}

type listCursor struct {
	At     *time.Time `json:"at"`
	ID     string     `json:"id"`
	Filter string     `json:"filter"`
}

func decodeCursor(text, filter string) (listCursor, error) {
	var c listCursor
	if text == "" {
		return c, nil
	}
	if len(text) > 1024 {
		return c, invalid("/cursor", "分页位置无效。")
	}
	b, e := base64.RawURLEncoding.DecodeString(text)
	if e != nil || json.Unmarshal(b, &c) != nil || c.At == nil || !access.ValidID(c.ID) || c.Filter != filter {
		return listCursor{}, invalid("/cursor", "分页位置不适用于当前筛选或空间。")
	}
	return c, nil
}
func encodeCursor(at time.Time, id, filter string) string {
	return base64.RawURLEncoding.EncodeToString(jsonBytes(listCursor{&at, id, filter}))
}
func filterKey(a access.Actor, scope Scope, filter MetricFilter) string {
	filter.Cursor = ""
	return fingerprint([]any{a.ID, scope, filter})
}
func normalizeFilter(f MetricFilter) (MetricFilter, error) {
	f.Query = strings.TrimSpace(f.Query)
	if len(f.Query) > 400 || f.Limit < 0 || f.Limit > 100 {
		return f, invalid("/filter", "筛选值过长或分页大小无效。")
	}
	if f.Limit == 0 {
		f.Limit = 30
	}
	if f.State != "" && f.State != "published" && f.State != "draft" {
		return f, invalid("/state", "状态筛选无效。")
	}
	return f, nil
}
func (s *Service) Metrics(ctx context.Context, a access.Actor, scope Scope, filter MetricFilter) (MetricPage, error) {
	out := MetricPage{Items: []MetricDefinition{}}
	filter, e := normalizeFilter(filter)
	if e != nil {
		return out, e
	}
	key := filterKey(a, scope, filter)
	cursor, e := decodeCursor(filter.Cursor, key)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "metrics.list", false, func(tx pgx.Tx) error {
		selectSQL, nameColumn := definitionSelect, "d.name"
		if scope.Entry == "project" {
			selectSQL, nameColumn = projectDefinitionSelect, "pv.name"
		}
		where := ` WHERE d.tenant_id=$1 AND d.deleted_at IS NULL AND ($2='' OR d.code ILIKE '%'||$2||'%' OR ` + nameColumn + ` ILIKE '%'||$2||'%') AND ($3::timestamptz IS NULL OR (d.created_at,d.id)<($3,$4::uuid))`
		if filter.State == "published" {
			where += ` AND EXISTS(SELECT 1 FROM derived_metric_versions v WHERE v.tenant_id=d.tenant_id AND v.definition_id=d.id)`
		}
		if filter.State == "draft" {
			if scope.Entry == "project" {
				return invalid("/state", "项目入口不显示租户草稿。")
			}
			where += ` AND EXISTS(SELECT 1 FROM derived_metric_drafts x WHERE x.tenant_id=d.tenant_id AND x.definition_id=d.id)`
		}
		rows, e := tx.Query(ctx, selectSQL+where+` ORDER BY d.created_at DESC,d.id DESC LIMIT $5`, scope.Tenant, filter.Query, cursor.At, nullable(cursor.ID), filter.Limit+1)
		if e != nil {
			return e
		}
		for rows.Next() {
			d, e := scanDefinition(rows)
			if e != nil {
				rows.Close()
				return e
			}
			out.Items = append(out.Items, d)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(out.Items) > filter.Limit {
			out.Items = out.Items[:filter.Limit]
			last := out.Items[len(out.Items)-1]
			out.NextCursor = encodeCursor(last.CreatedAt, last.ID, key)
		}
		return nil
	})
	return out, e
}
func (s *Service) MetricVersions(ctx context.Context, a access.Actor, scope Scope, id string, filter MetricFilter) (VersionPage, error) {
	out := VersionPage{Items: []MetricVersion{}}
	filter, e := normalizeFilter(filter)
	if e != nil {
		return out, e
	}
	key := fingerprint([]any{filterKey(a, scope, filter), id})
	cursor, e := decodeCursor(filter.Cursor, key)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "metrics.versions", false, func(tx pgx.Tx) error {
		if _, e := definitionTx(ctx, tx, scope, id, false); e != nil {
			return e
		}
		where := ""
		if scope.Entry == "project" {
			where = " AND (" + usableVersion + ")"
		}
		rows, e := tx.Query(ctx, versionSelect+` WHERE v.tenant_id=$1 AND v.definition_id=$2 AND ($3::timestamptz IS NULL OR (v.published_at,v.id)<($3,$4::uuid))`+where+` ORDER BY v.published_at DESC,v.id DESC LIMIT $5`, scope.Tenant, id, cursor.At, nullable(cursor.ID), filter.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := scanVersion(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if e := rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > filter.Limit {
			out.Items = out.Items[:filter.Limit]
			last := out.Items[len(out.Items)-1]
			out.NextCursor = encodeCursor(last.PublishedAt, last.ID, key)
		}
		return nil
	})
	return out, e
}
func (s *Service) Templates(ctx context.Context, a access.Actor, scope Scope) (TemplatePage, error) {
	out := TemplatePage{Items: []Template{}}
	e := s.transact(ctx, a, scope, "templates.list", false, func(pgx.Tx) error {
		for _, t := range compute.Templates() {
			out.Items = append(out.Items, Template{t.Key, t.Version, t.Name, t.Description, strconv.Itoa(t.InputCount)})
		}
		return nil
	})
	return out, e
}
func (s *Service) ExpandTemplate(ctx context.Context, a access.Actor, scope Scope, key string, in ExpandTemplateInput) (Rule, error) {
	var out Rule
	e := s.transact(ctx, a, scope, "templates.expand", false, func(tx pgx.Tx) error {
		rule, e := compute.ExpandTemplate(key, in.Inputs)
		if e != nil {
			return invalid("/template", e.Error())
		}
		p, e := s.compile(jsonBytes(rule), "channel", nil)
		if e != nil {
			return e
		}
		if _, e := publicReferences(ctx, tx, p, true); e != nil {
			return e
		}
		out = rule
		return nil
	})
	return out, e
}
func (s *Service) CreateMetric(ctx context.Context, a access.Actor, scope Scope, in CreateMetricInput, audit access.Meta) (MetricResult, error) {
	var out MetricResult
	if e := checkMeta(in.WriteMeta, false, false); e != nil {
		return out, e
	}
	if !codePattern.MatchString(in.Code) {
		return out, invalid("/code", "编号须为 1–64 位字母、数字、点、短横线或下划线，并以字母或数字开头。")
	}
	if e := checkName(in.Name, in.Description); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "metrics.create", true, func(tx pgx.Tx) error {
		var found bool
		var e error
		out, found, e = receipt(ctx, tx, a, scope, "metrics.create", in.WriteMeta, in)
		if e != nil || found {
			return e
		}
		p, e := s.compile(in.Rule, "channel", nil)
		if e != nil {
			return e
		}
		if _, e := publicReferences(ctx, tx, p, true); e != nil {
			return e
		}
		id := uuid.NewString()
		if _, e = tx.Exec(ctx, `INSERT INTO derived_metric_definitions(id,tenant_id,code,name,description,created_by,updated_by) VALUES($1,$2,$3,$4,$5,$6,$6)`, id, scope.Tenant, in.Code, strings.TrimSpace(in.Name), in.Description, a.ID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO derived_metric_drafts(tenant_id,definition_id,revision,rule) VALUES($1,$2,1,$3)`, scope.Tenant, id, in.Rule); e != nil {
			return e
		}
		out.Definition, e = definitionTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		return complete(ctx, tx, a, scope, "metrics.create", in.WriteMeta, in, out, nil, audit)
	})
	return out, e
}
func (s *Service) SaveMetricDraft(ctx context.Context, a access.Actor, scope Scope, id string, in SaveMetricInput, audit access.Meta) (MetricResult, error) {
	var out MetricResult
	if e := checkMeta(in.WriteMeta, true, false); e != nil {
		return out, e
	}
	if e := checkName(in.Name, in.Description); e != nil {
		return out, e
	}
	payload := []any{id, in}
	e := s.transact(ctx, a, scope, "metrics.save", true, func(tx pgx.Tx) error {
		var found bool
		var e error
		out, found, e = receipt(ctx, tx, a, scope, "metrics.save", in.WriteMeta, payload)
		if e != nil || found {
			return e
		}
		before, e := definitionTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		if before.Revision != in.Revision {
			return conflict()
		}
		if in.BaseVersionID != "" {
			v, e := versionTx(ctx, tx, scope, in.BaseVersionID, false)
			if e != nil {
				return e
			}
			if v.DefinitionID != id {
				return notFound()
			}
		}
		p, e := s.compile(in.Rule, "channel", nil)
		if e != nil {
			return e
		}
		if _, e := publicReferences(ctx, tx, p, true); e != nil {
			return e
		}
		var revision string
		e = tx.QueryRow(ctx, `UPDATE derived_metric_definitions SET name=$3,description=$4,revision=revision+1,updated_at=clock_timestamp(),updated_by=$5 WHERE tenant_id=$1 AND id=$2 RETURNING revision::text`, scope.Tenant, id, strings.TrimSpace(in.Name), in.Description, a.ID).Scan(&revision)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO derived_metric_drafts(tenant_id,definition_id,base_version_id,revision,rule) VALUES($1,$2,$3,$4::bigint,$5) ON CONFLICT(definition_id) DO UPDATE SET base_version_id=EXCLUDED.base_version_id,revision=EXCLUDED.revision,rule=EXCLUDED.rule,updated_at=clock_timestamp()`, scope.Tenant, id, nullable(in.BaseVersionID), revision, in.Rule); e != nil {
			return e
		}
		out.Definition, e = definitionTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		return complete(ctx, tx, a, scope, "metrics.save", in.WriteMeta, payload, out, before, audit)
	})
	return out, e
}
func (s *Service) validationTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, d MetricDefinition) (RuleValidation, *compute.Plan, error) {
	var out RuleValidation
	if d.Draft == nil {
		return out, nil, problem("draft_required", "请先保存草稿。", 409)
	}
	p, e := s.compile(d.Draft.Rule, "channel", nil)
	if e != nil {
		return out, nil, e
	}
	refs, e := publicReferences(ctx, tx, p, true)
	if e != nil {
		return out, nil, e
	}
	vector := map[string]int{}
	for id, version := range refs {
		vector[id] = version.StateRevision
	}
	out = RuleValidation{Valid: true, Issues: []Issue{}, CompiledPlanHash: p.PlanHash, OutputContractHash: p.OutputContractHash, NormalizedRule: p.Rule, InputCount: strconv.Itoa(len(p.Rule.Inputs)), NodeCount: strconv.Itoa(len(p.Rule.Nodes))}
	out.ValidationHash = fingerprint([]any{"derived-validation/1", a.ID, scope, d.ID, d.Revision, d.Draft.Revision, p.PlanHash, vector})
	return out, p, nil
}
func (s *Service) ValidateMetric(ctx context.Context, a access.Actor, scope Scope, id string, in ValidateInput) (RuleValidation, error) {
	var out RuleValidation
	e := s.transact(ctx, a, scope, "metrics.validate", false, func(tx pgx.Tx) error {
		d, e := definitionTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		if d.Draft == nil || d.Draft.Revision != in.DraftRevision {
			return conflict()
		}
		out, _, e = s.validationTx(ctx, tx, a, scope, d)
		return e
	})
	return out, e
}
func (s *Service) PublishMetric(ctx context.Context, a access.Actor, scope Scope, id string, in PublishMetricInput, audit access.Meta) (MetricResult, error) {
	var out MetricResult
	if e := checkMeta(in.WriteMeta, true, false); e != nil {
		return out, e
	}
	payload := []any{id, in}
	e := s.transact(ctx, a, scope, "metrics.publish", true, func(tx pgx.Tx) error {
		var found bool
		var e error
		out, found, e = receipt(ctx, tx, a, scope, "metrics.publish", in.WriteMeta, payload)
		if e != nil || found {
			return e
		}
		before, e := definitionTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		if before.Revision != in.Revision || before.Draft == nil || before.Draft.Revision != in.DraftRevision {
			return conflict()
		}
		validation, p, e := s.validationTx(ctx, tx, a, scope, before)
		if e != nil {
			return e
		}
		if validation.ValidationHash != in.ValidationHash {
			return problem("preview_changed", "发布依据已变化，请重新校验。", 409)
		}
		versionID := uuid.NewString()
		_, e = tx.Exec(ctx, `INSERT INTO derived_metric_versions(id,tenant_id,definition_id,version_no,name,rule,compiled_plan,schema_version,compiler_version,engine_version,content_hash,output_contract_hash,published_by) SELECT $1,$2,$3,COALESCE(max(version_no),0)+1,$4,$5,$6,$7,$8,$9,$10,$11,$12 FROM derived_metric_versions WHERE tenant_id=$2 AND definition_id=$3`, versionID, scope.Tenant, id, before.Name, before.Draft.Rule, p.Canonical, compute.SchemaVersion, compute.CompilerVersion, compute.EngineVersion, p.PlanHash, p.OutputContractHash, a.ID)
		if e != nil {
			return e
		}
		refs, e := publicReferences(ctx, tx, p, true)
		if e != nil {
			return e
		}
		ids := make([]string, 0, len(refs))
		for id := range refs {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, ref := range ids {
			if _, e = tx.Exec(ctx, `INSERT INTO derived_metric_references(tenant_id,version_id,engineering_version_id) VALUES($1,$2,$3)`, scope.Tenant, versionID, ref); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `DELETE FROM derived_metric_drafts WHERE tenant_id=$1 AND definition_id=$2`, scope.Tenant, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE derived_metric_definitions SET revision=revision+1,updated_at=clock_timestamp(),updated_by=$3 WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id, a.ID); e != nil {
			return e
		}
		v, e := versionTx(ctx, tx, scope, versionID, false)
		if e != nil {
			return e
		}
		out.Version = &v
		out.Definition, e = definitionTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		return complete(ctx, tx, a, scope, "metrics.publish", in.WriteMeta, payload, out, before, audit)
	})
	return out, e
}
func (s *Service) SetMetricVersionState(ctx context.Context, a access.Actor, scope Scope, id string, in VersionStateInput, audit access.Meta) (MetricResult, error) {
	var out MetricResult
	if e := checkMeta(in.WriteMeta, true, true); e != nil {
		return out, e
	}
	payload := []any{id, in}
	e := s.transact(ctx, a, scope, "versions.state", true, func(tx pgx.Tx) error {
		var found bool
		var e error
		out, found, e = receipt(ctx, tx, a, scope, "versions.state", in.WriteMeta, payload)
		if e != nil || found {
			return e
		}
		before, e := versionTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		if before.StateRevision != in.Revision {
			return conflict()
		}
		if _, e = tx.Exec(ctx, `UPDATE derived_metric_versions SET enabled=$3,state_revision=state_revision+1 WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id, in.Enabled); e != nil {
			return e
		}
		v, e := versionTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		out.Version = &v
		out.Definition, e = definitionTx(ctx, tx, scope, v.DefinitionID, false)
		if e != nil {
			return e
		}
		return complete(ctx, tx, a, scope, "versions.state", in.WriteMeta, payload, out, before, audit)
	})
	return out, e
}
func (s *Service) CopyMetric(ctx context.Context, a access.Actor, scope Scope, id string, in CopyMetricInput, audit access.Meta) (MetricResult, error) {
	var out MetricResult
	if e := checkMeta(in.WriteMeta, false, false); e != nil {
		return out, e
	}
	if !codePattern.MatchString(in.Code) {
		return out, invalid("/code", "编号格式不正确。")
	}
	if e := checkName(in.Name, ""); e != nil {
		return out, e
	}
	payload := []any{id, in}
	e := s.transact(ctx, a, scope, "metrics.copy", true, func(tx pgx.Tx) error {
		var found bool
		var e error
		out, found, e = receipt(ctx, tx, a, scope, "metrics.copy", in.WriteMeta, payload)
		if e != nil || found {
			return e
		}
		source, e := versionTx(ctx, tx, scope, in.SourceVersionID, false)
		if e != nil {
			return e
		}
		if source.DefinitionID != id {
			return notFound()
		}
		p, e := s.compile(source.Rule, "channel", nil)
		if e != nil {
			return e
		}
		if _, e := publicReferences(ctx, tx, p, true); e != nil {
			return e
		}
		newID := uuid.NewString()
		if _, e = tx.Exec(ctx, `INSERT INTO derived_metric_definitions(id,tenant_id,code,name,created_by,updated_by) VALUES($1,$2,$3,$4,$5,$5)`, newID, scope.Tenant, in.Code, strings.TrimSpace(in.Name), a.ID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO derived_metric_drafts(tenant_id,definition_id,revision,rule) VALUES($1,$2,1,$3)`, scope.Tenant, newID, source.Rule); e != nil {
			return e
		}
		out.Definition, e = definitionTx(ctx, tx, scope, newID, false)
		if e != nil {
			return e
		}
		return complete(ctx, tx, a, scope, "metrics.copy", in.WriteMeta, payload, out, map[string]string{"source_version_id": source.ID}, audit)
	})
	return out, e
}
func (s *Service) DeleteMetricDraft(ctx context.Context, a access.Actor, scope Scope, id string, in WriteMeta, audit access.Meta) (MetricResult, error) {
	var out MetricResult
	if e := checkMeta(in, true, true); e != nil {
		return out, e
	}
	payload := []any{id, in}
	e := s.transact(ctx, a, scope, "metrics.delete", true, func(tx pgx.Tx) error {
		var found bool
		var e error
		out, found, e = receipt(ctx, tx, a, scope, "metrics.delete", in, payload)
		if e != nil || found {
			return e
		}
		before, e := definitionTx(ctx, tx, scope, id, false)
		if e != nil {
			return e
		}
		if before.Revision != in.Revision {
			return conflict()
		}
		if before.LatestVersion != "0" {
			return problem("definition_in_use", "已发布定义永久保留，可停用版本。", 409)
		}
		if _, e = tx.Exec(ctx, `DELETE FROM derived_metric_drafts WHERE tenant_id=$1 AND definition_id=$2`, scope.Tenant, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE derived_metric_definitions SET status='deleted',deleted_at=clock_timestamp(),revision=revision+1,updated_at=clock_timestamp(),updated_by=$3 WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id, a.ID); e != nil {
			return e
		}
		out.Definition, e = definitionTx(ctx, tx, scope, id, true)
		if e != nil {
			return e
		}
		return complete(ctx, tx, a, scope, "metrics.delete", in, payload, out, before, audit)
	})
	return out, e
}
