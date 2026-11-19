-- Install using the maintenance owner after migration, in the application's
-- schema. Each deployment login inherits only its named NOLOGIN group.
BEGIN;
DO $$
DECLARE r text;
BEGIN
 IF (SELECT max(version_id) FROM goose_db_version WHERE is_applied)<>59 THEN
   RAISE EXCEPTION 'role grants require InfraPro schema 59';
 END IF;
 FOREACH r IN ARRAY ARRAY[
   'infrapro_catalog_reader','infrapro_session_check','infrapro_fact_writer',
   'infrapro_identity_api','infrapro_control_api','infrapro_query_api','infrapro_transfer_api',
   'infrapro_monitoring_publish','infrapro_monitoring_incremental','infrapro_monitoring_history','infrapro_monitoring_read',
   'infrapro_ingest_custody','infrapro_parsing_dispatch','infrapro_display_runtime','infrapro_display_assets',
   'infrapro_notification_worker','infrapro_spatial_worker','infrapro_compat_api',
   'infrapro_compat_ingest_worker','infrapro_compat_monitoring_worker'
 ] LOOP
   IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname=r) THEN EXECUTE format('CREATE ROLE %I NOLOGIN',r); END IF;
   EXECUTE format('GRANT USAGE ON SCHEMA %I TO %I',current_schema(),r);
 END LOOP;
END $$;

GRANT SELECT ON goose_db_version,authorization_state,roles,role_permissions,permission_catalog,account_roles,
 tenants,projects,tenant_memberships,tenant_member_roles,project_memberships,project_member_roles TO infrapro_catalog_reader;
GRANT SELECT(id,status,username,display_name,auth_version) ON accounts TO infrapro_catalog_reader;
GRANT UPDATE(architecture_lock) ON authorization_state,accounts,tenants,projects,device_tenant_states TO infrapro_catalog_reader;
GRANT SELECT(tenant_id) ON device_tenant_states TO infrapro_catalog_reader;
-- Both catalog IDs are constrained to the literal 1.
GRANT SELECT(id),UPDATE(id) ON engineering_catalog_state,device_catalog_state TO infrapro_catalog_reader;

GRANT infrapro_catalog_reader TO infrapro_session_check,infrapro_ingest_custody,infrapro_parsing_dispatch,
 infrapro_monitoring_publish,infrapro_monitoring_incremental,infrapro_monitoring_history,infrapro_monitoring_read,
 infrapro_notification_worker,infrapro_spatial_worker;
GRANT infrapro_session_check TO infrapro_identity_api,infrapro_control_api,infrapro_query_api,infrapro_transfer_api,
 infrapro_display_runtime,infrapro_display_assets,infrapro_monitoring_read;
GRANT SELECT ON platform_security_policy TO infrapro_session_check;
GRANT UPDATE(architecture_lock) ON platform_security_policy,account_sessions,account_mfa TO infrapro_session_check;
GRANT SELECT (id,account_id,revoked_at,expires_at,last_activity_at,reauthenticated_at,auth_version,
 authentication_method,mfa_verified_at,mfa_factor_version,policy_enforcement_generation) ON account_sessions TO infrapro_session_check;
GRANT SELECT (account_id,state,factor_version) ON account_mfa TO infrapro_session_check;

-- Domain transactions flush retained task facts and emit the existing durable
-- notification/audit handoffs. The package verifier retains its restricted role.
GRANT infrapro_audit_writer TO infrapro_fact_writer;
GRANT SELECT ON task_protocol,task_records,task_attempts,task_executions,task_events,task_relations,
 task_transaction_facts,notification_protocol,notification_events,notification_recipients,account_security_events TO infrapro_fact_writer;
GRANT INSERT, UPDATE ON task_records,task_executions,task_attempts TO infrapro_fact_writer;
GRANT INSERT ON task_events,task_relations,notification_events,notification_recipients,account_security_events TO infrapro_fact_writer;
GRANT UPDATE (source_key) ON notification_events TO infrapro_fact_writer;
GRANT SELECT, INSERT, DELETE ON task_transaction_facts TO infrapro_fact_writer;
GRANT infrapro_fact_writer TO infrapro_identity_api,infrapro_control_api,infrapro_transfer_api,infrapro_query_api,
 infrapro_monitoring_publish,infrapro_monitoring_incremental,infrapro_monitoring_history,infrapro_monitoring_read,
 infrapro_parsing_dispatch,infrapro_ingest_custody,infrapro_spatial_worker,infrapro_display_runtime,infrapro_notification_worker;

-- Expand only the named domain families in this release. No default privileges
-- on future tables, no TRUNCATE/DDL, no grant option and no access to other schemas.
CREATE TEMP TABLE arch_grants(role_name text, rights text, relations text) ON COMMIT DROP;
-- Reapplying the managed role profile also withdraws superseded ingestion
-- grants before installing individual domain command permissions.
DO $$
DECLARE relation record;
BEGIN
 FOR relation IN SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname=current_schema() AND c.relkind IN ('r','p') AND c.relname LIKE 'ingestion\_%' ESCAPE '\' LOOP
  EXECUTE format('REVOKE INSERT,UPDATE,DELETE ON %I.%I FROM infrapro_control_api',current_schema(),relation.relname);
 END LOOP;
