-- +goose Up
-- Additive definition storage. No sample business data or custom role grants.
CREATE TABLE derived_graph_states (
 tenant_id uuid PRIMARY KEY REFERENCES tenants(id),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0)
);
INSERT INTO derived_graph_states(tenant_id) SELECT id FROM tenants;
CREATE TABLE derived_metric_definitions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'),
 code_normalized text GENERATED ALWAYS AS (lower(code COLLATE "C")) STORED,
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 description text NOT NULL DEFAULT '' CHECK(length(description)<=4000),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','deleted')),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 deleted_at timestamptz,
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,code_normalized),
 CHECK((status='deleted')=(deleted_at IS NOT NULL))
);
CREATE INDEX derived_definitions_list ON derived_metric_definitions(tenant_id,status,created_at,id);
CREATE TABLE derived_metric_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL,
 definition_id uuid NOT NULL, version_no bigint NOT NULL CHECK(version_no>0),
 name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),
 rule jsonb NOT NULL CHECK(jsonb_typeof(rule)='object'),
 compiled_plan jsonb NOT NULL CHECK(jsonb_typeof(compiled_plan)='object'),
 schema_version text NOT NULL, compiler_version text NOT NULL, engine_version text NOT NULL,
 content_hash text NOT NULL CHECK(content_hash ~ '^[0-9a-f]{64}$'),
 output_contract_hash text NOT NULL CHECK(output_contract_hash ~ '^[0-9a-f]{64}$'),
 enabled boolean NOT NULL DEFAULT true, state_revision bigint NOT NULL DEFAULT 1 CHECK(state_revision>0),
 published_by uuid NOT NULL REFERENCES accounts(id), published_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(tenant_id,id), UNIQUE(tenant_id,definition_id,id), UNIQUE(tenant_id,definition_id,version_no),
 FOREIGN KEY(tenant_id,definition_id) REFERENCES derived_metric_definitions(tenant_id,id)
);
CREATE TABLE derived_metric_drafts (
 tenant_id uuid NOT NULL, definition_id uuid PRIMARY KEY, base_version_id uuid,
 revision bigint NOT NULL CHECK(revision>0), rule jsonb NOT NULL CHECK(jsonb_typeof(rule)='object'),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,definition_id) REFERENCES derived_metric_definitions(tenant_id,id),
 FOREIGN KEY(tenant_id,definition_id,base_version_id) REFERENCES derived_metric_versions(tenant_id,definition_id,id)
);
-- Every public metric/unit reference is protected and can be resolved in bulk.
CREATE TABLE derived_metric_references (
 tenant_id uuid NOT NULL, version_id uuid NOT NULL, engineering_version_id uuid NOT NULL REFERENCES engineering_versions(id),
 PRIMARY KEY(tenant_id,version_id,engineering_version_id),
 FOREIGN KEY(tenant_id,version_id) REFERENCES derived_metric_versions(tenant_id,id)
);
CREATE INDEX derived_engineering_references ON derived_metric_references(engineering_version_id);
CREATE TABLE derived_operation_receipts (
 tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid,
 scope_key text GENERATED ALWAYS AS (COALESCE(project_id::text,'tenant')) STORED,
 actor_id uuid NOT NULL REFERENCES accounts(id), request_id uuid NOT NULL,
 operation text NOT NULL, payload_hash text NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'), result jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(tenant_id,scope_key,actor_id,request_id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
CREATE TABLE derived_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid,
 actor_kind text NOT NULL CHECK(actor_kind IN ('user','system','operator')), actor_id uuid REFERENCES accounts(id),
 operation text NOT NULL, target_kind text NOT NULL, target_id uuid NOT NULL,
 request_id uuid NOT NULL, reason text NOT NULL, before jsonb, after jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id),
 CHECK(actor_kind<>'user' OR actor_id IS NOT NULL)
);
CREATE INDEX derived_changes_target ON derived_changes(tenant_id,target_kind,target_id,created_at,id);
-- +goose StatementBegin
CREATE FUNCTION derived_initialize_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO derived_graph_states(tenant_id) VALUES(NEW.id); RETURN NEW; END $$;
CREATE FUNCTION derived_definition_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'derived identities remain reserved' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF OLD.status='deleted' OR ROW(NEW.id,NEW.tenant_id,NEW.code,NEW.created_by,NEW.created_at) IS DISTINCT FROM ROW(OLD.id,OLD.tenant_id,OLD.code,OLD.created_by,OLD.created_at)
  THEN RAISE EXCEPTION 'derived identity is immutable' USING ERRCODE='23514'; END IF;
  IF NEW.revision<>OLD.revision+1 THEN RAISE EXCEPTION 'derived revision must advance' USING ERRCODE='23514'; END IF;
  IF NEW.deleted_at IS NOT NULL AND EXISTS(SELECT 1 FROM derived_metric_versions WHERE tenant_id=NEW.tenant_id AND definition_id=NEW.id)
  THEN RAISE EXCEPTION 'published definitions cannot be deleted' USING ERRCODE='23514'; END IF;
 END IF; RETURN NEW;
END $$;
CREATE FUNCTION derived_version_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'published derived version is permanent' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND ((to_jsonb(NEW)-'enabled'-'state_revision') IS DISTINCT FROM (to_jsonb(OLD)-'enabled'-'state_revision') OR NEW.state_revision<>OLD.state_revision+1)
 THEN RAISE EXCEPTION 'published derived content is immutable' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM derived_metric_definitions d WHERE d.tenant_id=NEW.tenant_id AND d.id=NEW.definition_id AND d.status='active')
 THEN RAISE EXCEPTION 'derived definition unavailable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION derived_draft_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM derived_metric_definitions d WHERE d.tenant_id=NEW.tenant_id AND d.id=NEW.definition_id AND d.status='active' AND d.revision=NEW.revision)
 THEN RAISE EXCEPTION 'derived draft scope or revision mismatch' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER derived_tenant_initialize AFTER INSERT ON tenants FOR EACH ROW EXECUTE FUNCTION derived_initialize_scope();
CREATE TRIGGER derived_definition_guard BEFORE INSERT OR UPDATE OR DELETE ON derived_metric_definitions FOR EACH ROW EXECUTE FUNCTION derived_definition_guard();
CREATE TRIGGER derived_version_guard BEFORE INSERT OR UPDATE OR DELETE ON derived_metric_versions FOR EACH ROW EXECUTE FUNCTION derived_version_guard();
CREATE TRIGGER derived_draft_guard BEFORE INSERT OR UPDATE ON derived_metric_drafts FOR EACH ROW EXECUTE FUNCTION derived_draft_guard();
CREATE TRIGGER derived_metric_reference_immutable BEFORE UPDATE OR DELETE ON derived_metric_references FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER derived_receipt_immutable BEFORE UPDATE OR DELETE ON derived_operation_receipts FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER derived_change_immutable BEFORE UPDATE OR DELETE ON derived_changes FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
-- +goose Down
-- Permanent definitions and evidence cannot be dropped by an automatic rollback.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'derived monitoring rollback requires a verified compatible restore plan'; END $$;
-- +goose StatementEnd
