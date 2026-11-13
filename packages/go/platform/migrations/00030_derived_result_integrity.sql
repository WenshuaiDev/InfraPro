-- +goose Up
-- Mutable maintenance state is separate from immutable generations/evidence.
CREATE TABLE derived_result_integrity (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 result_kind text NOT NULL CHECK(result_kind IN ('derived_generations','analysis_artifact_versions')),
 result_id uuid NOT NULL,
 state text NOT NULL CHECK(state IN ('available','unavailable')),
 checked_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 job_id uuid NOT NULL,
 PRIMARY KEY(tenant_id,result_kind,result_id),
 FOREIGN KEY(tenant_id,job_id) REFERENCES monitoring_jobs(tenant_id,id)
);
CREATE INDEX derived_result_integrity_unavailable ON derived_result_integrity(tenant_id,result_kind,result_id) WHERE state='unavailable';
-- +goose StatementBegin
CREATE FUNCTION derived_result_integrity_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR (TG_OP='UPDATE' AND (NEW.tenant_id,NEW.result_kind,NEW.result_id) IS DISTINCT FROM (OLD.tenant_id,OLD.result_kind,OLD.result_id)) THEN
  RAISE EXCEPTION 'result maintenance identity is immutable' USING ERRCODE='23514';
 END IF;
 IF (NEW.result_kind='derived_generations' AND NOT EXISTS(SELECT 1 FROM derived_generations WHERE tenant_id=NEW.tenant_id AND id=NEW.result_id)) OR
 (NEW.result_kind='analysis_artifact_versions' AND NOT EXISTS(SELECT 1 FROM analysis_artifact_versions WHERE tenant_id=NEW.tenant_id AND id=NEW.result_id)) THEN
  RAISE EXCEPTION 'result maintenance requires a published immutable target' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER derived_result_integrity_guard BEFORE INSERT OR UPDATE OR DELETE ON derived_result_integrity FOR EACH ROW EXECUTE FUNCTION derived_result_integrity_guard();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'result integrity evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
