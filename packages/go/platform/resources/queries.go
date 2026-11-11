package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

func ancestorsTx(ctx context.Context, tx pgx.Tx, t Target, r, facility Resource) ([]Resource, error) {
	out := []Resource{}
	if r.Kind == Facility {
		return out, nil
	}
	out = append(out, facility)
	var point *Resource
	parent := ""
	switch r.Kind {
	case Component:
		parent = ptrValue(r.ParentComponentID)
	case Point:
		parent = ptrValue(r.ComponentID)
	case Channel:
		p, e := load(ctx, tx, Target{Tenant: t.Tenant, Facility: t.Facility, Kind: Point, ID: r.PointID}, false)
		if e != nil {
			return nil, e
		}
		point = &p
		parent = ptrValue(p.ComponentID)
	}
	chain := []Resource{}
	seen := map[string]bool{}
	for parent != "" {
		if seen[parent] {
			return nil, invalid("/parent_component_id", "cycle", "构件归属不能形成环。")
		}
		seen[parent] = true
		p, e := load(ctx, tx, Target{Tenant: t.Tenant, Facility: t.Facility, Kind: Component, ID: parent}, false)
		if e != nil {
			return nil, e
		}
		chain = append(chain, p)
		parent = ptrValue(p.ParentComponentID)
	}
	slices.Reverse(chain)
	out = append(out, chain...)
	if point != nil {
		out = append(out, *point)
	}
	return out, nil
}
func (s *Service) Detail(ctx context.Context, a access.Actor, t Target) (Detail, error) {
	out := Detail{Ancestors: []Resource{}, Definitions: []engineering.Version{}}
	if e := checkTarget(t); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, access.ResourceOperation(string(t.Kind), "detail"), t.Tenant, func(tx pgx.Tx) error {
		f, e := lockFacility(ctx, tx, t, false, false)
		if e != nil {
			return e
		}
		r, e := load(ctx, tx, t, false)
		if e != nil {
			return e
		}
		out.Resource = r
		out.Ancestors, e = ancestorsTx(ctx, tx, t, r, f)
		if e != nil {
			return e
		}
		refs := map[string]engineering.Kind{f.TypeVersionID: engineering.TypeDefinition}
		if r.Kind == Channel {
			refs[r.MetricVersionID] = engineering.MetricDefinition
		}
		out.Definitions, e = fixedDefinitions(ctx, tx, refs)
		if e != nil {
			return e
		}
		manage := access.RequireScopedTx(ctx, tx, a.ID, access.ResourceOperation(string(t.Kind), "update"), t.Tenant) == nil
		out.Actions = actionAllowed(r, out.Ancestors, manage)
		return nil
	})
	return out, e
}