END $$;
INSERT INTO arch_grants VALUES
 ('infrapro_identity_api','SELECT,INSERT,UPDATE,DELETE','^(accounts|account_.*|authorization_state|platform_security_policy|tenants|tenant_.*|projects|project_(codes|memberships|member_roles|operation_receipts)|roles|role_permissions|system_initialization)$'),
 ('infrapro_identity_api','SELECT','^(permission_catalog|project_facility_links|engineering_resource_nodes|monitoring_data_grants|project_device_grants|device_products|devices|notification_.*)$'),
 ('infrapro_control_api','SELECT','^(account_avatar_objects|engineering_.*|analysis_.*|derived_.*|device(s|_.*)|product_classification_records|point_borehole_positions|facility_type_adoptions|project_.*|parsing_.*|ingestion_.*|monitoring_.*|spatial_.*|display_.*|file_.*|task_.*|audit_.*|notification_.*|account_notifications)$'),
 ('infrapro_control_api','INSERT','^ingestion_(relays|sources|source_revisions|source_confirmations|terminals|terminal_revisions|change_intents|execution_grants|execution_grant_history|recovery_jobs|recovery_generations|operation_receipts|changes)$'),
 ('infrapro_control_api','UPDATE','^ingestion_(sources|terminals|execution_grants|recovery_jobs|recovery_generations|change_intents)$'),
 ('infrapro_control_api','INSERT,UPDATE,DELETE','^(engineering_.*|analysis_.*|derived_.*|device(s|_.*)|product_classification_records|point_borehole_positions|facility_type_adoptions|project_(device_.*|resource_.*|facility_links|spatial_.*)|parsing_.*|monitoring_.*|spatial_.*|display_.*|file_.*|task_.*|notification_.*|audit_(exports|export_generations|export_objects|operations|event_links|command_receipts)|account_notifications)$'),
 ('infrapro_query_api','SELECT','^(analysis_.*|derived_.*|engineering_.*|point_borehole_positions|facility_type_adoptions|device(s|_.*)|project_(device_.*|resource_.*|facility_links|spatial_.*)|product_classification_records|monitoring_.*|display_.*|spatial_.*|task_.*)$'),
 ('infrapro_query_api','INSERT,UPDATE,DELETE','^(monitoring_(snapshots|raw_snapshots)|display_read_(queries|cursors|query_rows|query_sections|sections|section_cursors|jobs|monitoring_refs|selections))$'),
 ('infrapro_transfer_api','SELECT','^(account_avatar_objects|display_.*|file_.*|audit_(exports|export_generations|export_objects|export_read_leases|protocol|transfer_receipts)|engineering_.*|point_borehole_positions|facility_type_adoptions|spatial_.*|monitoring_.*|analysis_.*|derived_.*|device(s|_.*)|project_(device_.*|resource_.*|facility_links|spatial_.*)|ingestion_.*|parsing_.*|task_.*)$'),
 ('infrapro_transfer_api','INSERT,UPDATE,DELETE','^(account_avatar_objects|monitoring_(uploads|upload_parts|source_files|import_batches|jobs|receipts|audit)|file_(uploads|upload_parts|command_receipts|jobs|job_runs|job_attempts|read_leases|rate_windows|objects|budget_states|budget_reservations|attachments|attachment_revisions|records)|spatial_(uploads|upload_parts|blobs|assets|asset_versions|processing_jobs|receipts|changes)|display_(uploads|upload_parts|package_jobs|receipts|object_intents|events)|parsing_(samples|objects|operation_receipts)|audit_(export_read_leases|transfer_receipts))$'),
 ('infrapro_transfer_api','INSERT','^parsing_changes$'),
 ('infrapro_ingest_custody','SELECT','^(ingestion_.*|parsing_(ingress_protocol_versions|ingress_compatibilities|method_versions|configs|config_revisions|device_adoptions|device_heads|product_defaults)|devices|device_(products|manufacturers|product_versions|product_version_slots|outputs|resource_changes|tenant_states)|project_device_grants)$'),
 ('infrapro_ingest_custody','INSERT,UPDATE,DELETE','^ingestion_(ack_outbox|receive_intents|objects|receipts|relays|source_confirmations|sources|source_revisions|changes|change_intents|epochs|execution_grant_history|execution_grants|device_associations|terminals|terminal_revisions|integrity_events|storage_state)$'),
 ('infrapro_parsing_dispatch','SELECT','^(ingestion_.*|parsing_.*|devices|device_.*|project_device_.*)$'),
 ('infrapro_parsing_dispatch','INSERT,UPDATE,DELETE','^(ingestion_(receipts|units|unit_assignments|identity_decisions|demultiplex_decisions|parse_plans|attempts|results|recovery_jobs|recovery_generations|objects|integrity_events|storage_state)|parsing_(run_attempts|run_executions|runs|objects|samples))$'),
 ('infrapro_compat_monitoring_worker','SELECT','^(parsing_(samples|objects|runs|run_executions|run_attempts|method_versions|configs|config_revisions|device_adoptions|device_heads|product_defaults)|device_(products|product_versions|product_version_slots|resource_changes|tenant_states)|project_device_grants)$'),
 ('infrapro_compat_monitoring_worker','INSERT,UPDATE','^parsing_(objects|run_attempts)$'),
 ('infrapro_compat_monitoring_worker','UPDATE','^parsing_(samples|runs|run_executions)$'),
 ('infrapro_monitoring_publish','SELECT','^(monitoring_.*|engineering_.*|derived_.*|project_(resource_.*|facility_links)|devices|device_outputs|analysis_.*)$'),
 ('infrapro_monitoring_publish','INSERT,UPDATE,DELETE','^monitoring_(jobs|job_steps|queue_fairness|publications|publication_chunks|contribution_replacements|write_attempts|chunk_locations|chunks|change_ranges|changes|channel_versions|dirty_ranges|import_batches|batch_resolutions|selections|source_heads|tenant_states|relocations|relocation_copies|audit|task_execution_evidence)$'),
 ('infrapro_monitoring_incremental','SELECT','^(monitoring_.*|engineering_.*|derived_.*|analysis_.*|project_(resource_.*|facility_links)|device_outputs|devices)$'),
 ('infrapro_monitoring_incremental','INSERT,UPDATE,DELETE','^(derived_.*|analysis_(runs|artifact_input_ranges)|monitoring_(jobs|job_steps|queue_fairness|dirty_ranges|change_consumers|summary_heads|summary_replacements|raw_snapshots|snapshots|task_execution_evidence|audit))$'),
 ('infrapro_monitoring_history','SELECT','^(monitoring_.*|engineering_.*|derived_.*|analysis_.*|project_(resource_.*|facility_links)|device_outputs|devices)$'),
 ('infrapro_monitoring_history','INSERT,UPDATE,DELETE','^(derived_.*|analysis_.*|monitoring_(jobs|job_steps|queue_fairness|summary_heads|summary_replacements|raw_snapshots|snapshots|exports|task_execution_evidence|audit))$'),
 ('infrapro_monitoring_read','SELECT','^(monitoring_.*|derived_.*|analysis_.*|display_.*|engineering_.*|device(s|_.*)|project_(resource_.*|device_.*|spatial_.*|facility_links)|spatial_.*)$'),
 ('infrapro_monitoring_read','INSERT,UPDATE,DELETE','^(display_read_(jobs|query_rows|cursors|queries|query_sections|section_cursors|monitoring_refs)|monitoring_(jobs|queue_fairness|job_steps|task_execution_evidence|snapshots|raw_snapshots|audit))$'),
 ('infrapro_display_runtime','SELECT','^(display_.*|monitoring_(access_states|data_grants|tenant_states)|project_(resource_policies|resource_grants|device_access_states|spatial_states)|derived_channel_heads|engineering_resource_nodes|device_classification_state|point_borehole_positions)$'),
 ('infrapro_display_runtime','INSERT,UPDATE,DELETE','^display_(sessions|subjects|terminals|pairings|events|tool_sessions|subscriptions|subscription_events)$'),
 ('infrapro_display_assets','SELECT','^display_(environment|instances|configs|settings|grants|releases|subjects|sessions|terminals|packages|package_files|read_config_scopes|read_scope_members|read_scope_sets)$'),
 ('infrapro_spatial_worker','SELECT','^(spatial_.*|project_spatial_.*|engineering_.*|facility_type_adoptions|point_borehole_positions)$'),
 ('infrapro_spatial_worker','INSERT,UPDATE,DELETE','^spatial_(processing_jobs|asset_versions|blobs|changes|version_files|task_execution_evidence)$'),
 ('infrapro_notification_worker','SELECT','^(notification_.*|account_notifications|tenant_invitations|task_[a-z_]+|display_(instances|notification_handoffs|settings)|file_task_metadata|monitoring_task_metadata|spatial_task_metadata|parsing_task_metadata|ingestion_task_metadata)$'),
 ('infrapro_notification_worker','INSERT,UPDATE,DELETE','^(account_notifications|notification_(deliveries|recipients|inbox_states|worker_heartbeats|conditions|events)|display_notification_handoffs|tenant_invitations)$');
