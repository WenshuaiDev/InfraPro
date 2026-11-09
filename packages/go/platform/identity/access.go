package identity

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

type AuthContext struct {
	Account     Account       `json:"account"`
	Permissions []access.Code `json:"permissions"`
	Revision    string        `json:"authorization_revision"`
}
type Profile struct {
	Account       Account         `json:"account"`
	Authorization access.Snapshot `json:"authorization"`
}

func (s *Service) Access() *access.Service { return s.access }
func (s *Service) AccessActor(p Principal) access.Actor {
	return access.Actor{ID: p.Account.ID, Check: func(ctx context.Context, tx pgx.Tx, recent bool) error {
		return lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, recent)
	}}
}
func (s *Service) readAuthorized(ctx context.Context, p Principal, op access.Operation, fn func(pgx.Tx) error) error {
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		if e := access.Require(ctx, tx, p.Account.ID, op); e != nil {
			return e
		}
		if e := lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, false); e != nil {
			return e
		}
		return fn(tx)
	})
	if e != nil {
		s.access.LogDenial(op, p.Account.ID, e)
	}
	return e
}
func (s *Service) AuthContext(ctx context.Context, p Principal) (AuthContext, error) {
	var v AuthContext
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		rev, e := access.Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		if e = lockPrincipal(ctx, tx, p, s.cfg.SessionIdle, false); e != nil {
			return e
		}
		v.Account, e = scanAccount(tx.QueryRow(ctx, "SELECT "+accountColumns+" FROM accounts WHERE id=$1", p.Account.ID))
		if e != nil {
			return e
		}
		v.Revision = rev
		v.Permissions, e = access.PermissionsTx(ctx, tx, p.Account.ID)
		return e
	})
	return v, e
}
func (s *Service) ReadProfile(ctx context.Context, p Principal) (Profile, error) {
	var v Profile
	e := s.readAuthorized(ctx, p, access.OpProfileRead, func(tx pgx.Tx) error {
		var e error
		v.Account, e = scanAccount(tx.QueryRow(ctx, "SELECT "+accountColumns+" FROM accounts WHERE id=$1", p.Account.ID))
		if e != nil {
			return e
		}
		var rev string
		if e = tx.QueryRow(ctx, "SELECT revision::text FROM authorization_state WHERE id=1").Scan(&rev); e != nil {
			return e
		}
		v.Authorization, e = access.SnapshotTx(ctx, tx, p.Account.ID, rev)
		return e
	})
	return v, e
}
func (s *Service) SetAccountStatus(ctx context.Context, p Principal, id, version string, enabled bool, m access.Meta) (access.Account, error) {
	return s.access.SetAccountStatus(ctx, s.AccessActor(p), id, version, enabled, m, s.repo.SetStatusTx)
}
