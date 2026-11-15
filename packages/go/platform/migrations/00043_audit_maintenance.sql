-- +goose Up
ALTER TABLE audit_cleanup_plans ADD COLUMN apply_owner uuid, ADD COLUMN apply_until timestamptz;
CREATE TABLE audit_temp_reservations (
 id uuid PRIMARY KEY, node_id text NOT NULL, reserved_bytes bigint NOT NULL CHECK(reserved_bytes>0 AND reserved_bytes<=104857600),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),expires_at timestamptz NOT NULL,released_at timestamptz
);
CREATE TABLE display_audit_handoffs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), source_key text NOT NULL UNIQUE,
 event_id bigint,job_id uuid NOT NULL,generation bigint NOT NULL,fence bigint NOT NULL,
 tenant_id uuid NOT NULL,project_id uuid NOT NULL,instance_id uuid NOT NULL,
 state text NOT NULL CHECK(state IN ('accepted','succeeded','failed','cancelled')),
 safe_error_code text NOT NULL DEFAULT '',occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 translated_at timestamptz,attempts integer NOT NULL DEFAULT 0,last_error_code text NOT NULL DEFAULT ''
);
CREATE INDEX display_audit_pending ON display_audit_handoffs(occurred_at,id) WHERE translated_at IS NULL;
-- +goose StatementBegin
CREATE FUNCTION display_audit_validate() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE valid boolean;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF (to_jsonb(NEW)-'translated_at'-'attempts'-'last_error_code') IS DISTINCT FROM (to_jsonb(OLD)-'translated_at'-'attempts'-'last_error_code') THEN RAISE EXCEPTION 'immutable display audit handoff'; END IF;
  RETURN NEW;
 END IF;
 SELECT EXISTS(SELECT 1 FROM display_package_jobs j JOIN display_uploads u ON u.id=j.upload_id WHERE j.id=NEW.job_id AND j.execution_generation=NEW.generation AND j.fence=NEW.fence AND u.tenant_id=NEW.tenant_id AND u.project_id=NEW.project_id AND u.instance_id=NEW.instance_id
 AND NEW.state=CASE j.state WHEN 'queued' THEN 'accepted' WHEN 'ready' THEN 'succeeded' WHEN 'failed' THEN 'failed' WHEN 'canceled' THEN 'cancelled' ELSE '' END
 AND NEW.source_key=j.id::text||':'||j.execution_generation::text||':'||NEW.state AND NEW.occurred_at=j.task_observed_at AND NEW.safe_error_code=j.error_code) INTO valid;
 IF NOT valid THEN RAISE EXCEPTION 'invalid display audit handoff'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER display_audit_validate BEFORE INSERT OR UPDATE ON display_audit_handoffs FOR EACH ROW EXECUTE FUNCTION display_audit_validate();
CREATE FUNCTION display_audit_capture() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE action text; u display_uploads;
BEGIN
 IF TG_OP='UPDATE' AND NEW.state=OLD.state AND NEW.execution_generation=OLD.execution_generation THEN RETURN NEW; END IF;
 action:=CASE NEW.state WHEN 'queued' THEN 'accepted' WHEN 'ready' THEN 'succeeded' WHEN 'failed' THEN 'failed' WHEN 'canceled' THEN 'cancelled' ELSE NULL END;
 IF action IS NULL THEN RETURN NEW; END IF;
 SELECT * INTO u FROM display_uploads WHERE id=NEW.upload_id;
 INSERT INTO display_audit_handoffs(source_key,job_id,generation,fence,tenant_id,project_id,instance_id,state,safe_error_code,occurred_at)
 VALUES(NEW.id::text||':'||NEW.execution_generation::text||':'||action,NEW.id,NEW.execution_generation,NEW.fence,u.tenant_id,u.project_id,u.instance_id,action,NEW.error_code,NEW.task_observed_at) ON CONFLICT(source_key) DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER display_audit_capture AFTER INSERT OR UPDATE ON display_package_jobs FOR EACH ROW EXECUTE FUNCTION display_audit_capture();
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='infrapro_audit_observer') THEN CREATE ROLE infrapro_audit_observer NOLOGIN; END IF;
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='infrapro_audit_maintenance') THEN CREATE ROLE infrapro_audit_maintenance NOLOGIN; END IF;
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='infrapro_audit_worker') THEN CREATE ROLE infrapro_audit_worker NOLOGIN; END IF;
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='infrapro_audit_api') THEN CREATE ROLE infrapro_audit_api NOLOGIN; END IF;
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='infrapro_audit_writer') THEN CREATE ROLE infrapro_audit_writer NOLOGIN; END IF;
 EXECUTE format('GRANT USAGE ON SCHEMA %I TO infrapro_audit_observer,infrapro_audit_maintenance,infrapro_audit_worker,infrapro_audit_api',current_schema());
 EXECUTE format('GRANT USAGE ON SCHEMA %I TO infrapro_audit_writer',current_schema());
