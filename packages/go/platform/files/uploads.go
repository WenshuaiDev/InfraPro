package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"time"
)

func (s *Service) StartUpload(ctx context.Context, a access.Actor, scope Scope, in StartUploadInput) (CommandResult, error) {
	var out CommandResult
	if e := validateMeta(in.WriteMeta); e != nil {
		return out, e
	}
	size, e := integer(in.SizeBytes, 1)
	if e != nil {
		return out, e
	}
	format, e := fileFormat(in.Filename, in.DeclaredMediaType)
	if e != nil {
		return out, e
	}
	if in.Host != scope.Host() || !digestPattern.MatchString(in.SHA256) || !validText(in.DisplayName, 1, 200) || !validText(in.Description, 0, 1000) || (in.Intent != "add" && in.Intent != "replace") {
		return out, fault("invalid_request", 400)
	}
	if in.Intent == "replace" {
		if !access.ValidID(value(in.AttachmentID)) {
			return out, fault("invalid_request", 400)
		}
		if _, e = integer(value(in.ExpectedAttachmentRevision), 1); e != nil {
			return out, e
		}
	} else if in.AttachmentID != nil || in.ExpectedAttachmentRevision != nil {
		return out, fault("invalid_request", 400)
	}
	e = s.transact(ctx, a, scope, true, true, in.Epoch, func(tx pgx.Tx, st txState) error {
		var replay bool
		var e error
		out, replay, e = receiptTx(ctx, tx, st, a, in.WriteMeta, "upload.create", in)
		if replay || e != nil {
			return e
		}
		if size > s.options.MaxFileBytes {
			return fault("file_too_large", 413)
		}
		if e = st.mutable(); e != nil {
			return e
		}
		if !s.options.AcceptUploads {
			return fault("uploads_disabled", 409)
		}
		if e = s.rateTx(ctx, tx, a.ID, "create", s.options.CreateRequestsPerMinute); e != nil {
			return e
		}
		if in.Intent == "replace" {
			var revision, status string
			e = tx.QueryRow(ctx, `SELECT revision::text,status FROM file_attachments WHERE space_id=$1 AND host_id=$2 AND id=$3 FOR UPDATE`, st.SpaceID, st.HostID, *in.AttachmentID).Scan(&revision, &status)
			if e != nil {
				return notFound(e)
			}
			if status != "active" {
				return fault("attachment_revision_changed", 409)
			}
			if e = sameVersion(revision, *in.ExpectedAttachmentRevision); e != nil {
				return e
			}
		}
		var accountCount, tenantCount int
		e = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE u.actor_id=$2),count(*) FROM file_uploads u JOIN file_spaces sp ON sp.id=u.space_id WHERE sp.tenant_id=$1 AND u.state='receiving' AND u.idle_expires_at>$3 AND u.hard_expires_at>$3`, scope.TenantID, a.ID, s.now()).Scan(&accountCount, &tenantCount)
		if e != nil {
			return e
		}
		// Account activity is instance-wide, including uploads in other tenants.
		e = tx.QueryRow(ctx, `SELECT count(*) FROM file_uploads WHERE actor_id=$1 AND state='receiving' AND idle_expires_at>$2 AND hard_expires_at>$2`, a.ID, s.now()).Scan(&accountCount)
		if e != nil {
			return e
		}
		if accountCount >= s.options.MaxActivePerAccount || tenantCount >= s.options.MaxActivePerTenant {
			return fault("upload_limit", 429)
		}
		id := uuid.NewString()
		partCount := (size + s.options.PartBytes - 1) / s.options.PartBytes
		now := s.now()
		hard := now.Add(s.options.UploadHardTTL)
		policy := map[string]any{"schema_version": 1, "max_file_bytes": s.options.MaxFileBytes, "part_bytes": s.options.PartBytes, "format": format, "preview_max_pixels": s.options.PreviewMaxPixels, "preview_edge": s.options.PreviewEdge, "preview_bytes": s.options.PreviewBytes}
		_, e = tx.Exec(ctx, `INSERT INTO file_uploads(id,space_id,host_id,actor_id,tenant_membership_id,project_membership_id,intent,attachment_id,expected_attachment_revision,filename,display_name,description,expected_size,expected_sha256,declared_media_type,detected_format,part_size,part_count,policy_snapshot,restore_epoch,reason,idle_expires_at,hard_expires_at,request_id,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$25)`, id, st.SpaceID, st.HostID, a.ID, st.Auth.TenantMembershipID, taskrecord.Null(st.Auth.ProjectMembershipID), in.Intent, in.AttachmentID, in.ExpectedAttachmentRevision, in.Filename, in.DisplayName, in.Description, size, in.SHA256, in.DeclaredMediaType, format, s.options.PartBytes, partCount, taskrecord.JSON(policy), st.Epoch, in.Reason, minTime(now.Add(s.options.UploadIdleTTL), hard), hard, in.RequestID, now)
		if e != nil {
			return e
		}
		if e = s.reserveObjectsTx(ctx, tx, st, id, size*2+16<<20, hard); e != nil {
			return e
		}
		out = CommandResult{UploadID: &id, Revision: "1", State: "receiving", Changed: true}
		return s.commitReceipt(ctx, tx, st, a, in.WriteMeta, "upload.create", id, in, out)
	})
	return out, e
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

type upload struct {
	UploadDetail
	SpaceID, HostID, ActorID, TenantMember, ProjectMember, Format, MediaType, Reason string
	PolicyRaw                                                                        []byte
	HardExpiry                                                                       time.Time
	Size, PartBytes                                                                  int64
}

func uploadTx(ctx context.Context, tx pgx.Tx, space, id string, lock bool) (upload, error) {
	var u upload
	if space == "" || !access.ValidID(id) {
		return u, fault("file_not_found", 404)
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF u"
	}
	e := tx.QueryRow(ctx, `SELECT u.id::text,u.space_id::text,u.host_id::text,h.kind,h.object_id::text,u.actor_id::text,u.tenant_membership_id::text,COALESCE(u.project_membership_id::text,''),u.intent,u.attachment_id::text,u.expected_attachment_revision::text,u.filename,u.display_name,u.description,u.expected_size,u.expected_sha256,u.part_size,u.part_count,u.state,u.revision::text,u.restore_epoch::text,LEAST(u.idle_expires_at,u.hard_expires_at),u.hard_expires_at,u.created_at,u.detected_format,u.declared_media_type,u.reason,u.policy_snapshot FROM file_uploads u JOIN file_hosts h ON h.id=u.host_id WHERE u.space_id=$1 AND u.id=$2`+suffix, space, id).Scan(&u.ID, &u.SpaceID, &u.HostID, &u.Host.Kind, &u.Host.ID, &u.ActorID, &u.TenantMember, &u.ProjectMember, &u.Intent, &u.AttachmentID, &u.ExpectedAttachmentRevision, &u.Filename, &u.DisplayName, &u.Description, &u.Size, &u.SHA256, &u.PartBytes, &u.PartCount, &u.State, &u.Revision, &u.RestoreEpoch, &u.ExpiresAt, &u.HardExpiry, &u.CreatedAt, &u.Format, &u.MediaType, &u.Reason, &u.PolicyRaw)
	u.SizeBytes = number(u.Size)
	u.PartSize = number(u.PartBytes)
	u.Parts = []PartReceipt{}
	u.AllowedActions = []Action{}
	return u, notFound(e)
}
func (s *Service) receiving(u upload, st txState, a access.Actor) error {
	if u.ActorID != a.ID || u.TenantMember != st.Auth.TenantMembershipID || u.ProjectMember != st.Auth.ProjectMembershipID {
		return access.ErrDenied
	}
	if u.RestoreEpoch != st.Epoch {
		return fault("file_epoch_changed", 409)
	}
	if !s.now().Before(u.ExpiresAt) {
		return fault("upload_expired", 410)
	}
	if u.State != "receiving" {
		if u.State == "submitted" || u.State == "closed" {
			return fault("task_control_required", 409)
		}
		return fault("upload_expired", 410)
	}
	return st.mutable()
}
func (s *Service) PutPart(ctx context.Context, a access.Actor, scope Scope, id string, in PartInput, body io.Reader) (PartReceipt, error) {
	var result PartReceipt
	if in.Part < 1 || in.Size < 1 || !digestPattern.MatchString(in.SHA256) {
		return result, fault("invalid_request", 400)
	}
	ctx, cancel := context.WithTimeout(ctx, s.options.PartRequestTimeout)
	defer cancel()
	objectID, owner := uuid.NewString(), uuid.NewString()
	var key, space string
	var fence int64
	var repeated bool
	e := s.transact(ctx, a, scope, true, false, in.Epoch, func(tx pgx.Tx, st txState) error {
		if e := s.rateTx(ctx, tx, a.ID, "part", s.options.PartRequestsPerMinute); e != nil {
			return e
		}
		u, e := uploadTx(ctx, tx, st.SpaceID, id, true)
		if e != nil {
			return e
		}
		if e = s.receiving(u, st, a); e != nil {
			return e
		}
		if in.Part > u.PartCount {
			return fault("invalid_request", 400)
		}
		size := u.PartBytes
		if in.Part == u.PartCount {
			size = u.Size - int64(in.Part-1)*u.PartBytes
		}
		if size != in.Size {
			return fault("file_integrity_mismatch", 422)
		}
		var hash, state string
		var oldSize int64
		var expiry time.Time
		var received *time.Time
		e = tx.QueryRow(ctx, `SELECT sha256,expected_size,state,fence,lease_until,received_at FROM file_upload_parts WHERE upload_id=$1 AND part_no=$2 FOR UPDATE`, id, in.Part).Scan(&hash, &oldSize, &state, &fence, &expiry, &received)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		if e == nil {
			if hash != in.SHA256 || oldSize != in.Size {
				return fault("part_conflict", 409)
			}
			if state == "received" {
				result = PartReceipt{in.Part, number(in.Size), hash, *received}
				repeated = true
				return nil
			}
			if s.now().Before(expiry) {
				return fault("part_in_progress", 409)
			}
		}
		fence++
		space = st.SpaceID
		key = "files/v1/" + space + "/" + objectID
		if s.objects == nil {
			return fault("file_storage_unavailable", 503)
		}
		if e = s.objectBudgetTx(ctx, tx, id, in.Size); e != nil {
			return e
		}
		deadline := s.now().Add(s.options.ObjectIOTimeout)
		_, e = tx.Exec(ctx, `INSERT INTO file_objects(id,space_id,kind,object_key,content_type,expected_size,expected_sha256,state,writer_kind,writer_id,writer_generation,writer_fence,writer_epoch,io_deadline,lease_until) VALUES($1,$2,'part',$3,'application/octet-stream',$4,$5,'writing','upload',$6,0,$7,$8,$9,$9)`, objectID, space, key, in.Size, in.SHA256, id, fence, st.Epoch, deadline)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO file_upload_parts(space_id,upload_id,part_no,expected_size,sha256,object_id,state,fence,lease_owner,lease_until) VALUES($1,$2,$3,$4,$5,$6,'receiving',$7,$8,$9) ON CONFLICT(upload_id,part_no) DO UPDATE SET object_id=EXCLUDED.object_id,state='receiving',fence=EXCLUDED.fence,lease_owner=EXCLUDED.lease_owner,lease_until=EXCLUDED.lease_until,received_at=NULL`, space, id, in.Part, in.Size, in.SHA256, objectID, fence, owner, deadline)
		return e
	})
	if e != nil {
		return result, e
	}
	// Replays still prove the bytes, rather than trusting the declared digest.
	if repeated {
		h := sha256.New()
		n, e := io.Copy(h, io.LimitReader(body, in.Size+1))
		if e != nil || n != in.Size || hex.EncodeToString(h.Sum(nil)) != in.SHA256 {
			return PartReceipt{}, fault("file_integrity_mismatch", 422)
		}
		return result, nil
	}
	file, release, e := s.stage(ctx, scope, space, a.ID, objectID, in.Size, "object")
	if e != nil {
		return result, e
	}
	defer release()
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(file, h), io.LimitReader(body, in.Size+1))
	if e != nil || n != in.Size || hex.EncodeToString(h.Sum(nil)) != in.SHA256 {
		return result, fault("file_integrity_mismatch", 422)
	}
	if _, e = file.Seek(0, 0); e != nil {
		return result, e
	}
	ioctx, ioCancel := context.WithTimeout(ctx, s.options.ObjectIOTimeout)
	e = s.objects.Put(ioctx, key, file, in.Size, "application/octet-stream")
	ioCancel()
	if e != nil {
		return result, fault("file_storage_unavailable", 503)
	}
	e = s.transact(ctx, a, scope, true, false, in.Epoch, func(tx pgx.Tx, st txState) error {
		u, e := uploadTx(ctx, tx, st.SpaceID, id, true)
		if e != nil {
			return e
		}
		if e = s.receiving(u, st, a); e != nil {
			return e
		}
		now := s.now()
		tag, e := tx.Exec(ctx, `UPDATE file_upload_parts SET state='received',received_at=$5 WHERE upload_id=$1 AND part_no=$2 AND fence=$3 AND lease_owner=$4 AND state='receiving' AND lease_until>$5`, id, in.Part, fence, owner, now)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return fault("part_conflict", 409)
		}
		_, e = tx.Exec(ctx, `UPDATE file_objects SET state='available',actual_size=expected_size,actual_sha256=expected_sha256,last_verified_at=$2,revision=revision+1,updated_at=$2 WHERE id=$1 AND state='writing'`, objectID, now)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE file_uploads SET last_part_at=$2,idle_expires_at=LEAST($3,hard_expires_at),updated_at=$2 WHERE id=$1`, id, now, now.Add(s.options.UploadIdleTTL))
		if e != nil {
			return e
		}
		result = PartReceipt{in.Part, number(in.Size), in.SHA256, now}
		return nil
	})
	return result, e
}

type frozenPart struct {
	ObjectID string `json:"object_id"`
	Part     int    `json:"part"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
}
type frozenInput struct {
	Schema           int          `json:"schema_version"`
	UploadID         string       `json:"upload_id"`
	Host             HostRef      `json:"host"`
	Intent           string       `json:"intent"`
	AttachmentID     *string      `json:"attachment_id"`
	ExpectedRevision *string      `json:"expected_attachment_revision"`
	Filename         string       `json:"filename"`
	DisplayName      string       `json:"display_name"`
	Description      string       `json:"description"`
	Size             int64        `json:"size"`
	SHA256           string       `json:"sha256"`
	Format           string       `json:"format"`
	MediaType        string       `json:"media_type"`
	Parts            []frozenPart `json:"parts"`
	PreparedFileID   string       `json:"prepared_file_id,omitempty"`
	PolicyVersion    int          `json:"policy_version"`
	CheckPolicy      CheckPolicy  `json:"check_policy"`
	Reason           string       `json:"reason"`
}

