-- +goose Up
-- No catalog, hardware, installation, source or object grant is seeded.
CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public;
CREATE TABLE device_catalog_state (id integer PRIMARY KEY CHECK(id=1));
INSERT INTO device_catalog_state VALUES(1);
CREATE TABLE device_tenant_states (tenant_id uuid PRIMARY KEY REFERENCES tenants(id));
INSERT INTO device_tenant_states SELECT id FROM tenants;
CREATE TABLE device_manufacturers (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_tenant_id uuid REFERENCES tenants(id),
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
 code_normalized text GENERATED ALWAYS AS (lower(code COLLATE "C")) STORED,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 description text NOT NULL DEFAULT '' CHECK(length(description)<=2000),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id), deleted_at timestamptz
);
CREATE UNIQUE INDEX device_manufacturer_public_code ON device_manufacturers(code_normalized) WHERE owner_tenant_id IS NULL;
CREATE UNIQUE INDEX device_manufacturer_private_code ON device_manufacturers(owner_tenant_id,code_normalized) WHERE owner_tenant_id IS NOT NULL;
CREATE INDEX device_manufacturer_page ON device_manufacturers(owner_tenant_id,created_at,id);
CREATE TABLE device_products (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_tenant_id uuid REFERENCES tenants(id), manufacturer_id uuid NOT NULL REFERENCES device_manufacturers(id),
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
 code_normalized text GENERATED ALWAYS AS (lower(code COLLATE "C")) STORED,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id), deleted_at timestamptz
);
CREATE UNIQUE INDEX device_product_public_code ON device_products(manufacturer_id,code_normalized) WHERE owner_tenant_id IS NULL;
CREATE UNIQUE INDEX device_product_private_code ON device_products(owner_tenant_id,manufacturer_id,code_normalized) WHERE owner_tenant_id IS NOT NULL;
CREATE INDEX device_product_page ON device_products(owner_tenant_id,manufacturer_id,created_at,id);
CREATE TABLE device_product_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), product_id uuid NOT NULL REFERENCES device_products(id),
 number integer NOT NULL CHECK(number>0), schema_version integer NOT NULL CHECK(schema_version=1),
 content jsonb NOT NULL CHECK(jsonb_typeof(content)='object'), content_hash text NOT NULL,
 source_version_id uuid REFERENCES device_product_versions(id),
 published_at timestamptz NOT NULL DEFAULT clock_timestamp(), published_by uuid NOT NULL REFERENCES accounts(id),
 UNIQUE(product_id,number), UNIQUE(product_id,id)
);
CREATE TABLE device_product_drafts (
 product_id uuid PRIMARY KEY REFERENCES device_products(id), base_version_id uuid, copied_from_version_id uuid REFERENCES device_product_versions(id), source_snapshot jsonb,
 content jsonb NOT NULL CHECK(jsonb_typeof(content)='object'), revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_by uuid NOT NULL REFERENCES accounts(id),
 FOREIGN KEY(product_id,base_version_id) REFERENCES device_product_versions(product_id,id)
);
CREATE TABLE device_product_slot_keys (
 product_id uuid NOT NULL REFERENCES device_products(id), key text NOT NULL CHECK(key ~ '^[a-z][a-z0-9_]{0,63}$'),
 mode text NOT NULL CHECK(mode IN ('fixed','configurable')), core_signature text NOT NULL,
 PRIMARY KEY(product_id,key)
);
CREATE TABLE device_product_version_states (
 version_id uuid PRIMARY KEY REFERENCES device_product_versions(id), enabled boolean NOT NULL DEFAULT true,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_by uuid NOT NULL REFERENCES accounts(id)
);
CREATE TABLE device_product_version_slots (
 version_id uuid NOT NULL, product_id uuid NOT NULL, slot_key text NOT NULL,
 mode text NOT NULL CHECK(mode IN ('fixed','configurable')), slot_order integer NOT NULL, content jsonb NOT NULL,
 metric_version_id uuid REFERENCES engineering_versions(id), unit_version_id uuid REFERENCES engineering_versions(id), allowed_shapes text[],
 PRIMARY KEY(version_id,slot_key),
 FOREIGN KEY(product_id,version_id) REFERENCES device_product_versions(product_id,id),
 FOREIGN KEY(product_id,slot_key) REFERENCES device_product_slot_keys(product_id,key),
 CHECK((mode='fixed' AND metric_version_id IS NOT NULL AND allowed_shapes IS NULL) OR
       (mode='configurable' AND metric_version_id IS NULL AND unit_version_id IS NULL AND cardinality(allowed_shapes)>0 AND allowed_shapes <@ ARRAY['integer','decimal','enum']::text[]))
);
CREATE TABLE device_product_dependencies (
 product_version_id uuid NOT NULL REFERENCES device_product_versions(id), engineering_version_id uuid NOT NULL REFERENCES engineering_versions(id), path text NOT NULL,
 PRIMARY KEY(product_version_id,engineering_version_id,path)
);
CREATE INDEX device_product_dependency_reverse ON device_product_dependencies(engineering_version_id);
CREATE TABLE devices (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
 code_normalized text GENERATED ALWAYS AS (lower(code COLLATE "C")) STORED,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 product_id uuid NOT NULL, product_version_id uuid NOT NULL,
 serial_number text NOT NULL DEFAULT '' CHECK(length(serial_number)<=200), description text NOT NULL DEFAULT '' CHECK(length(description)<=2000),
 status text NOT NULL DEFAULT 'enabled' CHECK(status IN ('enabled','disabled')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), output_revision bigint NOT NULL DEFAULT 0 CHECK(output_revision>=0), installation_revision bigint NOT NULL DEFAULT 0 CHECK(installation_revision>=0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id), deleted_at timestamptz,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,id,product_version_id), UNIQUE(tenant_id,code_normalized),
 FOREIGN KEY(product_id,product_version_id) REFERENCES device_product_versions(product_id,id)
);
CREATE INDEX device_page ON devices(tenant_id,status,created_at,id);
CREATE INDEX device_product_reference ON devices(tenant_id,product_version_id);
CREATE INDEX device_serial ON devices(tenant_id,serial_number);
CREATE TABLE device_outputs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, device_id uuid NOT NULL, product_version_id uuid NOT NULL, slot_key text NOT NULL,
 generation integer NOT NULL CHECK(generation>0), name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200), description text NOT NULL DEFAULT '' CHECK(length(description)<=2000),
 metric_version_id uuid NOT NULL REFERENCES engineering_versions(id), unit_version_id uuid REFERENCES engineering_versions(id),
 value_shape text NOT NULL CHECK(value_shape IN ('integer','decimal','enum')),
 timestamp_meaning text NOT NULL CHECK(timestamp_meaning IN ('observation_time','interval_end')),
 aggregation_description text NOT NULL DEFAULT '' CHECK(length(aggregation_description)<=2000),
 observation_direction text NOT NULL DEFAULT '' CHECK(length(observation_direction)<=2000), reference_basis text NOT NULL DEFAULT '' CHECK(length(reference_basis)<=2000), semantic_hash text NOT NULL,
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(), ended_at timestamptz,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,device_id,id), UNIQUE(tenant_id,device_id,slot_key,generation),
 FOREIGN KEY(tenant_id,device_id,product_version_id) REFERENCES devices(tenant_id,id,product_version_id),
 FOREIGN KEY(product_version_id,slot_key) REFERENCES device_product_version_slots(version_id,slot_key),
 CONSTRAINT device_output_period_valid CHECK(ended_at IS NULL OR ended_at>started_at),
 CHECK(timestamp_meaning<>'interval_end' OR length(btrim(aggregation_description))>0),
 CHECK((value_shape='enum' AND unit_version_id IS NULL) OR (value_shape<>'enum' AND unit_version_id IS NOT NULL)),
 CONSTRAINT device_output_period_exclusion EXCLUDE USING gist (
  tenant_id public.gist_uuid_ops WITH =, device_id public.gist_uuid_ops WITH =, slot_key public.gist_text_ops WITH =, tstzrange(started_at,ended_at,'[)') WITH &&)
);
CREATE UNIQUE INDEX device_output_current ON device_outputs(tenant_id,device_id,slot_key) WHERE ended_at IS NULL;
CREATE INDEX device_output_metric ON device_outputs(metric_version_id);
CREATE INDEX device_output_unit ON device_outputs(unit_version_id);
CREATE TABLE device_installations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, device_id uuid NOT NULL,
 target_kind text NOT NULL CHECK(target_kind IN ('facility','component','point','location')), facility_id uuid, component_id uuid, point_id uuid,
 location_description text NOT NULL DEFAULT '' CHECK(length(location_description)<=2000), position jsonb CHECK(position IS NULL OR jsonb_typeof(position)='object'),
 installed_at timestamptz NOT NULL, uninstalled_at timestamptz, revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(), recorded_by uuid NOT NULL REFERENCES accounts(id), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_by uuid NOT NULL REFERENCES accounts(id),
 UNIQUE(tenant_id,device_id,id), UNIQUE(tenant_id,id),
 FOREIGN KEY(tenant_id,device_id) REFERENCES devices(tenant_id,id),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 FOREIGN KEY(tenant_id,facility_id,component_id) REFERENCES engineering_components(tenant_id,facility_id,id),
 FOREIGN KEY(tenant_id,facility_id,point_id) REFERENCES engineering_points(tenant_id,facility_id,id),
 CONSTRAINT device_installation_target CHECK(
  (target_kind='location' AND facility_id IS NULL AND component_id IS NULL AND point_id IS NULL AND length(btrim(location_description))>0) OR
  (target_kind='facility' AND facility_id IS NOT NULL AND component_id IS NULL AND point_id IS NULL) OR
  (target_kind='component' AND facility_id IS NOT NULL AND component_id IS NOT NULL AND point_id IS NULL) OR
  (target_kind='point' AND facility_id IS NOT NULL AND point_id IS NOT NULL AND component_id IS NULL)),
 CONSTRAINT device_installation_period_valid CHECK(uninstalled_at IS NULL OR uninstalled_at>installed_at),
 CONSTRAINT device_installation_period_exclusion EXCLUDE USING gist (
  tenant_id public.gist_uuid_ops WITH =, device_id public.gist_uuid_ops WITH =, tstzrange(installed_at,uninstalled_at,'[)') WITH &&)
);
CREATE INDEX device_installation_page ON device_installations(tenant_id,device_id,installed_at,id);
CREATE INDEX device_installation_target_reverse ON device_installations(tenant_id,facility_id,component_id,point_id);
CREATE TABLE device_installation_target_refs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), installation_id uuid NOT NULL, tenant_id uuid NOT NULL,
 target_kind text NOT NULL CHECK(target_kind IN ('facility','component','point','location')), facility_id uuid, component_id uuid, point_id uuid,
 first_revision bigint NOT NULL CHECK(first_revision>0),
 FOREIGN KEY(tenant_id,installation_id) REFERENCES device_installations(tenant_id,id),
 FOREIGN KEY(tenant_id,facility_id) REFERENCES engineering_facilities(tenant_id,id),
 FOREIGN KEY(tenant_id,facility_id,component_id) REFERENCES engineering_components(tenant_id,facility_id,id),
 FOREIGN KEY(tenant_id,facility_id,point_id) REFERENCES engineering_points(tenant_id,facility_id,id),
 UNIQUE(installation_id,first_revision),
 CHECK((target_kind='location' AND facility_id IS NULL AND component_id IS NULL AND point_id IS NULL) OR
       (target_kind='facility' AND facility_id IS NOT NULL AND component_id IS NULL AND point_id IS NULL) OR
       (target_kind='component' AND facility_id IS NOT NULL AND component_id IS NOT NULL AND point_id IS NULL) OR
       (target_kind='point' AND facility_id IS NOT NULL AND component_id IS NULL AND point_id IS NOT NULL))
);
CREATE INDEX device_installation_reference_reverse ON device_installation_target_refs(tenant_id,facility_id,component_id,point_id);
ALTER TABLE engineering_channels ADD UNIQUE(tenant_id,facility_id,point_id,id);
CREATE TABLE monitoring_source_heads (
 tenant_id uuid NOT NULL, facility_id uuid NOT NULL, point_id uuid NOT NULL, channel_id uuid NOT NULL, revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 PRIMARY KEY(tenant_id,channel_id), FOREIGN KEY(tenant_id,facility_id,point_id,channel_id) REFERENCES engineering_channels(tenant_id,facility_id,point_id,id)
);
CREATE TABLE monitoring_source_bindings (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, device_id uuid NOT NULL, output_id uuid NOT NULL,
 facility_id uuid NOT NULL, point_id uuid NOT NULL, channel_id uuid NOT NULL,
 started_at timestamptz NOT NULL, ended_at timestamptz, revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 semantic_hash text NOT NULL, output_snapshot jsonb NOT NULL, channel_snapshot jsonb NOT NULL,
 created_actor_scope text NOT NULL CHECK(created_actor_scope IN ('tenant','project')), created_project_id uuid,
 created_by uuid NOT NULL REFERENCES accounts(id), ended_by uuid REFERENCES accounts(id), created_reason text NOT NULL DEFAULT '', ended_reason text,
 UNIQUE(tenant_id,id),
 FOREIGN KEY(tenant_id,device_id,output_id) REFERENCES device_outputs(tenant_id,device_id,id),
 FOREIGN KEY(tenant_id,facility_id,point_id,channel_id) REFERENCES engineering_channels(tenant_id,facility_id,point_id,id),
 FOREIGN KEY(tenant_id,channel_id) REFERENCES monitoring_source_heads(tenant_id,channel_id),
 FOREIGN KEY(tenant_id,created_project_id) REFERENCES projects(tenant_id,id),
 CHECK((created_actor_scope='project')=(created_project_id IS NOT NULL)),
 CHECK((ended_at IS NULL AND ended_by IS NULL AND ended_reason IS NULL) OR (ended_at IS NOT NULL AND ended_by IS NOT NULL AND length(btrim(ended_reason))>0)),
 CONSTRAINT monitoring_source_period_valid CHECK(ended_at IS NULL OR ended_at>started_at),
 CONSTRAINT monitoring_source_channel_exclusion EXCLUDE USING gist (
  tenant_id public.gist_uuid_ops WITH =, channel_id public.gist_uuid_ops WITH =, tstzrange(started_at,ended_at,'[)') WITH &&),
 CONSTRAINT monitoring_source_output_exclusion EXCLUDE USING gist (
  tenant_id public.gist_uuid_ops WITH =, output_id public.gist_uuid_ops WITH =, tstzrange(started_at,ended_at,'[)') WITH &&)
);
CREATE UNIQUE INDEX monitoring_source_channel_current ON monitoring_source_bindings(tenant_id,channel_id) WHERE ended_at IS NULL;
CREATE UNIQUE INDEX monitoring_source_output_current ON monitoring_source_bindings(tenant_id,output_id) WHERE ended_at IS NULL;
CREATE INDEX monitoring_source_channel_page ON monitoring_source_bindings(tenant_id,channel_id,started_at,id);
CREATE INDEX monitoring_source_output_page ON monitoring_source_bindings(tenant_id,output_id,started_at,id);
CREATE INDEX monitoring_source_device_current ON monitoring_source_bindings(tenant_id,device_id) WHERE ended_at IS NULL;
CREATE TABLE project_device_access_states (
 tenant_id uuid NOT NULL, project_id uuid NOT NULL, revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_by uuid REFERENCES accounts(id),
 PRIMARY KEY(tenant_id,project_id), FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
INSERT INTO project_device_access_states(tenant_id,project_id) SELECT tenant_id,id FROM projects;
CREATE TABLE project_device_grants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('device','output')), device_id uuid NOT NULL, output_id uuid, can_use_source boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0), granted_at timestamptz NOT NULL DEFAULT clock_timestamp(), granted_by uuid NOT NULL REFERENCES accounts(id),
 ended_at timestamptz, ended_by uuid REFERENCES accounts(id), end_reason text,
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_device_access_states(tenant_id,project_id),
 FOREIGN KEY(tenant_id,device_id) REFERENCES devices(tenant_id,id), FOREIGN KEY(tenant_id,device_id,output_id) REFERENCES device_outputs(tenant_id,device_id,id),
 CHECK((kind='device' AND output_id IS NULL AND NOT can_use_source) OR (kind='output' AND output_id IS NOT NULL)),
 CHECK((ended_at IS NULL AND ended_by IS NULL AND end_reason IS NULL) OR (ended_at IS NOT NULL AND ended_by IS NOT NULL AND length(btrim(end_reason))>0))
);
CREATE UNIQUE INDEX project_device_current ON project_device_grants(tenant_id,project_id,device_id) WHERE kind='device' AND ended_at IS NULL;
CREATE UNIQUE INDEX project_output_current ON project_device_grants(tenant_id,project_id,output_id) WHERE kind='output' AND ended_at IS NULL;
CREATE INDEX project_device_grant_page ON project_device_grants(tenant_id,project_id,kind,granted_at,id);
CREATE INDEX project_device_grant_reverse ON project_device_grants(tenant_id,device_id,output_id);
ALTER TABLE project_resource_grants ADD COLUMN can_source_manage boolean NOT NULL DEFAULT false,
 ADD CONSTRAINT project_channel_source_capability CHECK(resource_kind='channel' OR NOT can_source_manage);
