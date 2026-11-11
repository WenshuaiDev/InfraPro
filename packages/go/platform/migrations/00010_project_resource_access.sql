-- +goose Up
CREATE TABLE project_resource_policies (
 tenant_id uuid NOT NULL, project_id uuid NOT NULL,
 allow_facility_create boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_by uuid REFERENCES accounts(id),
 PRIMARY KEY(tenant_id,project_id), FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
INSERT INTO project_resource_policies(tenant_id,project_id) SELECT tenant_id,id FROM projects;
CREATE TABLE project_facility_links (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, facility_id uuid NOT NULL,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','revoked')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), created_by uuid NOT NULL REFERENCES accounts(id),
 revoked_at timestamptz, revoked_by uuid REFERENCES accounts(id), revoke_reason text,
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 UNIQUE(tenant_id,project_id,facility_id,id),
 CHECK((status='active' AND revoked_at IS NULL AND revoked_by IS NULL AND revoke_reason IS NULL) OR
       (status='revoked' AND revoked_at IS NOT NULL AND revoked_by IS NOT NULL AND length(btrim(revoke_reason))>0))
);
CREATE UNIQUE INDEX project_facility_current ON project_facility_links(tenant_id,project_id,facility_id) WHERE status='active';
CREATE INDEX project_facility_reverse ON project_facility_links(tenant_id,facility_id,project_id) WHERE status='active';
CREATE INDEX project_facility_page ON project_facility_links(tenant_id,project_id,created_at,id);
CREATE TABLE project_resource_grants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, facility_id uuid NOT NULL, link_id uuid NOT NULL,
 resource_kind text NOT NULL CHECK(resource_kind IN ('facility','component','point','channel')),
 component_id uuid, point_id uuid, channel_id uuid,
 can_update boolean NOT NULL DEFAULT false, can_create_component boolean NOT NULL DEFAULT false,
 can_create_point boolean NOT NULL DEFAULT false, can_create_channel boolean NOT NULL DEFAULT false, can_lifecycle boolean NOT NULL DEFAULT false,
 source text NOT NULL CHECK(source IN ('tenant_grant','project_create')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), granted_at timestamptz NOT NULL DEFAULT clock_timestamp(), granted_by uuid NOT NULL REFERENCES accounts(id),
 ended_at timestamptz, ended_by uuid REFERENCES accounts(id), end_reason text,
 FOREIGN KEY(tenant_id,project_id,facility_id,link_id) REFERENCES project_facility_links(tenant_id,project_id,facility_id,id),
 FOREIGN KEY(tenant_id,facility_id,component_id) REFERENCES engineering_components(tenant_id,facility_id,id),
 FOREIGN KEY(tenant_id,facility_id,point_id) REFERENCES engineering_points(tenant_id,facility_id,id),
 FOREIGN KEY(tenant_id,facility_id,channel_id) REFERENCES engineering_channels(tenant_id,facility_id,id),
 CHECK((resource_kind='facility' AND component_id IS NULL AND point_id IS NULL AND channel_id IS NULL AND NOT can_create_channel AND NOT can_lifecycle) OR
       (resource_kind='component' AND component_id IS NOT NULL AND point_id IS NULL AND channel_id IS NULL AND NOT can_create_channel) OR
       (resource_kind='point' AND component_id IS NULL AND point_id IS NOT NULL AND channel_id IS NULL AND NOT can_create_component AND NOT can_create_point) OR
       (resource_kind='channel' AND component_id IS NULL AND point_id IS NULL AND channel_id IS NOT NULL AND NOT can_create_component AND NOT can_create_point AND NOT can_create_channel)),
 CHECK((ended_at IS NULL AND ended_by IS NULL AND end_reason IS NULL) OR (ended_at IS NOT NULL AND ended_by IS NOT NULL AND length(btrim(end_reason))>0))
);
CREATE UNIQUE INDEX project_grant_facility_current ON project_resource_grants(link_id) WHERE resource_kind='facility' AND ended_at IS NULL;
CREATE UNIQUE INDEX project_grant_component_current ON project_resource_grants(link_id,component_id) WHERE resource_kind='component' AND ended_at IS NULL;
CREATE UNIQUE INDEX project_grant_point_current ON project_resource_grants(link_id,point_id) WHERE resource_kind='point' AND ended_at IS NULL;
CREATE UNIQUE INDEX project_grant_channel_current ON project_resource_grants(link_id,channel_id) WHERE resource_kind='channel' AND ended_at IS NULL;
CREATE INDEX project_grant_scope ON project_resource_grants(tenant_id,project_id,facility_id,resource_kind) WHERE ended_at IS NULL;
ALTER TABLE project_facility_links ADD UNIQUE(tenant_id,project_id,id);
ALTER TABLE project_memberships ADD UNIQUE(tenant_id,project_id,account_id,tenant_membership_id,id);
CREATE TABLE project_resource_access_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, access_revision bigint NOT NULL,
 link_id uuid REFERENCES project_facility_links(id), operation text NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id),
 actor_scope text NOT NULL CHECK(actor_scope IN ('tenant','project')), actor_membership_id uuid NOT NULL,
 request_id uuid NOT NULL, reason text NOT NULL, before jsonb NOT NULL, after jsonb NOT NULL,
 changed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id),
 FOREIGN KEY(tenant_id,project_id,link_id) REFERENCES project_facility_links(tenant_id,project_id,id), UNIQUE(tenant_id,project_id,access_revision)
);
CREATE INDEX project_resource_access_history ON project_resource_access_changes(tenant_id,project_id,changed_at,id);
CREATE TABLE project_resource_operation_receipts (
 entry_scope text NOT NULL CHECK(entry_scope IN ('tenant','project')), tenant_id uuid NOT NULL, project_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES accounts(id), request_id uuid NOT NULL, operation text NOT NULL, payload_hash text NOT NULL,
 fingerprint_version integer NOT NULL DEFAULT 2 CHECK(fingerprint_version=2),
 tenant_membership_id uuid NOT NULL REFERENCES tenant_memberships(id), project_membership_id uuid REFERENCES project_memberships(id),
 link_id uuid REFERENCES project_facility_links(id), target_kind text NOT NULL, target_id uuid,
 result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(entry_scope,tenant_id,project_id,actor_id,request_id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id),
 FOREIGN KEY(tenant_id,actor_id,tenant_membership_id) REFERENCES tenant_memberships(tenant_id,account_id,id),
 FOREIGN KEY(tenant_id,project_id,actor_id,tenant_membership_id,project_membership_id) REFERENCES project_memberships(tenant_id,project_id,account_id,tenant_membership_id,id),
 FOREIGN KEY(tenant_id,project_id,link_id) REFERENCES project_facility_links(tenant_id,project_id,id),
 CHECK(entry_scope<>'project' OR (project_membership_id IS NOT NULL AND link_id IS NOT NULL))
);
ALTER TABLE engineering_facilities ADD COLUMN created_project_id uuid, ADD FOREIGN KEY(tenant_id,created_project_id) REFERENCES projects(tenant_id,id);
ALTER TABLE engineering_components ADD COLUMN created_project_id uuid, ADD FOREIGN KEY(tenant_id,created_project_id) REFERENCES projects(tenant_id,id);
ALTER TABLE engineering_points ADD COLUMN created_project_id uuid, ADD FOREIGN KEY(tenant_id,created_project_id) REFERENCES projects(tenant_id,id);
ALTER TABLE engineering_channels ADD COLUMN created_project_id uuid, ADD FOREIGN KEY(tenant_id,created_project_id) REFERENCES projects(tenant_id,id);
ALTER TABLE engineering_resource_changes
 ADD COLUMN actor_scope text NOT NULL DEFAULT 'tenant' CHECK(actor_scope IN ('tenant','project')),
 ADD COLUMN actor_project_id uuid, ADD FOREIGN KEY(tenant_id,actor_project_id) REFERENCES projects(tenant_id,id),
 ADD COLUMN actor_tenant_membership_id uuid REFERENCES tenant_memberships(id),
 ADD COLUMN actor_project_membership_id uuid REFERENCES project_memberships(id),
 ADD COLUMN actor_link_id uuid REFERENCES project_facility_links(id);
