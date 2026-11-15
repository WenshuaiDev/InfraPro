package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"
	"image"
	"io"
	"io/fs"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"time"
)

type claim struct {
	ID, SpaceID, HostID, Generation, Epoch, Owner, Actor, TenantMember, ProjectMember string
	Scope                                                                             Scope
	Fence                                                                             int64
	Attempt                                                                           int
	Input                                                                             frozenInput
}

func (s *Service) claim(ctx context.Context) (*claim, error) {
	var c *claim
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		epoch, mode, e := protocolTx(ctx, tx)
		if e != nil || mode != "online" {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT key FROM file_budget_states WHERE key='instance' FOR UPDATE`); e != nil {
			return e
		}
		v := claim{Epoch: epoch, Owner: uuid.NewString()}
		var raw []byte
		var deadline time.Time
		e = tx.QueryRow(ctx, `SELECT j.id::text,j.space_id::text,j.host_id::text,j.generation::text,sp.scope_kind,sp.tenant_id::text,COALESCE(sp.project_id::text,''),r.requested_by::text,r.tenant_membership_id::text,COALESCE(r.project_membership_id::text,''),r.input_snapshot,r.fence,r.attempt_no,r.deadline_at FROM file_job_runs r JOIN file_jobs j ON j.id=r.job_id AND j.generation=r.generation JOIN file_spaces sp ON sp.id=j.space_id WHERE r.restore_epoch=$1 AND ((r.state IN ('queued','retry_wait') AND r.available_at<=$2) OR (r.state='running' AND r.lease_until<=$2)) AND (SELECT count(*) FROM file_job_runs ar JOIN file_jobs aj ON aj.id=ar.job_id AND aj.generation=ar.generation JOIN file_spaces asp ON asp.id=aj.space_id WHERE asp.tenant_id=sp.tenant_id AND ar.state='running' AND ar.lease_until>$2)<$3 ORDER BY r.available_at,j.id LIMIT 1 FOR UPDATE OF r,j SKIP LOCKED`, epoch, s.now(), s.options.MaxRunningPerTenant).Scan(&v.ID, &v.SpaceID, &v.HostID, &v.Generation, &v.Scope.Kind, &v.Scope.TenantID, &v.Scope.ProjectID, &v.Actor, &v.TenantMember, &v.ProjectMember, &raw, &v.Fence, &v.Attempt, &deadline)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		v.Input, e = decodeInput(raw)
		if e != nil {
			return e
		}
		if !s.now().Before(deadline) || v.Attempt >= s.options.MaxAttempts {
			return s.finishTx(ctx, tx, v.ID, v.Generation, "failed", "execution_expired", v.Actor, "", "", "", true)
		}
		now := s.now()
		v.Fence++
		v.Attempt++
		_, e = tx.Exec(ctx, `UPDATE file_job_attempts SET finished_at=$3,outcome='superseded',error_code='lease_expired' WHERE job_id=$1 AND generation=$2 AND finished_at IS NULL`, v.ID, v.Generation, now)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_job_runs SET state='running',attempt_no=$3,fence=$4,lease_owner=$5,lease_until=$6,started_at=COALESCE(started_at,$7) WHERE job_id=$1 AND generation=$2`, v.ID, v.Generation, v.Attempt, v.Fence, v.Owner, now.Add(s.options.Lease), now)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO file_job_attempts(space_id,job_id,generation,attempt_no,fence,started_at,outcome) VALUES($1,$2,$3,$4,$5,$6,'running')`, v.SpaceID, v.ID, v.Generation, v.Attempt, v.Fence, now)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_jobs SET state='running',phase='preparing',error_code='',source_revision=source_revision+1,observed_at=$2,progress_completed=0,progress_total=$3 WHERE id=$1`, v.ID, now, v.Input.Size)
		if e != nil {
			return e
		}
		if _, e = s.projectJobTx(ctx, tx, v.ID, "", "claimed", ""); e != nil {
			return e
		}
		c = &v
		return nil
	})
	return c, e
}
func (c claim) actor() access.Actor {
	return access.Actor{ID: c.Actor, Check: func(ctx context.Context, tx pgx.Tx, _ bool) error {
		var valid bool
		e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accounts a JOIN tenant_memberships tm ON tm.account_id=a.id WHERE a.id=$1 AND a.status='active' AND tm.id=$2 AND tm.status='active' AND ($3::uuid IS NULL OR EXISTS(SELECT 1 FROM project_memberships pm WHERE pm.id=$3 AND pm.account_id=a.id AND pm.tenant_membership_id=tm.id AND pm.status='active')))`, c.Actor, c.TenantMember, taskrecord.Null(c.ProjectMember)).Scan(&valid)
		if e != nil {
			return e
		}
		if !valid {
			return access.ErrDenied
		}
		return nil
	}}
}
func (s *Service) withClaim(ctx context.Context, c claim, fn func(pgx.Tx, txState) error) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		st := txState{SpaceID: c.SpaceID, HostID: c.HostID}
		var e error
		st.Epoch, st.Mode, e = protocolTx(ctx, tx)
		if e != nil {
			return e
		}
		if st.Epoch != c.Epoch || st.Mode != "online" {
			return fault("worker_fenced", 409)
		}
		st.AuthRevision, e = access.Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `SELECT key FROM file_budget_states WHERE key='instance' FOR UPDATE`); e != nil {
			return e
		}
		// Personnel reads lock the account before the space. Preserve that order
		// for producer foreign keys while holding the shared write budget.
		if _, e = tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR KEY SHARE`, c.Actor); e != nil {
			return e
		}
		// Host row locks still serialize lifecycle changes when the old delegation
		// is no longer authorized; technical terminal facts must remain recordable.
		if c.Scope.Kind == "project" {
			_, e = tx.Exec(ctx, `SELECT id FROM projects WHERE tenant_id=$1 AND id=$2 FOR SHARE`, c.Scope.TenantID, c.Scope.ProjectID)
		} else {
			_, e = tx.Exec(ctx, `SELECT id FROM tenants WHERE id=$1 FOR SHARE`, c.Scope.TenantID)
		}
		if e != nil {
			return e
		}
		if e = tx.QueryRow(ctx, `SELECT content_revision::text FROM file_spaces WHERE id=$1 FOR UPDATE`, c.SpaceID).Scan(&st.ContentRevision); e != nil {
			return e
		}
		var state string
		e = tx.QueryRow(ctx, `SELECT r.state FROM file_job_runs r JOIN file_jobs j ON j.id=r.job_id AND j.generation=r.generation WHERE r.job_id=$1 AND r.generation=$2 AND r.fence=$3 AND r.lease_owner=$4 AND r.lease_until>$5 FOR UPDATE OF j,r`, c.ID, c.Generation, c.Fence, c.Owner, s.now()).Scan(&state)
		if e == pgx.ErrNoRows || state != "running" {
			return fault("worker_fenced", 409)
		}
		if e != nil {
			return e
		}
		st.Auth.Scope = c.Scope
		st.Auth.Host = c.Scope.Host()
		st.Auth.AccountID = c.Actor
		return fn(tx, st)
	})
}
func (s *Service) delegationTx(ctx context.Context, tx pgx.Tx, c claim) (HostAuthorization, error) {
	a := c.actor()
	if e := a.Check(ctx, tx, false); e != nil {
		return HostAuthorization{}, e
	}
	auth, e := s.hosts.ResolveTx(ctx, tx, a, c.Scope, c.Scope.Host(), Metadata)
	if e != nil {
		return auth, e
	}
	if !auth.CanManage || auth.TenantMembershipID != c.TenantMember || auth.ProjectMembershipID != c.ProjectMember {
		return auth, access.ErrDenied
	}
	return auth, nil
}
func (s *Service) phase(ctx context.Context, c claim, phase string) error {
	return s.withClaim(ctx, c, func(tx pgx.Tx, _ txState) error {
		_, e := tx.Exec(ctx, `UPDATE file_jobs SET phase=$2,source_revision=source_revision+1,observed_at=$3,progress_completed=0 WHERE id=$1`, c.ID, phase, s.now())
		if e != nil {
			return e
		}
		_, e = s.projectJobTx(ctx, tx, c.ID, "", "phase", "")
		return e
	})
}

