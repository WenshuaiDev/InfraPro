-- +goose Up
ALTER TABLE monitoring_jobs ADD COLUMN task_reason text NOT NULL DEFAULT '', ADD COLUMN task_revision bigint NOT NULL DEFAULT 0 CHECK(task_revision>=0), ADD COLUMN task_observed_at timestamptz NOT NULL DEFAULT clock_timestamp(), ADD COLUMN task_requested_by uuid REFERENCES accounts(id), ADD CHECK(notification_generation>0);
ALTER TABLE spatial_processing_jobs ADD COLUMN task_reason text NOT NULL DEFAULT '', ADD COLUMN task_revision bigint NOT NULL DEFAULT 0 CHECK(task_revision>=0), ADD COLUMN task_observed_at timestamptz NOT NULL DEFAULT clock_timestamp(), ADD COLUMN originator_account_id uuid REFERENCES accounts(id), ADD COLUMN task_requested_by uuid REFERENCES accounts(id), ADD CHECK(notification_generation>0);
ALTER TABLE display_package_jobs ADD COLUMN task_reason text NOT NULL DEFAULT '', ADD COLUMN task_revision bigint NOT NULL DEFAULT 0 CHECK(task_revision>=0), ADD COLUMN task_observed_at timestamptz NOT NULL DEFAULT clock_timestamp(), ADD COLUMN execution_generation bigint NOT NULL DEFAULT 1 CHECK(execution_generation>0), ADD COLUMN control_revision bigint NOT NULL DEFAULT 1 CHECK(control_revision>0), ADD COLUMN originator_account_id uuid, ADD COLUMN task_requested_by uuid;
UPDATE monitoring_jobs SET task_requested_by=actor_id;
UPDATE spatial_processing_jobs SET task_requested_by=actor_id;
-- Recover identity only from a unique, durable creation fact, never from the
-- mutable actor of a later retry. Conflicting/missing evidence remains unknown.
ALTER TABLE spatial_processing_jobs ADD COLUMN originator_evidence_ref uuid REFERENCES spatial_changes(id);
WITH proven AS (
 SELECT target_id,min(id::text)::uuid evidence,min(actor_id::text)::uuid actor
 FROM spatial_changes WHERE operation='versions.process' AND actor_id IS NOT NULL
 GROUP BY target_id HAVING count(DISTINCT actor_id)=1
)
UPDATE spatial_processing_jobs j SET originator_account_id=p.actor,originator_evidence_ref=p.evidence FROM proven p
WHERE p.target_id=j.id AND EXISTS(SELECT 1 FROM spatial_changes c WHERE c.id=p.evidence AND c.tenant_id=j.tenant_id);
UPDATE display_package_jobs j SET originator_account_id=u.actor_id,task_requested_by=u.actor_id FROM display_uploads u WHERE j.upload_id=u.id;
CREATE TABLE display_task_handoffs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), job_id uuid NOT NULL REFERENCES display_package_jobs(id),
 task_revision bigint NOT NULL, execution_generation bigint NOT NULL, schema_version int NOT NULL DEFAULT 1 CHECK(schema_version=1),
 snapshot jsonb NOT NULL, fingerprint text NOT NULL, occurred_at timestamptz NOT NULL,
 translated_at timestamptz, available_at timestamptz NOT NULL DEFAULT clock_timestamp(), attempts int NOT NULL DEFAULT 0,
 last_error_code text NOT NULL DEFAULT '', UNIQUE(job_id,task_revision), CHECK(octet_length(snapshot::text)<=32768)
);
CREATE INDEX display_task_pending ON display_task_handoffs(available_at,id) WHERE translated_at IS NULL;
CREATE TABLE monitoring_task_execution_evidence (
 job_id uuid NOT NULL REFERENCES monitoring_jobs(id), generation bigint NOT NULL,
 error_code text NOT NULL, report jsonb NOT NULL, progress jsonb NOT NULL,
 input_hash text NOT NULL, executor_ref text NOT NULL, finished_at timestamptz NOT NULL,
 PRIMARY KEY(job_id,generation)
);
CREATE TABLE spatial_task_execution_evidence (
 job_id uuid NOT NULL REFERENCES spatial_processing_jobs(id), generation bigint NOT NULL,
 error_code text NOT NULL, report jsonb NOT NULL, progress jsonb NOT NULL,
 input_hash text NOT NULL, executor_ref text NOT NULL, finished_at timestamptz NOT NULL,
 PRIMARY KEY(job_id,generation)
);
CREATE TABLE display_task_execution_evidence (
 job_id uuid NOT NULL REFERENCES display_package_jobs(id), generation bigint NOT NULL,
 error_code text NOT NULL, report jsonb NOT NULL, progress jsonb NOT NULL,
 input_hash text NOT NULL, executor_ref text NOT NULL, finished_at timestamptz NOT NULL,
 PRIMARY KEY(job_id,generation)
);
-- Source journals contain only bounded metadata. Full reports stay in evidence
-- tables owned by each source and are not granted to the index role.
-- +goose StatementBegin
CREATE FUNCTION task_safe_progress(p jsonb) RETURNS jsonb LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE pair text[]; completed text; total text;
BEGIN
 FOREACH pair SLICE 1 IN ARRAY ARRAY[['completed_chunks','total_chunks','units'],['completed_parts','total_parts','units'],['completed_files','files','files'],['bytes_read','total_bytes','bytes'],['output_rows','total_rows','rows'],['processed_rows','total_rows','rows'],['rows','total_rows','rows'],['input_rows','','rows'],['parsed_rows','','rows']] LOOP
  completed:=p->>pair[1];total:=p->>pair[2];
  IF completed ~ '^[0-9]{1,18}$' THEN
   IF NOT COALESCE(total ~ '^[0-9]{1,18}$',false) OR total::numeric<completed::numeric THEN total:=NULL;END IF;
   RETURN jsonb_build_object('completed',completed,'total',total,'unit',pair[3]);
  END IF;
 END LOOP;
 RETURN jsonb_build_object('total',NULL);
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION monitoring_task_snapshot(j monitoring_jobs) RETURNS jsonb LANGUAGE plpgsql STABLE AS $$
DECLARE typ text; module text:='monitoring'; kind text:='job'; root uuid:=j.id; phase text; st text:=j.state;
BEGIN
 IF j.actor_id IS NULL OR j.display_subject_id IS NOT NULL THEN RETURN NULL; END IF;
 CASE j.kind
 WHEN 'validate' THEN typ:='monitoring.import.validate';
 WHEN 'publish' THEN typ:='monitoring.import.adopt';
 WHEN 'export' THEN typ:='monitoring.export';
 WHEN 'summary' THEN IF j.input->>'mode'<>'query' OR j.input->>'mode' IS NULL THEN RETURN NULL; END IF; typ:='monitoring.query';
 WHEN 'derive' THEN IF j.input->>'mode'='automatic' THEN RETURN NULL; END IF; typ:=CASE WHEN j.input->>'mode'='trial' THEN 'derived.trial' ELSE 'derived.backfill' END;
 WHEN 'analysis' THEN typ:=CASE WHEN j.input->>'trial'='true' THEN 'analysis.trial' ELSE 'analysis.run' END;
 WHEN 'derived_replay' THEN typ:='derived.replay';
 WHEN 'analysis_compare' THEN typ:='analysis.compare';
 WHEN 'analysis_export' THEN typ:='analysis.export';
 WHEN 'derived_evidence_export' THEN typ:='derived.evidence.export';
 ELSE RETURN NULL; END CASE;
 IF j.kind IN ('derive','analysis','derived_replay','analysis_compare','analysis_export','derived_evidence_export') THEN module:='derived';kind:=CASE WHEN j.kind IN ('analysis_export','derived_evidence_export') THEN 'export' ELSE 'run' END;root:=j.target_id; END IF;
 IF j.cancel_requested AND j.state='running' THEN st:='cancelling'; END IF;
 IF j.state='running' AND j.progress->>'phase' IN ('waiting','waiting_dependencies','waiting_upstream') AND NOT j.cancel_requested THEN st:='waiting';END IF;
 phase:=CASE WHEN st='succeeded' THEN 'completed' WHEN st IN ('failed','needs_action') THEN 'failed' WHEN st IN ('cancelled','cancelling') THEN 'cancelled' WHEN st='waiting' THEN 'waiting' WHEN st='running' THEN CASE WHEN j.kind='validate' THEN 'validating' ELSE 'computing' END ELSE 'queued' END;
 RETURN jsonb_build_object('source_module',module,'source_kind',kind,'source_root_id',root,'type',typ,
 'scope',jsonb_build_object('kind',CASE WHEN j.project_id IS NULL THEN 'tenant' ELSE 'project' END,'tenant_id',j.tenant_id,'project_id',COALESCE(j.project_id::text,'')),
 'originator_account_id',j.actor_id,'origin_kind','person','requested_by',COALESCE(j.task_requested_by,j.actor_id),'executor_kind','person','executor_ref',j.actor_id::text,
 'generation',j.notification_generation::text,'source_revision',j.task_revision::text,'control_revision',j.revision::text,
 'state',st,'source_state',j.state,'phase',phase,'progress',task_safe_progress(j.progress),'error_code',COALESCE(j.error_code,''),
 'input_ref',jsonb_build_object('job_id',j.id::text,'target_id',j.target_id::text),'input_hash',j.input_hash,
 'result_ref',jsonb_build_object('target_id',j.target_id::text,'job_id',j.id::text,'generation',j.notification_generation::text),
 'created_at',j.created_at,'observed_at',j.task_observed_at,'history_incomplete',j.task_revision=0,'fence',j.fence::text,'attempt',j.attempts,
 'event',CASE WHEN j.task_revision=0 THEN 'baseline' ELSE j.state END,'reason',j.task_reason);
