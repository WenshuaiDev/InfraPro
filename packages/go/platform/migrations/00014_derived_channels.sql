-- +goose Up
-- Existing channel IDs and every device-source composite key remain valid.
ALTER TABLE engineering_channels
 ADD COLUMN source_kind text NOT NULL DEFAULT 'direct' CHECK(source_kind IN ('direct','derived')),
 ADD COLUMN owner_kind text NOT NULL DEFAULT 'point' CHECK(owner_kind IN ('facility','component','point')),
 ADD COLUMN component_id uuid,
 ADD COLUMN derived_metric_version_id uuid,
 ADD COLUMN quantity_spec jsonb,
 ADD COLUMN output_contract_hash text,
 ADD COLUMN output_semantics jsonb,
 ALTER COLUMN point_id DROP NOT NULL,
 ALTER COLUMN metric_version_id DROP NOT NULL,
 ADD UNIQUE(tenant_id,id),
 ADD FOREIGN KEY(tenant_id,facility_id,component_id) REFERENCES engineering_components(tenant_id,facility_id,id),
 ADD FOREIGN KEY(tenant_id,derived_metric_version_id) REFERENCES derived_metric_versions(tenant_id,id),
 ADD CONSTRAINT channel_owner_shape CHECK(
  (owner_kind='point' AND point_id IS NOT NULL AND component_id IS NULL) OR
  (owner_kind='component' AND component_id IS NOT NULL AND point_id IS NULL) OR
  (owner_kind='facility' AND point_id IS NULL AND component_id IS NULL)),
 ADD CONSTRAINT channel_source_shape CHECK(
  (source_kind='direct' AND owner_kind='point' AND metric_version_id IS NOT NULL AND derived_metric_version_id IS NULL AND quantity_spec IS NULL AND output_contract_hash IS NULL AND output_semantics IS NULL) OR
  (source_kind='derived' AND metric_version_id IS NULL AND unit_version_id IS NULL AND derived_metric_version_id IS NOT NULL AND quantity_spec IS NOT NULL AND jsonb_typeof(quantity_spec)='object' AND output_contract_hash IS NOT NULL AND output_contract_hash ~ '^[0-9a-f]{64}$' AND output_semantics IS NOT NULL AND jsonb_typeof(output_semantics)='object'));
