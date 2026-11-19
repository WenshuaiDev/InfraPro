-- Apply as schema maintenance owner after schema 59 and the architecture roles.
BEGIN;
DO $$
DECLARE r text;
BEGIN
 EXECUTE format('ALTER FUNCTION %I.event_publication_permit(boolean) SET search_path TO pg_catalog,%I,pg_temp',current_schema(),current_schema());
 FOREACH r IN ARRAY ARRAY['infrapro_event_admin','infrapro_event_relay'] LOOP
  IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname=r) THEN EXECUTE format('CREATE ROLE %I NOLOGIN',r);END IF;
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO %I',current_schema(),r);
 END LOOP;
END $$;
GRANT SELECT ON goose_db_version,event_destinations,event_protocol,event_status,event_consumer_positions TO infrapro_event_relay,infrapro_event_admin;
GRANT SELECT,UPDATE(lock_token) ON event_protocol TO infrapro_event_relay;
GRANT EXECUTE ON FUNCTION event_prune(bigint,integer) TO infrapro_event_relay;
GRANT SELECT,UPDATE(state,available_at,attempts,lease_until,fence,last_error,published_at,broker_stream,broker_sequence)
 ON event_outbox TO infrapro_event_relay;
GRANT SELECT ON event_work,event_quarantine,dispatch_ownership TO infrapro_event_relay;
GRANT SELECT,UPDATE(oldest_at,sampled_at,broker_bytes,broker_limit_bytes,storage_level,broker_sampled_at) ON event_pressure TO infrapro_event_relay;
GRANT SELECT,UPDATE ON event_protocol,dispatch_ownership,event_quarantine,event_work,event_pressure TO infrapro_event_admin;
GRANT SELECT,INSERT ON event_outbox,event_work,event_consumer_positions,event_quarantine,event_operations TO infrapro_event_admin;
GRANT SELECT,DELETE ON dispatch_leases TO infrapro_event_admin;
GRANT SELECT ON monitoring_tenant_states,event_destinations,event_status TO infrapro_event_admin;

CREATE TEMP TABLE event_grants(group_name text,roles text[],consumers text[]) ON COMMIT DROP;
INSERT INTO event_grants VALUES
 ('infrapro_ingest_custody',ARRAY['ingest-custody-worker'],'{}'),
 ('infrapro_parsing_dispatch',ARRAY['parsing-dispatch-worker'],ARRAY['identify','plan','parse']),
 ('infrapro_monitoring_publish',ARRAY['monitoring-publish-worker'],'{}'),
 ('infrapro_monitoring_history',ARRAY['monitoring-history-worker'],'{}'),
 ('infrapro_monitoring_incremental',ARRAY['monitoring-incremental-worker'],ARRAY['summary','derived']),
 ('infrapro_monitoring_read',ARRAY['monitoring-read-worker'],ARRAY['query-cache','display-cache']),
 ('infrapro_task_index','{}',ARRAY['task-index']),('infrapro_notification_worker','{}',ARRAY['notification']),
 ('infrapro_audit_worker','{}',ARRAY['audit']);
