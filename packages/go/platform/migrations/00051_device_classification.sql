-- +goose Up
CREATE TABLE device_category_identities (
 id uuid PRIMARY KEY, dictionary_code text NOT NULL DEFAULT 'device_category' CHECK(dictionary_code='device_category'),
 code text NOT NULL CHECK(code ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$'), code_normalized text GENERATED ALWAYS AS(lower(code)) STORED UNIQUE,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE device_category_releases (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), number bigint NOT NULL UNIQUE CHECK(number>0),
 content jsonb NOT NULL, content_hash text NOT NULL, created_by uuid NOT NULL REFERENCES accounts(id),
 reason text NOT NULL CHECK(length(btrim(reason))>0), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 created_txid bigint NOT NULL DEFAULT txid_current()
);
CREATE TABLE device_category_release_nodes (
 release_id uuid NOT NULL REFERENCES device_category_releases(id), category_id uuid NOT NULL REFERENCES device_category_identities(id),
 parent_id uuid, name text NOT NULL, description text NOT NULL DEFAULT '', sort_order integer NOT NULL,
 selectable boolean NOT NULL, status text NOT NULL CHECK(status IN ('active','inactive')),
 path uuid[] NOT NULL, name_path text[] NOT NULL, sensor boolean NOT NULL,
 PRIMARY KEY(release_id,category_id), FOREIGN KEY(release_id,parent_id) REFERENCES device_category_release_nodes(release_id,category_id) DEFERRABLE INITIALLY DEFERRED,
 CHECK(cardinality(path) BETWEEN 1 AND 128), CHECK(cardinality(path)=cardinality(name_path)), CHECK(path[cardinality(path)]=category_id), CHECK(parent_id IS DISTINCT FROM category_id)
);
CREATE INDEX device_category_ancestors ON device_category_release_nodes USING gin(path);
CREATE TABLE device_classification_state (
 id smallint PRIMARY KEY DEFAULT 1 CHECK(id=1), revision bigint NOT NULL DEFAULT 1,
 current_release_id uuid REFERENCES device_category_releases(id), draft_revision bigint NOT NULL DEFAULT 1,
 draft jsonb NOT NULL DEFAULT '{"dictionary_code":"device_category","sensor_root_id":"","nodes":[]}',
 draft_updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), draft_updated_by uuid REFERENCES accounts(id)
);
INSERT INTO device_classification_state(id) VALUES(1);
CREATE TABLE product_classification_records (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), product_version_id uuid NOT NULL REFERENCES device_product_versions(id),
 revision bigint NOT NULL CHECK(revision>0), category_id uuid, selected_release_id uuid,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), created_by uuid NOT NULL REFERENCES accounts(id), reason text NOT NULL,
 ended_at timestamptz, request_id uuid NOT NULL,
 FOREIGN KEY(selected_release_id,category_id) REFERENCES device_category_release_nodes(release_id,category_id),
 CHECK((category_id IS NULL)=(selected_release_id IS NULL)), UNIQUE(product_version_id,revision)
);
CREATE UNIQUE INDEX product_classification_current ON product_classification_records(product_version_id) WHERE ended_at IS NULL;
CREATE TABLE device_classification_records (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, device_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision>0), mode text NOT NULL CHECK(mode IN ('override','inherit')), category_id uuid, selected_release_id uuid,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), created_by uuid NOT NULL REFERENCES accounts(id), reason text NOT NULL CHECK(length(btrim(reason))>0),
 ended_at timestamptz, request_id uuid NOT NULL,
 FOREIGN KEY(tenant_id,device_id) REFERENCES devices(tenant_id,id),
 FOREIGN KEY(selected_release_id,category_id) REFERENCES device_category_release_nodes(release_id,category_id),
 CHECK((category_id IS NULL)=(selected_release_id IS NULL)), CHECK((mode='override')=(category_id IS NOT NULL)), UNIQUE(tenant_id,device_id,revision)
);
CREATE UNIQUE INDEX device_classification_current ON device_classification_records(tenant_id,device_id) WHERE ended_at IS NULL;
CREATE TABLE device_classification_previews (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), actor_id uuid NOT NULL REFERENCES accounts(id), tenant_id uuid REFERENCES tenants(id),
 membership_id uuid REFERENCES tenant_memberships(id), entry_scope text NOT NULL CHECK(entry_scope IN ('platform','tenant')),
 input jsonb NOT NULL, preview_hash text NOT NULL, evidence jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL,
 request_id uuid NOT NULL, UNIQUE(actor_id,request_id)
);
CREATE TABLE device_classification_changes (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), entry_scope text NOT NULL, tenant_id uuid REFERENCES tenants(id), actor_id uuid NOT NULL REFERENCES accounts(id),
 action text NOT NULL, target_kind text NOT NULL, target_id uuid NOT NULL, before jsonb, after jsonb NOT NULL,
 request_id uuid NOT NULL, reason text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE device_classification_receipts (
 actor_id uuid NOT NULL REFERENCES accounts(id), request_id uuid NOT NULL, entry_scope text NOT NULL, tenant_id uuid, membership_id uuid,
 action text NOT NULL, payload_hash text NOT NULL, result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(actor_id,request_id)
);
-- Legacy versions remain explicitly unclassified; no names or outputs are guessed.
INSERT INTO product_classification_records(product_version_id,revision,created_by,reason,request_id)
SELECT id,1,published_by,'旧产品迁移：未分类，待明确补充',gen_random_uuid() FROM device_product_versions;
CREATE VIEW device_effective_classifications AS
 SELECT d.tenant_id,d.id AS device_id,d.product_version_id,COALESCE(o.revision,0) AS override_revision,COALESCE(p.revision,0) AS product_classification_revision,
 CASE WHEN o.mode='override' THEN 'device_override' ELSE 'product_default' END AS origin,
 CASE WHEN o.mode='override' THEN o.category_id ELSE p.category_id END AS category_id,
 CASE WHEN o.mode='override' THEN o.selected_release_id ELSE p.selected_release_id END AS selected_release_id,
 CASE WHEN o.mode='override' THEN o.id ELSE p.id END AS record_id,
 s.current_release_id AS dictionary_release_id,s.revision AS classification_revision,
 CASE WHEN COALESCE(CASE WHEN o.mode='override' THEN o.category_id ELSE p.category_id END::text,'')='' THEN 'unclassified' WHEN n.category_id IS NULL THEN 'unavailable' ELSE 'classified' END AS state,
 n.name,n.path,n.name_path,n.sensor,n.status AS category_status
 FROM devices d CROSS JOIN device_classification_state s
 LEFT JOIN product_classification_records p ON p.product_version_id=d.product_version_id AND p.ended_at IS NULL
 LEFT JOIN device_classification_records o ON o.tenant_id=d.tenant_id AND o.device_id=d.id AND o.ended_at IS NULL
 LEFT JOIN device_category_release_nodes n ON n.release_id=s.current_release_id AND n.category_id=CASE WHEN o.mode='override' THEN o.category_id ELSE p.category_id END;
