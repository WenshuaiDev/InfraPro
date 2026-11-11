package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

var kindCapabilities = map[Kind][]string{
	Facility:  {"read", "update", "create_component", "create_point"},
	Component: {"read", "update", "create_component", "create_point", "lifecycle"},
	Point:     {"read", "update", "create_channel", "lifecycle"},
	Channel:   {"read", "update", "lifecycle"},
}

func capabilities(kind Kind, cs []string) ([]string, error) {
	if !slices.Contains(cs, "read") || len(cs) > 6 || tables[kind] == "" {
		return nil, problem("grant_capability_invalid", "每个对象须包含读取及该类型适用的能力。", 422)
	}
	out := []string{}
	for _, c := range cs {
		if !slices.Contains(kindCapabilities[kind], c) {
			return nil, problem("grant_capability_invalid", "授权包含该对象类型不适用的能力。", 422)
		}
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return out, nil
}
func grantInput(in GrantInput, fid string) (Grant, error) {
	g := Grant{Kind: in.Kind, ComponentID: in.ComponentID, PointID: in.PointID, ChannelID: in.ChannelID}
	ids := []*string{in.ComponentID, in.PointID, in.ChannelID}
	expected := map[Kind]int{Facility: -1, Component: 0, Point: 1, Channel: 2}[in.Kind]
	for i, id := range ids {
		if (i == expected) != (id != nil) || id != nil && !access.ValidID(*id) {
			return g, invalid("/add", "reference", "对象类型与唯一引用不匹配。")
		}
	}
	g.ResourceID = fid
	if expected >= 0 {
		g.ResourceID = *ids[expected]
	}
	var e error
	g.Capabilities, e = capabilities(g.Kind, in.Capabilities)
	return g, e
}
func accessChanged() error {
	return problem("resource_access_changed", "项目资源授权已变化，请刷新并复核当前输入。", 409)
}
func previewChanged() error {
	return problem("preview_changed", "预览依据已变化，请重新预览并确认。", 409)
}
func policyTx(ctx context.Context, tx pgx.Tx, tenant, project string) (AccessPolicy, error) {
	p := AccessPolicy{}
	if !access.ValidID(project) {
		return p, access.ErrProjectNotFound
	}
	e := tx.QueryRow(ctx, `SELECT r.tenant_id::text,r.project_id::text,p.name,p.status,p.version::text,r.allow_facility_create,r.revision::text FROM project_resource_policies r JOIN projects p ON p.id=r.project_id WHERE r.tenant_id=$1 AND r.project_id=$2`, tenant, project).Scan(&p.TenantID, &p.ProjectID, &p.ProjectName, &p.ProjectStatus, &p.ProjectVersion, &p.AllowFacilityCreate, &p.Revision)
	if e == pgx.ErrNoRows {
		return p, access.ErrProjectNotFound
	}
	return p, e
}
func linkTx(ctx context.Context, tx pgx.Tx, tenant, project, facility string) (*Link, error) {
	v := Link{}
	e := tx.QueryRow(ctx, `SELECT l.id::text,l.tenant_id::text,l.project_id::text,l.facility_id::text,f.name,f.code,l.status,l.revision::text,l.created_at FROM project_facility_links l JOIN engineering_facilities f ON f.id=l.facility_id WHERE l.tenant_id=$1 AND l.project_id=$2 AND l.facility_id=$3 AND l.status='active'`, tenant, project, facility).Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.FacilityID, &v.FacilityName, &v.FacilityCode, &v.Status, &v.Revision, &v.CreatedAt)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	return &v, e
}

const grantSelect = `SELECT g.id::text,g.link_id::text,g.resource_kind,COALESCE(g.component_id,g.point_id,g.channel_id,g.facility_id)::text,g.component_id::text,g.point_id::text,g.channel_id::text,n.name,n.code,g.source,g.revision::text,g.can_update,g.can_create_component,g.can_create_point,g.can_create_channel,g.can_lifecycle FROM project_resource_grants g JOIN engineering_resource_nodes n ON n.tenant_id=g.tenant_id AND n.kind=g.resource_kind AND n.id=COALESCE(g.component_id,g.point_id,g.channel_id,g.facility_id)`