CREATE UNIQUE INDEX derived_component_channel_code ON engineering_channels(tenant_id,component_id,code_normalized) WHERE owner_kind='component';
CREATE UNIQUE INDEX derived_facility_channel_code ON engineering_channels(tenant_id,facility_id,code_normalized) WHERE owner_kind='facility';
CREATE INDEX resource_channels_component ON engineering_channels(tenant_id,facility_id,component_id,created_at,id) WHERE owner_kind='component';
CREATE INDEX resource_channels_facility ON engineering_channels(tenant_id,facility_id,created_at,id) WHERE owner_kind='facility';
CREATE TABLE derived_channels (
 tenant_id uuid NOT NULL, channel_id uuid NOT NULL, PRIMARY KEY(tenant_id,channel_id),
 control_state text NOT NULL DEFAULT 'draft' CHECK(control_state IN ('draft','enabled','paused','retired')),
 control_revision bigint NOT NULL DEFAULT 1 CHECK(control_revision>0),
 mandate_revision bigint NOT NULL DEFAULT 1 CHECK(mandate_revision>0),
 config_revision bigint NOT NULL DEFAULT 1 CHECK(config_revision>0),
 draft_config_id uuid NOT NULL,
 execution_scope_root text,
 auto_start_ns bigint CHECK(auto_start_ns>=-2208988800000000000),
 stop_ns bigint CHECK(stop_ns>=-2208988800000000000),
 health text NOT NULL DEFAULT 'waiting' CHECK(health IN ('waiting','healthy','blocked')),
 blocked_reason text NOT NULL DEFAULT '',
 FOREIGN KEY(tenant_id,channel_id) REFERENCES engineering_channels(tenant_id,id)
);
CREATE TABLE derived_channel_configs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, channel_id uuid NOT NULL,
 definition_version_id uuid NOT NULL, input_bindings jsonb NOT NULL CHECK(jsonb_typeof(input_bindings)='object'),
 parameters jsonb NOT NULL CHECK(jsonb_typeof(parameters)='object'),
 rule jsonb NOT NULL CHECK(jsonb_typeof(rule)='object'), compiled_plan jsonb NOT NULL CHECK(jsonb_typeof(compiled_plan)='object'),
 semantic_hash text NOT NULL CHECK(semantic_hash ~ '^[0-9a-f]{64}$'),
 config_hash text NOT NULL CHECK(config_hash ~ '^[0-9a-f]{64}$'),
 output_contract_hash text NOT NULL CHECK(output_contract_hash ~ '^[0-9a-f]{64}$'),
 graph_revision bigint NOT NULL CHECK(graph_revision>0),
 created_by uuid NOT NULL REFERENCES accounts(id), created_at timestamptz NOT NULL DEFAULT clock_timestamp(), reason text NOT NULL,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,channel_id,id),
 FOREIGN KEY(tenant_id,channel_id) REFERENCES derived_channels(tenant_id,channel_id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(tenant_id,definition_version_id) REFERENCES derived_metric_versions(tenant_id,id)
);
ALTER TABLE derived_channels ADD FOREIGN KEY(tenant_id,channel_id,draft_config_id) REFERENCES derived_channel_configs(tenant_id,channel_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE derived_config_assignments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, channel_id uuid NOT NULL, config_id uuid NOT NULL,
 effective_range int8range NOT NULL CHECK(NOT isempty(effective_range) AND NOT lower_inf(effective_range) AND lower(effective_range)>=-2208988800000000000),
 created_revision bigint NOT NULL CHECK(created_revision>0), superseded_revision bigint CHECK(superseded_revision>created_revision),
 request_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,channel_id,config_id) REFERENCES derived_channel_configs(tenant_id,channel_id,id),
 CONSTRAINT derived_assignment_exclusion EXCLUDE USING gist(tenant_id public.gist_uuid_ops WITH =,channel_id public.gist_uuid_ops WITH =,effective_range WITH &&) WHERE(superseded_revision IS NULL)
);
CREATE INDEX derived_assignments_history ON derived_config_assignments(tenant_id,channel_id,created_revision);
CREATE TABLE derived_dependencies (
 tenant_id uuid NOT NULL, config_id uuid NOT NULL, slot_key text NOT NULL,
 input_channel_id uuid NOT NULL, input_kind text NOT NULL CHECK(input_kind IN ('direct','derived')),
 source_semantics jsonb NOT NULL CHECK(jsonb_typeof(source_semantics)='object'),
 impact jsonb NOT NULL CHECK(jsonb_typeof(impact)='object'),
 PRIMARY KEY(tenant_id,config_id,slot_key),
 FOREIGN KEY(tenant_id,config_id) REFERENCES derived_channel_configs(tenant_id,id),
 FOREIGN KEY(tenant_id,input_channel_id) REFERENCES engineering_channels(tenant_id,id)
);
CREATE INDEX derived_dependencies_reverse ON derived_dependencies(tenant_id,input_channel_id,config_id);
CREATE TABLE derived_channel_heads (
 tenant_id uuid NOT NULL, channel_id uuid NOT NULL, PRIMARY KEY(tenant_id,channel_id),
 head_revision bigint NOT NULL DEFAULT 1 CHECK(head_revision>0),
 current_root text, history_root text, summary_root text, coverage_root text,
 checked_source_position bigint NOT NULL DEFAULT 0 CHECK(checked_source_position>=0),
 FOREIGN KEY(tenant_id,channel_id) REFERENCES derived_channels(tenant_id,channel_id)
);
ALTER TABLE project_resource_grants
 ADD COLUMN can_create_derived_channel boolean NOT NULL DEFAULT false,
 ADD COLUMN can_manage_calculation boolean NOT NULL DEFAULT false,
 ADD CHECK(NOT can_create_derived_channel OR resource_kind IN ('facility','component','point')),
 ADD CHECK(NOT can_manage_calculation OR resource_kind='channel');
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION resource_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE n jsonb; o jsonb; fixed text; fid uuid; parent uuid; typ jsonb; metric jsonb; relation jsonb; parent_category text; maximum integer; count_normal integer;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'resource identities cannot be erased' USING ERRCODE='23514'; END IF;
 n:=to_jsonb(NEW);
 IF TG_OP='UPDATE' THEN
  o:=to_jsonb(OLD);
  FOREACH fixed IN ARRAY ARRAY['id','tenant_id','facility_id','code','type_version_id','category_key','point_id','metric_version_id','unit_version_id','observation_direction','reference_basis','timestamp_meaning','aggregation_description','created_at','created_by','source_kind','owner_kind','derived_metric_version_id','quantity_spec','output_contract_hash','output_semantics'] LOOP
   IF n->fixed IS DISTINCT FROM o->fixed THEN RAISE EXCEPTION 'resource identity or semantics are immutable' USING ERRCODE='23514'; END IF;
  END LOOP;
  IF TG_TABLE_NAME='engineering_channels' AND n->'component_id' IS DISTINCT FROM o->'component_id' THEN RAISE EXCEPTION 'channel owner is immutable' USING ERRCODE='23514'; END IF;
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
CREATE FUNCTION derived_channel_integrity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE tid uuid; cid uuid; kind text;
BEGIN
 tid:=NEW.tenant_id; IF TG_TABLE_NAME='engineering_channels' THEN cid:=NEW.id; ELSE cid:=NEW.channel_id; END IF;
 SELECT source_kind INTO kind FROM engineering_channels WHERE tenant_id=tid AND id=cid;
 IF (kind='derived') IS DISTINCT FROM EXISTS(SELECT 1 FROM derived_channels WHERE tenant_id=tid AND channel_id=cid)
 THEN RAISE EXCEPTION 'derived channel requires its calculation extension atomically' USING ERRCODE='23514'; END IF;
 IF kind='derived' AND NOT EXISTS(SELECT 1 FROM derived_channels d JOIN derived_channel_configs cfg ON cfg.tenant_id=d.tenant_id AND cfg.channel_id=d.channel_id AND cfg.id=d.draft_config_id JOIN derived_channel_heads h ON h.tenant_id=d.tenant_id AND h.channel_id=d.channel_id JOIN engineering_channels c ON c.tenant_id=d.tenant_id AND c.id=d.channel_id WHERE d.tenant_id=tid AND d.channel_id=cid AND cfg.output_contract_hash=c.output_contract_hash AND cfg.rule#>'{outputs,0,quantity_spec}'=c.quantity_spec)
 THEN RAISE EXCEPTION 'derived channel requires compatible configuration and result head atomically' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE FUNCTION derived_configuration_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM engineering_channels c JOIN derived_metric_versions v ON v.tenant_id=c.tenant_id AND v.id=NEW.definition_version_id JOIN derived_metric_versions initial ON initial.tenant_id=c.tenant_id AND initial.id=c.derived_metric_version_id
  WHERE c.tenant_id=NEW.tenant_id AND c.id=NEW.channel_id AND c.source_kind='derived' AND c.output_contract_hash=NEW.output_contract_hash AND v.definition_id=initial.definition_id)
 THEN RAISE EXCEPTION 'configuration is incompatible with fixed channel identity' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION derived_dependency_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM engineering_channels WHERE tenant_id=NEW.tenant_id AND id=NEW.input_channel_id AND source_kind=NEW.input_kind AND deleted_at IS NULL)
 THEN RAISE EXCEPTION 'dependency kind or scope mismatch' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION derived_assignment_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.superseded_revision IS NOT NULL OR NEW.superseded_revision IS NULL OR (to_jsonb(NEW)-'superseded_revision') IS DISTINCT FROM (to_jsonb(OLD)-'superseded_revision')
 THEN RAISE EXCEPTION 'assignment only permits superseding, not rewriting history' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION monitoring_direct_channel_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM engineering_channels WHERE tenant_id=NEW.tenant_id AND id=NEW.channel_id AND source_kind='direct')
 THEN RAISE EXCEPTION 'device source requires a direct channel' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION derived_resource_grant_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.resource_kind='channel' AND NOT EXISTS(SELECT 1 FROM engineering_channels c WHERE c.tenant_id=NEW.tenant_id AND c.id=NEW.channel_id AND ((c.source_kind='direct' AND NOT NEW.can_manage_calculation) OR (c.source_kind='derived' AND NOT NEW.can_source_manage)))
 THEN RAISE EXCEPTION 'calculation and device source grants cannot substitute each other' USING ERRCODE='23514'; END IF;
 IF EXISTS(SELECT 1 FROM projects WHERE tenant_id=NEW.tenant_id AND id=NEW.project_id AND status='archived') THEN
  IF TG_OP='INSERT' AND (NEW.can_create_derived_channel OR NEW.can_manage_calculation) OR TG_OP='UPDATE' AND (NEW.can_create_derived_channel AND NOT OLD.can_create_derived_channel OR NEW.can_manage_calculation AND NOT OLD.can_manage_calculation)
  THEN RAISE EXCEPTION 'archived project cannot gain calculation authority' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION derived_input_reference_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.deleted_at IS NOT NULL AND (EXISTS(SELECT 1 FROM derived_dependencies WHERE tenant_id=NEW.tenant_id AND input_channel_id=NEW.id) OR EXISTS(SELECT 1 FROM derived_channels d JOIN derived_channel_heads h USING(tenant_id,channel_id) WHERE d.tenant_id=NEW.tenant_id AND d.channel_id=NEW.id AND (d.control_state<>'draft' OR h.current_root IS NOT NULL OR h.history_root IS NOT NULL)))
 THEN RAISE EXCEPTION 'calculation reference or published history protects channel' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER derived_input_reference_guard BEFORE UPDATE ON engineering_channels FOR EACH ROW EXECUTE FUNCTION derived_input_reference_guard();
