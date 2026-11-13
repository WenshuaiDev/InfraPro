-- +goose Up
CREATE TABLE monitoring_retention_receipts (
 tenant_id uuid NOT NULL REFERENCES tenants(id),
 kind text NOT NULL CHECK(kind IN ('export','derived_temp','derived_attempt','derived_checkpoint','derived_trial','analysis_trial')),
 target text NOT NULL CHECK(length(target) BETWEEN 1 AND 200),
 state text NOT NULL CHECK(state IN ('pending','completed')),
 details jsonb NOT NULL CHECK(jsonb_typeof(details)='object' AND octet_length(details::text)<=65536),
 job_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),completed_at timestamptz,
 PRIMARY KEY(tenant_id,kind,target),
 FOREIGN KEY(tenant_id,job_id) REFERENCES monitoring_jobs(tenant_id,id),
 CHECK((state='completed')=(completed_at IS NOT NULL))
);
-- +goose StatementBegin
CREATE FUNCTION monitoring_retention_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.state='completed' OR
 (NEW.tenant_id,NEW.kind,NEW.target,NEW.details,NEW.created_at) IS DISTINCT FROM (OLD.tenant_id,OLD.kind,OLD.target,OLD.details,OLD.created_at) THEN
  RAISE EXCEPTION 'retention proof identity is immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER monitoring_retention_guard BEFORE UPDATE OR DELETE ON monitoring_retention_receipts FOR EACH ROW EXECUTE FUNCTION monitoring_retention_guard();
CREATE INDEX derived_mandate_trial_reference ON derived_execution_mandates(tenant_id,(basis->>'trial_receipt_id'));
-- +goose StatementBegin
CREATE FUNCTION derived_trial_retention_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'DELETE' THEN RAISE EXCEPTION 'trial evidence cannot be rewritten' USING ERRCODE='23514'; END IF;
 IF TG_TABLE_NAME='derived_trial_receipts' THEN
  IF OLD.expires_at>clock_timestamp() OR EXISTS(SELECT 1 FROM derived_execution_mandates WHERE tenant_id=OLD.tenant_id AND basis->>'trial_receipt_id'=OLD.id::text) OR
   NOT EXISTS(SELECT 1 FROM monitoring_retention_receipts WHERE tenant_id=OLD.tenant_id AND kind='derived_trial' AND target=OLD.id::text AND state='pending') THEN
   RAISE EXCEPTION 'referenced or live trial evidence must be retained' USING ERRCODE='23514';
  END IF;
 ELSE
  IF (OLD.result->>'expires_at')::timestamptz>clock_timestamp() OR NOT EXISTS(SELECT 1 FROM analysis_runs WHERE tenant_id=OLD.tenant_id AND run_id=OLD.run_id AND trial) OR
   NOT EXISTS(SELECT 1 FROM monitoring_retention_receipts WHERE tenant_id=OLD.tenant_id AND kind='analysis_trial' AND target=OLD.run_id::text AND state='pending') THEN
   RAISE EXCEPTION 'live analysis trial evidence must be retained' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN OLD;
END $$;
-- +goose StatementEnd
DROP TRIGGER derived_trial_receipt_immutable ON derived_trial_receipts;
CREATE TRIGGER derived_trial_receipt_immutable BEFORE UPDATE OR DELETE ON derived_trial_receipts FOR EACH ROW EXECUTE FUNCTION derived_trial_retention_guard();
DROP TRIGGER analysis_trial_receipt_immutable ON analysis_trial_receipts;
CREATE TRIGGER analysis_trial_receipt_immutable BEFORE UPDATE OR DELETE ON analysis_trial_receipts FOR EACH ROW EXECUTE FUNCTION derived_trial_retention_guard();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'retention receipts cannot be downgraded'; END $$;
-- +goose StatementEnd