DO $$
DECLARE item record; relation record; sequence_name text;
BEGIN
 FOR item IN SELECT * FROM arch_grants LOOP
   FOR relation IN SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
     WHERE n.nspname=current_schema() AND c.relkind IN ('r','p','v') AND c.relname ~ item.relations LOOP
      EXECUTE format('GRANT %s ON TABLE %I.%I TO %I',item.rights,current_schema(),relation.relname,item.role_name);
   END LOOP;
 END LOOP;
 -- Sequence USAGE follows INSERT privilege and is limited to the owning table.
 FOR item IN SELECT DISTINCT role_name FROM arch_grants UNION SELECT 'infrapro_fact_writer' LOOP
   FOR relation IN SELECT c.relname,a.attname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
     JOIN pg_attribute a ON a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped
     WHERE n.nspname=current_schema() AND c.relkind IN ('r','p') AND has_table_privilege(item.role_name,c.oid,'INSERT') LOOP
     sequence_name:=pg_get_serial_sequence(format('%I.%I',current_schema(),relation.relname),relation.attname);
     IF sequence_name IS NOT NULL THEN EXECUTE format('GRANT USAGE ON SEQUENCE %s TO %I',sequence_name,item.role_name); END IF;
   END LOOP;
 END LOOP;
END $$;

GRANT SELECT(created_at,updated_at,avatar_object_id,avatar_revision,username_normalized) ON accounts
 TO infrapro_transfer_api,infrapro_control_api,infrapro_notification_worker;
GRANT UPDATE(avatar_object_id,avatar_revision,updated_at) ON accounts TO infrapro_transfer_api;
-- The inbox rechecks whether the recipient issued each invitation before
-- including it in the unread summary. Tokens and invitation contents stay private.
GRANT SELECT(id,inviter_id) ON account_invitations TO infrapro_control_api;
GRANT EXECUTE ON FUNCTION architecture_notification_invitation_targets(uuid,uuid,uuid[]) TO infrapro_control_api;
-- Audited category-publication events resolve their original, published
-- dictionary through classification.releaseTx as the export's requester.
GRANT SELECT(id,number,content,content_hash,created_at,reason) ON device_category_releases TO infrapro_audit_worker;
GRANT SELECT(tenant_id,facility_id,kind,id,data) ON engineering_resource_records TO infrapro_audit_worker;
GRANT SELECT ON engineering_boreholes,engineering_monitoring_sections TO infrapro_audit_worker;
GRANT SELECT ON device_effective_classifications TO infrapro_audit_worker;
-- Device and product references are displayed using the effective published
-- category and its path. The current-release pointer suffices for this lookup.
GRANT SELECT(release_id,category_id,name,path,name_path,sensor,status) ON device_category_release_nodes TO infrapro_audit_worker;
GRANT SELECT(id,current_release_id) ON device_classification_state TO infrapro_audit_worker;
GRANT SELECT(id,product_version_id,revision,category_id,selected_release_id,ended_at) ON product_classification_records TO infrapro_audit_worker;
-- PKCE exchange, active display delivery and asset permit checks renew the
-- same personnel idle timestamp under READ COMMITTED.
GRANT UPDATE(last_activity_at) ON account_sessions TO infrapro_display_runtime,infrapro_display_assets,
 infrapro_query_api,infrapro_monitoring_read;
-- Pairing, start and the asset permit each revalidate the published binding
-- against the current project grants. These are visibility keys and public
-- metadata. No result object, payload, observation or write is exposed.
GRANT SELECT(id,status) ON project_facility_links TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,project_id,id,link_id,resource_kind,component_id,point_id,channel_id,monitoring_section_id,borehole_id,facility_id,ended_at)
 ON project_resource_grants TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,facility_id,id,kind,parent_id,parent_kind,deleted_at) ON engineering_resource_nodes TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,id,parent_component_id,deleted_at) ON engineering_components TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,id,component_id) ON engineering_points TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,id,facility_id,component_id,point_id,deleted_at,metric_version_id,derived_metric_version_id,unit_version_id,source_kind,quantity_spec,observation_direction,reference_basis)
 ON engineering_channels TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(id,content) ON engineering_versions TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,project_id,resource_grant_id,channel_id,revoked_at,capabilities,permitted) ON monitoring_data_grants TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,id,asset_id,state,content_available) ON spatial_asset_versions TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,id,facility_id) ON spatial_assets TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,project_id,version_id,facility_link_id,ended_at) ON project_spatial_grants TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,id,artifact_id) ON analysis_artifact_versions TO infrapro_display_runtime,infrapro_display_assets;
