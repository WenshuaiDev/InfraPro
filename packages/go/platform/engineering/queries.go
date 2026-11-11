package engineering

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

const blockedCTE = `WITH RECURSIVE blocked(id) AS (SELECT version_id FROM engineering_version_states WHERE NOT enabled UNION SELECT d.version_id FROM engineering_dependencies d JOIN blocked b ON b.id=d.target_id)`

func (s *Service) List(ctx context.Context, a access.Actor, scope Scope, f Filter) (Page, error) {
	out := Page{Items: []Summary{}, Offset: f.Offset, Limit: f.Limit}
	if out.Limit == 0 {
		out.Limit = 20
	}
	e := s.transact(ctx, a, scope.operation("list"), scope.Tenant, false, func(tx pgx.Tx) error {
		if out.Limit < 1 || out.Limit > 100 || f.Offset < 0 || f.Offset > 100000 || len([]rune(f.Query)) > 200 || !slices.Contains([]string{"", "usable", "unavailable", "draft"}, f.Status) {
			return access.Validation("filter", "无效的列表筛选或分页参数。")
		}
		base := ` FROM engineering_definitions d LEFT JOIN engineering_drafts dr ON dr.definition_id=d.id
 LEFT JOIN LATERAL (SELECT v.number,v.content FROM engineering_versions v WHERE v.definition_id=d.id AND (NOT $4 OR NOT EXISTS(SELECT 1 FROM blocked b WHERE b.id=v.id)) ORDER BY v.number DESC LIMIT 1) latest ON true
 WHERE d.scope_key=$1 AND d.kind=$2 AND d.deleted_at IS NULL AND (NOT $4 OR latest.number IS NOT NULL)
 AND ($3='' OR position(lower($3) in lower(d.code))>0 OR position(lower($3) in lower(CASE WHEN $4 THEN latest.content->>'name' ELSE COALESCE(dr.content->>'name',latest.content->>'name','') END))>0)
 AND ($5='' OR ($5='draft' AND NOT $4 AND dr.definition_id IS NOT NULL) OR ($5='usable' AND EXISTS(SELECT 1 FROM engineering_versions v WHERE v.definition_id=d.id AND NOT EXISTS(SELECT 1 FROM blocked b WHERE b.id=v.id))) OR ($5='unavailable' AND NOT EXISTS(SELECT 1 FROM engineering_versions v WHERE v.definition_id=d.id AND NOT EXISTS(SELECT 1 FROM blocked b WHERE b.id=v.id))))`
		args := []any{scope.key(), scope.Kind, strings.TrimSpace(f.Query), scope.Catalog, f.Status}
		if e := tx.QueryRow(ctx, blockedCTE+` SELECT count(*)`+base, args...).Scan(&out.Total); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, blockedCTE+` SELECT d.id::text,d.kind,d.code,CASE WHEN $4 THEN latest.content->>'name' ELSE COALESCE(dr.content->>'name',latest.content->>'name','') END,COALESCE(latest.number,0),NOT $4 AND dr.definition_id IS NOT NULL,EXISTS(SELECT 1 FROM engineering_versions v WHERE v.definition_id=d.id AND NOT EXISTS(SELECT 1 FROM blocked b WHERE b.id=v.id)),COALESCE(d.source_version::text,'')`+base+` ORDER BY d.created_at DESC,d.id DESC LIMIT $6 OFFSET $7`, append(args, out.Limit, out.Offset)...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v := Summary{}
			if e = rows.Scan(&v.ID, &v.Kind, &v.Code, &v.Name, &v.Latest, &v.HasDraft, &v.Usable, &v.SourceVersion); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		return rows.Err()
	})
	return out, e
}
func detailTx(ctx context.Context, tx pgx.Tx, scope Scope, id string) (Definition, error) {
	d, e := definitionTx(ctx, tx, scope, id, false)
	if e != nil {
		return d, e
	}
	r := resolve(ctx, tx)
	rows, e := tx.Query(ctx, `SELECT id::text FROM engineering_versions WHERE definition_id=$1 ORDER BY number DESC`, id)
	if e != nil {
		return d, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return d, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return d, e
	}
	deps := map[string]Version{}
	collect := func(c Content) error {
		v, e := r.dependencies(c)
		if e != nil {
			return e
		}
		for _, v := range v {
			deps[v.ID] = v
		}
		return nil
	}
	for _, id := range ids {
		v, e := r.version(id)
		if e != nil {
			return d, e
		}
		if scope.Catalog && !v.Usable {
			continue
		}
		d.Versions = append(d.Versions, v)
		if e = collect(v.Content); e != nil {
			return d, e
		}
	}
	if scope.Catalog {
		d.Draft = nil
		if len(d.Versions) == 0 {
			return Definition{}, access.ErrNotFound
		}
	} else if d.Draft != nil {
		if e = collect(d.Draft.Content); e != nil {
			return d, e
		}
	}
	for _, v := range deps {
		d.Dependencies = append(d.Dependencies, v)
	}
	slices.SortFunc(d.Dependencies, func(a, b Version) int { return strings.Compare(a.ID, b.ID) })
	return d, nil
}
func (s *Service) Detail(ctx context.Context, a access.Actor, scope Scope, id string) (Definition, error) {
	var out Definition
	e := s.transact(ctx, a, scope.operation("detail"), scope.Tenant, false, func(tx pgx.Tx) error { var e error; out, e = detailTx(ctx, tx, scope, id); return e })
	return out, e
}
func selectContent(d Definition, version string, rev int) (Content, error) {
	if version == "" {
		if d.Draft == nil {
			return Content{}, access.ErrNotFound
		}
		if e := revision(d, rev); e != nil {
			return Content{}, e
		}
		return d.Draft.Content, nil
	}
	for _, v := range d.Versions {
		if v.ID == version {
			return v.Content, nil
		}
	}
	return Content{}, access.ErrNotFound
}
func (s *Service) Preview(ctx context.Context, a access.Actor, scope Scope, id string, in PreviewInput) (Preview, error) {
	out := Preview{SampleOnly: true, Issues: []Issue{}, Dependencies: []Version{}}
	e := s.transact(ctx, a, scope.operation("preview"), scope.Tenant, false, func(tx pgx.Tx) error {
		d, e := detailTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		c, e := selectContent(d, in.VersionID, in.Revision)
		if e != nil {
			return e
		}
		r := resolve(ctx, tx)
		out.Content = c
		out.Issues = ValidateContent(scope.Kind, c)
		out.Dependencies, e = r.dependencies(c)
		if e != nil {
			return e
		}
		refs, e := r.checkReferences(c, in.VersionID == "", nil)
		if e != nil {
			return e
		}
		out.Issues = append(out.Issues, refs...)
		if scope.Kind == TypeDefinition && in.Sample != nil {
			out.Issues = append(out.Issues, ValidateSample(c, *in.Sample)...)
		}
		if scope.Kind == MonitoringTemplate && in.Sample != nil {
			if c.Template == nil || !slices.Contains(c.Template.TypeVersions, in.TypeVersion) {
				return access.Validation("type_version", "请选择模板的一个适用类型版本。")
			}
			v, e := r.public(in.TypeVersion, TypeDefinition)
			if e != nil {
				return e
			}
			out.Issues = append(out.Issues, ValidateSample(v.Content, *in.Sample)...)
		}
		if scope.Kind == MetricDefinition && len(in.Value) > 0 && c.Metric != nil {
			m := c.Metric
			if (m.Shape == "enum" && in.UnitVersion != "") || (m.Shape != "enum" && !slices.Contains(m.AllowedUnits, in.UnitVersion)) {
				add(&out.Issues, "metric_unit_mismatch", "请选择指标允许的单位。", "/value", "")
			}
			var v any
			if e = json.Unmarshal(in.Value, &v); e != nil {
				return access.Validation("value", "无效的 JSON 样例值。")
			}
			out.Issues = append(out.Issues, ValidateValues([]Field{{Key: "value", Shape: m.Shape, Required: true, Minimum: m.Minimum, Maximum: m.Maximum, Options: m.Options}}, map[string]any{"value": v}, "")...)
		}
		out.Valid = len(out.Issues) == 0
		return nil
	})
	return out, e
}

