-- +goose Up
-- This is immutable run identity/input evidence. Runtime state, cancellation,
-- lease, retry, progress and checkpoint truth remain in monitoring_jobs/steps.
CREATE TABLE derived_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid,
 scope_key text GENERATED ALWAYS AS (COALESCE(project_id::text,'tenant')) STORED,
 actor_id uuid REFERENCES accounts(id), channel_id uuid, config_id uuid,
 mode text NOT NULL CHECK(mode IN ('trial','automatic','backfill','correction','historical_replay','analysis','comparison')),
 job_id uuid NOT NULL, request_id uuid NOT NULL, request_hash text NOT NULL CHECK(request_hash ~ '^[0-9a-f]{64}$'),
 output_range int8range NOT NULL CHECK(NOT isempty(output_range) AND NOT lower_inf(output_range)), source_range int8range,
 config_revision bigint NOT NULL CHECK(config_revision>0), control_revision bigint NOT NULL CHECK(control_revision>0), mandate_revision bigint NOT NULL CHECK(mandate_revision>0),
 plan_hash text NOT NULL CHECK(plan_hash ~ '^[0-9a-f]{64}$'), input_requirements jsonb NOT NULL CHECK(jsonb_typeof(input_requirements)='array'),
 requested_at_ns bigint NOT NULL, reason text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,job_id), UNIQUE(tenant_id,scope_key,actor_id,request_id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id),
 FOREIGN KEY(tenant_id,channel_id,config_id) REFERENCES derived_channel_configs(tenant_id,channel_id,id),
 FOREIGN KEY(tenant_id,job_id) REFERENCES monitoring_jobs(tenant_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((channel_id IS NULL)=(config_id IS NULL)), CHECK(mode='automatic' OR actor_id IS NOT NULL)
);
CREATE INDEX derived_runs_page ON derived_runs(tenant_id,scope_key,created_at DESC,id);
CREATE INDEX derived_runs_channel ON derived_runs(tenant_id,channel_id,created_at DESC,id);
CREATE TABLE derived_run_raw_inputs (
 tenant_id uuid NOT NULL, run_id uuid NOT NULL, snapshot_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(tenant_id,run_id),
 FOREIGN KEY(tenant_id,run_id) REFERENCES derived_runs(tenant_id,id), FOREIGN KEY(tenant_id,snapshot_id) REFERENCES monitoring_raw_snapshots(tenant_id,id)
);
CREATE TABLE derived_trial_receipts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid, actor_id uuid NOT NULL REFERENCES accounts(id),
 run_id uuid NOT NULL, channel_id uuid NOT NULL, config_id uuid NOT NULL, config_hash text NOT NULL CHECK(config_hash ~ '^[0-9a-f]{64}$'),
 result jsonb NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=16777216),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '30 minutes',
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,run_id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id), FOREIGN KEY(tenant_id,run_id) REFERENCES derived_runs(tenant_id,id),
 FOREIGN KEY(tenant_id,channel_id,config_id) REFERENCES derived_channel_configs(tenant_id,channel_id,id)
);
CREATE TRIGGER derived_run_immutable BEFORE UPDATE OR DELETE ON derived_runs FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TRIGGER derived_run_raw_inputs_immutable BEFORE UPDATE OR DELETE ON derived_run_raw_inputs FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TRIGGER derived_trial_receipt_immutable BEFORE UPDATE OR DELETE ON derived_trial_receipts FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();

-- +goose StatementBegin
CREATE FUNCTION derived_run_job_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM monitoring_jobs j WHERE j.tenant_id=NEW.tenant_id AND j.id=NEW.job_id AND j.target_id=NEW.id AND j.project_id IS NOT DISTINCT FROM NEW.project_id AND j.actor_id IS NOT DISTINCT FROM NEW.actor_id AND j.kind IN ('derive','analysis','derived_replay','analysis_compare')) THEN
  RAISE EXCEPTION 'run and shared task ownership must agree' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER derived_run_job_guard AFTER INSERT ON derived_runs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION derived_run_job_guard();

-- +goose StatementBegin
CREATE FUNCTION derived_run_input_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM derived_runs r JOIN monitoring_raw_snapshots s ON s.tenant_id=r.tenant_id AND s.id=NEW.snapshot_id WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.run_id AND s.project_id IS NOT DISTINCT FROM r.project_id AND ((r.actor_id IS NOT NULL AND s.actor_id=r.actor_id) OR (r.actor_id IS NULL AND s.system_run_id=r.id))) THEN
  RAISE EXCEPTION 'fixed input snapshot belongs to another execution scope' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER derived_run_input_guard BEFORE INSERT ON derived_run_raw_inputs FOR EACH ROW EXECUTE FUNCTION derived_run_input_guard();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'calculation run evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
