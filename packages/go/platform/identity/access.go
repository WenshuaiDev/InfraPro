package identity

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"time"
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

// CheckDisplaySession keeps runtime reads tied to the current personnel session
// without storing its cookie or borrowing a manager's account for terminals.
func (s *Service) CheckDisplaySession(ctx context.Context, tx pgx.Tx, account, session string) error {
	return CheckDisplaySessionTx(ctx, tx, account, session, s.cfg.SessionIdle)
}
func CheckDisplaySessionTx(ctx context.Context, tx pgx.Tx, account, session string, idle time.Duration) error {
	if _, e := access.Lock(ctx, tx, false); e != nil {
		return e
	}
	policy, e := lockSecurityPolicy(ctx, tx, false)
	if e != nil {
		return e
	}
	var active bool
	if e := tx.QueryRow(ctx, `SELECT status='active' FROM accounts WHERE id=$1 FOR SHARE`, account).Scan(&active); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrUnauthorized
		}
		return e
	}
	if !active {
		return ErrUnauthorized
	}
	if e := checkMFASession(ctx, tx, account, session, policy, nil); e != nil {
		return e
	}
	// Snapshot materialization must not write the personnel session: concurrent
	// readers would otherwise fail with a serialization conflict. Delivery and
	// heartbeat transactions use READ COMMITTED and refresh the idle lease below.
	var isolation string
	if e := tx.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&isolation); e != nil {
		return e
	}
	if isolation != "read committed" {
		var valid bool
		e := tx.QueryRow(ctx, `SELECT ss.auth_version=a.auth_version AND ss.revoked_at IS NULL AND ss.expires_at>clock_timestamp() AND ss.last_activity_at>clock_timestamp()-$3::interval FROM account_sessions ss JOIN accounts a ON a.id=ss.account_id WHERE ss.id=$1 AND ss.account_id=$2`, session, account, interval(idle)).Scan(&valid)
		if errors.Is(e, pgx.ErrNoRows) || e == nil && !valid {
			return ErrUnauthorized
		}
		return e
	}
	tag, e := tx.Exec(ctx, `UPDATE account_sessions ss SET last_activity_at=clock_timestamp() FROM accounts a WHERE ss.id=$1 AND ss.account_id=$2 AND a.id=ss.account_id AND ss.auth_version=a.auth_version AND ss.revoked_at IS NULL AND ss.expires_at>clock_timestamp() AND ss.last_activity_at>clock_timestamp()-$3::interval`, session, account, interval(idle))
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrUnauthorized
	}
	return nil
}
