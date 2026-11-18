package identity

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func passwordCooldown(ctx context.Context, tx pgx.Tx, key string) (bool, error) {
	if _, e := tx.Exec(ctx, "INSERT INTO account_rate_limits(key_hash) VALUES($1) ON CONFLICT DO NOTHING", key); e != nil {
		return false, e
	}
	var cooling bool
	e := tx.QueryRow(ctx, "SELECT cooldown_until>now() FROM account_rate_limits WHERE key_hash=$1 FOR UPDATE", key).Scan(&cooling)
	return cooling, e
}
func passwordFailure(ctx context.Context, tx pgx.Tx, key string, threshold int) error {
	_, e := tx.Exec(ctx, `UPDATE account_rate_limits SET failures=LEAST(failures+1,1000),cooldown_until=CASE WHEN failures+1 >= $2 THEN now()+make_interval(secs=>LEAST(900,30*power(2,LEAST(failures+1-$2,5)))::double precision) ELSE now() END,updated_at=now() WHERE key_hash=$1`, key, threshold)
	return e
}
func passwordSuccess(ctx context.Context, tx pgx.Tx, key string) error {
	_, e := tx.Exec(ctx, "DELETE FROM account_rate_limits WHERE key_hash=$1", key)
	return e
}
func (r *Repository) LoadSession(ctx context.Context, hash string) (Principal, error) {
	var p Principal
	e := r.transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		policy, e := lockSecurityPolicy(ctx, tx, false)
		if e != nil {
			return e
		}
		var account string
		e = tx.QueryRow(ctx, "SELECT account_id::text FROM account_sessions WHERE token_hash=$1", hash).Scan(&account)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrUnauthorized
		}
		if e != nil {
			return e
		}
		if _, _, e = lockAuthAccount(ctx, tx, account); e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `SELECT s.id::text,s.account_id::text,s.auth_version,s.created_at,s.last_activity_at,s.expires_at,s.reauthenticated_at,s.user_agent,s.ip,a.username,a.display_name,a.status,a.created_at,a.avatar_revision::text,a.avatar_object_id IS NOT NULL FROM account_sessions s JOIN accounts a ON a.id=s.account_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND a.status='active' AND a.auth_version=s.auth_version`, hash).Scan(&p.Session.ID, &p.Account.ID, &p.Version, &p.Session.CreatedAt, &p.Session.LastActivityAt, &p.Session.ExpiresAt, &p.ReauthenticatedAt, &p.Session.UserAgent, &p.Session.IP, &p.Account.Username, &p.Account.DisplayName, &p.Account.Status, &p.Account.CreatedAt, &p.Account.AvatarRevision, &p.Account.HasAvatar)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrUnauthorized
		}
		if e != nil {
			return e
		}
		p.TokenHash = hash
		return checkMFASession(ctx, tx, p.Account.ID, p.Session.ID, policy, &p)
	})
	return p, e
}
func (r *Repository) CheckSession(ctx context.Context, p *Principal) error {
	fresh, e := r.LoadSession(ctx, p.TokenHash)
	if e != nil {
		return e
	}
	*p = fresh
	return nil
}
func revokeSessions(ctx context.Context, tx pgx.Tx, account, id string) error {
	if e := revokeChallenges(ctx, tx, account, id); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `WITH revoked AS (UPDATE account_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE account_id=$1 AND ($2='' OR id::text=$2) RETURNING id,token_hash) INSERT INTO account_cache_invalidations(session_id,token_hash) SELECT id,token_hash FROM revoked ON CONFLICT DO NOTHING`, account, id)
	return e
}
func (r *Repository) RevokeToken(ctx context.Context, hash string, m Meta) error {
	return r.transact(ctx, func(tx pgx.Tx) error {
		var account, id string
		e := tx.QueryRow(ctx, `SELECT a.id::text,s.id::text FROM accounts a JOIN account_sessions s ON s.account_id=a.id WHERE s.token_hash=$1 FOR UPDATE OF a`, hash).Scan(&account, &id)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		if e = revokeSessions(ctx, tx, account, id); e != nil {
			return e
		}
		return audit(ctx, tx, "logout", account, account, id, "success", m)
	})
}
func (r *Repository) Revoke(ctx context.Context, p Principal, id string, idle time.Duration, m Meta) error {
	return r.transact(ctx, func(tx pgx.Tx) error {
		if e := access.Require(ctx, tx, p.Account.ID, access.OpSessionRevoke); e != nil {
			return e
		}
		if e := lockPrincipal(ctx, tx, p, idle, false); e != nil {
			return e
		}
		if id != "" {
			var own bool
			if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM account_sessions WHERE id=$1 AND account_id=$2)", id, p.Account.ID).Scan(&own); e != nil {
				return e
			}
			if !own {
				return ErrNotFound
			}
		}
		if e := revokeSessions(ctx, tx, p.Account.ID, id); e != nil {
			return e
		}
		event := "session_revoked"
		if id == "" {
			event = "logout_all"
		}
		return securityReceipt(ctx, tx, event, p.Account.ID, p.Account.ID, id, m)
	})
}
func (r *Repository) ConfirmPassword(ctx context.Context, p Principal, password, newHash string, idle time.Duration, threshold int, m Meta) error {
	// Expensive verification happens before the authorization lock; the hash is rechecked inside.
	var checkedHash string
	if e := r.pool.QueryRow(ctx, "SELECT password_hash FROM account_passwords WHERE account_id=$1", p.Account.ID).Scan(&checkedHash); e != nil {
		return e
	}
	verified := VerifyPassword(checkedHash, password)

	var outcome error
	e := r.transact(ctx, func(tx pgx.Tx) error {
		if newHash != "" {
			if e := access.Require(ctx, tx, p.Account.ID, access.OpPasswordUpdate); e != nil {
				return e
			}
		}
		if err := lockPrincipal(ctx, tx, p, idle, false); err != nil {
			return err
		}
		key := digest("password:" + strings.ToLower(p.Account.Username))
		cooling, err := passwordCooldown(ctx, tx, key)
		if err != nil {
			return err
		}
		if cooling {
			outcome = ErrRate
			return nil
		}
		var old string
		if err = tx.QueryRow(ctx, "SELECT password_hash FROM account_passwords WHERE account_id=$1", p.Account.ID).Scan(&old); err != nil {
			return err
		}
		event := "reauthentication"
		if newHash != "" {
			event = "password_changed"
		}
		if old != checkedHash {
			return ErrUnauthorized
		}
		if !verified {
			outcome = FieldError("current_password", "当前密码不正确。")
			if err = passwordFailure(ctx, tx, key, threshold); err != nil {
				return err
			}
			return audit(ctx, tx, event, p.Account.ID, p.Account.ID, "", "failure", m)
		}
		if err = passwordSuccess(ctx, tx, key); err != nil {
			return err
		}
		if newHash == "" {
			_, err = tx.Exec(ctx, "UPDATE account_sessions SET reauthenticated_at=now() WHERE id=$1", p.Session.ID)
		} else {
			if _, err = tx.Exec(ctx, "UPDATE account_passwords SET password_hash=$2,updated_at=now() WHERE account_id=$1", p.Account.ID, newHash); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "UPDATE accounts SET auth_version=auth_version+1,updated_at=now() WHERE id=$1", p.Account.ID); err != nil {
				return err
			}
			if err = revokeSessions(ctx, tx, p.Account.ID, ""); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, "UPDATE account_password_resets SET revoked_at=now() WHERE account_id=$1 AND used_at IS NULL AND revoked_at IS NULL", p.Account.ID)
		}
		if err != nil {
			return err
		}
		return securityReceipt(ctx, tx, event, p.Account.ID, p.Account.ID, "", m)
	})
	if e != nil {
		return e
	}
	return outcome
}
func (r *Repository) Sessions(ctx context.Context, tx pgx.Tx, id string) ([]Principal, error) {
	rows, e := tx.Query(ctx, `SELECT s.id::text,s.token_hash,s.auth_version,s.created_at,s.last_activity_at,s.expires_at,s.user_agent,s.ip,s.authentication_method FROM account_sessions s JOIN accounts a ON a.id=s.account_id WHERE s.account_id=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND s.auth_version=a.auth_version AND a.status='active' ORDER BY s.created_at DESC`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	list := []Principal{}
	for rows.Next() {
		var p Principal
		p.Account.ID = id
		if e = rows.Scan(&p.Session.ID, &p.TokenHash, &p.Version, &p.Session.CreatedAt, &p.Session.LastActivityAt, &p.Session.ExpiresAt, &p.Session.UserAgent, &p.Session.IP, &p.Session.AuthenticationMethod); e != nil {
			return nil, e
		}
		list = append(list, p)
	}
	return list, rows.Err()
}
func (r *Repository) SaveActivity(ctx context.Context, id string, version int64, at time.Time, idle time.Duration) error {
	// The verified Redis activity may be ahead of durable activity. It may never revive a revoked session.
	_, e := r.pool.Exec(ctx, `UPDATE account_sessions s SET last_activity_at=GREATEST(s.last_activity_at,$3) FROM accounts a WHERE s.id=$1 AND a.id=s.account_id AND s.auth_version=$2 AND a.auth_version=$2 AND a.status='active' AND s.revoked_at IS NULL AND s.expires_at>now() AND $3::timestamptz<=now() AND $3::timestamptz>now()-$4::interval`, id, version, at, interval(idle))
	return e
}

type invalidation struct{ ID, Hash string }

func (r *Repository) Invalidations(ctx context.Context) ([]invalidation, error) {
	rows, e := r.pool.Query(ctx, "SELECT session_id::text,token_hash FROM account_cache_invalidations ORDER BY created_at LIMIT 500")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []invalidation{}
	for rows.Next() {
		var item invalidation
		if e = rows.Scan(&item.ID, &item.Hash); e != nil {
			return nil, e
		}
		v = append(v, item)
	}
	return v, rows.Err()
}
func (r *Repository) InvalidationDone(ctx context.Context, id string) error {
	_, e := r.pool.Exec(ctx, "DELETE FROM account_cache_invalidations WHERE session_id=$1", id)
	return e
}
