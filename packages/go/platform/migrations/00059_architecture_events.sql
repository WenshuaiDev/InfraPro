-- +goose Up
-- The session checker needs SELECT ... FOR SHARE. A column-scoped UPDATE grant
-- on these nullable lock markers confers that lock without changing identity,
-- credentials, tenant membership or policy fields.
ALTER TABLE accounts ADD COLUMN architecture_lock boolean;
ALTER TABLE tenants ADD COLUMN architecture_lock boolean;
ALTER TABLE projects ADD COLUMN architecture_lock boolean;
ALTER TABLE authorization_state ADD COLUMN architecture_lock boolean;
ALTER TABLE platform_security_policy ADD COLUMN architecture_lock boolean;
ALTER TABLE account_sessions ADD COLUMN architecture_lock boolean;
ALTER TABLE account_mfa ADD COLUMN architecture_lock boolean;
ALTER TABLE device_tenant_states ADD COLUMN architecture_lock boolean;
-- Runtime terminal/session rechecks lock a display instance without changing
-- its release pointer, revision, audience or configuration.
ALTER TABLE display_instances ADD COLUMN architecture_lock boolean;
CREATE TABLE event_protocol (
 id integer PRIMARY KEY CHECK (id=1), restore_epoch uuid NOT NULL DEFAULT gen_random_uuid(),
 lock_token boolean NOT NULL DEFAULT true CHECK(lock_token), enabled boolean NOT NULL DEFAULT false, frozen boolean NOT NULL DEFAULT false, restore_pending boolean NOT NULL DEFAULT false,
 min_retention_seconds integer NOT NULL DEFAULT 604800 CHECK(min_retention_seconds>0),
 soft_limit bigint NOT NULL DEFAULT 250000, hard_limit bigint NOT NULL DEFAULT 500000,
 CHECK (soft_limit>0 AND soft_limit<hard_limit)
);
INSERT INTO event_protocol(id) VALUES (1);
CREATE TABLE dispatch_ownership (
 lock_token boolean NOT NULL DEFAULT true CHECK(lock_token), role text NOT NULL, partition text NOT NULL DEFAULT 'all',
 mode text NOT NULL CHECK (mode IN ('compat','draining','split','frozen')),
 generation bigint NOT NULL DEFAULT 1, restore_epoch uuid NOT NULL, freeze_position bigint NOT NULL DEFAULT 0,
 changed_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(role,partition)
);
INSERT INTO dispatch_ownership(role,mode,restore_epoch)
 SELECT name,'compat',restore_epoch FROM event_protocol CROSS JOIN unnest(ARRAY[
 'ingest-custody-worker','parsing-dispatch-worker','monitoring-publish-worker','monitoring-incremental-worker',
 'monitoring-history-worker','monitoring-read-worker']) AS entries(name);
CREATE TABLE dispatch_leases (
 role text NOT NULL, partition text NOT NULL, instance uuid NOT NULL, generation bigint NOT NULL,
 restore_epoch uuid NOT NULL, lease_until timestamptz NOT NULL, PRIMARY KEY(role,partition,instance),
 FOREIGN KEY(role,partition) REFERENCES dispatch_ownership(role,partition)
);
CREATE INDEX dispatch_lease_expiry ON dispatch_leases(lease_until);
CREATE TABLE parsing_executor_slots (
 slot smallint PRIMARY KEY CHECK(slot BETWEEN 1 AND 4), execution_id uuid, scope text, automatic boolean, lease_until timestamptz
);
INSERT INTO parsing_executor_slots(slot) SELECT generate_series(1,4);
CREATE TABLE monitoring_change_origins (
 tenant_id uuid PRIMARY KEY REFERENCES tenants(id), prior_position bigint NOT NULL CHECK(prior_position>=0)
);
INSERT INTO monitoring_change_origins SELECT t.tenant_id,COALESCE(min(c.source_position)-1,t.commit_sequence)
 FROM monitoring_tenant_states t LEFT JOIN monitoring_changes c USING(tenant_id) GROUP BY t.tenant_id,t.commit_sequence;
