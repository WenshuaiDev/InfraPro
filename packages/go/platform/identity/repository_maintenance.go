package identity

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) ResetLink(ctx context.Context, username, hash string, ttl time.Duration, m Meta) (time.Time, error) {
	var expiry time.Time
	err := r.transact(ctx, func(tx pgx.Tx) error {
		a, e := scanAccount(tx.QueryRow(ctx, "SELECT "+accountColumns+" FROM accounts WHERE username_normalized=$1 FOR UPDATE", strings.ToLower(username)))
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE account_password_resets SET revoked_at=now() WHERE account_id=$1 AND used_at IS NULL AND revoked_at IS NULL", a.ID); e != nil {
			return e
		}
		var id string
		if e = tx.QueryRow(ctx, "INSERT INTO account_password_resets(account_id,token_hash,reason,expires_at) VALUES($1,$2,$3,now()+$4::interval) RETURNING id::text,expires_at", a.ID, hash, m.Reason, interval(ttl)).Scan(&id, &expiry); e != nil {
			return e
		}
		return audit(ctx, tx, "reset_link_created", "", a.ID, id, "success", m)
	})
	return expiry, err
}
func (r *Repository) InspectReset(ctx context.Context, hash string) (Inspection, error) {
	var i Inspection
	e := r.pool.QueryRow(ctx, "SELECT expires_at FROM account_password_resets WHERE token_hash=$1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at>now()", hash).Scan(&i.ExpiresAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrLink
	}
	return i, e
}
func (r *Repository) CompleteReset(ctx context.Context, tokenHash, passwordHash string, m Meta) error {
	return r.transact(ctx, func(tx pgx.Tx) error {
		var id, account string
		e := tx.QueryRow(ctx, `SELECT a.id::text FROM accounts a JOIN account_password_resets r ON r.account_id=a.id WHERE r.token_hash=$1 FOR UPDATE OF a`, tokenHash).Scan(&account)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrLink
		}
		if e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `SELECT id::text FROM account_password_resets WHERE token_hash=$1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at>now() FOR UPDATE`, tokenHash).Scan(&id)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrLink
		}
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE account_password_resets SET used_at=now() WHERE id=$1", id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE account_passwords SET password_hash=$2,updated_at=now() WHERE account_id=$1", account, passwordHash); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE accounts SET auth_version=auth_version+1,updated_at=now() WHERE id=$1", account); e != nil {
			return e
		}
		if e = revokeSessions(ctx, tx, account, ""); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE account_invitations SET revoked_at=now() WHERE inviter_id=$1 AND used_at IS NULL AND revoked_at IS NULL", account); e != nil {
			return e
		}
		// A reset is also the recovery from persistent password failure cooldown.
		if _, e = tx.Exec(ctx, "DELETE FROM account_rate_limits WHERE key_hash=(SELECT encode(sha256(convert_to('password:'||username_normalized,'UTF8')),'hex') FROM accounts WHERE id=$1)", account); e != nil {
			return e
		}
		return audit(ctx, tx, "password_reset", account, account, id, "success", m)
	})
}

