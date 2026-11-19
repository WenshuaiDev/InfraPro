-- InfraPro 2026-11-19, business database on host 37.
-- First run the release CLI migrations and permission catalog sync/check.
-- From the directory containing this file: sha256sum -c SHA256SUMS
-- Use psql -X -v ON_ERROR_STOP=1 -f apply-compat-59.sql with the existing
-- ip_maintenance connection to infrapro. Keep credentials out of arguments.
-- The four unmodified release scripts manage their own transactions. Every
-- step can be repeated after an interruption. Keep the write window closed
-- until the entire file succeeds. The site membership changes commit together.
\set ON_ERROR_STOP on
SET search_path TO public;

DO $site$
DECLARE
    login_name text;
BEGIN
    IF current_database() <> 'infrapro' OR current_user <> 'ip_maintenance'
       OR current_schema() IS DISTINCT FROM 'public'
       OR NOT EXISTS (
           SELECT FROM pg_catalog.pg_database
           WHERE datname = current_database() AND datdba = current_user::regrole
       ) THEN
        RAISE EXCEPTION 'Use the existing infrapro database owner ip_maintenance on the business cluster';
    END IF;
    IF (SELECT max(version_id) FROM public.goose_db_version WHERE is_applied) IS DISTINCT FROM 59 THEN
        RAISE EXCEPTION 'Complete the release migration to schema 59 before installing roles';
    END IF;
    FOREACH login_name IN ARRAY ARRAY[
        'ip_api','ip_ingest_worker','ip_monitoring_worker','ip_notification','ip_spatial_worker',
        'ip_display_runtime','ip_files_worker','ip_audit_worker','ip_task_index','ip_display_packager'
    ] LOOP
        IF NOT EXISTS (
            SELECT FROM pg_catalog.pg_roles WHERE rolname = login_name AND rolcanlogin AND rolinherit
              AND NOT (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication)
        ) THEN
            RAISE EXCEPTION 'Expected the existing limited production login %', login_name;
        END IF;
    END LOOP;
END
$site$;

\ir deploy/architecture/restore-base-grants.sql
\ir deploy/display/package-worker-role.sql
\ir deploy/architecture/postgres-roles.sql
\ir deploy/events/postgres-grants.sql

BEGIN;

-- Inherit as the real session's existing LOGIN. In particular the compatibility
-- restrictions on the current RLS identity must also apply to worker tickets.
-- PostgreSQL 17 supports role membership with SET disabled.
GRANT infrapro_compat_api               TO ip_api               WITH SET FALSE;
GRANT infrapro_compat_ingest_worker     TO ip_ingest_worker     WITH SET FALSE;
GRANT infrapro_compat_monitoring_worker TO ip_monitoring_worker WITH SET FALSE;
GRANT infrapro_notification_worker      TO ip_notification      WITH SET FALSE;
GRANT infrapro_spatial_worker           TO ip_spatial_worker    WITH SET FALSE;
GRANT infrapro_display_runtime          TO ip_display_runtime   WITH SET FALSE;

-- The original files/audit/task-index/package logins already inherit their
-- respective groups. The relay database, diagnostic logins and backup identity
-- have separate roles and are outside this production membership change.
DO $verify$
DECLARE
    binding record;
    login_name text;
