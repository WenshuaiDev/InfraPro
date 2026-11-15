package runtime

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

func bridgeDisplayNotifications(ctx context.Context, a *access.Service) error {
	return a.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		items, e := display.PendingNotificationsTx(ctx, tx)
		if e != nil {
			return e
		}
		for _, h := range items {
			save, e := tx.Begin(ctx)
			if e != nil {
				return e
			}
			e = translateDisplayNotification(ctx, save, h)
			if e != nil {
				if err := save.Rollback(ctx); err != nil {
					return err
				}
				if err := display.FailNotificationTx(ctx, tx, h.ID); err != nil {
					return err
				}
				continue
			}
			if e = save.Commit(ctx); e != nil {
				return e
			}
		}
		return nil
	})
}
func translateDisplayNotification(ctx context.Context, tx pgx.Tx, h display.NotificationHandoff) error {
	if e := display.ValidateNotificationTx(ctx, tx, h); e != nil {
		return e
	}
	typ := "display.changed"
	recipients := ne.Scoped(h.Recipients...)
	if h.Kind == "package" {
		typ = "display.package"
		recipients = ne.Personal(h.Recipients...)
	} else if h.Kind == "condition" {
		typ = "display.condition"
	}
	for i := range recipients {
		if ids := h.MembershipSnapshots[recipients[i].Account]; len(ids) == 2 {
			recipients[i].TenantMembership = ids[0]
			recipients[i].ProjectMembership = ids[1]
		}
	}
	event := ne.Event{TaskReference: h.TaskReference, Module: "display", Key: h.Event + ":" + typ, Type: typ, Scope: ne.BusinessScope(h.Scope.Tenant, h.Scope.Project), Target: ne.Target{Kind: "display", ID: h.Instance}, Outcome: h.Outcome, Recipients: recipients, OccurredAt: h.OccurredAt}

	if e := ne.EmitTx(ctx, tx, event); e != nil {
		return e
	}
	if h.Kind == "condition" {
		// The source already emits only actual state transitions. Preserve every
		// edge even if two bridge workers translate recovery before failure. Project
		// the condition from durable source facts; newer observations win the upsert.
		_, e := tx.Exec(ctx, `WITH edges AS (SELECT count(*) FILTER(WHERE outcome='missing') AS cycles,max(occurred_at) FILTER(WHERE outcome='missing') AS failed,max(occurred_at) FILTER(WHERE outcome='recovered') AS recovered FROM display_notification_handoffs WHERE kind='condition' AND tenant_id=$1 AND instance_id=$2 AND target_id=$3)
 INSERT INTO notification_conditions(source_module,condition_key,cycle,active,occurrences,first_at,last_at,recovered_at)
 SELECT 'display',$4,greatest(cycles,1),COALESCE(failed>recovered,recovered IS NULL),1,COALESCE(failed,recovered),greatest(failed,recovered),recovered FROM edges
 ON CONFLICT(source_module,condition_key) DO UPDATE SET cycle=greatest(notification_conditions.cycle,EXCLUDED.cycle),active=CASE WHEN EXCLUDED.last_at>=notification_conditions.last_at THEN EXCLUDED.active ELSE notification_conditions.active END,first_at=greatest(notification_conditions.first_at,EXCLUDED.first_at),last_at=greatest(notification_conditions.last_at,EXCLUDED.last_at),recovered_at=greatest(notification_conditions.recovered_at,EXCLUDED.recovered_at)`, h.Scope.Tenant, h.Instance, h.Target, h.Scope.Tenant+":"+h.Instance+":"+h.Target)
		if e != nil {
			return e
		}
	}

	return display.CompleteNotificationTx(ctx, tx, h.ID)
}
func ReplayDisplayNotification(ctx context.Context, a *access.Service, id, reason string) error {
	if !access.ValidID(id) {
		return access.ErrNotFound
	}
	if e := access.ValidReason(reason); e != nil {
		return e
	}
	return a.Transact(ctx, func(tx pgx.Tx) error {
		if e := display.ReplayNotificationTx(ctx, tx, id); e != nil {
			return e
		}
		return access.Audit(ctx, tx, "notification.handoff.replayed", "display_notification_handoff", id, "", nil, nil, access.Meta{ActorType: "cli", Reason: reason})
	})
}

// DisplayNotificationDiagnostics reads metadata only; it never exposes package
// contents, account names, tool credentials or runtime sessions.
func DisplayNotificationDiagnostics(ctx context.Context, a *access.Service) (map[string]any, error) {
	out := map[string]any{}
	e := a.Transact(ctx, func(tx pgx.Tx) error {
		var pending, failed, oldest int64
		if e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE attempts<8),count(*) FILTER(WHERE attempts>=8),COALESCE(extract(epoch FROM clock_timestamp()-min(occurred_at))::bigint,0) FROM display_notification_handoffs WHERE translated_at IS NULL`).Scan(&pending, &failed, &oldest); e != nil {
			return e
		}
		out["pending"], out["failed"], out["oldest_age_seconds"] = pending, failed, oldest
		rows, e := tx.Query(ctx, `SELECT id::text,error_code,attempts FROM display_notification_handoffs WHERE translated_at IS NULL AND error_code<>'' ORDER BY occurred_at LIMIT 50`)
		if e != nil {
			return e
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id, code string
			var attempts int
			if e = rows.Scan(&id, &code, &attempts); e != nil {
				return e
			}
			items = append(items, map[string]any{"id": id, "reason_code": code, "attempts": attempts})
		}
		out["failures"] = items
		return rows.Err()
	})
	return out, e
}
