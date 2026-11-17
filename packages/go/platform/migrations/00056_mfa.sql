-- +goose Up
CREATE TABLE platform_security_policy (
    id smallint PRIMARY KEY CHECK (id=1),
    require_mfa boolean NOT NULL DEFAULT false,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
    enforcement_generation bigint NOT NULL DEFAULT 0 CHECK (enforcement_generation>=0),
    updated_by uuid REFERENCES accounts(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO platform_security_policy(id) VALUES(1);
CREATE TABLE account_mfa (
    account_id uuid PRIMARY KEY REFERENCES accounts(id),
    state text NOT NULL DEFAULT 'disabled' CHECK (state IN ('disabled','enabled','reset_required')),
    factor_version bigint NOT NULL DEFAULT 1 CHECK (factor_version>0),
    secret_ciphertext bytea, secret_nonce bytea, key_id text,
    last_used_step bigint,
    recovery_generation bigint NOT NULL DEFAULT 0 CHECK (recovery_generation>=0),
    enabled_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((state='enabled' AND secret_ciphertext IS NOT NULL AND octet_length(secret_ciphertext)>16 AND secret_nonce IS NOT NULL AND octet_length(secret_nonce)=12 AND key_id IS NOT NULL AND enabled_at IS NOT NULL)
        OR (state<>'enabled' AND secret_ciphertext IS NULL AND secret_nonce IS NULL AND key_id IS NULL AND enabled_at IS NULL AND last_used_step IS NULL))
);
INSERT INTO account_mfa(account_id) SELECT id FROM accounts;
CREATE INDEX account_mfa_key ON account_mfa(key_id) WHERE key_id IS NOT NULL;
CREATE TABLE account_mfa_recovery_codes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL REFERENCES accounts(id),
    factor_version bigint NOT NULL CHECK (factor_version>0), generation bigint NOT NULL CHECK (generation>0),
    code_hash text NOT NULL UNIQUE, created_at timestamptz NOT NULL DEFAULT now(), used_at timestamptz, revoked_at timestamptz
);
CREATE INDEX account_mfa_recovery_owner ON account_mfa_recovery_codes(account_id,generation);
CREATE TABLE account_auth_challenges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL REFERENCES accounts(id),
    token_hash text UNIQUE,
    purpose text NOT NULL CHECK (purpose IN ('login_verify','login_enroll','login_recover','self_enroll','self_replace','admin_reset_grant')),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','completed','revoked')),
    created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL,
    completed_at timestamptz, revoked_at timestamptz, failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts>=0),
    account_auth_version bigint NOT NULL, factor_version bigint NOT NULL, policy_revision bigint NOT NULL,
    origin_session_id uuid, recovery_grant_id uuid, issuer_account_id uuid REFERENCES accounts(id),
    password_verified_at timestamptz,
    secret_ciphertext bytea, secret_nonce bytea, key_id text,
    CHECK ((secret_ciphertext IS NULL AND secret_nonce IS NULL AND key_id IS NULL) OR
      (state='pending' AND secret_ciphertext IS NOT NULL AND octet_length(secret_ciphertext)>16 AND secret_nonce IS NOT NULL AND octet_length(secret_nonce)=12 AND key_id IS NOT NULL)),
    CHECK ((purpose IN ('self_enroll','self_replace') AND origin_session_id IS NOT NULL AND token_hash IS NULL) OR
      (purpose NOT IN ('self_enroll','self_replace') AND origin_session_id IS NULL AND token_hash IS NOT NULL)),
    CHECK (expires_at>created_at)
);
CREATE UNIQUE INDEX account_auth_self_pending ON account_auth_challenges(account_id) WHERE state='pending' AND purpose IN ('self_enroll','self_replace');
CREATE UNIQUE INDEX account_auth_grant_pending ON account_auth_challenges(account_id) WHERE state='pending' AND purpose='admin_reset_grant';
CREATE INDEX account_auth_expiry ON account_auth_challenges(expires_at);
CREATE INDEX account_auth_owner ON account_auth_challenges(account_id,state);
CREATE INDEX account_auth_key ON account_auth_challenges(key_id) WHERE key_id IS NOT NULL;
ALTER TABLE account_sessions
    ADD COLUMN authentication_method text NOT NULL DEFAULT 'password' CHECK (authentication_method IN ('password','password_totp','password_recovery_code')),
    ADD COLUMN mfa_verified_at timestamptz,
    ADD COLUMN mfa_factor_version bigint,
    ADD COLUMN policy_enforcement_generation bigint NOT NULL DEFAULT 0 CHECK (policy_enforcement_generation>=0),
    ADD CONSTRAINT session_mfa_proof CHECK (
      (authentication_method='password' AND mfa_verified_at IS NULL AND mfa_factor_version IS NULL) OR
      (authentication_method<>'password' AND mfa_verified_at IS NOT NULL AND mfa_factor_version IS NOT NULL AND mfa_factor_version>0));
REVOKE ALL ON platform_security_policy,account_mfa,account_mfa_recovery_codes,account_auth_challenges FROM PUBLIC;
-- Only the existing identity database owner/maintenance connection accesses these
-- tables. No worker/content role is granted access to authentication material.

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'MFA migration is irreversible; use a compatible repair release'; END $$;
-- +goose StatementEnd