END; $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION spatial_task_snapshot(j spatial_processing_jobs) RETURNS jsonb LANGUAGE sql STABLE AS $$
 SELECT jsonb_build_object('source_module','spatial','source_kind','job','source_root_id',j.id,'type','spatial.process',
 'scope',jsonb_build_object('kind','tenant','tenant_id',j.tenant_id),
 'originator_account_id',COALESCE(j.originator_account_id::text,''),'origin_kind',CASE WHEN j.originator_account_id IS NULL THEN 'legacy_unknown' ELSE 'person' END,
 'requested_by',COALESCE(j.task_requested_by,j.actor_id),'executor_kind','person','executor_ref',j.actor_id::text,
 'generation',j.notification_generation::text,'source_revision',j.task_revision::text,'control_revision',j.revision::text,
 'state',CASE j.state WHEN 'canceled' THEN 'cancelled' ELSE j.state END,'source_state',j.state,
 'phase',CASE j.state WHEN 'running' THEN 'computing' WHEN 'succeeded' THEN 'completed' WHEN 'canceled' THEN 'cancelled' ELSE j.state END,
 'progress',jsonb_build_object('total',NULL),'error_code',j.error_code,
 'input_ref',jsonb_build_object('job_id',j.id::text,'version_id',j.version_id::text,'source_blob_id',j.source_blob_id::text),'input_hash',md5(j.source_blob_id::text),
 'result_ref',jsonb_build_object('version_id',j.version_id::text,'job_id',j.id::text,'generation',j.notification_generation::text),
 'created_at',j.created_at,'observed_at',j.task_observed_at,'history_incomplete',j.task_revision=0,'fence',j.fence::text,'attempt',j.attempt,
 'event',CASE WHEN j.task_revision=0 THEN 'baseline' ELSE j.state END,'reason',j.task_reason)
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION display_task_snapshot(j display_package_jobs) RETURNS jsonb LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $$
 SELECT jsonb_build_object('source_module','display','source_kind','job','source_root_id',j.id,'type','display.package',
 'scope',jsonb_build_object('kind','project','tenant_id',u.tenant_id,'project_id',u.project_id),
 'originator_account_id',j.originator_account_id,'origin_kind','person','requested_by',j.task_requested_by,'executor_kind','person','executor_ref',j.task_requested_by::text,
 'generation',j.execution_generation::text,'source_revision',j.task_revision::text,'control_revision',j.control_revision::text,
 'state',CASE j.state WHEN 'validating' THEN 'running' WHEN 'ready' THEN 'succeeded' WHEN 'canceled' THEN 'cancelled' ELSE j.state END,'source_state',j.state,
 'phase',CASE j.state WHEN 'ready' THEN 'completed' WHEN 'canceled' THEN 'cancelled' ELSE j.state END,
 'progress',jsonb_build_object('total',NULL),'error_code',j.error_code,
 'input_ref',jsonb_build_object('job_id',j.id::text,'upload_id',u.id::text),'input_hash',u.sha256,
 'result_ref',jsonb_build_object('job_id',j.id::text,'instance_id',u.instance_id::text,'upload_id',u.id::text,'generation',j.execution_generation::text),
 'created_at',j.created_at,'observed_at',j.task_observed_at,'history_incomplete',j.task_revision=0,'fence',j.fence::text,'attempt',j.attempt,
 'event',CASE WHEN j.task_revision=0 THEN 'baseline' ELSE j.state END,'reason',j.task_reason) FROM display_uploads u WHERE u.id=j.upload_id
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION monitoring_task_capture_before() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id OR NEW.project_id IS DISTINCT FROM OLD.project_id OR NEW.actor_id IS DISTINCT FROM OLD.actor_id OR NEW.kind IS DISTINCT FROM OLD.kind OR NEW.target_id IS DISTINCT FROM OLD.target_id OR NEW.input_hash IS DISTINCT FROM OLD.input_hash OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN RAISE EXCEPTION 'immutable task source identity'; END IF;
  IF NEW.state IS NOT DISTINCT FROM OLD.state AND NEW.progress IS NOT DISTINCT FROM OLD.progress AND NEW.notification_generation IS NOT DISTINCT FROM OLD.notification_generation AND NEW.attempts IS NOT DISTINCT FROM OLD.attempts AND NEW.fence IS NOT DISTINCT FROM OLD.fence AND NEW.cancel_requested IS NOT DISTINCT FROM OLD.cancel_requested AND NEW.error_code IS NOT DISTINCT FROM OLD.error_code THEN RETURN NEW; END IF;
  IF NEW.notification_generation<OLD.notification_generation OR NEW.notification_generation>OLD.notification_generation+1 THEN RAISE EXCEPTION 'invalid task generation'; END IF;
  IF NEW.notification_generation<>OLD.notification_generation AND OLD.state NOT IN ('succeeded','failed','cancelled','needs_action') THEN RAISE EXCEPTION 'retry requires terminal execution'; END IF;
  IF NEW.notification_generation<>OLD.notification_generation THEN
   INSERT INTO monitoring_task_execution_evidence(job_id,generation,error_code,report,progress,input_hash,executor_ref,finished_at) VALUES(OLD.id,OLD.notification_generation,COALESCE(OLD.error_code,''),'{}'::jsonb,OLD.progress,OLD.input_hash,COALESCE(OLD.actor_id::text,''),OLD.task_observed_at) ON CONFLICT DO NOTHING;
   IF OLD.task_revision=0 AND monitoring_task_snapshot(OLD) IS NOT NULL THEN INSERT INTO task_transaction_facts(change) VALUES(monitoring_task_snapshot(OLD)); PERFORM set_config('infrapro.task_changes','1',true); END IF;
  END IF;
  NEW.task_revision:=OLD.task_revision+1;
 ELSE NEW.task_revision:=1; NEW.task_requested_by:=NEW.actor_id; END IF;
 NEW.task_observed_at:=clock_timestamp();
 NEW.task_reason:=COALESCE(current_setting('infrapro.task_reason',true),'');
 IF TG_OP='INSERT' OR NEW.notification_generation<>OLD.notification_generation THEN
  NEW.task_requested_by:=COALESCE(NULLIF(current_setting('infrapro.task_actor',true),'')::uuid,NEW.task_requested_by);
 END IF;
 RETURN NEW; END; $$;
