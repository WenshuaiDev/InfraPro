package access

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/text/unicode/norm"
)

// Actor comes only from verified identity. Check revalidates the session in the SAME transaction.
type Actor struct {
	ID    string
	Check func(context.Context, pgx.Tx, bool) error
}

func (s *Service) read(ctx context.Context, a Actor, op Operation, fn func(pgx.Tx, string) error) error {
	e := s.Transact(ctx, func(tx pgx.Tx) error {
		rev, e := Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		if a.Check == nil {
			return ErrDenied
		}
		if e = RequireTx(ctx, tx, a.ID, op); e != nil {
			return e
		}
		if e = a.Check(ctx, tx, false); e != nil {
			return e
		}
		return fn(tx, rev)
	})
	if denied(e) {
		s.LogDenial(op, a.ID, e)
	}
	return e
}
func (s *Service) write(ctx context.Context, a Actor, op Operation, target, kind string, m Meta, fn func(pgx.Tx, []Code) error) error {
	e := s.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := Lock(ctx, tx, true); e != nil {
			return e
		}
		if a.Check == nil {
			return ErrDenied
		}
		if e := RequireTx(ctx, tx, a.ID, op); e != nil {
			return e
		}
		if e := ValidReason(m.Reason); e != nil {
			return e
		}
		if target != "" && !ValidID(target) {
			return ErrNotFound
		}
		// Lock every involved account in stable order before touching roles or credentials.
		rows, e := tx.Query(ctx, `SELECT id::text FROM accounts WHERE id::text=$1 OR ($3='account' AND id::text=$2) OR ($3='role' AND id IN(SELECT account_id FROM account_roles WHERE role_id::text=$2)) ORDER BY id FOR UPDATE`, a.ID, target, kind)
		if e != nil {
			return e
		}
		if _, e = readIDs(rows); e != nil {
			return e
		}
		if e = a.Check(ctx, tx, true); e != nil {
			return e
		}
		have, e := effective(ctx, tx, a.ID, false)
		if e != nil {
			return e
		}
		before, e := InvitationHolders(ctx, tx)
		if e != nil {
			return e
		}
		if e = fn(tx, have); e != nil {
			return e
		}
		if e = EnsureManager(ctx, tx); e != nil {
			return e
		}
		if kind == "role" && target != "" {
			rows, e := tx.Query(ctx, "SELECT account_id::text FROM account_roles WHERE role_id=$1 ORDER BY account_id", target)
			if e != nil {
				return e
			}
			ids, e := readIDs(rows)
			if e != nil {
				return e
			}
			for _, id := range ids {
				if e = Audit(ctx, tx, "account_authorization_changed", "account", id, id, nil, nil, Meta{ActorID: a.ID, ActorType: "account", Reason: "绑定角色配置发生变化", RequestID: m.RequestID}); e != nil {
					return e
				}
			}
		}
		if e = RevokeLostInvitations(ctx, tx, before); e != nil {
			return e
		}
		return Bump(ctx, tx)
	})
	if denied(e) {
		s.LogDenial(op, a.ID, e)
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) && pe.Code == "23505" {
		return &Error{Code: "name_taken", Message: "角色名称已被使用。", Status: 409}
	}
	return e
}
func within(have, want []Code) error {
	for _, p := range want {
		if known(p) && !slices.Contains(have, p) {
			return ErrScope
		}
	}
	return nil
}
func roleFields(name, description string) (string, error) {
	name = strings.TrimSpace(norm.NFKC.String(name))
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 50 {
		return "", Validation("name", "角色名称需为 1–50 个字符。")
	}
	if !utf8.ValidString(description) || utf8.RuneCountInString(description) > 500 {
		return "", Validation("description", "说明最多 500 字符。")
	}
	return name, nil
}
func (s *Service) CreateRole(ctx context.Context, a Actor, name, description string, m Meta) (Role, error) {
	var v Role
	m.ActorID = a.ID
	e := s.write(ctx, a, OpRoleCreate, "", "role", m, func(tx pgx.Tx, _ []Code) error {
		var e error
		name, e = roleFields(name, description)
		if e != nil {
			return e
		}
		var id string
		if e = tx.QueryRow(ctx, "INSERT INTO roles(name,name_normalized,description) VALUES($1,$2,$3) RETURNING id::text", name, strings.ToLower(name), description).Scan(&id); e != nil {
			return e
		}
		v, e = roleTx(ctx, tx, id)
		if e != nil {
			return e
		}
		return Audit(ctx, tx, "role_created", "role", id, "", nil, v, m)
	})
	return v, e
}

type RoleChange struct {
	ExpectedVersion           string
	Name, Description, Status string
	Permissions               []Code
}

