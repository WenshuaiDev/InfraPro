package resources

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
)

type ImpactProject struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}
type MutationPreview struct {
	Hash          string                   `json:"preview_hash"`
	Required      bool                     `json:"required"`
	CanSubmit     bool                     `json:"can_submit"`
	Shared        bool                     `json:"shared"`
	BlockedCode   string                   `json:"blocked_code,omitempty"`
	BlockedReason string                   `json:"blocked_reason,omitempty"`
	Changes       []engineering.Difference `json:"changes"`
	Projects      []ImpactProject          `json:"projects,omitempty"`
	Dependencies  map[string]int           `json:"dependencies,omitempty"`
}

func (s *Service) PreviewMutation(ctx context.Context, a access.Actor, scope Scope, target Target, action string, raw json.RawMessage) (MutationPreview, error) {
	if action == "create" {
		return MutationPreview{}, problem("invalid_request", "新增资源无需修改影响预览。", 400)
	}
	_, preview, e := s.mutate(ctx, a, scope, target, action, raw, access.Meta{}, true)
	return preview, e
}
func mutationImpact(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget, action string, m WriteMeta, in map[string]any, before *Resource, after Resource) (MutationPreview, error) {
	out := MutationPreview{CanSubmit: true, Changes: []engineering.Difference{}}
	if action == "delete" {
		after.DeletedAt = nil
	}
	if before == nil {
		return out, nil
	}
	shared, e := sharedTx(ctx, tx, auth, t)
	if e != nil {
		return out, e
	}
	out.Shared = shared
	structure := before.Kind == Component && ptrValue(before.ParentComponentID) != ptrValue(after.ParentComponentID) || before.Kind == Point && ptrValue(before.ComponentID) != ptrValue(after.ComponentID)
	changed := fingerprint(before) != fingerprint(after)
	out.Required = action != "update" || structure || shared && changed
	out.Changes = engineering.Differences(sourceProjection(*before, auth), sourceProjection(after, auth))
	// The full dependency set is hashed inside PostgreSQL, never sent to the project.
	var state string
	e = tx.QueryRow(ctx, descendantsSQL+` SELECT md5(jsonb_build_object(
 'resources',(SELECT jsonb_agg(to_jsonb(n) ORDER BY n.kind,n.id) FROM affected a JOIN engineering_resource_nodes n ON n.tenant_id=$1 AND n.kind=a.kind AND n.id=a.id),
 'references',(SELECT jsonb_agg(jsonb_build_array(v.project_id,v.link_id,v.kind,v.id,v.full_access,l.revision,p.revision,j.status,j.version) ORDER BY v.project_id,v.kind,v.id) FROM project_resource_visibility($1,NULL,$2) v JOIN affected a ON a.kind=v.kind AND a.id=v.id JOIN project_facility_links l ON l.id=v.link_id JOIN project_resource_policies p ON p.tenant_id=$1 AND p.project_id=v.project_id JOIN projects j ON j.id=v.project_id)
 )::text)`, auth.Tenant, t.Facility, t.Kind, t.ID).Scan(&state)
	if e != nil {
		return out, e
	}
	m.PreviewHash = ""
	out.Hash = fingerprint(struct {
		Scope                                                                 Scope
		Account, Member, ProjectMember, AccessRevision, ProjectVersion, State string
		Target                                                                Target
		Action                                                                string
		Meta                                                                  WriteMeta
		Input                                                                 map[string]any
		Before                                                                *Resource
		After                                                                 Resource
	}{auth.Scope, auth.Account, auth.TenantMembership, auth.ProjectAuth.MembershipID, auth.AccessRevision, auth.ProjectVersion, state, t.Target, action, m, in, before, after})
	if auth.Entry == "tenant" && auth.Governance {
		rows, e := tx.Query(ctx, `SELECT DISTINCT p.id::text,p.name,p.status FROM project_resource_visibility($1,NULL,$2) v JOIN projects p ON p.id=v.project_id WHERE v.kind=$3 AND v.id=$4 ORDER BY p.id::text LIMIT 101`, auth.Tenant, t.Facility, t.Kind, t.ID)
		if e != nil {
			return out, e
		}
		out.Projects = []ImpactProject{}
		for rows.Next() {
			v := ImpactProject{}
			if e = rows.Scan(&v.ID, &v.Name, &v.Status); e != nil {
				rows.Close()
				return out, e
			}
			out.Projects = append(out.Projects, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return out, e
		}
		// Names are informative, the server's complete dependency hash remains authoritative.
		if len(out.Projects) > 100 {
			out.Projects = out.Projects[:100]
		}
	}
	return out, nil
}
func previewBlock(err error, preview *MutationPreview, dryRun bool) error {
	var e *Error
	if dryRun && errors.As(err, &e) && (e.Code == "resource_referenced" || e.Code == "shared_resource_tenant_action_required") {
		preview.CanSubmit = false
		preview.BlockedCode = e.Code
		preview.BlockedReason = e.Message
		preview.Dependencies = e.Dependencies
		return nil
	}
	return err
}
