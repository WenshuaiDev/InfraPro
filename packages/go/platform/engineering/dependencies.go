package engineering

import (
	"context"
	"fmt"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type reference struct {
	ID, Path string
	Kind     Kind
}

func references(c Content) []reference {
	out := []reference{}
	add := func(id, path string, k Kind) {
		if id != "" {
			out = append(out, reference{id, path, k})
		}
	}
	fields := func(fs []Field, p string) {
		for i, f := range fs {
			add(f.UnitVersion, fmt.Sprintf("%s/%d/unit_version", p, i), UnitDefinition)
		}
	}
	fields(c.Fields, "/fields")
	for i, v := range c.Components {
		fields(v.Fields, fmt.Sprintf("/components/%d/fields", i))
	}
	if m := c.Metric; m != nil {
		add(m.StandardUnit, "/metric/standard_unit", UnitDefinition)
		for i, id := range m.AllowedUnits {
			add(id, fmt.Sprintf("/metric/allowed_units/%d", i), UnitDefinition)
		}
	}
	if t := c.Template; t != nil {
		for i, id := range t.TypeVersions {
			add(id, fmt.Sprintf("/template/type_versions/%d", i), TypeDefinition)
		}
		for i, item := range t.Items {
			for j, v := range item.Targets {
				add(v.TypeVersion, fmt.Sprintf("/template/items/%d/targets/%d/type_version", i, j), TypeDefinition)
			}
			for j, v := range item.Bindings {
				p := fmt.Sprintf("/template/items/%d/bindings/%d", i, j)
				add(v.MetricVersion, p+"/metric_version", MetricDefinition)
				add(v.UnitVersion, p+"/unit_version", UnitDefinition)
			}
		}
	}
	return out
}

type resolver struct {
	ctx   context.Context
	tx    pgx.Tx
	cache map[string]Version
}

func resolve(ctx context.Context, tx pgx.Tx) *resolver {
	return &resolver{ctx, tx, map[string]Version{}}
}
func (r *resolver) version(id string) (Version, error) {
	if v, ok := r.cache[id]; ok {
		return v, nil
	}
	v := Version{Issues: []Issue{}, Dependencies: []string{}}
	if !access.ValidID(id) {
		return v, access.ErrNotFound
	}
	e := r.tx.QueryRow(r.ctx, `SELECT v.id::text,v.definition_id::text,d.kind,d.code,v.number,v.content,v.digest,v.schema_version,v.schemas,v.unit_category,v.published_at,v.published_by::text,s.enabled,s.revision,s.reason FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id JOIN engineering_version_states s ON s.version_id=v.id WHERE v.id=$1`, id).Scan(&v.ID, &v.DefinitionID, &v.Kind, &v.Code, &v.Number, &v.Content, &v.Digest, &v.SchemaVersion, &v.Schemas, &v.UnitCategory, &v.PublishedAt, &v.PublishedBy, &v.Enabled, &v.StateRevision, &v.StateReason)
	if e == pgx.ErrNoRows {
		return v, access.ErrNotFound
	}
	if e != nil {
		return v, e
	}
	rows, e := r.tx.Query(r.ctx, `SELECT DISTINCT target_id::text FROM engineering_dependencies WHERE version_id=$1 ORDER BY target_id::text`, id)
	if e != nil {
		return v, e
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return v, e
		}
		v.Dependencies = append(v.Dependencies, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return v, e
	}
	v.Usable = v.Enabled
	if !v.Enabled {
		add(&v.Issues, "version_disabled", "版本已停用，不能新采用。", "/versions/"+id, id)
	}
	// Dependency kinds form a DAG: template -> type/metric -> unit. SQL rejects private and template targets.
	for _, dep := range v.Dependencies {
		d, e := r.version(dep)
		if e != nil {
			return v, e
		}
		if !d.Usable {
			v.Usable = false
			add(&v.Issues, "dependency_unavailable", "固定依赖已停用，不能新采用。", "/dependencies/"+dep, dep)
		}
	}
	r.cache[id] = v
	return v, nil
}
func (r *resolver) public(id string, kind Kind) (Version, error) {
	v, e := r.version(id)
	if e != nil {
		return v, e
	}
	var pub bool
	e = r.tx.QueryRow(r.ctx, `SELECT tenant_id IS NULL FROM engineering_definitions WHERE id=$1`, v.DefinitionID).Scan(&pub)
	if e != nil {
		return v, e
	}
	if !pub || v.Kind != kind {
		return Version{}, access.ErrNotFound
	}
	return v, nil
}
func (r *resolver) dependencies(c Content) ([]Version, error) {
	found := map[string]Version{}
	var walk func(Version) error
	walk = func(v Version) error {
		if _, ok := found[v.ID]; ok {
			return nil
		}
		found[v.ID] = v
		for _, id := range v.Dependencies {
			dep, e := r.version(id)
			if e != nil {
				return e
			}
			if e = walk(dep); e != nil {
				return e
			}
		}
		return nil
	}
	for _, ref := range references(c) {
		v, e := r.public(ref.ID, ref.Kind)
		if e != nil {
			return nil, e
		}
		if e = walk(v); e != nil {
			return nil, e
		}
	}
	out := []Version{}
	for _, v := range found {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b Version) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out, nil
}

// Explicit references are always checked, even when an otherwise incomplete draft is saved.
func (r *resolver) checkReferences(c Content, newUse bool, previous *Content) ([]Issue, error) {
	issues := []Issue{}
	old := map[string]bool{}
	if previous != nil {
		for _, v := range references(*previous) {
			old[v.ID] = true
		}
	}
	for _, ref := range references(c) {
		v, e := r.public(ref.ID, ref.Kind)
		if e != nil {
			if e == access.ErrNotFound {
				return nil, &Error{Code: "invalid_reference", Message: "引用的公共发布版本不存在或不可访问。", Status: 422, Issues: []Issue{issue("invalid_reference", "请选择可访问的明确发布版本。", ref.Path, "")}}
			}
			return nil, e
		}
		if newUse && !old[ref.ID] && !v.Usable {
			issues = append(issues, issue("dependency_unavailable", "依赖已不可新使用，请选择可用版本。", ref.Path, ref.ID))
		}
	}
	if c.Unit != nil && c.Unit.Category != "" {
		if _, e := r.category(c.Unit.Category); e != nil {
			return nil, e
		}
	}
	if m := c.Metric; m != nil && m.Category != "" {
		if _, e := r.category(m.Category); e != nil {
			return nil, e
		}
		for _, id := range m.AllowedUnits {
			v, e := r.public(id, UnitDefinition)
			if e != nil {
				return nil, e
			}
			if v.Content.Unit.Category != m.Category {
				add(&issues, "unit_category_mismatch", "允许单位与指标的单位类别不一致。", "/metric/allowed_units", id)
			}
		}
	}
	if t := c.Template; t != nil {
		for i, item := range t.Items {
			for j, target := range item.Targets {
				if target.TypeVersion == "" {
					continue
				}
				v, e := r.public(target.TypeVersion, TypeDefinition)
				if e != nil {
					return nil, e
				}
				found := target.Component == "root"
				for _, c := range v.Content.Components {
					found = found || c.Key == target.Component
				}
				if !found {
					add(&issues, "target_component_missing", "映射的构件类别在该类型版本中不存在。", fmt.Sprintf("/template/items/%d/targets/%d", i, j), item.Key)
				}
			}
			for j, b := range item.Bindings {
				if b.MetricVersion == "" {
					continue
				}
				v, e := r.public(b.MetricVersion, MetricDefinition)
				if e != nil {
					return nil, e
				}
				m := v.Content.Metric
				if (m.Shape == "enum" && b.UnitVersion != "") || (m.Shape != "enum" && !slices.Contains(m.AllowedUnits, b.UnitVersion)) {
					add(&issues, "metric_unit_mismatch", "绑定单位不属于指标允许单位。", fmt.Sprintf("/template/items/%d/bindings/%d/unit_version", i, j), item.Key)
				}
			}
		}
	}
	return issues, nil
}
func (r *resolver) category(k string) (Category, error) {
	v := Category{}
	e := r.tx.QueryRow(r.ctx, `SELECT key,name,description,revision FROM engineering_unit_categories WHERE key=$1`, k).Scan(&v.Key, &v.Name, &v.Description, &v.Revision)
	if e == pgx.ErrNoRows {
		return v, invalid([]Issue{issue("category_missing", "单位类别不存在。", "/category", k)})
	}
	return v, e
}
func (r *resolver) stable(previous, current Content) []Issue {
	out := []Issue{}
	check := func(old, fs []Field, p string) {
		for _, a := range old {
			for _, b := range fs {
				if a.Key != b.Key {
					continue
				}
				if a.Shape != b.Shape {
					add(&out, "stable_key_shape", "已发布属性键不能改用于另一种数据形态；请使用新键。", p+"/"+b.Key, b.Key)
				}
				category := func(id string) string {
					if id == "" {
						return ""
					}
					v, e := r.public(id, UnitDefinition)
					if e != nil || v.Content.Unit == nil {
						return "invalid"
					}
					return v.Content.Unit.Category
				}
				if category(a.UnitVersion) != category(b.UnitVersion) {
					add(&out, "stable_key_dimension", "已发布属性键不能改变量纲；请使用新键。", p+"/"+b.Key, b.Key)
				}
			}
		}
	}
	check(previous.Fields, current.Fields, "/fields")
	for _, a := range previous.Components {
		for _, b := range current.Components {
			if a.Key == b.Key {
				check(a.Fields, b.Fields, "/components/"+b.Key+"/fields")
			}
		}
	}
	if previous.Unit != nil && current.Unit != nil && previous.Unit.Category != current.Unit.Category {
		add(&out, "stable_unit_category", "同一单位身份不能改变量纲。", "/unit/category", "")
	}
	if previous.Metric != nil && current.Metric != nil && (previous.Metric.Shape != current.Metric.Shape || previous.Metric.Category != current.Metric.Category) {
		add(&out, "stable_metric_shape", "指标核心形态或量纲改变时请新建指标。", "/metric", "")
	}
	return out
}

// Historical dependency resolution is limited to an already authorized definition.
// New adoption separately requires read permission on each public source catalog.
func requireReferenceReads(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, c Content, previous *Content) error {
	old := map[string]bool{}
	if previous != nil {
		for _, ref := range references(*previous) {
			old[ref.ID] = true
		}
	}
	seen := map[Kind]bool{}
	for _, ref := range references(c) {
		if old[ref.ID] || seen[ref.Kind] {
			continue
		}
		seen[ref.Kind] = true
		op := access.EngineeringOperation(false, scope.Tenant != "", string(ref.Kind), "detail")
		if e := access.RequireScopedTx(ctx, tx, a.ID, op, scope.Tenant); e != nil {
			return e
		}
	}
	return nil
}
