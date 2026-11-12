package resources

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"slices"
)

func projectGrantTx(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget, full bool) (Grant, *Link, error) {
	link, e := linkTx(ctx, tx, auth.Tenant, auth.Project, t.Facility)
	if e != nil {
		return Grant{}, nil, e
	}
	if link == nil {
		return Grant{}, nil, notFound()
	}
	g, e := scanGrant(tx.QueryRow(ctx, grantSelect+` WHERE g.link_id=$1 AND g.resource_kind=$2 AND n.id=$3 AND g.ended_at IS NULL AND n.deleted_at IS NULL`, link.ID, t.Kind, t.ID))
	if e == nil {
		return g, link, nil
	}
	if e != pgx.ErrNoRows {
		return g, nil, e
	}
	if !full {
		var exists bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_resource_visibility($1,$2,$3) WHERE kind=$4 AND id=$5)`, auth.Tenant, auth.Project, t.Facility, t.Kind, t.ID).Scan(&exists)
		if e != nil {
			return g, nil, e
		}
		if exists {
			return Grant{Kind: t.Kind, ResourceID: t.ID, Capabilities: []string{}}, link, nil
		}
	}
	return g, nil, notFound()
}
func (a authorization) can(kind Kind, action string) bool {
	if a.Entry != "project" {
		return false
	}
	return slices.Contains(a.ProjectAuth.AllowedOperations, a.operation(string(kind), action))
}
func projectActions(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget, r Resource, ancestors []Resource, g Grant) (Actions, error) {
	out := actionAllowed(r, ancestors, true)
	out.Read = true
	out.Delete = false
	if auth.ProjectStatus == "archived" {
		return Actions{Read: true, Reason: "项目已归档；显示共享资源当前资料。"}, nil
	}
	has := func(cap string) bool { return slices.Contains(g.Capabilities, cap) }
	out.Edit = out.Edit && has("update") && auth.can(r.Kind, "update")
	out.CreateComponent = out.Add && has("create_component") && auth.can(Component, "create")
	out.CreatePoint = out.Add && has("create_point") && auth.can(Point, "create")
	out.CreateChannel = out.Add && has("create_channel") && auth.can(Channel, "create")
	out.Add = out.CreateComponent || out.CreatePoint || out.CreateChannel
	out.Retire = out.Retire && r.Kind != Facility && has("lifecycle") && auth.can(r.Kind, "retire")
	out.Restore = out.Restore && r.Kind != Facility && has("lifecycle") && auth.can(r.Kind, "restore")
	if out.Retire || out.Restore {
		ok, e := projectLifecycleAllowed(ctx, tx, auth, t)
		if e != nil {
			return out, e
		}
		if !ok {
			out.Retire = false
			out.Restore = false
			out.Reason = "退役或恢复涉及共享资源或未获准下级，请由租户统一处理。"
		}
	}
	if !out.Edit && !out.Add && !out.Retire && !out.Restore && out.Reason == "" {
		out.Reason = "当前角色与资源授权仅允许查看。"
	}
	return out, nil
}
func validateProjectWrite(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget, action string, m WriteMeta, r Resource, before *Resource) (*Link, error) {
	if auth.Entry != "project" {
		return nil, nil
	}
	if m.AccessRevision != auth.AccessRevision {
		return nil, accessChanged()
	}
	if action == "delete" || t.Kind == Facility && (action == "retire" || action == "restore") {
		return nil, access.ErrDenied
	}
	if action == "create" && t.Kind == Facility {
		if m.LinkRevision != "" {
			return nil, invalid("/expected_link_revision", "not_allowed", "新设施尚无关联修订。")
		}
		if !auth.AllowFacilityCreate {
			return nil, access.ErrDenied
		}
		return nil, nil
	}
	target := t
	capability := "update"
	switch action {
	case "create":
		switch t.Kind {
		case Component:
			capability = "create_component"
			target.Kind = Component
			target.ID = ptrValue(r.ParentComponentID)
		case Point:
			capability = "create_point"
			target.Kind = Component
			target.ID = ptrValue(r.ComponentID)
		case Channel:
			capability = "create_channel"
			target.Kind = Point
			target.ID = t.Point
		}
		if target.ID == "" {
			target.Kind = Facility
			target.ID = t.Facility
		}
	case "retire", "restore":
		capability = "lifecycle"
	}
	g, link, e := projectGrantTx(ctx, tx, auth, target, true)
	if e != nil {
		return nil, e
	}
	if m.LinkRevision != link.Revision {
		return nil, accessChanged()
	}
	if !slices.Contains(g.Capabilities, capability) {
		return nil, access.ErrDenied
	}
	if action == "update" && before != nil {
		if t.Kind == Component && ptrValue(before.ParentComponentID) != ptrValue(r.ParentComponentID) {
			return nil, problem("tenant_action_required", "已有构件调整父级须由租户统一处理。", 409)
		}
		if t.Kind == Point && ptrValue(before.ComponentID) != ptrValue(r.ComponentID) {
			owner := scoped(auth.Scope, Target{Kind: Component, Facility: t.Facility, ID: ptrValue(r.ComponentID)})
			if owner.ID == "" {
				owner.Kind = Facility
				owner.ID = t.Facility
			}
			g, _, e := projectGrantTx(ctx, tx, auth, owner, true)
			if e != nil {
				return nil, e
			}
			if !slices.Contains(g.Capabilities, "create_point") {
				return nil, access.ErrDenied
			}
		}
	}
	if action == "retire" || action == "restore" {
		ok, e := projectLifecycleAllowed(ctx, tx, auth, t)
		if e != nil {
			return nil, e
		}
		if !ok {
			return nil, problem("shared_resource_tenant_action_required", "资源由其他项目使用，或下级范围未完整授权，请由租户统一处理。", 409)
		}
	}
	return link, nil
}

const descendantsSQL = `WITH RECURSIVE affected(kind,id) AS (
 SELECT kind,id FROM engineering_resource_nodes WHERE tenant_id=$1 AND facility_id=$2 AND kind=$3 AND id=$4 AND deleted_at IS NULL
 UNION SELECT n.kind,n.id FROM engineering_resource_nodes n JOIN affected a ON n.parent_kind=a.kind AND n.parent_id=a.id WHERE n.tenant_id=$1 AND n.facility_id=$2 AND n.deleted_at IS NULL
)`

func projectLifecycleAllowed(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget) (bool, error) {
	var allowed bool
	e := tx.QueryRow(ctx, descendantsSQL+` SELECT NOT EXISTS(SELECT 1 FROM affected a WHERE NOT EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active' WHERE g.tenant_id=$1 AND g.project_id=$5 AND g.facility_id=$2 AND g.resource_kind=a.kind AND COALESCE(g.component_id,g.point_id,g.channel_id,g.facility_id)=a.id AND g.ended_at IS NULL AND g.can_lifecycle)) AND NOT EXISTS(SELECT 1 FROM project_resource_visibility($1,NULL,$2) v JOIN affected a ON a.kind=v.kind AND a.id=v.id WHERE v.project_id<>$5::uuid)`, auth.Tenant, t.Facility, t.Kind, t.ID, auth.Project).Scan(&allowed)
	return allowed, e
}
func sharedTx(ctx context.Context, tx pgx.Tx, auth authorization, t scopedTarget) (bool, error) {
	var shared bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_resource_visibility($1,NULL,$2) WHERE kind=$3 AND id=$4 AND ($5='' OR project_id<>NULLIF($5,'')::uuid))`, auth.Tenant, t.Facility, t.Kind, t.ID, auth.Project).Scan(&shared)
	return shared, e
}
func projectCreateGrant(ctx context.Context, tx pgx.Tx, auth authorization, r Resource, link *Link, request string, meta access.Meta) (string, string, string, error) {
	fid := r.FacilityID
	if r.Kind == Facility {
		fid = r.ID
	}
	if link == nil {
		link = &Link{}
		if e := tx.QueryRow(ctx, `INSERT INTO project_facility_links(tenant_id,project_id,facility_id,created_by) VALUES($1,$2,$3,$4) RETURNING id::text,revision::text`, auth.Tenant, auth.Project, fid, auth.Account).Scan(&link.ID, &link.Revision); e != nil {
			return "", "", "", e
		}
	} else {
		if e := tx.QueryRow(ctx, `UPDATE project_facility_links SET revision=revision+1 WHERE id=$1 RETURNING revision::text`, link.ID).Scan(&link.Revision); e != nil {
			return "", "", "", e
		}
	}
	g := Grant{Kind: r.Kind, ResourceID: r.ID, Capabilities: kindCapabilities[r.Kind], Name: r.Name, Code: r.Code}
	// Creating a channel never grants authority over its shared source relation.
	g.Capabilities = slices.DeleteFunc(slices.Clone(g.Capabilities), func(c string) bool { return c == "source_manage" })
	switch r.Kind {
	case Component:
		g.ComponentID = &r.ID
	case Point:
		g.PointID = &r.ID
	case Channel:
		g.ChannelID = &r.ID
	}
	g, e := saveGrant(ctx, tx, auth.Tenant, auth.Project, fid, link.ID, auth.Account, "project_create", g)
	if e != nil {
		return "", "", "", e
	}
	rev, e := recordAccessChange(ctx, tx, auth, auth.Project, link.ID, "project_create", request, "项目创建资源并获得标准授权", map[string]any{}, g, meta)
	return link.ID, link.Revision, rev, e
}
