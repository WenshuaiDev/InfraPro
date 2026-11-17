-- +goose Up
-- Additive production ingestion. No existing device or parser adoption is changed.
CREATE TABLE parsing_identification_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), method_id text NOT NULL, method_version text NOT NULL,
 entry jsonb NOT NULL, digest text NOT NULL, state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','enabled','deprecated','blocked')),
 revision bigint NOT NULL DEFAULT 1, available boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(method_id,method_version)
);
CREATE TABLE parsing_ingress_protocol_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), protocol_id text NOT NULL, protocol_version text NOT NULL,
 identification_id text NOT NULL, identification_version text NOT NULL, entry jsonb NOT NULL, digest text NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','enabled','deprecated','blocked')), revision bigint NOT NULL DEFAULT 1,
 available boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(protocol_id,protocol_version),
 FOREIGN KEY(identification_id,identification_version) REFERENCES parsing_identification_versions(method_id,method_version)
);
CREATE TABLE parsing_ingress_compatibilities (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), compatibility_id text NOT NULL, compatibility_revision bigint NOT NULL,
 protocol_id text NOT NULL, protocol_version text NOT NULL, method_id text NOT NULL, method_version text NOT NULL,
 entry jsonb NOT NULL, digest text NOT NULL, state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','enabled','deprecated','blocked')),
 revision bigint NOT NULL DEFAULT 1, available boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(compatibility_id,compatibility_revision),
 FOREIGN KEY(protocol_id,protocol_version) REFERENCES parsing_ingress_protocol_versions(protocol_id,protocol_version),
 FOREIGN KEY(method_id,method_version) REFERENCES parsing_method_versions(method_id,method_version)
);
CREATE TABLE parsing_ingestion_catalog_syncs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), actor_id uuid NOT NULL REFERENCES accounts(id), state text NOT NULL,
 error_code text NOT NULL DEFAULT '', catalog jsonb, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ingestion_relays (
 id uuid PRIMARY KEY, name text NOT NULL, configuration_ref text NOT NULL UNIQUE,
 revision bigint NOT NULL DEFAULT 1, enabled boolean NOT NULL DEFAULT true,
 status jsonb, observed_at timestamptz, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ingestion_sources (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), namespace_id uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
 tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid NOT NULL, manufacturer_id uuid NOT NULL REFERENCES device_manufacturers(id),
 manufacturer_scope text NOT NULL CHECK(manufacturer_scope IN ('public','tenant')), relay_id uuid NOT NULL REFERENCES ingestion_relays(id),
 name text NOT NULL, revision bigint NOT NULL DEFAULT 0, state text NOT NULL DEFAULT 'draft',
 applied_generation bigint NOT NULL DEFAULT 0, applied_revision bigint, application jsonb,
 planning_paused boolean NOT NULL DEFAULT true, blocked boolean NOT NULL DEFAULT false, error_code text NOT NULL DEFAULT '',
 synthetic boolean NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,project_id,id), FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
