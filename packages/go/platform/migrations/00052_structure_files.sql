-- +goose Up
CREATE TABLE engineering_resource_attachments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, facility_id uuid NOT NULL,
 resource_kind text NOT NULL CHECK(resource_kind IN ('facility','component','monitoring_section','borehole','point')), resource_id uuid NOT NULL,
 scope_kind text NOT NULL CHECK(scope_kind IN ('tenant','project')), project_id uuid,
 attachment_id uuid NOT NULL REFERENCES file_attachments(id), attachment_revision_id uuid NOT NULL REFERENCES file_attachment_revisions(id),
 purpose text NOT NULL CHECK(length(purpose) BETWEEN 1 AND 2000), created_by uuid NOT NULL REFERENCES accounts(id), reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), ended_at timestamptz, ended_reason text,
 FOREIGN KEY(tenant_id,facility_id,resource_kind,resource_id) REFERENCES engineering_structure_nodes(tenant_id,facility_id,kind,id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id),
 CHECK((scope_kind='tenant' AND project_id IS NULL) OR (scope_kind='project' AND project_id IS NOT NULL)),
 CHECK((ended_at IS NULL)=(ended_reason IS NULL))
);
CREATE INDEX engineering_attachment_owner ON engineering_resource_attachments(tenant_id,facility_id,resource_kind,resource_id,id);
CREATE UNIQUE INDEX engineering_attachment_active ON engineering_resource_attachments(tenant_id,resource_kind,resource_id,attachment_revision_id) WHERE ended_at IS NULL;
-- +goose StatementBegin
CREATE FUNCTION engineering_attachment_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR TG_OP='UPDATE' AND (OLD.ended_at IS NOT NULL OR (to_jsonb(NEW)-'ended_at'-'ended_reason') IS DISTINCT FROM (to_jsonb(OLD)-'ended_at'-'ended_reason') OR NEW.ended_at IS NULL) THEN
  RAISE EXCEPTION 'engineering attachment evidence is immutable' USING ERRCODE='23514';
 END IF;
 IF TG_OP='INSERT' AND NOT EXISTS(SELECT 1 FROM file_attachment_revisions r JOIN file_spaces s ON s.id=r.space_id WHERE r.id=NEW.attachment_revision_id AND r.attachment_id=NEW.attachment_id AND s.tenant_id=NEW.tenant_id AND s.scope_kind=NEW.scope_kind AND s.project_id IS NOT DISTINCT FROM NEW.project_id) THEN
  RAISE EXCEPTION 'attachment scope mismatch' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION engineering_attachment_resource_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL AND EXISTS(SELECT 1 FROM engineering_resource_attachments r WHERE r.tenant_id=NEW.tenant_id AND r.resource_kind=TG_ARGV[0] AND r.resource_id=NEW.id) THEN
  RAISE EXCEPTION 'retained engineering file evidence prevents deletion' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER engineering_attachment_immutable BEFORE INSERT OR UPDATE OR DELETE ON engineering_resource_attachments FOR EACH ROW EXECUTE FUNCTION engineering_attachment_guard();
CREATE TRIGGER engineering_attachment_retain BEFORE UPDATE OF deleted_at ON engineering_facilities FOR EACH ROW EXECUTE FUNCTION engineering_attachment_resource_guard('facility');
CREATE TRIGGER engineering_attachment_retain BEFORE UPDATE OF deleted_at ON engineering_components FOR EACH ROW EXECUTE FUNCTION engineering_attachment_resource_guard('component');
CREATE TRIGGER engineering_attachment_retain BEFORE UPDATE OF deleted_at ON engineering_monitoring_sections FOR EACH ROW EXECUTE FUNCTION engineering_attachment_resource_guard('monitoring_section');
CREATE TRIGGER engineering_attachment_retain BEFORE UPDATE OF deleted_at ON engineering_boreholes FOR EACH ROW EXECUTE FUNCTION engineering_attachment_resource_guard('borehole');
CREATE TRIGGER engineering_attachment_retain BEFORE UPDATE OF deleted_at ON engineering_points FOR EACH ROW EXECUTE FUNCTION engineering_attachment_resource_guard('point');
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'engineering file evidence requires restore of a verified backup'; END $$;
-- +goose StatementEnd
