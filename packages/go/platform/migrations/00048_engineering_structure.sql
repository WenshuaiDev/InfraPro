-- +goose Up
-- Independent identities, never special component categories.
CREATE TABLE engineering_monitoring_sections (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), facility_id uuid NOT NULL,
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
 code_normalized text GENERATED ALWAYS AS (lower(code COLLATE "C")) STORED,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 status text NOT NULL DEFAULT 'normal' CHECK(status IN ('normal','retired')), revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id), deleted_at timestamptz,
 created_project_id uuid, values jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(values)='object'),
 position jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(position)='object'),
 description text NOT NULL DEFAULT '', purpose text NOT NULL DEFAULT '', source_reference text NOT NULL DEFAULT '',
 UNIQUE(tenant_id,facility_id,id), UNIQUE(tenant_id,facility_id,code_normalized),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 FOREIGN KEY(tenant_id,created_project_id) REFERENCES projects(tenant_id,id)
);
CREATE TABLE engineering_boreholes (LIKE engineering_monitoring_sections INCLUDING DEFAULTS INCLUDING GENERATED INCLUDING CONSTRAINTS INCLUDING INDEXES);
ALTER TABLE engineering_boreholes ADD FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 ADD FOREIGN KEY(tenant_id,created_project_id) REFERENCES projects(tenant_id,id),
 ADD FOREIGN KEY(tenant_id) REFERENCES tenants(id),
 ADD FOREIGN KEY(created_by) REFERENCES accounts(id), ADD FOREIGN KEY(updated_by) REFERENCES accounts(id),
 ADD COLUMN borehole jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(borehole)='object');
CREATE INDEX structure_section_directory ON engineering_monitoring_sections(tenant_id,facility_id,created_at,id);
CREATE INDEX structure_borehole_directory ON engineering_boreholes(tenant_id,facility_id,created_at,id);
CREATE TABLE engineering_structure_nodes (
 tenant_id uuid NOT NULL, facility_id uuid NOT NULL, kind text NOT NULL CHECK(kind IN ('facility','component','monitoring_section','borehole','point')), id uuid NOT NULL,
 parent_kind text, parent_id uuid, revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 PRIMARY KEY(tenant_id,facility_id,kind,id),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 FOREIGN KEY(tenant_id,facility_id,parent_kind,parent_id) REFERENCES engineering_structure_nodes(tenant_id,facility_id,kind,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((kind='facility' AND id=facility_id AND parent_kind IS NULL AND parent_id IS NULL) OR
       (kind<>'facility' AND parent_kind IN ('facility','component','monitoring_section','borehole') AND parent_id IS NOT NULL)),
 CHECK(kind IS DISTINCT FROM parent_kind OR id<>parent_id)
);
CREATE INDEX structure_children ON engineering_structure_nodes(tenant_id,facility_id,parent_kind,parent_id,kind,id);
INSERT INTO engineering_structure_nodes(tenant_id,facility_id,kind,id,parent_kind,parent_id)
 SELECT tenant_id,id,'facility',id,NULL,NULL FROM engineering_facilities
 UNION ALL SELECT tenant_id,facility_id,'component',id,CASE WHEN parent_component_id IS NULL THEN 'facility' ELSE 'component' END,COALESCE(parent_component_id,facility_id) FROM engineering_components
 UNION ALL SELECT tenant_id,facility_id,'point',id,CASE WHEN component_id IS NULL THEN 'facility' ELSE 'component' END,COALESCE(component_id,facility_id) FROM engineering_points;
CREATE TABLE facility_type_adoptions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, facility_id uuid NOT NULL,
 from_version_id uuid NOT NULL REFERENCES engineering_versions(id), to_version_id uuid NOT NULL REFERENCES engineering_versions(id),
 resource_revision integer NOT NULL, structure_revision integer NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id),
 request_id uuid NOT NULL, reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 2000),
 preview_hash text NOT NULL, evidence jsonb NOT NULL, adopted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 UNIQUE(tenant_id,facility_id,resource_revision), UNIQUE(tenant_id,actor_id,request_id), CHECK(from_version_id<>to_version_id)
);
CREATE TRIGGER facility_type_adoption_immutable BEFORE UPDATE OR DELETE ON facility_type_adoptions FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
ALTER TABLE engineering_channels ADD COLUMN monitoring_section_id uuid, ADD COLUMN borehole_id uuid,
 ADD FOREIGN KEY(tenant_id,facility_id,monitoring_section_id) REFERENCES engineering_monitoring_sections(tenant_id,facility_id,id),
 ADD FOREIGN KEY(tenant_id,facility_id,borehole_id) REFERENCES engineering_boreholes(tenant_id,facility_id,id),
 DROP CONSTRAINT engineering_channels_owner_kind_check, DROP CONSTRAINT channel_owner_shape,
 ADD CHECK(owner_kind IN ('facility','component','monitoring_section','borehole','point')),
 ADD CONSTRAINT channel_owner_shape CHECK(
 (owner_kind='facility' AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id)=0) OR
 (owner_kind='component' AND component_id IS NOT NULL AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id)=1) OR
 (owner_kind='monitoring_section' AND monitoring_section_id IS NOT NULL AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id)=1) OR
 (owner_kind='borehole' AND borehole_id IS NOT NULL AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id)=1) OR
 (owner_kind='point' AND point_id IS NOT NULL AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id)=1));
