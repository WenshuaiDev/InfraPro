package auditrecord

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Command struct {
	Module, ActorID, MembershipKey, Action, ID, Parent string
	Scope                                              Scope
	// Payload must already be the producer's non-secret, typed intent. Never a
	// raw HTTP body or an identity password/reset/invitation credential.
	Payload any
}
type Receipt struct {
	OperationID           string
	Replayed              bool
	Response              json.RawMessage
	ResultCode, ResultRef string
}

func BeginCommandTx(ctx context.Context, tx pgx.Tx, c Command) (Receipt, error) {
	out := Receipt{}
	if c.ID == "" {
		return out, ErrCommandRequired
	}
	if !ID(c.ID) || !ID(c.ActorID) || !c.Scope.Valid() || len(c.Action) > 160 || !safeCode.MatchString(c.Action) || len(c.Module) > 64 || len(c.MembershipKey) > 200 {
		return out, ErrInvalid
	}
	d, ok := Lookup(c.Action)
	if !ok || d.Module != c.Module {
		return out, ErrUnregistered
	}
	_, e := Snapshot(c.Payload, d.Fields)
	if e != nil {
		return out, e
	}
	// Hash the complete typed, non-secret intent before disclosure redaction.
	// The digest is irreversible; omitted/truncated fields must still conflict.
	digest := Hash(c.Payload)
	key := Hash([]string{c.ActorID, c.MembershipKey, c.Scope.Key(), c.Action, c.ID})
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,58413))`, key); e != nil {
		return out, e
	}
	var previous string
	e = tx.QueryRow(ctx, `SELECT id::text,payload_hash FROM audit_operations WHERE actor_id=$1 AND membership_key=$2 AND scope_key=$3 AND action=$4 AND command_id=$5`, c.ActorID, c.MembershipKey, c.Scope.Key(), c.Action, c.ID).Scan(&out.OperationID, &previous)
	if e == nil {
		if previous != digest {
			return out, ErrCommandConflict
		}
		var expired bool
		e = tx.QueryRow(ctx, `SELECT safe_response,result_code,result_ref,tombstoned_at IS NOT NULL FROM audit_command_receipts WHERE operation_id=$1`, out.OperationID).Scan(&out.Response, &out.ResultCode, &out.ResultRef, &expired)
		if e == pgx.ErrNoRows {
			return out, ErrProtocol
		}
		if e != nil {
			return out, e
		}
		if expired {
			return out, ErrReceiptExpired
		}
		out.Replayed = true
		return out, nil
	}
	if e != pgx.ErrNoRows {
		return out, e
	}
	out.OperationID = uuid.NewString()
	if c.Parent != "" {
		if e = validateParent(ctx, tx, out.OperationID, c.Parent); e != nil {
			return out, e
		}
	}
	_, e = tx.Exec(ctx, `INSERT INTO audit_operations(id,source_module,scope_kind,tenant_id,project_id,actor_id,membership_key,scope_key,command_id,payload_hash,action,parent_operation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, out.OperationID, c.Module, c.Scope.Kind, null(c.Scope.Tenant), null(c.Scope.Project), c.ActorID, c.MembershipKey, c.Scope.Key(), c.ID, digest, c.Action, null(c.Parent))
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `SELECT set_config('infrapro.audit_operation',$1,true)`, out.OperationID)
	return out, e
}
func CompleteCommandTx(ctx context.Context, tx pgx.Tx, r Receipt, response any, code, ref string) error {
	if r.Replayed || !ID(r.OperationID) || len(code) > 120 || !safeCode.MatchString(code) || len(ref) > 160 || !safeCode.MatchString(ref) {
		return ErrInvalid
	}
	// Result receipts contain identifiers and concurrency tokens, not arbitrary
	// response DTOs, account data, links, or credentials.
	safe, e := Snapshot(response, []string{"id", "object_id", "role_id", "account_id", "tenant_id", "project_id", "version", "revision", "generation", "state", "status", "changed", "operation_id", "export_id", "task_id", "authorization_revision"})
	if e != nil || len(JSON(safe)) > 16384 {
		return ErrInvalid
	}
	_, e = tx.Exec(ctx, `INSERT INTO audit_command_receipts(operation_id,safe_response,result_code,result_ref) VALUES($1,$2,$3,$4)`, r.OperationID, JSON(safe), code, ref)
	return e
}