CREATE TABLE ingestion_source_revisions (
 source_id uuid NOT NULL REFERENCES ingestion_sources(id), revision bigint NOT NULL,
 snapshot jsonb NOT NULL, digest text NOT NULL, protocol_id text NOT NULL, protocol_version text NOT NULL,
 command_id uuid NOT NULL UNIQUE, expected_generation bigint NOT NULL, segregated boolean NOT NULL CHECK(segregated),
 created_by uuid NOT NULL REFERENCES accounts(id), reason text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(source_id,revision), FOREIGN KEY(protocol_id,protocol_version) REFERENCES parsing_ingress_protocol_versions(protocol_id,protocol_version)
);
CREATE TABLE ingestion_source_confirmations (
 source_id uuid NOT NULL, revision bigint NOT NULL, tenant_id uuid NOT NULL, digest text NOT NULL,
 actor_id uuid NOT NULL REFERENCES accounts(id), reason text NOT NULL, inherited_revision bigint, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(source_id,revision), FOREIGN KEY(source_id,revision) REFERENCES ingestion_source_revisions(source_id,revision),
 FOREIGN KEY(tenant_id,source_id) REFERENCES ingestion_sources(tenant_id,id)
);
CREATE TABLE ingestion_epochs (
 relay_id uuid NOT NULL REFERENCES ingestion_relays(id), epoch uuid NOT NULL,
 confirmed boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(relay_id,epoch)
);
CREATE TABLE ingestion_terminals (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, source_id uuid NOT NULL, terminal_key text NOT NULL,
 name text NOT NULL, mode text NOT NULL CHECK(mode IN ('single','multiple')), revision bigint NOT NULL DEFAULT 1,
 enabled boolean NOT NULL DEFAULT true, demultiplexer jsonb, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(source_id,terminal_key), UNIQUE(tenant_id,id), UNIQUE(tenant_id,source_id,id),
 FOREIGN KEY(tenant_id,source_id) REFERENCES ingestion_sources(tenant_id,id),
 CHECK((mode='multiple')=(demultiplexer IS NOT NULL)), CHECK(length(terminal_key)=11)
);
CREATE TABLE ingestion_terminal_revisions (
 terminal_id uuid NOT NULL REFERENCES ingestion_terminals(id), revision bigint NOT NULL, snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(terminal_id,revision)
);
CREATE TABLE ingestion_change_intents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, source_id uuid NOT NULL, terminal_id uuid NOT NULL,
 command_id uuid NOT NULL UNIQUE, state text NOT NULL DEFAULT 'pending', input jsonb NOT NULL, watermark jsonb,
 actor_id uuid NOT NULL REFERENCES accounts(id), reason text NOT NULL, error_code text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,source_id,terminal_id) REFERENCES ingestion_terminals(tenant_id,source_id,id)
);
CREATE UNIQUE INDEX ingestion_one_source_change ON ingestion_change_intents(source_id) WHERE state='pending';
CREATE TABLE ingestion_device_associations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, source_id uuid NOT NULL, terminal_id uuid NOT NULL,
 terminal_revision bigint NOT NULL, child_key text NOT NULL, device_id uuid NOT NULL, epoch uuid NOT NULL,
 start_sequence bigint NOT NULL CHECK(start_sequence>0), end_sequence bigint, enabled boolean NOT NULL DEFAULT true,
 input_view text NOT NULL CHECK(input_view IN ('complete_frame','json_reading')), demultiplexer jsonb,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(tenant_id,id),
 FOREIGN KEY(tenant_id,source_id,terminal_id) REFERENCES ingestion_terminals(tenant_id,source_id,id),
 FOREIGN KEY(terminal_id,terminal_revision) REFERENCES ingestion_terminal_revisions(terminal_id,revision),
 FOREIGN KEY(tenant_id,device_id) REFERENCES devices(tenant_id,id),
 CHECK(end_sequence IS NULL OR end_sequence>=start_sequence),
 EXCLUDE USING gist(terminal_id public.gist_uuid_ops WITH =,child_key public.gist_text_ops WITH =,epoch public.gist_uuid_ops WITH =,int8range(start_sequence,end_sequence,'[)') WITH &&)
);
CREATE INDEX ingestion_device_entries ON ingestion_device_associations(tenant_id,device_id) WHERE enabled AND end_sequence IS NULL;
CREATE TABLE ingestion_execution_grants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, source_id uuid NOT NULL, terminal_id uuid,
 identify boolean NOT NULL, measure boolean NOT NULL, enabled boolean NOT NULL, generation bigint NOT NULL DEFAULT 1,
 budget integer NOT NULL DEFAULT 10000 CHECK(budget BETWEEN 1 AND 100000),
 created_by uuid NOT NULL REFERENCES accounts(id), reason text NOT NULL, updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,source_id) REFERENCES ingestion_sources(tenant_id,id),
 FOREIGN KEY(tenant_id,source_id,terminal_id) REFERENCES ingestion_terminals(tenant_id,source_id,id)
);
CREATE UNIQUE INDEX ingestion_source_grant ON ingestion_execution_grants(source_id) WHERE terminal_id IS NULL;
CREATE UNIQUE INDEX ingestion_terminal_grant ON ingestion_execution_grants(terminal_id) WHERE terminal_id IS NOT NULL;
CREATE TABLE ingestion_execution_grant_history (
 grant_id uuid NOT NULL REFERENCES ingestion_execution_grants(id), generation bigint NOT NULL, snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(grant_id,generation)
);
CREATE TABLE ingestion_objects (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), object_key text NOT NULL UNIQUE CHECK(object_key LIKE 'ingestion/v1/%' AND position('..' IN object_key)=0),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'), byte_length bigint NOT NULL CHECK(byte_length>0),
 state text NOT NULL DEFAULT 'intent', created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ingestion_storage_state(id integer PRIMARY KEY CHECK(id=1),used_bytes bigint NOT NULL DEFAULT 0 CHECK(used_bytes>=0));