CREATE UNIQUE INDEX derived_section_channel_code ON engineering_channels(tenant_id,monitoring_section_id,code_normalized) WHERE owner_kind='monitoring_section';
CREATE UNIQUE INDEX derived_borehole_channel_code ON engineering_channels(tenant_id,borehole_id,code_normalized) WHERE owner_kind='borehole';
-- Preserve a specifically named legacy projection, including original IDs.
CREATE VIEW engineering_resource_nodes_legacy AS SELECT * FROM engineering_resource_nodes;
CREATE OR REPLACE VIEW engineering_resource_nodes AS
 SELECT r.tenant_id,r.id AS facility_id,'facility'::text AS kind,r.id,n.parent_kind,n.parent_id,r.code,r.name,r.status,r.deleted_at,r.created_at,r.revision FROM engineering_facilities r JOIN engineering_structure_nodes n ON (n.tenant_id,n.facility_id,n.kind,n.id)=(r.tenant_id,r.id,'facility',r.id)
 UNION ALL SELECT r.tenant_id,r.facility_id,'component',r.id,n.parent_kind,n.parent_id,r.code,r.name,r.status,r.deleted_at,r.created_at,r.revision FROM engineering_components r JOIN engineering_structure_nodes n ON (n.tenant_id,n.facility_id,n.kind,n.id)=(r.tenant_id,r.facility_id,'component',r.id)
 UNION ALL SELECT r.tenant_id,r.facility_id,'point',r.id,n.parent_kind,n.parent_id,r.code,r.name,r.status,r.deleted_at,r.created_at,r.revision FROM engineering_points r JOIN engineering_structure_nodes n ON (n.tenant_id,n.facility_id,n.kind,n.id)=(r.tenant_id,r.facility_id,'point',r.id)
 UNION ALL SELECT r.tenant_id,r.facility_id,'monitoring_section',r.id,n.parent_kind,n.parent_id,r.code,r.name,r.status,r.deleted_at,r.created_at,r.revision FROM engineering_monitoring_sections r JOIN engineering_structure_nodes n ON (n.tenant_id,n.facility_id,n.kind,n.id)=(r.tenant_id,r.facility_id,'monitoring_section',r.id)
 UNION ALL SELECT r.tenant_id,r.facility_id,'borehole',r.id,n.parent_kind,n.parent_id,r.code,r.name,r.status,r.deleted_at,r.created_at,r.revision FROM engineering_boreholes r JOIN engineering_structure_nodes n ON (n.tenant_id,n.facility_id,n.kind,n.id)=(r.tenant_id,r.facility_id,'borehole',r.id)
 UNION ALL SELECT tenant_id,facility_id,'channel',id,owner_kind,COALESCE(point_id,component_id,monitoring_section_id,borehole_id,facility_id),code,name,status,deleted_at,created_at,revision FROM engineering_channels;
-- The old view must not be a wrapper around the now-extended view.
CREATE OR REPLACE VIEW engineering_resource_nodes_legacy AS
 SELECT tenant_id,id AS facility_id,'facility'::text AS kind,id,NULL::text AS parent_kind,NULL::uuid AS parent_id,code,name,status,deleted_at,created_at,revision FROM engineering_facilities
 UNION ALL SELECT tenant_id,facility_id,'component',id,CASE WHEN parent_component_id IS NULL THEN 'facility' ELSE 'component' END,COALESCE(parent_component_id,facility_id),code,name,status,deleted_at,created_at,revision FROM engineering_components
 UNION ALL SELECT tenant_id,facility_id,'point',id,CASE WHEN component_id IS NULL THEN 'facility' ELSE 'component' END,COALESCE(component_id,facility_id),code,name,status,deleted_at,created_at,revision FROM engineering_points
 UNION ALL SELECT tenant_id,facility_id,'channel',id,owner_kind,COALESCE(point_id,component_id,facility_id),code,name,status,deleted_at,created_at,revision FROM engineering_channels WHERE owner_kind IN ('facility','component','point');
-- +goose StatementBegin
CREATE FUNCTION engineering_structure_ancestors(tid uuid,fid uuid,k text,rid uuid) RETURNS TABLE(kind text,id uuid,depth integer) LANGUAGE sql STABLE AS $$
 WITH RECURSIVE path(kind,id,depth,seen) AS (
 SELECT n.parent_kind,n.parent_id,1,ARRAY[n.kind||':'||n.id::text] FROM engineering_structure_nodes n WHERE n.tenant_id=tid AND n.facility_id=fid AND n.kind=k AND n.id=rid AND n.parent_id IS NOT NULL
 UNION ALL SELECT n.parent_kind,n.parent_id,p.depth+1,p.seen||(n.kind||':'||n.id::text) FROM path p JOIN engineering_structure_nodes n ON n.tenant_id=tid AND n.facility_id=fid AND n.kind=p.kind AND n.id=p.id WHERE n.parent_id IS NOT NULL AND NOT n.kind||':'||n.id::text=ANY(p.seen)
 ) SELECT kind,id,depth FROM path
$$;
CREATE FUNCTION engineering_nearest_structure(tid uuid,fid uuid,k text,rid uuid,want text) RETURNS uuid LANGUAGE sql STABLE AS $$
 SELECT id FROM engineering_structure_ancestors(tid,fid,k,rid) WHERE kind=want ORDER BY depth LIMIT 1
$$;
CREATE FUNCTION engineering_structure_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p record; depth integer; current_kind text; current_id uuid; entity text; found_id uuid; typ jsonb;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'structure identities are retained' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND (NEW.tenant_id,NEW.facility_id,NEW.kind,NEW.id) IS DISTINCT FROM (OLD.tenant_id,OLD.facility_id,OLD.kind,OLD.id) THEN RAISE EXCEPTION 'structure identity is immutable' USING ERRCODE='23514'; END IF;
 PERFORM id FROM engineering_facilities WHERE tenant_id=NEW.tenant_id AND id=NEW.facility_id FOR UPDATE;
 IF NEW.kind='facility' THEN RETURN NEW; END IF;
 entity:=CASE NEW.kind WHEN 'component' THEN 'engineering_components' WHEN 'point' THEN 'engineering_points' WHEN 'monitoring_section' THEN 'engineering_monitoring_sections' WHEN 'borehole' THEN 'engineering_boreholes' END;
 EXECUTE format('SELECT id FROM %I WHERE tenant_id=$1 AND facility_id=$2 AND id=$3',entity) INTO found_id USING NEW.tenant_id,NEW.facility_id,NEW.id;
 IF found_id IS NULL THEN RAISE EXCEPTION 'structure target missing' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' AND (SELECT count(*) FROM engineering_structure_nodes WHERE tenant_id=NEW.tenant_id AND facility_id=NEW.facility_id)+(SELECT count(*) FROM engineering_channels WHERE tenant_id=NEW.tenant_id AND facility_id=NEW.facility_id)>=100000 THEN RAISE EXCEPTION 'structure resource budget 100000 exceeded' USING ERRCODE='23514'; END IF;
 current_kind:=NEW.parent_kind; current_id:=NEW.parent_id; depth:=0;
 LOOP
  depth:=depth+1;
  IF depth>512 THEN RAISE EXCEPTION 'structure depth budget 512 exceeded' USING ERRCODE='23514'; END IF;
  IF current_kind=NEW.kind AND current_id=NEW.id THEN RAISE EXCEPTION 'structure cycle' USING ERRCODE='23514'; END IF;
  SELECT n.parent_kind,n.parent_id,r.deleted_at INTO p FROM engineering_structure_nodes n JOIN engineering_resource_nodes r USING(tenant_id,facility_id,kind,id) WHERE n.tenant_id=NEW.tenant_id AND n.facility_id=NEW.facility_id AND n.kind=current_kind AND n.id=current_id;
  IF NOT FOUND OR p.deleted_at IS NOT NULL OR current_kind='point' THEN RAISE EXCEPTION 'invalid structure parent' USING ERRCODE='23514'; END IF;
  EXIT WHEN current_kind='facility';
  current_kind:=p.parent_kind; current_id:=p.parent_id;
 END LOOP;
 RETURN NEW;
