package resources

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func (s *Service) DisplayStatisticsTx(ctx context.Context, tx pgx.Tx, p displayread.Plan) (displayread.Materialized, error) {
	q := p.Request.(*displayread.ResourceStatisticsRequest)
	scope := p.Scopes["target"]
	o := p.Authority.Owner
	a := p.Authority.Subject.ReadActor()
	all, e := s.displayCountMembersTx(ctx, tx, a, o.Tenant, o.Project, scope)
	if e != nil {
		return displayread.Materialized{}, e
	}
	items := []displayread.ResourceSummary{}
	visible := map[string]displayread.ResourceSummary{}
	for _, v := range all {
		visible[displayread.RefKey(displayread.Ref{Kind: v.Kind, ID: v.ID})] = v
		if matchesDisplayResource(v, q.Filters, "") && v.Kind != "channel" {
			items = append(items, v)
		}
	}
	_, channelsProvided := p.Scopes["channel_target"]
	if channelsProvided {
		channels, e := s.displayCountMembersTx(ctx, tx, a, o.Tenant, o.Project, p.Scopes["channel_target"])
		if e != nil {
			return displayread.Materialized{}, e
		}
		for _, v := range channels {
			_, direct := visible[displayread.RefKey(displayread.Ref{Kind: v.Kind, ID: v.ID})]
			_, facility := visible[displayread.RefKey(displayread.Ref{Kind: "facility", ID: v.FacilityID})]
			owner := false
			if v.Owner != nil {
				_, owner = visible[displayread.RefKey(*v.Owner)]
			}
			if (direct || facility || owner) && matchesDisplayResource(v, q.Filters, "") {
				items = append(items, v)
			}
		}
	}
	labels := slices.Contains(scope.FieldGroups, "resource.metadata")
	type bucket struct {
		label      string
		ref        *displayread.Ref
		visibility string
		items      []displayread.ResourceSummary
	}
	groups := map[string]*bucket{}
	for _, v := range items {
		if q.GroupBy == "none" {
			continue
		}
		key, label, visibility := "restricted_context", "受限工程归属", "restricted"
		var ref *displayread.Ref
		if labels {
			if f, ok := visible[displayread.RefKey(displayread.Ref{Kind: "facility", ID: v.FacilityID})]; ok && f.Kind == "facility" {
				if q.GroupBy == "facility" {
					key, label, visibility = f.ID, f.Name, "full"
					ref = &displayread.Ref{Kind: "facility", ID: f.ID}
				} else if f.Type != nil {
					key, label, visibility = f.Type.DefinitionID, f.Type.Name, "full"
					ref = &displayread.Ref{Kind: "facility_type", ID: f.Type.DefinitionID}
				}
			}
		}
		g := groups[key]
		if g == nil {
			g = &bucket{label: label, ref: ref, visibility: visibility}
			groups[key] = g
		}
		g.items = append(g.items, v)
	}
	keys := []string{}
	for key := range groups {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	result := displayread.Materialized{Rows: []json.RawMessage{}, Metadata: map[string]any{"definitions_version": "1", "population": "authorized_resources", "group_by": q.GroupBy, "additivity": "disjoint", "filters": q.Filters, "context_excluded": true, "count_basis": map[string]string{"deduplicate_by": "resource_kind_and_id", "metadata": "current_read_snapshot"}, "totals": resourceCounts(items, channelsProvided)}}
	if len(items) == 0 {
		result.State = "empty"
	}
	for _, key := range keys {
		g := groups[key]
		result.Rows = append(result.Rows, displayread.Bytes(displayread.StatisticsGroup[displayread.ResourceCounts]{Key: key, Label: g.label, Visibility: g.visibility, Ref: g.ref, Totals: resourceCounts(g.items, channelsProvided)}))
	}
	return result, nil
}
func resourceCounts(items []displayread.ResourceSummary, channels bool) displayread.ResourceCounts {
	counts := map[string]int{}
	status := map[string]map[string]string{}
	seen := map[string]bool{}
	direct, derived := 0, 0
	for _, v := range items {
		key := displayread.RefKey(displayread.Ref{Kind: v.Kind, ID: v.ID})
		if seen[key] {
			continue
		}
		seen[key] = true
		counts[v.Kind]++
		if status[v.Kind] == nil {
			status[v.Kind] = map[string]string{"normal": "0", "retired": "0"}
		}
		n, _ := strconv.Atoi(status[v.Kind][v.Status])
		status[v.Kind][v.Status] = strconv.Itoa(n + 1)
		if v.Kind == "channel" {
			if v.SourceKind == "derived" {
				derived++
			} else {
				direct++
			}
		}
	}
	for _, kind := range []string{"facility", "component", "point"} {
		if status[kind] == nil {
			status[kind] = map[string]string{"normal": "0", "retired": "0"}
		}
	}
	out := displayread.ResourceCounts{FacilityCount: strconv.Itoa(counts["facility"]), ComponentCount: strconv.Itoa(counts["component"]), PointCount: strconv.Itoa(counts["point"]), ChannelsState: "unavailable", StatusCounts: status}
	if channels {
		count, d, r := strconv.Itoa(counts["channel"]), strconv.Itoa(direct), strconv.Itoa(derived)
		out.ChannelCount, out.DirectCount, out.DerivedCount = &count, &d, &r
		out.ChannelsState = "provided"
		if status["channel"] == nil {
			status["channel"] = map[string]string{"normal": "0", "retired": "0"}
		}
	}
	return out
}

// Count projections have no tree, positions, attributes or per-row child scans.
// The owning module independently intersects the narrowed IDs with full project
// visibility before any count or grouping is computed.
func (s *Service) displayCountMembersTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, scope displayread.Scope) ([]displayread.ResourceSummary, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.ProjectResourceOperation("facility", "list"), tenant, project); e != nil {
		return nil, e
	}
	table := "engineering_resource_nodes"
	if displayread.StructureVersion(scope.StructureModel) == 1 {
		var unsupported bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM engineering_channels c JOIN project_resource_visibility($1,$2,NULL) v ON v.kind='channel' AND v.id=c.id AND v.full_access WHERE c.tenant_id=$1 AND c.id=ANY($3::uuid[]) AND c.owner_kind IN ('monitoring_section','borehole') AND c.deleted_at IS NULL)`, tenant, project, scope.IDs("channel")).Scan(&unsupported); e != nil {
			return nil, e
		}
		if unsupported {
			return nil, displayread.StructureUpgradeRequired()
		}
		table = "engineering_resource_nodes_legacy"
	}
	rows, e := tx.Query(ctx, `WITH visible AS MATERIALIZED(SELECT kind,id FROM project_resource_visibility($1,$2,NULL) WHERE full_access), selected AS (SELECT kind,id FROM jsonb_to_recordset($3::jsonb) AS s(kind text,id uuid))
 SELECT n.id::text,n.kind,n.facility_id::text,n.code,n.name,n.status,COALESCE(c.source_kind,''),t.definition_id::text,t.id::text,t.content->>'name',t.number::text,COALESCE(n.parent_kind,''),COALESCE(n.parent_id::text,'')
 FROM `+table+` n JOIN visible v ON v.kind=n.kind AND v.id=n.id JOIN selected s ON s.kind=n.kind AND s.id=n.id JOIN engineering_facilities f ON f.tenant_id=n.tenant_id AND f.id=n.facility_id JOIN engineering_versions t ON t.id=f.type_version_id LEFT JOIN engineering_channels c ON c.tenant_id=n.tenant_id AND c.id=n.id AND n.kind='channel'
 WHERE n.tenant_id=$1 AND n.deleted_at IS NULL AND f.deleted_at IS NULL ORDER BY n.kind,n.id`, tenant, project, displayread.Bytes(scope.Members))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []displayread.ResourceSummary{}
	for rows.Next() {
		v := displayread.ResourceSummary{Visibility: "full"}
		var typ displayread.TypeRef
		var owner displayread.Ref
		if e = rows.Scan(&v.ID, &v.Kind, &v.FacilityID, &v.Code, &v.Name, &v.Status, &v.SourceKind, &typ.DefinitionID, &typ.VersionID, &typ.Name, &typ.Version, &owner.Kind, &owner.ID); e != nil {
			return nil, e
		}
		v.Type = &typ
		if v.Kind == "channel" {
			if displayread.StructureVersion(scope.StructureModel) == 1 && displayread.NewStructureKind(owner.Kind) {
				return nil, displayread.StructureUpgradeRequired()
			}
			v.Owner = &owner
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