BEGIN
    FOR binding IN SELECT * FROM (VALUES
        ('ip_api',               'infrapro_compat_api',               true),
        ('ip_ingest_worker',     'infrapro_compat_ingest_worker',     true),
        ('ip_monitoring_worker', 'infrapro_compat_monitoring_worker', true),
        ('ip_notification',      'infrapro_notification_worker',      true),
        ('ip_spatial_worker',    'infrapro_spatial_worker',           true),
        ('ip_display_runtime',   'infrapro_display_runtime',          true),
        ('ip_files_worker',      'infrapro_files_worker',             false),
        ('ip_audit_worker',      'infrapro_audit_worker',             false),
        ('ip_task_index',        'infrapro_task_index',               false),
        ('ip_display_packager',  'infrapro_display_packager',         false)
    ) AS expected(app_login, group_name, new_grant) LOOP
        IF NOT pg_has_role(binding.app_login, binding.group_name, 'USAGE') THEN
            RAISE EXCEPTION '% must inherit its production group %', binding.app_login, binding.group_name;
        END IF;
        IF binding.new_grant AND NOT EXISTS (
            SELECT FROM pg_catalog.pg_auth_members m
            WHERE m.member = binding.app_login::regrole AND m.roleid = binding.group_name::regrole
              AND m.inherit_option AND NOT m.set_option AND NOT m.admin_option
        ) THEN
            RAISE EXCEPTION 'Invalid options for production membership % → %', binding.app_login, binding.group_name;
        END IF;
        IF pg_has_role(binding.app_login, 'infrapro_event_admin', 'MEMBER')
           OR pg_has_role(binding.app_login, 'infrapro_event_relay', 'MEMBER') THEN
            RAISE EXCEPTION 'Unexpected event administration or publisher membership on %', binding.app_login;
        END IF;
        IF binding.app_login <> 'ip_api' AND (
           has_column_privilege(binding.app_login, 'public.account_mfa', 'secret_ciphertext', 'SELECT')
           OR has_column_privilege(binding.app_login, 'public.account_auth_challenges', 'token_hash', 'SELECT')
           OR has_column_privilege(binding.app_login, 'public.account_mfa_recovery_codes', 'code_hash', 'SELECT')
        ) THEN
            RAISE EXCEPTION 'Unexpected access to personnel authentication material on %', binding.app_login;
        END IF;
    END LOOP;

    IF NOT (has_column_privilege('ip_api', 'public.account_mfa', 'key_id', 'SELECT')
       AND has_column_privilege('ip_api', 'public.account_auth_challenges', 'key_id', 'SELECT')
       AND has_column_privilege('ip_api', 'public.account_mfa', 'secret_ciphertext', 'UPDATE')
       AND has_column_privilege('ip_api', 'public.account_mfa_recovery_codes', 'code_hash', 'INSERT')
       AND has_column_privilege('ip_api', 'public.platform_security_policy', 'require_mfa', 'UPDATE')
       AND has_table_privilege('ip_api', 'public.account_avatar_objects', 'INSERT')
       AND has_column_privilege('ip_api', 'public.analysis_artifacts', 'deleted_at', 'UPDATE')
       AND has_function_privilege('ip_api', 'public.architecture_notification_invitation_targets(uuid,uuid,uuid[])', 'EXECUTE')) THEN
        RAISE EXCEPTION 'The release compatibility API identity, avatar or disclosure privileges are incomplete';
    END IF;
    FOREACH login_name IN ARRAY ARRAY['ip_api','ip_ingest_worker','ip_monitoring_worker'] LOOP
        IF NOT (has_column_privilege(login_name, 'public.dispatch_ownership', 'lock_token', 'UPDATE')
           AND has_table_privilege(login_name, 'public.dispatch_leases', 'INSERT')) THEN
            RAISE EXCEPTION 'Missing row-fenced dispatch access for %', login_name;
        END IF;
        IF has_column_privilege(login_name, 'public.dispatch_ownership', 'mode', 'UPDATE') THEN
            RAISE EXCEPTION 'A runtime login can select the active topology: %', login_name;
        END IF;
    END LOOP;
    IF pg_has_role('ip_monitoring_worker', 'infrapro_parsing_dispatch', 'MEMBER')
       OR pg_has_role('ip_ingest_worker', 'infrapro_monitoring_publish', 'MEMBER')
       OR NOT (has_column_privilege('ip_monitoring_worker', 'public.parsing_executor_slots', 'lease_until', 'UPDATE')
         AND has_function_privilege('ip_monitoring_worker', 'public.event_publication_permit(boolean)', 'EXECUTE')
         AND has_table_privilege('ip_notification', 'public.event_work', 'SELECT')
         AND has_column_privilege('ip_notification', 'public.monitoring_scheduler_leases', 'next_run', 'UPDATE')
         AND has_table_privilege('ip_notification', 'public.account_notifications', 'INSERT')
         AND has_function_privilege('ip_display_runtime', 'public.architecture_fixed_baseline_metadata(uuid,uuid,text,text)', 'EXECUTE')) THEN
        RAISE EXCEPTION 'Incomplete or mixed compatibility worker groups';
    END IF;
    IF (SELECT count(*) FROM pg_catalog.pg_policy WHERE
            (polrelid = 'public.dispatch_leases'::regclass AND polname = 'compat_workers_lease'
                AND NOT polpermissive AND polqual IS NOT NULL AND polwithcheck IS NOT NULL)
            OR (polrelid = 'public.dispatch_ownership'::regclass AND polname = 'compat_workers_owner'
                AND NOT polpermissive)) <> 2
       OR (SELECT count(*) FROM pg_catalog.pg_class WHERE
            oid IN ('public.dispatch_ownership'::regclass,'public.dispatch_leases'::regclass,'public.parsing_executor_slots'::regclass)
            AND relrowsecurity) <> 3 THEN
        RAISE EXCEPTION 'The release compatibility dispatch policies are incomplete';
    END IF;
END
$verify$;

SELECT current_database() AS database_name, current_user AS maintenance_owner,
       (SELECT max(version_id) FROM public.goose_db_version WHERE is_applied) AS schema_version;
SELECT member_role.rolname AS runtime_login, parent_role.rolname AS role_group,
       m.inherit_option, m.set_option, m.admin_option
FROM pg_catalog.pg_auth_members m
JOIN pg_catalog.pg_roles member_role ON member_role.oid = m.member
JOIN pg_catalog.pg_roles parent_role ON parent_role.oid = m.roleid
WHERE member_role.rolname IN ('ip_api','ip_ingest_worker','ip_monitoring_worker','ip_notification',
    'ip_spatial_worker','ip_display_runtime','ip_files_worker','ip_audit_worker','ip_task_index','ip_display_packager')
ORDER BY member_role.rolname, parent_role.rolname;
SELECT role, partition, mode, generation, restore_epoch FROM public.dispatch_ownership ORDER BY role, partition;
COMMIT;
