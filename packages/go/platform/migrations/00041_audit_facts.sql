-- +goose Up
-- The existing fact table remains the only public audit fact store. Unknown
-- history is deliberately unclassified until a reviewed backfill is applied.
CREATE TABLE audit_protocol (
 id integer PRIMARY KEY CHECK(id=1), version integer NOT NULL CHECK(version=1),
 mode text NOT NULL CHECK(mode IN ('migrating','ready','read_only','reconciling')),
 restore_epoch uuid NOT NULL DEFAULT gen_random_uuid(),
 cursor_secret text NOT NULL DEFAULT (gen_random_uuid()::text || gen_random_uuid()::text),
 projection_version bigint NOT NULL DEFAULT 1 CHECK(projection_version>0),
 cleanup_revision bigint NOT NULL DEFAULT 1 CHECK(cleanup_revision>0)
);
INSERT INTO audit_protocol(id,version,mode) VALUES(1,1,'ready');
CREATE TABLE audit_retention_policies (
 class text NOT NULL CHECK(class IN ('platform','tenant','project')),
 version bigint NOT NULL CHECK(version>0), keep_seconds bigint CHECK(keep_seconds>0),
 effective_at timestamptz NOT NULL DEFAULT clock_timestamp(), source text NOT NULL,
 PRIMARY KEY(class,version)
);
-- Installation must import the ACTUAL old configuration with policy plan/apply.
-- An unknown installation has no deletion capability, not a guessed 90 days.
INSERT INTO audit_retention_policies(class,version,source)
 SELECT c,1,'awaiting_configuration' FROM unnest(ARRAY['platform','tenant','project']) c;
ALTER TABLE account_security_events
 ADD COLUMN schema_version smallint NOT NULL DEFAULT 0 CHECK(schema_version IN (0,1)),
 ADD COLUMN source_module text CHECK(length(source_module) BETWEEN 1 AND 64),
 ADD COLUMN source_key text CHECK(length(source_key) BETWEEN 1 AND 240),
 ADD COLUMN effect_key text CHECK(length(effect_key) BETWEEN 1 AND 80),
 ADD COLUMN fingerprint text CHECK(length(fingerprint)=64),
 ADD COLUMN operation_id uuid,
 ADD COLUMN parent_operation_id uuid,
 ADD COLUMN request_id_kind text NOT NULL DEFAULT 'legacy_unknown' CHECK(request_id_kind IN ('http','cli','worker','command_legacy','legacy_unknown')),
 ADD COLUMN visibility_class text NOT NULL DEFAULT 'legacy_unclassified' CHECK(visibility_class IN ('platform','platform_tenant_notice','tenant','tenant_project_governance','project','legacy_unclassified')),
 ADD COLUMN category text NOT NULL DEFAULT 'security' CHECK(category IN ('security','management','business','task','file','maintenance')),
 ADD COLUMN phase text NOT NULL DEFAULT 'command' CHECK(phase IN ('command','execution','transfer','maintenance')),
 ADD COLUMN occurred_at timestamptz,
 ADD COLUMN time_source text NOT NULL DEFAULT 'legacy_recorded' CHECK(time_source IN ('native','legacy_recorded')),
 ADD COLUMN initiator_id uuid,
 ADD COLUMN requested_by uuid,
 ADD COLUMN executor_kind text NOT NULL DEFAULT '' CHECK(length(executor_kind)<=40),
 ADD COLUMN executor_ref text NOT NULL DEFAULT '' CHECK(length(executor_ref)<=160),
 ADD COLUMN object_revision text NOT NULL DEFAULT '' CHECK(length(object_revision)<=80),
 ADD COLUMN error_code text NOT NULL DEFAULT '' CHECK(length(error_code)<=120),
 ADD COLUMN changes jsonb NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(changes)='array' AND jsonb_array_length(changes)<=128 AND octet_length(changes::text)<=32768),
 ADD COLUMN safe_context jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(safe_context)='object' AND octet_length(safe_context::text)<=8192),
 ADD COLUMN evidence_quality text NOT NULL DEFAULT 'legacy_partial' CHECK(evidence_quality IN ('native','legacy_partial','source_handoff')),
 ADD COLUMN missing_fields text[] NOT NULL DEFAULT ARRAY['before','after','operation_id','task_generation'],
 ADD COLUMN retention_class text,
 ADD COLUMN retention_policy_version bigint,
 ADD COLUMN retain_until timestamptz,
 ADD CONSTRAINT audit_retention_policy FOREIGN KEY(retention_class,retention_policy_version) REFERENCES audit_retention_policies(class,version),
 ADD CONSTRAINT audit_native_shape CHECK(schema_version=0 OR (
   source_module IS NOT NULL AND source_key IS NOT NULL AND effect_key IS NOT NULL AND fingerprint IS NOT NULL AND occurred_at IS NOT NULL
   AND outcome IN ('success','failure','denied','conflict','cancelled','rate_limited','unknown')
   AND ((visibility_class='platform' AND project_id IS NULL)
     OR (visibility_class IN ('platform_tenant_notice','tenant','tenant_project_governance') AND tenant_id IS NOT NULL AND project_id IS NULL)
     OR (visibility_class='project' AND tenant_id IS NOT NULL AND project_id IS NOT NULL))));
