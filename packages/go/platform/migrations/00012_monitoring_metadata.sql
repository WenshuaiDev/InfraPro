-- +goose Up
-- Control metadata only. Raw observations, per-observation identity indexes and
-- window contributions belong to ClickHouse and immutable object manifests.
ALTER TABLE engineering_channels ADD UNIQUE(tenant_id,id);
ALTER TABLE project_resource_grants ADD UNIQUE(tenant_id,project_id,channel_id,id);

CREATE TABLE monitoring_environment_state (
 id integer PRIMARY KEY CHECK(id=1), mode text NOT NULL CHECK(mode IN ('ready','restoring','read_only')),
 backup_id uuid, verified_at timestamptz, access_revoked boolean NOT NULL DEFAULT false
);
INSERT INTO monitoring_environment_state(id,mode) VALUES(1,'ready');
CREATE TABLE monitoring_tenant_states (
 tenant_id uuid PRIMARY KEY REFERENCES tenants(id), commit_sequence bigint NOT NULL DEFAULT 0 CHECK(commit_sequence>=0),
 route_revision bigint NOT NULL DEFAULT 1 CHECK(route_revision>0), layout_version integer NOT NULL DEFAULT 1 CHECK(layout_version=1)
);
CREATE TABLE monitoring_access_states (
 tenant_id uuid NOT NULL, project_id uuid NOT NULL, revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 PRIMARY KEY(tenant_id,project_id), FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id)
);
CREATE TABLE monitoring_data_grants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, channel_id uuid NOT NULL,
 resource_grant_id uuid NOT NULL, permitted int8range NOT NULL CHECK(NOT isempty(permitted)),
 capabilities text[] NOT NULL CHECK(cardinality(capabilities)>0 AND cardinality(capabilities)<=6 AND array_position(capabilities,NULL) IS NULL AND capabilities <@ ARRAY['read','export','import','revoke','history','source']::text[]),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), granted_by uuid NOT NULL REFERENCES accounts(id), granted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 2000), revoked_at timestamptz, revoked_by uuid REFERENCES accounts(id), revoke_reason text,
 FOREIGN KEY(tenant_id,project_id,channel_id,resource_grant_id) REFERENCES project_resource_grants(tenant_id,project_id,channel_id,id),
 CHECK(lower_inf(permitted) OR lower(permitted)>=-2208988800000000000),
 CHECK((revoked_at IS NULL AND revoked_by IS NULL AND revoke_reason IS NULL) OR (revoked_at IS NOT NULL AND revoked_by IS NOT NULL AND length(btrim(revoke_reason))>0)),
 UNIQUE(tenant_id,id)
);
CREATE INDEX monitoring_grants_scope ON monitoring_data_grants(tenant_id,project_id,channel_id) WHERE revoked_at IS NULL;
CREATE INDEX monitoring_grants_resource ON monitoring_data_grants(resource_grant_id) WHERE revoked_at IS NULL;