GRANT SELECT(tenant_id,project_id,id,deleted_at) ON analysis_artifacts TO infrapro_display_runtime,infrapro_display_assets;
-- The trusted terminal re-resolves format-2 selectors in its pairing or
-- renewal transaction, using the issuer's current project authority.
GRANT SELECT(tenant_id,id,type_version_id,deleted_at) ON engineering_facilities TO infrapro_display_runtime;
GRANT SELECT(definition_id) ON engineering_versions TO infrapro_display_runtime;
GRANT SELECT(owner_kind,monitoring_section_id,borehole_id) ON engineering_channels TO infrapro_display_runtime;
GRANT SELECT(tenant_id,project_id,ended_at,kind,device_id,output_id) ON project_device_grants TO infrapro_display_runtime;
GRANT SELECT(tenant_id,id,product_id,deleted_at,status) ON devices TO infrapro_display_runtime;
GRANT SELECT(id,manufacturer_id) ON device_products TO infrapro_display_runtime;
GRANT SELECT(tenant_id,id) ON device_outputs TO infrapro_display_runtime;
GRANT SELECT(tenant_id,project_id) ON project_facility_links TO infrapro_display_runtime;
GRANT SELECT(kind,name,code) ON spatial_assets TO infrapro_display_runtime;
GRANT SELECT(deleted_at,number) ON spatial_asset_versions TO infrapro_display_runtime;
GRANT SELECT(asset_id,is_default) ON project_spatial_grants TO infrapro_display_runtime;
-- Derived channel membership requires checking the input grants for each
-- effective configuration, including any sealed baseline's dependency range.
GRANT SELECT(tenant_id,channel_id,draft_config_id) ON derived_channels TO infrapro_display_runtime;
GRANT SELECT(tenant_id,channel_id,id,config_id,effective_range,created_revision,superseded_revision)
 ON derived_config_assignments TO infrapro_display_runtime;
GRANT SELECT(tenant_id,channel_id,id,definition_version_id,input_bindings,parameters,rule,semantic_hash,config_hash,
 output_contract_hash,graph_revision,created_at,baseline_bindings,base_config_id,change_kind,semantic_intent_confirmed,reset_baselines)
 ON derived_channel_configs TO infrapro_display_runtime;
GRANT SELECT(id,mode) ON monitoring_environment_state TO infrapro_display_runtime;
GRANT EXECUTE ON FUNCTION architecture_fixed_baseline_metadata(uuid,uuid,text,text)
 TO infrapro_display_runtime,infrapro_control_api,infrapro_query_api,infrapro_transfer_api,
 infrapro_monitoring_publish,infrapro_monitoring_incremental,infrapro_monitoring_history,infrapro_monitoring_read;
-- The issuer and the bound display subject must still qualify for every
-- input of a fixed analysis version at pairing, renewal and delivery.
-- These immutable PG records prove the source intervals, adoption and
-- integrity. Physical observations and result blocks stay with the query owner.
GRANT SELECT(scope_key,config_id,name,status,revision,latest_version,created_at,updated_at)
 ON analysis_artifacts TO infrapro_display_runtime;
GRANT SELECT(version_no,run_id,config_version_id,output_range,raw_snapshot_id,source_position,output_rows,
 stale_at_publication,created_at,manifest_root,lineage_root) ON analysis_artifact_versions TO infrapro_display_runtime;
GRANT SELECT(tenant_id,id,scope_key) ON analysis_configs TO infrapro_display_runtime;
GRANT SELECT(tenant_id,id,config_id,version_no,name,body,config_hash,plan_hash,engine_version,compiler_version,
 base_version_id,reset_baselines,created_at) ON analysis_config_versions TO infrapro_display_runtime;
GRANT SELECT(tenant_id,run_id,config_id,config_version_id,artifact_id,artifact_version_id,trial,outputs,source_range,run_rule)
 ON analysis_runs TO infrapro_display_runtime;
GRANT SELECT(tenant_id,result_kind,result_id,state) ON derived_result_integrity TO infrapro_display_runtime;
GRANT SELECT(tenant_id,snapshot_id,proof,proof_hash) ON derived_input_sources TO infrapro_display_runtime;
GRANT SELECT(tenant_id,id,channel_id,config_id,raw_snapshot_id,manifest_root,output_range)
 ON derived_generations TO infrapro_display_runtime;
GRANT SELECT(tenant_id,id,schema_version,source_position,route_revision,needs,candidates,manifest,integrity,history,content_hash)
 ON monitoring_raw_snapshots TO infrapro_display_runtime;
GRANT SELECT(tenant_id,channel_id,integrity_revision,quarantined) ON monitoring_channel_versions TO infrapro_display_runtime;
GRANT SELECT(timestamp_meaning) ON engineering_channels TO infrapro_display_runtime;
GRANT SELECT(tenant_id,version_id,channel_id,required_range) ON analysis_artifact_input_ranges TO infrapro_display_runtime;
GRANT SELECT(tenant_id,event_id,channel_id,affected,ordinal) ON monitoring_change_ranges TO infrapro_display_runtime;
GRANT SELECT(tenant_id,id,source_position,change_type,source_id,created_at) ON monitoring_changes TO infrapro_display_runtime;
GRANT SELECT(tenant_id,id,sequence) ON monitoring_publications TO infrapro_display_runtime;
-- Project context and governance return bounded resource counts and catalog
-- revisions. Identity does not maintain the underlying resource or device grants.
GRANT SELECT(tenant_id,project_id,revision,allow_facility_create) ON project_resource_policies TO infrapro_identity_api;
-- Creating a project seeds its default-deny policy in the same transaction.
-- Subsequent changes to that policy remain under the engineering owner.
GRANT INSERT(tenant_id,project_id) ON project_resource_policies TO infrapro_identity_api;
GRANT SELECT(tenant_id,project_id,revision) ON project_device_access_states TO infrapro_identity_api;
GRANT SELECT(tenant_id,project_id,link_id,resource_kind,component_id,point_id,channel_id,monitoring_section_id,borehole_id,facility_id,ended_at,can_create_derived_channel,can_manage_calculation,can_source_manage)
 ON project_resource_grants TO infrapro_identity_api;
-- Project permission upgrades check whether any domain resource has already
-- been authorized before broadening the protected owner's capabilities.
GRANT SELECT(tenant_id) ON derived_channels,analysis_configs TO infrapro_identity_api;
GRANT SELECT(tenant_id,project_id,revision) ON monitoring_access_states TO infrapro_identity_api;
GRANT SELECT(tenant_id,project_id) ON monitoring_import_batches,project_spatial_grants,project_spatial_views,
 display_instances,file_spaces,audit_exports TO infrapro_identity_api;
GRANT SELECT(tenant_id,project_id,enabled,revision) ON display_settings TO infrapro_identity_api;
-- A notification handoff is authenticated against the immutable event,
-- originating package task and its exact generation before translation.
GRANT SELECT(id,tenant_id,project_id,instance_id,kind,details) ON display_events TO infrapro_notification_worker;
GRANT SELECT(id,upload_id) ON display_package_jobs TO infrapro_notification_worker;
GRANT SELECT(id,actor_id,tenant_id,project_id,instance_id) ON display_uploads TO infrapro_notification_worker;
GRANT SELECT(id,job_id) ON display_packages TO infrapro_notification_worker;
GRANT SELECT(job_id,execution_generation,task_revision,snapshot,fingerprint) ON display_task_handoffs TO infrapro_notification_worker;
-- NotificationTargetsTx resolves the recipient and source host again before
-- disclosing the filename. Task indexing continues through file_task_metadata.
GRANT SELECT(id,space_id,host_id,upload_id,task_snapshot) ON file_jobs TO infrapro_notification_worker;
GRANT SELECT(id,display_name) ON file_uploads TO infrapro_notification_worker;
GRANT SELECT(id,restore_epoch,mode) ON file_protocol TO infrapro_notification_worker;
-- Query RPCs persist expiring, authorized selection IDs and their sealed
-- display windows. Resource reads acquire a shared facility row lock. The
-- resource_guard BEFORE trigger rejects any change to the facility ID.
GRANT INSERT(tenant_id,project_id,actor_id,display_subject_id,channel_ids,selection_hash,authorization_revision,expires_at),
 UPDATE(read_windows,expires_at) ON monitoring_selections TO infrapro_query_api;
