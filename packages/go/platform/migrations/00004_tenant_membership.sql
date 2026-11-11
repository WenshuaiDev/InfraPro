-- +goose Up
ALTER TABLE tenant_memberships DROP CONSTRAINT tenant_memberships_status_check;
ALTER TABLE tenant_memberships DROP CONSTRAINT tenant_memberships_check;
ALTER TABLE tenant_memberships ADD CONSTRAINT tenant_membership_status CHECK(status IN ('active','suspended','ended'));
ALTER TABLE tenant_memberships ADD CONSTRAINT tenant_membership_end CHECK(
 (status IN ('active','suspended') AND ended_at IS NULL AND ended_by IS NULL AND end_reason IS NULL) OR
 (status='ended' AND ended_at IS NOT NULL AND ended_by IS NOT NULL AND end_reason IS NOT NULL AND char_length(end_reason) BETWEEN 1 AND 500));
ALTER TABLE tenant_memberships ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 ADD COLUMN status_changed_at timestamptz, ADD COLUMN status_changed_by uuid REFERENCES accounts(id),
 ADD COLUMN status_reason text NOT NULL DEFAULT '', ADD COLUMN source_invitation_id uuid,
 ADD CONSTRAINT tenant_membership_account UNIQUE(tenant_id,account_id,id);