END $$;
CREATE FUNCTION engineering_structure_projection() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 -- Canonical edges own these compatibility columns; callers never edit both facts.
 WITH RECURSIVE affected(kind,id) AS (SELECT NEW.kind,NEW.id UNION ALL SELECT n.kind,n.id FROM engineering_structure_nodes n JOIN affected a ON n.parent_kind=a.kind AND n.parent_id=a.id WHERE n.tenant_id=NEW.tenant_id AND n.facility_id=NEW.facility_id)
 UPDATE engineering_components c SET parent_component_id=engineering_nearest_structure(c.tenant_id,c.facility_id,'component',c.id,'component') FROM affected a WHERE a.kind='component' AND a.id=c.id AND c.tenant_id=NEW.tenant_id AND c.facility_id=NEW.facility_id AND c.deleted_at IS NULL AND c.parent_component_id IS DISTINCT FROM engineering_nearest_structure(c.tenant_id,c.facility_id,'component',c.id,'component');
 WITH RECURSIVE affected(kind,id) AS (SELECT NEW.kind,NEW.id UNION ALL SELECT n.kind,n.id FROM engineering_structure_nodes n JOIN affected a ON n.parent_kind=a.kind AND n.parent_id=a.id WHERE n.tenant_id=NEW.tenant_id AND n.facility_id=NEW.facility_id)
 UPDATE engineering_points p SET component_id=engineering_nearest_structure(p.tenant_id,p.facility_id,'point',p.id,'component') FROM affected a WHERE a.kind='point' AND a.id=p.id AND p.tenant_id=NEW.tenant_id AND p.facility_id=NEW.facility_id AND p.deleted_at IS NULL AND p.component_id IS DISTINCT FROM engineering_nearest_structure(p.tenant_id,p.facility_id,'point',p.id,'component');
 RETURN NULL;
END $$;
CREATE FUNCTION engineering_structure_entity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE k text; fid uuid; pk text; pid uuid; expected uuid; old_parent uuid; n record;
BEGIN
 k:=CASE TG_TABLE_NAME WHEN 'engineering_facilities' THEN 'facility' WHEN 'engineering_components' THEN 'component' WHEN 'engineering_points' THEN 'point' WHEN 'engineering_monitoring_sections' THEN 'monitoring_section' WHEN 'engineering_boreholes' THEN 'borehole' END;
 fid:=COALESCE((to_jsonb(NEW)->>'facility_id')::uuid,NEW.id);
 IF TG_OP='INSERT' THEN
  IF k='facility' THEN pk:=NULL;pid:=NULL;
  ELSE pid:=COALESCE((to_jsonb(NEW)->>'parent_component_id')::uuid,(to_jsonb(NEW)->>'component_id')::uuid);pk:=CASE WHEN pid IS NULL THEN 'facility' ELSE 'component' END;pid:=COALESCE(pid,fid); END IF;
  INSERT INTO engineering_structure_nodes(tenant_id,facility_id,kind,id,parent_kind,parent_id) VALUES(NEW.tenant_id,fid,k,NEW.id,pk,pid);
 ELSIF k IN ('component','point') THEN
  pid:=COALESCE((to_jsonb(NEW)->>'parent_component_id')::uuid,(to_jsonb(NEW)->>'component_id')::uuid);
  old_parent:=COALESCE((to_jsonb(OLD)->>'parent_component_id')::uuid,(to_jsonb(OLD)->>'component_id')::uuid);
  IF pid IS DISTINCT FROM old_parent THEN
   expected:=engineering_nearest_structure(NEW.tenant_id,fid,k,NEW.id,'component');
   IF pid IS DISTINCT FROM expected THEN
    SELECT * INTO n FROM engineering_structure_nodes WHERE tenant_id=NEW.tenant_id AND facility_id=fid AND kind=k AND id=NEW.id;
    IF n.parent_kind NOT IN ('facility','component') THEN RAISE EXCEPTION 'legacy parent write is ambiguous; use parent_ref' USING ERRCODE='23514'; END IF;
    pk:=CASE WHEN pid IS NULL THEN 'facility' ELSE 'component' END;
    UPDATE engineering_structure_nodes SET parent_kind=pk,parent_id=COALESCE(pid,fid),revision=revision+1 WHERE tenant_id=NEW.tenant_id AND facility_id=fid AND kind=k AND id=NEW.id;
   END IF;
  END IF;
 END IF;
 RETURN NULL;