ALTER TABLE engineering_resource_receipts ADD COLUMN fingerprint_version integer NOT NULL DEFAULT 1 CHECK(fingerprint_version IN (1,2));
-- +goose StatementBegin
CREATE FUNCTION project_resource_relation_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE field text; n jsonb; o jsonb; target_table text; target_id uuid; deleted timestamptz;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'resource access identities cannot be erased' USING ERRCODE='23514'; END IF;
 n:=to_jsonb(NEW);
 IF TG_OP='UPDATE' THEN
  o:=to_jsonb(OLD);
  FOREACH field IN ARRAY ARRAY['id','tenant_id','project_id','facility_id','link_id','resource_kind','component_id','point_id','channel_id','source','created_at','created_by','granted_at','granted_by'] LOOP
   IF n->field IS DISTINCT FROM o->field THEN RAISE EXCEPTION 'resource access identity is immutable' USING ERRCODE='23514'; END IF;
  END LOOP;
  IF TG_TABLE_NAME='project_facility_links' AND o->>'status'='revoked' THEN RAISE EXCEPTION 'revoked link is immutable' USING ERRCODE='23514'; END IF;
  IF TG_TABLE_NAME='project_resource_grants' AND o->>'ended_at' IS NOT NULL THEN RAISE EXCEPTION 'ended grant is immutable' USING ERRCODE='23514'; END IF;
 END IF;
 IF TG_TABLE_NAME='project_resource_grants' AND n->>'ended_at' IS NULL THEN
  IF NOT EXISTS(SELECT 1 FROM project_facility_links WHERE id=NEW.link_id AND status='active') THEN RAISE EXCEPTION 'grant needs active link' USING ERRCODE='23514'; END IF;
  target_table:=CASE NEW.resource_kind WHEN 'facility' THEN 'engineering_facilities' WHEN 'component' THEN 'engineering_components' WHEN 'point' THEN 'engineering_points' WHEN 'channel' THEN 'engineering_channels' END;
  target_id:=COALESCE(NEW.component_id,NEW.point_id,NEW.channel_id,NEW.facility_id);
  EXECUTE format('SELECT deleted_at FROM %I WHERE id=$1 AND tenant_id=$2 FOR SHARE',target_table) INTO deleted USING target_id,NEW.tenant_id;
  IF deleted IS NOT NULL THEN RAISE EXCEPTION 'deleted resources cannot be granted' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION project_resource_link_integrity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE lid uuid; state text; root_count integer; grant_count integer;
