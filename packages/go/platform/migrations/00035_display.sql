-- +goose Up
-- Durable display identity is separate from accounts. All cross-object links
-- carry tenant/project/instance to prevent accidental cross-project reuse.
CREATE TABLE display_environment (id int PRIMARY KEY CHECK(id=1), generation uuid NOT NULL DEFAULT gen_random_uuid());
INSERT INTO display_environment(id) VALUES(1);
CREATE TABLE display_settings (
 tenant_id uuid NOT NULL, project_id uuid NOT NULL, enabled boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), PRIMARY KEY(tenant_id,project_id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id)
);
CREATE TABLE display_instances (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120), state text NOT NULL DEFAULT 'enabled' CHECK(state IN ('enabled','disabled','archived')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), current_release_id uuid, draft_config_id uuid,
 viewers uuid[] NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,project_id,id), FOREIGN KEY(tenant_id,project_id) REFERENCES display_settings(tenant_id,project_id)
);
CREATE TABLE display_uploads (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES accounts(id), size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 134217728),
 sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'), state text NOT NULL DEFAULT 'receiving' CHECK(state IN ('receiving','uploaded','canceled','expired')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '24 hours',
 UNIQUE(tenant_id,project_id,instance_id,id), FOREIGN KEY(tenant_id,project_id,instance_id) REFERENCES display_instances(tenant_id,project_id,id)
);
CREATE TABLE display_upload_parts (
 upload_id uuid NOT NULL REFERENCES display_uploads(id), part int NOT NULL CHECK(part BETWEEN 0 AND 31),
 size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 1 AND 4194304), sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'), object_key text NOT NULL UNIQUE,
 PRIMARY KEY(upload_id,part)
);
CREATE TABLE display_package_jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), upload_id uuid NOT NULL UNIQUE REFERENCES display_uploads(id),
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','validating','ready','failed','canceled')),
 attempt int NOT NULL DEFAULT 0, fence bigint NOT NULL DEFAULT 0, lease_until timestamptz, error_code text NOT NULL DEFAULT '', issues jsonb NOT NULL DEFAULT '[]',
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE display_packages (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL,
 job_id uuid NOT NULL UNIQUE REFERENCES display_package_jobs(id), sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'), manifest jsonb NOT NULL,
 state text NOT NULL DEFAULT 'ready' CHECK(state IN ('ready','missing','reclaiming')), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,project_id,instance_id,id), FOREIGN KEY(tenant_id,project_id,instance_id) REFERENCES display_instances(tenant_id,project_id,id)
);
CREATE TABLE display_package_files (
 package_id uuid NOT NULL REFERENCES display_packages(id), path text NOT NULL, sha256 text NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'),
 size_bytes bigint NOT NULL CHECK(size_bytes BETWEEN 0 AND 33554432), media_type text NOT NULL, object_key text NOT NULL,
 PRIMARY KEY(package_id,path)
);
CREATE TABLE display_configs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL, package_id uuid NOT NULL,
 parameters jsonb NOT NULL, bindings jsonb NOT NULL, mode text NOT NULL CHECK(mode IN ('fixed','follow')),
 created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,project_id,instance_id,id), UNIQUE(tenant_id,project_id,instance_id,package_id,id),
 FOREIGN KEY(tenant_id,project_id,instance_id,package_id) REFERENCES display_packages(tenant_id,project_id,instance_id,id)
);
CREATE TABLE display_grants (
 instance_id uuid PRIMARY KEY, tenant_id uuid NOT NULL, project_id uuid NOT NULL, revision bigint NOT NULL DEFAULT 1,
 capabilities text[] NOT NULL DEFAULT '{}', scope jsonb NOT NULL DEFAULT '{}', fields text[] NOT NULL DEFAULT '{}',
 valid_until timestamptz NOT NULL, revoked boolean NOT NULL DEFAULT false, issued_by uuid NOT NULL REFERENCES accounts(id),
 FOREIGN KEY(tenant_id,project_id,instance_id) REFERENCES display_instances(tenant_id,project_id,id)
);
CREATE TABLE display_releases (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL,
 package_id uuid NOT NULL, config_id uuid NOT NULL, requirements jsonb NOT NULL, withdrawn boolean NOT NULL DEFAULT false,
 created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,project_id,instance_id,id),
 FOREIGN KEY(tenant_id,project_id,instance_id,package_id,config_id) REFERENCES display_configs(tenant_id,project_id,instance_id,package_id,id)
);
ALTER TABLE display_instances ADD FOREIGN KEY(tenant_id,project_id,id,current_release_id) REFERENCES display_releases(tenant_id,project_id,instance_id,id);
ALTER TABLE display_instances ADD FOREIGN KEY(tenant_id,project_id,id,draft_config_id) REFERENCES display_configs(tenant_id,project_id,instance_id,id);
CREATE TABLE display_terminals (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120), generation bigint NOT NULL DEFAULT 1,
 state text NOT NULL DEFAULT 'unpaired' CHECK(state IN ('unpaired','active','revoked')),
 token_hash text, pair_hash text, pair_expires_at timestamptz, valid_until timestamptz NOT NULL, environment uuid NOT NULL,
 actual_release_id uuid, last_seen_at timestamptz, last_error text NOT NULL DEFAULT '',
 UNIQUE(tenant_id,project_id,id), FOREIGN KEY(tenant_id,project_id,instance_id) REFERENCES display_instances(tenant_id,project_id,id),
 FOREIGN KEY(tenant_id,project_id,instance_id,actual_release_id) REFERENCES display_releases(tenant_id,project_id,instance_id,id)
);
CREATE TABLE display_subjects (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL,
 account_id uuid REFERENCES accounts(id), terminal_id uuid, person_session_id uuid,
 environment uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((account_id IS NOT NULL AND person_session_id IS NOT NULL AND terminal_id IS NULL) OR (account_id IS NULL AND person_session_id IS NULL AND terminal_id IS NOT NULL)),
 UNIQUE(tenant_id,project_id,id), UNIQUE(tenant_id,id), UNIQUE(tenant_id,project_id,instance_id,id),
 FOREIGN KEY(tenant_id,project_id,instance_id) REFERENCES display_instances(tenant_id,project_id,id),
 FOREIGN KEY(tenant_id,project_id,terminal_id) REFERENCES display_terminals(tenant_id,project_id,id)
);
CREATE TABLE display_sessions (
 last_data_at timestamptz, last_query_state text NOT NULL DEFAULT '', stream_connections bigint NOT NULL DEFAULT 0,
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL,
 subject_id uuid NOT NULL, release_id uuid, package_id uuid NOT NULL, config_id uuid NOT NULL, preview boolean NOT NULL DEFAULT false,
 token_hash text NOT NULL UNIQUE, asset_hash text NOT NULL UNIQUE, terminal_generation bigint,
 state text NOT NULL DEFAULT 'starting' CHECK(state IN ('starting','ready','degraded','closed','revoked','expired')),
 generation bigint NOT NULL DEFAULT 1, authorization_revision text NOT NULL, lease_until timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), last_seen_at timestamptz NOT NULL DEFAULT clock_timestamp(), last_error text NOT NULL DEFAULT '',
 UNIQUE(tenant_id,project_id,instance_id,id),
 FOREIGN KEY(tenant_id,project_id,instance_id,subject_id) REFERENCES display_subjects(tenant_id,project_id,instance_id,id),
 FOREIGN KEY(tenant_id,project_id,instance_id,release_id) REFERENCES display_releases(tenant_id,project_id,instance_id,id),
 FOREIGN KEY(tenant_id,project_id,instance_id,package_id,config_id) REFERENCES display_configs(tenant_id,project_id,instance_id,package_id,id)
);
CREATE TABLE display_pairings (
 code_hash text PRIMARY KEY, challenge text NOT NULL, audience text NOT NULL, session_id uuid NOT NULL REFERENCES display_sessions(id),
 expires_at timestamptz NOT NULL, used_at timestamptz
);
CREATE TABLE display_tool_sessions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL,
 account_id uuid NOT NULL REFERENCES accounts(id), person_session_id uuid NOT NULL, environment uuid NOT NULL,
 challenge text NOT NULL, audience text NOT NULL, code_hash text UNIQUE NOT NULL, token_hash text UNIQUE,
 pair_until timestamptz NOT NULL DEFAULT clock_timestamp()+interval '2 minutes', expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '10 minutes',
 FOREIGN KEY(tenant_id,project_id,instance_id) REFERENCES display_instances(tenant_id,project_id,id)
);
CREATE TABLE display_receipts (
 tenant_id uuid NOT NULL, project_id uuid NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id), request_id uuid NOT NULL,
 operation text NOT NULL, payload_hash text NOT NULL, result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,project_id,actor_id,request_id), FOREIGN KEY(tenant_id,project_id) REFERENCES display_settings(tenant_id,project_id)
);
CREATE TABLE display_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid,
 kind text NOT NULL, actor_id uuid REFERENCES accounts(id), details jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,project_id) REFERENCES display_settings(tenant_id,project_id),
 FOREIGN KEY(tenant_id,project_id,instance_id) REFERENCES display_instances(tenant_id,project_id,id)
);
CREATE INDEX display_events_scope ON display_events(tenant_id,project_id,id);
CREATE INDEX display_events_diagnostic ON display_events(instance_id,kind,created_at);
CREATE TABLE display_subscriptions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), session_id uuid NOT NULL REFERENCES display_sessions(id), query jsonb NOT NULL,
 generation bigint NOT NULL DEFAULT 1, sequence bigint NOT NULL DEFAULT 0, applied bigint NOT NULL DEFAULT 0,
 source_vector jsonb NOT NULL DEFAULT '{}', pending_query jsonb, state text NOT NULL DEFAULT 'active' CHECK(state IN ('active','invalid','canceled')),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), CHECK(applied<=sequence)
);
CREATE TABLE display_subscription_events (
 subscription_id uuid NOT NULL REFERENCES display_subscriptions(id), generation bigint NOT NULL, sequence bigint NOT NULL,
 event jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(subscription_id,generation,sequence)
);
-- Durable, mutually exclusive subject ownership; account queries are unchanged.
ALTER TABLE monitoring_selections ALTER COLUMN actor_id DROP NOT NULL;
ALTER TABLE monitoring_selections ADD COLUMN display_subject_id uuid;
ALTER TABLE monitoring_selections ADD FOREIGN KEY(tenant_id,project_id,display_subject_id) REFERENCES display_subjects(tenant_id,project_id,id);
ALTER TABLE monitoring_selections ADD CHECK(num_nonnulls(actor_id,display_subject_id)=1 AND (display_subject_id IS NULL OR project_id IS NOT NULL));
ALTER TABLE monitoring_snapshots ALTER COLUMN actor_id DROP NOT NULL;
ALTER TABLE monitoring_snapshots ADD COLUMN display_subject_id uuid;
ALTER TABLE monitoring_snapshots ADD FOREIGN KEY(tenant_id,display_subject_id) REFERENCES display_subjects(tenant_id,id);
ALTER TABLE monitoring_snapshots ADD CHECK(num_nonnulls(actor_id,display_subject_id)=1);
ALTER TABLE monitoring_jobs ADD COLUMN display_subject_id uuid;
ALTER TABLE monitoring_jobs ADD FOREIGN KEY(tenant_id,project_id,display_subject_id) REFERENCES display_subjects(tenant_id,project_id,id);
ALTER TABLE monitoring_jobs ADD CHECK(display_subject_id IS NULL OR (actor_id IS NULL AND project_id IS NOT NULL AND kind='summary' AND input->>'mode'='query'));
-- +goose StatementBegin
CREATE FUNCTION display_immutable_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR (to_jsonb(NEW)-TG_ARGV) IS DISTINCT FROM (to_jsonb(OLD)-TG_ARGV) THEN
 RAISE EXCEPTION 'display content is immutable' USING ERRCODE='23514'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER display_config_immutable BEFORE UPDATE OR DELETE ON display_configs FOR EACH ROW EXECUTE FUNCTION display_immutable_guard();
