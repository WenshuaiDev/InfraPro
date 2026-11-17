package testsupport

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/jackc/pgx/v5"
)

// LegacyPrincipal seeds the pre-MFA columns for genuine historical upgrade
// fixtures. Production identity must never operate against these old schemas.
func (e *Environment) LegacyPrincipal(ctx context.Context, username, password string, admin bool) (identity.Principal, error) {
	var p identity.Principal
	if e.BaseMigration >= 56 {
		return p, fmt.Errorf("legacy fixture requires a pre-MFA schema")
	}
	hash, err := identity.HashPassword(password)
	if err != nil {
		return p, err
	}
	tx, err := e.Pool.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(ctx)
	if _, err = access.Lock(ctx, tx, true); err != nil {
		return p, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO accounts(username,username_normalized,display_name) VALUES($1,$2,$1) RETURNING id::text,username,display_name,status,created_at,auth_version`, username, strings.ToLower(username)).Scan(&p.Account.ID, &p.Account.Username, &p.Account.DisplayName, &p.Account.Status, &p.Account.CreatedAt, &p.Version)
	if err != nil {
		return p, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO account_passwords(account_id,password_hash) VALUES($1,$2)`, p.Account.ID, hash); err != nil {
		return p, err
	}
	role := access.MemberKey
	if admin {
		role = access.AdminKey
	}
	if err = access.BindBuiltin(ctx, tx, p.Account.ID, role, access.Meta{ActorType: "maintenance", Reason: "历史版本迁移夹具"}); err != nil {
		return p, err
	}
	token := sha256.Sum256([]byte(rand.Text()))
	p.TokenHash = hex.EncodeToString(token[:])
	p.ReauthenticatedAt = time.Now()
	err = tx.QueryRow(ctx, `INSERT INTO account_sessions(account_id,token_hash,auth_version,expires_at,reauthenticated_at,user_agent,ip) VALUES($1,$2,$3,now()+interval '12 hours',now(),'historical fixture','127.0.0.1') RETURNING id::text,created_at,last_activity_at,expires_at`, p.Account.ID, p.TokenHash, p.Version).Scan(&p.Session.ID, &p.Session.CreatedAt, &p.Session.LastActivityAt, &p.Session.ExpiresAt)
	if err != nil {
		return p, err
	}
	p.Session.AuthenticationMethod = "password"
	return p, tx.Commit(ctx)
}

// FixtureActor retains the old session predicates until migration creates MFA
// state; subsequent calls immediately switch to the production proof predicate.
func (e *Environment) FixtureActor(p identity.Principal) access.Actor {
	if e.BaseMigration >= 56 {
		return e.Service.AccessActor(p)
	}
	return access.Actor{ID: p.Account.ID, Check: func(ctx context.Context, tx pgx.Tx, recent bool) error {
		var modern bool
		if err := tx.QueryRow(ctx, `SELECT to_regclass('account_mfa') IS NOT NULL`).Scan(&modern); err != nil {
			return err
		}
		if modern {
			return e.Service.AccessActor(p).Check(ctx, tx, recent)
		}
		var valid bool
		var at time.Time
		err := tx.QueryRow(ctx, `SELECT a.status='active' AND a.auth_version=$3 AND s.auth_version=$3 AND s.revoked_at IS NULL AND s.expires_at>now() AND s.last_activity_at>now()-interval '2 hours',s.reauthenticated_at FROM accounts a JOIN account_sessions s ON s.account_id=a.id WHERE a.id=$1 AND s.id=$2 FOR UPDATE OF a`, p.Account.ID, p.Session.ID, p.Version).Scan(&valid, &at)
		if err != nil {
			return err
		}
		if !valid {
			return identity.ErrUnauthorized
		}
		if recent && time.Since(at) > 5*time.Minute {
			return identity.ErrReauth
		}
		return nil
	}}
}
