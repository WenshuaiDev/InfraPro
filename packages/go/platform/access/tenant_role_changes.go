package access

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type TenantChange struct {
	ExpectedVersion           string            `json:"expected_version"`
	ExpectedMembershipVersion string            `json:"expected_membership_version"`
	Name                      string            `json:"name"`
	Description               string            `json:"description"`
	Permissions               []Code            `json:"permission_codes"`
	Status                    string            `json:"status"`
	RoleIDs                   []string          `json:"role_ids"`
	RoleVersions              map[string]string `json:"role_versions"`
	PreviewHash               string            `json:"preview_hash"`
	Reason                    string            `json:"reason"`
}
type TenantMemberImpact struct {
	InvitationGrantLost    bool                `json:"invitation_grant_lost"`
	Before                 TenantAuthorization `json:"before"`
	After                  TenantAuthorization `json:"after"`
	AddedPermissions       []Code              `json:"added_permissions"`
	RemovedPermissions     []Code              `json:"removed_permissions"`
	RetainedPermissions    []Code              `json:"retained_permissions"`
	AddedRoles             []string            `json:"added_role_ids"`
	RemovedRoles           []string            `json:"removed_role_ids"`
	InvalidatedInvitations int                 `json:"invalidated_invitations"`
}
type TenantImpact struct {
	Hash                   string                   `json:"preview_hash"`
	CatalogChecksum        string                   `json:"catalog_checksum"`
	TargetVersion          string                   `json:"target_version"`
	RoleVersions           map[string]string        `json:"role_versions"`
	BoundMembers           int                      `json:"bound_members"`
	EffectiveMembers       int                      `json:"effective_members"`
	GainingMembers         int                      `json:"gaining_members"`
	LosingMembers          int                      `json:"losing_members"`
	RestorableMembers      int                      `json:"restorable_members"`
	InvalidatedInvitations int                      `json:"invalidated_invitations"`
	BlockedReasons         []string                 `json:"blocked_reasons"`
	Members                Page[TenantMemberImpact] `json:"members"`
}
type TenantMutationResult struct {
	Role          *TenantRole          `json:"role,omitempty"`
	Authorization *TenantAuthorization `json:"authorization,omitempty"`
	Impact        TenantImpact         `json:"impact"`
	Revision      string               `json:"authorization_revision"`
}
type pendingTenantInvitation struct {
	ID        string
	Version   string
	ExpiresAt time.Time
	Live      bool
}
type tenantChangePlan struct {
	Impact    TenantImpact
	OldRole   TenantRole
	NewRole   TenantRole
	Members   []TenantMemberImpact
	Pending   map[string][]pendingTenantInvitation
	Operation Operation
}