CREATE TABLE monitoring_cache_floors (
 consumer text NOT NULL, tenant_id uuid NOT NULL REFERENCES tenants(id), domain_key uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision>=0), PRIMARY KEY(consumer,tenant_id,domain_key)
);
CREATE TABLE monitoring_scheduler_leases (lane text PRIMARY KEY, next_run timestamptz NOT NULL);
CREATE TABLE event_operations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), action text NOT NULL, operator_name name NOT NULL DEFAULT session_user,
 restore_epoch uuid NOT NULL, reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 300),
 details jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE event_outbox (
 event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(), producer text NOT NULL, type text NOT NULL,
 business_effect_key text NOT NULL, restore_epoch uuid NOT NULL,
 scope text NOT NULL CHECK(scope='platform' OR scope ~ '^[0-9a-f-]{36}$'),
 aggregate_id uuid NOT NULL, generation_or_position bigint NOT NULL CHECK(generation_or_position>=0),
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(), version integer NOT NULL DEFAULT 1,
 cause_id uuid, trace_id uuid,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','leased','sent')),
 available_at timestamptz NOT NULL DEFAULT clock_timestamp(), attempts integer NOT NULL DEFAULT 0,
 lease_until timestamptz, fence bigint NOT NULL DEFAULT 0, last_error text NOT NULL DEFAULT '', published_at timestamptz, broker_stream text, broker_sequence bigint,
 UNIQUE(producer,type,business_effect_key,restore_epoch), UNIQUE(restore_epoch,event_id),
 CHECK(type IN ('ingestion.receipt.stored','ingestion.units.ready','ingestion.plan.ready','monitoring.changed',
                'task.projection.ready','notification.intent.ready','audit.work.ready'))
);
CREATE INDEX event_outbox_pending ON event_outbox(restore_epoch,available_at,event_id) WHERE state='pending';
CREATE INDEX event_outbox_tenant_pressure ON event_outbox(restore_epoch,scope)
 WHERE state IN ('pending','leased') AND scope<>'platform';
CREATE INDEX event_outbox_oldest ON event_outbox(restore_epoch,
 (CASE WHEN split_part(type,'.',1) IN ('monitoring','ingestion') THEN split_part(type,'.',1) ELSE 'auxiliary' END),occurred_at)
 WHERE state IN ('pending','leased');
CREATE INDEX event_outbox_expired ON event_outbox(restore_epoch,lease_until,event_id) WHERE state='leased';
CREATE INDEX event_outbox_sent ON event_outbox(restore_epoch,published_at,event_id) WHERE state='sent';
CREATE INDEX event_outbox_puback ON event_outbox(restore_epoch,broker_stream,broker_sequence) WHERE broker_sequence IS NOT NULL;
CREATE INDEX event_outbox_position ON event_outbox(restore_epoch,scope,generation_or_position) WHERE type='monitoring.changed';

