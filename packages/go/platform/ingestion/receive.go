package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"time"
)

type object struct {
	ID   string
	Key  string
	Hash string
	Size int64
}

func objectTx(ctx context.Context, tx pgx.Tx, id string) (object, error) {
	var out object
	e := tx.QueryRow(ctx, `SELECT id::text,object_key,sha256,byte_length FROM ingestion_objects WHERE id=$1 AND state='available'`, id).Scan(&out.ID, &out.Key, &out.Hash, &out.Size)
	return out, e
}
func (s *Service) readObject(ctx context.Context, o object) (content []byte, failure error) {
	defer func() { observeDependency(ctx, "s3", failure) }()
	if s.objects == nil || o.Size < 1 || o.Size > c.MaxResponse {
		return nil, c.Fail("storage_unavailable")
	}
	reader, e := s.objects.Get(ctx, o.Key)
	if e != nil {
		return nil, c.Fail("integrity_failed")
	}
	defer reader.Close()
	b, e := io.ReadAll(io.LimitReader(reader, o.Size+1))
	if e != nil || int64(len(b)) != o.Size || c.Digest(b) != o.Hash {
		return nil, c.Fail("integrity_failed")
	}
	return b, nil
}
func (s *Service) put(ctx context.Context, o object, b []byte) error {
	if s.objects == nil {
		return c.Fail("storage_unavailable")
	}
	if int64(len(b)) != o.Size || c.Digest(b) != o.Hash {
		return c.Fail("integrity_failed")
	}
	// Conditional object creation may report "already exists" after a lost
	// response. Only a full authenticated reread of identical bytes resolves it.
	e := s.objects.Put(ctx, o.Key, bytes.NewReader(b), o.Size, "application/octet-stream")
	raw, check := s.readObject(ctx, o)
	if check != nil || !bytes.Equal(raw, b) {
		if e != nil {
			return c.Fail("storage_unavailable")
		}
		return c.Fail("integrity_failed")
	}
	return nil
}
func (s *Service) reserveTx(ctx context.Context, tx pgx.Tx, b []byte) (object, error) {
	out := object{ID: uuid.NewString(), Hash: c.Digest(b), Size: int64(len(b))}
	out.Key = "ingestion/v1/" + out.ID
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(754042)`); e != nil {
		return out, e
	}
	var used int64
	if e := tx.QueryRow(ctx, `SELECT used_bytes FROM ingestion_storage_state WHERE id=1 FOR UPDATE`).Scan(&used); e != nil {
		return out, e
	}
	if used+out.Size > s.options.StorageBudget*9/10 {
		return out, c.Fail("capacity_exceeded")
	}
	_, e := tx.Exec(ctx, `INSERT INTO ingestion_objects(id,object_key,sha256,byte_length) VALUES($1,$2,$3,$4)`, out.ID, out.Key, out.Hash, out.Size)
	return out, e
}
func (s *Service) saveObject(ctx context.Context, b []byte) (object, error) {
	var out object
	e := s.access.Transact(ctx, func(tx pgx.Tx) error { var e error; out, e = s.reserveTx(ctx, tx, b); return e })
	if e != nil {
		return out, e
	}
	if e = s.put(ctx, out, b); e != nil {
		return out, e
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE ingestion_objects SET state='available' WHERE id=$1`, out.ID)
		return e
	})
	return out, e
}
func (s *Service) Accept(ctx context.Context, envelope r.Envelope) (accepted string, failure error) {
	defer func() {
		if failure == nil {
			return
		}
		reason := code(failure)
		if reason != "receipt_conflict" && reason != "envelope_config_mismatch" {
			return
		}
		_ = s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `INSERT INTO ingestion_integrity_events(relay_id,epoch,receipt_id,source_id,source_revision,error_code,observed_digest) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(relay_id,epoch,receipt_id) DO UPDATE SET occurrences=ingestion_integrity_events.occurrences+1,last_seen_at=clock_timestamp()`, envelope.RelayID, envelope.Epoch, envelope.ReceiptID, envelope.IngressID, envelope.Revision, reason, envelope.EnvelopeDigest)
			return e
		})
	}()

	raw, e := r.Decode(envelope)
	if e != nil {
		return "", e
	}
	cfg, e := s.relay(envelope.RelayID)
	if e != nil || cfg.ID != envelope.RelayID {
		return "", c.Fail("relay_identity_mismatch")
	}
	var localID string
	var obj object
	var original string
	state, errorCode := "stored", ""
	var tenantID, projectID any
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, envelope.RelayID+"/"+envelope.Epoch+"/"+envelope.ReceiptID); e != nil {
			return e
		}
		var saved []byte
		e := tx.QueryRow(ctx, `SELECT id::text,envelope_digest,object_id::text FROM ingestion_receipts WHERE relay_id=$1 AND epoch=$2 AND receipt_id=$3`, envelope.RelayID, envelope.Epoch, envelope.ReceiptID).Scan(&localID, &original, &obj.ID)
		if e == nil {
			if original != envelope.EnvelopeDigest {
				return c.Fail("receipt_conflict")
			}
			obj, e = objectTx(ctx, tx, obj.ID)
			return e
		}
		if e != pgx.ErrNoRows {
			return e
		}
		e = tx.QueryRow(ctx, `SELECT snapshot FROM ingestion_source_revisions WHERE source_id=$1 AND revision=$2`, envelope.IngressID, envelope.Revision).Scan(&saved)
		if e == pgx.ErrNoRows {
			state, errorCode = "quarantined", "source_revision_unknown"
		} else if e != nil {
			return e
		} else {
			var source r.Source
			if e = json.Unmarshal(saved, &source); e != nil {
				return e
			}
			if e = r.Match(envelope, source); e != nil {
				return e
			}
			tenantID, projectID = source.TenantID, source.ProjectID
			var confirmed bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ingestion_source_confirmations WHERE source_id=$1 AND revision=$2 AND digest=$3)`, source.IngressID, source.Revision, source.Digest).Scan(&confirmed); e != nil {
				return e
			}
			if !confirmed {
				state, errorCode = "quarantined", "awaiting_source_confirmation"
			}
		}
		if _, e = tx.Exec(ctx, `INSERT INTO ingestion_epochs(relay_id,epoch,confirmed) SELECT $1,$2,NOT EXISTS(SELECT 1 FROM ingestion_epochs WHERE relay_id=$1) ON CONFLICT DO NOTHING`, envelope.RelayID, envelope.Epoch); e != nil {
			return e
		}
		var epochConfirmed bool
		if e = tx.QueryRow(ctx, `SELECT confirmed FROM ingestion_epochs WHERE relay_id=$1 AND epoch=$2`, envelope.RelayID, envelope.Epoch).Scan(&epochConfirmed); e != nil {
			return e
		}
		if !epochConfirmed {
			state, errorCode = "quarantined", "epoch_unconfirmed"
		}
		if envelope.EvidenceKind != "complete_message" {
			state, errorCode = "quarantined", "invalid_fragment"
		}
		e = tx.QueryRow(ctx, `SELECT i.envelope_digest,o.id::text,o.object_key,o.sha256,o.byte_length FROM ingestion_receive_intents i JOIN ingestion_objects o ON o.id=i.object_id WHERE relay_id=$1 AND epoch=$2 AND receipt_id=$3`, envelope.RelayID, envelope.Epoch, envelope.ReceiptID).Scan(&original, &obj.ID, &obj.Key, &obj.Hash, &obj.Size)
		if e == nil {
			if original != envelope.EnvelopeDigest {
				return c.Fail("receipt_conflict")
			}
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		obj, e = s.reserveTx(ctx, tx, raw)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO ingestion_receive_intents(relay_id,epoch,receipt_id,envelope_digest,object_id) VALUES($1,$2,$3,$4,$5)`, envelope.RelayID, envelope.Epoch, envelope.ReceiptID, envelope.EnvelopeDigest, obj.ID)
		return e
	})
	if e != nil {
		return "", e
	}
	if localID != "" {
		if _, e = s.readObject(ctx, obj); e != nil {
			return "", e
		}
	} else if e = s.put(ctx, obj, raw); e != nil {
		return "", e
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, envelope.RelayID+"/"+envelope.Epoch+"/"+envelope.ReceiptID); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE ingestion_objects SET state='available' WHERE id=$1`, obj.ID); e != nil {
			return e
		}
		saved := envelope
		saved.Payload = ""
		saved.DeliveryID = ""
		saved.LeaseToken = ""
		saved.LeaseUntil = time.Time{}
		saved.DeliveryAttempt = 0
		if localID == "" {
			localID = uuid.NewString()
			_, e := tx.Exec(ctx, `INSERT INTO ingestion_receipts(id,relay_id,epoch,receipt_id,source_id,source_revision,source_sequence,tenant_id,project_id,envelope,envelope_digest,object_id,state,error_code,received_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT(relay_id,epoch,receipt_id) DO NOTHING`, localID, envelope.RelayID, envelope.Epoch, envelope.ReceiptID, envelope.IngressID, envelope.Revision, envelope.Sequence, tenantID, projectID, k.JSON(saved), envelope.EnvelopeDigest, obj.ID, state, errorCode, envelope.ReceivedAt)
			if e != nil {
				return e
			}
		}
		var digest string
		if e := tx.QueryRow(ctx, `SELECT id::text,envelope_digest FROM ingestion_receipts WHERE relay_id=$1 AND epoch=$2 AND receipt_id=$3`, envelope.RelayID, envelope.Epoch, envelope.ReceiptID).Scan(&localID, &digest); e != nil {
			return e
		}
		if digest != envelope.EnvelopeDigest {
			return c.Fail("receipt_conflict")
		}
		ack := r.Ack{RelayID: envelope.RelayID, Epoch: envelope.Epoch, ReceiptID: envelope.ReceiptID, LeaseToken: envelope.LeaseToken, SHA256: envelope.SHA256, LocalReceiptID: localID}
		if _, e := tx.Exec(ctx, `INSERT INTO ingestion_ack_outbox(receipt_id,payload) VALUES($1,$2) ON CONFLICT(receipt_id) DO UPDATE SET payload=EXCLUDED.payload,state='pending',available_at=clock_timestamp()`, localID, k.JSON(ack)); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE ingestion_receive_intents SET state='committed' WHERE relay_id=$1 AND epoch=$2 AND receipt_id=$3`, envelope.RelayID, envelope.Epoch, envelope.ReceiptID)
		return e
	})
	return localID, e
}
func (s *Service) Pull(ctx context.Context, cfg RelayConfig) (bool, error) {
	if !s.options.Enabled || s.objects == nil {
		return false, nil
	}
	var capacity bool
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT (SELECT used_bytes FROM ingestion_storage_state WHERE id=1)+$1<$2 AND (SELECT count(*) FROM ingestion_receipts WHERE state IN ('stored','identifying'))<$3`, r.MaxPullBytes, s.options.StorageBudget*9/10, s.options.QueueBudget).Scan(&capacity)
	})
	if e != nil || !capacity {
		return false, e
	}
	var batch r.Delivery
	e = relayCall(ctx, cfg, false, "/v1/consumers/"+cfg.ConsumerID+"/pull", r.Pull{RequestID: uuid.NewString(), Limit: 100, MaxBytes: r.MaxPullBytes, WaitSeconds: 0}, &batch)
	if e != nil {
		return false, e
	}
	if len(batch.Items) > 100 {
		return false, c.Fail("relay_invalid_response")
	}
	total := int64(0)
	var failure error
	for _, v := range batch.Items {
		total += v.ByteLength
		if total > r.MaxPullBytes || v.RelayID != cfg.ID {
			return false, c.Fail("relay_invalid_response")
		}
		if _, e = s.Accept(ctx, v); e != nil {
			failure = e
			reason := "storage_unavailable"
			switch code(e) {
			case "envelope_config_mismatch", "receipt_conflict":
				reason = code(e)
			case "capacity_exceeded":
				reason = "capacity_exceeded"
			}
			_ = relayCall(ctx, cfg, false, "/v1/consumers/"+cfg.ConsumerID+"/releases", r.Releases{Items: []r.Release{{Ack: r.Ack{RelayID: v.RelayID, Epoch: v.Epoch, ReceiptID: v.ReceiptID, LeaseToken: v.LeaseToken, SHA256: v.SHA256}, Reason: reason, DelaySeconds: 30}}}, nil)
		}
	}
	return len(batch.Items) > 0, failure
}

// A short row lease claims at most 100 outbox entries. Each response is applied
// independently and only if its exact lease payload still matches.
func (s *Service) AckOne(ctx context.Context) (bool, error) {
	type item struct {
		id  string
		raw []byte
		ack r.Ack
	}
	list := []item{}
	var relay string
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `SELECT r.relay_id::text FROM ingestion_ack_outbox o JOIN ingestion_receipts r ON r.id=o.receipt_id WHERE o.state='pending' AND o.available_at<=clock_timestamp() ORDER BY o.available_at LIMIT 1`).Scan(&relay); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT o.receipt_id::text,o.payload FROM ingestion_ack_outbox o JOIN ingestion_receipts r ON r.id=o.receipt_id WHERE r.relay_id=$1 AND o.state='pending' AND o.available_at<=clock_timestamp() ORDER BY o.available_at FOR UPDATE OF o SKIP LOCKED LIMIT 100`, relay)
		if e != nil {
			return e
		}
		for rows.Next() {
			var v item
			if e = rows.Scan(&v.id, &v.raw); e != nil {
				rows.Close()
				return e
			}
			if e = json.Unmarshal(v.raw, &v.ack); e != nil {
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
			if _, e = tx.Exec(ctx, `UPDATE ingestion_ack_outbox SET available_at=clock_timestamp()+interval '15 seconds' WHERE receipt_id=$1`, v.id); e != nil {
				return e
			}
		}
		return nil
	})
	if e == pgx.ErrNoRows {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if len(list) == 0 {
		return false, nil
	}
	cfg, e := s.relay(relay)
	if e != nil {
		return true, e
	}
	req := r.Acks{Items: []r.Ack{}}
	for _, v := range list {
		req.Items = append(req.Items, v.ack)
	}
	var response r.AckResults
	callErr := relayCall(ctx, cfg, false, "/v1/consumers/"+cfg.ConsumerID+"/acks", req, &response)
	replies := map[string]r.AckResult{}
	for _, v := range response.Items {
		if _, ok := replies[v.ReceiptID]; ok {
			return true, c.Fail("relay_invalid_response")
		}
		replies[v.ReceiptID] = v
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		for _, v := range list {
			state, failure := "pending", code(callErr)
			if callErr == nil {
				reply, ok := replies[v.ack.ReceiptID]
				if !ok {
					failure = "relay_invalid_response"
				} else if reply.State == "acknowledged" {
					state = "acknowledged"
					failure = ""
				} else {
					failure = reply.ErrorCode
					if failure == "lease_expired" {
						state = "awaiting_redelivery"
					}
				}
			}
			if _, e := tx.Exec(ctx, `UPDATE ingestion_ack_outbox SET state=$2,attempts=attempts+1,error_code=$3,available_at=clock_timestamp()+interval '5 seconds' WHERE receipt_id=$1 AND payload=$4`, v.id, state, failure, v.raw); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return true, e
	}
	return true, callErr
}
