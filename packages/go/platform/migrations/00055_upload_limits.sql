-- +goose Up
-- File admission limits belong to deployment policy. Preserve size/part validity
-- and existing uploads; changing the policy must not invalidate stored evidence.
ALTER TABLE file_uploads DROP CONSTRAINT file_uploads_expected_size_check;
ALTER TABLE file_uploads ADD CONSTRAINT file_uploads_expected_size_check CHECK(expected_size BETWEEN 1 AND 9007199254740991);
ALTER TABLE file_records DROP CONSTRAINT file_records_size_bytes_check;
ALTER TABLE file_records ADD CONSTRAINT file_records_size_bytes_check CHECK(size_bytes BETWEEN 1 AND 9007199254740991);
ALTER TABLE spatial_uploads DROP CONSTRAINT spatial_uploads_size_check;
ALTER TABLE spatial_uploads ADD CONSTRAINT spatial_uploads_size_check CHECK(size BETWEEN 1 AND 9007199254740991);
ALTER TABLE spatial_upload_parts DROP CONSTRAINT spatial_upload_parts_part_check;
ALTER TABLE spatial_upload_parts ADD CONSTRAINT spatial_upload_parts_part_check CHECK(part >= 1);
ALTER TABLE display_uploads DROP CONSTRAINT display_uploads_size_bytes_check;
ALTER TABLE display_uploads ADD CONSTRAINT display_uploads_size_bytes_check CHECK(size_bytes BETWEEN 1 AND 9007199254740991);
ALTER TABLE display_upload_parts DROP CONSTRAINT display_upload_parts_part_check;
ALTER TABLE display_upload_parts ADD CONSTRAINT display_upload_parts_part_check CHECK(part >= 0);

-- +goose Down
-- Deliberately fails if larger accepted evidence exists; never delete it to roll back.
ALTER TABLE file_uploads DROP CONSTRAINT file_uploads_expected_size_check;
ALTER TABLE file_uploads ADD CONSTRAINT file_uploads_expected_size_check CHECK(expected_size BETWEEN 1 AND 104857600);
ALTER TABLE file_records DROP CONSTRAINT file_records_size_bytes_check;
ALTER TABLE file_records ADD CONSTRAINT file_records_size_bytes_check CHECK(size_bytes BETWEEN 1 AND 104857600);
ALTER TABLE spatial_uploads DROP CONSTRAINT spatial_uploads_size_check;
ALTER TABLE spatial_uploads ADD CONSTRAINT spatial_uploads_size_check CHECK(size BETWEEN 1 AND 268435456);
ALTER TABLE spatial_upload_parts DROP CONSTRAINT spatial_upload_parts_part_check;
ALTER TABLE spatial_upload_parts ADD CONSTRAINT spatial_upload_parts_part_check CHECK(part BETWEEN 1 AND 32);
ALTER TABLE display_uploads DROP CONSTRAINT display_uploads_size_bytes_check;
ALTER TABLE display_uploads ADD CONSTRAINT display_uploads_size_bytes_check CHECK(size_bytes BETWEEN 1 AND 134217728);
ALTER TABLE display_upload_parts DROP CONSTRAINT display_upload_parts_part_check;
ALTER TABLE display_upload_parts ADD CONSTRAINT display_upload_parts_part_check CHECK(part BETWEEN 0 AND 31);
