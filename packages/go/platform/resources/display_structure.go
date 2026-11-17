package resources

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
	"slices"
	"strconv"
	"strings"
)

type structureStatisticsProof struct {
	Items  []dr.ResourceSummary `json:"items"`
	Groups map[string][]string  `json:"groups"`
}

// Paths include self and use the canonical typed graph. Only fully qualified
// seeds are accepted; callers must project ancestor labels through their own
// resource view. This private seam never expands the public read population.
func (s *Service) displayStructurePathsTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, refs []dr.Ref) (map[string][]dr.Ref, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.ProjectResourceOperation("facility", "list"), tenant, project); e != nil {
		return nil, e
	}
	out := map[string][]dr.Ref{}
	if len(refs) > dr.MaxMembers {
		return nil, dr.Problem("budget_exceeded", "Structure membership exceeds the 50,000 member budget.", 429)
	}
	rows, e := tx.Query(ctx, `WITH RECURSIVE nodes AS MATERIALIZED(SELECT * FROM engineering_resource_nodes WHERE tenant_id=$1 AND deleted_at IS NULL), visible AS MATERIALIZED(SELECT * FROM project_resource_visibility($1,$2,NULL) WHERE full_access), seeds AS (SELECT n.* FROM jsonb_to_recordset($3::jsonb) r(kind text,id uuid) JOIN nodes n ON n.tenant_id=$1 AND n.kind=r.kind AND n.id=r.id JOIN visible v ON v.kind=n.kind AND v.id=n.id AND v.full_access WHERE n.deleted_at IS NULL), paths AS (
 SELECT kind AS seed_kind,id AS seed_id,kind,id,parent_kind,parent_id,0 AS depth FROM seeds
 UNION ALL SELECT p.seed_kind,p.seed_id,n.kind,n.id,n.parent_kind,n.parent_id,p.depth+1 FROM paths p JOIN nodes n ON n.tenant_id=$1 AND n.kind=p.parent_kind AND n.id=p.parent_id WHERE p.depth<514
 ) SELECT seed_kind,seed_id::text,jsonb_agg(jsonb_build_object('kind',kind,'id',id) ORDER BY depth),max(depth) FROM paths GROUP BY seed_kind,seed_id`, tenant, project, dr.Bytes(refs))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var seed dr.Ref
		var path []dr.Ref
		var depth int
		if e = rows.Scan(&seed.Kind, &seed.ID, &path, &depth); e != nil {
			return nil, e
		}
		if depth > 513 {
			return nil, dr.Problem("budget_exceeded", "Structure depth exceeds the supported budget.", 429)
		}
		out[dr.RefKey(seed)] = path
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	for _, ref := range refs {
		if _, ok := out[dr.RefKey(ref)]; !ok {
			return nil, dr.Hidden()
		}
	}
	return out, nil
}
func structureGroupRefs(path []dr.Ref, visible map[string]dr.ResourceSummary, kind, membership string) []dr.StructureGroupRef {
	groups := []dr.StructureGroupRef{}
	restricted := false
	for _, ref := range path {
		if ref.Kind != kind {
			continue
		}
		r, ok := visible[dr.RefKey(ref)]
		if ok && r.Visibility == "full" {
			v := ref
			groups = append(groups, dr.StructureGroupRef{Key: ref.ID, Label: r.Name, Visibility: "full", Ref: &v})
		} else if !restricted {
			restricted = true
			groups = append(groups, dr.StructureGroupRef{Key: "restricted_context", Label: "Restricted structure context", Visibility: "restricted"})
		}
		if membership == "nearest" {
			break
		}
	}
	if len(groups) == 0 {
		groups = append(groups, dr.StructureGroupRef{Key: "unassigned", Label: "Unassigned", Visibility: "unassigned"})
	}
	return groups
}

