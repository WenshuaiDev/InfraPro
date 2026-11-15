package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type GuardedRead struct {
	closeErr                error
	release                 func()
	Name, MediaType, SHA256 string
	Size                    int64
	s                       *Service
	a                       access.Actor
	v                       exportRecord
	lease                   string
	transfer                ar.Transfer
	ctx                     context.Context
	cancel                  context.CancelFunc
	file                    *os.File
	once                    sync.Once
	mu                      sync.Mutex
	sent                    int64
	outcome                 string
	done                    chan struct{}
}

func (s *Service) OpenRead(ctx context.Context, a access.Actor, scope Scope, id, generation, method, request string) (*GuardedRead, error) {
	if !ar.ID(id) || !ar.Number(generation, 1) || (method != "GET" && method != "HEAD") {
		return nil, Problem("audit_reference_invalid", 400)
	}
	readCtx, cancel := context.WithTimeout(ctx, s.options.ReadTimeout)
	g := &GuardedRead{s: s, a: a, lease: uuid.NewString(), ctx: readCtx, cancel: cancel, Name: "审计导出.csv", MediaType: "text/csv; charset=utf-8", outcome: "interrupted"}
	e := s.transaction(readCtx, a, scope, func(tx pgx.Tx, auth authorization) error {
		v, e := loadExportTx(readCtx, tx, id, generation, true)
		if e != nil {
			return e
		}
		p, e := s.presentExport(v, a, auth)
		if e != nil {
			return e
		}
		if !p.Owned || !auth.Has("export") {
			return access.ErrDenied
		}
		if p.ResultState != "available" {
			return resultError(p.ResultState)
		}
		if auth.Mode != "ready" {
			return Problem("audit_protocol_unavailable", 503)
		}
		if _, e = tx.Exec(readCtx, `SELECT pg_advisory_xact_lock(834327121)`); e != nil {
			return e
		}
		var mine, node int
		e = tx.QueryRow(readCtx, `SELECT count(*) FILTER(WHERE actor_id=$1),count(*) FILTER(WHERE node_id=$2) FROM audit_export_read_leases WHERE closed_at IS NULL AND expires_at>clock_timestamp()`, a.ID, s.options.NodeID).Scan(&mine, &node)
		if e != nil {
			return e
		}
		if mine >= s.options.DownloadsPerAccount || node >= s.options.DownloadsPerNode {
			return Problem("audit_download_limit", 429)
		}
		// The object row is the mutual exclusion point with cleanup intent.
		var state string
		if e = tx.QueryRow(readCtx, `SELECT state FROM audit_export_objects WHERE id=$1 FOR UPDATE`, v.ObjectID).Scan(&state); e != nil {
			return e
		}
		if state != "available" {
			return resultError(state)
		}
		g.v = v
		g.Size, e = strconv.ParseInt(*v.SizeBytes, 10, 64)
		if e != nil || g.Size > s.options.ExportMaxBytes {
			return Problem("audit_export_integrity", 503)
		}
		g.SHA256 = v.SHA256
		g.transfer, e = ar.StartTransferTx(readCtx, tx, ar.Transfer{Module: "audit", Actor: a.ID, Membership: auth.MembershipKey, Request: request, Epoch: auth.Epoch, Scope: scope, File: ar.Link{Kind: "domain_file", Module: "audit", TargetKind: "audit_export", TargetID: id, Generation: generation, Relation: "output"}}, method)
		if e != nil {
			return e
		}
		_, e = tx.Exec(readCtx, `INSERT INTO audit_export_read_leases(id,object_id,actor_id,node_id,restore_epoch,transfer_id,expires_at,hard_expires_at) VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()+$7*interval '1 second',clock_timestamp()+$8*interval '1 second')`, g.lease, v.ObjectID, a.ID, s.options.NodeID, auth.Epoch, g.transfer.ID, s.options.ReadLease.Seconds(), s.options.ReadTimeout.Seconds())
		return e
	})
	if e != nil {
		cancel()
		return nil, e
	}
	g.done = make(chan struct{})
	go func() {
		defer close(g.done)
		ticker := time.NewTicker(s.options.ReadHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-readCtx.Done():
				return
			case <-ticker.C:
				if g.Authorize(readCtx) != nil {
					cancel()
					return
				}
			}
		}
	}()
	fail := func(e error) (*GuardedRead, error) {
		g.mu.Lock()
		g.outcome = "error"
		g.mu.Unlock()
		g.Close()
		return nil, e
	}
	if s.objects == nil {
		return fail(Problem("audit_storage_unavailable", 503))
	}
	g.release, e = s.reserveTemp(readCtx, g.Size)
	if e != nil {
		return fail(e)
	}
	if e = os.MkdirAll(s.options.TempDirectory, 0700); e != nil {
		return fail(e)
	}
	g.file, e = os.CreateTemp(s.options.TempDirectory, "audit-read-*.csv")
	if e != nil {
		return fail(e)
	}
	reader, e := s.objects.Get(readCtx, g.v.ObjectKey)
	if e != nil {
		if errors.Is(e, fs.ErrNotExist) {
			g.markMissing()
			return fail(Problem("audit_export_missing", 503))
		}
		return fail(Problem("audit_storage_unavailable", 503))
	}
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(g.file, hash), io.LimitReader(reader, g.Size+1))
	closeError := reader.Close()
	if e != nil || closeError != nil {
		return fail(Problem("audit_storage_unavailable", 503))
	}
	if n != g.Size || hex.EncodeToString(hash.Sum(nil)) != g.SHA256 {
		g.markMissing()
		return fail(Problem("audit_export_integrity", 503))
	}
	if e = g.Authorize(readCtx); e != nil {
		return fail(e)
	}
	return g, nil
}
func resultError(state string) error {
	switch state {
	case "expired", "deleted", "delete_pending":
		return Problem("audit_export_expired", 410)
	case "invalidated":
		return Problem("audit_export_invalidated", 409)
	case "restricted":
		return access.ErrDenied
	case "missing":
		return Problem("audit_export_missing", 503)
	default:
		return Problem("audit_export_not_ready", 409)
	}
}
func (g *GuardedRead) boundary(ctx context.Context, fn func() error) error {
	if e := g.ctx.Err(); e != nil {
		return e
	}
	return g.s.transaction(ctx, g.a, g.v.Scope, func(tx pgx.Tx, auth authorization) error {
		if auth.Mode != "ready" || auth.Vector != g.v.Vector || auth.Epoch != g.v.Epoch || !auth.Has("export") || auth.MembershipKey != g.v.Membership {
			return Problem("audit_export_invalidated", 409)
		}
		var valid bool
		e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_export_read_leases l JOIN audit_export_objects o ON o.id=l.object_id WHERE l.id=$1 AND l.closed_at IS NULL AND l.expires_at>clock_timestamp() AND l.hard_expires_at>clock_timestamp() AND l.restore_epoch=$2 AND o.state='available' AND o.expires_at>clock_timestamp())`, g.lease, auth.Epoch).Scan(&valid)
		if e != nil {
			return e
		}
		if !valid {
			return resultError("expired")
		}
		if _, e = tx.Exec(ctx, `UPDATE audit_export_read_leases SET expires_at=LEAST(hard_expires_at,clock_timestamp()+$2*interval '1 second') WHERE id=$1`, g.lease, g.s.options.ReadLease.Seconds()); e != nil {
			return e
		}
		if fn != nil {
			if e = fn(); e != nil {
				return e
			}
		}
		g.mu.Lock()
		sent := g.sent
		g.mu.Unlock()
		_, e = tx.Exec(ctx, `UPDATE audit_transfer_receipts SET last_seen_at=clock_timestamp(),sent_bytes=$2 WHERE id=$1 AND closed_at IS NULL`, g.transfer.ID, sent)
		return e
	})
}
func (g *GuardedRead) Authorize(ctx context.Context) error { return g.boundary(ctx, nil) }
func (g *GuardedRead) Respond(ctx context.Context, fn func() error) error {
	e := g.boundary(ctx, fn)
	if e == nil {
		g.mu.Lock()
		g.outcome = "headers_only"
		g.mu.Unlock()
	}
	return e
}
func (g *GuardedRead) Write(ctx context.Context, offset, length int64, write func([]byte) (int, error)) error {
	if offset < 0 || length < 0 || offset > g.Size || length > g.Size-offset {
		return Problem("invalid_range", 416)
	}
	if _, e := g.file.Seek(offset, io.SeekStart); e != nil {
		return e
	}
	buf := make([]byte, 64<<10)
	for length > 0 {
		n, e := io.ReadFull(g.file, buf[:min(int64(len(buf)), length)])
		if e != nil {
			return e
		}
		e = g.boundary(ctx, func() error {
			n, e := write(buf[:n])
			g.mu.Lock()
			g.sent += int64(n)
			g.mu.Unlock()
			if e == nil && n != len(buf[:min(int64(len(buf)), length)]) {
				return io.ErrShortWrite
			}
			return e
		})
		if e != nil {
			g.mu.Lock()
			g.outcome = "interrupted"
			if isAuthorizationError(e) {
				g.outcome = "revoked"
			}
			g.mu.Unlock()
			return e
		}
		length -= int64(n)
	}
	g.mu.Lock()
	g.outcome = "complete"
	g.mu.Unlock()
	return nil
}
func (g *GuardedRead) markMissing() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = g.s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE audit_export_objects SET state='missing' WHERE id=$1 AND state='available'`, g.v.ObjectID)
		return e
	})
}
func (g *GuardedRead) Close() error {
	g.once.Do(func() {
		g.cancel()
		if g.done != nil {
			<-g.done
		}
		if g.file != nil {
			g.file.Close()
			os.Remove(g.file.Name())
		}
		if g.release != nil {
			g.release()
		}
		ctx, cancel := context.WithTimeout(context.Background(), g.s.options.FailedFactTimeout)
		defer cancel()
		g.mu.Lock()
		sent, outcome := g.sent, g.outcome
		g.mu.Unlock()
		g.closeErr = g.s.access.Transact(ctx, func(tx pgx.Tx) error {
			if _, e := tx.Exec(ctx, `UPDATE audit_export_read_leases SET closed_at=clock_timestamp() WHERE id=$1 AND closed_at IS NULL`, g.lease); e != nil {
				return e
			}
			return ar.FinishTransferTx(ctx, tx, g.transfer, outcome, &sent)
		})
		if g.closeErr != nil {
			g.s.access.AuditDiagnostic(g.transfer.Request, g.transfer.ID, "transfer_close_failed")
		}
	})
	return g.closeErr
}