-- +goose StatementEnd
CREATE TRIGGER monitoring_task_capture_before BEFORE INSERT OR UPDATE ON monitoring_jobs FOR EACH ROW EXECUTE FUNCTION monitoring_task_capture_before();
-- +goose StatementBegin
CREATE FUNCTION monitoring_task_capture_after() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE snapshot jsonb;
BEGIN
 IF TG_OP='UPDATE' AND NEW.task_revision=OLD.task_revision THEN RETURN NEW; END IF;
 snapshot:=monitoring_task_snapshot(NEW);
 IF snapshot IS NULL THEN RETURN NEW; END IF;
 IF NEW.state IN ('succeeded','failed','cancelled','needs_action') THEN
  INSERT INTO monitoring_task_execution_evidence(job_id,generation,error_code,report,progress,input_hash,executor_ref,finished_at)
  VALUES(NEW.id,NEW.notification_generation,COALESCE(NEW.error_code,''),'{}'::jsonb,NEW.progress,NEW.input_hash,COALESCE(NEW.actor_id::text,''),NEW.task_observed_at) ON CONFLICT DO NOTHING;
 END IF;
 INSERT INTO task_transaction_facts(change) VALUES(snapshot);
 PERFORM set_config('infrapro.task_changes','1',true);
 RETURN NEW; END; $$;