CREATE TABLE device_catalog_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_tenant_id uuid REFERENCES tenants(id), manufacturer_id uuid REFERENCES device_manufacturers(id), product_id uuid REFERENCES device_products(id), version_id uuid REFERENCES device_product_versions(id),
 operation text NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id), actor_name text NOT NULL, tenant_membership_id uuid REFERENCES tenant_memberships(id),
 request_id uuid NOT NULL, reason text NOT NULL DEFAULT '', before jsonb, after jsonb NOT NULL, changed_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX device_catalog_change_page ON device_catalog_changes(owner_tenant_id,changed_at,id);
CREATE TABLE device_resource_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), device_id uuid NOT NULL, output_id uuid, installation_id uuid, binding_id uuid,
 operation text NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id), actor_name text NOT NULL,
 actor_scope text NOT NULL CHECK(actor_scope IN ('tenant','project')), actor_project_id uuid, tenant_membership_id uuid NOT NULL REFERENCES tenant_memberships(id), project_membership_id uuid REFERENCES project_memberships(id),
 request_id uuid NOT NULL, reason text NOT NULL DEFAULT '', before jsonb, after jsonb NOT NULL, changed_fields jsonb NOT NULL,
 changed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,device_id) REFERENCES devices(tenant_id,id), FOREIGN KEY(tenant_id,device_id,output_id) REFERENCES device_outputs(tenant_id,device_id,id),
 FOREIGN KEY(tenant_id,device_id,installation_id) REFERENCES device_installations(tenant_id,device_id,id), FOREIGN KEY(tenant_id,binding_id) REFERENCES monitoring_source_bindings(tenant_id,id),
 FOREIGN KEY(tenant_id,actor_project_id) REFERENCES projects(tenant_id,id),
 CHECK((actor_scope='project')=(actor_project_id IS NOT NULL AND project_membership_id IS NOT NULL))
);
CREATE INDEX device_resource_change_page ON device_resource_changes(tenant_id,device_id,changed_at,id);
CREATE INDEX device_output_change_page ON device_resource_changes(tenant_id,output_id,changed_at,id);
CREATE TABLE project_device_access_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, access_revision bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES accounts(id), actor_name text NOT NULL, tenant_membership_id uuid NOT NULL REFERENCES tenant_memberships(id),
 request_id uuid NOT NULL, reason text NOT NULL, instructions jsonb NOT NULL, before jsonb NOT NULL, after jsonb NOT NULL,
 changed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,project_id) REFERENCES project_device_access_states(tenant_id,project_id), UNIQUE(tenant_id,project_id,access_revision)
);
CREATE INDEX device_access_change_page ON project_device_access_changes(tenant_id,project_id,changed_at,id);
CREATE TABLE device_operation_receipts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), entry_scope text NOT NULL CHECK(entry_scope IN ('platform','tenant','project')),
 tenant_id uuid REFERENCES tenants(id), project_id uuid, actor_id uuid NOT NULL REFERENCES accounts(id),
 tenant_membership_id uuid REFERENCES tenant_memberships(id), project_membership_id uuid REFERENCES project_memberships(id),
 request_id uuid NOT NULL, operation text NOT NULL, payload_hash text NOT NULL, target_kind text NOT NULL, target_id uuid NOT NULL,
 result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id),
 FOREIGN KEY(tenant_id,actor_id,tenant_membership_id) REFERENCES tenant_memberships(tenant_id,account_id,id),
 FOREIGN KEY(tenant_id,project_id,actor_id,tenant_membership_id,project_membership_id) REFERENCES project_memberships(tenant_id,project_id,account_id,tenant_membership_id,id),
 CHECK((entry_scope='platform' AND tenant_id IS NULL AND project_id IS NULL AND tenant_membership_id IS NULL AND project_membership_id IS NULL) OR
       (entry_scope='tenant' AND tenant_id IS NOT NULL AND tenant_membership_id IS NOT NULL AND project_id IS NULL AND project_membership_id IS NULL) OR
       (entry_scope='project' AND tenant_id IS NOT NULL AND project_id IS NOT NULL AND tenant_membership_id IS NOT NULL AND project_membership_id IS NOT NULL))
);
CREATE UNIQUE INDEX device_receipt_platform ON device_operation_receipts(actor_id,request_id) WHERE entry_scope='platform';
CREATE UNIQUE INDEX device_receipt_tenant ON device_operation_receipts(tenant_id,actor_id,request_id) WHERE entry_scope='tenant';
CREATE UNIQUE INDEX device_receipt_project ON device_operation_receipts(tenant_id,project_id,actor_id,request_id) WHERE entry_scope='project';
-- +goose StatementBegin
CREATE FUNCTION device_initialize_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='tenants' THEN INSERT INTO device_tenant_states VALUES(NEW.id);
 ELSE INSERT INTO project_device_access_states(tenant_id,project_id) VALUES(NEW.tenant_id,NEW.id); END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION device_identity_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n jsonb; o jsonb; field text; parent_owner uuid; parent_deleted timestamptz; parent_status text;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'device identities cannot be erased' USING ERRCODE='23514'; END IF;
 n:=to_jsonb(NEW);
 IF TG_OP='UPDATE' THEN
  o:=to_jsonb(OLD);
  FOREACH field IN ARRAY ARRAY['id','owner_tenant_id','tenant_id','manufacturer_id','code','product_id','product_version_id','created_at','created_by'] LOOP
   IF n->field IS DISTINCT FROM o->field THEN RAISE EXCEPTION 'device identity is immutable' USING ERRCODE='23514'; END IF;
  END LOOP;
  IF o->>'deleted_at' IS NOT NULL THEN RAISE EXCEPTION 'deleted identity is immutable' USING ERRCODE='23514'; END IF;
 END IF;
 IF TG_TABLE_NAME='device_products' AND TG_OP='INSERT' THEN
  SELECT owner_tenant_id,deleted_at,status INTO parent_owner,parent_deleted,parent_status FROM device_manufacturers WHERE id=NEW.manufacturer_id FOR SHARE;
  IF NOT FOUND OR parent_deleted IS NOT NULL OR parent_status<>'active' OR (parent_owner IS NOT NULL AND parent_owner IS DISTINCT FROM NEW.owner_tenant_id)
  THEN RAISE EXCEPTION 'invalid manufacturer owner or state' USING ERRCODE='23514'; END IF;
 ELSIF TG_TABLE_NAME='devices' AND TG_OP='INSERT' THEN
  SELECT owner_tenant_id,deleted_at INTO parent_owner,parent_deleted FROM device_products WHERE id=NEW.product_id FOR SHARE;
  IF NOT FOUND OR parent_deleted IS NOT NULL OR (parent_owner IS NOT NULL AND parent_owner<>NEW.tenant_id)
  THEN RAISE EXCEPTION 'invalid product owner or state' USING ERRCODE='23514'; END IF;
  IF NOT EXISTS(SELECT 1 FROM device_product_version_states WHERE version_id=NEW.product_version_id AND enabled)
  THEN RAISE EXCEPTION 'product version unavailable' USING ERRCODE='23514'; END IF;
 END IF;
 IF n->>'deleted_at' IS NOT NULL THEN
  IF TG_TABLE_NAME='device_manufacturers' AND EXISTS(SELECT 1 FROM device_products WHERE manufacturer_id=NEW.id)
  THEN RAISE EXCEPTION 'manufacturer referenced' USING ERRCODE='23514'; END IF;
  IF TG_TABLE_NAME='device_products' AND EXISTS(SELECT 1 FROM device_product_versions WHERE product_id=NEW.id)
  THEN RAISE EXCEPTION 'published product cannot be deleted' USING ERRCODE='23514'; END IF;
  IF TG_TABLE_NAME='devices' AND (EXISTS(SELECT 1 FROM device_installations WHERE device_id=NEW.id) OR EXISTS(SELECT 1 FROM monitoring_source_bindings WHERE device_id=NEW.id) OR EXISTS(SELECT 1 FROM project_device_grants WHERE device_id=NEW.id))
  THEN RAISE EXCEPTION 'device has business history' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION device_output_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n jsonb; o jsonb; slot jsonb; slot_mode text; metric jsonb;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'outputs cannot be erased' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  n:=to_jsonb(NEW)-ARRAY['name','description','ended_at','revision','updated_by']; o:=to_jsonb(OLD)-ARRAY['name','description','ended_at','revision','updated_by'];
  IF n IS DISTINCT FROM o OR OLD.ended_at IS NOT NULL THEN RAISE EXCEPTION 'output semantics are immutable' USING ERRCODE='23514'; END IF;
  IF NEW.ended_at IS NOT NULL AND EXISTS(SELECT 1 FROM monitoring_source_bindings WHERE output_id=NEW.id AND (ended_at IS NULL OR ended_at>NEW.ended_at))
  THEN RAISE EXCEPTION 'output has unclosed or later source' USING ERRCODE='23514'; END IF;
 ELSE
  IF NOT EXISTS(SELECT 1 FROM devices WHERE id=NEW.device_id AND tenant_id=NEW.tenant_id AND deleted_at IS NULL) THEN RAISE EXCEPTION 'device unavailable' USING ERRCODE='23514'; END IF;
 END IF;
 SELECT content,mode INTO slot,slot_mode FROM device_product_version_slots WHERE version_id=NEW.product_version_id AND slot_key=NEW.slot_key;
 SELECT v.content->'metric' INTO metric FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE v.id=NEW.metric_version_id AND d.kind='metrics' AND d.tenant_id IS NULL;
 IF metric IS NULL OR metric->>'shape'<>NEW.value_shape OR (NEW.value_shape<>'enum' AND NOT (metric->'allowed_units' ? NEW.unit_version_id::text))
 THEN RAISE EXCEPTION 'invalid output metric or unit' USING ERRCODE='23514'; END IF;
 IF NEW.unit_version_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE v.id=NEW.unit_version_id AND d.kind='units' AND d.tenant_id IS NULL AND v.content->'unit'->>'category'=metric->>'category')
 THEN RAISE EXCEPTION 'invalid output dimension' USING ERRCODE='23514'; END IF;
 IF slot_mode='fixed' AND (slot->>'metric_version_id'<>NEW.metric_version_id::text OR (slot->>'unit_version_id') IS DISTINCT FROM NEW.unit_version_id::text OR slot->>'timestamp_meaning'<>NEW.timestamp_meaning OR COALESCE(slot->>'aggregation_description','')<>NEW.aggregation_description)
 THEN RAISE EXCEPTION 'fixed output differs from product' USING ERRCODE='23514'; END IF;
 IF slot_mode='configurable' AND NOT (slot->'allowed_shapes' ? NEW.value_shape) THEN RAISE EXCEPTION 'output shape not allowed' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION device_installation_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'installation history cannot be erased' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND (to_jsonb(NEW)-ARRAY['target_kind','facility_id','component_id','point_id','location_description','position','installed_at','uninstalled_at','revision','updated_at','updated_by']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['target_kind','facility_id','component_id','point_id','location_description','position','installed_at','uninstalled_at','revision','updated_at','updated_by'])
 THEN RAISE EXCEPTION 'installation identity is immutable' USING ERRCODE='23514'; END IF;
 IF NEW.installed_at>clock_timestamp() OR NEW.uninstalled_at>clock_timestamp() THEN RAISE EXCEPTION 'future installation time' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM devices WHERE id=NEW.device_id AND tenant_id=NEW.tenant_id AND deleted_at IS NULL) THEN RAISE EXCEPTION 'device unavailable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION device_remember_installation_target() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO device_installation_target_refs(installation_id,tenant_id,target_kind,facility_id,component_id,point_id,first_revision)
 VALUES(NEW.id,NEW.tenant_id,NEW.target_kind,NEW.facility_id,NEW.component_id,NEW.point_id,NEW.revision);
 RETURN NEW;