// SetStatusTx is shared by maintenance and browser operations. Callers hold the authorization lock first.
func (r *Repository) SetStatusTx(ctx context.Context, tx pgx.Tx, id string, enabled, emergency bool, m access.Meta) error {
	a, e := scanAccount(tx.QueryRow(ctx, "SELECT "+accountColumns+" FROM accounts WHERE id=$1 FOR UPDATE", id))
	if e != nil {
		return e
	}
	if !enabled && !emergency {
		if e = access.ProtectTenantAdministrator(ctx, tx, id); e != nil {
			return e
		}
	}
	status := "active"
	if !enabled {
		status = "disabled"
		if a.Status == "active" && !emergency {
			var count int
			if e = tx.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE status='active'").Scan(&count); e != nil {
				return e
			}
			if count <= 1 {
				return access.ErrLastManager
			}
		}
		if e = revokeSessions(ctx, tx, id, ""); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE account_invitations SET revoked_at=now() WHERE inviter_id=$1 AND used_at IS NULL AND revoked_at IS NULL", id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE account_password_resets SET revoked_at=now() WHERE account_id=$1 AND used_at IS NULL AND revoked_at IS NULL", id); e != nil {
			return e
		}
	}
	if a.Status != status {
		if e = access.AuditTenantAvailability(ctx, tx, id, enabled, m); e != nil {
			return e
		}
	}
	_, e = tx.Exec(ctx, "UPDATE accounts SET status=$2,auth_version=auth_version+1,management_version=management_version+1,updated_at=now() WHERE id=$1", id, status)
	return e
}
func (r *Repository) SetStatus(ctx context.Context, username string, enabled, emergency bool, m Meta) error {
	return r.transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, true); e != nil {
			return e
		}
		a, e := scanAccount(tx.QueryRow(ctx, "SELECT "+accountColumns+" FROM accounts WHERE username_normalized=$1 FOR UPDATE", strings.ToLower(username)))
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if e = r.SetStatusTx(ctx, tx, a.ID, enabled, emergency, access.Meta{ActorType: "maintenance", Reason: m.Reason, UserAgent: m.UserAgent, RequestID: m.RequestID}); e != nil {
			return e
		}
		if !emergency {
			if e = access.EnsureManager(ctx, tx); e != nil {
				return e
			}
		}
		if e = access.Bump(ctx, tx); e != nil {
			return e
		}
		event := "account_disabled"
		if enabled {
			event = "account_enabled"
		}
		if emergency && !enabled {
			event = "account_emergency_disabled"
		}
		return access.Audit(ctx, tx, event, "account", a.ID, a.ID, a.Status, map[string]bool{"enabled": enabled, "emergency": emergency}, access.Meta{ActorType: "maintenance", Reason: m.Reason, UserAgent: m.UserAgent, RequestID: m.RequestID})
	})
}
func (r *Repository) RevokeAfterRestore(ctx context.Context, m Meta) error {
	return r.transact(ctx, func(tx pgx.Tx) error {
		// Exclusive maintenance transaction; callers must stop API traffic during backup restoration.
		if _, e := tx.Exec(ctx, "LOCK TABLE accounts IN EXCLUSIVE MODE"); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "UPDATE accounts SET auth_version=auth_version+1,updated_at=now()"); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `WITH revoked AS (UPDATE account_sessions SET revoked_at=COALESCE(revoked_at,now()) RETURNING id,token_hash) INSERT INTO account_cache_invalidations(session_id,token_hash) SELECT id,token_hash FROM revoked ON CONFLICT DO NOTHING`); e != nil {
			return e
		}
		for _, q := range []string{"UPDATE account_invitations SET revoked_at=now() WHERE used_at IS NULL AND revoked_at IS NULL", "UPDATE account_password_resets SET revoked_at=now() WHERE used_at IS NULL AND revoked_at IS NULL"} {
			if _, e := tx.Exec(ctx, q); e != nil {
				return e
			}
		}
		return audit(ctx, tx, "backup_access_revoked", "", "", "", "success", m)
	})
}
func (r *Repository) Audit(ctx context.Context, cursor string) (Page[AuditEvent], error) {
	page := Page[AuditEvent]{Items: []AuditEvent{}}
	before := int64(9223372036854775807)
	if cursor != "" {
		v, e := strconv.ParseInt(cursor, 10, 64)
		if e != nil || v < 1 {
			return page, FieldError("cursor", "分页位置无效。")
		}
		before = v
	}
	rows, e := r.pool.Query(ctx, "SELECT id,event_type,outcome,source_ip,user_agent,created_at,actor_id::text,account_id::text,object_id,reason,request_id,actor_type,object_type,details FROM account_security_events WHERE id<$1 ORDER BY id DESC LIMIT 101", before)
	if e != nil {
		return page, e
	}
	defer rows.Close()
	for rows.Next() {
		var v AuditEvent
		if e = rows.Scan(&v.ID, &v.EventType, &v.Outcome, &v.SourceIP, &v.UserAgent, &v.CreatedAt, &v.ActorID, &v.AccountID, &v.ObjectID, &v.Reason, &v.RequestID, &v.ActorType, &v.ObjectType, &v.Details); e != nil {
			return page, e
		}
		page.Items = append(page.Items, v)
	}
	if len(page.Items) > 100 {
		page.Items = page.Items[:100]
		page.NextCursor = strconv.FormatInt(page.Items[99].ID, 10)
	}
	return page, rows.Err()
}
func (r *Repository) Cleanup(ctx context.Context, retention time.Duration) error {
	// Small batches bound locks and WAL. Invitations are retained through the audit retention period.
	for _, q := range []string{
		"DELETE FROM account_sessions WHERE id IN (SELECT id FROM account_sessions WHERE expires_at<now()-$1::interval LIMIT 500)",
		"DELETE FROM account_invitations WHERE id IN (SELECT id FROM account_invitations WHERE expires_at<now()-$1::interval LIMIT 500)",
		"DELETE FROM account_password_resets WHERE id IN (SELECT id FROM account_password_resets WHERE expires_at<now()-$1::interval LIMIT 500)",
		"DELETE FROM account_security_events WHERE id IN (SELECT id FROM account_security_events WHERE created_at<now()-$1::interval LIMIT 500)",
	} {
		if _, e := r.pool.Exec(ctx, q, interval(retention)); e != nil {
			return e
		}
	}
	_, e := r.pool.Exec(ctx, "DELETE FROM account_rate_limits WHERE key_hash IN (SELECT key_hash FROM account_rate_limits WHERE updated_at<now()-interval '1 day' AND cooldown_until<now() LIMIT 500)")
	return e
}
func interval(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 6, 64) + " seconds"
}
