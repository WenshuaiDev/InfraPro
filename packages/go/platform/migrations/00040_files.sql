-- +goose Up
CREATE TABLE file_protocol (
 id integer PRIMARY KEY CHECK(id=1), schema_version integer NOT NULL DEFAULT 1 CHECK(schema_version=1),
 restore_epoch uuid NOT NULL DEFAULT gen_random_uuid(), mode text NOT NULL DEFAULT 'online' CHECK(mode IN ('online','read_only','reconciling')),
 updated_at timestamptz NOT NULL DEFAULT now(), last_reconcile_at timestamptz
);
INSERT INTO file_protocol(id) VALUES(1);
CREATE TABLE file_spaces (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid,
 scope_kind text NOT NULL CHECK(scope_kind IN ('tenant','project')), content_revision bigint NOT NULL DEFAULT 1 CHECK(content_revision>0),
 created_at timestamptz NOT NULL DEFAULT now(), CHECK((scope_kind='project')=(project_id IS NOT NULL)),
 FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
CREATE UNIQUE INDEX file_tenant_space ON file_spaces(tenant_id) WHERE project_id IS NULL;
CREATE UNIQUE INDEX file_project_space ON file_spaces(tenant_id,project_id) WHERE project_id IS NOT NULL;
CREATE TABLE file_hosts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), space_id uuid NOT NULL REFERENCES file_spaces(id), kind text NOT NULL CHECK(kind IN ('tenant.profile','project.profile')),
 object_id uuid NOT NULL, schema_version integer NOT NULL DEFAULT 1 CHECK(schema_version=1), created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(space_id,kind,object_id), UNIQUE(space_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION file_host_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' AND (NEW.space_id,NEW.kind,NEW.object_id) IS DISTINCT FROM (OLD.space_id,OLD.kind,OLD.object_id) THEN RAISE EXCEPTION 'immutable file host'; END IF;
 IF NOT EXISTS(SELECT 1 FROM file_spaces WHERE id=NEW.space_id AND ((scope_kind='tenant' AND NEW.kind='tenant.profile' AND tenant_id=NEW.object_id) OR (scope_kind='project' AND NEW.kind='project.profile' AND project_id=NEW.object_id))) THEN RAISE EXCEPTION 'file host scope mismatch'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER file_host_guard BEFORE INSERT OR UPDATE ON file_hosts FOR EACH ROW EXECUTE FUNCTION file_host_guard();
CREATE TABLE file_objects (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), space_id uuid NOT NULL REFERENCES file_spaces(id), kind text NOT NULL CHECK(kind IN ('part','source','preview')),
 object_key text NOT NULL UNIQUE, storage_version text, content_type text NOT NULL, expected_size bigint NOT NULL CHECK(expected_size>0),
 expected_sha256 text CHECK(expected_sha256 ~ '^[a-f0-9]{64}$'), actual_size bigint CHECK(actual_size>=0), actual_sha256 text CHECK(actual_sha256 ~ '^[a-f0-9]{64}$'),
 state text NOT NULL CHECK(state IN ('writing','available','delete_pending','deleted','missing','corrupt')),
 writer_kind text NOT NULL CHECK(writer_kind IN ('upload','job')), writer_id uuid NOT NULL, writer_generation bigint NOT NULL CHECK(writer_generation>=0),
 writer_fence bigint NOT NULL CHECK(writer_fence>0), writer_epoch uuid NOT NULL, io_deadline timestamptz NOT NULL, lease_until timestamptz NOT NULL,
 last_verified_at timestamptz, delete_after timestamptz, delete_attempts integer NOT NULL DEFAULT 0 CHECK(delete_attempts>=0), last_error_code text NOT NULL DEFAULT '' CHECK(length(last_error_code)<=100),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(space_id,id), CHECK(object_key='files/v1/'||space_id::text||'/'||id::text)
);
CREATE INDEX file_object_writer ON file_objects(writer_kind,writer_id,writer_generation);
CREATE INDEX file_object_pending ON file_objects(state,io_deadline,delete_after);
CREATE TABLE file_uploads (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), space_id uuid NOT NULL, host_id uuid NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id),
 tenant_membership_id uuid NOT NULL REFERENCES tenant_memberships(id), project_membership_id uuid REFERENCES project_memberships(id),
 intent text NOT NULL CHECK(intent IN ('add','replace')), attachment_id uuid, expected_attachment_revision bigint CHECK(expected_attachment_revision>0),
 filename text NOT NULL CHECK(length(filename) BETWEEN 1 AND 255), display_name text NOT NULL CHECK(length(display_name) BETWEEN 1 AND 200), description text NOT NULL DEFAULT '' CHECK(length(description)<=1000),
 expected_size bigint NOT NULL CHECK(expected_size BETWEEN 1 AND 104857600), expected_sha256 text NOT NULL CHECK(expected_sha256 ~ '^[a-f0-9]{64}$'), declared_media_type text NOT NULL,
 detected_format text CHECK(detected_format IN ('png','jpeg','pdf','txt','csv','docx','xlsx','pptx')),
 part_size bigint NOT NULL CHECK(part_size BETWEEN 1 AND 8388608), part_count integer NOT NULL CHECK(part_count>0),
 state text NOT NULL DEFAULT 'receiving' CHECK(state IN ('receiving','submitted','closed','cancelled','expired')), revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 policy_snapshot jsonb NOT NULL CHECK(policy_snapshot->>'schema_version'='1'), restore_epoch uuid NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500), last_part_at timestamptz, idle_expires_at timestamptz NOT NULL, hard_expires_at timestamptz NOT NULL,
 submitted_at timestamptz, request_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(space_id,id), UNIQUE(space_id,host_id,id), FOREIGN KEY(space_id,host_id) REFERENCES file_hosts(space_id,id),
 CHECK((intent='replace')=(attachment_id IS NOT NULL AND expected_attachment_revision IS NOT NULL)),
 CHECK(part_count=(expected_size+part_size-1)/part_size), CHECK(idle_expires_at<=hard_expires_at)
);
CREATE INDEX file_upload_owner ON file_uploads(space_id,actor_id,state,created_at DESC,id DESC);
CREATE INDEX file_upload_expiry ON file_uploads(state,idle_expires_at,hard_expires_at);
CREATE TABLE file_upload_parts (
 space_id uuid NOT NULL, upload_id uuid NOT NULL, part_no integer NOT NULL CHECK(part_no>=1), expected_size bigint NOT NULL CHECK(expected_size>0),
 sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'), object_id uuid NOT NULL, state text NOT NULL CHECK(state IN ('receiving','received')),
 fence bigint NOT NULL CHECK(fence>0), lease_owner uuid NOT NULL, lease_until timestamptz NOT NULL, received_at timestamptz,
 PRIMARY KEY(upload_id,part_no), FOREIGN KEY(space_id,upload_id) REFERENCES file_uploads(space_id,id), FOREIGN KEY(space_id,object_id) REFERENCES file_objects(space_id,id),
 CHECK((state='received')=(received_at IS NOT NULL))
);
CREATE TABLE file_records (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), space_id uuid NOT NULL, host_id uuid NOT NULL, upload_id uuid NOT NULL UNIQUE,
 original_name text NOT NULL, format text NOT NULL CHECK(format IN ('png','jpeg','pdf','txt','csv','docx','xlsx','pptx')), media_type text NOT NULL,
 size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 104857600), sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 source_object_id uuid NOT NULL, preview_object_id uuid, preview_state text NOT NULL CHECK(preview_state IN ('none','pending','available','unavailable')), preview_reason text NOT NULL DEFAULT '',
 lifecycle text NOT NULL DEFAULT 'ready' CHECK(lifecycle IN ('ready','reclaiming','deleted')), integrity text NOT NULL DEFAULT 'ok' CHECK(integrity IN ('ok','missing','corrupt','unknown')),
 verified_at timestamptz NOT NULL, ready_at timestamptz NOT NULL, unbound_until timestamptz, reclaimed_at timestamptz,
 UNIQUE(space_id,id), UNIQUE(space_id,host_id,id), FOREIGN KEY(space_id,host_id,upload_id) REFERENCES file_uploads(space_id,host_id,id),
 FOREIGN KEY(space_id,source_object_id) REFERENCES file_objects(space_id,id), FOREIGN KEY(space_id,preview_object_id) REFERENCES file_objects(space_id,id),
 CHECK((preview_state='available')=(preview_object_id IS NOT NULL))
);
CREATE TABLE file_attachments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), space_id uuid NOT NULL, host_id uuid NOT NULL,
 display_name text NOT NULL CHECK(length(display_name) BETWEEN 1 AND 200), description text NOT NULL DEFAULT '' CHECK(length(description)<=1000),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','trashed','purged')), revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 current_revision_id uuid NOT NULL, next_revision_no bigint NOT NULL DEFAULT 2 CHECK(next_revision_no>1),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id), trashed_by uuid REFERENCES accounts(id),
 trashed_at timestamptz, purge_after timestamptz, retention_policy_version integer NOT NULL DEFAULT 1 CHECK(retention_policy_version=1),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(space_id,id), UNIQUE(space_id,host_id,id), FOREIGN KEY(space_id,host_id) REFERENCES file_hosts(space_id,id),
 CHECK((status IN ('trashed','purged'))=(trashed_at IS NOT NULL AND purge_after IS NOT NULL AND trashed_by IS NOT NULL))
);
CREATE INDEX file_attachment_list ON file_attachments(space_id,status,created_at DESC,id DESC);
CREATE INDEX file_attachment_host ON file_attachments(space_id,host_id,status);
CREATE INDEX file_attachment_purge ON file_attachments(purge_after) WHERE status='trashed';
ALTER TABLE file_uploads ADD FOREIGN KEY(space_id,host_id,attachment_id) REFERENCES file_attachments(space_id,host_id,id);
CREATE TABLE file_jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), space_id uuid NOT NULL, upload_id uuid NOT NULL UNIQUE, host_id uuid NOT NULL,
 originator_id uuid NOT NULL REFERENCES accounts(id), origin_tenant_membership_id uuid NOT NULL REFERENCES tenant_memberships(id), origin_project_membership_id uuid REFERENCES project_memberships(id),
 generation bigint NOT NULL DEFAULT 1 CHECK(generation>0), source_revision bigint NOT NULL DEFAULT 1 CHECK(source_revision>0), control_revision bigint NOT NULL DEFAULT 1 CHECK(control_revision>0),
 state text NOT NULL CHECK(state IN ('queued','running','retry_wait','cancelling','succeeded','failed','cancelled','needs_action')),
 phase text NOT NULL CHECK(phase IN ('queued','preparing','validating','writing','sealing','waiting','completed','failed','cancelled')),
 progress_completed bigint NOT NULL DEFAULT 0 CHECK(progress_completed>=0), progress_total bigint CHECK(progress_total>=progress_completed), progress_unit text NOT NULL DEFAULT 'bytes' CHECK(progress_unit IN ('bytes','files','units')),
 error_code text NOT NULL DEFAULT '' CHECK(length(error_code)<=100), task_snapshot jsonb NOT NULL DEFAULT '{}' CHECK(octet_length(task_snapshot::text)<=32768),
 created_at timestamptz NOT NULL DEFAULT now(), observed_at timestamptz NOT NULL DEFAULT now(), UNIQUE(space_id,id),
 FOREIGN KEY(space_id,host_id,upload_id) REFERENCES file_uploads(space_id,host_id,id)
);
CREATE TABLE file_attachment_revisions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), space_id uuid NOT NULL, host_id uuid NOT NULL, attachment_id uuid NOT NULL, revision_no bigint NOT NULL CHECK(revision_no>0), file_id uuid NOT NULL,
 operation text NOT NULL CHECK(operation IN ('upload','replace','revert')), previous_revision_id uuid, reverted_from_revision_id uuid,
 display_name_snapshot text NOT NULL CHECK(length(display_name_snapshot) BETWEEN 1 AND 200), reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
 actor_id uuid NOT NULL REFERENCES accounts(id), job_id uuid, job_generation bigint CHECK(job_generation>0), created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(attachment_id,revision_no), UNIQUE(space_id,attachment_id,id), UNIQUE(space_id,id),
 FOREIGN KEY(space_id,host_id,attachment_id) REFERENCES file_attachments(space_id,host_id,id), FOREIGN KEY(space_id,host_id,file_id) REFERENCES file_records(space_id,host_id,id),
 FOREIGN KEY(space_id,attachment_id,previous_revision_id) REFERENCES file_attachment_revisions(space_id,attachment_id,id),
 FOREIGN KEY(space_id,attachment_id,reverted_from_revision_id) REFERENCES file_attachment_revisions(space_id,attachment_id,id),
 FOREIGN KEY(space_id,job_id) REFERENCES file_jobs(space_id,id), CHECK((job_id IS NULL)=(job_generation IS NULL))
);
ALTER TABLE file_attachments ADD FOREIGN KEY(space_id,id,current_revision_id) REFERENCES file_attachment_revisions(space_id,attachment_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE file_protection_refs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), space_id uuid NOT NULL, file_id uuid NOT NULL, owner_module text NOT NULL CHECK(length(owner_module) BETWEEN 1 AND 80),
 owner_kind text NOT NULL CHECK(length(owner_kind) BETWEEN 1 AND 80), owner_id uuid NOT NULL, owner_revision text NOT NULL CHECK(length(owner_revision) BETWEEN 1 AND 100),
 kind text NOT NULL CHECK(kind IN ('attachment_revision','prepared_input','consumer')), expires_at timestamptz, released_at timestamptz, release_reason text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(file_id,owner_module,owner_kind,owner_id,owner_revision,kind), FOREIGN KEY(space_id,file_id) REFERENCES file_records(space_id,id)
);
CREATE INDEX file_live_refs ON file_protection_refs(file_id) WHERE released_at IS NULL;
CREATE TABLE file_job_runs (
 space_id uuid NOT NULL, job_id uuid NOT NULL, generation bigint NOT NULL CHECK(generation>0), requested_by uuid NOT NULL REFERENCES accounts(id), executor_account_id uuid NOT NULL REFERENCES accounts(id),
 tenant_membership_id uuid NOT NULL REFERENCES tenant_memberships(id), project_membership_id uuid REFERENCES project_memberships(id),
 input_snapshot jsonb NOT NULL CHECK(input_snapshot->>'schema_version'='1'), input_hash text NOT NULL CHECK(input_hash ~ '^[a-f0-9]{64}$'), expected_attachment_revision bigint CHECK(expected_attachment_revision>0),
 state text NOT NULL CHECK(state IN ('queued','running','retry_wait','cancelling','succeeded','failed','cancelled','needs_action')), attempt_no integer NOT NULL DEFAULT 0 CHECK(attempt_no>=0),
 fence bigint NOT NULL DEFAULT 0 CHECK(fence>=0), lease_owner uuid, lease_until timestamptz, available_at timestamptz NOT NULL, deadline_at timestamptz NOT NULL, cancel_requested_at timestamptz,
 result_file_id uuid, result_attachment_id uuid, result_revision_id uuid, terminal_code text NOT NULL DEFAULT '' CHECK(length(terminal_code)<=100), safe_report jsonb NOT NULL DEFAULT '{"schema_version":1}',
 started_at timestamptz, finished_at timestamptz, input_retain_until timestamptz NOT NULL, restore_epoch uuid NOT NULL,
 PRIMARY KEY(job_id,generation), UNIQUE(space_id,job_id,generation), FOREIGN KEY(space_id,job_id) REFERENCES file_jobs(space_id,id),
 FOREIGN KEY(space_id,result_file_id) REFERENCES file_records(space_id,id), FOREIGN KEY(space_id,result_attachment_id,result_revision_id) REFERENCES file_attachment_revisions(space_id,attachment_id,id),
 CHECK((state IN ('succeeded','failed','cancelled','needs_action'))=(finished_at IS NOT NULL))
);
CREATE INDEX file_job_queue ON file_job_runs(state,available_at,lease_until);
CREATE TABLE file_job_attempts (
 space_id uuid NOT NULL, job_id uuid NOT NULL, generation bigint NOT NULL, attempt_no integer NOT NULL CHECK(attempt_no>0), fence bigint NOT NULL CHECK(fence>0),
 started_at timestamptz NOT NULL, finished_at timestamptz, outcome text NOT NULL CHECK(outcome IN ('running','succeeded','failed','cancelled','needs_action','superseded','retry_wait')),
 error_code text NOT NULL DEFAULT '', diagnostic_id uuid NOT NULL DEFAULT gen_random_uuid(),
 PRIMARY KEY(job_id,generation,attempt_no), FOREIGN KEY(space_id,job_id,generation) REFERENCES file_job_runs(space_id,job_id,generation)
);
CREATE TABLE file_read_leases (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), space_id uuid NOT NULL, file_id uuid NOT NULL, object_id uuid NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id),
 tenant_membership_id uuid NOT NULL REFERENCES tenant_memberships(id), project_membership_id uuid REFERENCES project_memberships(id),
 variant text NOT NULL CHECK(variant IN ('source','preview')), restore_epoch uuid NOT NULL, expires_at timestamptz NOT NULL, closed_at timestamptz, outcome text NOT NULL DEFAULT '', bytes_sent bigint NOT NULL DEFAULT 0 CHECK(bytes_sent>=0),
 FOREIGN KEY(space_id,file_id) REFERENCES file_records(space_id,id), FOREIGN KEY(space_id,object_id) REFERENCES file_objects(space_id,id)
);
CREATE INDEX file_live_reads ON file_read_leases(file_id,expires_at) WHERE closed_at IS NULL;
CREATE TABLE file_command_receipts (
 space_id uuid NOT NULL REFERENCES file_spaces(id), actor_id uuid NOT NULL REFERENCES accounts(id), request_id uuid NOT NULL, operation text NOT NULL, target_id uuid,
 payload_hash text NOT NULL CHECK(payload_hash ~ '^[a-f0-9]{64}$'), result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(space_id,actor_id,request_id)
);
CREATE TABLE file_budget_states (key text PRIMARY KEY, cleanup_cursor uuid, reserved_bytes bigint NOT NULL DEFAULT 0 CHECK(reserved_bytes>=0), slots integer NOT NULL DEFAULT 0 CHECK(slots>=0), revision bigint NOT NULL DEFAULT 1 CHECK(revision>0));
INSERT INTO file_budget_states(key) VALUES('instance');
CREATE TABLE file_budget_reservations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_kind text NOT NULL CHECK(owner_kind IN ('upload','job','object','read')), owner_id uuid NOT NULL, space_id uuid REFERENCES file_spaces(id),
 account_id uuid REFERENCES accounts(id), tenant_id uuid REFERENCES tenants(id), node_id text NOT NULL DEFAULT '',
 kind text NOT NULL CHECK(kind IN ('temporary_object','local_temp','execution','download')), reserved_bytes bigint NOT NULL CHECK(reserved_bytes>=0), actual_bytes bigint NOT NULL DEFAULT 0 CHECK(actual_bytes>=0), slots integer NOT NULL DEFAULT 1 CHECK(slots>=0),
 state text NOT NULL CHECK(state IN ('held','reconciling','released')), expires_at timestamptz NOT NULL, reconciled_at timestamptz, UNIQUE(owner_kind,owner_id,kind)
);
CREATE TABLE file_rate_windows (account_id uuid NOT NULL REFERENCES accounts(id), kind text NOT NULL CHECK(kind IN ('create','part','read')), window_start timestamptz NOT NULL, requests integer NOT NULL CHECK(requests>0), PRIMARY KEY(account_id,kind,window_start));
CREATE TABLE file_worker_heartbeats (worker_id text PRIMARY KEY, node_id text NOT NULL, build_version text NOT NULL, protocol_version integer NOT NULL CHECK(protocol_version=1), started_at timestamptz NOT NULL, last_seen_at timestamptz NOT NULL, checker_last_seen_at timestamptz, capabilities jsonb NOT NULL);
CREATE TABLE file_cleanup_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), mode text NOT NULL CHECK(mode IN ('plan','apply')), operator text NOT NULL, reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 500), policy_version integer NOT NULL DEFAULT 1 CHECK(policy_version=1), restore_epoch uuid NOT NULL,
 state text NOT NULL CHECK(state IN ('planned','running','completed','failed')), planned_at timestamptz NOT NULL, finished_at timestamptz, counts jsonb NOT NULL DEFAULT '{}'
);
CREATE TABLE file_cleanup_items (
 run_id uuid NOT NULL REFERENCES file_cleanup_runs(id), space_id uuid NOT NULL, object_id uuid NOT NULL, file_id uuid, candidate_revision bigint NOT NULL CHECK(candidate_revision>0), reason_code text NOT NULL,
 not_before timestamptz NOT NULL, state text NOT NULL CHECK(state IN ('planned','protected','stale','deleted','failed')), attempt_no integer NOT NULL DEFAULT 0 CHECK(attempt_no>=0), last_error_code text NOT NULL DEFAULT '',
 PRIMARY KEY(run_id,object_id), FOREIGN KEY(space_id,object_id) REFERENCES file_objects(space_id,id), FOREIGN KEY(space_id,file_id) REFERENCES file_records(space_id,id)
);
CREATE TABLE file_backup_manifests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), restore_epoch uuid NOT NULL, state text NOT NULL CHECK(state IN ('preparing','sealed','failed','released')), database_snapshot_ref text NOT NULL,
 policy_version integer NOT NULL DEFAULT 1 CHECK(policy_version=1), manifest_hash text CHECK(manifest_hash ~ '^[a-f0-9]{64}$'), hold_until timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE file_backup_items (
 manifest_id uuid NOT NULL REFERENCES file_backup_manifests(id), space_id uuid NOT NULL, object_id uuid NOT NULL, size_bytes bigint NOT NULL CHECK(size_bytes>0), sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'), storage_version text,
 copy_status text NOT NULL CHECK(copy_status IN ('pending','verified','failed')), PRIMARY KEY(manifest_id,object_id), FOREIGN KEY(space_id,object_id) REFERENCES file_objects(space_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION file_immutable_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'file history requires tombstones'; END IF;
 IF TG_TABLE_NAME='file_attachment_revisions' THEN RAISE EXCEPTION 'immutable file revision'; END IF;
 IF TG_TABLE_NAME='file_records' THEN
 IF (NEW.space_id,NEW.host_id,NEW.upload_id,NEW.original_name,NEW.format,NEW.media_type,NEW.size_bytes,NEW.sha256,NEW.source_object_id,NEW.ready_at) IS DISTINCT FROM (OLD.space_id,OLD.host_id,OLD.upload_id,OLD.original_name,OLD.format,OLD.media_type,OLD.size_bytes,OLD.sha256,OLD.source_object_id,OLD.ready_at) THEN RAISE EXCEPTION 'immutable file body'; END IF;
 END IF;
 IF TG_TABLE_NAME='file_objects' THEN
 IF (NEW.space_id,NEW.object_key,NEW.kind,NEW.expected_size,NEW.expected_sha256,NEW.writer_id,NEW.writer_generation,NEW.writer_fence,NEW.writer_epoch) IS DISTINCT FROM (OLD.space_id,OLD.object_key,OLD.kind,OLD.expected_size,OLD.expected_sha256,OLD.writer_id,OLD.writer_generation,OLD.writer_fence,OLD.writer_epoch) THEN RAISE EXCEPTION 'immutable object intent'; END IF;
 END IF;
 IF TG_TABLE_NAME='file_job_runs' THEN
  IF (NEW.space_id,NEW.job_id,NEW.generation,NEW.requested_by,NEW.executor_account_id,NEW.tenant_membership_id,NEW.project_membership_id,NEW.input_snapshot,NEW.input_hash,NEW.restore_epoch) IS DISTINCT FROM (OLD.space_id,OLD.job_id,OLD.generation,OLD.requested_by,OLD.executor_account_id,OLD.tenant_membership_id,OLD.project_membership_id,OLD.input_snapshot,OLD.input_hash,OLD.restore_epoch) THEN RAISE EXCEPTION 'immutable file execution input'; END IF;
  IF NEW.input_retain_until>OLD.input_retain_until THEN RAISE EXCEPTION 'cannot extend execution input lifetime'; END IF;
  IF OLD.finished_at IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'immutable file execution result'; END IF;
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER file_revision_immutable BEFORE UPDATE OR DELETE ON file_attachment_revisions FOR EACH ROW EXECUTE FUNCTION file_immutable_guard();
CREATE TRIGGER file_record_immutable BEFORE UPDATE OR DELETE ON file_records FOR EACH ROW EXECUTE FUNCTION file_immutable_guard();
CREATE TRIGGER file_object_immutable BEFORE UPDATE OR DELETE ON file_objects FOR EACH ROW EXECUTE FUNCTION file_immutable_guard();
CREATE TRIGGER file_run_immutable BEFORE UPDATE OR DELETE ON file_job_runs FOR EACH ROW EXECUTE FUNCTION file_immutable_guard();
-- +goose StatementBegin
CREATE FUNCTION file_upload_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sp file_spaces; tm tenant_memberships;
BEGIN
 SELECT * INTO sp FROM file_spaces WHERE id=NEW.space_id;
 SELECT * INTO tm FROM tenant_memberships WHERE id=NEW.tenant_membership_id;
 IF tm.tenant_id IS DISTINCT FROM sp.tenant_id OR tm.account_id IS DISTINCT FROM NEW.actor_id OR (sp.project_id IS NULL)<>(NEW.project_membership_id IS NULL) THEN RAISE EXCEPTION 'file upload membership mismatch'; END IF;
 IF sp.project_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM project_memberships WHERE id=NEW.project_membership_id AND tenant_id=sp.tenant_id AND project_id=sp.project_id AND account_id=NEW.actor_id AND tenant_membership_id=tm.id) THEN RAISE EXCEPTION 'file upload project membership mismatch'; END IF;
 IF TG_OP='UPDATE' THEN
  IF (NEW.space_id,NEW.host_id,NEW.actor_id,NEW.tenant_membership_id,NEW.project_membership_id,NEW.intent,NEW.attachment_id,NEW.expected_attachment_revision,NEW.filename,NEW.expected_size,NEW.expected_sha256,NEW.declared_media_type,NEW.part_size,NEW.part_count,NEW.policy_snapshot,NEW.restore_epoch,NEW.hard_expires_at) IS DISTINCT FROM (OLD.space_id,OLD.host_id,OLD.actor_id,OLD.tenant_membership_id,OLD.project_membership_id,OLD.intent,OLD.attachment_id,OLD.expected_attachment_revision,OLD.filename,OLD.expected_size,OLD.expected_sha256,OLD.declared_media_type,OLD.part_size,OLD.part_count,OLD.policy_snapshot,OLD.restore_epoch,OLD.hard_expires_at) THEN RAISE EXCEPTION 'immutable upload mandate'; END IF;
  IF OLD.state<>'receiving' AND NEW.state='receiving' THEN RAISE EXCEPTION 'cannot reopen upload'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION file_source_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='INSERT' AND NOT EXISTS(SELECT 1 FROM file_objects WHERE id=NEW.source_object_id AND space_id=NEW.space_id AND kind='source' AND state='available' AND actual_size=NEW.size_bytes AND actual_sha256=NEW.sha256) THEN RAISE EXCEPTION 'file body not verified'; END IF;
 IF TG_OP='UPDATE' AND NEW.unbound_until IS NOT NULL AND (OLD.unbound_until IS NULL OR NEW.unbound_until>OLD.unbound_until) THEN RAISE EXCEPTION 'cannot extend prepared lifetime'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION file_reference_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE lifecycle text;
BEGIN
 IF TG_OP='UPDATE' AND OLD.released_at IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'released protection is immutable'; END IF;
 SELECT f.lifecycle INTO lifecycle FROM file_records f WHERE f.id=NEW.file_id AND f.space_id=NEW.space_id FOR UPDATE;
 IF NEW.released_at IS NULL AND lifecycle IS DISTINCT FROM 'ready' THEN RAISE EXCEPTION 'file reclaiming'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION file_part_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE u file_uploads;
BEGIN
 SELECT * INTO u FROM file_uploads WHERE id=NEW.upload_id AND space_id=NEW.space_id;
 IF NEW.part_no>u.part_count OR NEW.expected_size<>LEAST(u.part_size,u.expected_size-(NEW.part_no-1)*u.part_size) THEN RAISE EXCEPTION 'file part boundary mismatch'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER file_upload_guard BEFORE INSERT OR UPDATE ON file_uploads FOR EACH ROW EXECUTE FUNCTION file_upload_guard();
CREATE TRIGGER file_source_guard BEFORE INSERT OR UPDATE ON file_records FOR EACH ROW EXECUTE FUNCTION file_source_guard();
CREATE TRIGGER file_reference_guard BEFORE INSERT OR UPDATE ON file_protection_refs FOR EACH ROW EXECUTE FUNCTION file_reference_guard();
CREATE TRIGGER file_part_guard BEFORE INSERT OR UPDATE ON file_upload_parts FOR EACH ROW EXECUTE FUNCTION file_part_guard();
CREATE VIEW file_task_metadata AS SELECT id,source_revision AS task_revision,task_snapshot AS snapshot FROM file_jobs WHERE task_snapshot<>'{}'::jsonb;
CREATE OR REPLACE VIEW task_retained_sources AS
 SELECT snapshot->>'source_module' AS source_module,snapshot->>'source_kind' AS source_kind,(snapshot->>'source_root_id')::uuid AS source_root_id FROM monitoring_task_metadata WHERE snapshot IS NOT NULL
 UNION ALL SELECT 'spatial','job',id FROM spatial_task_metadata
 UNION ALL SELECT 'display','job',id FROM display_task_metadata
 UNION ALL SELECT 'derived','group',id FROM derived_task_groups
 UNION ALL SELECT 'files','job',id FROM file_task_metadata;
GRANT SELECT ON file_task_metadata,task_retained_sources TO infrapro_task_index;
INSERT INTO task_source_states(source_module,source_kind,restore_epoch) SELECT 'files','job',restore_epoch FROM task_protocol WHERE id=1;
-- Safe observer projection; it cannot reveal titles, bodies, paths or actors.
CREATE VIEW file_operations_status AS SELECT
 (SELECT mode FROM file_protocol WHERE id=1) AS mode,
 (SELECT restore_epoch::text FROM file_protocol WHERE id=1) AS restore_epoch,
 (SELECT last_reconcile_at FROM file_protocol WHERE id=1) AS last_reconcile_at,
 (SELECT count(*)::text FROM file_uploads WHERE state='receiving') AS uploads_receiving,
 (SELECT count(*)::text FROM file_records WHERE unbound_until IS NOT NULL AND lifecycle='ready') AS prepared_unbound,
 (SELECT count(*)::text FROM file_jobs WHERE state IN ('queued','retry_wait')) AS jobs_queued,
 (SELECT count(*)::text FROM file_jobs WHERE state IN ('running','cancelling')) AS jobs_running,
 (SELECT count(*)::text FROM file_jobs WHERE state='failed') AS jobs_failed,
 (SELECT min(created_at) FROM file_jobs WHERE state='queued') AS oldest_queued_at,
 (SELECT COALESCE(sum(reserved_bytes),0)::text FROM file_budget_reservations WHERE kind='temporary_object' AND state<>'released') AS temporary_reserved,
 (SELECT COALESCE(sum(actual_bytes),0)::text FROM file_budget_reservations WHERE kind='temporary_object' AND state<>'released') AS temporary_actual,
 (SELECT count(*)::text FROM file_cleanup_items WHERE state='failed') AS cleanup_failed,
 (SELECT count(*)::text FROM file_records WHERE integrity='missing') AS content_missing,
 (SELECT count(*)::text FROM file_records WHERE integrity='corrupt') AS content_corrupt,
 (SELECT max(last_seen_at) FROM file_worker_heartbeats) AS worker_last_seen,
 (SELECT max(LEAST(last_seen_at,checker_last_seen_at)) FROM file_worker_heartbeats WHERE checker_last_seen_at IS NOT NULL) AS checker_last_seen;
-- Roles have no login. Operators bind separate deployment login credentials.
-- +goose StatementBegin
DO $$ DECLARE r text; t text; BEGIN
 FOREACH r IN ARRAY ARRAY['infrapro_files_worker','infrapro_files_observer','infrapro_files_maintenance','infrapro_files_backup'] LOOP
  IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=r) THEN EXECUTE format('CREATE ROLE %I NOLOGIN',r); END IF;
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO %I',current_schema(),r);
 END LOOP;
 FOR t IN SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename LIKE 'file\_%' ESCAPE '\' LOOP
  EXECUTE format('GRANT SELECT,INSERT,UPDATE,DELETE ON %I TO infrapro_files_worker,infrapro_files_maintenance',t);
  EXECUTE format('GRANT SELECT ON %I TO infrapro_files_backup',t);
 END LOOP;
 EXECUTE format('GRANT USAGE,SELECT ON SEQUENCE %s TO infrapro_files_worker,infrapro_files_maintenance',pg_get_serial_sequence('account_security_events','id'));