CREATE CONSTRAINT TRIGGER derived_channel_extension AFTER INSERT OR UPDATE ON engineering_channels DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION derived_channel_integrity();
CREATE CONSTRAINT TRIGGER derived_extension_channel AFTER INSERT OR UPDATE ON derived_channels DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION derived_channel_integrity();
CREATE TRIGGER derived_configuration_guard BEFORE INSERT ON derived_channel_configs FOR EACH ROW EXECUTE FUNCTION derived_configuration_guard();
CREATE TRIGGER derived_configuration_immutable BEFORE UPDATE OR DELETE ON derived_channel_configs FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER derived_dependency_guard BEFORE INSERT ON derived_dependencies FOR EACH ROW EXECUTE FUNCTION derived_dependency_guard();
CREATE TRIGGER derived_dependency_immutable BEFORE UPDATE OR DELETE ON derived_dependencies FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER derived_assignment_guard BEFORE UPDATE OR DELETE ON derived_config_assignments FOR EACH ROW EXECUTE FUNCTION derived_assignment_guard();
CREATE TRIGGER monitoring_direct_head BEFORE INSERT OR UPDATE ON monitoring_source_heads FOR EACH ROW EXECUTE FUNCTION monitoring_direct_channel_guard();
CREATE TRIGGER monitoring_direct_binding BEFORE INSERT OR UPDATE ON monitoring_source_bindings FOR EACH ROW EXECUTE FUNCTION monitoring_direct_channel_guard();
CREATE TRIGGER derived_resource_grant_guard BEFORE INSERT OR UPDATE ON project_resource_grants FOR EACH ROW EXECUTE FUNCTION derived_resource_grant_guard();
CREATE OR REPLACE VIEW engineering_resource_nodes AS
 SELECT tenant_id,id AS facility_id,'facility'::text AS kind,id,NULL::text AS parent_kind,NULL::uuid AS parent_id,code,name,status,deleted_at,created_at,revision FROM engineering_facilities
 UNION ALL SELECT tenant_id,facility_id,'component',id,CASE WHEN parent_component_id IS NULL THEN 'facility' ELSE 'component' END,COALESCE(parent_component_id,facility_id),code,name,status,deleted_at,created_at,revision FROM engineering_components
 UNION ALL SELECT tenant_id,facility_id,'point',id,CASE WHEN component_id IS NULL THEN 'facility' ELSE 'component' END,COALESCE(component_id,facility_id),code,name,status,deleted_at,created_at,revision FROM engineering_points
 UNION ALL SELECT tenant_id,facility_id,'channel',id,owner_kind,CASE owner_kind WHEN 'point' THEN point_id WHEN 'component' THEN component_id ELSE facility_id END,code,name,status,deleted_at,created_at,revision FROM engineering_channels;
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'derived identities and configuration history require a compatible verified restore'; END $$;
-- +goose StatementEnd