-- +goose StatementEnd
CREATE TRIGGER monitoring_task_capture_after AFTER INSERT OR UPDATE ON monitoring_jobs FOR EACH ROW EXECUTE FUNCTION monitoring_task_capture_after();
CREATE VIEW monitoring_task_metadata AS SELECT id,task_revision,monitoring_task_snapshot(j) AS snapshot FROM monitoring_jobs j;
-- +goose StatementBegin
CREATE FUNCTION spatial_task_capture_before() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id OR NEW.version_id IS DISTINCT FROM OLD.version_id OR NEW.source_blob_id IS DISTINCT FROM OLD.source_blob_id OR NEW.originator_account_id IS DISTINCT FROM OLD.originator_account_id OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN RAISE EXCEPTION 'immutable task source identity'; END IF;
  IF NEW.state IS NOT DISTINCT FROM OLD.state AND NEW.stage IS NOT DISTINCT FROM OLD.stage AND NEW.notification_generation IS NOT DISTINCT FROM OLD.notification_generation AND NEW.attempt IS NOT DISTINCT FROM OLD.attempt AND NEW.fence IS NOT DISTINCT FROM OLD.fence AND NEW.error_code IS NOT DISTINCT FROM OLD.error_code THEN RETURN NEW; END IF;
  IF NEW.notification_generation<OLD.notification_generation OR NEW.notification_generation>OLD.notification_generation+1 THEN RAISE EXCEPTION 'invalid task generation'; END IF;
  IF NEW.notification_generation<>OLD.notification_generation AND OLD.state NOT IN ('succeeded','failed','canceled') THEN RAISE EXCEPTION 'retry requires terminal execution'; END IF;
  IF NEW.notification_generation<>OLD.notification_generation THEN
   INSERT INTO spatial_task_execution_evidence(job_id,generation,error_code,report,progress,input_hash,executor_ref,finished_at) VALUES(OLD.id,OLD.notification_generation,COALESCE(OLD.error_code,''),OLD.report,'{}'::jsonb,md5(OLD.source_blob_id::text),COALESCE(OLD.actor_id::text,''),OLD.task_observed_at) ON CONFLICT DO NOTHING;
   IF OLD.task_revision=0 AND spatial_task_snapshot(OLD) IS NOT NULL THEN INSERT INTO task_transaction_facts(change) VALUES(spatial_task_snapshot(OLD)); PERFORM set_config('infrapro.task_changes','1',true); END IF;
  END IF;
  NEW.task_revision:=OLD.task_revision+1;
 ELSE NEW.task_revision:=1; NEW.originator_account_id:=NEW.actor_id; NEW.task_requested_by:=NEW.actor_id; END IF;
 NEW.task_observed_at:=clock_timestamp();
 NEW.task_reason:=COALESCE(current_setting('infrapro.task_reason',true),'');
 IF TG_OP='INSERT' OR NEW.notification_generation<>OLD.notification_generation THEN
  NEW.task_requested_by:=COALESCE(NULLIF(current_setting('infrapro.task_actor',true),'')::uuid,NEW.task_requested_by);
 END IF;
 RETURN NEW; END; $$;
