package identity

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// LogMFACounts emits aggregate gauges only. No account, IP, token, code or hash
// becomes a metric label; expired challenges are retained for at most 24 hours.
func (s *Service) LogMFACounts(ctx context.Context) error {
	var failures, pending int64
	e := s.repo.pool.QueryRow(ctx, `SELECT COALESCE(sum(failed_attempts),0),count(*) FILTER(WHERE state='pending' AND expires_at>now()) FROM account_auth_challenges WHERE created_at>now()-interval '24 hours'`).Scan(&failures, &pending)
	if e == nil {
		s.logger.Info("mfa authentication counts", "failures_retained_24h", failures, "pending_challenges", pending)
	}
	return e
}

type MFAKeyReference struct {
	KeyID      string `json:"key_id"`
	References int64  `json:"references"`
}

// MFAKeyReferences is a maintenance-only count, not a personnel secret API.
func (s *Service) MFAKeyReferences(ctx context.Context) ([]MFAKeyReference, error) {
	rows, e := s.repo.pool.Query(ctx, `SELECT key_id,count(*) FROM (SELECT key_id FROM account_mfa WHERE key_id IS NOT NULL UNION ALL SELECT key_id FROM account_auth_challenges WHERE key_id IS NOT NULL) refs GROUP BY key_id ORDER BY key_id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []MFAKeyReference{}
	for rows.Next() {
		var v MFAKeyReference
		if e = rows.Scan(&v.KeyID, &v.References); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) CleanupMFA(ctx context.Context) error {
	for _, q := range []string{
		`UPDATE account_auth_challenges SET state='revoked',revoked_at=now(),secret_ciphertext=NULL,secret_nonce=NULL,key_id=NULL WHERE id IN (SELECT id FROM account_auth_challenges WHERE state='pending' AND expires_at<=now() ORDER BY expires_at LIMIT 500 FOR UPDATE SKIP LOCKED)`,
		`DELETE FROM account_auth_challenges WHERE id IN (SELECT id FROM account_auth_challenges WHERE state<>'pending' AND COALESCE(completed_at,revoked_at)<now()-interval '24 hours' ORDER BY expires_at LIMIT 500 FOR UPDATE SKIP LOCKED)`,
	} {
		if _, e := r.pool.Exec(ctx, q); e != nil {
			return e
		}
	}
	return nil
}

// RotateMFAKeys is restartable: each bounded batch retains old decryptors and
// changes neither factor versions nor replay counters. No secret is returned.
func (s *Service) RotateMFAKeys(ctx context.Context) (int, error) {
	count := 0
	e := s.repo.transact(ctx, func(tx pgx.Tx) error {
		if _, e := access.Lock(ctx, tx, false); e != nil {
			return e
		}
		if _, e := lockSecurityPolicy(ctx, tx, false); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT id::text FROM accounts WHERE id IN (SELECT account_id FROM account_mfa WHERE key_id<>$1 UNION SELECT account_id FROM account_auth_challenges WHERE key_id<>$1) ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`, s.mfa.active)
		if e != nil {
			return e
		}
		var accounts []string
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			accounts = append(accounts, id)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		for _, id := range accounts {
			f, e := loadFactor(ctx, tx, id)
			if e != nil {
				return e
			}
			if f.Secret.KeyID != "" && f.Secret.KeyID != s.mfa.active {
				plain, e := s.mfa.open(f.Secret, id, "factor", f.Version)
				if e != nil {
					return e
				}
				sealed, e := s.mfa.seal(plain, id, "factor", f.Version)
				if e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `UPDATE account_mfa SET secret_ciphertext=$2,secret_nonce=$3,key_id=$4 WHERE account_id=$1`, id, sealed.Ciphertext, sealed.Nonce, sealed.KeyID); e != nil {
					return e
				}
				count++
			}
			rows, e := tx.Query(ctx, `SELECT id::text FROM account_auth_challenges WHERE account_id=$1 AND key_id<>$2 ORDER BY id LIMIT 500`, id, s.mfa.active)
			if e != nil {
				return e
			}
			var ids []string
			for rows.Next() {
				var id string
				if e = rows.Scan(&id); e != nil {
					rows.Close()
					return e
				}
				ids = append(ids, id)
			}
			rows.Close()
			if e = rows.Err(); e != nil {
				return e
			}
			for _, cid := range ids {
				c, e := readChallenge(ctx, tx, cid)
				if e != nil {
					return e
				}
				plain, e := s.mfa.open(c.Secret, id, "challenge:"+cid, c.FactorVersion)
				if e != nil {
					return e
				}
				sealed, e := s.mfa.seal(plain, id, "challenge:"+cid, c.FactorVersion)
				if e != nil {
					return e
				}
				if _, e = tx.Exec(ctx, `UPDATE account_auth_challenges SET secret_ciphertext=$2,secret_nonce=$3,key_id=$4 WHERE id=$1`, cid, sealed.Ciphertext, sealed.Nonce, sealed.KeyID); e != nil {
					return e
				}
				count++
			}
		}
		return nil
	})
	return count, e
}