INSERT INTO ingestion_storage_state(id)VALUES(1);
-- +goose StatementBegin
CREATE FUNCTION ingestion_storage_counter() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' THEN UPDATE ingestion_storage_state SET used_bytes=used_bytes+NEW.byte_length WHERE id=1;
 ELSIF NEW.state='deleted' AND OLD.state<>'deleted' THEN UPDATE ingestion_storage_state SET used_bytes=used_bytes-NEW.byte_length WHERE id=1;
 END IF; RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER ingestion_storage_counter AFTER INSERT OR UPDATE OF state ON ingestion_objects FOR EACH ROW EXECUTE FUNCTION ingestion_storage_counter();
CREATE TABLE ingestion_integrity_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),relay_id uuid NOT NULL,epoch uuid NOT NULL,receipt_id uuid NOT NULL,
 source_id uuid NOT NULL,source_revision bigint NOT NULL,error_code text NOT NULL,observed_digest text NOT NULL,
 occurrences bigint NOT NULL DEFAULT 1,first_seen_at timestamptz NOT NULL DEFAULT clock_timestamp(),last_seen_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(relay_id,epoch,receipt_id)
);
CREATE TABLE ingestion_receive_intents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), relay_id uuid NOT NULL REFERENCES ingestion_relays(id), epoch uuid NOT NULL, receipt_id uuid NOT NULL,
 envelope_digest text NOT NULL, object_id uuid NOT NULL REFERENCES ingestion_objects(id), state text NOT NULL DEFAULT 'pending',
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(relay_id,epoch,receipt_id)
);
CREATE TABLE ingestion_receipts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), relay_id uuid NOT NULL REFERENCES ingestion_relays(id), epoch uuid NOT NULL, receipt_id uuid NOT NULL,
 source_id uuid NOT NULL, source_revision bigint NOT NULL, source_sequence bigint NOT NULL,
 tenant_id uuid, project_id uuid, envelope jsonb NOT NULL, envelope_digest text NOT NULL, object_id uuid NOT NULL REFERENCES ingestion_objects(id),
 state text NOT NULL DEFAULT 'stored', error_code text NOT NULL DEFAULT '', received_at timestamptz NOT NULL, stored_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 current_generation bigint NOT NULL DEFAULT 1, UNIQUE(relay_id,epoch,receipt_id), UNIQUE(id,tenant_id)
);
CREATE INDEX ingestion_receipt_source ON ingestion_receipts(source_id,source_sequence);
CREATE INDEX ingestion_receipt_pending ON ingestion_receipts(state,stored_at);
-- Bound fresh claims per source; an empty source must not scan another source's backlog.
CREATE INDEX ingestion_fresh_source_cover ON ingestion_receipts(source_id,stored_at) INCLUDE(id,tenant_id,current_generation) WHERE state='stored' AND tenant_id IS NOT NULL;
CREATE TABLE ingestion_ack_outbox (
 receipt_id uuid PRIMARY KEY REFERENCES ingestion_receipts(id), payload jsonb NOT NULL, state text NOT NULL DEFAULT 'pending',
 attempts integer NOT NULL DEFAULT 0, available_at timestamptz NOT NULL DEFAULT clock_timestamp(), error_code text NOT NULL DEFAULT ''
);
CREATE TABLE ingestion_identity_decisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), receipt_id uuid NOT NULL REFERENCES ingestion_receipts(id), generation bigint NOT NULL,
 method jsonb NOT NULL, state text NOT NULL DEFAULT 'queued', error_code text NOT NULL DEFAULT '',
 fence bigint NOT NULL DEFAULT 0, attempts integer NOT NULL DEFAULT 0, lease_until timestamptz, available_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 response_object_id uuid REFERENCES ingestion_objects(id), response jsonb, grant_id uuid REFERENCES ingestion_execution_grants(id), grant_generation bigint,
 recovery_id uuid,recovery_generation bigint, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(receipt_id,generation)
);
CREATE INDEX ingestion_identity_queue ON ingestion_identity_decisions(state,available_at,lease_until,receipt_id);
CREATE TABLE ingestion_units (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), receipt_id uuid NOT NULL REFERENCES ingestion_receipts(id), decision_id uuid NOT NULL REFERENCES ingestion_identity_decisions(id),
 unit_key text NOT NULL, terminal_key text NOT NULL, child_key text NOT NULL DEFAULT '',
 view jsonb NOT NULL, message_kind text NOT NULL, tenant_id uuid, device_id uuid, terminal_id uuid, association_id uuid,
 state text NOT NULL DEFAULT 'identified', error_code text NOT NULL DEFAULT '', assignment jsonb,
 parent_unit_id uuid REFERENCES ingestion_units(id), fence bigint NOT NULL DEFAULT 0, attempts integer NOT NULL DEFAULT 0, lease_until timestamptz, available_at timestamptz NOT NULL DEFAULT clock_timestamp(), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(decision_id,unit_key), UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,device_id) REFERENCES devices(tenant_id,id),
 FOREIGN KEY(tenant_id,terminal_id) REFERENCES ingestion_terminals(tenant_id,id), FOREIGN KEY(tenant_id,association_id) REFERENCES ingestion_device_associations(tenant_id,id)
);
CREATE INDEX ingestion_unit_queue ON ingestion_units(state,created_at,available_at);
CREATE INDEX ingestion_unit_query ON ingestion_units(tenant_id,device_id,created_at,id);
CREATE INDEX ingestion_receipt_tenant_pending ON ingestion_receipts(tenant_id,state,stored_at);
CREATE TABLE ingestion_unit_assignments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), unit_id uuid NOT NULL REFERENCES ingestion_units(id), snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ingestion_parse_plans (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, unit_id uuid NOT NULL, decision_id uuid NOT NULL REFERENCES ingestion_identity_decisions(id),
 device_id uuid NOT NULL, adoption_id uuid NOT NULL, snapshot jsonb NOT NULL,
 state text NOT NULL DEFAULT 'queued', error_code text NOT NULL DEFAULT '', fence bigint NOT NULL DEFAULT 0,
 attempts integer NOT NULL DEFAULT 0, lease_until timestamptz, available_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), finished_at timestamptz,
 UNIQUE(unit_id,decision_id), FOREIGN KEY(tenant_id,unit_id) REFERENCES ingestion_units(tenant_id,id),
 FOREIGN KEY(tenant_id,device_id,adoption_id) REFERENCES parsing_device_adoptions(tenant_id,device_id,id)
);
CREATE INDEX ingestion_parse_queue ON ingestion_parse_plans(state,available_at,tenant_id);
-- Queue claims and grant accounting must not deserialize every pending plan snapshot.
CREATE INDEX ingestion_parse_source_ready ON ingestion_parse_plans(((snapshot->'source'->>'ingress_id')::uuid),available_at) INCLUDE(id,tenant_id,state,lease_until) WHERE state IN ('queued','retry_wait','parsing');
CREATE INDEX ingestion_parse_grant_queue ON ingestion_parse_plans(tenant_id,((snapshot->'execution_grant')->>'id')) WHERE state IN ('queued','parsing','retry_wait');
CREATE TABLE ingestion_attempts (
 execution_id uuid PRIMARY KEY, stage text NOT NULL CHECK(stage IN ('identify','parse','demultiplex')), target_id uuid NOT NULL,
 fence bigint NOT NULL, attempt integer NOT NULL CHECK(attempt BETWEEN 1 AND 5), response_object_id uuid REFERENCES ingestion_objects(id),
 state text NOT NULL DEFAULT 'running', error_code text NOT NULL DEFAULT '',
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(), finished_at timestamptz
);
CREATE TABLE ingestion_demultiplex_decisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), unit_id uuid NOT NULL REFERENCES ingestion_units(id), execution_id uuid NOT NULL REFERENCES ingestion_attempts(execution_id),
 method jsonb NOT NULL, response_object_id uuid NOT NULL REFERENCES ingestion_objects(id), response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ingestion_results (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), plan_id uuid NOT NULL UNIQUE REFERENCES ingestion_parse_plans(id),
 execution_id uuid NOT NULL REFERENCES ingestion_attempts(execution_id), response_object_id uuid NOT NULL REFERENCES ingestion_objects(id),
 status text NOT NULL, mapping jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ingestion_recovery_jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid REFERENCES tenants(id), actor_id uuid NOT NULL REFERENCES accounts(id),
 kind text NOT NULL, input jsonb NOT NULL, state text NOT NULL DEFAULT 'queued', revision bigint NOT NULL DEFAULT 1,
 generation bigint NOT NULL DEFAULT 1, items jsonb NOT NULL DEFAULT '[]', task_snapshot jsonb, reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ingestion_recovery_generations(job_id uuid NOT NULL REFERENCES ingestion_recovery_jobs(id),generation bigint NOT NULL,snapshot jsonb NOT NULL,PRIMARY KEY(job_id,generation));
