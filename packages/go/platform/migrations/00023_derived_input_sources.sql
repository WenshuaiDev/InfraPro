-- +goose Up
-- Raw evidence stays in the native monitoring reader. This immutable companion
-- records the exact published upstream generations selected by a consumer.
CREATE TABLE derived_input_sources (
 tenant_id uuid NOT NULL,
 snapshot_id uuid NOT NULL,
 proof jsonb NOT NULL CHECK(jsonb_typeof(proof)='object' AND octet_length(proof::text)<=8388608),
 proof_hash text NOT NULL CHECK(proof_hash ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,snapshot_id),
 FOREIGN KEY(tenant_id,snapshot_id) REFERENCES monitoring_raw_snapshots(tenant_id,id)
);
CREATE TRIGGER derived_input_sources_immutable BEFORE UPDATE OR DELETE ON derived_input_sources FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
ALTER TABLE derived_replay_targets ADD COLUMN upstream_inputs jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(upstream_inputs)='array' AND octet_length(upstream_inputs::text)<=8388608);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'fixed upstream input evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
