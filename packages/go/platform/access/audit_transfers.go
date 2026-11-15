package access

import (
	"context"
	"errors"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
	"io"
	"sync"
	"time"
)

type transportContextKey struct{}
type Transport struct{ Method, Request string }

func WithTransport(ctx context.Context, method, request string) context.Context {
	return context.WithValue(ctx, transportContextKey{}, Transport{method, ar.SafeRequestID(request)})
}
func TransportMetadata(ctx context.Context) Transport {
	v, _ := ctx.Value(transportContextKey{}).(Transport)
	if v.Method == "" {
		v.Method = "GET"
	}
	return v
}

// TransferObserver observes a source-owned read. It never grants authorization,
// opens a file or supplies storage addresses. Sources still gate every block.
type TransferObserver struct {
	service *Service
	fact    ar.Transfer
	once    sync.Once
	mu      sync.Mutex
	bytes   int64
	outcome string
	err     error
}

func (s *Service) BeginTransferTx(ctx context.Context, tx pgx.Tx, actor, membership string, scope ar.Scope, module string, file ar.Link) (*TransferObserver, error) {
	v := TransportMetadata(ctx)
	if membership == "" && scope.Kind != "platform" {
		var e error
		if scope.Kind == "project" {
			e = tx.QueryRow(ctx, `SELECT tm.id::text||':'||pm.id::text FROM tenant_memberships tm JOIN project_memberships pm ON pm.tenant_membership_id=tm.id WHERE tm.tenant_id=$1 AND tm.account_id=$2 AND tm.status='active' AND pm.project_id=$3 AND pm.status='active'`, scope.Tenant, actor, scope.Project).Scan(&membership)
		} else {
			e = tx.QueryRow(ctx, `SELECT id::text FROM tenant_memberships WHERE tenant_id=$1 AND account_id=$2 AND status='active'`, scope.Tenant, actor).Scan(&membership)
		}
		if e != nil {
			return nil, e
		}
	}
	var epoch string
	if e := tx.QueryRow(ctx, `SELECT restore_epoch::text FROM audit_protocol WHERE id=1 FOR SHARE`).Scan(&epoch); e != nil {
		return nil, e
	}
	t, e := ar.StartTransferTx(ctx, tx, ar.Transfer{Module: module, Scope: scope, Actor: actor, Membership: membership, Request: v.Request, Epoch: epoch, File: file}, v.Method)
	if e != nil {
		return nil, e
	}
	return &TransferObserver{service: s, fact: t, outcome: "interrupted"}, nil
}
func (t *TransferObserver) Add(n int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.bytes += int64(n)
}
func (t *TransferObserver) End(err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err == nil {
		t.outcome = "complete"
		return
	}
	c := ar.ClassifyFailure(err)
	t.outcome = "error"
	if c.Outcome == "denied" || c.Outcome == "conflict" {
		t.outcome = "revoked"
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrShortWrite) {
		t.outcome = "interrupted"
	}
}
func (t *TransferObserver) Headers() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.outcome = "headers_only"
}
func (t *TransferObserver) HeartbeatTx(ctx context.Context, tx pgx.Tx) error {
	if t == nil {
		return nil
	}
	tag, e := tx.Exec(ctx, `UPDATE audit_transfer_receipts SET last_seen_at=clock_timestamp() WHERE id=$1 AND closed_at IS NULL AND epoch=(SELECT restore_epoch FROM audit_protocol WHERE id=1)`, t.fact.ID)
	if e == nil && tag.RowsAffected() != 1 {
		return ar.ErrProtocol
	}
	return e
}
func (t *TransferObserver) CloseTx(ctx context.Context, tx pgx.Tx) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return ar.FinishTransferTx(ctx, tx, t.fact, t.outcome, &t.bytes)
}
func (t *TransferObserver) Diagnose(err error) {
	if t == nil || err == nil {
		return
	}
	persistFailureCount.Add(1)
	if t.service.logger != nil {
		t.service.logger.Error("audit_persist_failure", "request_id", t.fact.Request, "transfer_id", t.fact.ID, "error_code", "transfer_close_failed")
	}
}
func (t *TransferObserver) Close() error {
	if t == nil {
		return nil
	}
	t.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		t.err = t.service.Transact(ctx, func(tx pgx.Tx) error { return t.CloseTx(ctx, tx) })
		t.Diagnose(t.err)
	})
	return t.err
}

type observedWriter struct {
	io.Writer
	transfer *TransferObserver
}

func (w observedWriter) Write(p []byte) (int, error) {
	n, e := w.Writer.Write(p)
	w.transfer.Add(n)
	return n, e
}
func (t *TransferObserver) Wrap(write func(context.Context, io.Writer) error) func(context.Context, io.Writer) error {
	return func(ctx context.Context, w io.Writer) (result error) {
		readCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		done := make(chan struct{})
		go func() {
			defer close(done)
			timer := time.NewTicker(10 * time.Second)
			defer timer.Stop()
			for {
				select {
				case <-readCtx.Done():
					return
				case <-timer.C:
					beat, stop := context.WithTimeout(readCtx, 2*time.Second)
					e := t.service.Transact(beat, func(tx pgx.Tx) error { return t.HeartbeatTx(beat, tx) })
					stop()
					if e != nil {
						cancel()
						return
					}
				}
			}
		}()
		result = write(readCtx, observedWriter{w, t})
		cancel()
		<-done
		t.End(result)
		if e := t.Close(); result == nil {
			result = e
		}
		return
	}
}