-- These schema-59 technical tables share a business prefix but their watermarks
-- and reservations are written only by their owning worker.
REVOKE INSERT,UPDATE,DELETE ON monitoring_change_origins,monitoring_cache_floors,monitoring_scheduler_leases,parsing_executor_slots FROM infrapro_control_api;
GRANT SELECT ON event_pressure TO infrapro_ingest_custody,infrapro_monitoring_publish;
GRANT EXECUTE ON FUNCTION event_publication_permit(boolean) TO infrapro_monitoring_publish,infrapro_monitoring_incremental,infrapro_monitoring_history;
GRANT SELECT,UPDATE ON parsing_executor_slots TO infrapro_parsing_dispatch;
GRANT SELECT ON monitoring_change_origins TO infrapro_monitoring_incremental;
GRANT SELECT,INSERT,UPDATE ON monitoring_cache_floors TO infrapro_monitoring_read;
GRANT SELECT ON monitoring_cache_floors TO infrapro_query_api,infrapro_display_runtime,infrapro_control_api;
GRANT SELECT,INSERT,UPDATE ON monitoring_scheduler_leases TO infrapro_monitoring_publish,infrapro_monitoring_incremental;
-- RLS restricts workers sharing one business schema to the destinations they
-- execute. The publisher cannot manufacture facts. Rotor operations are
-- exposed only to the deployment's offline maintenance login.
DO $$
DECLARE g record; tab text; policy_name text; rows_allowed text;
BEGIN
 FOREACH tab IN ARRAY ARRAY['event_work','event_consumptions','event_consumer_positions','event_quarantine','event_outbox','dispatch_ownership','dispatch_leases'] LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',tab);
  FOREACH policy_name IN ARRAY ARRAY['infrapro_event_admin','infrapro_event_relay'] LOOP
   EXECUTE format('DROP POLICY IF EXISTS %I ON %I',policy_name,tab);
   EXECUTE format('CREATE POLICY %I ON %I TO %I USING (true) WITH CHECK (true)',policy_name,tab,policy_name);
  END LOOP;
 END LOOP;
 FOR g IN SELECT * FROM event_grants LOOP
  EXECUTE format('GRANT SELECT,UPDATE(lock_token) ON event_protocol TO %I',g.group_name);
  IF cardinality(g.roles)>0 THEN
   EXECUTE format('GRANT SELECT,UPDATE(lock_token) ON dispatch_ownership TO %I',g.group_name);
   EXECUTE format('GRANT SELECT,INSERT,DELETE,UPDATE ON dispatch_leases TO %I',g.group_name);
   FOREACH tab IN ARRAY ARRAY['dispatch_ownership','dispatch_leases'] LOOP
    rows_allowed:=format('partition=''all'' AND role=ANY(%L::text[])',g.roles);
    EXECUTE format('DROP POLICY IF EXISTS %I ON %I',g.group_name,tab);
    EXECUTE format('CREATE POLICY %I ON %I TO %I USING (%s) WITH CHECK (%s)',g.group_name,tab,g.group_name,rows_allowed,rows_allowed);
   END LOOP;
  END IF;
  IF cardinality(g.consumers)>0 THEN
   EXECUTE format('GRANT SELECT ON event_outbox,event_destinations TO %I',g.group_name);
   EXECUTE format('GRANT SELECT,UPDATE(state,available_at,lease_until,fence,attempts) ON event_work TO %I',g.group_name);
   EXECUTE format('GRANT SELECT,INSERT,UPDATE ON event_consumptions,event_consumer_positions,event_quarantine TO %I',g.group_name);
   FOREACH tab IN ARRAY ARRAY['event_work','event_consumptions','event_consumer_positions','event_quarantine','event_outbox'] LOOP
    rows_allowed:=format('consumer=ANY(%L::text[])',g.consumers);
    IF tab='event_outbox' THEN rows_allowed:=format('type IN (SELECT type FROM event_destinations WHERE consumer=ANY(%L::text[]))',g.consumers);END IF;
    EXECUTE format('DROP POLICY IF EXISTS %I ON %I',g.group_name,tab);
    EXECUTE format('CREATE POLICY %I ON %I TO %I USING (%s) WITH CHECK (%s)',g.group_name,tab,g.group_name,rows_allowed,rows_allowed);
   END LOOP;
  END IF;
 END LOOP;
END $$;
-- A compatibility API may attempt a short trial only while the legacy
-- incremental owner is selected. It cannot acquire split authority.
GRANT SELECT,UPDATE(lock_token) ON event_protocol,dispatch_ownership TO infrapro_compat_api;
GRANT SELECT,INSERT,DELETE,UPDATE ON dispatch_leases TO infrapro_compat_api;
DROP POLICY IF EXISTS compat_trial_owner ON dispatch_ownership;
CREATE POLICY compat_trial_owner ON dispatch_ownership TO infrapro_compat_api
 USING (role='monitoring-incremental-worker' AND partition='all' AND mode='compat');
