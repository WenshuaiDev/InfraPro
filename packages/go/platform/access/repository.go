package access

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func roleTx(ctx context.Context, tx pgx.Tx, id string) (Role, error) {
	r := Role{Permissions: []Code{}}
	e := tx.QueryRow(ctx, `SELECT id::text,name,description,COALESCE(builtin_key,''),binding_scope,status,version::text,created_at,COALESCE(owner_tenant_id::text,''),COALESCE(owner_project_id::text,''),COALESCE(source_role_id::text,''),COALESCE(source_role_version::text,''),assignment_policy,(SELECT count(*) FROM account_roles WHERE role_id=roles.id) FROM roles WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&r.ID, &r.Name, &r.Description, &r.BuiltinKey, &r.BindingScope, &r.Status, &r.Version, &r.CreatedAt, &r.OwnerTenantID, &r.OwnerProjectID, &r.SourceRoleID, &r.SourceRoleVersion, &r.AssignmentPolicy, &r.AccountCount)
	if e != nil {
		return r, notFound(e)
	}
	rows, e := tx.Query(ctx, "SELECT permission_code FROM role_permissions WHERE role_id=$1 ORDER BY permission_code", id)
	if e != nil {
		return r, e
	}
	defer rows.Close()
	for rows.Next() {
		var p Code
		if e = rows.Scan(&p); e != nil {
			return r, e
		}
		r.Permissions = append(r.Permissions, p)
	}
	return r, rows.Err()
}
func accountTx(ctx context.Context, tx pgx.Tx, id string) (Account, error) {
	var a Account
	e := tx.QueryRow(ctx, "SELECT id::text,username,display_name,status,management_version::text,created_at FROM accounts WHERE id=$1", id).Scan(&a.ID, &a.Username, &a.DisplayName, &a.Status, &a.Version, &a.CreatedAt)
	if e != nil {
		return a, notFound(e)
	}
	s, e := snapshotTx(ctx, tx, id, "")
	a.Roles = s.Roles
	return a, e
}

type Filter struct {
	Cursor, Query, Status, RoleID, ObjectID, ObjectType, From, To, TenantID, Event string
	Limit                                                                          int
	Category, Outcome                                                              string
}

func (f Filter) validate() (int, error) {
	if f.Limit == 0 {
		f.Limit = 50
	}
	if f.Limit < 1 || f.Limit > 100 {
		return 0, Validation("limit", "每页数量为 1–100。")
	}
	if len(f.Query) > 200 {
		return 0, Validation("q", "搜索内容过长。")
	}
	if f.Status != "" && f.Status != "active" && f.Status != "disabled" {
		return 0, Validation("status", "状态无效。")
	}
	if f.RoleID != "" && !ValidID(f.RoleID) {
		return 0, Validation("role_id", "角色无效。")
	}
	return f.Limit, nil
}
func (s *Service) Permissions(ctx context.Context, a Actor) ([]Permission, error) {
	e := s.read(ctx, a, OpPermissionRead, func(pgx.Tx, string) error { return nil })
	return Catalog(), e
}
func (s *Service) Roles(ctx context.Context, a Actor, f Filter) (Page[Role], error) {
	v := Page[Role]{Items: []Role{}}
	e := s.read(ctx, a, OpRoleList, func(tx pgx.Tx, _ string) error {
		limit, e := f.validate()
		if e != nil {
			return e
		}
		if f.Cursor != "" && !ValidID(f.Cursor) {
			return Validation("cursor", "分页位置无效。")
		}
		rows, e := tx.Query(ctx, `SELECT id::text FROM roles WHERE binding_scope='platform' AND deleted_at IS NULL AND ($1='' OR name ILIKE '%'||$1||'%') AND ($2='' OR status=$2) AND ($3='' OR id::text>$3) ORDER BY id LIMIT $4`, f.Query, f.Status, f.Cursor, limit+1)
		if e != nil {
			return e
		}
		ids, e := readIDs(rows)
		if e != nil {
			return e
		}
		if len(ids) > limit {
			ids = ids[:limit]
			v.NextCursor = ids[limit-1]
		}
		for _, id := range ids {
			r, e := roleTx(ctx, tx, id)
			if e != nil {
				return e
			}
			v.Items = append(v.Items, r)
		}
		return nil
	})
	return v, e
}
func readIDs(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if e := rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (s *Service) Role(ctx context.Context, a Actor, id string) (Role, error) {
	var v Role
	e := s.read(ctx, a, OpRoleRead, func(tx pgx.Tx, _ string) error {
		if !ValidID(id) {
			return ErrNotFound
		}
		var e error
		v, e = roleTx(ctx, tx, id)
		if e == nil && v.BindingScope != "platform" {
			return ErrNotFound
		}
		return e
	})
	return v, e
}
func (s *Service) Accounts(ctx context.Context, a Actor, f Filter) (Page[Account], error) {
	v := Page[Account]{Items: []Account{}}
	e := s.read(ctx, a, OpAccountList, func(tx pgx.Tx, _ string) error {
		limit, e := f.validate()
		if e != nil {
			return e
		}
		if f.Cursor != "" && !ValidID(f.Cursor) {
			return Validation("cursor", "分页位置无效。")
		}
		rows, e := tx.Query(ctx, `SELECT id::text FROM accounts WHERE ($1='' OR username ILIKE '%'||$1||'%' OR display_name ILIKE '%'||$1||'%') AND ($2='' OR status=$2) AND ($3='' OR id::text>$3) AND ($4='' OR EXISTS(SELECT 1 FROM account_roles WHERE account_id=accounts.id AND role_id::text=$4)) ORDER BY id LIMIT $5`, f.Query, f.Status, f.Cursor, f.RoleID, limit+1)
		if e != nil {
			return e
		}
		ids, e := readIDs(rows)
		if e != nil {
			return e
		}
		if len(ids) > limit {
			ids = ids[:limit]
			v.NextCursor = ids[limit-1]
		}
		for _, id := range ids {
			a, e := accountTx(ctx, tx, id)
			if e != nil {
				return e
			}
			v.Items = append(v.Items, a)
		}
		return nil
	})
	return v, e
}
func (s *Service) Account(ctx context.Context, a Actor, id string) (Account, error) {
	var v Account
	e := s.read(ctx, a, OpAccountRead, func(tx pgx.Tx, _ string) error {
		if !ValidID(id) {
			return ErrNotFound
		}
		var e error
		v, e = accountTx(ctx, tx, id)
		return e
	})
	return v, e
}
func (s *Service) Authorization(ctx context.Context, a Actor, id string) (Snapshot, error) {
	var v Snapshot
	e := s.read(ctx, a, OpAccountAuthorization, func(tx pgx.Tx, rev string) error {
		if !ValidID(id) {
			return ErrNotFound
		}
		if _, e := accountTx(ctx, tx, id); e != nil {
			return e
		}
		var e error
		v, e = snapshotTx(ctx, tx, id, rev)
		return e
	})
	return v, e
}
