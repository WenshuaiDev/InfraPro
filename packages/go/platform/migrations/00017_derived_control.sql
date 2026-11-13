-- +goose Up
CREATE TABLE derived_execution_mandates (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, channel_id uuid NOT NULL, config_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision>0), output_ranges int8multirange NOT NULL CHECK(output_ranges<>'{}'::int8multirange),
 input_requirements jsonb NOT NULL CHECK(jsonb_typeof(input_requirements)='array'),
 content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
 created_by uuid NOT NULL REFERENCES accounts(id), project_id uuid, basis jsonb NOT NULL CHECK(jsonb_typeof(basis)='object'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), reason text NOT NULL,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,channel_id,revision), UNIQUE(tenant_id,channel_id,id),
 FOREIGN KEY(tenant_id,channel_id,config_id) REFERENCES derived_channel_configs(tenant_id,channel_id,id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id)
);
ALTER TABLE derived_channels ADD COLUMN active_mandate_id uuid, ADD COLUMN paused_at_ns bigint CHECK(paused_at_ns>=-2208988800000000000),
 ADD FOREIGN KEY(tenant_id,channel_id,active_mandate_id) REFERENCES derived_execution_mandates(tenant_id,channel_id,id);
CREATE TRIGGER derived_mandate_immutable BEFORE UPDATE OR DELETE ON derived_execution_mandates FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TABLE derived_control_previews (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid, actor_id uuid NOT NULL REFERENCES accounts(id), channel_id uuid NOT NULL,
 proposal jsonb NOT NULL CHECK(jsonb_typeof(proposal)='object'), content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '10 minutes',
 FOREIGN KEY(tenant_id,channel_id) REFERENCES derived_channels(tenant_id,channel_id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id)
);
CREATE INDEX derived_control_preview_expiry ON derived_control_previews(expires_at);
CREATE TABLE derived_dirty_ranges (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),tenant_id uuid NOT NULL,channel_id uuid NOT NULL,
 affected int8range NOT NULL CHECK(NOT isempty(affected) AND NOT lower_inf(affected)),
 required_position bigint NOT NULL CHECK(required_position>0),config_revision bigint NOT NULL,graph_revision bigint NOT NULL,
 reason text NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,channel_id) REFERENCES derived_channels(tenant_id,channel_id)
);
CREATE INDEX derived_dirty_channel ON derived_dirty_ranges(tenant_id,channel_id,required_position);
CREATE INDEX derived_dirty_overlap ON derived_dirty_ranges USING gist(tenant_id public.gist_uuid_ops,channel_id public.gist_uuid_ops,affected);
CREATE TABLE derived_schedules (
 tenant_id uuid NOT NULL,channel_id uuid NOT NULL,config_id uuid NOT NULL,clock_kind text NOT NULL,
 next_due_ns bigint NOT NULL,revision bigint NOT NULL CHECK(revision>0),PRIMARY KEY(tenant_id,channel_id,config_id),
 FOREIGN KEY(tenant_id,channel_id,config_id) REFERENCES derived_channel_configs(tenant_id,channel_id,id)
);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'execution mandate history cannot be downgraded'; END $$;
-- +goose StatementEnd
