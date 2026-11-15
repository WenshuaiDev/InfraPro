package auditrecord

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// NotificationTx receives the exact committed producer reference. The local
// context joins effects in this transaction regardless of producer ordering;
// neither timestamps nor names are used as correlation keys.
func NotificationTx(ctx context.Context, tx pgx.Tx, id, module string, scope Scope) error {
	if !ID(id) || !scope.Valid() {
		return ErrInvalid
	}
	op, err := TransactionOperationTx(ctx, tx)
	if err != nil {
		return err
	}
	links, err := notificationLinksTx(ctx, tx)
	if err != nil {
		return err
	}
	l := Link{Kind: "notification_event", Module: module, TargetKind: "notification_event", TargetID: id, Relation: "notification", Scope: scope}
	found := false
	for _, old := range links {
		if old.TargetID == id {
			found = true
		}
	}
	if !found {
		links = append(links, l)
	}
	if len(links) > 32 {
		return ErrInvalid
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('infrapro.audit_notifications',$1,true)`, string(JSON(links))); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM account_security_events WHERE operation_id=$1 AND COALESCE(tenant_id::text,'')=$2 AND COALESCE(project_id::text,'')=$3`, op, scope.Tenant, scope.Project)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, fact := range ids {
		if err = LinkTx(ctx, tx, EventRef{ID: fact, OperationID: op}, []Link{l}); err != nil {
			return err
		}
	}
	return nil
}
func notificationLinksTx(ctx context.Context, tx pgx.Tx) ([]Link, error) {
	var raw string
	e := tx.QueryRow(ctx, `SELECT COALESCE(current_setting('infrapro.audit_notifications',true),'')`).Scan(&raw)
	links := []Link{}
	if e == nil && raw != "" {
		e = json.Unmarshal([]byte(raw), &links)
	}
	return links, e
}
func finishLinksTx(ctx context.Context, tx pgx.Tx, ref EventRef, f Fact) error {
	links, e := notificationLinksTx(ctx, tx)
	if e != nil {
		return e
	}
	for _, l := range links {
		if l.Scope == f.Scope {
			f.Links = append(f.Links, l)
		}
	}
	return LinkTx(ctx, tx, ref, f.Links)
}
