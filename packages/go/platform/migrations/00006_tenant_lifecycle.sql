-- +goose Up
ALTER TABLE tenants
 ADD COLUMN status text NOT NULL DEFAULT 'active',
 ADD COLUMN status_changed_at timestamptz,
 ADD COLUMN status_changed_by uuid REFERENCES accounts(id),
 ADD COLUMN status_change_reason text,
 ADD COLUMN status_notice text NOT NULL DEFAULT '',
 ADD CONSTRAINT tenant_status_valid CHECK (status IN ('active','disabled')),
 ADD CONSTRAINT tenant_status_metadata CHECK (
   (status_changed_at IS NULL AND status_changed_by IS NULL AND status_change_reason IS NULL)
   OR (status_changed_at IS NOT NULL AND status_changed_by IS NOT NULL AND status_change_reason IS NOT NULL
       AND char_length(btrim(status_change_reason)) BETWEEN 1 AND 500)),
 ADD CONSTRAINT tenant_status_notice CHECK (
   (status='active' AND status_notice='') OR
   (status='disabled' AND status_changed_at IS NOT NULL AND char_length(btrim(status_notice)) BETWEEN 1 AND 500));

-- +goose Down
-- State cannot safely be discarded: an old API would reopen disabled tenants.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'tenant lifecycle requires forward repair or a matching backup restore'; END $$;
-- +goose StatementEnd