// DisplayStructureMembershipTx freezes grouping at metadata query preparation.
// Reauthorization later may reject delivery, but never recomputes these paths.
func (s *Service) DisplayStructureMembershipTx(ctx context.Context, tx pgx.Tx, p dr.Plan, resourceScope dr.Scope, refs []dr.Ref, kind, membership string) (map[string][]dr.StructureGroupRef, error) {
	if resourceScope.StructureModel != 2 {
		return nil, dr.StructureUpgradeRequired()
	}
	o := p.Authority.Owner
	a := p.Authority.Subject.ReadActor()
	items, e := s.DisplayDirectoryTx(ctx, tx, a, o.Tenant, o.Project, resourceScope, false)
	if e != nil {
		return nil, e
	}
	visible := map[string]dr.ResourceSummary{}
	for _, r := range items {
		visible[dr.RefKey(dr.Ref{Kind: r.Kind, ID: r.ID})] = r
	}
	paths, e := s.displayStructurePathsTx(ctx, tx, a, o.Tenant, o.Project, refs)
	if e != nil {
		return nil, e
	}
	groups := map[string][]dr.StructureGroupRef{}
	for key, path := range paths {
		groups[key] = structureGroupRefs(path, visible, kind, membership)
	}
	return groups, nil
}
func structureCounts(items []dr.ResourceSummary, channels bool) dr.StructureCounts {
	base := resourceCounts(items, channels)
	for _, kind := range []string{"monitoring_section", "borehole"} {
		if base.StatusCounts[kind] == nil {
			base.StatusCounts[kind] = map[string]string{"normal": "0", "retired": "0"}
		}
	}
	sections, holes := map[string]bool{}, map[string]bool{}
	types := map[string]bool{}
	for _, r := range items {
		if r.Kind == "monitoring_section" {
			sections[r.ID] = true
			if r.Type != nil {
				types[r.Type.DefinitionID] = true
			}
		}
		if r.Kind == "borehole" {
			holes[r.ID] = true
		}
	}
	return dr.StructureCounts{ResourceCounts: base, MonitoringSectionCount: strconv.Itoa(len(sections)), BoreholeCount: strconv.Itoa(len(holes)), SectionTypeCount: strconv.Itoa(len(types))}
}
func (s *Service) DisplayStructureStatisticsTx(ctx context.Context, tx pgx.Tx, p dr.Plan) (dr.Materialized, error) {
	q := p.Request.(*dr.StructureStatisticsRequest)
	scope := p.Scopes["target"]
	o := p.Authority.Owner
	a := p.Authority.Subject.ReadActor()
	out := dr.Materialized{Rows: []json.RawMessage{}}
	if scope.StructureModel != 2 {
		return out, dr.StructureUpgradeRequired()
	}
	all, e := s.DisplayDirectoryTx(ctx, tx, a, o.Tenant, o.Project, scope, false)
	if e != nil {
		return out, e
	}
	items := []dr.ResourceSummary{}
	visible := map[string]dr.ResourceSummary{}
	for _, r := range all {
		visible[dr.RefKey(dr.Ref{Kind: r.Kind, ID: r.ID})] = r
		if r.Kind != "channel" && matchesDisplayResource(r, q.Filters, "") {
			items = append(items, r)
		}
	}
	channelScope, hasChannels := p.Scopes["channel_target"]
	if hasChannels {
		if channelScope.StructureModel != 2 {
			return out, dr.StructureUpgradeRequired()
		}
		channels, e := s.DisplayDirectoryTx(ctx, tx, a, o.Tenant, o.Project, channelScope, false)
		if e != nil {
			return out, e
		}
		for _, c := range channels {
			if c.Kind == "channel" && matchesDisplayResource(c, q.Filters, "") {
				items = append(items, c)
			}
		}
	}
	refs := []dr.Ref{}
	for _, r := range items {
		refs = append(refs, dr.Ref{Kind: r.Kind, ID: r.ID})
	}
	paths, e := s.displayStructurePathsTx(ctx, tx, a, o.Tenant, o.Project, refs)
	if e != nil {
		return out, e
	}
	labels := slices.Contains(scope.FieldGroups, "resource.metadata")
	type bucket struct {
		ref                       dr.StructureGroupRef
		items                     []dr.ResourceSummary
		self, direct, descendants []dr.ResourceSummary
	}
	groups := map[string]*bucket{}
	proof := structureStatisticsProof{Items: items, Groups: map[string][]string{}}
	add := func(ref dr.StructureGroupRef) *bucket {
		b := groups[ref.Key]
		if b == nil {
			b = &bucket{ref: ref}
			groups[ref.Key] = b
		}
		return b
	}
	if dr.NewStructureKind(q.GroupBy) {
		for _, r := range all {
			if r.Kind == q.GroupBy && matchesDisplayResource(r, q.Filters, "") {
				ref := dr.Ref{Kind: r.Kind, ID: r.ID}
				g := dr.StructureGroupRef{Key: r.ID, Label: r.Name, Visibility: "full", Ref: &ref}
				if !labels {
					g = dr.StructureGroupRef{Key: "restricted_context", Label: "Restricted structure context", Visibility: "restricted"}
				}
				add(g)
			}
		}
	}
	for _, r := range items {
		if q.GroupBy == "none" {
			continue
		}
		path := paths[dr.RefKey(dr.Ref{Kind: r.Kind, ID: r.ID})]
		keys := []dr.StructureGroupRef{}
		if !labels {
			keys = []dr.StructureGroupRef{{Key: "restricted_context", Label: "Restricted structure context", Visibility: "restricted"}}
		} else if dr.NewStructureKind(q.GroupBy) {
			keys = structureGroupRefs(path, visible, q.GroupBy, q.Membership)
		} else {
			f, ok := visible[dr.RefKey(dr.Ref{Kind: "facility", ID: r.FacilityID})]
			if !ok || f.Visibility != "full" {
				keys = []dr.StructureGroupRef{{Key: "restricted_context", Label: "Restricted facility context", Visibility: "restricted"}}
			} else if q.GroupBy == "facility" {
				ref := dr.Ref{Kind: "facility", ID: f.ID}
				keys = []dr.StructureGroupRef{{Key: f.ID, Label: f.Name, Visibility: "full", Ref: &ref}}
			} else if f.Type != nil {
				ref := dr.Ref{Kind: "facility_type", ID: f.Type.DefinitionID}
				keys = []dr.StructureGroupRef{{Key: ref.ID, Label: f.Type.Name, Visibility: "full", Ref: &ref}}
			}
		}
		for _, key := range keys {
			add(key).items = append(add(key).items, r)
			refKey := dr.RefKey(dr.Ref{Kind: r.Kind, ID: r.ID})
			proof.Groups[refKey] = append(proof.Groups[refKey], key.Key)
		}
		// Self, immediate children, and all descendants are separate projections,
		// independent of the mutually-exclusive nearest population.
		if dr.NewStructureKind(q.GroupBy) && labels {
			for depth, ref := range path {
				if ref.Kind != q.GroupBy {
					continue
				}
				b := groups[ref.ID]
				if b == nil {
					continue
				}
				switch depth {
				case 0:
					b.self = append(b.self, r)
				case 1:
					b.direct = append(b.direct, r)
					b.descendants = append(b.descendants, r)
				default:
					b.descendants = append(b.descendants, r)
				}
			}
		}
	}
	applicability := map[string]map[string]string{}
	for _, kind := range []string{"monitoring_section", "borehole"} {
		applicability[kind] = map[string]string{"applicable": "0", "not_applicable": "0", "undeclared": "0"}
	}
	versions := map[string]engineering.Content{}
	for _, r := range items {
		if r.Kind != "facility" || r.Type == nil {
			continue
		}
		content, ok := versions[r.Type.VersionID]
		if !ok {
			if e = tx.QueryRow(ctx, `SELECT content FROM engineering_versions WHERE id=$1`, r.Type.VersionID).Scan(&content); e != nil {
				return out, e
			}
			versions[r.Type.VersionID] = content
		}
		for kind, counts := range applicability {
			state := engineering.StructureApplicability(content, kind)
			n, _ := strconv.Atoi(counts[state])
			counts[state] = strconv.Itoa(n + 1)
		}
	}
	keys := []string{}
	for key := range groups {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		b := groups[key]
		g := dr.StructureGroup{StructureGroupRef: b.ref, Totals: structureCounts(b.items, hasChannels)}
		if b.ref.Ref != nil && dr.NewStructureKind(q.GroupBy) {
			self, direct, desc := structureCounts(b.self, hasChannels), structureCounts(b.direct, hasChannels), structureCounts(b.descendants, hasChannels)
			g.Self = &self
			g.DirectChildren = &direct
			g.Descendants = &desc
		}
		out.Rows = append(out.Rows, dr.Bytes(g))
	}
	additivity := "disjoint"
	if q.Membership == "subtree" && dr.NewStructureKind(q.GroupBy) {
		additivity = "overlapping"
	}
	out.Metadata = map[string]any{"definitions_version": "2", "structure_model": 2, "population": "authorized_resources", "group_by": q.GroupBy, "membership": q.Membership, "additivity": additivity, "totals": structureCounts(items, hasChannels), "applicability": applicability, "context_excluded": true, "filters": q.Filters, "count_basis": map[string]string{"deduplicate_by": "kind_and_resource_id", "metadata": "fixed_structure_snapshot", "applicability": "fully_authorized_facilities_only"}}
	out.Proof = dr.Bytes(proof)
	if len(items) == 0 {
		out.State = "empty"
	}
	return out, nil
}

