-- +goose Up
CREATE TABLE derived_replay_targets (
 tenant_id uuid NOT NULL, run_id uuid NOT NULL, generation_id uuid NOT NULL,
 baselines jsonb NOT NULL CHECK(jsonb_typeof(baselines)='array' AND octet_length(baselines::text)<=262144),
 PRIMARY KEY(tenant_id,run_id),
 FOREIGN KEY(tenant_id,run_id) REFERENCES derived_runs(tenant_id,id),
 FOREIGN KEY(tenant_id,generation_id) REFERENCES derived_generations(tenant_id,id)
);
CREATE TABLE derived_replay_receipts (
 tenant_id uuid NOT NULL, run_id uuid NOT NULL,
 result jsonb NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=4194304),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,run_id), FOREIGN KEY(tenant_id,run_id) REFERENCES derived_replay_targets(tenant_id,run_id)
);
CREATE TRIGGER derived_replay_targets_immutable BEFORE UPDATE OR DELETE ON derived_replay_targets FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TRIGGER derived_replay_receipts_immutable BEFORE UPDATE OR DELETE ON derived_replay_receipts FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
-- +goose StatementBegin
CREATE FUNCTION derived_replay_target_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM derived_runs r JOIN derived_generations g ON g.tenant_id=r.tenant_id AND g.channel_id=r.channel_id AND g.config_id=r.config_id WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.run_id AND g.id=NEW.generation_id AND r.mode='historical_replay' AND g.output_range @> r.output_range) THEN
  RAISE EXCEPTION 'replay target must match immutable generation and covered range' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER derived_replay_target_guard BEFORE INSERT ON derived_replay_targets FOR EACH ROW EXECUTE FUNCTION derived_replay_target_guard();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'historical replay evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
