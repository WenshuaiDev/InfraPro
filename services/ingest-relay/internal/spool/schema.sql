CREATE TABLE IF NOT EXISTS relay_schema(version integer PRIMARY KEY CHECK(version=1));
INSERT INTO relay_schema VALUES(1) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS relay_state(
 id integer PRIMARY KEY CHECK(id=1), relay_id uuid NOT NULL, spool_epoch uuid NOT NULL,
 used_bytes bigint NOT NULL DEFAULT 0 CHECK(used_bytes>=0), rejected_count bigint NOT NULL DEFAULT 0,
 last_rejected_at timestamptz, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE IF NOT EXISTS relay_source_revisions(
 ingress_id uuid NOT NULL, revision bigint NOT NULL, digest text NOT NULL, source jsonb NOT NULL,
 PRIMARY KEY(ingress_id,revision)
);
CREATE TABLE IF NOT EXISTS relay_listener_bindings(
 ingress_id uuid PRIMARY KEY, generation bigint NOT NULL, revision bigint NOT NULL,
 active boolean NOT NULL, state text NOT NULL, error_code text NOT NULL DEFAULT '',
 observed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(ingress_id,revision) REFERENCES relay_source_revisions(ingress_id,revision)
);
CREATE TABLE IF NOT EXISTS relay_source_counters(
 spool_epoch uuid NOT NULL, ingress_id uuid NOT NULL, sequence bigint NOT NULL DEFAULT 0,
 used_bytes bigint NOT NULL DEFAULT 0 CHECK(used_bytes>=0),
 PRIMARY KEY(spool_epoch,ingress_id)
);
CREATE TABLE IF NOT EXISTS relay_receipts(
 receipt_id uuid PRIMARY KEY, spool_epoch uuid NOT NULL, ingress_id uuid NOT NULL, source_sequence bigint NOT NULL,
 envelope jsonb NOT NULL, envelope_digest text NOT NULL, sha256 text NOT NULL, byte_length bigint NOT NULL,
 payload bytea, principal_id text NOT NULL, producer_delivery_id text,
 received_at timestamptz NOT NULL, committed_at timestamptz NOT NULL, deleted_at timestamptz,
 UNIQUE(spool_epoch,ingress_id,source_sequence)
);
CREATE UNIQUE INDEX IF NOT EXISTS relay_producer_idempotency ON relay_receipts(ingress_id,principal_id,producer_delivery_id) WHERE producer_delivery_id IS NOT NULL;
CREATE TABLE IF NOT EXISTS relay_deliveries(
 receipt_id uuid PRIMARY KEY REFERENCES relay_receipts(receipt_id), state text NOT NULL DEFAULT 'pending',
 delivery_id uuid, attempt integer NOT NULL DEFAULT 0, lease_token uuid, lease_until timestamptz,
 available_at timestamptz NOT NULL DEFAULT clock_timestamp(), reason text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS relay_pending ON relay_deliveries(state,available_at,lease_until);
CREATE TABLE IF NOT EXISTS relay_ack_receipts(
 receipt_id uuid PRIMARY KEY REFERENCES relay_receipts(receipt_id), local_receipt_id uuid NOT NULL,
 sha256 text NOT NULL, acknowledged_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE IF NOT EXISTS relay_command_receipts(
 command_id uuid PRIMARY KEY, kind text NOT NULL, request_digest text NOT NULL, result jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE IF NOT EXISTS relay_connection_events(
 id uuid PRIMARY KEY, ingress_id uuid NOT NULL, connection_id uuid NOT NULL, code text NOT NULL,
 byte_length bigint NOT NULL DEFAULT 0, created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

ALTER TABLE relay_state ADD COLUMN IF NOT EXISTS invalid_bytes bigint NOT NULL DEFAULT 0 CHECK(invalid_bytes>=0);
ALTER TABLE relay_source_counters ADD COLUMN IF NOT EXISTS invalid_bytes bigint NOT NULL DEFAULT 0 CHECK(invalid_bytes>=0);
CREATE INDEX IF NOT EXISTS relay_connection_event_time ON relay_connection_events(created_at);

-- Count payload plus a bounded envelope/row allowance. Tombstone metadata keeps
-- consuming capacity after seven-day payload cleanup; never delete receipt IDs.
ALTER TABLE relay_receipts ADD COLUMN IF NOT EXISTS accounted_bytes bigint NOT NULL DEFAULT 0;
UPDATE relay_receipts SET accounted_bytes=byte_length+octet_length(envelope::text)+512 WHERE accounted_bytes=0;
UPDATE relay_state SET used_bytes=COALESCE((SELECT sum(accounted_bytes-CASE WHEN payload IS NULL THEN byte_length ELSE 0 END) FROM relay_receipts),0),invalid_bytes=COALESCE((SELECT sum(accounted_bytes-CASE WHEN payload IS NULL THEN byte_length ELSE 0 END) FROM relay_receipts WHERE envelope->>'evidence_kind'='invalid_fragment'),0) WHERE id=1;
UPDATE relay_source_counters c SET used_bytes=COALESCE((SELECT sum(r.accounted_bytes-CASE WHEN r.payload IS NULL THEN r.byte_length ELSE 0 END) FROM relay_receipts r WHERE r.ingress_id=c.ingress_id AND r.spool_epoch=c.spool_epoch),0),invalid_bytes=COALESCE((SELECT sum(r.accounted_bytes-CASE WHEN r.payload IS NULL THEN r.byte_length ELSE 0 END) FROM relay_receipts r WHERE r.ingress_id=c.ingress_id AND r.spool_epoch=c.spool_epoch AND r.envelope->>'evidence_kind'='invalid_fragment'),0);
