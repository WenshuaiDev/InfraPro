package notifications

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

type AuditDeliverySummary struct {
	State      string
	Count      string
	ObservedAt time.Time
	ErrorCode  string
}

// AuditDeliverySummaryTx accepts only already-authorized notification EVENT
// references from the audit adapter. It never joins inboxes or preferences and
// cannot return recipient identities, message bodies, read or archive state.
func AuditDeliverySummaryTx(ctx context.Context, tx pgx.Tx, ids []string) (map[string]AuditDeliverySummary, error) {
	if len(ids) > 100 {
		return nil, fmt.Errorf("notification_reference_limit")
	}
	rows, e := tx.Query(ctx, `SELECT e.id::text,e.recipient_count::text,
 CASE WHEN e.recipient_count=0 THEN 'suppressed'
 WHEN count(d.event_id)<e.recipient_count OR bool_or(d.state='pending') THEN 'pending'
 WHEN bool_or(d.state='processing') THEN 'processing'
 WHEN bool_or(d.state='retry') THEN 'retry'
 WHEN bool_or(d.state='failed') THEN 'failed'
 WHEN bool_and(d.state='suppressed') THEN 'suppressed'
 WHEN bool_or(d.state='suppressed') THEN 'partially_delivered'
 ELSE 'delivered' END,transaction_timestamp()
 FROM notification_events e LEFT JOIN notification_deliveries d ON d.event_id=e.id WHERE e.id=ANY($1::uuid[]) GROUP BY e.id`, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]AuditDeliverySummary{}
	for rows.Next() {
		var id string
		var v AuditDeliverySummary
		if e = rows.Scan(&id, &v.Count, &v.State, &v.ObservedAt); e != nil {
			return nil, e
		}
		if v.State == "failed" {
			v.ErrorCode = "delivery_failed"
		}
		out[id] = v
	}
	return out, rows.Err()
}