-- +goose StatementEnd
CREATE TRIGGER spatial_task_capture_before BEFORE INSERT OR UPDATE ON spatial_processing_jobs FOR EACH ROW EXECUTE FUNCTION spatial_task_capture_before();
-- +goose StatementBegin
CREATE FUNCTION spatial_task_capture_after() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE snapshot jsonb;
BEGIN
 IF TG_OP='UPDATE' AND NEW.task_revision=OLD.task_revision THEN RETURN NEW; END IF;
 snapshot:=spatial_task_snapshot(NEW);
 IF snapshot IS NULL THEN RETURN NEW; END IF;
 IF NEW.state IN ('succeeded','failed','canceled') THEN
  INSERT INTO spatial_task_execution_evidence(job_id,generation,error_code,report,progress,input_hash,executor_ref,finished_at)
  VALUES(NEW.id,NEW.notification_generation,COALESCE(NEW.error_code,''),NEW.report,'{}'::jsonb,md5(NEW.source_blob_id::text),COALESCE(NEW.actor_id::text,''),NEW.task_observed_at) ON CONFLICT DO NOTHING;
 END IF;
 INSERT INTO task_transaction_facts(change) VALUES(snapshot);
 PERFORM set_config('infrapro.task_changes','1',true);
 RETURN NEW; END; $$;
