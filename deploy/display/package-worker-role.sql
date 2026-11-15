-- Run as the deployment DBA in the migrated database. This creates a NOLOGIN
-- role only; attach the installation's dedicated login externally. No password
-- or membership of a business role is included here. Default schema: public.
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='infrapro_display_packager') THEN
  CREATE ROLE infrapro_display_packager NOLOGIN;
 END IF;
END $$;
GRANT USAGE ON SCHEMA public TO infrapro_display_packager;
GRANT SELECT, UPDATE ON display_package_jobs, display_uploads TO infrapro_display_packager;
GRANT SELECT ON display_upload_parts TO infrapro_display_packager;
GRANT SELECT, INSERT ON display_packages, display_package_files TO infrapro_display_packager;
GRANT SELECT, INSERT, DELETE ON display_object_intents TO infrapro_display_packager;
GRANT SELECT, INSERT, DELETE ON display_events TO infrapro_display_packager;
GRANT SELECT, INSERT ON display_notification_handoffs TO infrapro_display_packager;
GRANT USAGE, SELECT ON SEQUENCE display_events_id_seq TO infrapro_display_packager;
-- Bounded display-only retention, invoked by the worker once per minute.
GRANT SELECT, UPDATE ON display_sessions, display_subscriptions TO infrapro_display_packager;
GRANT SELECT, DELETE ON display_pairings, display_tool_sessions, display_subscription_events, display_receipts TO infrapro_display_packager;
-- Do not grant accounts, sessions, resources, monitoring, device, derived or
-- spatial tables, ClickHouse credentials, general object prefixes, or schema DDL.

-- Task metadata and evidence stay in the display permission domain.
GRANT SELECT, INSERT ON display_task_handoffs, display_task_execution_evidence TO infrapro_display_packager;

-- Audit handoff is domain owned; the packager receives no public audit access.
GRANT SELECT,INSERT ON display_audit_handoffs TO infrapro_display_packager;