END $$;
CREATE FUNCTION device_semantic_text(value text) RETURNS text LANGUAGE sql IMMUTABLE STRICT AS $$
 SELECT btrim(replace(value,E'\r\n',E'\n'),U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000')
$$;
CREATE FUNCTION monitoring_source_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE output device_outputs; channel engineering_channels; shape text;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'source history cannot be erased' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.ended_at IS NOT NULL OR NEW.ended_at IS NULL OR NEW.revision<>OLD.revision+1 OR
   (to_jsonb(NEW)-ARRAY['ended_at','ended_by','ended_reason','revision']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['ended_at','ended_by','ended_reason','revision'])
  THEN RAISE EXCEPTION 'source only permits one closing transition' USING ERRCODE='23514'; END IF;
 ELSE
  SELECT * INTO output FROM device_outputs WHERE id=NEW.output_id AND tenant_id=NEW.tenant_id FOR SHARE;
  SELECT * INTO channel FROM engineering_channels WHERE id=NEW.channel_id AND tenant_id=NEW.tenant_id FOR SHARE;
  SELECT content->'metric'->>'shape' INTO shape FROM engineering_versions WHERE id=channel.metric_version_id;
  IF output.ended_at IS NOT NULL OR output.started_at>NEW.started_at OR output.id IS NULL OR channel.id IS NULL OR channel.deleted_at IS NOT NULL OR channel.status<>'normal' OR NEW.ended_at IS NOT NULL
  THEN RAISE EXCEPTION 'source endpoint unavailable' USING ERRCODE='23514'; END IF;
  IF NOT EXISTS(SELECT 1 FROM devices WHERE id=NEW.device_id AND tenant_id=NEW.tenant_id AND deleted_at IS NULL AND status='enabled')
  THEN RAISE EXCEPTION 'device disabled or deleted' USING ERRCODE='23514'; END IF;
  IF output.metric_version_id<>channel.metric_version_id OR output.unit_version_id IS DISTINCT FROM channel.unit_version_id OR output.value_shape<>shape OR output.timestamp_meaning<>channel.timestamp_meaning OR
   device_semantic_text(output.aggregation_description)<>device_semantic_text(channel.aggregation_description) OR
   device_semantic_text(output.observation_direction)<>device_semantic_text(channel.observation_direction) OR
   device_semantic_text(output.reference_basis)<>device_semantic_text(channel.reference_basis)
  THEN RAISE EXCEPTION 'source semantics incompatible' USING ERRCODE='23514'; END IF;
  IF EXISTS(WITH RECURSIVE path AS (
    SELECT * FROM engineering_resource_nodes WHERE tenant_id=NEW.tenant_id AND kind='channel' AND id=NEW.channel_id
    UNION ALL SELECT n.* FROM engineering_resource_nodes n JOIN path p ON n.tenant_id=p.tenant_id AND n.kind=p.parent_kind AND n.id=p.parent_id
   ) SELECT 1 FROM path WHERE status<>'normal' OR deleted_at IS NOT NULL)
  THEN RAISE EXCEPTION 'source target retired' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION device_grant_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'device grants cannot be erased' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND (OLD.ended_at IS NOT NULL OR
  (to_jsonb(NEW)-ARRAY['can_use_source','revision','ended_at','ended_by','end_reason']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['can_use_source','revision','ended_at','ended_by','end_reason']))
 THEN RAISE EXCEPTION 'grant identity is immutable' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM devices WHERE id=NEW.device_id AND tenant_id=NEW.tenant_id AND deleted_at IS NULL) THEN RAISE EXCEPTION 'device unavailable' USING ERRCODE='23514'; END IF;
 IF EXISTS(SELECT 1 FROM projects WHERE id=NEW.project_id AND status='archived') THEN
  IF TG_OP='INSERT' THEN RAISE EXCEPTION 'archived project cannot gain device access' USING ERRCODE='23514';
  ELSIF NEW.can_use_source AND NOT OLD.can_use_source THEN RAISE EXCEPTION 'archived project cannot gain source use' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION device_resource_reference_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.deleted_at IS NOT NULL AND (
  EXISTS(SELECT 1 FROM device_installation_target_refs r WHERE r.tenant_id=NEW.tenant_id AND
   ((TG_TABLE_NAME='engineering_facilities' AND r.facility_id=NEW.id) OR (TG_TABLE_NAME='engineering_components' AND r.component_id=NEW.id) OR (TG_TABLE_NAME='engineering_points' AND r.point_id=NEW.id))) OR
  EXISTS(SELECT 1 FROM monitoring_source_bindings b WHERE b.tenant_id=NEW.tenant_id AND
   ((TG_TABLE_NAME='engineering_facilities' AND b.facility_id=NEW.id) OR (TG_TABLE_NAME='engineering_points' AND b.point_id=NEW.id) OR (TG_TABLE_NAME='engineering_channels' AND b.channel_id=NEW.id))))
 THEN RAISE EXCEPTION 'resource has installation or source history' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- Publication projections and provenance are checked at commit, after all rows