-- A bounded, indexed per-destination repair path. It is created with the fact so
-- a broker loss, an advisory loss, or a late PG commit cannot hide an event.
CREATE TABLE event_work (
 consumer text NOT NULL, restore_epoch uuid NOT NULL, event_id uuid NOT NULL,
 state text NOT NULL DEFAULT 'awaiting' CHECK(state IN ('awaiting','ready','running','done','quarantined')),
 available_at timestamptz NOT NULL DEFAULT clock_timestamp(), lease_until timestamptz,
 fence bigint NOT NULL DEFAULT 0, attempts integer NOT NULL DEFAULT 0,
 PRIMARY KEY(consumer,restore_epoch,event_id),
 FOREIGN KEY(restore_epoch,event_id) REFERENCES event_outbox(restore_epoch,event_id) ON DELETE CASCADE
);
CREATE INDEX event_work_pending ON event_work(consumer,restore_epoch,state,available_at,event_id) WHERE state IN ('awaiting','ready');
CREATE INDEX event_work_expired ON event_work(consumer,restore_epoch,lease_until,event_id) WHERE state='running';
CREATE TABLE event_consumptions (
 consumer text NOT NULL, restore_epoch uuid NOT NULL, event_id uuid NOT NULL, accepted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 via text NOT NULL CHECK(via IN ('broker','pg')), duplicates bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(consumer,restore_epoch,event_id),
 FOREIGN KEY(consumer,restore_epoch,event_id) REFERENCES event_work(consumer,restore_epoch,event_id) ON DELETE CASCADE
);
CREATE TABLE event_consumer_positions (
 consumer text NOT NULL, restore_epoch uuid NOT NULL, scope uuid NOT NULL, position bigint NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY(consumer,restore_epoch,scope)
);
CREATE TABLE event_quarantine (
 consumer text NOT NULL, restore_epoch uuid NOT NULL, event_id uuid NOT NULL, aggregate_id uuid,
 reason text NOT NULL CHECK(reason IN ('unsupported_version','bad_message','bad_reference','work_exhausted','position_gap','delivery_exhausted')),
 attempts bigint NOT NULL, state text NOT NULL DEFAULT 'open' CHECK(state IN ('open','redriven','resolved')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(consumer,restore_epoch,event_id)
);
CREATE INDEX event_quarantine_open ON event_quarantine(restore_epoch,consumer,updated_at) WHERE state='open';
CREATE TABLE event_destinations(type text NOT NULL, consumer text NOT NULL PRIMARY KEY);
INSERT INTO event_destinations VALUES
 ('ingestion.receipt.stored','identify'),('ingestion.units.ready','plan'),('ingestion.plan.ready','parse'),
 ('monitoring.changed','summary'),('monitoring.changed','derived'),('monitoring.changed','query-cache'),('monitoring.changed','display-cache'),
 ('task.projection.ready','task-index'),('notification.intent.ready','notification'),('audit.work.ready','audit');
-- The stream budget only throttles ingress of NEW business work. Once accepted,
-- recovery, authorization, acknowledgements and ordinary UI reads keep running.
CREATE TABLE event_pressure (
 family text PRIMARY KEY, pending bigint NOT NULL DEFAULT 0, oldest_at timestamptz,
 broker_bytes bigint NOT NULL DEFAULT 0,broker_limit_bytes bigint NOT NULL DEFAULT 0,
 storage_level text NOT NULL DEFAULT 'normal' CHECK(storage_level IN ('normal','soft','hard')),
 broker_sampled_at timestamptz,
 sampled_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
INSERT INTO event_pressure(family) VALUES ('ingestion'),('monitoring'),('auxiliary');
-- Eligibility may be read during the candidate scan. Only the chosen formal
-- job reserves the shared clock, in the same transaction as its lease.
-- +goose StatementBegin
CREATE FUNCTION event_publication_permit(reserve boolean) RETURNS boolean
 LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
DECLARE pressure text;
BEGIN
 SELECT CASE WHEN NOT p.enabled THEN 'normal'
   WHEN p.frozen OR e.pending>=p.hard_limit OR e.storage_level='hard' THEN 'hard'
   WHEN e.pending>=p.soft_limit OR e.storage_level='soft' THEN 'soft'
   ELSE 'normal' END INTO STRICT pressure
 FROM event_protocol p JOIN event_pressure e ON e.family='monitoring' WHERE p.id=1;
 IF pressure='normal' THEN RETURN true; END IF;
 IF pressure='hard' THEN RETURN false; END IF;
 IF NOT reserve THEN
  RETURN NOT EXISTS (SELECT FROM monitoring_scheduler_leases
    WHERE lane='event-formal-publication' AND next_run>statement_timestamp());
 END IF;
 INSERT INTO monitoring_scheduler_leases(lane,next_run)
 VALUES ('event-formal-publication',clock_timestamp()+interval '100 milliseconds')
 ON CONFLICT(lane) DO UPDATE SET next_run=excluded.next_run
 WHERE monitoring_scheduler_leases.next_run<=statement_timestamp();
 RETURN FOUND;
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION event_publication_permit(boolean) FROM PUBLIC;

-- Collate changes per committing transaction. Acquiring the three counter
-- locks only at commit and in family order avoids adding a global lock between
-- domain locks. Rejection rolls back both the fact and its external ack intent.
CREATE TABLE event_pressure_deltas (
 transaction_id xid8 NOT NULL, family text NOT NULL, delta bigint NOT NULL, admissions bigint NOT NULL,
 PRIMARY KEY(transaction_id,family)
);
-- +goose StatementBegin
CREATE FUNCTION event_budget_delta() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
DECLARE family_name text; amount int; admissions int:=0;
BEGIN
 IF TG_OP='INSERT' THEN
  amount:=1;
  IF NEW.type IN ('ingestion.receipt.stored','monitoring.changed') THEN admissions:=1; END IF;
 ELSIF NEW.state='sent' AND OLD.state<>'sent' THEN amount:=-1;
 ELSE RETURN NEW;
 END IF;
 family_name:=split_part(NEW.type,'.',1);
 IF family_name NOT IN ('monitoring','ingestion') THEN family_name:='auxiliary'; END IF;
 INSERT INTO event_pressure_deltas VALUES(pg_current_xact_id(),family_name,amount,admissions)
 ON CONFLICT(transaction_id,family) DO UPDATE SET delta=event_pressure_deltas.delta+EXCLUDED.delta,
 admissions=event_pressure_deltas.admissions+EXCLUDED.admissions;
 RETURN NEW;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION event_apply_budget() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
DECLARE entry record; n bigint; cap bigint; paused boolean;
BEGIN
 SELECT hard_limit,frozen INTO cap,paused FROM event_protocol WHERE id=1;
 FOR entry IN WITH batch AS(DELETE FROM event_pressure_deltas WHERE transaction_id=NEW.transaction_id RETURNING *)
 SELECT * FROM batch ORDER BY family LOOP
  UPDATE event_pressure SET pending=pending+entry.delta WHERE family=entry.family RETURNING pending INTO n;
  IF n>cap AND entry.admissions>0 AND NOT paused THEN RAISE EXCEPTION 'event_outbox_backpressure' USING ERRCODE='53300'; END IF;
 END LOOP;
 RETURN NULL;
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION event_budget_delta(),event_apply_budget() FROM PUBLIC;
CREATE TRIGGER event_budget_write AFTER INSERT OR UPDATE OF state ON event_outbox FOR EACH ROW EXECUTE FUNCTION event_budget_delta();
CREATE CONSTRAINT TRIGGER event_budget_commit AFTER INSERT ON event_pressure_deltas
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION event_apply_budget();
-- +goose StatementBegin
CREATE FUNCTION event_capture_fact() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
DECLARE
 epoch uuid; recording boolean; hard bigint;
 name text; who text; domain_scope uuid; item uuid; seq bigint; fact uuid; trace uuid;
BEGIN
 SELECT enabled,restore_epoch,hard_limit INTO recording,epoch,hard FROM event_protocol WHERE id=1 FOR SHARE;
 IF NOT recording THEN RETURN NEW; END IF;
 CASE TG_TABLE_NAME
 WHEN 'ingestion_receipts' THEN
   name:='ingestion.receipt.stored';who:='ingest-custody-worker';item:=NEW.id;domain_scope:=NEW.tenant_id;seq:=NEW.current_generation;
 WHEN 'ingestion_units' THEN
   name:='ingestion.units.ready';who:='parsing-dispatch-worker';item:=NEW.id;
   SELECT r.tenant_id,d.generation INTO STRICT domain_scope,seq FROM ingestion_receipts r
     JOIN ingestion_identity_decisions d ON d.receipt_id=r.id WHERE d.id=NEW.decision_id AND r.id=NEW.receipt_id;
 WHEN 'ingestion_parse_plans' THEN
   name:='ingestion.plan.ready';who:='parsing-dispatch-worker';item:=NEW.id;domain_scope:=NEW.tenant_id;
   SELECT generation INTO STRICT seq FROM ingestion_identity_decisions WHERE id=NEW.decision_id;
 WHEN 'monitoring_changes' THEN
   name:='monitoring.changed';who:='monitoring';item:=NEW.id;domain_scope:=NEW.tenant_id;seq:=NEW.source_position;
 WHEN 'task_events' THEN
   name:='task.projection.ready';who:='taskrecord';item:=NEW.id;seq:=NEW.source_revision;
   SELECT tenant_id INTO STRICT domain_scope FROM task_records WHERE id=NEW.task_id;
 WHEN 'notification_events' THEN
   name:='notification.intent.ready';who:='notificationevent';item:=NEW.id;domain_scope:=NEW.tenant_id;seq:=1;
 WHEN 'audit_export_generations' THEN
   name:='audit.work.ready';who:='audit';item:=NEW.export_id;seq:=NEW.generation;
   SELECT tenant_id INTO STRICT domain_scope FROM audit_exports WHERE id=NEW.export_id;
 ELSE RAISE EXCEPTION 'unsupported source';
 END CASE;
 IF TG_TABLE_NAME IN ('ingestion_receipts','monitoring_changes') AND EXISTS(
   SELECT 1 FROM event_pressure WHERE family=split_part(name,'.',1) AND (pending>=hard OR storage_level='hard')) THEN
   RAISE EXCEPTION 'event_outbox_backpressure' USING ERRCODE='53300';
 END IF;
 IF current_setting('infrapro.monitoring_trace_id',true) ~ '^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$' THEN
   trace:=current_setting('infrapro.monitoring_trace_id')::uuid;
 END IF;
 INSERT INTO event_outbox(producer,type,business_effect_key,restore_epoch,scope,aggregate_id,generation_or_position,trace_id)
   VALUES(who,name,item::text||'/'||seq,epoch,COALESCE(domain_scope::text,'platform'),item,seq,trace)
   ON CONFLICT(producer,type,business_effect_key,restore_epoch) DO NOTHING RETURNING event_id INTO fact;
 IF fact IS NOT NULL THEN
   INSERT INTO event_work(consumer,restore_epoch,event_id) SELECT consumer,epoch,fact FROM event_destinations WHERE type=name;
   PERFORM pg_notify('infrapro_events_'||md5(TG_TABLE_SCHEMA),'');
 END IF;
 RETURN NEW;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION event_capture_fact() FROM PUBLIC;
CREATE TRIGGER architecture_capture AFTER INSERT ON ingestion_receipts FOR EACH ROW EXECUTE FUNCTION event_capture_fact();
CREATE TRIGGER architecture_capture AFTER INSERT ON ingestion_units FOR EACH ROW EXECUTE FUNCTION event_capture_fact();
CREATE TRIGGER architecture_capture AFTER INSERT ON ingestion_parse_plans FOR EACH ROW EXECUTE FUNCTION event_capture_fact();
CREATE TRIGGER architecture_capture AFTER INSERT ON monitoring_changes FOR EACH ROW EXECUTE FUNCTION event_capture_fact();
CREATE TRIGGER architecture_capture AFTER INSERT ON task_events FOR EACH ROW WHEN (NEW.event_kind='snapshot') EXECUTE FUNCTION event_capture_fact();
CREATE TRIGGER architecture_capture AFTER INSERT ON notification_events FOR EACH ROW EXECUTE FUNCTION event_capture_fact();
CREATE TRIGGER architecture_capture AFTER INSERT ON audit_export_generations FOR EACH ROW EXECUTE FUNCTION event_capture_fact();


-- +goose StatementBegin
CREATE FUNCTION event_restore_guard() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
BEGIN
 UPDATE event_protocol SET frozen=true,restore_pending=true WHERE id=1 AND enabled;
 RETURN NEW;
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION event_restore_guard() FROM PUBLIC;
CREATE TRIGGER event_restore_guard AFTER UPDATE OF restore_epoch ON audit_protocol
 FOR EACH ROW WHEN(OLD.restore_epoch IS DISTINCT FROM NEW.restore_epoch) EXECUTE FUNCTION event_restore_guard();

-- +goose StatementBegin
CREATE FUNCTION event_prune(retention_seconds bigint,batch_size integer) RETURNS bigint
 LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $$
DECLARE removed bigint;
BEGIN
 IF retention_seconds<1 OR batch_size NOT BETWEEN 1 AND 1024 THEN RAISE EXCEPTION 'invalid event cleanup bounds'; END IF;
 WITH eligible AS (SELECT o.event_id FROM event_outbox o JOIN event_protocol p USING(restore_epoch)
 WHERE o.state='sent' AND NOT p.frozen AND o.published_at<clock_timestamp()-GREATEST(p.min_retention_seconds,retention_seconds)*interval '1 second'
 AND NOT EXISTS(SELECT 1 FROM event_work w WHERE (w.event_id,w.restore_epoch)=(o.event_id,o.restore_epoch) AND w.state<>'done')
 AND NOT EXISTS(SELECT 1 FROM event_quarantine q WHERE (q.event_id,q.restore_epoch)=(o.event_id,o.restore_epoch) AND q.state='open')
 AND CASE o.type
  WHEN 'ingestion.receipt.stored' THEN EXISTS(SELECT 1 FROM ingestion_receipts r WHERE r.id=o.aggregate_id)
  WHEN 'ingestion.units.ready' THEN EXISTS(SELECT 1 FROM ingestion_units u WHERE u.id=o.aggregate_id)
  WHEN 'ingestion.plan.ready' THEN EXISTS(SELECT 1 FROM ingestion_parse_plans p WHERE p.id=o.aggregate_id)
  WHEN 'monitoring.changed' THEN EXISTS(SELECT 1 FROM monitoring_changes c WHERE c.id=o.aggregate_id AND c.source_position=o.generation_or_position)
  WHEN 'task.projection.ready' THEN EXISTS(SELECT 1 FROM task_events t WHERE t.id=o.aggregate_id AND t.source_revision=o.generation_or_position)
  WHEN 'notification.intent.ready' THEN EXISTS(SELECT 1 FROM notification_events n WHERE n.id=o.aggregate_id)
  WHEN 'audit.work.ready' THEN EXISTS(SELECT 1 FROM audit_export_generations a WHERE a.export_id=o.aggregate_id AND a.generation=o.generation_or_position)
  ELSE false END
 AND (o.type<>'monitoring.changed' OR NOT EXISTS(SELECT 1 FROM event_destinations d WHERE d.type=o.type AND NOT EXISTS (
 SELECT 1 FROM event_consumer_positions cp WHERE cp.consumer=d.consumer AND cp.restore_epoch=o.restore_epoch AND cp.scope::text=o.scope AND cp.position>=o.generation_or_position)))
 ORDER BY o.published_at,o.event_id LIMIT batch_size FOR UPDATE OF o SKIP LOCKED)
 DELETE FROM event_outbox o USING eligible WHERE o.event_id=eligible.event_id;
 GET DIAGNOSTICS removed=ROW_COUNT;RETURN removed;
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION event_prune(bigint,integer) FROM PUBLIC;
-- Schema names come from the migration connection. Temporary objects must not
-- shadow the definer functions' tables or builtins.
-- +goose StatementBegin
DO $$
DECLARE sig text;
BEGIN
 FOREACH sig IN ARRAY ARRAY['event_capture_fact()','event_budget_delta()','event_apply_budget()','event_restore_guard()','event_prune(bigint,integer)','event_publication_permit(boolean)'] LOOP
  EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO pg_catalog,%I,pg_temp',current_schema(),sig,current_schema());
 END LOOP;
END $$;
-- +goose StatementEnd

CREATE VIEW event_status AS
 SELECT d.consumer,d.type,p.restore_epoch,
 (SELECT count(*) FROM event_work w WHERE w.consumer=d.consumer AND w.restore_epoch=p.restore_epoch AND w.state IN ('awaiting','ready','running')) AS pending_count,
 (SELECT COALESCE(extract(epoch FROM clock_timestamp()-min(w.available_at)),0)::bigint FROM event_work w WHERE w.consumer=d.consumer AND w.restore_epoch=p.restore_epoch AND w.state IN ('awaiting','ready')) AS oldest_seconds,
 (SELECT count(*) FROM event_quarantine q WHERE q.consumer=d.consumer AND q.restore_epoch=p.restore_epoch AND q.state='open') AS quarantined,
 (SELECT COALESCE(sum(c.duplicates),0)::bigint FROM event_consumptions c WHERE c.consumer=d.consumer AND c.restore_epoch=p.restore_epoch) AS redeliveries
 FROM event_destinations d CROSS JOIN event_protocol p;

-- architecture:fixed-baseline-metadata:begin
-- The original checksum covers encoding/json of the Go fixedBaseline struct,
-- including its private rational number and the order of all input ranges.
-- Reconstruct that encoding before returning the two authorization fields.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION architecture_fixed_baseline_metadata(uuid,uuid,text,text)
RETURNS TABLE(snapshot jsonb,input_ranges jsonb)
LANGUAGE sql STABLE SECURITY DEFINER AS $baseline$
 WITH stored AS MATERIALIZED (
   SELECT b.evidence AS doc,b.evidence_hash,b.reference_key,b.snapshot_id
   FROM derived_fixed_baselines b
   WHERE b.tenant_id=$1 AND b.config_id=$2 AND b.reference_key=$3
 ), ranges AS MATERIALIZED (
   SELECT a.item,a.ord,row_to_json(need) AS encoded
   FROM stored s CROSS JOIN LATERAL jsonb_array_elements(
     CASE WHEN jsonb_typeof(s.doc->'input_ranges')='array' THEN s.doc->'input_ranges' ELSE '[]'::jsonb END
   ) WITH ORDINALITY AS a(item,ord)
   CROSS JOIN LATERAL (SELECT coalesce(a.item->>'channel_id','') AS channel_id,
     a.item->>'from_ns' AS from_ns,a.item->>'to_ns' AS to_ns) AS need
 ), fields AS (
   SELECT s.*,row_to_json(raw_snapshot) AS projected_snapshot,
     CASE WHEN s.doc->'input_ranges'='null'::jsonb THEN 'null'::json
       ELSE array_to_json(ARRAY(SELECT r.encoded FROM ranges r ORDER BY r.ord)) END AS projected_ranges
   FROM stored s CROSS JOIN LATERAL (SELECT
     coalesce(s.doc->'snapshot'->>'id','') AS id,coalesce(s.doc->'snapshot'->>'schema_version','') AS schema_version,
     coalesce(s.doc->'snapshot'->>'source_position','') AS source_position,coalesce(s.doc->'snapshot'->>'route_revision','') AS route_revision,
     coalesce(s.doc->'snapshot'->>'content_hash','') AS content_hash) AS raw_snapshot
 ), numbers AS (
   SELECT d.val,d.signed FROM stored s CROSS JOIN LATERAL (VALUES
     (s.doc->'at_ns',true),(s.doc->'count',false),(s.doc->'excluded_count',false)) AS d(val,signed)
   UNION ALL
   SELECT bound.val,true FROM ranges r CROSS JOIN LATERAL
     (VALUES (r.item->'from_ns'),(r.item->'to_ns')) AS bound(val) WHERE bound.val IS DISTINCT FROM 'null'::jsonb
 ), canonical AS (
   SELECT f.*,left(row_to_json(required)::text,-1) || coalesce(optional.fields,'') || '}' AS body
   FROM fields f CROSS JOIN LATERAL (SELECT
     coalesce(f.doc->>'reference','') AS reference,f.projected_snapshot AS snapshot,f.projected_ranges AS input_ranges,
     coalesce(f.doc->>'numerator','') AS numerator,coalesce(f.doc->>'denominator','') AS denominator,f.doc->>'at_ns' AS at_ns,
     coalesce(f.doc->>'quality','') AS quality,f.doc->>'count' AS count,f.doc->>'excluded_count' AS excluded_count) AS required
   CROSS JOIN LATERAL (
     SELECT string_agg(',' || to_json(k.name)::text || ':' || to_json(f.doc->>k.name)::text,'' ORDER BY k.ord) AS fields
     FROM unnest(ARRAY['observation_id','adoption_id','result_id','generation_id']) WITH ORDINALITY AS k(name,ord)
     WHERE coalesce(f.doc->>k.name,'')<>''
   ) AS optional
 ), checked AS MATERIALIZED (
   SELECT c.*,c.reference_key=(c.doc->>'reference') AND c.snapshot_id::text=(c.doc->'snapshot'->>'id')
     AND c.body::jsonb=c.doc
     AND NOT EXISTS (SELECT FROM numbers n WHERE NOT coalesce(CASE
       WHEN jsonb_typeof(n.val)='string' AND (n.val#>>'{}') ~ '^(0|-?[1-9][0-9]{0,19})$'
       THEN CASE WHEN n.signed THEN (n.val#>>'{}')::numeric BETWEEN -9223372036854775808 AND 9223372036854775807
         ELSE (n.val#>>'{}')::numeric BETWEEN 0 AND 18446744073709551615 END ELSE false END,false))
     AND c.evidence_hash=encode(sha256(convert_to(
       replace(replace(replace(replace(replace(c.body,'&',E'\\u0026'),'<',E'\\u003c'),'>',E'\\u003e'),
         chr(8232),E'\\u2028'),chr(8233),E'\\u2029'),'UTF8')),'hex') AS valid
   FROM canonical c
 )
 SELECT CASE WHEN valid THEN projected_snapshot::jsonb END,CASE WHEN valid THEN projected_ranges::jsonb END
 FROM checked WHERE valid IS NOT TRUE OR $4 IS NULL OR evidence_hash=$4
$baseline$;
-- +goose StatementEnd
-- architecture:fixed-baseline-metadata:acl
-- Exclude a caller's temporary objects and arbitrary search_path entries.
-- +goose StatementBegin
DO $$ BEGIN
 EXECUTE format('ALTER FUNCTION %I.architecture_fixed_baseline_metadata(uuid,uuid,text,text) SET search_path TO pg_catalog,%I,pg_temp',current_schema(),current_schema());
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION architecture_fixed_baseline_metadata(uuid,uuid,text,text) FROM PUBLIC;
-- pg_dump --no-acl contains the schema without its cluster-global groups.
-- The deployment role setup grants the split callers when those groups exist.
-- +goose StatementBegin
DO $$ BEGIN
 IF to_regrole('infrapro_audit_worker') IS NOT NULL THEN
  GRANT EXECUTE ON FUNCTION architecture_fixed_baseline_metadata(uuid,uuid,text,text) TO infrapro_audit_worker;
 END IF;
END $$;
-- +goose StatementEnd
-- architecture:fixed-baseline-metadata:end

-- architecture:notification-invitation-targets:begin
-- The inbox reader may inspect only its caller's actual personal receipts.
-- Invitation management owns the token, acceptance context and close reason.
-- +goose StatementBegin
CREATE FUNCTION architecture_notification_invitation_targets(uuid,uuid,uuid[])
RETURNS TABLE(invitation_id uuid,is_recipient boolean,inviter_id text,inviter_membership_id text,
              effective_status text,membership_status text,accepted_tenant_name text)
LANGUAGE sql VOLATILE SECURITY DEFINER AS $invitation$
 WITH targets AS (
   SELECT DISTINCT e.target_id FROM notification_events e
   JOIN notification_recipients r ON r.event_id=e.id AND r.account_id=$1 AND r.policy='personal'
   WHERE e.id=ANY($3) AND e.source_module='tenancy' AND e.event_type='invitation.changed'
     AND e.scope='tenant' AND e.tenant_id=$2 AND e.target_kind='tenant_invitation'
 ), invitations AS (
   SELECT i.id,i.tenant_id,i.target_account_id,i.inviter_account_id,i.inviter_membership_id,
          i.status,i.accepted_membership_id,i.accepted_context_snapshot,i.target_account_id=$1 AS recipient,
          CASE WHEN i.status='pending' AND i.expires_at<=clock_timestamp() THEN 'expired' ELSE i.status END AS current_status
   FROM targets t JOIN tenant_invitations i ON i.id=CASE
     WHEN t.target_id ~ '^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$' THEN t.target_id::uuid END
   WHERE i.tenant_id=$2 AND (i.target_account_id=$1 OR (i.inviter_account_id=$1 AND i.status='accepted'))
 )
 SELECT i.id,i.recipient,
   CASE WHEN i.recipient AND i.current_status='pending' THEN i.inviter_account_id::text ELSE '' END,
   CASE WHEN i.recipient AND i.current_status='pending' THEN i.inviter_membership_id::text ELSE '' END,
   i.current_status,CASE WHEN i.recipient AND i.status='accepted' THEN coalesce(m.status,'') ELSE '' END,
   CASE WHEN i.recipient AND i.status='accepted' AND m.status='active' AND t.status='active'
             AND jsonb_typeof(i.accepted_context_snapshot)='object'
             AND coalesce(jsonb_typeof(i.accepted_context_snapshot#>'{tenant,name}'),'null') IN ('string','null')
        THEN coalesce(i.accepted_context_snapshot#>>'{tenant,name}','') END
 FROM invitations i JOIN tenants t ON t.id=i.tenant_id
 LEFT JOIN tenant_memberships m ON m.tenant_id=i.tenant_id AND m.id=i.accepted_membership_id AND m.account_id=i.target_account_id
$invitation$;
-- +goose StatementEnd
-- architecture:notification-invitation-targets:acl
-- +goose StatementBegin
DO $$ BEGIN
 EXECUTE format('ALTER FUNCTION %I.architecture_notification_invitation_targets(uuid,uuid,uuid[]) SET search_path TO pg_catalog,%I,pg_temp',current_schema(),current_schema());
END $$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION architecture_notification_invitation_targets(uuid,uuid,uuid[]) FROM PUBLIC;
-- +goose StatementBegin
DO $$ BEGIN
 IF to_regrole('infrapro_control_api') IS NOT NULL THEN
  GRANT EXECUTE ON FUNCTION architecture_notification_invitation_targets(uuid,uuid,uuid[]) TO infrapro_control_api;
 END IF;
END $$;
-- +goose StatementEnd
-- architecture:notification-invitation-targets:end
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'retained event evidence and dispatch ownership cannot be downgraded'; END $$;
-- +goose StatementEnd
