-- +goose Up
CREATE TABLE derived_backfill_previews (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),tenant_id uuid NOT NULL,project_id uuid,actor_id uuid NOT NULL REFERENCES accounts(id),channel_id uuid NOT NULL,
 proposal jsonb NOT NULL CHECK(jsonb_typeof(proposal)='object'),content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '10 minutes',
 FOREIGN KEY(tenant_id,channel_id) REFERENCES derived_channels(tenant_id,channel_id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id)
);
CREATE INDEX derived_backfill_preview_expiry ON derived_backfill_previews(expires_at);
-- +goose Down
DROP TABLE derived_backfill_previews;