END $$;
CREATE FUNCTION engineering_structure_entity_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE k text; fid uuid; typ jsonb; f text;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'engineering identities are retained' USING ERRCODE='23514'; END IF;
 k:=CASE TG_TABLE_NAME WHEN 'engineering_facilities' THEN 'facility' WHEN 'engineering_components' THEN 'component' WHEN 'engineering_points' THEN 'point' WHEN 'engineering_monitoring_sections' THEN 'monitoring_section' WHEN 'engineering_boreholes' THEN 'borehole' END;
 fid:=COALESCE((to_jsonb(NEW)->>'facility_id')::uuid,NEW.id);
 IF k IN ('monitoring_section','borehole') THEN
  PERFORM id FROM engineering_facilities WHERE tenant_id=NEW.tenant_id AND id=fid AND deleted_at IS NULL FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'invalid facility' USING ERRCODE='23514'; END IF;
  SELECT v.content INTO typ FROM engineering_facilities r JOIN engineering_versions v ON v.id=r.type_version_id WHERE r.tenant_id=NEW.tenant_id AND r.id=fid;
  IF typ->>'metamodel_version'<>'2' OR typ#>>ARRAY['structure',k,'supported'] IS DISTINCT FROM 'true' THEN RAISE EXCEPTION 'node kind not supported by adopted type' USING ERRCODE='23514'; END IF;
  IF TG_OP='UPDATE' THEN
   FOREACH f IN ARRAY ARRAY['id','tenant_id','facility_id','code','created_at','created_by','created_project_id'] LOOP
    IF to_jsonb(NEW)->f IS DISTINCT FROM to_jsonb(OLD)->f THEN RAISE EXCEPTION 'engineering identity immutable' USING ERRCODE='23514'; END IF;
   END LOOP;
   IF OLD.deleted_at IS NOT NULL THEN RAISE EXCEPTION 'deleted identity immutable' USING ERRCODE='23514'; END IF;
  END IF;
 END IF;
 IF NEW.deleted_at IS NOT NULL AND (EXISTS(SELECT 1 FROM engineering_resource_references WHERE tenant_id=NEW.tenant_id AND facility_id=fid AND ((source_kind=k AND source_id=NEW.id) OR (target_kind=k AND target_id=NEW.id))) OR EXISTS(SELECT 1 FROM point_borehole_positions WHERE tenant_id=NEW.tenant_id AND facility_id=fid AND ((k='point' AND point_id=NEW.id) OR (k='borehole' AND borehole_id=NEW.id)))) THEN RAISE EXCEPTION 'retained structure reference prevents deletion' USING ERRCODE='23514'; END IF;
 IF NEW.deleted_at IS NOT NULL AND EXISTS(SELECT 1 FROM engineering_resource_nodes WHERE tenant_id=NEW.tenant_id AND facility_id=fid AND parent_kind=k AND parent_id=NEW.id AND deleted_at IS NULL) THEN RAISE EXCEPTION 'structure children prevent deletion' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER structure_guard BEFORE INSERT OR UPDATE OR DELETE ON engineering_structure_nodes FOR EACH ROW EXECUTE FUNCTION engineering_structure_guard();
CREATE TRIGGER structure_projection AFTER UPDATE OF parent_kind,parent_id ON engineering_structure_nodes FOR EACH ROW EXECUTE FUNCTION engineering_structure_projection();
CREATE TRIGGER structure_facility_entity AFTER INSERT OR UPDATE ON engineering_facilities FOR EACH ROW EXECUTE FUNCTION engineering_structure_entity();
CREATE TRIGGER structure_component_entity AFTER INSERT OR UPDATE ON engineering_components FOR EACH ROW EXECUTE FUNCTION engineering_structure_entity();
CREATE TRIGGER structure_point_entity AFTER INSERT OR UPDATE ON engineering_points FOR EACH ROW EXECUTE FUNCTION engineering_structure_entity();
CREATE TRIGGER structure_section_entity AFTER INSERT OR UPDATE ON engineering_monitoring_sections FOR EACH ROW EXECUTE FUNCTION engineering_structure_entity();
CREATE TRIGGER structure_borehole_entity AFTER INSERT OR UPDATE ON engineering_boreholes FOR EACH ROW EXECUTE FUNCTION engineering_structure_entity();
CREATE TRIGGER structure_facility_guard BEFORE UPDATE ON engineering_facilities FOR EACH ROW EXECUTE FUNCTION engineering_structure_entity_guard();
CREATE TRIGGER structure_component_guard BEFORE UPDATE ON engineering_components FOR EACH ROW EXECUTE FUNCTION engineering_structure_entity_guard();
CREATE TRIGGER structure_point_guard BEFORE UPDATE ON engineering_points FOR EACH ROW EXECUTE FUNCTION engineering_structure_entity_guard();
CREATE TRIGGER structure_section_guard BEFORE INSERT OR UPDATE OR DELETE ON engineering_monitoring_sections FOR EACH ROW EXECUTE FUNCTION engineering_structure_entity_guard();
CREATE TRIGGER structure_borehole_guard BEFORE INSERT OR UPDATE OR DELETE ON engineering_boreholes FOR EACH ROW EXECUTE FUNCTION engineering_structure_entity_guard();

ALTER TABLE engineering_resource_changes DROP CONSTRAINT engineering_resource_changes_resource_kind_check,
 ADD CHECK(resource_kind IN ('facility','component','monitoring_section','borehole','point','channel'));
ALTER TABLE project_resource_grants ADD COLUMN monitoring_section_id uuid, ADD COLUMN borehole_id uuid,
 ADD COLUMN can_create_monitoring_section boolean NOT NULL DEFAULT false, ADD COLUMN can_create_borehole boolean NOT NULL DEFAULT false,
 ADD FOREIGN KEY(tenant_id,facility_id,monitoring_section_id) REFERENCES engineering_monitoring_sections(tenant_id,facility_id,id),
 ADD FOREIGN KEY(tenant_id,facility_id,borehole_id) REFERENCES engineering_boreholes(tenant_id,facility_id,id);
-- +goose StatementBegin
DO $$ DECLARE r record; BEGIN
 FOR r IN SELECT conname FROM pg_constraint WHERE conrelid='project_resource_grants'::regclass AND contype='c' AND (pg_get_constraintdef(oid) LIKE '%resource_kind%' OR pg_get_constraintdef(oid) LIKE '%can_create_derived_channel%') LOOP
  EXECUTE format('ALTER TABLE project_resource_grants DROP CONSTRAINT %I',r.conname);
 END LOOP;