func (s *Service) DisplayStructureDrilldownTx(ctx context.Context, tx pgx.Tx, p dr.Plan, raw json.RawMessage, q *dr.DrilldownRequest) (dr.Scope, json.RawMessage, error) {
	if q.Basis != "resource_count" || q.Purpose != "resources" {
		return dr.Scope{}, nil, dr.Invalid("/basis", "结构下钻须选择资源数量和资源目录。")
	}
	var proof structureStatisticsProof
	if e := json.Unmarshal(raw, &proof); e != nil {
		return dr.Scope{}, nil, e
	}
	selected := structureStatisticsProof{Items: []dr.ResourceSummary{}, Groups: proof.Groups}
	scope := dr.Scope{Domain: "resources", StructureModel: 2, Members: []dr.Member{}}
	for _, r := range proof.Items {
		if q.GroupKey == "" || slices.Contains(proof.Groups[dr.RefKey(dr.Ref{Kind: r.Kind, ID: r.ID})], q.GroupKey) {
			selected.Items = append(selected.Items, r)
			scope.Members = append(scope.Members, dr.Member{Kind: r.Kind, ID: r.ID})
		}
	}
	scope.Hash = dr.Hash([]any{p.Query.ID, q, scope.Members})
	return scope, dr.Bytes(selected), nil
}

