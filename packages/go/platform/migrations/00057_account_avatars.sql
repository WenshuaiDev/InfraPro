-- +goose Up
CREATE TABLE account_avatar_objects (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES accounts(id),
    object_key text NOT NULL UNIQUE,
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    bytes integer NOT NULL CHECK (bytes BETWEEN 1 AND 524288),
    state text NOT NULL CHECK (state IN ('pending','ready','deleting','deleted')),
    cleanup_after timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, id)
);
ALTER TABLE accounts ADD COLUMN avatar_object_id uuid;
ALTER TABLE accounts ADD COLUMN avatar_revision bigint NOT NULL DEFAULT 0 CHECK (avatar_revision >= 0);
ALTER TABLE accounts ADD CONSTRAINT account_avatar_owner FOREIGN KEY (id, avatar_object_id) REFERENCES account_avatar_objects(account_id, id);
CREATE INDEX account_avatar_cleanup ON account_avatar_objects(cleanup_after) WHERE state <> 'deleted';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Account avatar migration is irreversible; restore a verified backup instead'; END $$;
-- +goose StatementEnd
