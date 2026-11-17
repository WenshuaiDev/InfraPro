-- +goose Up
ALTER TABLE device_installations ADD monitoring_section_id uuid, ADD borehole_id uuid;
ALTER TABLE device_installations ADD FOREIGN KEY(tenant_id,facility_id,monitoring_section_id) REFERENCES engineering_monitoring_sections(tenant_id,facility_id,id), ADD FOREIGN KEY(tenant_id,facility_id,borehole_id) REFERENCES engineering_boreholes(tenant_id,facility_id,id);
-- +goose StatementBegin
DO $$ DECLARE c record; BEGIN FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='device_installations'::regclass AND contype='c' AND (pg_get_constraintdef(oid) LIKE '%target_kind%' OR pg_get_constraintdef(oid) LIKE '%component_id%') LOOP EXECUTE format('ALTER TABLE device_installations DROP CONSTRAINT %I',c.conname); END LOOP; END $$;
-- +goose StatementEnd
ALTER TABLE device_installations ADD CONSTRAINT device_installations_typed_target CHECK(
(target_kind='location' AND facility_id IS NULL AND num_nonnulls(component_id,point_id,monitoring_section_id,borehole_id)=0) OR
(target_kind='facility' AND facility_id IS NOT NULL AND num_nonnulls(component_id,point_id,monitoring_section_id,borehole_id)=0) OR
(facility_id IS NOT NULL AND num_nonnulls(component_id,point_id,monitoring_section_id,borehole_id)=1 AND ((target_kind='component' AND component_id IS NOT NULL) OR (target_kind='point' AND point_id IS NOT NULL) OR (target_kind='monitoring_section' AND monitoring_section_id IS NOT NULL) OR (target_kind='borehole' AND borehole_id IS NOT NULL))));
CREATE INDEX device_installations_structure ON device_installations(tenant_id,monitoring_section_id,borehole_id);
ALTER TABLE device_installation_target_refs ADD monitoring_section_id uuid, ADD borehole_id uuid;
ALTER TABLE device_installation_target_refs ADD FOREIGN KEY(tenant_id,facility_id,monitoring_section_id) REFERENCES engineering_monitoring_sections(tenant_id,facility_id,id), ADD FOREIGN KEY(tenant_id,facility_id,borehole_id) REFERENCES engineering_boreholes(tenant_id,facility_id,id);
-- +goose StatementBegin
DO $$ DECLARE c record; BEGIN FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='device_installation_target_refs'::regclass AND contype='c' AND (pg_get_constraintdef(oid) LIKE '%target_kind%' OR pg_get_constraintdef(oid) LIKE '%component_id%') LOOP EXECUTE format('ALTER TABLE device_installation_target_refs DROP CONSTRAINT %I',c.conname); END LOOP; END $$;
-- +goose StatementEnd
ALTER TABLE device_installation_target_refs ADD CONSTRAINT device_installation_target_refs_typed_target CHECK(
(target_kind='location' AND facility_id IS NULL AND num_nonnulls(component_id,point_id,monitoring_section_id,borehole_id)=0) OR
(target_kind='facility' AND facility_id IS NOT NULL AND num_nonnulls(component_id,point_id,monitoring_section_id,borehole_id)=0) OR
(facility_id IS NOT NULL AND num_nonnulls(component_id,point_id,monitoring_section_id,borehole_id)=1 AND ((target_kind='component' AND component_id IS NOT NULL) OR (target_kind='point' AND point_id IS NOT NULL) OR (target_kind='monitoring_section' AND monitoring_section_id IS NOT NULL) OR (target_kind='borehole' AND borehole_id IS NOT NULL))));
CREATE INDEX device_installation_target_refs_structure ON device_installation_target_refs(tenant_id,monitoring_section_id,borehole_id);
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION device_installation_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'installation history cannot be erased' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND (to_jsonb(NEW)-ARRAY['target_kind','facility_id','component_id','point_id','monitoring_section_id','borehole_id','location_description','position','installed_at','uninstalled_at','revision','updated_at','updated_by']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['target_kind','facility_id','component_id','point_id','monitoring_section_id','borehole_id','location_description','position','installed_at','uninstalled_at','revision','updated_at','updated_by'])
 THEN RAISE EXCEPTION 'installation identity is immutable' USING ERRCODE='23514'; END IF;
 IF NEW.target_kind='location' AND length(btrim(NEW.location_description))=0 THEN RAISE EXCEPTION 'location description required' USING ERRCODE='23514'; END IF;
 IF NEW.installed_at>clock_timestamp() OR NEW.uninstalled_at>clock_timestamp() THEN RAISE EXCEPTION 'future installation time' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM devices WHERE id=NEW.device_id AND tenant_id=NEW.tenant_id AND deleted_at IS NULL) THEN RAISE EXCEPTION 'device unavailable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION device_remember_installation_target() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO device_installation_target_refs(installation_id,tenant_id,target_kind,facility_id,component_id,point_id,monitoring_section_id,borehole_id,first_revision)
 VALUES(NEW.id,NEW.tenant_id,NEW.target_kind,NEW.facility_id,NEW.component_id,NEW.point_id,NEW.monitoring_section_id,NEW.borehole_id,NEW.revision);
 RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION device_resource_reference_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.deleted_at IS NOT NULL AND (
  EXISTS(SELECT 1 FROM device_installation_target_refs r WHERE r.tenant_id=NEW.tenant_id AND
   ((TG_TABLE_NAME='engineering_facilities' AND r.facility_id=NEW.id) OR (TG_TABLE_NAME='engineering_components' AND r.component_id=NEW.id) OR (TG_TABLE_NAME='engineering_points' AND r.point_id=NEW.id) OR (TG_TABLE_NAME='engineering_monitoring_sections' AND r.monitoring_section_id=NEW.id) OR (TG_TABLE_NAME='engineering_boreholes' AND r.borehole_id=NEW.id))) OR
  EXISTS(SELECT 1 FROM monitoring_source_bindings b WHERE b.tenant_id=NEW.tenant_id AND
   ((TG_TABLE_NAME='engineering_facilities' AND b.facility_id=NEW.id) OR (TG_TABLE_NAME='engineering_points' AND b.point_id=NEW.id) OR (TG_TABLE_NAME='engineering_channels' AND b.channel_id=NEW.id))))
 THEN RAISE EXCEPTION 'resource has installation or source history' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER section_installation_refs BEFORE UPDATE ON engineering_monitoring_sections FOR EACH ROW EXECUTE FUNCTION device_resource_reference_guard();