END $$;
-- +goose StatementEnd
GRANT SELECT ON goose_db_version,file_operations_status TO infrapro_files_observer,infrapro_files_worker,infrapro_files_maintenance,infrapro_files_backup;
GRANT SELECT(role_id) ON account_roles TO infrapro_files_worker,infrapro_files_maintenance;
GRANT SELECT(id,status,username,display_name) ON accounts TO infrapro_files_worker,infrapro_files_maintenance;
GRANT SELECT ON tenants,projects,tenant_memberships,project_memberships,roles,role_permissions,permission_catalog,tenant_member_roles,project_member_roles,authorization_state TO infrapro_files_worker,infrapro_files_maintenance;
GRANT UPDATE(id) ON accounts,tenants,projects,authorization_state TO infrapro_files_worker,infrapro_files_maintenance;
GRANT SELECT ON task_protocol,task_records,task_executions,task_events,task_attempts,task_transaction_facts TO infrapro_files_worker,infrapro_files_maintenance;
GRANT INSERT,UPDATE ON task_records,task_executions,task_attempts TO infrapro_files_worker,infrapro_files_maintenance;
GRANT INSERT ON task_events,task_relations,account_security_events TO infrapro_files_worker,infrapro_files_maintenance;
GRANT DELETE ON task_transaction_facts TO infrapro_files_worker,infrapro_files_maintenance;
GRANT SELECT ON notification_protocol,notification_events,notification_recipients TO infrapro_files_worker,infrapro_files_maintenance;
GRANT INSERT ON notification_events,notification_recipients TO infrapro_files_worker,infrapro_files_maintenance;
GRANT UPDATE(source_key) ON notification_events TO infrapro_files_worker,infrapro_files_maintenance;
GRANT UPDATE ON file_protocol,file_jobs,file_job_runs,file_job_attempts,file_backup_manifests,file_backup_items,file_budget_reservations,file_uploads,file_read_leases TO infrapro_files_backup;
GRANT INSERT ON file_backup_manifests,file_backup_items TO infrapro_files_backup;
GRANT SELECT ON task_protocol,task_records,task_executions,task_events,task_attempts,task_transaction_facts TO infrapro_files_backup;
GRANT INSERT,UPDATE ON task_records,task_executions,task_attempts TO infrapro_files_backup;
GRANT INSERT ON task_events,task_relations TO infrapro_files_backup;
GRANT DELETE ON task_transaction_facts TO infrapro_files_backup;
-- +goose Down
-- Files contain immutable history. Rollback requires the documented frozen backup.
DO $$ BEGIN RAISE EXCEPTION 'restore a consistent backup to roll back files'; END $$;
