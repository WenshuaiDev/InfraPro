-- +goose Up
-- Durable input evidence is independent of interactive query snapshot expiry.
CREATE TABLE monitoring_raw_snapshots (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 project_id uuid, actor_id uuid REFERENCES accounts(id), system_run_id uuid,
 schema_version text NOT NULL CHECK(schema_version='raw-input-snapshot/1'),
 source_position bigint NOT NULL CHECK(source_position>=0), route_revision bigint NOT NULL CHECK(route_revision>0),
 needs jsonb NOT NULL CHECK(jsonb_typeof(needs)='array'), candidates jsonb NOT NULL CHECK(jsonb_typeof(candidates)='array'), manifest jsonb NOT NULL CHECK(jsonb_typeof(manifest)='array'),
 integrity jsonb NOT NULL CHECK(jsonb_typeof(integrity)='object'), history boolean NOT NULL,
 content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id), FOREIGN KEY(tenant_id,project_id) REFERENCES project_resource_policies(tenant_id,project_id),
 CHECK((actor_id IS NOT NULL AND system_run_id IS NULL) OR (actor_id IS NULL AND system_run_id IS NOT NULL))
);

-- Position allocation uses the existing tenant commit row lock, never a sequence.
-- Consumers have their own cursors; consuming summary dirty ranges cannot erase
-- these records or make a derived consumer miss a raw publication/revocation.
CREATE TABLE monitoring_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 source_position bigint NOT NULL CHECK(source_position>0), schema_version text NOT NULL DEFAULT 'monitoring-change/1' CHECK(schema_version='monitoring-change/1'),
 change_type text NOT NULL CHECK(change_type IN ('raw_publish','raw_revoke','raw_readopt','derived_publish','config_assignment_change','channel_control_change')),
 source_kind text NOT NULL CHECK(source_kind IN ('direct','derived')),
 source_id uuid NOT NULL, impact_hash text NOT NULL CHECK(impact_hash ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), UNIQUE(tenant_id,id), UNIQUE(tenant_id,source_position)
);
CREATE TABLE monitoring_change_ranges (
 tenant_id uuid NOT NULL, event_id uuid NOT NULL, ordinal integer NOT NULL CHECK(ordinal>=0),
 channel_id uuid NOT NULL, affected int8range NOT NULL CHECK(NOT isempty(affected)),
 PRIMARY KEY(tenant_id,event_id,ordinal), FOREIGN KEY(tenant_id,event_id) REFERENCES monitoring_changes(tenant_id,id),
 FOREIGN KEY(tenant_id,channel_id) REFERENCES engineering_channels(tenant_id,id)
);
CREATE INDEX monitoring_change_ranges_channel ON monitoring_change_ranges(tenant_id,channel_id,event_id);
CREATE INDEX monitoring_change_ranges_range ON monitoring_change_ranges USING gist(tenant_id public.gist_uuid_ops,channel_id public.gist_uuid_ops,affected);
CREATE TABLE monitoring_change_consumers (
 tenant_id uuid NOT NULL REFERENCES tenants(id), consumer text NOT NULL CHECK(length(consumer) BETWEEN 1 AND 100),
 durable_cursor bigint NOT NULL DEFAULT 0 CHECK(durable_cursor>=0), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,consumer)
);

-- +goose StatementBegin
CREATE FUNCTION monitoring_calculation_evidence_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'calculation evidence is immutable' USING ERRCODE='23514';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER monitoring_raw_snapshot_immutable BEFORE UPDATE OR DELETE ON monitoring_raw_snapshots FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TRIGGER monitoring_change_immutable BEFORE UPDATE OR DELETE ON monitoring_changes FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();
CREATE TRIGGER monitoring_change_range_immutable BEFORE UPDATE OR DELETE ON monitoring_change_ranges FOR EACH ROW EXECUTE FUNCTION monitoring_calculation_evidence_immutable();

ALTER TABLE monitoring_jobs DROP CONSTRAINT monitoring_jobs_kind_check;
ALTER TABLE monitoring_jobs ADD CONSTRAINT monitoring_jobs_kind_check CHECK(kind IN ('validate','publish','summary','export','verify','relocate','backup','restore','derive','analysis','derived_replay','analysis_compare'));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'durable calculation evidence cannot be downgraded'; END $$;
-- +goose StatementEnd
