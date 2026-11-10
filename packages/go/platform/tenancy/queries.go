package tenancy

import (
	"context"
	"strconv"
	"strings"
	"time"

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
	e := tx.QueryRow(ctx, `SELECT t.id::text,t.name,t.code,t.description,t.version::text,t.created_at,t.updated_at,t.created_by::text,t.updated_by::text,m.id::text,a.id::text,a.username,a.display_name,a.status='active' FROM tenants t JOIN tenant_memberships m ON m.id=t.administrator_membership_id JOIN accounts a ON a.id=m.account_id WHERE t.id=$1`+suffix, id).Scan(&v.ID, &v.Name, &v.Code, &v.Description, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.CreatedBy, &v.UpdatedBy, &v.MembershipID, &v.Administrator.ID, &v.Administrator.Username, &v.Administrator.DisplayName, &v.AdministratorAvailable)
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
		v = Context{MembershipID: mid, Roles: roles, Permissions: snap.Permissions, Sources: snap.Sources, Revision: rev}
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
		rows, e := tx.Query(ctx, `SELECT t.id::text FROM tenants t JOIN tenant_memberships m ON m.id=t.administrator_membership_id JOIN accounts a ON a.id=m.account_id WHERE ($1='' OR t.name ILIKE '%'||$1||'%' OR EXISTS(SELECT 1 FROM tenant_codes c WHERE c.tenant_id=t.id AND c.code ILIKE '%'||$1||'%')) AND ($2='' OR t.id::text>$2) AND (NOT $3 OR a.status<>'active') ORDER BY t.id LIMIT $4`, f.Query, f.Cursor, f.Unavailable, f.Limit+1)
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
		rows, e := tx.Query(ctx, `SELECT t.id::text,t.name,t.code,m.id::text FROM tenant_memberships m JOIN tenants t ON t.id=m.tenant_id WHERE m.account_id=$1 AND m.status='active' AND ($2='' OR t.name ILIKE '%'||$2||'%' OR t.code ILIKE '%'||$2||'%') AND ($3='' OR t.id::text>$3) ORDER BY t.id LIMIT $4`, account, f.Query, f.Cursor, f.Limit+1)
		if e != nil {
			return e
		}
		for rows.Next() {
			var t MyTenant
			if e = rows.Scan(&t.ID, &t.Name, &t.Code, &t.MembershipID); e != nil {
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
func (s *Service) Candidates(ctx context.Context, a access.Actor, f Filter) (access.Page[Account], error) {
	v := access.Page[Account]{Items: []Account{}}
	e := s.transact(ctx, a, access.OpTenantCandidates, "", "", func(tx pgx.Tx, _ string) error {
		if e := f.validate(true); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT id::text,username,display_name FROM accounts WHERE status='active' AND (username ILIKE '%'||$1||'%' OR display_name ILIKE '%'||$1||'%') AND ($2='' OR id::text>$2) ORDER BY id LIMIT $3`, f.Query, f.Cursor, f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var a Account
			if e = rows.Scan(&a.ID, &a.Username, &a.DisplayName); e != nil {
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
	v := access.Page[AuditEvent]{Items: []AuditEvent{}}
	e := s.transact(ctx, a, access.OpTenantAudit, id, "", func(tx pgx.Tx, _ string) error {
		cursor := f.Cursor
		f.Cursor = ""
		if e := f.validate(false); e != nil {
			return e
		}
		before := int64(9223372036854775807)
		if cursor != "" {
			var e error
			before, e = strconv.ParseInt(cursor, 10, 64)
			if e != nil || before < 1 {
				return access.Validation("cursor", "分页位置无效。")
			}
		}
		var from, to *time.Time
		for _, p := range []struct {
			raw  string
			dest **time.Time
		}{{f.From, &from}, {f.To, &to}} {
			if p.raw != "" {
				t, e := time.Parse(time.RFC3339, p.raw)
				if e != nil {
					return access.Validation("time", "时间格式无效。")
				}
				*p.dest = &t
			}
		}
		if from != nil && to != nil && from.After(*to) {
			return access.Validation("time", "起始时间不得晚于结束时间。")
		}
		events := []string{"tenant.created", "tenant.profile.updated", "tenant.administrator.transferred", "tenant.administrator.unavailable", "tenant.administrator.available"}
		rows, e := tx.Query(ctx, `SELECT id::text,tenant_id::text,event_type,COALESCE(actor_id::text,''),actor_type,actor_scope,reason,request_id,created_at,details FROM account_security_events WHERE tenant_id=$1 AND id<$2 AND event_type=ANY($3::text[]) AND ($4='' OR event_type=$4) AND ($5::timestamptz IS NULL OR created_at>=$5) AND ($6::timestamptz IS NULL OR created_at<=$6) AND ($7='' OR reason ILIKE '%'||$7||'%' OR event_type ILIKE '%'||$7||'%') ORDER BY id DESC LIMIT $8`, id, before, events, f.Event, from, to, strings.TrimSpace(f.Query), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var ev AuditEvent
			if e = rows.Scan(&ev.ID, &ev.TenantID, &ev.Event, &ev.ActorID, &ev.ActorType, &ev.ActorScope, &ev.Reason, &ev.RequestID, &ev.CreatedAt, &ev.Details); e != nil {
				return e
			}
			v.Items = append(v.Items, ev)
		}
		if len(v.Items) > f.Limit {
			v.Items = v.Items[:f.Limit]
			v.NextCursor = v.Items[f.Limit-1].ID
		}
		return rows.Err()
	})
	return v, e
}
