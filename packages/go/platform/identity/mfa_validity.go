package identity

import (
	"context"
	"errors"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type securityPolicyMemo struct{}

func lockSecurityPolicy(ctx context.Context, tx pgx.Tx, exclusive bool) (SecurityPolicy, error) {
	read := func() (SecurityPolicy, error) {
		var v SecurityPolicy
		lock := " FOR SHARE"
		if exclusive {
			lock = " FOR UPDATE"
		}
		e := tx.QueryRow(ctx, `SELECT require_mfa,revision::text,enforcement_generation,COALESCE(updated_by::text,''),updated_at FROM platform_security_policy WHERE id=1`+lock).Scan(&v.RequireMFA, &v.Revision, &v.Generation, &v.UpdatedBy, &v.UpdatedAt)
		if errors.Is(e, pgx.ErrNoRows) {
			e = ErrUnavailable
		}
		return v, e
	}
	if exclusive {
		return read()
	}
	return access.MemoTx(tx, securityPolicyMemo{}, read)
}
func loadFactor(ctx context.Context, tx pgx.Tx, account string) (mfaFactor, error) {
	var f mfaFactor
	e := tx.QueryRow(ctx, `SELECT state,factor_version,recovery_generation,secret_ciphertext,secret_nonce,COALESCE(key_id,''),last_used_step,enabled_at FROM account_mfa WHERE account_id=$1`, account).Scan(&f.State, &f.Version, &f.Generation, &f.Secret.Ciphertext, &f.Secret.Nonce, &f.Secret.KeyID, &f.LastStep, &f.EnabledAt)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrUnavailable
	}
	return f, e
}
func validMFAProof(f mfaFactor, policy SecurityPolicy, method string, verified *time.Time, version *int64, generation int64) error {
	proof := (method == "password_totp" || method == "password_recovery_code") && verified != nil && version != nil && *version == f.Version
	if f.State == "reset_required" || ((f.State == "enabled" || policy.RequireMFA) && !proof) || (!proof && generation < policy.Generation) {
		return ErrMFALogin
	}
	return nil
}

// Every personnel path calls this rule using current durable session fields,
// never trusting a serialized cache proof or caller-supplied authentication level.
func checkMFASession(ctx context.Context, tx pgx.Tx, account, session string, policy SecurityPolicy, p *Principal) error {
	f, e := loadFactor(ctx, tx, account)
	if e != nil {
		return e
	}
	var method string
	var verified *time.Time
	var version *int64
	var generation int64
	e = tx.QueryRow(ctx, `SELECT authentication_method,mfa_verified_at,mfa_factor_version,policy_enforcement_generation FROM account_sessions WHERE id=$1 AND account_id=$2`, session, account).Scan(&method, &verified, &version, &generation)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrUnauthorized
	}
	if e != nil {
		return e
	}
	if e = validMFAProof(f, policy, method, verified, version, generation); e != nil {
		return e
	}
	if p != nil {
		p.Session.AuthenticationMethod = method
		p.MFAVerifiedAt = verified
		p.MFAFactorVersion = version
		p.PolicyGeneration = generation
	}
	return nil
}
