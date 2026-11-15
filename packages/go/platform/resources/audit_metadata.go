package resources

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s *Service) AuditMetadataTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, kind, id string) (string, error) {
	if strings.HasPrefix(kind, "project_resource_") || kind == "project_facility_link" {
		if scope.Entry != "tenant" {
			return "", access.ErrDenied
		}
		if _, e := s.authorizeTx(ctx, tx, a, access.ResourceAccessOperation("access", "detail"), scope); e != nil {
			return "", e
		}
		return "项目资源授权", nil
	}
	target := Target{Kind: Kind(strings.TrimPrefix(kind, "engineering_")), ID: id}
	v, e := s.deviceTargetTx(ctx, tx, a, scope, target)
	return v.Resource.Name, e
}

func (s *Service) AuditGrantLocationTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, kind, id string) (map[string]string, error) {
	if _, e := s.AuditMetadataTx(ctx, tx, a, scope, kind, id); e != nil {
		return nil, e
	}
	project, facility := id, ""
	if kind != "project_resource_policy" {
		if e := tx.QueryRow(ctx, `SELECT project_id::text,facility_id::text FROM project_facility_links WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&project, &facility); e != nil {
			return nil, e
		}
	}
	var exists bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE tenant_id=$1 AND id=$2)`, scope.Tenant, project).Scan(&exists); e != nil {
		return nil, e
	}
	if !exists {
		return nil, access.ErrNotFound
	}
	return map[string]string{"project": project, "facility": facility}, nil
}

func (s *Service) AuditLocationTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, kind, id string) (map[string]string, error) {
	v, e := s.deviceTargetTx(ctx, tx, a, scope, Target{Kind: Kind(strings.TrimPrefix(kind, "engineering_")), ID: id})
	if e != nil {
		return nil, e
	}
	r := v.Resource
	facility := r.FacilityID
	if r.Kind == "facility" {
		facility = r.ID
	}
	return map[string]string{"facility": facility, "point": r.PointID}, nil
}
