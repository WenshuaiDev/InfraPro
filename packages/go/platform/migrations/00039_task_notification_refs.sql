-- +goose Up
ALTER TABLE notification_events DROP CONSTRAINT notification_events_schema_version_check;
ALTER TABLE notification_events ADD COLUMN task_reference jsonb;
ALTER TABLE notification_events ADD CHECK(schema_version IN (1,2));
ALTER TABLE notification_events ADD CHECK((schema_version=1 AND task_reference IS NULL) OR (schema_version=2 AND jsonb_typeof(task_reference)='object' AND task_reference ?& ARRAY['source_module','source_kind','source_root_id','generation'] AND task_reference->>'generation' ~ '^[1-9][0-9]*$'));
ALTER TABLE display_notification_handoffs ADD COLUMN task_reference jsonb;
UPDATE notification_protocol SET version=2 WHERE id=1;
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'notification history cannot be downgraded'; END $$;
-- +goose StatementEnd