BEGIN
 lid:=CASE WHEN TG_TABLE_NAME='project_facility_links' THEN (to_jsonb(NEW)->>'id')::uuid ELSE (to_jsonb(NEW)->>'link_id')::uuid END;
 SELECT status INTO state FROM project_facility_links WHERE id=lid;
 SELECT count(*), count(*) FILTER(WHERE resource_kind='facility') INTO grant_count,root_count FROM project_resource_grants WHERE link_id=lid AND ended_at IS NULL;
 IF (state='active' AND root_count<>1) OR (state='revoked' AND grant_count<>0) THEN RAISE EXCEPTION 'link and current grants inconsistent' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE FUNCTION resource_source_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.created_project_id IS DISTINCT FROM OLD.created_project_id THEN RAISE EXCEPTION 'resource creation source is immutable' USING ERRCODE='23514'; END IF;
 IF NEW.deleted_at IS NOT NULL AND EXISTS(SELECT 1 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active'
  WHERE g.ended_at IS NULL AND g.tenant_id=NEW.tenant_id AND
  ((TG_TABLE_NAME='engineering_facilities' AND g.facility_id=NEW.id) OR (TG_TABLE_NAME='engineering_components' AND g.component_id=NEW.id) OR
   (TG_TABLE_NAME='engineering_points' AND g.point_id=NEW.id) OR (TG_TABLE_NAME='engineering_channels' AND g.channel_id=NEW.id)))
 THEN RAISE EXCEPTION 'resource has current project references' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER resource_link_guard BEFORE UPDATE OR DELETE ON project_facility_links FOR EACH ROW EXECUTE FUNCTION project_resource_relation_guard();
