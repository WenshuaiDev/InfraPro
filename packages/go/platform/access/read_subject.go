package access

import (
	"context"
	"github.com/jackc/pgx/v5"
	"slices"
)

type sharedPrincipalCheckKey struct{}
type sharedPrincipalCheck struct {
	account    string
	connection *pgx.Conn
}

// SessionActor binds a verified personnel session to its transaction checker.
// Regular commands retain the exclusive account lock. Only an input-read check
// on this same actor can request the shared mode.
func SessionActor(id string, verify func(context.Context, pgx.Tx, bool, bool) error) Actor {
	return Actor{ID: id, personnelSession: true, Check: func(ctx context.Context, tx pgx.Tx, recent bool) error {
		read, ok := ctx.Value(sharedPrincipalCheckKey{}).(sharedPrincipalCheck)
		return verify(ctx, tx, recent, !recent && ok && read.account == id && read.connection == tx.Conn())
	}}
}

// CheckInputReadTx checks the actor inside the caller's input-proof transaction.
// Personnel lock policy then account and validate the original session. Display
// subjects verify their own durable identity. A durable worker delegation locks
// its owner's account before its existing eligibility check.
func (a Actor) CheckInputReadTx(ctx context.Context, tx pgx.Tx) error {
	if a.Check == nil {
		return ErrDenied
	}
	if a.personnelSession {
		read := sharedPrincipalCheck{account: a.ID, connection: tx.Conn()}
		return a.Check(context.WithValue(ctx, sharedPrincipalCheckKey{}, read), tx, false)
	}
	if a.subject == nil {
		if _, err := tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR SHARE`, a.ID); err != nil {
			return err
		}
	}
	return a.Check(ctx, tx, false)
}

// ReadSubject is constructed inside a verified runtime, never decoded from a
// client request. Its owner is a durable display_subjects row, not an account.
// Check and Authorize run inside each domain's existing authorization boundary.
type ReadSubject struct {
	ID, Tenant, Project, Revision string
	Check                         func(context.Context, pgx.Tx) error
	Authorize                     func(context.Context, pgx.Tx, Operation) (ProjectAuthorization, error)
	// Windows is an additional ceiling for recursive raw/derived input reads.
	Windows func(context.Context, pgx.Tx, []string) (map[string][]ReadWindow, error)
	// HistoryWindows is deliberately separate: ordinary read windows never
	// imply permission to inspect revoked observations.
	HistoryWindows func(context.Context, pgx.Tx, []string) (map[string][]ReadWindow, error)
}
type ReadWindow struct{ From, To *int64 }

// ReadActor is an internal compatibility carrier for existing read algorithms.
// It has no personnel identity and can never authorize a business write.
func (s *ReadSubject) ReadActor() Actor {
	return Actor{ID: s.ID, subject: s, Check: func(ctx context.Context, tx pgx.Tx, recent bool) error {
		if recent || s.Check == nil {
			return ErrDenied
		}
		return s.Check(ctx, tx)
	}}
}
func (a Actor) ReadSubject() *ReadSubject { return a.subject }
func (a Actor) OwnerAccount() any {
	if a.subject != nil {
		return nil
	}
	return a.ID
}
func (a Actor) OwnerSubject() any {
	if a.subject == nil {
		return nil
	}
	return a.subject.ID
}
func ReadOperation(op Operation) bool {
	p, ok := PolicyFor(op)
	if !ok || p.Class != "project" {
		return false
	}
	if p.Method == "GET" {
		return true
	}
	return MonitoringReadOperation(op) || DerivedReadOperation(op) || SpatialReadOperation(op)
}
func RequireActorProjectTx(ctx context.Context, tx pgx.Tx, a Actor, op Operation, tenant, project string) (ProjectAuthorization, error) {
	if a.subject == nil {
		return RequireProjectTx(ctx, tx, a.ID, op, tenant, project)
	}
	s := a.subject
	if !ReadOperation(op) || tenant != s.Tenant || project != s.Project || s.Authorize == nil {
		return ProjectAuthorization{}, ErrDenied
	}
	v, e := s.Authorize(ctx, tx, op)
	if e != nil {
		return v, e
	}
	p, _ := PolicyFor(op)
	for _, c := range p.Permissions {
		if !slices.Contains(v.Permissions, c) {
			return v, ErrDenied
		}
	}
	return v, nil
}

// CheckReadBoundary rejects subject carriers at all shared write transaction seams.
func CheckReadBoundary(a Actor, scope string, op Operation, write bool) error {
	if a.subject != nil && (scope != "project" || write || !ReadOperation(op)) {
		return ErrDenied
	}
	return nil
}