END $$;
-- +goose StatementEnd
ALTER TABLE project_resource_grants ADD CHECK(resource_kind IN ('facility','component','monitoring_section','borehole','point','channel')),
 ADD CHECK((resource_kind='facility' AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id,channel_id)=0 AND NOT can_lifecycle) OR
 (resource_kind='component' AND component_id IS NOT NULL AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id,channel_id)=1) OR
 (resource_kind='monitoring_section' AND monitoring_section_id IS NOT NULL AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id,channel_id)=1) OR
 (resource_kind='borehole' AND borehole_id IS NOT NULL AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id,channel_id)=1) OR
 (resource_kind='point' AND point_id IS NOT NULL AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id,channel_id)=1) OR
 (resource_kind='channel' AND channel_id IS NOT NULL AND num_nonnulls(component_id,monitoring_section_id,borehole_id,point_id,channel_id)=1)),
 ADD CHECK(NOT (can_create_component OR can_create_point OR can_create_monitoring_section OR can_create_borehole) OR resource_kind IN ('facility','component','monitoring_section','borehole')),
 ADD CHECK(NOT can_create_channel OR resource_kind='point'),
 ADD CHECK(NOT can_create_derived_channel OR resource_kind<>'channel'),
 ADD CHECK(NOT can_manage_calculation OR resource_kind='channel'),
 ADD CHECK(NOT can_source_manage OR resource_kind='channel');
CREATE UNIQUE INDEX project_grant_section_current ON project_resource_grants(link_id,monitoring_section_id) WHERE resource_kind='monitoring_section' AND ended_at IS NULL;
CREATE UNIQUE INDEX project_grant_borehole_current ON project_resource_grants(link_id,borehole_id) WHERE resource_kind='borehole' AND ended_at IS NULL;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION resource_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n jsonb; o jsonb; fixed text; fid uuid; parent uuid; typ jsonb; metric jsonb; relation jsonb; parent_category text; maximum integer; count_normal integer;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'resource identities cannot be erased' USING ERRCODE='23514'; END IF;
 n:=to_jsonb(NEW);
 IF TG_OP='UPDATE' THEN
  o:=to_jsonb(OLD);
  FOREACH fixed IN ARRAY ARRAY['id','tenant_id','facility_id','code','category_key','point_id','monitoring_section_id','borehole_id','metric_version_id','unit_version_id','observation_direction','reference_basis','timestamp_meaning','aggregation_description','created_at','created_by','source_kind','owner_kind','derived_metric_version_id','quantity_spec','output_contract_hash','output_semantics'] LOOP
   IF n->fixed IS DISTINCT FROM o->fixed THEN RAISE EXCEPTION 'resource identity or semantics are immutable' USING ERRCODE='23514'; END IF;
  END LOOP;
  IF TG_TABLE_NAME='engineering_channels' AND n->'component_id' IS DISTINCT FROM o->'component_id' THEN RAISE EXCEPTION 'channel owner is immutable' USING ERRCODE='23514'; END IF;
  IF TG_TABLE_NAME='engineering_facilities' AND n->'type_version_id' IS DISTINCT FROM o->'type_version_id' AND NOT EXISTS(SELECT 1 FROM facility_type_adoptions a JOIN engineering_versions x ON x.id=a.from_version_id JOIN engineering_versions y ON y.id=a.to_version_id WHERE a.tenant_id=NEW.tenant_id AND a.facility_id=NEW.id AND a.from_version_id=(o->>'type_version_id')::uuid AND a.to_version_id=(n->>'type_version_id')::uuid AND a.resource_revision=NEW.revision AND x.definition_id=y.definition_id) THEN RAISE EXCEPTION 'type change requires explicit compatible adoption' USING ERRCODE='23514'; END IF;
  IF OLD.deleted_at IS NOT NULL THEN RAISE EXCEPTION 'deleted resource is immutable' USING ERRCODE='23514'; END IF;
 END IF;
 IF TG_TABLE_NAME='engineering_facilities' THEN
  fid:=NEW.id;
  IF NOT EXISTS(SELECT 1 FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE v.id=NEW.type_version_id AND d.kind='type-definitions' AND d.tenant_id IS NULL)
  THEN RAISE EXCEPTION 'invalid facility definition' USING ERRCODE='23514'; END IF;
 ELSE
  fid:=NEW.facility_id;
  PERFORM id FROM engineering_facilities WHERE id=fid AND tenant_id=NEW.tenant_id FOR UPDATE;
  IF NOT EXISTS(SELECT 1 FROM engineering_facilities WHERE id=fid AND tenant_id=NEW.tenant_id AND deleted_at IS NULL)
  THEN RAISE EXCEPTION 'invalid facility' USING ERRCODE='23514'; END IF;
 END IF;
 IF TG_TABLE_NAME='engineering_components' THEN
  parent:=NEW.parent_component_id;
  SELECT v.content INTO typ FROM engineering_facilities f JOIN engineering_versions v ON v.id=f.type_version_id WHERE f.id=fid;
  IF NOT EXISTS(SELECT 1 FROM jsonb_array_elements(typ->'components') c WHERE c->>'key'=NEW.category_key)
  THEN RAISE EXCEPTION 'invalid component category' USING ERRCODE='23514'; END IF;
  parent_category:='root';
  IF parent IS NOT NULL THEN
   SELECT category_key INTO parent_category FROM engineering_components WHERE id=parent AND tenant_id=NEW.tenant_id AND facility_id=fid AND deleted_at IS NULL;
   IF parent_category IS NULL THEN RAISE EXCEPTION 'invalid parent' USING ERRCODE='23514'; END IF;
   IF EXISTS(WITH RECURSIVE ancestors AS (SELECT id,parent_component_id FROM engineering_components WHERE id=parent UNION SELECT c.id,c.parent_component_id FROM engineering_components c JOIN ancestors a ON c.id=a.parent_component_id) SELECT 1 FROM ancestors WHERE id=NEW.id)
   THEN RAISE EXCEPTION 'component cycle' USING ERRCODE='23514'; END IF;
  END IF;
  SELECT r INTO relation FROM jsonb_array_elements(typ->'relations') r WHERE r->>'parent'=parent_category AND r->>'child'=NEW.category_key;
  IF relation IS NULL THEN RAISE EXCEPTION 'invalid component relation' USING ERRCODE='23514'; END IF;
  maximum:=(relation->>'max_count')::integer;
  IF NEW.status='normal' AND NEW.deleted_at IS NULL AND maximum IS NOT NULL THEN
   SELECT count(*) INTO count_normal FROM engineering_components WHERE tenant_id=NEW.tenant_id AND facility_id=fid AND parent_component_id IS NOT DISTINCT FROM parent AND category_key=NEW.category_key AND status='normal' AND deleted_at IS NULL AND id<>NEW.id;
   IF count_normal>=maximum THEN RAISE EXCEPTION 'component maximum exceeded' USING ERRCODE='23514'; END IF;
  END IF;
 ELSIF TG_TABLE_NAME='engineering_points' THEN
  IF NEW.component_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM engineering_components WHERE id=NEW.component_id AND tenant_id=NEW.tenant_id AND facility_id=fid AND deleted_at IS NULL)
  THEN RAISE EXCEPTION 'invalid point owner' USING ERRCODE='23514'; END IF;
 ELSIF TG_TABLE_NAME='engineering_channels' THEN
  IF NEW.source_kind='derived' THEN
   IF NEW.owner_kind='point' AND NOT EXISTS(SELECT 1 FROM engineering_points WHERE tenant_id=NEW.tenant_id AND facility_id=fid AND id=NEW.point_id AND deleted_at IS NULL)
    OR NEW.owner_kind='component' AND NOT EXISTS(SELECT 1 FROM engineering_components WHERE tenant_id=NEW.tenant_id AND facility_id=fid AND id=NEW.component_id AND deleted_at IS NULL)
   THEN RAISE EXCEPTION 'invalid derived channel owner' USING ERRCODE='23514'; END IF;
   IF NEW.owner_kind IN ('monitoring_section','borehole') AND NOT EXISTS(SELECT 1 FROM engineering_resource_nodes WHERE tenant_id=NEW.tenant_id AND facility_id=fid AND kind=NEW.owner_kind AND id=COALESCE(NEW.monitoring_section_id,NEW.borehole_id) AND deleted_at IS NULL) THEN RAISE EXCEPTION 'invalid derived structure owner' USING ERRCODE='23514'; END IF;
   IF NOT EXISTS(SELECT 1 FROM derived_metric_versions WHERE tenant_id=NEW.tenant_id AND id=NEW.derived_metric_version_id)
   THEN RAISE EXCEPTION 'derived output contract mismatch' USING ERRCODE='23514'; END IF;
  ELSE
  IF NOT EXISTS(SELECT 1 FROM engineering_points WHERE id=NEW.point_id AND tenant_id=NEW.tenant_id AND facility_id=fid AND deleted_at IS NULL)
  THEN RAISE EXCEPTION 'invalid channel owner' USING ERRCODE='23514'; END IF;
  SELECT v.content->'metric' INTO metric FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE v.id=NEW.metric_version_id AND d.kind='metrics' AND d.tenant_id IS NULL;
  IF metric IS NULL OR (metric->>'shape'='enum' AND NEW.unit_version_id IS NOT NULL) OR (metric->>'shape'<>'enum' AND (NEW.unit_version_id IS NULL OR NOT (metric->'allowed_units' ? NEW.unit_version_id::text)))
  THEN RAISE EXCEPTION 'invalid channel metric or unit' USING ERRCODE='23514'; END IF;
  IF NEW.unit_version_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE v.id=NEW.unit_version_id AND d.kind='units' AND d.tenant_id IS NULL AND v.content->'unit'->>'category'=metric->>'category')
  THEN RAISE EXCEPTION 'invalid channel dimension' USING ERRCODE='23514'; END IF;
  END IF;
 END IF;
 IF NEW.deleted_at IS NOT NULL THEN
  IF TG_TABLE_NAME='engineering_facilities' THEN
   IF EXISTS(SELECT 1 FROM engineering_components WHERE facility_id=fid AND deleted_at IS NULL) OR EXISTS(SELECT 1 FROM engineering_points WHERE facility_id=fid AND deleted_at IS NULL) OR EXISTS(SELECT 1 FROM engineering_channels WHERE facility_id=fid AND (deleted_at IS NULL OR source_kind='derived'))
   THEN RAISE EXCEPTION 'facility referenced' USING ERRCODE='23514'; END IF;
  ELSIF TG_TABLE_NAME='engineering_components' THEN
   IF EXISTS(SELECT 1 FROM engineering_components WHERE parent_component_id=NEW.id AND deleted_at IS NULL) OR EXISTS(SELECT 1 FROM engineering_points WHERE component_id=NEW.id AND deleted_at IS NULL) OR EXISTS(SELECT 1 FROM engineering_channels WHERE component_id=NEW.id AND (deleted_at IS NULL OR source_kind='derived'))
   THEN RAISE EXCEPTION 'component referenced' USING ERRCODE='23514'; END IF;
  ELSIF TG_TABLE_NAME='engineering_points' THEN
   IF EXISTS(SELECT 1 FROM engineering_channels WHERE point_id=NEW.id AND (deleted_at IS NULL OR source_kind='derived'))
   THEN RAISE EXCEPTION 'point referenced' USING ERRCODE='23514'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION resource_change_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_table text; target jsonb;
