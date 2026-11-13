-- +goose Up
CREATE TABLE analysis_artifacts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),tenant_id uuid NOT NULL,project_id uuid,config_id uuid NOT NULL,
 scope_key text GENERATED ALWAYS AS (COALESCE(project_id::text,'tenant')) STORED,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200), status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),latest_version bigint NOT NULL DEFAULT 0 CHECK(latest_version>=0),
 created_by uuid NOT NULL REFERENCES accounts(id),created_at timestamptz NOT NULL DEFAULT clock_timestamp(),updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id),UNIQUE(tenant_id,config_id,id),
 FOREIGN KEY(tenant_id,config_id) REFERENCES analysis_configs(tenant_id,id),FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id)
);
CREATE INDEX analysis_artifacts_page ON analysis_artifacts(tenant_id,scope_key,status,created_at DESC,id);
CREATE TABLE analysis_runs (
 tenant_id uuid NOT NULL,run_id uuid NOT NULL,config_id uuid NOT NULL,config_version_id uuid NOT NULL,
 artifact_id uuid,artifact_version_id uuid,trial boolean NOT NULL,outputs jsonb NOT NULL CHECK(jsonb_typeof(outputs)='array' AND jsonb_array_length(outputs) BETWEEN 1 AND 16),
 source_range int8range,run_rule jsonb NOT NULL CHECK(jsonb_typeof(run_rule)='object'),PRIMARY KEY(tenant_id,run_id), UNIQUE(tenant_id,artifact_version_id),
 FOREIGN KEY(tenant_id,run_id) REFERENCES derived_runs(tenant_id,id),FOREIGN KEY(tenant_id,config_id,config_version_id) REFERENCES analysis_config_versions(tenant_id,config_id,id),
 FOREIGN KEY(tenant_id,config_id,artifact_id) REFERENCES analysis_artifacts(tenant_id,config_id,id),
 CHECK((trial AND artifact_id IS NULL AND artifact_version_id IS NULL) OR (NOT trial AND artifact_id IS NOT NULL AND artifact_version_id IS NOT NULL))
);
CREATE INDEX analysis_runs_config ON analysis_runs(tenant_id,config_id,run_id);
CREATE TRIGGER analysis_run_immutable BEFORE UPDATE OR DELETE ON analysis_runs FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TABLE analysis_artifact_versions (
 id uuid PRIMARY KEY,tenant_id uuid NOT NULL,artifact_id uuid NOT NULL,version_no bigint NOT NULL CHECK(version_no>0),run_id uuid NOT NULL,
 config_version_id uuid NOT NULL,output_range int8range NOT NULL,raw_snapshot_id uuid NOT NULL,source_position bigint NOT NULL CHECK(source_position>=0),
 manifest_root text NOT NULL CHECK(manifest_root ~ '^[0-9a-f]{64}$'),lineage_root text NOT NULL CHECK(lineage_root ~ '^[0-9a-f]{64}$'),
 output_rows bigint NOT NULL CHECK(output_rows>=0),stale_at_publication boolean NOT NULL,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id),UNIQUE(tenant_id,artifact_id,version_no),UNIQUE(tenant_id,run_id),
 FOREIGN KEY(tenant_id,artifact_id) REFERENCES analysis_artifacts(tenant_id,id),FOREIGN KEY(tenant_id,run_id) REFERENCES analysis_runs(tenant_id,run_id),
 FOREIGN KEY(tenant_id,config_version_id) REFERENCES analysis_config_versions(tenant_id,id), FOREIGN KEY(tenant_id,raw_snapshot_id) REFERENCES monitoring_raw_snapshots(tenant_id,id)
);
CREATE TRIGGER analysis_artifact_version_immutable BEFORE UPDATE OR DELETE ON analysis_artifact_versions FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TABLE analysis_artifact_input_ranges (
 tenant_id uuid NOT NULL,version_id uuid NOT NULL,channel_id uuid NOT NULL,required_range int8range NOT NULL CHECK(NOT isempty(required_range)),
 PRIMARY KEY(tenant_id,version_id,channel_id,required_range),FOREIGN KEY(tenant_id,version_id) REFERENCES analysis_artifact_versions(tenant_id,id),FOREIGN KEY(tenant_id,channel_id) REFERENCES engineering_channels(tenant_id,id)
);
CREATE INDEX analysis_artifact_input_channel ON analysis_artifact_input_ranges(tenant_id,channel_id);
CREATE TRIGGER analysis_artifact_input_immutable BEFORE UPDATE OR DELETE ON analysis_artifact_input_ranges FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TABLE analysis_artifact_changes (
 tenant_id uuid NOT NULL,version_id uuid NOT NULL,event_id uuid NOT NULL,checked_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,version_id,event_id),FOREIGN KEY(tenant_id,version_id) REFERENCES analysis_artifact_versions(tenant_id,id), FOREIGN KEY(tenant_id,event_id) REFERENCES monitoring_changes(tenant_id,id)
);
CREATE TRIGGER analysis_artifact_change_immutable BEFORE UPDATE OR DELETE ON analysis_artifact_changes FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TABLE analysis_trial_receipts (
 tenant_id uuid NOT NULL,run_id uuid NOT NULL,result jsonb NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=16777216),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(tenant_id,run_id),FOREIGN KEY(tenant_id,run_id) REFERENCES analysis_runs(tenant_id,run_id)
);
CREATE TRIGGER analysis_trial_receipt_immutable BEFORE UPDATE OR DELETE ON analysis_trial_receipts FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
-- +goose StatementBegin
CREATE FUNCTION analysis_scope_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='analysis_artifacts' THEN
  IF TG_OP='UPDATE' AND (NEW.id IS DISTINCT FROM OLD.id OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id OR NEW.project_id IS DISTINCT FROM OLD.project_id OR NEW.config_id IS DISTINCT FROM OLD.config_id OR NEW.created_by IS DISTINCT FROM OLD.created_by OR NEW.created_at IS DISTINCT FROM OLD.created_at) THEN
   RAISE EXCEPTION 'analysis artifact identity and workspace are immutable' USING ERRCODE='23514';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM analysis_configs WHERE tenant_id=NEW.tenant_id AND id=NEW.config_id AND project_id IS NOT DISTINCT FROM NEW.project_id) THEN
   RAISE EXCEPTION 'analysis artifact belongs to configuration workspace' USING ERRCODE='23514';
  END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM derived_runs r JOIN analysis_configs c ON c.tenant_id=r.tenant_id AND c.project_id IS NOT DISTINCT FROM r.project_id WHERE r.tenant_id=NEW.tenant_id AND r.id=NEW.run_id AND c.id=NEW.config_id AND r.mode='analysis' AND r.channel_id IS NULL) THEN
   RAISE EXCEPTION 'analysis run belongs to configuration workspace' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER analysis_artifact_scope_guard BEFORE INSERT OR UPDATE ON analysis_artifacts FOR EACH ROW EXECUTE FUNCTION analysis_scope_guard();
CREATE TRIGGER analysis_run_scope_guard BEFORE INSERT ON analysis_runs FOR EACH ROW EXECUTE FUNCTION analysis_scope_guard();
-- +goose StatementBegin
CREATE FUNCTION analysis_publication_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM analysis_runs a JOIN derived_runs r ON r.tenant_id=a.tenant_id AND r.id=a.run_id JOIN monitoring_jobs j ON j.tenant_id=r.tenant_id AND j.id=r.job_id JOIN derived_run_raw_inputs i ON i.tenant_id=r.tenant_id AND i.run_id=r.id WHERE a.tenant_id=NEW.tenant_id AND a.run_id=NEW.run_id AND NOT a.trial AND a.artifact_id=NEW.artifact_id AND a.artifact_version_id=NEW.id AND a.config_version_id=NEW.config_version_id AND r.output_range=NEW.output_range AND i.snapshot_id=NEW.raw_snapshot_id AND j.state='succeeded') THEN
  RAISE EXCEPTION 'analysis artifact and shared task must publish atomically' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER analysis_publication_guard AFTER INSERT ON analysis_artifact_versions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION analysis_publication_guard();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'analysis run and artifact evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
