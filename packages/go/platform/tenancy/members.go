package tenancy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// Profile deliberately excludes platform governance and account security metadata.
type Profile struct {
	Summary
	Description            string    `json:"description"`
	Administrator          Account   `json:"administrator"`
	AdministratorAvailable bool      `json:"administrator_available"`
	UpdatedAt              time.Time `json:"updated_at"`
	Version                string    `json:"version"`
}

func (t Tenant) Profile() Profile {
	return Profile{t.Summary, t.Description, t.Administrator, t.AdministratorAvailable, t.UpdatedAt, t.Version}
}

type RoleGrant struct {
	Permissions   []access.Code `json:"permission_codes"`
	RoleID        string        `json:"role_id"`
	Status        string        `json:"status"`
	Version       string        `json:"version"`
	OwnerTenantID string        `json:"owner_tenant_id"`
	ID            string        `json:"binding_id"`
	Name          string        `json:"name"`
	Key           string        `json:"key"`
	AssignedAt    time.Time     `json:"assigned_at"`
	AssignedBy    string        `json:"assigned_by"`
	RevokedAt     *time.Time    `json:"revoked_at"`
	RevokedBy     *string       `json:"revoked_by"`
}
type Member struct {
	ID              string      `json:"id"`
	TenantID        string      `json:"tenant_id"`
	Account         Account     `json:"account"`
	Status          string      `json:"membership_status"`
	Available       bool        `json:"account_available"`
	Administrator   bool        `json:"is_current_administrator"`
	Roles           []RoleGrant `json:"roles"`
	RoleEffective   bool        `json:"role_effective"`
	JoinedAt        time.Time   `json:"joined_at"`
	JoinedBy        string      `json:"joined_by"`
	StatusChangedAt time.Time   `json:"status_changed_at"`
	StatusChangedBy string      `json:"status_changed_by"`
	StatusReason    string      `json:"status_reason"`
	Version         string      `json:"version"`
	EndedAt         *time.Time  `json:"ended_at"`
	EndReason       string      `json:"end_reason"`
	InvitationID    string      `json:"source_invitation_id"`
}
type MemberInput struct {
	Version string `json:"expected_version"`
	Reason  string `json:"reason"`
}
type MembershipResult struct {
	TenantID     string `json:"tenant_id"`
	MembershipID string `json:"membership_id"`
	Status       string `json:"membership_status"`
	Version      string `json:"membership_version"`
	Revision     string `json:"authorization_revision"`
}

const memberSelect = `SELECT m.id::text,m.tenant_id::text,a.id::text,a.username,a.display_name,m.status,a.status='active',m.id=t.administrator_membership_id,
 m.joined_at,m.joined_by::text,m.status_changed_at,m.status_changed_by::text,m.status_reason,m.version::text,m.ended_at,COALESCE(m.end_reason,''),COALESCE(m.source_invitation_id::text,''),
 COALESCE((SELECT jsonb_agg(jsonb_build_object('binding_id',mr.id,'name',r.name,'key',COALESCE(r.builtin_key,''),'role_id',r.id,'permission_codes',COALESCE((SELECT jsonb_agg(rp.permission_code ORDER BY rp.permission_code) FROM role_permissions rp JOIN permission_catalog p ON p.code=rp.permission_code WHERE rp.role_id=r.id AND p.lifecycle='active'),'[]'::jsonb),'status',r.status,'version',r.version::text,'owner_tenant_id',COALESCE(r.owner_tenant_id::text,''),'assigned_at',mr.assigned_at,'assigned_by',mr.assigned_by,'revoked_at',mr.revoked_at,'revoked_by',mr.revoked_by) ORDER BY mr.assigned_at,mr.id) FROM tenant_member_roles mr JOIN roles r ON r.id=mr.role_id WHERE mr.membership_id=m.id AND mr.tenant_id=m.tenant_id),'[]'::jsonb)
 FROM tenant_memberships m JOIN accounts a ON a.id=m.account_id JOIN tenants t ON t.id=m.tenant_id`