BEGIN
 target_table:=CASE NEW.resource_kind WHEN 'facility' THEN 'engineering_facilities' WHEN 'component' THEN 'engineering_components' WHEN 'point' THEN 'engineering_points' WHEN 'channel' THEN 'engineering_channels' WHEN 'monitoring_section' THEN 'engineering_monitoring_sections' WHEN 'borehole' THEN 'engineering_boreholes' END;
 EXECUTE format('SELECT to_jsonb(r) FROM %I r WHERE id=$1 AND tenant_id=$2',target_table) INTO target USING NEW.resource_id,NEW.tenant_id;
 IF target IS NULL OR COALESCE(target->>'facility_id',target->>'id')<>NEW.facility_id::text OR (target->>'revision')::integer<>NEW.resource_revision
 THEN RAISE EXCEPTION 'history target scope mismatch' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION project_resource_relation_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE field text; n jsonb; o jsonb; target_table text; target_id uuid; deleted timestamptz;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'resource access identities cannot be erased' USING ERRCODE='23514'; END IF;
 n:=to_jsonb(NEW);
 IF TG_OP='UPDATE' THEN
  o:=to_jsonb(OLD);
  FOREACH field IN ARRAY ARRAY['id','tenant_id','project_id','facility_id','link_id','resource_kind','component_id','point_id','channel_id','monitoring_section_id','borehole_id','source','created_at','created_by','granted_at','granted_by'] LOOP
   IF n->field IS DISTINCT FROM o->field THEN RAISE EXCEPTION 'resource access identity is immutable' USING ERRCODE='23514'; END IF;
  END LOOP;
  IF TG_TABLE_NAME='project_facility_links' AND o->>'status'='revoked' THEN RAISE EXCEPTION 'revoked link is immutable' USING ERRCODE='23514'; END IF;
  IF TG_TABLE_NAME='project_resource_grants' AND o->>'ended_at' IS NOT NULL THEN RAISE EXCEPTION 'ended grant is immutable' USING ERRCODE='23514'; END IF;
 END IF;
 IF TG_TABLE_NAME='project_resource_grants' AND n->>'ended_at' IS NULL THEN
  IF NOT EXISTS(SELECT 1 FROM project_facility_links WHERE id=NEW.link_id AND status='active') THEN RAISE EXCEPTION 'grant needs active link' USING ERRCODE='23514'; END IF;
  target_table:=CASE NEW.resource_kind WHEN 'facility' THEN 'engineering_facilities' WHEN 'component' THEN 'engineering_components' WHEN 'point' THEN 'engineering_points' WHEN 'channel' THEN 'engineering_channels' WHEN 'monitoring_section' THEN 'engineering_monitoring_sections' WHEN 'borehole' THEN 'engineering_boreholes' END;
  target_id:=COALESCE(NEW.component_id,NEW.point_id,NEW.channel_id,NEW.monitoring_section_id,NEW.borehole_id,NEW.facility_id);
  EXECUTE format('SELECT deleted_at FROM %I WHERE id=$1 AND tenant_id=$2 FOR SHARE',target_table) INTO deleted USING target_id,NEW.tenant_id;
  IF deleted IS NOT NULL THEN RAISE EXCEPTION 'deleted resources cannot be granted' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION project_resource_visibility(tid uuid,pid uuid,fid uuid) RETURNS TABLE(project_id uuid,link_id uuid,kind text,id uuid,full_access boolean) LANGUAGE sql STABLE AS $$
 WITH RECURSIVE visible(project_id,link_id,kind,id,full_access) AS (
 SELECT g.project_id,g.link_id,g.resource_kind,COALESCE(g.component_id,g.point_id,g.channel_id,g.monitoring_section_id,g.borehole_id,g.facility_id),true
 FROM project_resource_grants g JOIN project_facility_links l ON l.id=g.link_id AND l.status='active'
 WHERE g.tenant_id=tid AND (pid IS NULL OR g.project_id=pid) AND (fid IS NULL OR g.facility_id=fid) AND g.ended_at IS NULL
 UNION
 SELECT v.project_id,v.link_id,n.parent_kind,n.parent_id,false FROM visible v JOIN engineering_resource_nodes n ON n.tenant_id=tid AND n.kind=v.kind AND n.id=v.id
 WHERE n.parent_id IS NOT NULL
 ) SELECT project_id,link_id,kind,id,bool_or(full_access) FROM visible GROUP BY project_id,link_id,kind,id
