package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"io/fs"
	"os"
	"sync"
	"time"
)

// GuardedRead never exposes its object key or raw reader to HTTP. Headers and
// each output chunk must pass the current personnel and attachment boundary.
type GuardedRead struct {
	extraGuard                                           func(context.Context, pgx.Tx) error
	transfer                                             *access.TransferObserver
	closeErr                                             error
	Name, MediaType, SHA256, Variant                     string
	Size                                                 int64
	s                                                    *Service
	actor                                                access.Actor
	scope                                                Scope
	attachment, revision, epoch, lease, fileID, objectID string
	staged                                               *os.File
	release                                              func()
	ctx                                                  context.Context
	cancel                                               context.CancelFunc
	once                                                 sync.Once
	heartbeatDone                                        chan struct{}
	bytes                                                int64
	outcome                                              string
}

func (s *Service) OpenRead(ctx context.Context, a access.Actor, scope Scope, attachment, revision, variant string) (*GuardedRead, error) {
	return s.OpenReadWithGuard(ctx, a, scope, attachment, revision, variant, nil)
}

// OpenReadWithGuard preserves the file boundary and checks a concrete consumer's authority before headers and every chunk.
func (s *Service) OpenReadWithGuard(ctx context.Context, a access.Actor, scope Scope, attachment, revision, variant string, guard func(context.Context, pgx.Tx) error) (*GuardedRead, error) {
	if !access.ValidID(attachment) || !access.ValidID(revision) || variant != "source" && variant != "preview" {
		return nil, fault("file_not_found", 404)
	}
	readCtx, cancel := context.WithTimeout(ctx, s.options.ReadTimeout)
	g := &GuardedRead{extraGuard: guard, s: s, actor: a, scope: scope, attachment: attachment, revision: revision, Variant: variant, lease: uuid.NewString(), ctx: readCtx, cancel: cancel, outcome: "interrupted"}
	var space string
	e := s.transactBudget(readCtx, a, scope, false, false, true, "", func(tx pgx.Tx, st txState) error {
		if st.Mode == "reconciling" {
			return fault("file_protocol_unavailable", 503)
		}
		if g.extraGuard != nil {
			if e := g.extraGuard(ctx, tx); e != nil {
				return e
			}
		}
		if !st.Auth.CanDownload {
			return access.ErrDenied
		}
		if e := s.rateTx(readCtx, tx, a.ID, "read", s.options.ReadRequestsPerMinute); e != nil {
			return e
		}
		detail, e := attachmentTx(readCtx, tx, st, attachment, false)
		if e != nil {
			return e
		}
		if detail.Status != "active" {
			return fault("file_not_found", 404)
		}
		var state, integrity, objectState string
		objectColumn := "f.source_object_id"
		if variant == "preview" {
			objectColumn = "f.preview_object_id"
		}
		e = tx.QueryRow(readCtx, `SELECT f.id::text,f.original_name,o.id::text,o.expected_size,o.expected_sha256,o.content_type,f.lifecycle,f.integrity,o.state FROM file_attachment_revisions r JOIN file_records f ON f.id=r.file_id JOIN file_objects o ON o.id=`+objectColumn+` WHERE r.space_id=$1 AND r.attachment_id=$2 AND r.id=$3 FOR UPDATE OF f`, st.SpaceID, attachment, revision).Scan(&g.fileID, &g.Name, &g.objectID, &g.Size, &g.SHA256, &g.MediaType, &state, &integrity, &objectState)
		if e != nil {
			return notFound(e)
		}
		if state != "ready" {
			return fault("file_reclaiming", 409)
		}
		if integrity != "ok" || objectState != "available" {
			if integrity == "missing" || objectState == "missing" {
				return fault("file_missing", 503)
			}
			return fault("file_corrupt", 503)
		}
		var nodeCount, accountCount int
		e = tx.QueryRow(readCtx, `SELECT count(*) FILTER(WHERE node_id=$1),count(*) FILTER(WHERE account_id=$2) FROM file_budget_reservations WHERE kind='download' AND state<>'released'`, s.options.NodeID, a.ID).Scan(&nodeCount, &accountCount)
		if e != nil {
			return e
		}
		if nodeCount >= s.options.MaxDownloadsPerNode || accountCount >= s.options.MaxDownloadsPerAccount {
			return fault("rate_limited", 429)
		}
		g.epoch = st.Epoch
		space = st.SpaceID
		_, e = tx.Exec(readCtx, `INSERT INTO file_read_leases(id,space_id,file_id,object_id,actor_id,tenant_membership_id,project_membership_id,variant,restore_epoch,expires_at) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,$8,$9,$10)`, g.lease, space, g.fileID, g.objectID, a.ID, st.Auth.TenantMembershipID, st.Auth.ProjectMembershipID, variant, st.Epoch, s.now().Add(s.options.ReadLease))
		if e != nil {
			return e
		}
		g.transfer, e = s.access.BeginTransferTx(readCtx, tx, a.ID, st.Auth.TenantMembershipID+":"+st.Auth.ProjectMembershipID, ar.Scope{Kind: scope.Kind, Tenant: scope.TenantID, Project: scope.ProjectID}, "files", ar.Link{Kind: "file_revision", Module: "files", TargetKind: "attachment", TargetID: attachment, Revision: revision, Relation: "input"})
		if e != nil {
			return e
		}
		_, e = tx.Exec(readCtx, `INSERT INTO file_budget_reservations(owner_kind,owner_id,space_id,account_id,tenant_id,node_id,kind,reserved_bytes,state,expires_at) VALUES('read',$1,$2,$3,$4,$5,'download',0,'held',$6)`, g.lease, space, a.ID, scope.TenantID, s.options.NodeID, s.now().Add(s.options.ReadTimeout))
		return e
	})
	if e != nil {
		cancel()
		return nil, e
	}
	g.heartbeatDone = make(chan struct{})
	go func() {
		defer close(g.heartbeatDone)
		ticker := time.NewTicker(s.options.ReadHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-readCtx.Done():
				return
			case <-ticker.C:
				if e := g.Authorize(readCtx); e != nil {
					cancel()
					return
				}
			}
		}
	}()
	fail := func(e error) (*GuardedRead, error) { g.Close(); return nil, e }
	if s.objects == nil {
		return fail(fault("file_storage_unavailable", 503))
	}
	staged, release, e := s.stage(readCtx, scope, space, a.ID, g.lease, g.Size, "read")
	if e != nil {
		return fail(e)
	}
	g.staged = staged
	g.release = release
	source, e := s.objects.Get(readCtx, "files/v1/"+space+"/"+g.objectID)
	if e != nil {
		if errors.Is(e, fs.ErrNotExist) {
			g.markIntegrity("missing")
			return fail(fault("file_missing", 503))
		}
		return fail(fault("file_storage_unavailable", 503))
	}
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(staged, hash), io.LimitReader(source, g.Size+1))
	closeErr := source.Close()
	if e != nil || closeErr != nil {
		return fail(fault("file_storage_unavailable", 503))
	}
	if n != g.Size || hex.EncodeToString(hash.Sum(nil)) != g.SHA256 {
		g.markIntegrity("corrupt")
		return fail(fault("file_corrupt", 503))
	}
	if e = g.Authorize(readCtx); e != nil {
		return fail(e)
	}
	return g, nil
}
func (g *GuardedRead) boundary(ctx context.Context, fn func() error) error {
	if e := g.ctx.Err(); e != nil {
		return e
	}
	return g.s.transact(ctx, g.actor, g.scope, false, false, "", func(tx pgx.Tx, st txState) error {
		if st.Epoch != g.epoch || st.Mode == "reconciling" {
			return fault("file_epoch_changed", 409)
		}
		if g.extraGuard != nil {
			if e := g.extraGuard(ctx, tx); e != nil {
				return e
			}
		}
		if !st.Auth.CanDownload {
			return access.ErrDenied
		}
		detail, e := attachmentTx(ctx, tx, st, g.attachment, false)
		if e != nil {
			return e
		}
		if detail.Status != "active" {
			return fault("file_not_found", 404)
		}
		var valid bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_read_leases l JOIN file_records f ON f.id=l.file_id JOIN file_objects o ON o.id=l.object_id JOIN file_attachment_revisions r ON r.file_id=f.id WHERE l.id=$1 AND l.closed_at IS NULL AND l.expires_at>$2 AND l.tenant_membership_id=$3 AND l.project_membership_id IS NOT DISTINCT FROM NULLIF($4,'')::uuid AND f.lifecycle='ready' AND f.integrity='ok' AND o.state='available' AND r.attachment_id=$5 AND r.id=$6)`, g.lease, g.s.now(), st.Auth.TenantMembershipID, st.Auth.ProjectMembershipID, g.attachment, g.revision).Scan(&valid)
		if e != nil {
			return e
		}
		if !valid {
			return fault("file_not_found", 404)
		}
		_, e = tx.Exec(ctx, `UPDATE file_read_leases SET expires_at=$2 WHERE id=$1`, g.lease, g.s.now().Add(g.s.options.ReadLease))
		if e != nil {
			return e
		}
		if e = g.transfer.HeartbeatTx(ctx, tx); e != nil {
			return e
		}
		if fn != nil {
			return fn()
		}
		return nil
	})
}
func (g *GuardedRead) Authorize(ctx context.Context) error { return g.boundary(ctx, nil) }

// Respond guards header-only responses (HEAD, conditions and range errors) too.
func (g *GuardedRead) Respond(ctx context.Context, fn func() error) error {
	e := g.boundary(ctx, fn)
	if e == nil {
		g.transfer.Headers()
	}
	return e
}

// Write transmits at most 64 KiB per authorization transaction. The transport
// callback must impose the two-second write deadline before writing its chunk.
func (g *GuardedRead) Write(ctx context.Context, offset, length int64, write func([]byte) (int, error)) error {
	if offset < 0 || length < 0 || offset > g.Size || length > g.Size-offset {
		return fault("invalid_range", 416)
	}
	if _, e := g.staged.Seek(offset, 0); e != nil {
		return e
	}
	buf := make([]byte, g.s.options.ChunkBytes)
	for length > 0 {
		n, e := io.ReadFull(g.staged, buf[:min(int64(len(buf)), length)])
		if e != nil {
			return e
		}
		e = g.boundary(ctx, func() error {
			sent, e := write(buf[:n])
			g.bytes += int64(sent)
			g.transfer.Add(sent)
			if e == nil && sent != n {
				return io.ErrShortWrite
			}
			return e
		})
		if e != nil {
			g.outcome = "authorization_or_transport_ended"
			g.transfer.End(e)
			return e
		}
		length -= int64(n)
	}
	g.outcome = "completed"
	g.transfer.End(nil)
	return nil
}
func (g *GuardedRead) markIntegrity(state string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = g.s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, _, e := protocolTx(ctx, tx); e != nil {
			return e
		}
		// The source and preview have separate integrity outcomes. A missing optional
		// preview never corrupts an already verified original.
		if _, e := tx.Exec(ctx, `SELECT id FROM file_records WHERE id=$1 FOR UPDATE`, g.fileID); e != nil {
			return e
		}
		if g.Variant == "source" {
			if _, e := tx.Exec(ctx, `UPDATE file_records SET integrity=$2 WHERE id=$1`, g.fileID, state); e != nil {
				return e
			}
		}
		_, e := tx.Exec(ctx, `UPDATE file_objects SET state=$2,revision=revision+1,last_error_code=$2 WHERE id=$1 AND state='available'`, g.objectID, state)
		return e
	})
}
func (g *GuardedRead) Close() error {
	g.once.Do(func() {
		g.cancel()
		if g.heartbeatDone != nil {
			<-g.heartbeatDone
		}
		if g.release != nil {
			g.release()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		g.closeErr = g.s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE file_read_leases SET closed_at=$2,outcome=$3,bytes_sent=$4 WHERE id=$1 AND closed_at IS NULL`, g.lease, g.s.now(), g.outcome, g.bytes)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE file_budget_reservations SET state='released',reconciled_at=$2 WHERE owner_kind='read' AND owner_id=$1 AND kind='download'`, g.lease, g.s.now())
			if e != nil {
				return e
			}
			return g.transfer.CloseTx(ctx, tx)
		})
		g.transfer.Diagnose(g.closeErr)
	})
	return g.closeErr
}