-- of the publication transaction exist. No consumer may reinterpret a slot key.
CREATE FUNCTION device_draft_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE owner uuid; source_owner uuid;
BEGIN
 IF TG_OP='UPDATE' AND (to_jsonb(NEW)-ARRAY['content','revision','updated_at','updated_by']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['content','revision','updated_at','updated_by'])
 THEN RAISE EXCEPTION 'draft provenance is immutable' USING ERRCODE='23514'; END IF;
 IF NEW.copied_from_version_id IS NOT NULL THEN
  SELECT owner_tenant_id INTO owner FROM device_products WHERE id=NEW.product_id;
  SELECT p.owner_tenant_id INTO source_owner FROM device_product_versions v JOIN device_products p ON p.id=v.product_id WHERE v.id=NEW.copied_from_version_id;
  IF source_owner IS NOT NULL AND source_owner IS DISTINCT FROM owner THEN RAISE EXCEPTION 'invalid copy owner' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION device_version_state_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'version state cannot be erased' USING ERRCODE='23514'; END IF;
 IF NEW.version_id<>OLD.version_id OR NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'invalid version state revision' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION device_published_integrity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE vid uuid; body jsonb; pid uuid; owner uuid; source_owner uuid;
BEGIN
 vid:=COALESCE((to_jsonb(NEW)->>'version_id')::uuid,(to_jsonb(NEW)->>'product_version_id')::uuid,(to_jsonb(NEW)->>'id')::uuid);
 SELECT v.content,v.product_id,p.owner_tenant_id INTO body,pid,owner FROM device_product_versions v JOIN device_products p ON p.id=v.product_id WHERE v.id=vid;
 SELECT p.owner_tenant_id INTO source_owner FROM device_product_versions v JOIN device_product_versions source ON source.id=v.source_version_id JOIN device_products p ON p.id=source.product_id WHERE v.id=vid;
 IF source_owner IS NOT NULL AND source_owner IS DISTINCT FROM owner THEN RAISE EXCEPTION 'invalid published copy owner' USING ERRCODE='23514'; END IF;
 IF jsonb_typeof(body->'output_slots') IS DISTINCT FROM 'array' OR body->>'schema_version' IS DISTINCT FROM '1'
 THEN RAISE EXCEPTION 'invalid product publication' USING ERRCODE='23514'; END IF;
 IF jsonb_array_length(body->'output_slots')>256 OR NOT EXISTS(SELECT 1 FROM device_product_version_states WHERE version_id=vid) OR
  (SELECT count(*) FROM device_product_version_slots WHERE version_id=vid)<>jsonb_array_length(body->'output_slots') OR
  EXISTS(SELECT 1 FROM jsonb_array_elements(body->'output_slots') slot WHERE NOT EXISTS(SELECT 1 FROM device_product_version_slots s WHERE s.version_id=vid AND s.slot_key=slot->>'key' AND s.content=slot AND s.mode=slot->>'mode' AND s.slot_order=(slot->>'order')::integer))
 THEN RAISE EXCEPTION 'incomplete product publication' USING ERRCODE='23514'; END IF;
 IF EXISTS(SELECT 1 FROM device_product_version_slots s JOIN device_product_slot_keys k ON k.product_id=s.product_id AND k.key=s.slot_key WHERE s.version_id=vid AND
  (s.mode<>k.mode OR s.metric_version_id::text IS DISTINCT FROM s.content->>'metric_version_id' OR s.unit_version_id::text IS DISTINCT FROM s.content->>'unit_version_id' OR
   (s.mode='configurable' AND (to_jsonb(s.allowed_shapes) IS DISTINCT FROM s.content->'allowed_shapes' OR cardinality(s.allowed_shapes)<>(SELECT count(DISTINCT x) FROM unnest(s.allowed_shapes) x))) OR
   (s.mode='fixed' AND EXISTS(SELECT 1 FROM device_product_version_slots previous_slot WHERE previous_slot.product_id=pid AND previous_slot.slot_key=s.slot_key AND previous_slot.version_id<>vid AND
    (previous_slot.mode<>s.mode OR previous_slot.metric_version_id<>s.metric_version_id OR previous_slot.unit_version_id IS DISTINCT FROM s.unit_version_id OR previous_slot.content->>'timestamp_meaning' IS DISTINCT FROM s.content->>'timestamp_meaning' OR device_semantic_text(COALESCE(previous_slot.content->>'aggregation_description',''))<>device_semantic_text(COALESCE(s.content->>'aggregation_description','')))))))
 THEN RAISE EXCEPTION 'published slot meaning differs from projection or prior version' USING ERRCODE='23514'; END IF;
 IF EXISTS(SELECT 1 FROM device_product_version_slots s WHERE s.version_id=vid AND s.mode='fixed' AND
  (NOT EXISTS(SELECT 1 FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE v.id=s.metric_version_id AND d.kind='metrics' AND d.tenant_id IS NULL) OR
   (s.unit_version_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id WHERE v.id=s.unit_version_id AND d.kind='units' AND d.tenant_id IS NULL)) OR
   NOT EXISTS(SELECT 1 FROM device_product_dependencies dep WHERE dep.product_version_id=vid AND dep.engineering_version_id=s.metric_version_id) OR
   (s.unit_version_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM device_product_dependencies dep WHERE dep.product_version_id=vid AND dep.engineering_version_id=s.unit_version_id)))) OR
  EXISTS(SELECT 1 FROM device_product_dependencies dep WHERE dep.product_version_id=vid AND NOT EXISTS(SELECT 1 FROM device_product_version_slots s WHERE s.version_id=vid AND (s.metric_version_id=dep.engineering_version_id OR s.unit_version_id=dep.engineering_version_id)))
 THEN RAISE EXCEPTION 'invalid fixed public product dependency' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER device_tenant_initialize AFTER INSERT ON tenants FOR EACH ROW EXECUTE FUNCTION device_initialize_scope();
