package access

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"
)

// TenantAuthorization distinguishes stored configuration from current and restorable rights.
type TenantAuthorization struct {
	Snapshot
	TenantStatus          string   `json:"tenant_status"`
	MembershipID          string   `json:"membership_id"`
	MembershipVersion     string   `json:"membership_version"`
	MembershipStatus      string   `json:"membership_status"`
	AccountID             string   `json:"account_id"`
	AccountStatus         string   `json:"account_status"`
	Username              string   `json:"username"`
	DisplayName           string   `json:"display_name"`
	Administrator         bool     `json:"is_current_administrator"`
	ConfiguredPermissions []Code   `json:"configured_permissions"`
	RestorablePermissions []Code   `json:"restorable_permissions"`
	InactiveReasons       []string `json:"inactive_reasons"`
}

// catalog argument deliberately comes from the database during release comparisons.
func tenantCodesTx(ctx context.Context, tx pgx.Tx) (map[Code]Permission, error) {
	rows, e := tx.Query(ctx, `SELECT code,name,group_name,scope,description,risk,lifecycle,tenant_custom_role_assignable FROM permission_catalog WHERE scope='tenant' ORDER BY code`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[Code]Permission{}
	for rows.Next() {
		var p Permission
		if e = rows.Scan(&p.Code, &p.Name, &p.Group, &p.Scope, &p.Description, &p.Risk, &p.Lifecycle, &p.TenantCustomRoleAssignable); e != nil {
			return nil, e
		}
		out[p.Code] = p
	}
	return out, rows.Err()
}
func explainTenant(v *TenantAuthorization, catalog map[Code]Permission) {
	v.Permissions = []Code{}
	v.ConfiguredPermissions = []Code{}
	v.RestorablePermissions = []Code{}
	v.Sources = map[Code][]string{}
	v.InactiveReasons = []string{}
	if v.TenantStatus == "disabled" {
		v.InactiveReasons = append(v.InactiveReasons, "tenant_disabled")
	}
	if v.AccountStatus != "active" {
		v.InactiveReasons = append(v.InactiveReasons, "account_unavailable")
	}
	if v.MembershipStatus != "active" {
		v.InactiveReasons = append(v.InactiveReasons, "membership_"+v.MembershipStatus)
	}
	for _, r := range v.Roles {
		for _, code := range r.Permissions {
			p, ok := catalog[code]
			if !ok || p.Lifecycle != "active" {
				continue
			}
			if !slices.Contains(v.ConfiguredPermissions, code) {
				v.ConfiguredPermissions = append(v.ConfiguredPermissions, code)
			}
			if r.Status != "active" {
				continue
			}
			if !slices.Contains(v.RestorablePermissions, code) {
				v.RestorablePermissions = append(v.RestorablePermissions, code)
			}
			if v.TenantStatus != "disabled" && v.AccountStatus == "active" && v.MembershipStatus == "active" {
				if !slices.Contains(v.Permissions, code) {
					v.Permissions = append(v.Permissions, code)
				}
				v.Sources[code] = append(v.Sources[code], r.ID)
			}
		}
	}
	slices.Sort(v.Permissions)
	slices.Sort(v.ConfiguredPermissions)
	slices.Sort(v.RestorablePermissions)
}
func TenantAuthorizationTx(ctx context.Context, tx pgx.Tx, tenant, member, revision string) (TenantAuthorization, error) {
	v := TenantAuthorization{Snapshot: Snapshot{Revision: revision, Roles: []Role{}}}
	if !ValidID(tenant) || !ValidID(member) {
		return v, ErrNotFound
	}
	e := tx.QueryRow(ctx, `SELECT m.id::text,m.version::text,m.status,a.id::text,a.status,a.username,a.display_name,m.id=t.administrator_membership_id,t.status FROM tenant_memberships m JOIN accounts a ON a.id=m.account_id JOIN tenants t ON t.id=m.tenant_id WHERE m.tenant_id=$1 AND m.id=$2`, tenant, member).Scan(&v.MembershipID, &v.MembershipVersion, &v.MembershipStatus, &v.AccountID, &v.AccountStatus, &v.Username, &v.DisplayName, &v.Administrator, &v.TenantStatus)
	if e != nil {
		return v, notFound(e)
	}
	rows, e := tx.Query(ctx, `SELECT role_id::text FROM tenant_member_roles WHERE tenant_id=$1 AND membership_id=$2 AND revoked_at IS NULL ORDER BY role_id`, tenant, member)
	if e != nil {
		return v, e
	}
	ids, e := readIDs(rows)
	if e != nil {
		return v, e
	}
	for _, id := range ids {
		r, e := tenantRoleTx(ctx, tx, tenant, id)
		if e != nil {
			return v, ErrCatalog
		}
		v.Roles = append(v.Roles, r.Role)
	}
	catalog, e := tenantCodesTx(ctx, tx)
	if e != nil {
		return v, e
	}
	explainTenant(&v, catalog)
	return v, nil
}

// RequireTenantTargetCoverageTx is also used by member lifecycle operations.
func RequireTenantTargetCoverageTx(ctx context.Context, tx pgx.Tx, actor, tenant, member string) error {
	target, e := TenantAuthorizationTx(ctx, tx, tenant, member, "")
	if e != nil {
		return e
	}
	if target.AccountID == actor {
		return &Error{Code: "self_membership_protected", Message: "不能通过成员管理操作自己。", Status: 409}
	}
	_, have, e := TenantSnapshotTx(ctx, tx, actor, tenant, "")
	if e != nil {
		return e
	}
	return within(have.Permissions, target.ConfiguredPermissions)
}
func (s *Service) TenantMemberAuthorization(ctx context.Context, a Actor, tenant, member string) (TenantAuthorization, error) {
	var v TenantAuthorization
	e := s.tenantTransaction(ctx, a, OpTenantMemberAuthorization, tenant, false, func(tx pgx.Tx, rev string, _ []Code) error {
		var e error
		v, e = TenantAuthorizationTx(ctx, tx, tenant, member, rev)
		return e
	})
	return v, e
}
