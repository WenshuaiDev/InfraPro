-- +goose Up
-- Actual resources are tenant-owned. Deleted identities and codes remain reserved.
CREATE TABLE engineering_facilities (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
 code_normalized text GENERATED ALWAYS AS (lower(code COLLATE "C")) STORED,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 status text NOT NULL DEFAULT 'normal' CHECK(status IN ('normal','retired')),
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id), deleted_at timestamptz,
 values jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(values)='object'),
 position jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(position)='object'),
 type_version_id uuid NOT NULL REFERENCES engineering_versions(id), structure_revision integer NOT NULL DEFAULT 1 CHECK(structure_revision>0),
 description text NOT NULL DEFAULT '', source_reference text NOT NULL DEFAULT '',
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,code_normalized)
);
CREATE TABLE engineering_components (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
 code_normalized text GENERATED ALWAYS AS (lower(code COLLATE "C")) STORED,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 status text NOT NULL DEFAULT 'normal' CHECK(status IN ('normal','retired')),
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id), deleted_at timestamptz,
 values jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(values)='object'),
 position jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(position)='object'),
 facility_id uuid NOT NULL, parent_component_id uuid, category_key text NOT NULL,
 description text NOT NULL DEFAULT '',
 UNIQUE(tenant_id,facility_id,id), UNIQUE(tenant_id,facility_id,code_normalized),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 FOREIGN KEY(tenant_id,facility_id,parent_component_id) REFERENCES engineering_components(tenant_id,facility_id,id)
);
CREATE TABLE engineering_points (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
 code_normalized text GENERATED ALWAYS AS (lower(code COLLATE "C")) STORED,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 status text NOT NULL DEFAULT 'normal' CHECK(status IN ('normal','retired')),
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id), deleted_at timestamptz,
 position jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(position)='object'),
 facility_id uuid NOT NULL, component_id uuid,
 purpose text NOT NULL DEFAULT '', direction_description text NOT NULL DEFAULT '', reference_description text NOT NULL DEFAULT '', source_reference text NOT NULL DEFAULT '',
 UNIQUE(tenant_id,facility_id,id), UNIQUE(tenant_id,facility_id,code_normalized),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 FOREIGN KEY(tenant_id,facility_id,component_id) REFERENCES engineering_components(tenant_id,facility_id,id)
);
CREATE TABLE engineering_channels (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
 code_normalized text GENERATED ALWAYS AS (lower(code COLLATE "C")) STORED,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 status text NOT NULL DEFAULT 'normal' CHECK(status IN ('normal','retired')),
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id), deleted_at timestamptz,
 facility_id uuid NOT NULL, point_id uuid NOT NULL,
 metric_version_id uuid NOT NULL REFERENCES engineering_versions(id), unit_version_id uuid REFERENCES engineering_versions(id),
 observation_direction text NOT NULL DEFAULT '', reference_basis text NOT NULL DEFAULT '',
 timestamp_meaning text NOT NULL CHECK(timestamp_meaning IN ('observation_time','interval_end')),
 aggregation_description text NOT NULL DEFAULT '', sampling jsonb CHECK(sampling IS NULL OR jsonb_typeof(sampling)='object'), description text NOT NULL DEFAULT '',
 CHECK(timestamp_meaning<>'interval_end' OR length(btrim(aggregation_description))>0),
 UNIQUE(tenant_id,facility_id,id), UNIQUE(tenant_id,point_id,code_normalized),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 FOREIGN KEY(tenant_id,facility_id,point_id) REFERENCES engineering_points(tenant_id,facility_id,id)
);
CREATE INDEX resources_facility_list ON engineering_facilities(tenant_id,status,created_at,id);
CREATE INDEX resources_component_children ON engineering_components(tenant_id,facility_id,parent_component_id,created_at,id);
CREATE INDEX resources_points_owner ON engineering_points(tenant_id,facility_id,component_id,created_at,id);
CREATE INDEX resources_channels_point ON engineering_channels(tenant_id,point_id,created_at,id);
CREATE INDEX resources_type_reference ON engineering_facilities(type_version_id);
CREATE INDEX resources_metric_reference ON engineering_channels(metric_version_id);
CREATE INDEX resources_unit_reference ON engineering_channels(unit_version_id);
CREATE TABLE engineering_resource_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, facility_id uuid NOT NULL,
 resource_kind text NOT NULL CHECK(resource_kind IN ('facility','component','point','channel')), resource_id uuid NOT NULL,
 resource_revision integer NOT NULL CHECK(resource_revision>0), operation text NOT NULL,
 actor_id uuid NOT NULL REFERENCES accounts(id), actor_name text NOT NULL, request_id uuid NOT NULL,
 changed_at timestamptz NOT NULL DEFAULT clock_timestamp(), reason text NOT NULL,
 before jsonb, after jsonb NOT NULL, changed_fields jsonb NOT NULL, definition_versions jsonb NOT NULL,
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 UNIQUE(tenant_id,resource_kind,resource_id,resource_revision)
);
CREATE INDEX resources_changes_cursor ON engineering_resource_changes(tenant_id,facility_id,changed_at,id);
CREATE TABLE engineering_resource_receipts (
 tenant_id uuid NOT NULL REFERENCES tenants(id), actor_id uuid NOT NULL REFERENCES accounts(id), request_id uuid NOT NULL,
 operation text NOT NULL, payload_hash text NOT NULL, result jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(tenant_id,actor_id,request_id)
);
-- +goose StatementBegin
CREATE FUNCTION resource_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n jsonb; o jsonb; fixed text; fid uuid; parent uuid; typ jsonb; metric jsonb; relation jsonb; parent_category text; maximum integer; count_normal integer;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'resource identities cannot be erased' USING ERRCODE='23514'; END IF;
 n:=to_jsonb(NEW);
 IF TG_OP='UPDATE' THEN
  o:=to_jsonb(OLD);
  FOREACH fixed IN ARRAY ARRAY['id','tenant_id','facility_id','code','type_version_id','category_key','point_id','metric_version_id','unit_version_id','observation_direction','reference_basis','timestamp_meaning','aggregation_description','created_at','created_by'] LOOP
   IF n->fixed IS DISTINCT FROM o->fixed THEN RAISE EXCEPTION 'resource identity or semantics are immutable' USING ERRCODE='23514'; END IF;
  END LOOP;
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
  IF NOT EXISTS(SELECT 1 FROM engineering_points WHERE id=NEW.point_id AND tenant_id=NEW.tenant_id AND facility_id=fid AND deleted_at IS NULL)
  THEN RAISE EXCEPTION 'invalid channel owner' USING ERRCODE='23514'; END IF;
  SELECT v.content->'metric' INTO metric FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE v.id=NEW.metric_version_id AND d.kind='metrics' AND d.tenant_id IS NULL;
  IF metric IS NULL OR (metric->>'shape'='enum' AND NEW.unit_version_id IS NOT NULL) OR (metric->>'shape'<>'enum' AND (NEW.unit_version_id IS NULL OR NOT (metric->'allowed_units' ? NEW.unit_version_id::text)))
  THEN RAISE EXCEPTION 'invalid channel metric or unit' USING ERRCODE='23514'; END IF;
  IF NEW.unit_version_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE v.id=NEW.unit_version_id AND d.kind='units' AND d.tenant_id IS NULL AND v.content->'unit'->>'category'=metric->>'category')
  THEN RAISE EXCEPTION 'invalid channel dimension' USING ERRCODE='23514'; END IF;
 END IF;
 IF NEW.deleted_at IS NOT NULL THEN
  IF TG_TABLE_NAME='engineering_facilities' THEN
   IF EXISTS(SELECT 1 FROM engineering_components WHERE facility_id=fid AND deleted_at IS NULL) OR EXISTS(SELECT 1 FROM engineering_points WHERE facility_id=fid AND deleted_at IS NULL)
   THEN RAISE EXCEPTION 'facility referenced' USING ERRCODE='23514'; END IF;
  ELSIF TG_TABLE_NAME='engineering_components' THEN
   IF EXISTS(SELECT 1 FROM engineering_components WHERE parent_component_id=NEW.id AND deleted_at IS NULL) OR EXISTS(SELECT 1 FROM engineering_points WHERE component_id=NEW.id AND deleted_at IS NULL)
   THEN RAISE EXCEPTION 'component referenced' USING ERRCODE='23514'; END IF;
  ELSIF TG_TABLE_NAME='engineering_points' THEN
   IF EXISTS(SELECT 1 FROM engineering_channels WHERE point_id=NEW.id AND deleted_at IS NULL)
   THEN RAISE EXCEPTION 'point referenced' USING ERRCODE='23514'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION resource_change_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_table text; target jsonb;
