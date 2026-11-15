package files

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/jackc/pgx/v5"
)

func (s *Service) AuditMetadataTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id string) (string, error) {
	if a.ReadSubject() != nil {
		return "", access.ErrDenied
	}
	auth, e := s.hosts.ResolveTx(ctx, tx, a, scope, scope.Host(), Metadata)
	if e != nil {
		return "", e
	}
	if !auth.CanRead {
		return "", access.ErrDenied
	}
	var name string
	// Every candidate is scoped through its owner space. Uploads and preparations
	// retain their original personal/manager visibility instead of inheriting
	// attachment metadata access merely from their shared prefix.
	e = tx.QueryRow(ctx, `SELECT name FROM (
 SELECT a.display_name AS name FROM file_attachments a JOIN file_spaces s ON s.id=a.space_id WHERE s.tenant_id=$1 AND s.project_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid AND a.id::text=$3 AND (a.status='active' OR $5)
 UNION ALL SELECT u.filename FROM file_uploads u JOIN file_spaces s ON s.id=u.space_id WHERE s.tenant_id=$1 AND s.project_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid AND u.id::text=$3 AND (u.actor_id=$4::uuid OR $5)
 UNION ALL SELECT '文件检查与绑定' FROM file_jobs j JOIN file_spaces s ON s.id=j.space_id WHERE s.tenant_id=$1 AND s.project_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid AND j.id::text=$3 AND (j.originator_id=$4::uuid OR $5)
 ) candidates LIMIT 1`, scope.TenantID, scope.ProjectID, id, a.ID, auth.ManagePermission).Scan(&name)
	return name, e
}

// AuditTaskTx only qualifies the fixed execution link. Result bodies, private
// object locations and retry input inspection belong to the opened task page.
func (s *Service) AuditTaskTx(ctx context.Context, tx pgx.Tx, a access.Actor, ref taskrecord.SourceRef) (tasks.View, error) {
	if ref.Module != "files" || ref.Kind != "job" {
		return tasks.View{}, access.ErrNotFound
	}
	_, c, e := s.metadataStateTx(ctx, tx, a, ref.RootID)
	if e != nil {
		return tasks.View{}, e
	}
	_, label, _, e := tasks.ScopeTx(ctx, tx, a, c.Scope)
	if e != nil {
		return tasks.View{}, e
	}
	return tasks.Present(c, label), nil
}

// AuditReferenceTx resolves the concrete metadata kind and verifies immutable
// revision ownership after the existing personal/manager disclosure check.
func (s *Service) AuditReferenceTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, id, revision string) (string, error) {
	if _, e := s.AuditMetadataTx(ctx, tx, a, scope, id); e != nil {
		return "", e
	}
	var kind string
	e := tx.QueryRow(ctx, `SELECT kind FROM (SELECT 'file_attachment' AS kind,id FROM file_attachments UNION ALL SELECT 'file_upload',id FROM file_uploads UNION ALL SELECT 'file_job',id FROM file_jobs) v WHERE id=$1`, id).Scan(&kind)
	if e != nil {
		return "", e
	}
	if revision != "" {
		var valid bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_attachment_revisions WHERE id=$1 AND attachment_id=$2)`, revision, id).Scan(&valid)
		if e != nil {
			return "", e
		}
		if !valid {
			return "", access.ErrNotFound
		}
	}
	return kind, nil
}