// Differences describe explicit snapshots; they never follow latest-version pointers.
func Differences(before, after any) []Difference {
	normalize := func(v any) any { b, _ := json.Marshal(v); var out any; _ = json.Unmarshal(b, &out); return out }
	out := []Difference{}
	var walk func(string, any, any)
	walk = func(path string, a, b any) {
		if reflect.DeepEqual(a, b) {
			return
		}
		am, aok := a.(map[string]any)
		bm, bok := b.(map[string]any)
		if aok && bok {
			keys := []string{}
			for k := range am {
				keys = append(keys, k)
			}
			for k := range bm {
				if _, ok := am[k]; !ok {
					keys = append(keys, k)
				}
			}
			slices.Sort(keys)
			for _, k := range keys {
				walk(path+"/"+k, am[k], bm[k])
			}
			return
		}
		aa, aok := a.([]any)
		ba, bok := b.([]any)
		if aok && bok {
			for i := 0; i < max(len(aa), len(ba)); i++ {
				var av, bv any
				if i < len(aa) {
					av = aa[i]
				}
				if i < len(ba) {
					bv = ba[i]
				}
				walk(path+"/"+strconv.Itoa(i), av, bv)
			}
			return
		}
		out = append(out, Difference{path, a, b})
	}
	walk("", normalize(before), normalize(after))
	return out
}
func (s *Service) Diff(ctx context.Context, a access.Actor, scope Scope, id, from, to string, rev int) ([]Difference, error) {
	out := []Difference{}
	e := s.transact(ctx, a, scope.operation("diff"), scope.Tenant, false, func(tx pgx.Tx) error {
		d, e := detailTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		before, e := selectContent(d, from, rev)
		if e != nil {
			return e
		}
		after, e := selectContent(d, to, rev)
		if e != nil {
			return e
		}
		out = Differences(before, after)
		return nil
	})
	return out, e
}
func (s *Service) Impact(ctx context.Context, a access.Actor, scope Scope, id, version string) (Impact, error) {
	out := Impact{Public: []Summary{}}
	e := s.transact(ctx, a, scope.operation("impact"), scope.Tenant, false, func(tx pgx.Tx) error {
		d, e := detailTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if _, e = selectContent(d, version, 0); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `WITH RECURSIVE refs(id) AS (SELECT version_id FROM engineering_dependencies WHERE target_id::text=$1 UNION SELECT d.version_id FROM engineering_dependencies d JOIN refs r ON r.id=d.target_id) SELECT DISTINCT d.id::text,d.kind,d.code,v.content->>'name',v.number,COALESCE(d.tenant_id::text,'') FROM refs r JOIN engineering_versions v ON v.id=r.id JOIN engineering_definitions d ON d.id=v.definition_id`, version)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v := Summary{}
			var tenant string
			if e = rows.Scan(&v.ID, &v.Kind, &v.Code, &v.Name, &v.Latest, &tenant); e != nil {
				return e
			}
			if tenant == scope.owner() {
				out.Public = append(out.Public, v)
			} else if tenant != "" {
				out.PrivateReferences = true
			}
		}
		if e = rows.Err(); e != nil {
			return e
		}
		rows.Close()
		if s.usage != nil {
			out.ResourceReferences, e = s.usage.HasResourceReferences(ctx, tx, version)
		}
		return e
	})
	return out, e
}
func diagnosticError(issues []Issue) error {
	if len(issues) == 0 {
		return nil
	}
	for _, i := range issues {
		if i.Code == "dependency_unavailable" {
			return &Error{Code: "dependency_unavailable", Message: "依赖状态已变化，当前操作不能继续。", Status: 409, Issues: issues}
		}
	}
	return invalid(issues)
}
func checkedDraft(r *resolver, kind Kind, c Content, previous *Content) ([]Issue, error) {
	if e := contentBoundary(kind, c); e != nil {
		return nil, e
	}
	refs, e := r.checkReferences(c, true, previous)
	if e != nil {
		return nil, e
	}
	if e = diagnosticError(refs); e != nil {
		return nil, e
	}
	return ValidateContent(kind, c), nil
}
func auditSummary(d Definition, version string, changes []Difference) map[string]any {
	name := ""
	if d.Draft != nil {
		name = d.Draft.Content.Name
	}
	return map[string]any{"name": name, "id": d.ID, "code": d.Code, "kind": d.Kind, "version_id": version, "source_version": d.SourceVersion, "changes": changes}
}
func missingDraft() error {
	return problem("draft_missing", "当前没有草稿，请从明确版本建立修订草稿。", 409)
}
func stateMessage(enabled bool) string {
	if enabled {
		return "恢复版本"
	}
	return "停用版本"
}

// Receipt only returns this actor's result after current read and original action
// permissions have been revalidated. Tombstones still bind the resource kind/scope.
func (s *Service) Receipt(ctx context.Context, a access.Actor, scope Scope, request string) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope.operation("receipt"), scope.Tenant, false, func(tx pgx.Tx) error {
		if !access.ValidID(request) {
			return access.ErrNotFound
		}
		var operation string
		e := tx.QueryRow(ctx, `SELECT r.operation,r.result FROM engineering_operation_receipts r JOIN engineering_definitions d ON d.id::text=r.result->>'definition_id' WHERE r.scope_key=$1 AND r.actor_id=$2 AND r.request_id=$3 AND d.kind=$4 AND d.scope_key=$1`, scope.key(), a.ID, request, scope.Kind).Scan(&operation, &out)
		if e == pgx.ErrNoRows {
			return access.ErrNotFound
		}
		if e != nil {
			return e
		}
		return access.RequireScopedTx(ctx, tx, a.ID, scope.operation(operation), scope.Tenant)
	})
	return out, e
}