func scanMember(row pgx.Row) (Member, error) {
	v := Member{}
	e := row.Scan(&v.ID, &v.TenantID, &v.Account.ID, &v.Account.Username, &v.Account.DisplayName, &v.Status, &v.Available, &v.Administrator, &v.JoinedAt, &v.JoinedBy, &v.StatusChangedAt, &v.StatusChangedBy, &v.StatusReason, &v.Version, &v.EndedAt, &v.EndReason, &v.InvitationID, &v.Roles)
	v.RoleEffective = v.Available && v.Status == "active"
	return v, e
}
func memberTx(ctx context.Context, tx pgx.Tx, tenant, id string) (Member, error) {
	if !access.ValidID(id) {
		return Member{}, errMember
	}
	v, e := scanMember(tx.QueryRow(ctx, memberSelect+` WHERE m.tenant_id=$1 AND m.id=$2`, tenant, id))
	if e == pgx.ErrNoRows {
		e = errMember
	}
	return v, e
}

var errMember = problem("member_not_found", "成员不存在或不属于当前租户。", 404)

type pageCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func cursorFor(at time.Time, id string) string {
	b, _ := json.Marshal(pageCursor{at, id})
	return base64.RawURLEncoding.EncodeToString(b)
}
func membershipFilter(f *Filter) (*time.Time, string, error) {
	raw := f.Cursor
	f.Cursor = ""
	if e := f.validate(false); e != nil {
		return nil, "", e
	}
	if raw == "" {
		return nil, "", nil
	}
	b, e := base64.RawURLEncoding.DecodeString(raw)
	var c pageCursor
	if e != nil || json.Unmarshal(b, &c) != nil || !access.ValidID(c.ID) || c.At.IsZero() {
		return nil, "", access.Validation("cursor", "分页位置无效。")
	}
	return &c.At, c.ID, nil
}
func (s *Service) Members(ctx context.Context, a access.Actor, tenant string, f Filter) (access.Page[Member], error) {
	out := access.Page[Member]{Items: []Member{}}
	e := s.transact(ctx, a, access.OpTenantMemberList, tenant, "", func(tx pgx.Tx, _ string) error {
		at, id, e := membershipFilter(&f)
		if e != nil {
			return e
		}
		if !oneOf(f.Status, "", "active", "suspended", "ended", "all") || !oneOf(f.Role, "", "tenant_admin", "tenant_member") {
			return access.Validation("filter", "成员筛选无效。")
		}
		rows, e := tx.Query(ctx, memberSelect+` WHERE m.tenant_id=$1 AND (($2='' AND m.status<>'ended') OR $2='all' OR m.status=$2)
 AND ($3='' OR a.username ILIKE '%'||$3||'%' OR a.display_name ILIKE '%'||$3||'%')
 AND ($4='' OR EXISTS(SELECT 1 FROM tenant_member_roles mr JOIN roles r ON r.id=mr.role_id WHERE mr.membership_id=m.id AND r.builtin_key=$4 AND (m.status='ended' OR mr.revoked_at IS NULL)))
 AND ($5::timestamptz IS NULL OR (m.joined_at,m.id)<($5,$6::uuid)) ORDER BY m.joined_at DESC,m.id DESC LIMIT $7`, tenant, f.Status, f.Query, f.Role, at, nullable(id), f.Limit+1)
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
			v := out.Items[f.Limit-1]
			out.NextCursor = cursorFor(v.JoinedAt, v.ID)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) Member(ctx context.Context, a access.Actor, tenant, id string) (Member, error) {
	var out Member
	e := s.transact(ctx, a, access.OpTenantMemberDetail, tenant, "", func(tx pgx.Tx, _ string) error { var e error; out, e = memberTx(ctx, tx, tenant, id); return e })
	return out, e
}
func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Service) ChangeMember(ctx context.Context, a access.Actor, tenant, id string, op access.Operation, in MemberInput, m access.Meta) (MembershipResult, error) {
	var out MembershipResult
	if !oneOf(string(op), string(access.OpTenantMemberSuspend), string(access.OpTenantMemberResume), string(access.OpTenantMemberRemove)) {
		return out, access.ErrDenied
	}
	e := s.transact(ctx, a, op, tenant, id, func(tx pgx.Tx, _ string) error {
		old, e := memberTx(ctx, tx, tenant, id)
		if e != nil {
			return e
		}
		if old.Administrator {
			return problem("tenant_admin_transfer_required", "请先由平台交接当前管理员职责。", 409)
		}
		if e = access.RequireTenantTargetCoverageTx(ctx, tx, a.ID, tenant, id); e != nil {
			return e
		}
		if e = version(old.Version, in.Version); e != nil {
			return e
		}
		in.Reason = strings.TrimSpace(in.Reason)
		if e = access.ValidReason(in.Reason); e != nil {
			return e
		}
		status, event := "suspended", "tenant.member.suspended"
		switch op {
		case access.OpTenantMemberSuspend:
			if old.Status != "active" {
				return errMemberState
			}
		case access.OpTenantMemberResume:
			if old.Status != "suspended" {
				return errMemberState
			}
			if !old.Available {
				return errTarget
			}
			status, event = "active", "tenant.member.resumed"
		case access.OpTenantMemberRemove:
			if old.Status == "ended" {
				return errMemberState
			}
			status, event = "ended", "tenant.member.removed"
		}
		if e = changeMemberTx(ctx, tx, tenant, id, status, a.ID, in.Reason); e != nil {
			return e
		}
		if status != "active" {
			if e = access.InvalidateTenantInvitations(ctx, tx, access.InvitationRevocation{InviterMembershipID: id}, meta(a, m, tenant, "member_unavailable", "tenant")); e != nil {
				return e
			}
		}
		if e = access.Bump(ctx, tx); e != nil {
			return e
		}
		v, e := memberTx(ctx, tx, tenant, id)
		if e != nil {
			return e
		}
		if e = access.Audit(ctx, tx, event, "tenant_member", id, "", old, v, meta(a, m, tenant, in.Reason, "tenant")); e != nil {
			return e
		}
		out = MembershipResult{TenantID: tenant, MembershipID: id, Status: v.Status, Version: v.Version}
		return tx.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&out.Revision)
	})
	return out, e
}