CREATE TRIGGER display_release_immutable BEFORE UPDATE OR DELETE ON display_releases FOR EACH ROW EXECUTE FUNCTION display_immutable_guard('withdrawn');
CREATE TRIGGER display_package_immutable BEFORE UPDATE ON display_packages FOR EACH ROW EXECUTE FUNCTION display_immutable_guard('state');
CREATE TRIGGER display_file_immutable BEFORE UPDATE ON display_package_files FOR EACH ROW EXECUTE FUNCTION display_immutable_guard();
CREATE FUNCTION display_snapshot_owner_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM monitoring_selections s WHERE s.tenant_id=NEW.tenant_id AND s.id=NEW.selection_id AND s.actor_id IS NOT DISTINCT FROM NEW.actor_id AND s.display_subject_id IS NOT DISTINCT FROM NEW.display_subject_id) THEN
 RAISE EXCEPTION 'snapshot owner differs from selection' USING ERRCODE='23514'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER display_snapshot_owner BEFORE INSERT OR UPDATE ON monitoring_snapshots FOR EACH ROW EXECUTE FUNCTION display_snapshot_owner_guard();
-- +goose StatementEnd

CREATE TABLE display_object_intents(object_key text PRIMARY KEY, expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '25 hours');

-- +goose Down
SELECT 1/0; -- Durable releases and read-subject ownership require an explicit restore.
