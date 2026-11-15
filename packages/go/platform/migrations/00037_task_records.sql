-- +goose Up
CREATE TABLE task_protocol (
 id int PRIMARY KEY CHECK(id=1), schema_version int NOT NULL CHECK(schema_version=1),
 mode text NOT NULL CHECK(mode IN ('registering','backfilling','ready','paused')),
 restore_epoch uuid NOT NULL DEFAULT gen_random_uuid(), cursor_secret text NOT NULL DEFAULT (gen_random_uuid()::text||gen_random_uuid()::text),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
INSERT INTO task_protocol(id,schema_version,mode) VALUES(1,1,'backfilling');
CREATE TABLE task_records (
 id uuid PRIMARY KEY, source_module text NOT NULL, source_kind text NOT NULL, source_root_id uuid NOT NULL,
 task_type text NOT NULL, scope_kind text NOT NULL CHECK(scope_kind IN ('tenant','project')),
 tenant_id uuid NOT NULL REFERENCES tenants(id), project_id uuid,
 originator_account_id uuid REFERENCES accounts(id), origin_kind text NOT NULL CHECK(origin_kind IN ('person','delegated','internal','read_subject','legacy_unknown')),
 created_at timestamptz NOT NULL, current_generation bigint NOT NULL CHECK(current_generation>0),
 source_revision bigint NOT NULL CHECK(source_revision>=0), revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 state text NOT NULL, phase text NOT NULL, progress jsonb NOT NULL DEFAULT '{}', result_ref jsonb NOT NULL DEFAULT '{}',
 source_observed_at timestamptz NOT NULL, projected_at timestamptz NOT NULL DEFAULT clock_timestamp(), history_incomplete boolean NOT NULL,
 snapshot jsonb NOT NULL, fingerprint text NOT NULL,
 UNIQUE(source_module,source_kind,source_root_id), FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id),
 CHECK((scope_kind='project')=(project_id IS NOT NULL)), CHECK(origin_kind<>'person' OR originator_account_id IS NOT NULL),
 CHECK(octet_length(snapshot::text)<=32768)
);
CREATE INDEX task_records_person ON task_records(originator_account_id,created_at DESC,id DESC);
CREATE INDEX task_records_scope ON task_records(tenant_id,project_id,created_at DESC,id DESC);
CREATE INDEX task_records_type ON task_records(task_type,state,created_at DESC,id DESC);
CREATE TABLE task_executions (
 task_id uuid NOT NULL REFERENCES task_records(id), generation bigint NOT NULL CHECK(generation>0),
 requested_by uuid REFERENCES accounts(id), executor_kind text NOT NULL, executor_ref text NOT NULL,
 input_ref jsonb NOT NULL, input_hash text NOT NULL, started_at timestamptz, finished_at timestamptz,
 state text NOT NULL, reason_code text NOT NULL DEFAULT '', result_ref jsonb NOT NULL DEFAULT '{}',
 history_incomplete boolean NOT NULL DEFAULT false, source_revision bigint NOT NULL,
 PRIMARY KEY(task_id,generation)
);
CREATE TABLE task_attempts (
 task_id uuid NOT NULL, generation bigint NOT NULL, unit_key text NOT NULL, source_fence bigint NOT NULL CHECK(source_fence>0),
 attempt_no int NOT NULL CHECK(attempt_no>0), started_at timestamptz NOT NULL, finished_at timestamptz,
 outcome text NOT NULL, reason_code text NOT NULL DEFAULT '', diagnostic_ref jsonb NOT NULL DEFAULT '{}',
 PRIMARY KEY(task_id,generation,unit_key,source_fence), FOREIGN KEY(task_id,generation) REFERENCES task_executions(task_id,generation)
);
CREATE TABLE task_relations (
 parent_task_id uuid NOT NULL REFERENCES task_records(id), child_task_id uuid NOT NULL REFERENCES task_records(id),
 relation_kind text NOT NULL CHECK(relation_kind IN ('dependency','member','recreated_from')),
 generation bigint, source_ref jsonb NOT NULL DEFAULT '{}',
 PRIMARY KEY(parent_task_id,child_task_id,relation_kind), CHECK(parent_task_id<>child_task_id)
);
CREATE TABLE task_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), task_id uuid NOT NULL, generation bigint NOT NULL,
 source_revision bigint NOT NULL, event_kind text NOT NULL, actor_id uuid REFERENCES accounts(id),
 occurred_at timestamptz NOT NULL, safe_details jsonb NOT NULL, fingerprint text NOT NULL,
 UNIQUE(task_id,source_revision,event_kind), FOREIGN KEY(task_id,generation) REFERENCES task_executions(task_id,generation),
 CHECK(octet_length(safe_details::text)<=8192)
);
CREATE INDEX task_events_history ON task_events(task_id,generation,occurred_at DESC,id DESC);
CREATE TABLE task_source_states (
 source_module text NOT NULL, source_kind text NOT NULL, backfill_cursor jsonb NOT NULL DEFAULT '{}',
 backfill_complete boolean NOT NULL DEFAULT false, last_reconciled_at timestamptz,
 lease_owner text, lease_until timestamptz, fence bigint NOT NULL DEFAULT 0,
 restore_epoch uuid NOT NULL, last_error_code text NOT NULL DEFAULT '', PRIMARY KEY(source_module,source_kind)
);
INSERT INTO task_source_states(source_module,source_kind,restore_epoch)
 SELECT m,k,restore_epoch FROM task_protocol CROSS JOIN (VALUES ('monitoring','job'),('derived','run'),('derived','export'),('spatial','job'),('display','job'),('derived','group')) AS s(m,k);