func scanGrant(row pgx.Row) (Grant, error) {
	g := Grant{Capabilities: []string{"read"}}
	var update, component, point, channel, lifecycle bool
	e := row.Scan(&g.ID, &g.LinkID, &g.Kind, &g.ResourceID, &g.ComponentID, &g.PointID, &g.ChannelID, &g.Name, &g.Code, &g.Source, &g.Revision, &update, &component, &point, &channel, &lifecycle)
	for _, c := range []struct {
		yes  bool
		name string
	}{{update, "update"}, {component, "create_component"}, {point, "create_point"}, {channel, "create_channel"}, {lifecycle, "lifecycle"}} {
		if c.yes {
			g.Capabilities = append(g.Capabilities, c.name)
		}
	}
	slices.Sort(g.Capabilities)
	return g, e
}
func currentGrants(ctx context.Context, tx pgx.Tx, link string, ids, targets []string, all bool) ([]Grant, error) {
	out := []Grant{}
	rows, e := tx.Query(ctx, grantSelect+` WHERE g.link_id=$1 AND g.ended_at IS NULL AND ($4 OR g.id=ANY($2::uuid[]) OR n.id=ANY($3::uuid[])) ORDER BY g.resource_kind,n.id LIMIT 1001`, link, ids, targets, all)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		g, e := scanGrant(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func normalizeAccess(in *AccessInput, facility string) error {
	if !access.ValidID(in.RequestID) || !decimalRevision(in.AccessRevision) {
		return problem("invalid_request", "请求编号或授权修订无效。", 400)
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" || utf8.RuneCountInString(in.Reason) > 2000 {
		return invalid("/reason", "required", "请填写原因，最多 2000 字符。")
	}
	in.PreviewHash = strings.TrimSpace(in.PreviewHash)
	if facility == "" {
		if in.Action != "policy" || in.AllowFacilityCreate == nil || in.LinkRevision != "" || len(in.Add)+len(in.Update)+len(in.End) > 0 || in.SelectCurrent != nil {
			return problem("invalid_request", "政策操作只接受新建开关。", 400)
		}
	} else {
		if !access.ValidID(facility) || !slices.Contains([]string{"create_link", "change_grants", "revoke_link"}, in.Action) || in.AllowFacilityCreate != nil {
			return problem("invalid_request", "设施授权指令无效。", 400)
		}
		if in.Action == "create_link" && in.LinkRevision != "" || in.Action != "create_link" && !decimalRevision(in.LinkRevision) {
			return problem("invalid_request", "请提供当前关联修订，新建关联不填写该字段。", 400)
		}
		if in.Action == "revoke_link" && (len(in.Add)+len(in.Update)+len(in.End) > 0 || in.SelectCurrent != nil) {
			return problem("invalid_request", "撤销关联不接受额外对象变更。", 400)
		}
		if in.Action == "create_link" && (len(in.Update)+len(in.End) > 0) {
			return problem("invalid_request", "新建关联不能修改历史授权。", 400)
		}
	}
	if len(in.Add)+len(in.Update)+len(in.End) > 1000 {
		return invalid("/add", "batch_limit", "每次最多处理 1,000 个对象，请分批操作。")
	}
	for i := range in.Add {
		cs, e := capabilities(in.Add[i].Kind, in.Add[i].Capabilities)
		if e != nil {
			return e
		}
		in.Add[i].Capabilities = cs
	}
	for i := range in.Update {
		slices.Sort(in.Update[i].Capabilities)
		in.Update[i].Capabilities = slices.Compact(in.Update[i].Capabilities)
	}
	if in.SelectCurrent != nil {
		for kind, cs := range in.SelectCurrent.Kinds {
			v, e := capabilities(kind, cs)
			if e != nil {
				return e
			}
			in.SelectCurrent.Kinds[kind] = v
		}
	}
	return nil
}
func decimalRevision(v string) bool {
	n, e := strconv.ParseInt(v, 10, 64)
	return e == nil && n > 0 && strconv.FormatInt(n, 10) == v
}
func emptyAccessPreview() AccessPreview {
	return AccessPreview{Add: []Grant{}, Update: []Grant{}, End: []Grant{}, Context: []ContextNode{}}
}
func (s *Service) PreviewAccessChange(ctx context.Context, a access.Actor, tenant, project, facility string, in AccessInput) (AccessPreview, error) {
	out := emptyAccessPreview()
	if e := normalizeAccess(&in, facility); e != nil {
		return out, e
	}
	kind := "facility"
	if facility == "" {
		kind = "policy"
	}
	e := s.transact(ctx, a, access.ResourceAccessOperation(kind, "preview"), TenantScope(tenant), false, func(tx pgx.Tx, auth authorization) error {
		var e error
		out, e = planAccess(ctx, tx, auth, project, facility, in)
		return e
	})
	return out, e
}
func planAccess(ctx context.Context, tx pgx.Tx, auth authorization, project, facility string, in AccessInput) (AccessPreview, error) {
	out := emptyAccessPreview()
	p, e := policyTx(ctx, tx, auth.Tenant, project)
	if e != nil {
		return out, e
	}
	out.Policy = p
	if in.AccessRevision != p.Revision {
		return out, accessChanged()
	}
	adds := append([]GrantInput{}, in.Add...)
	if in.SelectCurrent != nil {
		more, e := expandCurrent(ctx, tx, auth.Tenant, facility, *in.SelectCurrent)
		if e != nil {
			return out, e
		}
		adds = append(adds, more...)
	}
	ids, targets := []string{}, []string{}
	for _, u := range in.Update {
		if !access.ValidID(u.ID) {
			return out, accessChanged()
		}
		ids = append(ids, u.ID)
	}
	for _, u := range in.End {
		if !access.ValidID(u.ID) {
			return out, accessChanged()
		}
		ids = append(ids, u.ID)
	}
	for _, g := range adds {
		v, e := grantInput(g, facility)
		if e != nil {
			return out, e
		}
		targets = append(targets, v.ResourceID)
	}
	if len(ids)+len(targets) > 1000 {
		return out, invalid("/add", "batch_limit", "每次最多处理 1,000 个对象，请分批选择。")
	}
	before := []Grant{}
	var resource Resource
	if facility != "" {
		resource, e = lockFacility(ctx, tx, scoped(auth.Scope, Target{Kind: Facility, Facility: facility, ID: facility}), false, false)
		if e != nil {
			return out, e
		}
		out.Link, e = linkTx(ctx, tx, auth.Tenant, project, facility)
		if e != nil {
			return out, e
		}
		if in.Action == "create_link" {
			if out.Link != nil {
				return out, accessChanged()
			}
		} else {
			if out.Link == nil || out.Link.Revision != in.LinkRevision {
				return out, accessChanged()
			}
			before, e = currentGrants(ctx, tx, out.Link.ID, ids, targets, in.Action == "revoke_link")
			if e != nil {
				return out, e
			}
		}
	}
	if in.Action == "policy" {
		if p.ProjectStatus == "archived" && *in.AllowFacilityCreate && !p.AllowFacilityCreate {
			return out, problem("project_archived", "已归档项目不能扩张资源授权。", 409)
		}
		out.Changed = p.AllowFacilityCreate != *in.AllowFacilityCreate
	} else {
		if len(before) > 1000 && in.Action == "revoke_link" {
			return out, invalid("/end", "batch_limit", "请先分批缩减授权至 1,000 个对象以内，再结束关联。")
		}
		byID := map[string]Grant{}
		byTarget := map[string]Grant{}
		for _, g := range before {
			byID[g.ID] = g
			byTarget[string(g.Kind)+":"+g.ResourceID] = g
		}
		nodes := map[string]struct{ Name, Code string }{}
		rows, e := tx.Query(ctx, `SELECT kind||':'||id::text,name,code FROM engineering_resource_nodes WHERE tenant_id=$1 AND facility_id=$2 AND id=ANY($3::uuid[]) AND deleted_at IS NULL`, auth.Tenant, facility, targets)
		if e != nil {
			return out, e
		}
		for rows.Next() {
			var key string
			var n struct{ Name, Code string }
			if e = rows.Scan(&key, &n.Name, &n.Code); e != nil {
				rows.Close()
				return out, e
			}
			nodes[key] = n
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return out, e
		}
		seen := map[string]bool{}
		for index, input := range adds {
			g, e := grantInput(input, facility)
			if e != nil {
				return out, e
			}
			key := string(g.Kind) + ":" + g.ResourceID
			if seen[key] {
				return out, invalid("/add", "duplicate", "同一对象不能在指令中重复或冲突。")
			}
			seen[key] = true
			if existing, ok := byTarget[key]; ok {
				if index < len(in.Add) {
					return out, invalid("/add", "already_granted", "该对象已有授权，请修改现有授权。")
				}
				if !slices.Equal(existing.Capabilities, g.Capabilities) {
					if p.ProjectStatus == "archived" {
						for _, c := range g.Capabilities {
							if !slices.Contains(existing.Capabilities, c) {
								return out, problem("project_archived", "已归档项目只能缩减授权。", 409)
							}
						}
					}
					existing.Capabilities = g.Capabilities
					out.Update = append(out.Update, existing)
				}
				continue
			}
			n, ok := nodes[key]
			if !ok {
				return out, notFound()
			}
			g.Name = n.Name
			g.Code = n.Code
			g.Source = "tenant_grant"
			out.Add = append(out.Add, g)
		}
		for _, u := range in.Update {
			g, ok := byID[u.ID]
			if !ok || u.Revision != g.Revision {
				return out, accessChanged()
			}
			key := string(g.Kind) + ":" + g.ResourceID
			if seen[key] {
				return out, invalid("/update", "duplicate", "同一对象不能在指令中重复或冲突。")
			}
			seen[key] = true
			cs, e := capabilities(g.Kind, u.Capabilities)
			if e != nil {
				return out, e
			}
			if p.ProjectStatus == "archived" {
				for _, c := range cs {
					if !slices.Contains(g.Capabilities, c) {
						return out, problem("project_archived", "已归档项目只能缩减授权。", 409)
					}
				}
			}
			if !slices.Equal(cs, g.Capabilities) {
				g.Capabilities = cs
				out.Update = append(out.Update, g)
			}
		}
		for _, u := range in.End {
			g, ok := byID[u.ID]
			if !ok || u.Revision != g.Revision {
				return out, accessChanged()
			}
			key := string(g.Kind) + ":" + g.ResourceID
			if seen[key] {
				return out, invalid("/end", "duplicate", "同一对象不能在指令中重复或冲突。")
			}
			seen[key] = true
			if g.Kind == Facility {
				return out, invalid("/end", "root", "取消设施读取须结束整条关联。")
			}
			out.End = append(out.End, g)
		}
		if in.Action == "create_link" {
			n := 0
			for _, g := range out.Add {
				if g.Kind == Facility {
					n++
				}
			}
			if n != 1 {
				return out, invalid("/add", "root", "新建关联必须包含且仅包含一条设施读取授权。")
			}
		}
		if in.Action == "revoke_link" {
			out.End = before
		}
		if len(out.Add)+len(out.Update)+len(out.End) > 1000 {
			return out, invalid("/add", "batch_limit", "每次最多处理 1,000 个对象，请分批选择。")
		}
		if p.ProjectStatus == "archived" && (in.Action == "create_link" || len(out.Add) > 0) {
			return out, problem("project_archived", "已归档项目不能新建关联或授权对象。", 409)
		}
		// Previews identify ended full records that remain necessary ancestor context.
		if out.Link != nil && in.Action != "revoke_link" && len(out.End) > 0 {
			ended := []string{}
			for _, g := range out.End {
				ended = append(ended, g.ResourceID)
			}
			rows, e := tx.Query(ctx, `WITH RECURSIVE remaining(kind,id) AS (
 SELECT resource_kind,COALESCE(component_id,point_id,channel_id,facility_id) FROM project_resource_grants WHERE link_id=$1 AND ended_at IS NULL AND NOT COALESCE(component_id,point_id,channel_id,facility_id)=ANY($2::uuid[])
 UNION SELECT n.parent_kind,n.parent_id FROM remaining r JOIN engineering_resource_nodes n ON n.tenant_id=$3 AND n.kind=r.kind AND n.id=r.id WHERE n.parent_id IS NOT NULL)
 SELECT DISTINCT n.id::text,n.kind,n.code,n.name,n.status FROM remaining r JOIN engineering_resource_nodes n ON n.tenant_id=$3 AND n.kind=r.kind AND n.id=r.id WHERE n.id=ANY($2::uuid[])`, out.Link.ID, ended, auth.Tenant)
			if e != nil {
				return out, e
			}
			for rows.Next() {
				n := ContextNode{Visibility: "context", FacilityID: facility}
				if e = rows.Scan(&n.ID, &n.Kind, &n.Code, &n.Name, &n.Status); e != nil {
					rows.Close()
					return out, e
				}
				out.Context = append(out.Context, n)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return out, e
			}
		}
		out.Changed = in.Action == "revoke_link" || len(out.Add)+len(out.Update)+len(out.End) > 0
	}
	out.Before = before
	in.PreviewHash = ""
	var resourceState string
	if facility != "" {
		e = tx.QueryRow(ctx, `SELECT md5(jsonb_build_object(
 'objects',(SELECT jsonb_agg(to_jsonb(n) ORDER BY n.kind,n.id) FROM engineering_resource_nodes n WHERE n.tenant_id=$1 AND n.facility_id=$2),
 'links',(SELECT jsonb_agg(jsonb_build_array(id,project_id,status,revision) ORDER BY id) FROM project_facility_links WHERE tenant_id=$1 AND facility_id=$2)
 )::text)`, auth.Tenant, facility).Scan(&resourceState)
		if e != nil {
			return out, e
		}
	}
	// Include resource revisions and the expanded selected objects; additions after preview change this hash.
	out.Hash = fingerprint(struct {
		Auth          any
		Input         AccessInput
		Plan          AccessPreview
		Resource      Resource
		ResourceState string
	}{[]string{auth.Account, auth.TenantMembership, auth.Tenant, project}, in, out, resource, resourceState})
	return out, nil
}
func expandCurrent(ctx context.Context, tx pgx.Tx, tenant, facility string, sel CurrentSelection) ([]GrantInput, error) {
	if tables[sel.RootKind] == "" || !access.ValidID(sel.RootID) || len(sel.Kinds) == 0 {
		return nil, invalid("/select_current", "reference", "请选择明确的根对象及下级类型。")
	}
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM engineering_resource_nodes WHERE tenant_id=$1 AND facility_id=$2 AND kind=$3 AND id=$4 AND deleted_at IS NULL)`, tenant, facility, sel.RootKind, sel.RootID).Scan(&exists); e != nil {
		return nil, e
	}
	if !exists {
		return nil, notFound()
	}

	rows, e := tx.Query(ctx, `WITH RECURSIVE descendants AS (
 SELECT kind,id FROM engineering_resource_nodes WHERE tenant_id=$1 AND facility_id=$2 AND kind=$3 AND id=$4 AND deleted_at IS NULL
 UNION SELECT n.kind,n.id FROM engineering_resource_nodes n JOIN descendants d ON n.parent_kind=d.kind AND n.parent_id=d.id WHERE n.tenant_id=$1 AND n.facility_id=$2 AND n.deleted_at IS NULL)
 SELECT kind,id::text FROM descendants WHERE kind=ANY($5::text[]) AND id<>$4::uuid ORDER BY kind,id LIMIT 1001`, tenant, facility, sel.RootKind, sel.RootID, func() []string {
		v := []string{}
		for k := range sel.Kinds {
			v = append(v, string(k))
		}
		return v
	}())
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []GrantInput{}
	for rows.Next() {
		var kind Kind
		var id string
		if e = rows.Scan(&kind, &id); e != nil {
			return nil, e
		}
		g := GrantInput{Kind: kind, Capabilities: sel.Kinds[kind]}
		switch kind {
		case Component:
			g.ComponentID = &id
		case Point:
			g.PointID = &id
		case Channel:
			g.ChannelID = &id
		}
		out = append(out, g)
	}
	if len(out) > 1000 {
		return nil, invalid("/select_current", "batch_limit", "当前集合超过 1,000 个对象，请分批选择。")
	}
	return out, rows.Err()
}
func saveGrant(ctx context.Context, tx pgx.Tx, tenant, project, facility, link, actor, source string, g Grant) (Grant, error) {
	cs := g.Capabilities
	var e error
	e = tx.QueryRow(ctx, `INSERT INTO project_resource_grants(tenant_id,project_id,facility_id,link_id,resource_kind,component_id,point_id,channel_id,can_update,can_create_component,can_create_point,can_create_channel,can_lifecycle,source,granted_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id::text,revision::text`, tenant, project, facility, link, g.Kind, g.ComponentID, g.PointID, g.ChannelID, slices.Contains(cs, "update"), slices.Contains(cs, "create_component"), slices.Contains(cs, "create_point"), slices.Contains(cs, "create_channel"), slices.Contains(cs, "lifecycle"), source, actor).Scan(&g.ID, &g.Revision)
	g.LinkID = link
	g.Source = source
	return g, e
}
func (s *Service) ChangeAccess(ctx context.Context, a access.Actor, tenant, project, facility string, in AccessInput, meta access.Meta) (AccessResult, error) {
	out := AccessResult{}
	if e := normalizeAccess(&in, facility); e != nil {
		return out, e
	}
	kind := "facility"
	if facility == "" {
		kind = "policy"
	}
	op := access.ResourceAccessOperation(kind, "change")
	e := s.transact(ctx, a, op, TenantScope(tenant), true, func(tx pgx.Tx, auth authorization) error {
		if _, e := policyTx(ctx, tx, tenant, project); e != nil {
			return e
		}
		hash := fingerprint(struct {
			Scope    Scope
			Project  string
			Facility string
			Member   string
			Input    AccessInput
		}{auth.Scope, project, facility, auth.TenantMembership, in})
		var oldHash, oldOp, oldMember string
		e := tx.QueryRow(ctx, `SELECT operation,payload_hash,result,tenant_membership_id::text FROM project_resource_operation_receipts WHERE entry_scope='tenant' AND tenant_id=$1 AND project_id=$2 AND actor_id=$3 AND request_id=$4`, tenant, project, a.ID, in.RequestID).Scan(&oldOp, &oldHash, &out, &oldMember)
		if e == nil {
			if oldMember != auth.TenantMembership {
				return notFound()
			}
			if oldHash != hash || oldOp != string(op) {
				return problem("request_payload_conflict", "该请求编号已用于不同内容。", 409)
			}
			out.Replayed = true
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if facility != "" {
			if _, e = lockFacility(ctx, tx, scoped(auth.Scope, Target{Kind: Facility, Facility: facility, ID: facility}), true, false); e != nil {
				return e
			}
		}
		plan, e := planAccess(ctx, tx, auth, project, facility, in)
		if e != nil {
			return e
		}
		if in.PreviewHash == "" {
			return problem("preview_required", "请先预览并确认本次授权变更。", 409)
		}
		if plan.Hash != in.PreviewHash {
			return previewChanged()
		}
		out = AccessResult{ProjectID: project, AccessRevision: plan.Policy.Revision, Changed: plan.Changed}
		if plan.Link != nil {
			out.LinkID = plan.Link.ID
			out.LinkRevision = plan.Link.Revision
		}
		if plan.Changed {
			if in.Action == "policy" {
				if _, e = tx.Exec(ctx, `UPDATE project_resource_policies SET allow_facility_create=$3 WHERE tenant_id=$1 AND project_id=$2`, tenant, project, *in.AllowFacilityCreate); e != nil {
					return e
				}
			} else {
				if in.Action == "create_link" {
					if e = tx.QueryRow(ctx, `INSERT INTO project_facility_links(tenant_id,project_id,facility_id,created_by) VALUES($1,$2,$3,$4) RETURNING id::text,revision::text`, tenant, project, facility, a.ID).Scan(&out.LinkID, &out.LinkRevision); e != nil {
						return e
					}
				}
				for i, g := range plan.Add {
					plan.Add[i], e = saveGrant(ctx, tx, tenant, project, facility, out.LinkID, a.ID, "tenant_grant", g)
					if e != nil {
						return e
					}
				}
				for i, g := range plan.Update {
					cs := g.Capabilities
					if e = tx.QueryRow(ctx, `UPDATE project_resource_grants SET can_update=$2,can_create_component=$3,can_create_point=$4,can_create_channel=$5,can_lifecycle=$6,revision=revision+1 WHERE id=$1 RETURNING revision::text`, g.ID, slices.Contains(cs, "update"), slices.Contains(cs, "create_component"), slices.Contains(cs, "create_point"), slices.Contains(cs, "create_channel"), slices.Contains(cs, "lifecycle")).Scan(&plan.Update[i].Revision); e != nil {
						return e
					}
				}
				for _, g := range plan.End {
					if _, e = tx.Exec(ctx, `UPDATE project_resource_grants SET ended_at=clock_timestamp(),ended_by=$2,end_reason=$3,revision=revision+1 WHERE id=$1`, g.ID, a.ID, in.Reason); e != nil {
						return e
					}
				}
				if in.Action == "revoke_link" {
					e = tx.QueryRow(ctx, `UPDATE project_facility_links SET status='revoked',revoked_at=clock_timestamp(),revoked_by=$2,revoke_reason=$3,revision=revision+1 WHERE id=$1 RETURNING revision::text`, out.LinkID, a.ID, in.Reason).Scan(&out.LinkRevision)
				} else if in.Action != "create_link" {
					e = tx.QueryRow(ctx, `UPDATE project_facility_links SET revision=revision+1 WHERE id=$1 RETURNING revision::text`, out.LinkID).Scan(&out.LinkRevision)
				}
				if e != nil {
					return e
				}
			}
			out.AccessRevision, e = recordAccessChange(ctx, tx, auth, project, out.LinkID, in.Action, in.RequestID, in.Reason, map[string]any{"policy": plan.Policy, "link": plan.Link, "ended": plan.End, "grants": plan.Before}, map[string]any{"added": plan.Add, "updated": plan.Update, "ended": plan.End, "allow_facility_create": in.AllowFacilityCreate}, meta)
			if e != nil {
				return e
			}
		}
		_, e = tx.Exec(ctx, `INSERT INTO project_resource_operation_receipts(entry_scope,tenant_id,project_id,actor_id,request_id,operation,payload_hash,tenant_membership_id,link_id,target_kind,target_id,result) VALUES('tenant',$1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,NULLIF($10,'')::uuid,$11)`, tenant, project, a.ID, in.RequestID, op, hash, auth.TenantMembership, out.LinkID, kind, facility, out)
		return e
	})
	return out, e
}
func recordAccessChange(ctx context.Context, tx pgx.Tx, auth authorization, project, link, operation, request, reason string, before, after any, meta access.Meta) (string, error) {
	var rev string
	e := tx.QueryRow(ctx, `UPDATE project_resource_policies SET revision=revision+1,updated_at=clock_timestamp(),updated_by=$3 WHERE tenant_id=$1 AND project_id=$2 RETURNING revision::text`, auth.Tenant, project, auth.Account).Scan(&rev)
	if e != nil {
		return "", e
	}
	member := auth.TenantMembership
	if auth.Entry == "project" {
		member = auth.ProjectAuth.MembershipID
	}
	_, e = tx.Exec(ctx, `INSERT INTO project_resource_access_changes(tenant_id,project_id,access_revision,link_id,operation,actor_id,actor_scope,actor_membership_id,request_id,reason,before,after) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11,$12)`, auth.Tenant, project, rev, link, operation, auth.Account, auth.Entry, member, request, reason, before, after)
	if e != nil {
		return "", e
	}
	if e = access.Bump(ctx, tx); e != nil {
		return "", e
	}
	objectType := "project_resource_grant_set"
	objectID := link
	event := map[string]string{"policy": "resource_access.policy.updated", "create_link": "resource_access.link.created", "revoke_link": "resource_access.link.revoked"}[operation]
	if event == "" {
		event = "resource_access.grants.changed"
	}
	if operation == "policy" {
		objectType = "project_resource_policy"
		objectID = project
	} else if operation == "create_link" || operation == "revoke_link" {
		objectType = "project_facility_link"
	}
	meta.ActorID = auth.Account
	meta.TenantID = auth.Tenant
	meta.ActorScope = auth.Entry
	meta.ProjectID = ""
	meta.Reason = "项目资源授权已更新"
	summary := map[string]any{"operation": operation, "access_revision": rev}
	if auth.Entry == "tenant" {
		if e = access.Audit(ctx, tx, event, objectType, objectID, "", nil, summary, meta); e != nil {
			return "", e
		}
	}
	// The project audit deliberately excludes the tenant actor, free text reason and resource body.
	meta.ProjectID = project
	meta.ActorScope = auth.Entry
	meta.ActorID = ""
	meta.ActorType = "system"
	e = access.Audit(ctx, tx, "project.resource_access.changed", objectType, objectID, "", nil, summary, meta)
	return rev, e
}

// Scoped opaque cursors also bind current authorization revision and membership.
func accessPageCursor(auth authorization, p AccessPolicy, facility, category, raw string) (string, error) {
	stamp := fingerprint([]string{auth.Account, auth.TenantMembership, p.TenantID, p.ProjectID, p.Revision, facility, category})
	if raw == "" {
		return "", nil
	}
	b, e := base64.RawURLEncoding.DecodeString(raw)
	parts := strings.Split(string(b), "|")
	if e != nil || len(parts) != 2 || parts[0] != stamp || !access.ValidID(parts[1]) {
		return "", problem("invalid_request", "分页依据已变化，请回到第一页。", 400)
	}
	return parts[1], nil
}
func accessNextCursor(auth authorization, p AccessPolicy, facility, category, id string) string {
	stamp := fingerprint([]string{auth.Account, auth.TenantMembership, p.TenantID, p.ProjectID, p.Revision, facility, category})
	return base64.RawURLEncoding.EncodeToString([]byte(stamp + "|" + id))
}
func (s *Service) AccessDetail(ctx context.Context, a access.Actor, tenant, project, facility string, f AccessFilter) (AccessDetail, error) {
	out := AccessDetail{Links: []Link{}, Grants: []Grant{}, CandidateGrants: []Grant{}, Candidates: Page{Items: []Item{}}}
	kind := "access"
	if facility != "" {
		kind = "facility"
	}
	e := s.transact(ctx, a, access.ResourceAccessOperation(kind, "detail"), TenantScope(tenant), false, func(tx pgx.Tx, auth authorization) error {
		if f.Limit == 0 {
			f.Limit = 25
		}
		if f.Limit < 1 || f.Limit > 100 {
			return problem("invalid_request", "分页大小须为 1–100。", 400)
		}
		var e error
		out.Policy, e = policyTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		id, e := accessPageCursor(auth, out.Policy, facility, "links", f.Cursor)
		if e != nil {
			return e
		}
		if facility == "" {
			rows, e := tx.Query(ctx, `SELECT l.id::text,l.tenant_id::text,l.project_id::text,l.facility_id::text,f.name,f.code,l.status,l.revision::text,l.created_at FROM project_facility_links l JOIN engineering_facilities f ON f.id=l.facility_id WHERE l.tenant_id=$1 AND l.project_id=$2 AND l.status='active' AND ($3='' OR l.id>NULLIF($3,'')::uuid) ORDER BY l.id LIMIT $4`, tenant, project, id, f.Limit+1)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				v := Link{}
				if e = rows.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.FacilityID, &v.FacilityName, &v.FacilityCode, &v.Status, &v.Revision, &v.CreatedAt); e != nil {
					return e
				}
				out.Links = append(out.Links, v)
			}
			if len(out.Links) > f.Limit {
				out.Links = out.Links[:f.Limit]
				out.NextCursor = accessNextCursor(auth, out.Policy, facility, "links", out.Links[f.Limit-1].ID)
			}
			return rows.Err()
		}
		if !access.ValidID(facility) {
			return notFound()
		}
		if _, e = lockFacility(ctx, tx, scoped(auth.Scope, Target{Kind: Facility, Facility: facility, ID: facility}), false, false); e != nil {
			return e
		}
		out.Link, e = linkTx(ctx, tx, tenant, project, facility)
		if e != nil {
			return e
		}
		gid, e := accessPageCursor(auth, out.Policy, facility, "grants", f.GrantCursor)
		if e != nil {
			return e
		}
		if out.Link != nil {
			rows, e := tx.Query(ctx, grantSelect+` WHERE g.link_id=$1 AND g.ended_at IS NULL AND ($2='' OR g.id>NULLIF($2,'')::uuid) ORDER BY g.id LIMIT $3`, out.Link.ID, gid, f.Limit+1)
			if e != nil {
				return e
			}
			for rows.Next() {
				g, e := scanGrant(rows)
				if e != nil {
					rows.Close()
					return e
				}
				out.Grants = append(out.Grants, g)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			if len(out.Grants) > f.Limit {
				out.Grants = out.Grants[:f.Limit]
				out.GrantNextCursor = accessNextCursor(auth, out.Policy, facility, "grants", out.Grants[f.Limit-1].ID)
			}
		}
		if f.CandidateKind == "" {
			f.CandidateKind = Component
		}
		if tables[f.CandidateKind] == "" {
			return problem("invalid_request", "候选类型无效。", 400)
		}
		if e = normalizeFilter(&f.Candidate); e != nil {
			return e
		}
		category := "candidates:" + string(f.CandidateKind) + ":" + fingerprint([]string{f.Candidate.Query, f.Candidate.Status, f.Candidate.Parent, f.Candidate.Owner})
		cid, e := accessPageCursor(auth, out.Policy, facility, category, f.Candidate.Cursor)
		if e != nil {
			return e
		}
		facilityCol := "r.facility_id"
		if f.CandidateKind == Facility {
			facilityCol = "r.id"
		}
		args := []any{tenant, facility, cid, f.Candidate.Limit + 1, f.Candidate.Query, f.Candidate.Status}
		where := ""
		if f.CandidateKind == Component || f.CandidateKind == Point {
			owner := f.Candidate.Owner
			column := "component_id"
			if f.CandidateKind == Component {
				owner = f.Candidate.Parent
				column = "parent_component_id"
			}
			if owner == "root" {
				where = " AND r." + column + " IS NULL"
			} else if owner != "" && owner != "all" {
				if !access.ValidID(owner) {
					return problem("invalid_request", "候选归属无效。", 400)
				}
				where = " AND r." + column + "=$7"
				args = append(args, owner)
			}
		}
		if f.CandidateKind == Channel && f.Candidate.Owner != "" && f.Candidate.Owner != "all" {
			owner := f.Candidate.Owner
			pointOwner := "p.component_id IS NULL"
			if owner != "root" {
				if !access.ValidID(owner) {
					return problem("invalid_request", "候选归属无效。", 400)
				}
				pointOwner = "p.component_id=$7"
				args = append(args, owner)
			}
			where = " AND EXISTS(SELECT 1 FROM engineering_points p WHERE p.tenant_id=r.tenant_id AND p.facility_id=r.facility_id AND p.id=r.point_id AND p.deleted_at IS NULL AND " + pointOwner + ")"
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(r) FROM `+tables[f.CandidateKind]+` r WHERE r.tenant_id=$1 AND `+facilityCol+`=$2 AND r.deleted_at IS NULL AND ($3='' OR r.id>NULLIF($3,'')::uuid) AND ($5='' OR r.name ILIKE '%'||$5||'%' OR r.code ILIKE '%'||$5||'%') AND ($6='all' OR r.status=$6)`+where+` ORDER BY r.id LIMIT $4`, args...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			item := Item{Resource: Resource{Kind: f.CandidateKind}}
			var b []byte
			if e = rows.Scan(&b); e != nil {
				return e
			}
			if e = json.Unmarshal(b, &item.Resource); e != nil {
				return e
			}
			out.Candidates.Items = append(out.Candidates.Items, item)
		}
		if len(out.Candidates.Items) > f.Candidate.Limit {
			out.Candidates.Items = out.Candidates.Items[:f.Candidate.Limit]
			out.Candidates.NextCursor = accessNextCursor(auth, out.Policy, facility, category, out.Candidates.Items[f.Candidate.Limit-1].ID)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if out.Link != nil && len(out.Candidates.Items) > 0 {
			ids := []string{}
			for _, item := range out.Candidates.Items {
				ids = append(ids, item.ID)
			}
			out.CandidateGrants, e = currentGrants(ctx, tx, out.Link.ID, nil, ids, false)
		}
		return e
	})
	return out, e
}
func (s *Service) AccessReceipt(ctx context.Context, a access.Actor, tenant, project, id string) (AccessResult, error) {
	out := AccessResult{}
	e := s.transact(ctx, a, access.ResourceAccessOperation("request", "detail"), TenantScope(tenant), false, func(tx pgx.Tx, auth authorization) error {
		if !access.ValidID(project) || !access.ValidID(id) {
			return notFound()
		}
		e := tx.QueryRow(ctx, `SELECT result FROM project_resource_operation_receipts WHERE entry_scope='tenant' AND tenant_id=$1 AND project_id=$2 AND actor_id=$3 AND request_id=$4 AND tenant_membership_id=$5`, tenant, project, a.ID, id, auth.TenantMembership).Scan(&out)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		out.Replayed = e == nil
		return e
	})
	return out, e
}
func (s *Service) AccessChanges(ctx context.Context, a access.Actor, tenant, project string, f AccessFilter) (AccessChanges, error) {
	out := AccessChanges{Items: []AccessChange{}}
	e := s.transact(ctx, a, access.ResourceAccessOperation("access", "changes"), TenantScope(tenant), false, func(tx pgx.Tx, auth authorization) error {
		p, e := policyTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		if f.Limit == 0 {
			f.Limit = 25
		}
		if f.Limit < 1 || f.Limit > 100 {
			return problem("invalid_request", "分页大小须为 1–100。", 400)
		}
		id, e := accessPageCursor(auth, p, "", "history", f.Cursor)
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT id::text,access_revision::text,operation,actor_id::text,actor_scope,reason,before,after,changed_at FROM project_resource_access_changes WHERE tenant_id=$1 AND project_id=$2 AND ($3='' OR access_revision<(SELECT access_revision FROM project_resource_access_changes WHERE id=NULLIF($3,'')::uuid AND tenant_id=$1 AND project_id=$2)) ORDER BY access_revision DESC LIMIT $4`, tenant, project, id, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v := AccessChange{}
			if e = rows.Scan(&v.ID, &v.Revision, &v.Operation, &v.ActorID, &v.ActorScope, &v.Reason, &v.Before, &v.After, &v.ChangedAt); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			out.NextCursor = accessNextCursor(auth, p, "", "history", out.Items[f.Limit-1].ID)
		}
		return rows.Err()
	})
	return out, e
}
