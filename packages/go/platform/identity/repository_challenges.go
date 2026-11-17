package identity

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

func (s *Service) mfaReady(ctx context.Context) error {
	rows, e := s.repo.pool.Query(ctx, `SELECT key_id FROM account_mfa WHERE key_id IS NOT NULL UNION SELECT key_id FROM account_auth_challenges WHERE key_id IS NOT NULL`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return e
		}
		if s.mfa.keys[id] == nil {
			return ErrUnavailable
		}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	var complete bool
	e = s.repo.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_security_policy WHERE id=1) AND NOT EXISTS(SELECT 1 FROM accounts a LEFT JOIN account_mfa f ON f.account_id=a.id WHERE f.account_id IS NULL)`).Scan(&complete)
	if e == nil && !complete {
		return ErrUnavailable
	}
	return e
}
func lockAuthAccount(ctx context.Context, tx pgx.Tx, id string) (Account, int64, error) {
	var a Account
	var v int64
	e := tx.QueryRow(ctx, `SELECT id::text,username,display_name,status,created_at,auth_version FROM accounts WHERE id=$1 FOR UPDATE`, id).Scan(&a.ID, &a.Username, &a.DisplayName, &a.Status, &a.CreatedAt, &v)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrChallenge
	}
	return a, v, e
}
func createSession(ctx context.Context, tx pgx.Tx, a Account, version int64, hash, method string, f mfaFactor, policy SecurityPolicy, passwordAt, now, expiry time.Time, m Meta) (Principal, error) {
	p := Principal{Account: a, Version: version, TokenHash: hash, ReauthenticatedAt: passwordAt, PolicyGeneration: policy.Generation}
	p.Session.AuthenticationMethod = method
	if method != "password" {
		p.MFAVerifiedAt = &now
		p.MFAFactorVersion = &f.Version
	}
	e := tx.QueryRow(ctx, `INSERT INTO account_sessions(token_hash,account_id,auth_version,expires_at,user_agent,ip,reauthenticated_at,authentication_method,mfa_verified_at,mfa_factor_version,policy_enforcement_generation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text,created_at,last_activity_at,expires_at`, hash, a.ID, version, expiry, limit(m.UserAgent, 300), limit(m.IP, 100), passwordAt, method, p.MFAVerifiedAt, p.MFAFactorVersion, policy.Generation).Scan(&p.Session.ID, &p.Session.CreatedAt, &p.Session.LastActivityAt, &p.Session.ExpiresAt)
	p.Session.UserAgent = limit(m.UserAgent, 300)
	p.Session.IP = limit(m.IP, 100)
	if e == nil {
		e = audit(ctx, tx, "login", a.ID, a.ID, p.Session.ID, "success", m)
	}
	return p, e
}
func (s *Service) deliver(ctx context.Context, p Principal, token string, result LoginResult) (AuthDelivery, error) {
	if e := s.cache.Put(ctx, p, s.cacheTTL(p)); e != nil {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.repo.RevokeToken(clean, p.TokenHash, Meta{}); err != nil {
			s.logger.Error("failed to revoke undispatched session")
		}
		return AuthDelivery{}, ErrUnavailable
	}
	result.Status = "authenticated"
	result.Account = &p.Account
	return AuthDelivery{Result: result, SessionToken: token, ExpiresAt: p.Session.ExpiresAt}, nil
}
func (s *Service) BeginLogin(ctx context.Context, username, password, previous string, m Meta) (AuthDelivery, error) {
	var out AuthDelivery
	if e := s.Rate(ctx, "login", m.IP, strings.ToLower(username)); e != nil {
		return out, e
	}
	if len(username) > 128 {
		return out, ErrCredentials
	}
	// Argon2 work occurs before policy/account locks. The exact hash and version
	// are checked again under the common lock sequence before issuing anything.
	var id, hash, status string
	var checkedVersion int64
	e := s.repo.pool.QueryRow(ctx, `SELECT a.id::text,p.password_hash,a.status,a.auth_version FROM accounts a JOIN account_passwords p ON p.account_id=a.id WHERE a.username_normalized=$1`, strings.ToLower(username)).Scan(&id, &hash, &status, &checkedVersion)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	found := e == nil
	if !found {
		hash = s.dummy
	}
	verified := false
	e = s.passwordWork(ctx, func() error { verified = VerifyPassword(hash, password); return nil })
	if e != nil {
		return out, e
	}
	token, e := newToken()
	if e != nil {
		return out, e
	}
	var p Principal
	var outcome error
	e = s.repo.transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		policy, e := lockSecurityPolicy(ctx, tx, false)
		if e != nil {
			return e
		}
		var a Account
		var version int64
		if found {
			a, version, e = lockAuthAccount(ctx, tx, id)
			if e != nil {
				return e
			}
			var current string
			if e = tx.QueryRow(ctx, `SELECT password_hash FROM account_passwords WHERE account_id=$1`, id).Scan(&current); e != nil {
				return e
			}
			if current != hash || checkedVersion != version {
				return ErrCredentials
			}
		}
		key := digest("password:" + strings.ToLower(username))
		cooling, e := passwordCooldown(ctx, tx, key)
		if e != nil {
			return e
		}
		if cooling {
			outcome = ErrRate
			return audit(ctx, tx, "login", "", id, "", "rate_limited", m)
		}
		if !found || !verified || status != "active" || a.Status != "active" {
			outcome = ErrCredentials
			if e = passwordFailure(ctx, tx, key, s.cfg.PasswordFailures); e != nil {
				return e
			}
			return audit(ctx, tx, "login", "", id, "", "failure", m)
		}
		if e = passwordSuccess(ctx, tx, key); e != nil {
			return e
		}
		f, e := loadFactor(ctx, tx, id)
		if e != nil {
			return e
		}
		if validToken(previous) {
			if _, e = tx.Exec(ctx, `UPDATE account_auth_challenges SET state='revoked',revoked_at=now(),secret_ciphertext=NULL,secret_nonce=NULL,key_id=NULL WHERE token_hash=$1 AND state='pending'`, challengeDigest(previous)); e != nil {
				return e
			}
		}
		now := s.now()
		out.ExpiresAt = now.Add(s.cfg.SessionAbsolute)
		if f.State == "disabled" && !policy.RequireMFA {
			p, e = createSession(ctx, tx, a, version, digest(token), "password", f, policy, now, now, out.ExpiresAt, m)
			return e
		}
		purpose := "login_verify"
		ttl := 5 * time.Minute
		if f.State == "reset_required" {
			purpose = "login_recover"
		} else if f.State == "disabled" {
			purpose = "login_enroll"
			ttl = 10 * time.Minute
		}
		var c authChallenge
		c = authChallenge{AccountID: id, Purpose: purpose, State: "pending", AuthVersion: version, FactorVersion: f.Version, ExpiresAt: now.Add(ttl), PasswordAt: now}
		e = tx.QueryRow(ctx, `INSERT INTO account_auth_challenges(account_id,token_hash,purpose,expires_at,account_auth_version,factor_version,policy_revision,password_verified_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, id, challengeDigest(token), purpose, c.ExpiresAt, version, f.Version, policy.Revision, now).Scan(&c.ID)
		out.Result.LoginChallenge = challengeView(c, a, policy)
		out.ChallengeToken = token
		out.ExpiresAt = c.ExpiresAt
		return e
	})
	if e != nil {
		return AuthDelivery{}, e
	}
	if outcome != nil {
		return AuthDelivery{}, outcome
	}
	if p.Session.ID != "" {
		return s.deliver(ctx, p, token, LoginResult{})
	}
	return out, nil
}
func challengeView(c authChallenge, a Account, policy SecurityPolicy) LoginChallenge {
	status := "mfa_required"
	var methods []string
	switch c.Purpose {
	case "login_enroll":
		status = "mfa_enrollment_required"
	case "login_recover":
		status = "mfa_recovery_required"
	default:
		methods = []string{"totp", "recovery_code"}
	}
	return LoginChallenge{Status: status, ID: c.ID, AccountLabel: a.Username, ExpiresAt: c.ExpiresAt, Methods: methods, PolicyRequired: policy.RequireMFA}
}
func readChallenge(ctx context.Context, tx pgx.Tx, id string) (authChallenge, error) {
	var c authChallenge
	e := tx.QueryRow(ctx, `SELECT id::text,account_id::text,purpose,state,account_auth_version,factor_version,expires_at,COALESCE(password_verified_at,created_at),COALESCE(origin_session_id::text,''),COALESCE(recovery_grant_id::text,''),secret_ciphertext,secret_nonce,COALESCE(key_id,''),failed_attempts FROM account_auth_challenges WHERE id=$1 FOR UPDATE`, id).Scan(&c.ID, &c.AccountID, &c.Purpose, &c.State, &c.AuthVersion, &c.FactorVersion, &c.ExpiresAt, &c.PasswordAt, &c.Origin, &c.Grant, &c.Secret.Ciphertext, &c.Secret.Nonce, &c.Secret.KeyID, &c.Failures)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrChallenge
	}
	return c, e
}
func (s *Service) loginChallengeTx(ctx context.Context, tx pgx.Tx, token, expected string) (authChallenge, Account, mfaFactor, SecurityPolicy, error) {
	var c authChallenge
	var a Account
	var f mfaFactor
	var policy SecurityPolicy
	invalid := func(e error) (authChallenge, Account, mfaFactor, SecurityPolicy, error) { return c, a, f, policy, e }
	if !validToken(token) || (expected != "" && !uuidPattern.MatchString(expected)) {
		return invalid(ErrChallenge)
	}
	if _, e := access.Lock(ctx, tx, false); e != nil {
		return invalid(e)
	}
	var e error
	policy, e = lockSecurityPolicy(ctx, tx, false)
	if e != nil {
		return invalid(e)
	}
	var id, account string
	e = tx.QueryRow(ctx, `SELECT id::text,account_id::text FROM account_auth_challenges WHERE token_hash=$1 AND purpose IN ('login_verify','login_enroll','login_recover')`, challengeDigest(token)).Scan(&id, &account)
	if errors.Is(e, pgx.ErrNoRows) {
		return invalid(ErrChallenge)
	}
	if e != nil {
		return invalid(e)
	}
	if expected != "" && id != expected {
		return invalid(ErrChallenge)
	}
	var version int64
	a, version, e = lockAuthAccount(ctx, tx, account)
	if e != nil {
		return invalid(e)
	}
	f, e = loadFactor(ctx, tx, account)
	if e != nil {
		return invalid(e)
	}
	c, e = readChallenge(ctx, tx, id)
	if e != nil {
		return invalid(e)
	}
	if c.State == "completed" {
		return invalid(ErrChallengeCompleted)
	}
	if c.State != "pending" || !c.ExpiresAt.After(s.now()) || a.Status != "active" || c.AuthVersion != version || c.FactorVersion != f.Version {
		return invalid(ErrChallenge)
	}
	return c, a, f, policy, nil
}
func (s *Service) Challenge(ctx context.Context, token string) (LoginChallenge, error) {
	out := LoginChallenge{Status: "none"}
	if !validToken(token) {
		return out, nil
	}
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		c, a, _, p, e := s.loginChallengeTx(ctx, tx, token, "")
		if e != nil {
			return e
		}
		out = challengeView(c, a, p)
		return nil
	})
	if errors.Is(e, ErrChallenge) || errors.Is(e, ErrChallengeCompleted) {
		return LoginChallenge{Status: "none"}, nil
	}
	return out, e
}
func (s *Service) CancelChallenge(ctx context.Context, token, id string) error {
	if !validToken(token) || !uuidPattern.MatchString(id) {
		return ErrChallenge
	}
	return s.repo.transact(ctx, func(tx pgx.Tx) error {
		var matched bool
		e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_auth_challenges WHERE id=$1 AND token_hash=$2 AND purpose IN ('login_verify','login_enroll','login_recover'))`, id, challengeDigest(token)).Scan(&matched)
		if e != nil {
			return e
		}
		if !matched {
			return ErrChallenge
		}
		_, e = tx.Exec(ctx, `UPDATE account_auth_challenges SET state='revoked',revoked_at=now(),secret_ciphertext=NULL,secret_nonce=NULL,key_id=NULL WHERE id=$1 AND state='pending'`, id)
		return e
	})
}
func (s *Service) CancelCurrentChallenge(ctx context.Context, token string) error {
	if !validToken(token) {
		return nil
	}
	_, e := s.repo.pool.Exec(ctx, `UPDATE account_auth_challenges SET state='revoked',revoked_at=now(),secret_ciphertext=NULL,secret_nonce=NULL,key_id=NULL WHERE token_hash=$1 AND state='pending'`, challengeDigest(token))
	return e
}
func mfaCooldown(ctx context.Context, tx pgx.Tx, account string) (error, error) {
	key := digest("mfa:" + account)
	if _, e := passwordCooldown(ctx, tx, key); e != nil {
		return nil, e
	}
	var seconds float64
	e := tx.QueryRow(ctx, `SELECT GREATEST(0,EXTRACT(EPOCH FROM cooldown_until-clock_timestamp()))::float8 FROM account_rate_limits WHERE key_hash=$1`, key).Scan(&seconds)
	if e != nil {
		return nil, e
	}
	if seconds > 0 {
		return &Error{Code: "rate_limited", Message: "验证码尝试过于频繁，请稍后重试。", Status: 429, RetryAfter: int(math.Ceil(seconds))}, nil
	}
	return nil, nil
}

// The account bucket is shared across challenge types, API nodes and source IPs.
// Only a verified challenge may reach it; anonymous guesses create no account key.
func (s *Service) mfaAccountRate(ctx context.Context, account string) error {
	return s.cache.Rate(ctx, "mfa:credential", account, s.cfg.CredentialRequestLimit, time.Minute)
}
func mfaFailure(ctx context.Context, tx pgx.Tx, c authChallenge, m Meta) error {
	if e := passwordFailure(ctx, tx, digest("mfa:"+c.AccountID), 5); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `UPDATE account_auth_challenges SET failed_attempts=failed_attempts+1,state=CASE WHEN failed_attempts>=4 THEN 'revoked' ELSE state END,revoked_at=CASE WHEN failed_attempts>=4 THEN now() ELSE revoked_at END,secret_ciphertext=CASE WHEN failed_attempts>=4 THEN NULL ELSE secret_ciphertext END,secret_nonce=CASE WHEN failed_attempts>=4 THEN NULL ELSE secret_nonce END,key_id=CASE WHEN failed_attempts>=4 THEN NULL ELSE key_id END WHERE id=$1`, c.ID)
	if e != nil {
		return e
	}
	return audit(ctx, tx, "mfa_verification", "", c.AccountID, c.ID, "failure", m, map[string]any{"phase": c.Purpose, "failure_category": "invalid_code", "failed_attempts": c.Failures + 1})
}
func completeChallenge(ctx context.Context, tx pgx.Tx, id string) error {
	_, e := tx.Exec(ctx, `UPDATE account_auth_challenges SET state='completed',completed_at=now(),secret_ciphertext=NULL,secret_nonce=NULL,key_id=NULL WHERE id=$1 AND state='pending'`, id)
	return e
}
func (s *Service) CompleteLogin(ctx context.Context, token, id, method, code string, m Meta) (AuthDelivery, error) {
	if !uuidPattern.MatchString(id) {
		return AuthDelivery{}, ErrChallenge
	}
	if method != "totp" && method != "recovery_code" {
		return AuthDelivery{}, FieldError("method", "请选择动态码或恢复码。")
	}
	if e := s.Rate(ctx, "mfa", m.IP, ""); e != nil {
		return AuthDelivery{}, e
	}
	next, e := newToken()
	if e != nil {
		return AuthDelivery{}, e
	}
	var p Principal
	var outcome error
	e = s.repo.transact(ctx, func(tx pgx.Tx) error {
		c, a, f, policy, e := s.loginChallengeTx(ctx, tx, token, id)
		if e != nil {
			return e
		}
		if c.Purpose != "login_verify" || f.State != "enabled" {
			return ErrChallenge
		}
		if e = s.mfaAccountRate(ctx, a.ID); e != nil {
			return e
		}
		if outcome, e = mfaCooldown(ctx, tx, a.ID); e != nil || outcome != nil {
			return e
		}
		accepted := false
		authMethod := "password_totp"
		if method == "totp" {
			secret, e := s.mfa.open(f.Secret, a.ID, "factor", f.Version)
			if e != nil {
				return e
			}
			step, ok := matchTOTPStep(secret, code, s.now(), f.LastStep)
			if ok {
				tag, e := tx.Exec(ctx, `UPDATE account_mfa SET last_used_step=$2 WHERE account_id=$1 AND (last_used_step IS NULL OR last_used_step<$2)`, a.ID, step)
				if e != nil {
					return e
				}
				accepted = tag.RowsAffected() == 1
			}
		} else {
			authMethod = "password_recovery_code"
			tag, e := tx.Exec(ctx, `UPDATE account_mfa_recovery_codes SET used_at=now() WHERE account_id=$1 AND factor_version=$2 AND generation=$3 AND code_hash=$4 AND used_at IS NULL AND revoked_at IS NULL`, a.ID, f.Version, f.Generation, recoveryDigest(a.ID, f.Generation, code))
			if e != nil {
				return e
			}
			accepted = tag.RowsAffected() == 1
		}
		if !accepted {
			outcome = ErrMFACode
			return mfaFailure(ctx, tx, c, m)
		}
		if e = passwordSuccess(ctx, tx, digest("mfa:"+a.ID)); e != nil {
			return e
		}
		if e = completeChallenge(ctx, tx, c.ID); e != nil {
			return e
		}
		if method == "recovery_code" {
			if e = audit(ctx, tx, "mfa_recovery_code_used", a.ID, a.ID, c.ID, "success", m); e != nil {
				return e
			}
		}
		p, e = createSession(ctx, tx, a, c.AuthVersion, digest(next), authMethod, f, policy, c.PasswordAt, s.now(), s.now().Add(s.cfg.SessionAbsolute), m)
		return e
	})
	if e != nil {
		return AuthDelivery{}, e
	}
	if outcome != nil {
		return AuthDelivery{}, outcome
	}
	return s.deliver(ctx, p, next, LoginResult{OperationID: id})
}