CREATE TRIGGER resource_grant_guard BEFORE INSERT OR UPDATE OR DELETE ON project_resource_grants FOR EACH ROW EXECUTE FUNCTION project_resource_relation_guard();
CREATE CONSTRAINT TRIGGER resource_link_integrity AFTER INSERT OR UPDATE ON project_facility_links DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION project_resource_link_integrity();
CREATE CONSTRAINT TRIGGER resource_grant_integrity AFTER INSERT OR UPDATE ON project_resource_grants DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION project_resource_link_integrity();
CREATE TRIGGER resource_access_history_immutable BEFORE UPDATE OR DELETE ON project_resource_access_changes FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER resource_access_receipt_immutable BEFORE UPDATE OR DELETE ON project_resource_operation_receipts FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER resource_facility_source BEFORE UPDATE ON engineering_facilities FOR EACH ROW EXECUTE FUNCTION resource_source_guard();
CREATE TRIGGER resource_component_source BEFORE UPDATE ON engineering_components FOR EACH ROW EXECUTE FUNCTION resource_source_guard();
CREATE TRIGGER resource_point_source BEFORE UPDATE ON engineering_points FOR EACH ROW EXECUTE FUNCTION resource_source_guard();
CREATE TRIGGER resource_channel_source BEFORE UPDATE ON engineering_channels FOR EACH ROW EXECUTE FUNCTION resource_source_guard();
-- Parent edges for bounded SQL authorization, ancestor projection and shared impact.
CREATE VIEW engineering_resource_nodes AS
 SELECT tenant_id,id AS facility_id,'facility'::text AS kind,id,NULL::text AS parent_kind,NULL::uuid AS parent_id,code,name,status,deleted_at,created_at,revision FROM engineering_facilities
 UNION ALL SELECT tenant_id,facility_id,'component',id,CASE WHEN parent_component_id IS NULL THEN 'facility' ELSE 'component' END,COALESCE(parent_component_id,facility_id),code,name,status,deleted_at,created_at,revision FROM engineering_components
 UNION ALL SELECT tenant_id,facility_id,'point',id,CASE WHEN component_id IS NULL THEN 'facility' ELSE 'component' END,COALESCE(component_id,facility_id),code,name,status,deleted_at,created_at,revision FROM engineering_points
 UNION ALL SELECT tenant_id,facility_id,'channel',id,'point',point_id,code,name,status,deleted_at,created_at,revision FROM engineering_channels;
-- +goose StatementBegin
CREATE FUNCTION project_resource_visibility(tid uuid,pid uuid,fid uuid) RETURNS TABLE(project_id uuid,link_id uuid,kind text,id uuid,full_access boolean) LANGUAGE sql STABLE AS $$
 WITH RECURSIVE visible(project_id,link_id,kind,id,full_access) AS (
 SELECT g.project_id,g.link_id,g.resource_kind,COALESCE(g.component_id,g.point_id,g.channel_id,g.facility_id),true
 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active'
 WHERE g.tenant_id=tid AND (pid IS NULL OR g.project_id=pid) AND (fid IS NULL OR g.facility_id=fid) AND g.ended_at IS NULL
 UNION
 SELECT v.project_id,v.link_id,n.parent_kind,n.parent_id,false FROM visible v JOIN engineering_resource_nodes n ON n.tenant_id=tid AND n.kind=v.kind AND n.id=v.id
 WHERE n.parent_id IS NOT NULL
 ) SELECT project_id,link_id,kind,id,bool_or(full_access) FROM visible GROUP BY project_id,link_id,kind,id
$$;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Project resource access and history are irreversible; restore a verified backup'; END $$;
-- +goose StatementEnd
