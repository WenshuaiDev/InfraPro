package identity

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AccountMFA struct {
	State         string     `json:"state"`
	FactorVersion string     `json:"factor_version"`
	EnabledAt     *time.Time `json:"enabled_at"`
}

func (s *Service) AccountMFA(ctx context.Context, p Principal, id string) (AccountMFA, error) {
	var v AccountMFA
	if !uuidPattern.MatchString(id) {
		return v, ErrNotFound
	}
	e := s.readAuthorized(ctx, p, access.OpAccountMFARead, func(tx pgx.Tx) error {
		f, e := loadFactor(ctx, tx, id)
		v = AccountMFA{f.State, strconv.FormatInt(f.Version, 10), f.EnabledAt}
		return e
	})
	return v, e
}
func (s *Service) ResetMFA(ctx context.Context, p Principal, id, expected, command string, m Meta) (MFAResult, error) {
	var out MFAResult
	if id == p.Account.ID {
		return out, problem("self_mfa_reset_forbidden", "请在账号安全页更换自己的验证器。", 403)
	}
	if !uuidPattern.MatchString(id) {
		return out, ErrNotFound
	}
	if e := reason(m); e != nil {
		return out, e
	}
	token, e := newToken()
	if e != nil {
		return out, e
	}
	e = s.repo.transact(ctx, func(tx pgx.Tx) error {
		if e := access.Require(ctx, tx, p.Account.ID, access.OpAccountMFAReset); e != nil {
			return e
		}
		policy, e := lockSecurityPolicy(ctx, tx, false)
		if e != nil {
			return e
		}
		accounts := []string{p.Account.ID, id}
		sort.Strings(accounts)
		for _, account := range accounts {
			if _, _, e = lockAuthAccount(ctx, tx, account); e != nil {
				return e
			}
		}
		if e = lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, true); e != nil {
			return e
		}
		receipt, e := mfaCommand(ctx, tx, p, "mfa_reset", command, map[string]string{"account_id": id, "expected_factor_version": expected, "reason": m.Reason})
		if e != nil {
			return e
		}
		if receipt.Replayed {
			out.MFAStatus, e = statusTx(ctx, tx, id, policy)
			out.OperationID = receipt.OperationID
			out.SecretUnavailable = true
			return e
		}
		out, e = s.resetMFATx(ctx, tx, id, expected, token, p.Account.ID, "account", policy, m)
		if e != nil {
			return e
		}
		out.OperationID = receipt.OperationID
		return finishMFACommand(ctx, tx, receipt, out, id)
	})
	return out, e
}
func (s *Service) resetMFATx(ctx context.Context, tx pgx.Tx, id, expected, token, actor, actorType string, policy SecurityPolicy, m Meta) (MFAResult, error) {
	var out MFAResult
	f, e := loadFactor(ctx, tx, id)
	if e != nil {
		return out, e
	}
	if f.State != "enabled" && f.State != "reset_required" {
		return out, ErrMFAState
	}
	if expected != "" && expected != strconv.FormatInt(f.Version, 10) {
		return out, ErrVersion
	}
	f.Version++
	if _, e = tx.Exec(ctx, `UPDATE account_mfa SET state='reset_required',factor_version=$2,secret_ciphertext=NULL,secret_nonce=NULL,key_id=NULL,last_used_step=NULL,enabled_at=NULL,updated_at=now() WHERE account_id=$1`, id, f.Version); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `UPDATE account_mfa_recovery_codes SET revoked_at=COALESCE(revoked_at,now()) WHERE account_id=$1`, id); e != nil {
		return out, e
	}
	var version int64
	if e = tx.QueryRow(ctx, `UPDATE accounts SET auth_version=auth_version+1,updated_at=now() WHERE id=$1 RETURNING auth_version`, id).Scan(&version); e != nil {
		return out, e
	}
	if e = revokeSessions(ctx, tx, id, ""); e != nil {
		return out, e
	}
	expiry := s.now().Add(30 * time.Minute)
	grant := uuid.NewString()
	var issuer *string
	if actor != "" {
		issuer = &actor
	}
	if _, e = tx.Exec(ctx, `INSERT INTO account_auth_challenges(id,account_id,token_hash,purpose,expires_at,account_auth_version,factor_version,policy_revision,issuer_account_id) VALUES($1,$2,$3,'admin_reset_grant',$4,$5,$6,$7,$8)`, grant, id, grantDigest(token), expiry, version, f.Version, policy.Revision, issuer); e != nil {
		return out, e
	}
	_, e = ar.AppendLegacyTx(ctx, tx, ar.LegacyInput{Event: "mfa_reset", ActorID: actor, ActorType: actorType, AccountID: id, ObjectType: "account_mfa", ObjectID: id, Outcome: "success", Reason: m.Reason, IP: m.IP, UserAgent: m.UserAgent, RequestID: m.RequestID, Entry: "platform", After: map[string]any{"state": "reset_required", "factor_version": f.Version, "expires_at": expiry, "recovery_grant_id": grant}})
	if e != nil {
		return out, e
	}
	if e = access.NotificationReceiptTx(ctx, tx, "identity", "security.changed", "mfa_reset", actor, ne.Scope{Kind: "account"}, ne.Target{Kind: "security"}, ne.Personal(id)); e != nil {
		return out, e
	}
	out.MFAStatus, e = statusTx(ctx, tx, id, policy)
	out.RecoveryURL = strings.TrimRight(s.cfg.PublicConsoleURL, "/") + "/mfa-recovery#token=" + token
	out.ExpiresAt = &expiry
	return out, e
}
func (s *Service) MaintenanceResetMFA(ctx context.Context, username, operator string, m Meta) (MFAResult, error) {
	var out MFAResult
	if strings.TrimSpace(operator) == "" || len([]rune(operator)) > 100 {
		return out, FieldError("operator", "请填写 1–100 字符的维护操作员说明。")
	}
	if e := reason(m); e != nil {
		return out, e
	}
	token, e := newToken()
	if e != nil {
		return out, e
	}
	m.UserAgent = "infrapro maintenance: " + operator
	m.RequestID = uuid.NewString()
	e = s.repo.transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		policy, e := lockSecurityPolicy(ctx, tx, false)
		if e != nil {
			return e
		}
		var id string
		e = tx.QueryRow(ctx, `SELECT id::text FROM accounts WHERE username_normalized=$1`, strings.ToLower(username)).Scan(&id)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if _, _, e = lockAuthAccount(ctx, tx, id); e != nil {
			return e
		}
		out, e = s.resetMFATx(ctx, tx, id, "", token, "", "maintenance", policy, m)
		out.OperationID = m.RequestID
		return e
	})
	return out, e
}
func (s *Service) ClaimMFARecovery(ctx context.Context, token, id, grantToken string, m Meta) (AuthDelivery, error) {
	var out AuthDelivery
	if !uuidPattern.MatchString(id) {
		return out, ErrChallenge
	}
	if e := s.Rate(ctx, "mfa-recovery", m.IP, ""); e != nil {
		return out, e
	}
	var outcome error
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		c, a, f, policy, e := s.loginChallengeTx(ctx, tx, token, id)
		if e != nil {
			return e
		}
		if c.Purpose != "login_recover" || f.State != "reset_required" {
			return ErrChallenge
		}
		if e = s.mfaAccountRate(ctx, a.ID); e != nil {
			return e
		}
		if outcome, e = mfaCooldown(ctx, tx, a.ID); e != nil || outcome != nil {
			return e
		}
		var grant string
		var expiry time.Time
		e = tx.QueryRow(ctx, `UPDATE account_auth_challenges SET state='completed',completed_at=now() WHERE token_hash=$1 AND account_id=$2 AND purpose='admin_reset_grant' AND state='pending' AND expires_at>$3 AND account_auth_version=$4 AND factor_version=$5 RETURNING id::text,expires_at`, grantDigest(grantToken), a.ID, s.now(), c.AuthVersion, f.Version).Scan(&grant, &expiry)
		if errors.Is(e, pgx.ErrNoRows) || !validToken(grantToken) {
			outcome = ErrMFACode
			return mfaFailure(ctx, tx, c, m)
		}
		if e != nil {
			return e
		}
		c.Purpose = "login_enroll"
		c.ExpiresAt = c.PasswordAt.Add(10 * time.Minute)
		if expiry.Before(c.ExpiresAt) {
			c.ExpiresAt = expiry
		}
		if _, e = tx.Exec(ctx, `UPDATE account_auth_challenges SET purpose='login_enroll',recovery_grant_id=$2,expires_at=$3 WHERE id=$1`, c.ID, grant, c.ExpiresAt); e != nil {
			return e
		}
		out.Result.LoginChallenge = challengeView(c, a, policy)
		out.ChallengeToken = token
		out.ExpiresAt = c.ExpiresAt
		return nil
	})
	if e != nil {
		return AuthDelivery{}, e
	}
	return out, outcome
}
