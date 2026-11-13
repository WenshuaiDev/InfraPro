-- +goose Up
CREATE TABLE analysis_configs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid,
 scope_key text GENERATED ALWAYS AS (COALESCE(project_id::text,'tenant')) STORED,
 code text NOT NULL, code_normalized text GENERATED ALWAYS AS (lower(code)) STORED,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200), description text NOT NULL DEFAULT '' CHECK(length(description)<=4000),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')), revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 draft_revision bigint NOT NULL DEFAULT 1 CHECK(draft_revision>0), draft jsonb NOT NULL CHECK(jsonb_typeof(draft)='object' AND octet_length(draft::text)<=1048576),
 latest_version bigint NOT NULL DEFAULT 0 CHECK(latest_version>=0),
 created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,scope_key,code_normalized),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id)
);
CREATE INDEX analysis_configs_page ON analysis_configs(tenant_id,scope_key,status,updated_at DESC,id);
-- +goose StatementBegin
CREATE FUNCTION analysis_config_identity_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.id IS DISTINCT FROM OLD.id OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id OR NEW.project_id IS DISTINCT FROM OLD.project_id OR NEW.code IS DISTINCT FROM OLD.code OR NEW.created_by IS DISTINCT FROM OLD.created_by OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
  RAISE EXCEPTION 'analysis configuration identity and workspace are immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER analysis_config_identity_guard BEFORE UPDATE ON analysis_configs FOR EACH ROW EXECUTE FUNCTION analysis_config_identity_guard();
CREATE TABLE analysis_config_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, config_id uuid NOT NULL, version_no bigint NOT NULL CHECK(version_no>0),
 name text NOT NULL, body jsonb NOT NULL CHECK(jsonb_typeof(body)='object' AND octet_length(body::text)<=1048576),
 compiled_plan jsonb NOT NULL CHECK(jsonb_typeof(compiled_plan)='object'),
 config_hash text NOT NULL CHECK(config_hash ~ '^[0-9a-f]{64}$'), plan_hash text NOT NULL CHECK(plan_hash ~ '^[0-9a-f]{64}$'),
 engine_version text NOT NULL, compiler_version text NOT NULL,
 base_version_id uuid, reset_baselines boolean NOT NULL DEFAULT false,
 created_by uuid NOT NULL REFERENCES accounts(id), reason text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,config_id,version_no), UNIQUE(tenant_id,config_id,id),
 FOREIGN KEY(tenant_id,config_id) REFERENCES analysis_configs(tenant_id,id),
 FOREIGN KEY(tenant_id,config_id,base_version_id) REFERENCES analysis_config_versions(tenant_id,config_id,id)
);
CREATE TRIGGER analysis_version_immutable BEFORE UPDATE OR DELETE ON analysis_config_versions FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TABLE analysis_version_inputs (
 tenant_id uuid NOT NULL, version_id uuid NOT NULL, input_key text NOT NULL, channel_id uuid NOT NULL,
 PRIMARY KEY(tenant_id,version_id,input_key), FOREIGN KEY(tenant_id,version_id) REFERENCES analysis_config_versions(tenant_id,id),
 FOREIGN KEY(tenant_id,channel_id) REFERENCES engineering_channels(tenant_id,id)
);
CREATE INDEX analysis_version_input_channel ON analysis_version_inputs(tenant_id,channel_id,version_id);
CREATE TRIGGER analysis_version_input_immutable BEFORE UPDATE OR DELETE ON analysis_version_inputs FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TABLE analysis_version_references (
 tenant_id uuid NOT NULL, version_id uuid NOT NULL, engineering_version_id uuid NOT NULL REFERENCES engineering_versions(id),
 PRIMARY KEY(tenant_id,version_id,engineering_version_id), FOREIGN KEY(tenant_id,version_id) REFERENCES analysis_config_versions(tenant_id,id)
);
CREATE INDEX analysis_version_public_reference ON analysis_version_references(engineering_version_id);
CREATE TRIGGER analysis_version_reference_immutable BEFORE UPDATE OR DELETE ON analysis_version_references FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();

-- Shared fixed-baseline references have a real FK to either immutable kind.
-- Deferred validation prevents orphan keys; the permanent subtype records
-- and this registry cannot be rewritten or deleted.
CREATE TABLE derived_calculation_config_keys (
 tenant_id uuid NOT NULL REFERENCES tenants(id), id uuid NOT NULL, kind text NOT NULL CHECK(kind IN ('channel','analysis')),
 PRIMARY KEY(tenant_id,id)
);
INSERT INTO derived_calculation_config_keys SELECT tenant_id,id,'channel' FROM derived_channel_configs;
-- +goose StatementBegin
CREATE FUNCTION derived_register_calculation_config() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO derived_calculation_config_keys(tenant_id,id,kind) VALUES(NEW.tenant_id,NEW.id,CASE WHEN TG_TABLE_NAME='derived_channel_configs' THEN 'channel' ELSE 'analysis' END);
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER derived_register_channel_config AFTER INSERT ON derived_channel_configs FOR EACH ROW EXECUTE FUNCTION derived_register_calculation_config();
CREATE TRIGGER derived_register_analysis_config AFTER INSERT ON analysis_config_versions FOR EACH ROW EXECUTE FUNCTION derived_register_calculation_config();
-- +goose StatementBegin
CREATE FUNCTION derived_calculation_config_key_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.kind='channel' AND NOT EXISTS(SELECT 1 FROM derived_channel_configs WHERE tenant_id=NEW.tenant_id AND id=NEW.id)) OR (NEW.kind='analysis' AND NOT EXISTS(SELECT 1 FROM analysis_config_versions WHERE tenant_id=NEW.tenant_id AND id=NEW.id)) THEN
  RAISE EXCEPTION 'calculation configuration key requires immutable subtype' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER derived_calculation_config_key_guard AFTER INSERT ON derived_calculation_config_keys DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION derived_calculation_config_key_guard();
CREATE TRIGGER derived_calculation_config_key_immutable BEFORE UPDATE OR DELETE ON derived_calculation_config_keys FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
ALTER TABLE derived_fixed_baselines DROP CONSTRAINT derived_fixed_baselines_tenant_id_config_id_fkey;
ALTER TABLE derived_fixed_baselines ADD CONSTRAINT derived_fixed_baselines_config_key FOREIGN KEY(tenant_id,config_id) REFERENCES derived_calculation_config_keys(tenant_id,id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'analysis configuration evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