CREATE UNIQUE INDEX audit_source_fact ON account_security_events(source_module,source_key,effect_key) WHERE source_key IS NOT NULL;
CREATE INDEX audit_scope_time ON account_security_events(visibility_class,tenant_id,project_id,created_at DESC,id DESC);
CREATE INDEX audit_personnel_time ON account_security_events(created_at DESC,id DESC);
CREATE INDEX audit_personnel_scope_time ON account_security_events(tenant_id,project_id,created_at DESC,id DESC);
CREATE INDEX audit_scope_actor ON account_security_events(tenant_id,project_id,actor_id,created_at DESC,id DESC);
CREATE INDEX audit_scope_object ON account_security_events(tenant_id,project_id,object_type,object_id,created_at DESC,id DESC);
CREATE INDEX audit_operation ON account_security_events(operation_id) WHERE operation_id IS NOT NULL;
CREATE INDEX audit_request ON account_security_events(request_id) WHERE request_id<>'';
CREATE INDEX audit_expiry ON account_security_events(retain_until,id) WHERE retain_until IS NOT NULL;

CREATE TABLE audit_operations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), source_module text NOT NULL CHECK(length(source_module)<=64),
 scope_kind text NOT NULL CHECK(scope_kind IN ('platform','tenant','project')),
 tenant_id uuid, project_id uuid, actor_id uuid, membership_key text NOT NULL DEFAULT '',
 scope_key text NOT NULL, command_id uuid, payload_hash text CHECK(length(payload_hash)=64),
 action text NOT NULL CHECK(length(action)<=160), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 parent_operation_id uuid REFERENCES audit_operations(id),
 CHECK(parent_operation_id IS DISTINCT FROM id),
 CHECK((scope_kind='platform' AND tenant_id IS NULL AND project_id IS NULL AND scope_key='platform')
 OR (scope_kind='tenant' AND tenant_id IS NOT NULL AND project_id IS NULL AND scope_key='tenant:'||tenant_id::text)
 OR (scope_kind='project' AND tenant_id IS NOT NULL AND project_id IS NOT NULL AND scope_key='project:'||project_id::text)),
 FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id)
);
CREATE UNIQUE INDEX audit_command_identity ON audit_operations(COALESCE(actor_id::text,''),membership_key,scope_key,action,command_id) WHERE command_id IS NOT NULL;
CREATE TABLE audit_command_receipts (
 operation_id uuid PRIMARY KEY REFERENCES audit_operations(id),
 safe_response jsonb NOT NULL CHECK(octet_length(safe_response::text)<=16384),
 result_code text NOT NULL, result_ref text NOT NULL DEFAULT '', completed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 tombstoned_at timestamptz
);
CREATE TABLE audit_event_links (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), event_id bigint NOT NULL REFERENCES account_security_events(id),
 link_kind text NOT NULL CHECK(link_kind IN ('business','task_execution','file_revision','domain_file','notification_event','caused_by')),
 source_module text NOT NULL CHECK(length(source_module)<=64), target_kind text NOT NULL CHECK(length(target_kind)<=80),
 target_id text NOT NULL CHECK(length(target_id)<=80), generation bigint CHECK(generation>0), revision text CHECK(length(revision)<=80),
 relation text NOT NULL CHECK(relation IN ('subject','input','output','execution','notification','caused_by','retry_of')),
 scope_kind text NOT NULL CHECK(scope_kind IN ('platform','tenant','project')), tenant_id uuid, project_id uuid,
 link_key text NOT NULL CHECK(length(link_key)=64), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(event_id,link_key), CHECK(link_kind<>'task_execution' OR generation IS NOT NULL),
 CHECK(link_kind<>'file_revision' OR revision IS NOT NULL)
);
CREATE INDEX audit_link_reverse ON audit_event_links(source_module,target_kind,target_id,generation);
CREATE TABLE audit_transfer_receipts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), operation_id uuid NOT NULL,
 file_ref jsonb NOT NULL CHECK(jsonb_typeof(file_ref)='object' AND octet_length(file_ref::text)<=2048),
 source_module text NOT NULL, scope_kind text NOT NULL, tenant_id uuid, project_id uuid,
 actor_id uuid NOT NULL, membership_key text NOT NULL, request_id text NOT NULL,
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(), last_seen_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 closed_at timestamptz, outcome text CHECK(outcome IN ('complete','interrupted','revoked','error','interrupted_unknown','headers_only')),
 sent_bytes bigint CHECK(sent_bytes>=0), epoch uuid NOT NULL,
 CHECK((closed_at IS NULL)=(outcome IS NULL))
);
CREATE INDEX audit_transfer_open ON audit_transfer_receipts(last_seen_at) WHERE closed_at IS NULL;
CREATE TABLE audit_source_states (
 source_module text NOT NULL, source_kind text NOT NULL, backfill_cursor bigint NOT NULL DEFAULT 0,
 complete boolean NOT NULL DEFAULT false, pending_count bigint NOT NULL DEFAULT 0,
 observed_at timestamptz, error_code text NOT NULL DEFAULT '', PRIMARY KEY(source_module,source_kind)
);
CREATE TABLE audit_worker_states (
 id text PRIMARY KEY, heartbeat_at timestamptz NOT NULL, epoch uuid NOT NULL, error_code text NOT NULL DEFAULT ''
);
CREATE TABLE audit_retention_holds (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), object_kind text NOT NULL CHECK(object_kind IN ('all','event','export_object','backup')),
 object_id text NOT NULL, reason text NOT NULL CHECK(char_length(reason) BETWEEN 1 AND 500),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL, released_at timestamptz,
 CHECK(expires_at>created_at)
);
CREATE TABLE audit_cleanup_plans (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), kind text NOT NULL CHECK(kind IN ('facts','objects','policy','backfill')),
 revision bigint NOT NULL DEFAULT 1, epoch uuid NOT NULL, cutoff timestamptz NOT NULL,
 scope jsonb NOT NULL DEFAULT '{}', state text NOT NULL DEFAULT 'planned' CHECK(state IN ('planned','applied','rejected','partial')),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(), applied_at timestamptz, reason text NOT NULL DEFAULT '', summary jsonb NOT NULL DEFAULT '{}'
);
CREATE TABLE audit_cleanup_items (
 plan_id uuid NOT NULL REFERENCES audit_cleanup_plans(id), object_id text NOT NULL,
 policy_version bigint, state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','protected','deleted','failed')),
 PRIMARY KEY(plan_id,object_id)
);
CREATE INDEX audit_cleanup_pending ON audit_cleanup_items(plan_id,state);
CREATE TABLE audit_maintenance_receipts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), request_id uuid NOT NULL UNIQUE, operation text NOT NULL,
 plan_id uuid, reason text NOT NULL CHECK(char_length(reason) BETWEEN 1 AND 500),
 summary jsonb NOT NULL CHECK(octet_length(summary::text)<=8192), created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
