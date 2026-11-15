package auditrecord

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// LegacyInput adapts existing producers without creating a second success fact.
// Missing snapshots remain missing; current names are never backdated.
type LegacyInput struct {
	Links                                                                                 []Link
	Initiator, RequestedBy, ExecutorKind, ExecutorRef                                     string
	Event, ObjectType, ObjectID, AccountID                                                string
	Before, After                                                                         any
	ActorID, ActorType, IP, UserAgent, RequestID, Reason, Tenant, Project, Entry, Outcome string
}

func AppendLegacyTx(ctx context.Context, tx pgx.Tx, in LegacyInput) (EventRef, error) {
	d, ok := Lookup(in.Event)
	if !ok {
		return EventRef{}, fmt.Errorf("%w: %s", ErrUnregistered, in.Event)
	}
	scope := Scope{Kind: "platform", Tenant: in.Tenant, Project: in.Project}
	if in.Tenant != "" {
		scope.Kind = "tenant"
	}
	if in.Project != "" {
		scope.Kind = "project"
	}
	visibility, e := Visibility(d, scope, in.Entry)
	if e != nil {
		return EventRef{}, e
	}
	before, e := Snapshot(in.Before, d.Fields)
	if e != nil {
		return EventRef{}, e
	}
	after, e := Snapshot(in.After, d.Fields)
	if e != nil {
		return EventRef{}, e
	}
	operation, e := TransactionOperationTx(ctx, tx)
	if e != nil {
		return EventRef{}, e
	}
	var transaction string
	if e = tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text`).Scan(&transaction); e != nil {
		return EventRef{}, e
	}
	actorType := in.ActorType
	if actorType == "" {
		actorType = "account"
	}
	if actorType == "cli" {
		actorType = "maintenance"
	}
	f := Fact{Module: d.Module, SourceKey: transaction + ":" + in.Event + ":" + in.ObjectID, Effect: "change", Event: in.Event, Scope: scope, Visibility: visibility, Outcome: in.Outcome, ActorID: in.ActorID, ActorType: actorType, AccountID: in.AccountID, ObjectType: in.ObjectType, ObjectID: in.ObjectID, OperationID: operation, RequestID: SafeRequestID(in.RequestID), RequestKind: "legacy_unknown", Reason: in.Reason, IP: in.IP, UserAgent: in.UserAgent, Changes: Diff(before, after, in.Before != nil, in.After != nil), Details: map[string]any{"before": before, "after": after}, Context: map[string]any{}, Missing: []string{}}
	// Existing domain commands may accept longer reasons. Keep their native
	// receipt/validation contract while making the audit truncation explicit.
	if utf8.RuneCountInString(in.Reason) > 500 {
		clean, _ := SafeText(in.Reason, utf8.RuneCountInString(in.Reason))
		f.Reason, _ = SafeText(clean, 480)
		f.Reason += "（超长原因已截断）"
		f.Context["reason_sha256"] = Hash(clean)
		f.Context["reason_length"] = utf8.RuneCountInString(clean)
	}
	if in.Before == nil {
		f.Missing = append(f.Missing, "before")
	}
	if in.After == nil {
		f.Missing = append(f.Missing, "after")
	}
	if actorType != "account" {
		f.Initiator = in.ActorID
		f.ActorID = ""
		f.ExecutorKind = actorType
		f.ExecutorRef = actorType
		f.RequestKind = "worker"
	}
	if in.Initiator != "" {
		f.Initiator = in.Initiator
	}
	if in.RequestedBy != "" {
		f.RequestedBy = in.RequestedBy
	}
	if in.ExecutorKind != "" {
		f.ExecutorKind = in.ExecutorKind
		f.ExecutorRef = in.ExecutorRef
	}
	if in.ActorID != "" {
		var name, username string
		if e = tx.QueryRow(ctx, `SELECT display_name,username FROM accounts WHERE id=$1`, in.ActorID).Scan(&name, &username); e != nil {
			return EventRef{}, e
		}
		if actorType == "account" {
			f.Context["actor_name"] = name
			f.Context["actor_username"] = username
			f.Context["actor_name_source"] = "snapshot"
			f.Details["actor"] = map[string]string{"id": in.ActorID, "display_name": name, "username": username}
		} else {
			f.Context["initiator_name"] = name
		}
	}
	// Deletion has no after snapshot. The last recorded label is still evidence,
	// and an empty post-delete struct must not erase the actual prior name.
	for _, snapshot := range []any{after, before} {
		m, ok := snapshot.(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"name", "display_name", "title", "filename"} {
			if v, exists := m[key].(string); exists && v != "" && f.Context["object_label"] == nil {
				f.Context["object_label"] = v
				f.Context["object_label_source"] = "snapshot"
				break
			}
		}
		if v, ok := m["revision"].(string); ok && f.ObjectRevision == "" {
			f.ObjectRevision = v
		}
	}
	command, e := CommandTx(ctx, tx)
	if e != nil {
		return EventRef{}, e
	}
	if command.OperationID != "" {
		f.OperationID = command.OperationID
		f.Initiator = command.Initiator
		f.RequestedBy = command.RequestedBy
		f.Context["before_state"] = command.BeforeState
		f.Context["action"] = command.Action
		if command.AfterState != "" {
			f.Details["task"] = map[string]string{"task_id": command.TaskID, "generation": command.Generation, "before_state": command.BeforeState, "after_state": command.AfterState, "original_actor": command.Initiator, "actor_id": command.ActorID, "requested_by": command.RequestedBy}
		}
		f.RequestKind = "command_legacy"
		f.Links = append(f.Links, Link{Kind: "task_execution", Module: command.Module, TargetKind: command.Kind, TargetID: command.RootID, Generation: command.Generation, Relation: "execution", Scope: command.Scope})
	}
	f.Links = append(f.Links, in.Links...)
	if ID(f.ObjectID) && f.ObjectType != "" {
		f.Links = append(f.Links, Link{Kind: "business", Module: f.Module, TargetKind: f.ObjectType, TargetID: f.ObjectID, Revision: f.ObjectRevision, Relation: "subject", Scope: f.Scope})
	}
	return AppendTx(ctx, tx, f)
}
