package projects

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const projectSelect = `SELECT p.id::text,p.tenant_id::text,t.name,p.name,p.code,p.status,p.version::text,
 om.id::text,oa.id::text,oa.username,oa.display_name,(om.status='active' AND oa.status='active'),
 COALESCE(am.tenant_membership_id::text,''),COALESCE(aa.id::text,''),COALESCE(aa.username,''),COALESCE(aa.display_name,''),COALESCE(am.status='active' AND atm.status='active' AND aa.status='active',false),
 COALESCE(p.administrator_membership_id::text,''),p.administrator_membership_id IS NULL,COALESCE(p.planned_start::text,''),COALESCE(p.planned_end::text,''),p.created_at,p.updated_at,
 p.purpose,p.description,p.created_by::text,p.updated_by::text,COALESCE(p.archived_at::text,''),COALESCE(p.archived_by::text,''),COALESCE(p.archive_reason,''),COALESCE(p.management_gap_reason,'')
 FROM projects p JOIN tenants t ON t.id=p.tenant_id JOIN tenant_memberships om ON om.id=p.owner_tenant_membership_id JOIN accounts oa ON oa.id=om.account_id
 LEFT JOIN project_memberships am ON am.id=p.administrator_membership_id LEFT JOIN tenant_memberships atm ON atm.id=am.tenant_membership_id LEFT JOIN accounts aa ON aa.id=am.account_id`

func scanProject(row pgx.Row) (Project, error) {
	var v Project
	e := row.Scan(&v.ID, &v.TenantID, &v.TenantName, &v.Name, &v.Code, &v.Status, &v.Version, &v.Owner.MembershipID, &v.Owner.AccountID, &v.Owner.Username, &v.Owner.DisplayName, &v.Owner.Available, &v.Administrator.MembershipID, &v.Administrator.AccountID, &v.Administrator.Username, &v.Administrator.DisplayName, &v.Administrator.Available, &v.AdministratorMembershipID, &v.ManagementGap, &v.PlannedStart, &v.PlannedEnd, &v.CreatedAt, &v.UpdatedAt, &v.Purpose, &v.Description, &v.CreatedBy, &v.UpdatedBy, &v.ArchivedAt, &v.ArchivedBy, &v.ArchiveReason, &v.ManagementGapReason)
	if e == pgx.ErrNoRows {
		return v, access.ErrProjectNotFound
	}
	return v, e
}
func projectTx(ctx context.Context, tx pgx.Tx, tenant, project string) (Project, error) {
	return scanProject(tx.QueryRow(ctx, projectSelect+` WHERE p.tenant_id=$1 AND p.id=$2`, tenant, project))
}
func minimal(v Summary) Summary {
	return Summary{ID: v.ID, TenantID: v.TenantID, TenantName: v.TenantName, Name: v.Name, Code: v.Code, Status: v.Status, Version: v.Version}
}
func (s *Service) Context(ctx context.Context, a access.Actor, tenant, project string) (Context, error) {
	return s.context(ctx, a, tenant, project, access.OpProjectContext)
}
func (s *Service) Availability(ctx context.Context, a access.Actor, tenant, project string) (Context, error) {
	return s.context(ctx, a, tenant, project, access.OpProjectAvailability)
}
func (s *Service) context(ctx context.Context, a access.Actor, tenant, project string, op access.Operation) (Context, error) {
	var out Context
	e := s.transact(ctx, a, op, tenant, project, func(tx pgx.Tx, rev string, auth access.ProjectAuthorization) error {
		v, e := projectTx(ctx, tx, tenant, project)
		if e != nil {
			return e
		}
		out = Context{Project: minimal(v.Summary), Authorization: auth, CanEnter: access.WorkspacePath("project", tenant, project, v.Status, auth.Permissions) != ""}
		var allow bool
		if e = tx.QueryRow(ctx, `SELECT revision::text,allow_facility_create FROM project_resource_policies WHERE tenant_id=$1 AND project_id=$2`, tenant, project).Scan(&out.ResourceAccessRevision, &allow); e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `SELECT revision::text FROM project_device_access_states WHERE tenant_id=$1 AND project_id=$2`, tenant, project).Scan(&out.DeviceAccessRevision); e != nil {
			return e
		}
		out.CanCreateFacility = allow && slices.Contains(auth.AllowedOperations, access.ProjectResourceOperation("facility", "create"))

		return nil
	})
	return out, e
}
func (s *Service) Profile(ctx context.Context, a access.Actor, tenant, project string) (Project, error) {
	var out Project
	e := s.transact(ctx, a, access.OpProjectProfile, tenant, project, func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		var e error
		out, e = projectTx(ctx, tx, tenant, project)
		return e
	})
	return out, e
}
func (s *Service) Governance(ctx context.Context, a access.Actor, tenant, project string) (Summary, error) {
	var out Summary
	e := s.transact(ctx, a, access.OpProjectGovernanceDetail, tenant, project, func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		p, e := projectTx(ctx, tx, tenant, project)
		out = p.Summary
		return e
	})
	return out, e
}