CREATE TABLE ingestion_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), scope_key text NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id),
 operation text NOT NULL, target_id uuid NOT NULL, request_id uuid NOT NULL, reason text NOT NULL,
 before_snapshot jsonb, after_snapshot jsonb, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE ingestion_operation_receipts (
 scope_key text NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id), operation text NOT NULL, request_id uuid NOT NULL,
 payload_hash text NOT NULL, result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(scope_key,actor_id,operation,request_id)
);
-- +goose StatementBegin
CREATE FUNCTION ingestion_frozen_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE mutable text[];
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'ingestion_evidence_retained'; END IF;
 CASE TG_TABLE_NAME
 WHEN 'ingestion_sources' THEN mutable:=ARRAY['name','revision','state','applied_generation','applied_revision','application','planning_paused','blocked','error_code'];
 WHEN 'ingestion_receipts' THEN mutable:=ARRAY['state','error_code','current_generation','tenant_id','project_id'];
 WHEN 'ingestion_units' THEN mutable:=ARRAY['state','error_code','tenant_id','device_id','terminal_id','association_id','assignment','fence','attempts','lease_until','available_at'];
 WHEN 'ingestion_identity_decisions' THEN
 IF OLD.state IN ('identified','rejected','blocked') AND to_jsonb(NEW) IS DISTINCT FROM to_jsonb(OLD) THEN RAISE EXCEPTION 'ingestion_decision_final'; END IF;
 mutable:=ARRAY['state','error_code','fence','attempts','lease_until','available_at','response_object_id','response'];
 WHEN 'ingestion_recovery_jobs' THEN mutable:=ARRAY['state','actor_id','generation','revision','items','task_snapshot','updated_at'];
 WHEN 'ingestion_parse_plans' THEN mutable:=ARRAY['state','error_code','fence','attempts','lease_until','available_at','finished_at'];
 WHEN 'ingestion_device_associations' THEN mutable:=ARRAY['end_sequence','enabled'];
 WHEN 'ingestion_objects' THEN mutable:=ARRAY['state'];
 WHEN 'parsing_identification_versions','parsing_ingress_protocol_versions','parsing_ingress_compatibilities' THEN mutable:=ARRAY['state','revision','available'];
 ELSE mutable:=ARRAY[]::text[];
 END CASE;
 IF (to_jsonb(NEW)-mutable) IS DISTINCT FROM (to_jsonb(OLD)-mutable) THEN RAISE EXCEPTION 'ingestion_snapshot_immutable'; END IF;
 RETURN NEW;