$$;

CREATE FUNCTION engineering_structure_retained_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE k text; BEGIN
 k:=CASE TG_TABLE_NAME WHEN 'engineering_monitoring_sections' THEN 'monitoring_section' ELSE 'borehole' END;
 IF NEW.deleted_at IS NOT NULL AND (EXISTS(SELECT 1 FROM project_resource_grants WHERE tenant_id=NEW.tenant_id AND resource_kind=k AND COALESCE(monitoring_section_id,borehole_id)=NEW.id AND ended_at IS NULL) OR EXISTS(SELECT 1 FROM engineering_channels WHERE tenant_id=NEW.tenant_id AND owner_kind=k AND COALESCE(monitoring_section_id,borehole_id)=NEW.id)) THEN RAISE EXCEPTION 'structure identity has retained references' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER structure_section_retained BEFORE UPDATE ON engineering_monitoring_sections FOR EACH ROW EXECUTE FUNCTION engineering_structure_retained_guard();
CREATE TRIGGER structure_borehole_retained BEFORE UPDATE ON engineering_boreholes FOR EACH ROW EXECUTE FUNCTION engineering_structure_retained_guard();
CREATE VIEW engineering_resource_records AS
 SELECT r.tenant_id,r.id AS facility_id,'facility'::text AS kind,r.id,to_jsonb(r)||jsonb_build_object('kind','facility','parent_ref',CASE WHEN n.parent_id IS NULL THEN NULL ELSE jsonb_build_object('kind',n.parent_kind,'id',n.parent_id) END) AS data FROM engineering_facilities r JOIN engineering_structure_nodes n ON n.tenant_id=r.tenant_id AND n.facility_id=r.id AND n.kind='facility' AND n.id=r.id
 UNION ALL SELECT r.tenant_id,r.facility_id AS facility_id,'component'::text AS kind,r.id,to_jsonb(r)||jsonb_build_object('kind','component','parent_ref',CASE WHEN n.parent_id IS NULL THEN NULL ELSE jsonb_build_object('kind',n.parent_kind,'id',n.parent_id) END) AS data FROM engineering_components r JOIN engineering_structure_nodes n ON n.tenant_id=r.tenant_id AND n.facility_id=r.facility_id AND n.kind='component' AND n.id=r.id
 UNION ALL SELECT r.tenant_id,r.facility_id AS facility_id,'monitoring_section'::text AS kind,r.id,to_jsonb(r)||jsonb_build_object('kind','monitoring_section','parent_ref',CASE WHEN n.parent_id IS NULL THEN NULL ELSE jsonb_build_object('kind',n.parent_kind,'id',n.parent_id) END) AS data FROM engineering_monitoring_sections r JOIN engineering_structure_nodes n ON n.tenant_id=r.tenant_id AND n.facility_id=r.facility_id AND n.kind='monitoring_section' AND n.id=r.id
 UNION ALL SELECT r.tenant_id,r.facility_id AS facility_id,'borehole'::text AS kind,r.id,to_jsonb(r)||jsonb_build_object('kind','borehole','parent_ref',CASE WHEN n.parent_id IS NULL THEN NULL ELSE jsonb_build_object('kind',n.parent_kind,'id',n.parent_id) END) AS data FROM engineering_boreholes r JOIN engineering_structure_nodes n ON n.tenant_id=r.tenant_id AND n.facility_id=r.facility_id AND n.kind='borehole' AND n.id=r.id
 UNION ALL SELECT r.tenant_id,r.facility_id AS facility_id,'point'::text AS kind,r.id,to_jsonb(r)||jsonb_build_object('kind','point','parent_ref',CASE WHEN n.parent_id IS NULL THEN NULL ELSE jsonb_build_object('kind',n.parent_kind,'id',n.parent_id) END) AS data FROM engineering_points r JOIN engineering_structure_nodes n ON n.tenant_id=r.tenant_id AND n.facility_id=r.facility_id AND n.kind='point' AND n.id=r.id
 UNION ALL SELECT r.tenant_id,r.facility_id AS facility_id,'channel'::text AS kind,r.id,to_jsonb(r)||jsonb_build_object('kind','channel','parent_ref',jsonb_build_object('kind',r.owner_kind,'id',COALESCE(r.point_id,r.component_id,r.monitoring_section_id,r.borehole_id,r.facility_id))) AS data FROM engineering_channels r;
