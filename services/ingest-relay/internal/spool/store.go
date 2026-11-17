package spool

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

//go:embed schema.sql
var schema string

type Store struct {
	Pool    *pgxpool.Pool
	RelayID string
	Budget  int64
}

func JSON(v any) []byte { b, _ := json.Marshal(v); return b }
func (s *Store) Migrate(ctx context.Context) error {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, schema); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO relay_state(id,relay_id,spool_epoch) VALUES(1,$1,$2) ON CONFLICT DO NOTHING`, s.RelayID, uuid.NewString()); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) Ready(ctx context.Context) error {
	var durable bool
	e := s.Pool.QueryRow(ctx, `SELECT current_setting('fsync')='on' AND current_setting('synchronous_commit') IN ('on','remote_apply','remote_write') AND current_setting('transaction_read_only')='off' AND EXISTS(SELECT 1 FROM relay_schema WHERE version=1) AND EXISTS(SELECT 1 FROM relay_state WHERE id=1 AND relay_id=$1)`, s.RelayID).Scan(&durable)
	if e != nil {
		return e
	}
	if !durable {
		return c.Fail("spool_not_durable")
	}
	return nil
}

type Receive struct {
	Source                              r.Source
	Principal, DeliveryID, ConnectionID string
	ConnectionSequence                  int64
	ReceivedAt                          time.Time
	Raw                                 []byte
	Invalid, Truncated                  bool
}

func (s *Store) Receive(ctx context.Context, in Receive) (r.Envelope, bool, error) {
	var out r.Envelope
	if len(in.Raw) < 1 || len(in.Raw) > r.MaxBody || in.Invalid && len(in.Raw) > 8192 {
		return out, false, c.Fail("limit_exceeded")
	}
	if e := s.Ready(ctx); e != nil {
		return out, false, e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return out, false, e
	}
	defer tx.Rollback(ctx)
	// Publication holds the exclusive lock until both binding and revision commit.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(754041,hashtext(current_schema()))`); e != nil {
		return out, false, e
	}
	var published bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM relay_source_revisions r JOIN relay_listener_bindings b USING(ingress_id) WHERE r.ingress_id=$1 AND r.revision=$2 AND r.digest=$3 AND b.active)`, in.Source.IngressID, in.Source.Revision, in.Source.Digest).Scan(&published); e != nil {
		return out, false, e
	}
	if !published {
		return out, false, c.Fail("source_not_applied")
	}
	var epoch string
	var used, invalidUsed int64
	if e = tx.QueryRow(ctx, `SELECT spool_epoch::text,used_bytes,invalid_bytes FROM relay_state WHERE id=1 FOR UPDATE`).Scan(&epoch, &used, &invalidUsed); e != nil {
		return out, false, e
	}
	if in.DeliveryID != "" {
		var raw []byte
		var hash string
		e = tx.QueryRow(ctx, `SELECT envelope,sha256 FROM relay_receipts WHERE ingress_id=$1 AND principal_id=$2 AND producer_delivery_id=$3`, in.Source.IngressID, in.Principal, in.DeliveryID).Scan(&raw, &hash)
		if e == nil {
			if hash != c.Digest(in.Raw) {
				return out, false, c.Fail("idempotency_conflict")
			}
			if e = json.Unmarshal(raw, &out); e != nil {
				return out, false, e
			}
			return out, true, nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return out, false, e
		}
	}
	if _, e = tx.Exec(ctx, `INSERT INTO relay_source_counters(spool_epoch,ingress_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, epoch, in.Source.IngressID); e != nil {
		return out, false, e
	}
	var sequence, sourceUsed, sourceInvalid int64
	if e = tx.QueryRow(ctx, `SELECT sequence,(SELECT sum(used_bytes) FROM relay_source_counters WHERE ingress_id=$2),(SELECT sum(invalid_bytes) FROM relay_source_counters WHERE ingress_id=$2) FROM relay_source_counters WHERE spool_epoch=$1 AND ingress_id=$2 FOR UPDATE`, epoch, in.Source.IngressID).Scan(&sequence, &sourceUsed, &sourceInvalid); e != nil {
		return out, false, e
	}
	var committed time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&committed); e != nil {
		return out, false, e
	}
	if in.ReceivedAt.IsZero() {
		in.ReceivedAt = committed
	}
	out = r.Wrap(in.Source, epoch, uuid.NewString(), in.Principal, sequence+1, in.Raw, in.ReceivedAt, committed)
	out.ProducerDeliveryID = in.DeliveryID
	out.ConnectionID = in.ConnectionID
	out.ConnectionSequence = in.ConnectionSequence
	if in.Invalid {
		out.EvidenceKind = "invalid_fragment"
		out.Truncated = in.Truncated
	}
	out.EnvelopeDigest = r.EnvelopeDigest(out)
	saved := out
	saved.Payload = ""
	accounted := int64(len(in.Raw) + len(JSON(saved)) + 512)
	badSize := int64(0)
	if in.Invalid {
		badSize = accounted
	}
	if used+accounted > s.Budget*9/10 || sourceUsed+accounted > in.Source.Budget*9/10 || in.Invalid && (invalidUsed+badSize > min(s.Budget/100, 16<<20) || sourceInvalid+badSize > min(in.Source.Budget/100, 1<<20)) {
		if _, e = tx.Exec(ctx, `UPDATE relay_state SET rejected_count=rejected_count+1,last_rejected_at=clock_timestamp() WHERE id=1`); e != nil {
			return out, false, e
		}
		if e = tx.Commit(ctx); e != nil {
			return out, false, e
		}
		return out, false, c.Fail("capacity_exceeded")
	}

	var delivery any
	if in.DeliveryID != "" {
		delivery = in.DeliveryID
	}
	if _, e = tx.Exec(ctx, `INSERT INTO relay_receipts(receipt_id,spool_epoch,ingress_id,source_sequence,envelope,envelope_digest,sha256,byte_length,payload,principal_id,producer_delivery_id,received_at,committed_at,accounted_bytes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, out.ReceiptID, epoch, out.IngressID, out.Sequence, JSON(saved), out.EnvelopeDigest, out.SHA256, len(in.Raw), in.Raw, in.Principal, delivery, out.ReceivedAt, out.CommittedAt, accounted); e != nil {
		return out, false, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO relay_deliveries(receipt_id) VALUES($1)`, out.ReceiptID); e != nil {
		return out, false, e
	}
	if _, e = tx.Exec(ctx, `UPDATE relay_source_counters SET sequence=sequence+1,used_bytes=used_bytes+$3,invalid_bytes=invalid_bytes+$4 WHERE spool_epoch=$1 AND ingress_id=$2`, epoch, out.IngressID, accounted, badSize); e != nil {
		return out, false, e
	}
	if _, e = tx.Exec(ctx, `UPDATE relay_state SET used_bytes=used_bytes+$1,invalid_bytes=invalid_bytes+$2 WHERE id=1`, accounted, badSize); e != nil {
		return out, false, e
	}
	return out, false, tx.Commit(ctx)
}
func (s *Store) Pull(ctx context.Context, in r.Pull) (r.Delivery, error) {
	out := r.Delivery{Items: []r.Envelope{}}
	if _, e := uuid.Parse(in.RequestID); e != nil {
		return out, c.Fail("invalid_pull")
	}
	if in.Limit < 1 || in.Limit > 100 || in.MaxBytes < r.MaxBody || in.MaxBytes > r.MaxPullBytes || in.WaitSeconds < 0 || in.WaitSeconds > 25 {
		return out, c.Fail("invalid_pull")
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, `SELECT r.envelope,r.payload,d.attempt FROM relay_deliveries d JOIN relay_receipts r USING(receipt_id) WHERE (d.state='pending' OR d.state='leased' AND d.lease_until<clock_timestamp()) AND d.available_at<=clock_timestamp() ORDER BY d.available_at,r.committed_at FOR UPDATE OF d SKIP LOCKED LIMIT $1`, in.Limit)
	if e != nil {
		return out, e
	}
	total := 0
	for rows.Next() {
		var raw, payload []byte
		var attempt int
		if e = rows.Scan(&raw, &payload, &attempt); e != nil {
			rows.Close()
			return out, e
		}
		var v r.Envelope
		if e = json.Unmarshal(raw, &v); e != nil {
			rows.Close()
			return out, e
		}
		if total+len(payload) > in.MaxBytes {
			break
		}
		total += len(payload)
		v.Payload = base64.StdEncoding.EncodeToString(payload)
		v.DeliveryID = uuid.NewString()
		v.LeaseToken = uuid.NewString()
		v.DeliveryAttempt = attempt + 1
		v.LeaseUntil = time.Now().UTC().Add(r.Lease)
		out.Items = append(out.Items, v)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return out, e
	}
	for _, v := range out.Items {
		if _, e = tx.Exec(ctx, `UPDATE relay_deliveries SET state='leased',delivery_id=$2,attempt=$3,lease_token=$4,lease_until=$5 WHERE receipt_id=$1`, v.ReceiptID, v.DeliveryID, v.DeliveryAttempt, v.LeaseToken, v.LeaseUntil); e != nil {
			return out, e
		}
	}
	return out, tx.Commit(ctx)
}
func (s *Store) Ack(ctx context.Context, in r.Ack) (r.AckResult, error) {
	out := r.AckResult{ReceiptID: in.ReceiptID, State: "rejected"}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	var state, hash, epoch, lease string
	var valid bool
	e = tx.QueryRow(ctx, `SELECT d.state,r.sha256,r.spool_epoch::text,COALESCE(d.lease_token::text,''),COALESCE(d.lease_until>clock_timestamp(),false) FROM relay_receipts r JOIN relay_deliveries d USING(receipt_id) WHERE receipt_id=$1 FOR UPDATE OF d`, in.ReceiptID).Scan(&state, &hash, &epoch, &lease, &valid)
	if e != nil {
		return out, e
	}
	if in.RelayID != s.RelayID || in.Epoch != epoch || in.SHA256 != hash {
		return out, c.Fail("receipt_conflict")
	}
	if state == "acknowledged" {
		var original string
		if e = tx.QueryRow(ctx, `SELECT local_receipt_id::text FROM relay_ack_receipts WHERE receipt_id=$1`, in.ReceiptID).Scan(&original); e != nil {
			return out, e
		}
		if original != in.LocalReceiptID {
			return out, c.Fail("receipt_conflict")
		}
		out.State = "acknowledged"
		return out, nil
	}
	if state != "leased" || !valid || lease != in.LeaseToken {
		return out, c.Fail("lease_expired")
	}
	if _, e = tx.Exec(ctx, `INSERT INTO relay_ack_receipts(receipt_id,local_receipt_id,sha256) VALUES($1,$2,$3)`, in.ReceiptID, in.LocalReceiptID, in.SHA256); e != nil {
		return out, e
	}
	if _, e = tx.Exec(ctx, `UPDATE relay_deliveries SET state='acknowledged',lease_until=NULL WHERE receipt_id=$1`, in.ReceiptID); e != nil {
		return out, e
	}
	out.State = "acknowledged"
	return out, tx.Commit(ctx)
}
func (s *Store) Release(ctx context.Context, in r.Release) error {
	if in.DelaySeconds < 0 || in.DelaySeconds > 300 {
		return c.Fail("invalid_release")
	}
	state := "pending"
	switch in.Reason {
	case "envelope_config_mismatch", "receipt_conflict":
		state = "quarantined"
	case "storage_unavailable", "capacity_exceeded", "processing_delayed":
	default:
		return c.Fail("invalid_release")
	}
	tag, e := s.Pool.Exec(ctx, `UPDATE relay_deliveries d SET state=$4,reason=$5,available_at=clock_timestamp()+$6*interval '1 second',lease_until=NULL FROM relay_receipts r WHERE d.receipt_id=r.receipt_id AND r.receipt_id=$1 AND d.lease_token=$2 AND r.sha256=$3 AND d.state='leased' AND d.lease_until>clock_timestamp() AND r.spool_epoch=$7`, in.ReceiptID, in.LeaseToken, in.SHA256, state, in.Reason, in.DelaySeconds, in.Epoch)
	if e == nil && tag.RowsAffected() != 1 {
		return c.Fail("lease_expired")
	}
	return e
}
func (s *Store) Watermark(ctx context.Context, in r.WatermarkRequest) (r.Watermark, error) {
	out := r.Watermark{WatermarkRequest: in}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT id FROM relay_state WHERE id=1 FOR UPDATE`); e != nil {
		return out, e
	}
	var hash string
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT request_digest,result FROM relay_command_receipts WHERE command_id=$1`, in.CommandID).Scan(&hash, &raw)
	if e == nil {
		if hash != r.Hash(in) {
			return out, c.Fail("command_conflict")
		}
		e = json.Unmarshal(raw, &out)
		return out, e
	}
	if e != pgx.ErrNoRows {
		return out, e
	}
	var epoch string
	if e = tx.QueryRow(ctx, `SELECT spool_epoch::text FROM relay_state WHERE id=1`).Scan(&epoch); e != nil {
		return out, e
	}
	if in.Epoch != "" && in.Epoch != epoch {
		return out, c.Fail("epoch_mismatch")
	}
	out.Epoch = epoch
	e = tx.QueryRow(ctx, `SELECT sequence FROM relay_source_counters WHERE spool_epoch=$1 AND ingress_id=$2`, epoch, in.IngressID).Scan(&out.Sequence)
	if e != nil && e != pgx.ErrNoRows {
		return out, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO relay_command_receipts(command_id,kind,request_digest,result) VALUES($1,'watermark',$2,$3)`, in.CommandID, r.Hash(in), JSON(out)); e != nil {
		return out, e
	}
	return out, tx.Commit(ctx)
}
func (s *Store) Cleanup(ctx context.Context) error {
	if _, e := s.Pool.Exec(ctx, `DELETE FROM relay_connection_events WHERE created_at<clock_timestamp()-interval '7 days' OR id IN (SELECT id FROM relay_connection_events ORDER BY created_at DESC OFFSET 10000)`); e != nil {
		return e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT id FROM relay_state WHERE id=1 FOR UPDATE`); e != nil {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT r.receipt_id::text,r.spool_epoch::text,r.ingress_id::text,r.byte_length,CASE WHEN r.envelope->>'evidence_kind'='invalid_fragment' THEN r.byte_length ELSE 0 END FROM relay_receipts r JOIN relay_ack_receipts a USING(receipt_id) WHERE a.acknowledged_at<clock_timestamp()-interval '7 days' AND r.payload IS NOT NULL FOR UPDATE OF r LIMIT 100`)
	if e != nil {
		return e
	}
	type item struct {
		id, epoch, source string
		size, badSize     int64
	}
	list := []item{}
	for rows.Next() {
		var v item
		if e = rows.Scan(&v.id, &v.epoch, &v.source, &v.size, &v.badSize); e != nil {
			rows.Close()
			return e
		}
		list = append(list, v)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return e
	}
	for _, v := range list {
		if _, e = tx.Exec(ctx, `UPDATE relay_receipts SET payload=NULL,deleted_at=clock_timestamp() WHERE receipt_id=$1`, v.id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE relay_source_counters SET used_bytes=used_bytes-$3,invalid_bytes=invalid_bytes-$4 WHERE spool_epoch=$1 AND ingress_id=$2`, v.epoch, v.source, v.size, v.badSize); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE relay_state SET used_bytes=used_bytes-$1,invalid_bytes=invalid_bytes-$2 WHERE id=1`, v.size, v.badSize); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
