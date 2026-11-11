package access

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type TenantRole struct {
	Role
	MemberCount          int  `json:"member_count"`
	EffectiveMemberCount int  `json:"effective_member_count"`
	CanEdit              bool `json:"can_edit"`
	CanCopy              bool `json:"can_copy"`
}
type TenantPermission struct {
	Permission
	Selectable        bool   `json:"selectable"`
	UnavailableReason string `json:"unavailable_reason"`
}
type TenantRoleCreateInput struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Permissions   []Code `json:"permission_codes"`
	RequestID     string `json:"client_request_id"`
	SourceVersion string `json:"source_version"`
	Reason        string `json:"reason"`
}
type TenantRoleResult struct {
	Role     TenantRole `json:"role"`
	Revision string     `json:"authorization_revision"`
	Replayed bool       `json:"replayed"`
}

func hashValue(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) tenantTransaction(ctx context.Context, a Actor, op Operation, tenant string, write bool, fn func(pgx.Tx, string, []Code) error) error {
	e := s.Transact(ctx, func(tx pgx.Tx) error {
		if a.Check == nil {
			return ErrDenied
		}
		rev, e := Lock(ctx, tx, write)
		if e != nil {
			return e
		}
		if write {
			rows, e := tx.Query(ctx, `SELECT id::text FROM accounts WHERE id::text=$1 OR id IN(SELECT account_id FROM tenant_memberships WHERE tenant_id::text=$2) ORDER BY id FOR UPDATE`, a.ID, tenant)
			if e != nil {
				return e
			}
			if _, e = readIDs(rows); e != nil {
				return e
			}
		}
		if e = a.Check(ctx, tx, write); e != nil {
			return e
		}
		if e = RequireScopedTx(ctx, tx, a.ID, op, tenant); e != nil {
			return e
		}
		_, snap, e := TenantSnapshotTx(ctx, tx, a.ID, tenant, rev)
		if e != nil {
			return e
		}
		return fn(tx, rev, snap.Permissions)
	})
	s.LogDenial(op, a.ID, e)
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "23505":
			return &Error{Code: "name_taken", Message: "角色名称或创建请求已被使用。", Status: 409}
		case "55P03", "57014", "40P01":
			return &Error{Code: "operation_busy", Message: "操作繁忙，请核实结果后重试。", Status: 503}
		}
	}
	return e
}
func tenantRoleTx(ctx context.Context, tx pgx.Tx, tenant, id string) (TenantRole, error) {
	var out TenantRole
	if !ValidID(id) {
		return out, ErrNotFound
	}
	r, e := roleTx(ctx, tx, id)
	if e != nil {
		return out, e
	}
	if r.BindingScope != "tenant" || (r.OwnerTenantID != "" && r.OwnerTenantID != tenant) {
		return out, ErrNotFound
	}
	out.Role = r
	out.CanCopy = r.AssignmentPolicy == "manual"
	out.CanEdit = r.OwnerTenantID == tenant && tenant != ""
	if tenant != "" {
		e = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE m.status='active' AND a.status='active' AND $3='active') FROM tenant_member_roles mr JOIN tenant_memberships m ON m.id=mr.membership_id JOIN accounts a ON a.id=m.account_id WHERE mr.tenant_id=$1 AND mr.role_id=$2 AND mr.revoked_at IS NULL`, tenant, id, r.Status).Scan(&out.MemberCount, &out.EffectiveMemberCount)
	}
	return out, e
}
func (s *Service) TenantPermissions(ctx context.Context, a Actor, tenant string) ([]TenantPermission, error) {
	out := []TenantPermission{}
	e := s.tenantTransaction(ctx, a, OpTenantPermissionList, tenant, false, func(tx pgx.Tx, _ string, have []Code) error {
		for _, p := range Catalog() {
			if p.Scope != "tenant" {
				continue
			}
			v := TenantPermission{Permission: p, Selectable: p.Lifecycle == "active" && p.TenantCustomRoleAssignable && slices.Contains(have, p.Code)}
			if !p.TenantCustomRoleAssignable {
				v.UnavailableReason = "administrator_reserved"
			} else if p.Lifecycle != "active" {
				v.UnavailableReason = "retired"
			} else if !v.Selectable {
				v.UnavailableReason = "outside_actor_permissions"
			}
			out = append(out, v)
		}
		return nil
	})
	return out, e
}
func tenantRolePage(ctx context.Context, tx pgx.Tx, tenant string, f Filter) (Page[TenantRole], error) {
	out := Page[TenantRole]{Items: []TenantRole{}}
	if f.Limit == 0 {
		f.Limit = 20
	}
	limit, e := f.validate()
	if e != nil {
		return out, e
	}
	if f.Cursor != "" && !ValidID(f.Cursor) {
		return out, Validation("cursor", "分页位置无效。")
	}
	if f.ObjectType != "" && f.ObjectType != "preset" && f.ObjectType != "custom" {
		return out, Validation("source", "来源筛选无效。")
	}
	rows, e := tx.Query(ctx, `SELECT id::text FROM roles WHERE binding_scope='tenant' AND deleted_at IS NULL AND (owner_tenant_id IS NULL OR owner_tenant_id::text=$1) AND ($2='' OR name ILIKE '%'||$2||'%') AND ($3='' OR status=$3) AND ($4='' OR id::text>$4) AND ($5='' OR ($5='preset' AND owner_tenant_id IS NULL) OR ($5='custom' AND owner_tenant_id IS NOT NULL)) ORDER BY id LIMIT $6`, tenant, f.Query, f.Status, f.Cursor, f.ObjectType, limit+1)
	if e != nil {
		return out, e
	}
	ids, e := readIDs(rows)
	if e != nil {
		return out, e
	}
	if len(ids) > limit {
		ids = ids[:limit]
		out.NextCursor = ids[limit-1]
	}
	for _, id := range ids {
		v, e := tenantRoleTx(ctx, tx, tenant, id)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}
func (s *Service) TenantRoles(ctx context.Context, a Actor, tenant string, f Filter) (Page[TenantRole], error) {
	var out Page[TenantRole]
	e := s.tenantTransaction(ctx, a, OpTenantRoleList, tenant, false, func(tx pgx.Tx, _ string, _ []Code) error {
		var e error
		out, e = tenantRolePage(ctx, tx, tenant, f)
		return e
	})
	return out, e
}
func (s *Service) TenantRole(ctx context.Context, a Actor, tenant, id string) (TenantRole, error) {
	var out TenantRole
	e := s.tenantTransaction(ctx, a, OpTenantRoleDetail, tenant, false, func(tx pgx.Tx, _ string, _ []Code) error {
		var e error
		out, e = tenantRoleTx(ctx, tx, tenant, id)
		return e
	})
	return out, e
}
func (s *Service) TenantPresets(ctx context.Context, a Actor, f Filter) (Page[TenantRole], error) {
	var out Page[TenantRole]
	e := s.read(ctx, a, OpTenantPresetList, func(tx pgx.Tx, _ string) error { var e error; out, e = tenantRolePage(ctx, tx, "", f); return e })
	return out, e
}
func (s *Service) TenantPreset(ctx context.Context, a Actor, id string) (TenantRole, error) {
	var out TenantRole
	e := s.read(ctx, a, OpTenantPresetDetail, func(tx pgx.Tx, _ string) error { var e error; out, e = tenantRoleTx(ctx, tx, "", id); return e })
	return out, e
}
func validateTenantPermissions(codes, have []Code) error {
	if codes == nil || len(codes) > len(Catalog()) {
		return Validation("permission_codes", "请提交权限数组，空数组表示没有能力。")
	}
	seen := map[Code]bool{}
	for _, c := range codes {
		valid := false
		for _, p := range Catalog() {
			if p.Code == c && p.Scope == "tenant" && p.Lifecycle == "active" && p.TenantCustomRoleAssignable {
				valid = true
				break
			}
		}
		if !valid || seen[c] {
			return Validation("permission_codes", "权限未知、重复、已退役或为管理员保留能力："+string(c))
		}
		seen[c] = true
	}
	return within(have, codes)
}
func tenantRoleName(ctx context.Context, tx pgx.Tx, name, description string) (string, error) {
	name, e := roleFields(name, description)
	if e != nil {
		return "", e
	}
	var reserved bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roles WHERE binding_scope='tenant' AND owner_tenant_id IS NULL AND deleted_at IS NULL AND name_normalized=$1)`, strings.ToLower(name)).Scan(&reserved)
	if e != nil {
		return "", e
	}
	if reserved {
		return "", &Error{Code: "name_reserved", Message: "该名称为平台预置角色保留。", Status: 409}
	}
	return name, nil
}
func setTenantPermissions(ctx context.Context, tx pgx.Tx, id string, codes []Code) error {
	// Retain unchanged historical references; only newly added codes run insertion validation.
	if _, e := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id=$1 AND NOT(permission_code=ANY($2::text[]))`, id, codes); e != nil {
		return e
	}
	for _, p := range codes {
		if _, e := tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) VALUES($1,$2,'tenant') ON CONFLICT DO NOTHING`, id, p); e != nil {
			return e
		}
	}
	return nil
}
func tenantMeta(a Actor, tenant, reason string, m Meta) Meta {
	m.ActorID = a.ID
	m.TenantID = tenant
	m.ActorScope = "tenant"
	m.Reason = strings.TrimSpace(reason)
	return m
}
func (s *Service) CreateTenantRole(ctx context.Context, a Actor, tenant, source string, in TenantRoleCreateInput, m Meta) (TenantRoleResult, error) {
	var out TenantRoleResult
	op := OpTenantRoleCreate
	if source != "" {
		op = OpTenantRoleCopy
	}
	e := s.tenantTransaction(ctx, a, op, tenant, true, func(tx pgx.Tx, _ string, have []Code) error {
		if e := ValidReason(in.Reason); e != nil {
			return e
		}
		if !ValidID(in.RequestID) {
			return Validation("client_request_id", "请提供有效的请求 ID。")
		}
		var e error
		in.Name, e = roleFields(in.Name, in.Description)
		if e != nil {
			return e
		}
		in.Permissions = slices.Clone(in.Permissions)
		slices.Sort(in.Permissions)
		if e = validateTenantPermissions(in.Permissions, have); e != nil {
			return e
		}
		if len(in.Permissions) > 0 && !slices.Contains(have, TenantRoleSetPermissions) {
			return ErrDenied
		}
		hash := hashValue(struct {
			Input  TenantRoleCreateInput
			Source string
		}{in, source})
		var id, oldHash string
		var deleted bool
		e = tx.QueryRow(ctx, `SELECT id::text,creation_payload_hash,deleted_at IS NOT NULL FROM roles WHERE owner_tenant_id=$1 AND created_by=$2 AND creation_request_id=$3`, tenant, a.ID, in.RequestID).Scan(&id, &oldHash, &deleted)
		if e == nil {
			if hash != oldHash || deleted {
				return &Error{Code: "request_conflict", Message: "该创建请求已用于其他内容或角色已删除。", Status: 409}
			}
			out.Replayed = true
		} else if e != pgx.ErrNoRows {
			return e
		} else {
			in.Name, e = tenantRoleName(ctx, tx, in.Name, in.Description)
			if e != nil {
				return e
			}
			if source != "" {
				r, e := tenantRoleTx(ctx, tx, tenant, source)
				if e != nil {
					return e
				}
				if !r.CanCopy {
					return &Error{Code: "role_protected", Message: "管理员角色不可复制。", Status: 409}
				}
				if e = checkVersion(r.Version, in.SourceVersion); e != nil {
					return e
				}
				if e = within(have, r.Permissions); e != nil {
					return e
				}
			} else if in.SourceVersion != "" {
				return Validation("source_version", "创建角色不接受来源版本。")
			}
			e = tx.QueryRow(ctx, `INSERT INTO roles(name,name_normalized,description,binding_scope,owner_tenant_id,source_role_id,source_role_version,created_by,updated_by,creation_request_id,creation_payload_hash) VALUES($1,$2,$3,'tenant',$4,NULLIF($5,'')::uuid,NULLIF($6,'')::bigint,$7,$7,$8,$9) RETURNING id::text`, in.Name, strings.ToLower(in.Name), in.Description, tenant, source, in.SourceVersion, a.ID, in.RequestID, hash).Scan(&id)
			if e != nil {
				return e
			}
			if e = setTenantPermissions(ctx, tx, id, in.Permissions); e != nil {
				return e
			}
			if e = Bump(ctx, tx); e != nil {
				return e
			}
		}
		out.Role, e = tenantRoleTx(ctx, tx, tenant, id)
		if e != nil {
			return e
		}
		if !out.Replayed {
			event := "tenant.role.created"
			if source != "" {
				event = "tenant.role.copied"
			}
			if e = Audit(ctx, tx, event, "tenant_role", id, "", nil, out.Role, tenantMeta(a, tenant, in.Reason, m)); e != nil {
				return e
			}
		}
		return tx.QueryRow(ctx, `SELECT revision::text FROM authorization_state WHERE id=1`).Scan(&out.Revision)
	})
	return out, e
}