// References must come from resources or history already authorized in this transaction.
func fixedDefinitions(ctx context.Context, tx pgx.Tx, refs map[string]engineering.Kind) ([]engineering.Version, error) {
	out := []engineering.Version{}
	seen := map[string]bool{}
	for id, kind := range refs {
		v, deps, e := engineering.ResolvePublicTx(ctx, tx, id, kind, false)
		if e != nil {
			return nil, e
		}
		for _, v := range append([]engineering.Version{v}, deps...) {
			if !seen[v.ID] {
				seen[v.ID] = true
				out = append(out, v)
			}
		}
	}
	slices.SortFunc(out, func(a, b engineering.Version) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

type cursor struct {
	Scope string    `json:"scope"`
	Time  time.Time `json:"time"`
	ID    string    `json:"id"`
}

func cursorScope(t Target, f Filter, history bool) string {
	f.Cursor = ""
	f.Limit = 0
	return fingerprint(struct {
		Target  Target
		Filter  Filter
		History bool
	}{t, f, history})
}
func readCursor(t Target, f Filter, history bool) (cursor, error) {
	out := cursor{Scope: cursorScope(t, f, history)}
	if f.Cursor == "" {
		return out, nil
	}
	b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
	if e != nil || strict(b, &out) != nil || out.Scope != cursorScope(t, f, history) || !access.ValidID(out.ID) || out.Time.IsZero() {
		return out, problem("invalid_request", "分页游标无效或不属于当前查询。", 400)
	}
	return out, nil
}
func nextCursor(t Target, f Filter, history bool, at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString(jsonBytes(cursor{cursorScope(t, f, history), at, id}))
}
func normalizeFilter(f *Filter) error {
	f.Query = strings.TrimSpace(f.Query)
	if f.Limit == 0 {
		f.Limit = 25
	}
	if f.Limit < 1 || f.Limit > 100 || utf8.RuneCountInString(f.Query) > 200 {
		return problem("invalid_request", "分页大小须为 1–100，搜索最多 200 字符。", 400)
	}
	if f.Status == "" {
		f.Status = "normal"
	}
	if !slices.Contains([]string{"normal", "retired", "all"}, f.Status) {
		return problem("invalid_request", "状态筛选无效。", 400)
	}
	if f.Deleted != "" && f.Deleted != "only" {
		return problem("invalid_request", "删除筛选无效。", 400)
	}
	for _, id := range []string{f.TypeDefinition, f.ID} {
		if id != "" && !access.ValidID(id) {
			return problem("invalid_request", "筛选引用无效。", 400)
		}
	}
	return nil
}
func (s *Service) List(ctx context.Context, a access.Actor, t Target, f Filter) (Page, error) {
	out := Page{Items: []Item{}}
	if e := checkTarget(t); e != nil {
		return out, e
	}
	if e := normalizeFilter(&f); e != nil {
		return out, e
	}
	if t.Kind == Component && f.Parent == "" {
		f.Parent = "root"
	}
	c, e := readCursor(t, f, false)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, access.ResourceOperation(string(t.Kind), "list"), t.Tenant, func(tx pgx.Tx) error {
		if t.Kind != Facility {
			if _, e := lockFacility(ctx, tx, t, false, false); e != nil {
				return e
			}
		}
		owner := f.Owner
		if t.Kind == Component {
			owner = f.Parent
		}
		if owner != "" && owner != "root" && owner != "all" {
			if !access.ValidID(owner) {
				return problem("invalid_request", "归属筛选无效。", 400)
			}
			if _, e := load(ctx, tx, Target{Kind: Component, Tenant: t.Tenant, Facility: t.Facility, ID: owner}, false); e != nil {
				return e
			}
		}
		if t.Kind == Channel {
			if _, e := load(ctx, tx, Target{Kind: Point, Tenant: t.Tenant, Facility: t.Facility, ID: t.Point}, false); e != nil {
				return e
			}
		}
		args := []any{t.Tenant, f.Query, f.Status, f.Limit + 1, c.ID, c.Time}
		where := `r.tenant_id::text=$1 AND ($2='' OR r.name ILIKE '%'||$2||'%' OR r.code ILIKE '%'||$2||'%') AND ($3='all' OR r.status=$3) AND ($5='' OR (r.created_at,r.id)>($6::timestamptz,NULLIF($5,'')::uuid))`
		if f.Deleted == "only" {
			where += " AND r.deleted_at IS NOT NULL"
		} else {
			where += " AND r.deleted_at IS NULL"
		}
		if t.Kind == Facility && f.TypeDefinition != "" {
			where += ` AND EXISTS(SELECT 1 FROM engineering_versions v WHERE v.id=r.type_version_id AND v.definition_id::text=$7)`
			args = append(args, f.TypeDefinition)
		}
		if t.Kind != Facility {
			where += " AND r.facility_id::text=$7"
			args = append(args, t.Facility)
		}
		if t.Kind == Component || t.Kind == Point {
			col := "component_id"
			if t.Kind == Component {
				col = "parent_component_id"
			}
			if owner == "root" {
				where += " AND r." + col + " IS NULL"
			} else if owner != "" && owner != "all" {
				where += " AND r." + col + "::text=$8"
				args = append(args, owner)
			}
		}
		if t.Kind == Channel {
			where += " AND r.point_id::text=$8"
			args = append(args, t.Point)
		}
		children := "false"
		if t.Kind == Component {
			children = `EXISTS(SELECT 1 FROM engineering_components c WHERE c.tenant_id=r.tenant_id AND c.facility_id=r.facility_id AND c.parent_component_id=r.id AND c.deleted_at IS NULL)`
		}
		projection := "to_jsonb(r)"
		if t.Kind == Facility {
			projection += ` || (SELECT jsonb_build_object('type_name',v.content->>'name','type_number',v.number,'type_definition_id',v.definition_id) FROM engineering_versions v WHERE v.id=r.type_version_id)`
		}
		rows, e := tx.Query(ctx, `SELECT `+projection+`,`+children+` FROM `+tables[t.Kind]+` r WHERE `+where+` ORDER BY r.created_at,r.id LIMIT $4`, args...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			item := Item{Resource: Resource{Kind: t.Kind}}
			var raw []byte
			if e = rows.Scan(&raw, &item.HasChildren); e != nil {
				return e
			}
			if e = json.Unmarshal(raw, &item); e != nil {
				return e
			}
			out.Items = append(out.Items, item)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			last := out.Items[f.Limit-1]
			out.NextCursor = nextCursor(t, f, false, last.CreatedAt, last.ID)
			out.Items = out.Items[:f.Limit]
		}
		if f.Deleted == "only" {
			for i, item := range out.Items {
				r := item.Resource
				out.Items[i] = Item{Resource: Resource{ID: r.ID, TenantID: r.TenantID, Kind: r.Kind, Code: r.Code, Name: r.Name, Status: r.Status, Revision: r.Revision, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, CreatedBy: r.CreatedBy, UpdatedBy: r.UpdatedBy, DeletedAt: r.DeletedAt}}
			}
		}
		return nil
	})
	return out, e
}
func (s *Service) Changes(ctx context.Context, a access.Actor, t Target, f Filter) (Changes, error) {
	out := Changes{Items: []Change{}, Definitions: []engineering.Version{}}
	if e := checkTarget(t); e != nil {
		return out, e
	}
	if e := normalizeFilter(&f); e != nil {
		return out, e
	}
	if f.Kind != "" && tables[Kind(f.Kind)] == "" {
		return out, problem("invalid_request", "历史种类筛选无效。", 400)
	}
	c, e := readCursor(t, f, true)
	if e != nil {
		return out, e
	}
	e = s.transact(ctx, a, access.ResourceOperation("facility", "changes"), t.Tenant, func(tx pgx.Tx) error {
		facility, e := lockFacility(ctx, tx, t, false, true)
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(c) FROM engineering_resource_changes c WHERE tenant_id::text=$1 AND facility_id::text=$2 AND ($3='' OR resource_kind=$3) AND ($4='' OR resource_id::text=$4) AND ($5='' OR (changed_at,id)<($6::timestamptz,NULLIF($5,'')::uuid)) ORDER BY changed_at DESC,id DESC LIMIT $7`, t.Tenant, t.Facility, f.Kind, f.ID, c.ID, c.Time, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var b []byte
			var v Change
			if e = rows.Scan(&b); e != nil {
				return e
			}
			if e = json.Unmarshal(b, &v); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		rows.Close()
		if len(out.Items) > f.Limit {
			last := out.Items[f.Limit-1]
			out.NextCursor = nextCursor(t, f, true, last.ChangedAt, last.ID)
			out.Items = out.Items[:f.Limit]
		}
		refs := map[string]engineering.Kind{}
		if len(out.Items) > 0 {
			refs[facility.TypeVersionID] = engineering.TypeDefinition
		}
		for _, change := range out.Items {
			if change.ResourceKind == Channel {
				refs[change.After.MetricVersionID] = engineering.MetricDefinition
			}
		}
		out.Definitions, e = fixedDefinitions(ctx, tx, refs)
		return e
	})
	return out, e
}