-- +goose StatementBegin
CREATE FUNCTION device_classification_record_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR TG_OP='UPDATE' AND (OLD.ended_at IS NOT NULL OR NEW.ended_at IS NULL OR (to_jsonb(NEW)-'ended_at') IS DISTINCT FROM (to_jsonb(OLD)-'ended_at')) THEN RAISE EXCEPTION 'classification history is immutable' USING ERRCODE='23514'; END IF;
 IF TG_OP='INSERT' AND NEW.category_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM device_category_release_nodes n JOIN device_classification_state s ON s.current_release_id=n.release_id WHERE n.release_id=NEW.selected_release_id AND n.category_id=NEW.category_id AND n.selectable AND n.status='active' AND NOT EXISTS(SELECT 1 FROM device_category_release_nodes a WHERE a.release_id=n.release_id AND a.category_id=ANY(n.path) AND a.status='inactive')) THEN RAISE EXCEPTION 'classification selection is unavailable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION device_category_publication_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE body jsonb; sensor_root uuid; count_nodes integer;
BEGIN
 body:=NEW.content; sensor_root:=NULLIF(body->>'sensor_root_id','')::uuid;
 SELECT count(*) INTO count_nodes FROM device_category_release_nodes WHERE release_id=NEW.id;
 IF body->>'dictionary_code'<>'device_category' OR jsonb_typeof(body->'nodes')<>'array' OR count_nodes<>jsonb_array_length(body->'nodes') OR count_nodes>10000 THEN RAISE EXCEPTION 'invalid dictionary publication' USING ERRCODE='23514'; END IF;
 IF sensor_root IS NULL OR NOT EXISTS(SELECT 1 FROM device_category_release_nodes WHERE release_id=NEW.id AND category_id=sensor_root) THEN RAISE EXCEPTION 'sensor root must be explicitly declared' USING ERRCODE='23514'; END IF;
 IF EXISTS(SELECT 1 FROM device_category_release_nodes n WHERE n.release_id=NEW.id AND ((n.parent_id IS NULL AND n.path<>ARRAY[n.category_id]) OR (n.parent_id IS NOT NULL AND n.path IS DISTINCT FROM (SELECT p.path||n.category_id FROM device_category_release_nodes p WHERE p.release_id=n.release_id AND p.category_id=n.parent_id)) OR n.sensor<>(sensor_root=ANY(n.path)) OR cardinality(n.path)<>(SELECT count(DISTINCT v) FROM unnest(n.path)v))) THEN RAISE EXCEPTION 'dictionary path integrity failed' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE FUNCTION device_category_node_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'published category immutable' USING ERRCODE='23514'; END IF;
 IF NOT EXISTS(SELECT 1 FROM device_category_releases WHERE id=NEW.release_id AND created_txid=txid_current()) THEN RAISE EXCEPTION 'published release cannot gain nodes' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER device_category_identity_immutable BEFORE UPDATE OR DELETE ON device_category_identities FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_category_release_immutable BEFORE UPDATE OR DELETE ON device_category_releases FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_category_node_immutable BEFORE INSERT OR UPDATE OR DELETE ON device_category_release_nodes FOR EACH ROW EXECUTE FUNCTION device_category_node_guard();