var errMemberState = problem("member_state_conflict", "成员状态已变化，请刷新后重新确认。", 409)
var errTarget = problem("target_account_unavailable", "目标账号不可用。", 409)

func changeMemberTx(ctx context.Context, tx pgx.Tx, tenant, id, status, actor, reason string) error {
	if status != "active" {
		if e := access.ProtectProjectAdministrator(ctx, tx, "", id); e != nil {
			return e
		}
	}
	if status == "ended" {
		if e := access.CascadeProjectMemberships(ctx, tx, id, access.Meta{ActorID: actor, TenantID: tenant, ActorScope: "tenant", Reason: reason}); e != nil {
			return e
		}
	}
	if status == "ended" {
		if _, e := tx.Exec(ctx, `UPDATE tenant_member_roles SET revoked_at=clock_timestamp(),revoked_by=$3 WHERE tenant_id=$1 AND membership_id=$2 AND revoked_at IS NULL`, tenant, id, actor); e != nil {
			return e
		}
	}
	_, e := tx.Exec(ctx, `UPDATE tenant_memberships SET status=$3,version=version+1,status_changed_at=clock_timestamp(),status_changed_by=$4,status_reason=$5,
 ended_at=CASE WHEN $3='ended' THEN clock_timestamp() ELSE NULL END,ended_by=CASE WHEN $3='ended' THEN $4::uuid ELSE NULL END,end_reason=CASE WHEN $3='ended' THEN $5 ELSE NULL END WHERE tenant_id=$1 AND id=$2`, tenant, id, status, actor, reason)
	return e
}
