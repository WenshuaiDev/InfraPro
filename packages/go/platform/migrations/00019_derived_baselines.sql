-- +goose Up
ALTER TABLE derived_channel_configs ADD COLUMN baseline_bindings jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(baseline_bindings)='object');
CREATE TABLE derived_fixed_baselines (
 tenant_id uuid NOT NULL, config_id uuid NOT NULL, reference_key text NOT NULL,
 snapshot_id uuid NOT NULL, evidence jsonb NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<1048576),
 evidence_hash text NOT NULL CHECK(evidence_hash ~ '^[0-9a-f]{64}$'),
 created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,config_id,reference_key),
 FOREIGN KEY(tenant_id,config_id) REFERENCES derived_channel_configs(tenant_id,id),
 FOREIGN KEY(tenant_id,snapshot_id) REFERENCES monitoring_raw_snapshots(tenant_id,id)
);
CREATE TRIGGER derived_fixed_baseline_immutable BEFORE UPDATE OR DELETE ON derived_fixed_baselines FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'fixed baseline evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