CREATE TRIGGER device_project_initialize AFTER INSERT ON projects FOR EACH ROW EXECUTE FUNCTION device_initialize_scope();
CREATE TRIGGER device_manufacturer_guard BEFORE INSERT OR UPDATE OR DELETE ON device_manufacturers FOR EACH ROW EXECUTE FUNCTION device_identity_guard();
CREATE TRIGGER device_product_guard BEFORE INSERT OR UPDATE OR DELETE ON device_products FOR EACH ROW EXECUTE FUNCTION device_identity_guard();
CREATE TRIGGER device_guard BEFORE INSERT OR UPDATE OR DELETE ON devices FOR EACH ROW EXECUTE FUNCTION device_identity_guard();
CREATE TRIGGER device_output_guard BEFORE INSERT OR UPDATE OR DELETE ON device_outputs FOR EACH ROW EXECUTE FUNCTION device_output_guard();
CREATE TRIGGER device_installation_guard BEFORE INSERT OR UPDATE OR DELETE ON device_installations FOR EACH ROW EXECUTE FUNCTION device_installation_guard();
CREATE TRIGGER device_installation_targets AFTER INSERT OR UPDATE ON device_installations FOR EACH ROW EXECUTE FUNCTION device_remember_installation_target();
CREATE TRIGGER monitoring_source_guard BEFORE INSERT OR UPDATE OR DELETE ON monitoring_source_bindings FOR EACH ROW EXECUTE FUNCTION monitoring_source_guard();
CREATE TRIGGER device_grant_guard BEFORE INSERT OR UPDATE OR DELETE ON project_device_grants FOR EACH ROW EXECUTE FUNCTION device_grant_guard();
CREATE TRIGGER device_facility_reference BEFORE UPDATE ON engineering_facilities FOR EACH ROW EXECUTE FUNCTION device_resource_reference_guard();
CREATE TRIGGER device_component_reference BEFORE UPDATE ON engineering_components FOR EACH ROW EXECUTE FUNCTION device_resource_reference_guard();
CREATE TRIGGER device_point_reference BEFORE UPDATE ON engineering_points FOR EACH ROW EXECUTE FUNCTION device_resource_reference_guard();
CREATE TRIGGER device_channel_reference BEFORE UPDATE ON engineering_channels FOR EACH ROW EXECUTE FUNCTION device_resource_reference_guard();
CREATE TRIGGER device_draft_guard BEFORE INSERT OR UPDATE ON device_product_drafts FOR EACH ROW EXECUTE FUNCTION device_draft_guard();
CREATE TRIGGER device_version_state_guard BEFORE UPDATE OR DELETE ON device_product_version_states FOR EACH ROW EXECUTE FUNCTION device_version_state_guard();
CREATE TRIGGER device_version_immutable BEFORE UPDATE OR DELETE ON device_product_versions FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_slot_immutable BEFORE UPDATE OR DELETE ON device_product_version_slots FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_slot_key_immutable BEFORE UPDATE OR DELETE ON device_product_slot_keys FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_dependency_immutable BEFORE UPDATE OR DELETE ON device_product_dependencies FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_installation_ref_immutable BEFORE UPDATE OR DELETE ON device_installation_target_refs FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_catalog_history_immutable BEFORE UPDATE OR DELETE ON device_catalog_changes FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_resource_history_immutable BEFORE UPDATE OR DELETE ON device_resource_changes FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_access_history_immutable BEFORE UPDATE OR DELETE ON project_device_access_changes FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_receipt_immutable BEFORE UPDATE OR DELETE ON device_operation_receipts FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE CONSTRAINT TRIGGER device_publication_integrity AFTER INSERT ON device_product_versions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION device_published_integrity();
CREATE CONSTRAINT TRIGGER device_dependency_integrity AFTER INSERT ON device_product_dependencies DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION device_published_integrity();
CREATE CONSTRAINT TRIGGER device_slot_integrity AFTER INSERT ON device_product_version_slots DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION device_published_integrity();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Device identities and history are irreversible; restore a verified backup'; END $$;
-- +goose StatementEnd