DROP POLICY IF EXISTS compat_trial_lease ON dispatch_leases;
CREATE POLICY compat_trial_lease ON dispatch_leases TO infrapro_compat_api
 USING (role='monitoring-incremental-worker' AND partition='all'
 AND EXISTS(SELECT FROM dispatch_ownership o WHERE o.role=dispatch_leases.role AND o.partition=dispatch_leases.partition
 AND o.restore_epoch=dispatch_leases.restore_epoch AND o.mode='compat'))
 WITH CHECK (role='monitoring-incremental-worker' AND partition='all'
 AND EXISTS(SELECT FROM dispatch_ownership o WHERE o.role=dispatch_leases.role AND o.partition=dispatch_leases.partition
 AND o.generation=dispatch_leases.generation AND o.restore_epoch=dispatch_leases.restore_epoch AND o.mode='compat'));
-- The old monitoring process runs manual parsing. Its login inherits the
-- monitoring groups and receives only the manual lane of parsing dispatch.
GRANT SELECT,UPDATE(lock_token) ON dispatch_ownership TO infrapro_compat_monitoring_worker;
GRANT SELECT,INSERT,UPDATE,DELETE ON dispatch_leases TO infrapro_compat_monitoring_worker;
DROP POLICY IF EXISTS compat_manual_owner ON dispatch_ownership;
CREATE POLICY compat_manual_owner ON dispatch_ownership TO infrapro_compat_monitoring_worker
 USING (role='parsing-dispatch-worker' AND partition='all' AND mode='compat');
DROP POLICY IF EXISTS compat_manual_lease ON dispatch_leases;
CREATE POLICY compat_manual_lease ON dispatch_leases TO infrapro_compat_monitoring_worker
 USING (role='parsing-dispatch-worker' AND partition='all');
-- Restrictive policies also cover permissions inherited from the split roles.
DROP POLICY IF EXISTS compat_workers_owner ON dispatch_ownership;
CREATE POLICY compat_workers_owner ON dispatch_ownership AS RESTRICTIVE
 TO infrapro_compat_ingest_worker,infrapro_compat_monitoring_worker
 USING (mode='compat' AND partition='all');
DROP POLICY IF EXISTS compat_workers_lease ON dispatch_leases;
-- The same running instance may reacquire its own role after two cutovers.
-- Its conflicting old tuple is visible in the current restore epoch. Only a
-- ticket for the currently assigned generation may be inserted or renewed.
CREATE POLICY compat_workers_lease ON dispatch_leases AS RESTRICTIVE
 TO infrapro_compat_ingest_worker,infrapro_compat_monitoring_worker
 USING (partition='all' AND EXISTS(SELECT FROM dispatch_ownership o
   WHERE o.role=dispatch_leases.role AND o.partition=dispatch_leases.partition
   AND o.restore_epoch=dispatch_leases.restore_epoch AND o.mode='compat'))
 WITH CHECK (partition='all' AND EXISTS(SELECT FROM dispatch_ownership o
   WHERE o.role=dispatch_leases.role AND o.partition=dispatch_leases.partition
   AND o.generation=dispatch_leases.generation AND o.restore_epoch=dispatch_leases.restore_epoch AND o.mode='compat'));
ALTER TABLE parsing_executor_slots ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS parser_slots ON parsing_executor_slots;
CREATE POLICY parser_slots ON parsing_executor_slots TO infrapro_parsing_dispatch
 USING (true) WITH CHECK (true);
GRANT SELECT,UPDATE(execution_id,scope,automatic,lease_until) ON parsing_executor_slots TO infrapro_compat_monitoring_worker;
DROP POLICY IF EXISTS compat_manual_slots ON parsing_executor_slots;
CREATE POLICY compat_manual_slots ON parsing_executor_slots TO infrapro_compat_monitoring_worker
 USING ((automatic IS DISTINCT FROM true OR lease_until<=statement_timestamp()) AND EXISTS(
   SELECT FROM dispatch_ownership WHERE role='parsing-dispatch-worker' AND partition='all' AND mode='compat'))
 WITH CHECK (automatic IS DISTINCT FROM true AND EXISTS(
   SELECT FROM dispatch_ownership WHERE role='parsing-dispatch-worker' AND partition='all' AND mode='compat'));
COMMIT;
