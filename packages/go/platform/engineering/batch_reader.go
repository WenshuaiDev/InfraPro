package engineering

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"slices"
)

// ResolvePublicBatchTx resolves roots and their complete fixed dependency closure
// in one query under the caller's catalog lock. It authorizes no business object.
// Only immutable public definitions enter the result; missing roots fail closed.
func ResolvePublicBatchTx(ctx context.Context, tx pgx.Tx, ids []string, newUse bool) (map[string]Version, error) {
	out := map[string]Version{}
	if len(ids) == 0 {
		return out, nil
	}
	for _, id := range ids {
		if !access.ValidID(id) {
			return nil, access.ErrNotFound
		}
	}
	rows, e := tx.Query(ctx, `WITH RECURSIVE selected(id) AS(SELECT unnest($1::uuid[]) UNION SELECT dep.target_id FROM engineering_dependencies dep JOIN selected s ON s.id=dep.version_id)
 SELECT to_jsonb(v)||jsonb_build_object('kind',d.kind,'code',d.code,'enabled',s.enabled,'state_revision',s.revision,'state_reason',s.reason,'dependencies',COALESCE((SELECT jsonb_agg(DISTINCT target_id ORDER BY target_id) FROM engineering_dependencies WHERE version_id=v.id),'[]'::jsonb))
 FROM engineering_versions v JOIN selected x ON x.id=v.id JOIN engineering_definitions d ON d.id=v.definition_id JOIN engineering_version_states s ON s.version_id=v.id WHERE d.tenant_id IS NULL AND d.kind IN ('type-definitions','metrics','units') AND d.deleted_at IS NULL`, ids)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var raw []byte
		var v Version
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return nil, e
		}
		if e = json.Unmarshal(raw, &v); e != nil {
			rows.Close()
			return nil, e
		}
		v.Issues = []Issue{}
		v.Usable = v.Enabled
		out[v.ID] = v
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	done, visiting := map[string]bool{}, map[string]bool{}
	var check func(string) error
	check = func(id string) error {
		if done[id] {
			return nil
		}
		v, ok := out[id]
		if !ok {
			return access.ErrNotFound
		}
		if visiting[id] {
			return problem("definition_unavailable", "固定依赖包含循环。", 409)
		}
		visiting[id] = true
		if v.SchemaVersion != MetamodelVersion {
			return problem("definition_unavailable", "不支持该定义格式。", 409)
		}
		if !v.Enabled {
			add(&v.Issues, "version_disabled", "版本已停用，不能新采用。", "/versions/"+id, id)
		}
		for _, dep := range v.Dependencies {
			if e := check(dep); e != nil {
				return e
			}
			if !out[dep].Usable {
				v.Usable = false
				add(&v.Issues, "dependency_unavailable", "固定依赖已停用，不能新采用。", "/dependencies/"+dep, dep)
			}
		}
		out[id] = v
		done[id] = true
		return nil
	}
	for _, id := range ids {
		if e = check(id); e != nil {
			return nil, e
		}
		if newUse && !out[id].Usable {
			return nil, problem("definition_unavailable", "所选定义或其固定依赖已不可新采用。", 409)
		}
	}
	return out, nil
}

// PublicDependencySet returns only this root set's closure from a batched map.
func PublicDependencySet(all map[string]Version, ids []string) []Version {
	seen := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if seen[id] {
			return
		}
		v, ok := all[id]
		if !ok {
			return
		}
		seen[id] = true
		for _, dep := range v.Dependencies {
			visit(dep)
		}
	}
	for _, id := range ids {
		visit(id)
	}
	keys := make([]string, 0, len(seen))
	for id := range seen {
		keys = append(keys, id)
	}
	slices.Sort(keys)
	out := []Version{}
	for _, id := range keys {
		out = append(out, all[id])
	}
	return out
}