END $$;
CREATE FUNCTION audit_link_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' AND EXISTS(SELECT 1 FROM audit_cleanup_inflight WHERE transaction_id=pg_current_xact_id() AND event_id=OLD.event_id) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'audit_link_immutable';
END $$;
CREATE TRIGGER audit_link_immutable BEFORE UPDATE OR DELETE ON audit_event_links FOR EACH ROW EXECUTE FUNCTION audit_link_immutable();

CREATE FUNCTION audit_apply_fact_cleanup(plan uuid,expected bigint,request uuid,why text) RETURNS jsonb
 LANGUAGE plpgsql SECURITY DEFINER AS $$
DECLARE p audit_cleanup_plans; current_epoch uuid; candidate record; deleted_count integer:=0; protected_count integer:=0; result jsonb;
BEGIN
 IF request IS NULL OR char_length(trim(why)) NOT BETWEEN 1 AND 500 THEN RAISE EXCEPTION 'audit_maintenance_input_invalid'; END IF;
 SELECT summary INTO result FROM audit_maintenance_receipts WHERE request_id=request AND operation='cleanup.facts' AND plan_id=plan;
 IF FOUND THEN RETURN result; END IF;
 PERFORM pg_advisory_xact_lock(834327122);
 SELECT restore_epoch INTO current_epoch FROM audit_protocol WHERE id=1 AND mode='ready' FOR SHARE;
 IF NOT FOUND THEN RAISE EXCEPTION 'audit_protocol_unavailable'; END IF;
 SELECT * INTO p FROM audit_cleanup_plans WHERE id=plan AND kind='facts' FOR UPDATE;
 IF NOT FOUND OR p.revision<>expected OR p.epoch<>current_epoch OR p.state NOT IN ('planned','partial') THEN RAISE EXCEPTION 'audit_plan_changed'; END IF;
 IF (SELECT count(*) FROM audit_cleanup_items WHERE plan_id=plan)>500 THEN RAISE EXCEPTION 'audit_plan_limit'; END IF;
 IF EXISTS(SELECT 1 FROM audit_export_generations WHERE state IN ('queued','running','retry_wait','cancelling') AND snapshot_at IS NULL)
 OR EXISTS(SELECT 1 FROM display_audit_handoffs WHERE translated_at IS NULL) THEN RAISE EXCEPTION 'audit_history_in_use'; END IF;
 FOR candidate IN SELECT i.object_id,i.policy_version FROM audit_cleanup_items i WHERE i.plan_id=plan AND i.state<>'deleted' ORDER BY i.object_id LOOP
  PERFORM 1 FROM account_security_events e WHERE e.id::text=candidate.object_id AND e.retention_policy_version=candidate.policy_version AND e.retain_until IS NOT NULL AND e.retain_until<=clock_timestamp() AND e.retain_until<=p.cutoff FOR UPDATE;
  IF NOT FOUND OR EXISTS(SELECT 1 FROM audit_retention_holds h WHERE h.released_at IS NULL AND h.expires_at>clock_timestamp() AND (h.object_kind IN ('all','backup') OR h.object_kind='event' AND h.object_id=candidate.object_id)) THEN
   UPDATE audit_cleanup_items SET state='protected' WHERE plan_id=plan AND object_id=candidate.object_id;protected_count:=protected_count+1;CONTINUE;
  END IF;
  INSERT INTO audit_cleanup_inflight(transaction_id,event_id) VALUES(pg_current_xact_id(),candidate.object_id::bigint);
  DELETE FROM audit_event_links WHERE event_id=candidate.object_id::bigint;
  DELETE FROM account_security_events WHERE id=candidate.object_id::bigint;
  DELETE FROM audit_cleanup_inflight WHERE transaction_id=pg_current_xact_id() AND event_id=candidate.object_id::bigint;
  UPDATE audit_cleanup_items SET state='deleted' WHERE plan_id=plan AND object_id=candidate.object_id;deleted_count:=deleted_count+1;
 END LOOP;
 UPDATE audit_cleanup_plans SET state=CASE WHEN protected_count=0 THEN 'applied' ELSE 'partial' END,revision=revision+1,applied_at=clock_timestamp(),reason=why WHERE id=plan;
 result:=jsonb_build_object('deleted',deleted_count,'protected',protected_count,'plan_id',plan);
 INSERT INTO audit_maintenance_receipts(request_id,operation,plan_id,reason,summary) VALUES(request,'cleanup.facts',plan,why,result);
 RETURN result;
