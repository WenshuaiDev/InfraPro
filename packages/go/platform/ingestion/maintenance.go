package ingestion

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// CleanupOne removes only unattached objects after a full day. Durable receipt,
// receive-intent, attempt and decision references always retain their bytes.
// A bounded worker call cannot publish a result after this grace period.
func (s *Service) CleanupOne(ctx context.Context) (bool, error) {
	if !s.options.Enabled || s.objects == nil {
		return false, nil
	}
	var obj object
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `UPDATE ingestion_objects SET state='deleting' WHERE id=(SELECT o.id FROM ingestion_objects o WHERE o.created_at<clock_timestamp()-interval '24 hours' AND o.state<>'deleted' AND NOT EXISTS(SELECT 1 FROM ingestion_receive_intents x WHERE x.object_id=o.id) AND NOT EXISTS(SELECT 1 FROM ingestion_receipts x WHERE x.object_id=o.id) AND NOT EXISTS(SELECT 1 FROM ingestion_attempts x WHERE x.response_object_id=o.id) AND NOT EXISTS(SELECT 1 FROM ingestion_identity_decisions x WHERE x.response_object_id=o.id) AND NOT EXISTS(SELECT 1 FROM ingestion_demultiplex_decisions x WHERE x.response_object_id=o.id) AND NOT EXISTS(SELECT 1 FROM ingestion_results x WHERE x.response_object_id=o.id) ORDER BY o.created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id::text,object_key,sha256,byte_length`).Scan(&obj.ID, &obj.Key, &obj.Hash, &obj.Size)
	})
	if e == pgx.ErrNoRows {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	if e = s.objects.Delete(ctx, obj.Key); e != nil {
		return true, e
	}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE ingestion_objects SET state='deleted' WHERE id=$1 AND state='deleting'`, obj.ID)
		return e
	})
	return true, e
}
