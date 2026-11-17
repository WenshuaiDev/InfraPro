package access

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
	"time"
)

// mutationCommand is assembled by the owning operation from typed, non-secret
// fields. It is never populated from a generic request body or HTTP middleware.
type mutationCommand struct {
	action  string
	payload map[string]any
	result  func() string
	replay  func(context.Context, pgx.Tx, string) error
}

func beginMutationTx(ctx context.Context, tx pgx.Tx, a Actor, m Meta, scope ar.Scope, membership string) (ar.Receipt, error) {
	if !m.CommandRequired && m.CommandID == "" {
		return ar.Receipt{}, nil
	}
	if m.command == nil {
		return ar.Receipt{}, ar.ErrInvalid
	}
	return ar.BeginCommandTx(ctx, tx, ar.Command{Module: mutationModule(m.command.action), ActorID: a.ID, MembershipKey: membership, Scope: scope, Action: m.command.action, ID: m.CommandID, Payload: m.command.payload})
}
func mutationModule(action string) string { d, _ := ar.Lookup(action); return d.Module }
func completeMutationTx(ctx context.Context, tx pgx.Tx, m Meta, r ar.Receipt) error {
	if r.OperationID == "" {
		return nil
	}
	return ar.CompleteCommandTx(ctx, tx, r, map[string]string{"id": m.command.result()}, "ok", m.command.result())
}

func (s *Service) resolveMutation(a Actor, op Operation, m Meta, original error) error {
	if m.command == nil || m.CommandID == "" || m.command.replay == nil {
		return original
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	e := s.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := Lock(ctx, tx, false); e != nil {
			return e
		}
		if a.Check == nil {
			return ErrDenied
		}
		if e := a.Check(ctx, tx, false); e != nil {
			return e
		}
		if e := RequireTx(ctx, tx, a.ID, op); e != nil {
			return e
		}
		r, e := beginMutationTx(ctx, tx, a, m, ar.Scope{Kind: "platform"}, "")
		if e != nil {
			return e
		}
		if !r.Replayed {
			return original
		} // Roll back the empty probe; never re-execute an ambiguous command.
		return m.command.replay(ctx, tx, r.ResultRef)
	})
	if e != nil {
		return original
	}
	return nil
}

// CommandOperations is the finite list of business endpoints introducing the
// command header. Endpoints with native request IDs retain their original wire protocol.
func CommandOperations() []Operation {
	return []Operation{OpSecurityPolicyUpdate, OpAccountMFAReset, OpMFADisable, OpMFARecoveryRegenerate, OpRoleCreate, OpRoleUpdate, OpRoleSetPermissions, OpRoleSetStatus, OpRoleDelete, OpAccountAssignRoles, OpAccountDisable, OpAccountEnable, OpTenantRoleUpdate, OpTenantRolePermissions, OpTenantRoleStatus, OpTenantRoleArchive, OpTenantMemberRoles}
}
