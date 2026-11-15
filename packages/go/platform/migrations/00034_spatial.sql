-- +goose Up
CREATE TABLE spatial_assets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 facility_id uuid, kind text NOT NULL CHECK(kind IN ('layout','model','geolayer')),
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200), category text NOT NULL DEFAULT 'other',
 source_note text NOT NULL DEFAULT '', status text NOT NULL DEFAULT 'normal' CHECK(status IN ('normal','retired')),
 current_version_id uuid, revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,facility_id,id), UNIQUE(tenant_id,code),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 CHECK(kind='geolayer' OR facility_id IS NOT NULL)
);
CREATE TABLE spatial_blobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 object_key text NOT NULL UNIQUE, sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 size bigint NOT NULL CHECK(size>0), media_type text NOT NULL, name text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(tenant_id,id)
);
CREATE TABLE spatial_asset_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, asset_id uuid NOT NULL,
 number integer NOT NULL CHECK(number>0), state text NOT NULL DEFAULT 'draft' CHECK(state IN ('draft','published','withdrawn')),
 base_version_id uuid, source_blob_id uuid, format text NOT NULL DEFAULT '',
 manifest jsonb NOT NULL DEFAULT '{}', manifest_hash text NOT NULL DEFAULT '',
 settings jsonb NOT NULL DEFAULT '{}', candidates jsonb NOT NULL DEFAULT '[]', change_note text NOT NULL DEFAULT '',
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), content_available boolean NOT NULL DEFAULT true,
 created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(), published_at timestamptz,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,asset_id,id), UNIQUE(tenant_id,asset_id,number),
 FOREIGN KEY(tenant_id,asset_id) REFERENCES spatial_assets(tenant_id,id),
 FOREIGN KEY(tenant_id,asset_id,base_version_id) REFERENCES spatial_asset_versions(tenant_id,asset_id,id),
 FOREIGN KEY(tenant_id,source_blob_id) REFERENCES spatial_blobs(tenant_id,id),
 CHECK(state='draft' OR (published_at IS NOT NULL AND source_blob_id IS NOT NULL AND manifest_hash ~ '^[a-f0-9]{64}$'))
);
ALTER TABLE spatial_assets ADD FOREIGN KEY(tenant_id,id,current_version_id) REFERENCES spatial_asset_versions(tenant_id,asset_id,id);
CREATE INDEX spatial_assets_page ON spatial_assets(tenant_id,kind,facility_id,created_at,id);
CREATE TABLE spatial_version_files (
 tenant_id uuid NOT NULL, version_id uuid NOT NULL, file_id text NOT NULL, blob_id uuid NOT NULL,
 PRIMARY KEY(tenant_id,version_id,file_id),
 FOREIGN KEY(tenant_id,version_id) REFERENCES spatial_asset_versions(tenant_id,id),
 FOREIGN KEY(tenant_id,blob_id) REFERENCES spatial_blobs(tenant_id,id)
);
CREATE TABLE spatial_point_placements (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, asset_id uuid NOT NULL, facility_id uuid NOT NULL,
 version_id uuid NOT NULL, point_id uuid NOT NULL, page_id text NOT NULL DEFAULT '',
 anchor jsonb NOT NULL, element_key text NOT NULL DEFAULT '', label_offset jsonb NOT NULL DEFAULT '[0,0]',
 main_channel_id uuid, basis_hash text NOT NULL, copied_from uuid,
 UNIQUE(version_id,page_id,point_id),
 FOREIGN KEY(tenant_id,facility_id,asset_id) REFERENCES spatial_assets(tenant_id,facility_id,id),
 FOREIGN KEY(tenant_id,asset_id,version_id) REFERENCES spatial_asset_versions(tenant_id,asset_id,id),
 FOREIGN KEY(tenant_id,facility_id,point_id) REFERENCES engineering_points(tenant_id,facility_id,id),
 FOREIGN KEY(tenant_id,facility_id,main_channel_id) REFERENCES engineering_channels(tenant_id,facility_id,id)
);
CREATE TABLE spatial_component_links (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, asset_id uuid NOT NULL, facility_id uuid NOT NULL,
 version_id uuid NOT NULL, component_id uuid NOT NULL, element_key text NOT NULL DEFAULT '', page_id text NOT NULL DEFAULT '',
 region jsonb NOT NULL DEFAULT '[]', basis_hash text NOT NULL, main_channel_id uuid,
 FOREIGN KEY(tenant_id,facility_id,asset_id) REFERENCES spatial_assets(tenant_id,facility_id,id),
 FOREIGN KEY(tenant_id,asset_id,version_id) REFERENCES spatial_asset_versions(tenant_id,asset_id,id),
 FOREIGN KEY(tenant_id,facility_id,component_id) REFERENCES engineering_components(tenant_id,facility_id,id),
 FOREIGN KEY(tenant_id,facility_id,main_channel_id) REFERENCES engineering_channels(tenant_id,facility_id,id)
);
CREATE UNIQUE INDEX spatial_element_owner ON spatial_component_links(version_id,element_key) WHERE element_key<>'';
CREATE TABLE spatial_feature_links (
 tenant_id uuid NOT NULL, version_id uuid NOT NULL, feature_key text NOT NULL, facility_id uuid NOT NULL,
 PRIMARY KEY(version_id,feature_key), FOREIGN KEY(tenant_id,version_id) REFERENCES spatial_asset_versions(tenant_id,id),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id)
);
CREATE TABLE spatial_uploads (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, version_id uuid NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id),
 filename text NOT NULL, size bigint NOT NULL CHECK(size BETWEEN 1 AND 268435456), sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 state text NOT NULL DEFAULT 'uploading' CHECK(state IN ('uploading','sealed','canceled')), revision bigint NOT NULL DEFAULT 1,
 source_blob_id uuid, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '24 hours',
 UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,version_id) REFERENCES spatial_asset_versions(tenant_id,id),
 FOREIGN KEY(tenant_id,source_blob_id) REFERENCES spatial_blobs(tenant_id,id)
);
CREATE UNIQUE INDEX spatial_upload_active ON spatial_uploads(version_id) WHERE state='uploading';
CREATE TABLE spatial_upload_parts (
 tenant_id uuid NOT NULL, upload_id uuid NOT NULL, part integer NOT NULL CHECK(part BETWEEN 1 AND 32),
 object_key text NOT NULL UNIQUE, size bigint NOT NULL CHECK(size BETWEEN 1 AND 8388608), sha256 text NOT NULL CHECK(sha256 ~ '^[a-f0-9]{64}$'),
 PRIMARY KEY(upload_id,part), FOREIGN KEY(tenant_id,upload_id) REFERENCES spatial_uploads(tenant_id,id)
);
CREATE TABLE spatial_processing_jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, version_id uuid NOT NULL, source_blob_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES accounts(id), membership_id uuid NOT NULL REFERENCES tenant_memberships(id), state text NOT NULL DEFAULT 'queued' CHECK(state IN ('queued','running','succeeded','failed','canceled')),
 stage text NOT NULL DEFAULT 'queued', error_code text NOT NULL DEFAULT '', report jsonb NOT NULL DEFAULT '{}',
 attempt integer NOT NULL DEFAULT 1, fence bigint NOT NULL DEFAULT 0, lease_until timestamptz, lease_owner text,
 revision bigint NOT NULL DEFAULT 1, version_revision bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,version_id) REFERENCES spatial_asset_versions(tenant_id,id), FOREIGN KEY(tenant_id,source_blob_id) REFERENCES spatial_blobs(tenant_id,id)
);
CREATE UNIQUE INDEX spatial_job_active ON spatial_processing_jobs(version_id) WHERE state IN ('queued','running');
CREATE INDEX spatial_job_queue ON spatial_processing_jobs(state,lease_until,created_at);
CREATE TABLE project_spatial_states (
 tenant_id uuid NOT NULL, project_id uuid NOT NULL, revision bigint NOT NULL DEFAULT 1,
 PRIMARY KEY(tenant_id,project_id), FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
CREATE TABLE project_spatial_grants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, asset_id uuid NOT NULL,
 version_id uuid NOT NULL, facility_id uuid, facility_link_id uuid, source_download boolean NOT NULL DEFAULT false, is_default boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1, created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(), ended_at timestamptz,
 FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id),
 FOREIGN KEY(tenant_id,asset_id,version_id) REFERENCES spatial_asset_versions(tenant_id,asset_id,id),
 FOREIGN KEY(tenant_id,facility_id,asset_id) REFERENCES spatial_assets(tenant_id,facility_id,id),
 FOREIGN KEY(tenant_id,project_id,facility_id,facility_link_id) REFERENCES project_facility_links(tenant_id,project_id,facility_id,id),
 CHECK((facility_id IS NULL)=(facility_link_id IS NULL))
);
CREATE UNIQUE INDEX spatial_grant_current ON project_spatial_grants(tenant_id,project_id,version_id) WHERE ended_at IS NULL;
CREATE UNIQUE INDEX spatial_grant_default ON project_spatial_grants(tenant_id,project_id,asset_id) WHERE ended_at IS NULL AND is_default;
CREATE TABLE project_spatial_views (
 tenant_id uuid NOT NULL, project_id uuid NOT NULL, id uuid NOT NULL, config jsonb NOT NULL,
 revision bigint NOT NULL DEFAULT 1, updated_by uuid NOT NULL REFERENCES accounts(id),
 PRIMARY KEY(tenant_id,project_id,id), FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
CREATE TABLE spatial_receipts (
 tenant_id uuid NOT NULL, scope_key text NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id), request_id uuid NOT NULL,
 membership_key text NOT NULL, operation text NOT NULL, payload_hash text NOT NULL, result jsonb NOT NULL,
 PRIMARY KEY(tenant_id,scope_key,actor_id,request_id)
);
CREATE TABLE spatial_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid,
 actor_id uuid REFERENCES accounts(id), operation text NOT NULL, target_id uuid NOT NULL, request_id uuid,
 reason text NOT NULL DEFAULT '', details jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION spatial_version_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.state<>'draft' THEN RAISE EXCEPTION 'published spatial history is retained' USING ERRCODE='23514'; END IF;
  RETURN OLD;
 END IF;
 IF NEW.tenant_id<>OLD.tenant_id OR NEW.asset_id<>OLD.asset_id OR NEW.id<>OLD.id OR NEW.number<>OLD.number THEN
  RAISE EXCEPTION 'spatial version identity is immutable' USING ERRCODE='23514';
 END IF;
 IF OLD.state<>'draft' AND (to_jsonb(NEW)-ARRAY['state','revision','content_available']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['state','revision','content_available']) THEN
  RAISE EXCEPTION 'published spatial content is immutable' USING ERRCODE='23514';
 END IF;
 IF OLD.state='withdrawn' AND NEW.state<>'withdrawn' OR OLD.state='published' AND NEW.state='draft' THEN
  RAISE EXCEPTION 'spatial publication cannot be undone' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER spatial_version_guard BEFORE UPDATE OR DELETE ON spatial_asset_versions FOR EACH ROW EXECUTE FUNCTION spatial_version_guard();
