-- +goose Up
-- Notification protocol version is checked by every producer through AppendTx.
CREATE TABLE notification_protocol (id integer PRIMARY KEY CHECK(id=1), version integer NOT NULL, enabled_at timestamptz NOT NULL DEFAULT clock_timestamp());
INSERT INTO notification_protocol VALUES (1,1,clock_timestamp());
CREATE TABLE notification_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), source_module text NOT NULL, source_key text NOT NULL,
 event_type text NOT NULL, schema_version integer NOT NULL CHECK(schema_version=1),
 scope text NOT NULL CHECK(scope IN ('account','platform','tenant','project')),
 tenant_id uuid REFERENCES tenants(id), project_id uuid, actor_id uuid REFERENCES accounts(id),
 target_kind text NOT NULL, target_id text NOT NULL DEFAULT '', outcome text NOT NULL, reason_code text NOT NULL DEFAULT '',
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(), created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 fingerprint text NOT NULL, recipient_count integer NOT NULL CHECK(recipient_count>=0),
 UNIQUE(source_module,source_key), FOREIGN KEY(tenant_id,project_id) REFERENCES projects(tenant_id,id),
 CHECK((scope IN ('account','platform') AND tenant_id IS NULL AND project_id IS NULL) OR (scope='tenant' AND tenant_id IS NOT NULL AND project_id IS NULL) OR (scope='project' AND tenant_id IS NOT NULL AND project_id IS NOT NULL))
);
CREATE TABLE notification_recipients (
 event_id uuid NOT NULL REFERENCES notification_events(id), account_id uuid NOT NULL REFERENCES accounts(id),
 policy text NOT NULL CHECK(policy IN ('personal','scope')), relationship text NOT NULL, expanded_at timestamptz,
 tenant_membership_id uuid REFERENCES tenant_memberships(id), project_membership_id uuid REFERENCES project_memberships(id),
 PRIMARY KEY(event_id,account_id)
);
CREATE INDEX notification_recipients_pending ON notification_recipients(event_id,account_id) WHERE expanded_at IS NULL;
CREATE TABLE notification_deliveries (
 event_id uuid NOT NULL, account_id uuid NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','processing','retry','delivered','suppressed','failed')),
 reason_code text NOT NULL DEFAULT '', attempts integer NOT NULL DEFAULT 0,
 available_at timestamptz NOT NULL DEFAULT clock_timestamp(), lease_until timestamptz, fence bigint NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(event_id,account_id), FOREIGN KEY(event_id,account_id) REFERENCES notification_recipients(event_id,account_id)
);
CREATE INDEX notification_delivery_queue ON notification_deliveries(available_at,event_id,account_id) WHERE state IN ('pending','retry','processing');
CREATE TABLE notification_inbox_states (
 account_id uuid PRIMARY KEY REFERENCES accounts(id), sequence bigint NOT NULL DEFAULT 0, revision bigint NOT NULL DEFAULT 0
);
CREATE TABLE account_notifications (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), account_id uuid NOT NULL REFERENCES accounts(id), event_id uuid NOT NULL,
 sequence bigint NOT NULL CHECK(sequence>0), template_version integer NOT NULL DEFAULT 1,
 delivered_at timestamptz NOT NULL DEFAULT clock_timestamp(), read_at timestamptz, archived_at timestamptz,
 revision bigint NOT NULL DEFAULT 1, UNIQUE(event_id,account_id), UNIQUE(account_id,sequence),
 FOREIGN KEY(event_id,account_id) REFERENCES notification_recipients(event_id,account_id), CHECK(archived_at IS NULL OR read_at IS NOT NULL)
);
CREATE INDEX notification_inbox ON account_notifications(account_id,sequence DESC);
CREATE INDEX notification_unread ON account_notifications(account_id,sequence DESC) WHERE read_at IS NULL AND archived_at IS NULL;
CREATE INDEX notification_event_scope ON notification_events(tenant_id,project_id,event_type);
CREATE TABLE notification_preferences (
 account_id uuid PRIMARY KEY REFERENCES accounts(id), tasks boolean NOT NULL DEFAULT true, updates boolean NOT NULL DEFAULT true, revision bigint NOT NULL DEFAULT 1
);
CREATE TABLE notification_conditions (
 source_module text NOT NULL, condition_key text NOT NULL, cycle bigint NOT NULL DEFAULT 1,
 active boolean NOT NULL DEFAULT true, occurrences bigint NOT NULL DEFAULT 1,
 first_at timestamptz NOT NULL DEFAULT clock_timestamp(), last_at timestamptz NOT NULL DEFAULT clock_timestamp(), recovered_at timestamptz,
 PRIMARY KEY(source_module,condition_key)
);
CREATE TABLE notification_worker_heartbeats (worker_id text PRIMARY KEY, heartbeat_at timestamptz NOT NULL DEFAULT clock_timestamp());
-- Display worker has access to this domain-owned handoff only, not the inbox.
CREATE TABLE display_notification_handoffs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), event_id bigint UNIQUE REFERENCES display_events(id),
 tenant_id uuid NOT NULL, project_id uuid NOT NULL, instance_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('package','management','condition')), target_id uuid NOT NULL,
 outcome text NOT NULL, recipients uuid[] NOT NULL, membership_snapshots jsonb NOT NULL DEFAULT '{}'::jsonb, occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 translated_at timestamptz, attempts integer NOT NULL DEFAULT 0, available_at timestamptz NOT NULL DEFAULT clock_timestamp(), error_code text NOT NULL DEFAULT '',
 FOREIGN KEY(tenant_id,project_id,instance_id) REFERENCES display_instances(tenant_id,project_id,id)
);
CREATE INDEX display_notification_pending ON display_notification_handoffs(occurred_at) WHERE translated_at IS NULL;
ALTER TABLE monitoring_jobs ADD COLUMN notification_generation bigint NOT NULL DEFAULT 1;
ALTER TABLE spatial_processing_jobs ADD COLUMN notification_generation bigint NOT NULL DEFAULT 1;
-- +goose Down
-- Refuse a destructive downgrade: restore a complete coordinated backup instead.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'notifications contain durable personal state; restore a coordinated backup instead of down-migrating'; END $$;
-- +goose StatementEnd