func difference[T comparable](a, b []T) []T {
	v := []T{}
	for _, x := range a {
		if !slices.Contains(b, x) {
			v = append(v, x)
		}
	}
	return v
}
func roleIDs(rs []Role) []string {
	v := []string{}
	for _, r := range rs {
		v = append(v, r.ID)
	}
	slices.Sort(v)
	return v
}
func previewOperation(op Operation) Operation {
	switch op {
	case OpTenantRolePermissionsPreview:
		return OpTenantRolePermissions
	case OpTenantRoleStatusPreview:
		return OpTenantRoleStatus
	case OpTenantRoleArchivePreview:
		return OpTenantRoleArchive
	case OpTenantMemberRolesPreview:
		return OpTenantMemberRoles
	}
	return op
}
func prepareTenantChange(ctx context.Context, tx pgx.Tx, a Actor, tenant, id string, op Operation, in TenantChange, have []Code) (tenantChangePlan, error) {
	p := tenantChangePlan{Operation: previewOperation(op), Members: []TenantMemberImpact{}, Pending: map[string][]pendingTenantInvitation{}, Impact: TenantImpact{CatalogChecksum: Checksum(), RoleVersions: map[string]string{}, BlockedReasons: []string{}, Members: Page[TenantMemberImpact]{Items: []TenantMemberImpact{}}}}
	var ids []string
	catalog, e := tenantCodesTx(ctx, tx)
	if e != nil {
		return p, e
	}
	if p.Operation == OpTenantMemberRoles {
		old, e := TenantAuthorizationTx(ctx, tx, tenant, id, "")
		if e != nil {
			return p, e
		}
		if old.Administrator {
			return p, &Error{Code: "role_protected", Message: "当前管理员只能通过平台交接流程变更。", Status: 409}
		}
		if old.MembershipStatus == "ended" {
			return p, &Error{Code: "member_state_conflict", Message: "已结束的成员关系不可授权。", Status: 409}
		}
		if e = checkVersion(old.MembershipVersion, in.ExpectedMembershipVersion); e != nil {
			return p, e
		}
		if e = within(have, old.ConfiguredPermissions); e != nil {
			return p, e
		}
		if in.RoleIDs == nil || len(in.RoleIDs) > 100 {
			return p, Validation("role_ids", "请提交最多 100 项的完整角色数组。")
		}
		after := old
		after.Roles = []Role{}
		seen := map[string]bool{}
		oldIDs := roleIDs(old.Roles)
		for _, rid := range in.RoleIDs {
			rid = strings.ToLower(rid)
			if !ValidID(rid) || seen[rid] {
				return p, Validation("role_ids", "角色 ID 无效或重复。")
			}
			seen[rid] = true
			r, e := tenantRoleTx(ctx, tx, tenant, rid)
			if e != nil {
				return p, e
			}
			if r.AssignmentPolicy != "manual" {
				return p, &Error{Code: "role_protected", Message: "管理员角色不能通过一般授权分配。", Status: 409}
			}
			if !slices.Contains(oldIDs, rid) {
				if old.AccountStatus != "active" {
					return p, &Error{Code: "target_account_unavailable", Message: "账号不可用时只能撤销角色。", Status: 409}
				}
				if r.Status != "active" {
					return p, &Error{Code: "role_disabled", Message: "停用角色不能新增绑定。", Status: 409}
				}
				if e = validateTenantPermissions(r.Permissions, have); e != nil {
					return p, e
				}
			}
			if e = within(have, r.Permissions); e != nil {
				return p, e
			}
			after.Roles = append(after.Roles, r.Role)
		}
		slices.SortFunc(after.Roles, func(a, b Role) int { return strings.Compare(a.ID, b.ID) })
		explainTenant(&after, catalog)
		p.Members = append(p.Members, TenantMemberImpact{Before: old, After: after})
		p.Impact.TargetVersion = old.MembershipVersion
		ids = []string{id}
	} else {
		p.OldRole, e = tenantRoleTx(ctx, tx, tenant, id)
		if e != nil {
			return p, e
		}
		if !p.OldRole.CanEdit {
			return p, &Error{Code: "role_protected", Message: "平台预置角色由发布流程维护。", Status: 409}
		}
		if e = checkVersion(p.OldRole.Version, in.ExpectedVersion); e != nil {
			return p, e
		}
		if e = within(have, p.OldRole.Permissions); e != nil {
			return p, e
		}
		p.NewRole = p.OldRole
		p.Impact.TargetVersion = p.OldRole.Version
		p.Impact.RoleVersions[id] = p.OldRole.Version
		switch p.Operation {
		case OpTenantRolePermissions:
			if e = validateTenantPermissions(in.Permissions, have); e != nil {
				return p, e
			}
			p.NewRole.Permissions = slices.Clone(in.Permissions)
			slices.Sort(p.NewRole.Permissions)
		case OpTenantRoleStatus:
			if in.Status != "active" && in.Status != "disabled" {
				return p, Validation("status", "角色状态无效。")
			}
			if in.Status == "active" {
				if e = validateTenantPermissions(p.OldRole.Permissions, have); e != nil {
					return p, e
				}
			}
			p.NewRole.Status = in.Status
		case OpTenantRoleArchive:
			if p.OldRole.MemberCount > 0 {
				p.Impact.BlockedReasons = append(p.Impact.BlockedReasons, "角色仍有未撤销绑定，包括暂停成员及不可用账号，请先全部解绑。")
			}
		case OpTenantRoleUpdate:
			p.NewRole.Name, e = tenantRoleName(ctx, tx, in.Name, in.Description)
			if e != nil {
				return p, e
			}
			p.NewRole.Description = in.Description
		default:
			return p, ErrDenied
		}
		rows, e := tx.Query(ctx, `SELECT membership_id::text FROM tenant_member_roles WHERE tenant_id=$1 AND role_id=$2 AND revoked_at IS NULL ORDER BY membership_id`, tenant, id)
		if e != nil {
			return p, e
		}
		ids, e = readIDs(rows)
		if e != nil {
			return p, e
		}
		for _, mid := range ids {
			old, e := TenantAuthorizationTx(ctx, tx, tenant, mid, "")
			if e != nil {
				return p, e
			}
			after := old
			after.Roles = slices.Clone(old.Roles)
			for i, r := range after.Roles {
				if r.ID == id {
					after.Roles[i] = p.NewRole.Role
				}
			}
			explainTenant(&after, catalog)
			p.Members = append(p.Members, TenantMemberImpact{Before: old, After: after})
		}
	}
	p.Impact.BoundMembers = len(ids)
	for i := range p.Members {
		v := &p.Members[i]
		v.AddedPermissions = difference(v.After.Permissions, v.Before.Permissions)
		v.RemovedPermissions = difference(v.Before.Permissions, v.After.Permissions)
		v.RetainedPermissions = difference(v.Before.Permissions, v.RemovedPermissions)
		v.AddedRoles = difference(roleIDs(v.After.Roles), roleIDs(v.Before.Roles))
		v.RemovedRoles = difference(roleIDs(v.Before.Roles), roleIDs(v.After.Roles))
		for _, r := range append(slices.Clone(v.Before.Roles), v.After.Roles...) {
			p.Impact.RoleVersions[r.ID] = r.Version
		}
		active := v.Before.AccountStatus == "active" && v.Before.MembershipStatus == "active"
		if active {
			p.Impact.EffectiveMembers++
		}
		if len(v.AddedPermissions) > 0 {
			p.Impact.GainingMembers++
		}
		if len(v.RemovedPermissions) > 0 {
			p.Impact.LosingMembers++
		}
		if !active && !slices.Equal(v.Before.RestorablePermissions, v.After.RestorablePermissions) {
			p.Impact.RestorableMembers++
		}
		rows, e := tx.Query(ctx, `SELECT id::text,version::text,expires_at,expires_at>clock_timestamp() FROM tenant_invitations WHERE tenant_id=$1 AND inviter_membership_id=$2 AND status='pending' ORDER BY id`, tenant, v.Before.MembershipID)
		if e != nil {
			return p, e
		}
		pending := []pendingTenantInvitation{}
		for rows.Next() {
			var x pendingTenantInvitation
			if e = rows.Scan(&x.ID, &x.Version, &x.ExpiresAt, &x.Live); e != nil {
				rows.Close()
				return p, e
			}
			pending = append(pending, x)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return p, e
		}
		p.Pending[v.Before.MembershipID] = pending
		fixed, e := TenantInvitationFixedCodesTx(ctx, tx)
		if e != nil {
			return p, e
		}
		v.InvitationGrantLost = InvitationGrantEligible(v.Before, fixed) && !InvitationGrantEligible(v.After, fixed)
		if v.InvitationGrantLost {
			for _, x := range pending {
				if x.Live {
					v.InvalidatedInvitations++
				}
			}
		}
		p.Impact.InvalidatedInvitations += v.InvalidatedInvitations
	}
	normalized := in
	normalized.PreviewHash = ""
	normalized.Reason = ""
	normalized.RoleVersions = nil
	normalized.RoleIDs = slices.Clone(in.RoleIDs)
	slices.Sort(normalized.RoleIDs)
	normalized.Permissions = slices.Clone(in.Permissions)
	slices.Sort(normalized.Permissions)
	p.Impact.Hash = hashValue(struct {
		Actor, Tenant, Target string
		Operation             Operation
		Input                 TenantChange
		Role                  TenantRole
		Members               []TenantMemberImpact
		Pending               map[string][]pendingTenantInvitation
		Catalog               string
	}{a.ID, tenant, id, p.Operation, normalized, p.OldRole, p.Members, p.Pending, Checksum()})
	return p, nil
}
func pageTenantImpact(p *tenantChangePlan, f Filter, canRead bool) error {
	if f.Limit == 0 {
		f.Limit = 20
	}
	limit, e := f.validate()
	if e != nil {
		return e
	}
	if f.Cursor != "" && !ValidID(f.Cursor) {
		return Validation("cursor", "分页位置无效。")
	}
	if !canRead {
		return nil
	}
	for _, v := range p.Members {
		if v.Before.MembershipID > f.Cursor {
			p.Impact.Members.Items = append(p.Impact.Members.Items, v)
		}
	}
	if len(p.Impact.Members.Items) > limit {
		p.Impact.Members.Items = p.Impact.Members.Items[:limit]
		p.Impact.Members.NextCursor = p.Impact.Members.Items[limit-1].Before.MembershipID
	}
	return nil
}
func (s *Service) PreviewTenantChange(ctx context.Context, a Actor, tenant, id string, op Operation, in TenantChange, f Filter) (TenantImpact, error) {
	var out TenantImpact
	switch op {
	case OpTenantRolePermissionsPreview, OpTenantRoleStatusPreview, OpTenantRoleArchivePreview, OpTenantMemberRolesPreview:
	default:
		return out, ErrDenied
	}
	e := s.tenantTransaction(ctx, a, op, tenant, false, func(tx pgx.Tx, _ string, have []Code) error {
		p, e := prepareTenantChange(ctx, tx, a, tenant, id, op, in, have)
		if e != nil {
			return e
		}
		if e = pageTenantImpact(&p, f, slices.Contains(have, TenantMemberRead)); e != nil {
			return e
		}
		out = p.Impact
		return nil
	})
	return out, e
}
func (s *Service) ChangeTenantAuthorization(ctx context.Context, a Actor, tenant, id string, op Operation, in TenantChange, m Meta) (TenantMutationResult, error) {
	var out TenantMutationResult
	switch op {
	case OpTenantRoleUpdate, OpTenantRolePermissions, OpTenantRoleStatus, OpTenantRoleArchive, OpTenantMemberRoles:
	default:
		return out, ErrDenied
	}
	e := s.tenantTransaction(ctx, a, op, tenant, true, func(tx pgx.Tx, _ string, have []Code) error {
		if e := ValidReason(in.Reason); e != nil {
			return e
		}
		p, e := prepareTenantChange(ctx, tx, a, tenant, id, op, in, have)
		if e != nil {
			return e
		}
		if len(p.Impact.BlockedReasons) > 0 {
			return &Error{Code: "role_in_use", Message: p.Impact.BlockedReasons[0], Status: 409}
		}
		if op != OpTenantRoleUpdate && in.PreviewHash != p.Impact.Hash {
			return &Error{Code: "impact_changed", Message: "变更影响已变化，请重新预览后确认。", Status: 409}
		}
		if op == OpTenantMemberRoles {
			if in.RoleVersions == nil {
				return Validation("role_versions", "请提交预览返回的角色版本集合。")
			}
			if hashValue(in.RoleVersions) != hashValue(p.Impact.RoleVersions) {
				return ErrConflict
			}
		}
		m = tenantMeta(a, tenant, in.Reason, m)
		changed := true
		switch op {
		case OpTenantMemberRoles:
			v := p.Members[0]
			changed = len(v.AddedRoles)+len(v.RemovedRoles) > 0
			for _, rid := range v.RemovedRoles {
				if _, e = tx.Exec(ctx, `UPDATE tenant_member_roles SET revoked_at=clock_timestamp(),revoked_by=$4 WHERE tenant_id=$1 AND membership_id=$2 AND role_id=$3 AND revoked_at IS NULL`, tenant, id, rid, a.ID); e != nil {
					return e
				}
			}
			for _, rid := range v.AddedRoles {
				if _, e = tx.Exec(ctx, `INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) VALUES($1,$2,$3,$4)`, tenant, id, rid, a.ID); e != nil {
					return e
				}
			}
			if changed {
				if _, e = tx.Exec(ctx, `UPDATE tenant_memberships SET version=version+1 WHERE id=$1 AND tenant_id=$2`, id, tenant); e != nil {
					return e
				}
			}
		case OpTenantRolePermissions:
			changed = !slices.Equal(p.OldRole.Permissions, p.NewRole.Permissions)
			if changed {
				if e = setTenantPermissions(ctx, tx, id, p.NewRole.Permissions); e != nil {
					return e
				}
			}
		case OpTenantRoleStatus:
			changed = p.OldRole.Status != p.NewRole.Status
			if changed {
				_, e = tx.Exec(ctx, `UPDATE roles SET status=$2 WHERE id=$1`, id, p.NewRole.Status)
			}
		case OpTenantRoleUpdate:
			changed = p.OldRole.Name != p.NewRole.Name || p.OldRole.Description != p.NewRole.Description
			if changed {
				_, e = tx.Exec(ctx, `UPDATE roles SET name=$2,name_normalized=$3,description=$4 WHERE id=$1`, id, p.NewRole.Name, strings.ToLower(p.NewRole.Name), p.NewRole.Description)
			}
		case OpTenantRoleArchive:
			_, e = tx.Exec(ctx, `UPDATE roles SET deleted_at=clock_timestamp() WHERE id=$1`, id)
		}
		if e != nil {
			return e
		}
		if changed {
			if op != OpTenantMemberRoles {
				if _, e = tx.Exec(ctx, `UPDATE roles SET version=version+1,updated_by=$2,updated_at=clock_timestamp() WHERE id=$1`, id, a.ID); e != nil {
					return e
				}
			}
			for _, v := range p.Members {
				if v.InvitationGrantLost {
					revocation := m
					revocation.Reason = "invitation_permission_lost"
					if e = InvalidateTenantInvitations(ctx, tx, InvitationRevocation{InviterMembershipID: v.Before.MembershipID}, revocation); e != nil {
						return e
					}
				}
			}
			if e = Bump(ctx, tx); e != nil {
				return e
			}
		}
		if e = tx.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&out.Revision); e != nil {
			return e
		}
		if op == OpTenantMemberRoles {
			v, e := TenantAuthorizationTx(ctx, tx, tenant, id, out.Revision)
			if e != nil {
				return e
			}
			out.Authorization = &v
			if changed {
				if e = Audit(ctx, tx, "tenant.member.roles_changed", "tenant_member", id, "", p.Members[0].Before, v, m); e != nil {
					return e
				}
			}
		} else {
			var after any = map[string]any{"deleted": true, "previous": p.OldRole}
			if op != OpTenantRoleArchive {
				v, e := tenantRoleTx(ctx, tx, tenant, id)
				if e != nil {
					return e
				}
				out.Role = &v
				after = v
			}
			if changed {
				events := map[Operation]string{OpTenantRoleUpdate: "updated", OpTenantRolePermissions: "permissions_changed", OpTenantRoleStatus: "status_changed", OpTenantRoleArchive: "deleted"}
				var beforeAudit any = p.OldRole
				var afterAudit any = map[string]any{"role": after, "impact": p.Impact}
				if op == OpTenantRoleUpdate {
					beforeFields := map[string]any{"version": p.OldRole.Version}
					afterFields := map[string]any{"version": out.Role.Version}
					if p.OldRole.Name != p.NewRole.Name {
						beforeFields["name"] = p.OldRole.Name
						afterFields["name"] = p.NewRole.Name
					}
					if p.OldRole.Description != p.NewRole.Description {
						beforeFields["description"] = p.OldRole.Description
						afterFields["description"] = p.NewRole.Description
					}
					beforeAudit = beforeFields
					afterAudit = afterFields
				}
				if e = Audit(ctx, tx, "tenant.role."+events[op], "tenant_role", id, "", beforeAudit, afterAudit, m); e != nil {
					return e
				}
				for _, v := range p.Members {
					if op != OpTenantRoleUpdate {
						final, e := TenantAuthorizationTx(ctx, tx, tenant, v.Before.MembershipID, out.Revision)
						if e != nil {
							return e
						}
						if e = Audit(ctx, tx, "tenant.member.authorization_changed", "tenant_member", v.Before.MembershipID, "", v.Before, final, m); e != nil {
							return e
						}
					}
				}
			}
		}
		if e = pageTenantImpact(&p, Filter{}, slices.Contains(have, TenantMemberRead)); e != nil {
			return e
		}
		out.Impact = p.Impact
		return nil
	})
	return out, e
}
func (s *Service) TenantRoleMembers(ctx context.Context, a Actor, tenant, id string, f Filter) (Page[TenantAuthorization], error) {
	out := Page[TenantAuthorization]{Items: []TenantAuthorization{}}
	e := s.tenantTransaction(ctx, a, OpTenantRoleMembers, tenant, false, func(tx pgx.Tx, rev string, _ []Code) error {
		if _, e := tenantRoleTx(ctx, tx, tenant, id); e != nil {
			return e
		}
		if f.Limit == 0 {
			f.Limit = 20
		}
		limit, e := f.validate()
		if e != nil {
			return e
		}
		if f.Cursor != "" && !ValidID(f.Cursor) {
			return Validation("cursor", "分页位置无效。")
		}
		rows, e := tx.Query(ctx, `SELECT membership_id::text FROM tenant_member_roles WHERE tenant_id=$1 AND role_id=$2 AND revoked_at IS NULL AND ($3='' OR membership_id::text>$3) ORDER BY membership_id LIMIT $4`, tenant, id, f.Cursor, limit+1)
		if e != nil {
			return e
		}
		ids, e := readIDs(rows)
		if e != nil {
			return e
		}
		if len(ids) > limit {
			ids = ids[:limit]
			out.NextCursor = ids[limit-1]
		}
		for _, mid := range ids {
			v, e := TenantAuthorizationTx(ctx, tx, tenant, mid, rev)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, v)
		}
		return nil
	})
	return out, e
}