-- Accessible only inside the plan-validating SECURITY DEFINER procedure.
CREATE TABLE audit_cleanup_inflight (transaction_id xid8 NOT NULL, event_id bigint NOT NULL, PRIMARY KEY(transaction_id,event_id));
REVOKE ALL ON audit_cleanup_inflight FROM PUBLIC;
-- +goose StatementBegin
CREATE FUNCTION audit_fact_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF EXISTS(SELECT 1 FROM audit_cleanup_inflight WHERE transaction_id=pg_current_xact_id() AND event_id=OLD.id) THEN RETURN OLD; END IF;
  RAISE EXCEPTION 'audit_history_immutable';
 END IF;
 IF OLD.schema_version=1 OR (to_jsonb(OLD)-ARRAY['visibility_class','category','phase','occurred_at','time_source','missing_fields','retention_class','retention_policy_version','retain_until'])
  IS DISTINCT FROM (to_jsonb(NEW)-ARRAY['visibility_class','category','phase','occurred_at','time_source','missing_fields','retention_class','retention_policy_version','retain_until']) THEN
  RAISE EXCEPTION 'audit_history_immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_fact_immutable BEFORE UPDATE OR DELETE ON account_security_events FOR EACH ROW EXECUTE FUNCTION audit_fact_immutable();
