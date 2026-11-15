package files

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (s *Service) reserveObjectsTx(ctx context.Context, tx pgx.Tx, st txState, id string, size int64, expiry time.Time) error {
	var reserved int64
	e := tx.QueryRow(ctx, `SELECT COALESCE(sum(reserved_bytes),0) FROM file_budget_reservations WHERE kind='temporary_object' AND state<>'released'`).Scan(&reserved)
	if e != nil {
		return e
	}
	if reserved+size > s.options.TemporaryObjectBudget {
		return fault("temporary_budget_exceeded", 429)
	}
	_, e = tx.Exec(ctx, `INSERT INTO file_budget_reservations(owner_kind,owner_id,space_id,account_id,tenant_id,kind,reserved_bytes,state,expires_at) VALUES('upload',$1,$2,$3,$4,'temporary_object',$5,'held',$6)`, id, st.SpaceID, st.Auth.AccountID, st.Auth.Scope.TenantID, size, expiry)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE file_budget_states SET reserved_bytes=$1,revision=revision+1 WHERE key='instance'`, reserved+size)
	return e
}
func (s *Service) objectBudgetTx(ctx context.Context, tx pgx.Tx, uploadID string, extra int64) error {
	var reserved, allocated, total int64
	e := tx.QueryRow(ctx, `SELECT reserved_bytes FROM file_budget_reservations WHERE owner_kind='upload' AND owner_id=$1 AND kind='temporary_object' AND state<>'released' FOR UPDATE`, uploadID).Scan(&reserved)
	if e != nil {
		return e
	}
	e = tx.QueryRow(ctx, `SELECT COALESCE(sum(expected_size),0) FROM file_objects WHERE state<>'deleted' AND ((writer_kind='upload' AND writer_id=$1) OR (writer_kind='job' AND writer_id IN(SELECT id FROM file_jobs WHERE upload_id=$1)))`, uploadID).Scan(&allocated)
	if e != nil {
		return e
	}
	missing := allocated + extra - reserved
	if missing <= 0 {
		return nil
	}
	e = tx.QueryRow(ctx, `SELECT COALESCE(sum(reserved_bytes),0) FROM file_budget_reservations WHERE kind='temporary_object' AND state<>'released'`).Scan(&total)
	if e != nil {
		return e
	}
	if total+missing > s.options.TemporaryObjectBudget {
		return fault("temporary_budget_exceeded", 429)
	}
	_, e = tx.Exec(ctx, `UPDATE file_budget_reservations SET reserved_bytes=reserved_bytes+$2 WHERE owner_kind='upload' AND owner_id=$1 AND kind='temporary_object'`, uploadID, missing)
	return e
}
func (s *Service) queueBudgetTx(ctx context.Context, tx pgx.Tx, tenant string) error {
	var own, total int
	e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE sp.tenant_id=$1),count(*) FROM file_jobs j JOIN file_spaces sp ON sp.id=j.space_id WHERE j.state IN ('queued','retry_wait')`, tenant).Scan(&own, &total)
	if e != nil {
		return e
	}
	if own >= s.options.MaxQueuedPerTenant || total >= s.options.MaxQueuedPerInstance {
		return fault("queue_limit", 429)
	}
	return nil
}
func (s *Service) stage(ctx context.Context, scope Scope, space, account, owner string, size int64, kind string) (*os.File, func(), error) {
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `SELECT key FROM file_budget_states WHERE key='instance' FOR UPDATE`); e != nil {
			return e
		}
		var total int64
		e := tx.QueryRow(ctx, `SELECT COALESCE(sum(reserved_bytes),0) FROM file_budget_reservations WHERE node_id=$1 AND kind='local_temp' AND state<>'released'`, s.options.NodeID).Scan(&total)
		if e != nil {
			return e
		}
		if total+size > s.options.LocalTempBudget {
			return fault("temporary_budget_exceeded", 429)
		}
		_, e = tx.Exec(ctx, `INSERT INTO file_budget_reservations(owner_kind,owner_id,space_id,account_id,tenant_id,node_id,kind,reserved_bytes,state,expires_at) VALUES($1,$2,$3,$4,$5,$6,'local_temp',$7,'held',$8)`, kind, owner, taskrecord.Null(space), taskrecord.Null(account), taskrecord.Null(scope.TenantID), s.options.NodeID, size, s.now().Add(s.options.ReadTimeout))
		return e
	})
	if e != nil {
		return nil, nil, e
	}
	releaseBudget := func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.access.Transact(releaseCtx, func(tx pgx.Tx) error {
			_, e := tx.Exec(releaseCtx, `UPDATE file_budget_reservations SET state='released',reconciled_at=$3 WHERE owner_kind=$1 AND owner_id=$2 AND kind='local_temp'`, kind, owner, s.now())
			return e
		})
	}
	if e = os.MkdirAll(s.options.TempDirectory, 0700); e != nil {
		releaseBudget()
		return nil, nil, e
	}
	f, e := os.OpenFile(filepath.Join(s.options.TempDirectory, "stage-"+owner), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		releaseBudget()
		return nil, nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		os.Remove(f.Name())
		releaseBudget()
		return nil, nil, e
	}
	return f, func() {
		f.Close()
		if os.Remove(f.Name()) == nil {
			releaseBudget()
		}
	}, nil
}