END $$;
DO $$ BEGIN
 EXECUTE format('ALTER FUNCTION audit_apply_fact_cleanup(uuid,bigint,uuid,text) SET search_path TO pg_catalog,%I',current_schema());
 EXECUTE format('ALTER FUNCTION audit_link_immutable() SET search_path TO pg_catalog,%I',current_schema());
 EXECUTE format('ALTER FUNCTION audit_fact_immutable() SET search_path TO pg_catalog,%I',current_schema());
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION audit_apply_fact_cleanup(uuid,bigint,uuid,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION audit_apply_fact_cleanup(uuid,bigint,uuid,text) TO infrapro_audit_maintenance;
REVOKE ALL ON audit_cleanup_inflight FROM infrapro_audit_api,infrapro_audit_worker,infrapro_audit_observer,infrapro_audit_maintenance;
CREATE VIEW audit_status_metadata AS SELECT p.version,p.mode,p.restore_epoch,p.projection_version,
 (SELECT count(*) FROM account_security_events WHERE visibility_class='legacy_unclassified') AS unclassified,
 (SELECT count(*) FROM audit_export_generations WHERE state IN ('queued','running','retry_wait','cancelling')) AS active_exports,
 (SELECT count(*) FROM audit_export_objects WHERE state='intent') AS object_intents,
 (SELECT count(*) FROM audit_transfer_receipts WHERE closed_at IS NULL) AS open_transfers,
 (SELECT count(*) FROM display_audit_handoffs WHERE translated_at IS NULL) AS pending_handoffs,
 (SELECT max(heartbeat_at) FROM audit_worker_states) AS worker_seen FROM audit_protocol p WHERE p.id=1;
CREATE VIEW audit_verification_metadata AS SELECT count(*) AS checked,count(*) FILTER(WHERE schema_version=1 AND (fingerprint IS NULL OR source_key IS NULL OR operation_id IS NULL OR NOT EXISTS(SELECT 1 FROM audit_operations o WHERE o.id=operation_id))) AS invalid,count(*) FILTER(WHERE visibility_class='legacy_unclassified') AS unclassified FROM account_security_events;
GRANT SELECT ON audit_verification_metadata TO infrapro_audit_observer;
GRANT SELECT ON audit_status_metadata,audit_source_states,audit_worker_states,audit_retention_policies,audit_cleanup_plans,audit_cleanup_items,audit_maintenance_receipts TO infrapro_audit_observer;
GRANT infrapro_audit_observer TO infrapro_audit_maintenance;
GRANT UPDATE ON audit_export_objects,audit_export_read_leases,audit_export_generations,audit_exports,audit_transfer_receipts,audit_temp_reservations TO infrapro_audit_maintenance;
GRANT DELETE ON audit_export_rows TO infrapro_audit_maintenance;
GRANT SELECT,INSERT ON audit_cleanup_plans,audit_cleanup_items,audit_retention_holds,audit_maintenance_receipts,audit_retention_policies TO infrapro_audit_maintenance;
GRANT UPDATE ON audit_cleanup_plans,audit_cleanup_items,audit_retention_holds,audit_source_states,audit_protocol TO infrapro_audit_maintenance;
GRANT SELECT ON account_security_events,audit_event_links,audit_exports,audit_export_generations,audit_export_objects,audit_export_rows,audit_export_read_leases,audit_command_receipts,audit_operations,audit_transfer_receipts,display_audit_handoffs TO infrapro_audit_maintenance;
GRANT UPDATE(visibility_class,category,phase,occurred_at,time_source,missing_fields,retention_class,retention_policy_version,retain_until) ON account_security_events TO infrapro_audit_maintenance;
GRANT SELECT,INSERT ON account_security_events,audit_operations,audit_command_receipts,audit_event_links,audit_transfer_receipts TO infrapro_audit_api,infrapro_audit_worker;
GRANT SELECT ON audit_protocol,audit_retention_policies TO infrapro_audit_api,infrapro_audit_worker;
GRANT UPDATE(last_seen_at,closed_at,outcome,sent_bytes) ON audit_transfer_receipts TO infrapro_audit_api,infrapro_audit_worker;
GRANT SELECT,INSERT,UPDATE ON audit_exports,audit_export_generations,audit_export_objects,audit_export_read_leases,audit_temp_reservations TO infrapro_audit_api,infrapro_audit_worker;
GRANT SELECT,INSERT ON audit_export_rows TO infrapro_audit_worker;
GRANT SELECT,INSERT,UPDATE ON audit_worker_states,audit_source_states TO infrapro_audit_worker;
GRANT SELECT,UPDATE ON display_audit_handoffs TO infrapro_audit_worker;
GRANT DELETE ON audit_export_rows TO infrapro_audit_worker;
GRANT USAGE ON SEQUENCE account_security_events_id_seq TO infrapro_audit_api,infrapro_audit_worker;
GRANT SELECT ON goose_db_version TO infrapro_audit_observer,infrapro_audit_maintenance,infrapro_audit_worker;
GRANT SELECT(id,fingerprint,operation_id,source_module,source_key,effect_key,tenant_id,project_id,visibility_class) ON account_security_events TO infrapro_audit_writer;
GRANT INSERT ON account_security_events TO infrapro_audit_writer;
GRANT USAGE ON SEQUENCE account_security_events_id_seq TO infrapro_audit_writer;
GRANT SELECT ON audit_protocol,audit_retention_policies TO infrapro_audit_writer;
GRANT UPDATE(id) ON audit_protocol TO infrapro_audit_writer,infrapro_audit_api,infrapro_audit_worker;
GRANT SELECT(translated_at,tenant_id,project_id) ON display_audit_handoffs TO infrapro_audit_api;
GRANT SELECT,INSERT ON audit_operations,audit_command_receipts,audit_event_links,audit_transfer_receipts TO infrapro_audit_writer;
GRANT UPDATE(last_seen_at,closed_at,outcome,sent_bytes) ON audit_transfer_receipts TO infrapro_audit_writer;
GRANT infrapro_audit_writer TO infrapro_files_worker,infrapro_files_maintenance,infrapro_files_backup,infrapro_audit_maintenance;
GRANT SELECT(id,status,username,display_name,auth_version) ON accounts TO infrapro_audit_worker;
GRANT SELECT ON authorization_state,roles,role_permissions,permission_catalog,account_roles,tenant_memberships,tenant_member_roles,project_memberships,project_member_roles,tenants,projects TO infrapro_audit_worker;
GRANT UPDATE(id) ON authorization_state,accounts,tenants,projects TO infrapro_audit_worker;
GRANT SELECT ON notification_protocol,notification_events,notification_recipients,notification_deliveries TO infrapro_audit_worker;
GRANT INSERT ON notification_events,notification_recipients TO infrapro_audit_worker;
GRANT UPDATE(source_key) ON notification_events TO infrapro_audit_worker;
GRANT SELECT ON task_protocol,task_records,task_executions,task_events,task_attempts,task_transaction_facts TO infrapro_audit_worker;
GRANT INSERT,UPDATE ON task_records,task_executions,task_attempts TO infrapro_audit_worker;
GRANT INSERT ON task_events,task_relations TO infrapro_audit_worker;
GRANT DELETE ON task_transaction_facts TO infrapro_audit_worker;
GRANT SELECT(id,upload_id,execution_generation,fence) ON display_package_jobs TO infrapro_audit_worker;
GRANT SELECT(id,actor_id,tenant_id,project_id,instance_id) ON display_uploads TO infrapro_audit_worker;
GRANT SELECT(job_id,execution_generation,task_revision,snapshot) ON display_task_handoffs TO infrapro_audit_worker;
-- Dedicated content verification and coordinated restore capabilities are not
-- inherited by metadata observers or routine cleanup credentials.
-- +goose StatementBegin
DO $$ DECLARE r text; BEGIN
 FOREACH r IN ARRAY ARRAY['infrapro_audit_content','infrapro_audit_restore'] LOOP
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=r) THEN EXECUTE format('CREATE ROLE %I NOLOGIN',r); END IF;
 EXECUTE format('GRANT USAGE ON SCHEMA %I TO %I',current_schema(),r);
 END LOOP;
