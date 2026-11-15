package display

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
)

func readWindow(from, to *string) (monitoring.Window, error) {
	w := monitoring.Window{}
	for _, p := range []struct {
		input  *string
		output **int64
	}{{from, &w.From}, {to, &w.To}} {
		if p.input != nil {
			v, e := time.Parse(time.RFC3339Nano, *p.input)
			if e != nil || v.Year() < 1900 || v.Year() > 2261 {
				return w, invalid("/range", "需要 1900–2261 年内的 RFC3339Nano 时刻。")
			}
			n := v.UnixNano()
			*p.output = &n
		}
	}
	if w.From != nil && w.To != nil && *w.From >= *w.To {
		return w, invalid("/range", "结束时刻必须大于开始时刻。")
	}
	return w, nil
}
func normalizeIDs(ids []string) error {
	if len(ids) > 500 {
		return invalid("/ids", "最多选择 500 个对象。")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !access.ValidID(id) || seen[id] {
			return invalid("/ids", "标识无效或重复。")
		}
		seen[id] = true
	}
	return nil
}
func dynamicIDsTx(ctx context.Context, tx pgx.Tx, scope Scope, d *Dynamic) ([]string, error) {
	if d == nil {
		return []string{}, nil
	}
	if len(d.FacilityIDs) == 0 || len(d.FacilityIDs) > 50 {
		return nil, invalid("/dynamic/facility_ids", "动态绑定需要明确设施范围（最多 50 个）。")
	}
	if e := normalizeIDs(d.FacilityIDs); e != nil {
		return nil, e
	}
	if e := normalizeIDs(d.ComponentIDs); e != nil {
		return nil, e
	}
	if d.MetricID != "" && !access.ValidID(d.MetricID) {
		return nil, invalid("/dynamic/metric_version_id", "指标版本无效。")
	}
	if !slices.Contains([]string{"", "numeric", "integer", "decimal", "enum", "boolean"}, d.ValueKind) || !slices.Contains([]string{"", "direct", "derived"}, d.SourceKind) {
		return nil, invalid("/dynamic", "值类型或来源筛选无效。")
	}
	rows, e := tx.Query(ctx, `WITH RECURSIVE components AS(SELECT id FROM engineering_components WHERE tenant_id=$1 AND id=ANY($4::uuid[]) AND deleted_at IS NULL UNION SELECT c.id FROM engineering_components c JOIN components p ON p.id=c.parent_component_id WHERE c.tenant_id=$1 AND c.deleted_at IS NULL) SELECT c.id::text FROM engineering_channels c LEFT JOIN engineering_points p ON p.tenant_id=c.tenant_id AND p.id=c.point_id LEFT JOIN engineering_versions m ON m.id=c.metric_version_id WHERE c.tenant_id=$1 AND c.facility_id=ANY($3::uuid[]) AND c.deleted_at IS NULL AND (COALESCE(cardinality($4::uuid[]),0)=0 OR COALESCE(c.component_id,p.component_id) IN (SELECT id FROM components)) AND ($5='' OR c.metric_version_id::text=$5 OR c.derived_metric_version_id::text=$5) AND ($6='' OR c.source_kind=$6) AND ($7='' OR CASE WHEN c.source_kind='derived' THEN 'decimal' ELSE m.content->'metric'->>'shape' END=$7 OR $7='numeric' AND (c.source_kind='derived' OR m.content->'metric'->>'shape' IN ('integer','decimal'))) AND EXISTS(SELECT 1 FROM monitoring_data_grants d JOIN project_resource_grants g ON g.id=d.resource_grant_id JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE d.tenant_id=c.tenant_id AND d.project_id=$2 AND d.channel_id=c.id AND d.revoked_at IS NULL AND 'read'=ANY(d.capabilities) AND g.ended_at IS NULL) ORDER BY c.id LIMIT 501`, scope.Tenant, scope.Project, d.FacilityIDs, d.ComponentIDs, d.MetricID, d.SourceKind, d.ValueKind)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	if len(out) > 500 {
		return nil, problem("budget_exceeded", "动态范围超过 500 通道，请缩小范围。", 429)
	}
	if !d.IncludeFuture && d.BaselineIDs != nil {
		out = slices.DeleteFunc(out, func(id string) bool { return !slices.Contains(d.BaselineIDs, id) })
	}
	return out, nil
}
func freezeDynamic(ctx context.Context, tx pgx.Tx, scope Scope, d *Dynamic) error {
	if d == nil {
		return nil
	}
	d.BaselineIDs = nil
	ids, e := dynamicIDsTx(ctx, tx, scope, d)
	if e != nil {
		return e
	}
	if !d.IncludeFuture {
		d.BaselineIDs = ids
	}
	return nil
}
func resolveBindingTx(ctx context.Context, tx pgx.Tx, scope Scope, b Binding) ([]string, error) {
	if b.Dynamic != nil {
		if b.Kind != "channels" || len(b.IDs) > 0 {
			return nil, invalid("/binding", "动态范围只用于通道且不能同时指定 IDs。")
		}
		return dynamicIDsTx(ctx, tx, scope, b.Dynamic)
	}
	if e := normalizeIDs(b.IDs); e != nil {
		return nil, e
	}
	return slices.Clone(b.IDs), nil
}
func grantIDsTx(ctx context.Context, tx pgx.Tx, scope Scope, g Grant, kind string) ([]string, error) {
	var ids []string
	switch kind {
	case "channels":
		ids = slices.Clone(g.Scope.Channels)
		dynamic, e := dynamicIDsTx(ctx, tx, scope, g.Scope.Dynamic)
		if e != nil {
			return nil, e
		}
		ids = append(ids, dynamic...)
	case "resources":
		ids = slices.Clone(g.Scope.Resources)
	case "spatial":
		ids = slices.Clone(g.Scope.Spatial)
	case "artifacts":
		ids = slices.Clone(g.Scope.Artifacts)
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

func effectiveBindingIDsTx(ctx context.Context, tx pgx.Tx, scope Scope, b Binding, g *Grant) ([]string, error) {
	ids, e := resolveBindingTx(ctx, tx, scope, b)
	if e != nil || g == nil || b.Dynamic == nil {
		return ids, e
	}
	allowed, e := grantIDsTx(ctx, tx, scope, *g, b.Kind)
	if e != nil {
		return nil, e
	}
	// Future objects enter only if both the configured filter and current grant
	// include them. Narrow grants never turn into an implicit future delegation.
	return slices.DeleteFunc(ids, func(id string) bool { return !slices.Contains(allowed, id) }), nil
}
func refsTx(ctx context.Context, tx pgx.Tx, scope Scope, kind string, ids []string) error {
	if e := normalizeIDs(ids); e != nil {
		return e
	}
	if len(ids) == 0 {
		return nil
	}
	var query string
	switch kind {
	case "channels":
		query = `SELECT count(DISTINCT c.id) FROM engineering_channels c JOIN monitoring_data_grants d ON d.tenant_id=c.tenant_id AND d.channel_id=c.id JOIN project_resource_grants g ON g.id=d.resource_grant_id JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE c.tenant_id=$1 AND d.project_id=$2 AND c.id=ANY($3::uuid[]) AND c.deleted_at IS NULL AND d.revoked_at IS NULL AND g.ended_at IS NULL AND 'read'=ANY(d.capabilities)`
	case "resources":
		query = `SELECT count(DISTINCT n.id) FROM engineering_resource_nodes n JOIN LATERAL project_resource_visibility(n.tenant_id,$2,n.facility_id) v ON v.kind=n.kind AND v.id=n.id WHERE n.tenant_id=$1 AND n.id=ANY($3::uuid[]) AND n.deleted_at IS NULL`
	case "spatial":
		query = `SELECT count(DISTINCT v.id) FROM spatial_asset_versions v JOIN spatial_assets a ON a.tenant_id=v.tenant_id AND a.id=v.asset_id JOIN project_spatial_grants g ON g.tenant_id=v.tenant_id AND g.version_id=v.id LEFT JOIN project_facility_links l ON l.id=g.facility_link_id WHERE v.tenant_id=$1 AND g.project_id=$2 AND v.id=ANY($3::uuid[]) AND v.state='published' AND v.content_available AND g.ended_at IS NULL AND (a.facility_id IS NULL OR l.status='active')`
	case "artifacts":
		query = `SELECT count(*) FROM analysis_artifact_versions v JOIN analysis_artifacts a ON a.tenant_id=v.tenant_id AND a.id=v.artifact_id WHERE v.tenant_id=$1 AND a.project_id=$2 AND v.id=ANY($3::uuid[])`
	default:
		return invalid("/kind", "不支持的对象类型。")
	}
	var count int
	if e := tx.QueryRow(ctx, query, scope.Tenant, scope.Project, ids).Scan(&count); e != nil {
		return e
	}
	if count != len(ids) {
		return hidden()
	}
	return nil
}
func semanticsTx(ctx context.Context, tx pgx.Tx, scope Scope, schema BindingSchema, ids []string) error {
	if len(ids) == 0 || schema.Kind != "channels" {
		return nil
	}
	rows, e := tx.Query(ctx, `SELECT c.id::text,c.source_kind,CASE WHEN c.source_kind='derived' THEN 'decimal' ELSE m.content->'metric'->>'shape' END,COALESCE(c.metric_version_id::text,c.derived_metric_version_id::text,''),COALESCE(u.content->'unit'->>'category',''),c.observation_direction,c.reference_basis FROM engineering_channels c LEFT JOIN engineering_versions m ON m.id=c.metric_version_id LEFT JOIN engineering_versions u ON u.id=COALESCE(c.unit_version_id,NULLIF(c.quantity_spec->>'base_unit_version_id','')::uuid) WHERE c.tenant_id=$1 AND c.id=ANY($2::uuid[]) AND c.deleted_at IS NULL`, scope.Tenant, ids)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id, source, kind, metric, dimension, direction, basis string
		if e = rows.Scan(&id, &source, &kind, &metric, &dimension, &direction, &basis); e != nil {
			return e
		}
		matches := func(want, actual string) bool { return want == "" || want == actual }
		numeric := schema.ValueKind == "numeric" && slices.Contains([]string{"integer", "decimal"}, kind)
		if !(numeric || matches(schema.ValueKind, kind)) || !matches(schema.SourceKind, source) || !matches(schema.MetricID, metric) || !matches(schema.UnitDimension, dimension) || !matches(schema.Direction, direction) || !matches(schema.Basis, basis) {
			return problem("binding_invalid", "通道的指标、值类型、单位维度、方向或基准与绑定声明不兼容。", 422)
		}
	}
	return rows.Err()
}
func (s *Service) validateBindingsTx(ctx context.Context, tx pgx.Tx, scope Scope, m Manifest, bindings map[string]Binding, g *Grant, publish bool) error {
	for name, b := range bindings {
		schema, ok := m.Bindings[name]
		if !ok || schema.Kind != b.Kind {
			return invalid("/bindings/"+name, "绑定名称或对象类型不匹配。")
		}
		if !publish && b.Dynamic != nil {
			if e := freezeDynamic(ctx, tx, scope, b.Dynamic); e != nil {
				return e
			}
			bindings[name] = b
		}
		ids, e := effectiveBindingIDsTx(ctx, tx, scope, b, g)
		if e != nil {
			return e
		}
		if len(ids) < schema.Min || schema.Max > 0 && len(ids) > schema.Max || publish && schema.Required && len(ids) == 0 {
			return invalid("/bindings/"+name, "绑定数量不满足声明。")
		}
		if e = refsTx(ctx, tx, scope, b.Kind, ids); e != nil {
			return e
		}
		if e = semanticsTx(ctx, tx, scope, schema, ids); e != nil {
			return e
		}
		w, e := readWindow(b.From, b.To)
		if e != nil {
			return e
		}
		if b.Kind == "channels" && len(ids) > 0 && (b.From != nil || b.To != nil) {
			var covered int
			e = tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT d.channel_id FROM monitoring_data_grants d JOIN project_resource_grants g ON g.id=d.resource_grant_id JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE d.tenant_id=$1 AND d.project_id=$2 AND d.channel_id=ANY($3::uuid[]) AND d.revoked_at IS NULL AND g.ended_at IS NULL AND 'read'=ANY(d.capabilities) GROUP BY d.channel_id HAVING range_agg(d.permitted) @> int8range($4,$5,'[)')) qualified`, scope.Tenant, scope.Project, ids, w.From, w.To).Scan(&covered)
			if e != nil {
				return e
			}
			if covered != len(ids) {
				return invalid("/bindings/"+name, "所需时段未完整获得当前项目数据授权。")
			}
		}
		if g != nil {
			allowed, e := grantIDsTx(ctx, tx, scope, *g, b.Kind)
			if e != nil {
				return e
			}
			for _, id := range ids {
				if !slices.Contains(allowed, id) {
					return problem("binding_invalid", "绑定超出当前大屏读取授权。", 422)
				}
			}
			limit, e := readWindow(g.Scope.From, g.Scope.To)
			if e != nil {
				return e
			}
			if (b.From != nil || b.To != nil) && !limit.Covers(w) {
				return problem("binding_invalid", "绑定时段超出读取授权。", 422)
			}
		}
	}
	if publish {
		for name, b := range m.Bindings {
			if b.Required {
				if _, ok := bindings[name]; !ok {
					return invalid("/bindings/"+name, "缺少必需绑定。")
				}
			}
		}
	}
	return nil
}
func (s *Service) saveGrantTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, v Instance, auth access.ProjectAuthorization, g Grant) error {
	old, e := grantTx(ctx, tx, v.ID)
	if e != nil && e != pgx.ErrNoRows {
		return e
	}
	if (auth.ProjectStatus == "archived" || v.State == "archived") && !g.Revoked && !grantNarrows(old, g) {
		return access.ErrDenied
	}
	if !g.Revoked && (g.ValidUntil.Before(time.Now()) || g.ValidUntil.After(time.Now().Add(366*24*time.Hour))) {
		return invalid("/valid_until", "委托需在一年内到期。")
	}
	if g.Revoked {
		if e == pgx.ErrNoRows {
			return hidden()
		}
		_, e = tx.Exec(ctx, `UPDATE display_grants SET revoked=true,revision=revision+1 WHERE instance_id=$1`, v.ID)
		if e != nil {
			return e
		}
		return s.revokeTx(ctx, tx, scope, v.ID, "grant_revoked")
	}
	for _, cap := range g.Capabilities {
		if _, ok := capabilities[cap]; !ok {
			return invalid("/capabilities", "能力未注册。")
		}
	}
	for _, p := range s.permissionsFor(g.Capabilities) {
		if !slices.Contains(auth.Permissions, p) {
			return access.ErrDenied
		}
	}
	// A grant contains only public projections, not arbitrary database columns.
	for _, f := range g.Fields {
		if !slices.Contains([]string{"metadata", "values", "quality", "source", "properties"}, f) {
			return invalid("/fields", "字段集合不支持。")
		}
	}
	if len(g.Fields) == 0 {
		g.Fields = []string{"metadata", "values", "quality"}
	}
	if e = freezeDynamic(ctx, tx, scope, g.Scope.Dynamic); e != nil {
		return e
	}
	if _, e = readWindow(g.Scope.From, g.Scope.To); e != nil {
		return e
	}
	for _, kind := range []string{"channels", "resources", "spatial", "artifacts"} {
		ids, e := grantIDsTx(ctx, tx, scope, g, kind)
		if e != nil {
			return e
		}
		if e = refsTx(ctx, tx, scope, kind, ids); e != nil {
			return e
		}
	}
	_, e = tx.Exec(ctx, `INSERT INTO display_grants(tenant_id,project_id,instance_id,capabilities,scope,fields,valid_until,issued_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(instance_id) DO UPDATE SET capabilities=EXCLUDED.capabilities,scope=EXCLUDED.scope,fields=EXCLUDED.fields,valid_until=EXCLUDED.valid_until,issued_by=EXCLUDED.issued_by,revoked=false,revision=display_grants.revision+1`, scope.Tenant, scope.Project, v.ID, g.Capabilities, jsonBytes(g.Scope), g.Fields, g.ValidUntil, a.ID)
	if e != nil {
		return e
	}
	if old.Revision != "" {
		return s.revokeTx(ctx, tx, scope, v.ID, "grant_changed")
	}
	return nil
}
func scopeFingerprint(g Grant) string {
	return hash([]any{g.Revision, g.Capabilities, g.Scope, g.Fields})
}
func bindingKind(method string) string {
	switch strings.Split(method, ".")[0] {
	case "monitoring", "channels", "sources":
		return "channels"
	case "spatial":
		return "spatial"
	case "artifacts":
		return "artifacts"
	default:
		return "resources"
	}
}

func grantNarrows(old, next Grant) bool {
	if old.Revision == "" || old.Revoked || next.ValidUntil.After(old.ValidUntil) {
		return false
	}
	for _, sets := range [][2][]string{{old.Capabilities, next.Capabilities}, {old.Fields, next.Fields}, {old.Scope.Channels, next.Scope.Channels}, {old.Scope.Resources, next.Scope.Resources}, {old.Scope.Spatial, next.Scope.Spatial}, {old.Scope.Artifacts, next.Scope.Artifacts}} {
		for _, v := range sets[1] {
			if !slices.Contains(sets[0], v) {
				return false
			}
		}
	}
	if next.Scope.Dynamic != nil && hash(old.Scope.Dynamic) != hash(next.Scope.Dynamic) {
		return false
	}
	before, e := readWindow(old.Scope.From, old.Scope.To)
	if e != nil {
		return false
	}
	after, e := readWindow(next.Scope.From, next.Scope.To)
	return e == nil && before.Covers(after)
}
func (s *Service) validateRequiredBindingsTx(ctx context.Context, tx pgx.Tx, scope Scope, m Manifest, bindings map[string]Binding, g *Grant) error {
	required := map[string]BindingSchema{}
	selected := map[string]Binding{}
	for name, schema := range m.Bindings {
		if schema.Required {
			required[name] = schema
			if b, ok := bindings[name]; ok {
				selected[name] = b
			}
		}
	}
	m.Bindings = required
	return s.validateBindingsTx(ctx, tx, scope, m, selected, g, true)
}
func intersectBindingRange(target *Binding, next Binding) error {
	a, e := readWindow(target.From, target.To)
	if e != nil {
		return e
	}
	b, e := readWindow(next.From, next.To)
	if e != nil {
		return e
	}
	v, ok := a.Intersect(b)
	if !ok {
		return invalid("/target", "不同绑定的时段不相交，请指定命名绑定。")
	}
	if v.From != nil {
		s := time.Unix(0, *v.From).UTC().Format(time.RFC3339Nano)
		target.From = &s
	}
	if v.To != nil {
		s := time.Unix(0, *v.To).UTC().Format(time.RFC3339Nano)
		target.To = &s
	}
	return nil
}