-- +goose StatementEnd
CREATE TRIGGER spatial_task_capture_after AFTER INSERT OR UPDATE ON spatial_processing_jobs FOR EACH ROW EXECUTE FUNCTION spatial_task_capture_after();
CREATE VIEW spatial_task_metadata AS SELECT id,task_revision,spatial_task_snapshot(j) AS snapshot FROM spatial_processing_jobs j;
-- +goose StatementBegin
CREATE FUNCTION display_task_capture_before() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.upload_id IS DISTINCT FROM OLD.upload_id OR NEW.originator_account_id IS DISTINCT FROM OLD.originator_account_id OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN RAISE EXCEPTION 'immutable task source identity'; END IF;
  IF NEW.state IS NOT DISTINCT FROM OLD.state AND NEW.attempt IS NOT DISTINCT FROM OLD.attempt AND NEW.fence IS NOT DISTINCT FROM OLD.fence AND NEW.error_code IS NOT DISTINCT FROM OLD.error_code AND NEW.execution_generation IS NOT DISTINCT FROM OLD.execution_generation THEN RETURN NEW; END IF;
  IF NEW.execution_generation<OLD.execution_generation OR NEW.execution_generation>OLD.execution_generation+1 THEN RAISE EXCEPTION 'invalid task generation'; END IF;
  IF NEW.execution_generation<>OLD.execution_generation AND OLD.state NOT IN ('ready','failed','canceled') THEN RAISE EXCEPTION 'retry requires terminal execution'; END IF;
  IF NEW.execution_generation<>OLD.execution_generation THEN
   INSERT INTO display_task_execution_evidence(job_id,generation,error_code,report,progress,input_hash,executor_ref,finished_at) VALUES(OLD.id,OLD.execution_generation,COALESCE(OLD.error_code,''),OLD.issues,'{}'::jsonb,(SELECT sha256 FROM display_uploads WHERE id=OLD.upload_id),COALESCE(OLD.task_requested_by::text,''),OLD.task_observed_at) ON CONFLICT DO NOTHING;
   IF OLD.task_revision=0 THEN INSERT INTO display_task_handoffs(job_id,task_revision,execution_generation,snapshot,fingerprint,occurred_at) VALUES(OLD.id,0,OLD.execution_generation,display_task_snapshot(OLD),md5(display_task_snapshot(OLD)::text),OLD.task_observed_at) ON CONFLICT DO NOTHING; END IF;
  END IF;
  NEW.task_revision:=OLD.task_revision+1;
 ELSE NEW.task_revision:=1; SELECT actor_id INTO NEW.originator_account_id FROM display_uploads WHERE id=NEW.upload_id; NEW.task_requested_by:=NEW.originator_account_id; END IF;
 NEW.task_observed_at:=clock_timestamp();
 NEW.task_reason:=COALESCE(current_setting('infrapro.task_reason',true),'');
 IF TG_OP='INSERT' OR NEW.execution_generation<>OLD.execution_generation THEN
  NEW.task_requested_by:=COALESCE(NULLIF(current_setting('infrapro.task_actor',true),'')::uuid,NEW.task_requested_by);
 END IF;
 IF TG_OP='UPDATE' THEN NEW.control_revision:=OLD.control_revision+1; END IF;
 RETURN NEW; END; $$;
