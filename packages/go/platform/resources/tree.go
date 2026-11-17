package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// TreeRef contains only navigation data. Device source details have separate permissions.
type TreeRef struct {
	ID         string     `json:"id"`
	Kind       Kind       `json:"kind"`
	FacilityID string     `json:"facility_id"`
	ParentKind *Kind      `json:"parent_kind"`
	ParentID   *string    `json:"parent_id"`
	Name       string     `json:"name"`
	Code       string     `json:"code"`
	Status     string     `json:"status"`
	DeletedAt  *time.Time `json:"deleted_at"`
}
type TreeNode struct {
	TreeRef
	CreatedAt        time.Time `json:"created_at"`
	HasChildren      bool      `json:"has_children"`
	Ancestors        []TreeRef `json:"ancestors"`
	TypeDefinitionID string    `json:"type_definition_id"`
	TypeName         string    `json:"type_name"`
	TypeNumber       int       `json:"type_number"`
	MetricName       string    `json:"metric_name"`
	UnitSymbol       string    `json:"unit_symbol"`
}
type TreeType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type TreePage struct {
	Items      []TreeNode   `json:"items"`
	NextCursor string       `json:"next_cursor"`
	Total      int          `json:"total"`
	Counts     map[Kind]int `json:"counts"`
	Types      []TreeType   `json:"types"`
}
type TreeFilter struct {
	Filter
	FacilityID string `json:"facility_id"`
	ParentKind Kind   `json:"parent_kind"`
	ParentID   string `json:"parent_id"`
	NodeID     string `json:"node_id"`
}
type treeCursor struct {
	Scope string    `json:"scope"`
	Kind  Kind      `json:"kind"`
	Time  time.Time `json:"time"`
	ID    string    `json:"id"`
}

// Tree preserves the tenant entry point used by existing callers.
func (s *Service) Tree(ctx context.Context, a access.Actor, tenant string, f TreeFilter) (TreePage, error) {
	return s.ScopedTree(ctx, a, TenantScope(tenant), f)
}

// ScopedTree rechecks membership and visibility for every page, search and path lookup.
func (s *Service) ScopedTree(ctx context.Context, a access.Actor, scope Scope, f TreeFilter) (TreePage, error) {
	out := TreePage{Items: []TreeNode{}, Types: []TreeType{}, Counts: map[Kind]int{Facility: 0, Component: 0, Point: 0, Channel: 0}}
	if e := normalizeFilter(&f.Filter); e != nil {
		return out, e
	}
	if f.NodeID != "" {
		if !access.ValidID(f.NodeID) || !access.ValidID(f.FacilityID) || f.ParentKind != "" || f.ParentID != "" || f.Deleted != "" {
			return out, problem("invalid_request", "资源路径范围无效。", 400)
		}
	} else if f.ParentKind == "" {
		if f.ParentID != "" || f.FacilityID != "" {
			return out, problem("invalid_request", "请选择完整的父对象范围。", 400)
		}
	} else if !slices.Contains([]Kind{Facility, Component, MonitoringSection, Borehole, Point}, f.ParentKind) || !access.ValidID(f.ParentID) || !access.ValidID(f.FacilityID) || f.Deleted != "" || f.ParentKind == Facility && f.ParentID != f.FacilityID {
		return out, problem("invalid_request", "父对象范围无效。", 400)
	}
	e := s.transact(ctx, a, scope.operation("tree", "list"), scope, false, func(tx pgx.Tx, auth authorization) error {
		if auth.Entry == "project" && f.Deleted != "" {
			return access.ErrDenied
		}
		if f.ParentKind != "" {
			t := scoped(auth.Scope, Target{Kind: f.ParentKind, ID: f.ParentID, Facility: f.FacilityID})
			if auth.Entry == "project" {
				if _, _, e := projectGrantTx(ctx, tx, auth, t, false); e != nil {
					return e
				}
			}
			if _, e := lockFacility(ctx, tx, t, false, false); e != nil {
				return e
			}
			if _, e := load(ctx, tx, t, false); e != nil {
				return e
			}
		}
		binding := f
		binding.Cursor, binding.Limit = "", 0
		stamp := fingerprint([]any{auth.Scope, auth.Account, auth.TenantMembership, auth.ProjectAuth.MembershipID, auth.AccessRevision, binding})
		c := treeCursor{}
		if f.Cursor != "" {
			b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
			if e != nil || strict(b, &c) != nil || c.Scope != stamp || !access.ValidID(c.ID) || tables[c.Kind] == "" || c.Time.IsZero() {
				return problem("invalid_request", "分页游标无效或不属于当前查询。", 400)
			}
		}
		var raw []byte
		if e := tx.QueryRow(ctx, treeSQL, scope.Tenant, f.FacilityID, f.ParentKind, f.ParentID, f.Query, f.Deleted, f.Status, f.TypeDefinition, c.Kind, c.Time, c.ID, f.Limit+1, scope.Project, f.NodeID).Scan(&raw); e != nil {
			return e
		}
		if e := json.Unmarshal(raw, &out); e != nil {
			return e
		}
		if len(out.Items) > f.Limit {
			last := out.Items[f.Limit-1]
			out.NextCursor = base64.RawURLEncoding.EncodeToString(jsonBytes(treeCursor{stamp, last.Kind, last.CreatedAt, last.ID}))
			out.Items = out.Items[:f.Limit]
		}
		return nil
	})
	return out, e
}

