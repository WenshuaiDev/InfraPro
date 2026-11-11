package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

func ancestorsTx(ctx context.Context, tx pgx.Tx, t scopedTarget, r, facility Resource) ([]Resource, error) {
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
		p, e := load(ctx, tx, scoped(t.Scope, Target{Facility: t.Facility, Kind: Point, ID: r.PointID}), false)
		if e != nil {
			return nil, e
		}
		point = &p
		parent = ptrValue(p.ComponentID)
	}
	chain := []Resource{}
	if parent != "" {
		rows, e := tx.Query(ctx, `WITH RECURSIVE path AS (
 SELECT c.*,1 AS depth FROM engineering_components c WHERE c.tenant_id=$1 AND c.facility_id=$2 AND c.id=$3 AND c.deleted_at IS NULL
 UNION ALL SELECT c.*,p.depth+1 FROM engineering_components c JOIN path p ON c.id=p.parent_component_id WHERE c.tenant_id=$1 AND c.facility_id=$2 AND c.deleted_at IS NULL)
 SELECT to_jsonb(path) FROM path ORDER BY depth DESC`, t.Tenant, t.Facility, parent)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			r := Resource{Kind: Component}
			var raw []byte
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return nil, e
			}
			if e = json.Unmarshal(raw, &r); e != nil {
				rows.Close()
				return nil, e
			}
			chain = append(chain, r)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		if len(chain) == 0 {
			return nil, notFound()
		}
	}
	out = append(out, chain...)
	if point != nil {
		out = append(out, *point)
	}
	return out, nil
}
func (s *Service) Detail(ctx context.Context, a access.Actor, scope Scope, target Target) (Detail, error) {
	t := scoped(scope, target)
	out := Detail{Ancestors: []Resource{}, Definitions: []engineering.Version{}}
	if e := checkTarget(t); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, t.operation(string(t.Kind), "detail"), t.Scope, false, func(tx pgx.Tx, auth authorization) error {
		f, e := lockFacility(ctx, tx, t, false, false)
		if e != nil {
			return e
		}
		var grant Grant
		if auth.Entry == "project" {
			var link *Link
			grant, link, e = projectGrantTx(ctx, tx, auth, t, true)
			if e != nil {
				return e
			}
			out.AccessRevision = auth.AccessRevision
			out.LinkID = link.ID
			out.LinkRevision = link.Revision
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
		manage := access.RequireScopedTx(ctx, tx, a.ID, t.operation(string(t.Kind), "update"), t.Tenant) == nil
		out.Actions = actionAllowed(r, out.Ancestors, manage)
		out.Actions.Read = true
		out.Actions.CreateComponent = out.Actions.Add && (r.Kind == Facility || r.Kind == Component)
		out.Actions.CreatePoint = out.Actions.Add && (r.Kind == Facility || r.Kind == Component)
		out.Actions.CreateChannel = out.Actions.Add && r.Kind == Point
		out.Shared, e = sharedTx(ctx, tx, auth, t)
		if e != nil {
			return e
		}
		if auth.Entry == "project" {
			out.Actions, e = projectActions(ctx, tx, auth, t, r, out.Ancestors, grant)
			if e != nil {
				return e
			}
			out.Ancestors, e = projectAncestors(ctx, tx, auth, t.Facility, out.Ancestors)
			if e != nil {
				return e
			}
		} else {
			for i := range out.Ancestors {
				out.Ancestors[i] = sourceProjection(out.Ancestors[i], auth)
			}
		}
		out.Resource = sourceProjection(r, auth)
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

func cursorScope(t scopedTarget, f Filter, history bool) string {
	f.Cursor = ""
	f.Limit = 0
	return fingerprint(struct {
		Target  scopedTarget
		Filter  Filter
		History bool
	}{t, f, history})
}
func readCursor(t scopedTarget, f Filter, history bool) (cursor, error) {
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
func nextCursor(t scopedTarget, f Filter, history bool, at time.Time, id string) string {
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
func (s *Service) List(ctx context.Context, a access.Actor, scope Scope, target Target, f Filter) (Page, error) {
	t := scoped(scope, target)
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
	e := s.transact(ctx, a, t.operation(string(t.Kind), "list"), t.Scope, false, func(tx pgx.Tx, auth authorization) error {
		t.CursorScope = fingerprint([]string{auth.Account, auth.TenantMembership, auth.ProjectAuth.MembershipID, auth.AccessRevision})
		c, e := readCursor(t, f, false)
		if e != nil {
			return e
		}
		if auth.Entry == "project" {
			if f.Deleted != "" {
				return access.ErrDenied
			}
			if t.Facility != "" {
				link, e := linkTx(ctx, tx, auth.Tenant, auth.Project, t.Facility)
				if e != nil {
					return e
				}
				if link == nil {
					return notFound()
				}
			}
		}

		out.AccessRevision = auth.AccessRevision
		out.CanCreateFacility = auth.Entry == "project" && auth.AllowFacilityCreate && auth.can(Facility, "create")
		if auth.Entry == "tenant" {
			out.CanCreateFacility = access.RequireScopedTx(ctx, tx, a.ID, access.ResourceOperation("facility", "create"), auth.Tenant) == nil
		}

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
			if auth.Entry == "project" {
				if _, _, e := projectGrantTx(ctx, tx, auth, scoped(t.Scope, Target{Kind: Component, Facility: t.Facility, ID: owner}), false); e != nil {
					return e
				}
			}
			if _, e := load(ctx, tx, scoped(t.Scope, Target{Kind: Component, Facility: t.Facility, ID: owner}), false); e != nil {
				return e
			}
		}
		if t.Kind == Channel {
			if auth.Entry == "project" {
				if _, _, e := projectGrantTx(ctx, tx, auth, scoped(t.Scope, Target{Kind: Point, Facility: t.Facility, ID: t.Point}), false); e != nil {
					return e
				}
			}
			if _, e := load(ctx, tx, scoped(t.Scope, Target{Kind: Point, Facility: t.Facility, ID: t.Point}), false); e != nil {
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
		cte := ""
		if auth.Entry == "project" {
			args = append(args, auth.Project, t.Facility)
			cte = fmt.Sprintf(`WITH visible AS MATERIALIZED (SELECT * FROM project_resource_visibility($1::text::uuid,$%d::uuid,NULLIF($%d,'')::uuid)) `, len(args)-1, len(args))
			where += ` AND EXISTS(SELECT 1 FROM visible v WHERE v.kind='` + string(t.Kind) + `' AND v.id=r.id)`
			projection += ` || jsonb_build_object('visibility',CASE WHEN EXISTS(SELECT 1 FROM visible v WHERE v.kind='` + string(t.Kind) + `' AND v.id=r.id AND v.full_access) THEN 'full' ELSE 'context' END)`
			if t.Kind == Component {
				children = `EXISTS(SELECT 1 FROM engineering_components c JOIN visible v ON v.kind='component' AND v.id=c.id WHERE c.tenant_id=r.tenant_id AND c.facility_id=r.facility_id AND c.parent_component_id=r.id AND c.deleted_at IS NULL)`
			}
		}
		rows, e := tx.Query(ctx, cte+`SELECT `+projection+`,`+children+` FROM `+tables[t.Kind]+` r WHERE `+where+` ORDER BY r.created_at,r.id LIMIT $4`, args...)
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
		for i := range out.Items {
			out.Items[i].Resource = sourceProjection(out.Items[i].Resource, auth)
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
func (s *Service) Changes(ctx context.Context, a access.Actor, scope Scope, target Target, f Filter) (Changes, error) {
	t := scoped(scope, target)
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
	e := s.transact(ctx, a, t.operation("facility", "changes"), t.Scope, false, func(tx pgx.Tx, auth authorization) error {
		t.CursorScope = fingerprint([]string{auth.Account, auth.TenantMembership, auth.ProjectAuth.MembershipID, auth.AccessRevision})
		c, e := readCursor(t, f, true)
		if e != nil {
			return e
		}
		if auth.Entry == "project" {
			if f.Deleted != "" {
				return access.ErrDenied
			}
			if t.Facility != "" {
				link, e := linkTx(ctx, tx, auth.Tenant, auth.Project, t.Facility)
				if e != nil {
					return e
				}
				if link == nil {
					return notFound()
				}
			}
		}

		facility, e := lockFacility(ctx, tx, t, false, true)
		if e != nil {
			return e
		}
		args := []any{t.Tenant, t.Facility, f.Kind, f.ID, c.ID, c.Time, f.Limit + 1}
		scopeWhere := ""
		if auth.Entry == "project" {
			args = append(args, auth.Project)
			scopeWhere = ` AND EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE g.tenant_id=c.tenant_id AND g.project_id=$8 AND g.facility_id=c.facility_id AND g.resource_kind=c.resource_kind AND COALESCE(g.component_id,g.point_id,g.channel_id,g.facility_id)=c.resource_id AND g.ended_at IS NULL)`
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(c) FROM engineering_resource_changes c WHERE tenant_id::text=$1 AND facility_id::text=$2 AND ($3='' OR resource_kind=$3) AND ($4='' OR resource_id::text=$4) AND ($5='' OR (changed_at,id)<($6::timestamptz,NULLIF($5,'')::uuid))`+scopeWhere+` ORDER BY changed_at DESC,id DESC LIMIT $7`, args...)
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
		if e != nil {
			return e
		}
		if auth.Entry == "project" {
			return projectHistory(ctx, tx, auth, t.Facility, out.Items)
		}
		for i := range out.Items {
			c := &out.Items[i]
			c.After = sourceProjection(c.After, auth)
			if c.Before != nil {
				r := sourceProjection(*c.Before, auth)
				c.Before = &r
			}
			if !auth.Governance {
				c.ActorProjectID = ""
				c.ActorLinkID = ""
				c.ActorProjectMembershipID = ""
			}
		}
		return nil
	})
	return out, e
}
