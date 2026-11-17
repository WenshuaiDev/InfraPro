-- +goose Up
CREATE TABLE engineering_structure_record_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, facility_id uuid NOT NULL,
 resource_kind text NOT NULL, resource_id uuid NOT NULL, resource_revision integer NOT NULL,
 action text NOT NULL, evidence jsonb NOT NULL, reason text NOT NULL,
 request_id uuid NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id), actor_project_id uuid,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,facility_id,resource_kind,resource_id) REFERENCES engineering_structure_nodes(tenant_id,facility_id,kind,id),
 UNIQUE(tenant_id,request_id)
);
CREATE INDEX engineering_structure_record_page ON engineering_structure_record_changes(tenant_id,facility_id,resource_kind,resource_id,created_at,id);
CREATE TRIGGER engineering_structure_record_immutable BEFORE UPDATE OR DELETE ON engineering_structure_record_changes FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TABLE engineering_legacy_section_evidence (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), facility_id uuid,
 resource_kind text NOT NULL, resource_id uuid NOT NULL, device_id uuid,
 source_table text NOT NULL, source_id uuid NOT NULL, source_revision text NOT NULL, source_path text NOT NULL,
 entry_index integer NOT NULL, original jsonb NOT NULL, historical boolean NOT NULL,
 captured_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(source_table,source_id,source_path,entry_index)
);
CREATE TABLE engineering_legacy_section_resolutions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), evidence_id uuid NOT NULL REFERENCES engineering_legacy_section_evidence(id),
 status text NOT NULL CHECK(status IN ('pending','mapped','historical')), target_kind text, target_id uuid,
 reason text NOT NULL CHECK(length(btrim(reason))>0), actor_id uuid NOT NULL REFERENCES accounts(id), request_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((status='mapped')=(target_kind IS NOT NULL AND target_id IS NOT NULL)), CHECK(target_kind IS NULL OR target_kind='monitoring_section')
);
CREATE INDEX engineering_legacy_evidence_owner ON engineering_legacy_section_evidence(tenant_id,facility_id,resource_kind,resource_id);
CREATE INDEX engineering_legacy_evidence_device ON engineering_legacy_section_evidence(tenant_id,device_id);
CREATE TRIGGER engineering_legacy_section_immutable BEFORE UPDATE OR DELETE ON engineering_legacy_section_evidence FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER engineering_legacy_resolution_immutable BEFORE UPDATE OR DELETE ON engineering_legacy_section_resolutions FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
-- Capture every occurrence, retaining source/revision/path/index. Names never imply identity.
INSERT INTO engineering_legacy_section_evidence(tenant_id,facility_id,resource_kind,resource_id,source_table,source_id,source_revision,source_path,entry_index,original,historical)
SELECT r.tenant_id,r.facility_id,r.kind,r.id,'engineering_'||CASE r.kind WHEN 'facility' THEN 'facilities' WHEN 'component' THEN 'components' ELSE 'points' END,r.id,r.data->>'revision','position.sections',x.ordinality::integer-1,x.value,false
FROM engineering_resource_records r CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(r.data#>'{position,sections}')='array' THEN r.data#>'{position,sections}' ELSE '[]'::jsonb END) WITH ORDINALITY x
WHERE r.kind IN ('facility','component','point');
INSERT INTO engineering_legacy_section_evidence(tenant_id,facility_id,resource_kind,resource_id,source_table,source_id,source_revision,source_path,entry_index,original,historical)
SELECT c.tenant_id,c.facility_id,c.resource_kind,c.resource_id,'engineering_resource_changes',c.id,c.resource_revision::text,v.path||'.position.sections',x.ordinality::integer-1,x.value,true
FROM engineering_resource_changes c CROSS JOIN LATERAL (VALUES('before',c.before),('after',c.after))v(path,data)
CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(v.data#>'{position,sections}')='array' THEN v.data#>'{position,sections}' ELSE '[]'::jsonb END) WITH ORDINALITY x;
INSERT INTO engineering_legacy_section_evidence(tenant_id,facility_id,resource_kind,resource_id,device_id,source_table,source_id,source_revision,source_path,entry_index,original,historical)
SELECT d.tenant_id,d.facility_id,'installation',d.id,d.device_id,'device_installations',d.id,d.revision::text,'position.sections',x.ordinality::integer-1,x.value,d.uninstalled_at IS NOT NULL
FROM device_installations d CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(d.position->'sections')='array' THEN d.position->'sections' ELSE '[]'::jsonb END) WITH ORDINALITY x;
INSERT INTO engineering_legacy_section_evidence(tenant_id,facility_id,resource_kind,resource_id,device_id,source_table,source_id,source_revision,source_path,entry_index,original,historical)
SELECT c.tenant_id,COALESCE(NULLIF(v.data->>'facility_id','')::uuid,d.facility_id),'installation',COALESCE(NULLIF(v.data->>'id','')::uuid,d.id),c.device_id,'device_resource_changes',c.id,COALESCE(v.data->>'revision','unknown'),v.path||'.position.sections',x.ordinality::integer-1,x.value,true
FROM device_resource_changes c JOIN device_installations d ON d.id=c.installation_id CROSS JOIN LATERAL (VALUES('before',c.before),('after',c.after),('after.installation',c.after->'installation'),('after.previous_installation',c.after->'previous_installation'))v(path,data)
CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(v.data#>'{position,sections}')='array' THEN v.data#>'{position,sections}' ELSE '[]'::jsonb END) WITH ORDINALITY x;
-- Only the current projection changes. Historical changes remain byte-for-byte intact.
UPDATE engineering_facilities SET position=position-'sections' WHERE position ? 'sections';
UPDATE engineering_components SET position=position-'sections' WHERE position ? 'sections';
UPDATE engineering_points SET position=position-'sections' WHERE position ? 'sections';
-- The target and revision have not changed; do not append a duplicate target-reference revision.
ALTER TABLE device_installations DISABLE TRIGGER device_installation_targets;
UPDATE device_installations SET position=position-'sections' WHERE position ? 'sections';
ALTER TABLE device_installations ENABLE TRIGGER device_installation_targets;
-- +goose StatementBegin
CREATE FUNCTION engineering_current_position_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.position ? 'sections' THEN RAISE EXCEPTION 'legacy_section_write_rejected: use a typed monitoring_section reference' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION engineering_structure_depth_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE excessive boolean;
BEGIN
 WITH RECURSIVE tree AS (
 SELECT n.kind,n.id,0 AS depth,ARRAY[n.kind||':'||n.id] AS path,false AS cycle FROM engineering_structure_nodes n WHERE n.tenant_id=NEW.tenant_id AND n.facility_id=NEW.facility_id AND n.kind='facility'
 UNION ALL SELECT n.kind,n.id,t.depth+1,t.path||(n.kind||':'||n.id),(n.kind||':'||n.id)=ANY(t.path)
 FROM tree t JOIN engineering_structure_nodes n ON n.tenant_id=NEW.tenant_id AND n.facility_id=NEW.facility_id AND n.parent_kind=t.kind AND n.parent_id=t.id WHERE t.depth<=512 AND NOT t.cycle
 ) SELECT EXISTS(SELECT 1 FROM tree WHERE depth>512 OR cycle) INTO excessive;
 IF excessive THEN RAISE EXCEPTION 'structure_budget_exceeded: depth 512' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER facility_position_current BEFORE INSERT OR UPDATE OF position ON engineering_facilities FOR EACH ROW EXECUTE FUNCTION engineering_current_position_guard();
CREATE TRIGGER component_position_current BEFORE INSERT OR UPDATE OF position ON engineering_components FOR EACH ROW EXECUTE FUNCTION engineering_current_position_guard();
CREATE TRIGGER point_position_current BEFORE INSERT OR UPDATE OF position ON engineering_points FOR EACH ROW EXECUTE FUNCTION engineering_current_position_guard();
CREATE TRIGGER section_position_current BEFORE INSERT OR UPDATE OF position ON engineering_monitoring_sections FOR EACH ROW EXECUTE FUNCTION engineering_current_position_guard();
CREATE TRIGGER borehole_position_current BEFORE INSERT OR UPDATE OF position ON engineering_boreholes FOR EACH ROW EXECUTE FUNCTION engineering_current_position_guard();
CREATE TRIGGER installation_position_current BEFORE INSERT OR UPDATE OF position ON device_installations FOR EACH ROW EXECUTE FUNCTION engineering_current_position_guard();
CREATE CONSTRAINT TRIGGER engineering_structure_depth AFTER UPDATE ON engineering_structure_nodes DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION engineering_structure_depth_guard();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Structure evidence cannot be discarded; restore a verified compatible backup'; END $$;
-- +goose StatementEnd
