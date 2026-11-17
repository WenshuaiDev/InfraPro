-- +goose Up
-- Old content and digests remain immutable; new publications choose their format.
ALTER TABLE engineering_versions DROP CONSTRAINT engineering_versions_schema_version_check;
ALTER TABLE engineering_versions ADD CONSTRAINT engineering_versions_schema_version_check CHECK(schema_version IN ('1','2'));
ALTER TABLE engineering_versions ADD CONSTRAINT engineering_versions_content_version_check CHECK(content->>'metamodel_version'=schema_version);
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Structure metamodel publications are retained; restore a verified backup instead'; END $$;
-- +goose StatementEnd