CREATE TABLE monitoring_source_files (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), object_key text NOT NULL,
 original_name text NOT NULL CHECK(length(original_name) BETWEEN 1 AND 255), size_bytes bigint NOT NULL CHECK(size_bytes>=0),
 sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'), created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id), UNIQUE(object_key)
);
CREATE TABLE monitoring_import_batches (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid,
 created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 state text NOT NULL DEFAULT 'uploading' CHECK(state IN ('uploading','received','validating','needs_resolution','ready','preparing','published','completed_noop','failed','cancelled','revoked')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), source_file_id uuid, original_name text NOT NULL, expected_size bigint NOT NULL CHECK(expected_size>=0),
 mapping_revision bigint NOT NULL DEFAULT 0 CHECK(mapping_revision>=0), validation_hash text, validation_object text, report_object text,
 counts jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(counts)='object'), semantic_snapshot jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(semantic_snapshot)='object'),
 content_revisions jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(content_revisions)='object'), error_code text, readopt_batch_id uuid,
 UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id),
 FOREIGN KEY(tenant_id,source_file_id) REFERENCES monitoring_source_files(tenant_id,id),
 FOREIGN KEY(tenant_id,readopt_batch_id) REFERENCES monitoring_import_batches(tenant_id,id)
);
CREATE INDEX monitoring_imports_page ON monitoring_import_batches(tenant_id,created_at,id);
CREATE INDEX monitoring_imports_queue ON monitoring_import_batches(tenant_id,state,updated_at);
CREATE TABLE monitoring_upload_parts (
 tenant_id uuid NOT NULL, batch_id uuid NOT NULL, part integer NOT NULL CHECK(part>0), size_bytes bigint NOT NULL CHECK(size_bytes>0),
 sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'), object_key text NOT NULL UNIQUE,
 PRIMARY KEY(tenant_id,batch_id,part), FOREIGN KEY(tenant_id,batch_id) REFERENCES monitoring_import_batches(tenant_id,id)
);
CREATE TABLE monitoring_import_mappings (
 tenant_id uuid NOT NULL, batch_id uuid NOT NULL, revision bigint NOT NULL CHECK(revision>0), mapping jsonb NOT NULL CHECK(jsonb_typeof(mapping)='object'),
 sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'), created_at timestamptz NOT NULL DEFAULT clock_timestamp(), created_by uuid NOT NULL REFERENCES accounts(id),
 PRIMARY KEY(tenant_id,batch_id,revision), FOREIGN KEY(tenant_id,batch_id) REFERENCES monitoring_import_batches(tenant_id,id)
);
CREATE TABLE monitoring_batch_resolutions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, batch_id uuid NOT NULL, mapping_revision bigint NOT NULL,
 preview_hash text NOT NULL CHECK(preview_hash ~ '^[0-9a-f]{64}$'), object_key text NOT NULL UNIQUE, target_revisions jsonb NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 2000), created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,batch_id,preview_hash), FOREIGN KEY(tenant_id,batch_id,mapping_revision) REFERENCES monitoring_import_mappings(tenant_id,batch_id,revision)
);
CREATE TABLE monitoring_channel_versions (
 tenant_id uuid NOT NULL, channel_id uuid NOT NULL, content_revision bigint NOT NULL DEFAULT 0 CHECK(content_revision>=0),
 revoke_revision bigint NOT NULL DEFAULT 0 CHECK(revoke_revision>=0), integrity_revision bigint NOT NULL DEFAULT 0 CHECK(integrity_revision>=0), quarantined boolean NOT NULL DEFAULT false, first_published_sequence bigint,
 PRIMARY KEY(tenant_id,channel_id), FOREIGN KEY(tenant_id,channel_id) REFERENCES engineering_channels(tenant_id,id)
);
CREATE TABLE monitoring_chunks (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, batch_id uuid NOT NULL, mapping_revision bigint NOT NULL,
 logical_shard integer NOT NULL CHECK(logical_shard BETWEEN 0 AND 255), utc_month integer NOT NULL,
 first_ns bigint NOT NULL CHECK(first_ns>=-2208988800000000000), last_ns bigint NOT NULL CHECK(last_ns>=first_ns),
 row_count bigint NOT NULL CHECK(row_count>0 AND row_count<=1000000), payload_hash text NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'), source_object text NOT NULL, summary_object text NOT NULL,
 UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,batch_id,mapping_revision) REFERENCES monitoring_import_mappings(tenant_id,batch_id,revision)
);
CREATE INDEX monitoring_chunks_range ON monitoring_chunks(tenant_id,logical_shard,utc_month,first_ns,last_ns);
CREATE TABLE monitoring_write_attempts (
 id uuid PRIMARY KEY, tenant_id uuid NOT NULL, chunk_id uuid NOT NULL, fence bigint NOT NULL CHECK(fence>0),
 state text NOT NULL CHECK(state IN ('writing','uncertain','abandoned','sealed')), physical_shard text NOT NULL, route_revision bigint NOT NULL CHECK(route_revision>0),
 verification_hash text, replica_epochs jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT clock_timestamp(), sealed_at timestamptz,
 UNIQUE(tenant_id,chunk_id,id), FOREIGN KEY(tenant_id,chunk_id) REFERENCES monitoring_chunks(tenant_id,id),
 CHECK(state<>'sealed' OR (sealed_at IS NOT NULL AND verification_hash IS NOT NULL))
);
CREATE TABLE monitoring_publications (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, batch_id uuid NOT NULL, sequence bigint NOT NULL CHECK(sequence>0),
 manifest_hash text NOT NULL CHECK(manifest_hash ~ '^[0-9a-f]{64}$'), committed_at timestamptz NOT NULL DEFAULT clock_timestamp(), committed_by uuid NOT NULL REFERENCES accounts(id),
 revoke_sequence bigint CHECK(revoke_sequence>sequence), revoked_at timestamptz, revoked_by uuid REFERENCES accounts(id), revoke_reason text,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,batch_id), UNIQUE(tenant_id,sequence),
 FOREIGN KEY(tenant_id,batch_id) REFERENCES monitoring_import_batches(tenant_id,id),
 CHECK((revoke_sequence IS NULL AND revoked_at IS NULL AND revoked_by IS NULL AND revoke_reason IS NULL) OR (revoke_sequence IS NOT NULL AND revoked_at IS NOT NULL AND revoked_by IS NOT NULL AND length(btrim(revoke_reason))>0))
);
CREATE TABLE monitoring_publication_chunks (
 tenant_id uuid NOT NULL, publication_id uuid NOT NULL, chunk_id uuid NOT NULL, attempt_id uuid NOT NULL,
 adopted_count bigint NOT NULL CHECK(adopted_count>0), first_adopted_count bigint NOT NULL CHECK(first_adopted_count>=0 AND first_adopted_count<=adopted_count),
 quality_counts jsonb NOT NULL, PRIMARY KEY(tenant_id,chunk_id),
 FOREIGN KEY(tenant_id,publication_id) REFERENCES monitoring_publications(tenant_id,id),
 FOREIGN KEY(tenant_id,chunk_id,attempt_id) REFERENCES monitoring_write_attempts(tenant_id,chunk_id,id)
);
-- Additional verified locations never change the original publication evidence.
CREATE TABLE monitoring_chunk_locations (
 tenant_id uuid NOT NULL, chunk_id uuid NOT NULL, route_revision bigint NOT NULL CHECK(route_revision>0), attempt_id uuid NOT NULL,
 PRIMARY KEY(tenant_id,chunk_id,route_revision),
 FOREIGN KEY(tenant_id,chunk_id,attempt_id) REFERENCES monitoring_write_attempts(tenant_id,chunk_id,id)
);
CREATE TABLE monitoring_summary_heads (
 tenant_id uuid NOT NULL, channel_id uuid NOT NULL, level text NOT NULL, revision bigint NOT NULL CHECK(revision>0),
 watermark bigint NOT NULL CHECK(watermark>=0), revoke_revision bigint NOT NULL CHECK(revoke_revision>=0), root_object text NOT NULL,
 PRIMARY KEY(tenant_id,channel_id,level), FOREIGN KEY(tenant_id,channel_id) REFERENCES monitoring_channel_versions(tenant_id,channel_id)
);
CREATE TABLE monitoring_summary_replacements (
 tenant_id uuid NOT NULL, channel_id uuid NOT NULL, content_revision bigint NOT NULL, integrity_revision bigint NOT NULL,
 root_object text NOT NULL, job_id uuid NOT NULL, PRIMARY KEY(tenant_id,channel_id,content_revision,integrity_revision),
 FOREIGN KEY(tenant_id,channel_id) REFERENCES monitoring_channel_versions(tenant_id,channel_id)
);
CREATE TABLE monitoring_contribution_replacements (
 tenant_id uuid NOT NULL, chunk_id uuid NOT NULL, job_id uuid NOT NULL, object_key text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(tenant_id,chunk_id,job_id),
 FOREIGN KEY(tenant_id,chunk_id) REFERENCES monitoring_chunks(tenant_id,id)
);
CREATE TABLE monitoring_dirty_ranges (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, channel_id uuid NOT NULL, affected int8range NOT NULL CHECK(NOT isempty(affected)),
 target_sequence bigint NOT NULL CHECK(target_sequence>0), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,channel_id) REFERENCES monitoring_channel_versions(tenant_id,channel_id)
);
CREATE INDEX monitoring_dirty_channel ON monitoring_dirty_ranges(tenant_id,channel_id,target_sequence);

