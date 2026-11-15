package files

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"time"
)

const revisionColumns = `r.id::text,r.revision_no::text,r.file_id::text,f.original_name,f.format,f.media_type,f.size_bytes::text,f.sha256,CASE WHEN f.lifecycle<>'ready' THEN f.lifecycle WHEN f.integrity<>'ok' THEN f.integrity ELSE 'available' END,f.preview_state,f.preview_reason,r.operation,r.reason,r.actor_id::text,r.created_at,r.previous_revision_id::text,r.reverted_from_revision_id::text,r.job_id::text,r.job_generation::text`

func revisionScan(v *AttachmentRevision) []any {
	return []any{&v.ID, &v.RevisionNo, &v.FileID, &v.OriginalName, &v.Format, &v.MediaType, &v.SizeBytes, &v.SHA256, &v.FileState, &v.PreviewState, &v.PreviewReason, &v.Operation, &v.Reason, &v.CreatedBy, &v.CreatedAt, &v.PreviousRevisionID, &v.RevertedFromRevisionID, &v.JobID, &v.JobGeneration}
}
func attachmentTx(ctx context.Context, tx pgx.Tx, st txState, id string, lock bool) (AttachmentDetail, error) {
	var v AttachmentDetail
	if st.SpaceID == "" || !access.ValidID(id) {
		return v, fault("file_not_found", 404)
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF a"
	}
	args := []any{&v.ID, &v.Host.Kind, &v.Host.ID, &v.DisplayName, &v.Description, &v.Status, &v.Revision, &v.CreatedBy, &v.UpdatedBy, &v.CreatedAt, &v.UpdatedAt, &v.PurgeAfter}
	args = append(args, revisionScan(&v.CurrentRevision)...)
	e := tx.QueryRow(ctx, `SELECT a.id::text,h.kind,h.object_id::text,a.display_name,a.description,a.status,a.revision::text,a.created_by::text,a.updated_by::text,a.created_at,a.updated_at,a.purge_after,`+revisionColumns+` FROM file_attachments a JOIN file_hosts h ON h.id=a.host_id JOIN file_attachment_revisions r ON r.id=a.current_revision_id JOIN file_records f ON f.id=r.file_id WHERE a.space_id=$1 AND a.host_id=$2 AND a.id=$3`+suffix, st.SpaceID, st.HostID, id).Scan(args...)
	if e != nil {
		return v, notFound(e)
	}
	if v.Status != "active" && !st.Auth.ManagePermission {
		return AttachmentDetail{}, fault("file_not_found", 404)
	}
	return v, nil
}
func (s *Service) attachmentActions(v *AttachmentDetail, st txState) {
	readable := v.Status == "active" && v.CurrentRevision.FileState == "available" && st.Auth.CanDownload && st.Mode != "reconciling"
	editable := st.Auth.CanManage && st.Mode == "online"
	v.AllowedActions = []Action{{Kind: "download", Enabled: readable, ReasonCode: "download_unavailable"}, {Kind: "preview", Enabled: readable && v.CurrentRevision.PreviewState == "available", ReasonCode: v.CurrentRevision.PreviewReason}}
	for _, kind := range []string{"rename", "replace", "revert", "trash"} {
		v.AllowedActions = append(v.AllowedActions, Action{Kind: kind, Enabled: editable && v.Status == "active", ReasonCode: st.Auth.Reason, RequiresReason: true})
	}
	v.AllowedActions = append(v.AllowedActions, Action{Kind: "restore", Enabled: editable && v.Status == "trashed" && v.PurgeAfter != nil && s.now().Before(*v.PurgeAfter), RequiresReason: true})
	for i := range v.AllowedActions {
		if v.AllowedActions[i].Enabled {
			v.AllowedActions[i].ReasonCode = ""
		} else if v.AllowedActions[i].ReasonCode == "" {
			v.AllowedActions[i].ReasonCode = "action_unavailable"
		}
	}
}
func (s *Service) Attachment(ctx context.Context, a access.Actor, scope Scope, id string) (AttachmentDetail, error) {
	var v AttachmentDetail
	e := s.transact(ctx, a, scope, false, false, "", func(tx pgx.Tx, st txState) error {
		var e error
		v, e = attachmentTx(ctx, tx, st, id, false)
		if e == nil {
			s.attachmentActions(&v, st)
		}
		return e
	})
	return v, e
}
func (s *Service) ChangeAttachment(ctx context.Context, a access.Actor, scope Scope, id string, in AttachmentCommand) (CommandResult, error) {
	var out CommandResult
	if e := validateMeta(in.WriteMeta); e != nil {
		return out, e
	}
	if in.Action != "update" && in.Action != "revert" && in.Action != "trash" && in.Action != "restore" {
		return out, fault("invalid_request", 400)
	}
	if in.Action == "update" {
		if in.DisplayName == nil && in.Description == nil || in.DisplayName != nil && !validText(*in.DisplayName, 1, 200) || in.Description != nil && !validText(*in.Description, 0, 1000) || in.RevisionID != "" {
			return out, fault("invalid_request", 400)
		}
	} else if in.DisplayName != nil || in.Description != nil {
		return out, fault("invalid_request", 400)
	}
	if in.Action == "revert" && !access.ValidID(in.RevisionID) {
		return out, fault("invalid_request", 400)
	}
	operation := "attachment." + in.Action
	payload := struct {
		ID, Action string
		Input      AttachmentCommand
	}{id, in.Action, in}
	e := s.transact(ctx, a, scope, true, false, in.Epoch, func(tx pgx.Tx, st txState) error {
		var replay bool
		var e error
		out, replay, e = receiptTx(ctx, tx, st, a, in.WriteMeta, operation, payload)
		if replay || e != nil {
			return e
		}
		if e = st.mutable(); e != nil {
			return e
		}
		old, e := attachmentTx(ctx, tx, st, id, true)
		if e != nil {
			return e
		}
		if e = sameVersion(old.Revision, in.ExpectedRevision); e != nil {
			return e
		}
		if old.Status == "purged" {
			return fault("attachment_purged", 410)
		}
		now := s.now()
		var revisionID string
		switch in.Action {
		case "update":
			if old.Status != "active" {
				return fault("attachment_revision_changed", 409)
			}
			_, e = tx.Exec(ctx, `UPDATE file_attachments SET display_name=COALESCE($2,display_name),description=COALESCE($3,description) WHERE id=$1`, id, in.DisplayName, in.Description)
		case "trash":
			if old.Status != "active" {
				return fault("attachment_revision_changed", 409)
			}
			_, e = tx.Exec(ctx, `UPDATE file_attachments SET status='trashed',trashed_by=$2,trashed_at=$3,purge_after=$4 WHERE id=$1`, id, a.ID, now, now.Add(s.options.TrashTTL))
		case "restore":
			if old.Status != "trashed" {
				return fault("attachment_revision_changed", 409)
			}
			if old.PurgeAfter == nil || !now.Before(*old.PurgeAfter) {
				return fault("attachment_purged", 410)
			}
			rows, err := tx.Query(ctx, `SELECT DISTINCT file_id::text FROM file_attachment_revisions WHERE attachment_id=$1 ORDER BY file_id::text`, id)
			if err != nil {
				return err
			}
			ids := []string{}
			for rows.Next() {
				var file string
				if err = rows.Scan(&file); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, file)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			for _, file := range ids {
				if err = s.lockReadyTx(ctx, tx, st.SpaceID, st.HostID, file); err != nil {
					return err
				}
			}
			_, e = tx.Exec(ctx, `UPDATE file_attachments SET status='active',trashed_by=NULL,trashed_at=NULL,purge_after=NULL WHERE id=$1`, id)
		case "revert":
			if old.Status != "active" {
				return fault("attachment_revision_changed", 409)
			}
			var file string
			e = tx.QueryRow(ctx, `SELECT file_id::text FROM file_attachment_revisions WHERE space_id=$1 AND attachment_id=$2 AND id=$3`, st.SpaceID, id, in.RevisionID).Scan(&file)
			if e != nil {
				return notFound(e)
			}
			if e = s.lockReadyTx(ctx, tx, st.SpaceID, st.HostID, file); e != nil {
				return e
			}
			revisionID = uuid.NewString()
			var no int64
			e = tx.QueryRow(ctx, `SELECT next_revision_no FROM file_attachments WHERE id=$1`, id).Scan(&no)
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `INSERT INTO file_attachment_revisions(id,space_id,host_id,attachment_id,revision_no,file_id,operation,previous_revision_id,reverted_from_revision_id,display_name_snapshot,reason,actor_id,created_at) VALUES($1,$2,$3,$4,$5,$6,'revert',$7,$8,$9,$10,$11,$12)`, revisionID, st.SpaceID, st.HostID, id, no, file, old.CurrentRevision.ID, in.RevisionID, old.DisplayName, in.Reason, a.ID, now)
			if e != nil {
				return e
			}
			if e = s.ProtectTx(ctx, tx, Protection{SpaceID: st.SpaceID, HostID: st.HostID, FileID: file, Module: "files", OwnerKind: "revision", OwnerID: revisionID, OwnerRevision: number(no), Kind: "attachment_revision"}); e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `UPDATE file_attachments SET current_revision_id=$2,next_revision_no=next_revision_no+1 WHERE id=$1`, id, revisionID)
		}
		if e != nil {
			return e
		}
		var revision, state string
		e = tx.QueryRow(ctx, `UPDATE file_attachments SET revision=revision+1,updated_by=$2,updated_at=$3 WHERE id=$1 RETURNING revision::text,status`, id, a.ID, now).Scan(&revision, &state)
		if e != nil {
			return e
		}
		out = CommandResult{AttachmentID: &id, AttachmentRevisionID: pointer(revisionID), Revision: revision, State: state, Changed: true}
		in.WriteMeta.auditBefore = old
		updated, e := attachmentTx(ctx, tx, st, id, false)
		if e != nil {
			return e
		}
		in.WriteMeta.auditAfter = updated
		return s.commitReceipt(ctx, tx, st, a, in.WriteMeta, operation, id, payload, out)
	})
	return out, e
}
func (s *Service) lockReadyTx(ctx context.Context, tx pgx.Tx, space, host, file string) error {
	var lifecycle, integrity, objectState string
	e := tx.QueryRow(ctx, `SELECT f.lifecycle,f.integrity,o.state FROM file_records f JOIN file_objects o ON o.id=f.source_object_id WHERE f.space_id=$1 AND f.host_id=$2 AND f.id=$3 FOR UPDATE OF f`, space, host, file).Scan(&lifecycle, &integrity, &objectState)
	if e != nil {
		return notFound(e)
	}
	if lifecycle != "ready" {
		return fault("file_reclaiming", 409)
	}
	if integrity != "ok" || objectState != "available" {
		if integrity == "missing" || objectState == "missing" {
			return fault("file_missing", 503)
		}
		return fault("file_corrupt", 503)
	}
	return nil
}

