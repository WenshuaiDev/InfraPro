package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

// DisplayDirectoryTx uses the caller's already narrowed IDs, and independently
// rechecks project visibility. It never expands a selected parent into children.
func (s *Service) DisplayDirectoryTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, scope displayread.Scope, ancestors bool) ([]displayread.ResourceSummary, error) {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.ProjectResourceOperation("facility", "list"), tenant, project); e != nil {
		return nil, e
	}
	nodes := "engineering_resource_nodes"
	if scope.StructureModel != 2 {
		// Full current grants are checked before reporting an unrepresentable owner.
		var unsupported bool
		e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM engineering_channels c JOIN project_resource_visibility($1,$2,NULL) v ON v.kind='channel' AND v.id=c.id AND v.full_access WHERE c.tenant_id=$1 AND c.id=ANY($3::uuid[]) AND c.owner_kind IN ('monitoring_section','borehole') AND c.deleted_at IS NULL)`, tenant, project, scope.IDs("channel")).Scan(&unsupported)
		if e != nil {
			return nil, e
		}
		if unsupported {
			return nil, displayread.StructureUpgradeRequired()
		}
		nodes = "engineering_resource_nodes_legacy"
	}
	query := strings.ReplaceAll(`WITH RECURSIVE nodes AS MATERIALIZED(SELECT * FROM engineering_resource_nodes WHERE tenant_id=$1 AND deleted_at IS NULL), visible AS MATERIALIZED(SELECT * FROM project_resource_visibility($1,$2,NULL)),
 selected AS MATERIALIZED(SELECT n.* FROM nodes n JOIN visible v ON v.id=n.id AND v.kind=n.kind AND v.full_access JOIN jsonb_to_recordset($3::jsonb) m(kind text,id uuid) ON m.kind=n.kind AND m.id=n.id WHERE n.tenant_id=$1 AND n.deleted_at IS NULL),
 tree AS (SELECT * FROM selected UNION SELECT n.* FROM nodes n JOIN tree child ON child.parent_id=n.id AND child.parent_kind=n.kind WHERE $4 AND n.tenant_id=$1 AND n.deleted_at IS NULL),
 parents AS MATERIALIZED(SELECT DISTINCT parent_kind,parent_id FROM tree WHERE parent_id IS NOT NULL)
 SELECT jsonb_build_object('id',n.id,'kind',n.kind,'facility_id',n.facility_id,'parent_ref',CASE WHEN n.parent_id IS NULL THEN NULL ELSE jsonb_build_object('kind',n.parent_kind,'id',n.parent_id) END,
 'code',n.code,'name',n.name,'status',n.status,'revision',n.revision::text,'created_at',n.created_at,'visibility',CASE WHEN selected_node.id IS NOT NULL THEN 'full' ELSE 'context' END,
 'type_ref',jsonb_build_object('definition_id',t.definition_id,'version_id',t.id,'name',t.content->>'name','version',t.number::text),
 'has_visible_children',parent_node.parent_id IS NOT NULL,
 'source_kind',COALESCE(ch.source_kind,''),'metric_version_id',COALESCE(ch.metric_version_id::text,''),
 'metric_ref',CASE WHEN ch.id IS NULL THEN NULL ELSE jsonb_build_object('kind',CASE WHEN ch.source_kind='derived' THEN 'derived' ELSE 'engineering' END,'version_id',COALESCE(ch.derived_metric_version_id,ch.metric_version_id)) END,
	 'owner_ref',CASE WHEN ch.id IS NULL THEN NULL ELSE jsonb_build_object('kind',n.parent_kind,'id',n.parent_id) END),
 er.data||jsonb_build_object('facility_id',CASE WHEN n.kind='facility' THEN '' ELSE n.facility_id::text END), t.content
 FROM tree n JOIN engineering_resource_records er ON er.tenant_id=n.tenant_id AND er.kind=n.kind AND er.id=n.id LEFT JOIN selected selected_node ON selected_node.id=n.id AND selected_node.kind=n.kind LEFT JOIN parents parent_node ON parent_node.parent_id=n.id AND parent_node.parent_kind=n.kind
 JOIN engineering_facilities f ON f.tenant_id=n.tenant_id AND f.id=n.facility_id JOIN engineering_versions t ON t.id=f.type_version_id LEFT JOIN engineering_channels ch ON ch.tenant_id=n.tenant_id AND n.kind='channel' AND ch.id=n.id
 LEFT JOIN engineering_points ep ON ep.tenant_id=n.tenant_id AND n.kind='point' AND ep.id=n.id LEFT JOIN engineering_components ec ON ec.tenant_id=n.tenant_id AND n.kind='component' AND ec.id=n.id
 WHERE f.deleted_at IS NULL ORDER BY n.kind,n.code,n.id`, "engineering_resource_nodes", nodes)
	rows, e := tx.Query(ctx, query, tenant, project, displayread.Bytes(scope.Members), ancestors)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []displayread.ResourceSummary{}
	for rows.Next() {
		var raw, basis []byte
		var v displayread.ResourceSummary
		var content engineering.Content
		if e = rows.Scan(&raw, &basis, &content); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &v); e != nil {
			return nil, e
		}
		var r Resource
		if e = json.Unmarshal(basis, &r); e != nil {
			return nil, e
		}
		v.BasisHash = k.Hash([]any{r.FacilityID, r.ParentRef, r.Borehole, r.Position})
		if scope.StructureModel == 2 {
			v.StructureApplicability = map[string]string{"monitoring_section": engineering.StructureApplicability(content, "monitoring_section"), "borehole": engineering.StructureApplicability(content, "borehole")}
		}
		if v.Visibility == "context" {
			v = displayread.ResourceSummary{ID: v.ID, Kind: v.Kind, Code: v.Code, Name: v.Name, Visibility: "context", Parent: v.Parent, HasVisibleChildren: v.HasVisibleChildren}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func matchesDisplayResource(v displayread.ResourceSummary, f displayread.ResourceFilters, search string) bool {
	if v.Visibility != "full" {
		return false
	}
	if len(f.Kinds) > 0 && !slices.Contains(f.Kinds, v.Kind) || len(f.FacilityIDs) > 0 && !slices.Contains(f.FacilityIDs, v.FacilityID) || len(f.Statuses) > 0 && !slices.Contains(f.Statuses, v.Status) || len(f.TypeIDs) > 0 && (v.Type == nil || !slices.Contains(f.TypeIDs, v.Type.DefinitionID)) {
		return false
	}
	return search == "" || strings.Contains(strings.ToLower(v.Code), strings.ToLower(search)) || strings.Contains(strings.ToLower(v.Name), strings.ToLower(search))
}
func (s *Service) DisplayReadTx(ctx context.Context, tx pgx.Tx, p displayread.Plan) (displayread.Materialized, error) {
	scope := p.Scopes["target"]
	a := p.Authority.Subject.ReadActor()
	owner := p.Authority.Owner
	hierarchy := p.Method == "resources.hierarchy" || p.Method == "resources.tree" || p.Method == "resources.detail"
	items, e := s.DisplayDirectoryTx(ctx, tx, a, owner.Tenant, owner.Project, scope, hierarchy)
	if e != nil {
		return displayread.Materialized{}, e
	}
	result := displayread.Materialized{Rows: []json.RawMessage{}, Metadata: map[string]any{"structure_model": displayread.StructureVersion(scope.StructureModel), "structure_projection": map[bool]string{true: "canonical", false: "legacy"}[scope.StructureModel == 2]}}
	switch q := p.Request.(type) {
	case *displayread.ResourcesRequest:
		filtered := []displayread.ResourceSummary{}
		for _, v := range items {
			if matchesDisplayResource(v, q.Filters, q.Search) {
				filtered = append(filtered, v)
			}
		}
		if p.Method == "resources.types" {
			types := map[string]displayread.ResourceType{}
			for _, v := range filtered {
				if v.Kind != "facility" || v.Type == nil {
					continue
				}
				entry := types[v.Type.DefinitionID]
				entry.ID = v.Type.DefinitionID
				entry.Name = v.Type.Name
				if !slices.ContainsFunc(entry.Versions, func(r displayread.TypeRef) bool { return r.VersionID == v.Type.VersionID }) {
					entry.Versions = append(entry.Versions, *v.Type)
				}
				types[entry.ID] = entry
			}
			keys := []string{}
			for id := range types {
				keys = append(keys, id)
			}
			slices.Sort(keys)
			for _, id := range keys {
				result.Rows = append(result.Rows, displayread.Bytes(types[id]))
			}
			return result, nil
		}
		slices.SortStableFunc(filtered, func(a, b displayread.ResourceSummary) int {
			var n int
			switch q.Sort {
			case "name":
				n = strings.Compare(a.Name, b.Name)
			case "created_at":
				n = a.CreatedAt.Compare(*b.CreatedAt)
			case "code":
				n = strings.Compare(a.Code, b.Code)
			default:
				n = strings.Compare(a.Kind, b.Kind)
				if n == 0 {
					n = strings.Compare(a.Code, b.Code)
				}
			}
			if n == 0 {
				n = strings.Compare(a.ID, b.ID)
			}
			return n
		})
		for _, v := range filtered {
			result.Rows = append(result.Rows, displayread.Bytes(v))
		}
	case *displayread.ResourceTreeRequest:
		byID := map[string]displayread.ResourceSummary{}
		for _, v := range items {
			byID[displayread.RefKey(displayread.Ref{Kind: v.Kind, ID: v.ID})] = v
		}
		switch q.Mode {
		case "root":
			for _, v := range items {
				if v.Kind == "facility" {
					result.Rows = append(result.Rows, displayread.Bytes(v))
				}
			}
		case "children":
			parent, ok := byID[displayread.RefKey(*q.Parent)]
			if !ok || parent.Kind != q.Parent.Kind {
				return result, displayread.Hidden()
			}
			for _, v := range items {
				if v.Parent != nil && *v.Parent == *q.Parent {
					result.Rows = append(result.Rows, displayread.Bytes(v))
				}
			}
		case "path":
			node, ok := byID[displayread.RefKey(*q.Node)]
			if !ok || node.Kind != q.Node.Kind || node.Visibility != "full" {
				return result, displayread.Hidden()
			}
			path := []displayread.ResourceSummary{}
			for {
				path = append(path, node)
				if node.Parent == nil {
					break
				}
				node, ok = byID[displayread.RefKey(*node.Parent)]
				if !ok {
					break
				}
			}
			slices.Reverse(path)
			for _, v := range path {
				result.Rows = append(result.Rows, displayread.Bytes(v))
			}
		}
	case *displayread.ResourceDetailRequest:
		var selected displayread.ResourceSummary
		for _, v := range items {
			if v.ID == q.ID && (q.Kind == "" || v.Kind == q.Kind) && v.Visibility == "full" {
				if selected.ID != "" {
					return result, displayread.Invalid("/kind", "此身份对应多个资源种类，请明确指定 kind。")
				}
				selected = v
			}
		}
		if selected.ID == "" {
			return result, displayread.Hidden()
		}
		detail, e := s.displayDetailTx(ctx, tx, owner.Tenant, scope, selected, items)
		if e != nil {
			return result, e
		}
		result.Single = detail
	default:
		return result, displayread.Invalid("/method", "资源读取方法不匹配。")
	}
	return result, nil
}
func (s *Service) displayDetailTx(ctx context.Context, tx pgx.Tx, tenant string, scope displayread.Scope, summary displayread.ResourceSummary, items []displayread.ResourceSummary) (displayread.ResourceDetail, error) {
	out := displayread.ResourceDetail{StructureModel: displayread.StructureVersion(scope.StructureModel), StructureProjection: map[bool]string{true: "canonical", false: "legacy"}[scope.StructureModel == 2], Summary: summary, Ancestors: []displayread.ResourceSummary{}, Definitions: []displayread.DefinitionVersion{}}
	var raw []byte
	table := tables[Kind(summary.Kind)]
	if table == "" {
		return out, displayread.Hidden()
	}
	if e := tx.QueryRow(ctx, `SELECT to_jsonb(r) FROM `+table+` r WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, tenant, summary.ID).Scan(&raw); e != nil {
		return out, e
	}
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(raw, &fields); e != nil {
		return out, e
	}
	byID := map[string]displayread.ResourceSummary{}
	for _, v := range items {
		byID[displayread.RefKey(displayread.Ref{Kind: v.Kind, ID: v.ID})] = v
	}
	node := summary
	for node.Parent != nil {
		parent, ok := byID[displayread.RefKey(*node.Parent)]
		if !ok {
			break
		}
		out.Ancestors = append(out.Ancestors, parent)
		node = parent
	}
	slices.Reverse(out.Ancestors)
	if slices.Contains(scope.FieldGroups, "resource.attributes") {
		var description string
		_ = json.Unmarshal(fields["description"], &description)
		out.Description = &description
		if scope.StructureModel == 2 && displayread.NewStructureKind(summary.Kind) {
			var purpose string
			_ = json.Unmarshal(fields["purpose"], &purpose)
			out.Purpose = &purpose
		}
		if v := fields["values"]; len(v) > 0 {
			d := json.NewDecoder(bytes.NewReader(v))
			d.UseNumber()
			if e := d.Decode(&out.Values); e != nil {
				return out, e
			}
			out.Values = exactValues(out.Values).(map[string]any)
		}
	}
	if slices.Contains(scope.FieldGroups, "resource.position") && len(fields["position"]) > 0 {
		if e := json.Unmarshal(fields["position"], &out.Position); e != nil {
			return out, e
		}
	}
	if scope.StructureModel == 2 && slices.Contains(scope.FieldGroups, "resource.position") {
		if summary.Kind == "borehole" {
			if e := json.Unmarshal(fields["borehole"], &out.Borehole); e != nil {
				return out, e
			}
		}
		if summary.Kind == "point" {
			var hole *string
			if e := tx.QueryRow(ctx, `SELECT engineering_nearest_structure($1,$2,'point',$3,'borehole')::text`, tenant, summary.FacilityID, summary.ID).Scan(&hole); e != nil {
				return out, e
			}
			out.BoreholePositionState = "unassigned"
			if hole != nil {
				out.BoreholePositionState = "restricted"
				if visible, ok := byID[displayread.RefKey(displayread.Ref{Kind: "borehole", ID: *hole})]; ok && visible.Visibility == "full" {
					out.BoreholePositionState = "not_recorded"
					var raw []byte
					e := tx.QueryRow(ctx, `SELECT to_jsonb(p) FROM point_borehole_positions p WHERE tenant_id=$1 AND point_id=$2 AND borehole_id=$3 AND ended_at IS NULL`, tenant, summary.ID, *hole).Scan(&raw)
					if e != nil && e != pgx.ErrNoRows {
						return out, e
					}
					if e == nil {
						if e = json.Unmarshal(raw, &out.BoreholePosition); e != nil {
							return out, e
						}
						out.BoreholePositionState = "provided"
						var changed bool
						if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM engineering_resource_changes c WHERE c.tenant_id=$1 AND c.resource_kind='borehole' AND c.resource_id=$2 AND c.resource_revision>$3 AND c.before->'borehole' IS DISTINCT FROM c.after->'borehole')`, tenant, *hole, out.BoreholePosition.BoreholeRevision).Scan(&changed); e != nil {
							return out, e
						}
						if changed {
							out.BoreholePositionState = "basis_needs_review"
						}
					}
				}
			}
		}
	}
	_ = json.Unmarshal(fields["category_key"], &out.CategoryKey)
	ids := []string{summary.Type.VersionID}
	if summary.Metric != nil && summary.Metric.Kind == "engineering" {
		ids = append(ids, summary.Metric.VersionID)
	}
	var unit string
	_ = json.Unmarshal(fields["unit_version_id"], &unit)
	if unit != "" {
		ids = append(ids, unit)
	}
	rows, e := tx.Query(ctx, `SELECT v.id::text,v.definition_id::text,d.kind,v.number::text,v.content FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE v.id=ANY($1::uuid[]) ORDER BY v.id`, ids)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v displayread.DefinitionVersion
		if e = rows.Scan(&v.ID, &v.DefinitionID, &v.Kind, &v.Number, &v.Content); e != nil {
			rows.Close()
			return out, e
		}
		if v.ID == unit {
			copy := v
			out.Unit = &copy
		}
		out.Definitions = append(out.Definitions, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if summary.Metric != nil && summary.Metric.Kind == "derived" {
		var v displayread.DefinitionVersion
		e = tx.QueryRow(ctx, `SELECT id::text,definition_id::text,'derived',version_no::text,jsonb_build_object('name',name,'output',rule->'output') FROM derived_metric_versions WHERE tenant_id=$1 AND id=$2`, tenant, summary.Metric.VersionID).Scan(&v.ID, &v.DefinitionID, &v.Kind, &v.Number, &v.Content)
		if e != nil {
			return out, e
		}
		out.Definitions = append(out.Definitions, v)
	}
	for name, to := range map[string]*string{"observation_direction": &out.ObservationDirection, "reference_basis": &out.ReferenceBasis, "timestamp_meaning": &out.TimestampMeaning} {
		_ = json.Unmarshal(fields[name], to)
	}
	out.Sampling = fields["sampling"]
	out.QuantitySpec = fields["quantity_spec"]
	return out, nil
}
func exactValues(value any) any {
	switch v := value.(type) {
	case json.Number:
		return v.String()
	case map[string]any:
		for k, x := range v {
			v[k] = exactValues(x)
		}
	case []any:
		for i, x := range v {
			v[i] = exactValues(x)
		}
	}
	return value
}