CREATE TABLE monitoring_jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid, actor_id uuid REFERENCES accounts(id),
 trace_id uuid NOT NULL DEFAULT COALESCE(NULLIF(current_setting('infrapro.monitoring_trace_id',true),'')::uuid,gen_random_uuid()),
 kind text NOT NULL CHECK(kind IN ('validate','publish','summary','export','verify','relocate','backup','restore')), target_id uuid NOT NULL, target_revision bigint NOT NULL CHECK(target_revision>=0),
 input_hash text NOT NULL CHECK(input_hash ~ '^[0-9a-f]{64}$'), input jsonb NOT NULL CHECK(jsonb_typeof(input)='object'),
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','succeeded','failed','cancelled','needs_action','retry_wait')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), fence bigint NOT NULL DEFAULT 0 CHECK(fence>=0), lease_owner text, lease_until timestamptz,
 attempts integer NOT NULL DEFAULT 0 CHECK(attempts>=0), available_at timestamptz NOT NULL DEFAULT clock_timestamp(), cancel_requested boolean NOT NULL DEFAULT false,
 progress jsonb NOT NULL DEFAULT '{}', error_code text, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,kind,target_id,target_revision,input_hash),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id),
 CHECK(state<>'running' OR (lease_owner IS NOT NULL AND lease_until IS NOT NULL AND fence>0))
);
CREATE INDEX monitoring_jobs_claim ON monitoring_jobs(kind,available_at,created_at) WHERE state IN ('queued','retry_wait','running');
CREATE INDEX monitoring_jobs_actor ON monitoring_jobs(tenant_id,actor_id,created_at,id);
CREATE TABLE monitoring_job_steps (
 tenant_id uuid NOT NULL, job_id uuid NOT NULL, step_key text NOT NULL, fence bigint NOT NULL CHECK(fence>0),
 state text NOT NULL CHECK(state IN ('running','completed','failed')), result jsonb NOT NULL DEFAULT '{}', updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,job_id,step_key), FOREIGN KEY(tenant_id,job_id) REFERENCES monitoring_jobs(tenant_id,id)
);
CREATE TABLE monitoring_selections (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid, actor_id uuid NOT NULL REFERENCES accounts(id),
 channel_ids uuid[] NOT NULL CHECK(cardinality(channel_ids)<=50000), selection_hash text NOT NULL CHECK(selection_hash ~ '^[0-9a-f]{64}$'),
 authorization_revision text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id)
);
CREATE TABLE monitoring_snapshots (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, selection_id uuid NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id),
 watermark bigint NOT NULL CHECK(watermark>=0), route_revision bigint NOT NULL CHECK(route_revision>0), query_hash text NOT NULL,
 revision_vector jsonb NOT NULL, authorization_revision text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 lease_until timestamptz, UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,selection_id) REFERENCES monitoring_selections(tenant_id,id)
);
CREATE TABLE monitoring_exports (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, job_id uuid NOT NULL, snapshot_id uuid NOT NULL,
 format text NOT NULL CHECK(format IN ('csv','parquet')), manifest_object text, expires_at timestamptz NOT NULL, invalidated_at timestamptz,
 UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,job_id) REFERENCES monitoring_jobs(tenant_id,id), FOREIGN KEY(tenant_id,snapshot_id) REFERENCES monitoring_snapshots(tenant_id,id)
);
CREATE TABLE monitoring_routes (
 tenant_id uuid NOT NULL REFERENCES tenants(id), logical_shard integer NOT NULL CHECK(logical_shard BETWEEN 0 AND 255),
 revision bigint NOT NULL CHECK(revision>0), physical_shard text NOT NULL, state text NOT NULL CHECK(state IN ('active','retained','barrier')),
 PRIMARY KEY(tenant_id,logical_shard,revision)
);
CREATE UNIQUE INDEX monitoring_route_active ON monitoring_routes(tenant_id,logical_shard) WHERE state IN ('active','barrier');
CREATE TABLE monitoring_relocations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), logical_shard integer NOT NULL CHECK(logical_shard BETWEEN 0 AND 255),
 source_revision bigint NOT NULL, target_shard text NOT NULL, state text NOT NULL CHECK(state IN ('planned','copying','paused','verifying','barrier','switched','completed','failed','reverting')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), manifest_object text, checkpoint jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE monitoring_relocation_copies (
 tenant_id uuid NOT NULL, relocation_id uuid NOT NULL REFERENCES monitoring_relocations(id), chunk_id uuid NOT NULL, attempt_id uuid NOT NULL,
 PRIMARY KEY(tenant_id,relocation_id,chunk_id), FOREIGN KEY(tenant_id,chunk_id,attempt_id) REFERENCES monitoring_write_attempts(tenant_id,chunk_id,id)
);
CREATE TABLE monitoring_receipts (
 scope_kind text NOT NULL CHECK(scope_kind IN ('tenant','project')), tenant_id uuid NOT NULL REFERENCES tenants(id), scope_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES accounts(id), operation text NOT NULL, request_id uuid NOT NULL,
 payload_hash text NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'), result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(scope_kind,tenant_id,scope_id,actor_id,operation,request_id)
);
CREATE TABLE monitoring_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid, actor_id uuid REFERENCES accounts(id),
 operation text NOT NULL, target_id uuid NOT NULL, reason text NOT NULL, trace_id text NOT NULL, summary jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id)
);
CREATE INDEX monitoring_audit_page ON monitoring_audit(tenant_id,created_at,id);