type cursor struct {
	Time time.Time
	ID   string
}

func pageCursor(t time.Time, id string) string {
	b, _ := json.Marshal(cursor{t, id})
	return base64.RawURLEncoding.EncodeToString(b)
}
func (f *Filter) validate() (cursor, error) {
	var c cursor
	f.Query = strings.TrimSpace(f.Query)
	if utf8.RuneCountInString(f.Query) > 100 {
		return c, access.Validation("q", "搜索最多 100 字符。")
	}
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit < 1 || f.Limit > 100 {
		return c, access.Validation("limit", "每页 1–100 条。")
	}
	if f.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		if e != nil || json.Unmarshal(b, &c) != nil || !access.ValidID(c.ID) || c.Time.IsZero() {
			return c, access.Validation("cursor", "分页位置无效。")
		}
	}
	return c, nil
}
func (s *Service) List(ctx context.Context, a access.Actor, tenant string, f Filter, mine bool) (access.Page[Context], error) {
	out := access.Page[Context]{Items: []Context{}}
	op := access.OpProjectGovernanceList
	if mine {
		op = access.OpMyProjects
	}
	e := s.transact(ctx, a, op, tenant, "", func(tx pgx.Tx, rev string, _ access.ProjectAuthorization) error {
		cur, e := f.validate()
		if e != nil {
			return e
		}
		if f.Status == "" && !mine {
			f.Status = "active"
		}
		if f.Status != "" && f.Status != "active" && f.Status != "archived" && f.Status != "all" {
			return access.Validation("status", "状态无效。")
		}
		if f.Owner != "" && !access.ValidID(f.Owner) {
			return access.Validation("owner", "负责人无效。")
		}
		if f.Unavailable != "" && f.Unavailable != "true" && f.Unavailable != "false" {
			return access.Validation("administrator_unavailable", "筛选值无效。")
		}
		rows, e := tx.Query(ctx, projectSelect+` WHERE ($1='' OR p.tenant_id::text=$1) AND ($2='' OR p.name ILIKE '%'||$2||'%' OR p.code ILIKE '%'||$2||'%') AND ($3 IN ('','all') OR p.status=$3)
 AND ($4='' OR p.owner_tenant_membership_id::text=$4) AND ($5='' OR COALESCE(am.status='active' AND atm.status='active' AND aa.status='active',false)=($5='false'))
 AND ($6='' OR (p.created_at,p.id)<($7::timestamptz,NULLIF($6,'')::uuid))
 AND (NOT $8 OR EXISTS(SELECT 1 FROM project_memberships pm JOIN tenant_memberships tm ON tm.id=pm.tenant_membership_id WHERE pm.project_id=p.id AND pm.account_id=$9 AND pm.status<>'ended' AND tm.status='active')) ORDER BY p.created_at DESC,p.id DESC LIMIT $10`, tenant, f.Query, f.Status, f.Owner, f.Unavailable, cur.ID, cur.Time, mine, a.ID, f.Limit+1)
		if e != nil {
			return e
		}
		list := []Project{}
		for rows.Next() {
			v, e := scanProject(rows)
			if e != nil {
				rows.Close()
				return e
			}
			list = append(list, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(list) > f.Limit {
			list = list[:f.Limit]
			last := list[len(list)-1]
			out.NextCursor = pageCursor(last.CreatedAt, last.ID)
		}
		for _, p := range list {
			v := Context{Project: p.Summary, Authorization: access.ProjectAuthorization{
				Snapshot:              access.Snapshot{Revision: rev, Permissions: []access.Code{}, Roles: []access.Role{}, Sources: map[access.Code][]string{}},
				ConfiguredPermissions: []access.Code{}, RestorablePermissions: []access.Code{}, InactiveReasons: []string{}, AllowedOperations: []access.Operation{}, Bindings: []access.ProjectBinding{},
			}}
			if mine {
				v.Authorization, e = access.ProjectSnapshotTx(ctx, tx, a.ID, p.TenantID, p.ID, rev)
				if e != nil {
					return e
				}
				v.CanEnter = access.WorkspacePath("project", p.TenantID, p.ID, p.Status, v.Authorization.Permissions) != ""
				v.Project = minimal(p.Summary)
			}
			out.Items = append(out.Items, v)
		}
		return nil
	})
	return out, e
}
func (s *Service) Candidates(ctx context.Context, a access.Actor, tenant, project string, op access.Operation, q string) (access.Page[Person], error) {
	out := access.Page[Person]{Items: []Person{}}
	allowed := map[access.Operation]bool{access.OpProjectCreateCandidates: true, access.OpProjectRecoveryCandidates: true, access.OpProjectMemberCandidates: true, access.OpProjectOwnerCandidates: true, access.OpProjectAdministratorCandidates: true}
	if !allowed[op] {
		return out, access.ErrDenied
	}
	e := s.transact(ctx, a, op, tenant, project, func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		q = strings.TrimSpace(q)
		if utf8.RuneCountInString(q) < 2 || utf8.RuneCountInString(q) > 100 {
			return access.Validation("q", "请输入 2–100 字符搜索本租户有效成员。")
		}
		rows, e := tx.Query(ctx, `SELECT tm.id::text,a.id::text,a.username,a.display_name FROM tenant_memberships tm JOIN accounts a ON a.id=tm.account_id WHERE tm.tenant_id=$1 AND tm.status='active' AND a.status='active' AND (a.username ILIKE '%'||$2||'%' OR a.display_name ILIKE '%'||$2||'%') ORDER BY a.username_normalized,tm.id LIMIT 20`, tenant, q)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v Person
			v.Available = true
			if e = rows.Scan(&v.MembershipID, &v.AccountID, &v.Username, &v.DisplayName); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		return rows.Err()
	})
	return out, e
}

const memberSelect = `SELECT m.id::text,m.tenant_membership_id::text,m.account_id::text,a.username,a.display_name,a.status,tm.status,m.status,m.version::text,COALESCE(m.id=p.administrator_membership_id,false),m.joined_at,m.status_reason,COALESCE(m.ended_at::text,'') FROM project_memberships m JOIN accounts a ON a.id=m.account_id JOIN tenant_memberships tm ON tm.id=m.tenant_membership_id JOIN projects p ON p.id=m.project_id`

func scanMember(row pgx.Row) (Member, error) {
	var v Member
	e := row.Scan(&v.ID, &v.TenantMembershipID, &v.AccountID, &v.Username, &v.DisplayName, &v.AccountStatus, &v.TenantMembershipStatus, &v.Status, &v.Version, &v.Administrator, &v.JoinedAt, &v.StatusReason, &v.EndedAt)
	if e == pgx.ErrNoRows {
		return v, access.ErrNotFound
	}
	return v, e
}
func memberTx(ctx context.Context, tx pgx.Tx, tenant, project, id string) (Member, error) {
	if !access.ValidID(id) {
		return Member{}, access.ErrNotFound
	}
	return scanMember(tx.QueryRow(ctx, memberSelect+` WHERE m.tenant_id=$1 AND m.project_id=$2 AND m.id=$3`, tenant, project, id))
}
func (s *Service) Members(ctx context.Context, a access.Actor, tenant, project, role string, f Filter) (access.Page[Member], error) {
	out := access.Page[Member]{Items: []Member{}}
	op := access.OpProjectMembers
	if role != "" {
		op = access.OpProjectRoleMembers
	}
	e := s.transact(ctx, a, op, tenant, project, func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		cur, e := f.validate()
		if e != nil {
			return e
		}
		if f.Status != "" && f.Status != "all" && f.Status != "active" && f.Status != "suspended" && f.Status != "ended" {
			return access.Validation("status", "状态无效。")
		}
		if role != "" {
			if _, e = access.ProjectRoleTx(ctx, tx, tenant, project, role); e != nil {
				return e
			}
		}
		rows, e := tx.Query(ctx, memberSelect+` WHERE m.tenant_id=$1 AND m.project_id=$2 AND (($3='' AND m.status<>'ended') OR $3='all' OR m.status=$3) AND ($4='' OR a.username ILIKE '%'||$4||'%' OR a.display_name ILIKE '%'||$4||'%') AND ($5='' OR (m.joined_at,m.id)<($6::timestamptz,NULLIF($5,'')::uuid)) AND ($7='' OR EXISTS(SELECT 1 FROM project_member_roles mr WHERE mr.membership_id=m.id AND mr.role_id::text=$7 AND mr.revoked_at IS NULL)) ORDER BY m.joined_at DESC,m.id DESC LIMIT $8`, tenant, project, f.Status, f.Query, cur.ID, cur.Time, role, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := scanMember(rows)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			last := out.Items[len(out.Items)-1]
			out.NextCursor = pageCursor(last.JoinedAt, last.ID)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) Member(ctx context.Context, a access.Actor, tenant, project, id string) (Member, error) {
	var v Member
	e := s.transact(ctx, a, access.OpProjectMember, tenant, project, func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		var e error
		v, e = memberTx(ctx, tx, tenant, project, id)
		return e
	})
	return v, e
}
func (s *Service) Authorization(ctx context.Context, a access.Actor, tenant, project, id string) (access.ProjectAuthorization, error) {
	var v access.ProjectAuthorization
	e := s.transact(ctx, a, access.OpProjectAuthorization, tenant, project, func(tx pgx.Tx, rev string, _ access.ProjectAuthorization) error {
		var e error
		v, e = access.ProjectAuthorizationTx(ctx, tx, tenant, project, id, rev)
		return e
	})
	return v, e
}
func (s *Service) Permissions(ctx context.Context, a access.Actor, tenant, project string) ([]access.Permission, error) {
	out := []access.Permission{}
	e := s.transact(ctx, a, access.OpProjectPermissions, tenant, project, func(_ pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		for _, p := range access.Catalog() {
			if p.Scope == "project" {
				out = append(out, p)
			}
		}
		return nil
	})
	return out, e
}
func (s *Service) Roles(ctx context.Context, a access.Actor, tenant, project string, f Filter, presets bool) (access.Page[access.Role], error) {
	out := access.Page[access.Role]{Items: []access.Role{}}
	op := access.OpProjectRoles
	if presets {
		op = access.OpProjectPresetList
	}
	e := s.transact(ctx, a, op, tenant, project, func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		cur, e := f.validate()
		if e != nil {
			return e
		}
		if f.Status != "" && f.Status != "active" && f.Status != "disabled" {
			return access.Validation("status", "状态无效。")
		}
		rows, e := tx.Query(ctx, `SELECT id::text FROM roles WHERE binding_scope='project' AND deleted_at IS NULL AND (owner_project_id IS NULL OR owner_project_id::text=$1) AND ($2='' OR name ILIKE '%'||$2||'%') AND ($3='' OR status=$3) AND ($4='' OR (created_at,id)<($5::timestamptz,NULLIF($4,'')::uuid)) ORDER BY created_at DESC,id DESC LIMIT $6`, project, f.Query, f.Status, cur.ID, cur.Time, f.Limit+1)
		if e != nil {
			return e
		}
		list, e := ids(rows)
		if e != nil {
			return e
		}
		more := len(list) > f.Limit
		if more {
			list = list[:f.Limit]
		}
		for _, id := range list {
			r, e := access.ProjectRoleTx(ctx, tx, tenant, project, id)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, r)
		}
		if more {
			last := out.Items[len(out.Items)-1]
			out.NextCursor = pageCursor(last.CreatedAt, last.ID)
		}
		return nil
	})
	return out, e
}
func (s *Service) Role(ctx context.Context, a access.Actor, tenant, project, id string, preset bool) (access.Role, error) {
	var out access.Role
	op := access.OpProjectRole
	if preset {
		op = access.OpProjectPresetDetail
	}
	e := s.transact(ctx, a, op, tenant, project, func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		var e error
		out, e = access.ProjectRoleTx(ctx, tx, tenant, project, id)
		return e
	})
	return out, e
}
func (s *Service) Audit(ctx context.Context, a access.Actor, tenant, project string, f Filter) (access.Page[access.AuditEvent], error) {
	out := access.Page[access.AuditEvent]{Items: []access.AuditEvent{}}
	e := s.transact(ctx, a, access.OpProjectAudit, tenant, project, func(tx pgx.Tx, _ string, _ access.ProjectAuthorization) error {
		raw := f.Cursor
		f.Cursor = ""
		if _, e := f.validate(); e != nil {
			return e
		}
		before := int64(9223372036854775807)
		if raw != "" {
			var e error
			before, e = strconv.ParseInt(raw, 10, 64)
			if e != nil || before < 1 {
				return access.Validation("cursor", "分页位置无效。")
			}
		}
		var from, to *time.Time
		for _, p := range []struct {
			raw string
			out **time.Time
		}{{f.From, &from}, {f.To, &to}} {
			if p.raw != "" {
				v, e := time.Parse(time.RFC3339, p.raw)
				if e != nil {
					return access.Validation("time", "时间格式无效。")
				}
				*p.out = &v
			}
		}
		if from != nil && to != nil && from.After(*to) {
			return access.Validation("time", "时间范围无效。")
		}
		rows, e := tx.Query(ctx, `SELECT id::text,tenant_id::text,project_id::text,actor_scope,event_type,actor_type,COALESCE(actor_id::text,''),object_type,object_id,reason,request_id,created_at,details FROM account_security_events WHERE tenant_id=$1 AND project_id=$2 AND id<$3 AND ($4='' OR event_type=$4) AND ($5='' OR object_id=$5) AND ($6='' OR actor_id::text=$6) AND ($7='' OR reason ILIKE '%'||$7||'%' OR event_type ILIKE '%'||$7||'%') AND ($8::timestamptz IS NULL OR created_at>=$8) AND ($9::timestamptz IS NULL OR created_at<=$9) ORDER BY id DESC LIMIT $10`, tenant, project, before, f.Event, f.ObjectID, f.ActorID, f.Query, from, to, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v access.AuditEvent
			if e = rows.Scan(&v.ID, &v.TenantID, &v.ProjectID, &v.ActorScope, &v.EventType, &v.ActorType, &v.ActorID, &v.ObjectType, &v.ObjectID, &v.Reason, &v.RequestID, &v.CreatedAt, &v.Details); e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		if len(out.Items) > f.Limit {
			out.Items = out.Items[:f.Limit]
			out.NextCursor = out.Items[len(out.Items)-1].ID
		}
		return rows.Err()
	})
	return out, e
}