-- +goose StatementEnd
CREATE TRIGGER display_task_capture_before BEFORE INSERT OR UPDATE ON display_package_jobs FOR EACH ROW EXECUTE FUNCTION display_task_capture_before();
-- +goose StatementBegin
CREATE FUNCTION display_task_capture_after() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE snapshot jsonb;
BEGIN
 IF TG_OP='UPDATE' AND NEW.task_revision=OLD.task_revision THEN RETURN NEW; END IF;
 snapshot:=display_task_snapshot(NEW);
 IF snapshot IS NULL THEN RETURN NEW; END IF;
 IF NEW.state IN ('ready','failed','canceled') THEN
  INSERT INTO display_task_execution_evidence(job_id,generation,error_code,report,progress,input_hash,executor_ref,finished_at)
  VALUES(NEW.id,NEW.execution_generation,COALESCE(NEW.error_code,''),NEW.issues,'{}'::jsonb,(SELECT sha256 FROM display_uploads WHERE id=NEW.upload_id),COALESCE(NEW.task_requested_by::text,''),NEW.task_observed_at) ON CONFLICT DO NOTHING;
 END IF;
 INSERT INTO display_task_handoffs(job_id,task_revision,execution_generation,snapshot,fingerprint,occurred_at) VALUES(NEW.id,NEW.task_revision,NEW.execution_generation,snapshot,md5(snapshot::text),NEW.task_observed_at);
 RETURN NEW; END; $$;
-- +goose StatementEnd
CREATE TRIGGER display_task_capture_after AFTER INSERT OR UPDATE ON display_package_jobs FOR EACH ROW EXECUTE FUNCTION display_task_capture_after();
CREATE VIEW display_task_metadata AS SELECT id,task_revision,display_task_snapshot(j) AS snapshot FROM display_package_jobs j;
CREATE TABLE derived_task_groups(id uuid PRIMARY KEY,tenant_id uuid NOT NULL REFERENCES tenants(id),project_id uuid,actor_id uuid NOT NULL REFERENCES accounts(id),request_id uuid NOT NULL,snapshot jsonb NOT NULL,UNIQUE(tenant_id,actor_id,request_id),FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id));
CREATE TABLE derived_task_group_members(group_id uuid NOT NULL REFERENCES derived_task_groups(id),run_id uuid NOT NULL REFERENCES derived_runs(id),PRIMARY KEY(group_id,run_id));
-- Reconstruct old groups only when every returned member still has matching
-- immutable owner/scope/mode evidence. Missing or ambiguous receipts stay flat.
-- +goose StatementBegin
DO $$ DECLARE receipt record; root uuid; snapshot jsonb; BEGIN
 FOR receipt IN SELECT r.* FROM derived_operation_receipts r
 WHERE r.operation='channels.backfill' AND jsonb_typeof(r.result->'runs')='array'
 AND jsonb_array_length(r.result->'runs')>1
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(r.result->'runs') item LEFT JOIN derived_runs d ON d.id::text=item->>'id' AND d.tenant_id=r.tenant_id AND d.project_id IS NOT DISTINCT FROM r.project_id AND d.actor_id=r.actor_id AND d.mode='backfill' WHERE d.id IS NULL)
 AND jsonb_array_length(r.result->'runs')=(SELECT count(DISTINCT item->>'id') FROM jsonb_array_elements(r.result->'runs') item)
 LOOP
 root:=gen_random_uuid();
 snapshot:=jsonb_build_object('source_module','derived','source_kind','group','source_root_id',root,'type','derived.backfill_group',
 'scope',jsonb_build_object('kind',CASE WHEN receipt.project_id IS NULL THEN 'tenant' ELSE 'project' END,'tenant_id',receipt.tenant_id,'project_id',COALESCE(receipt.project_id::text,'')),
 'originator_account_id',receipt.actor_id,'origin_kind','person','requested_by',receipt.actor_id,'executor_kind','none','executor_ref','',
 'generation','1','source_revision','0','control_revision','1','state','queued','source_state','display_only','phase','queued','progress',jsonb_build_object('total',NULL),
 'input_ref',jsonb_build_object('request_id',receipt.request_id::text),'input_hash',receipt.payload_hash,'result_ref','{}'::jsonb,
 'created_at',receipt.created_at,'observed_at',receipt.created_at,'history_incomplete',true,'fence','0','attempt',0,'event','baseline');
 INSERT INTO derived_task_groups VALUES(root,receipt.tenant_id,receipt.project_id,receipt.actor_id,receipt.request_id,snapshot);
 INSERT INTO derived_task_group_members SELECT root,(item->>'id')::uuid FROM jsonb_array_elements(receipt.result->'runs') item;
 END LOOP;