-- +goose StatementBegin
CREATE FUNCTION monitoring_grant_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'monitoring grant history is permanent' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND ((to_jsonb(NEW)-ARRAY['revision','revoked_at','revoked_by','revoke_reason']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['revision','revoked_at','revoked_by','revoke_reason']) OR OLD.revoked_at IS NOT NULL OR NEW.revision<>OLD.revision+1 OR NEW.revoked_at IS NULL)
 THEN RAISE EXCEPTION 'monitoring grant is immutable except one revocation' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' AND NOT EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id WHERE g.id=NEW.resource_grant_id AND g.tenant_id=NEW.tenant_id AND g.project_id=NEW.project_id AND g.channel_id=NEW.channel_id AND g.resource_kind='channel' AND g.ended_at IS NULL AND l.status='active')
 THEN RAISE EXCEPTION 'monitoring grant needs an active channel resource grant' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION monitoring_publication_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'published monitoring evidence is permanent' USING ERRCODE='23514'; END IF;
 IF (to_jsonb(NEW)-ARRAY['revoke_sequence','revoked_at','revoked_by','revoke_reason']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['revoke_sequence','revoked_at','revoked_by','revoke_reason']) OR OLD.revoke_sequence IS NOT NULL OR NEW.revoke_sequence IS NULL
 THEN RAISE EXCEPTION 'publication is immutable except one revocation' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION monitoring_selected_attempt_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM monitoring_write_attempts a JOIN monitoring_chunks c ON c.tenant_id=a.tenant_id AND c.id=a.chunk_id JOIN monitoring_publications p ON p.tenant_id=c.tenant_id AND p.batch_id=c.batch_id WHERE a.tenant_id=NEW.tenant_id AND a.chunk_id=NEW.chunk_id AND a.id=NEW.attempt_id AND a.state='sealed' AND p.id=NEW.publication_id AND c.row_count=NEW.adopted_count)
 THEN RAISE EXCEPTION 'publication requires a sealed matching complete attempt' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION monitoring_channel_reference_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL) AND EXISTS(SELECT 1 FROM monitoring_channel_versions WHERE tenant_id=NEW.tenant_id AND channel_id=NEW.id AND first_published_sequence IS NOT NULL)
 THEN RAISE EXCEPTION 'channel has permanent monitoring observations' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION monitoring_attempt_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.state='sealed' OR (to_jsonb(NEW)-ARRAY['state','verification_hash','replica_epochs','sealed_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','verification_hash','replica_epochs','sealed_at'])
 THEN RAISE EXCEPTION 'physical attempt identity and sealed evidence are immutable' USING ERRCODE='23514'; END IF;
 IF NEW.state='sealed' AND (OLD.state<>'writing' OR NOT EXISTS(SELECT 1 FROM monitoring_chunks c WHERE c.tenant_id=NEW.tenant_id AND c.id=NEW.chunk_id AND c.payload_hash=NEW.verification_hash))
 THEN RAISE EXCEPTION 'only verified writing attempts can be sealed' USING ERRCODE='23514'; END IF;
 IF OLD.state IN ('uncertain','abandoned') AND NEW.state<>'abandoned'
 THEN RAISE EXCEPTION 'uncertain attempts cannot be resumed' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER monitoring_grant_immutable BEFORE INSERT OR UPDATE OR DELETE ON monitoring_data_grants FOR EACH ROW EXECUTE FUNCTION monitoring_grant_guard();
