package access

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

func Audit(ctx context.Context, tx pgx.Tx, event, objectType, object, account string, before, after any, m Meta) error {
	truncate := func(value string, n int) string {
		r := []rune(value)
		if len(r) > n {
			return string(r[:n])
		}
		return value
	}
	m.IP = truncate(m.IP, 100)
	m.UserAgent = truncate(m.UserAgent, 300)
	m.RequestID = truncate(m.RequestID, 100)
	if m.ActorScope == "" {
		m.ActorScope = "platform"
	}
	if m.ActorType == "" {
		m.ActorType = "account"
	}
	payload := map[string]any{"before": before, "after": after}
	if m.TenantID != "" && m.ActorID != "" {
		var username, display string
		if e := tx.QueryRow(ctx, "SELECT username,display_name FROM accounts WHERE id=$1", m.ActorID).Scan(&username, &display); e != nil {
			return e
		}
		payload["actor"] = map[string]string{"id": m.ActorID, "username": username, "display_name": display}
	}
	details, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO account_security_events(event_type,actor_id,account_id,object_id,outcome,reason,source_ip,user_agent,request_id,actor_type,object_type,details,tenant_id,actor_scope) VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,$4,'success',$5,$6,$7,$8,$9,$10,$11,NULLIF($12,'')::uuid,$13)`, event, m.ActorID, account, object, m.Reason, m.IP, m.UserAgent, m.RequestID, m.ActorType, objectType, details, m.TenantID, m.ActorScope)
	return e
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
func checkProjection(ctx context.Context, tx pgx.Tx) error {
	var count int
	if e := tx.QueryRow(ctx, "SELECT count(*) FROM permission_catalog").Scan(&count); e != nil {
		return e
	}
	if count != len(Catalog()) {
		return ErrCatalog
	}
	for _, p := range Catalog() {
		var actual Permission
		var version string
		e := tx.QueryRow(ctx, "SELECT code,name,group_name,scope,description,risk,lifecycle,catalog_version FROM permission_catalog WHERE code=$1", p.Code).Scan(&actual.Code, &actual.Name, &actual.Group, &actual.Scope, &actual.Description, &actual.Risk, &actual.Lifecycle, &version)
		if e != nil {
			return e
		}
		if actual != p || version != Checksum() {
			return ErrCatalog
		}
	}
	for _, b := range Builtins() {
		var id string
		if e := tx.QueryRow(ctx, "SELECT id::text FROM roles WHERE builtin_key=$1 AND deleted_at IS NULL", b.Key).Scan(&id); e != nil {
			return e
		}
		r, e := roleTx(ctx, tx, id)
		if e != nil {
			return e
		}
		a := slices.Clone(b.Permissions)
		slices.Sort(a)
		if r.BindingScope != b.BindingScope || r.Name != b.Name || r.Description != b.Description || r.Status != "active" || !slices.Equal(r.Permissions, a) {
			return ErrCatalog
		}
	}
	return CheckTenantStructure(ctx, tx)
}
func (s *Service) Sync(ctx context.Context) error {
	return s.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, "SELECT id FROM authorization_state WHERE id=1 FOR UPDATE"); e != nil {
			return e
		}
		// A matching projection is a true no-op (including versions and audit).
		var old string
		if e := tx.QueryRow(ctx, "SELECT catalog_checksum FROM authorization_state WHERE id=1").Scan(&old); e != nil {
			return e
		}
		if old == Checksum() && checkProjection(ctx, tx) == nil {
			return nil
		}
		before, e := InvitationHolders(ctx, tx)
		if e != nil {
			return e
		}
		codes := []string{}
		for _, p := range Catalog() {
			codes = append(codes, string(p.Code))
		}
		var unknown int
		if e = tx.QueryRow(ctx, "SELECT count(*) FROM permission_catalog WHERE NOT(code=ANY($1::text[]))", codes).Scan(&unknown); e != nil {
			return e
		}
		if unknown > 0 {
			return fmt.Errorf("catalog contains unknown codes; retired permissions must remain in the code catalog")
		}
		for _, p := range Catalog() {
			_, e = tx.Exec(ctx, `INSERT INTO permission_catalog(code,name,group_name,scope,description,risk,lifecycle,catalog_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(code) DO UPDATE SET name=$2,group_name=$3,scope=$4,description=$5,risk=$6,lifecycle=$7,catalog_version=$8`, p.Code, p.Name, p.Group, p.Scope, p.Description, p.Risk, p.Lifecycle, Checksum())
			if e != nil {
				return e
			}
		}
		for _, b := range Builtins() {
			var id string
			e = tx.QueryRow(ctx, `INSERT INTO roles(name,name_normalized,description,builtin_key,binding_scope) VALUES($1,$2,$3,$4,$5) ON CONFLICT(builtin_key) DO UPDATE SET name=$1,name_normalized=$2,description=$3,binding_scope=$5,status='active',deleted_at=NULL,version=roles.version+1,updated_at=now() RETURNING id::text`, b.Name, strings.ToLower(b.Name), b.Description, b.Key, b.BindingScope).Scan(&id)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, "DELETE FROM role_permissions WHERE role_id=$1", id); e != nil {
				return e
			}
			for _, c := range b.Permissions {
				if _, e = tx.Exec(ctx, "INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,$3)", id, c, b.BindingScope); e != nil {
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
		if _, e = tx.Exec(ctx, "UPDATE authorization_state SET catalog_checksum=$1,synced_at=now(),revision=revision+1 WHERE id=1", Checksum()); e != nil {
			return e
		}
		return Audit(ctx, tx, "catalog_synced", "catalog", Checksum(), "", old, Checksum(), Meta{ActorType: "release", Reason: "同步当前发布权限目录及内置角色"})
	})
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