CREATE TRIGGER borehole_installation_refs BEFORE UPDATE ON engineering_boreholes FOR EACH ROW EXECUTE FUNCTION device_resource_reference_guard();

-- Extend existing placements without changing any point-placement identity.
ALTER TABLE spatial_point_placements ADD target_kind text NOT NULL DEFAULT 'point', ADD target_id uuid;
ALTER TABLE spatial_point_placements DISABLE TRIGGER spatial_placement_guard;
UPDATE spatial_point_placements SET target_id=point_id;
ALTER TABLE spatial_point_placements ENABLE TRIGGER spatial_placement_guard;
ALTER TABLE spatial_point_placements ALTER point_id DROP NOT NULL, ALTER target_id SET NOT NULL;
ALTER TABLE spatial_point_placements ADD FOREIGN KEY(tenant_id,facility_id,target_kind,target_id) REFERENCES engineering_structure_nodes(tenant_id,facility_id,kind,id);
ALTER TABLE spatial_point_placements ADD CONSTRAINT spatial_typed_placement CHECK((target_kind='point' AND point_id=target_id AND point_id IS NOT NULL) OR (target_kind IN ('monitoring_section','borehole') AND point_id IS NULL));
CREATE UNIQUE INDEX spatial_typed_placement_unique ON spatial_point_placements(version_id,page_id,target_kind,target_id);
-- +goose StatementBegin
CREATE FUNCTION spatial_typed_placement_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.target_kind='point' THEN NEW.target_id:=COALESCE(NEW.target_id,NEW.point_id); END IF;
 IF TG_OP='UPDATE' AND (NEW.target_kind,NEW.target_id) IS DISTINCT FROM (OLD.target_kind,OLD.target_id) THEN RAISE EXCEPTION 'placement target identity is immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION structure_spatial_reference_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.deleted_at IS NOT NULL AND EXISTS(SELECT 1 FROM spatial_point_placements p WHERE p.tenant_id=NEW.tenant_id AND p.target_kind=CASE TG_TABLE_NAME WHEN 'engineering_monitoring_sections' THEN 'monitoring_section' ELSE 'borehole' END AND p.target_id=NEW.id) THEN RAISE EXCEPTION 'structure has spatial references' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER spatial_typed_placement BEFORE INSERT OR UPDATE ON spatial_point_placements FOR EACH ROW EXECUTE FUNCTION spatial_typed_placement_guard();
CREATE TRIGGER section_spatial_reference BEFORE UPDATE ON engineering_monitoring_sections FOR EACH ROW EXECUTE FUNCTION structure_spatial_reference_guard();
CREATE TRIGGER borehole_spatial_reference BEFORE UPDATE ON engineering_boreholes FOR EACH ROW EXECUTE FUNCTION structure_spatial_reference_guard();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'New installation owners cannot be written by a legacy program; restore a compatible backup'; END $$;
-- +goose StatementEnd
