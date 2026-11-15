package files

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Sweep expires technical leases and reclaims abandoned local files only after
// obtaining their OS lock. Unknown remote writes remain charged until deletion.
func (s *Service) Sweep(ctx context.Context) error {
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, mode, e := protocolTx(ctx, tx)
		if e != nil {
			return e
		}
		if mode != "online" {
			return nil
		}
		if _, e = tx.Exec(ctx, `SELECT key FROM file_budget_states WHERE key='instance' FOR UPDATE`); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_uploads SET state='expired',revision=revision+1,updated_at=$1 WHERE state='receiving' AND LEAST(idle_expires_at,hard_expires_at)<=$1`, s.now())
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_read_leases SET closed_at=$1,outcome='lease_expired' WHERE closed_at IS NULL AND expires_at<=$1`, s.now())
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `DELETE FROM file_rate_windows WHERE window_start<$1`, s.now().Add(-time.Hour))
		return e
	})
	if e != nil {
		return e
	}
	type reservation struct{ Kind, Owner string }
	items := []reservation{}
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT owner_kind,owner_id::text FROM file_budget_reservations WHERE node_id=$1 AND kind='local_temp' AND state<>'released' AND expires_at<=$2`, s.options.NodeID, s.now())
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v reservation
			if e = rows.Scan(&v.Kind, &v.Owner); e != nil {
				return e
			}
			items = append(items, v)
		}
		return rows.Err()
	})
	if e != nil {
		return e
	}
	for _, item := range items {
		path := filepath.Join(s.options.TempDirectory, "stage-"+item.Owner)
		f, e := os.OpenFile(path, os.O_RDWR, 0600)
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if e == nil {
			if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
				f.Close()
				continue
			}
			e = os.Remove(path)
			f.Close()
			if e != nil && !errors.Is(e, os.ErrNotExist) {
				return e
			}
		}
		e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE file_budget_reservations SET state='released',reconciled_at=$3 WHERE owner_kind=$1 AND owner_id=$2 AND kind='local_temp'`, item.Kind, item.Owner, s.now())
			return e
		})
		if e != nil {
			return e
		}
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE file_budget_reservations b SET state='released',reconciled_at=$1 WHERE kind='download' AND state<>'released' AND EXISTS(SELECT 1 FROM file_read_leases l WHERE l.id=b.owner_id AND l.closed_at IS NOT NULL) AND NOT EXISTS(SELECT 1 FROM file_budget_reservations t WHERE t.owner_id=b.owner_id AND t.kind='local_temp' AND t.state<>'released')`, s.now())
		return e
	})
}
