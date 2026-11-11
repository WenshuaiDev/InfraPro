-- +goose Up
ALTER TABLE role_permissions DROP CONSTRAINT permission_role_scope;
ALTER TABLE permission_catalog DROP CONSTRAINT permission_binding_key;
ALTER TABLE permission_catalog DROP COLUMN binding_scope;
ALTER TABLE permission_catalog DROP CONSTRAINT permission_scope;
ALTER TABLE permission_catalog ADD CONSTRAINT permission_scope CHECK(scope IN ('self','platform','tenant','project'));
ALTER TABLE permission_catalog ADD COLUMN binding_scope text GENERATED ALWAYS AS (CASE WHEN scope IN ('self','platform') THEN 'platform' WHEN scope='tenant' THEN 'tenant' WHEN scope='project' THEN 'project' END) STORED;
ALTER TABLE permission_catalog ADD CONSTRAINT permission_binding_key UNIQUE(code,binding_scope);
ALTER TABLE role_permissions ADD CONSTRAINT permission_role_scope FOREIGN KEY(permission_code,binding_scope) REFERENCES permission_catalog(code,binding_scope);
ALTER TABLE permission_catalog ADD COLUMN project_custom_role_assignable boolean NOT NULL DEFAULT false CHECK(NOT project_custom_role_assignable OR scope='project');
ALTER TABLE roles DROP CONSTRAINT roles_binding_scope_check;
ALTER TABLE roles ADD CONSTRAINT roles_binding_scope_check CHECK(binding_scope IN ('platform','tenant','project'));
ALTER TABLE roles DROP CONSTRAINT role_owner_scope;
ALTER TABLE roles DROP CONSTRAINT role_assignment_policy;
ALTER TABLE roles ADD COLUMN owner_project_id uuid;
ALTER TABLE roles ADD CONSTRAINT role_owner_scope CHECK(
 (binding_scope='platform' AND owner_tenant_id IS NULL AND owner_project_id IS NULL) OR
 (binding_scope='tenant' AND owner_project_id IS NULL AND ((builtin_key IS NOT NULL AND owner_tenant_id IS NULL) OR (builtin_key IS NULL AND owner_tenant_id IS NOT NULL))) OR
 (binding_scope='project' AND ((builtin_key IS NOT NULL AND owner_tenant_id IS NULL AND owner_project_id IS NULL) OR (builtin_key IS NULL AND owner_tenant_id IS NOT NULL AND owner_project_id IS NOT NULL))));
