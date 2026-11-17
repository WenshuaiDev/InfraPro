package identity

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func statusTx(ctx context.Context, tx pgx.Tx, account string, policy SecurityPolicy) (MFAStatus, error) {
	f, e := loadFactor(ctx, tx, account)
	if e != nil {
		return MFAStatus{}, e
	}
	v := MFAStatus{State: f.State, FactorVersion: strconv.FormatInt(f.Version, 10), RecoveryGeneration: strconv.FormatInt(f.Generation, 10), EnabledAt: f.EnabledAt, PolicyRequired: policy.RequireMFA, CanDisable: f.State == "enabled" && !policy.RequireMFA}
	e = tx.QueryRow(ctx, `SELECT count(*) FROM account_mfa_recovery_codes WHERE account_id=$1 AND factor_version=$2 AND generation=$3 AND used_at IS NULL AND revoked_at IS NULL`, account, f.Version, f.Generation).Scan(&v.RecoveryCodesRemaining)
	return v, e
}
func (s *Service) MFA(ctx context.Context, p Principal) (MFAStatus, error) {
	var v MFAStatus
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		if e := lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, false); e != nil {
			return e
		}
		policy, e := lockSecurityPolicy(ctx, tx, false)
		if e != nil {
			return e
		}
		v, e = statusTx(ctx, tx, p.Account.ID, policy)
		return e
	})
	return v, e
}
func revokeChallenges(ctx context.Context, tx pgx.Tx, account, session string) error {
	_, e := tx.Exec(ctx, `UPDATE account_auth_challenges SET state='revoked',revoked_at=now(),secret_ciphertext=NULL,secret_nonce=NULL,key_id=NULL WHERE account_id=$1 AND state='pending' AND ($2='' OR origin_session_id::text=$2)`, account, session)
	return e
}
func (s *Service) setupTx(ctx context.Context, tx pgx.Tx, c authChallenge, a Account) (EnrollmentSetup, error) {
	var secret string
	var e error
	if c.Secret.KeyID == "" {
		secret, e = s.mfa.newSecret()
		if e != nil {
			return EnrollmentSetup{}, e
		}
		c.Secret, e = s.mfa.seal(secret, a.ID, "challenge:"+c.ID, c.FactorVersion)
		if e != nil {
			return EnrollmentSetup{}, e
		}
		if _, e = tx.Exec(ctx, `UPDATE account_auth_challenges SET secret_ciphertext=$2,secret_nonce=$3,key_id=$4 WHERE id=$1`, c.ID, c.Secret.Ciphertext, c.Secret.Nonce, c.Secret.KeyID); e != nil {
			return EnrollmentSetup{}, e
		}
	} else {
		secret, e = s.mfa.open(c.Secret, a.ID, "challenge:"+c.ID, c.FactorVersion)
		if e != nil {
			return EnrollmentSetup{}, e
		}
	}
	qr, e := enrollmentQR(secret, s.cfg.MFAIssuer, a.Username)
	return EnrollmentSetup{ID: c.ID, Issuer: s.cfg.MFAIssuer, AccountLabel: a.Username, Secret: secret, QR: qr, ExpiresAt: c.ExpiresAt}, e
}
func (s *Service) StartLoginEnrollment(ctx context.Context, token, id string, m Meta) (EnrollmentSetup, error) {
	var v EnrollmentSetup
	if !uuidPattern.MatchString(id) {
		return v, ErrChallenge
	}
	if e := s.Rate(ctx, "mfa", m.IP, ""); e != nil {
		return v, e
	}
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		c, a, _, _, e := s.loginChallengeTx(ctx, tx, token, id)
		if e != nil {
			return e
		}
		if c.Purpose != "login_enroll" {
			return ErrChallenge
		}
		if e = s.mfaAccountRate(ctx, a.ID); e != nil {
			return e
		}
		v, e = s.setupTx(ctx, tx, c, a)
		return e
	})
	return v, e
}
func (s *Service) StartSelfEnrollment(ctx context.Context, p Principal, mode, expected string, m Meta) (EnrollmentSetup, error) {
	var v EnrollmentSetup
	if mode != "enable" && mode != "replace" {
		return v, FieldError("mode", "请选择启用或更换验证器。")
	}
	if e := s.Rate(ctx, "mfa-self", m.IP, p.Account.ID); e != nil {
		return v, e
	}
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		if e := lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, true); e != nil {
			return e
		}
		policy, e := lockSecurityPolicy(ctx, tx, false)
		if e != nil {
			return e
		}
		f, e := loadFactor(ctx, tx, p.Account.ID)
		if e != nil {
			return e
		}
		if expected != strconv.FormatInt(f.Version, 10) {
			return ErrVersion
		}
		if (mode == "enable" && f.State != "disabled") || (mode == "replace" && f.State != "enabled") {
			return ErrMFAState
		}
		purpose := "self_enroll"
		if mode == "replace" {
			purpose = "self_replace"
		}
		var existing string
		e = tx.QueryRow(ctx, `SELECT id::text FROM account_auth_challenges WHERE account_id=$1 AND origin_session_id=$2 AND purpose=$3 AND state='pending' AND expires_at>$4 AND account_auth_version=$5 AND factor_version=$6`, p.Account.ID, p.Session.ID, purpose, s.now(), p.Version, f.Version).Scan(&existing)
		if e == nil {
			c, e := readChallenge(ctx, tx, existing)
			if e != nil {
				return e
			}
			v, e = s.setupTx(ctx, tx, c, p.Account)
			return e
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE account_auth_challenges SET state='revoked',revoked_at=now(),secret_ciphertext=NULL,secret_nonce=NULL,key_id=NULL WHERE account_id=$1 AND purpose IN ('self_enroll','self_replace') AND state='pending'`, p.Account.ID); e != nil {
			return e
		}
		var passwordAt time.Time
		if e = tx.QueryRow(ctx, `SELECT reauthenticated_at FROM account_sessions WHERE id=$1`, p.Session.ID).Scan(&passwordAt); e != nil {
			return e
		}
		c := authChallenge{ID: uuid.NewString(), AccountID: p.Account.ID, Purpose: purpose, AuthVersion: p.Version, FactorVersion: f.Version, Origin: p.Session.ID, PasswordAt: passwordAt, ExpiresAt: s.now().Add(10 * time.Minute)}
		if _, e = tx.Exec(ctx, `INSERT INTO account_auth_challenges(id,account_id,purpose,expires_at,account_auth_version,factor_version,policy_revision,password_verified_at,origin_session_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, c.ID, c.AccountID, c.Purpose, c.ExpiresAt, c.AuthVersion, c.FactorVersion, policy.Revision, passwordAt, p.Session.ID); e != nil {
			return e
		}
		v, e = s.setupTx(ctx, tx, c, p.Account)
		return e
	})
	return v, e
}
func (s *Service) CancelSelfEnrollment(ctx context.Context, p Principal, id string) error {
	if !uuidPattern.MatchString(id) {
		return ErrChallenge
	}
	return s.repo.transact(ctx, func(tx pgx.Tx) error {
		if e := lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, false); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE account_auth_challenges SET state='revoked',revoked_at=now(),secret_ciphertext=NULL,secret_nonce=NULL,key_id=NULL WHERE id=$1 AND account_id=$2 AND origin_session_id=$3 AND state='pending'`, id, p.Account.ID, p.Session.ID)
		return e
	})
}
func (s *Service) selfChallengeTx(ctx context.Context, tx pgx.Tx, p Principal, id string) (authChallenge, Account, mfaFactor, SecurityPolicy, error) {
	var c authChallenge
	var f mfaFactor
	var policy SecurityPolicy
	invalid := func(e error) (authChallenge, Account, mfaFactor, SecurityPolicy, error) {
		return c, p.Account, f, policy, e
	}
	if !uuidPattern.MatchString(id) {
		return invalid(ErrChallenge)
	}
	if e := lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, true); e != nil {
		return invalid(e)
	}
	var e error
	policy, e = lockSecurityPolicy(ctx, tx, false)
	if e != nil {
		return invalid(e)
	}
	f, e = loadFactor(ctx, tx, p.Account.ID)
	if e != nil {
		return invalid(e)
	}
	c, e = readChallenge(ctx, tx, id)
	if e != nil {
		return invalid(e)
	}
	if c.AccountID != p.Account.ID || c.Origin != p.Session.ID || c.AuthVersion != p.Version || c.FactorVersion != f.Version {
		return invalid(ErrChallenge)
	}
	if c.State == "completed" {
		return invalid(ErrChallengeCompleted)
	}
	if c.State != "pending" || !c.ExpiresAt.After(s.now()) || (c.Purpose != "self_enroll" && c.Purpose != "self_replace") {
		return invalid(ErrChallenge)
	}
	// A password reconfirmation during enrollment is honored, but TOTP itself
	// never refreshes password authentication time.
	e = tx.QueryRow(ctx, `SELECT reauthenticated_at FROM account_sessions WHERE id=$1`, p.Session.ID).Scan(&c.PasswordAt)
	return c, p.Account, f, policy, e
}
func (s *Service) replaceCodesTx(ctx context.Context, tx pgx.Tx, account string, f *mfaFactor) ([]string, error) {
	codes, e := s.mfa.recoveryCodes()
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE account_mfa_recovery_codes SET revoked_at=COALESCE(revoked_at,now()) WHERE account_id=$1`, account); e != nil {
		return nil, e
	}
	f.Generation++
	if _, e = tx.Exec(ctx, `UPDATE account_mfa SET recovery_generation=$2,updated_at=now() WHERE account_id=$1`, account, f.Generation); e != nil {
		return nil, e
	}
	for _, code := range codes {
		if _, e = tx.Exec(ctx, `INSERT INTO account_mfa_recovery_codes(account_id,factor_version,generation,code_hash) VALUES($1,$2,$3,$4)`, account, f.Version, f.Generation, recoveryDigest(account, f.Generation, code)); e != nil {
			return nil, e
		}
	}
	return codes, nil
}

