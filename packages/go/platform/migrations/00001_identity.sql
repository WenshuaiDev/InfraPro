-- +goose Up
CREATE TABLE accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL,
    username_normalized text NOT NULL UNIQUE,
    display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 50),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    auth_version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (username ~ '^[A-Za-z0-9_-]{3,32}$'),
    CHECK (username_normalized = lower(username))
);
CREATE TABLE account_passwords (
    account_id uuid PRIMARY KEY REFERENCES accounts(id),
    password_hash text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE system_initialization (
    id integer PRIMARY KEY CHECK (id = 1),
    completed_at timestamptz,
    first_account_id uuid REFERENCES accounts(id),
    CHECK ((completed_at IS NULL) = (first_account_id IS NULL))
);
INSERT INTO system_initialization(id) VALUES (1);
CREATE TABLE account_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash text NOT NULL UNIQUE,
    account_id uuid NOT NULL REFERENCES accounts(id),
    auth_version bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_activity_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    reauthenticated_at timestamptz NOT NULL DEFAULT now(),
    user_agent text NOT NULL,
    ip text NOT NULL
);
CREATE INDEX account_sessions_owner ON account_sessions(account_id);
CREATE INDEX account_sessions_expiry ON account_sessions(expires_at);
CREATE TABLE account_invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    inviter_id uuid NOT NULL REFERENCES accounts(id),
    token_hash text NOT NULL UNIQUE,
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    revoked_at timestamptz,
    invitee_id uuid REFERENCES accounts(id),
    CHECK ((used_at IS NULL) = (invitee_id IS NULL))
);
CREATE INDEX account_invitations_owner ON account_invitations(inviter_id, created_at DESC, id DESC);
CREATE TABLE account_password_resets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES accounts(id),
    token_hash text NOT NULL UNIQUE,
    reason text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    revoked_at timestamptz
);
CREATE UNIQUE INDEX account_password_resets_pending ON account_password_resets(account_id) WHERE used_at IS NULL AND revoked_at IS NULL;
CREATE TABLE account_security_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_type text NOT NULL,
    actor_id uuid REFERENCES accounts(id),
    account_id uuid REFERENCES accounts(id),
    object_id text NOT NULL DEFAULT '',
    outcome text NOT NULL,
    reason text NOT NULL DEFAULT '',
    source_ip text NOT NULL DEFAULT '',
    user_agent text NOT NULL DEFAULT '',
    request_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX account_security_events_owner ON account_security_events(account_id, id DESC);
CREATE TABLE account_rate_limits (
    key_hash text PRIMARY KEY,
    failures integer NOT NULL DEFAULT 0,
    cooldown_until timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- A durable invalidation outbox also works when the maintenance process has no Redis.
CREATE TABLE account_cache_invalidations (
    session_id uuid PRIMARY KEY,
    token_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- Deliberately irreversible: account history and initialization must not be reset by rollback.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Identity migration is irreversible; restore a verified backup instead'; END $$;
-- +goose StatementEnd
