package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
)

type structureGraph map[string]Resource

func refKey(k Kind, id string) string     { return string(k) + ":" + id }
func resourceRef(r Resource) StructureRef { return StructureRef{r.Kind, r.ID} }
func (g structureGraph) ancestors(r Resource) ([]Resource, error) {
	out := []Resource{}
	seen := map[string]bool{refKey(r.Kind, r.ID): true}
	p := r.ParentRef
	budget := MaxStructureDepth
	if r.Kind == Channel {
		budget++
	}
	for p != nil {
		key := refKey(p.Kind, p.ID)
		if seen[key] {
			return nil, invalid("/parent_ref", "cycle", "工程结构不能包含循环。")
		}
		seen[key] = true
		n, ok := g[key]
		if !ok {
			return nil, notFound()
		}
		out = append(out, n)
		if len(out) > budget {
			return nil, problem("structure_budget_exceeded", "工程树最多 512 层，请调整结构。", 422)
		}
		p = n.ParentRef
	}
	return out, nil
}
func (g structureGraph) nearest(r Resource, kind Kind, includeSelf bool) *StructureRef {
	if includeSelf && r.Kind == kind {
		p := resourceRef(r)
		return &p
	}
	ancestors, e := g.ancestors(r)
	if e != nil {
		return nil
	}
	for _, v := range ancestors {
		if v.Kind == kind {
			p := resourceRef(v)
			return &p
		}
	}
	return nil
}
func (g structureGraph) subtree(root StructureRef) []Resource {
	children := map[string][]Resource{}
	for _, v := range g {
		if v.ParentRef != nil {
			key := refKey(v.ParentRef.Kind, v.ParentRef.ID)
			children[key] = append(children[key], v)
		}
	}
	out := []Resource{}
	queue := []string{refKey(root.Kind, root.ID)}
	seen := map[string]bool{}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if seen[key] {
			continue
		}
		seen[key] = true
		v, ok := g[key]
		if !ok {
			continue
		}
		out = append(out, v)
		for _, child := range children[key] {
			queue = append(queue, refKey(child.Kind, child.ID))
		}
	}
	slices.SortFunc(out, func(a, b Resource) int {
		if refKey(a.Kind, a.ID) < refKey(b.Kind, b.ID) {
			return -1
		}
		if refKey(a.Kind, a.ID) > refKey(b.Kind, b.ID) {
			return 1
		}
		return 0
	})
	return out
}
func loadStructureGraph(ctx context.Context, tx pgx.Tx, tenant, facility string, retained bool) (structureGraph, error) {
	out := structureGraph{}
	rows, e := tx.Query(ctx, `SELECT data FROM engineering_resource_records WHERE tenant_id=$1 AND facility_id=$2 AND ($3 OR data->>'deleted_at' IS NULL) ORDER BY kind,id LIMIT 100001`, tenant, facility, retained)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var r Resource
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &r); e != nil {
			return nil, e
		}
		out[refKey(r.Kind, r.ID)] = r
	}
	if len(out) > MaxStructureNodes {
		return nil, problem("structure_budget_exceeded", "单设施对象超过 100,000 个处理预算，不能截断执行。", 422)
	}
	return out, rows.Err()
}
func (g structureGraph) validate(c engineering.Content) error {
	counts := map[string]int{}
	for _, r := range g {
		if r.Kind == Channel {
			continue
		}
		ancestors, e := g.ancestors(r)
		if e != nil {
			return e
		}
		if r.Kind != Facility {
			if len(ancestors) == 0 || ancestors[len(ancestors)-1].Kind != Facility || ancestors[len(ancestors)-1].ID != r.FacilityID {
				return invalid("/parent_ref", "facility_boundary", "主父级必须属于同一设施。")
			}
			if !isStructureContainer(ancestors[0].Kind) {
				return invalid("/parent_ref", "point_not_container", "测点不作为结构容器。")
			}
		}
		fields, ok := engineering.StructureFields(c, string(r.Kind), r.CategoryKey)
		if !ok {
			return invalid("/type_version_id", "structure_not_supported", "类型不支持既有节点或构件类别。")
		}
		if r.Kind != Point {
			if e = valuesValid(fields, r.Values); e != nil {
				return e
			}
		}
		if r.Kind != Component {
			continue
		}
		parentID, parentCat := "root", "root"
		for _, p := range ancestors {
			if p.Kind == Component {
				parentID, parentCat = p.ID, p.CategoryKey
				break
			}
		}
		var found *engineering.Relation
		for _, relation := range c.Relations {
			if relation.Parent == parentCat && relation.Child == r.CategoryKey {
				v := relation
				found = &v
				break
			}
		}
		if found == nil {
			return invalid("/parent_ref", "component_relation", "子树内构件的最近构件类别关系不被类型允许。")
		}
		key := parentID + ":" + r.CategoryKey
		if r.Status == "normal" && r.DeletedAt == nil {
			counts[key]++
			if found.MaxCount != nil && counts[key] > *found.MaxCount {
				return invalid("/parent_ref", "component_count", fmt.Sprintf("最近构件上级的 %s 类别超过 %d 个上限。", r.CategoryKey, *found.MaxCount))
			}
		}
	}
	return nil
}
