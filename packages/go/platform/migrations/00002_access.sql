-- +goose Up
CREATE TABLE permission_catalog (
 code text PRIMARY KEY, name text NOT NULL, group_name text NOT NULL, scope text NOT NULL,
 description text NOT NULL, risk text NOT NULL, lifecycle text NOT NULL CHECK (lifecycle IN ('active','retired')),
 catalog_version text NOT NULL
);
CREATE TABLE roles (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 50),
 name_normalized text NOT NULL, description text NOT NULL DEFAULT '' CHECK (char_length(description)<=500),
 builtin_key text UNIQUE, status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz
);
CREATE UNIQUE INDEX roles_name ON roles(name_normalized) WHERE deleted_at IS NULL;
CREATE TABLE role_permissions (
 role_id uuid NOT NULL REFERENCES roles(id), permission_code text NOT NULL REFERENCES permission_catalog(code),
 PRIMARY KEY(role_id,permission_code)
);
CREATE INDEX role_permissions_code ON role_permissions(permission_code,role_id);
CREATE TABLE account_roles (
 account_id uuid NOT NULL REFERENCES accounts(id), role_id uuid NOT NULL REFERENCES roles(id),
 assigned_at timestamptz NOT NULL DEFAULT now(), source text NOT NULL, actor_id uuid REFERENCES accounts(id),
 PRIMARY KEY(account_id,role_id)
);
CREATE INDEX account_roles_role ON account_roles(role_id,account_id);
CREATE TABLE authorization_state (
 id integer PRIMARY KEY CHECK(id=1), revision bigint NOT NULL DEFAULT 0,
 catalog_checksum text NOT NULL DEFAULT '', synced_at timestamptz
);
INSERT INTO authorization_state(id) VALUES(1);
ALTER TABLE accounts ADD COLUMN management_version bigint NOT NULL DEFAULT 1;
ALTER TABLE account_security_events ADD COLUMN actor_type text NOT NULL DEFAULT 'account';
ALTER TABLE account_security_events ADD COLUMN object_type text NOT NULL DEFAULT 'account';
ALTER TABLE account_security_events ADD COLUMN details jsonb NOT NULL DEFAULT '{}';
CREATE INDEX account_security_events_target ON account_security_events(object_type,object_id,id DESC);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Access history is irreversible; restore a verified backup instead'; END $$;
-- +goose StatementEnd
