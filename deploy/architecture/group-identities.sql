-- Role names are cluster-wide. pg_restore --no-acl still recreates policies
-- referring to their groups, so provision these identities on a new cluster first.
DO $$ DECLARE n text; BEGIN
 FOREACH n IN ARRAY ARRAY['infrapro_audit_api','infrapro_audit_content','infrapro_audit_maintenance','infrapro_audit_observer','infrapro_audit_restore','infrapro_audit_worker','infrapro_audit_writer','infrapro_catalog_reader','infrapro_compat_api','infrapro_compat_ingest_worker','infrapro_compat_monitoring_worker','infrapro_control_api','infrapro_display_assets','infrapro_display_packager','infrapro_display_runtime','infrapro_event_admin','infrapro_event_relay','infrapro_fact_writer','infrapro_files_backup','infrapro_files_maintenance','infrapro_files_observer','infrapro_files_worker','infrapro_identity_api','infrapro_ingest_custody','infrapro_monitoring_history','infrapro_monitoring_incremental','infrapro_monitoring_publish','infrapro_monitoring_read','infrapro_notification_worker','infrapro_parsing_dispatch','infrapro_query_api','infrapro_session_check','infrapro_spatial_worker','infrapro_task_index','infrapro_transfer_api'] LOOP
  IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname=n) THEN EXECUTE format('CREATE ROLE %I NOLOGIN',n); END IF;
  IF EXISTS(SELECT FROM pg_roles WHERE rolname=n AND (rolcanlogin OR rolsuper OR rolbypassrls)) THEN
    RAISE EXCEPTION 'unsafe existing deployment group %',n;
  END IF;
 END LOOP;
END $$;
