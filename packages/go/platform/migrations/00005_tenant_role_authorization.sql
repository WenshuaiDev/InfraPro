-- +goose Up
-- Validate the old model before relaxing ordinary memberships; never repair ambiguous data.
-- +goose StatementBegin
DO $$ DECLARE broken uuid; BEGIN
 SELECT id INTO broken FROM tenants WHERE NOT tenant_structure_valid(id) LIMIT 1;
 IF broken IS NOT NULL THEN RAISE EXCEPTION 'invalid v4 tenant structure: %',broken; END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE permission_catalog ADD COLUMN tenant_custom_role_assignable boolean NOT NULL DEFAULT false;
ALTER TABLE permission_catalog ADD CONSTRAINT tenant_assignable_scope CHECK(NOT tenant_custom_role_assignable OR scope='tenant');
ALTER TABLE roles ADD COLUMN owner_tenant_id uuid REFERENCES tenants(id),
 ADD COLUMN source_role_id uuid REFERENCES roles(id), ADD COLUMN source_role_version bigint,
 ADD COLUMN assignment_policy text NOT NULL DEFAULT 'manual' CHECK(assignment_policy IN ('manual','administrator_transfer_only')),
 ADD COLUMN created_by uuid REFERENCES accounts(id), ADD COLUMN updated_by uuid REFERENCES accounts(id),
 ADD COLUMN creation_request_id uuid, ADD COLUMN creation_payload_hash text,
 ADD CONSTRAINT role_owner_scope CHECK((binding_scope='platform' AND owner_tenant_id IS NULL) OR
  (binding_scope='tenant' AND ((owner_tenant_id IS NULL AND builtin_key IS NOT NULL) OR (owner_tenant_id IS NOT NULL AND builtin_key IS NULL)))),
 ADD CONSTRAINT role_source_pair CHECK((source_role_id IS NULL)=(source_role_version IS NULL)),
 ADD CONSTRAINT role_assignment_policy CHECK((assignment_policy='administrator_transfer_only')=(COALESCE(builtin_key,'')='tenant_admin')) NOT VALID;
UPDATE roles SET assignment_policy='administrator_transfer_only' WHERE builtin_key='tenant_admin';
ALTER TABLE roles VALIDATE CONSTRAINT role_assignment_policy;
DROP INDEX roles_name;
CREATE UNIQUE INDEX roles_platform_name ON roles(name_normalized) WHERE binding_scope='platform' AND deleted_at IS NULL;
CREATE UNIQUE INDEX roles_preset_name ON roles(name_normalized) WHERE binding_scope='tenant' AND owner_tenant_id IS NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX roles_tenant_name ON roles(owner_tenant_id,name_normalized) WHERE owner_tenant_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX roles_creation_request ON roles(owner_tenant_id,created_by,creation_request_id) WHERE owner_tenant_id IS NOT NULL;
CREATE INDEX roles_tenant_page ON roles(owner_tenant_id,status,id) WHERE deleted_at IS NULL;
CREATE INDEX tenant_role_members ON tenant_member_roles(tenant_id,role_id,membership_id) WHERE revoked_at IS NULL;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tenant_structure_valid(tid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS(SELECT 1 FROM tenants t WHERE t.id=tid AND (
  NOT EXISTS(SELECT 1 FROM tenant_memberships m JOIN tenant_member_roles mr ON mr.membership_id=m.id AND mr.tenant_id=t.id JOIN roles r ON r.id=mr.role_id
    WHERE m.id=t.administrator_membership_id AND m.tenant_id=t.id AND m.status='active' AND mr.revoked_at IS NULL AND r.builtin_key='tenant_admin' AND r.status='active' AND r.deleted_at IS NULL)
  OR (SELECT count(*) FROM tenant_member_roles WHERE membership_id=t.administrator_membership_id AND revoked_at IS NULL)<>1
  OR EXISTS(SELECT 1 FROM tenant_memberships m JOIN tenant_member_roles mr ON mr.membership_id=m.id AND mr.tenant_id=t.id JOIN roles r ON r.id=mr.role_id
    WHERE m.tenant_id=t.id AND mr.revoked_at IS NULL AND (m.status='ended' OR r.deleted_at IS NOT NULL OR r.binding_scope<>'tenant'
     OR (r.owner_tenant_id IS NOT NULL AND r.owner_tenant_id<>t.id)
     OR (r.builtin_key='tenant_admin' AND m.id<>t.administrator_membership_id)))))
$$;
CREATE FUNCTION tenant_roles_valid() RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS(SELECT 1 FROM roles r JOIN roles b ON b.name_normalized=r.name_normalized AND b.binding_scope='tenant' AND b.owner_tenant_id IS NULL AND b.deleted_at IS NULL
   WHERE r.owner_tenant_id IS NOT NULL AND r.deleted_at IS NULL)
 AND NOT EXISTS(SELECT 1 FROM roles r JOIN role_permissions rp ON rp.role_id=r.id JOIN permission_catalog p ON p.code=rp.permission_code
   WHERE r.binding_scope='tenant' AND COALESCE(r.builtin_key,'')<>'tenant_admin' AND (p.scope<>'tenant' OR (p.lifecycle='active' AND NOT p.tenant_custom_role_assignable)))
 AND NOT EXISTS(SELECT 1 FROM tenants WHERE NOT tenant_structure_valid(id))
$$;
CREATE FUNCTION check_tenant_role_structure() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT tenant_roles_valid() THEN RAISE EXCEPTION 'tenant role structure invalid' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE FUNCTION check_new_tenant_permission() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.binding_scope='tenant' AND EXISTS(SELECT 1 FROM roles WHERE id=NEW.role_id AND owner_tenant_id IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM permission_catalog WHERE code=NEW.permission_code AND scope='tenant' AND lifecycle='active' AND tenant_custom_role_assignable)
 THEN RAISE EXCEPTION 'permission cannot be newly assigned to a tenant role' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION check_new_tenant_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.revoked_at IS NULL AND (TG_OP='INSERT' OR OLD.revoked_at IS NOT NULL) AND NOT EXISTS(SELECT 1 FROM roles WHERE id=NEW.role_id AND status='active' AND deleted_at IS NULL)
 THEN RAISE EXCEPTION 'cannot add a disabled or deleted tenant role' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER tenant_role_structure AFTER INSERT OR UPDATE OR DELETE ON roles DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_tenant_role_structure();
CREATE CONSTRAINT TRIGGER tenant_permission_structure AFTER INSERT OR UPDATE OR DELETE ON role_permissions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_tenant_role_structure();
CREATE CONSTRAINT TRIGGER tenant_catalog_structure AFTER INSERT OR UPDATE OR DELETE ON permission_catalog DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_tenant_role_structure();
CREATE TRIGGER tenant_new_permission BEFORE INSERT OR UPDATE ON role_permissions FOR EACH ROW EXECUTE FUNCTION check_new_tenant_permission();
CREATE TRIGGER tenant_new_binding BEFORE INSERT OR UPDATE ON tenant_member_roles FOR EACH ROW EXECUTE FUNCTION check_new_tenant_binding();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Role and grant history is irreversible; restore a verified backup instead'; END $$;
-- +goose StatementEnd