CREATE TABLE task_worker_heartbeats (
 worker_id text PRIMARY KEY, heartbeat_at timestamptz NOT NULL, protocol_version int NOT NULL,
 restore_epoch uuid NOT NULL, safe_error_code text NOT NULL DEFAULT ''
);
-- This is an in-transaction change journal, never an execution queue. The common
-- transaction boundary consumes it with taskrecord.ApplyTx before commit.
CREATE TABLE task_transaction_facts (
 id bigserial PRIMARY KEY, transaction_id xid8 NOT NULL DEFAULT pg_current_xact_id(), change jsonb NOT NULL
);
CREATE INDEX task_transaction_facts_tx ON task_transaction_facts(transaction_id,id);
-- +goose StatementBegin
CREATE FUNCTION task_history_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='task_events' OR TG_OP='DELETE' THEN RAISE EXCEPTION 'task history is immutable'; END IF;
 IF TG_TABLE_NAME='task_executions' AND OLD.finished_at IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'closed execution is immutable'; END IF;
 IF TG_TABLE_NAME='task_attempts' AND OLD.finished_at IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'closed attempt is immutable'; END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER task_execution_immutable BEFORE UPDATE OR DELETE ON task_executions FOR EACH ROW EXECUTE FUNCTION task_history_immutable();
CREATE TRIGGER task_attempt_immutable BEFORE UPDATE OR DELETE ON task_attempts FOR EACH ROW EXECUTE FUNCTION task_history_immutable();
CREATE TRIGGER task_event_immutable BEFORE UPDATE OR DELETE ON task_events FOR EACH ROW EXECUTE FUNCTION task_history_immutable();
-- +goose StatementBegin
CREATE FUNCTION task_relation_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(734917);
 IF EXISTS(WITH RECURSIVE links(id) AS (SELECT NEW.child_task_id UNION SELECT child_task_id FROM task_relations JOIN links ON parent_task_id=links.id) SELECT 1 FROM links WHERE id=NEW.parent_task_id) THEN RAISE EXCEPTION 'cyclic task relation'; END IF;
 RETURN NEW;
END; $$;
-- +goose StatementEnd
CREATE TRIGGER task_relation_guard BEFORE INSERT ON task_relations FOR EACH ROW EXECUTE FUNCTION task_relation_guard();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'task history cannot be downgraded'; END $$;
-- +goose StatementEnd