UPDATE tenant_memberships SET status_changed_at=COALESCE(ended_at,joined_at),status_changed_by=COALESCE(ended_by,joined_by),status_reason=COALESCE(end_reason,'');
-- Validate queued legacy constraint triggers before further ALTER TABLE statements.
SET CONSTRAINTS ALL IMMEDIATE;
SET CONSTRAINTS ALL DEFERRED;
ALTER TABLE tenant_memberships ALTER COLUMN status_changed_at SET NOT NULL, ALTER COLUMN status_changed_at SET DEFAULT now(), ALTER COLUMN status_changed_by SET NOT NULL;
DROP INDEX tenant_one_active_member;
CREATE UNIQUE INDEX tenant_one_current_member ON tenant_memberships(tenant_id,account_id) WHERE status IN ('active','suspended');
CREATE INDEX tenant_membership_page ON tenant_memberships(tenant_id,joined_at DESC,id DESC);
CREATE TABLE tenant_invitations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
 target_account_id uuid NOT NULL REFERENCES accounts(id), inviter_account_id uuid NOT NULL REFERENCES accounts(id), inviter_membership_id uuid NOT NULL,
 role_key text NOT NULL DEFAULT 'tenant_member' CHECK(role_key='tenant_member'),
 token_hash text NOT NULL UNIQUE CHECK(token_hash ~ '^[a-f0-9]{64}$'),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL CHECK(expires_at>created_at),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','accepted','expired','revoked','replaced','invalidated')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), closed_at timestamptz, closed_by uuid REFERENCES accounts(id), close_reason text,
 accepted_at timestamptz, accepted_by uuid REFERENCES accounts(id), accepted_membership_id uuid, accepted_context_snapshot jsonb,
 replaces_id uuid, replaced_by_id uuid,
 creation_request_id uuid NOT NULL, creation_payload_hash text NOT NULL, creation_operation text NOT NULL CHECK(creation_operation IN ('create','regenerate')),
 UNIQUE(tenant_id,target_account_id,id), UNIQUE(tenant_id,inviter_account_id,creation_operation,creation_request_id), UNIQUE(replaces_id), UNIQUE(replaced_by_id),
 FOREIGN KEY(tenant_id,inviter_account_id,inviter_membership_id) REFERENCES tenant_memberships(tenant_id,account_id,id),
 FOREIGN KEY(tenant_id,target_account_id,accepted_membership_id) REFERENCES tenant_memberships(tenant_id,account_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(tenant_id,target_account_id,replaces_id) REFERENCES tenant_invitations(tenant_id,target_account_id,id) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(tenant_id,target_account_id,replaced_by_id) REFERENCES tenant_invitations(tenant_id,target_account_id,id) DEFERRABLE INITIALLY DEFERRED,
 CHECK((status='pending' AND closed_at IS NULL AND closed_by IS NULL AND close_reason IS NULL) OR (status<>'pending' AND closed_at IS NOT NULL AND close_reason IS NOT NULL AND char_length(close_reason) BETWEEN 1 AND 500)),
 CHECK((status='accepted' AND accepted_at IS NOT NULL AND accepted_by IS NOT NULL AND accepted_by=target_account_id AND accepted_membership_id IS NOT NULL AND accepted_context_snapshot IS NOT NULL) OR
 (status<>'accepted' AND accepted_at IS NULL AND accepted_by IS NULL AND accepted_membership_id IS NULL AND accepted_context_snapshot IS NULL))
);
ALTER TABLE tenant_memberships ADD CONSTRAINT tenant_member_invitation FOREIGN KEY(tenant_id,account_id,source_invitation_id) REFERENCES tenant_invitations(tenant_id,target_account_id,id) DEFERRABLE INITIALLY DEFERRED;
CREATE UNIQUE INDEX tenant_invitation_pending ON tenant_invitations(tenant_id,target_account_id) WHERE status='pending';
CREATE INDEX tenant_invitation_page ON tenant_invitations(tenant_id,created_at DESC,id DESC);
CREATE INDEX tenant_invitation_inviter ON tenant_invitations(inviter_membership_id) WHERE status='pending';
CREATE INDEX tenant_invitation_target ON tenant_invitations(target_account_id) WHERE status='pending';
-- One current role per current relation; any number of ordinary members, exactly one administrator.
-- +goose StatementBegin
CREATE FUNCTION tenant_structure_valid(tid uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS(SELECT 1 FROM tenants t WHERE t.id=tid AND (
  NOT EXISTS(SELECT 1 FROM tenant_memberships m JOIN tenant_member_roles mr ON mr.membership_id=m.id AND mr.tenant_id=t.id JOIN roles r ON r.id=mr.role_id
   WHERE m.id=t.administrator_membership_id AND m.tenant_id=t.id AND m.status='active' AND mr.revoked_at IS NULL AND r.builtin_key='tenant_admin' AND r.status='active' AND r.deleted_at IS NULL)
  OR EXISTS(SELECT 1 FROM tenant_memberships m WHERE m.tenant_id=t.id AND (
   (m.status='ended' AND EXISTS(SELECT 1 FROM tenant_member_roles mr WHERE mr.membership_id=m.id AND mr.revoked_at IS NULL)) OR
   (m.status<>'ended' AND ((SELECT count(*) FROM tenant_member_roles mr WHERE mr.membership_id=m.id AND mr.revoked_at IS NULL)<>1 OR
    NOT EXISTS(SELECT 1 FROM tenant_member_roles mr JOIN roles r ON r.id=mr.role_id WHERE mr.membership_id=m.id AND mr.revoked_at IS NULL AND r.status='active' AND r.deleted_at IS NULL
      AND r.builtin_key=CASE WHEN m.id=t.administrator_membership_id THEN 'tenant_admin' ELSE 'tenant_member' END)))))))
$$;
CREATE OR REPLACE FUNCTION check_tenant_administrator() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE tid uuid;
BEGIN
 IF TG_TABLE_NAME='tenants' THEN tid=COALESCE(NEW.id,OLD.id); ELSE tid=COALESCE(NEW.tenant_id,OLD.tenant_id); END IF;
 IF NOT tenant_structure_valid(tid) THEN RAISE EXCEPTION 'tenant membership structure invalid: %',tid USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
CREATE FUNCTION check_tenant_invitation_result() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE tid uuid;
BEGIN
 tid=COALESCE(NEW.tenant_id,OLD.tenant_id);
 IF EXISTS(SELECT 1 FROM tenant_invitations i WHERE i.tenant_id=tid AND i.status='accepted' AND NOT EXISTS(
  SELECT 1 FROM tenant_memberships m WHERE m.id=i.accepted_membership_id AND m.tenant_id=i.tenant_id AND m.account_id=i.target_account_id AND m.source_invitation_id=i.id))
 OR EXISTS(SELECT 1 FROM tenant_memberships m WHERE m.tenant_id=tid AND m.source_invitation_id IS NOT NULL AND NOT EXISTS(
  SELECT 1 FROM tenant_invitations i WHERE i.id=m.source_invitation_id AND i.status='accepted' AND i.accepted_membership_id=m.id))
 THEN RAISE EXCEPTION 'tenant invitation result invalid' USING ERRCODE='23514'; END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER tenant_invitation_result AFTER INSERT OR UPDATE OR DELETE ON tenant_invitations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_tenant_invitation_result();
CREATE CONSTRAINT TRIGGER tenant_membership_source AFTER INSERT OR UPDATE OR DELETE ON tenant_memberships DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_tenant_invitation_result();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Membership history is irreversible; restore a verified backup instead'; END $$;
-- +goose StatementEnd
