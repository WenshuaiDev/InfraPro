package files

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// EvidenceRevision is a fixed file revision, not a second storage or upload path.
type EvidenceRevision struct {
	Filename string `json:"filename"`
	SpaceID  string `json:"-"`
	HostID   string `json:"-"`
	FileID   string `json:"-"`
	Name     string `json:"name"`
	Number   string `json:"number"`
	SHA256   string `json:"sha256"`
}

// ResolveEvidenceTx uses the same profile and attachment permissions as the original file workspace.
func (s *Service) ResolveEvidenceTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, attachment, revision string) (EvidenceRevision, error) {
	var out EvidenceRevision
	if !validScope(scope) || !access.ValidID(attachment) || !access.ValidID(revision) || s.hosts == nil {
		return out, access.ErrDenied
	}
	_, mode, e := protocolTx(ctx, tx)
	if e != nil {
		return out, e
	}
	if mode == "reconciling" {
		return out, fault("file_protocol_unavailable", 503)
	}
	if _, e = s.hosts.ResolveTx(ctx, tx, a, scope, scope.Host(), Metadata); e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `SELECT r.space_id::text,r.host_id::text,r.file_id::text,r.display_name_snapshot,r.revision_no::text,f.sha256,f.original_name FROM file_attachment_revisions r JOIN file_spaces sp ON sp.id=r.space_id JOIN file_hosts h ON h.id=r.host_id JOIN file_attachments a ON a.id=r.attachment_id JOIN file_records f ON f.id=r.file_id WHERE sp.tenant_id=$1 AND sp.scope_kind=$2 AND sp.project_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid AND r.attachment_id=$4 AND r.id=$5 AND a.status='active' AND f.lifecycle='ready' AND f.integrity='ok' AND h.kind=$6 AND h.object_id=$7`, scope.TenantID, scope.Kind, scope.ProjectID, attachment, revision, scope.Host().Kind, scope.Host().ID).Scan(&out.SpaceID, &out.HostID, &out.FileID, &out.Name, &out.Number, &out.SHA256, &out.Filename)
	if e == pgx.ErrNoRows {
		return out, fault("file_not_found", 404)
	}
	return out, e
}
