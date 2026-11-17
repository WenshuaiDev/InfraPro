package identity

import (
	"encoding/json"
	"time"
)

var (
	ErrMFALogin           = problem("mfa_login_required", "安全策略已变化，请重新登录完成多因素认证。", 401)
	ErrChallenge          = problem("mfa_challenge_invalid", "认证步骤已失效，请重新验证密码。", 401)
	ErrChallengeCompleted = problem("mfa_challenge_completed", "认证步骤已完成，请核对登录状态。", 409)
	ErrMFACode            = problem("mfa_code_invalid", "验证码无效，请使用尚未使用的当前验证码或恢复码。", 400)
	ErrMFAState           = problem("mfa_state_conflict", "安全配置已变化，请刷新后重新操作。", 409)
	ErrMFAPolicy          = problem("mfa_required_by_policy", "平台要求使用多因素认证，当前不能关闭。", 409)
	ErrVersion            = problem("version_conflict", "配置版本已变化，请刷新并核对后重试。", 409)
)

type LoginChallenge struct {
	Status         string    `json:"status"`
	ID             string    `json:"challenge_id,omitempty"`
	AccountLabel   string    `json:"account_label,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitzero"`
	Methods        []string  `json:"methods,omitempty"`
	PolicyRequired bool      `json:"policy_required"`
}
type LoginResult struct {
	LoginChallenge
	Account       *Account `json:"account,omitempty"`
	RecoveryCodes []string `json:"recovery_codes,omitempty"`
	OperationID   string   `json:"operation_id,omitempty"`
}

// The authenticated arm deliberately has no challenge metadata. Keeping the
// wire union exact prevents old callers from treating a pending login as an account.
func (v LoginResult) MarshalJSON() ([]byte, error) {
	if v.Status == "authenticated" {
		return json.Marshal(struct {
			Status        string   `json:"status"`
			Account       *Account `json:"account"`
			RecoveryCodes []string `json:"recovery_codes,omitempty"`
			OperationID   string   `json:"operation_id,omitempty"`
		}{v.Status, v.Account, v.RecoveryCodes, v.OperationID})
	}
	type wire LoginResult
	return json.Marshal(wire(v))
}

// Tokens are transport-only and never serialized into response bodies or receipts.
type AuthDelivery struct {
	Result                       LoginResult
	SessionToken, ChallengeToken string
	ExpiresAt                    time.Time
}
type EnrollmentSetup struct {
	ID           string    `json:"enrollment_id"`
	Issuer       string    `json:"issuer"`
	AccountLabel string    `json:"account_label"`
	Secret       string    `json:"secret"`
	QR           string    `json:"qr_png_base64"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type MFAStatus struct {
	State                  string     `json:"state"`
	FactorVersion          string     `json:"factor_version"`
	RecoveryGeneration     string     `json:"recovery_generation"`
	RecoveryCodesRemaining int        `json:"recovery_codes_remaining"`
	EnabledAt              *time.Time `json:"enabled_at"`
	PolicyRequired         bool       `json:"policy_required"`
	CanDisable             bool       `json:"can_disable"`
}
type MFAResult struct {
	MFAStatus
	RecoveryCodes     []string   `json:"recovery_codes,omitempty"`
	OperationID       string     `json:"operation_id,omitempty"`
	SecretUnavailable bool       `json:"secret_unavailable,omitempty"`
	RecoveryURL       string     `json:"recovery_url,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}
type SecurityPolicy struct {
	RequireMFA     bool      `json:"require_mfa"`
	Revision       string    `json:"revision"`
	Generation     int64     `json:"-"`
	UpdatedBy      string    `json:"updated_by"`
	UpdatedByLabel string    `json:"updated_by_label"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type MFAImpact struct {
	Active           int       `json:"active_accounts"`
	Enabled          int       `json:"enabled_accounts"`
	Unenrolled       int       `json:"unenrolled_accounts"`
	ResetRequired    int       `json:"reset_required_accounts"`
	PasswordSessions int       `json:"potential_password_sessions"`
	ComputedAt       time.Time `json:"computed_at"`
}
type PolicyResult struct {
	Policy        SecurityPolicy `json:"policy"`
	Changed       bool           `json:"changed"`
	SessionAction string         `json:"session_action"`
	OperationID   string         `json:"operation_id"`
}
type mfaFactor struct {
	State               string
	Version, Generation int64
	Secret              sealedSecret
	LastStep            *int64
	EnabledAt           *time.Time
}
type authChallenge struct {
	ID, AccountID, Purpose, State, Origin, Grant string
	AuthVersion, FactorVersion                   int64
	ExpiresAt, PasswordAt                        time.Time
	Secret                                       sealedSecret
	Failures                                     int
}