END $$;
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['parsing_identification_versions','parsing_ingress_protocol_versions','parsing_ingress_compatibilities','ingestion_sources','ingestion_source_revisions','ingestion_source_confirmations','ingestion_terminal_revisions','ingestion_device_associations','ingestion_execution_grant_history','ingestion_objects','ingestion_receipts','ingestion_identity_decisions','ingestion_recovery_jobs','ingestion_units','ingestion_unit_assignments','ingestion_parse_plans','ingestion_results','ingestion_demultiplex_decisions','ingestion_changes','ingestion_operation_receipts'] LOOP
  EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION ingestion_frozen_guard()',tab||'_frozen',tab);
 END LOOP;
END $$;
-- +goose StatementEnd

ALTER TABLE task_records DROP CONSTRAINT task_records_scope_kind_check;
ALTER TABLE task_records ADD CONSTRAINT task_records_scope_kind_check CHECK(
 (scope_kind IN ('tenant','project') AND tenant_id IS NOT NULL AND (source_module NOT IN ('parsing','ingestion') OR scope_kind='tenant')) OR
 (scope_kind='platform' AND tenant_id IS NULL AND project_id IS NULL AND
 ((source_module='audit' AND source_kind='export' AND task_type='audit.export') OR (source_module='parsing' AND source_kind='run' AND task_type='parsing.test') OR (source_module='ingestion' AND source_kind='recovery' AND task_type='ingestion.reprocess'))));