END $$;
-- +goose StatementEnd
CREATE VIEW task_source_relations AS
 SELECT 'derived'::text source_module,'group'::text source_kind,m.group_id source_root_id,'member'::text relation_kind,'derived'::text child_module,'run'::text child_kind,m.run_id child_root_id FROM derived_task_group_members m
 UNION ALL SELECT 'derived','run',d.run_id,'dependency','derived','run',d.dependency_run_id FROM derived_run_dependencies d;
CREATE TABLE task_index_repairs(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),handoff_id uuid REFERENCES display_task_handoffs(id),repair_kind text NOT NULL DEFAULT 'replay_handoff' CHECK(repair_kind IN ('replay_handoff','restore_epoch')),reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),created_at timestamptz NOT NULL DEFAULT clock_timestamp());
-- Index maintenance uses safe views and journals, never source mutation grants.
-- +goose StatementBegin
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='infrapro_task_index') THEN CREATE ROLE infrapro_task_index NOLOGIN; END IF;
END $$;
-- +goose StatementEnd
GRANT SELECT ON task_protocol,task_records,task_executions,task_attempts,task_relations,task_events,task_source_states,task_worker_heartbeats,task_transaction_facts TO infrapro_task_index;
GRANT INSERT,UPDATE ON task_records,task_executions,task_attempts,task_source_states,task_worker_heartbeats TO infrapro_task_index;
GRANT INSERT ON task_relations,task_events TO infrapro_task_index;
GRANT UPDATE(mode,restore_epoch,updated_at) ON task_protocol TO infrapro_task_index;
GRANT SELECT ON monitoring_task_metadata,spatial_task_metadata,display_task_metadata TO infrapro_task_index;
GRANT SELECT,UPDATE ON display_task_handoffs TO infrapro_task_index;
GRANT DELETE,UPDATE(id) ON task_transaction_facts TO infrapro_task_index;
GRANT INSERT ON task_index_repairs TO infrapro_task_index;
GRANT SELECT ON derived_task_groups,task_source_relations TO infrapro_task_index;
GRANT SELECT ON goose_db_version TO infrapro_task_index;
CREATE VIEW task_index_backlog AS SELECT
 EXISTS(SELECT 1 FROM display_task_handoffs WHERE translated_at IS NULL) OR EXISTS(SELECT 1 FROM task_transaction_facts) AS pending;
CREATE VIEW task_retained_sources AS
 SELECT snapshot->>'source_module' AS source_module,snapshot->>'source_kind' AS source_kind,(snapshot->>'source_root_id')::uuid AS source_root_id FROM monitoring_task_metadata WHERE snapshot IS NOT NULL
 UNION ALL SELECT 'spatial','job',id FROM spatial_task_metadata
 UNION ALL SELECT 'display','job',id FROM display_task_metadata
 UNION ALL SELECT 'derived','group',id FROM derived_task_groups;
GRANT SELECT ON task_index_backlog,task_retained_sources TO infrapro_task_index;
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'task source evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