CREATE TRIGGER monitoring_publication_immutable BEFORE UPDATE OR DELETE ON monitoring_publications FOR EACH ROW EXECUTE FUNCTION monitoring_publication_guard();
CREATE TRIGGER monitoring_attempt_selected BEFORE INSERT ON monitoring_publication_chunks FOR EACH ROW EXECUTE FUNCTION monitoring_selected_attempt_guard();
CREATE TRIGGER monitoring_attempt_immutable BEFORE UPDATE OR DELETE ON monitoring_write_attempts FOR EACH ROW EXECUTE FUNCTION monitoring_attempt_guard();
CREATE TRIGGER monitoring_upload_part_immutable BEFORE UPDATE OR DELETE ON monitoring_upload_parts FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_source_immutable BEFORE UPDATE OR DELETE ON monitoring_source_files FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_mapping_immutable BEFORE UPDATE OR DELETE ON monitoring_import_mappings FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_resolution_immutable BEFORE UPDATE OR DELETE ON monitoring_batch_resolutions FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_chunk_immutable BEFORE UPDATE OR DELETE ON monitoring_chunks FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_selection_immutable BEFORE UPDATE OR DELETE ON monitoring_publication_chunks FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_chunk_locations_immutable BEFORE UPDATE OR DELETE ON monitoring_chunk_locations FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_summary_replacements_immutable BEFORE UPDATE OR DELETE ON monitoring_summary_replacements FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_contribution_replacements_immutable BEFORE UPDATE OR DELETE ON monitoring_contribution_replacements FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_relocation_copies_immutable BEFORE UPDATE OR DELETE ON monitoring_relocation_copies FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_receipt_immutable BEFORE UPDATE OR DELETE ON monitoring_receipts FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_audit_immutable BEFORE UPDATE OR DELETE ON monitoring_audit FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER monitoring_channel_protected BEFORE UPDATE ON engineering_channels FOR EACH ROW EXECUTE FUNCTION monitoring_channel_reference_guard();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Monitoring evidence is permanent; use a verified restore or forward repair'; END $$;
-- +goose StatementEnd
