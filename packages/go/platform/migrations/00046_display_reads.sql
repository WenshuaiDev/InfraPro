-- +goose Up
-- Expand first. Existing format-1 rows and immutable publications are unchanged.
ALTER TABLE display_grants ADD COLUMN scope_format smallint NOT NULL DEFAULT 1 CHECK(scope_format IN (1,2));
ALTER TABLE display_grants ADD COLUMN read_scopes jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(read_scopes)='array');
ALTER TABLE display_configs ADD COLUMN config_format smallint NOT NULL DEFAULT 1 CHECK(config_format IN (1,2));
ALTER TABLE display_configs ADD COLUMN read_views jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(read_views)='object');
ALTER TABLE display_grants ADD CHECK(scope_format=2 OR read_scopes='[]'::jsonb);
ALTER TABLE display_configs ADD CHECK(config_format=2 OR read_views='{}'::jsonb);
ALTER TABLE monitoring_selections ADD COLUMN read_windows jsonb CHECK(read_windows IS NULL OR jsonb_typeof(read_windows)='object');

CREATE TABLE display_read_scope_sets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL,
 domain text NOT NULL CHECK(domain IN ('resources','channels','devices','spatial')),
 owner_kind text NOT NULL CHECK(owner_kind IN ('grant','config')), config_id uuid, grant_revision bigint,
 name text NOT NULL, definition jsonb NOT NULL, digest text NOT NULL, member_count integer NOT NULL CHECK(member_count BETWEEN 0 AND 50000),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((owner_kind='config' AND config_id IS NOT NULL AND grant_revision IS NULL) OR (owner_kind='grant' AND config_id IS NULL AND grant_revision>0)),
 UNIQUE(tenant_id,project_id,instance_id,id), UNIQUE(id,domain),
 FOREIGN KEY(tenant_id,project_id,instance_id) REFERENCES display_instances(tenant_id,project_id,id),
 FOREIGN KEY(tenant_id,project_id,instance_id,config_id) REFERENCES display_configs(tenant_id,project_id,instance_id,id) DEFERRABLE INITIALLY DEFERRED
);
CREATE UNIQUE INDEX display_read_scope_config ON display_read_scope_sets(config_id,name) WHERE config_id IS NOT NULL;
CREATE UNIQUE INDEX display_read_scope_grant ON display_read_scope_sets(instance_id,grant_revision,name) WHERE grant_revision IS NOT NULL;
CREATE TABLE display_read_scope_members (
 set_id uuid NOT NULL, domain text NOT NULL, kind text NOT NULL, object_id uuid NOT NULL,
 PRIMARY KEY(set_id,kind,object_id), FOREIGN KEY(set_id,domain) REFERENCES display_read_scope_sets(id,domain),
 CHECK((domain='resources' AND kind IN ('facility','component','point','channel')) OR (domain='channels' AND kind='channel') OR (domain='devices' AND kind IN ('device','output')) OR (domain='spatial' AND kind='version'))
);
CREATE TABLE display_read_config_scopes (
 tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL, config_id uuid NOT NULL, name text NOT NULL, set_id uuid NOT NULL,
 PRIMARY KEY(config_id,name),
 FOREIGN KEY(tenant_id,project_id,instance_id,config_id) REFERENCES display_configs(tenant_id,project_id,instance_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(tenant_id,project_id,instance_id,set_id) REFERENCES display_read_scope_sets(tenant_id,project_id,instance_id,id)
);
CREATE TRIGGER display_read_set_immutable BEFORE UPDATE OR DELETE ON display_read_scope_sets FOR EACH ROW EXECUTE FUNCTION display_immutable_guard();
CREATE TRIGGER display_read_members_immutable BEFORE UPDATE OR DELETE ON display_read_scope_members FOR EACH ROW EXECUTE FUNCTION display_immutable_guard();
CREATE TRIGGER display_read_config_scope_immutable BEFORE UPDATE OR DELETE ON display_read_config_scopes FOR EACH ROW EXECUTE FUNCTION display_immutable_guard();

CREATE TABLE display_read_queries (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL,
 subject_id uuid NOT NULL, session_id uuid NOT NULL, config_id uuid NOT NULL, environment uuid NOT NULL,
 method text NOT NULL, request jsonb NOT NULL, scope_hash text NOT NULL, filter_hash text NOT NULL, authority_hash text NOT NULL,
 source_vector jsonb NOT NULL, scopes jsonb NOT NULL, included_field_groups text[] NOT NULL,
 state text NOT NULL CHECK(state IN ('queued','running','ready','partial','empty','failed','canceled','expired')),
 read_snapshot_id uuid NOT NULL DEFAULT gen_random_uuid(), metadata_as_of timestamptz NOT NULL DEFAULT clock_timestamp(),
 metadata jsonb NOT NULL DEFAULT '{}', completeness jsonb NOT NULL DEFAULT '{"scope":"complete","data":"pending","reasons":[]}',
 row_count bigint NOT NULL DEFAULT 0 CHECK(row_count>=0), result_bytes bigint NOT NULL DEFAULT 0 CHECK(result_bytes>=0),
 error_code text NOT NULL DEFAULT '', consumers integer NOT NULL DEFAULT 1 CHECK(consumers>=0),
 work jsonb, proof jsonb, parent_query_id uuid REFERENCES display_read_queries(id) ON DELETE CASCADE,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), completed_at timestamptz, expires_at timestamptz NOT NULL, canceled_at timestamptz,
 UNIQUE(tenant_id,project_id,instance_id,id), UNIQUE(id,subject_id),
 FOREIGN KEY(tenant_id,project_id,instance_id,subject_id) REFERENCES display_subjects(tenant_id,project_id,instance_id,id),
 FOREIGN KEY(tenant_id,project_id,instance_id,session_id) REFERENCES display_sessions(tenant_id,project_id,instance_id,id),
 FOREIGN KEY(tenant_id,project_id,instance_id,config_id) REFERENCES display_configs(tenant_id,project_id,instance_id,id)
);
CREATE INDEX display_read_query_expiry ON display_read_queries(expires_at);
CREATE INDEX display_read_query_subject ON display_read_queries(subject_id,id);
CREATE INDEX display_read_query_active ON display_read_queries(tenant_id,project_id,instance_id,state) WHERE state IN ('queued','running');
CREATE INDEX display_read_cache ON display_read_queries(session_id,method,scope_hash,filter_hash,completed_at DESC) WHERE state IN ('ready','partial','empty');
ALTER TABLE monitoring_snapshots ADD COLUMN display_managed boolean NOT NULL DEFAULT false;
CREATE TABLE display_read_monitoring_refs (
 query_id uuid PRIMARY KEY REFERENCES display_read_queries(id) ON DELETE CASCADE,
 snapshot_id uuid NOT NULL REFERENCES monitoring_snapshots(id) ON DELETE CASCADE
);
CREATE INDEX display_read_monitoring_snapshot ON display_read_monitoring_refs(snapshot_id);
ALTER TABLE display_subscriptions ADD COLUMN read_query_id uuid;
CREATE TABLE display_read_query_rows (
 query_id uuid NOT NULL REFERENCES display_read_queries(id) ON DELETE CASCADE, ordinal bigint NOT NULL CHECK(ordinal>=0),
 sort_key text NOT NULL, data jsonb NOT NULL, PRIMARY KEY(query_id,ordinal)
);
CREATE TABLE display_read_cursors (
 token text PRIMARY KEY, query_id uuid NOT NULL REFERENCES display_read_queries(id) ON DELETE CASCADE,
 ordinal bigint NOT NULL CHECK(ordinal>=0), page_size integer NOT NULL CHECK(page_size BETWEEN 1 AND 1000),
 UNIQUE(query_id,ordinal,page_size)
);
CREATE TABLE display_read_query_sections (
 query_id uuid NOT NULL REFERENCES display_read_queries(id) ON DELETE CASCADE,
 section text NOT NULL CHECK(section IN ('coverage','effective_windows')),
 ordinal bigint NOT NULL CHECK(ordinal>=0),data jsonb NOT NULL,PRIMARY KEY(query_id,section,ordinal)
);
CREATE TABLE display_read_section_cursors (
 token text PRIMARY KEY,query_id uuid NOT NULL REFERENCES display_read_queries(id) ON DELETE CASCADE,
 section text NOT NULL,ordinal bigint NOT NULL CHECK(ordinal>=0),page_size integer NOT NULL CHECK(page_size BETWEEN 1 AND 1000),
 UNIQUE(query_id,section,ordinal,page_size)
);
CREATE TABLE display_read_jobs (
 query_id uuid PRIMARY KEY REFERENCES display_read_queries(id) ON DELETE CASCADE,
 state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','done','failed','canceled')),
 worker text, fence bigint NOT NULL DEFAULT 0, attempts integer NOT NULL DEFAULT 0,
 lease_until timestamptz, available_at timestamptz NOT NULL DEFAULT clock_timestamp(), deadline timestamptz NOT NULL DEFAULT clock_timestamp()+interval '120 seconds',
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), first_started_at timestamptz, updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX display_read_job_claim ON display_read_jobs(state,lease_until,created_at) WHERE state IN ('queued','running');
CREATE TABLE display_read_selections (
 token text PRIMARY KEY, query_id uuid NOT NULL, subject_id uuid NOT NULL, group_key text NOT NULL,
 basis text NOT NULL CHECK(basis IN ('raw_active','raw_samples','derived_logical','derived_samples')),
 purpose text NOT NULL CHECK(purpose IN ('details','trends','statistics')),
 selection jsonb NOT NULL, expires_at timestamptz NOT NULL,
 FOREIGN KEY(query_id,subject_id) REFERENCES display_read_queries(id,subject_id) ON DELETE CASCADE
);