BEGIN
 target_table:=CASE NEW.resource_kind WHEN 'facility' THEN 'engineering_facilities' WHEN 'component' THEN 'engineering_components' WHEN 'point' THEN 'engineering_points' WHEN 'channel' THEN 'engineering_channels' END;
 EXECUTE format('SELECT to_jsonb(r) FROM %I r WHERE id=$1 AND tenant_id=$2',target_table) INTO target USING NEW.resource_id,NEW.tenant_id;
 IF target IS NULL OR COALESCE(target->>'facility_id',target->>'id')<>NEW.facility_id::text OR (target->>'revision')::integer<>NEW.resource_revision
 THEN RAISE EXCEPTION 'history target scope mismatch' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER resource_facilities_guard BEFORE INSERT OR UPDATE OR DELETE ON engineering_facilities FOR EACH ROW EXECUTE FUNCTION resource_guard();
CREATE TRIGGER resource_components_guard BEFORE INSERT OR UPDATE OR DELETE ON engineering_components FOR EACH ROW EXECUTE FUNCTION resource_guard();
CREATE TRIGGER resource_points_guard BEFORE INSERT OR UPDATE OR DELETE ON engineering_points FOR EACH ROW EXECUTE FUNCTION resource_guard();
CREATE TRIGGER resource_channels_guard BEFORE INSERT OR UPDATE OR DELETE ON engineering_channels FOR EACH ROW EXECUTE FUNCTION resource_guard();
CREATE TRIGGER resource_change_scope BEFORE INSERT ON engineering_resource_changes FOR EACH ROW EXECUTE FUNCTION resource_change_guard();
CREATE TRIGGER resource_change_immutable BEFORE UPDATE OR DELETE ON engineering_resource_changes FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER resource_receipt_immutable BEFORE UPDATE OR DELETE ON engineering_resource_receipts FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Resource identities and history are irreversible; restore a verified backup'; END $$;
-- +goose StatementEnd
