-- +goose Up
ALTER TABLE analysis_artifacts ADD COLUMN deleted_at timestamptz;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM analysis_artifacts WHERE deleted_at IS NOT NULL) THEN
        RAISE EXCEPTION 'Cannot remove tombstones for deleted analysis artifacts';
    END IF;
    ALTER TABLE analysis_artifacts DROP COLUMN deleted_at;
END $$;
-- +goose StatementEnd