// The sole factor installation path is shared by required login, recovery and
// self-service. The challenge, new factor, codes and session commit together.
func (s *Service) ConfirmEnrollment(ctx context.Context, p *Principal, token, id, code string, m Meta) (AuthDelivery, error) {
	if !uuidPattern.MatchString(id) {
		return AuthDelivery{}, ErrChallenge
	}
	if e := s.Rate(ctx, "mfa", m.IP, ""); e != nil {
		return AuthDelivery{}, e
	}
	next, e := newToken()
	if e != nil {
		return AuthDelivery{}, e
	}
	var session Principal
	var codes []string
	var outcome error
	e = s.repo.transact(ctx, func(tx pgx.Tx) error {
		var c authChallenge
		var a Account
		var f mfaFactor
		var policy SecurityPolicy
		var e error
		if p == nil {
			c, a, f, policy, e = s.loginChallengeTx(ctx, tx, token, id)
		} else {
			c, a, f, policy, e = s.selfChallengeTx(ctx, tx, *p, id)
		}
		if e != nil {
			return e
		}
		if (c.Purpose != "login_enroll" && c.Purpose != "self_enroll" && c.Purpose != "self_replace") || c.Secret.KeyID == "" {
			return ErrChallenge
		}
		if e = s.mfaAccountRate(ctx, a.ID); e != nil {
			return e
		}
		if outcome, e = mfaCooldown(ctx, tx, a.ID); e != nil || outcome != nil {
			return e
		}
		secret, e := s.mfa.open(c.Secret, a.ID, "challenge:"+c.ID, c.FactorVersion)
		if e != nil {
			return e
		}
		step, ok := matchTOTPStep(secret, code, s.now(), nil)
		if !ok {
			outcome = ErrMFACode
			return mfaFailure(ctx, tx, c, m)
		}
		event := "mfa_enabled"
		if f.State == "enabled" {
			event = "mfa_replaced"
		} else if f.State == "reset_required" {
			if c.Grant == "" {
				return ErrChallenge
			}
			event = "mfa_recovered"
		}
		f.Version++
		f.State = "enabled"
		f.Secret, e = s.mfa.seal(secret, a.ID, "factor", f.Version)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE account_mfa SET state='enabled',factor_version=$2,secret_ciphertext=$3,secret_nonce=$4,key_id=$5,last_used_step=$6,enabled_at=now(),updated_at=now() WHERE account_id=$1`, a.ID, f.Version, f.Secret.Ciphertext, f.Secret.Nonce, f.Secret.KeyID, step); e != nil {
			return e
		}
		if e = completeChallenge(ctx, tx, c.ID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE accounts SET auth_version=auth_version+1,updated_at=now() WHERE id=$1`, a.ID); e != nil {
			return e
		}
		if e = revokeSessions(ctx, tx, a.ID, ""); e != nil {
			return e
		}
		if e = revokeChallenges(ctx, tx, a.ID, ""); e != nil {
			return e
		}
		codes, e = s.replaceCodesTx(ctx, tx, a.ID, &f)
		if e != nil {
			return e
		}
		if e = passwordSuccess(ctx, tx, digest("mfa:"+a.ID)); e != nil {
			return e
		}
		if e = securityReceipt(ctx, tx, event, a.ID, a.ID, a.ID, m, map[string]any{"enable_mode": c.Purpose, "recovery_grant_id": c.Grant}); e != nil {
			return e
		}
		expiry := s.now().Add(s.cfg.SessionAbsolute)
		if p != nil && p.Session.ExpiresAt.Before(expiry) {
			expiry = p.Session.ExpiresAt
		}
		session, e = createSession(ctx, tx, a, c.AuthVersion+1, digest(next), "password_totp", f, policy, c.PasswordAt, s.now(), expiry, m)
		return e
	})
	if e != nil {
		return AuthDelivery{}, e
	}
	if outcome != nil {
		return AuthDelivery{}, outcome
	}
	return s.deliver(ctx, session, next, LoginResult{RecoveryCodes: codes, OperationID: id})
}
