package access

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

type CatalogRoleDiff struct {
	Key    string  `json:"builtin_key"`
	Before *Role   `json:"before"`
	After  Builtin `json:"after"`
}
type CatalogTenantImpact struct {
	TenantID               string `json:"tenant_id"`
	Members                int    `json:"members"`
	LosingMembers          int    `json:"losing_members"`
	InvalidatedInvitations int    `json:"invalidated_invitations"`
}
type CatalogSyncPreview struct {
	Projects       []CatalogProjectImpact `json:"projects"`
	BeforeChecksum string                 `json:"before_checksum"`
	AfterChecksum  string                 `json:"after_checksum"`
	Changed        bool                   `json:"changed"`
	Permissions    []Permission           `json:"permissions"`
	Roles          []CatalogRoleDiff      `json:"roles"`
	Conflicts      []string               `json:"conflicts"`
	Tenants        []CatalogTenantImpact  `json:"tenants"`
}
type syncPlan struct {
	ProjectBefore       map[string]ProjectAuthorization
	Report              CatalogSyncPreview
	Before              map[string]TenantAuthorization
	After               map[string]TenantAuthorization
	LostInvitationGrant map[string]bool
	Tenant              map[string]string
}

func prepareSync(ctx context.Context, tx pgx.Tx) (syncPlan, error) {
	p := syncPlan{Report: CatalogSyncPreview{AfterChecksum: Checksum(), Projects: []CatalogProjectImpact{}, Permissions: []Permission{}, Roles: []CatalogRoleDiff{}, Conflicts: []string{}, Tenants: []CatalogTenantImpact{}}, ProjectBefore: map[string]ProjectAuthorization{}, Before: map[string]TenantAuthorization{}, After: map[string]TenantAuthorization{}, LostInvitationGrant: map[string]bool{}, Tenant: map[string]string{}}
	if e := tx.QueryRow(ctx, `SELECT catalog_checksum FROM authorization_state WHERE id=1`).Scan(&p.Report.BeforeChecksum); e != nil {
		return p, e
	}
	expected := map[Code]Permission{}
	for _, v := range Catalog() {
		expected[v.Code] = v
	}
	rows, e := tx.Query(ctx, `SELECT code,name,group_name,scope,description,risk,lifecycle,tenant_custom_role_assignable,project_custom_role_assignable FROM permission_catalog ORDER BY code`)
	if e != nil {
		return p, e
	}
	old := map[Code]Permission{}
	for rows.Next() {
		var v Permission
		if e = rows.Scan(&v.Code, &v.Name, &v.Group, &v.Scope, &v.Description, &v.Risk, &v.Lifecycle, &v.TenantCustomRoleAssignable, &v.ProjectCustomRoleAssignable); e != nil {
			rows.Close()
			return p, e
		}
		old[v.Code] = v
		if _, ok := expected[v.Code]; !ok {
			p.Report.Conflicts = append(p.Report.Conflicts, "未知权限须保留退役定义："+string(v.Code))
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return p, e
	}
	for _, v := range Catalog() {
		if old[v.Code] != v {
			p.Report.Permissions = append(p.Report.Permissions, v)
		}
	}
	builtins := map[string]Builtin{}
	for _, b := range Builtins() {
		builtins[b.Key] = b
	}
	rows, e = tx.Query(ctx, `SELECT id::text FROM roles WHERE builtin_key IS NOT NULL ORDER BY id`)
	if e != nil {
		return p, e
	}
	ids, e := readIDs(rows)
	if e != nil {
		return p, e
	}
	actual := map[string]Role{}
	for _, id := range ids {
		r, e := roleTx(ctx, tx, id)
		if e != nil {
			return p, e
		}
		actual[r.BuiltinKey] = r
		if _, ok := builtins[r.BuiltinKey]; !ok {
			p.Report.Conflicts = append(p.Report.Conflicts, "未知内置角色或仍被引用的移除定义："+r.BuiltinKey)
		}
	}
	for _, b := range Builtins() {
		v, ok := actual[b.Key]
		codes := slices.Clone(b.Permissions)
		slices.Sort(codes)
		if !ok || v.Name != b.Name || v.Description != b.Description || v.Status != "active" || v.BindingScope != b.BindingScope || v.AssignmentPolicy != b.AssignmentPolicy || !slices.Equal(v.Permissions, codes) {
			diff := CatalogRoleDiff{Key: b.Key, After: b}
			if ok {
				diff.Before = &v
			}
			p.Report.Roles = append(p.Report.Roles, diff)
		}
		if b.BindingScope == "tenant" {
			rows, e = tx.Query(ctx, `SELECT owner_tenant_id::text||' / '||id::text||' / '||name FROM roles WHERE binding_scope='tenant' AND owner_tenant_id IS NOT NULL AND deleted_at IS NULL AND name_normalized=$1 ORDER BY owner_tenant_id,id`, strings.ToLower(b.Name))
			if e != nil {
				return p, e
			}
			conflicts, e := readIDs(rows)
			if e != nil {
				return p, e
			}
			p.Report.Conflicts = append(p.Report.Conflicts, conflicts...)
		}
	}
	oldFixed := []Code{}
	newFixed := []Code{}
	for _, code := range actual[TenantMemberKey].Permissions {
		if old[code].Lifecycle == "active" {
			oldFixed = append(oldFixed, code)
		}
	}
	for _, code := range builtins[TenantMemberKey].Permissions {
		if expected[code].Lifecycle == "active" {
			newFixed = append(newFixed, code)
		}
	}
	// Compare live database facts with the new catalog, never with known() from the new code.
	rows, e = tx.Query(ctx, `SELECT id::text,tenant_id::text FROM tenant_memberships WHERE status<>'ended' ORDER BY id`)
	if e != nil {
		return p, e
	}
	for rows.Next() {
		var id, tid string
		if e = rows.Scan(&id, &tid); e != nil {
			rows.Close()
			return p, e
		}
		p.Tenant[id] = tid
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return p, e
	}
	totals := map[string]CatalogTenantImpact{}
	for mid, tid := range p.Tenant {
		before, e := TenantAuthorizationTx(ctx, tx, tid, mid, "")
		if e != nil {
			return p, e
		}
		after := before
		after.Roles = slices.Clone(before.Roles)
		for i, r := range after.Roles {
			if b, ok := builtins[r.BuiltinKey]; ok {
				after.Roles[i].Permissions = slices.Clone(b.Permissions)
				slices.Sort(after.Roles[i].Permissions)
				after.Roles[i].Name = b.Name
				after.Roles[i].Description = b.Description
				after.Roles[i].Status = "active"
				after.Roles[i].AssignmentPolicy = b.AssignmentPolicy
			}
		}
		explainTenant(&after, expected)
		p.LostInvitationGrant[mid] = InvitationGrantEligible(before, oldFixed) && !InvitationGrantEligible(after, newFixed)
		if hashValue(before) != hashValue(after) || p.LostInvitationGrant[mid] {
			p.Before[mid] = before
			p.After[mid] = after
			v := totals[tid]
			v.TenantID = tid
			v.Members++
			if len(difference(InvitationQualificationPermissions(before), InvitationQualificationPermissions(after))) > 0 {
				v.LosingMembers++
			}
			if p.LostInvitationGrant[mid] {
				var n int
				if e = tx.QueryRow(ctx, `SELECT count(*) FROM tenant_invitations WHERE inviter_membership_id=$1 AND status='pending' AND expires_at>clock_timestamp()`, mid).Scan(&n); e != nil {
					return p, e
				}
				v.InvalidatedInvitations += n
			}
			totals[tid] = v
		}
	}
	for _, v := range totals {
		p.Report.Tenants = append(p.Report.Tenants, v)
	}
	slices.SortFunc(p.Report.Tenants, func(a, b CatalogTenantImpact) int { return strings.Compare(a.TenantID, b.TenantID) })
	if e = prepareProjectSync(ctx, tx, &p, old, expected, builtins); e != nil {
		return p, e
	}
	p.Report.Changed = p.Report.BeforeChecksum != Checksum() || len(p.Report.Roles) > 0 || len(p.Report.Permissions) > 0
	return p, nil
}
func (s *Service) SyncPreview(ctx context.Context) (CatalogSyncPreview, error) {
	var out CatalogSyncPreview
	e := s.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY`); e != nil {
			return e
		}
		p, e := prepareSync(ctx, tx)
		out = p.Report
		return e
	})
	return out, e
}
func (s *Service) Sync(ctx context.Context) error {
	return s.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT id FROM authorization_state WHERE id=1 FOR UPDATE`); e != nil {
			return e
		}
		p, e := prepareSync(ctx, tx)
		if e != nil {
			return e
		}
		if len(p.Report.Conflicts) > 0 {
			return fmt.Errorf("catalog sync conflicts: %s", strings.Join(p.Report.Conflicts, "; "))
		}
		if !p.Report.Changed {
			return checkProjection(ctx, tx)
		}
		before, e := InvitationHolders(ctx, tx)
		if e != nil {
			return e
		}
		for _, v := range Catalog() {
			_, e = tx.Exec(ctx, `INSERT INTO permission_catalog(code,name,group_name,scope,description,risk,lifecycle,tenant_custom_role_assignable,project_custom_role_assignable,catalog_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(code) DO UPDATE SET name=$2,group_name=$3,scope=$4,description=$5,risk=$6,lifecycle=$7,tenant_custom_role_assignable=$8,project_custom_role_assignable=$9,catalog_version=$10`, v.Code, v.Name, v.Group, v.Scope, v.Description, v.Risk, v.Lifecycle, v.TenantCustomRoleAssignable, v.ProjectCustomRoleAssignable, Checksum())
			if e != nil {
				return e
			}
		}
		for _, diff := range p.Report.Roles {
			b := diff.After
			var id string
			e = tx.QueryRow(ctx, `INSERT INTO roles(name,name_normalized,description,builtin_key,binding_scope,assignment_policy) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(builtin_key) DO UPDATE SET name=$1,name_normalized=$2,description=$3,binding_scope=$5,assignment_policy=$6,status='active',version=roles.version+1,updated_at=clock_timestamp() RETURNING id::text`, b.Name, strings.ToLower(b.Name), b.Description, b.Key, b.BindingScope, b.AssignmentPolicy).Scan(&id)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1`, id); e != nil {
				return e
			}
			for _, code := range b.Permissions {
				if _, e = tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,$3)`, id, code, b.BindingScope); e != nil {
					return e
				}
			}
		}
		if e = EnsureManager(ctx, tx); e != nil {
			return e
		}
		if e = RevokeLostInvitations(ctx, tx, before); e != nil {
			return e
		}
		mids := []string{}
		for mid := range p.Before {
			mids = append(mids, mid)
		}
		slices.Sort(mids)
		for _, mid := range mids {
			old := p.Before[mid]
			after, e := TenantAuthorizationTx(ctx, tx, p.Tenant[mid], mid, "")
			if e != nil {
				return e
			}
			m := Meta{ActorType: "release", ActorScope: "platform", TenantID: p.Tenant[mid], Reason: "同步平台预置角色及权限目录"}
			if p.LostInvitationGrant[mid] {
				if e = InvalidateTenantInvitations(ctx, tx, InvitationRevocation{InviterMembershipID: mid}, m); e != nil {
					return e
				}
			}
			if e = Audit(ctx, tx, "tenant.authorization.preset_synced", "tenant_member", mid, "", old, after, m); e != nil {
				return e
			}
		}
		for _, impact := range p.Report.Projects {
			after, e := ProjectAuthorizationTx(ctx, tx, impact.TenantID, impact.ProjectID, impact.MembershipID, "")
			if e != nil {
				return e
			}
			m := Meta{ActorType: "release", ActorScope: "platform", TenantID: impact.TenantID, ProjectID: impact.ProjectID, Reason: "同步平台项目预置角色及权限目录"}
			if e = Audit(ctx, tx, "project.authorization.preset_synced", "project_member", impact.MembershipID, "", p.ProjectBefore[impact.MembershipID], after, m); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE authorization_state SET catalog_checksum=$1,synced_at=clock_timestamp(),revision=revision+1 WHERE id=1`, Checksum()); e != nil {
			return e
		}
		if e = checkProjection(ctx, tx); e != nil {
			return e
		}
		// Platform release history contains counts and definitions, never individual project grants.
		publication := map[string]any{"before_checksum": p.Report.BeforeChecksum, "after_checksum": p.Report.AfterChecksum, "permissions": p.Report.Permissions, "roles": p.Report.Roles, "tenants": p.Report.Tenants, "affected_project_members": len(p.Report.Projects)}
		return Audit(ctx, tx, "catalog_synced", "catalog", Checksum(), "", p.Report.BeforeChecksum, publication, Meta{ActorType: "release", Reason: "同步当前发布权限目录及内置角色"})
	})
}