// RunOnce executes at most one real file job. All object IO and parsing occur
// outside database transactions. A heartbeat never extends business authority.
func (s *Service) RunOnce(ctx context.Context) (bool, error) {
	c, e := s.claim(ctx)
	if e != nil || c == nil {
		return false, e
	}
	attemptCtx, cancel := context.WithTimeout(ctx, s.options.ExecutionTimeout)
	defer cancel()
	defer diagnosticreport.TrackDeadline(ctx, attemptCtx)()
	done := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(s.options.Heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-attemptCtx.Done():
				return
			case <-ticker.C:
				e := s.withClaim(attemptCtx, *c, func(tx pgx.Tx, _ txState) error {
					_, e := tx.Exec(attemptCtx, `UPDATE file_job_runs SET lease_until=$3 WHERE job_id=$1 AND generation=$2`, c.ID, c.Generation, s.now().Add(s.options.Lease))
					return e
				})
				if e != nil {
					cancel()
					return
				}
			}
		}
	}()
	file, e := s.prepare(attemptCtx, *c)
	if e == nil {
		e = s.bind(attemptCtx, *c, file)
	}
	close(done)
	<-heartbeatDone
	if e != nil {
		finishCtx, finishCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer finishCancel()
		var fe *Error
		if errors.As(e, &fe) && fe.Code == "worker_fenced" {
			return true, nil
		}
		finishErr := s.failAttempt(finishCtx, *c, e, file)
		if finishErr != nil {
			return true, finishErr
		}
	}
	return true, nil
}
func (s *Service) prepare(ctx context.Context, c claim) (string, error) {
	var existing string
	e := s.withClaim(ctx, c, func(tx pgx.Tx, st txState) error {
		if _, e := s.delegationTx(ctx, tx, c); e != nil {
			return e
		}
		var until *time.Time
		var state, integrity string
		e := tx.QueryRow(ctx, `SELECT id::text,unbound_until,lifecycle,integrity FROM file_records WHERE upload_id=$1`, c.Input.UploadID).Scan(&existing, &until, &state, &integrity)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		if state != "ready" || integrity != "ok" {
			return fault("file_missing", 503)
		}
		if until == nil || !s.now().Before(*until) {
			return fault("prepared_file_expired", 410)
		}
		return s.lockReadyTx(ctx, tx, st.SpaceID, st.HostID, existing)
	})
	if e != nil {
		return existing, e
	}
	if existing != "" {
		return existing, nil
	}
	if c.Input.PreparedFileID != "" {
		return "", fault("file_missing", 503)
	}
	if s.objects == nil {
		return "", fault("file_storage_unavailable", 503)
	}
	if s.checker == nil {
		return "", fault("file_worker_unavailable", 503)
	}
	staged, release, e := s.stage(ctx, c.Scope, c.SpaceID, c.Actor, c.Owner, c.Input.Size+16<<20, "job")
	if e != nil {
		return "", e
	}
	defer release()
	full := sha256.New()
	var total int64
	for _, part := range c.Input.Parts {
		r, e := s.objects.Get(ctx, "files/v1/"+c.SpaceID+"/"+part.ObjectID)
		if e != nil {
			if errors.Is(e, fs.ErrNotExist) {
				return "", fault("file_missing", 503)
			}
			return "", fault("file_storage_unavailable", 503)
		}
		hash := sha256.New()
		n, e := io.Copy(io.MultiWriter(staged, full, hash), io.LimitReader(r, part.Size+1))
		closeErr := r.Close()
		if e != nil || closeErr != nil {
			return "", fault("file_storage_unavailable", 503)
		}
		if n != part.Size || hex.EncodeToString(hash.Sum(nil)) != part.SHA256 {
			return "", fault("file_integrity_mismatch", 422)
		}
		total += n
	}
	if total != c.Input.Size || hex.EncodeToString(full.Sum(nil)) != c.Input.SHA256 {
		return "", fault("file_integrity_mismatch", 422)
	}
	if e = staged.Sync(); e != nil {
		return "", e
	}
	if e = s.phase(ctx, c, "validating"); e != nil {
		return "", e
	}
	dir, e := os.MkdirTemp(s.options.TempDirectory, "attempt-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(dir)
	if e = os.Link(staged.Name(), filepath.Join(dir, "input")); e != nil {
		return "", e
	}
	request := CheckRequest{Protocol: 1, Operation: "validate", RequestID: uuid.NewString(), JobID: c.ID, Generation: c.Generation, Fence: number(c.Fence), Nonce: uuid.NewString(), Directory: filepath.Base(dir), Input: "input", Output: "preview", Size: c.Input.Size, SHA256: c.Input.SHA256, Format: c.Input.Format, Policy: c.Input.CheckPolicy}
	checkCtx, checkCancel := context.WithTimeout(ctx, s.options.CheckerTimeout)
	result, e := s.checker.Check(checkCtx, request)
	checkCancel()
	if e != nil {
		return "", fault("file_worker_unavailable", 503)
	}
	if result.Status != "ok" {
		return "", fault(result.Code, 422)
	}
	previewState, previewReason := "none", ""
	var preview *CheckResult
	if c.Input.Format == "png" || c.Input.Format == "jpeg" {
		request.Operation = "preview"
		request.RequestID = uuid.NewString()
		previewCtx, previewCancel := context.WithTimeout(ctx, s.options.CheckerTimeout)
		r, err := s.checker.Check(previewCtx, request)
		previewCancel()
		previewState = "unavailable"
		previewReason = "preview_unavailable"
		if err == nil && r.Status == "ok" && r.Preview == "preview" && r.Size > 0 && r.Size <= c.Input.CheckPolicy.MaxBytes && r.Width > 0 && r.Height > 0 && r.Width <= c.Input.CheckPolicy.Edge && r.Height <= c.Input.CheckPolicy.Edge && digestPattern.MatchString(r.SHA256) {
			preview = &r
			previewState = "available"
			previewReason = ""
		} else if err == nil && r.Code != "" {
			previewReason = r.Code
		}
	}
	if e = s.phase(ctx, c, "writing"); e != nil {
		return "", e
	}
	source, e := s.writeOutput(ctx, c, "source", staged, c.Input.Size, c.Input.SHA256, c.Input.MediaType)
	if e != nil {
		return "", e
	}
	var previewObject string
	if preview != nil {
		previewPath := filepath.Join(dir, "preview")
		info, statErr := os.Lstat(previewPath)
		var p *os.File
		err := statErr
		if err == nil {
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				err = fault("preview_invalid", 422)
			} else {
				p, err = os.Open(previewPath)
			}
		}
		if err == nil {
			cfg, format, checkErr := image.DecodeConfig(io.LimitReader(p, c.Input.CheckPolicy.MaxBytes+1))
			if checkErr != nil || format != c.Input.Format || cfg.Width != preview.Width || cfg.Height != preview.Height {
				p.Close()
				p = nil
				err = fault("preview_invalid", 422)
			}
		}
		if err == nil {
			previewObject, err = s.writeOutput(ctx, c, "preview", p, preview.Size, preview.SHA256, c.Input.MediaType)
			p.Close()
		}
		if err != nil {
			previewState = "unavailable"
			previewReason = "preview_storage_unavailable"
			previewObject = ""
		}
	}
	fileID := uuid.NewString()
	e = s.withClaim(ctx, c, func(tx pgx.Tx, st txState) error {
		now := s.now()
		until := now.Add(s.options.PreparedTTL)
		_, e := tx.Exec(ctx, `INSERT INTO file_records(id,space_id,host_id,upload_id,original_name,format,media_type,size_bytes,sha256,source_object_id,preview_object_id,preview_state,preview_reason,verified_at,ready_at,unbound_until) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14,$15)`, fileID, c.SpaceID, c.HostID, c.Input.UploadID, c.Input.Filename, c.Input.Format, c.Input.MediaType, c.Input.Size, c.Input.SHA256, source, taskrecord.Null(previewObject), previewState, previewReason, now, until)
		if e != nil {
			return e
		}
		return s.ProtectTx(ctx, tx, Protection{SpaceID: st.SpaceID, HostID: st.HostID, FileID: fileID, Module: "files", OwnerKind: "upload", OwnerID: c.Input.UploadID, OwnerRevision: "1", Kind: "prepared_input", ExpiresAt: &until})
	})
	return fileID, e
}
func (s *Service) writeOutput(ctx context.Context, c claim, kind string, reader *os.File, size int64, hash, media string) (string, error) {
	// The trusted parent independently verifies checker outputs before accepting
	// their descriptors or writing any object.
	if _, e := reader.Seek(0, 0); e != nil {
		return "", e
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(reader, size+1))
	if e != nil || n != size || hex.EncodeToString(h.Sum(nil)) != hash {
		return "", fault("file_integrity_mismatch", 422)
	}
	if _, e = reader.Seek(0, 0); e != nil {
		return "", e
	}
	id := uuid.NewString()
	key := "files/v1/" + c.SpaceID + "/" + id
	e = s.withClaim(ctx, c, func(tx pgx.Tx, _ txState) error {
		if e := s.objectBudgetTx(ctx, tx, c.Input.UploadID, size); e != nil {
			return e
		}
		deadline := s.now().Add(s.options.ObjectIOTimeout)
		_, e := tx.Exec(ctx, `INSERT INTO file_objects(id,space_id,kind,object_key,content_type,expected_size,expected_sha256,state,writer_kind,writer_id,writer_generation,writer_fence,writer_epoch,io_deadline,lease_until) VALUES($1,$2,$3,$4,$5,$6,$7,'writing','job',$8,$9,$10,$11,$12,$12)`, id, c.SpaceID, kind, key, media, size, hash, c.ID, c.Generation, c.Fence, c.Epoch, deadline)
		return e
	})
	if e != nil {
		return "", e
	}
	ioctx, cancel := context.WithTimeout(ctx, s.options.ObjectIOTimeout)
	e = s.objects.Put(ioctx, key, reader, size, media)
	cancel()
	if e != nil {
		return "", fault("file_storage_unavailable", 503)
	}
	verifyCtx, verifyCancel := context.WithTimeout(ctx, s.options.ObjectIOTimeout)
	body, verifyErr := s.objects.Get(verifyCtx, key)
	if verifyErr == nil {
		verified := sha256.New()
		actual, readErr := io.Copy(verified, io.LimitReader(body, size+1))
		closeErr := body.Close()
		if readErr != nil || closeErr != nil {
			verifyErr = fault("file_storage_unavailable", 503)
		} else if actual != size || hex.EncodeToString(verified.Sum(nil)) != hash {
			verifyErr = fault("file_integrity_mismatch", 422)
		}
	}
	verifyCancel()
	if verifyErr != nil {
		return "", verifyErr
	}
	e = s.withClaim(ctx, c, func(tx pgx.Tx, _ txState) error {
		_, e := tx.Exec(ctx, `UPDATE file_objects SET state='available',actual_size=expected_size,actual_sha256=expected_sha256,last_verified_at=$2,revision=revision+1,updated_at=$2 WHERE id=$1 AND state='writing'`, id, s.now())
		return e
	})
	return id, e
}
func (s *Service) bind(ctx context.Context, c claim, file string) error {
	if e := s.phase(ctx, c, "sealing"); e != nil {
		return e
	}
	return s.withClaim(ctx, c, func(tx pgx.Tx, st txState) error {
		auth, err := s.delegationTx(ctx, tx, c)
		if err != nil {
			var ae *access.Error
			if !errors.As(err, &ae) {
				return err
			}
			return s.finishTx(ctx, tx, c.ID, c.Generation, "needs_action", "authorization_changed", c.Actor, file, "", "", true)
		}
		st.Auth = auth
		var old AttachmentDetail
		if c.Input.Intent == "replace" {
			old, err = attachmentTx(ctx, tx, st, value(c.Input.AttachmentID), true)
			if err != nil {
				var fe *Error
				if !errors.As(err, &fe) {
					return err
				}
			}
			if err != nil || old.Status != "active" || old.Revision != value(c.Input.ExpectedRevision) {
				return s.finishTx(ctx, tx, c.ID, c.Generation, "needs_action", "attachment_revision_changed", c.Actor, file, "", "", true)
			}
		}
		if err = s.lockReadyTx(ctx, tx, c.SpaceID, c.HostID, file); err != nil {
			return err
		}
		var until *time.Time
		if err = tx.QueryRow(ctx, `SELECT unbound_until FROM file_records WHERE id=$1`, file).Scan(&until); err != nil {
			return err
		}
		if until == nil || !s.now().Before(*until) {
			return fault("prepared_file_expired", 410)
		}
		attachment, revisionID, no, operation := old.ID, uuid.NewString(), int64(1), "upload"
		now := s.now()
		if attachment == "" {
			attachment = uuid.NewString()
			_, err = tx.Exec(ctx, `INSERT INTO file_attachments(id,space_id,host_id,display_name,description,current_revision_id,created_by,updated_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$7,$8,$8)`, attachment, c.SpaceID, c.HostID, c.Input.DisplayName, c.Input.Description, revisionID, c.Actor, now)
		} else {
			operation = "replace"
			err = tx.QueryRow(ctx, `UPDATE file_attachments SET current_revision_id=$2,next_revision_no=next_revision_no+1,revision=revision+1,updated_by=$3,updated_at=$4 WHERE id=$1 RETURNING next_revision_no-1`, attachment, revisionID, c.Actor, now).Scan(&no)
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO file_attachment_revisions(id,space_id,host_id,attachment_id,revision_no,file_id,operation,previous_revision_id,display_name_snapshot,reason,actor_id,job_id,job_generation,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, revisionID, c.SpaceID, c.HostID, attachment, no, file, operation, taskrecord.Null(old.CurrentRevision.ID), c.Input.DisplayName, c.Input.Reason, c.Actor, c.ID, c.Generation, now)
		if err != nil {
			return err
		}
		if err = s.ProtectTx(ctx, tx, Protection{SpaceID: c.SpaceID, HostID: c.HostID, FileID: file, Module: "files", OwnerKind: "revision", OwnerID: revisionID, OwnerRevision: number(no), Kind: "attachment_revision"}); err != nil {
			return err
		}
		if err = s.ReleaseTx(ctx, tx, Release{Protection: Protection{SpaceID: c.SpaceID, HostID: c.HostID, FileID: file, Module: "files", OwnerKind: "upload", OwnerID: c.Input.UploadID, OwnerRevision: "1", Kind: "prepared_input"}, Reason: "附件绑定完成"}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE file_records SET unbound_until=NULL WHERE id=$1`, file)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE file_uploads SET state='closed',revision=revision+1,updated_at=$2 WHERE id=$1`, c.Input.UploadID, now)
		if err != nil {
			return err
		}
		if err = s.finishTx(ctx, tx, c.ID, c.Generation, "succeeded", "", c.Actor, file, attachment, revisionID, true); err != nil {
			return err
		}
		updated, e := attachmentTx(ctx, tx, st, attachment, false)
		if e != nil {
			return e
		}
		var previous any
		if old.ID != "" {
			previous = old
		}
		return s.audit(ctx, tx, st, c.Actor, WriteMeta{Reason: c.Input.Reason, RequestID: c.Input.UploadID, auditBefore: previous, auditAfter: updated, auditWorker: true, auditJob: c.ID, auditGeneration: c.Generation}, "attachment.bound", attachment, map[string]string{"job_id": c.ID, "generation": c.Generation, "file_id": file, "revision_id": revisionID})
	})
}
func (s *Service) failAttempt(ctx context.Context, c claim, cause error, file string) error {
	return s.withClaim(ctx, c, func(tx pgx.Tx, _ txState) error {
		code := "file_processing_failed"
		var fe *Error
		var ae *access.Error
		if errors.As(cause, &fe) {
			code = fe.Code
		}
		if errors.Is(cause, context.Canceled) {
			code = "worker_interrupted"
		}
		if errors.As(cause, &ae) {
			code = "authorization_changed"
		}
		if (storageFailure(cause) || errors.Is(cause, context.Canceled)) && c.Attempt < s.options.MaxAttempts {
			delay := []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second}[min(c.Attempt-1, 3)]
			delay += time.Duration(rand.Int64N(int64(delay/4) + 1))
			_, e := tx.Exec(ctx, `UPDATE file_job_runs SET state='retry_wait',lease_until=NULL,available_at=$3 WHERE job_id=$1 AND generation=$2`, c.ID, c.Generation, s.now().Add(delay))
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE file_jobs SET state='retry_wait',phase='waiting',error_code=$2,source_revision=source_revision+1,observed_at=$3 WHERE id=$1`, c.ID, code, s.now())
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE file_job_attempts SET outcome='retry_wait',error_code=$3,finished_at=$4 WHERE job_id=$1 AND generation=$2 AND finished_at IS NULL`, c.ID, c.Generation, code, s.now())
			if e != nil {
				return e
			}
			_, e = s.projectJobTx(ctx, tx, c.ID, "", "retry_wait", code)
			return e
		}
		state := "failed"
		if code == "authorization_changed" && file != "" {
			state = "needs_action"
		}
		return s.finishTx(ctx, tx, c.ID, c.Generation, state, code, c.Actor, file, "", "", true)
	})
}
func (s *Service) WorkerHeartbeat(ctx context.Context, worker string, checkerAvailable bool) error {
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		var checked *time.Time
		now := s.now()
		if checkerAvailable {
			checked = &now
		}
		_, e := tx.Exec(ctx, `INSERT INTO file_worker_heartbeats(worker_id,node_id,build_version,protocol_version,started_at,last_seen_at,checker_last_seen_at,capabilities) VALUES($1,$2,'files/1',1,$3,$3,$4,'{"prepare":true}') ON CONFLICT(worker_id) DO UPDATE SET last_seen_at=EXCLUDED.last_seen_at,checker_last_seen_at=EXCLUDED.checker_last_seen_at`, worker, s.options.NodeID, now, checked)
		return e
	})
}
func (s *Service) checkerAvailable(ctx context.Context) bool {
	if socket, ok := s.checker.(SocketChecker); ok {
		dial := net.Dialer{Timeout: 200 * time.Millisecond}
		conn, e := dial.DialContext(ctx, "unix", socket.Socket)
		if e != nil {
			return false
		}
		conn.Close()
		return true
	}
	return s.checker != nil
}
func (s *Service) RunWorker(ctx context.Context, worker string, reporters ...diagnosticreport.Sink) error {
	if !validText(worker, 1, 100) {
		return fmt.Errorf("invalid worker identity")
	}
	reporter := diagnosticreport.Optional(reporters)
	reporter.Idle("cleanup", time.Now().Add(s.options.CleanupInterval))
	prepare := diagnosticreport.NewGroup(reporter, "prepare", s.options.Workers)
	group, workCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		heartbeat := time.NewTicker(s.options.WorkerHeartbeat)
		defer heartbeat.Stop()
		cleanup := time.NewTicker(s.options.CleanupInterval)
		defer cleanup.Stop()
		beat := func() error {
			reporter.Begin("heartbeat", diagnosticreport.Deadline(workCtx))
			e := s.WorkerHeartbeat(workCtx, worker, s.checkerAvailable(workCtx))
			reporter.End("heartbeat", diagnosticreport.Result(e), time.Now().Add(s.options.WorkerHeartbeat))
			reporter.Dependency("postgres", e == nil, diagnosticreport.Result(e))
			return e
		}
		if e := beat(); e != nil {
			return e
		}
		for {
			select {
			case <-workCtx.Done():
				return nil
			case <-heartbeat.C:
				if e := beat(); e != nil {
					return e
				}
			case <-cleanup.C:
				reporter.Begin("cleanup", diagnosticreport.Deadline(workCtx))
				e := func() error {
					if e := s.Sweep(workCtx); e != nil {
						return e
					}
					plan, e := s.CleanupPlan(workCtx, worker, "定期文件生命周期维护")
					if e != nil {
						return e
					}
					_, e = s.CleanupApply(workCtx, plan.ID, "定期执行已核对文件清理计划")
					return e
				}()
				reporter.End("cleanup", diagnosticreport.Result(e), time.Now().Add(s.options.CleanupInterval))
				var fe *Error
				if errors.As(e, &fe) && fe.Code == "file_protocol_unavailable" {
					continue
				}
				if e != nil {
					return e
				}
			}
		}
	})
	for n := 0; n < s.options.Workers; n++ {
		group.Go(func() error {
			for {
				if workCtx.Err() != nil {
					return nil
				}
				prepare.Begin(n, diagnosticreport.Deadline(workCtx))
				worked, e := s.RunOnce(prepare.Context(workCtx, n))
				next := time.Now()
				if !worked {
					next = next.Add(time.Second)
				}
				prepare.End(n, diagnosticreport.Result(e), next)
				if e != nil {
					if workCtx.Err() != nil {
						return nil
					}
					return e
				}
				if !worked {
					select {
					case <-workCtx.Done():
						return nil
					case <-time.After(time.Second):
					}
				}
			}
		})
	}
	return group.Wait()
}