-- Monotonic domain revisions are safe cache hints, never the only authority.
CREATE TABLE display_read_revisions (
 tenant_id uuid NOT NULL REFERENCES tenants(id), domain text NOT NULL,
 revision bigint NOT NULL DEFAULT 1, safety_revision bigint NOT NULL DEFAULT 1, PRIMARY KEY(tenant_id,domain)
);

-- The owning write transaction advances durable invalidation evidence. Normal
-- metadata edits/new members do not rewrite existing query projections.
-- +goose StatementBegin
CREATE FUNCTION display_read_changed() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n jsonb:=to_jsonb(NEW); o jsonb:=to_jsonb(OLD); tid uuid; unsafe boolean:=false;
BEGIN
 tid:=COALESCE(n->>'tenant_id',o->>'tenant_id')::uuid;
 IF TG_ARGV[1]='relations' THEN unsafe:=true;
 ELSIF TG_ARGV[1]='spatial_grant' THEN
  unsafe:=TG_OP='DELETE' OR (TG_OP='UPDATE' AND (n-'is_default'-'revision') IS DISTINCT FROM (o-'is_default'-'revision'));
 ELSE
  unsafe:=TG_OP='DELETE' OR (TG_OP='UPDATE' AND (
   n->>'deleted_at' IS DISTINCT FROM o->>'deleted_at' OR
   (n->>'state' IN ('withdrawn','missing') AND n->>'state' IS DISTINCT FROM o->>'state') OR
   (n->>'content_available'='false' AND n->>'content_available' IS DISTINCT FROM o->>'content_available')));
 END IF;
 INSERT INTO display_read_revisions(tenant_id,domain,revision,safety_revision) VALUES(tid,TG_ARGV[0],1,CASE WHEN unsafe THEN 1 ELSE 0 END)
 ON CONFLICT(tenant_id,domain) DO UPDATE SET revision=display_read_revisions.revision+1,safety_revision=display_read_revisions.safety_revision+CASE WHEN unsafe THEN 1 ELSE 0 END;
 RETURN COALESCE(NEW,OLD);