-- QueueAnalytic creates only a person/terminal's background summary. Releasing
-- its display query can cancel that summary, never another execution lane.
REVOKE INSERT,UPDATE,DELETE ON monitoring_jobs FROM infrapro_query_api;
GRANT INSERT(tenant_id,project_id,actor_id,display_subject_id,kind,target_id,target_revision,input_hash,input),
 UPDATE(cancel_requested,state,revision,updated_at) ON monitoring_jobs TO infrapro_query_api;
ALTER TABLE monitoring_jobs ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS architecture_job_access ON monitoring_jobs;
CREATE POLICY architecture_job_access ON monitoring_jobs USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS architecture_query_job_create ON monitoring_jobs;
CREATE POLICY architecture_query_job_create ON monitoring_jobs AS RESTRICTIVE FOR INSERT TO infrapro_query_api
 WITH CHECK (pg_has_role(current_user,'infrapro_control_api','MEMBER') OR
  (kind='summary' AND input->>'mode'='query' AND num_nonnulls(actor_id,display_subject_id)=1));
DROP POLICY IF EXISTS architecture_query_job_cancel ON monitoring_jobs;
CREATE POLICY architecture_query_job_cancel ON monitoring_jobs AS RESTRICTIVE FOR UPDATE TO infrapro_query_api
 USING (pg_has_role(current_user,'infrapro_control_api','MEMBER') OR
  (kind='summary' AND input->>'mode'='query' AND display_subject_id IS NOT NULL))
 WITH CHECK (pg_has_role(current_user,'infrapro_control_api','MEMBER') OR
  (kind='summary' AND input->>'mode'='query' AND display_subject_id IS NOT NULL AND
   cancel_requested AND state IN ('running','cancelled')));
GRANT UPDATE(id) ON engineering_facilities TO infrapro_display_runtime,infrapro_query_api,infrapro_transfer_api,
 infrapro_monitoring_read,infrapro_monitoring_incremental,infrapro_monitoring_history;
-- Derived reads lock the tenant graph at its observed revision. PostgreSQL
-- requires a column UPDATE privilege to take this lock. The row is visible
-- to the locking reader, while WITH CHECK forbids even a no-op UPDATE. The
-- mixed-role compatibility executables keep their original command authority.
GRANT UPDATE(tenant_id) ON derived_graph_states TO infrapro_query_api,infrapro_transfer_api,infrapro_monitoring_read;
ALTER TABLE derived_graph_states ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS architecture_graph_access ON derived_graph_states;
CREATE POLICY architecture_graph_access ON derived_graph_states USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS architecture_graph_lock_only ON derived_graph_states;
CREATE POLICY architecture_graph_lock_only ON derived_graph_states AS RESTRICTIVE FOR UPDATE
 TO infrapro_query_api,infrapro_transfer_api,infrapro_monitoring_read USING (true)
 WITH CHECK (pg_has_role(current_user,'infrapro_control_api','USAGE') OR
             pg_has_role(current_user,'infrapro_monitoring_incremental','USAGE') OR
             pg_has_role(current_user,'infrapro_monitoring_history','USAGE'));
-- File transfers share-lock the protocol singleton constrained to id=1 and
-- advance only the current space revision when a command commits its receipt.
GRANT UPDATE(id) ON file_protocol TO infrapro_transfer_api;
GRANT UPDATE(content_revision) ON file_spaces TO infrapro_transfer_api;
-- An audit download locks the export and its generation before deciding if
-- the caller can read it. PostgreSQL needs an UPDATE privilege for each lock.
-- The transfer owner may also mark the object missing after a failed readback.
GRANT UPDATE(id) ON audit_exports TO infrapro_transfer_api;
GRANT UPDATE(export_id) ON audit_export_generations TO infrapro_transfer_api;
GRANT UPDATE(state) ON audit_export_objects TO infrapro_transfer_api;
-- HEAD and GET reserve verified download bytes against the node budget, then
-- release that reservation after the object is closed.
GRANT SELECT(id,node_id,reserved_bytes,expires_at,released_at),
 INSERT(id,node_id,reserved_bytes,expires_at), UPDATE(released_at) ON audit_temp_reservations TO infrapro_transfer_api;
CREATE OR REPLACE FUNCTION architecture_guard_audit_transfer() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF pg_has_role(current_user,'infrapro_transfer_api','USAGE')
    AND NOT pg_has_role(current_user,'infrapro_compat_api','USAGE') THEN
   IF TG_TABLE_NAME='audit_export_objects' THEN
     IF OLD.state='available' AND NEW.state='missing' THEN RETURN NEW; END IF;
   ELSIF TG_TABLE_NAME='audit_temp_reservations' THEN
     IF OLD.released_at IS NULL AND NEW.released_at IS NOT NULL THEN RETURN NEW; END IF;
   END IF;
   RAISE EXCEPTION 'audit transfer only permits an integrity mark or a first reservation release' USING ERRCODE='42501';
 END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION architecture_guard_audit_transfer() FROM PUBLIC;
DROP TRIGGER IF EXISTS architecture_transfer_guard ON audit_exports;
CREATE TRIGGER architecture_transfer_guard BEFORE UPDATE OF id ON audit_exports
 FOR EACH ROW EXECUTE FUNCTION architecture_guard_audit_transfer();
DROP TRIGGER IF EXISTS architecture_transfer_guard ON audit_export_generations;
CREATE TRIGGER architecture_transfer_guard BEFORE UPDATE OF export_id ON audit_export_generations
 FOR EACH ROW EXECUTE FUNCTION architecture_guard_audit_transfer();
DROP TRIGGER IF EXISTS architecture_transfer_guard ON audit_export_objects;
CREATE TRIGGER architecture_transfer_guard BEFORE UPDATE OF state ON audit_export_objects
 FOR EACH ROW EXECUTE FUNCTION architecture_guard_audit_transfer();
