package access

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

// Audit adapts existing source-owned transactions to the single fact writer.
func Audit(ctx context.Context, tx pgx.Tx, event, objectType, object, account string, before, after any, m Meta) error {
	if e := taskrecord.ObserveAuditCommandTx(ctx, tx); e != nil {
		return e
	}
	_, err := auditrecord.AppendLegacyTx(ctx, tx, auditrecord.LegacyInput{Links: m.AuditLinks, Event: event, ObjectType: objectType, ObjectID: object, AccountID: account, Before: before, After: after, ActorID: m.ActorID, ActorType: m.ActorType, IP: m.IP, UserAgent: m.UserAgent, RequestID: m.RequestID, Reason: m.Reason, Tenant: m.TenantID, Project: m.ProjectID, Entry: m.ActorScope, Outcome: "success"})
	return err
}
func BindBuiltin(ctx context.Context, tx pgx.Tx, account, key string, m Meta) error {
	var role string
	if e := tx.QueryRow(ctx, "SELECT id::text FROM roles WHERE builtin_key=$1 AND status='active' AND deleted_at IS NULL", key).Scan(&role); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `INSERT INTO account_roles(account_id,role_id,source,actor_id) VALUES($1,$2,$3,NULLIF($4,'')::uuid) ON CONFLICT DO NOTHING`, account, role, m.ActorType, m.ActorID)
	if e != nil {
		return e
	}
	if e = Bump(ctx, tx); e != nil {
		return e
	}
	return Audit(ctx, tx, "initial_role_bound", "account", account, account, nil, []string{role}, m)
}
func (s *Service) Check(ctx context.Context) error {
	return s.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := Lock(ctx, tx, false); e != nil {
			return e
		}
		return checkProjection(ctx, tx)
	})
}

// Ready takes the authorization lock on every authenticated request. Compare
// the complete projection in two result sets so an additional process hop
// does not multiply hundreds of catalog round trips. Only compiled release
// constants are retained; database state is read again on every invocation.
var expectedPermissions = sync.OnceValue(func() map[Code]Permission {
	out := map[Code]Permission{}
	for _, p := range Catalog() {
		out[p.Code] = p
	}
	return out
})
var expectedBuiltins = sync.OnceValue(func() map[string]Builtin {
	out := map[string]Builtin{}
	for _, b := range Builtins() {
		b.Permissions = slices.Clone(b.Permissions)
		slices.Sort(b.Permissions)
		out[b.Key] = b
	}
	return out
})

func checkProjection(ctx context.Context, tx pgx.Tx) error {
	permissions := expectedPermissions()
	rows, err := tx.Query(ctx, `SELECT code,name,group_name,scope,description,risk,lifecycle,tenant_custom_role_assignable,project_custom_role_assignable,catalog_version FROM permission_catalog`)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var p Permission
		var version string
		if err := rows.Scan(&p.Code, &p.Name, &p.Group, &p.Scope, &p.Description, &p.Risk, &p.Lifecycle, &p.TenantCustomRoleAssignable, &p.ProjectCustomRoleAssignable, &version); err != nil {
			return err
		}
		expected, ok := permissions[p.Code]
		if !ok || expected != p || version != Checksum() {
			return ErrCatalog
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(permissions) {
		return ErrCatalog
	}
	roles := expectedBuiltins()
	rows, err = tx.Query(ctx, `SELECT r.builtin_key,r.name,r.description,r.assignment_policy,r.binding_scope,r.status,
  r.owner_tenant_id IS NULL AND r.deleted_at IS NULL,
  ARRAY(SELECT rp.permission_code FROM role_permissions rp WHERE rp.role_id=r.id ORDER BY rp.permission_code)
  FROM roles r WHERE r.builtin_key IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	count = 0
	for rows.Next() {
		var role Builtin
		var status string
		var unowned bool
		if err := rows.Scan(&role.Key, &role.Name, &role.Description, &role.AssignmentPolicy, &role.BindingScope, &status, &unowned, &role.Permissions); err != nil {
			return err
		}
		expected, ok := roles[role.Key]
		if !ok || !unowned || status != "active" || role.Name != expected.Name || role.Description != expected.Description || role.AssignmentPolicy != expected.AssignmentPolicy || role.BindingScope != expected.BindingScope || !slices.Equal(role.Permissions, expected.Permissions) {
			return ErrCatalog
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(roles) {
		return ErrCatalog
	}
	if err := CheckTenantStructure(ctx, tx); err != nil {
		return err
	}
	return CheckProjectStructure(ctx, tx)
}

func (s *Service) RecoverAdmin(ctx context.Context, username, reason string) error {
	if e := ValidReason(reason); e != nil {
		return e
	}
	return s.Transact(ctx, func(tx pgx.Tx) error {
		// Recovery can precede a new catalog sync, but requires a valid existing admin role.
		if _, e := tx.Exec(ctx, "SELECT id FROM authorization_state WHERE id=1 FOR UPDATE"); e != nil {
			return e
		}
		var id string
		if e := tx.QueryRow(ctx, "SELECT id::text FROM accounts WHERE username_normalized=$1 FOR UPDATE", strings.ToLower(username)).Scan(&id); e != nil {
			return notFound(e)
		}
		before, e := snapshotTx(ctx, tx, id, "")
		if e != nil {
			return e
		}
		if e = BindBuiltin(ctx, tx, id, AdminKey, Meta{ActorType: "maintenance", Reason: reason, UserAgent: "infrapro"}); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE accounts SET management_version=management_version+1 WHERE id=$1", id); e != nil {
			return e
		}
		after, e := snapshotTx(ctx, tx, id, "")
		if e != nil {
			return e
		}
		return Audit(ctx, tx, "admin_recovered", "account", id, id, before, after, Meta{ActorType: "maintenance", Reason: reason, UserAgent: "infrapro"})
	})
}
