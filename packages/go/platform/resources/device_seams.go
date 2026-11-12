package resources

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
	"slices"
)

// DeviceTarget contains only data authorized by the resource module. The caller
// already holds authorization, catalog and tenant coordination locks and must
// use this same transaction. Full grants are required, never ancestor context.
type DeviceTarget struct {
	Resource                     Resource
	Retired                      bool
	SourceManage                 bool
	LinkRevision, AccessRevision string
}

func (s *Service) DeviceTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, target Target) (DeviceTarget, error) {
	return s.deviceTargetTx(ctx, tx, a, scope, target)
}
func (s *Service) deviceTargetTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, target Target) (DeviceTarget, error) {
	out := DeviceTarget{}
	if !scope.valid() || tables[target.Kind] == "" || !access.ValidID(target.ID) {
		return out, notFound()
	}
	if target.Facility == "" {
		e := tx.QueryRow(ctx, `SELECT facility_id::text FROM engineering_resource_nodes WHERE tenant_id=$1 AND kind=$2 AND id=$3 AND deleted_at IS NULL`, scope.Tenant, target.Kind, target.ID).Scan(&target.Facility)
		if e == pgx.ErrNoRows {
			return out, notFound()
		}
		if e != nil {
			return out, e
		}
	}
	t := scoped(scope, target)
	if e := checkTarget(t); e != nil {
		return out, e
	}
	auth, e := s.authorizeTx(ctx, tx, a, t.operation(string(t.Kind), "detail"), scope)
	if e != nil {
		return out, e
	}
	f, e := lockFacility(ctx, tx, t, false, false)
	if e != nil {
		return out, e
	}
	if scope.Entry == "project" {
		g, link, e := projectGrantTx(ctx, tx, auth, t, true)
		if e != nil {
			return out, e
		}
		out.SourceManage = slices.Contains(g.Capabilities, "source_manage")
		out.LinkRevision = link.Revision
		out.AccessRevision = auth.AccessRevision
	}
	r, e := load(ctx, tx, t, false)
	if e != nil {
		return out, e
	}
	ancestors, e := ancestorsTx(ctx, tx, t, r, f)
	if e != nil {
		return out, e
	}
	out.Resource = r
	out.Retired = r.Status != "normal"
	for _, r := range ancestors {
		out.Retired = out.Retired || r.Status != "normal"
	}
	return out, nil
}

// ExternalImpact contains no target names or hidden counts. Hash covers complete
// retained relationships; reference checking applies to history as well.
type ExternalImpact struct {
	Hash                                           string
	Referenced, CurrentInstallation, CurrentSource bool
}
type ExternalReferences interface {
	ResourceImpactTx(context.Context, pgx.Tx, Scope, Target) (ExternalImpact, error)
}

func (s *Service) WithExternalReferences(r ExternalReferences) *Service { s.external = r; return s }
