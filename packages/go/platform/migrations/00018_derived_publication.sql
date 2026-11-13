-- +goose Up
CREATE TABLE derived_run_publication_targets (
 tenant_id uuid NOT NULL,run_id uuid NOT NULL,channel_id uuid NOT NULL,generation_id uuid NOT NULL,
 target_hash text NOT NULL CHECK(target_hash ~ '^[0-9a-f]{64}$'),required_position bigint NOT NULL CHECK(required_position>0),
 graph_revision bigint NOT NULL CHECK(graph_revision>0),PRIMARY KEY(tenant_id,run_id),UNIQUE(tenant_id,target_hash),UNIQUE(tenant_id,generation_id),
 FOREIGN KEY(tenant_id,run_id) REFERENCES derived_runs(tenant_id,id),FOREIGN KEY(tenant_id,channel_id) REFERENCES derived_channels(tenant_id,channel_id)
);
CREATE TRIGGER derived_run_target_immutable BEFORE UPDATE OR DELETE ON derived_run_publication_targets FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TABLE derived_write_attempts (
 tenant_id uuid NOT NULL,id uuid NOT NULL,run_id uuid NOT NULL,chunk_id uuid NOT NULL,output_range int8range NOT NULL CHECK(NOT isempty(output_range)),
 fence bigint NOT NULL CHECK(fence>0),state text NOT NULL CHECK(state IN ('writing','sealed','abandoned')),
 physical_shard text NOT NULL,route_revision bigint NOT NULL CHECK(route_revision>0),
 manifest jsonb CHECK(manifest IS NULL OR jsonb_typeof(manifest)='object'),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),sealed_at timestamptz,
 PRIMARY KEY(tenant_id,id),FOREIGN KEY(tenant_id,run_id) REFERENCES derived_runs(tenant_id,id),CHECK((state='sealed')=(sealed_at IS NOT NULL AND manifest IS NOT NULL))
);
CREATE INDEX derived_attempt_run ON derived_write_attempts(tenant_id,run_id,chunk_id);
-- +goose StatementBegin
CREATE FUNCTION derived_attempt_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.state<>'writing' OR NEW.state NOT IN ('sealed','abandoned') OR
 (to_jsonb(NEW)-ARRAY['state','manifest','sealed_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','manifest','sealed_at']) THEN
  RAISE EXCEPTION 'result attempt only permits one final seal or abandonment' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER derived_attempt_guard BEFORE UPDATE OR DELETE ON derived_write_attempts FOR EACH ROW EXECUTE FUNCTION derived_attempt_guard();
CREATE TABLE derived_generations (
 id uuid PRIMARY KEY,tenant_id uuid NOT NULL,channel_id uuid NOT NULL,run_id uuid NOT NULL,config_id uuid NOT NULL,
 output_range int8range NOT NULL CHECK(NOT isempty(output_range) AND NOT lower_inf(output_range)),
 manifest_root text NOT NULL CHECK(manifest_root ~ '^[0-9a-f]{64}$'),lineage_root text NOT NULL CHECK(lineage_root ~ '^[0-9a-f]{64}$'),
 raw_snapshot_id uuid NOT NULL,source_position bigint NOT NULL CHECK(source_position>=0),publish_position bigint NOT NULL CHECK(publish_position>0),
 row_count bigint NOT NULL CHECK(row_count>=0),status_counts jsonb NOT NULL CHECK(jsonb_typeof(status_counts)='object'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),UNIQUE(tenant_id,id),UNIQUE(tenant_id,run_id),
 FOREIGN KEY(tenant_id,run_id) REFERENCES derived_runs(tenant_id,id),FOREIGN KEY(tenant_id,channel_id,config_id) REFERENCES derived_channel_configs(tenant_id,channel_id,id),
 FOREIGN KEY(tenant_id,raw_snapshot_id) REFERENCES monitoring_raw_snapshots(tenant_id,id)
);
CREATE INDEX derived_generation_history ON derived_generations(tenant_id,channel_id,created_at DESC,id);
CREATE INDEX derived_generation_range ON derived_generations USING gist(tenant_id public.gist_uuid_ops,channel_id public.gist_uuid_ops,output_range);
CREATE TRIGGER derived_generation_immutable BEFORE UPDATE OR DELETE ON derived_generations FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TABLE derived_publication_receipts (
 tenant_id uuid NOT NULL,run_id uuid NOT NULL,generation_id uuid NOT NULL,head_revision bigint NOT NULL,current_root text NOT NULL,history_root text NOT NULL,
 PRIMARY KEY(tenant_id,run_id),FOREIGN KEY(tenant_id,run_id) REFERENCES derived_runs(tenant_id,id),FOREIGN KEY(tenant_id,generation_id) REFERENCES derived_generations(tenant_id,id)
);
CREATE TRIGGER derived_publication_receipt_immutable BEFORE UPDATE OR DELETE ON derived_publication_receipts FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'formal result history cannot be downgraded'; END $$;
-- +goose StatementEnd
