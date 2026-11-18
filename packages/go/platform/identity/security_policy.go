package identity

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

func (s *Service) Policy(ctx context.Context, p Principal) (SecurityPolicy, error) {
	var v SecurityPolicy
	e := s.readAuthorized(ctx, p, access.OpSecurityPolicyRead, func(tx pgx.Tx) error {
		var e error
		v, e = lockSecurityPolicy(ctx, tx, false)
		if e != nil {
			return e
		}
		return policyUpdater(ctx, tx, &v)
	})
	return v, e
}
func policyUpdater(ctx context.Context, tx pgx.Tx, policy *SecurityPolicy) error {
	if policy.UpdatedBy == "" {
		policy.UpdatedByLabel = "系统维护"
		return nil
	}
	return tx.QueryRow(ctx, `SELECT CASE WHEN display_name='' THEN username ELSE display_name||' · '||username END FROM accounts WHERE id=$1`, policy.UpdatedBy).Scan(&policy.UpdatedByLabel)
}
func (s *Service) PolicyImpact(ctx context.Context, p Principal) (MFAImpact, error) {
	var v MFAImpact
	e := s.readAuthorized(ctx, p, access.OpSecurityPolicyImpact, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE f.state='enabled'),count(*) FILTER(WHERE f.state='disabled'),count(*) FILTER(WHERE f.state='reset_required'),(SELECT count(*) FROM account_sessions WHERE revoked_at IS NULL AND expires_at>now() AND authentication_method='password'),clock_timestamp() FROM accounts a JOIN account_mfa f ON f.account_id=a.id WHERE a.status='active'`).Scan(&v.Active, &v.Enabled, &v.Unenrolled, &v.ResetRequired, &v.PasswordSessions, &v.ComputedAt)
	})
	return v, e
}
func mfaCommand(ctx context.Context, tx pgx.Tx, p Principal, action, id string, payload any) (ar.Receipt, error) {
	return ar.BeginCommandTx(ctx, tx, ar.Command{Module: "identity", ActorID: p.Account.ID, Scope: ar.Scope{Kind: "platform"}, Action: action, ID: id, Payload: payload})
}
func finishMFACommand(ctx context.Context, tx pgx.Tx, r ar.Receipt, v MFAResult, account string) error {
	return ar.CompleteCommandTx(ctx, tx, r, map[string]any{"account_id": account, "state": v.State, "version": v.FactorVersion, "generation": v.RecoveryGeneration}, "ok", account)
}
func (s *Service) UpdatePolicy(ctx context.Context, p Principal, required bool, expected, command string, m Meta) (PolicyResult, error) {
	var out PolicyResult
	if e := reason(m); e != nil {
		return out, e
	}
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		if e := access.Require(ctx, tx, p.Account.ID, access.OpSecurityPolicyUpdate); e != nil {
			return e
		}
		policy, e := lockSecurityPolicy(ctx, tx, true)
		if e != nil {
			return e
		}
		if e = lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, true); e != nil {
			return e
		}
		if e = policyUpdater(ctx, tx, &policy); e != nil {
			return e
		}
		receipt, e := mfaCommand(ctx, tx, p, "mfa_policy_changed", command, map[string]any{"require_mfa": required, "expected_version": expected, "reason": m.Reason})
		if e != nil {
			return e
		}
		out = PolicyResult{Policy: policy, SessionAction: "keep", OperationID: receipt.OperationID}
		if receipt.Replayed {
			var safe struct {
				Changed bool `json:"changed"`
			}
			if e = json.Unmarshal(receipt.Response, &safe); e != nil {
				return e
			}
			out.Changed = safe.Changed
			return nil
		}
		if policy.Revision != expected {
			return ErrVersion
		}
		out.Changed = required != policy.RequireMFA
		if out.Changed {
			before := policy
			e = tx.QueryRow(ctx, `UPDATE platform_security_policy SET require_mfa=$1,revision=revision+1,enforcement_generation=enforcement_generation+CASE WHEN $1 THEN 1 ELSE 0 END,updated_by=$2,updated_at=now() WHERE id=1 RETURNING require_mfa,revision::text,enforcement_generation,updated_by::text,updated_at`, required, p.Account.ID).Scan(&policy.RequireMFA, &policy.Revision, &policy.Generation, &policy.UpdatedBy, &policy.UpdatedAt)
			if e != nil {
				return e
			}
			_, e = ar.AppendLegacyTx(ctx, tx, ar.LegacyInput{Event: "mfa_policy_changed", ActorID: p.Account.ID, ActorType: "account", ObjectType: "platform_security_policy", ObjectID: "1", Links: []ar.Link{{Kind: "business", Module: "identity", TargetKind: "platform_security_policy", TargetID: "1", Relation: "subject", Scope: ar.Scope{Kind: "platform"}}}, Outcome: "success", Reason: m.Reason, IP: m.IP, UserAgent: m.UserAgent, RequestID: m.RequestID, Entry: "platform", Before: map[string]any{"require_mfa": before.RequireMFA, "revision": before.Revision, "enforcement_generation": before.Generation}, After: map[string]any{"require_mfa": required, "revision": policy.Revision, "enforcement_generation": policy.Generation}})
			if e != nil {
				return e
			}
			if e = policyUpdater(ctx, tx, &policy); e != nil {
				return e
			}
		}
		out.Policy = policy
		if required && p.Session.AuthenticationMethod == "password" {
			out.SessionAction = "sign_in_again"
		}
		return ar.CompleteCommandTx(ctx, tx, receipt, map[string]any{"revision": policy.Revision, "changed": out.Changed}, "ok", "1")
	})
	return out, e
}
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, p Principal, expected, generation, command string, m Meta) (MFAResult, error) {
	var out MFAResult
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		if e := lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, true); e != nil {
			return e
		}
		policy, e := lockSecurityPolicy(ctx, tx, false)
		if e != nil {
			return e
		}
		receipt, e := mfaCommand(ctx, tx, p, "mfa_recovery_codes_regenerated", command, map[string]string{"expected_factor_version": expected, "expected_recovery_generation": generation})
		if e != nil {
			return e
		}
		out.OperationID = receipt.OperationID
		if receipt.Replayed {
			out.SecretUnavailable = true
			out.MFAStatus, e = statusTx(ctx, tx, p.Account.ID, policy)
			return e
		}
		f, e := loadFactor(ctx, tx, p.Account.ID)
		if e != nil {
			return e
		}
		if f.State != "enabled" {
			return ErrMFAState
		}
		if strconv.FormatInt(f.Version, 10) != expected || strconv.FormatInt(f.Generation, 10) != generation {
			return ErrVersion
		}
		out.RecoveryCodes, e = s.replaceCodesTx(ctx, tx, p.Account.ID, &f)
		if e != nil {
			return e
		}
		out.MFAStatus, e = statusTx(ctx, tx, p.Account.ID, policy)
		if e != nil {
			return e
		}
		if e = securityReceipt(ctx, tx, "mfa_recovery_codes_regenerated", p.Account.ID, p.Account.ID, p.Account.ID, m); e != nil {
			return e
		}
		return finishMFACommand(ctx, tx, receipt, out, p.Account.ID)
	})
	return out, e
}
func (s *Service) DisableMFA(ctx context.Context, p Principal, expected, command string, m Meta) (MFAResult, AuthDelivery, error) {
	var out MFAResult
	var session Principal
	var delivery AuthDelivery
	token, e := newToken()
	if e != nil {
		return out, delivery, e
	}
	e = s.repo.transact(ctx, func(tx pgx.Tx) error {
		if e := lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, true); e != nil {
			return e
		}
		policy, e := lockSecurityPolicy(ctx, tx, false)
		if e != nil {
			return e
		}
		receipt, e := mfaCommand(ctx, tx, p, "mfa_disabled", command, map[string]string{"expected_factor_version": expected})
		if e != nil {
			return e
		}
		out.OperationID = receipt.OperationID
		if receipt.Replayed {
			out.SecretUnavailable = true
			out.MFAStatus, e = statusTx(ctx, tx, p.Account.ID, policy)
			return e
		}
		if policy.RequireMFA {
			return ErrMFAPolicy
		}
		f, e := loadFactor(ctx, tx, p.Account.ID)
		if e != nil {
			return e
		}
		if f.State != "enabled" {
			return ErrMFAState
		}
		if expected != strconv.FormatInt(f.Version, 10) {
			return ErrVersion
		}
		f.Version++
		f.State = "disabled"
		if _, e = tx.Exec(ctx, `UPDATE account_mfa SET state='disabled',factor_version=$2,secret_ciphertext=NULL,secret_nonce=NULL,key_id=NULL,last_used_step=NULL,enabled_at=NULL,updated_at=now() WHERE account_id=$1`, p.Account.ID, f.Version); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE account_mfa_recovery_codes SET revoked_at=COALESCE(revoked_at,now()) WHERE account_id=$1`, p.Account.ID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE accounts SET auth_version=auth_version+1,updated_at=now() WHERE id=$1`, p.Account.ID); e != nil {
			return e
		}
		if e = revokeSessions(ctx, tx, p.Account.ID, ""); e != nil {
			return e
		}
		var passwordAt time.Time
		if e = tx.QueryRow(ctx, `SELECT reauthenticated_at FROM account_sessions WHERE id=$1`, p.Session.ID).Scan(&passwordAt); e != nil {
			return e
		}
		session, e = createSession(ctx, tx, p.Account, p.Version+1, digest(token), "password", f, policy, passwordAt, s.now(), p.Session.ExpiresAt, m)
		if e != nil {
			return e
		}
		out.MFAStatus, e = statusTx(ctx, tx, p.Account.ID, policy)
		if e != nil {
			return e
		}
		if e = securityReceipt(ctx, tx, "mfa_disabled", p.Account.ID, p.Account.ID, p.Account.ID, m); e != nil {
			return e
		}
		return finishMFACommand(ctx, tx, receipt, out, p.Account.ID)
	})
	if e == nil && session.Session.ID != "" {
		delivery, e = s.deliver(ctx, session, token, LoginResult{})
	}
	return out, delivery, e
}