DROP TRIGGER IF EXISTS architecture_transfer_guard ON audit_temp_reservations;
CREATE TRIGGER architecture_transfer_guard BEFORE UPDATE OF released_at ON audit_temp_reservations
 FOR EACH ROW EXECUTE FUNCTION architecture_guard_audit_transfer();
-- Finishing an upload traverses the same transactional display Command
-- prelude and revision checks as management. Package transitions also record
-- immutable task and audit handoffs through the existing table triggers.
GRANT INSERT(tenant_id,project_id) ON display_settings TO infrapro_transfer_api;
GRANT UPDATE(revision) ON display_instances TO infrapro_transfer_api;
GRANT UPDATE(architecture_lock) ON display_instances TO infrapro_display_runtime;
REVOKE INSERT,UPDATE,DELETE ON display_uploads,display_upload_parts,display_receipts,display_package_jobs,display_object_intents
 FROM infrapro_display_runtime;
-- Only the verifier registers validated packages and their immutable files.
-- Control owns cancel, retry and reclaim requests for existing inputs.
REVOKE INSERT,UPDATE,DELETE ON display_package_files,display_packages,display_package_jobs,display_upload_parts,display_object_intents
 FROM infrapro_control_api;
GRANT UPDATE(state) ON display_packages TO infrapro_control_api;
GRANT UPDATE(state,execution_generation,fence,lease_until,error_code,issues) ON display_package_jobs TO infrapro_control_api;
GRANT INSERT(job_id,task_revision,execution_generation,snapshot,fingerprint,occurred_at)
 ON display_task_handoffs TO infrapro_transfer_api;
GRANT INSERT(job_id,generation,error_code,report,progress,input_hash,executor_ref,finished_at)
 ON display_task_execution_evidence TO infrapro_transfer_api;
GRANT INSERT(source_key,job_id,generation,fence,tenant_id,project_id,instance_id,state,safe_error_code,occurred_at)
 ON display_audit_handoffs TO infrapro_transfer_api;
-- User commands change association eligibility or cancel an existing recovery.
-- Raw acceptance, automatic decisions, and measurement result creation use the
-- custody and dispatch roles.
GRANT UPDATE(enabled) ON ingestion_device_associations TO infrapro_control_api;
GRANT UPDATE(state,fence,lease_until,error_code) ON ingestion_identity_decisions TO infrapro_control_api;
GRANT UPDATE(state,fence,lease_until,error_code,finished_at) ON ingestion_parse_plans TO infrapro_control_api;
GRANT UPDATE(confirmed) ON ingestion_epochs TO infrapro_parsing_dispatch;
-- Control can commit an authorized revocation. The immutable publication and
-- its physical chunk associations are created only by the publication worker.
REVOKE INSERT,UPDATE,DELETE ON monitoring_publications,monitoring_publication_chunks FROM infrapro_control_api;
GRANT UPDATE(revoke_sequence,revoked_at,revoked_by,revoke_reason) ON monitoring_publications TO infrapro_control_api;
REVOKE UPDATE,DELETE ON monitoring_publications,monitoring_publication_chunks FROM infrapro_monitoring_publish;
-- Publication and formal calculation materialize the initial 256 logical
-- routes on first use. Controlled relocations advance the route state.
GRANT INSERT(tenant_id,logical_shard,revision,physical_shard,state) ON monitoring_routes
 TO infrapro_monitoring_publish,infrapro_monitoring_incremental,infrapro_monitoring_history;
GRANT UPDATE(state) ON monitoring_routes TO infrapro_monitoring_publish;
-- Both an incremental calculation and a backfill publish their sealed generation
-- in the same transaction as its durable source position and matching range.
GRANT UPDATE(commit_sequence) ON monitoring_tenant_states TO infrapro_monitoring_incremental,infrapro_monitoring_history;
GRANT INSERT(tenant_id,source_position,change_type,source_kind,source_id,impact_hash) ON monitoring_changes
 TO infrapro_monitoring_incremental,infrapro_monitoring_history;
GRANT INSERT(tenant_id,event_id,ordinal,channel_id,affected) ON monitoring_change_ranges
 TO infrapro_monitoring_incremental,infrapro_monitoring_history;
ALTER TABLE monitoring_changes ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS architecture_change_access ON monitoring_changes;
CREATE POLICY architecture_change_access ON monitoring_changes USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS architecture_derived_publication ON monitoring_changes;
CREATE POLICY architecture_derived_publication ON monitoring_changes AS RESTRICTIVE FOR INSERT
 TO infrapro_monitoring_incremental,infrapro_monitoring_history WITH CHECK (
  pg_has_role(current_user,'infrapro_monitoring_publish','USAGE') OR
  (change_type='derived_publish' AND source_kind='derived' AND EXISTS (
   SELECT 1 FROM derived_generations g WHERE g.tenant_id=monitoring_changes.tenant_id
    AND g.channel_id=monitoring_changes.source_id AND g.publish_position=monitoring_changes.source_position)));
ALTER TABLE monitoring_change_ranges ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS architecture_change_range_access ON monitoring_change_ranges;
CREATE POLICY architecture_change_range_access ON monitoring_change_ranges USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS architecture_derived_publication_range ON monitoring_change_ranges;
CREATE POLICY architecture_derived_publication_range ON monitoring_change_ranges AS RESTRICTIVE FOR INSERT
 TO infrapro_monitoring_incremental,infrapro_monitoring_history WITH CHECK (
  pg_has_role(current_user,'infrapro_monitoring_publish','USAGE') OR EXISTS (
   SELECT 1 FROM monitoring_changes c JOIN derived_generations g ON g.tenant_id=c.tenant_id
    AND g.channel_id=c.source_id AND g.publish_position=c.source_position
   WHERE c.tenant_id=monitoring_change_ranges.tenant_id AND c.id=monitoring_change_ranges.event_id
    AND c.change_type='derived_publish' AND c.source_kind='derived'
    AND g.channel_id=monitoring_change_ranges.channel_id AND g.output_range=monitoring_change_ranges.affected));
-- Only the automatic calculation owner opens, coalesces and recovers condition
-- cycles. Other producers, including the display handoff, retain their own rows.
GRANT SELECT(source_module,condition_key,active,cycle,occurrences,first_at), INSERT(source_module,condition_key),
 UPDATE(active,cycle,occurrences,first_at,last_at,recovered_at) ON notification_conditions TO infrapro_monitoring_incremental;