END $$;
-- +goose StatementEnd
GRANT infrapro_audit_observer TO infrapro_audit_content;
GRANT SELECT(id,object_key,sha256,size_bytes,state) ON audit_export_objects TO infrapro_audit_content;
GRANT infrapro_audit_maintenance,infrapro_files_backup TO infrapro_audit_restore;
GRANT SELECT,UPDATE ON task_protocol,task_source_states TO infrapro_audit_restore;
GRANT SELECT ON authorization_state TO infrapro_audit_restore;
GRANT SELECT ON audit_temp_reservations TO infrapro_audit_restore;
GRANT UPDATE(id) ON authorization_state TO infrapro_audit_restore;
-- Draft deletion must preserve fixed task generations and their source evidence.
ALTER TABLE spatial_asset_versions ADD COLUMN deleted_at timestamptz;
ALTER TABLE spatial_asset_versions ADD CHECK(deleted_at IS NULL OR state='draft');
-- These are business metadata consumed by source-owned qualification adapters.
-- No credentials, raw observations, object bodies, storage keys or inboxes.
GRANT SELECT ON engineering_definitions,engineering_versions,engineering_drafts,engineering_version_states,engineering_dependencies,engineering_facilities,engineering_components,engineering_points,engineering_channels,project_resource_policies,project_facility_links,project_resource_grants,device_manufacturers,device_products,device_product_versions,device_product_drafts,device_product_version_states,device_product_version_slots,device_outputs,devices,device_installations,project_device_access_states,project_device_grants,monitoring_source_heads,monitoring_source_bindings,monitoring_access_states,monitoring_data_grants,monitoring_jobs,monitoring_import_batches,monitoring_import_mappings,monitoring_batch_resolutions,monitoring_snapshots,monitoring_selections,monitoring_channel_versions,monitoring_publications,monitoring_publication_chunks,derived_runs,derived_channels,derived_channel_configs,derived_metric_definitions,derived_metric_versions,derived_metric_drafts,derived_metric_references,derived_result_integrity,derived_generations,derived_run_raw_inputs,derived_fixed_baselines,analysis_configs,analysis_config_versions,analysis_version_inputs,analysis_version_references,analysis_artifacts,analysis_artifact_versions TO infrapro_audit_worker;
GRANT SELECT ON project_spatial_states,project_spatial_grants TO infrapro_audit_worker;
GRANT SELECT(id,tenant_id,name,facility_id) ON spatial_assets TO infrapro_audit_worker;
GRANT SELECT(id,tenant_id,asset_id,state,content_available) ON spatial_asset_versions TO infrapro_audit_worker;
GRANT SELECT(id,tenant_id,project_id,name,state) ON display_instances TO infrapro_audit_worker;
GRANT SELECT ON file_spaces TO infrapro_audit_worker;
GRANT SELECT(id,space_id,display_name,status) ON file_attachments TO infrapro_audit_worker;
GRANT SELECT(id,space_id,filename,actor_id) ON file_uploads TO infrapro_audit_worker;
GRANT SELECT(id,space_id,originator_id) ON file_jobs TO infrapro_audit_worker;
GRANT SELECT(id,tenant_id,job_id,expires_at,invalidated_at) ON monitoring_exports TO infrapro_audit_worker;
-- Finite metadata needed by current-reference resolution; no object keys or tokens.
GRANT SELECT(id,attachment_id) ON file_attachment_revisions TO infrapro_audit_worker;
GRANT SELECT(id,tenant_id,project_id,instance_id) ON display_packages TO infrapro_audit_worker;
GRANT SELECT(id,tenant_id,target_account_id) ON tenant_invitations TO infrapro_audit_worker;
GRANT SELECT ON monitoring_task_metadata TO infrapro_audit_worker;