ALTER TABLE roles ADD CONSTRAINT role_assignment_policy CHECK((assignment_policy='administrator_transfer_only')=(COALESCE(builtin_key,'') IN ('tenant_admin','project_admin')));
DROP INDEX roles_tenant_name;
DROP INDEX roles_creation_request;
CREATE UNIQUE INDEX roles_tenant_name ON roles(owner_tenant_id,name_normalized) WHERE binding_scope='tenant' AND owner_tenant_id IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX roles_creation_request ON roles(owner_tenant_id,created_by,creation_request_id) WHERE binding_scope='tenant' AND owner_tenant_id IS NOT NULL;
CREATE UNIQUE INDEX roles_project_name ON roles(owner_project_id,name_normalized) WHERE binding_scope='project' AND deleted_at IS NULL;
CREATE UNIQUE INDEX roles_project_preset_name ON roles(name_normalized) WHERE binding_scope='project' AND builtin_key IS NOT NULL AND deleted_at IS NULL;
CREATE TABLE projects (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 100 AND name=btrim(name)),
 code text NOT NULL CHECK(code ~ '^[a-z0-9][a-z0-9_-]{2,31}$'),
 purpose text NOT NULL CHECK(char_length(purpose) BETWEEN 1 AND 1000 AND purpose=btrim(purpose)),
 description text NOT NULL DEFAULT '' CHECK(char_length(description)<=2000),
 owner_tenant_membership_id uuid NOT NULL, administrator_membership_id uuid,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
 planned_start date, planned_end date, CHECK(planned_start IS NULL OR planned_end IS NULL OR planned_start<=planned_end),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 archived_at timestamptz, archived_by uuid REFERENCES accounts(id), archive_reason text,
 management_gap_reason text, management_gap_at timestamptz, management_gap_by uuid REFERENCES accounts(id), previous_administrator_membership_id uuid,
 UNIQUE(tenant_id,id),
 FOREIGN KEY(tenant_id,owner_tenant_membership_id) REFERENCES tenant_memberships(tenant_id,id),
 CHECK((status='active' AND archived_at IS NULL AND archived_by IS NULL AND archive_reason IS NULL) OR (status='archived' AND archived_at IS NOT NULL AND archived_by IS NOT NULL AND archive_reason IS NOT NULL AND char_length(archive_reason) BETWEEN 1 AND 500)),
 CHECK((administrator_membership_id IS NOT NULL AND management_gap_reason IS NULL AND management_gap_at IS NULL AND management_gap_by IS NULL AND previous_administrator_membership_id IS NULL) OR
 (administrator_membership_id IS NULL AND management_gap_reason IS NOT NULL AND char_length(management_gap_reason) BETWEEN 1 AND 500 AND management_gap_at IS NOT NULL AND management_gap_by IS NOT NULL AND previous_administrator_membership_id IS NOT NULL))
);
CREATE TABLE project_codes (
 tenant_id uuid NOT NULL REFERENCES tenants(id), code text NOT NULL CHECK(code ~ '^[a-z0-9][a-z0-9_-]{2,31}$'), project_id uuid NOT NULL,
 first_used_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(tenant_id,code), UNIQUE(tenant_id,code,project_id),
 FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
ALTER TABLE projects ADD CONSTRAINT project_current_code FOREIGN KEY(tenant_id,code,id) REFERENCES project_codes(tenant_id,code,project_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE project_memberships (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, tenant_membership_id uuid NOT NULL, account_id uuid NOT NULL,
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','suspended','ended')), version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 joined_at timestamptz NOT NULL DEFAULT now(), joined_by uuid NOT NULL REFERENCES accounts(id),
 status_changed_at timestamptz NOT NULL DEFAULT now(), status_changed_by uuid NOT NULL REFERENCES accounts(id), status_reason text NOT NULL DEFAULT '' CHECK(char_length(status_reason)<=500),
 ended_at timestamptz, ended_by uuid REFERENCES accounts(id), end_reason text,
 UNIQUE(tenant_id,project_id,id), FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id),
 FOREIGN KEY(tenant_id,account_id,tenant_membership_id) REFERENCES tenant_memberships(tenant_id,account_id,id),
 CHECK((status<>'ended' AND ended_at IS NULL AND ended_by IS NULL AND end_reason IS NULL) OR (status='ended' AND ended_at IS NOT NULL AND ended_by IS NOT NULL AND end_reason IS NOT NULL AND char_length(end_reason) BETWEEN 1 AND 500))
);
CREATE UNIQUE INDEX project_one_current_member ON project_memberships(project_id,account_id) WHERE status<>'ended';
CREATE INDEX project_member_parent ON project_memberships(tenant_membership_id);
CREATE INDEX project_member_account ON project_memberships(account_id,project_id);
CREATE INDEX project_member_page ON project_memberships(project_id,joined_at DESC,id DESC);
CREATE INDEX project_page ON projects(tenant_id,status,created_at DESC,id DESC);
ALTER TABLE projects ADD CONSTRAINT project_current_admin FOREIGN KEY(tenant_id,id,administrator_membership_id) REFERENCES project_memberships(tenant_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE projects ADD CONSTRAINT project_previous_admin FOREIGN KEY(tenant_id,id,previous_administrator_membership_id) REFERENCES project_memberships(tenant_id,project_id,id) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE roles ADD CONSTRAINT project_role_owner FOREIGN KEY(owner_tenant_id,owner_project_id) REFERENCES projects(tenant_id,id);
CREATE TABLE project_member_roles (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, project_id uuid NOT NULL, membership_id uuid NOT NULL, role_id uuid NOT NULL,
 binding_scope text NOT NULL DEFAULT 'project' CHECK(binding_scope='project'), assigned_at timestamptz NOT NULL DEFAULT now(), assigned_by uuid NOT NULL REFERENCES accounts(id), revoked_at timestamptz, revoked_by uuid REFERENCES accounts(id),
 FOREIGN KEY(tenant_id,project_id,membership_id) REFERENCES project_memberships(tenant_id,project_id,id),
 FOREIGN KEY(role_id,binding_scope) REFERENCES roles(id,binding_scope), CHECK((revoked_at IS NULL)=(revoked_by IS NULL))
);
CREATE UNIQUE INDEX project_active_role ON project_member_roles(membership_id,role_id) WHERE revoked_at IS NULL;
CREATE INDEX project_role_members ON project_member_roles(project_id,role_id,membership_id) WHERE revoked_at IS NULL;
CREATE TABLE project_operation_receipts (
 tenant_id uuid NOT NULL REFERENCES tenants(id), actor_id uuid NOT NULL REFERENCES accounts(id), operation text NOT NULL, client_request_id uuid NOT NULL,
 payload_hash text NOT NULL, project_id uuid NOT NULL, object_id uuid NOT NULL, version bigint NOT NULL, completed_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,actor_id,operation,client_request_id), FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
ALTER TABLE account_security_events ADD COLUMN project_id uuid;
ALTER TABLE account_security_events ADD CONSTRAINT event_project_scope CHECK(project_id IS NULL OR tenant_id IS NOT NULL);
ALTER TABLE account_security_events ADD CONSTRAINT event_project FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id);
ALTER TABLE account_security_events DROP CONSTRAINT account_security_events_actor_scope_check;
ALTER TABLE account_security_events ADD CONSTRAINT account_security_events_actor_scope_check CHECK(actor_scope IN ('platform','tenant','project'));
CREATE INDEX project_audit_page ON account_security_events(project_id,id DESC) WHERE project_id IS NOT NULL;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tenant_roles_valid() RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS(SELECT 1 FROM roles r JOIN roles b ON b.name_normalized=r.name_normalized AND b.binding_scope='tenant' AND b.owner_tenant_id IS NULL AND b.deleted_at IS NULL
   WHERE r.binding_scope='tenant' AND r.owner_tenant_id IS NOT NULL AND r.deleted_at IS NULL)
 AND NOT EXISTS(SELECT 1 FROM roles r JOIN role_permissions rp ON rp.role_id=r.id JOIN permission_catalog p ON p.code=rp.permission_code
   WHERE r.binding_scope='tenant' AND COALESCE(r.builtin_key,'')<>'tenant_admin' AND (p.scope<>'tenant' OR (p.lifecycle='active' AND NOT p.tenant_custom_role_assignable)))
 AND NOT EXISTS(SELECT 1 FROM tenants WHERE NOT tenant_structure_valid(id))
$$;
CREATE FUNCTION project_structure_valid(pid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS(SELECT 1 FROM projects p WHERE p.id=pid AND (
 (p.administrator_membership_id IS NOT NULL AND (
  NOT EXISTS(SELECT 1 FROM project_memberships m JOIN tenant_memberships tm ON tm.id=m.tenant_membership_id JOIN project_member_roles mr ON mr.membership_id=m.id JOIN roles r ON r.id=mr.role_id
   WHERE m.id=p.administrator_membership_id AND m.project_id=p.id AND m.status='active' AND tm.status='active' AND mr.revoked_at IS NULL AND r.builtin_key='project_admin' AND r.status='active' AND r.deleted_at IS NULL)
  OR (SELECT count(*) FROM project_member_roles WHERE membership_id=p.administrator_membership_id AND revoked_at IS NULL)<>1))
 OR (p.administrator_membership_id IS NULL AND NOT EXISTS(SELECT 1 FROM project_memberships oldm JOIN tenant_memberships oldtm ON oldtm.id=oldm.tenant_membership_id WHERE oldm.id=p.previous_administrator_membership_id AND oldm.project_id=p.id AND oldm.status='ended' AND oldtm.status='ended'))
 OR EXISTS(SELECT 1 FROM project_memberships m JOIN tenant_memberships tm ON tm.id=m.tenant_membership_id WHERE m.project_id=p.id AND tm.status='ended' AND m.status<>'ended')
 OR EXISTS(SELECT 1 FROM project_member_roles mr JOIN project_memberships m ON m.id=mr.membership_id JOIN roles r ON r.id=mr.role_id WHERE mr.project_id=p.id AND mr.revoked_at IS NULL AND (
   m.status='ended' OR r.deleted_at IS NOT NULL OR r.binding_scope<>'project' OR (r.owner_project_id IS NOT NULL AND r.owner_project_id<>p.id)
   OR (r.builtin_key='project_admin' AND (p.administrator_membership_id IS NULL OR m.id<>p.administrator_membership_id))))))
$$;
CREATE FUNCTION project_roles_valid() RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS(SELECT 1 FROM projects WHERE NOT project_structure_valid(id))
 AND NOT EXISTS(SELECT 1 FROM roles r JOIN roles b ON b.name_normalized=r.name_normalized AND b.binding_scope='project' AND b.builtin_key IS NOT NULL AND b.deleted_at IS NULL WHERE r.binding_scope='project' AND r.owner_project_id IS NOT NULL AND r.deleted_at IS NULL)
 AND NOT EXISTS(SELECT 1 FROM roles r JOIN role_permissions rp ON rp.role_id=r.id JOIN permission_catalog pc ON pc.code=rp.permission_code WHERE r.binding_scope='project' AND COALESCE(r.builtin_key,'')<>'project_admin' AND (pc.scope<>'project' OR (pc.lifecycle='active' AND NOT pc.project_custom_role_assignable)))
$$;
CREATE FUNCTION check_project_structure() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE pid uuid;
BEGIN
 IF TG_TABLE_NAME='projects' THEN pid=COALESCE(NEW.id,OLD.id);
 ELSIF TG_TABLE_NAME IN ('project_memberships','project_member_roles') THEN pid=COALESCE(NEW.project_id,OLD.project_id);
 ELSIF TG_TABLE_NAME='tenant_memberships' THEN
  IF EXISTS(SELECT 1 FROM project_memberships WHERE tenant_membership_id=COALESCE(NEW.id,OLD.id) AND NOT project_structure_valid(project_id)) THEN RAISE EXCEPTION 'project parent invalid' USING ERRCODE='23514'; END IF;
  RETURN NULL;
 ELSE
  IF NOT project_roles_valid() THEN RAISE EXCEPTION 'project roles invalid' USING ERRCODE='23514'; END IF;
  RETURN NULL;
 END IF;
 IF NOT project_structure_valid(pid) THEN RAISE EXCEPTION 'project structure invalid' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE FUNCTION check_new_project_permission() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.binding_scope='project' AND EXISTS(SELECT 1 FROM roles WHERE id=NEW.role_id AND builtin_key IS DISTINCT FROM 'project_admin')
 AND NOT EXISTS(SELECT 1 FROM permission_catalog WHERE code=NEW.permission_code AND scope='project' AND lifecycle='active' AND project_custom_role_assignable)
 THEN RAISE EXCEPTION 'permission cannot be assigned to project role' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION check_new_project_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.revoked_at IS NULL AND (TG_OP='INSERT' OR OLD.revoked_at IS NOT NULL OR NEW.role_id<>OLD.role_id OR NEW.membership_id<>OLD.membership_id)
 AND NOT EXISTS(SELECT 1 FROM roles WHERE id=NEW.role_id AND status='active' AND deleted_at IS NULL)
 THEN RAISE EXCEPTION 'cannot add disabled project role' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION project_gap_transition() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.administrator_membership_id IS NULL AND (TG_OP='INSERT' OR OLD.administrator_membership_id IS NOT NULL) THEN
  IF TG_OP='INSERT' OR NEW.previous_administrator_membership_id IS DISTINCT FROM OLD.administrator_membership_id OR NOT EXISTS(
   SELECT 1 FROM project_memberships m JOIN tenant_memberships tm ON tm.id=m.tenant_membership_id JOIN tenants t ON t.id=m.tenant_id JOIN accounts a ON a.id=m.account_id
   WHERE m.id=OLD.administrator_membership_id AND t.administrator_membership_id=tm.id AND a.status='disabled') THEN
   RAISE EXCEPTION 'project gap requires disabled tenant administrator repair' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION project_member_identity_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.id,NEW.tenant_id,NEW.project_id,NEW.tenant_membership_id,NEW.account_id) IS DISTINCT FROM (OLD.id,OLD.tenant_id,OLD.project_id,OLD.tenant_membership_id,OLD.account_id) THEN
  RAISE EXCEPTION 'project membership identity immutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION project_code_permanent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'historical project codes are permanent' USING ERRCODE='23514';
END $$;
CREATE FUNCTION project_identity_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.tenant_id<>OLD.tenant_id OR NEW.id<>OLD.id THEN RAISE EXCEPTION 'project identity immutable' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER project_gap BEFORE INSERT OR UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION project_gap_transition();
CREATE TRIGGER project_member_identity BEFORE UPDATE ON project_memberships FOR EACH ROW EXECUTE FUNCTION project_member_identity_immutable();
CREATE TRIGGER project_code_history BEFORE UPDATE OR DELETE ON project_codes FOR EACH ROW EXECUTE FUNCTION project_code_permanent();
CREATE TRIGGER project_identity BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION project_identity_immutable();
CREATE TRIGGER project_new_permission BEFORE INSERT OR UPDATE ON role_permissions FOR EACH ROW EXECUTE FUNCTION check_new_project_permission();
CREATE TRIGGER project_new_binding BEFORE INSERT OR UPDATE ON project_member_roles FOR EACH ROW EXECUTE FUNCTION check_new_project_binding();
CREATE CONSTRAINT TRIGGER project_structure AFTER INSERT OR UPDATE ON projects DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_project_structure();
CREATE CONSTRAINT TRIGGER project_member_structure AFTER INSERT OR UPDATE OR DELETE ON project_memberships DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_project_structure();
CREATE CONSTRAINT TRIGGER project_binding_structure AFTER INSERT OR UPDATE OR DELETE ON project_member_roles DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_project_structure();
CREATE CONSTRAINT TRIGGER project_parent_structure AFTER INSERT OR UPDATE OR DELETE ON tenant_memberships DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_project_structure();
CREATE CONSTRAINT TRIGGER project_role_structure AFTER INSERT OR UPDATE OR DELETE ON roles DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_project_structure();
CREATE CONSTRAINT TRIGGER project_permission_structure AFTER INSERT OR UPDATE OR DELETE ON role_permissions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_project_structure();
CREATE CONSTRAINT TRIGGER project_catalog_structure AFTER INSERT OR UPDATE OR DELETE ON permission_catalog DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_project_structure();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Project history is irreversible; restore a verified backup instead'; END $$;
-- +goose StatementEnd
