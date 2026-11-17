package display

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func (s *Service) pollReadSubscription(ctx context.Context, v Session, sub subscriptionRow) ([]SubscriptionEvent, error) {
	a := s.ReadAuthority(v)
	method := subscriptionMethod(sub.query.Kind)
	var plan dr.Plan
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		var e error
		plan, e = dr.InspectTx(ctx, tx, a, method, sub.query.Query)
		return e
	})
	if e != nil {
		return nil, e
	}
	vector := plan.Evidence.Vector
	vector["scope"] = dr.Hash(plan.Scopes)
	vector["authority"] = plan.Evidence.Authorization
	vector["safety"] = plan.Evidence.Safety
	release := func() {
		if sub.readQueryID != "" {
			_, _ = s.reads.Read(ctx, a, "queries.release", dr.Bytes(dr.QueryControlRequest{QueryID: sub.readQueryID}), "")
		}
	}
	reset := func(code string) ([]SubscriptionEvent, error) {
		release()
		if e := s.resetSubscription(ctx, sub.id, sub.generation); e != nil {
			return nil, e
		}
		return []SubscriptionEvent{{Subscription: sub.id, Generation: fmt.Sprint(sub.generation + 1), Sequence: "0", Kind: "reset", Code: code}}, nil
	}
	if sub.sequence > 0 && (sub.vector["scope"] != vector["scope"] || sub.vector["authority"] != vector["authority"] || sub.vector["safety"] != vector["safety"]) {
		return reset("scope_changed")
	}
	if sub.sequence > sub.applied {
		if time.Since(sub.updated) >= 5*time.Minute {
			return reset("cursor_expired")
		}
		if sub.readQueryID != "" {
			if _, e = s.reads.Read(ctx, a, "queries.get", dr.Bytes(dr.QueryRequest{QueryID: sub.readQueryID}), sub.query.Generation); e != nil {
				var failure *dr.Error
				if errors.As(e, &failure) && slices.Contains([]string{"reset_required", "query_expired", "aborted", "scope_unavailable", "field_not_granted", "not_found"}, failure.Code) {
					return reset(failure.Code)
				}
				return nil, e
			}
		}
		events := []SubscriptionEvent{}
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			if _, e := access.Lock(ctx, tx, false); e != nil {
				return e
			}
			if e := s.reads.ValidateStoredTx(ctx, tx, a, sub.readQueryID); e != nil {
				return e
			}
			rows, e := tx.Query(ctx, `SELECT event FROM display_subscription_events WHERE subscription_id=$1 AND generation=$2 AND sequence>$3 ORDER BY sequence LIMIT 1`, sub.id, sub.generation, sub.applied)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var event SubscriptionEvent
				if e = rows.Scan(&event); e != nil {
					return e
				}
				if event.Result != nil {
					event.Result.NamedRead = true
				}
				events = append(events, event)
			}
			return rows.Err()
		})
		if e != nil {
			return nil, e
		}
		if len(events) == 0 {
			return reset("cursor_expired")
		}
		return events, nil
	}
	var follow struct {
		Follow int `json:"follow_seconds"`
	}
	_ = json.Unmarshal(sub.query.Query, &follow)
	interval := 5 * time.Second
	if method == "statistics.monitoring" {
		interval = 10 * time.Second
	}
	if slices.Contains([]string{"statistics.resources", "statistics.structure", "statistics.devices", "statistics.deviceCategories"}, method) {
		interval = 30 * time.Second
	}
	pending := len(sub.pending) > 0 && sub.readQueryID != ""
	if !pending && sub.sequence > 0 && (time.Since(sub.updated) < interval || follow.Follow == 0 && hash(sub.vector) == hash(vector)) {
		return nil, nil
	}
	var result Envelope
	if pending {
		result, e = s.readQuery(ctx, v, Call{Method: "queries.get", Params: dr.Bytes(dr.QueryRequest{QueryID: sub.readQueryID}), Generation: sub.query.Generation})
		vector = sub.vector
	} else {
		result, e = s.Query(ctx, v, Call{Method: method, Params: sub.query.Query, Generation: sub.query.Generation})
	}
	if e != nil {
		var failure *dr.Error
		if errors.As(e, &failure) && slices.Contains([]string{"reset_required", "query_expired", "aborted", "scope_unavailable", "field_not_granted", "not_found"}, failure.Code) {
			return reset(failure.Code)
		}
		return nil, e
	}
	if result.Query == nil {
		return nil, problem("query_failed", "订阅查询缺少持久标识。", 409)
	}
	if !pending && sub.readQueryID != "" && sub.readQueryID != result.Query.ID {
		release()
	}
	var nextPending any
	kind := "update"
	if sub.sequence == 0 {
		kind = "snapshot"
	}
	if result.State == "pending" {
		kind = "pending"
		nextPending = dr.Bytes(dr.QueryRequest{QueryID: result.Query.ID})
	}
	event := SubscriptionEvent{Subscription: sub.id, Generation: fmt.Sprint(sub.generation), Sequence: fmt.Sprint(sub.sequence + 1), Kind: kind, Result: &result}
	delivered := false
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if e := s.reads.ValidateStoredTx(ctx, tx, a, result.Query.ID); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE display_subscriptions SET sequence=sequence+1,source_vector=$4,pending_query=$5,read_query_id=$6,updated_at=clock_timestamp() WHERE id=$1 AND generation=$2 AND sequence=$3 AND state='active'`, sub.id, sub.generation, sub.sequence, dr.Bytes(vector), nextPending, result.Query.ID)
		if e != nil || tag.RowsAffected() == 0 {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO display_subscription_events(subscription_id,generation,sequence,event) VALUES($1,$2,$3,$4)`, sub.id, sub.generation, sub.sequence+1, dr.Bytes(event)); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `DELETE FROM display_subscription_events WHERE subscription_id=$1 AND (generation<>$2 OR sequence<$3-16)`, sub.id, sub.generation, sub.sequence)
		delivered = e == nil
		return e
	})
	if e != nil {
		return nil, e
	}
	if !delivered {
		return nil, nil
	}
	return []SubscriptionEvent{event}, nil
}
