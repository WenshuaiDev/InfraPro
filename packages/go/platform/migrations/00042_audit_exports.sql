-- +goose Up
ALTER TABLE task_records ALTER COLUMN tenant_id DROP NOT NULL;
ALTER TABLE task_records DROP CONSTRAINT task_records_scope_kind_check;
ALTER TABLE task_records ADD CONSTRAINT task_records_scope_kind_check CHECK(
 (scope_kind IN ('tenant','project') AND tenant_id IS NOT NULL) OR
 (scope_kind='platform' AND tenant_id IS NULL AND project_id IS NULL AND source_module='audit' AND source_kind='export' AND task_type='audit.export'));

CREATE TABLE audit_exports (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), scope_kind text NOT NULL, tenant_id uuid, project_id uuid,
 scope_key text NOT NULL, owner_id uuid NOT NULL REFERENCES accounts(id), membership_key text NOT NULL,
 filter jsonb NOT NULL CHECK(octet_length(filter::text)<=8192), field_profile text NOT NULL CHECK(field_profile IN ('standard','technical')),
 reason text NOT NULL CHECK(char_length(reason) BETWEEN 1 AND 500), operation_id uuid NOT NULL REFERENCES audit_operations(id),
 current_generation bigint NOT NULL DEFAULT 1, source_revision bigint NOT NULL DEFAULT 1, control_revision bigint NOT NULL DEFAULT 1,
 task_snapshot jsonb, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((scope_kind='platform' AND tenant_id IS NULL AND project_id IS NULL AND scope_key='platform') OR
 (scope_kind='tenant' AND tenant_id IS NOT NULL AND project_id IS NULL AND scope_key='tenant:'||tenant_id::text) OR
 (scope_kind='project' AND tenant_id IS NOT NULL AND project_id IS NOT NULL AND scope_key='project:'||project_id::text)),
 FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
CREATE TABLE audit_export_generations (
 export_id uuid NOT NULL REFERENCES audit_exports(id), generation bigint NOT NULL CHECK(generation>0), requested_by uuid NOT NULL,
 operation_id uuid NOT NULL REFERENCES audit_operations(id), authorization_vector text NOT NULL, projection_version bigint NOT NULL,
 restore_epoch uuid NOT NULL, state text NOT NULL CHECK(state IN ('queued','running','retry_wait','cancelling','succeeded','failed','cancelled')),
 phase text NOT NULL DEFAULT 'queued', fence bigint NOT NULL DEFAULT 0, attempt int NOT NULL DEFAULT 0,
 lease_owner text, lease_until timestamptz, next_attempt_at timestamptz, error_code text NOT NULL DEFAULT '',
 snapshot_at timestamptz, snapshot_upper_id bigint, snapshot_rows bigint, snapshot_bytes bigint, snapshot_digest text,
 object_id uuid, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), finished_at timestamptz,
 PRIMARY KEY(export_id,generation), CHECK((state IN ('succeeded','failed','cancelled'))=(finished_at IS NOT NULL))
);
CREATE INDEX audit_export_queue ON audit_export_generations(state,next_attempt_at,created_at) WHERE state IN ('queued','running','retry_wait','cancelling');
CREATE TABLE audit_export_rows (
 export_id uuid NOT NULL, generation bigint NOT NULL, ordinal bigint NOT NULL CHECK(ordinal>0), event_id bigint NOT NULL,
 projection jsonb NOT NULL CHECK(octet_length(projection::text)<=65536),
 PRIMARY KEY(export_id,generation,ordinal), FOREIGN KEY(export_id,generation) REFERENCES audit_export_generations(export_id,generation)
);
CREATE TABLE audit_export_objects (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), export_id uuid NOT NULL, generation bigint NOT NULL,
 object_key text NOT NULL UNIQUE CHECK(object_key LIKE 'audit-exports/v1/%' AND position('..' IN object_key)=0),
 fence bigint NOT NULL, restore_epoch uuid NOT NULL, state text NOT NULL CHECK(state IN ('intent','available','expired','invalidated','missing','delete_pending','deleted')),
 size_bytes bigint CHECK(size_bytes>=0), sha256 text CHECK(sha256 IS NULL OR length(sha256)=64),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz, deleted_at timestamptz,
 FOREIGN KEY(export_id,generation) REFERENCES audit_export_generations(export_id,generation)
);
CREATE TABLE audit_export_read_leases (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), object_id uuid NOT NULL REFERENCES audit_export_objects(id), actor_id uuid NOT NULL,
 node_id text NOT NULL, restore_epoch uuid NOT NULL, transfer_id uuid NOT NULL REFERENCES audit_transfer_receipts(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL, hard_expires_at timestamptz NOT NULL,
 closed_at timestamptz
);
CREATE INDEX audit_export_read_live ON audit_export_read_leases(object_id,expires_at) WHERE closed_at IS NULL;
-- +goose StatementBegin
CREATE FUNCTION audit_export_terminal_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state IN ('succeeded','failed','cancelled') AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'audit_execution_terminal_immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_export_terminal_immutable BEFORE UPDATE ON audit_export_generations FOR EACH ROW EXECUTE FUNCTION audit_export_terminal_immutable();
-- +goose StatementEnd
INSERT INTO task_source_states(source_module,source_kind,restore_epoch,backfill_complete) SELECT 'audit','export',restore_epoch,true FROM task_protocol WHERE id=1;
INSERT INTO audit_source_states(source_module,source_kind,complete) VALUES('audit','export',true);
CREATE VIEW audit_task_metadata AS SELECT id AS root_id,source_revision AS revision,task_snapshot AS snapshot FROM audit_exports WHERE task_snapshot IS NOT NULL;
CREATE OR REPLACE VIEW task_retained_sources AS
 SELECT snapshot->>'source_module' AS source_module,snapshot->>'source_kind' AS source_kind,(snapshot->>'source_root_id')::uuid AS source_root_id FROM monitoring_task_metadata WHERE snapshot IS NOT NULL
 UNION ALL SELECT 'spatial','job',id FROM spatial_task_metadata
 UNION ALL SELECT 'display','job',id FROM display_task_metadata
 UNION ALL SELECT 'derived','group',id FROM derived_task_groups
 UNION ALL SELECT 'files','job',id FROM file_task_metadata
 UNION ALL SELECT 'audit','export',id FROM audit_exports;
GRANT SELECT ON audit_task_metadata,task_retained_sources TO infrapro_task_index;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Audit export history is irreversible; restore a consistent backup'; END $$;
-- +goose StatementEnd