// One statement gives counts, result identities and ancestor paths one snapshot.
// The resource-node view preserves table indexes and never loads a full tree into the client.
const treeSQL = `WITH RECURSIVE visible AS MATERIALIZED (
 SELECT * FROM project_resource_visibility($1::uuid,NULLIF($13,'')::uuid,NULLIF($2,'')::uuid) WHERE $13<>''
), nodes AS NOT MATERIALIZED (
 SELECT n.*,($13='' OR COALESCE(p.full_access,false)) AS full_access FROM engineering_resource_nodes n
 LEFT JOIN visible p ON p.kind=n.kind AND p.id=n.id
 WHERE n.tenant_id=$1::uuid AND ($2='' OR n.facility_id=NULLIF($2,'')::uuid) AND ($13='' OR p.id IS NOT NULL)
), candidates AS MATERIALIZED (
 SELECT n.*,v.definition_id,v.content->>'name' AS type_name,v.number AS type_number,
 COALESCE(m.content->>'name','') AS metric_name,COALESCE(u.content->'unit'->>'symbol','') AS unit_symbol
 FROM nodes n
 JOIN engineering_facilities f ON f.tenant_id=n.tenant_id AND f.id=n.facility_id
 JOIN engineering_versions v ON v.id=f.type_version_id
 LEFT JOIN engineering_channels ch ON n.kind='channel' AND ch.tenant_id=n.tenant_id AND ch.id=n.id
 LEFT JOIN engineering_versions m ON m.id=ch.metric_version_id
 LEFT JOIN engineering_versions u ON u.id=ch.unit_version_id
 WHERE n.tenant_id=$1::uuid AND ($2='' OR n.facility_id=NULLIF($2,'')::uuid)
 AND (($14<>'' AND n.id=NULLIF($14,'')::uuid) OR ($14='' AND (($3='' AND ($5<>'' AND $6='' OR n.kind='facility')) OR (n.parent_kind=$3 AND n.parent_id=NULLIF($4,'')::uuid))))
 AND (($6='only' AND n.kind='facility' AND n.deleted_at IS NOT NULL) OR ($6='' AND n.deleted_at IS NULL AND f.deleted_at IS NULL))
 AND ($7='all' OR n.status=$7) AND ($8='' OR v.definition_id=NULLIF($8,'')::uuid)
 AND ($5='' OR strpos(lower(n.name),lower($5))>0 OR strpos(lower(n.code),lower($5))>0)
), page AS (
 SELECT * FROM candidates n WHERE ($11='' OR (n.kind,n.created_at,n.id)>($9,$10::timestamptz,NULLIF($11,'')::uuid)) ORDER BY n.kind,n.created_at,n.id LIMIT $12
), paths AS (
 SELECT n.kind AS leaf_kind,n.id AS leaf_id,p.*,1 AS depth FROM page n
 JOIN nodes p ON p.facility_id=n.facility_id AND p.kind=n.parent_kind AND p.id=n.parent_id AND p.deleted_at IS NULL
 UNION ALL SELECT a.leaf_kind,a.leaf_id,p.*,a.depth+1 FROM paths a
 JOIN nodes p ON p.facility_id=a.facility_id AND p.kind=a.parent_kind AND p.id=a.parent_id AND p.deleted_at IS NULL
)
SELECT jsonb_build_object(
 'items',COALESCE((SELECT jsonb_agg(jsonb_build_object(
 'id',n.id,'kind',n.kind,'facility_id',n.facility_id,'parent_kind',n.parent_kind,'parent_id',n.parent_id,'name',n.name,'code',n.code,'status',n.status,'deleted_at',n.deleted_at,'created_at',n.created_at,
 'type_definition_id',CASE WHEN n.deleted_at IS NULL AND n.full_access THEN n.definition_id::text ELSE '' END,'type_name',CASE WHEN n.deleted_at IS NULL AND n.full_access THEN n.type_name ELSE '' END,'type_number',CASE WHEN n.deleted_at IS NULL AND n.full_access THEN n.type_number ELSE 0 END,'metric_name',n.metric_name,'unit_symbol',n.unit_symbol,
 'has_children',n.deleted_at IS NULL AND EXISTS(SELECT 1 FROM nodes child WHERE child.facility_id=n.facility_id AND child.parent_kind=n.kind AND child.parent_id=n.id AND child.deleted_at IS NULL),
 'ancestors',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',p.id,'kind',p.kind,'facility_id',p.facility_id,'parent_kind',p.parent_kind,'parent_id',p.parent_id,'name',p.name,'code',p.code,'status',p.status,'deleted_at',p.deleted_at) ORDER BY p.depth DESC) FROM paths p WHERE p.leaf_kind=n.kind AND p.leaf_id=n.id),'[]'::jsonb)
 ) ORDER BY n.kind,n.created_at,n.id) FROM page n),'[]'::jsonb),
 'next_cursor','',
 'total',(SELECT count(*) FROM candidates),
 'counts',(SELECT jsonb_build_object('facility',count(*) FILTER(WHERE kind='facility'),'component',count(*) FILTER(WHERE kind='component'),'monitoring_section',count(*) FILTER(WHERE kind='monitoring_section'),'borehole',count(*) FILTER(WHERE kind='borehole'),'point',count(*) FILTER(WHERE kind='point'),'channel',count(*) FILTER(WHERE kind='channel')) FROM candidates),
 'types',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',definition_id,'name',name) ORDER BY name,definition_id) FROM (
 SELECT DISTINCT ON(v.definition_id) v.definition_id,v.content->>'name' AS name FROM engineering_facilities f JOIN engineering_versions v ON v.id=f.type_version_id WHERE f.tenant_id=$1::uuid AND f.deleted_at IS NULL AND ($13='' OR EXISTS(SELECT 1 FROM visible p WHERE p.kind='facility' AND p.id=f.id AND p.full_access)) ORDER BY v.definition_id,v.number DESC
 ) types),'[]'::jsonb)
)`
