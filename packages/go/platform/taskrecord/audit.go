package taskrecord

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

// auditTransitionTx is only called for the owner's current transaction journal.
// Index replay and historical backfill do not create execution facts.
func auditTransitionTx(ctx context.Context, tx pgx.Tx, c Change) error {
	if c.HistoryIncomplete || c.Module == "audit" || c.Kind == "group" {
		return nil
	}
	var exists, closed bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_executions WHERE task_id=$1 AND generation=$2),EXISTS(SELECT 1 FROM task_executions WHERE task_id=$1 AND generation=$2 AND finished_at IS NOT NULL)`, c.SourceRef.ID(), c.Generation).Scan(&exists, &closed)
	if e != nil {
		return e
	}
	phase := ""
	if Terminal(c.State) && !closed {
		phase = c.State
	} else if !exists && Active(c.State) {
		phase = "accepted"
	}
	if phase == "" {
		return nil
	}
	op, e := ar.TransactionOperationTx(ctx, tx)
	if e != nil {
		return e
	}
	scope := ar.Scope{Kind: c.Scope.Kind, Tenant: c.Scope.Tenant, Project: c.Scope.Project}
	f := ar.Fact{Module: c.Module, SourceKey: c.SourceRef.Key() + ":" + c.Generation + ":" + phase, Effect: "execution", Event: c.Module + ".execution." + phase, Scope: scope, ActorType: "worker", ExecutorKind: c.ExecutorKind, ExecutorRef: c.ExecutorRef, Initiator: c.Originator, RequestedBy: c.RequestedBy, ObjectType: c.Type, ObjectID: c.RootID, ObjectRevision: c.Revision, OperationID: op, RequestKind: "worker", Outcome: "success", ErrorCode: c.ErrorCode, OccurredAt: c.ObservedAt, Missing: []string{}, Links: []ar.Link{{Kind: "task_execution", Module: c.Module, TargetKind: c.Kind, TargetID: c.RootID, Generation: c.Generation, Scope: scope, Relation: "execution"}}, Changes: ar.Diff(nil, map[string]string{"state": c.State, "generation": c.Generation}, false, true)}
	if phase == "accepted" && c.RequestedBy != "" {
		f.ActorID = c.RequestedBy
		f.ActorType = "account"
		f.ExecutorKind = ""
		f.ExecutorRef = ""
		f.RequestKind = "command_legacy"
	}
	if phase == "failed" || phase == "needs_action" {
		f.Outcome = "failure"
	}
	if phase == "cancelled" {
		f.Outcome = "cancelled"
	}
	_, e = ar.AppendTx(ctx, tx, f)
	return e
}
