-- +goose Up
ALTER TABLE roles ADD COLUMN binding_scope text NOT NULL DEFAULT 'platform' CHECK(binding_scope IN ('platform','tenant'));
ALTER TABLE roles ADD CONSTRAINT roles_scope_key UNIQUE(id,binding_scope);
DROP INDEX roles_name;
CREATE UNIQUE INDEX roles_name ON roles(binding_scope,name_normalized) WHERE deleted_at IS NULL;
ALTER TABLE permission_catalog ADD CONSTRAINT permission_scope CHECK(scope IN ('self','platform','tenant'));
ALTER TABLE permission_catalog ADD COLUMN binding_scope text GENERATED ALWAYS AS (CASE WHEN scope='tenant' THEN 'tenant' ELSE 'platform' END) STORED;
ALTER TABLE permission_catalog ADD CONSTRAINT permission_binding_key UNIQUE(code,binding_scope);
ALTER TABLE account_roles ADD COLUMN binding_scope text NOT NULL DEFAULT 'platform' CHECK(binding_scope='platform');
ALTER TABLE account_roles ADD CONSTRAINT account_role_scope FOREIGN KEY(role_id,binding_scope) REFERENCES roles(id,binding_scope);
ALTER TABLE role_permissions ADD COLUMN binding_scope text NOT NULL DEFAULT 'platform';
ALTER TABLE role_permissions ADD CONSTRAINT role_permission_scope FOREIGN KEY(role_id,binding_scope) REFERENCES roles(id,binding_scope);
ALTER TABLE role_permissions ADD CONSTRAINT permission_role_scope FOREIGN KEY(permission_code,binding_scope) REFERENCES permission_catalog(code,binding_scope);

CREATE TABLE tenants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 name text NOT NULL CHECK(char_length(name) BETWEEN 1 AND 100 AND name=btrim(name)),
 code text NOT NULL UNIQUE CHECK(code ~ '^[a-z0-9][a-z0-9_-]{2,31}$'),
 description text NOT NULL DEFAULT '' CHECK(char_length(description)<=1000),
 administrator_membership_id uuid NOT NULL,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_by uuid NOT NULL REFERENCES accounts(id), updated_by uuid NOT NULL REFERENCES accounts(id),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 creation_request_id uuid NOT NULL, creation_payload_hash text NOT NULL,
 UNIQUE(created_by,creation_request_id)
);
CREATE TABLE tenant_codes (
 code text PRIMARY KEY CHECK(code ~ '^[a-z0-9][a-z0-9_-]{2,31}$'),
 tenant_id uuid NOT NULL REFERENCES tenants(id), first_used_at timestamptz NOT NULL DEFAULT now(), UNIQUE(code,tenant_id)
);
ALTER TABLE tenants ADD CONSTRAINT tenant_current_code FOREIGN KEY(code,id) REFERENCES tenant_codes(code,tenant_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE tenant_memberships (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id), account_id uuid NOT NULL REFERENCES accounts(id),
 status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','ended')),
 joined_at timestamptz NOT NULL DEFAULT now(), joined_by uuid NOT NULL REFERENCES accounts(id),
 ended_at timestamptz, ended_by uuid REFERENCES accounts(id), end_reason text,
 UNIQUE(tenant_id,id),
 CHECK((status='active' AND ended_at IS NULL AND ended_by IS NULL AND end_reason IS NULL) OR (status='ended' AND ended_at IS NOT NULL AND ended_by IS NOT NULL AND char_length(end_reason) BETWEEN 1 AND 500))
);
CREATE UNIQUE INDEX tenant_one_active_member ON tenant_memberships(tenant_id,account_id) WHERE status='active';
CREATE INDEX tenant_member_account ON tenant_memberships(account_id,tenant_id) WHERE status='active';
ALTER TABLE tenants ADD CONSTRAINT tenant_current_admin FOREIGN KEY(id,administrator_membership_id) REFERENCES tenant_memberships(tenant_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE tenant_member_roles (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, membership_id uuid NOT NULL,
 role_id uuid NOT NULL, binding_scope text NOT NULL DEFAULT 'tenant' CHECK(binding_scope='tenant'),
 assigned_at timestamptz NOT NULL DEFAULT now(), assigned_by uuid NOT NULL REFERENCES accounts(id),
 revoked_at timestamptz, revoked_by uuid REFERENCES accounts(id),
 FOREIGN KEY(tenant_id,membership_id) REFERENCES tenant_memberships(tenant_id,id),
 FOREIGN KEY(role_id,binding_scope) REFERENCES roles(id,binding_scope),
 CHECK((revoked_at IS NULL)=(revoked_by IS NULL))
);
CREATE UNIQUE INDEX tenant_active_role ON tenant_member_roles(membership_id,role_id) WHERE revoked_at IS NULL;
ALTER TABLE account_security_events ADD COLUMN tenant_id uuid REFERENCES tenants(id);
ALTER TABLE account_security_events ADD COLUMN actor_scope text NOT NULL DEFAULT 'platform' CHECK(actor_scope IN ('platform','tenant'));
CREATE INDEX tenant_audit_page ON account_security_events(tenant_id,id DESC) WHERE tenant_id IS NOT NULL;

-- Enforce this iteration's single administrator even for accidental direct SQL writes.
-- +goose StatementBegin
CREATE FUNCTION check_tenant_administrator() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE tid uuid; current_member uuid; active_count integer;
BEGIN
 IF TG_TABLE_NAME='tenants' THEN tid=COALESCE(NEW.id,OLD.id); ELSE tid=COALESCE(NEW.tenant_id,OLD.tenant_id); END IF;
 SELECT administrator_membership_id INTO current_member FROM tenants WHERE id=tid;
 IF NOT FOUND THEN RETURN NULL; END IF;
 SELECT count(*) INTO active_count FROM tenant_memberships WHERE tenant_id=tid AND status='active';
 IF active_count<>1 OR NOT EXISTS(SELECT 1 FROM tenant_memberships m JOIN tenant_member_roles mr ON mr.tenant_id=m.tenant_id AND mr.membership_id=m.id JOIN roles r ON r.id=mr.role_id WHERE m.tenant_id=tid AND m.id=current_member AND m.status='active' AND mr.revoked_at IS NULL AND r.builtin_key='tenant_admin' AND r.status='active' AND r.deleted_at IS NULL) THEN
  RAISE EXCEPTION 'tenant administrator structure invalid: %',tid USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER tenant_administrator_valid AFTER INSERT OR UPDATE ON tenants DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_tenant_administrator();
CREATE CONSTRAINT TRIGGER tenant_member_valid AFTER INSERT OR UPDATE OR DELETE ON tenant_memberships DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_tenant_administrator();
CREATE CONSTRAINT TRIGGER tenant_binding_valid AFTER INSERT OR UPDATE OR DELETE ON tenant_member_roles DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_tenant_administrator();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Tenant membership and code history is irreversible; restore a verified backup instead'; END $$;
-- +goose StatementEnd