INSERT INTO task_source_states(source_module,source_kind,restore_epoch,backfill_complete) SELECT 'ingestion','recovery',restore_epoch,true FROM task_protocol WHERE id=1;
INSERT INTO audit_source_states(source_module,source_kind,complete) VALUES('ingestion','recovery',true);
CREATE VIEW ingestion_task_metadata AS SELECT id,revision,task_snapshot AS snapshot FROM ingestion_recovery_jobs WHERE task_snapshot IS NOT NULL;
CREATE OR REPLACE VIEW task_retained_sources AS
 SELECT snapshot->>'source_module' AS source_module,snapshot->>'source_kind' AS source_kind,(snapshot->>'source_root_id')::uuid AS source_root_id FROM monitoring_task_metadata WHERE snapshot IS NOT NULL
 UNION ALL SELECT 'spatial','job',id FROM spatial_task_metadata
 UNION ALL SELECT 'display','job',id FROM display_task_metadata
 UNION ALL SELECT 'derived','group',id FROM derived_task_groups
 UNION ALL SELECT 'files','job',id FROM file_task_metadata
 UNION ALL SELECT 'audit','export',id FROM audit_exports
 UNION ALL SELECT 'parsing','run',id FROM parsing_runs
 UNION ALL SELECT 'ingestion','recovery',id FROM ingestion_recovery_jobs;
GRANT SELECT ON ingestion_task_metadata,task_retained_sources TO infrapro_task_index;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Ingestion evidence is retained; restore a consistent backup'; END $$;
-- +goose StatementEnd