ALTER TABLE notification_conditions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS architecture_condition_access ON notification_conditions;
CREATE POLICY architecture_condition_access ON notification_conditions USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS architecture_derived_condition ON notification_conditions;
CREATE POLICY architecture_derived_condition ON notification_conditions AS RESTRICTIVE TO infrapro_monitoring_incremental
 USING (source_module='derivedmonitoring') WITH CHECK (source_module='derivedmonitoring');
-- Exporting a frozen selection checks the lifetime of an optional display
-- query reference, including for an ordinary tenant or project export.
GRANT SELECT(query_id,snapshot_id) ON display_read_monitoring_refs TO infrapro_monitoring_history;
GRANT SELECT(id,state,expires_at) ON display_read_queries TO infrapro_monitoring_history;
-- Export and background summary queries materialize the original tree head.
-- Discharge a dirty range only after its own tenant/channel has reached it.
GRANT DELETE ON monitoring_dirty_ranges TO infrapro_monitoring_history,infrapro_monitoring_read;
ALTER TABLE monitoring_dirty_ranges ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS architecture_dirty_access ON monitoring_dirty_ranges;
CREATE POLICY architecture_dirty_access ON monitoring_dirty_ranges USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS architecture_dirty_covered ON monitoring_dirty_ranges;
CREATE POLICY architecture_dirty_covered ON monitoring_dirty_ranges AS RESTRICTIVE FOR DELETE
 TO infrapro_monitoring_incremental,infrapro_monitoring_history,infrapro_monitoring_read USING (EXISTS (
  SELECT 1 FROM monitoring_summary_heads h WHERE h.tenant_id=monitoring_dirty_ranges.tenant_id
   AND h.channel_id=monitoring_dirty_ranges.channel_id AND h.level='tree-v1' AND h.watermark>=monitoring_dirty_ranges.target_sequence));
GRANT INSERT(tenant_id,channel_id,level,revision,watermark,revoke_revision,root_object),
 UPDATE(revision,watermark,revoke_revision,root_object) ON monitoring_summary_heads TO infrapro_monitoring_read;
ALTER TABLE monitoring_summary_heads ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS architecture_summary_access ON monitoring_summary_heads;
CREATE POLICY architecture_summary_access ON monitoring_summary_heads USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS architecture_query_summary_head ON monitoring_summary_heads;
CREATE POLICY architecture_query_summary_head ON monitoring_summary_heads AS RESTRICTIVE TO infrapro_monitoring_read
 USING (true) WITH CHECK (pg_has_role(current_user,'infrapro_monitoring_history','USAGE') OR
  (level='tree-v1' AND EXISTS (SELECT 1 FROM monitoring_channel_versions v JOIN monitoring_tenant_states t USING(tenant_id)
   WHERE v.tenant_id=monitoring_summary_heads.tenant_id AND v.channel_id=monitoring_summary_heads.channel_id
    AND v.content_revision>=monitoring_summary_heads.revision AND v.revoke_revision>=monitoring_summary_heads.revoke_revision
    AND t.commit_sequence>=monitoring_summary_heads.watermark)));
-- During an operator-requested rebuild, the worker publishes the exact object
-- in each completed checkpoint before releasing the repaired channel.
GRANT INSERT(tenant_id,chunk_id,job_id,object_key) ON monitoring_contribution_replacements TO infrapro_monitoring_history;
ALTER TABLE monitoring_contribution_replacements ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS architecture_contribution_access ON monitoring_contribution_replacements;
CREATE POLICY architecture_contribution_access ON monitoring_contribution_replacements USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS architecture_repaired_contribution ON monitoring_contribution_replacements;
CREATE POLICY architecture_repaired_contribution ON monitoring_contribution_replacements AS RESTRICTIVE FOR INSERT
 TO infrapro_monitoring_history WITH CHECK (pg_has_role(current_user,'infrapro_monitoring_publish','USAGE') OR EXISTS (
  SELECT 1 FROM monitoring_jobs j JOIN monitoring_job_steps s ON s.tenant_id=j.tenant_id AND s.job_id=j.id AND s.fence=j.fence
   WHERE j.tenant_id=monitoring_contribution_replacements.tenant_id AND j.id=monitoring_contribution_replacements.job_id
    AND j.kind='summary' AND j.input->>'mode'='rebuild' AND j.state='running' AND j.lease_until>clock_timestamp() AND NOT j.cancel_requested
    AND s.state='completed' AND s.step_key='contribution:'||monitoring_contribution_replacements.chunk_id::text
    AND s.result->>'chunk'=monitoring_contribution_replacements.chunk_id::text AND s.result->>'object'=monitoring_contribution_replacements.object_key));
GRANT UPDATE(quarantined) ON monitoring_channel_versions TO infrapro_monitoring_history;
ALTER TABLE monitoring_channel_versions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS architecture_channel_version_access ON monitoring_channel_versions;
CREATE POLICY architecture_channel_version_access ON monitoring_channel_versions USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS architecture_repaired_channel ON monitoring_channel_versions;
CREATE POLICY architecture_repaired_channel ON monitoring_channel_versions AS RESTRICTIVE FOR UPDATE
 TO infrapro_monitoring_history USING (true) WITH CHECK (pg_has_role(current_user,'infrapro_monitoring_publish','USAGE') OR
  (NOT quarantined AND EXISTS (SELECT 1 FROM monitoring_summary_replacements r JOIN monitoring_jobs j ON j.tenant_id=r.tenant_id AND j.id=r.job_id
   WHERE r.tenant_id=monitoring_channel_versions.tenant_id AND r.channel_id=monitoring_channel_versions.channel_id
    AND r.content_revision=monitoring_channel_versions.content_revision AND r.integrity_revision=monitoring_channel_versions.integrity_revision
    AND j.target_id=r.channel_id AND j.kind='summary' AND j.input->>'mode'='rebuild'
    AND j.state='running' AND j.lease_until>clock_timestamp() AND NOT j.cancel_requested)));
-- An interactive analysis may seal exactly the receipt for its live trial job.
GRANT INSERT(tenant_id,run_id,result) ON analysis_trial_receipts TO infrapro_monitoring_incremental;
ALTER TABLE analysis_trial_receipts ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS architecture_analysis_trial_access ON analysis_trial_receipts;
CREATE POLICY architecture_analysis_trial_access ON analysis_trial_receipts USING (true) WITH CHECK (true);
DROP POLICY IF EXISTS architecture_live_analysis_trial ON analysis_trial_receipts;
CREATE POLICY architecture_live_analysis_trial ON analysis_trial_receipts AS RESTRICTIVE FOR INSERT
 TO infrapro_monitoring_incremental WITH CHECK (
  result->>'trial_receipt_id'=run_id::text AND EXISTS (
   SELECT 1 FROM analysis_runs a JOIN derived_runs r ON r.tenant_id=a.tenant_id AND r.id=a.run_id
   JOIN monitoring_jobs j ON j.tenant_id=r.tenant_id AND j.id=r.job_id AND j.target_id=r.id
   WHERE a.tenant_id=analysis_trial_receipts.tenant_id AND a.run_id=analysis_trial_receipts.run_id
    AND a.trial AND r.mode='analysis' AND j.kind='analysis' AND j.input->>'trial'='true'
    AND j.state='running' AND j.lease_until>clock_timestamp() AND NOT j.cancel_requested));
