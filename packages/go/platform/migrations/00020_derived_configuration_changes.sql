-- +goose Up
ALTER TABLE derived_channel_configs
 ADD COLUMN base_config_id uuid,
 ADD COLUMN change_kind text NOT NULL DEFAULT 'initial' CHECK(change_kind IN ('initial','ordinary','correction')),
 ADD COLUMN semantic_intent_confirmed boolean NOT NULL DEFAULT false,
 ADD COLUMN reset_baselines boolean NOT NULL DEFAULT false,
 ADD FOREIGN KEY(tenant_id,channel_id,base_config_id) REFERENCES derived_channel_configs(tenant_id,channel_id,id),
 ADD CHECK(change_kind='correction' OR NOT semantic_intent_confirmed AND NOT reset_baselines);
CREATE TABLE derived_config_previews (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),tenant_id uuid NOT NULL,project_id uuid,actor_id uuid NOT NULL REFERENCES accounts(id),channel_id uuid NOT NULL,
 proposal jsonb NOT NULL CHECK(jsonb_typeof(proposal)='object'),content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '10 minutes',
 FOREIGN KEY(tenant_id,channel_id) REFERENCES derived_channels(tenant_id,channel_id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id)
);
CREATE INDEX derived_config_preview_expiry ON derived_config_previews(expires_at);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'configuration assignment history cannot be downgraded'; END $$;
-- +goose StatementEnd
