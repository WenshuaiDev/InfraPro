package derivedmonitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// QualifyArtifactVersionTx reuses the recursive input proof in a host's short
// publication transaction. There is no physical result scan or new snapshot.
func (s *Service) QualifyArtifactVersionTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, version string) error {
	if _, e := access.RequireActorProjectTx(ctx, tx, a, access.DerivedOperation(scope.Entry, "analysis.artifacts.detail"), scope.Tenant, scope.Project); e != nil {
		return e
	}
	var id string
	if e := tx.QueryRow(ctx, `SELECT a.id::text FROM analysis_artifact_versions v JOIN analysis_artifacts a ON a.tenant_id=v.tenant_id AND a.id=v.artifact_id WHERE v.tenant_id=$1 AND a.scope_key=$2 AND v.id=$3`, scope.Tenant, scope.key(), version).Scan(&id); e != nil {
		return notFound()
	}
	_, e := s.analysisArtifactProofTx(ctx, tx, a, scope, id, version)
	return e
}

// ResolveArtifactVersion returns an owning artifact only after current recursive
// input qualification. A fixed version reference is not an authorization token.
func (s *Service) ResolveArtifactVersion(ctx context.Context, a access.Actor, scope Scope, version string) (string, error) {
	var id string
	if !access.ValidID(version) {
		return "", notFound()
	}
	e := s.transact(ctx, a, scope, "analysis.artifacts.detail", false, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `SELECT a.id::text FROM analysis_artifact_versions v JOIN analysis_artifacts a ON a.tenant_id=v.tenant_id AND a.id=v.artifact_id WHERE v.tenant_id=$1 AND a.scope_key=$2 AND v.id=$3`, scope.Tenant, scope.key(), version).Scan(&id)
		if e != nil {
			return notFound()
		}
		_, e = s.analysisArtifactProofTx(ctx, tx, a, scope, id, version)
		return e
	})
	return id, e
}