CREATE FUNCTION audit_append_only() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit_history_immutable'; END $$;
CREATE TRIGGER audit_operation_immutable BEFORE UPDATE OR DELETE ON audit_operations FOR EACH ROW EXECUTE FUNCTION audit_append_only();
CREATE TRIGGER audit_policy_immutable BEFORE UPDATE OR DELETE ON audit_retention_policies FOR EACH ROW EXECUTE FUNCTION audit_append_only();
CREATE TRIGGER audit_maintenance_immutable BEFORE UPDATE OR DELETE ON audit_maintenance_receipts FOR EACH ROW EXECUTE FUNCTION audit_append_only();
CREATE FUNCTION audit_receipt_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.tombstoned_at IS NOT NULL OR NEW.tombstoned_at IS NULL OR NEW.safe_response<>'{}'::jsonb
  OR (to_jsonb(OLD)-ARRAY['safe_response','tombstoned_at']) IS DISTINCT FROM (to_jsonb(NEW)-ARRAY['safe_response','tombstoned_at']) THEN RAISE EXCEPTION 'audit_receipt_immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_receipt_immutable BEFORE UPDATE OR DELETE ON audit_command_receipts FOR EACH ROW EXECUTE FUNCTION audit_receipt_immutable();
CREATE FUNCTION audit_transfer_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.closed_at IS NOT NULL OR NEW.last_seen_at<OLD.last_seen_at OR (NEW.sent_bytes IS NOT NULL AND NEW.sent_bytes<COALESCE(OLD.sent_bytes,0))
  OR (to_jsonb(OLD)-ARRAY['last_seen_at','closed_at','outcome','sent_bytes']) IS DISTINCT FROM (to_jsonb(NEW)-ARRAY['last_seen_at','closed_at','outcome','sent_bytes']) THEN RAISE EXCEPTION 'audit_transfer_immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_transfer_immutable BEFORE UPDATE OR DELETE ON audit_transfer_receipts FOR EACH ROW EXECUTE FUNCTION audit_transfer_immutable();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Audit facts are irreversible; restore a consistent verified backup'; END $$;
-- +goose StatementEnd