CREATE CONSTRAINT TRIGGER device_category_publication AFTER INSERT ON device_category_releases DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION device_category_publication_guard();
CREATE TRIGGER product_classification_record_guard BEFORE INSERT OR UPDATE OR DELETE ON product_classification_records FOR EACH ROW EXECUTE FUNCTION device_classification_record_guard();
CREATE TRIGGER device_classification_record_guard BEFORE INSERT OR UPDATE OR DELETE ON device_classification_records FOR EACH ROW EXECUTE FUNCTION device_classification_record_guard();
CREATE TRIGGER device_classification_changes_immutable BEFORE UPDATE OR DELETE ON device_classification_changes FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_classification_previews_immutable BEFORE UPDATE OR DELETE ON device_classification_previews FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TRIGGER device_classification_receipts_immutable BEFORE UPDATE OR DELETE ON device_classification_receipts FOR EACH ROW EXECUTE FUNCTION engineering_immutable();
CREATE TABLE engineering_category_dependencies (
 version_id uuid NOT NULL REFERENCES engineering_versions(id), item_key text NOT NULL,
 release_id uuid NOT NULL, category_id uuid NOT NULL,
 PRIMARY KEY(version_id,item_key), FOREIGN KEY(release_id,category_id) REFERENCES device_category_release_nodes(release_id,category_id)
);
CREATE TRIGGER engineering_category_dependency_immutable BEFORE UPDATE OR DELETE ON engineering_category_dependencies FOR EACH ROW EXECUTE FUNCTION engineering_immutable();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Classification history requires compatible programs; restore a verified compatible backup'; END $$;
-- +goose StatementEnd