CREATE TABLE point_borehole_positions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, facility_id uuid NOT NULL, point_id uuid NOT NULL,
 borehole_id uuid NOT NULL, revision bigint NOT NULL CHECK(revision>0), distance_m double precision CHECK(distance_m>=0 AND distance_m<'Infinity'::float8),
 reference_start text NOT NULL, direction text NOT NULL, unit text NOT NULL CHECK(unit='m'),
 vertical_depth_m double precision CHECK(vertical_depth_m>=0 AND vertical_depth_m<'Infinity'::float8), vertical_datum text NOT NULL DEFAULT '',
 source text NOT NULL DEFAULT '', measured_at date, borehole_revision integer NOT NULL, consistency_reason text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), created_by uuid NOT NULL REFERENCES accounts(id), reason text NOT NULL,
 ended_at timestamptz, ended_reason text, FOREIGN KEY(tenant_id,facility_id,point_id) REFERENCES engineering_points(tenant_id,facility_id,id),
 FOREIGN KEY(tenant_id,facility_id,borehole_id) REFERENCES engineering_boreholes(tenant_id,facility_id,id),
 UNIQUE(tenant_id,point_id,revision), CHECK(distance_m IS NULL OR length(btrim(reference_start))>0 AND length(btrim(direction))>0),
 CHECK(vertical_depth_m IS NULL OR length(btrim(vertical_datum))>0), CHECK((ended_at IS NULL)=(ended_reason IS NULL))
);
CREATE UNIQUE INDEX point_borehole_position_current ON point_borehole_positions(tenant_id,point_id) WHERE ended_at IS NULL;
CREATE TABLE engineering_resource_references (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, facility_id uuid NOT NULL,
 source_kind text NOT NULL, source_id uuid NOT NULL, target_kind text NOT NULL, target_id uuid NOT NULL, purpose text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), created_by uuid NOT NULL REFERENCES accounts(id), ended_at timestamptz, ended_reason text,
 FOREIGN KEY(tenant_id,facility_id,source_kind,source_id) REFERENCES engineering_structure_nodes(tenant_id,facility_id,kind,id),
 FOREIGN KEY(tenant_id,facility_id,target_kind,target_id) REFERENCES engineering_structure_nodes(tenant_id,facility_id,kind,id),
 CHECK((source_kind,source_id)<>(target_kind,target_id)), CHECK(length(btrim(purpose)) BETWEEN 1 AND 2000)
);
CREATE UNIQUE INDEX engineering_reference_current ON engineering_resource_references(tenant_id,facility_id,source_kind,source_id,target_kind,target_id) WHERE ended_at IS NULL;
-- +goose StatementBegin
CREATE FUNCTION engineering_position_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'position history is retained' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.ended_at IS NOT NULL OR NEW.ended_at IS NULL OR (to_jsonb(NEW)-'ended_at'-'ended_reason') IS DISTINCT FROM (to_jsonb(OLD)-'ended_at'-'ended_reason') THEN RAISE EXCEPTION 'position history is immutable' USING ERRCODE='23514'; END IF;
 ELSE
  PERFORM id FROM engineering_facilities WHERE tenant_id=NEW.tenant_id AND id=NEW.facility_id FOR UPDATE;
  IF engineering_nearest_structure(NEW.tenant_id,NEW.facility_id,'point',NEW.point_id,'borehole') IS DISTINCT FROM NEW.borehole_id THEN RAISE EXCEPTION 'position must use nearest borehole' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION engineering_position_structure_integrity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM point_borehole_positions p WHERE p.tenant_id=NEW.tenant_id AND p.facility_id=NEW.facility_id AND p.ended_at IS NULL AND p.borehole_id IS DISTINCT FROM engineering_nearest_structure(p.tenant_id,p.facility_id,'point',p.point_id,'borehole')) THEN RAISE EXCEPTION 'move requires explicit position disposition' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE FUNCTION engineering_reference_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR TG_OP='UPDATE' AND (OLD.ended_at IS NOT NULL OR NEW.ended_at IS NULL OR (to_jsonb(NEW)-'ended_at'-'ended_reason') IS DISTINCT FROM (to_jsonb(OLD)-'ended_at'-'ended_reason')) THEN RAISE EXCEPTION 'reference history is retained' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' AND (NOT EXISTS(SELECT 1 FROM engineering_resource_nodes WHERE tenant_id=NEW.tenant_id AND facility_id=NEW.facility_id AND kind=NEW.source_kind AND id=NEW.source_id AND deleted_at IS NULL) OR NOT EXISTS(SELECT 1 FROM engineering_resource_nodes WHERE tenant_id=NEW.tenant_id AND facility_id=NEW.facility_id AND kind=NEW.target_kind AND id=NEW.target_id AND deleted_at IS NULL)) THEN RAISE EXCEPTION 'reference target unavailable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER point_borehole_position_guard BEFORE INSERT OR UPDATE OR DELETE ON point_borehole_positions FOR EACH ROW EXECUTE FUNCTION engineering_position_guard();
CREATE CONSTRAINT TRIGGER structure_position_integrity AFTER UPDATE ON engineering_structure_nodes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION engineering_position_structure_integrity();
CREATE TRIGGER engineering_reference_guard BEFORE INSERT OR UPDATE OR DELETE ON engineering_resource_references FOR EACH ROW EXECUTE FUNCTION engineering_reference_guard();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Mixed engineering structure is not writable by legacy programs; restore a verified compatible backup'; END $$;
-- +goose StatementEnd
