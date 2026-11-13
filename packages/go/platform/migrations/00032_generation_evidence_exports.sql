-- +goose Up
ALTER TABLE monitoring_jobs DROP CONSTRAINT monitoring_jobs_kind_check;
ALTER TABLE monitoring_jobs ADD CONSTRAINT monitoring_jobs_kind_check CHECK(kind IN ('validate','publish','summary','export','verify','relocate','backup','restore','derive','analysis','derived_replay','analysis_compare','analysis_export','derived_evidence_export'));
ALTER TABLE monitoring_exports DROP CONSTRAINT monitoring_exports_source_kind_check;
ALTER TABLE monitoring_exports ADD CONSTRAINT monitoring_exports_source_kind_check CHECK(source_kind IN ('monitoring','analysis','derived_evidence'));
ALTER TABLE monitoring_exports ADD COLUMN generation_id uuid;
ALTER TABLE monitoring_exports ADD FOREIGN KEY(tenant_id,generation_id) REFERENCES derived_generations(tenant_id,id);
ALTER TABLE monitoring_exports DROP CONSTRAINT monitoring_export_source_check;
ALTER TABLE monitoring_exports ADD CONSTRAINT monitoring_export_source_check CHECK((source_kind='monitoring' AND snapshot_id IS NOT NULL AND artifact_version_id IS NULL AND generation_id IS NULL) OR (source_kind='analysis' AND snapshot_id IS NULL AND artifact_version_id IS NOT NULL AND generation_id IS NULL) OR (source_kind='derived_evidence' AND snapshot_id IS NULL AND artifact_version_id IS NULL AND generation_id IS NOT NULL));
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION monitoring_export_source_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND (NEW.id,NEW.tenant_id,NEW.job_id,NEW.snapshot_id,NEW.format,NEW.source_kind,NEW.artifact_version_id,NEW.generation_id,NEW.expires_at) IS DISTINCT FROM (OLD.id,OLD.tenant_id,OLD.job_id,OLD.snapshot_id,OLD.format,OLD.source_kind,OLD.artifact_version_id,OLD.generation_id,OLD.expires_at) THEN
  RAISE EXCEPTION 'export identity is immutable' USING ERRCODE='23514';
 END IF;
 IF NEW.source_kind='analysis' AND NOT EXISTS(SELECT 1 FROM monitoring_jobs j JOIN analysis_artifact_versions v ON v.tenant_id=j.tenant_id AND v.id=NEW.artifact_version_id JOIN analysis_artifacts a ON a.tenant_id=v.tenant_id AND a.id=v.artifact_id WHERE j.tenant_id=NEW.tenant_id AND j.id=NEW.job_id AND j.kind='analysis_export' AND j.target_id=NEW.id AND j.actor_id IS NOT NULL AND j.project_id IS NOT DISTINCT FROM a.project_id) THEN
  RAISE EXCEPTION 'export and artifact must belong to the same workspace' USING ERRCODE='23514';
 END IF;
 IF NEW.source_kind='derived_evidence' AND NOT EXISTS(SELECT 1 FROM monitoring_jobs j JOIN derived_generations g ON g.tenant_id=j.tenant_id AND g.id=NEW.generation_id WHERE j.tenant_id=NEW.tenant_id AND j.id=NEW.job_id AND j.kind='derived_evidence_export' AND j.target_id=NEW.id AND j.actor_id IS NOT NULL) THEN
  RAISE EXCEPTION 'evidence export requires a published tenant generation' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'permanent generation evidence export references cannot be downgraded'; END $$;
-- +goose StatementEnd