END $$;
-- +goose StatementEnd
CREATE TRIGGER display_read_facility AFTER INSERT OR UPDATE OR DELETE ON engineering_facilities FOR EACH ROW EXECUTE FUNCTION display_read_changed('resources','metadata');
CREATE TRIGGER display_read_component AFTER INSERT OR UPDATE OR DELETE ON engineering_components FOR EACH ROW EXECUTE FUNCTION display_read_changed('resources','metadata');
CREATE TRIGGER display_read_point AFTER INSERT OR UPDATE OR DELETE ON engineering_points FOR EACH ROW EXECUTE FUNCTION display_read_changed('resources','metadata');
CREATE TRIGGER display_read_channel AFTER INSERT OR UPDATE OR DELETE ON engineering_channels FOR EACH ROW EXECUTE FUNCTION display_read_changed('resources','metadata');
CREATE TRIGGER display_read_device AFTER INSERT OR UPDATE OR DELETE ON devices FOR EACH ROW EXECUTE FUNCTION display_read_changed('devices','metadata');
CREATE TRIGGER display_read_output AFTER INSERT OR UPDATE OR DELETE ON device_outputs FOR EACH ROW EXECUTE FUNCTION display_read_changed('outputs','relations');
CREATE TRIGGER display_read_installation AFTER INSERT OR UPDATE OR DELETE ON device_installations FOR EACH ROW EXECUTE FUNCTION display_read_changed('installations','relations');
CREATE TRIGGER display_read_source AFTER INSERT OR UPDATE OR DELETE ON monitoring_source_bindings FOR EACH ROW EXECUTE FUNCTION display_read_changed('sources','relations');
CREATE TRIGGER display_read_spatial_asset AFTER INSERT OR UPDATE OR DELETE ON spatial_assets FOR EACH ROW EXECUTE FUNCTION display_read_changed('spatial','metadata');
CREATE TRIGGER display_read_spatial_version AFTER INSERT OR UPDATE OR DELETE ON spatial_asset_versions FOR EACH ROW EXECUTE FUNCTION display_read_changed('spatial','metadata');
CREATE TRIGGER display_read_spatial_grant AFTER INSERT OR UPDATE OR DELETE ON project_spatial_grants FOR EACH ROW EXECUTE FUNCTION display_read_changed('spatial_grants','spatial_grant');
CREATE TRIGGER display_read_spatial_setting AFTER INSERT OR UPDATE OR DELETE ON project_spatial_views FOR EACH ROW EXECUTE FUNCTION display_read_changed('spatial_settings','metadata');