func (s *Service) DisplayStructureSelectionTx(ctx context.Context, tx pgx.Tx, p dr.Plan, raw json.RawMessage) (dr.Materialized, error) {
	out := dr.Materialized{Rows: []json.RawMessage{}, Metadata: map[string]any{"structure_model": 2, "structure_projection": "canonical", "population_basis": "fixed_parent_query"}, Proof: raw}
	var proof structureStatisticsProof
	if e := json.Unmarshal(raw, &proof); e != nil {
		return out, e
	}
	scope := p.Scopes["target"]
	for _, field := range scope.FieldGroups {
		if field != "resource.metadata" {
			return out, dr.Invalid("/field_groups", "结构下钻提供固定目录，详细属性请独立查询。")
		}
	}
	current, e := s.DisplayDirectoryTx(ctx, tx, p.Authority.Subject.ReadActor(), p.Authority.Owner.Tenant, p.Authority.Owner.Project, scope, false)
	if e != nil {
		return out, e
	}
	if len(current) != len(scope.Members) {
		return out, dr.Hidden()
	}
	allowed := map[string]bool{}
	for _, m := range scope.Members {
		allowed[dr.RefKey(dr.Ref{Kind: m.Kind, ID: m.ID})] = true
	}
	q := p.Request.(*dr.ResourcesRequest)
	items := []dr.ResourceSummary{}
	for _, r := range proof.Items {
		if allowed[dr.RefKey(dr.Ref{Kind: r.Kind, ID: r.ID})] && matchesDisplayResource(r, q.Filters, q.Search) {
			items = append(items, r)
		}
	}
	slices.SortFunc(items, func(a, b dr.ResourceSummary) int {
		var n int
		switch q.Sort {
		case "name":
			n = strings.Compare(a.Name, b.Name)
		case "created_at":
			if a.CreatedAt != nil && b.CreatedAt != nil {
				n = a.CreatedAt.Compare(*b.CreatedAt)
			}
		default:
			n = strings.Compare(a.Code, b.Code)
		}
		if n != 0 {
			return n
		}
		return strings.Compare(dr.RefKey(dr.Ref{Kind: a.Kind, ID: a.ID}), dr.RefKey(dr.Ref{Kind: b.Kind, ID: b.ID}))
	})
	for _, r := range items {
		out.Rows = append(out.Rows, dr.Bytes(r))
	}
	return out, nil
}