-- Durable invalidation evidence is append-only. Project spatial grant
-- changes use the same global authorization revision as other control commands.
GRANT INSERT(tenant_id,version_id,event_id) ON analysis_artifact_changes TO infrapro_monitoring_incremental;
GRANT UPDATE(revision) ON authorization_state TO infrapro_control_api;
GRANT SELECT(method_id,method_version,entry,state,available) ON parsing_identification_versions TO infrapro_ingest_custody;
-- Resolving the sealed measurement method's output bindings verifies its
-- versioned unit and metric definitions both at the watermark and at dispatch.
GRANT SELECT(id,definition_id,number,content,digest,schema_version,schemas,unit_category,published_at,published_by)
 ON engineering_versions TO infrapro_ingest_custody,infrapro_parsing_dispatch;
GRANT SELECT(id,kind,code,tenant_id) ON engineering_definitions TO infrapro_ingest_custody,infrapro_parsing_dispatch;
GRANT SELECT(version_id,enabled,revision,reason) ON engineering_version_states TO infrapro_ingest_custody,infrapro_parsing_dispatch;
GRANT SELECT(version_id,target_id) ON engineering_dependencies TO infrapro_ingest_custody,infrapro_parsing_dispatch;
-- Existing AFTER triggers advance the display revision inside the owning
-- domain transaction. The constrained application users do not need a direct
-- way to manufacture other tenants' revision changes.
REVOKE INSERT,UPDATE,DELETE ON display_read_revisions FROM infrapro_control_api;
REVOKE ALL ON FUNCTION display_read_changed() FROM PUBLIC;
ALTER FUNCTION display_read_changed() SECURITY DEFINER;
DO $$ BEGIN
 EXECUTE format('ALTER FUNCTION %I.display_read_changed() SET search_path TO pg_catalog,%I,pg_temp',current_schema(),current_schema());
END $$;
-- Tenant and project AFTER INSERT triggers establish these initial states.
GRANT INSERT ON derived_graph_states,device_tenant_states,project_device_access_states TO infrapro_identity_api;
-- Grant the durable clock only to the owning lane, including maintenance in
-- display, notifications, ordinary files and manual parsing.
ALTER TABLE monitoring_scheduler_leases ENABLE ROW LEVEL SECURITY;
DO $$
DECLARE clock record;
BEGIN
 FOR clock IN SELECT * FROM (VALUES
  ('infrapro_identity_api', 'lane IN (''identity/session-activity'',''identity/identity-cleanup'',''identity/identity-retention'')'),
  ('infrapro_monitoring_incremental', 'lane=''derived-schedule'''),
  ('infrapro_monitoring_publish','lane=''storage-verification'''),
  ('infrapro_monitoring_read','lane=''display-read-cleanup'''),
  ('infrapro_parsing_dispatch','lane=''manual-parsing-cleanup'''),
  ('infrapro_compat_monitoring_worker','lane=''manual-parsing-cleanup'''),
  ('infrapro_files_worker','lane=''file-object-cleanup'' OR lane LIKE ''file-sweep/%'''),
  ('infrapro_notification_worker','lane=''notification-maintenance''')
 ) AS r(role_name,predicate) LOOP
   EXECUTE format('GRANT SELECT,INSERT,UPDATE(next_run) ON monitoring_scheduler_leases TO %I',clock.role_name);
   EXECUTE format('DROP POLICY IF EXISTS %I ON monitoring_scheduler_leases',clock.role_name);
   EXECUTE format('CREATE POLICY %I ON monitoring_scheduler_leases TO %I USING (%s) WITH CHECK (%s)',
       clock.role_name,clock.role_name,clock.predicate,clock.predicate);
 END LOOP;
END $$;
GRANT SELECT,UPDATE(state) ON display_packages TO infrapro_display_assets;
GRANT SELECT,INSERT ON display_events,display_notification_handoffs TO infrapro_display_assets;
GRANT USAGE ON SEQUENCE display_events_id_seq TO infrapro_display_assets;
GRANT infrapro_audit_api TO infrapro_control_api;
GRANT infrapro_identity_api,infrapro_control_api,infrapro_query_api,infrapro_transfer_api TO infrapro_compat_api;
GRANT infrapro_ingest_custody,infrapro_parsing_dispatch TO infrapro_compat_ingest_worker;
GRANT infrapro_monitoring_publish,infrapro_monitoring_incremental,infrapro_monitoring_history,infrapro_monitoring_read TO infrapro_compat_monitoring_worker;
-- A query login may materialize the existing summary/query operation. It
-- cannot enqueue, relabel or retire publication, calculation or history jobs.
CREATE OR REPLACE FUNCTION architecture_guard_query_job() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (pg_has_role(current_user,'infrapro_query_api','USAGE') OR pg_has_role(current_user,'infrapro_monitoring_read','USAGE'))
    AND NOT pg_has_role(current_user,'infrapro_compat_api','USAGE')
    AND NOT pg_has_role(current_user,'infrapro_compat_monitoring_worker','USAGE') THEN
   IF TG_OP<>'INSERT' AND (OLD.kind<>'summary' OR OLD.input->>'mode' IS DISTINCT FROM 'query') THEN
     RAISE EXCEPTION 'query role requires a query job' USING ERRCODE='42501';
   END IF;
   IF TG_OP<>'DELETE' AND (NEW.kind<>'summary' OR NEW.input->>'mode' IS DISTINCT FROM 'query') THEN
     RAISE EXCEPTION 'query role requires a query job' USING ERRCODE='42501';
   END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION architecture_guard_query_job() FROM PUBLIC;
DROP TRIGGER IF EXISTS architecture_query_job_guard ON monitoring_jobs;
CREATE TRIGGER architecture_query_job_guard BEFORE INSERT OR UPDATE OR DELETE ON monitoring_jobs
 FOR EACH ROW EXECUTE FUNCTION architecture_guard_query_job();
-- No query/asset/worker group is a member of the credential owner. No role gets
-- ownership, general schema creation, superuser or a business login password.
COMMIT;
