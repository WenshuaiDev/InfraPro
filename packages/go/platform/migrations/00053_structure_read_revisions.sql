-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION engineering_channel_graph_budget() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM id FROM engineering_facilities WHERE tenant_id=NEW.tenant_id AND id=NEW.facility_id FOR UPDATE;
 IF (SELECT count(*) FROM engineering_structure_nodes WHERE tenant_id=NEW.tenant_id AND facility_id=NEW.facility_id)+(SELECT count(*) FROM engineering_channels WHERE tenant_id=NEW.tenant_id AND facility_id=NEW.facility_id)>=100000 THEN
  RAISE EXCEPTION 'structure resource budget 100000 exceeded' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER engineering_channel_graph_budget BEFORE INSERT ON engineering_channels FOR EACH ROW EXECUTE FUNCTION engineering_channel_graph_budget();
ALTER TABLE display_read_scope_members DROP CONSTRAINT display_read_scope_members_check;
ALTER TABLE display_read_scope_members ADD CONSTRAINT display_read_scope_members_check CHECK((domain='resources' AND kind IN ('facility','component','monitoring_section','borehole','point','channel')) OR (domain='channels' AND kind='channel') OR (domain='devices' AND kind IN ('device','output')) OR (domain='spatial' AND kind='version'));
ALTER TABLE display_read_selections DROP CONSTRAINT display_read_selections_basis_check;
ALTER TABLE display_read_selections DROP CONSTRAINT display_read_selections_purpose_check;
ALTER TABLE display_read_selections ADD CONSTRAINT display_read_selections_basis_check CHECK(basis IN ('raw_active','raw_samples','derived_logical','derived_samples','resource_count','device_count'));
ALTER TABLE display_read_selections ADD CONSTRAINT display_read_selections_purpose_check CHECK(purpose IN ('details','trends','statistics','resources','devices'));
-- Metadata revisions refresh new queries without rewriting a frozen query's
-- grouping. Deletion increments the safety revision and rejects old delivery.
CREATE TRIGGER display_read_section AFTER INSERT OR UPDATE OR DELETE ON engineering_monitoring_sections FOR EACH ROW EXECUTE FUNCTION display_read_changed('resources','metadata');
CREATE TRIGGER display_read_borehole AFTER INSERT OR UPDATE OR DELETE ON engineering_boreholes FOR EACH ROW EXECUTE FUNCTION display_read_changed('resources','metadata');
CREATE TRIGGER display_read_structure AFTER INSERT OR UPDATE OR DELETE ON engineering_structure_nodes FOR EACH ROW EXECUTE FUNCTION display_read_changed('structure','metadata');
CREATE TRIGGER display_read_borehole_position AFTER INSERT OR UPDATE OR DELETE ON point_borehole_positions FOR EACH ROW EXECUTE FUNCTION display_read_changed('structure_positions','metadata');
CREATE TRIGGER display_read_structure_reference AFTER INSERT OR UPDATE OR DELETE ON engineering_resource_references FOR EACH ROW EXECUTE FUNCTION display_read_changed('structure_references','metadata');
CREATE TRIGGER display_read_structure_attachment AFTER INSERT OR UPDATE OR DELETE ON engineering_resource_attachments FOR EACH ROW EXECUTE FUNCTION display_read_changed('structure_attachments','metadata');
-- +goose Down
DROP TRIGGER engineering_channel_graph_budget ON engineering_channels;
DROP FUNCTION engineering_channel_graph_budget();
ALTER TABLE display_read_selections DROP CONSTRAINT display_read_selections_basis_check;
ALTER TABLE display_read_selections DROP CONSTRAINT display_read_selections_purpose_check;
ALTER TABLE display_read_selections ADD CONSTRAINT display_read_selections_basis_check CHECK(basis IN ('raw_active','raw_samples','derived_logical','derived_samples'));
ALTER TABLE display_read_selections ADD CONSTRAINT display_read_selections_purpose_check CHECK(purpose IN ('details','trends','statistics'));
-- A fixed new-model grant cannot be translated into a legacy grant on downgrade.
ALTER TABLE display_read_scope_members DROP CONSTRAINT display_read_scope_members_check;
ALTER TABLE display_read_scope_members ADD CONSTRAINT display_read_scope_members_check CHECK((domain='resources' AND kind IN ('facility','component','point','channel')) OR (domain='channels' AND kind='channel') OR (domain='devices' AND kind IN ('device','output')) OR (domain='spatial' AND kind='version'));
DROP TRIGGER display_read_structure_attachment ON engineering_resource_attachments;
DROP TRIGGER display_read_structure_reference ON engineering_resource_references;
DROP TRIGGER display_read_borehole_position ON point_borehole_positions;
DROP TRIGGER display_read_structure ON engineering_structure_nodes;
DROP TRIGGER display_read_borehole ON engineering_boreholes;
DROP TRIGGER display_read_section ON engineering_monitoring_sections;