func frozenPartsTx(ctx context.Context, tx pgx.Tx, id string) ([]frozenPart, error) {
	out := []frozenPart{}
	rows, e := tx.Query(ctx, `SELECT p.part_no,p.expected_size,p.sha256,p.object_id::text FROM file_upload_parts p JOIN file_objects o ON o.id=p.object_id WHERE p.upload_id=$1 AND p.state='received' AND o.state='available' ORDER BY p.part_no`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var v frozenPart
		if e = rows.Scan(&v.Part, &v.Size, &v.SHA256, &v.ObjectID); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) CompleteUpload(ctx context.Context, a access.Actor, scope Scope, id string, in RevisionCommand) (CommandResult, error) {
	var out CommandResult
	if e := validateMeta(in.WriteMeta); e != nil {
		return out, e
	}
	payload := struct {
		ID    string
		Input RevisionCommand
	}{id, in}
	e := s.transact(ctx, a, scope, true, false, in.Epoch, func(tx pgx.Tx, st txState) error {
		var replay bool
		var e error
		out, replay, e = receiptTx(ctx, tx, st, a, in.WriteMeta, "upload.complete", payload)
		if replay || e != nil {
			return e
		}
		u, e := uploadTx(ctx, tx, st.SpaceID, id, true)
		if e != nil {
			return e
		}
		if e = s.receiving(u, st, a); e != nil {
			return e
		}
		if e = sameVersion(u.Revision, in.ExpectedRevision); e != nil {
			return e
		}
		parts, e := frozenPartsTx(ctx, tx, id)
		if e != nil {
			return e
		}
		var size int64
		for i, p := range parts {
			if p.Part != i+1 {
				return fault("upload_incomplete", 409)
			}
			size += p.Size
		}
		if len(parts) != u.PartCount || size != u.Size {
			return fault("upload_incomplete", 409)
		}
		if e = s.queueBudgetTx(ctx, tx, scope.TenantID); e != nil {
			return e
		}
		var policy struct {
			MaxPixels int64 `json:"preview_max_pixels"`
			Edge      int   `json:"preview_edge"`
			MaxBytes  int64 `json:"preview_bytes"`
		}
		if e = json.Unmarshal(u.PolicyRaw, &policy); e != nil {
			return e
		}
		input := frozenInput{CheckPolicy: CheckPolicy{MaxPixels: policy.MaxPixels, Edge: policy.Edge, MaxBytes: policy.MaxBytes}, Reason: u.Reason, Schema: 1, UploadID: id, Host: u.Host, Intent: u.Intent, AttachmentID: u.AttachmentID, ExpectedRevision: u.ExpectedAttachmentRevision, Filename: u.Filename, DisplayName: u.DisplayName, Description: u.Description, Size: u.Size, SHA256: u.SHA256, Format: u.Format, MediaType: u.MediaType, Parts: parts, PolicyVersion: 1}
		job := uuid.NewString()
		now := s.now()
		_, e = tx.Exec(ctx, `INSERT INTO file_jobs(id,space_id,upload_id,host_id,originator_id,origin_tenant_membership_id,origin_project_membership_id,state,phase,created_at,observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,'queued','queued',$8,$8)`, job, st.SpaceID, id, st.HostID, a.ID, u.TenantMember, taskrecord.Null(u.ProjectMember), now)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO file_job_runs(space_id,job_id,generation,requested_by,executor_account_id,tenant_membership_id,project_membership_id,input_snapshot,input_hash,expected_attachment_revision,state,available_at,deadline_at,input_retain_until,restore_epoch) VALUES($1,$2,1,$3,$3,$4,$5,$6,$7,$8,'queued',$9,$10,$11,$12)`, st.SpaceID, job, a.ID, u.TenantMember, taskrecord.Null(u.ProjectMember), taskrecord.JSON(input), taskrecord.Hash(input), u.ExpectedAttachmentRevision, now, now.Add(s.options.QueueTimeout), now.Add(s.options.QueueTimeout+s.options.RetryInputTTL), st.Epoch)
		if e != nil {
			return e
		}
		var revision string
		e = tx.QueryRow(ctx, `UPDATE file_uploads SET state='submitted',revision=revision+1,submitted_at=$2,updated_at=$2 WHERE id=$1 RETURNING revision::text`, id, now).Scan(&revision)
		if e != nil {
			return e
		}
		ref, e := s.projectJobTx(ctx, tx, job, a.ID, "submitted", in.Reason)
		if e != nil {
			return e
		}
		generation, control := "1", "1"
		out = CommandResult{UploadID: &id, Revision: revision, State: "submitted", JobID: &job, TaskRef: &ref, JobGeneration: &generation, JobControlRevision: &control, Changed: true}
		return s.commitReceipt(ctx, tx, st, a, in.WriteMeta, "upload.complete", id, payload, out)
	})
	return out, e
}
func (s *Service) CancelUpload(ctx context.Context, a access.Actor, scope Scope, id string, in RevisionCommand) (CommandResult, error) {
	var out CommandResult
	if e := validateMeta(in.WriteMeta); e != nil {
		return out, e
	}
	payload := struct {
		ID    string
		Input RevisionCommand
	}{id, in}
	e := s.transact(ctx, a, scope, true, false, in.Epoch, func(tx pgx.Tx, st txState) error {
		var replay bool
		var e error
		out, replay, e = receiptTx(ctx, tx, st, a, in.WriteMeta, "upload.cancel", payload)
		if replay || e != nil {
			return e
		}
		if e = st.mutable(); e != nil {
			return e
		}
		u, e := uploadTx(ctx, tx, st.SpaceID, id, true)
		if e != nil {
			return e
		}
		if u.ActorID != a.ID && !st.Auth.ManagePermission {
			return access.ErrDenied
		}
		if e = sameVersion(u.Revision, in.ExpectedRevision); e != nil {
			return e
		}
		if u.State != "receiving" {
			return fault("task_control_required", 409)
		}
		var revision string
		e = tx.QueryRow(ctx, `UPDATE file_uploads SET state='cancelled',revision=revision+1,updated_at=$2 WHERE id=$1 RETURNING revision::text`, id, s.now()).Scan(&revision)
		if e != nil {
			return e
		}
		out = CommandResult{UploadID: &id, Revision: revision, State: "cancelled", Changed: true}
		return s.commitReceipt(ctx, tx, st, a, in.WriteMeta, "upload.cancel", id, payload, out)
	})
	return out, e
}
func (s *Service) Upload(ctx context.Context, a access.Actor, scope Scope, id string) (UploadDetail, error) {
	var out UploadDetail
	e := s.transact(ctx, a, scope, false, false, "", func(tx pgx.Tx, st txState) error {
		u, e := uploadTx(ctx, tx, st.SpaceID, id, false)
		if e != nil {
			return e
		}
		if u.ActorID != a.ID && !st.Auth.ManagePermission {
			return fault("file_not_found", 404)
		}
		rows, e := tx.Query(ctx, `SELECT part_no,expected_size::text,sha256,received_at FROM file_upload_parts WHERE upload_id=$1 AND state='received' ORDER BY part_no`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			var p PartReceipt
			if e = rows.Scan(&p.Part, &p.SizeBytes, &p.SHA256, &p.ReceivedAt); e != nil {
				rows.Close()
				return e
			}
			u.Parts = append(u.Parts, p)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		var j JobSummary
		e = tx.QueryRow(ctx, `SELECT id::text,generation::text,control_revision::text,state,phase FROM file_jobs WHERE upload_id=$1`, id).Scan(&j.ID, &j.Generation, &j.ControlRevision, &j.State, &j.Phase)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		if e == nil {
			j.TaskRef = jobRef(j.ID, j.Generation)
			u.Job = &j
		}
		e = tx.QueryRow(ctx, `SELECT unbound_until FROM file_records WHERE upload_id=$1`, id).Scan(&u.PreparedUntil)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		enabled := s.receiving(u, st, a) == nil
		u.AllowedActions = []Action{{Kind: "resume", Enabled: enabled, ReasonCode: st.Auth.Reason}, {Kind: "cancel", Enabled: enabled, RequiresReason: true}}
		if u.Job != nil && u.Job.State == "needs_action" {
			u.AllowedActions = append(u.AllowedActions, Action{Kind: "reconfirm", Enabled: st.Auth.CanManage && st.Mode == "online" && u.PreparedUntil != nil && s.now().Before(*u.PreparedUntil), RequiresReason: true})
		}
		out = u.UploadDetail
		return nil
	})
	return out, e
}
func jobRef(id, generation string) taskrecord.Ref {
	return taskrecord.Ref{SourceRef: taskrecord.SourceRef{Module: "files", Kind: "job", RootID: id}, Generation: generation}
}
func decodeInput(raw []byte) (frozenInput, error) {
	var v frozenInput
	e := json.Unmarshal(raw, &v)
	return v, e
}
func storageFailure(e error) bool {
	var v *Error
	return errors.As(e, &v) && (v.Code == "file_storage_unavailable" || v.Code == "file_worker_unavailable" || v.Code == "temporary_budget_exceeded")
}
