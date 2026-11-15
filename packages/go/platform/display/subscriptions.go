package display

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/jackc/pgx/v5"
	"slices"
	"strconv"
	"strings"
	"time"
)

type SubscriptionInput struct {
	Kind       string          `json:"kind"`
	Query      json.RawMessage `json:"query"`
	Generation string          `json:"generation"`
}
type SubscriptionEvent struct {
	Subscription string    `json:"subscription_id"`
	Generation   string    `json:"generation"`
	Sequence     string    `json:"sequence"`
	Kind         string    `json:"kind"`
	Result       *Envelope `json:"result,omitempty"`
	Code         string    `json:"code,omitempty"`
}

func (s *Service) Subscribe(ctx context.Context, v Session, in SubscriptionInput) (string, error) {
	if !slices.Contains([]string{"latest", "trends", "overview", "statistics", "alerts"}, in.Kind) {

		return "", invalid("/kind", "订阅类型无效。")
	}
	if s.providers[subscriptionMethod(in.Kind)] == nil {
		return "", problem("capability_unavailable", "订阅能力尚未接入。", 409)
	}
	q, e := decodeQuery(in.Query)
	if e != nil {
		return "", e
	}
	if q.Cursor != "" || q.Snapshot != "" || q.Selection != "" {
		return "", invalid("/query", "连续订阅不接受外部快照或游标。")
	}
	if len(in.Generation) > 80 {
		return "", invalid("/generation", "查询代次过长。")
	}
	var id string
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		g, _, e := s.checkSessionTx(ctx, tx, v, false)
		if e != nil {
			return e
		}
		if !slices.Contains(g.Capabilities, subscriptionMethod(in.Kind)) {
			return hidden()
		}
		if _, e = s.resolveTargetsTx(ctx, tx, v, subscriptionMethod(in.Kind), &q); e != nil {
			return e
		}
		var count int
		if _, e = tx.Exec(ctx, `SELECT id FROM display_sessions WHERE id=$1 FOR UPDATE`, v.ID); e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `SELECT count(*) FROM display_subscriptions WHERE session_id=$1 AND state IN ('active','invalid')`, v.ID).Scan(&count)
		if e != nil {
			return e
		}
		if count >= s.options.MaxSubscriptions {
			return problem("budget_exceeded", "会话订阅数达到限制。", 429)
		}
		return tx.QueryRow(ctx, `INSERT INTO display_subscriptions(session_id,query) VALUES($1,$2) RETURNING id::text`, v.ID, jsonBytes(in)).Scan(&id)
	})
	return id, e
}
func (s *Service) CancelSubscription(ctx context.Context, v Session, id string) error {
	if !access.ValidID(id) {
		return hidden()
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `UPDATE display_subscriptions SET state='canceled',generation=generation+1 WHERE id=$1 AND session_id=$2`, id, v.ID)
		if e == nil && tag.RowsAffected() != 1 {
			return hidden()
		}
		return e
	})
}
func (s *Service) Ack(ctx context.Context, v Session, id, generation, sequence string) error {
	if !access.ValidID(id) || !revision(generation) || !revision(sequence) {
		return invalid("/cursor", "消费游标无效。")
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, _, e := s.checkSessionTx(ctx, tx, v, false); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE display_subscriptions SET applied=$4,state=CASE WHEN state='invalid' THEN 'canceled' ELSE state END,updated_at=clock_timestamp() WHERE id=$1 AND session_id=$2 AND generation=$3 AND state IN ('active','invalid') AND $4::bigint<=sequence AND $4::bigint>=applied AND $4::bigint<=applied+1`, id, v.ID, generation, sequence)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return conflict()
		}
		return nil
	})
}

// Poll is a durable at-least-once feed. Source vectors are captured BEFORE the
// first query. If a source advances during that query the next poll reads again.
// Events remain unacknowledged until the application's consumer completes.
func (s *Service) Poll(ctx context.Context, v Session) ([]SubscriptionEvent, error) {
	out := []SubscriptionEvent{}
	type row struct {
		id                            string
		state                         string
		query                         SubscriptionInput
		generation, sequence, applied int64
		vector                        map[string]string
		updated                       time.Time
		pending                       json.RawMessage
	}
	subscriptions := []row{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		current, e := scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, v.ID))
		if e != nil {
			return e
		}
		v = current
		if _, _, e = s.checkSessionTx(ctx, tx, v, false); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT id::text,query,generation,sequence,applied,source_vector,updated_at,pending_query,state FROM display_subscriptions WHERE session_id=$1 AND state IN ('active','invalid') ORDER BY id LIMIT $2`, v.ID, s.options.MaxSubscriptions)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var r row
			if e = rows.Scan(&r.id, &r.query, &r.generation, &r.sequence, &r.applied, &r.vector, &r.updated, &r.pending, &r.state); e != nil {
				return e
			}
			subscriptions = append(subscriptions, r)
		}
		return rows.Err()
	})
	if e != nil {
		return out, e
	}
	for _, sub := range subscriptions {
		// Invalid bindings are a terminal status, not a fresh data generation.
		// Reconstruct that status after event retention until the app acknowledges it.
		if sub.state == "invalid" {
			out = append(out, SubscriptionEvent{Subscription: sub.id, Generation: fmt.Sprint(sub.generation), Sequence: fmt.Sprint(sub.sequence), Kind: "status", Code: "binding_invalid"})
			continue
		}
		if sub.sequence > sub.applied {
			var pending []SubscriptionEvent
			e = s.access.Transact(ctx, func(tx pgx.Tx) error {
				rows, e := tx.Query(ctx, `SELECT event FROM display_subscription_events WHERE subscription_id=$1 AND generation=$2 AND sequence>$3 ORDER BY sequence LIMIT 16`, sub.id, sub.generation, sub.applied)
				if e != nil {
					return e
				}
				defer rows.Close()
				for rows.Next() {
					var event SubscriptionEvent
					if e = rows.Scan(&event); e != nil {
						return e
					}
					pending = append(pending, event)
				}
				return rows.Err()
			})
			if e != nil {
				return out, e
			}
			if len(pending) > 0 && time.Since(sub.updated) < 5*time.Minute {
				out = append(out, pending...)
				continue
			}
			if e = s.resetSubscription(ctx, sub.id, sub.generation); e != nil {
				return out, e
			}
			out = append(out, SubscriptionEvent{Subscription: sub.id, Generation: fmt.Sprint(sub.generation + 1), Sequence: "0", Kind: "reset", Code: "cursor_expired"})
			s.RecordDiagnostic(ctx, v, "resynchronized", "cursor_expired")
			continue
		}
		var vector map[string]string
		q, e := decodeQuery(sub.query.Query)
		if e != nil {
			return out, e
		}
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			if _, e := access.Lock(ctx, tx, false); e != nil {
				return e
			}
			ids, e := s.resolveTargetsTx(ctx, tx, v, subscriptionMethod(sub.query.Kind), &q)
			if e != nil {
				return e
			}
			vector, e = s.sourceVectorTx(ctx, tx, v, ids)
			if e != nil {
				return e
			}
			vector["scope"] = hash(ids)
			return e
		})
		if e != nil {
			if de, ok := e.(*Error); ok && de.Code == "binding_invalid" {
				if e = s.invalidateSubscription(ctx, sub.id, sub.generation, sub.sequence, de.Code); e != nil {
					return out, e
				}
				continue
			}
			return out, e
		}
		if sub.sequence > 0 && hash(vector) == hash(sub.vector) && time.Since(sub.updated) < 5*time.Second {
			continue
		}
		if sub.sequence > 0 && sub.vector["scope"] != vector["scope"] {
			if e = s.resetSubscription(ctx, sub.id, sub.generation); e != nil {
				return out, e
			}
			out = append(out, SubscriptionEvent{Subscription: sub.id, Generation: fmt.Sprint(sub.generation + 1), Sequence: "0", Kind: "reset", Code: "scope_changed"})
			s.RecordDiagnostic(ctx, v, "resynchronized", "scope_changed")
			continue
		}
		params := sub.query.Query
		if len(sub.pending) > 0 {
			params = sub.pending
			vector = sub.vector
		}
		result, e := s.Query(ctx, v, Call{Method: subscriptionMethod(sub.query.Kind), Params: params, Generation: sub.query.Generation})
		if e != nil {
			if de, ok := e.(*Error); ok && de.Code == "binding_invalid" {
				if e = s.invalidateSubscription(ctx, sub.id, sub.generation, sub.sequence, de.Code); e != nil {
					return out, e
				}
				continue
			}
			return out, e
		}
		var pending any
		if result.State == "pending" {
			var projection struct {
				Selection string `json:"selection_id"`
				Snapshot  struct {
					ID string `json:"id"`
				} `json:"snapshot"`
				Filters struct {
					Range      *monitoring.Range `json:"range"`
					AllHistory bool              `json:"all_history"`
					Qualities  []string          `json:"qualities"`
					Timezone   string            `json:"timezone"`
				} `json:"filters"`
			}
			if e = json.Unmarshal(jsonBytes(result.Data), &projection); e != nil {
				return out, e
			}
			resumed, e := decodeQuery(params)
			if e != nil {
				return out, e
			}
			resumed.Selection = projection.Selection
			resumed.Snapshot = projection.Snapshot.ID
			resumed.Range = projection.Filters.Range
			resumed.AllHistory = projection.Filters.AllHistory
			resumed.Qualities = projection.Filters.Qualities
			resumed.Timezone = projection.Filters.Timezone
			resumed.FollowSeconds = 0
			pending = jsonBytes(resumed)
		}
		kind := "update"
		if sub.sequence == 0 {
			kind = "snapshot"
		}
		if result.State == "pending" {
			kind = "pending"
		}
		event := SubscriptionEvent{Subscription: sub.id, Generation: fmt.Sprint(sub.generation), Sequence: fmt.Sprint(sub.sequence + 1), Kind: kind, Result: &result}
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			if _, e := access.Lock(ctx, tx, false); e != nil {
				return e
			}
			current, e := scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, v.ID))
			if e != nil {
				return e
			}
			if _, _, e = s.checkSessionTx(ctx, tx, current, false); e != nil {
				return e
			}
			tag, e := tx.Exec(ctx, `UPDATE display_subscriptions SET sequence=sequence+1,source_vector=$4,pending_query=$5,updated_at=clock_timestamp() WHERE id=$1 AND generation=$2 AND sequence=$3 AND state='active'`, sub.id, sub.generation, sub.sequence, jsonBytes(vector), pending)
			if e != nil {
				return e
			}
			if tag.RowsAffected() == 0 {
				return nil
			}
			_, e = tx.Exec(ctx, `INSERT INTO display_subscription_events(subscription_id,generation,sequence,event) VALUES($1,$2,$3,$4)`, sub.id, sub.generation, sub.sequence+1, jsonBytes(event))
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `DELETE FROM display_subscription_events WHERE subscription_id=$1 AND (generation<>$2 OR sequence<$3-16)`, sub.id, sub.generation, sub.sequence)
			if e == nil {
				out = append(out, event)
			}
			return e
		})
		if e != nil {
			return out, e
		}
	}
	return out, nil
}
func (s *Service) resetSubscription(ctx context.Context, id string, generation int64) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE display_subscriptions SET generation=generation+1,sequence=0,applied=0,source_vector='{}',pending_query=NULL,updated_at=clock_timestamp() WHERE id=$1 AND generation=$2 AND state='active'`, id, generation)
		return e
	})
}
func EventID(e SubscriptionEvent) string {
	return strings.Join([]string{e.Subscription, e.Generation, e.Sequence}, ":")
}
func ParseEventID(s string) bool {
	parts := strings.Split(s, ":")
	if len(parts) != 3 || !access.ValidID(parts[0]) {
		return false
	}
	for _, v := range parts[1:] {
		if _, e := strconv.ParseInt(v, 10, 64); e != nil {
			return false
		}
	}
	return true
}

func (s *Service) invalidateSubscription(ctx context.Context, id string, generation, sequence int64, code string) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		event := SubscriptionEvent{Subscription: id, Generation: fmt.Sprint(generation), Sequence: fmt.Sprint(sequence + 1), Kind: "status", Code: code}
		tag, e := tx.Exec(ctx, `UPDATE display_subscriptions SET state='invalid',sequence=sequence+1,updated_at=clock_timestamp() WHERE id=$1 AND generation=$2 AND sequence=$3 AND state='active'`, id, generation, sequence)
		if e != nil || tag.RowsAffected() == 0 {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO display_subscription_events(subscription_id,generation,sequence,event) VALUES($1,$2,$3,$4)`, id, generation, sequence+1, jsonBytes(event))
		return e
	})
}

func subscriptionMethod(kind string) string {
	if kind == "alerts" {
		return "alerts.subscribe"
	}
	return "monitoring." + kind
}