// ProtectTx is intentionally finite: there is no registered external consumer
// in this release. A future consumer must add a real owner resolver and tests.
func (s *Service) ProtectTx(ctx context.Context, tx pgx.Tx, in Protection) error {
	if in.Module != "files" || in.Kind != "attachment_revision" && in.Kind != "prepared_input" || !access.ValidID(in.OwnerID) || !validText(in.OwnerRevision, 1, 100) {
		return access.ErrDenied
	}
	if e := s.lockReadyTx(ctx, tx, in.SpaceID, in.HostID, in.FileID); e != nil {
		return e
	}
	var valid bool
	var e error
	if in.Kind == "attachment_revision" && in.OwnerKind == "revision" {
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_attachment_revisions WHERE space_id=$1 AND host_id=$2 AND file_id=$3 AND id=$4 AND revision_no::text=$5)`, in.SpaceID, in.HostID, in.FileID, in.OwnerID, in.OwnerRevision).Scan(&valid)
	} else if in.Kind == "prepared_input" && in.OwnerKind == "upload" {
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_records WHERE space_id=$1 AND host_id=$2 AND id=$3 AND upload_id=$4 AND unbound_until=$5 AND unbound_until>$6)`, in.SpaceID, in.HostID, in.FileID, in.OwnerID, in.ExpiresAt, s.now()).Scan(&valid)
	}
	if e != nil {
		return e
	}
	if !valid {
		return access.ErrDenied
	}
	var released, expires *time.Time
	e = tx.QueryRow(ctx, `SELECT released_at,expires_at FROM file_protection_refs WHERE file_id=$1 AND owner_module=$2 AND owner_kind=$3 AND owner_id=$4 AND owner_revision=$5 AND kind=$6`, in.FileID, in.Module, in.OwnerKind, in.OwnerID, in.OwnerRevision, in.Kind).Scan(&released, &expires)
	if e == nil {
		if released != nil || (expires == nil) != (in.ExpiresAt == nil) || expires != nil && !expires.Equal(*in.ExpiresAt) {
			return fault("request_conflict", 409)
		}
		return nil
	}
	if e != pgx.ErrNoRows {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO file_protection_refs(space_id,file_id,owner_module,owner_kind,owner_id,owner_revision,kind,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, in.SpaceID, in.FileID, in.Module, in.OwnerKind, in.OwnerID, in.OwnerRevision, in.Kind, in.ExpiresAt)
	return e
}
func (s *Service) ReleaseTx(ctx context.Context, tx pgx.Tx, in Release) error {
	if in.Module != "files" || !validText(in.Reason, 1, 500) {
		return access.ErrDenied
	}
	if _, e := tx.Exec(ctx, `SELECT id FROM file_records WHERE space_id=$1 AND host_id=$2 AND id=$3 FOR UPDATE`, in.SpaceID, in.HostID, in.FileID); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `UPDATE file_protection_refs SET released_at=$7,release_reason=$8 WHERE space_id=$1 AND file_id=$2 AND owner_module=$3 AND owner_kind=$4 AND owner_id=$5 AND owner_revision=$6 AND kind=$9 AND released_at IS NULL`, in.SpaceID, in.FileID, in.Module, in.OwnerKind, in.OwnerID, in.OwnerRevision, s.now(), in.Reason, in.Kind)
	return e
}
