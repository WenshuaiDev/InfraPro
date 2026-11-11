-- +goose Up
CREATE TABLE engineering_catalog_state (id integer PRIMARY KEY CHECK(id=1));
INSERT INTO engineering_catalog_state VALUES(1);
CREATE TABLE engineering_unit_categories (
 key text PRIMARY KEY CHECK(key ~ '^[a-z][a-z0-9_]{0,63}$'),
 name text NOT NULL, description text NOT NULL DEFAULT '', revision integer NOT NULL DEFAULT 1 CHECK(revision>0)
);
CREATE TABLE engineering_definitions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 kind text NOT NULL CHECK(kind IN ('type-definitions','metrics','units','monitoring-templates')),
 tenant_id uuid REFERENCES tenants(id),
 scope_key text GENERATED ALWAYS AS (COALESCE(tenant_id::text,'public')) STORED,
 code text NOT NULL CHECK(code ~ '^[a-z][a-z0-9_]{0,63}$'),
 source_version uuid, source_snapshot jsonb,
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_by uuid NOT NULL REFERENCES accounts(id),
 deleted_at timestamptz,
 CHECK(tenant_id IS NULL OR kind='monitoring-templates'),
 UNIQUE(scope_key,kind,code)
);
CREATE TABLE engineering_versions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), definition_id uuid NOT NULL REFERENCES engineering_definitions(id),
 number integer NOT NULL CHECK(number>0), content jsonb NOT NULL CHECK(jsonb_typeof(content)='object'),
 digest text NOT NULL, schema_version text NOT NULL CHECK(schema_version='1'), schemas jsonb NOT NULL,
 unit_category jsonb,
 published_at timestamptz NOT NULL DEFAULT clock_timestamp(), published_by uuid NOT NULL REFERENCES accounts(id),
 UNIQUE(definition_id,number), UNIQUE(definition_id,id)
);
ALTER TABLE engineering_definitions ADD CONSTRAINT engineering_source FOREIGN KEY(source_version) REFERENCES engineering_versions(id);
CREATE TABLE engineering_drafts (
 definition_id uuid PRIMARY KEY REFERENCES engineering_definitions(id),
 revision integer NOT NULL CHECK(revision>0), base_version uuid,
 content jsonb NOT NULL CHECK(jsonb_typeof(content)='object'),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_by uuid NOT NULL REFERENCES accounts(id),
 FOREIGN KEY(definition_id,base_version) REFERENCES engineering_versions(definition_id,id)
);
CREATE TABLE engineering_version_states (
 version_id uuid PRIMARY KEY REFERENCES engineering_versions(id), enabled boolean NOT NULL DEFAULT true,
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0), reason text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_by uuid NOT NULL REFERENCES accounts(id)
);
CREATE TABLE engineering_dependencies (
 version_id uuid NOT NULL REFERENCES engineering_versions(id), target_id uuid NOT NULL REFERENCES engineering_versions(id),
 path text NOT NULL, kind text NOT NULL, PRIMARY KEY(version_id,path,target_id), CHECK(version_id<>target_id)
);
CREATE INDEX engineering_dependencies_target ON engineering_dependencies(target_id);
CREATE INDEX engineering_definitions_scope ON engineering_definitions(scope_key,kind,created_at,id) WHERE deleted_at IS NULL;
CREATE TABLE engineering_operation_receipts (
 scope_key text NOT NULL, actor_id uuid NOT NULL REFERENCES accounts(id), request_id uuid NOT NULL,
 operation text NOT NULL, payload_hash text NOT NULL, result jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(scope_key,actor_id,request_id)
);
-- +goose StatementBegin
CREATE FUNCTION engineering_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'engineering published history is immutable' USING ERRCODE='23514'; END $$;
CREATE FUNCTION engineering_identity_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.id,NEW.kind,NEW.tenant_id,NEW.code,NEW.source_version,NEW.source_snapshot) IS DISTINCT FROM (OLD.id,OLD.kind,OLD.tenant_id,OLD.code,OLD.source_version,OLD.source_snapshot)
 THEN RAISE EXCEPTION 'engineering identity is immutable' USING ERRCODE='23514'; END IF;
 IF NEW.deleted_at IS NOT NULL AND EXISTS(SELECT 1 FROM engineering_versions WHERE definition_id=NEW.id)
 THEN RAISE EXCEPTION 'published definition cannot be deleted' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION engineering_dependency_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM engineering_version_states WHERE version_id=NEW.version_id)
 THEN RAISE EXCEPTION 'published dependency list is immutable' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id
   JOIN engineering_versions target ON target.id=NEW.target_id JOIN engineering_definitions td ON td.id=target.definition_id
   WHERE v.id=NEW.version_id AND ((d.kind IN ('type-definitions','metrics') AND td.kind='units') OR
   (d.kind='monitoring-templates' AND td.kind IN ('type-definitions','metrics','units'))))
 THEN RAISE EXCEPTION 'invalid engineering dependency direction' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM engineering_versions v JOIN engineering_definitions d ON d.id=v.definition_id
   WHERE v.id=NEW.target_id AND d.tenant_id IS NULL AND d.kind<>'monitoring-templates')
 THEN RAISE EXCEPTION 'engineering dependencies must be public type, metric or unit versions' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION engineering_publication_complete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM engineering_version_states WHERE version_id=NEW.id)
 THEN RAISE EXCEPTION 'incomplete engineering publication' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION engineering_category_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM engineering_versions WHERE unit_category->>'key'=OLD.key)
 OR EXISTS(SELECT 1 FROM engineering_drafts WHERE content->'unit'->>'category'=OLD.key OR content->'metric'->>'category'=OLD.key)
 THEN RAISE EXCEPTION 'referenced unit category is immutable' USING ERRCODE='23514'; END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 IF NEW.key<>OLD.key THEN RAISE EXCEPTION 'category key is immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER engineering_complete AFTER INSERT ON engineering_versions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION engineering_publication_complete();
CREATE TRIGGER engineering_version_history BEFORE UPDATE OR DELETE ON engineering_versions FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER engineering_dependency_history BEFORE UPDATE OR DELETE ON engineering_dependencies FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER engineering_dependency_scope_guard BEFORE INSERT ON engineering_dependencies FOR EACH ROW EXECUTE FUNCTION engineering_dependency_scope();
CREATE TRIGGER engineering_identity_guard BEFORE UPDATE ON engineering_definitions FOR EACH ROW EXECUTE FUNCTION engineering_identity_immutable();
CREATE TRIGGER engineering_code_reservation BEFORE DELETE ON engineering_definitions FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER engineering_category_usage BEFORE UPDATE OR DELETE ON engineering_unit_categories FOR EACH ROW EXECUTE FUNCTION engineering_category_guard();
CREATE TRIGGER engineering_receipt_history BEFORE UPDATE OR DELETE ON engineering_operation_receipts FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Engineering history is irreversible; restore a verified backup instead'; END $$;
-- +goose StatementEnd