GRANT SELECT(id,tenant_id,version_id) ON spatial_uploads,spatial_processing_jobs TO infrapro_audit_worker;

GRANT UPDATE(attempts,last_error_code) ON display_audit_handoffs TO infrapro_audit_maintenance;

GRANT SELECT(host_id,task_snapshot) ON file_jobs TO infrapro_audit_worker;
GRANT SELECT(restore_epoch,mode,id) ON file_protocol TO infrapro_audit_worker;
GRANT SELECT ON engineering_resource_nodes,engineering_catalog_state,device_catalog_state,spatial_task_metadata,display_task_metadata,derived_config_assignments TO infrapro_audit_worker;
GRANT UPDATE(id) ON engineering_facilities,engineering_catalog_state,device_catalog_state TO infrapro_audit_worker;
GRANT SELECT ON device_tenant_states TO infrapro_audit_worker;
GRANT UPDATE(tenant_id) ON device_tenant_states TO infrapro_audit_worker;
GRANT SELECT(id,mode) ON monitoring_environment_state TO infrapro_audit_worker;
GRANT SELECT ON monitoring_tenant_states TO infrapro_audit_worker;
GRANT SELECT ON derived_calculation_config_keys TO infrapro_audit_worker;
-- Fixed-input provenance used for current permission and integrity checks;
-- these contain channel/range/hash metadata, never observation values.
GRANT SELECT(id,tenant_id,schema_version,source_position,route_revision,needs,candidates,manifest,integrity,history,content_hash) ON monitoring_raw_snapshots TO infrapro_audit_worker;
GRANT SELECT ON monitoring_changes,monitoring_change_ranges TO infrapro_audit_worker;
GRANT SELECT(run_id,expires_at) ON derived_trial_receipts TO infrapro_audit_worker;
GRANT SELECT(source_kind,artifact_version_id,generation_id,format) ON monitoring_exports TO infrapro_audit_worker;
GRANT SELECT ON analysis_artifact_input_ranges,analysis_runs,analysis_comparisons TO infrapro_audit_worker;
GRANT SELECT ON derived_input_sources,derived_replay_targets TO infrapro_audit_worker;
CREATE VIEW analysis_trial_receipt_metadata WITH (security_barrier=true) AS SELECT tenant_id,run_id,(result->>'expires_at')::timestamptz AS expires_at FROM analysis_trial_receipts;
REVOKE ALL ON analysis_trial_receipt_metadata FROM PUBLIC;
GRANT SELECT ON analysis_trial_receipt_metadata TO infrapro_audit_worker;
GRANT SELECT(id,tenant_id,project_id,config) ON project_spatial_views TO infrapro_audit_worker;
GRANT SELECT(status) ON spatial_assets TO infrapro_audit_worker;
GRANT SELECT(source_blob_id,manifest_hash,deleted_at) ON spatial_asset_versions TO infrapro_audit_worker;
GRANT SELECT(attempt) ON display_package_jobs TO infrapro_audit_worker;
GRANT SELECT(state) ON display_uploads TO infrapro_audit_worker;
GRANT SELECT(state,job_id) ON display_packages TO infrapro_audit_worker;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Audit maintenance history is irreversible'; END $$;
-- +goose StatementEnd

