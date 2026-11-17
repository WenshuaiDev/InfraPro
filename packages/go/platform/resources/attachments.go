package resources

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/jackc/pgx/v5"
)

type AttachmentRef struct {
	AttachmentID string `json:"attachment_id"`
	RevisionID   string `json:"attachment_revision_id"`
}

func (s *Service) WithFiles(v *files.Service) *Service { s.files = v; return s }
func fileScope(scope Scope) files.Scope {
	return files.Scope{Kind: scope.Entry, TenantID: scope.Tenant, ProjectID: scope.Project}
}
func (s *Service) resolveAttachment(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, v *AttachmentRef) (files.EvidenceRevision, error) {
	if s.files == nil || v == nil {
		return files.EvidenceRevision{}, problem("unavailable", "工程附件服务不可用。", 503)
	}
	return s.files.ResolveEvidenceTx(ctx, tx, a, fileScope(scope), v.AttachmentID, v.RevisionID)
}
func (s *Service) recordAttachment(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, t scopedTarget, in StructureRecordInput) error {
	if in.Action == "end_attachment" {
		tag, e := tx.Exec(ctx, `UPDATE engineering_resource_attachments SET ended_at=clock_timestamp(),ended_reason=$6 WHERE tenant_id=$1 AND facility_id=$2 AND resource_kind=$3 AND resource_id=$4 AND id=$5 AND ended_at IS NULL`, t.Tenant, t.Facility, t.Kind, t.ID, in.RecordID, in.Reason)
		if e == nil && tag.RowsAffected() != 1 {
			return notFound()
		}
		return e
	}
	revision, e := s.resolveAttachment(ctx, tx, a, scope, in.Attachment)
	if e != nil {
		return e
	}
	var id string
	e = tx.QueryRow(ctx, `INSERT INTO engineering_resource_attachments(tenant_id,facility_id,resource_kind,resource_id,scope_kind,project_id,attachment_id,attachment_revision_id,purpose,created_by,reason) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8,$9,$10,$11) RETURNING id::text`, t.Tenant, t.Facility, t.Kind, t.ID, scope.Entry, scope.Project, in.Attachment.AttachmentID, in.Attachment.RevisionID, in.Purpose, a.ID, in.Reason).Scan(&id)
	if e != nil {
		return e
	}
	return s.files.ProtectTx(ctx, tx, files.Protection{SpaceID: revision.SpaceID, HostID: revision.HostID, FileID: revision.FileID, Module: "engineering_structure", OwnerKind: "attachment", OwnerID: id, OwnerRevision: "1", Kind: "consumer"})
}
func (s *Service) attachmentRecordTx(ctx context.Context, tx pgx.Tx, a access.Actor, auth authorization, t scopedTarget, id string) (AttachmentRef, error) {
	var ref AttachmentRef
	var scope Scope
	var ended bool
	e := tx.QueryRow(ctx, `SELECT scope_kind,COALESCE(project_id::text,''),attachment_id::text,attachment_revision_id::text,ended_at IS NOT NULL FROM engineering_resource_attachments WHERE tenant_id=$1 AND facility_id=$2 AND resource_kind=$3 AND resource_id=$4 AND id=$5`, t.Tenant, t.Facility, t.Kind, t.ID, id).Scan(&scope.Entry, &scope.Project, &ref.AttachmentID, &ref.RevisionID, &ended)
	if e == pgx.ErrNoRows {
		return ref, notFound()
	}
	if e != nil {
		return ref, e
	}
	if scope.Entry != auth.Entry || scope.Project != auth.Project || ended && auth.Entry == "project" && !auth.can(Facility, "changes") {
		return ref, notFound()
	}
	_, e = s.resolveAttachment(ctx, tx, a, auth.Scope, &ref)
	return ref, e
}
func (s *Service) filterAttachments(ctx context.Context, tx pgx.Tx, a access.Actor, auth authorization, t scopedTarget, items []json.RawMessage) ([]json.RawMessage, error) {
	out := []json.RawMessage{}
	for _, raw := range items {
		var entry map[string]any
		if e := json.Unmarshal(raw, &entry); e != nil {
			return nil, e
		}
		id, _ := entry["id"].(string)
		ref, e := s.attachmentRecordTx(ctx, tx, a, auth, t, id)
		if e != nil {
			var f *files.Error
			var r *Error
			if errors.Is(e, access.ErrDenied) || errors.As(e, &f) && f.Status < 500 || errors.As(e, &r) && r.Status == 404 {
				continue
			}
			return nil, e
		}
		info, e := s.resolveAttachment(ctx, tx, a, auth.Scope, &ref)
		if e != nil {
			return nil, e
		}
		entry["name"], entry["number"], entry["sha256"], entry["filename"] = info.Name, info.Number, info.SHA256, info.Filename
		b, e := json.Marshal(entry)
		if e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, nil
}

// OpenAttachment guards the file's original host and this resource on every delivery chunk.
func (s *Service) OpenAttachment(ctx context.Context, a access.Actor, scope Scope, target Target, id, variant string) (*files.GuardedRead, error) {
	if s.files == nil || !access.ValidID(id) {
		return nil, notFound()
	}
	t := scoped(scope, target)
	if e := checkTarget(t); e != nil {
		return nil, e
	}
	var ref AttachmentRef
	guard := func(ctx context.Context, tx pgx.Tx) error {
		auth, e := s.authorizeTx(ctx, tx, a, scope.operation(string(t.Kind), "structure-details"), scope)
		if e != nil {
			return e
		}
		if _, e = load(ctx, tx, t, false); e != nil {
			return e
		}
		if auth.Entry == "project" {
			if _, _, e = projectGrantTx(ctx, tx, auth, t, true); e != nil {
				return e
			}
		}
		current, e := s.attachmentRecordTx(ctx, tx, a, auth, t, id)
		if e != nil {
			return e
		}
		if ref.RevisionID != "" && ref != current {
			return notFound()
		}
		if ref.RevisionID == "" {
			ref = current
		}
		return nil
	}
	e := s.transact(ctx, a, scope.operation(string(t.Kind), "structure-details"), scope, false, func(tx pgx.Tx, _ authorization) error { return guard(ctx, tx) })
	if e != nil {
		return nil, e
	}
	return s.files.OpenReadWithGuard(ctx, a, fileScope(scope), ref.AttachmentID, ref.RevisionID, variant, guard)
}
