-- +goose Up
-- Additive only: existing devices intentionally have no adoption record.
CREATE TABLE parsing_method_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), method_id text NOT NULL, method_version text NOT NULL,
 entry jsonb NOT NULL, manifest_digest text NOT NULL CHECK(manifest_digest ~ '^[a-f0-9]{64}$'),
 artifact_digest text NOT NULL CHECK(artifact_digest ~ '^[a-f0-9]{64}$'),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','enabled','deprecated','blocked')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), available boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(method_id,method_version)
);
CREATE TABLE parsing_catalog_syncs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), actor_id uuid NOT NULL REFERENCES accounts(id),
 state text NOT NULL CHECK(state IN ('succeeded','failed')), error_code text NOT NULL DEFAULT '',
 catalog jsonb, difference jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE parsing_configs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid REFERENCES tenants(id),
 owner_kind text NOT NULL CHECK(owner_kind IN ('product','device')), product_id uuid NOT NULL,
 product_version_id uuid NOT NULL, device_id uuid, name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 enabled boolean NOT NULL DEFAULT true, revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(id,product_version_id),
 FOREIGN KEY(product_id,product_version_id) REFERENCES device_product_versions(product_id,id),
 FOREIGN KEY(tenant_id,device_id,product_version_id) REFERENCES devices(tenant_id,id,product_version_id),
 CHECK((owner_kind='device' AND tenant_id IS NOT NULL AND device_id IS NOT NULL) OR (owner_kind='product' AND device_id IS NULL))
);
CREATE TABLE parsing_config_revisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), config_id uuid NOT NULL, product_version_id uuid NOT NULL,
 number bigint NOT NULL CHECK(number>0), method_id text NOT NULL, method_version text NOT NULL,
 snapshot jsonb NOT NULL, content_digest text NOT NULL CHECK(content_digest ~ '^[a-f0-9]{64}$'),
 source_revision_id uuid REFERENCES parsing_config_revisions(id), reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 2000),
 created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(config_id,number), UNIQUE(product_version_id,id), UNIQUE(config_id,id),
 FOREIGN KEY(config_id,product_version_id) REFERENCES parsing_configs(id,product_version_id),
 FOREIGN KEY(method_id,method_version) REFERENCES parsing_method_versions(method_id,method_version)
);
CREATE TABLE parsing_product_defaults (
 product_version_id uuid PRIMARY KEY REFERENCES device_product_versions(id), config_revision_id uuid,
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0), updated_by uuid REFERENCES accounts(id), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(product_version_id,config_revision_id) REFERENCES parsing_config_revisions(product_version_id,id)
);
CREATE TABLE parsing_device_heads (
 tenant_id uuid NOT NULL, device_id uuid PRIMARY KEY, revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 FOREIGN KEY(tenant_id,device_id) REFERENCES devices(tenant_id,id)
);
CREATE TABLE parsing_device_adoptions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, device_id uuid NOT NULL, product_version_id uuid NOT NULL,
 mode text NOT NULL CHECK(mode IN ('unconfigured','inherit_snapshot','override')), config_revision_id uuid,
 source_default_revision bigint, snapshot jsonb, revision bigint NOT NULL CHECK(revision>0),
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(), ended_at timestamptz,
 actor_id uuid NOT NULL REFERENCES accounts(id), reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 2000),
 UNIQUE(tenant_id,device_id,id), UNIQUE(device_id,revision),
 FOREIGN KEY(tenant_id,device_id,product_version_id) REFERENCES devices(tenant_id,id,product_version_id),
 FOREIGN KEY(product_version_id,config_revision_id) REFERENCES parsing_config_revisions(product_version_id,id),
 CHECK((mode='unconfigured' AND config_revision_id IS NULL AND snapshot IS NULL) OR (mode<>'unconfigured' AND config_revision_id IS NOT NULL AND snapshot IS NOT NULL)),
 CHECK(ended_at IS NULL OR ended_at>started_at),
 EXCLUDE USING gist(device_id public.gist_uuid_ops WITH =,tstzrange(started_at,ended_at,'[)') WITH &&)
);
CREATE UNIQUE INDEX parsing_device_current ON parsing_device_adoptions(device_id) WHERE ended_at IS NULL;
CREATE TABLE parsing_objects (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), scope_key text NOT NULL, object_key text NOT NULL UNIQUE CHECK(object_key LIKE 'parsing/v1/%' AND position('..' IN object_key)=0),
 state text NOT NULL DEFAULT 'intent' CHECK(state IN ('intent','available','delete_pending','deleted')),
 size_bytes bigint NOT NULL CHECK(size_bytes>=0), sha256 text CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(scope_key,id)
);
CREATE TABLE parsing_samples (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), scope_key text NOT NULL, tenant_id uuid REFERENCES tenants(id),
 object_id uuid NOT NULL, representation_object_id uuid NOT NULL,
 representation text NOT NULL CHECK(representation IN ('hex','utf8','binary')), source text NOT NULL CHECK(source IN ('synthetic','uploaded')),
 state text NOT NULL DEFAULT 'uploading' CHECK(state IN ('uploading','ready','expired')), sha256 text, byte_length integer,
 received_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), created_by uuid NOT NULL REFERENCES accounts(id),
 UNIQUE(scope_key,id), FOREIGN KEY(scope_key,object_id) REFERENCES parsing_objects(scope_key,id),
 FOREIGN KEY(scope_key,representation_object_id) REFERENCES parsing_objects(scope_key,id),
 CHECK(scope_key=COALESCE('tenant:'||tenant_id::text,'platform')),
 CHECK(state<>'ready' OR (sha256 ~ '^[a-f0-9]{64}$' AND byte_length BETWEEN 1 AND 4194304))
);
CREATE TABLE parsing_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), scope_key text NOT NULL, tenant_id uuid REFERENCES tenants(id),
 kind text NOT NULL CHECK(kind IN ('method_test','product_test','device_test')), sample_id uuid NOT NULL,
 method_id text NOT NULL, method_version text NOT NULL, config_revision_id uuid REFERENCES parsing_config_revisions(id),
 product_version_id uuid REFERENCES device_product_versions(id), device_id uuid, adoption_id uuid,
 input_snapshot jsonb NOT NULL, compared_to_run_id uuid, current_generation bigint NOT NULL DEFAULT 1 CHECK(current_generation>0),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), control_revision bigint NOT NULL DEFAULT 1 CHECK(control_revision>0),
 task_snapshot jsonb, created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(scope_key,id), FOREIGN KEY(scope_key,sample_id) REFERENCES parsing_samples(scope_key,id),
 FOREIGN KEY(scope_key,compared_to_run_id) REFERENCES parsing_runs(scope_key,id),
 FOREIGN KEY(method_id,method_version) REFERENCES parsing_method_versions(method_id,method_version),
 FOREIGN KEY(tenant_id,device_id) REFERENCES devices(tenant_id,id),
 FOREIGN KEY(tenant_id,device_id,adoption_id) REFERENCES parsing_device_adoptions(tenant_id,device_id,id),
 CHECK(scope_key=COALESCE('tenant:'||tenant_id::text,'platform')),
 CHECK((kind='device_test')=(device_id IS NOT NULL)), CHECK(kind<>'product_test' OR config_revision_id IS NOT NULL)
);
CREATE TABLE parsing_run_executions (
 run_id uuid NOT NULL REFERENCES parsing_runs(id), generation bigint NOT NULL CHECK(generation>0),
 actor_id uuid NOT NULL REFERENCES accounts(id), state text NOT NULL CHECK(state IN ('queued','running','succeeded','failed','canceled')),
 fence bigint NOT NULL DEFAULT 0, lease_until timestamptz, result_object_id uuid REFERENCES parsing_objects(id),
 response_status text, mapping jsonb NOT NULL DEFAULT '[]', error_code text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), started_at timestamptz, finished_at timestamptz,
 PRIMARY KEY(run_id,generation), CHECK((state IN ('succeeded','failed','canceled'))=(finished_at IS NOT NULL)),
 CHECK(state<>'succeeded' OR result_object_id IS NOT NULL)
);
CREATE INDEX parsing_queue ON parsing_run_executions(state,created_at) WHERE state IN ('queued','running');
CREATE TABLE parsing_run_attempts (
 execution_id uuid PRIMARY KEY, run_id uuid NOT NULL, generation bigint NOT NULL, attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 3),
 fence bigint NOT NULL, state text NOT NULL CHECK(state IN ('running','succeeded','failed')), error_code text NOT NULL DEFAULT '',
 response_object_id uuid REFERENCES parsing_objects(id), started_at timestamptz NOT NULL DEFAULT clock_timestamp(), finished_at timestamptz,
 FOREIGN KEY(run_id,generation) REFERENCES parsing_run_executions(run_id,generation), UNIQUE(run_id,generation,fence,attempt)
);
CREATE TABLE parsing_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), scope_key text NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id),
 operation text NOT NULL, target_id uuid NOT NULL, request_id uuid NOT NULL, reason text NOT NULL,
 before_snapshot jsonb, after_snapshot jsonb, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE parsing_operation_receipts (
 scope_key text NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id), operation text NOT NULL, request_id uuid NOT NULL,
 payload_hash text NOT NULL, result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(scope_key,actor_id,operation,request_id)
);
-- +goose StatementBegin
CREATE FUNCTION parsing_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'parsing_immutable_evidence'; END $$;
CREATE TRIGGER parsing_revision_immutable BEFORE UPDATE OR DELETE ON parsing_config_revisions FOR EACH ROW EXECUTE FUNCTION parsing_immutable();
CREATE FUNCTION parsing_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE c parsing_configs; owner uuid;
BEGIN
 IF TG_TABLE_NAME='parsing_method_versions' THEN
  IF OLD.entry IS DISTINCT FROM NEW.entry OR OLD.manifest_digest<>NEW.manifest_digest OR OLD.artifact_digest<>NEW.artifact_digest OR OLD.method_id<>NEW.method_id OR OLD.method_version<>NEW.method_version THEN RAISE EXCEPTION 'parsing_version_immutable'; END IF;
 ELSIF TG_TABLE_NAME='parsing_configs' THEN
  IF TG_OP='UPDATE' AND (to_jsonb(NEW)-ARRAY['name','enabled','revision','updated_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['name','enabled','revision','updated_at']) THEN RAISE EXCEPTION 'parsing_owner_immutable'; END IF;
  SELECT owner_tenant_id INTO owner FROM device_products WHERE id=NEW.product_id;
  IF NEW.owner_kind='product' AND NEW.tenant_id IS DISTINCT FROM owner OR NEW.owner_kind='device' AND owner IS NOT NULL AND owner<>NEW.tenant_id THEN RAISE EXCEPTION 'parsing_owner_mismatch'; END IF;
 ELSIF TG_TABLE_NAME='parsing_product_defaults' THEN
 IF NEW.config_revision_id IS NOT NULL THEN
  SELECT pc.* INTO c FROM parsing_configs pc JOIN parsing_config_revisions r ON r.config_id=pc.id WHERE r.id=NEW.config_revision_id;
  IF c.owner_kind<>'product' OR c.product_version_id<>NEW.product_version_id THEN RAISE EXCEPTION 'parsing_default_mismatch'; END IF;
 END IF;
 ELSIF TG_TABLE_NAME='parsing_device_adoptions' THEN
 IF TG_OP='UPDATE' AND (OLD.ended_at IS NOT NULL OR (to_jsonb(NEW)-'ended_at') IS DISTINCT FROM (to_jsonb(OLD)-'ended_at')) THEN RAISE EXCEPTION 'parsing_adoption_immutable'; END IF;
 IF NEW.config_revision_id IS NOT NULL THEN
  SELECT pc.* INTO c FROM parsing_configs pc JOIN parsing_config_revisions r ON r.config_id=pc.id WHERE r.id=NEW.config_revision_id;
  IF c.tenant_id IS NOT NULL AND c.tenant_id<>NEW.tenant_id OR c.device_id IS NOT NULL AND c.device_id<>NEW.device_id THEN RAISE EXCEPTION 'parsing_adoption_owner_mismatch'; END IF;
 END IF;
 ELSIF TG_TABLE_NAME='parsing_run_executions' THEN
  IF OLD.state IN ('succeeded','failed','canceled') AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'parsing_terminal_immutable'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER parsing_method_guard BEFORE UPDATE ON parsing_method_versions FOR EACH ROW EXECUTE FUNCTION parsing_guard();
CREATE TRIGGER parsing_config_guard BEFORE INSERT OR UPDATE ON parsing_configs FOR EACH ROW EXECUTE FUNCTION parsing_guard();
CREATE TRIGGER parsing_default_guard BEFORE INSERT OR UPDATE ON parsing_product_defaults FOR EACH ROW EXECUTE FUNCTION parsing_guard();
CREATE TRIGGER parsing_adoption_guard BEFORE INSERT OR UPDATE ON parsing_device_adoptions FOR EACH ROW EXECUTE FUNCTION parsing_guard();
CREATE TRIGGER parsing_execution_guard BEFORE UPDATE ON parsing_run_executions FOR EACH ROW EXECUTE FUNCTION parsing_guard();
CREATE FUNCTION parsing_frozen_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE oldj jsonb; newj jsonb; scope text; obj uuid; cfg parsing_configs; rev parsing_config_revisions;
BEGIN
 newj:=to_jsonb(NEW);
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'parsing_evidence_retained'; END IF;
 IF TG_OP='UPDATE' THEN
  oldj:=to_jsonb(OLD);
  CASE TG_TABLE_NAME
   WHEN 'parsing_runs' THEN IF (oldj-ARRAY['revision','control_revision','current_generation','task_snapshot']) IS DISTINCT FROM (newj-ARRAY['revision','control_revision','current_generation','task_snapshot']) THEN RAISE EXCEPTION 'parsing_input_immutable'; END IF;
   WHEN 'parsing_samples' THEN IF (oldj-ARRAY['state','sha256','byte_length']) IS DISTINCT FROM (newj-ARRAY['state','sha256','byte_length']) OR OLD.state<>'uploading' AND (oldj-'state') IS DISTINCT FROM (newj-'state') THEN RAISE EXCEPTION 'parsing_sample_immutable'; END IF;
   WHEN 'parsing_objects' THEN IF (oldj-ARRAY['state','sha256','size_bytes']) IS DISTINCT FROM (newj-ARRAY['state','sha256','size_bytes']) OR OLD.state<>'intent' AND (oldj-'state') IS DISTINCT FROM (newj-'state') THEN RAISE EXCEPTION 'parsing_object_immutable'; END IF;
   ELSE NULL;
  END CASE;
 END IF;
 IF TG_TABLE_NAME='parsing_runs' THEN
  IF NEW.config_revision_id IS NOT NULL THEN
   SELECT * INTO rev FROM parsing_config_revisions WHERE id=NEW.config_revision_id;
   SELECT * INTO cfg FROM parsing_configs WHERE id=rev.config_id;
   IF cfg.tenant_id IS NOT NULL AND cfg.tenant_id IS DISTINCT FROM NEW.tenant_id OR cfg.device_id IS NOT NULL AND cfg.device_id IS DISTINCT FROM NEW.device_id OR cfg.product_version_id IS DISTINCT FROM NEW.product_version_id OR rev.method_id<>NEW.method_id OR rev.method_version<>NEW.method_version THEN RAISE EXCEPTION 'parsing_run_owner_mismatch'; END IF;
  END IF;
  IF NEW.kind='method_test' AND (NEW.config_revision_id IS NOT NULL OR NEW.adoption_id IS NOT NULL OR NEW.product_version_id IS NOT NULL) THEN RAISE EXCEPTION 'parsing_run_context_mismatch'; END IF;
 ELSIF TG_TABLE_NAME IN ('parsing_run_executions','parsing_run_attempts') THEN
  SELECT scope_key INTO scope FROM parsing_runs WHERE id=NEW.run_id;
  obj:=COALESCE(newj->>'result_object_id',newj->>'response_object_id')::uuid;
  IF obj IS NOT NULL AND NOT EXISTS(SELECT 1 FROM parsing_objects WHERE id=obj AND scope_key=scope AND state='available') THEN RAISE EXCEPTION 'parsing_result_object_mismatch'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER parsing_run_frozen BEFORE INSERT OR UPDATE OR DELETE ON parsing_runs FOR EACH ROW EXECUTE FUNCTION parsing_frozen_guard();
CREATE TRIGGER parsing_sample_frozen BEFORE UPDATE OR DELETE ON parsing_samples FOR EACH ROW EXECUTE FUNCTION parsing_frozen_guard();
CREATE TRIGGER parsing_object_frozen BEFORE UPDATE OR DELETE ON parsing_objects FOR EACH ROW EXECUTE FUNCTION parsing_frozen_guard();
CREATE TRIGGER parsing_result_scope BEFORE INSERT OR UPDATE ON parsing_run_executions FOR EACH ROW EXECUTE FUNCTION parsing_frozen_guard();
CREATE TRIGGER parsing_attempt_scope BEFORE INSERT OR UPDATE ON parsing_run_attempts FOR EACH ROW EXECUTE FUNCTION parsing_frozen_guard();
CREATE TRIGGER parsing_adoption_retained BEFORE DELETE ON parsing_device_adoptions FOR EACH ROW EXECUTE FUNCTION parsing_immutable();
CREATE TRIGGER parsing_change_retained BEFORE UPDATE OR DELETE ON parsing_changes FOR EACH ROW EXECUTE FUNCTION parsing_immutable();
CREATE TRIGGER parsing_receipt_retained BEFORE UPDATE OR DELETE ON parsing_operation_receipts FOR EACH ROW EXECUTE FUNCTION parsing_immutable();
-- +goose StatementEnd
ALTER TABLE task_records DROP CONSTRAINT task_records_scope_kind_check;
ALTER TABLE task_records ADD CONSTRAINT task_records_scope_kind_check CHECK(
 (scope_kind IN ('tenant','project') AND tenant_id IS NOT NULL AND (source_module<>'parsing' OR scope_kind='tenant')) OR
 (scope_kind='platform' AND tenant_id IS NULL AND project_id IS NULL AND
 ((source_module='audit' AND source_kind='export' AND task_type='audit.export') OR (source_module='parsing' AND source_kind='run' AND task_type='parsing.test'))));
INSERT INTO task_source_states(source_module,source_kind,restore_epoch,backfill_complete) SELECT 'parsing','run',restore_epoch,true FROM task_protocol WHERE id=1;
INSERT INTO audit_source_states(source_module,source_kind,complete) VALUES('parsing','run',true);
CREATE VIEW parsing_task_metadata AS SELECT id,revision,task_snapshot AS snapshot FROM parsing_runs WHERE task_snapshot IS NOT NULL;
CREATE OR REPLACE VIEW task_retained_sources AS
 SELECT snapshot->>'source_module' AS source_module,snapshot->>'source_kind' AS source_kind,(snapshot->>'source_root_id')::uuid AS source_root_id FROM monitoring_task_metadata WHERE snapshot IS NOT NULL
 UNION ALL SELECT 'spatial','job',id FROM spatial_task_metadata
 UNION ALL SELECT 'display','job',id FROM display_task_metadata
 UNION ALL SELECT 'derived','group',id FROM derived_task_groups
 UNION ALL SELECT 'files','job',id FROM file_task_metadata
 UNION ALL SELECT 'audit','export',id FROM audit_exports
 UNION ALL SELECT 'parsing','run',id FROM parsing_runs;
GRANT SELECT ON parsing_task_metadata,task_retained_sources TO infrapro_task_index;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Parsing evidence is retained; restore a consistent backup'; END $$;
-- +goose StatementEnd