func (s *Service) ChangeRole(ctx context.Context, a Actor, id string, op Operation, c RoleChange, m Meta) (Role, error) {
	id = strings.ToLower(id)
	var v Role
	m.ActorID = a.ID
	switch op {
	case OpRoleUpdate, OpRoleSetPermissions, OpRoleSetStatus, OpRoleDelete:
	default:
		return v, ErrDenied
	}
	e := s.write(ctx, a, op, id, "role", m, func(tx pgx.Tx, have []Code) error {
		old, e := roleTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if old.BuiltinKey != "" {
			return Validation("role", "内置角色由系统维护，不可修改。")
		}
		if e = checkVersion(old.Version, c.ExpectedVersion); e != nil {
			return e
		}
		if e = within(have, old.Permissions); e != nil {
			return e
		}
		switch op {
		case OpRoleUpdate:
			name, e := roleFields(c.Name, c.Description)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, "UPDATE roles SET name=$2,name_normalized=$3,description=$4 WHERE id=$1", id, name, strings.ToLower(name), c.Description)
			if e != nil {
				return e
			}
		case OpRoleSetPermissions:
			if c.Permissions == nil || len(c.Permissions) > len(Catalog()) {
				return Validation("permission_codes", "请提交有效的权限列表。")
			}
			seen := map[Code]bool{}
			for _, p := range c.Permissions {
				if !known(p) || seen[p] || strings.HasPrefix(string(p), "tenant.") {
					return Validation("permission_codes", "权限未知、已退役或重复。")
				}
				seen[p] = true
			}
			if e = within(have, c.Permissions); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, "DELETE FROM role_permissions WHERE role_id=$1", id); e != nil {
				return e
			}
			for _, p := range c.Permissions {
				if _, e = tx.Exec(ctx, "INSERT INTO role_permissions(role_id,permission_code) VALUES($1,$2)", id, p); e != nil {
					return e
				}
			}
		case OpRoleSetStatus:
			if c.Status != "active" && c.Status != "disabled" {
				return Validation("status", "角色状态无效。")
			}
			if _, e = tx.Exec(ctx, "UPDATE roles SET status=$2 WHERE id=$1", id, c.Status); e != nil {
				return e
			}
		case OpRoleDelete:
			if old.AccountCount != 0 {
				return &Error{Code: "role_in_use", Message: "角色仍被账号绑定，请先解除全部绑定。", Status: 409}
			}
			if _, e = tx.Exec(ctx, "UPDATE roles SET deleted_at=now() WHERE id=$1", id); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, "UPDATE roles SET version=version+1,updated_at=now() WHERE id=$1", id); e != nil {
			return e
		}
		if op != OpRoleDelete {
			v, e = roleTx(ctx, tx, id)
			if e != nil {
				return e
			}
		}
		return Audit(ctx, tx, string(op), "role", id, "", old, v, m)
	})
	return v, e
}
func (s *Service) AssignRoles(ctx context.Context, a Actor, id, version string, ids []string, m Meta) (Account, error) {
	id = strings.ToLower(id)
	var result Account
	m.ActorID = a.ID
	e := s.write(ctx, a, OpAccountAssignRoles, id, "account", m, func(tx pgx.Tx, have []Code) error {
		old, e := accountTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if e = checkVersion(old.Version, version); e != nil {
			return e
		}
		if ids == nil || len(ids) > 100 {
			return Validation("role_ids", "角色列表不能为空值且最多 100 项，可提交空数组。")
		}
		oldIDs := []string{}
		for _, r := range old.Roles {
			oldIDs = append(oldIDs, r.ID)
		}
		seen := map[string]bool{}
		ids = slices.Clone(ids)
		for i := range ids {
			ids[i] = strings.ToLower(ids[i])
		}
		slices.Sort(ids)
		for _, rid := range ids {
			if !ValidID(rid) || seen[rid] {
				return Validation("role_ids", "角色 ID 无效或重复。")
			}
			seen[rid] = true
			r, e := roleTx(ctx, tx, rid)
			if e != nil {
				return e
			}
			if r.BindingScope != "platform" {
				return Validation("role_ids", "租户角色只能绑定租户成员。")
			}
			if !slices.Contains(oldIDs, rid) {
				if r.Status != "active" {
					return Validation("role_ids", "不能新增绑定停用角色。")
				}
				if e = within(have, r.Permissions); e != nil {
					return e
				}
			}
		}
		for _, r := range old.Roles {
			if !seen[r.ID] {
				if e = within(have, r.Permissions); e != nil {
					return e
				}
			}
		}
		if _, e = tx.Exec(ctx, "DELETE FROM account_roles WHERE account_id=$1 AND NOT(role_id::text=ANY($2::text[]))", id, ids); e != nil {
			return e
		}
		for _, rid := range ids {
			if _, e = tx.Exec(ctx, `INSERT INTO account_roles(account_id,role_id,source,actor_id) VALUES($1,$2,'account',$3) ON CONFLICT DO NOTHING`, id, rid, a.ID); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, "UPDATE accounts SET management_version=management_version+1,updated_at=now() WHERE id=$1", id); e != nil {
			return e
		}
		result, e = accountTx(ctx, tx, id)
		if e != nil {
			return e
		}
		return Audit(ctx, tx, "account_roles_changed", "account", id, id, old.Roles, result.Roles, m)
	})
	return result, e
}

// StatusWriter is supplied by identity so session/reset/invitation semantics have one owner.
type StatusWriter func(context.Context, pgx.Tx, string, bool, bool, Meta) error

func (s *Service) SetAccountStatus(ctx context.Context, a Actor, id, version string, enabled bool, m Meta, update StatusWriter) (Account, error) {
	id = strings.ToLower(id)
	var result Account
	op := OpAccountDisable
	if enabled {
		op = OpAccountEnable
	}
	m.ActorID = a.ID
	e := s.write(ctx, a, op, id, "account", m, func(tx pgx.Tx, have []Code) error {
		old, e := accountTx(ctx, tx, id)
		if e != nil {
			return e
		}
		if e = checkVersion(old.Version, version); e != nil {
			return e
		}
		configured, e := effective(ctx, tx, id, true)
		if e != nil {
			return e
		}
		if e = within(have, configured); e != nil {
			return e
		}
		if update == nil {
			return ErrDenied
		}
		if e = update(ctx, tx, id, enabled, false, m); e != nil {
			return e
		}
		result, e = accountTx(ctx, tx, id)
		if e != nil {
			return e
		}
		return Audit(ctx, tx, string(op), "account", id, id, old, result, m)
	})
	return result, e
}
