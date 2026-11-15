package auditrecord

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"strconv"
)

type Transfer struct {
	ID, Operation, Module, Actor, Membership, Request, Epoch string
	Scope                                                    Scope
	File                                                     Link
}

func StartTransferTx(ctx context.Context, tx pgx.Tx, t Transfer, method string) (Transfer, error) {
	if !ID(t.Actor) || !t.Scope.Valid() || (method != "GET" && method != "HEAD") {
		return t, ErrInvalid
	}
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Operation == "" {
		var e error
		t.Operation, e = TransactionOperationTx(ctx, tx)
		if e != nil {
			return t, e
		}
	}
	t.File.Scope = t.Scope
	_, e := tx.Exec(ctx, `INSERT INTO audit_transfer_receipts(id,operation_id,file_ref,source_module,scope_kind,tenant_id,project_id,actor_id,membership_key,request_id,epoch) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, t.ID, t.Operation, JSON(t.File), t.Module, t.Scope.Kind, null(t.Scope.Tenant), null(t.Scope.Project), t.Actor, t.Membership, SafeRequestID(t.Request), t.Epoch)
	if e != nil {
		return t, e
	}
	_, e = AppendTx(ctx, tx, Fact{Module: t.Module, SourceKey: t.ID, Effect: "authorized", Event: t.Module + ".download.authorized", Scope: t.Scope, ActorID: t.Actor, ActorType: "account", OperationID: t.Operation, ObjectType: t.File.TargetKind, ObjectID: t.File.TargetID, Outcome: "success", RequestID: SafeRequestID(t.Request), Context: map[string]any{"transfer_id": t.ID, "method": method}, Links: []Link{t.File}})
	return t, e
}
func FinishTransferTx(ctx context.Context, tx pgx.Tx, t Transfer, outcome string, sent *int64) error {
	var n any
	if sent != nil {
		n = *sent
	}
	tag, e := tx.Exec(ctx, `UPDATE audit_transfer_receipts SET closed_at=clock_timestamp(),last_seen_at=clock_timestamp(),outcome=$2,sent_bytes=$3 WHERE id=$1 AND closed_at IS NULL`, t.ID, outcome, n)
	if e != nil || tag.RowsAffected() == 0 {
		return e
	}
	c := map[string]any{"transfer_id": t.ID, "transfer_outcome": outcome}
	missing := []string{}
	if sent != nil {
		c["sent_bytes"] = strconv.FormatInt(*sent, 10)
	} else {
		missing = append(missing, "sent_bytes")
	}
	result := "failure"
	if outcome == "complete" || outcome == "headers_only" {
		result = "success"
	}
	event := "finished"
	if outcome == "interrupted_unknown" {
		event = "interrupted_unknown"
	}
	_, e = AppendTx(ctx, tx, Fact{Module: t.Module, SourceKey: t.ID, Effect: "closed", Event: t.Module + ".download." + event, Scope: t.Scope, ActorID: t.Actor, ActorType: "account", OperationID: t.Operation, ObjectType: t.File.TargetKind, ObjectID: t.File.TargetID, Outcome: result, RequestID: SafeRequestID(t.Request), Context: c, Missing: missing, Links: []Link{t.File}})
	return e
}
