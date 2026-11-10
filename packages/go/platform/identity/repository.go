package identity

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func (r *Repository) transact(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background()) // rollback also when the caller was canceled
	if _, err = tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const accountColumns = "id::text,username,display_name,status,created_at"

func scanAccount(row pgx.Row) (a Account, err error) {
	err = row.Scan(&a.ID, &a.Username, &a.DisplayName, &a.Status, &a.CreatedAt)
	return
}
func (r *Repository) Account(ctx context.Context, id string) (Account, error) {
	return scanAccount(r.pool.QueryRow(ctx, "SELECT "+accountColumns+" FROM accounts WHERE id=$1", id))
}
func (r *Repository) Find(ctx context.Context, username string) (Account, error) {
	a, e := scanAccount(r.pool.QueryRow(ctx, "SELECT "+accountColumns+" FROM accounts WHERE username_normalized=$1", strings.ToLower(username)))
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return a, e
}
func insertAccount(ctx context.Context, tx pgx.Tx, username, display, hash string) (Account, error) {
	a, e := scanAccount(tx.QueryRow(ctx, "INSERT INTO accounts(username,username_normalized,display_name) VALUES($1,$2,$3) RETURNING "+accountColumns, username, strings.ToLower(username), display))
	var pe *pgconn.PgError
	if errors.As(e, &pe) && pe.Code == "23505" {
		return a, ErrConflict
	}
	if e != nil {
		return a, e
	}
	_, e = tx.Exec(ctx, "INSERT INTO account_passwords(account_id,password_hash) VALUES($1,$2)", a.ID, hash)
	return a, e
}
func audit(ctx context.Context, tx pgx.Tx, event, actor, account, object, outcome string, m Meta) error {
	m = trimMeta(m)
	actorType, objectType := "account", "account"
	switch event {
	case "bootstrap", "reset_link_created", "backup_access_revoked":
		actorType = "maintenance"
	}
	switch event {
	case "login", "logout", "logout_all", "session_revoked", "reauthentication":
		objectType = "session"
	case "invitation_created", "invitation_revoked", "invitation_accepted":
		objectType = "invitation"
	case "reset_link_created", "password_reset":
		objectType = "password_reset"
	}
	_, err := tx.Exec(ctx, `INSERT INTO account_security_events(event_type,actor_id,account_id,object_id,outcome,reason,source_ip,user_agent,request_id,actor_type,object_type) VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10,$11)`, event, actor, account, object, outcome, m.Reason, m.IP, m.UserAgent, m.RequestID, actorType, objectType)
	return err
}
func (r *Repository) Initialized(ctx context.Context) (bool, error) {
	var done bool
	e := r.pool.QueryRow(ctx, "SELECT completed_at IS NOT NULL FROM system_initialization WHERE id=1").Scan(&done)
	return done, e
}
func (r *Repository) Bootstrap(ctx context.Context, username, display, hash string, m Meta) (Account, error) {
	var a Account
	err := r.transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		var done bool
		if e := tx.QueryRow(ctx, "SELECT completed_at IS NOT NULL FROM system_initialization WHERE id=1 FOR UPDATE").Scan(&done); e != nil {
			return e
		}
		if done {
			return problem("already_initialized", "系统已初始化，请使用账号恢复流程。", 409)
		}
		var e error
		a, e = insertAccount(ctx, tx, username, display, hash)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE system_initialization SET completed_at=now(),first_account_id=$1 WHERE id=1", a.ID); e != nil {
			return e
		}
		if e = access.BindBuiltin(ctx, tx, a.ID, access.AdminKey, access.Meta{ActorType: "maintenance", Reason: "首次初始化", UserAgent: m.UserAgent}); e != nil {
			return e
		}
		return audit(ctx, tx, "bootstrap", "", a.ID, a.ID, "success", m)
	})
	return a, err
}

// Sensitive browser writes lock the account before any session or single-use credential.
// The session is rechecked under that lock so a concurrent password reset cannot authorize a later write.
func lockPrincipal(ctx context.Context, tx pgx.Tx, p Principal, idle time.Duration, recent bool) error {
	var active bool
	var version int64
	if e := tx.QueryRow(ctx, "SELECT status='active',auth_version FROM accounts WHERE id=$1 FOR UPDATE", p.Account.ID).Scan(&active, &version); e != nil {
		return e
	}
	if !active || version != p.Version {
		return ErrUnauthorized
	}
	var valid bool
	var reauth time.Time
	e := tx.QueryRow(ctx, `SELECT revoked_at IS NULL AND expires_at>now() AND auth_version=$3 AND GREATEST(last_activity_at,$4::timestamptz)>now()-$5::interval,reauthenticated_at FROM account_sessions WHERE id=$1 AND account_id=$2`, p.Session.ID, p.Account.ID, p.Version, p.Session.LastActivityAt, interval(idle)).Scan(&valid, &reauth)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrUnauthorized
	}
	if e != nil {
		return e
	}
	if !valid {
		return ErrUnauthorized
	}
	if recent && time.Since(reauth) > 5*time.Minute {
		return ErrReauth
	}
	return nil
}
func (r *Repository) Profile(ctx context.Context, p Principal, display string, idle time.Duration, m Meta) (Account, error) {
	var a Account
	e := r.transact(ctx, func(tx pgx.Tx) error {
		if e := access.Require(ctx, tx, p.Account.ID, access.OpProfileUpdate); e != nil {
			return e
		}
		if err := lockPrincipal(ctx, tx, p, idle, false); err != nil {
			return err
		}
		var err error
		a, err = scanAccount(tx.QueryRow(ctx, "UPDATE accounts SET display_name=$2,updated_at=now() WHERE id=$1 RETURNING "+accountColumns, p.Account.ID, display))
		if err != nil {
			return err
		}
		return audit(ctx, tx, "profile_updated", p.Account.ID, p.Account.ID, p.Account.ID, "success", m)
	})
	return a, e
}
func (r *Repository) Events(ctx context.Context, tx pgx.Tx, id, cursor string) (Page[SecurityEvent], error) {
	page := Page[SecurityEvent]{Items: []SecurityEvent{}}
	before := int64(9223372036854775807)
	if cursor != "" {
		v, e := strconv.ParseInt(cursor, 10, 64)
		if e != nil || v < 1 {
			return page, FieldError("cursor", "分页位置无效。")
		}
		before = v
	}
	rows, e := tx.Query(ctx, `SELECT id,event_type,outcome,source_ip,user_agent,created_at FROM account_security_events WHERE tenant_id IS NULL AND account_id=$1 AND id<$2 ORDER BY id DESC LIMIT 51`, id, before)
	if e != nil {
		return page, e
	}
	defer rows.Close()
	for rows.Next() {
		var v SecurityEvent
		if e = rows.Scan(&v.ID, &v.EventType, &v.Outcome, &v.SourceIP, &v.UserAgent, &v.CreatedAt); e != nil {
			return page, e
		}
		page.Items = append(page.Items, v)
	}
	if len(page.Items) > 50 {
		page.Items = page.Items[:50]
		page.NextCursor = strconv.FormatInt(page.Items[49].ID, 10)
	}
	return page, rows.Err()
}