-- +goose Down
-- Binary downgrade is an operator procedure, not destructive schema rollback.
-- Refuse while a format-2 configuration is retained, including withdrawn history.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM display_configs WHERE config_format=2) OR EXISTS(SELECT 1 FROM display_grants WHERE scope_format=2) THEN
  RAISE EXCEPTION 'format-2 display data must be retained; use documented binary downgrade without dropping schema';
 END IF;
END $$;
-- +goose StatementEnd
DROP TRIGGER display_read_facility ON engineering_facilities;
DROP TRIGGER display_read_component ON engineering_components;
DROP TRIGGER display_read_point ON engineering_points;
DROP TRIGGER display_read_channel ON engineering_channels;
DROP TRIGGER display_read_device ON devices;
DROP TRIGGER display_read_output ON device_outputs;
DROP TRIGGER display_read_installation ON device_installations;
DROP TRIGGER display_read_source ON monitoring_source_bindings;
DROP TRIGGER display_read_spatial_asset ON spatial_assets;
DROP TRIGGER display_read_spatial_version ON spatial_asset_versions;
DROP TRIGGER display_read_spatial_grant ON project_spatial_grants;
DROP TRIGGER display_read_spatial_setting ON project_spatial_views;
DROP FUNCTION display_read_changed();
DROP TABLE display_read_revisions,display_read_selections,display_read_jobs,display_read_section_cursors,display_read_query_sections,display_read_cursors,display_read_query_rows,display_read_monitoring_refs,display_read_queries,display_read_config_scopes,display_read_scope_members,display_read_scope_sets;
ALTER TABLE monitoring_snapshots DROP COLUMN display_managed;
ALTER TABLE display_subscriptions DROP COLUMN read_query_id;
ALTER TABLE display_configs DROP COLUMN read_views, DROP COLUMN config_format;
ALTER TABLE display_grants DROP COLUMN read_scopes, DROP COLUMN scope_format;
ALTER TABLE monitoring_selections DROP COLUMN read_windows;
