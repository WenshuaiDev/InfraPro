package resources

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

func contextResource(r Resource) Resource {
	return Resource{ID: r.ID, Kind: r.Kind, Code: r.Code, Name: r.Name, CategoryKey: r.CategoryKey, ParentComponentID: r.ParentComponentID, ComponentID: r.ComponentID, PointID: r.PointID, FacilityID: r.FacilityID, Status: r.Status, Visibility: "context"}
}
func (r Resource) MarshalJSON() ([]byte, error) {
	if r.Visibility == "context" {
		return json.Marshal(ContextNode{ID: r.ID, Kind: r.Kind, Code: r.Code, Name: r.Name, CategoryKey: r.CategoryKey, ParentComponentID: r.ParentComponentID, ComponentID: r.ComponentID, PointID: r.PointID, FacilityID: r.FacilityID, Status: r.Status, Visibility: "context"})
	}
	type plain Resource
	return json.Marshal(plain(r))
}

// Embedded marshalers otherwise swallow the containing response's extra fields.
func mergeResource(r Resource, extra any) ([]byte, error) {
	var m, fields map[string]json.RawMessage
	if e := json.Unmarshal(jsonBytes(r), &m); e != nil {
		return nil, e
	}
	if e := json.Unmarshal(jsonBytes(extra), &fields); e != nil {
		return nil, e
	}
	for k, v := range fields {
		m[k] = v
	}
	return json.Marshal(m)
}
func (i Item) MarshalJSON() ([]byte, error) {
	var tenantChildren *bool
	if i.Visibility == "" {
		tenantChildren = &i.HasChildren
	}
	return mergeResource(i.Resource, struct {
		HasChildren        *bool  `json:"has_children,omitempty"`
		HasVisibleChildren bool   `json:"has_visible_children"`
		TypeName           string `json:"type_name,omitempty"`
		TypeNumber         int    `json:"type_number,omitempty"`
		TypeDefinitionID   string `json:"type_definition_id,omitempty"`
	}{tenantChildren, i.HasChildren, i.TypeName, i.TypeNumber, i.TypeDefinitionID})
}
func (d Detail) MarshalJSON() ([]byte, error) {
	type extra Detail
	v := extra(d)
	v.Resource = Resource{}
	// An explicit wrapper avoids invoking Resource.MarshalJSON through embedding.
	return mergeResource(d.Resource, struct {
		Ancestors      []Resource `json:"ancestors"`
		Definitions    any        `json:"definitions"`
		Actions        Actions    `json:"actions"`
		AccessRevision string     `json:"access_revision,omitempty"`
		LinkID         string     `json:"link_id,omitempty"`
		LinkRevision   string     `json:"link_revision,omitempty"`
		Shared         bool       `json:"shared"`
	}{v.Ancestors, v.Definitions, v.Actions, v.AccessRevision, v.LinkID, v.LinkRevision, v.Shared})
}
func projectResource(r Resource, auth authorization) Resource {
	if r.Visibility == "context" {
		return contextResource(r)
	}
	r.Visibility = "full"
	// IDs identifying people are private even when their public work is shared.
	r.CreatedBy = ""
	r.UpdatedBy = ""
	if ptrValue(r.CreatedProjectID) != auth.Project {
		r.CreatedProjectID = nil
	}
	return r
}
func sourceProjection(r Resource, auth authorization) Resource {
	if auth.Entry == "project" {
		return projectResource(r, auth)
	}
	if !auth.Governance {
		r.CreatedProjectID = nil
	}
	return r
}
func projectAncestors(ctx context.Context, tx pgx.Tx, auth authorization, facility string, rs []Resource) ([]Resource, error) {
	ids := []string{}
	for _, r := range rs {
		ids = append(ids, r.ID)
	}
	rows, e := tx.Query(ctx, `SELECT id::text,full_access FROM project_resource_visibility($1,$2,$3) WHERE id=ANY($4::uuid[])`, auth.Tenant, auth.Project, facility, ids)
	if e != nil {
		return nil, e
	}
	visible := map[string]bool{}
	for rows.Next() {
		var id string
		var full bool
		if e = rows.Scan(&id, &full); e != nil {
			rows.Close()
			return nil, e
		}
		visible[id] = full
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := []Resource{}
	for _, r := range rs {
		if full, ok := visible[r.ID]; ok {
			if !full {
				r = contextResource(r)
			}
			out = append(out, projectResource(r, auth))
		}
	}
	return out, nil
}
func projectHistory(ctx context.Context, tx pgx.Tx, auth authorization, facility string, changes []Change) error {
	ids := []string{}
	for _, c := range changes {
		for _, r := range []*Resource{c.Before, &c.After} {
			if r != nil {
				for _, id := range []string{ptrValue(r.ComponentID), ptrValue(r.ParentComponentID), r.PointID} {
					if id != "" {
						ids = append(ids, id)
					}
				}
			}
		}
	}
	rows, e := tx.Query(ctx, `SELECT id::text FROM project_resource_visibility($1,$2,$3) WHERE id=ANY($4::uuid[])`, auth.Tenant, auth.Project, facility, ids)
	if e != nil {
		return e
	}
	visible := map[string]bool{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		visible[id] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for i := range changes {
		c := &changes[i]
		if c.ActorProjectID != auth.Project {
			c.ActorID = ""
			c.ActorName = "其他授权范围更新"
			c.RequestID = ""
			c.Reason = "公共资源资料发生变更"
			c.ActorProjectID = ""
			c.ActorScope = "other"
		}
		c.ActorTenantMembershipID = ""
		c.ActorProjectMembershipID = ""
		c.ActorLinkID = ""
		for _, r := range []*Resource{c.Before, &c.After} {
			if r == nil {
				continue
			}
			*r = projectResource(*r, auth)
			if r.ComponentID != nil && !visible[*r.ComponentID] {
				r.ComponentID = nil
				r.HistoricalContextHidden = true
			}
			if r.ParentComponentID != nil && !visible[*r.ParentComponentID] {
				r.ParentComponentID = nil
				r.HistoricalContextHidden = true
			}
			if r.PointID != "" && !visible[r.PointID] {
				r.PointID = ""
				r.HistoricalContextHidden = true
			}
		}
	}
	return nil
}