-- +goose StatementBegin
CREATE FUNCTION spatial_child_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE v uuid; t uuid;
BEGIN
 IF TG_OP='DELETE' THEN v:=OLD.version_id; t:=OLD.tenant_id; ELSE v:=NEW.version_id; t:=NEW.tenant_id; END IF;
 IF NOT EXISTS(SELECT 1 FROM spatial_asset_versions WHERE tenant_id=t AND id=v AND state='draft') THEN
  RAISE EXCEPTION 'only spatial drafts are editable' USING ERRCODE='23514';
 END IF;
 IF TG_OP='UPDATE' AND (NEW.version_id<>OLD.version_id OR NEW.tenant_id<>OLD.tenant_id) THEN
  RAISE EXCEPTION 'spatial relation version is immutable' USING ERRCODE='23514';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER spatial_file_guard BEFORE INSERT OR UPDATE OR DELETE ON spatial_version_files FOR EACH ROW EXECUTE FUNCTION spatial_child_guard();
CREATE TRIGGER spatial_placement_guard BEFORE INSERT OR UPDATE OR DELETE ON spatial_point_placements FOR EACH ROW EXECUTE FUNCTION spatial_child_guard();
CREATE TRIGGER spatial_link_guard BEFORE INSERT OR UPDATE OR DELETE ON spatial_component_links FOR EACH ROW EXECUTE FUNCTION spatial_child_guard();
CREATE TRIGGER spatial_feature_guard BEFORE INSERT OR UPDATE OR DELETE ON spatial_feature_links FOR EACH ROW EXECUTE FUNCTION spatial_child_guard();
-- +goose StatementBegin
CREATE FUNCTION spatial_asset_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.tenant_id<>OLD.tenant_id OR NEW.kind<>OLD.kind OR NEW.facility_id IS DISTINCT FROM OLD.facility_id OR NEW.code<>OLD.code THEN
  RAISE EXCEPTION 'spatial asset ownership is immutable' USING ERRCODE='23514';
 END IF;
 IF NEW.current_version_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM spatial_asset_versions WHERE tenant_id=NEW.tenant_id AND asset_id=NEW.id AND id=NEW.current_version_id AND state='published') THEN
  RAISE EXCEPTION 'current spatial version must be published' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER spatial_asset_guard BEFORE UPDATE ON spatial_assets FOR EACH ROW EXECUTE FUNCTION spatial_asset_guard();
-- +goose StatementBegin
CREATE FUNCTION spatial_link_ended() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status='revoked' AND OLD.status='active' THEN
  UPDATE project_spatial_grants SET ended_at=clock_timestamp(),revision=revision+1,is_default=false WHERE tenant_id=NEW.tenant_id AND project_id=NEW.project_id AND facility_link_id=NEW.id AND ended_at IS NULL;
  INSERT INTO project_spatial_states(tenant_id,project_id) VALUES(NEW.tenant_id,NEW.project_id) ON CONFLICT(tenant_id,project_id) DO UPDATE SET revision=project_spatial_states.revision+1;
 END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER spatial_link_ended AFTER UPDATE ON project_facility_links FOR EACH ROW EXECUTE FUNCTION spatial_link_ended();
-- +goose Down
SELECT 1/0; -- Published spatial history requires an explicit restore, never destructive rollback.
