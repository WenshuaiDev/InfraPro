package tenancy

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

func tenantTx(ctx context.Context, tx pgx.Tx, id string, lock bool) (Tenant, error) {
	var v Tenant
	if !access.ValidID(id) {
		return v, access.ErrTenantNotFound
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF t"
	}
	e := tx.QueryRow(ctx, `SELECT t.id::text,t.name,t.code,t.description,t.version::text,t.created_at,t.updated_at,t.created_by::text,t.updated_by::text,m.id::text,a.id::text,a.username,a.display_name,a.status='active',t.status,t.status_changed_at,t.status_changed_by::text,t.status_change_reason,t.status_notice FROM tenants t JOIN tenant_memberships m ON m.id=t.administrator_membership_id JOIN accounts a ON a.id=m.account_id WHERE t.id=$1`+suffix, id).Scan(&v.ID, &v.Name, &v.Code, &v.Description, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.CreatedBy, &v.UpdatedBy, &v.MembershipID, &v.Administrator.ID, &v.Administrator.Username, &v.Administrator.DisplayName, &v.AdministratorAvailable, &v.Status, &v.StatusChangedAt, &v.StatusChangedBy, &v.StatusChangeReason, &v.StatusNotice)
	if e == pgx.ErrNoRows {
		e = access.ErrTenantNotFound
	}
	return v, e
}
func (s *Service) Get(ctx context.Context, a access.Actor, id string, platform bool) (Tenant, error) {
	var v Tenant
	op := access.OpTenantProfileRead
	if platform {
		op = access.OpTenantRead
	}
	e := s.transact(ctx, a, op, id, "", func(tx pgx.Tx, _ string) error { var e error; v, e = tenantTx(ctx, tx, id, false); return e })
	if !platform {
		v.StatusChangeReason = nil
		v.StatusChangedBy = nil
	}
	return v, e
}
func (s *Service) Context(ctx context.Context, a access.Actor, id string) (Context, error) {
	var v Context
	e := s.transact(ctx, a, access.OpTenantContext, id, "", func(tx pgx.Tx, rev string) error {
		mid, snap, e := access.TenantSnapshotTx(ctx, tx, a.ID, id, rev)
		if e != nil {
			return e
		}
		roles := []TenantRole{}
		for _, r := range snap.Roles {
			roles = append(roles, TenantRole{ID: r.ID, Name: r.Name, BindingScope: r.BindingScope})
		}
		auth, e := access.TenantAuthorizationTx(ctx, tx, id, mid, rev)
		if e != nil {
			return e
		}
		v = Context{CanEnter: access.WorkspacePath("tenant", id, "", "active", snap.Permissions) != "", Authorization: auth, MembershipID: mid, Roles: roles, Permissions: snap.Permissions, Sources: snap.Sources, Revision: rev}
		return tx.QueryRow(ctx, `SELECT id::text,name,code FROM tenants WHERE id=$1`, id).Scan(&v.Tenant.ID, &v.Tenant.Name, &v.Tenant.Code)
	})
	return v, e
}
func ids(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	v := []string{}
	for rows.Next() {
		var id string
		if e := rows.Scan(&id); e != nil {
			return nil, e
		}
		v = append(v, id)
	}
	return v, rows.Err()
}
func (s *Service) List(ctx context.Context, a access.Actor, f Filter) (access.Page[Tenant], error) {
	v := access.Page[Tenant]{Items: []Tenant{}}
	e := s.transact(ctx, a, access.OpTenantList, "", "", func(tx pgx.Tx, _ string) error {
		if e := f.validate(false); e != nil {
			return e
		}
		if f.Status != "" && f.Status != "active" && f.Status != "disabled" {
			return access.Validation("status", "租户状态筛选无效。")
		}
		rows, e := tx.Query(ctx, `SELECT t.id::text FROM tenants t JOIN tenant_memberships m ON m.id=t.administrator_membership_id JOIN accounts a ON a.id=m.account_id WHERE ($1='' OR t.name ILIKE '%'||$1||'%' OR EXISTS(SELECT 1 FROM tenant_codes c WHERE c.tenant_id=t.id AND c.code ILIKE '%'||$1||'%')) AND ($2='' OR t.id::text>$2) AND (NOT $3 OR a.status<>'active') AND ($5='' OR t.status=$5) ORDER BY t.id LIMIT $4`, f.Query, f.Cursor, f.Unavailable, f.Limit+1, f.Status)
		if e != nil {
			return e
		}
		list, e := ids(rows)
		if e != nil {
			return e
		}
		if len(list) > f.Limit {
			list = list[:f.Limit]
			v.NextCursor = list[f.Limit-1]
		}
		for _, id := range list {
			t, e := tenantTx(ctx, tx, id, false)
			if e != nil {
				return e
			}
			v.Items = append(v.Items, t)
		}
		return nil
	})
	return v, e
}
func (s *Service) Memberships(ctx context.Context, a access.Actor, account string, f Filter) (access.Page[MyTenant], error) {
	v := access.Page[MyTenant]{Items: []MyTenant{}}
	op := access.OpMyTenants
	if account != "" {
		op = access.OpAccountTenants
	} else {
		account = a.ID
	}
	e := s.transact(ctx, a, op, "", "", func(tx pgx.Tx, rev string) error {
		if !access.ValidID(account) {
			return access.ErrNotFound
		}
		if e := f.validate(false); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT t.id::text,t.name,t.code,m.id::text,t.status FROM tenant_memberships m JOIN tenants t ON t.id=m.tenant_id WHERE m.account_id=$1 AND m.status='active' AND ($2='' OR t.name ILIKE '%'||$2||'%' OR t.code ILIKE '%'||$2||'%') AND ($3='' OR t.id::text>$3) ORDER BY t.id LIMIT $4`, account, f.Query, f.Cursor, f.Limit+1)
		if e != nil {
			return e
		}
		for rows.Next() {
			var t MyTenant
			if e = rows.Scan(&t.ID, &t.Name, &t.Code, &t.MembershipID, &t.Status); e != nil {
				rows.Close()
				return e
			}
			v.Items = append(v.Items, t)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(v.Items) > f.Limit {
			v.Items = v.Items[:f.Limit]
			v.NextCursor = v.Items[f.Limit-1].ID
		}
		for i := range v.Items {
			auth, err := access.TenantAuthorizationTx(ctx, tx, v.Items[i].ID, v.Items[i].MembershipID, rev)
			if err != nil {
				return err
			}
			v.Items[i].CanEnter = access.WorkspacePath("tenant", v.Items[i].ID, "", v.Items[i].Status, auth.Permissions) != ""
			if !v.Items[i].CanEnter {
				v.Items[i].Restriction = "no_tenant_permissions"
				if v.Items[i].Status == "disabled" {
					v.Items[i].Restriction = "tenant_disabled"
				}
				v.Items[i].Roles = []TenantRole{}
				if op == access.OpMyTenants {
					continue
				}
			}
			// Governance summaries also explain preserved roles when the account is disabled.
			rows, e := tx.Query(ctx, `SELECT r.id::text,r.name,r.binding_scope FROM tenant_member_roles mr JOIN roles r ON r.id=mr.role_id WHERE mr.membership_id=$1 AND mr.revoked_at IS NULL AND r.status='active' AND r.deleted_at IS NULL ORDER BY r.id`, v.Items[i].MembershipID)
			if e != nil {
				return e
			}
			v.Items[i].Roles = []TenantRole{}
			for rows.Next() {
				r := TenantRole{}
				if e = rows.Scan(&r.ID, &r.Name, &r.BindingScope); e != nil {
					rows.Close()
					return e
				}
				v.Items[i].Roles = append(v.Items[i].Roles, r)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
		}
		return nil
	})
	return v, e
}
func (s *Service) Candidates(ctx context.Context, a access.Actor, f Filter) (access.Page[AdministratorCandidate], error) {
	v := access.Page[AdministratorCandidate]{Items: []AdministratorCandidate{}}
	e := s.transact(ctx, a, access.OpTenantCandidates, "", "", func(tx pgx.Tx, _ string) error {
		if e := f.validate(true); e != nil {
			return e
		}
		if f.TenantID != "" {
			if !access.ValidID(f.TenantID) {
				return access.Validation("tenant_id", "租户编号无效。")
			}
			if e := access.RequireTx(ctx, tx, a.ID, access.OpTenantTransfer); e != nil {
				return e
			}
		}
		rows, e := tx.Query(ctx, `SELECT a.id::text,a.username,a.display_name,COALESCE(m.status,''),CASE WHEN $4='' THEN '' WHEN m.status='suspended' THEN 'resume_required' WHEN m.status='active' THEN 'reuse' ELSE 'new' END,COALESCE((SELECT jsonb_agg(r.name ORDER BY r.name) FROM tenant_member_roles mr JOIN roles r ON r.id=mr.role_id WHERE mr.membership_id=m.id AND mr.revoked_at IS NULL),'[]'::jsonb) FROM accounts a LEFT JOIN tenant_memberships m ON m.account_id=a.id AND m.tenant_id::text=$4 AND m.status IN ('active','suspended') WHERE a.status='active' AND (a.username ILIKE '%'||$1||'%' OR a.display_name ILIKE '%'||$1||'%') AND ($2='' OR a.id::text>$2) ORDER BY a.id LIMIT $3`, f.Query, f.Cursor, f.Limit+1, f.TenantID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var a AdministratorCandidate
			if e = rows.Scan(&a.ID, &a.Username, &a.DisplayName, &a.MembershipStatus, &a.TransferMode, &a.ReplacedRoles); e != nil {
				return e
			}
			v.Items = append(v.Items, a)
		}
		if len(v.Items) > f.Limit {
			v.Items = v.Items[:f.Limit]
			v.NextCursor = v.Items[f.Limit-1].ID
		}
		return rows.Err()
	})
	return v, e
}
func (s *Service) Audit(ctx context.Context, a access.Actor, id string, f Filter) (access.Page[AuditEvent], error) {
	page, e := s.access.ReadAudit(ctx, a, auditrecord.Scope{Kind: "tenant", Tenant: id}, access.Filter{Cursor: f.Cursor, Query: f.Query, Limit: f.Limit, Event: f.Event, From: f.From, To: f.To, ObjectID: f.ObjectID, ObjectType: f.ObjectType})
	out := access.Page[AuditEvent]{Items: []AuditEvent{}, NextCursor: page.NextCursor}
	for _, v := range page.Items {
		out.Items = append(out.Items, AuditEvent{ID: v.ID, TenantID: v.TenantID, Event: v.EventType, ActorID: v.ActorID, ActorType: v.ActorType, ActorScope: v.ActorScope, Reason: v.Reason, RequestID: v.RequestID, CreatedAt: v.CreatedAt, Details: v.Details})
	}
	return out, e
}

// Older profile/transfer snapshots can include governance fields. Redact at every nesting level.
func redactStatusMetadata(v any) {
	switch v := v.(type) {
	case map[string]any:
		delete(v, "status_change_reason")
		delete(v, "status_changed_by")
		for _, child := range v {
			redactStatusMetadata(child)
		}
	case []any:
		for _, child := range v {
			redactStatusMetadata(child)
		}
	}
}
