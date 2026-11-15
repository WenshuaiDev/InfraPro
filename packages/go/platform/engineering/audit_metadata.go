package engineering

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// AuditMetadataTx reuses definition ownership and detail authorization; audit
// history never grants access to an unpublished definition.
func (s *Service) AuditMetadataTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, id string) (string, error) {
	var kind Kind
	if e := tx.QueryRow(ctx, `SELECT kind FROM engineering_definitions WHERE id::text=$1 AND tenant_id IS NOT DISTINCT FROM NULLIF($2,'')::uuid AND deleted_at IS NULL`, id, tenant).Scan(&kind); e != nil {
		return "", e
	}
	scope := Scope{Tenant: tenant, Kind: kind}
	if e := access.RequireScopedTx(ctx, tx, a.ID, scope.operation("detail"), tenant); e != nil {
		return "", e
	}
	d, e := definitionTx(ctx, tx, scope, id, false)
	return d.Code, e
}

func (s *Service) AuditKindTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, id string) (string, error) {
	if _, e := s.AuditMetadataTx(ctx, tx, a, tenant, id); e != nil {
		return "", e
	}
	var kind string
	e := tx.QueryRow(ctx, `SELECT kind FROM engineering_definitions WHERE id=$1`, id).Scan(&kind)
	return kind, e
}
